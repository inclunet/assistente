package wailsapi

import (
	"assistente/internal/acpregistry"
	"assistente/internal/apidto"
	"context"
	"errors"
	"testing"
)

func TestACPRegistryNotWired(t *testing.T) {
	t.Parallel()
	api := NewACPRegistry()
	if _, err := api.GetACPCatalog(); !errors.Is(err, ErrACPRegistryNotWired) {
		t.Fatalf("GetACPCatalog: got %v", err)
	}
	if _, err := api.RefreshACPCatalog(); !errors.Is(err, ErrACPRegistryNotWired) {
		t.Fatalf("RefreshACPCatalog: got %v", err)
	}
}

func TestACPRegistryNilRegistryIsNotWired(t *testing.T) {
	t.Parallel()
	api := NewACPRegistry()
	AttachACPRegistry(api, stubSession{}, nil, func(context.Context, acpregistry.Catalog) apidto.ACPCatalog {
		return apidto.ACPCatalog{}
	})
	if _, err := api.GetACPCatalog(); !errors.Is(err, ErrACPRegistryNotWired) {
		t.Fatalf("GetACPCatalog com registry nil: got %v", err)
	}
}

// TestACPRegistrySemAuthNaoTocaNoCatalogo cobre o fail-closed dos dois métodos:
// sem contexto autenticado nada do domínio roda — nem a montagem do catálogo,
// nem a ida ao registro — e o erro da sessão sobe como veio.
func TestACPRegistrySemAuthNaoTocaNoCatalogo(t *testing.T) {
	t.Parallel()
	semAuth := errors.New("sessão não autenticada")
	casos := map[string]func(*ACPRegistry) (apidto.ACPCatalog, error){
		"GetACPCatalog":     (*ACPRegistry).GetACPCatalog,
		"RefreshACPCatalog": (*ACPRegistry).RefreshACPCatalog,
	}
	for nome, chamar := range casos {
		t.Run(nome, func(t *testing.T) {
			t.Parallel()
			api := NewACPRegistry()
			AttachACPRegistry(api, stubSession{err: semAuth}, acpregistry.New(acpregistry.Config{}),
				func(context.Context, acpregistry.Catalog) apidto.ACPCatalog {
					t.Fatal("montou o catálogo sem contexto autenticado")
					return apidto.ACPCatalog{}
				})

			catalog, err := chamar(api)

			if !errors.Is(err, semAuth) {
				t.Fatalf("erro = %v, quer o da sessão", err)
			}
			if len(catalog.Agents) != 0 {
				t.Errorf("catálogo = %+v, quer vazio sem auth", catalog)
			}
		})
	}
}
