package controllers

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/apidto"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/events"
)

type credentialControllerStore struct {
	entries map[string]credentials.StoredCredential
}

func (s *credentialControllerStore) SaveCredential(_ context.Context, entry credentials.StoredCredential) error {
	if entry.ID == "" {
		entry.ID = "credential-" + entry.Pattern
	}
	if s.entries == nil {
		s.entries = make(map[string]credentials.StoredCredential)
	}
	s.entries[entry.Pattern] = entry
	return nil
}

func (s *credentialControllerStore) ListCredentials(context.Context) ([]credentials.StoredCredential, error) {
	entries := make([]credentials.StoredCredential, 0, len(s.entries))
	for _, entry := range s.entries {
		entries = append(entries, entry)
	}
	return entries, nil
}

func (s *credentialControllerStore) DeleteCredential(_ context.Context, pattern string) error {
	delete(s.entries, pattern)
	return nil
}

func (*credentialControllerStore) SaveKeyWrap(context.Context, credentials.KeyWrap) error { return nil }
func (*credentialControllerStore) GetKeyWrap(context.Context, string) (*credentials.KeyWrap, error) {
	return nil, nil
}
func (*credentialControllerStore) HasKeyWrap(context.Context, string) (bool, error) {
	return false, nil
}

type credentialRevisionRefresherSpy struct {
	patterns []string
	advanced []string
	err      error
	after    func(string) error
}

func (s *credentialRevisionRefresherSpy) RefreshCredentialPatternRevisions(_ context.Context, pattern string) error {
	s.patterns = append(s.patterns, pattern)
	if s.after != nil {
		if err := s.after(pattern); err != nil {
			return err
		}
	}
	return s.err
}

func (s *credentialRevisionRefresherSpy) AdvanceCredentialPatternRevisions(_ context.Context, pattern string) error {
	s.advanced = append(s.advanced, pattern)
	return s.err
}

func TestCredentialMutationsRefreshProviderIdentityAfterChangingVault(t *testing.T) {
	store := &credentialControllerStore{}
	wantPresent := true
	refresher := &credentialRevisionRefresherSpy{
		after: func(pattern string) error {
			_, exists := store.entries[pattern]
			if exists != wantPresent {
				return errors.New("registry atualizado antes da mutação persistida")
			}
			return nil
		},
	}
	manager := credentials.NewManagerWithStore(make([]byte, 32), store, true)
	manager.SetCredentialRevisionRefresher(refresher)
	controller := NewCredentialsController(CredentialsControllerConfig{CredMgr: manager})
	ctx := database.WithUserID(context.Background(), "owner-a")
	input := apidto.CredentialInput{Pattern: "shared.example.test", Source: "static", Type: "bearer", Token: "token-value"}

	if err := controller.UpsertCredentialWithContext(ctx, input); err != nil {
		t.Fatalf("criar credencial: %v", err)
	}
	if len(refresher.patterns) != 1 || refresher.patterns[0] != input.Pattern {
		t.Fatalf("upsert não atualizou a revisão após persistir: %v", refresher.patterns)
	}
	if _, exists := store.entries[input.Pattern]; !exists {
		t.Fatal("credencial não foi persistida depois da invalidação")
	}
	wantPresent = false
	if err := controller.DeleteCredentialWithContext(ctx, input.Pattern); err != nil {
		t.Fatalf("remover credencial: %v", err)
	}
	if len(refresher.patterns) != 2 || refresher.patterns[1] != input.Pattern {
		t.Fatalf("delete não atualizou a revisão após remover: %v", refresher.patterns)
	}
	if _, exists := store.entries[input.Pattern]; exists {
		t.Fatal("credencial permaneceu persistida após exclusão")
	}
}

func TestCredentialMutationReportsRegistryRefreshFailureAfterVaultCommit(t *testing.T) {
	store := &credentialControllerStore{}
	refresher := &credentialRevisionRefresherSpy{err: errors.New("database unavailable")}
	manager := credentials.NewManagerWithStore(make([]byte, 32), store, true)
	manager.SetCredentialRevisionRefresher(refresher)
	controller := NewCredentialsController(CredentialsControllerConfig{CredMgr: manager})
	ctx := database.WithUserID(context.Background(), "owner-a")
	input := apidto.CredentialInput{Pattern: "shared.example.test", Source: "static", Type: "bearer", Token: "token-value"}

	if err := controller.UpsertCredentialWithContext(ctx, input); err == nil {
		t.Fatal("upsert deveria reportar falha ao atualizar o registry")
	}
	if _, exists := store.entries[input.Pattern]; !exists {
		t.Fatal("commit do cofre foi revertido apesar de a mutação persistida ter ocorrido")
	}
}

func TestEphemeralCredentialMutationsAdvanceProviderIdentity(t *testing.T) {
	manager := credentials.NewManager([]byte("test-key-exactly-32-bytes-long!!"))
	refresher := &credentialRevisionRefresherSpy{}
	manager.SetCredentialRevisionRefresher(refresher)
	ctx := database.WithUserID(context.Background(), "owner-a")
	pattern := "ephemeral.example.test"
	auth := &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "token-value"}

	if err := manager.RegisterPatternWithContext(ctx, pattern, auth); err != nil {
		t.Fatalf("criar credencial em memória: %v", err)
	}
	if len(refresher.advanced) != 1 || refresher.advanced[0] != pattern {
		t.Fatalf("mutação em memória não avançou a revisão: %v", refresher.advanced)
	}
	if err := manager.DeletePattern(ctx, pattern); err != nil {
		t.Fatalf("remover credencial em memória: %v", err)
	}
	if len(refresher.advanced) != 2 || refresher.advanced[1] != pattern {
		t.Fatalf("remoção em memória não avançou a revisão: %v", refresher.advanced)
	}
}

func TestClearAllCredentialsRefreshesEveryPatternAfterDeleting(t *testing.T) {
	store := &credentialControllerStore{}
	manager := credentials.NewManagerWithStore(make([]byte, 32), store, true)
	ctx := database.WithUserID(context.Background(), "owner-a")
	patterns := []string{"first.shared.example.test", "second.shared.example.test"}
	for _, pattern := range patterns {
		if err := manager.RegisterPatternWithContext(ctx, pattern, &credentials.AuthConfig{
			Source: "static", Type: "bearer", Token: "token-value",
		}); err != nil {
			t.Fatalf("salvar credencial %q: %v", pattern, err)
		}
	}
	refresher := &credentialRevisionRefresherSpy{
		after: func(pattern string) error {
			if _, exists := store.entries[pattern]; exists {
				return errors.New("registry atualizado antes da exclusão persistida")
			}
			return nil
		},
	}
	manager.SetCredentialRevisionRefresher(refresher)
	controller := NewSettingsController(SettingsControllerConfig{
		CredMgr: manager, Emitter: events.NoopEmitter{},
	})

	if err := controller.ClearAllCredentials(ctx); err != nil {
		t.Fatalf("ClearAllCredentials: %v", err)
	}
	if len(refresher.patterns) != len(patterns) {
		t.Fatalf("patterns atualizados = %v, esperado %v", refresher.patterns, patterns)
	}
	if len(store.entries) != 0 {
		t.Fatalf("credenciais não removidas: %v", store.entries)
	}
}

func TestClearAllCredentialsReturnsRegistryRefreshErrorAfterDeletion(t *testing.T) {
	store := &credentialControllerStore{}
	manager := credentials.NewManagerWithStore(make([]byte, 32), store, true)
	ctx := database.WithUserID(context.Background(), "owner-a")
	pattern := "shared.example.test"
	if err := manager.RegisterPatternWithContext(ctx, pattern, &credentials.AuthConfig{
		Source: "static", Type: "bearer", Token: "token-value",
	}); err != nil {
		t.Fatalf("salvar credencial: %v", err)
	}
	manager.SetCredentialRevisionRefresher(&credentialRevisionRefresherSpy{err: errors.New("database unavailable")})
	controller := NewSettingsController(SettingsControllerConfig{CredMgr: manager, Emitter: events.NoopEmitter{}})

	if err := controller.ClearAllCredentials(ctx); err == nil {
		t.Fatal("ClearAllCredentials deveria reportar falha ao atualizar o registry")
	}
	if _, exists := store.entries[pattern]; exists {
		t.Fatal("exclusão persistida deveria ocorrer antes de atualizar o registry")
	}
}
