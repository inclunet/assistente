package controllers

import (
	"context"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/llm"
	"assistente/internal/providers"
)

// cofreDeMentira registra o que passaria pelo cofre de credenciais. Para um
// agente de código a resposta certa é "nada": o login dele é feito no CLI.
type cofreDeMentira struct {
	registrados []string
}

func (c *cofreDeMentira) RegisterPatternWithContext(_ context.Context, pattern string, _ *credentials.AuthConfig) error {
	c.registrados = append(c.registrados, pattern)
	return nil
}

func (c *cofreDeMentira) GetByPattern(string) (*credentials.AuthConfig, error) { return nil, nil }

func (c *cofreDeMentira) GetByPatternWithContext(context.Context, string) (*credentials.AuthConfig, error) {
	return nil, nil
}

func (c *cofreDeMentira) DeletePattern(context.Context, string) error { return nil }

func controladorDeProvedores(t *testing.T) (*LLMController, *llm.ProviderRegistry, *cofreDeMentira) {
	t.Helper()
	registry := llm.NewProviderRegistry()
	cofre := &cofreDeMentira{}
	svc := providers.NewService(providers.ServiceConfig{
		Registry: registry,
		CredMgr:  cofre,
		Store:    providers.NewMemoryStore(),
	})
	return NewLLMController(LLMControllerConfig{LLMRegistry: registry, ProviderSvc: svc}), registry, cofre
}

// O formulário do agente manda `base_url` vazia de propósito: um agente não tem
// endereço, e é o comando que o endereça (AEP-0084 D12). Este teste fixa que a
// fronteira aceita esse payload como ele sai da tela e que o que fica salvo é o
// comando — sem URL exigida e sem credencial guardada.
func TestCriarAgentePelaFronteiraGuardaOComandoESemURL(t *testing.T) {
	ctrl, registry, cofre := controladorDeProvedores(t)

	res, err := ctrl.CreateLLMProvider(context.Background(), CreateLLMProviderRequest{
		ID:         "cursor-1",
		Name:       "Cursor local",
		Type:       "cursor",
		APIFormat:  "acp",
		BaseURL:    "",
		ACPCommand: "/usr/local/bin/cursor-agent",
		ACPArgs:    []string{"acp"},
	})
	if err != nil {
		t.Fatalf("a fronteira recusou o payload do formulário de agente: %v", err)
	}

	if res["api_format"] != "acp" {
		t.Errorf("api_format = %v, queria acp", res["api_format"])
	}
	if res["base_url"] != "" {
		t.Errorf("base_url = %v, queria vazia", res["base_url"])
	}
	if res["acp_command"] != "/usr/local/bin/cursor-agent" {
		t.Errorf("acp_command = %v", res["acp_command"])
	}

	salvo := registry.Get("cursor-1")
	if salvo == nil {
		t.Fatal("provedor de agente não ficou salvo")
	}
	if !salvo.IsACP() || salvo.ACPCommand != "/usr/local/bin/cursor-agent" {
		t.Errorf("o que ficou salvo não sobe o agente: %+v", salvo)
	}
	if len(salvo.ACPArgs) != 1 || salvo.ACPArgs[0] != "acp" {
		t.Errorf("argumentos salvos = %#v", salvo.ACPArgs)
	}
	if len(cofre.registrados) != 0 {
		t.Errorf("um agente não tem credencial, e algo foi para o cofre: %v", cofre.registrados)
	}
}

// A tela nunca manda chave para um agente, mas a fronteira é pública: recusar é
// o que impede um segredo inútil de entrar no cofre por outro caminho.
func TestCriarAgentePelaFronteiraRecusaChaveDeAPI(t *testing.T) {
	ctrl, registry, cofre := controladorDeProvedores(t)

	_, err := ctrl.CreateLLMProvider(context.Background(), CreateLLMProviderRequest{
		ID:         "cursor-1",
		Name:       "Cursor local",
		Type:       "cursor",
		APIFormat:  "acp",
		APIKey:     "sk-secreta",
		ACPCommand: "/usr/local/bin/cursor-agent",
	})

	if err == nil {
		t.Fatal("a fronteira aceitou credencial para um provedor que não tem onde usá-la")
	}
	if registry.Get("cursor-1") != nil {
		t.Error("o provedor foi criado apesar da recusa")
	}
	if len(cofre.registrados) != 0 {
		t.Errorf("a chave recusada chegou ao cofre: %v", cofre.registrados)
	}
}
