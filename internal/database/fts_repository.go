package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// FTSRepository encapsula busca full-text (FTS5) e busca em conteudo de mensagens com um *gorm.DB injetado.
type FTSRepository struct {
	db *gorm.DB
}

// NewFTSRepository cria um FTSRepository com o *gorm.DB injetado.
func NewFTSRepository(database *gorm.DB) *FTSRepository {
	return &FTSRepository{db: database}
}

// MessageSearchResult representa um resultado de busca no conteúdo de mensagens
type MessageSearchResult struct {
	ConversationID    string    `json:"conversation_id"`
	ConversationTitle string    `json:"conversation_title"`
	MessageID         string    `json:"message_id"`
	Role              string    `json:"role"`
	Snippet           string    `json:"snippet"`
	Rank              float64   `json:"rank"`
	CreatedAt         time.Time `json:"created_at"`
}

// initFTS5 cria a tabela FTS5 e triggers de sincronização.
// Idempotente — pode ser chamada múltiplas vezes sem efeito.
func initFTS5() error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("erro ao obter sql.DB: %w", err)
	}

	stmts := []string{
		// Tabela FTS5 virtual (content-sync externo via triggers)
		`CREATE VIRTUAL TABLE IF NOT EXISTS chat_messages_fts USING fts5(
			content,
			role UNINDEXED,
			conversation_id UNINDEXED,
			content='chat_messages',
			content_rowid='rowid',
			tokenize='unicode61 remove_diacritics 2'
		)`,

		// Trigger INSERT: indexa apenas user e assistant
		`CREATE TRIGGER IF NOT EXISTS chat_messages_fts_insert AFTER INSERT ON chat_messages
		WHEN NEW.role IN ('user', 'assistant')
		BEGIN
			INSERT INTO chat_messages_fts(rowid, content, role, conversation_id)
			VALUES (NEW.rowid, NEW.content, NEW.role, NEW.conversation_id);
		END`,

		// Trigger DELETE: remove do índice
		`CREATE TRIGGER IF NOT EXISTS chat_messages_fts_delete AFTER DELETE ON chat_messages
		WHEN OLD.role IN ('user', 'assistant')
		BEGIN
			INSERT INTO chat_messages_fts(chat_messages_fts, rowid, content, role, conversation_id)
			VALUES ('delete', OLD.rowid, OLD.content, OLD.role, OLD.conversation_id);
		END`,

		// Trigger UPDATE: atualiza no índice
		`CREATE TRIGGER IF NOT EXISTS chat_messages_fts_update AFTER UPDATE OF content ON chat_messages
		WHEN NEW.role IN ('user', 'assistant')
		BEGIN
			INSERT INTO chat_messages_fts(chat_messages_fts, rowid, content, role, conversation_id)
			VALUES ('delete', OLD.rowid, OLD.content, OLD.role, OLD.conversation_id);
			INSERT INTO chat_messages_fts(rowid, content, role, conversation_id)
			VALUES (NEW.rowid, NEW.content, NEW.role, NEW.conversation_id);
		END`,
	}

	for _, stmt := range stmts {
		if _, err := sqlDB.Exec(stmt); err != nil {
			return fmt.Errorf("erro FTS5 setup: %w\nSQL: %s", err, stmt)
		}
	}

	return nil
}

// RebuildFTSIndex reconstrói o índice FTS5 a partir das mensagens existentes.
// Limpa o índice e repovoa apenas com mensagens de user/assistant.
//
// Aceita ctx para permitir cancelamento via timeout/Cancel ao caller, mesmo
// que a operação seja instance-wide e não filtre por userID (Minor I do
// re-review do AEP-0052: simetria com o resto da API *WithContext).
//
// SECURITY: instance-wide — opera sobre o índice FTS global, sem filtro
// de userID. O entry point Wails (App.RebuildSearchIndex) exige sessão
// autenticada antes de chamar (ver internal/app/db.go), garantindo que
// nenhum disparo aconteça pré-login mesmo sendo uma operação de banco
// que ignora o escopo.
func RebuildFTSIndex(ctx context.Context) error {
	return NewFTSRepository(db).RebuildFTSIndex(ctx)
}

func (r *FTSRepository) RebuildFTSIndex(ctx context.Context) error {
	db := r.db
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	if _, err := sqlDB.ExecContext(ctx, `INSERT INTO chat_messages_fts(chat_messages_fts) VALUES('delete-all')`); err != nil {
		return fmt.Errorf("erro ao limpar FTS: %w", err)
	}

	if _, err := sqlDB.ExecContext(ctx, `
		INSERT INTO chat_messages_fts(rowid, content, role, conversation_id)
		SELECT rowid, content, role, conversation_id
		FROM chat_messages
		WHERE role IN ('user', 'assistant') AND content != ''
	`); err != nil {
		return fmt.Errorf("erro ao repopular FTS: %w", err)
	}

	return nil
}

// SearchMessageContentWithContext busca no conteúdo das mensagens das
// conversas do usuário do contexto usando FTS5 + BM25. query suporta sintaxe
// FTS5: palavras, "frases exatas", prefixo*, operadores OR/AND/NOT. Retorna
// até `limit` resultados ranqueados por relevância.
//
// SECURITY: fail-closed. Sem userID no ctx, retorna ErrUserScopeRequired —
// FTS5 indexa todas as conversas do banco e a junção com `conversations`
// só protege se filtrarmos por user_id obrigatoriamente. AEP-0052.
func SearchMessageContentWithContext(ctx context.Context, query string, limit int) ([]MessageSearchResult, error) {
	return NewFTSRepository(db).SearchMessageContentWithContext(ctx, query, limit)
}

func (r *FTSRepository) SearchMessageContentWithContext(ctx context.Context, query string, limit int) ([]MessageSearchResult, error) {
	db := r.db
	userID, err := RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var results []MessageSearchResult

	baseSQL := `
		SELECT
			m.conversation_id,
			c.title AS conversation_title,
			m.id AS message_id,
			fts.role,
			snippet(chat_messages_fts, 0, '>>>', '<<<', '...', 48) AS snippet,
			bm25(chat_messages_fts) AS rank,
			m.created_at
		FROM chat_messages_fts fts
		JOIN chat_messages m ON m.rowid = fts.rowid
		JOIN conversations c ON c.id = m.conversation_id
		WHERE chat_messages_fts MATCH ?
		  AND c.user_id = ?
	`
	args := []interface{}{query, userID}
	baseSQL += `
		ORDER BY bm25(chat_messages_fts)
		LIMIT ?
	`
	args = append(args, limit)

	err = db.WithContext(ctx).Raw(baseSQL, args...).Scan(&results).Error

	if err != nil {
		if strings.Contains(err.Error(), "fts5: syntax error") || strings.Contains(err.Error(), "no such column") {
			escapedQuery := `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
			args[0] = escapedQuery
			err = db.WithContext(ctx).Raw(baseSQL, args...).Scan(&results).Error
			if err != nil {
				return nil, fmt.Errorf("erro na busca FTS5: %w", err)
			}
		} else {
			return nil, fmt.Errorf("erro na busca FTS5: %w", err)
		}
	}

	return results, nil
}
