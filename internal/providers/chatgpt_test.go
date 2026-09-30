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

func TestChatGPTCreateRollbackDoesNotLeaveOrphan(t *testing.T) {
	s, _, ctx := chatGPTTestService(t)
	db := database.DB()
	if err := db.Exec("CREATE TRIGGER reject_provider_insert BEFORE INSERT ON llm_providers BEGIN SELECT RAISE(ABORT, 'insert denied'); END").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateChatGPTConnection(ctx, "Failed create"); err == nil {
		t.Fatal("expected failure")
	}
	var count int64
	if err := db.Model(&database.CredentialEntry{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("orphan credentials: %d %v", count, err)
	}
	if len(s.registry.List()) != 0 {
		t.Fatal("uncommitted provider in registry")
	}
	if err := db.Exec("DROP TRIGGER reject_provider_insert").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateChatGPTConnection(ctx, "Retry"); err != nil {
		t.Fatal(err)
	}
}
func TestImportedChatGPTCreateRollbackPreservesReference(t *testing.T) {
	s, mgr, ctx := chatGPTTestService(t)
	p := &llm.ProviderConfig{ID: "imported", Name: "Imported", Type: llm.ProviderChatGPT, BaseURL: "https://api.openai.com/v1", APIFormat: llm.APIFormatOpenAIResponses, CredentialPattern: "oauth:foreign", AuthMode: llm.AuthModeRequired}
	if err := s.store.Save(ctx, []*llm.ProviderConfig{p}); err != nil {
		t.Fatal(err)
	}
	db := database.DB()
	if err := db.Exec("CREATE TRIGGER reject_provider_update BEFORE UPDATE ON llm_providers BEGIN SELECT RAISE(ABORT, 'update denied'); END").Error; err != nil {
		t.Fatal(err)
	}
	store, err := mgr.OAuthStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ensureChatGPTAuthorization(ctx, store, p); err == nil {
		t.Fatal("expected failure")
	}
	var count int64
	if err = db.Model(&database.CredentialEntry{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("orphan credentials: %d %v", count, err)
	}
	saved, err := s.store.Get(ctx, p.ID)
	if err != nil || saved.CredentialPattern != "oauth:foreign" {
		t.Fatal("import reference changed on failure", err)
	}
}

func TestImportedChatGPTRejectsStaleRecovery(t *testing.T) {
	s, mgr, ctx := chatGPTTestService(t)
	p := &llm.ProviderConfig{ID: "imported", Name: "Imported", Type: llm.ProviderChatGPT, BaseURL: "https://api.openai.com/v1", APIFormat: llm.APIFormatOpenAIResponses, CredentialPattern: "oauth:foreign", AuthMode: llm.AuthModeRequired}
	if err := s.store.Save(ctx, []*llm.ProviderConfig{p}); err != nil {
		t.Fatal(err)
	}
	store, err := mgr.OAuthStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	winner, err := s.ensureChatGPTAuthorization(ctx, store, p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ensureChatGPTAuthorization(ctx, store, p); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatal("stale recovery accepted", err)
	}
	var count int64
	if err = database.DB().Model(&database.CredentialEntry{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("credentials=%d err=%v", count, err)
	}
	saved, err := s.store.Get(ctx, p.ID)
	if err != nil || saved.CredentialPattern != "oauth:"+winner {
		t.Fatal("winner overwritten", err)
	}
}
func TestChatGPTDefaultModelSaveFailurePreservesConnection(t *testing.T) {
	s, mgr, ctx := chatGPTTestService(t)
	created, err := s.CreateChatGPTConnection(ctx, "Connected")
	if err != nil {
		t.Fatal(err)
	}
	store, _ := mgr.OAuthStore(ctx)
	r, _ := store.Load(ctx, created.ID)
	r.State = "connected"
	r.Revision++
	if err = store.CompareAndSwap(ctx, r, r.Revision-1); err != nil {
		t.Fatal(err)
	}
	if err = database.DB().Exec("CREATE TRIGGER reject_default BEFORE UPDATE ON llm_providers BEGIN SELECT RAISE(ABORT, 'write denied'); END").Error; err != nil {
		t.Fatal(err)
	}
	s.setChatGPTDefaultModel(ctx, s.registry.Get(created.ID), "account-model")
	summary, err := s.ChatGPTConnection(ctx, created.ID)
	if err != nil || summary.State != "connected" {
		t.Fatal("optional default invalidated connection", err)
	}
	if s.registry.Get(created.ID).DefaultModel != "" {
		t.Fatal("unpersisted default published")
	}
}
