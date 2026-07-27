package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"assistente/internal/configdir"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/logging"
	"assistente/internal/portability"
)

// LegacyChannelItem is one filesystem channel config ready for DB import.
type LegacyChannelItem struct {
	Slug   string
	Config *ChannelConfig
	Path   string
}

// LegacyConfigSource returns a read-only source over channels/*.json across basePaths.
func LegacyConfigSource() portability.LegacyImportSource {
	return legacyConfigSource{}
}

type legacyConfigSource struct{}

func (legacyConfigSource) ListLegacyImportFiles(context.Context) ([]portability.LegacyImportFile, error) {
	seen := make(map[string]struct{})
	var out []portability.LegacyImportFile
	for _, base := range configdir.GetBasePaths() {
		dir := filepath.Join(base, channelsSubdir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !strings.HasSuffix(strings.ToLower(name), ".json") {
				continue
			}
			slug := strings.TrimSuffix(name, filepath.Ext(name))
			if _, ok := seen[slug]; ok {
				continue
			}
			seen[slug] = struct{}{}
			out = append(out, portability.LegacyImportFile{
				Name:     slug,
				Filename: name,
				Path:     filepath.Join(dir, name),
				Source:   "channels",
			})
		}
	}
	return out, nil
}

func (legacyConfigSource) ReadLegacyImportFile(_ context.Context, filename string) ([]byte, error) {
	fname := filename
	if !strings.HasSuffix(strings.ToLower(fname), ".json") {
		fname = filename + ".json"
	}
	for _, base := range configdir.GetBasePaths() {
		path := filepath.Join(base, channelsSubdir, fname)
		data, err := os.ReadFile(path)
		if err == nil {
			return data, nil
		}
	}
	return nil, fmt.Errorf("arquivo de canal legado %s não encontrado", filename)
}

func channelExistsForUser(userID, slug string) (bool, error) {
	if !usingDB() || storeDB == nil {
		return false, fmt.Errorf("channels DB não habilitado")
	}
	var count int64
	q := storeDB.Model(&database.Channel{}).Where("slug = ?", slug)
	if strings.TrimSpace(userID) == "" {
		q = q.Where("user_id = '' OR user_id IS NULL")
	} else {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// MigrateChannelSecrets moves plaintext tokens into CredManager and sets refs.
// Does not persist the config — caller saves afterwards. Safe if credMgr is nil.
func MigrateChannelSecrets(ctx context.Context, slug string, cfg *ChannelConfig, credMgr *credentials.Manager) error {
	if cfg == nil || credMgr == nil || !credMgr.CanPersist() {
		return nil
	}
	slug = strings.TrimSpace(slug)
	type tokenField struct {
		plain *string
		ref   *string
		key   string
	}
	fields := []tokenField{
		{&cfg.BotToken, &cfg.BotTokenRef, "bot_token"},
		{&cfg.AppToken, &cfg.AppTokenRef, "app_token"},
		{&cfg.APIToken, &cfg.APITokenRef, "api_token"},
	}
	for _, f := range fields {
		plain := strings.TrimSpace(*f.plain)
		if plain == "" {
			continue
		}
		ref := strings.TrimSpace(*f.ref)
		if ref == "" {
			ref = fmt.Sprintf("channel:%s:%s", slug, f.key)
			*f.ref = ref
		}
		if err := credMgr.RegisterPatternWithContext(ctx, ref, &credentials.AuthConfig{
			Type:  "secret",
			Token: plain,
		}); err != nil {
			return fmt.Errorf("migrar secret %s: %w", ref, err)
		}
		*f.plain = ""
	}
	return nil
}

// ImportLegacyChannelsWithContext imports channels/*.json (and contacts.json side-effect)
// into the DB. Idempotent per (user_id, slug). Never deletes legacy files.
func ImportLegacyChannelsWithContext(ctx context.Context, credMgr *credentials.Manager) (portability.LegacyImportResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return portability.LegacyImportResult{ResourceType: "channels"}, err
	}
	if !usingDB() {
		return portability.LegacyImportResult{
			ResourceType: "channels",
			Warnings:     []string{"channels DB não habilitado; importação legada ignorada"},
		}, nil
	}

	result, err := portability.ImportLegacyResourcesWithContext(ctx, portability.LegacyImportRequest[LegacyChannelItem]{
		ResourceType: "channels",
		Source:       LegacyConfigSource(),
		FileSuffix:   ".json",
		Parse: func(file portability.LegacyImportFile, data []byte) (LegacyChannelItem, error) {
			var cfg ChannelConfig
			if err := json.Unmarshal(data, &cfg); err != nil {
				return LegacyChannelItem{}, err
			}
			slug := file.Name
			if slug == "" {
				slug = strings.TrimSuffix(file.Filename, filepath.Ext(file.Filename))
			}
			if strings.TrimSpace(cfg.Type) == "" {
				cfg.Type = slug
			}
			if strings.TrimSpace(cfg.DisplayName) == "" {
				cfg.DisplayName = defaultDisplayName(cfg.Type)
			}
			return LegacyChannelItem{Slug: slug, Config: &cfg, Path: file.Path}, nil
		},
		Import: func(ctx context.Context, item LegacyChannelItem) (bool, error) {
			exists, err := channelExistsForUser(userID, item.Slug)
			if err != nil {
				return false, err
			}
			if exists {
				return false, nil
			}
			cfg := item.Config
			if cfg == nil {
				return false, fmt.Errorf("config nil")
			}
			// Prefer authenticated user as owner; keep explicit owner only if same user.
			owner := strings.TrimSpace(cfg.OwnerUserID)
			if owner == "" || owner == userID {
				cfg.OwnerUserID = userID
			} else {
				// Canal de outro usuário no FS — não importa no escopo atual.
				logging.Warnf(ctx, "channels.legacy-import",
					"[Channels] pulando %s: OwnerUserID=%s difere do usuário autenticado", item.Slug, owner)
				return false, nil
			}
			if err := MigrateChannelSecrets(ctx, item.Slug, cfg, credMgr); err != nil {
				return false, err
			}
			if err := Save(item.Slug, cfg); err != nil {
				return false, err
			}
			return true, nil
		},
	})
	if err != nil {
		return result, err
	}

	// contacts.json (read-only): importa contatos dos canais já no DB.
	contactsResult := importLegacyContactsFile(ctx, userID)
	result.Imported += contactsResult.Imported
	result.Skipped += contactsResult.Skipped
	result.Failed += contactsResult.Failed
	result.Warnings = append(result.Warnings, contactsResult.Warnings...)
	result.Errors = append(result.Errors, contactsResult.Errors...)
	return result, nil
}

func importLegacyContactsFile(ctx context.Context, userID string) portability.LegacyImportResult {
	result := portability.LegacyImportResult{ResourceType: "channel_contacts"}
	resolver := configdir.NewResolver("")
	data, _, err := resolver.Read("contacts.json")
	if err != nil || len(data) == 0 {
		return result
	}
	var file map[string][]struct {
		ID           string `json:"id"`
		DisplayName  string `json:"display_name"`
		Username     string `json:"username"`
		AuthorizedAt string `json:"authorized_at"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		result.Failed++
		result.Errors = append(result.Errors, fmt.Sprintf("erro ao parsear contacts.json: %v", err))
		return result
	}

	for slug, list := range file {
		channelID, ownerID, err := ChannelIDBySlug(slug)
		if err != nil || channelID == "" {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("contacts.json: canal %s ausente no DB — contatos ignorados", slug))
			continue
		}
		if ownerID != "" && ownerID != userID {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("contacts.json: canal %s pertence a outro usuário — ignorado", slug))
			continue
		}
		for _, c := range list {
			if strings.TrimSpace(c.ID) == "" {
				result.Skipped++
				continue
			}
			var count int64
			if err := storeDB.Model(&database.ChannelContact{}).
				Where("channel_id = ? AND external_id = ?", channelID, c.ID).
				Count(&count).Error; err != nil {
				result.Failed++
				result.Errors = append(result.Errors, err.Error())
				continue
			}
			if count > 0 {
				result.Skipped++
				continue
			}
			row := database.ChannelContact{
				UserID:      userID,
				ChannelID:   channelID,
				ExternalID:  c.ID,
				DisplayName: c.DisplayName,
				Username:    c.Username,
			}
			if ts := strings.TrimSpace(c.AuthorizedAt); ts != "" {
				if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
					row.AuthorizedAt = &parsed
				}
			}
			if err := storeDB.Create(&row).Error; err != nil {
				if strings.Contains(strings.ToUpper(err.Error()), "UNIQUE") {
					result.Skipped++
					continue
				}
				result.Failed++
				result.Errors = append(result.Errors, fmt.Sprintf("%s/%s: %v", slug, c.ID, err))
				continue
			}
			result.Imported++
		}
	}
	return result
}
