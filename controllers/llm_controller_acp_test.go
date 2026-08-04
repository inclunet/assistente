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
	apagados    []string
	guardados   map[string]*credentials.AuthConfig
}

func (c *cofreDeMentira) RegisterPatternWithContext(_ context.Context, pattern string, auth *credentials.AuthConfig) error {
	c.registrados = append(c.registrados, pattern)
	if c.guardados == nil {
		c.guardados = map[string]*credentials.AuthConfig{}
	}
	c.guardados[pattern] = auth
	return nil
}

func (c *cofreDeMentira) GetByPattern(pattern string) (*credentials.AuthConfig, error) {
	return c.guardados[pattern], nil
}

func (c *cofreDeMentira) GetByPatternWithContext(_ context.Context, pattern string) (*credentials.AuthConfig, error) {
	return c.guardados[pattern], nil
}

func (c *cofreDeMentira) DeletePattern(_ context.Context, pattern string) error {
	c.apagados = append(c.apagados, pattern)
	delete(c.guardados, pattern)
	return nil
}

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

// Trocar o tipo de um provedor salvo para um agente é caminho de tela — o
// formulário de edição deixa —, e o que sai daqui não pode continuar descrevendo
// o provedor antigo: URL que não endereça nada, e pior, um ponteiro para
// credencial que faria a lista mostrar o agente com chave configurada, contra o
// D12 do AEP-0084.
func TestVirarAgentePelaFronteiraLargaURLECredencial(t *testing.T) {
	ctrl, registry, cofre := controladorDeProvedores(t)
	ctx := context.Background()

	if _, err := ctrl.CreateLLMProvider(ctx, CreateLLMProviderRequest{
		ID: "prov", Name: "OpenAI", Type: "openai",
		BaseURL: "https://api.openai.com/v1", APIKey: "sk-secreta",
	}); err != nil {
		t.Fatalf("Create falhou: %v", err)
	}

	res, err := ctrl.UpdateLLMProvider(ctx, "prov", UpdateLLMProviderRequest{
		Type: "cursor", APIFormat: "acp",
		ACPCommand: "/usr/local/bin/cursor-agent",
		ACPArgs:    &[]string{"acp"},
	})
	if err != nil {
		t.Fatalf("Update falhou: %v", err)
	}

	if res["base_url"] != "" {
		t.Errorf("base_url = %v, queria vazia: o agente é endereçado pelo comando", res["base_url"])
	}
	if res["credential_pattern"] != "" {
		t.Errorf("credential_pattern = %v, queria vazio", res["credential_pattern"])
	}
	if res["credential_configured"] != false {
		t.Error("a lista mostraria o agente com chave configurada")
	}
	if res["auth_mode"] != "none" {
		t.Errorf("auth_mode = %v, queria none", res["auth_mode"])
	}
	if res["acp_command"] != "/usr/local/bin/cursor-agent" {
		t.Errorf("acp_command = %v", res["acp_command"])
	}

	salvo := registry.Get("prov")
	if salvo == nil {
		t.Fatal("o provedor sumiu na edição")
	}
	if salvo.BaseURL != "" || salvo.CredentialPattern != "" {
		t.Errorf("o que ficou salvo herdou endereço ou credencial do provedor antigo: %+v", salvo)
	}
	// O ponteiro para a credencial sai; o segredo fica. O padrão é por hostname e
	// outro provedor da mesma casa pode estar usando o mesmo — apagar aqui
	// derrubaria a autenticação dele sem ninguém pedir.
	if len(cofre.apagados) != 0 {
		t.Errorf("a edição apagou credencial do cofre: %v", cofre.apagados)
	}
	if cofre.guardados["api.openai.com"] == nil {
		t.Error("o segredo do cofre, que pode ser de outro provedor, foi perdido")
	}
}

// O caminho de volta pela mesma fronteira: o agente que vira provedor HTTP larga
// o comando e ganha endereço e credencial.
func TestVoltarAProvedorHTTPPelaFronteiraLargaOComando(t *testing.T) {
	ctrl, registry, _ := controladorDeProvedores(t)
	ctx := context.Background()

	if _, err := ctrl.CreateLLMProvider(ctx, CreateLLMProviderRequest{
		ID: "prov", Name: "Cursor local", Type: "cursor", APIFormat: "acp",
		ACPCommand: "/usr/local/bin/cursor-agent", ACPArgs: []string{"acp"},
	}); err != nil {
		t.Fatalf("Create falhou: %v", err)
	}

	res, err := ctrl.UpdateLLMProvider(ctx, "prov", UpdateLLMProviderRequest{
		Type: "openai", APIFormat: "openai",
		BaseURL: "https://api.openai.com/v1", APIKey: "sk-nova",
	})
	if err != nil {
		t.Fatalf("Update falhou: %v", err)
	}

	if res["acp_command"] != "" {
		t.Errorf("acp_command = %v, queria vazio", res["acp_command"])
	}
	if args, ok := res["acp_args"].([]string); !ok || len(args) != 0 {
		t.Errorf("acp_args = %#v, queria lista vazia", res["acp_args"])
	}
	if res["credential_pattern"] != "api.openai.com" {
		t.Errorf("credential_pattern = %v, queria o hostname novo", res["credential_pattern"])
	}
	if res["credential_configured"] != true {
		t.Error("a chave nova não apareceu como configurada")
	}
	// O "sem autenticação" era decisão de quando não havia para onde mandar
	// credencial: mantê-lo faria a API responder 401 sem explicar por quê.
	if res["auth_mode"] == "none" {
		t.Error("o provedor HTTP herdou o 'sem autenticação' do agente")
	}

	salvo := registry.Get("prov")
	if salvo == nil {
		t.Fatal("o provedor sumiu na edição")
	}
	if salvo.ACPCommand != "" || len(salvo.ACPArgs) != 0 {
		t.Errorf("sobrou configuração de agente no que ficou salvo: %+v", salvo)
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
