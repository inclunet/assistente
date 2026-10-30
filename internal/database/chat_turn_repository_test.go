package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newChatTurnTestDB(t *testing.T, file bool) *gorm.DB {
	t.Helper()
	name := ":memory:"
	if file {
		name = filepath.Join(t.TempDir(), "turns.db")
	}
	db, err := gorm.Open(sqlite.Open(name), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.Exec("PRAGMA foreign_keys=ON").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Conversation{}, &ChatMessage{}, &ToolCatalog{}, &ToolInvocation{}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"user", "other-user"} {
		if err := db.Create(&User{UUIDModel: UUIDModel{ID: id}, Username: id}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, conv := range []Conversation{
		{UUIDModel: UUIDModel{ID: "conv"}, UserID: "user"},
		{UUIDModel: UUIDModel{ID: "other"}, UserID: "other-user"},
	} {
		if err := db.Create(&conv).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func seedChatTurnTestMessages(t *testing.T, db *gorm.DB, conv, turn, response string) {
	t.Helper()
	if err := db.Create(&ChatMessage{UUIDModel: UUIDModel{ID: turn}, ConversationID: conv, Role: "user"}).Error; err != nil {
		t.Fatal(err)
	}
	if response != "" {
		if err := db.Create(&ChatMessage{UUIDModel: UUIDModel{ID: response}, ConversationID: conv, TurnID: &turn, Role: "assistant", Content: "antes"}).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func TestChatTurnLifecycleAndStaleTerminal(t *testing.T) {
	db := newChatTurnTestDB(t, false)
	seedChatTurnTestMessages(t, db, "conv", " turn ", " response ")
	if err := migrateChatTurns(db); err != nil {
		t.Fatal(err)
	}
	r := NewChatTurnRepository(db)
	ctx := WithUserID(context.Background(), "user")
	first, err := r.Begin(ctx, "conv", " turn ", " response ", TurnModeNew)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Begin(ctx, "conv", " turn ", "", TurnModeRetry); !errors.Is(err, ErrChatTurnActive) {
		t.Fatalf("segunda execução: %v", err)
	}
	for range 2 {
		if err = r.MarkStreaming(ctx, "conv", " turn ", first.ExecutionID); err != nil {
			t.Fatal(err)
		}
	}
	if err = r.Finish(ctx, "conv", " turn ", first.ExecutionID, TurnCompleted, "completed"); err != nil {
		t.Fatal(err)
	}
	second, err := r.Begin(ctx, "conv", " turn ", "", TurnModeContinue)
	if err != nil {
		t.Fatal(err)
	}
	if first.ExecutionID == second.ExecutionID {
		t.Fatal("tentativa reutilizou executionId")
	}
	if err = r.Finish(ctx, "conv", " turn ", first.ExecutionID, TurnFailed, "provider_error"); !errors.Is(err, ErrChatTurnStale) {
		t.Fatalf("terminal atrasado aceito: %v", err)
	}
	turn, err := r.Get(ctx, "conv", " turn ")
	if err != nil || turn.ActiveExecutionID == nil || *turn.ActiveExecutionID != second.ExecutionID || turn.State != TurnPreparing || *turn.ResponseMessageID != " response " {
		t.Fatalf("retry alterado: %+v %v", turn, err)
	}
	if err = r.Finish(ctx, "conv", " turn ", second.ExecutionID, TurnFailed, "persistence_error"); err != nil {
		t.Fatal(err)
	}
	if err = r.Finish(ctx, "conv", " turn ", second.ExecutionID, TurnFailed, "persistence_error"); !errors.Is(err, ErrChatTurnStale) {
		t.Fatalf("terminal repetido aceito: %v", err)
	}
	if _, err = r.Begin(ctx, "conv", " turn ", "", TurnModeNew); !errors.Is(err, ErrChatTurnInvalid) {
		t.Fatalf("new reutilizou turno executado: %v", err)
	}
}

func TestChatTurnScopeAndLinkConstraints(t *testing.T) {
	db := newChatTurnTestDB(t, false)
	seedChatTurnTestMessages(t, db, "conv", "turn", "response")
	seedChatTurnTestMessages(t, db, "other", "other-turn", "other-response")
	if err := migrateChatTurns(db); err != nil {
		t.Fatal(err)
	}
	r := NewChatTurnRepository(db)
	ctx := WithUserID(context.Background(), "user")
	for _, response := range []string{"other-response", "turn", "missing"} {
		if _, err := r.Begin(ctx, "conv", "turn", response, TurnModeRetry); err == nil {
			t.Fatalf("vínculo inválido aceito: %q", response)
		}
	}
	if _, err := r.Get(context.Background(), "conv", "turn"); !errors.Is(err, ErrUserScopeRequired) {
		t.Fatalf("sem owner: %v", err)
	}
	if _, err := r.Begin(ctx, "other", "other-turn", "other-response", TurnModeRetry); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("outro owner: %v", err)
	}
	if _, err := r.Get(ctx, "other", "other-turn"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("leitura outro owner: %v", err)
	}
	for _, statement := range []string{
		`UPDATE chat_turns SET response_message_id='other-response' WHERE user_message_id='turn'`,
		`UPDATE chat_turns SET state='completed',active_execution_id='missing' WHERE user_message_id='turn'`,
		`UPDATE chat_messages SET conversation_id='other' WHERE id='response'`,
		`UPDATE chat_messages SET role='assistant' WHERE id='turn'`,
	} {
		if err := db.Exec(statement).Error; err == nil {
			t.Fatalf("SQL inválido aceito: %s", statement)
		}
	}
	var count int64
	if err := db.Model(&ChatTurnAttempt{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("falha deixou tentativa: %d %v", count, err)
	}
}

func TestChatTurnFinishSharesResponseTransaction(t *testing.T) {
	db := newChatTurnTestDB(t, false)
	seedChatTurnTestMessages(t, db, "conv", "turn", "response")
	if err := migrateChatTurns(db); err != nil {
		t.Fatal(err)
	}
	ctx := WithUserID(context.Background(), "user")
	r := NewChatTurnRepository(db)
	attempt, err := r.Begin(ctx, "conv", "turn", "response", TurnModeRetry)
	if err != nil {
		t.Fatal(err)
	}
	abort := errors.New("falha posterior")
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&ChatMessage{}).Where("id='response'").Update("content", "depois").Error; err != nil {
			return err
		}
		if err := NewChatTurnRepository(tx).Finish(ctx, "conv", "turn", attempt.ExecutionID, TurnCompleted, "completed"); err != nil {
			return err
		}
		return abort
	})
	if !errors.Is(err, abort) {
		t.Fatal(err)
	}
	turn, err := r.Get(ctx, "conv", "turn")
	if err != nil || turn.State != TurnPreparing {
		t.Fatalf("terminal escapou rollback: %+v %v", turn, err)
	}
	var message ChatMessage
	if err := db.First(&message, "id='response'").Error; err != nil || message.Content != "antes" {
		t.Fatalf("resposta escapou rollback: %+v %v", message, err)
	}
}

func TestChatTurnConcurrentBeginRecoveryAndCascade(t *testing.T) {
	db := newChatTurnTestDB(t, false)
	seedChatTurnTestMessages(t, db, "conv", "turn", "response")
	seedChatTurnTestMessages(t, db, "other", "other-turn", "other-response")
	if err := migrateChatTurns(db); err != nil {
		t.Fatal(err)
	}
	ctx := WithUserID(context.Background(), "user")
	r := NewChatTurnRepository(db)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := r.Begin(ctx, "conv", "turn", "response", TurnModeRetry)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	winners := 0
	for err := range errs {
		if err == nil {
			winners++
		} else if !errors.Is(err, ErrChatTurnActive) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("%d vencedores", winners)
	}
	otherCtx := WithUserID(context.Background(), "other-user")
	if _, err := r.Begin(otherCtx, "other", "other-turn", "other-response", TurnModeRetry); err != nil {
		t.Fatal(err)
	}
	if err := r.RecoverInterruptedBeforeStartup(ctx); err != nil {
		t.Fatal(err)
	}
	turn, err := r.Get(ctx, "conv", "turn")
	if err != nil || turn.State != TurnInterrupted || turn.ActiveExecutionID != nil {
		t.Fatalf("recuperação: %+v %v", turn, err)
	}
	other, err := r.Get(otherCtx, "other", "other-turn")
	if err != nil || other.ActiveExecutionID == nil {
		t.Fatalf("outro usuário recuperado: %+v %v", other, err)
	}
	if err := db.Delete(&ChatMessage{}, "id='turn'").Error; err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&ChatTurnAttempt{}).Where("turn_id='turn'").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("tentativa órfã: %d %v", count, err)
	}
	// Exclusão em execução também remove a FK circular entre turno e tentativa.
	if err := db.Delete(&ChatMessage{}, "id='other-turn'").Error; err != nil {
		t.Fatal(err)
	}
}

func TestMigrateChatTurnsBackfillAndIdempotency(t *testing.T) {
	db := newChatTurnTestDB(t, false)
	seedChatTurnTestMessages(t, db, "conv", "turn", "final")
	seedChatTurnTestMessages(t, db, "conv", "unanswered", "")
	if err := db.Model(&ChatMessage{}).Where("id='final'").Updates(map[string]any{"created_at": time.Unix(1, 0), "updated_at": time.Unix(1, 0), "total_tokens": 99}).Error; err != nil {
		t.Fatal(err)
	}
	turnID := "turn"
	intermediate := ChatMessage{UUIDModel: UUIDModel{ID: "intermediate", CreatedAt: time.Unix(2, 0), UpdatedAt: time.Unix(3, 0)}, ConversationID: "conv", TurnID: &turnID, Role: "assistant"}
	if err := db.Create(&intermediate).Error; err != nil {
		t.Fatal(err)
	}
	catalog := ToolCatalog{Name: "echo", Origin: "builtin"}
	if err := db.Create(&catalog).Error; err != nil {
		t.Fatal(err)
	}
	inv := ToolInvocation{UserID: "user", ToolCatalogID: catalog.ID, OriginType: "chat", OriginID: turnID, TurnID: &turnID, ToolCallID: "call", Status: "succeeded", Metadata: `{"display":{"assistant_message_id":"intermediate"}}`}
	if err := db.Create(&inv).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := migrateChatTurns(db); err != nil {
			t.Fatal(err)
		}
	}
	r := NewChatTurnRepository(db)
	ctx := WithUserID(context.Background(), "user")
	turn, err := r.Get(ctx, "conv", "turn")
	if err != nil || turn.State != TurnHistoricalUnknown || turn.Origin != "legacy" || turn.ActiveExecutionID != nil || turn.ResponseMessageID == nil || *turn.ResponseMessageID != "final" {
		t.Fatalf("backfill: %+v %v", turn, err)
	}
	unanswered, err := r.Get(ctx, "conv", "unanswered")
	if err != nil || unanswered.ResponseMessageID != nil {
		t.Fatalf("inventou resposta: %+v %v", unanswered, err)
	}
	var count int64
	if err := db.Model(&ChatTurnAttempt{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("inventou tentativa: %d %v", count, err)
	}
}

func TestMigrateChatTurnsBackupAndBatches(t *testing.T) {
	db := newChatTurnTestDB(t, true)
	for i := range 130 {
		seedChatTurnTestMessages(t, db, "conv", fmt.Sprintf("turn-%03d", i), "")
	}
	if err := migrateChatTurns(db); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&ChatTurn{}).Count(&count).Error; err != nil || count != 130 {
		t.Fatalf("lotes: %d %v", count, err)
	}
	sqlDB, _ := db.DB()
	path, err := sqliteMainDatabasePath(sqlDB)
	if err != nil {
		t.Fatal(err)
	}
	backup := path + ".pre-chat-turns-v41.bak"
	digest, size, err := fileSHA256(backup)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(backup + ".manifest")
	if err != nil || string(manifest) != fmt.Sprintf("sha256=%s\nbytes=%d\n", digest, size) {
		t.Fatalf("manifesto: %s %v", manifest, err)
	}
	restored, err := gorm.Open(sqlite.Open(backup), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	restoredSQL, _ := restored.DB()
	defer func() { _ = restoredSQL.Close() }()
	if restored.Migrator().HasTable("chat_turns") {
		t.Fatal("backup não é anterior à migração")
	}
	if err := restored.Model(&ChatMessage{}).Count(&count).Error; err != nil || count != 130 {
		t.Fatalf("restauração: %d %v", count, err)
	}
}

func TestMigrateChatTurnsPreservesOrphanOutsideTurnMetadata(t *testing.T) {
	db := newChatTurnTestDB(t, false)
	seedChatTurnTestMessages(t, db, "conv", "valid", "response")
	// Simula corrupção legada que não pode satisfazer a FK da nova tabela.
	if err := db.Exec("PRAGMA foreign_keys=OFF").Error; err != nil {
		t.Fatal(err)
	}
	seedChatTurnTestMessages(t, db, "missing", "orphan", "")
	if err := db.Exec("PRAGMA foreign_keys=ON").Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateChatTurns(db); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&ChatMessage{}).Count(&count).Error; err != nil || count != 3 {
		t.Fatalf("histórico alterado: %d %v", count, err)
	}
	var turns []ChatTurn
	if err := db.Find(&turns).Error; err != nil || len(turns) != 1 || turns[0].UserMessageID != "valid" {
		t.Fatalf("metadados incorretos: %+v %v", turns, err)
	}
}

func TestMigrateChatTurnsRollsBackSchemaAndPreservesHistory(t *testing.T) {
	db := newChatTurnTestDB(t, false)
	seedChatTurnTestMessages(t, db, "conv", "turn", "response")
	// Falha real de leitura: não existe o ledger exigido pela seleção histórica.
	if err := db.Exec("DROP TABLE tool_invocations").Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateChatTurns(db); err == nil {
		t.Fatal("ignorou erro de leitura")
	}
	if db.Migrator().HasTable("chat_turns") || db.Migrator().HasTable("chat_turn_attempts") {
		t.Fatal("DDL parcial persistido")
	}
	var count int64
	if err := db.Model(&ChatMessage{}).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("histórico alterado: %d %v", count, err)
	}
}

func TestMigrateChatTurnsRejectsForeignKeyCorruption(t *testing.T) {
	db := newChatTurnTestDB(t, false)
	if err := createChatTurnSchema(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("PRAGMA foreign_keys=OFF").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&ChatTurnAttempt{ExecutionID: "orphan", TurnID: "missing", Mode: TurnModeNew, State: TurnPreparing, StartedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("PRAGMA foreign_keys=ON").Error; err != nil {
		t.Fatal(err)
	}
	seedChatTurnTestMessages(t, db, "conv", "turn", "response")
	if err := migrateChatTurns(db); err == nil {
		t.Fatal("ignorou corrupção de FK")
	}
	var count int64
	if err := db.Model(&ChatTurn{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("backfill parcial persistido: %d %v", count, err)
	}
}

func TestMigrateChatTurnsNativeOriginAndRetrySelection(t *testing.T) {
	for _, origin := range []string{"", "\t", "\n", "\u00a0", "mcp_native"} {
		t.Run(fmt.Sprintf("origin-%q", origin), func(t *testing.T) {
			db := newChatTurnTestDB(t, false)
			seedChatTurnTestMessages(t, db, "conv", "turn", "final")
			turn := "turn"
			old := ChatMessage{UUIDModel: UUIDModel{ID: "old", CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(1, 0)}, ConversationID: "conv", TurnID: &turn, Role: "assistant"}
			if err := db.Create(&old).Error; err != nil {
				t.Fatal(err)
			}
			catalog := ToolCatalog{Name: "native", Origin: "mcp_native"}
			if err := db.Create(&catalog).Error; err != nil {
				t.Fatal(err)
			}
			// Tentativa antiga chega depois no relógio e marcaria final como intermediário.
			for attempt := 1; attempt <= 2; attempt++ {
				metadataOrigin := "builtin"
				if attempt == 2 {
					metadataOrigin = origin
				}
				metadata, _ := json.Marshal(map[string]any{"display": map[string]any{"assistant_message_id": "final", "origin": metadataOrigin}})
				inv := ToolInvocation{UserID: "user", ToolCatalogID: catalog.ID, OriginType: "chat", TurnID: &turn, ToolCallID: "call", Attempt: attempt, Status: "succeeded", Metadata: string(metadata), QueuedAt: time.Unix(int64(3-attempt), 0)}
				if err := db.Create(&inv).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := migrateChatTurns(db); err != nil {
				t.Fatal(err)
			}
			got, err := NewChatTurnRepository(db).Get(WithUserID(context.Background(), "user"), "conv", turn)
			if err != nil || got.ResponseMessageID == nil || *got.ResponseMessageID != "final" {
				t.Fatalf("representante: %+v %v", got, err)
			}
		})
	}
}
