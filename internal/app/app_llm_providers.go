package app

import (
	"assistente/internal/logging"
	"assistente/internal/providers"
	"context"
	"fmt"
	"maps"
	"time"

	"assistente/controllers"
	"assistente/internal/database"
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
	created, err := a.llmCtrl.CreateLLMProvider(ctx, req)
	if err != nil {
		return nil, err
	}
	// ACPEnv não atravessa a fronteira Create (token costuma parar aí). O env
	// do alvo binário instalado vem do installed.json, pelo agent_id — a tela
	// só escolhe o agente; o app aplica a configuração publicada no registro.
	if id, _ := created["id"].(string); id != "" {
		a.applyInstalledBinaryEnv(ctx, id, req.ACPAgentID)
	}
	return created, nil
}

func (a *App) UpdateLLMProvider(id string, req controllers.UpdateLLMProviderRequest) (map[string]interface{}, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	updated, err := a.llmCtrl.UpdateLLMProvider(ctx, id, req)
	if err != nil {
		return nil, err
	}
	agentID := ""
	if req.ACPAgentID != nil {
		agentID = *req.ACPAgentID
	} else if existing := a.llmCtrl.GetLLMProvider(id); existing != nil {
		agentID = existing.ACPAgentID
	}
	a.applyInstalledBinaryEnv(ctx, id, agentID)
	return updated, nil
}

// applyInstalledBinaryEnv põe no provedor o env{} do artefato binário que o
// app instalou para este agente (AEP-0086). Sem UI de edição de ACPEnv: a
// fonte é o registro via installed.json, e o cofre continua em ACPCredentialEnv.
func (a *App) applyInstalledBinaryEnv(ctx context.Context, providerID, agentID string) {
	if a.providerSvc == nil || providerID == "" || agentID == "" {
		return
	}
	installation, ok := a.acpCatalogServices().installer.Installed(agentID)
	if !ok {
		return
	}
	// Env vazio também aplica: senão um update que tira o env{} do registro
	// deixaria no provedor as variáveis da instalação anterior.
	env := maps.Clone(installation.Env)
	if env == nil {
		env = map[string]string{}
	}
	if _, err := a.providerSvc.Update(ctx, providerID, providers.UpdateRequest{ACPEnv: &env}); err != nil {
		logging.Warnf(ctx, "llm-providers",
			"não foi possível aplicar o env do agente instalado %s ao provedor %s: %v",
			agentID, providerID, err)
	}
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

// saveLLMProviders, loadLLMProviders e ensureDefaultProvider são helpers
// internos chamados pós-login (a partir de reloadUserScopedRuntime ou de
// callbacks acionados após o usuário estar autenticado). Falham fechado
// quando não há sessão ativa — o caminho de bootstrap pré-login deve usar
// CreateDefaultLLMProvider, que marca o ctx com WithBootstrap explicitamente.
func (a *App) saveLLMProviders() error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.providerSvc.Save(ctx)
}

func (a *App) loadLLMProviders() error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.providerSvc.Load(ctx)
}

func (a *App) ensureDefaultProvider() {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return
	}
	a.providerSvc.EnsureDefault(ctx)
}

// ============================================================================
// LLM Client / Provider Init
// ============================================================================

// initLLMClient inicializa o cliente LLM usando o provider do perfil ativo
func (a *App) initLLMClient() {
	activeProfile, err := a.profileManager.GetActive()
	if err != nil || activeProfile == nil {
		logging.Infof(context.Background(), "app.app-llm-providers", "[initLLMClient] Perfil ativo não encontrado: %v", err)
		return
	}
	activeProfile = a.resolveProfileDefaults(activeProfile)

	provider := a.llmRegistry.Get(activeProfile.Chat.LLMProvider)
	if provider == nil {
		logging.Infof(context.Background(), "app.app-llm-providers", "[initLLMClient] Provedor LLM não encontrado: %s", activeProfile.Chat.LLMProvider)
		return
	}

	logging.Infof(context.Background(), "app.app-llm-providers", "[initLLMClient] Provedor ativo: %s (api_format=%s)", provider.Name, provider.GetAPIFormat())
}

// ReloadLLMClient recarrega o cliente LLM (chamado quando config muda)
func (a *App) ReloadLLMClient() {
	a.initLLMClient()
}

// resolveProfileDefaults substitui sentinelas "$default" no profile pelo
// provedor/modelo padrão do usuário autenticado.
//
// Blocker C do re-review do AEP-0052: a versão anterior usava
// bootstrapAwareCtx e justificava como "read-only seguro pré-login";
// o reviewer corretamente apontou que misturar caminhos pré- e pós-login
// num helper aumenta superfície sem motivo. Sem sessão devolvemos o profile
// inalterado de forma explícita; os callers de produção
// (initLLMClient/resolveSpeechProfile) só rodam pós-login mesmo, então o
// caminho "sem sessão" só existe para defesa em testes/boot incompleto.
func (a *App) resolveProfileDefaults(p *profiles.Profile) *profiles.Profile {
	if a.providerSvc == nil {
		return p
	}
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return p
	}
	return a.providerSvc.ResolveProfileDefaults(ctx, p)
}

// initLLMProviders inicializa o registro de provedores LLM a partir do store.
// Só faz sentido após o login — sem userID o repositório não tem provedores
// para devolver e o registry global ficaria vazio mesmo se prosseguíssemos.
//
// Recebe `ctx` do caller (pós-P1-2 do re-review do PR #94) para que
// timeouts/cancels propaguem até o store. Se `ctx == nil`, derivamos um
// ctx autenticado próprio — preserva o contrato anterior para call sites
// que ainda não passam ctx (boot single-user pré-login).
func (a *App) initLLMProviders(ctx context.Context) {
	if ctx == nil {
		authed, err := a.requireAuthenticatedContext()
		if err != nil {
			return
		}
		ctx = authed
	}
	if _, err := database.RequireUserID(ctx); err != nil {
		return
	}
	if err := a.providerSvc.Load(ctx); err != nil {
		count, countErr := a.providerSvc.Count(ctx)
		if countErr != nil || count == 0 {
			logging.Infof(ctx, "app.app-llm-providers", "Nenhum provedor encontrado. Configure um provedor nas configurações ou crie um perfil.")
		}
	}
}

// CreateDefaultLLMProvider cria o primeiro provedor durante o wizard ou
// CLI setup. É um dos poucos pontos legítimos de bootstrap pré-login: quando
// o ctx do app não carrega userID (caminho CLI antes do primeiro login),
// marcamos explicitamente com WithBootstrap para que providers.DBStore.Save
// aceite a gravação. Pós-login (wizard de UI rodando após AuthGate) o ctx
// já carrega userID e WithBootstrap não é aplicado — o provedor é criado
// com user_id do usuário autenticado.
func (a *App) CreateDefaultLLMProvider(providerType, apiKey string) error {
	ctx := a.internalBootstrapCtx()
	if _, ok := database.UserIDFromContext(ctx); !ok {
		ctx = database.WithBootstrap(ctx)
	}
	return a.providerSvc.CreateFromTemplate(ctx, providerType, apiKey)
}

// getChatProviderForProvider é uma fina camada de delegação para providerSvc.GetChatProvider.
// Mantida para uso em testes de integração que combinam resolveProfileDefaults com routing.
func (a *App) getChatProviderForProvider(providerID string) (llm.ChatProvider, error) {
	if a.providerSvc == nil {
		return nil, fmt.Errorf("provider service not initialized")
	}
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.providerSvc.GetChatProvider(ctx, providerID)
}
