package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"gorm.io/gorm"
)

var ErrMessageContentChanged = errors.New("message changed before command")

// The editor's original text and the complete revision are checked/read from
// the same transaction, before the command is admitted. Neither is persisted.
func MessageEditCommandSnapshotWithContext(ctx context.Context, conversationID, messageID, originalContent string) (string, error) {
	return messageOriginalCommandSnapshot(ctx, conversationID, messageID, originalContent, true)
}

func MessageSourceCommandSnapshotWithContext(ctx context.Context, conversationID, messageID, originalContent string) (string, error) {
	return messageOriginalCommandSnapshot(ctx, conversationID, messageID, originalContent, false)
}

func messageOriginalCommandSnapshot(ctx context.Context, conversationID, messageID, originalContent string, editing bool) (string, error) {
	var revision string
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		message, err := NewMessageRepository(tx).GetMessageWithContext(ctx, messageID)
		if err != nil {
			return err
		}
		if message.ConversationID != conversationID || message.Content != originalContent || message.ParentID != nil || (message.Role != "user" && (editing || message.Role != "assistant")) {
			return ErrMessageContentChanged
		}
		revision, err = messageCommandRevisionTx(ctx, tx, conversationID, messageID, false)
		return err
	})
	return revision, err
}

// MessageCommandSnapshotWithContext returns an opaque runtime-only revision.
// Delete includes the conversation because its existing semantics cascade into
// descendants/turns and technical invocations, including newly inserted rows.
func MessageCommandSnapshotWithContext(ctx context.Context, conversationID, messageID string, deleting bool) (string, error) {
	var revision string
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		revision, err = messageCommandRevisionTx(ctx, tx, conversationID, messageID, deleting)
		return err
	})
	return revision, err
}

func messageCommandRevisionTx(ctx context.Context, tx *gorm.DB, conversationID, messageID string, deleting bool) (string, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return "", err
	}
	message, err := NewMessageRepository(tx).GetMessageWithContext(ctx, messageID)
	if err != nil {
		return "", err
	}
	if message.ConversationID != conversationID {
		return "", gorm.ErrRecordNotFound
	}
	if deleting {
		// Legacy cascade follows parent links. Refuse malformed cross-conversation
		// links rather than deleting anything outside the captured conversation.
		var external int64
		if err := tx.Model(&ChatMessage{}).Where("conversation_id <> ? AND parent_id IN (?)", conversationID,
			tx.Model(&ChatMessage{}).Select("id").Where("conversation_id = ?", conversationID)).Count(&external).Error; err != nil {
			return "", err
		}
		if external != 0 {
			return "", ErrMessageContentChanged
		}
		return conversationContentRevisionTx(ctx, tx, conversationID)
	}
	raw, err := json.Marshal(message)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

// MutateMessageIfUnchangedWithinLifecycleWithContext compares and writes inside
// one BEGIN IMMEDIATE. The caller owns the lifecycle gate, including events.
func MutateMessageIfUnchangedWithinLifecycleWithContext(ctx context.Context, conversationID, messageID, expected string, deleting bool) (*ChatMessage, error) {
	return mutateMessageIfUnchangedWithinLifecycle(ctx, conversationID, messageID, expected, deleting, nil)
}

func UpdateMessageIfUnchangedWithinLifecycleWithContext(ctx context.Context, conversationID, messageID, expected, content string) (*ChatMessage, error) {
	return mutateMessageIfUnchangedWithinLifecycle(ctx, conversationID, messageID, expected, false, &content)
}

func mutateMessageIfUnchangedWithinLifecycle(ctx context.Context, conversationID, messageID, expected string, deleting bool, content *string) (*ChatMessage, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var updated *ChatMessage
	err := WithSQLiteMaintenance(ctx, func() error {
		return withSQLiteImmediateTransaction(ctx, db, "message.command", func(tx *gorm.DB) error {
			revision, err := messageCommandRevisionTx(ctx, tx, conversationID, messageID, deleting)
			if err != nil {
				return err
			}
			if expected == "" || revision != expected {
				return ErrMessageContentChanged
			}
			repo := NewMessageRepository(tx)
			if deleting {
				return repo.DeleteMessageWithContext(ctx, messageID)
			}
			if content != nil {
				message, err := repo.GetMessageWithContext(ctx, messageID)
				if err != nil {
					return err
				}
				if message.Role != "user" || message.ParentID != nil {
					return ErrMessageContentChanged
				}
				// Textual edits must preserve the message's generation metadata.
				if err := repo.UpdateMessageTextWithContext(ctx, messageID, *content); err != nil {
					return err
				}
				updated, err = repo.GetMessageWithContext(ctx, messageID)
				return err
			}
			// Already in BEGIN IMMEDIATE; do not nest the legacy pin transaction.
			if err := tx.Model(&ChatMessage{}).Where("id = ? AND conversation_id = ?", messageID, conversationID).Update("pinned", gorm.Expr("NOT pinned")).Error; err != nil {
				return err
			}
			updated, err = repo.GetMessageWithContext(ctx, messageID)
			return err
		})
	})
	return updated, err
}
