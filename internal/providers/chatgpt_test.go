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
	"time"
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
	s.setChatGPTDefaultModel(ctx, store, created.ID, s.registry.Get(created.ID), "account-model")
	summary, err := s.ChatGPTConnection(ctx, created.ID)
	if err != nil || summary.State != "connected" {
		t.Fatal("optional default invalidated connection", err)
	}
	if s.registry.Get(created.ID).DefaultModel != "" {
		t.Fatal("unpersisted default published")
	}
}

func TestChatGPTDeleteRejectsActiveReauthorization(t *testing.T) {
	s, mgr, ctx := chatGPTTestService(t)
	created, err := s.CreateChatGPTConnection(ctx, "Reauthorizing")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DisconnectChatGPT(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	key := "owner:" + created.ID
	s.oauthMu.Lock()
	s.oauthAttempts[key] = func() {}
	s.oauthMu.Unlock()
	if err = s.Delete(ctx, created.ID); err == nil || err.Error() != "chatgpt_authorization_in_progress" {
		t.Fatal("active grant deleted", err)
	}
	store, _ := mgr.OAuthStore(ctx)
	if _, err = store.Load(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if s.registry.Get(created.ID) == nil {
		t.Fatal("provider lost")
	}
	s.oauthMu.Lock()
	delete(s.oauthAttempts, key)
	s.oauthMu.Unlock()
	if err = s.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
}

func TestFirstChatGPTProviderBecomesDefault(t *testing.T) {
	s, _, ctx := chatGPTTestService(t)
	first, err := s.CreateChatGPTConnection(ctx, "First")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateChatGPTConnection(ctx, "Second")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{first.ID, second.ID} {
		persisted, err := s.store.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if persisted.IsDefault != (id == first.ID) || s.registry.Get(id).IsDefault != persisted.IsDefault {
			t.Fatalf("default mismatch for %s", id)
		}
	}
	// Another local user's providers must not affect first-provider selection.
	other := database.WithUserID(context.Background(), "other")
	created, err := s.CreateChatGPTConnection(other, "Other")
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := s.store.Get(other, created.ID)
	if err != nil || !persisted.IsDefault {
		t.Fatalf("other default: %v", err)
	}
}

func TestChatGPTDeleteRejectsChangedAuthorizationReference(t *testing.T) {
	s, mgr, ctx := chatGPTTestService(t)
	old, err := s.CreateChatGPTConnection(ctx, "Original")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DisconnectChatGPT(ctx, old.ID); err != nil {
		t.Fatal(err)
	}
	store, _ := mgr.OAuthStore(ctx)
	replacement, _ := s.oauth.Pending("replacement", "owner", "chatgpt")
	replacement.State = "disconnected"
	if err = store.Create(ctx, replacement); err != nil {
		t.Fatal(err)
	}
	// A second instance updates the persisted reference while this registry is stale.
	updated, _ := s.store.Get(ctx, old.ID)
	updated.CredentialPattern = "oauth:replacement"
	if err = s.store.Save(ctx, []*llm.ProviderConfig{updated}); err != nil {
		t.Fatal(err)
	}
	if err = s.Delete(ctx, old.ID); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("stale delete: %v", err)
	}
	for _, id := range []string{old.ID, replacement.ID} {
		if _, err = store.Load(ctx, id); err != nil {
			t.Fatalf("authorization %s lost: %v", id, err)
		}
	}
	current, err := s.store.Get(ctx, old.ID)
	if err != nil || current.CredentialPattern != "oauth:replacement" || s.registry.Get(old.ID) == nil {
		t.Fatal("consumer lost")
	}
}

type blockedChatGPTPreflight struct {
	ProviderStore
	entered chan context.Context
	release chan struct{}
}

func (s *blockedChatGPTPreflight) Get(ctx context.Context, _ string) (*llm.ProviderConfig, error) {
	s.entered <- ctx
	<-s.release
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("preflight was not canceled")
}
func TestChatGPTCancelDuringPreflight(t *testing.T) {
	s, _, ctx := chatGPTTestService(t)
	created, err := s.CreateChatGPTConnection(ctx, "Pending")
	if err != nil {
		t.Fatal(err)
	}
	blocked := &blockedChatGPTPreflight{ProviderStore: s.store, entered: make(chan context.Context, 1), release: make(chan struct{})}
	s.store = blocked
	done := make(chan error, 1)
	go func() { _, err := s.AuthorizeChatGPT(ctx, created.ID, "Return"); done <- err }()
	select {
	case <-blocked.entered:
	case <-time.After(time.Second):
		close(blocked.release)
		t.Fatal("preflight not reached")
	}
	err = s.CancelChatGPT(ctx, created.ID)
	close(blocked.release)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel lost during preflight: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("authorization survived cancel")
	}
	s.oauthMu.Lock()
	attempts := len(s.oauthAttempts)
	s.oauthMu.Unlock()
	if attempts != 0 {
		t.Fatal("attempt retained")
	}
}

func TestImportedChatGPTCanBeDeletedWithoutVault(t *testing.T) {
	s, mgr, ctx := chatGPTTestService(t)
	p := &llm.ProviderConfig{ID: "imported", Name: "Imported", Type: llm.ProviderChatGPT, BaseURL: "https://api.openai.com/v1", APIFormat: llm.APIFormatOpenAIResponses, CredentialPattern: "oauth:foreign", AuthMode: llm.AuthModeRequired}
	if err := s.store.Save(ctx, []*llm.ProviderConfig{p}); err != nil {
		t.Fatal(err)
	}
	if err := s.Load(ctx); err != nil {
		t.Fatal(err)
	}
	mgr.Reset(nil, false)
	if err := s.Delete(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if current, _ := s.store.Get(ctx, p.ID); current != nil || s.registry.Get(p.ID) != nil {
		t.Fatal("import retained")
	}
}
func TestChatGPTWithEnvelopeCannotBeDeletedWithoutVault(t *testing.T) {
	s, mgr, ctx := chatGPTTestService(t)
	p, err := s.CreateChatGPTConnection(ctx, "Account")
	if err != nil {
		t.Fatal(err)
	}
	mgr.Reset(nil, false)
	if err = s.Delete(ctx, p.ID); err == nil {
		t.Fatal("deleted unreadable authorization")
	}
	var entry database.CredentialEntry
	if err = database.DB().First(&entry, "id = ?", p.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current, _ := s.store.Get(ctx, p.ID); current == nil || s.registry.Get(p.ID) == nil {
		t.Fatal("provider lost")
	}
}

type unreadableOptionalAuthorization struct{ oauthflow.Store }

func (s unreadableOptionalAuthorization) Load(context.Context, string) (oauthflow.Record, error) {
	return oauthflow.Record{}, errors.New("temporary read failure")
}
func TestChatGPTOptionalModelSkipsUnconfirmedAuthorization(t *testing.T) {
	s, mgr, ctx := chatGPTTestService(t)
	created, err := s.CreateChatGPTConnection(ctx, "Connected")
	if err != nil {
		t.Fatal(err)
	}
	store, _ := mgr.OAuthStore(ctx)
	record, _ := store.Load(ctx, created.ID)
	record.State = "connected"
	record.Revision++
	if err = store.CompareAndSwap(ctx, record, record.Revision-1); err != nil {
		t.Fatal(err)
	}
	s.setChatGPTDefaultModel(ctx, unreadableOptionalAuthorization{store}, created.ID, s.registry.Get(created.ID), "account-model")
	status, err := s.ChatGPTConnection(ctx, created.ID)
	if err != nil || status.State != "connected" {
		t.Fatalf("optional read changed connection: %v", err)
	}
	saved, err := s.store.Get(ctx, created.ID)
	if err != nil || saved.DefaultModel != "" || s.registry.Get(created.ID).DefaultModel != "" {
		t.Fatal("default saved without confirming authorization")
	}
}

func TestChatGPTDefaultModelUsesCurrentConsumer(t *testing.T) {
	for _, change := range []string{"rename", "delete", "rebind", "chosen_model", "disconnect"} {
		t.Run(change, func(t *testing.T) {
			s, mgr, ctx := chatGPTTestService(t)
			created, err := s.CreateChatGPTConnection(ctx, "Original")
			if err != nil {
				t.Fatal(err)
			}
			store, _ := mgr.OAuthStore(ctx)
			r, _ := store.Load(ctx, created.ID)
			r.State = "connected"
			r.Revision++
			if err := store.CompareAndSwap(ctx, r, r.Revision-1); err != nil {
				t.Fatal(err)
			}
			stale := s.registry.Get(created.ID)
			current, err := database.GetLLMProviderWithContext(ctx, created.ID)
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "delete":
				err = database.DB().Delete(current).Error
			case "rename":
				err = database.DB().Model(current).Update("name", "Imported name").Error
			case "rebind":
				err = database.DB().Model(current).Update("credential_pattern", "oauth:another-grant").Error
			case "chosen_model":
				err = database.DB().Model(current).Update("default_model", "chosen-model").Error
			case "disconnect":
				r.State = "disconnected"
				r.Revision++
				err = store.CompareAndSwap(ctx, r, r.Revision-1)
			}
			if err != nil {
				t.Fatal(err)
			}
			s.setChatGPTDefaultModel(ctx, store, created.ID, stale, "account-model")
			saved, err := database.GetLLMProviderWithContext(ctx, created.ID)
			if change == "delete" {
				if err == nil {
					t.Fatal("deleted provider resurrected")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "rename":
				if saved.Name != "Imported name" || saved.DefaultModel != "account-model" || s.registry.Get(created.ID).Name != saved.Name {
					t.Fatal("stale snapshot published")
				}
			case "chosen_model":
				if saved.DefaultModel != "chosen-model" || s.registry.Get(created.ID).DefaultModel != "chosen-model" {
					t.Fatal("explicit model overwritten")
				}
			default:
				if saved.DefaultModel != "" {
					t.Fatal("changed authorization accepted")
				}
			}
		})
	}
}
