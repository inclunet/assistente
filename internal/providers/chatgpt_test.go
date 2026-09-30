package providers

import (
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/oauthflow"
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func chatGPTTestService(t *testing.T) (*Service, *credentials.Manager, context.Context) {
	t.Helper()
	db := acpTestDB(t)
	if err := db.AutoMigrate(&database.CredentialEntry{}); err != nil {
		t.Fatal(err)
	}
	mgr := credentials.NewManagerWithStore(bytes.Repeat([]byte{4}, 32), credentials.NewDBStore(), true)
	service := NewService(ServiceConfig{Registry: llm.NewProviderRegistry(), CredMgr: mgr, Store: NewDBStore()})
	return service, mgr, database.WithUserID(context.Background(), "owner")
}
func TestChatGPTPendingCanDisconnectAndDelete(t *testing.T) {
	service, _, ctx := chatGPTTestService(t)
	created, err := service.CreateChatGPTConnection(ctx, "Canceled consent")
	if err != nil {
		t.Fatal(err)
	}
	if created.State != "pending" {
		t.Fatal(created.State)
	}
	confirmed, err := service.DisconnectChatGPT(ctx, created.ID)
	if err != nil || !confirmed {
		t.Fatalf("disconnect: %v", err)
	}
	if err = service.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if service.registry.Get(created.ID) != nil {
		t.Fatal("provider retained")
	}
}
func TestImportedChatGPTRecoversWithoutForeignRegistration(t *testing.T) {
	service, mgr, ctx := chatGPTTestService(t)
	imported := &llm.ProviderConfig{ID: "imported-provider", Name: "Imported ChatGPT", Type: llm.ProviderChatGPT, BaseURL: "https://api.openai.com/v1", APIFormat: llm.APIFormatOpenAIResponses, CredentialPattern: "oauth:foreign-authorization", AuthMode: llm.AuthModeRequired}
	if err := service.store.Save(ctx, []*llm.ProviderConfig{imported}); err != nil {
		t.Fatal(err)
	}
	if err := service.Load(ctx); err != nil {
		t.Fatal(err)
	}
	status, err := service.ChatGPTConnection(ctx, imported.ID)
	if err != nil || status.State != "disconnected" {
		t.Fatalf("import status: %+v %v", status, err)
	}
	store, err := mgr.OAuthStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id, err := service.ensureChatGPTAuthorization(ctx, store, imported)
	if err != nil {
		t.Fatal(err)
	}
	if id == "foreign-authorization" {
		t.Fatal("reused foreign authorization")
	}
	record, err := store.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if record.Client.ID != "" || record.Tokens.Access != "" || record.State != "pending" {
		t.Fatal("fabricated registration")
	}
	updated, err := service.store.Get(ctx, imported.ID)
	if err != nil || updated.CredentialPattern != "oauth:"+id {
		t.Fatalf("reference: %v", err)
	}
	// Repeating the explicit action reuses the newly created pending record.
	again, err := service.ensureChatGPTAuthorization(ctx, store, updated)
	if err != nil || again != id {
		t.Fatal("duplicate local registration")
	}
}
func TestImportedChatGPTWithoutAuthorizationCanBeDeleted(t *testing.T) {
	service, _, ctx := chatGPTTestService(t)
	p := &llm.ProviderConfig{ID: "imported", Name: "Imported", Type: llm.ProviderChatGPT, BaseURL: "https://api.openai.com/v1", APIFormat: llm.APIFormatOpenAIResponses, CredentialPattern: "oauth:missing", AuthMode: llm.AuthModeRequired}
	if err := service.store.Save(ctx, []*llm.ProviderConfig{p}); err != nil {
		t.Fatal(err)
	}
	if err := service.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
}

func TestChatGPTUnicodeLabel(t *testing.T) {
	s, _, ctx := chatGPTTestService(t)
	if _, err := s.CreateChatGPTConnection(ctx, strings.Repeat("é", 100)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateChatGPTConnection(ctx, strings.Repeat("é", 101)); err == nil {
		t.Fatal("accepted oversized label")
	}
}
func TestChatGPTDeleteRollbackRetainsBothRecords(t *testing.T) {
	s, mgr, ctx := chatGPTTestService(t)
	created, err := s.CreateChatGPTConnection(ctx, "Rollback")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DisconnectChatGPT(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	db := database.DB()
	if err = db.Exec("CREATE TRIGGER reject_provider_delete BEFORE DELETE ON llm_providers BEGIN SELECT RAISE(ABORT, 'delete denied'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err = s.Delete(ctx, created.ID); err == nil {
		t.Fatal("expected delete failure")
	}
	store, err := mgr.OAuthStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Load(ctx, created.ID); err != nil {
		t.Fatal("authorization lost on failed delete", err)
	}
	if p, err := s.store.Get(ctx, created.ID); err != nil || p == nil || s.registry.Get(created.ID) == nil {
		t.Fatal("provider lost on failed delete")
	}
	if err = db.Exec("DROP TRIGGER reject_provider_delete").Error; err != nil {
		t.Fatal(err)
	}
	if err = s.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Load(ctx, created.ID); !errors.Is(err, oauthflow.ErrNotFound) {
		t.Fatal("authorization retained", err)
	}
	if p, _ := s.store.Get(ctx, created.ID); p != nil {
		t.Fatal("provider retained")
	}
	if err = s.Delete(ctx, created.ID); err == nil {
		t.Fatal("second delete should fail without panic")
	}
}
