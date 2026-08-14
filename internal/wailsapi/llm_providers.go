package wailsapi

import (
	"assistente/controllers"
	"assistente/internal/apidto"
	"assistente/internal/llm"
	"context"
	"sync"
	"time"
)

// LLMProvidersHooks agrupa side effects do App que o bind não deve conhecer
// diretamente (AEP-0088).
type LLMProvidersHooks struct {
	// ApplyInstalledBinaryEnv aplica o env{} do artefato ACP instalado ao
	// provedor após Create/Update (AEP-0086).
	ApplyInstalledBinaryEnv func(ctx context.Context, providerID, agentID string)
	// ReloadClient reinicializa o cliente LLM ativo (ReloadLLMClient).
	ReloadClient func()
	// PersistDelete remove o provedor do store após a remoção no registry
	// (Service.Delete só tira da memória).
	PersistDelete func(ctx context.Context, id string) error
	// CreateDefault cria o primeiro provedor no wizard/CLI setup.
	// Pré-login: NÃO usa WithUser — preserva bootstrap (WithBootstrap).
	CreateDefault func(providerType, apiKey string) error
}

// LLMProviders é o bind Wails do domínio llm_providers (AEP-0088).
// Auth via WithUser na maioria dos métodos. CreateDefaultLLMProvider é
// dual-mode pré/pós-login (wizard/CLI) e NÃO usa WithUser — documentado
// abaixo; não entra em UnauthenticatedAppMethods (allowlist só de *App).
type LLMProviders struct {
	mu      sync.RWMutex
	session Session
	ctrl    *controllers.LLMController
	hooks   LLMProvidersHooks
}

// NewLLMProviders cria o bind vazio; AttachLLMProviders preenche deps no startup.
func NewLLMProviders() *LLMProviders {
	return &LLMProviders{}
}

// AttachLLMProviders associa Session, controller e hooks após o startup.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachLLMProviders(api *LLMProviders, session Session, ctrl *controllers.LLMController, hooks LLMProvidersHooks) {
	if api == nil {
		return
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	api.session = session
	api.ctrl = ctrl
	api.hooks = hooks
}

func (p *LLMProviders) deps() (Session, *controllers.LLMController, LLMProvidersHooks, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.session == nil || p.ctrl == nil {
		return nil, nil, LLMProvidersHooks{}, ErrLLMProvidersNotWired
	}
	return p.session, p.ctrl, p.hooks, nil
}

// GetLLMProviders lista provedores do registry em memória.
func (p *LLMProviders) GetLLMProviders() ([]*llm.ProviderConfig, error) {
	session, ctrl, _, err := p.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]*llm.ProviderConfig, error) {
		return ctrl.GetLLMProviders(), nil
	})
}

// GetLLMProvider retorna um provedor pelo ID.
func (p *LLMProviders) GetLLMProvider(id string) (*llm.ProviderConfig, error) {
	session, ctrl, _, err := p.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*llm.ProviderConfig, error) {
		return ctrl.GetLLMProvider(id), nil
	})
}

// GetActiveProviderInfo retorna info do provedor do perfil ativo.
func (p *LLMProviders) GetActiveProviderInfo() (map[string]interface{}, error) {
	session, ctrl, _, err := p.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (map[string]interface{}, error) {
		return ctrl.GetActiveProviderInfo(ctx), nil
	})
}

// GetLLMProvidersWithStatus lista provedores com status de credencial.
func (p *LLMProviders) GetLLMProvidersWithStatus() ([]map[string]interface{}, error) {
	session, ctrl, _, err := p.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]map[string]interface{}, error) {
		return ctrl.GetLLMProvidersWithStatus(ctx), nil
	})
}

// TestLLMProvider testa conectividade de um provedor.
func (p *LLMProviders) TestLLMProvider(req apidto.TestLLMProviderRequest) (bool, error) {
	session, ctrl, _, err := p.deps()
	if err != nil {
		return false, err
	}
	return WithUser(session, func(ctx context.Context) (bool, error) {
		return ctrl.TestLLMProvider(ctx, req)
	})
}

// ListModelsRaw lista modelos disponíveis no endpoint do provedor.
func (p *LLMProviders) ListModelsRaw(req apidto.TestLLMProviderRequest) ([]string, error) {
	session, ctrl, _, err := p.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(authCtx context.Context) ([]string, error) {
		ctx, cancel := context.WithTimeout(authCtx, 15*time.Second)
		defer cancel()
		return ctrl.ListModelsRaw(ctx, req)
	})
}

// CreateLLMProvider cria um provedor e aplica env do binário ACP instalado.
func (p *LLMProviders) CreateLLMProvider(req apidto.CreateLLMProviderRequest) (map[string]interface{}, error) {
	session, ctrl, hooks, err := p.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (map[string]interface{}, error) {
		created, err := ctrl.CreateLLMProvider(ctx, req)
		if err != nil {
			return nil, err
		}
		if id, _ := created["id"].(string); id != "" && hooks.ApplyInstalledBinaryEnv != nil {
			hooks.ApplyInstalledBinaryEnv(ctx, id, req.ACPAgentID)
		}
		return created, nil
	})
}

// UpdateLLMProvider atualiza um provedor e reaplica env do binário ACP.
func (p *LLMProviders) UpdateLLMProvider(id string, req apidto.UpdateLLMProviderRequest) (map[string]interface{}, error) {
	session, ctrl, hooks, err := p.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (map[string]interface{}, error) {
		updated, err := ctrl.UpdateLLMProvider(ctx, id, req)
		if err != nil {
			return nil, err
		}
		agentID := ""
		if req.ACPAgentID != nil {
			agentID = *req.ACPAgentID
		} else if existing := ctrl.GetLLMProvider(id); existing != nil {
			agentID = existing.ACPAgentID
		}
		if hooks.ApplyInstalledBinaryEnv != nil {
			hooks.ApplyInstalledBinaryEnv(ctx, id, agentID)
		}
		return updated, nil
	})
}

// SetDefaultProvider define o provedor padrão (reinicia cliente via controller).
func (p *LLMProviders) SetDefaultProvider(id string) error {
	session, ctrl, _, err := p.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.SetDefaultProvider(ctx, id)
	})
	return err
}

// DeleteLLMProvider remove do registry e persiste a exclusão no store.
func (p *LLMProviders) DeleteLLMProvider(id string) error {
	session, ctrl, hooks, err := p.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		if err := ctrl.DeleteLLMProvider(ctx, id); err != nil {
			return struct{}{}, err
		}
		if hooks.PersistDelete != nil {
			return struct{}{}, hooks.PersistDelete(ctx, id)
		}
		return struct{}{}, nil
	})
	return err
}

// ReloadLLMClient reinicializa o cliente LLM ativo.
func (p *LLMProviders) ReloadLLMClient() error {
	session, _, hooks, err := p.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		if hooks.ReloadClient != nil {
			hooks.ReloadClient()
		}
		return struct{}{}, nil
	})
	return err
}

// CreateDefaultLLMProvider cria o primeiro provedor no wizard ou CLI setup.
//
// Auth: NÃO usa WithUser. É um dos poucos pontos legítimos de bootstrap
// pré-login (wizard/CLI antes do primeiro login). O hook CreateDefault no App
// marca o ctx com WithBootstrap quando não há userID. Pós-login (wizard após
// AuthGate) o ctx já carrega userID. Não entra em UnauthenticatedAppMethods
// porque a allowlist só lista métodos do *App.
func (p *LLMProviders) CreateDefaultLLMProvider(providerType, apiKey string) error {
	_, _, hooks, err := p.deps()
	if err != nil {
		return err
	}
	if hooks.CreateDefault == nil {
		return ErrLLMProvidersNotWired
	}
	return hooks.CreateDefault(providerType, apiKey)
}
