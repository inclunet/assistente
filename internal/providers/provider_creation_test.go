package providers

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/llm"
)

// O mock de falha continua cobrindo a mesma recusa de publicação, agora pela
// porta de inserção que o fluxo de criação efetivamente usa.
func (s failingProviderSave) Create(context.Context, *llm.ProviderConfig) error {
	return errors.New("disk unavailable")
}

func TestProviderCreationRefusesPersistedIDsAbsentFromRegistry(t *testing.T) {
	for _, useDB := range []bool{false, true} {
		for _, fromTemplate := range []bool{false, true} {
			name := "memory/form"
			if useDB {
				name = "database/form"
			}
			if fromTemplate {
				name += "/template"
			}
			t.Run(name, func(t *testing.T) {
				var store ProviderStore = NewMemoryStore()
				if useDB {
					acpTestDB(t)
					store = NewDBStore()
				}
				ctx := database.WithUserID(context.Background(), "owner")
				template, err := BuiltinTemplate("openai")
				if err != nil {
					t.Fatal(err)
				}
				original := &llm.ProviderConfig{ID: template.ID, Name: "Preservado", Type: llm.ProviderOpenAI, BaseURL: "https://original.example.com/v1", DefaultModel: "original-model", CredentialPattern: "original.example.com"}
				if err := store.Create(ctx, original); err != nil {
					t.Fatal(err)
				}
				registry := llm.NewProviderRegistry()
				mgr := credentials.NewManager(nil)
				service := NewService(ServiceConfig{Registry: registry, Store: store, CredMgr: mgr})
				if fromTemplate {
					err = service.CreateFromTemplate(ctx, "openai", "attempted-secret")
				} else {
					_, err = service.Create(ctx, CreateRequest{ID: original.ID, Name: "Replacement", Type: "openai", BaseURL: original.BaseURL, APIKey: "attempted-secret"})
				}
				if !errors.Is(err, database.ErrLLMProviderAlreadyExists) {
					t.Fatalf("criação não recusada: %v", err)
				}
				if registry.Get(original.ID) != nil {
					t.Fatal("publicou provider após recusa de criação")
				}
				saved, err := store.Get(ctx, original.ID)
				if err != nil {
					t.Fatal(err)
				}
				if saved.Name != original.Name || saved.BaseURL != original.BaseURL || saved.DefaultModel != original.DefaultModel || saved.CompatibilityRevision != original.CompatibilityRevision || saved.ConfigRevision != original.ConfigRevision {
					t.Fatalf("sobrescreveu provedor persistido: %+v", saved)
				}
				for _, pattern := range []string{original.CredentialPattern, template.CredentialPattern} {
					auth, err := mgr.GetConfigByPatternWithContext(ctx, pattern)
					if err != nil || auth != nil {
						t.Fatalf("criação recusada alterou credencial: %v", err)
					}
				}
				exists, err := store.Exists(ctx, original.ID)
				if err != nil || !exists {
					t.Fatalf("identidade persistida perdida: %v", err)
				}
			})
		}
	}
}

func TestDBStoreCreateRequiresScopeAndPreservesDuplicateIdentity(t *testing.T) {
	acpTestDB(t)
	store := NewDBStore()
	if err := store.Create(context.Background(), &llm.ProviderConfig{ID: "duplicate"}); !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("criação sem escopo: %v", err)
	}
	if _, err := store.Exists(context.Background(), "duplicate"); !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("consulta sem escopo: %v", err)
	}
	owner := database.WithUserID(context.Background(), "owner")
	original := &llm.ProviderConfig{ID: "duplicate", Name: "Original", Type: llm.ProviderOpenAI, BaseURL: "https://example.com/v1"}
	if err := store.Create(owner, original); err != nil {
		t.Fatal(err)
	}
	replacement := &llm.ProviderConfig{ID: original.ID, Name: "Replacement", Type: llm.ProviderOpenAI, BaseURL: "https://replacement.example.com/v1"}
	if err := store.Create(database.WithUserID(context.Background(), "other"), replacement); !errors.Is(err, database.ErrLLMProviderAlreadyExists) {
		t.Fatalf("ID duplicado aceito: %v", err)
	}
	saved, err := store.Get(owner, original.ID)
	if err != nil || saved.Name != original.Name || saved.BaseURL != original.BaseURL {
		t.Fatalf("linha original modificada: %v", err)
	}
	if replacement.CompatibilityRevision != 0 || replacement.ConfigRevision != 0 {
		t.Fatal("publicou revisão antes de inserção recusada")
	}
}

type racingCreationStore struct {
	ProviderStore
	entered chan struct{}
	release chan struct{}
}

func (s *racingCreationStore) Exists(context.Context, string) (bool, error) {
	s.entered <- struct{}{}
	<-s.release
	return false, nil
}

func TestConcurrentProviderCreationReservesIDBeforeWritingCredentials(t *testing.T) {
	for _, fromTemplate := range []bool{false, true} {
		name := "form"
		if fromTemplate {
			name = "template"
		}
		t.Run(name, func(t *testing.T) {
			base := NewMemoryStore()
			store := &racingCreationStore{ProviderStore: base, entered: make(chan struct{}, 2), release: make(chan struct{})}
			mgr := credentials.NewManager(nil)
			service := NewService(ServiceConfig{Registry: llm.NewProviderRegistry(), Store: store, CredMgr: mgr})
			ctx := database.WithUserID(context.Background(), "owner")
			template, err := BuiltinTemplate("openai")
			if err != nil {
				t.Fatal(err)
			}
			type attempt struct {
				key string
				err error
			}
			done := make(chan attempt, 2)
			for _, key := range []string{"first-secret", "second-secret"} {
				go func(key string) {
					var err error
					if fromTemplate {
						err = service.CreateFromTemplate(ctx, "openai", key)
					} else {
						_, err = service.Create(ctx, CreateRequest{ID: template.ID, Name: "Concurrent", Type: "openai", BaseURL: template.BaseURL, APIKey: key})
					}
					done <- attempt{key, err}
				}(key)
			}
			for i := 0; i < 2; i++ {
				select {
				case <-store.entered:
				case <-time.After(3 * time.Second):
					close(store.release)
					t.Fatal("não atingiu o preflight concorrente")
				}
			}
			close(store.release)
			winner := ""
			for i := 0; i < 2; i++ {
				result := <-done
				if result.err == nil {
					if winner != "" {
						t.Fatal("duas criações venceram")
					}
					winner = result.key
				} else if !errors.Is(result.err, database.ErrLLMProviderAlreadyExists) {
					t.Fatalf("recusa inesperada: %v", result.err)
				}
			}
			auth, err := mgr.GetConfigByPatternWithContext(ctx, template.CredentialPattern)
			if err != nil || auth == nil || winner == "" || auth.Token != winner {
				t.Fatalf("credencial do vencedor foi substituída: %v", err)
			}
		})
	}
}

type failingCreationCredentials struct {
	CredentialManager
	failure error
}

func (m failingCreationCredentials) RegisterPatternWithContext(context.Context, string, *credentials.AuthConfig) error {
	return m.failure
}

func TestCredentialFailureRollsBackUnpublishedProviderReservation(t *testing.T) {
	for _, useDB := range []bool{false, true} {
		for _, bootstrap := range []bool{false, true} {
			for _, fromTemplate := range []bool{false, true} {
				name := "memory"
				if useDB {
					name = "database"
				}
				if bootstrap {
					name += "/bootstrap"
				} else {
					name += "/user"
				}
				if fromTemplate {
					name += "/template"
				} else {
					name += "/form"
				}
				t.Run(name, func(t *testing.T) {
					var store ProviderStore = NewMemoryStore()
					if useDB {
						if err := database.MigrateLLMModelCapabilities(acpTestDB(t)); err != nil {
							t.Fatal(err)
						}
						store = NewDBStore()
					}
					ctx := database.WithUserID(context.Background(), "owner")
					if bootstrap {
						ctx = database.WithBootstrap(context.Background())
					}
					failure := errors.New("credential write failed")
					registry := llm.NewProviderRegistry()
					service := NewService(ServiceConfig{Registry: registry, Store: store, CredMgr: failingCreationCredentials{failure: failure}})
					template, err := BuiltinTemplate("openai")
					if err != nil {
						t.Fatal(err)
					}
					if fromTemplate {
						err = service.CreateFromTemplate(ctx, "openai", "key")
					} else {
						_, err = service.Create(ctx, CreateRequest{ID: template.ID, Name: "Pending", Type: "openai", BaseURL: template.BaseURL, APIKey: "key"})
					}
					if !errors.Is(err, failure) {
						t.Fatalf("erro de credencial perdido: %v", err)
					}
					if exists, err := store.Exists(ctx, template.ID); err != nil || exists {
						t.Fatalf("reserva permaneceu após falha: %v", err)
					}
					if registry.Get(template.ID) != nil {
						t.Fatal("publicou criação fracassada")
					}
				})
			}
		}
	}
}

func TestProviderReservationRollbackPreservesChangedConfiguration(t *testing.T) {
	for _, useDB := range []bool{false, true} {
		name := "memory"
		if useDB {
			name = "database"
		}
		t.Run(name, func(t *testing.T) {
			var store ProviderStore = NewMemoryStore()
			if useDB {
				if err := database.MigrateLLMModelCapabilities(acpTestDB(t)); err != nil {
					t.Fatal(err)
				}
				store = NewDBStore()
			}
			ctx := database.WithUserID(context.Background(), "owner")
			provider := &llm.ProviderConfig{ID: "reservation", Name: "Pending", Type: llm.ProviderOpenAI, BaseURL: "https://example.com/v1"}
			if err := store.Create(ctx, provider); err != nil {
				t.Fatal(err)
			}
			changed := *provider
			changed.Name = "Changed elsewhere"
			if err := store.Save(ctx, []*llm.ProviderConfig{&changed}); err != nil {
				t.Fatal(err)
			}
			if err := store.RollbackCreate(ctx, provider); err == nil {
				t.Fatal("removeu configuração alterada")
			}
			saved, err := store.Get(ctx, provider.ID)
			if err != nil || saved.Name != changed.Name {
				t.Fatalf("configuração perdida: %v", err)
			}
		})
	}
}

func TestProviderReservationRollbackPreservesConcurrentModelFacts(t *testing.T) {
	for _, bootstrap := range []bool{false, true} {
		name := "user"
		if bootstrap {
			name = "bootstrap"
		}
		t.Run(name, func(t *testing.T) {
			db := acpTestDB(t)
			if err := database.MigrateLLMModelCapabilities(db); err != nil {
				t.Fatal(err)
			}
			if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
				t.Fatal(err)
			}
			ctx := database.WithUserID(context.Background(), "owner")
			if bootstrap {
				ctx = database.WithBootstrap(context.Background())
			}
			store := NewDBStore()
			provider := &llm.ProviderConfig{ID: "concurrent-catalog", Name: "Pending", Type: llm.ProviderOpenAI, BaseURL: "https://example.com/v1"}
			if err := store.Create(ctx, provider); err != nil {
				t.Fatal(err)
			}
			repository := database.NewLLMModelCapabilitiesRepository(db)
			model, err := repository.SaveModel(ctx, provider.ID, "model", "Model")
			if err != nil {
				t.Fatal(err)
			}
			fact := &database.LLMModelCapabilityField{ModelID: model.ID, CapabilityKey: "chat", FieldKey: "temperature", SupportState: "unsupported", Source: "execution_observation", Scope: "connection", ProviderCompatibilityRevision: provider.CompatibilityRevision, ObservedAt: time.Now().Add(-time.Second)}
			if err := repository.RecordField(ctx, fact, nil); err != nil {
				t.Fatal(err)
			}
			if err := store.RollbackCreate(ctx, provider); err == nil {
				t.Fatal("rollback apagou reserva que já possuía modelo")
			}
			var preserved database.LLMProvider
			if err := db.Where("id = ?", provider.ID).First(&preserved).Error; err != nil {
				t.Fatalf("provedor perdido: %v", err)
			}
			readCtx := ctx
			if bootstrap {
				readCtx = database.WithUserID(context.Background(), "reader")
			}
			resolution, err := repository.Resolve(readCtx, model.ID, time.Now())
			if err != nil || resolution.Fields["chat"]["temperature"].State != "unsupported" {
				t.Fatalf("fatos concorrentes perdidos: %+v %v", resolution, err)
			}
		})
	}
}

func TestProviderReservationRollbackPreservesChangedCompatibilityRevision(t *testing.T) {
	for _, bootstrap := range []bool{false, true} {
		name := "user"
		if bootstrap {
			name = "bootstrap"
		}
		t.Run(name, func(t *testing.T) {
			db := acpTestDB(t)
			if err := database.MigrateLLMModelCapabilities(db); err != nil {
				t.Fatal(err)
			}
			ctx := database.WithUserID(context.Background(), "owner")
			if bootstrap {
				ctx = database.WithBootstrap(context.Background())
			}
			store := NewDBStore()
			provider := &llm.ProviderConfig{ID: "changed-compatibility", Name: "Pending", Type: llm.ProviderOpenAI, BaseURL: "https://example.com/v1", CredentialPattern: "example.com"}
			if err := store.Create(ctx, provider); err != nil {
				t.Fatal(err)
			}
			// A credential commit can advance compatibility without changing the
			// provider configuration or creating any model. The original receipt
			// must not delete that identity if a later credential read fails.
			if err := db.Exec("UPDATE llm_providers SET compatibility_revision = compatibility_revision + 1 WHERE id = ?", provider.ID).Error; err != nil {
				t.Fatal(err)
			}
			if err := store.RollbackCreate(ctx, provider); err == nil {
				t.Fatal("rollback apagou provedor com revisão de compatibilidade alterada")
			}
			var preserved database.LLMProvider
			if err := db.Where("id = ?", provider.ID).First(&preserved).Error; err != nil {
				t.Fatalf("identidade perdida: %v", err)
			}
			if preserved.ConfigRevision != provider.ConfigRevision || preserved.CompatibilityRevision != provider.CompatibilityRevision+1 {
				t.Fatalf("revisões alteradas: %+v", preserved)
			}
		})
	}
}
