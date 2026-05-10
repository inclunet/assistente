package app

import (
	"context"
	"fmt"
	"log"
	"time"

	"assistente/controllers"
	"assistente/internal/llm"
	"assistente/internal/profiles"
)

// ============================================================================
// LLM Provider API — delegação para LLMController
// Os métodos abaixo existem para manter compatibilidade com o Wails Bind
// enquanto a migração para controllers/ está em andamento (Strangler Fig).
// ============================================================================

func (a *App) GetLLMProviders() []*llm.ProviderConfig       { return a.llmCtrl.GetLLMProviders() }
func (a *App) GetLLMProvider(id string) *llm.ProviderConfig { return a.llmCtrl.GetLLMProvider(id) }
func (a *App) GetActiveProviderInfo() map[string]interface{} {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil
	}
	return a.llmCtrl.GetActiveProviderInfo(ctx)
}
func (a *App) GetLLMProvidersWithStatus() []map[string]interface{} {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil
	}
	return a.llmCtrl.GetLLMProvidersWithStatus(ctx)
}

func (a *App) TestLLMProvider(req controllers.TestLLMProviderRequest) (ok bool, retErr error) {
	if a.ctx == nil {
		return false, fmt.Errorf("aplicação ainda não está pronta, aguarde")
	}
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return false, err
	}
	return a.llmCtrl.TestLLMProvider(ctx, req)
}

func (a *App) ListModelsRaw(req controllers.TestLLMProviderRequest) (models []string, retErr error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("aplicação ainda não está pronta, aguarde")
	}
	authCtx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(authCtx, 15*time.Second)
	defer cancel()
	return a.llmCtrl.ListModelsRaw(ctx, req)
}

func (a *App) CreateLLMProvider(req controllers.CreateLLMProviderRequest) (map[string]interface{}, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.llmCtrl.CreateLLMProvider(ctx, req)
}

func (a *App) UpdateLLMProvider(id string, req controllers.UpdateLLMProviderRequest) (map[string]interface{}, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.llmCtrl.UpdateLLMProvider(ctx, id, req)
}

func (a *App) SetDefaultProvider(id string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.llmCtrl.SetDefaultProvider(ctx, id)
}

func (a *App) DeleteLLMProvider(_ context.Context, id string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.llmCtrl.DeleteLLMProvider(ctx, id)
}

// saveLLMProviders e helpers permanecem em App pois são chamados internamente.
func (a *App) saveLLMProviders() error { return a.providerSvc.Save(a.authenticatedContext()) }
func (a *App) loadLLMProviders() error { return a.providerSvc.Load(a.authenticatedContext()) }
func (a *App) ensureDefaultProvider()  { a.providerSvc.EnsureDefault(a.authenticatedContext()) }

// ============================================================================
// LLM Client / Provider Init
// ============================================================================

// initLLMClient inicializa o cliente LLM usando o provider do perfil ativo
func (a *App) initLLMClient() {
	activeProfile, err := a.profileManager.GetActive()
	if err != nil || activeProfile == nil {
		log.Printf("[initLLMClient] Perfil ativo não encontrado: %v", err)
		return
	}
	activeProfile = a.resolveProfileDefaults(activeProfile)

	provider := a.llmRegistry.Get(activeProfile.Chat.LLMProvider)
	if provider == nil {
		log.Printf("[initLLMClient] Provedor LLM não encontrado: %s", activeProfile.Chat.LLMProvider)
		return
	}

	log.Printf("[initLLMClient] Provedor ativo: %s (api_format=%s)", provider.Name, provider.GetAPIFormat())
}

// ReloadLLMClient recarrega o cliente LLM (chamado quando config muda)
func (a *App) ReloadLLMClient() {
	a.initLLMClient()
}

// resolveProfileDefaults substitui sentinelas "$default" no profile pelo provedor/modelo padrão.
func (a *App) resolveProfileDefaults(p *profiles.Profile) *profiles.Profile {
	if a.providerSvc == nil {
		return p
	}
	return a.providerSvc.ResolveProfileDefaults(a.authenticatedContext(), p)
}

// initLLMProviders inicializa o registro de provedores LLM a partir do store.
func (a *App) initLLMProviders() {
	if err := a.loadLLMProviders(); err != nil {
		count, countErr := a.providerSvc.Count(a.authenticatedContext())
		if countErr != nil || count == 0 {
			log.Printf("Nenhum provedor encontrado. Configure um provedor nas configurações ou crie um perfil.")
		}
	}
}

// CreateDefaultLLMProvider cria o primeiro provedor durante o wizard.
func (a *App) CreateDefaultLLMProvider(providerType, apiKey string) error {
	return a.providerSvc.CreateFromTemplate(a.authenticatedContext(), providerType, apiKey)
}

// getChatProviderForProvider é uma fina camada de delegação para providerSvc.GetChatProvider.
// Mantida para uso em testes de integração que combinam resolveProfileDefaults com routing.
func (a *App) getChatProviderForProvider(providerID string) (llm.ChatProvider, error) {
	if a.providerSvc == nil {
		return nil, fmt.Errorf("provider service not initialized")
	}
	return a.providerSvc.GetChatProvider(a.authenticatedContext(), providerID)
}
