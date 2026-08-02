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

// Configuração de agente num provedor HTTP é recusada antes de qualquer
// efeito colateral: a validação do registro só roda depois de a credencial ir
// para o cofre, e um provedor que nem chegou a existir não pode deixar segredo
// para trás.
func TestConfiguracaoDeAgenteEmProvedorHTTPParaAntesDoCofre(t *testing.T) {
	casos := map[string]CreateRequest{
		"comando":    {ACPCommand: "cursor-agent"},
		"argumentos": {ACPArgs: []string{"acp"}},
		"ambiente":   {ACPEnv: map[string]string{"CURSOR_LOG": "debug"}},
	}

	for nome, extra := range casos {
		t.Run(nome, func(t *testing.T) {
			svc, spy := acpService(t)
			req := extra
			req.ID = "openai"
			req.Name = "OpenAI"
			req.Type = string(llm.ProviderOpenAI)
			req.BaseURL = "https://api.openai.com/v1"
			req.APIKey = "sk-secreta"

			if _, err := svc.Create(context.Background(), req); err == nil {
				t.Fatal("esperava recusa da configuração de agente")
			}
			if len(spy.registrados) != 0 {
				t.Errorf("a credencial foi para o cofre de um provedor que não existe: %v", spy.registrados)
			}
			if svc.registry.Get("openai") != nil {
				t.Error("o provedor não deveria ter sido criado")
			}
		})
	}
}

// Virar agente é perder o endereço. A URL antiga ficaria no banco sem ninguém
// para usá-la, e quem for depurar a linha vai acreditar nela.
func TestVirarAgenteApagaOEnderecoAntigo(t *testing.T) {
	svc, _ := acpService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, CreateRequest{
		ID: "prov", Name: "Prov", Type: string(llm.ProviderOpenAI),
		BaseURL: "https://api.openai.com/v1",
	}); err != nil {
		t.Fatalf("Create falhou: %v", err)
	}

	res, err := svc.Update(ctx, "prov", UpdateRequest{
		APIFormat:  string(llm.APIFormatACP),
		ACPCommand: "cursor-agent",
		ACPArgs:    &[]string{"acp"},
	})
	if err != nil {
		t.Fatalf("Update falhou: %v", err)
	}
	if res.Provider.BaseURL != "" {
		t.Errorf("base_url = %q, esperado vazio", res.Provider.BaseURL)
	}
	if res.Provider.CredentialPattern != "" {
		t.Errorf("credential_pattern = %q, esperado vazio", res.Provider.CredentialPattern)
	}
	if res.Provider.AuthMode != llm.AuthModeNone {
		t.Errorf("auth_mode = %q, esperado none", res.Provider.AuthMode)
	}
}

// Voltar um agente para HTTP tem que ser edição, e não apagar e recriar: o
// comando antigo sai junto com o formato, senão a própria validação — que
// recusa comando em provedor HTTP — travaria a mudança.
func TestAgentePodeVoltarASerProvedorHTTP(t *testing.T) {
	svc, _ := acpService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, CreateRequest{
		ID: "cursor", Name: "Cursor", APIFormat: string(llm.APIFormatACP),
		ACPCommand: "cursor-agent", ACPArgs: []string{"acp"},
		ACPEnv: map[string]string{"CURSOR_LOG": "debug"},
	}); err != nil {
		t.Fatalf("Create falhou: %v", err)
	}

	res, err := svc.Update(ctx, "cursor", UpdateRequest{
		APIFormat: string(llm.APIFormatOpenAI),
		BaseURL:   "https://api.openai.com/v1",
	})
	if err != nil {
		t.Fatalf("Update falhou: %v", err)
	}
	if res.Provider.ACPCommand != "" || len(res.Provider.ACPArgs) != 0 || len(res.Provider.ACPEnv) != 0 {
		t.Errorf("sobrou configuração de agente: %+v", res.Provider)
	}
	if res.Provider.CredentialPattern != "api.openai.com" {
		t.Errorf("credential_pattern = %q, esperado o hostname novo", res.Provider.CredentialPattern)
	}
}

// Mandar comando de agente para um provedor que continua HTTP é engano de quem
// edita: recusar avisa, e limpar em silêncio deixaria a pessoa achando que
// configurou alguma coisa.
func TestComandoDeAgenteEmProvedorHTTPEhRecusadoNaEdicao(t *testing.T) {
	svc, _ := acpService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, CreateRequest{
		ID: "openai", Name: "OpenAI", Type: string(llm.ProviderOpenAI),
		BaseURL: "https://api.openai.com/v1",
	}); err != nil {
		t.Fatalf("Create falhou: %v", err)
	}

	if _, err := svc.Update(ctx, "openai", UpdateRequest{ACPCommand: "cursor-agent"}); err == nil {
		t.Fatal("esperava recusa do comando em provedor HTTP")
	}
	if got := svc.registry.Get("openai"); got == nil || got.ACPCommand != "" {
		t.Errorf("o provedor não deveria ter guardado o comando: %#v", got)
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
