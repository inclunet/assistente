package providers

import (
	"context"
	"fmt"
	"time"

	"assistente/internal/profiles"
)

// HealthState representa o estado de saúde live da conexão com o provider ativo.
type HealthState string

const (
	// HealthOnline indica que o endpoint está acessível e autenticado.
	HealthOnline HealthState = "online"
	// HealthOffline indica que o endpoint está inacessível ou rejeitou a autenticação.
	HealthOffline HealthState = "offline"
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
