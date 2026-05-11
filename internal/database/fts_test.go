package database

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTestDBWithFTS(t *testing.T) {
	t.Helper()
	var err error
	db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	if err := db.AutoMigrate(&Conversation{}, &ChatMessage{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	if err := initFTS5(); err != nil {
		t.Fatalf("failed to init FTS5: %v", err)
	}

	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		db = nil
	})
}

func seedTestMessages(t *testing.T) (conv1ID, conv2ID string) {
	t.Helper()

	conv1 := &Conversation{Title: "Projeto autenticação JWT", UserID: testUserID}
	db.Create(conv1)
	conv2 := &Conversation{Title: "Deploy Kubernetes", UserID: testUserID}
	db.Create(conv2)

	msgs := []ChatMessage{
		{UUIDModel: UUIDModel{CreatedAt: time.Now().Add(-5 * time.Hour)}, ConversationID: conv1.ID, Role: "user", Content: "Como implementar autenticação JWT no nosso backend Go?"},
		{UUIDModel: UUIDModel{CreatedAt: time.Now().Add(-4 * time.Hour)}, ConversationID: conv1.ID, Role: "assistant", Content: "Para implementar JWT em Go, recomendo usar a biblioteca golang-jwt. Primeiro crie um middleware que valide o token no header Authorization."},
		{UUIDModel: UUIDModel{CreatedAt: time.Now().Add(-3 * time.Hour)}, ConversationID: conv1.ID, Role: "user", Content: "E o refresh token, como fazemos?"},
		{UUIDModel: UUIDModel{CreatedAt: time.Now().Add(-2 * time.Hour)}, ConversationID: conv1.ID, Role: "assistant", Content: "O refresh token deve ter expiração mais longa. Armazene no banco com rotação automática. Quando o access token expirar, o client envia o refresh token para obter um novo par."},
		{UUIDModel: UUIDModel{CreatedAt: time.Now().Add(-1 * time.Hour)}, ConversationID: conv1.ID, Role: "tool", Content: "resultado da tool: arquivo lido com sucesso"},
		{UUIDModel: UUIDModel{CreatedAt: time.Now().Add(-6 * time.Hour)}, ConversationID: conv2.ID, Role: "user", Content: "Preciso fazer deploy da aplicação no Kubernetes com rolling update"},
		{UUIDModel: UUIDModel{CreatedAt: time.Now().Add(-5 * time.Hour)}, ConversationID: conv2.ID, Role: "assistant", Content: "Para rolling update no Kubernetes, configure a strategy no Deployment YAML com maxSurge e maxUnavailable. Use readiness probes para garantir que os pods estejam prontos."},
	}
	for i := range msgs {
		if err := db.Create(&msgs[i]).Error; err != nil {
			t.Fatalf("failed to create message: %v", err)
		}
	}
	return conv1.ID, conv2.ID
}

func TestSearchMessageContent_BasicSearch(t *testing.T) {
	setupTestDBWithFTS(t)
	seedTestMessages(t)

	results, err := SearchMessageContentWithContext(testCtx(), "JWT", 20)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results for 'JWT', got none")
	}
	for _, r := range results {
		if r.Role != "user" && r.Role != "assistant" {
			t.Errorf("unexpected role: %s (should be user or assistant)", r.Role)
		}
	}
}

func TestSearchMessageContent_IgnoresToolMessages(t *testing.T) {
	setupTestDBWithFTS(t)
	seedTestMessages(t)

	results, err := SearchMessageContentWithContext(testCtx(), "resultado da tool", 20)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("tool messages should not be indexed, got %d results", len(results))
	}
}

func TestSearchMessageContent_RankedByRelevance(t *testing.T) {
	setupTestDBWithFTS(t)
	seedTestMessages(t)

	results, err := SearchMessageContentWithContext(testCtx(), "autenticação", 20)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results for 'autenticação'")
	}
	if results[0].ConversationTitle != "Projeto autenticação JWT" {
		t.Errorf("expected first result from JWT conversation, got: %s", results[0].ConversationTitle)
	}
}

func TestSearchMessageContent_PhraseSearch(t *testing.T) {
	setupTestDBWithFTS(t)
	seedTestMessages(t)

	results, err := SearchMessageContentWithContext(testCtx(), `"rolling update"`, 20)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results for phrase 'rolling update'")
	}
	if results[0].ConversationTitle != "Deploy Kubernetes" {
		t.Errorf("expected result from Kubernetes conversation, got: %s", results[0].ConversationTitle)
	}
}

func TestSearchMessageContent_EmptyQuery(t *testing.T) {
	setupTestDBWithFTS(t)
	seedTestMessages(t)

	results, err := SearchMessageContentWithContext(testCtx(), "", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Error("empty query should return nil")
	}
}

func TestSearchMessageContent_NoResults(t *testing.T) {
	setupTestDBWithFTS(t)
	seedTestMessages(t)

	results, err := SearchMessageContentWithContext(testCtx(), "blockchain", 20)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected no results for 'blockchain', got %d", len(results))
	}
}

func TestSearchMessageContent_FallbackOnSyntaxError(t *testing.T) {
	setupTestDBWithFTS(t)
	seedTestMessages(t)

	// Caracteres que podem causar erro de sintaxe no FTS5
	results, err := SearchMessageContentWithContext(testCtx(), "Go?", 20)
	if err != nil {
		t.Fatalf("should fallback gracefully, got error: %v", err)
	}
	// Pode ou não ter resultados, mas não deve dar erro
	_ = results
}

func TestSearchMessageContent_LimitRespected(t *testing.T) {
	setupTestDBWithFTS(t)
	seedTestMessages(t)

	results, err := SearchMessageContentWithContext(testCtx(), "token", 2)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) > 2 {
		t.Errorf("expected at most 2 results, got %d", len(results))
	}
}

func TestSearchMessageContentWithContextScopesByUser(t *testing.T) {
	setupTestDBWithFTS(t)

	anaConv := &Conversation{Title: "Ana auth", UserID: "user-ana"}
	if err := db.Create(anaConv).Error; err != nil {
		t.Fatalf("create ana conversation: %v", err)
	}
	leoConv := &Conversation{Title: "Leo auth", UserID: "user-leo"}
	if err := db.Create(leoConv).Error; err != nil {
		t.Fatalf("create leo conversation: %v", err)
	}
	for _, msg := range []ChatMessage{
		{ConversationID: anaConv.ID, Role: "user", Content: "shared-secret alpha"},
		{ConversationID: leoConv.ID, Role: "user", Content: "shared-secret beta"},
	} {
		if err := db.Create(&msg).Error; err != nil {
			t.Fatalf("create message: %v", err)
		}
	}

	results, err := SearchMessageContentWithContext(WithUserID(context.Background(), "user-ana"), "shared-secret", 20)
	if err != nil {
		t.Fatalf("search scoped: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one scoped result, got %d: %#v", len(results), results)
	}
	if results[0].ConversationID != anaConv.ID {
		t.Fatalf("expected ana conversation only, got %#v", results[0])
	}
}

func TestRebuildFTSIndex(t *testing.T) {
	setupTestDBWithFTS(t)
	seedTestMessages(t)

	// Busca deve funcionar (triggers popularam)
	results1, _ := SearchMessageContentWithContext(testCtx(), "JWT", 20)
	if len(results1) == 0 {
		t.Fatal("expected results before rebuild")
	}

	// Rebuild e verifica que continua funcionando
	if err := RebuildFTSIndex(context.Background()); err != nil {
		t.Fatalf("rebuild failed: %v", err)
	}

	results2, _ := SearchMessageContentWithContext(testCtx(), "JWT", 20)
	if len(results2) == 0 {
		t.Fatal("expected results after rebuild")
	}
	if len(results1) != len(results2) {
		t.Errorf("result count changed after rebuild: %d → %d", len(results1), len(results2))
	}
}

func TestFTS5_DeleteMessageRemovesFromIndex(t *testing.T) {
	setupTestDBWithFTS(t)

	conv := &Conversation{Title: "Test", UserID: testUserID}
	db.Create(conv)

	msg := &ChatMessage{ConversationID: conv.ID, Role: "user", Content: "palavra unica xyzzy123"}
	db.Create(msg)

	results, _ := SearchMessageContentWithContext(testCtx(), "xyzzy123", 20)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	db.Delete(msg)

	results, _ = SearchMessageContentWithContext(testCtx(), "xyzzy123", 20)
	if len(results) != 0 {
		t.Errorf("expected 0 results after delete, got %d", len(results))
	}
}
