package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

var ErrConversationContentChanged = errors.New("conversation content changed before clear")

// ConversationContentSnapshotWithContext é o contrato de captura pré-confirmação.
func ConversationContentSnapshotWithContext(ctx context.Context, id string) (string, error) {
	return GetConversationContentRevisionWithContext(ctx, id)
}

func (r *MessageRepository) ConversationContentSnapshotWithContext(ctx context.Context, id string) (string, error) {
	return r.GetConversationContentRevisionWithContext(ctx, id)
}

// GetConversationContentRevisionWithContext retorna apenas um digest opaco;
// nenhuma mensagem ou payload técnico sai do repository. A transação garante
// que todas as tabelas pertencem ao mesmo snapshot SQLite.
func GetConversationContentRevisionWithContext(ctx context.Context, id string) (string, error) {
	return NewMessageRepository(db).GetConversationContentRevisionWithContext(ctx, id)
}

func (r *MessageRepository) GetConversationContentRevisionWithContext(ctx context.Context, id string) (string, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return "", err
	}
	var revision string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		revision, err = conversationContentRevisionTx(ctx, tx, id)
		return err
	})
	return revision, err
}

// ClearConversationContentIfUnchangedWithinLifecycleWithContext exige que o
// caller mantenha WithConversationLifecycle até terminar os efeitos pós-commit.
// Não readquire esse gate não reentrante. BEGIN IMMEDIATE impede escritores
// concorrentes entre o snapshot comparado e a limpeza.
func ClearConversationContentIfUnchangedWithinLifecycleWithContext(ctx context.Context, id, expectedRevision string) error {
	return NewMessageRepository(db).ClearConversationContentIfUnchangedWithinLifecycleWithContext(ctx, id, expectedRevision)
}

func (r *MessageRepository) ClearConversationContentIfUnchangedWithinLifecycleWithContext(ctx context.Context, id, expectedRevision string) error {
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	return WithSQLiteMaintenance(ctx, func() error {
		return withSQLiteImmediateTransaction(ctx, r.db, "conversation.clear_unchanged", func(tx *gorm.DB) error {
			revision, err := conversationContentRevisionTx(ctx, tx, id)
			if err != nil {
				return err
			}
			if expectedRevision == "" || revision != expectedRevision {
				return ErrConversationContentChanged
			}
			return clearConversationContentTx(ctx, tx, id)
		})
	})
}

func conversationContentRevisionTx(ctx context.Context, tx *gorm.DB, id string) (string, error) {
	userID, err := RequireUserID(ctx)
	if err != nil {
		return "", err
	}
	if _, err := NewConversationRepository(tx).GetConversationInfoWithContext(ctx, id); err != nil {
		return "", err
	}
	// Inclui todas as colunas persistidas (inclusive payloads e timestamps):
	// count/max(id)/updated_at isolados não detectam edições de conteúdo.
	queries := []*gorm.DB{
		tx.Table("conversations").Where("id = ? AND user_id = ?", id, userID),
		tx.Table("chat_messages").Where("conversation_id = ?", id),
	}
	hasInvocations, err := sqliteTableExists(ctx, tx, &ToolInvocation{})
	if err != nil {
		return "", err
	}
	if hasInvocations {
		messageIDs := tx.Model(&ChatMessage{}).Select("id").Where("conversation_id = ?", id)
		turnIDs := tx.Model(&ChatMessage{}).Select("turn_id").Where("conversation_id = ? AND turn_id IS NOT NULL AND turn_id <> ''", id)
		queries = append(queries, tx.Table("tool_invocations").Where(
			"user_id = ? AND origin_type = ? AND (conversation_id = ? OR (conversation_id IS NULL AND (origin_id IN (?) OR origin_id IN (?))))",
			userID, "chat", id, messageIDs, turnIDs))
	}
	hash := sha256.New()
	encoder := json.NewEncoder(hash)
	for index, query := range queries {
		var rows []map[string]interface{}
		if err := query.WithContext(ctx).Order("id ASC").Find(&rows).Error; err != nil {
			return "", err
		}
		if err := encoder.Encode(struct {
			Table int
			Rows  []map[string]interface{}
		}{index, rows}); err != nil {
			return "", fmt.Errorf("hash conversation snapshot: %w", err)
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
