package plugins

import (
	"context"
	"errors"

	"time"

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
	if active {
		snap, err := i.installedSnapshot(ctx, id, digest)
		if err != nil {
			return err
		}
		for _, c := range DiscoverComponents(snap) {
			if c.Kind == "profile" && c.Key == key && c.Reason == "" {
				row = database.PluginProfileActivation{InstallationID: id, Digest: digest, ComponentKey: key, Slug: profiles.PluginPrefix + uuid.NewString(), Definition: snap.Files[c.Entry]}
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
		if _, err := resolveProfile(ctx, tx, row.Definition); err != nil {
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
	return resolveProfile(ctx, i.db, row.Definition)
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
	for _, row := range rows {
		p, err := resolveProfile(ctx, i.db, row.Definition)
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

func resolveProfile(ctx context.Context, db *gorm.DB, raw []byte) (*profiles.Profile, error) {
	p, err := readPackageProfile(raw)
	if err != nil {
		return nil, err
	}
	repo := database.NewProviderRepository(db)
	err = p.MapProviderReferences(func(slug string) (string, error) {
		row, err := repo.GetBySlug(ctx, slug)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", missingProfileProvider(slug)
		}
		if err != nil {
			return "", err
		}
		return row.ID, nil
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

type missingProfileProvider string

func (e missingProfileProvider) Error() string {
	return "PLUGIN_PROFILE_PROVIDER_MISSING: " + string(e)
}
func (e missingProfileProvider) Unwrap() error { return gorm.ErrRecordNotFound }
