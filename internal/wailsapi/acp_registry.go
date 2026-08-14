package wailsapi

import (
	"assistente/internal/acpregistry"
	"assistente/internal/apidto"
	"context"
	"sync"
)

// ACPRegistry é o bind Wails do domínio acp_registry — catálogo do registro
// oficial ACP para a tela (AEP-0088). Helpers de montagem (acpCatalogOf etc.)
// permanecem no *App.
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
type ACPRegistry struct {
	mu        sync.RWMutex
	session   Session
	registry  *acpregistry.Service
	catalogOf func(ctx context.Context, catalog acpregistry.Catalog) apidto.ACPCatalog
}

// NewACPRegistry cria o bind vazio; AttachACPRegistry preenche deps no startup.
func NewACPRegistry() *ACPRegistry {
	return &ACPRegistry{}
}

// AttachACPRegistry associa Session, serviço do registro e montagem do catálogo
// após o startup. Função de pacote (não método) para não entrar no Bind do Wails.
func AttachACPRegistry(
	api *ACPRegistry,
	session Session,
	registry *acpregistry.Service,
	catalogOf func(ctx context.Context, catalog acpregistry.Catalog) apidto.ACPCatalog,
) {
	if api == nil {
		return
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	api.session = session
	api.registry = registry
	api.catalogOf = catalogOf
}

func (api *ACPRegistry) deps() (Session, *acpregistry.Service, func(context.Context, acpregistry.Catalog) apidto.ACPCatalog, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	if api.session == nil || api.registry == nil || api.catalogOf == nil {
		return nil, nil, nil, ErrACPRegistryNotWired
	}
	return api.session, api.registry, api.catalogOf, nil
}

// GetACPCatalog devolve o catálogo do registro ACP para a tela de provedores.
//
// Ela abre sem rede (D2): o que está em cache é servido na hora e a revalidação
// acontece em segundo plano. Sem cache e sem rede, o catálogo vem vazio com o
// motivo — e isso não é erro desta chamada, é o estado que a tela explica.
func (api *ACPRegistry) GetACPCatalog() (apidto.ACPCatalog, error) {
	session, registry, catalogOf, err := api.deps()
	if err != nil {
		return apidto.ACPCatalog{}, err
	}
	return WithUser(session, func(ctx context.Context) (apidto.ACPCatalog, error) {
		return catalogOf(ctx, registry.Catalog(ctx)), nil
	})
}

// RefreshACPCatalog busca o índice agora, a pedido de quem clicou.
//
// Ela existe porque recarregar é ato explícito (D2): a revalidação automática só
// acontece depois do prazo, e quem estava sem rede quando a tela abriu não tem
// por que esperar por ele para tentar de novo. Falha não custa o catálogo que já
// estava servindo — o que volta é o anterior, com o motivo.
func (api *ACPRegistry) RefreshACPCatalog() (apidto.ACPCatalog, error) {
	session, registry, catalogOf, err := api.deps()
	if err != nil {
		return apidto.ACPCatalog{}, err
	}
	return WithUser(session, func(ctx context.Context) (apidto.ACPCatalog, error) {
		// O erro não sobe: ele já está dentro do catálogo, como motivo que a tela
		// diz no idioma de quem lê. Subir também faria a tela mostrar duas versões
		// da mesma falha, uma delas em português.
		catalog, _ := registry.Refresh(ctx)
		return catalogOf(ctx, catalog), nil
	})
}
