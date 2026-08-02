package providers

import (
	"context"
	"strings"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/llm"
)

// credSpy registra o que o serviço tentaria guardar de segredo. Para um agente
// ACP a resposta certa é "nada".
type credSpy struct {
	registrados []string
}

func (c *credSpy) RegisterPatternWithContext(_ context.Context, pattern string, _ *credentials.AuthConfig) error {
	c.registrados = append(c.registrados, pattern)
	return nil
}

func (c *credSpy) GetByPattern(string) (*credentials.AuthConfig, error) { return nil, nil }

func (c *credSpy) GetByPatternWithContext(context.Context, string) (*credentials.AuthConfig, error) {
	return nil, nil
}

func (c *credSpy) DeletePattern(context.Context, string) error { return nil }

func acpService(t *testing.T) (*Service, *credSpy) {
	t.Helper()
	spy := &credSpy{}
	return NewService(ServiceConfig{
		Registry: llm.NewProviderRegistry(),
		CredMgr:  spy,
		Store:    NewMemoryStore(),
	}), spy
}

func TestCriarProvedorACPDispensaURLECredencial(t *testing.T) {
	svc, spy := acpService(t)

	res, err := svc.Create(context.Background(), CreateRequest{
		ID: "cursor", Name: "Cursor", Type: string(llm.ProviderCustom),
		APIFormat:  string(llm.APIFormatACP),
		ACPCommand: "cursor-agent",
		ACPArgs:    []string{"acp"},
		ACPEnv:     map[string]string{"CURSOR_LOG": "debug"},
	})
	if err != nil {
		t.Fatalf("Create falhou: %v", err)
	}
	if res.CredentialPattern != "" || res.CredentialConfigured {
		t.Errorf("agente não tem hostname para casar credencial: %+v", res)
	}
	if len(spy.registrados) != 0 {
		t.Errorf("nada deveria ter ido para o cofre, foi %v", spy.registrados)
	}
	if res.Provider.AuthMode != llm.AuthModeNone {
		t.Errorf("auth_mode = %q, esperado none", res.Provider.AuthMode)
	}
	if got := svc.registry.Get("cursor"); got == nil || got.ACPArgs[0] != "acp" {
		t.Errorf("provedor não ficou registrado com os argumentos: %#v", got)
	}
}

// Uma chave mandada para um agente não autenticaria nada: o login é feito no
// CLI dele. Recusar avisa a pessoa; aceitar guardaria um segredo inútil.
func TestCriarProvedorACPRecusaChaveDeAPI(t *testing.T) {
	svc, spy := acpService(t)

	_, err := svc.Create(context.Background(), CreateRequest{
		ID: "cursor", Name: "Cursor", APIFormat: string(llm.APIFormatACP),
		ACPCommand: "cursor-agent", APIKey: "sk-secreta",
	})
	if err == nil {
		t.Fatal("esperava recusa da chave")
	}
	if len(spy.registrados) != 0 {
		t.Errorf("a chave chegou ao cofre mesmo com a recusa: %v", spy.registrados)
	}
	if svc.registry.Get("cursor") != nil {
		t.Error("o provedor não deveria ter sido criado")
	}
}

func TestCriarProvedorACPSemComandoFalha(t *testing.T) {
	svc, _ := acpService(t)

	_, err := svc.Create(context.Background(), CreateRequest{
		ID: "cursor", Name: "Cursor", APIFormat: string(llm.APIFormatACP),
	})
	if err == nil || !strings.Contains(err.Error(), "acp_command") {
		t.Fatalf("erro = %v, esperado cobrar o comando", err)
	}
}

// Tirar todos os argumentos é edição legítima: com "vazio é não mexer" ela
// seria impossível, e o agente continuaria subindo no modo antigo.
func TestEdicaoConsegueLimparOsArgumentosDoAgente(t *testing.T) {
	svc, _ := acpService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, CreateRequest{
		ID: "cursor", Name: "Cursor", APIFormat: string(llm.APIFormatACP),
		ACPCommand: "cursor-agent", ACPArgs: []string{"acp", "--force"},
	}); err != nil {
		t.Fatalf("Create falhou: %v", err)
	}

	vazio := []string{}
	res, err := svc.Update(ctx, "cursor", UpdateRequest{ACPArgs: &vazio})
	if err != nil {
		t.Fatalf("Update falhou: %v", err)
	}
	if len(res.Provider.ACPArgs) != 0 {
		t.Errorf("argumentos = %#v, esperado nenhum", res.Provider.ACPArgs)
	}

	// E o que não foi mandado continua onde estava.
	if res.Provider.ACPCommand != "cursor-agent" {
		t.Errorf("comando = %q, esperado intacto", res.Provider.ACPCommand)
	}
}

// A edição troca o provedor removendo e registrando de novo. Se a validação
// só acontecesse no registro, uma edição inválida faria o provedor sumir da
// lista em vez de a edição falhar.
func TestEdicaoInvalidaNaoFazOProvedorSumir(t *testing.T) {
	svc, _ := acpService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, CreateRequest{
		ID: "openai", Name: "OpenAI", Type: string(llm.ProviderOpenAI),
		BaseURL: "https://api.openai.com/v1",
	}); err != nil {
		t.Fatalf("Create falhou: %v", err)
	}

	if _, err := svc.Update(ctx, "openai", UpdateRequest{APIFormat: string(llm.APIFormatACP)}); err == nil {
		t.Fatal("virar agente sem informar comando deveria falhar")
	}
	sobrevivente := svc.registry.Get("openai")
	if sobrevivente == nil {
		t.Fatal("o provedor sumiu da lista por causa de uma edição recusada")
	}
	if sobrevivente.GetAPIFormat() == llm.APIFormatACP {
		t.Error("a edição recusada não podia ter mudado o formato")
	}
}
