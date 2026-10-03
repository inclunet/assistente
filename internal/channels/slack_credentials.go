package channels

import (
	"context"
	"strings"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SaveSlackWithCredentials persists both static roles and the channel in one
// vault transaction. Empty form fields preserve existing secrets; removal is
// explicit. Legacy plaintext is input-only and never reaches channel storage.
func SaveSlackWithCredentials(ctx context.Context, cfg *ChannelConfig, manager *credentials.Manager) error {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if cfg == nil || manager == nil || (cfg.OwnerUserID != "" && cfg.OwnerUserID != userID) {
		return credentials.ErrStaticConnection
	}
	mu.Lock()
	defer mu.Unlock()
	if !usingDB() {
		return ErrDBNotEnabled
	}
	var persisted *ChannelConfig
	err = manager.UpdateStaticConnection(ctx, func(tx *gorm.DB) (credentials.StaticConnectionUpdate, error) {
		existing, err := findChannelRowForUser(tx, "slack", userID)
		if err != nil {
			return credentials.StaticConnectionUpdate{}, err
		}
		candidate := *cfg
		candidate.OwnerUserID, candidate.Type = userID, "slack"
		if cfg.RemoveBotToken || cfg.RemoveAppToken {
			candidate.Enabled = false
		}
		candidate.ID, candidate.CredentialID = uuid.NewString(), ""
		legacy := map[credentials.SecretRole]string{}
		if existing != nil {
			old := RowToConfig(existing, nil)
			candidate.ID, candidate.CredentialID = existing.ID, existing.CredentialID
			if candidate.ReplyChatIDs == nil {
				candidate.ReplyChatIDs = old.ReplyChatIDs
			}
			if candidate.DisplayName == "" {
				candidate.DisplayName = old.DisplayName
			}
			if existing.CredentialID == "" {
				legacy[credentials.RoleBotToken], legacy[credentials.RoleAppToken] = existing.BotTokenRef, existing.AppTokenRef
			}
		} else {
			// Existing user-owned static entries may predate the channel row.
			legacy[credentials.RoleBotToken], legacy[credentials.RoleAppToken] = cfg.BotTokenRef, cfg.AppTokenRef
		}
		if candidate.CredentialID == "" {
			for _, role := range []credentials.SecretRole{credentials.RoleBotToken, credentials.RoleAppToken} {
				if legacy[role] != "" {
					continue
				}
				ref := "channel:slack:" + string(role)
				var count int64
				if err := tx.Model(&database.CredentialEntry{}).Where("user_id = ? AND pattern = ?", userID, ref).Count(&count).Error; err != nil {
					return credentials.StaticConnectionUpdate{}, err
				}
				if count != 0 {
					legacy[role] = ref
				}
			}
		}
		for role, ref := range legacy {
			if ref == "" {
				delete(legacy, role)
				continue
			}
			// Do not delete a general-purpose/shared secret during migration.
			if ref != "channel:slack:"+string(role) {
				return credentials.StaticConnectionUpdate{}, credentials.ErrStaticConnection
			}
			var other int64
			if err := tx.Model(&database.Channel{}).Where("user_id = ? AND id <> ? AND (bot_token_ref = ? OR app_token_ref = ? OR api_token_ref = ?)", userID, candidate.ID, ref, ref, ref).Count(&other).Error; err != nil {
				return credentials.StaticConnectionUpdate{}, err
			}
			if other != 0 {
				return credentials.StaticConnectionUpdate{}, credentials.ErrStaticConnection
			}
		}
		changes := map[credentials.SecretRole]*string{}
		bot, app := strings.TrimSpace(cfg.BotToken), strings.TrimSpace(cfg.AppToken)
		if cfg.RemoveBotToken {
			bot = ""
			changes[credentials.RoleBotToken] = &bot
		} else if bot != "" {
			changes[credentials.RoleBotToken] = &bot
		}
		if cfg.RemoveAppToken {
			app = ""
			changes[credentials.RoleAppToken] = &app
		} else if app != "" {
			changes[credentials.RoleAppToken] = &app
		}
		return credentials.StaticConnectionUpdate{ID: candidate.CredentialID, Integration: "slack", ConsumerID: candidate.ID, Changes: changes, Legacy: legacy, RecoveryRoles: []credentials.SecretRole{credentials.RoleBotToken, credentials.RoleAppToken},
			Commit: func(tx *gorm.DB, id string, present map[credentials.SecretRole]bool) error {
				candidate.CredentialID = id
				candidate.BotToken, candidate.AppToken = "", ""
				candidate.BotTokenRef, candidate.AppTokenRef = "", ""
				if present[credentials.RoleBotToken] {
					candidate.BotTokenRef = credentials.StaticConnectionPattern(id)
				}
				if present[credentials.RoleAppToken] {
					candidate.AppTokenRef = credentials.StaticConnectionPattern(id)
				}
				candidate.RemoveBotToken, candidate.RemoveAppToken = false, false
				row := ConfigToRow("slack", &candidate)
				row.ID = candidate.ID
				if existing != nil {
					row.CreatedAt = existing.CreatedAt
					err = tx.Save(&row).Error
				} else {
					err = tx.Create(&row).Error
				}
				if err != nil {
					return err
				}
				if cfg.Conversations != nil {
					if err := syncConversations(ctx, tx, row.ID, userID, cfg.Conversations); err != nil {
						return err
					}
				}
				persisted = &candidate
				return nil
			}}, nil
	})
	if err != nil {
		return err
	}
	*cfg = *persisted
	rememberOwner("slack", userID)
	return nil
}
