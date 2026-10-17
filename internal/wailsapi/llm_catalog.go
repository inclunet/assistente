package wailsapi

import (
	"assistente/internal/apidto"
	"assistente/internal/llmcatalog"
	"context"
	"sync"
)

// LLMCatalog expõe o catálogo público de provedores/modelos sem permitir que a
// interface escolha URL de origem ou altere a política de segurança.
type LLMCatalog struct {
	mu      sync.RWMutex
	session Session
	service *llmcatalog.Service
}

func NewLLMCatalog() *LLMCatalog { return &LLMCatalog{} }

func AttachLLMCatalog(api *LLMCatalog, session Session, service *llmcatalog.Service) {
	if api == nil {
		return
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	api.session = session
	api.service = service
}

func (api *LLMCatalog) deps() (Session, *llmcatalog.Service, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	if api.session == nil || api.service == nil {
		return nil, nil, ErrLLMCatalogNotWired
	}
	return api.session, api.service, nil
}

// GetLLMCatalog devolve o snapshot atual sem esperar pela rede.
func (api *LLMCatalog) GetLLMCatalog() (apidto.LLMCatalogResponse, error) {
	session, service, err := api.deps()
	if err != nil {
		return apidto.LLMCatalogResponse{}, err
	}
	return WithUser(session, func(context.Context) (apidto.LLMCatalogResponse, error) {
		return llmCatalogResponse(service.Snapshot()), nil
	})
}

// RefreshLLMCatalog tenta buscar uma nova versão a pedido do usuário. A chamada
// termina com o último snapshot, mantendo o documento válido em caso de falha.
func (api *LLMCatalog) RefreshLLMCatalog() (apidto.LLMCatalogResponse, error) {
	session, service, err := api.deps()
	if err != nil {
		return apidto.LLMCatalogResponse{}, err
	}
	return WithUser(session, func(ctx context.Context) (apidto.LLMCatalogResponse, error) {
		return llmCatalogResponse(service.Refresh(ctx)), nil
	})
}

func llmCatalogResponse(snapshot llmcatalog.Snapshot) apidto.LLMCatalogResponse {
	response := apidto.LLMCatalogResponse{
		Document:   snapshot.Document,
		AgeSeconds: int64(snapshot.Age.Seconds()),
		FromCache:  snapshot.FromCache,
		Stale:      snapshot.Stale,
		Refreshing: snapshot.Refreshing,
		ReasonCode: snapshot.ReasonCode,
	}
	if !snapshot.FetchedAt.IsZero() {
		response.FetchedAt = snapshot.FetchedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	return response
}
