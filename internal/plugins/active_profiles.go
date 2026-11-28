package plugins

import (
	"assistente/internal/logging"
	"assistente/internal/pluginpackage"
	"assistente/internal/providers"
	"context"
	"encoding/json"
	"errors"

	"time"

	"assistente/internal/allowlist"
	"assistente/internal/authorization"
	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/profiles"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (i *Inventory) SetProfileActive(ctx context.Context, id, digest, key string, active bool, admit commandsecurity.Admission) error {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if admit == nil {
		return database.ErrUserScopeRequired
	}
	var row database.PluginProfileActivation
	var requirements map[string]string
	if active {
		snap, err := i.installedSnapshot(ctx, id, digest)
		if err != nil {
			return err
		}
		components := DiscoverComponents(snap)
		for _, c := range components {
			if c.Kind == "profile" && c.Key == key && c.Reason == "" {
				row = database.PluginProfileActivation{InstallationID: id, Digest: digest, ComponentKey: key, Slug: profiles.PluginPrefix + uuid.NewString(), Definition: snap.Files[c.Entry]}
				requirements = packageProviderRequirements(snap, components)
				break
			}
		}
		if row.Slug == "" {
			return ErrComponentUnavailable
		}
	}
	publish, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return database.WithSQLiteAdmittedCommit(publish, i.db, "plugins.profile.activation", func(tx *gorm.DB) error {
		var installed database.PluginInstallation
		if err := tx.Where("id=? AND user_id=? AND digest=?", id, owner, digest).Take(&installed).Error; err != nil {
			return err
		}
		if !active {
			return tx.Where("installation_id=? AND component_key=?", id, key).Delete(&database.PluginProfileActivation{}).Error
		}
		if installed.Disabled {
			return ErrInstallationDisabled
		}
		if _, err := resolveProfile(ctx, tx, row.Definition, requirements, id, digest); err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "installation_id"}, {Name: "component_key"}}, DoNothing: true}).Create(&row).Error
	}, admit)
}

func (i *Inventory) profileQuery(ctx context.Context) (*gorm.DB, error) {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	return i.db.WithContext(ctx).Model(&database.PluginProfileActivation{}).Joins("JOIN plugin_installations p ON p.id=plugin_profile_activations.installation_id AND p.digest=plugin_profile_activations.digest").Where("p.user_id=? AND p.disabled=?", owner, false), nil
}

func (i *Inventory) GetPluginProfile(ctx context.Context, slug string) (*profiles.Profile, error) {
	q, err := i.profileQuery(ctx)
	if err != nil {
		return nil, err
	}
	var row database.PluginProfileActivation
	if err := q.Where("plugin_profile_activations.slug=?", slug).Take(&row).Error; err != nil {
		return nil, err
	}
	return i.resolveActivation(ctx, row, make(map[string]map[string]string))
}

func (i *Inventory) ListPluginProfiles(ctx context.Context) ([]profiles.ProfileInfo, error) {
	q, err := i.profileQuery(ctx)
	if err != nil {
		return nil, err
	}
	var rows []database.PluginProfileActivation
	if err := q.Order("plugin_profile_activations.slug").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]profiles.ProfileInfo, 0, len(rows))
	cached := make(map[string]map[string]string)
	for _, row := range rows {
		p, err := i.resolveActivation(ctx, row, cached)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return nil, errors.Join(ErrComponentUnavailable, err)
		}
		result = append(result, profiles.ProfileInfo{Name: p.Name, Slug: row.Slug, Description: p.Description, Icon: p.Icon, Source: "plugin"})
	}
	return result, nil
}

// An invalid supplied definition remains an unsatisfied requirement even when its file is missing.
func packageProviderRequirements(snap *pluginpackage.LocalSnapshot, components []Component) map[string]string {
	required := map[string]string{}
	for _, c := range components {
		if c.Kind == "provider" {
			digest, _ := providers.PackageDefinitionDigest(snap.Files[c.Entry], c.Slug)
			required[c.Slug] = digest
		}
	}
	return required
}

func (i *Inventory) resolveActivation(ctx context.Context, row database.PluginProfileActivation, cached map[string]map[string]string) (*profiles.Profile, error) {
	required, ok := cached[row.Digest]
	if !ok {
		var revision database.PluginRevision
		if err := i.db.WithContext(ctx).Select("provider_requirements").Where("installation_id=? AND digest=?", row.InstallationID, row.Digest).Take(&revision).Error; err != nil {
			return nil, err
		}
		if revision.ProviderRequirements == nil {
			snap, err := i.installedSnapshot(ctx, row.InstallationID, row.Digest)
			if err != nil {
				return nil, err
			}
			required = packageProviderRequirements(snap, nativeComponents(snap))
			raw, err := json.Marshal(required)
			if err != nil {
				return nil, err
			}
			// Somente metadados derivados. Uma revisão substituída nunca recebe o índice da sucessora.
			if err := database.WithSQLiteImmediateTransaction(ctx, i.db, "plugins.profile.index", func(tx *gorm.DB) error {
				return tx.Model(&database.PluginRevision{}).Where("installation_id=? AND digest=? AND provider_requirements IS NULL", row.InstallationID, row.Digest).UpdateColumn("provider_requirements", raw).Error
			}); err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				logging.Warnf(ctx, "plugins", "reason=provider_requirement_index_write_failed")
			}
		} else if err := json.Unmarshal(revision.ProviderRequirements, &required); err != nil {
			return nil, err
		}
		cached[row.Digest] = required
	}
	return resolveProfile(ctx, i.db, row.Definition, required, row.InstallationID, row.Digest)
}

func resolveProfile(ctx context.Context, db *gorm.DB, raw []byte, required map[string]string, installationID, digest string) (*profiles.Profile, error) {
	p, err := readPackageProfile(raw)
	if err != nil {
		return nil, err
	}
	if slug := p.Chat.CommandAllowlist; slug != "" {
		policies := allowlist.NewManager(allowlist.Options{Repository: authorization.NewRepository(func() *gorm.DB { return db })})
		ok, err := policies.HasProfilePolicy(ctx, slug)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, missingProfileAllowlist(slug)
		}
	}
	repo := database.NewProviderRepository(db)
	chatSlug := p.Chat.LLMProvider
	// Uma conexão ChatGPT só pode suprir a dependência de texto.
	chatOnly := chatSlug != "" && chatSlug != p.Voice.Assistant.LLMProviderID && chatSlug != p.Voice.User.LLMProviderID && chatSlug != p.Voice.System.LLMProviderID && chatSlug != p.Input.LLMProviderID
	resolved := make(map[string]string)
	err = p.MapProviderReferences(func(slug string) (string, error) {
		if id, ok := resolved[slug]; ok {
			return id, nil
		}
		_, bundled := required[slug]
		row, err := repo.GetBySlug(ctx, slug)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", missingProfileProvider{slug: slug, chatOnly: !bundled && chatOnly && slug == chatSlug}
		}
		if err != nil {
			return "", err
		}
		if expected, ok := required[slug]; ok {
			current, err := providers.NativeDefinitionDigest(row)
			if err != nil || current != expected {
				return "", unavailableProfileProvider(slug)
			}
			var count int64
			if err := db.WithContext(ctx).Model(&database.PluginProviderBinding{}).Where("installation_id=? AND component_key=? AND provider_id=? AND (consumer_only=? OR digest=?)", installationID, "providers/"+slug, row.ID, false, digest).Count(&count).Error; err != nil {
				return "", err
			}
			if count == 0 {
				return "", unavailableProfileProvider(slug)
			}
		}
		resolved[slug] = row.ID
		return row.ID, nil
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

type unavailableProfileProvider string

func (e unavailableProfileProvider) Error() string {
	return "PLUGIN_PROFILE_PROVIDER_UNSATISFIED: " + string(e)
}
func (e unavailableProfileProvider) Unwrap() error { return gorm.ErrRecordNotFound }

type missingProfileProvider struct {
	slug     string
	chatOnly bool
}

func (e missingProfileProvider) Error() string {
	return "PLUGIN_PROFILE_PROVIDER_MISSING: " + e.slug
}
func (e missingProfileProvider) Unwrap() error { return gorm.ErrRecordNotFound }

type missingProfileAllowlist string

func (e missingProfileAllowlist) Error() string {
	return "PLUGIN_PROFILE_ALLOWLIST_MISSING: " + string(e)
}
func (e missingProfileAllowlist) Unwrap() error { return gorm.ErrRecordNotFound }

// DescribeProfileDependencies resolves the displayed revision without activating a profile.
func (i *Inventory) DescribeProfileDependencies(ctx context.Context, components []Component, snapshot *pluginpackage.LocalSnapshot, id string) error {
	required := packageProviderRequirements(snapshot, components)
	for n := range components {
		if components[n].Kind == "profile" && components[n].Reason == "" {
			definition := snapshot.Files[components[n].Entry]
			_, err := resolveProfile(ctx, i.db, definition, required, id, snapshot.Digest)
			if err != nil {
				var missing missingProfileProvider
				var pending unavailableProfileProvider
				var policy missingProfileAllowlist
				switch {
				case errors.As(err, &missing):
					components[n].MissingProvider = missing.slug
					components[n].CanCreateChatGPT = missing.chatOnly
				case errors.As(err, &pending):
					components[n].PendingProvider = string(pending)
				case errors.As(err, &policy):
					components[n].MissingAllowlist = string(policy)
				default:
					return err
				}
			}
		}
	}
	return nil
}
