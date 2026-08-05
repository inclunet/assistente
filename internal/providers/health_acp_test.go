package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"assistente/internal/acp"
	"assistente/internal/credentials"
	"assistente/internal/llm"
)

// agenteFalso é um agente de código de mentira: responde ao handshake e à
// abertura de sessão sem subir processo, para o health poder ser exercitado sem
// depender de um CLI instalado na máquina do teste.
type agenteFalso struct {
	caps      acp.Capabilities
	sessaoErr error
	sessoes   int
}

func (a *agenteFalso) NewSession(context.Context, string) (acp.Session, error) {
	a.sessoes++
	if a.sessaoErr != nil {
		return nil, a.sessaoErr
	}
	return &sessaoFalsa{}, nil
}

func (a *agenteFalso) LoadSession(context.Context, string, string) (acp.Session, error) {
	return nil, errors.New("não usado no teste")
}

func (a *agenteFalso) Capabilities(context.Context) (acp.Capabilities, error) { return a.caps, nil }

func (a *agenteFalso) CloseSession(context.Context, string) error { return nil }

// Options e InvalidateOptions completam o contrato do cliente. A sonda de health
// não pergunta modelo nenhum — o que ela quer saber é se o agente responde e se
// aceita abrir sessão —, então a lista vazia aqui é a resposta fiel.
func (a *agenteFalso) Options(context.Context, string) ([]acp.ConfigOption, error) {
	return nil, nil
}

func (a *agenteFalso) InvalidateOptions() {}

func (a *agenteFalso) Call(context.Context, string, any) (json.RawMessage, error) {
	return nil, errors.New("não usado no teste")
}

func (a *agenteFalso) Close() error { return nil }

type sessaoFalsa struct{}

func (s *sessaoFalsa) ID() string { return "sess-1" }

func (s *sessaoFalsa) Prompt(context.Context, []acp.Content, acp.UpdateSink) (acp.StopReason, error) {
	return acp.StopEndTurn, nil
}

func (s *sessaoFalsa) Close(context.Context) error  { return nil }
func (s *sessaoFalsa) Cancel(context.Context) error { return nil }

func (s *sessaoFalsa) ConfigOptions() []acp.ConfigOption { return nil }

// Commands fecha o contrato da sessão. A sonda de health abre a sessão só para
// ver se o agente aceita abrir uma: comando nenhum é o que ela tem a dizer.
func (s *sessaoFalsa) Commands() []acp.Command { return nil }

func (s *sessaoFalsa) SetConfigOption(context.Context, string, string) ([]acp.ConfigOption, error) {
	return nil, nil
}

func providerDeAgente() *llm.ProviderConfig {
	return &llm.ProviderConfig{
		ID:         "cursor-1",
		Name:       "Cursor local",
		Type:       llm.ProviderCursor,
		APIFormat:  llm.APIFormatACP,
		AuthMode:   llm.AuthModeNone,
		ACPCommand: "cursor-agent",
		ACPArgs:    []string{"acp"},
	}
}

// serviceDeAgente monta o serviço com o provedor de agente registrado e o
// transporte ACP trocado pelo agente falso.
func serviceDeAgente(t *testing.T, provider *llm.ProviderConfig, agente acp.Client) *Service {
	t.Helper()
	registry := llm.NewProviderRegistry()
	if provider != nil {
		if err := registry.Register(provider); err != nil {
			t.Fatalf("registrar provedor: %v", err)
		}
	}
	mgr := acp.NewManager(acp.ManagerConfig{
		WorkDir: func() (string, error) { return t.TempDir(), nil },
		Dial: func(acp.Config, acp.RequestHandler) (acp.Client, error) {
			if agente == nil {
				return nil, errors.New("agente não instalado")
			}
			return agente, nil
		},
	})
	t.Cleanup(mgr.Shutdown)
	return NewService(ServiceConfig{
		Registry:   registry,
		CredMgr:    credentials.NewManager(nil),
		ACPManager: mgr,
	})
}

func TestCheckHealthAgenteOnline(t *testing.T) {
	agente := &agenteFalso{caps: acp.Capabilities{AgentName: "Cursor"}}
	svc := serviceDeAgente(t, providerDeAgente(), agente)

	res := svc.CheckHealth(context.Background(), profileForProvider("cursor-1"))

	if res.State != HealthOnline {
		t.Fatalf("estado = %q (erro %q), esperado online", res.State, res.Error)
	}
	if !res.Reachable || !res.AuthOK {
		t.Errorf("reachable=%v authOK=%v", res.Reachable, res.AuthOK)
	}
	if res.ProviderID != "cursor-1" || res.ProviderName != "Cursor local" {
		t.Errorf("metadados do provedor = %+v", res)
	}
	if res.Model != "" {
		t.Errorf("modelo = %q; o health de um agente não confirma modelo nenhum", res.Model)
	}
	if agente.sessoes != 1 {
		t.Errorf("sessões abertas pela sonda = %d, esperado 1", agente.sessoes)
	}
}

func TestCheckHealthAgenteSemLoginTemEstadoProprio(t *testing.T) {
	agente := &agenteFalso{
		caps:      acp.Capabilities{AgentName: "Cursor"},
		sessaoErr: fmt.Errorf("abrir sessão no agente ACP: %w", acp.ErrNotAuthenticated),
	}
	svc := serviceDeAgente(t, providerDeAgente(), agente)

	res := svc.CheckHealth(context.Background(), profileForProvider("cursor-1"))

	if res.State != HealthUnauthenticated {
		t.Fatalf("estado = %q, esperado unauthenticated", res.State)
	}
	if !res.Reachable {
		t.Error("o agente respondeu ao handshake: reachable deveria ser verdadeiro")
	}
	if res.AuthOK {
		t.Error("authOK verdadeiro sem login")
	}
	if res.ErrorType != "agent_not_authenticated" {
		t.Errorf("tipo de erro = %q; a tela precisa dele para instruir o login", res.ErrorType)
	}
}

func TestCheckHealthAgenteQueNaoSobeEhOffline(t *testing.T) {
	svc := serviceDeAgente(t, providerDeAgente(), nil)

	res := svc.CheckHealth(context.Background(), profileForProvider("cursor-1"))

	if res.State != HealthOffline {
		t.Fatalf("estado = %q, esperado offline", res.State)
	}
	if res.ErrorType != "agent_unreachable" {
		t.Errorf("tipo de erro = %q", res.ErrorType)
	}
	if res.Error == "" {
		t.Error("offline sem motivo nenhum")
	}
}

func TestCheckHealthAgenteSemServicoNaoSondaNemMente(t *testing.T) {
	registry := llm.NewProviderRegistry()
	if err := registry.Register(providerDeAgente()); err != nil {
		t.Fatalf("registrar provedor: %v", err)
	}
	svc := NewService(ServiceConfig{Registry: registry, CredMgr: credentials.NewManager(nil)})

	res := svc.CheckHealth(context.Background(), profileForProvider("cursor-1"))

	if res.State != HealthOffline {
		t.Fatalf("estado = %q, esperado offline", res.State)
	}
	if res.ErrorType != "acp_manager_missing" {
		t.Errorf("tipo de erro = %q; sem serviço o problema não é do agente", res.ErrorType)
	}
}
