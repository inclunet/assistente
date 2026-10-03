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
					continue
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
		return credentials.StaticConnectionUpdate{ID: row.CredentialID, Integration: "slack", ConsumerID: row.ID, Legacy: legacy, Changes: map[credentials.SecretRole]*string{credentials.RoleBotToken: &bot, credentials.RoleAppToken: &app}, Commit: func(tx *gorm.DB, id string, present map[credentials.SecretRole]bool) error {
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
