package providers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"assistente/internal/credentials"
	"assistente/internal/llm"
)

// Probes command usam o mesmo transport do chat; não materializam uma chave
// estática que impediria observar 401 e renovar o cache. Outros managers/fontes
// mantêm a aplicação existente, sem cache implícito.
func (s *Service) prepareProbeAuth(ctx context.Context, req TestRequest, target *http.Request, client *http.Client) error {
	restrictProbeRedirect(client, req.BaseURL)
	cm, ok := s.credMgr.(*credentials.Manager)
	if !ok || strings.TrimSpace(req.APIKey) != "" {
		return s.applyProbeAuth(ctx, req, target)
	}
	parsed, err := url.Parse(req.BaseURL)
	if err != nil {
		return err
	}
	pattern := parsed.Hostname()
	mode := credentials.AuthRequired
	if req.ProviderID != "" {
		if s.registry == nil {
			return s.applyProbeAuth(ctx, req, target)
		}
		provider := s.registry.Get(req.ProviderID)
		if provider == nil {
			return fmt.Errorf("provedor não encontrado")
		}
		if provider.EffectiveAuthMode() == llm.AuthModeNone {
			return s.applyProbeAuth(ctx, req, target)
		}
		if !sameCredentialOrigin(req.BaseURL, provider.BaseURL) {
			return fmt.Errorf("URL alterada: informe uma credencial para testar o novo destino")
		}
		pattern = provider.CredentialPattern
		if provider.EffectiveAuthMode() == llm.AuthModeOptional {
			mode = credentials.AuthOptional
		}
	}
	config, err := cm.GetConfigByPatternWithContext(ctx, pattern)
	if err != nil || config == nil || (config.Source != "command" && config.Source != "oauth") {
		return s.applyProbeAuth(ctx, req, target)
	}
	transport := credentials.NewCredentialTransportWithMode(cm, pattern, mode)
	if client.Transport != nil {
		transport.Base = client.Transport
	}

	client.Transport = transport
	return nil
}

func restrictProbeRedirect(client *http.Client, baseURL string) {
	previousRedirect := client.CheckRedirect
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if !sameCredentialOrigin(baseURL, next.URL.String()) {
			return fmt.Errorf("redirecionamento para outra origem recusado na sondagem autenticada")
		}
		if previousRedirect != nil {
			return previousRedirect(next, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("limite de redirecionamentos excedido")
		}
		return nil
	}
}
