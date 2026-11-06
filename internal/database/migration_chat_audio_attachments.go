package database

import (
	"fmt"
	"gorm.io/gorm"
)

// Audio becomes a single referenced binary per user. General media remains in
// its existing column until its own verified cutover; no fallback audio reader.
func MigrateChatAudioAttachments(db *gorm.DB) error {
	if err := ensureLegacyChatMediaColumns(db); err != nil {
		return err
	}
	var count int64
	if err := db.Model(&ChatMessage{}).Where("COALESCE(audio,'')<>''").Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		if err := backupChatMigration(db, ".pre-chat-audio-v45.bak"); err != nil {
			return err
		}
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, sql := range chatAttachmentSchema {
			if err := tx.Exec(sql).Error; err != nil {
				return err
			}
		}
		if err := migrateOwnedMessageAudio(tx, ""); err != nil {
			return err
		}
		for _, sql := range chatAttachmentGuards {
			if err := tx.Exec(sql).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

var chatAttachmentSchema = []string{
	`CREATE TABLE IF NOT EXISTS chat_attachment_contents (
 id TEXT PRIMARY KEY NOT NULL,
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 sha256 TEXT NOT NULL CHECK(length(sha256)=64),
 size INTEGER NOT NULL CHECK(size>0 AND size=length(content)),
 content BLOB NOT NULL CHECK(typeof(content)='blob'), UNIQUE(user_id,sha256))`,
	`CREATE TABLE IF NOT EXISTS chat_message_attachments (
 message_id TEXT NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,
 purpose TEXT NOT NULL CHECK(purpose IN ('audio','media')),
 position INTEGER NOT NULL CHECK(position>=0 AND (purpose<>'audio' OR position=0)),
 content_id TEXT NOT NULL REFERENCES chat_attachment_contents(id) ON DELETE CASCADE,
 origin TEXT NOT NULL CHECK(origin IN ('received','generated','legacy_unknown')),
 mime_type TEXT NOT NULL,
 PRIMARY KEY(message_id,purpose,position))`,
	`CREATE INDEX IF NOT EXISTS idx_chat_attachments_content ON chat_message_attachments(content_id)`,
	`CREATE TRIGGER IF NOT EXISTS chat_attachment_content_immutable BEFORE UPDATE ON chat_attachment_contents BEGIN SELECT RAISE(ABORT,'immutable attachment content'); END`,
	`CREATE TRIGGER IF NOT EXISTS chat_attachment_owner_insert BEFORE INSERT ON chat_message_attachments
 WHEN NOT EXISTS(SELECT 1 FROM chat_messages m JOIN conversations c ON c.id=m.conversation_id JOIN chat_attachment_contents b ON b.id=NEW.content_id WHERE m.id=NEW.message_id AND c.user_id=b.user_id)
 BEGIN SELECT RAISE(ABORT,'attachment owner mismatch'); END`,
	`CREATE TRIGGER IF NOT EXISTS chat_attachment_owner_update BEFORE UPDATE ON chat_message_attachments
 WHEN NOT EXISTS(SELECT 1 FROM chat_messages m JOIN conversations c ON c.id=m.conversation_id JOIN chat_attachment_contents b ON b.id=NEW.content_id WHERE m.id=NEW.message_id AND c.user_id=b.user_id)
 BEGIN SELECT RAISE(ABORT,'attachment owner mismatch'); END`,
	`CREATE TRIGGER IF NOT EXISTS chat_attachment_message_owner BEFORE UPDATE OF conversation_id ON chat_messages
 WHEN EXISTS(SELECT 1 FROM chat_message_attachments a JOIN chat_attachment_contents b ON b.id=a.content_id WHERE a.message_id=OLD.id AND NOT EXISTS(SELECT 1 FROM conversations c WHERE c.id=NEW.conversation_id AND c.user_id=b.user_id))
 BEGIN SELECT RAISE(ABORT,'attachment owner mismatch'); END`,
	`CREATE TRIGGER IF NOT EXISTS chat_attachment_conversation_owner BEFORE UPDATE OF user_id ON conversations
 WHEN NEW.user_id IS NOT OLD.user_id AND EXISTS(SELECT 1 FROM chat_messages m JOIN chat_message_attachments a ON a.message_id=m.id WHERE m.conversation_id=OLD.id)
 BEGIN SELECT RAISE(ABORT,'attachment owner mismatch'); END`,
	`CREATE TRIGGER IF NOT EXISTS chat_attachment_insert AFTER INSERT ON chat_message_attachments BEGIN
 UPDATE chat_messages SET updated_at=strftime('%Y-%m-%d %H:%M:%f','now') WHERE id=NEW.message_id;
 UPDATE chat_timeline_versions SET revision=revision+1 WHERE conversation_id=(SELECT conversation_id FROM chat_messages WHERE id=NEW.message_id); END`,
	`CREATE TRIGGER IF NOT EXISTS chat_attachment_delete AFTER DELETE ON chat_message_attachments BEGIN
 UPDATE chat_messages SET updated_at=strftime('%Y-%m-%d %H:%M:%f','now') WHERE id=OLD.message_id;
 UPDATE chat_timeline_versions SET revision=revision+1 WHERE conversation_id=(SELECT conversation_id FROM chat_messages WHERE id=OLD.message_id);
 DELETE FROM chat_attachment_contents WHERE id=OLD.content_id AND NOT EXISTS(SELECT 1 FROM chat_message_attachments WHERE content_id=OLD.content_id); END`,
	`CREATE TRIGGER IF NOT EXISTS chat_attachment_update AFTER UPDATE ON chat_message_attachments BEGIN
 UPDATE chat_messages SET updated_at=strftime('%Y-%m-%d %H:%M:%f','now') WHERE id IN (OLD.message_id,NEW.message_id);
 UPDATE chat_timeline_versions SET revision=revision+1 WHERE conversation_id IN (SELECT conversation_id FROM chat_messages WHERE id IN (OLD.message_id,NEW.message_id));
 DELETE FROM chat_attachment_contents WHERE id=OLD.content_id AND NOT EXISTS(SELECT 1 FROM chat_message_attachments WHERE content_id=OLD.content_id); END`,
}
var chatAttachmentGuards = []string{
	`CREATE TRIGGER IF NOT EXISTS chat_audio_retired_insert BEFORE INSERT ON chat_messages WHEN COALESCE(NEW.audio,'')<>'' OR COALESCE(NEW.audio_mime_type,'')<>'' BEGIN SELECT RAISE(ABORT,'audio requires attachment writer'); END`,
	`CREATE TRIGGER IF NOT EXISTS chat_audio_retired_update BEFORE UPDATE OF audio,audio_mime_type ON chat_messages WHEN COALESCE(NEW.audio,'')<>'' OR COALESCE(NEW.audio_mime_type,'')<>'' BEGIN SELECT RAISE(ABORT,'audio requires attachment writer'); END`,
}

// Adoption calls this after assigning the real owner, in the same transaction.
func migrateOwnedMessageAudio(tx *gorm.DB, userID string) error {
	last := ""
	for {
		var rows []struct {
			ID              string
			Audio           string
			AudioMimeType   string
			Media           string
			StoredUpdatedAt *string `gorm:"column:stored_updated_at"`
		}
		if err := tx.Table("chat_messages").Select("id, audio, audio_mime_type, media, CAST(updated_at AS TEXT) AS stored_updated_at").Where("id>? AND COALESCE(audio,'')<>''", last).Where("conversation_id IN (SELECT id FROM conversations WHERE COALESCE(user_id,'')<>'' AND (?='' OR user_id=?))", userID, userID).Order("id").Limit(1).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		for _, m := range rows {
			decoded, err := decodeAttachment(m.Audio)
			if err != nil {
				return fmt.Errorf("migrar áudio da mensagem %s: %w", m.ID, err)
			}
			if err := storeMessageAudio(tx, m.ID, m.Audio, m.AudioMimeType, legacyAudioOrigin(m.Media, decoded)); err != nil {
				return err
			}
			if err := tx.Exec("UPDATE chat_messages SET audio='',audio_mime_type='',updated_at=? WHERE id=?", m.StoredUpdatedAt, m.ID).Error; err != nil {
				return err
			}
			last = m.ID
		}
	}

	return nil
}
