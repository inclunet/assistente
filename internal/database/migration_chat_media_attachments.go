package database

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// Schemas predating v46 physically contained these fields. Runtime models no
// longer create them; historical migrations own their transitional layout.
func ensureLegacyChatMediaColumns(tx *gorm.DB) error {
	for _, column := range []string{"media", "audio", "audio_mime_type"} {
		if !tx.Migrator().HasColumn("chat_messages", column) {
			if err := tx.Exec("ALTER TABLE chat_messages ADD COLUMN " + column + " TEXT").Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func MigrateChatMediaAttachments(database *gorm.DB) error {
	if !database.Migrator().HasColumn("chat_messages", "media") && !database.Migrator().HasColumn("chat_messages", "audio") && !database.Migrator().HasColumn("chat_messages", "audio_mime_type") && database.Migrator().HasColumn("chat_message_attachments", "metadata") {
		return nil
	}
	var missingConversations int64
	if err := database.Table("chat_messages m").Joins("LEFT JOIN conversations c ON c.id=m.conversation_id").Where("c.id IS NULL AND (COALESCE(m.media,'')<>'' OR COALESCE(m.audio,'')<>'')").Count(&missingConversations).Error; err != nil {
		return err
	}
	if missingConversations > 0 {
		return fmt.Errorf("migração de anexos bloqueada: %d mensagens com conteúdo legado sem conversa; reconciliação offline necessária", missingConversations)
	}
	var orphans int64
	if err := database.Table("chat_messages m").Joins("JOIN conversations c ON c.id=m.conversation_id").Where("COALESCE(c.user_id,'')='' AND (COALESCE(m.media,'')<>'' OR COALESCE(m.audio,'')<>'')").Count(&orphans).Error; err != nil {
		return err
	}
	if orphans > 0 {
		return errMigrationDeferred
	}
	if err := backupChatMigration(database, ".pre-chat-media-v46.bak"); err != nil {
		return err
	}
	return database.Connection(func(connection *gorm.DB) error {
		var foreignKeys int
		if err := connection.Raw("PRAGMA foreign_keys").Scan(&foreignKeys).Error; err != nil {
			return err
		}
		if err := connection.Exec("PRAGMA foreign_keys=OFF").Error; err != nil {
			return err
		}
		migrationErr := connection.Transaction(func(tx *gorm.DB) error {
			var triggers []struct{ Name string }
			if err := tx.Raw("SELECT name FROM sqlite_master WHERE type='trigger' AND (name LIKE 'chat_attachment_%' OR name LIKE 'chat_audio_retired_%' OR name='chat_timeline_chat_messages_update')").Scan(&triggers).Error; err != nil {
				return err
			}
			hadTimelineTrigger := false
			for _, trigger := range triggers {
				if trigger.Name == "chat_timeline_chat_messages_update" {
					hadTimelineTrigger = true
				}
				if err := tx.Exec(`DROP TRIGGER "` + strings.ReplaceAll(trigger.Name, `"`, `""`) + `"`).Error; err != nil {
					return err
				}
			}
			contentSchema := strings.Replace(chatAttachmentSchema[0], "IF NOT EXISTS chat_attachment_contents", "chat_attachment_contents_v46", 1)
			contentSchema = strings.Replace(contentSchema, "size>0", "size>=0", 1)
			linkSchema := strings.Replace(chatAttachmentSchema[1], "IF NOT EXISTS chat_message_attachments", "chat_message_attachments_v46", 1)
			linkSchema = strings.Replace(linkSchema, "content_id TEXT NOT NULL", "content_id TEXT", 1)
			linkSchema = strings.Replace(linkSchema, "PRIMARY KEY(message_id,purpose,position)", `metadata TEXT NOT NULL DEFAULT '{}',
 data_field TEXT NOT NULL DEFAULT '' CHECK(data_field IN ('','data','url')),
 data_prefix TEXT NOT NULL DEFAULT '',
 CHECK(content_id IS NOT NULL OR purpose='media'),
 PRIMARY KEY(message_id,purpose,position)`, 1)
			for _, statement := range []string{contentSchema, linkSchema,
				"INSERT INTO chat_attachment_contents_v46 SELECT * FROM chat_attachment_contents",
				"INSERT INTO chat_message_attachments_v46(message_id,purpose,position,content_id,origin,mime_type) SELECT message_id,purpose,position,content_id,origin,mime_type FROM chat_message_attachments",
				"DROP TABLE chat_message_attachments", "DROP TABLE chat_attachment_contents",
				"ALTER TABLE chat_attachment_contents_v46 RENAME TO chat_attachment_contents",
				"ALTER TABLE chat_message_attachments_v46 RENAME TO chat_message_attachments",
			} {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			last := ""
			for {
				var row struct{ ID, Media string }
				result := tx.Table("chat_messages").Select("id,media").Where("id>? AND COALESCE(media,'')<>''", last).Order("id").Limit(1).Scan(&row)
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected == 0 {
					break
				}
				if err := storeMessageMedia(tx, row.ID, row.Media); err != nil {
					return fmt.Errorf("migrar mídia da mensagem %s: %w", row.ID, err)
				}
				last = row.ID
			}
			// Links removed as duplicate audio must not leave content without references.
			if err := tx.Exec("DELETE FROM chat_attachment_contents WHERE NOT EXISTS (SELECT 1 FROM chat_message_attachments WHERE content_id=chat_attachment_contents.id)").Error; err != nil {
				return err
			}
			var remainingAudio int64
			if err := tx.Table("chat_messages").Where("COALESCE(audio,'')<>''").Count(&remainingAudio).Error; err != nil {
				return err
			}
			if remainingAudio > 0 {
				return fmt.Errorf("corte de anexos recusado: %d áudios ainda legados", remainingAudio)
			}
			for _, column := range []string{"media", "audio", "audio_mime_type"} {
				if err := tx.Exec("ALTER TABLE chat_messages DROP COLUMN " + column).Error; err != nil {
					return err
				}
			}
			for _, statement := range chatAttachmentSchema[2:] {
				statement = strings.Replace(statement, "WHEN NOT EXISTS(SELECT 1 FROM chat_messages", "WHEN NEW.content_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM chat_messages", 1)
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			if hadTimelineTrigger {
				if err := installTimelineSourceTriggers(tx, "chat_messages", "conversation_id=OLD.conversation_id", "conversation_id=NEW.conversation_id"); err != nil {
					return err
				}
			}
			for _, table := range []string{"chat_attachment_contents", "chat_message_attachments"} {
				var violations []struct{ Table string }
				if err := tx.Raw("PRAGMA foreign_key_check(" + table + ")").Scan(&violations).Error; err != nil {
					return err
				}
				if len(violations) > 0 {
					return fmt.Errorf("integridade referencial após migração de anexos em %s: %d violações", table, len(violations))
				}
			}
			lastContent := ""
			for {
				var row struct {
					ID, SHA256 string
					Size       int
					Content    []byte
				}
				result := tx.Table("chat_attachment_contents").Where("id>?", lastContent).Order("id").Limit(1).Scan(&row)
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected == 0 {
					break
				}
				if err := validateAttachmentContent(row.Content, row.Size, row.SHA256); err != nil {
					return fmt.Errorf("conteúdo de anexo %s: %w", row.ID, err)
				}
				lastContent = row.ID
			}
			var integrity []string
			if err := tx.Raw("PRAGMA integrity_check").Scan(&integrity).Error; err != nil {
				return err
			}
			if len(integrity) != 1 || integrity[0] != "ok" {
				return fmt.Errorf("integridade SQLite após migração de anexos inválida")
			}
			return nil
		})
		restore := fmt.Sprintf("PRAGMA foreign_keys=%d", foreignKeys)
		if err := connection.Exec(restore).Error; err != nil {
			return fmt.Errorf("restaurar foreign keys: %w (migração: %v)", err, migrationErr)
		}
		return migrationErr
	})
}
