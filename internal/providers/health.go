package providers

import (
	"context"
	"fmt"
	"time"

	"assistente/internal/acp"
	"assistente/internal/llm"
	"assistente/internal/profiles"
)

// HealthState representa o estado de saúde live da conexão com o provider ativo.
type HealthState string

const (
	// HealthOnline indica que o endpoint está acessível e autenticado.
	HealthOnline HealthState = "online"
	// HealthOffline indica que o endpoint está inacessível ou rejeitou a autenticação.
	HealthOffline HealthState = "offline"
	// HealthUnauthenticated indica um agente de código instalado e de pé, mas
	// sem login. Estado próprio porque a saída é outra: não se conserta
	// endereço nem credencial no app, roda-se o login do CLI do agente
	// (AEP-0084 D12).
	HealthUnauthenticated HealthState = "unauthenticated"
)

// HealthResult é o resultado de uma sondagem de saúde do provider ativo.
// Diferente de ListWithStatus (que reporta status de CREDENCIAL configurada),
// este resultado reflete uma verificação LIVE de conectividade + latência.
type HealthResult struct {
	State        HealthState
	ProviderID   string
	ProviderName string
	Model        string
	// LatencyMs é o tempo de ida e volta da sondagem em milissegundos.
	LatencyMs int64
	Reachable bool
	AuthOK    bool
	Error     string
	ErrorType string
}

// CheckHealth realiza uma sondagem live do provider do perfil ativo e mede a
// latência. Reaproveita ProbeConnection (mesma rota /models usada pelo wizard
// e pela validação de providers) — NÃO duplica a lógica de teste de conexão.
//
// Resolve sentinelas $default do perfil e recupera a credencial persistida do
// provider antes de sondar. Requer um ctx com userID quando o provider usa
// credenciais (a busca por padrão é user-scoped).
func (s *Service) CheckHealth(ctx context.Context, activeProfile *profiles.Profile) HealthResult {
	res := HealthResult{State: HealthOffline}
	if activeProfile == nil {
		res.Error = "perfil ativo não encontrado"
		res.ErrorType = "profile_missing"
		return res
	}

	resolved := s.ResolveProfileDefaults(ctx, activeProfile)
	if resolved == nil || s.registry == nil {
		res.Error = "registro de provedores não inicializado"
		res.ErrorType = "registry_missing"
		return res
	}

	provider := s.registry.Get(resolved.Chat.LLMProvider)
	if provider == nil {
		res.Error = fmt.Sprintf("provedor não encontrado: %s", resolved.Chat.LLMProvider)
		res.ErrorType = "provider_missing"
		return res
	}

	res.ProviderID = provider.ID
	res.ProviderName = provider.Name
	res.Model = resolved.Chat.Model
	if res.Model == "" {
		res.Model = provider.DefaultModel
	}

	// Agente de código não tem endpoint: sondá-lo é subir o processo, fazer o
	// handshake e ver se ele aceita abrir sessão (AEP-0084 D12).
	if provider.IsACP() {
		return s.checkACPHealth(ctx, provider, res)
	}

	apiKey := ""
	if provider.CredentialPattern != "" && s.credMgr != nil {
		if auth, err := s.credMgr.GetByPatternWithContext(ctx, provider.CredentialPattern); err == nil && auth != nil {
			apiKey = auth.Token
		}
	}

	start := time.Now()
	probe := s.ProbeConnection(ctx, provider.BaseURL, apiKey)
	res.LatencyMs = time.Since(start).Milliseconds()
	res.Reachable = probe.URLReachable
	res.AuthOK = probe.AuthOK

	if probe.URLReachable && probe.AuthOK {
		res.State = HealthOnline
		return res
	}

	res.State = HealthOffline
	res.Error = probe.ErrorDetail
	res.ErrorType = probe.ErrorType
	return res
}

// checkACPHealth traduz a sondagem do agente para o mesmo resultado que o
// indicador de conexão já consome. O modelo sai da conta: a lista de modelos de
// um agente vem da sessão dele, e o que o perfil guardar não é o que o health
// tem como confirmar.
//
// "Alcançável" aqui é o processo de pé e apresentado; "autenticado" é ele ter
// aceitado abrir sessão. Sem manager, o app não tem como sondar — e dizer
// offline sugeriria um agente com problema, quando o que falta é o serviço.
func (s *Service) checkACPHealth(ctx context.Context, provider *llm.ProviderConfig, res HealthResult) HealthResult {
	res.Model = ""
	if s.acpMgr == nil {
		res.State = HealthOffline
		res.Error = "serviço de agentes de código não inicializado"
		res.ErrorType = "acp_manager_missing"
		return res
	}

	report := s.acpMgr.Probe(ctx, acp.ProviderSpec{
		ID:            provider.ID,
		Name:          provider.Name,
		Command:       provider.ACPCommand,
		Args:          provider.ACPArgs,
		Env:           provider.ACPEnv,
		CredentialEnv: provider.ACPCredentialEnv,
	})
	res.LatencyMs = report.Latency.Milliseconds()
	res.Error = report.Error

	switch report.State {
	case acp.HealthOnline:
		res.State = HealthOnline
		res.Reachable = true
		res.AuthOK = true
		res.Error = ""
	case acp.HealthUnauthenticated:
		res.State = HealthUnauthenticated
		// Alcançável: o agente subiu e falou. O que falta é o login, e é isso
		// que o tipo de erro precisa dizer para a tela instruir em vez de
		// mandar conferir caminho.
		res.Reachable = true
		res.ErrorType = "agent_not_authenticated"
	default:
		res.State = HealthOffline
		res.ErrorType = "agent_unreachable"
	}
	return res
}
