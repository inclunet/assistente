package database

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// MigrateMessageRevisions gives every persisted message incarnation an opaque
// revision, independently of timestamps. Triggers cover SQL, bulk updates and
// legacy writers as well as repository methods. The side table keeps this
// concurrency metadata out of message payloads and generated UI bindings.
func MigrateMessageRevisions(database *gorm.DB) error {
	return database.Transaction(func(tx *gorm.DB) error {
		statements := []string{
			`CREATE TABLE IF NOT EXISTS chat_message_revisions (
				message_id TEXT PRIMARY KEY NOT NULL,
				revision TEXT NOT NULL CHECK(length(revision) = 64)
			)`,
			`INSERT INTO chat_message_revisions (message_id, revision)
			 SELECT id, lower(hex(randomblob(32))) FROM chat_messages
			 WHERE id NOT IN (SELECT message_id FROM chat_message_revisions)`,
			`CREATE TRIGGER IF NOT EXISTS chat_message_revision_insert AFTER INSERT ON chat_messages
			 BEGIN
				INSERT INTO chat_message_revisions (message_id, revision)
				VALUES (NEW.id, lower(hex(randomblob(32))))
				ON CONFLICT(message_id) DO UPDATE SET revision = excluded.revision;
			 END`,
			`CREATE TRIGGER IF NOT EXISTS chat_message_revision_update AFTER UPDATE ON chat_messages
			 BEGIN
				DELETE FROM chat_message_revisions WHERE message_id = OLD.id AND OLD.id <> NEW.id;
				INSERT INTO chat_message_revisions (message_id, revision)
				VALUES (NEW.id, lower(hex(randomblob(32))))
				ON CONFLICT(message_id) DO UPDATE SET revision = excluded.revision;
			 END`,
			`CREATE TRIGGER IF NOT EXISTS chat_message_revision_delete AFTER DELETE ON chat_messages
			 BEGIN
				DELETE FROM chat_message_revisions WHERE message_id = OLD.id;
			 END`,
		}
		for _, statement := range statements {
			if err := tx.Exec(statement).Error; err != nil {
				return fmt.Errorf("migrate message revisions: %w", err)
			}
		}
		return nil
	})
}

// Read only after checking message ownership, in the same snapshot transaction.
// Missing metadata is an error, never permission to fall back to timestamps.
func messageStoredRevisionTx(ctx context.Context, tx *gorm.DB, messageID string) (string, error) {
	var row struct{ Revision string }
	if err := tx.WithContext(ctx).Table("chat_message_revisions").
		Where("message_id = ?", messageID).Take(&row).Error; err != nil {
		return "", err
	}
	if len(row.Revision) != 64 {
		return "", ErrMessageContentChanged
	}
	return row.Revision, nil
}
