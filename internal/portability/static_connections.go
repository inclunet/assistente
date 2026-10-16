package portability

import (
	"context"
	"errors"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// A portable connection identifies its purpose, not the source vault's UUID or
// user. Components only travel inside the password-encrypted credential blob,
// gated by IncludeCredentials in BuildExportFileWithContext.
const slackConnectionBackupPattern = "channel:slack:connection"

// Normalize historical pairs before both conflict analysis and import. The
// normalized entry has no source UUID, so it cannot bypass the target conflict.
func normalizeStaticConnectionExports(items []CredentialExport) ([]CredentialExport, error) {
	result := make([]CredentialExport, 0, len(items))
	components := map[credentials.SecretRole]string{}
	composed := false
	for _, item := range items {
		var role credentials.SecretRole
		switch item.Pattern {
		case "channel:slack:bot_token":
			role = credentials.RoleBotToken
		case "channel:slack:app_token":
			role = credentials.RoleAppToken
		case slackConnectionBackupPattern:
			if composed {
				return nil, credentials.ErrStaticConnection
			}
			composed = true
		}
		if role == "" {
			result = append(result, item)
			continue
		}
		if (item.Source != "" && item.Source != "static") || item.AuthType != "secret" || item.SourceConfig != nil || item.Username != "" || item.Password != "" || len(item.Headers) != 0 || item.ClientID != "" || item.ClientSecret != "" || item.RefreshURL != "" || item.ExpiresAt != 0 || item.StaticComponents != nil {
			return nil, credentials.ErrStaticConnection
		}
		if _, duplicate := components[role]; duplicate {
			return nil, credentials.ErrStaticConnection
		}
		components[role] = item.Token
	}
	if len(components) != 0 {
		if composed {
			return nil, credentials.ErrStaticConnection
		}
		result = append(result, CredentialExport{Pattern: slackConnectionBackupPattern, Source: "static", AuthType: credentials.StaticConnectionType, StaticComponents: components, staticPartial: true})
	}
	return result, nil
}

func exportStaticConnections(ctx context.Context, manager *credentials.Manager) ([]CredentialExport, error) {
	if !database.DB().Migrator().HasColumn(&database.Channel{}, "CredentialID") {
		return nil, nil
	}
	var rows []database.Channel
	if err := database.ScopeByUser(ctx, database.DB(), "user_id").Where("slug = ? AND credential_id <> ''", "slack").Find(&rows).Error; err != nil {
		return nil, err
	}
	var result []CredentialExport
	for _, row := range rows {
		components, err := manager.ResolveStaticComponents(ctx, row.CredentialID, "slack", row.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, CredentialExport{Source: "static", AuthType: credentials.StaticConnectionType, Pattern: slackConnectionBackupPattern, StaticComponents: components})
	}
	return result, nil
}

func validateStaticConnectionExport(cred CredentialExport) error {
	if cred.Pattern != slackConnectionBackupPattern || cred.AuthType != credentials.StaticConnectionType || cred.Source != "static" || cred.SourceConfig != nil || cred.Token != "" || cred.Username != "" || cred.Password != "" || len(cred.Headers) != 0 || cred.ClientID != "" || cred.ClientSecret != "" || cred.RefreshURL != "" || cred.ExpiresAt != 0 || cred.ID != "" {
		return credentials.ErrStaticConnection
	}
	for role := range cred.StaticComponents {
		if role != credentials.RoleBotToken && role != credentials.RoleAppToken {
			return credentials.ErrStaticConnection
		}
	}
	return nil
}

// Restore is atomic, user-bound and never activates a new connection. Existing
// settings remain intact; overwriting requires the normal import conflict
// decision and does not recreate the historical pair.
func importStaticConnection(ctx context.Context, manager *credentials.Manager, cred CredentialExport, allowOverwrite bool) error {
	user, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	return manager.UpdateStaticConnection(ctx, func(tx *gorm.DB) (credentials.StaticConnectionUpdate, error) {
		var row database.Channel
		err := tx.Where("user_id = ? AND slug = ?", user, "slack").First(&row).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return credentials.StaticConnectionUpdate{}, err
		}
		exists := err == nil
		if exists && !allowOverwrite {
			return credentials.StaticConnectionUpdate{}, credentials.ErrStaticConnection
		}
		if !exists {
			row = database.Channel{UUIDModel: database.UUIDModel{ID: uuid.NewString()}, UserID: user, Slug: "slack", Type: "slack", DisplayName: "Slack", MaxHistory: 50, MaxContacts: 1}
		}
		legacy := map[credentials.SecretRole]string{}
		if row.CredentialID == "" {
			for role, ref := range map[credentials.SecretRole]string{credentials.RoleBotToken: row.BotTokenRef, credentials.RoleAppToken: row.AppTokenRef} {
				if ref == "" {
					ref = "channel:slack:" + string(role)
					var count int64
					if err := tx.Model(&database.CredentialEntry{}).Where("user_id = ? AND pattern = ?", user, ref).Count(&count).Error; err != nil {
						return credentials.StaticConnectionUpdate{}, err
					}
					if count == 0 {
						continue
					}
				}
				if !allowOverwrite {
					return credentials.StaticConnectionUpdate{}, credentials.ErrStaticConnection
				}
				if ref != "channel:slack:"+string(role) {
					return credentials.StaticConnectionUpdate{}, credentials.ErrStaticConnection
				}
				var count int64
				if err := tx.Model(&database.Channel{}).Where("user_id = ? AND id <> ? AND (bot_token_ref = ? OR app_token_ref = ? OR api_token_ref = ?)", user, row.ID, ref, ref, ref).Count(&count).Error; err != nil {
					return credentials.StaticConnectionUpdate{}, err
				}
				if count != 0 {
					return credentials.StaticConnectionUpdate{}, credentials.ErrStaticConnection
				}
				legacy[role] = ref
			}
		}
		bot, app := cred.StaticComponents[credentials.RoleBotToken], cred.StaticComponents[credentials.RoleAppToken]
		changes := map[credentials.SecretRole]*string{credentials.RoleBotToken: &bot, credentials.RoleAppToken: &app}
		if cred.staticPartial {
			for role := range changes {
				if _, provided := cred.StaticComponents[role]; !provided {
					delete(changes, role)
				}
			}
		}
		return credentials.StaticConnectionUpdate{ID: row.CredentialID, Integration: "slack", ConsumerID: row.ID, Legacy: legacy, Changes: changes, RecoveryRoles: []credentials.SecretRole{credentials.RoleBotToken, credentials.RoleAppToken}, Commit: func(tx *gorm.DB, id string, present map[credentials.SecretRole]bool) error {
			row.CredentialID = id
			row.BotTokenRef, row.AppTokenRef = "", ""
			if present[credentials.RoleBotToken] {
				row.BotTokenRef = credentials.StaticConnectionPattern(id)
			}
			if present[credentials.RoleAppToken] {
				row.AppTokenRef = credentials.StaticConnectionPattern(id)
			}
			if !present[credentials.RoleBotToken] || !present[credentials.RoleAppToken] {
				row.Enabled = false
			}
			if exists {
				return tx.Save(&row).Error
			}
			return tx.Create(&row).Error
		}}, nil
	})
}
