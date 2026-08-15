package wailsapi

import (
	"context"
	"sync"

	"assistente/internal/llm"
	"assistente/internal/profiles"
	"assistente/internal/providers"
)

// LLMModelsHooks agrupa side effects do App que o bind não deve conhecer
// diretamente (AEP-0088). O mapa de streaming permanece no *App — o bind só
// dispara cancel via hook (sem duplicar estado).
type LLMModelsHooks struct {
	// CancelStreaming cancela o streaming LLM em andamento para a conversa
	// (streamMgr.Cancel). Semântica fire-and-forget no App; o bind só propaga
	// erro de auth/wire.
	CancelStreaming func(conversationID string)
}

// LLMModels é o bind Wails do domínio llm_models (AEP-0088): catálogo/lista de
// modelos, refresh e cancel de streaming. CRUD de provedores fica em LLMProviders.
// Auth só via WithUser.
type LLMModels struct {
	mu          sync.RWMutex
	session     Session
	providerSvc *providers.Service
	profileMgr  *profiles.Manager
	hooks       LLMModelsHooks
}

// NewLLMModels cria o bind vazio; AttachLLMModels preenche deps no startup.
func NewLLMModels() *LLMModels {
	return &LLMModels{}
}

// AttachLLMModels associa Session, provider service, profile manager e hooks.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachLLMModels(
	api *LLMModels,
	session Session,
	providerSvc *providers.Service,
	profileMgr *profiles.Manager,
	hooks LLMModelsHooks,
) {
	if api == nil {
		return
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	api.session = session
	api.providerSvc = providerSvc
	api.profileMgr = profileMgr
	api.hooks = hooks
}

func (m *LLMModels) deps() (Session, *providers.Service, *profiles.Manager, LLMModelsHooks, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.session == nil || m.providerSvc == nil || m.profileMgr == nil || m.hooks.CancelStreaming == nil {
		return nil, nil, nil, LLMModelsHooks{}, ErrLLMModelsNotWired
	}
	return m.session, m.providerSvc, m.profileMgr, m.hooks, nil
}

// GetModels retorna a lista de modelos do provedor do perfil ativo.
func (m *LLMModels) GetModels() ([]string, error) {
	session, providerSvc, profileMgr, _, err := m.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]string, error) {
		activeProfile, _ := profileMgr.GetActive()
		return providerSvc.GetModels(ctx, activeProfile)
	})
}

// GetModelsByProvider retorna a lista de modelos de um provedor específico.
func (m *LLMModels) GetModelsByProvider(providerID string) ([]string, error) {
	session, providerSvc, _, _, err := m.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]string, error) {
		return providerSvc.GetModelsByProvider(ctx, providerID)
	})
}

// RefreshModels relista os modelos do provedor ativo descartando cache (AEP-0084 D6).
func (m *LLMModels) RefreshModels() ([]string, error) {
	session, providerSvc, profileMgr, _, err := m.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]string, error) {
		activeProfile, _ := profileMgr.GetActive()
		return providerSvc.RefreshModels(ctx, activeProfile)
	})
}

// RefreshModelsByProvider é o mesmo para um provedor escolhido pelo identificador.
func (m *LLMModels) RefreshModelsByProvider(providerID string) ([]string, error) {
	session, providerSvc, _, _, err := m.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]string, error) {
		return providerSvc.RefreshModelsByProvider(ctx, providerID)
	})
}

// GetModelCatalogByProvider lista modelos com rótulo legível e flag de agente (AEP-0084).
func (m *LLMModels) GetModelCatalogByProvider(providerID string) (llm.ModelCatalog, error) {
	session, providerSvc, _, _, err := m.deps()
	if err != nil {
		return llm.ModelCatalog{}, err
	}
	return WithUser(session, func(ctx context.Context) (llm.ModelCatalog, error) {
		return providerSvc.GetModelCatalogByProvider(ctx, providerID)
	})
}

// RefreshModelCatalogByProvider é o mesmo descartando cache do provedor (AEP-0084 D6).
func (m *LLMModels) RefreshModelCatalogByProvider(providerID string) (llm.ModelCatalog, error) {
	session, providerSvc, _, _, err := m.deps()
	if err != nil {
		return llm.ModelCatalog{}, err
	}
	return WithUser(session, func(ctx context.Context) (llm.ModelCatalog, error) {
		return providerSvc.RefreshModelCatalogByProvider(ctx, providerID)
	})
}

// CancelStreamingForConversation cancela o streaming LLM em andamento para uma
// conversa (barge-in SIP/UI). Erro de auth/wire (inclui hook ausente); o cancel
// em si não falha.
func (m *LLMModels) CancelStreamingForConversation(conversationID string) error {
	session, _, _, hooks, err := m.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		hooks.CancelStreaming(conversationID)
		return struct{}{}, nil
	})
	return err
}
