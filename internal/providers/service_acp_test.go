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

// Desvincular o provedor do catálogo é edição legítima, e é o único jeito de
// consertar um provedor cujo `id` o registro aposentou sem apagá-lo e refazer
// o comando à mão. O agente apontado à mão é caminho válido (AEP-0086 D3), e
// com "vazio é não mexer" não haveria como voltar para ele.
func TestEdicaoConsegueDesvincularOAgenteDoCatalogo(t *testing.T) {
	svc, _ := acpService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, CreateRequest{
		ID: "cursor", Name: "Cursor", APIFormat: string(llm.APIFormatACP),
		ACPCommand: "cursor-agent", ACPAgentID: "cursor",
	}); err != nil {
		t.Fatalf("Create falhou: %v", err)
	}

	semAgente := ""
	res, err := svc.Update(ctx, "cursor", UpdateRequest{ACPAgentID: &semAgente})
	if err != nil {
		t.Fatalf("Update falhou: %v", err)
	}
	if res.Provider.ACPAgentID != "" {
		t.Errorf("agente = %q, esperado nenhum", res.Provider.ACPAgentID)
	}
	// O comando é o que faz o agente subir, e desvincular do catálogo não é
	// pedido para deixar o provedor sem nada para executar.
	if res.Provider.ACPCommand != "cursor-agent" {
		t.Errorf("comando = %q, esperado intacto", res.Provider.ACPCommand)
	}
}

// Não mandar o campo continua sendo não mexer: uma edição que só troca o nome
// não pode desvincular o provedor do catálogo por omissão.
func TestEdicaoSemFalarDoAgenteNaoOTira(t *testing.T) {
	svc, _ := acpService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, CreateRequest{
		ID: "cursor", Name: "Cursor", APIFormat: string(llm.APIFormatACP),
		ACPCommand: "cursor-agent", ACPAgentID: "cursor",
	}); err != nil {
		t.Fatalf("Create falhou: %v", err)
	}

	res, err := svc.Update(ctx, "cursor", UpdateRequest{Name: "Cursor do trabalho"})
	if err != nil {
		t.Fatalf("Update falhou: %v", err)
	}
	if res.Provider.ACPAgentID != "cursor" {
		t.Errorf("agente = %q, esperado intacto", res.Provider.ACPAgentID)
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

// Desligar a passagem de credencial é mandar o mapa vazio, e precisa funcionar
// na hora: é a reversão que o D12 promete, e sem ela a única saída seria apagar
// o provedor.
func TestEdicaoConsegueDesligarACredencialDoCofre(t *testing.T) {
	svc, _ := acpService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, CreateRequest{
		ID: "codex", Name: "Codex", APIFormat: string(llm.APIFormatACP),
		ACPCommand:       "codex-acp",
		ACPCredentialEnv: map[string]string{"OPENAI_API_KEY": "api.openai.com"},
	}); err != nil {
		t.Fatalf("Create falhou: %v", err)
	}

	vazio := map[string]string{}
	res, err := svc.Update(ctx, "codex", UpdateRequest{ACPCredentialEnv: &vazio})
	if err != nil {
		t.Fatalf("Update falhou: %v", err)
	}
	if len(res.Provider.ACPCredentialEnv) != 0 {
		t.Errorf("credenciais do cofre = %#v, esperado nenhuma", res.Provider.ACPCredentialEnv)
	}
	if res.Provider.ACPCommand != "codex-acp" {
		t.Errorf("comando = %q, esperado intacto", res.Provider.ACPCommand)
	}
}

// Ligar a passagem guarda a referência, e nada mais: o cofre não recebe entrada
// nova por causa disso, e o provedor continua sem credencial própria.
func TestCriarAgenteComCredencialDoCofreGuardaSoAReferencia(t *testing.T) {
	svc, spy := acpService(t)

	res, err := svc.Create(context.Background(), CreateRequest{
		ID: "codex", Name: "Codex", APIFormat: string(llm.APIFormatACP),
		ACPCommand:       "codex-acp",
		ACPCredentialEnv: map[string]string{"OPENAI_API_KEY": "api.openai.com"},
	})
	if err != nil {
		t.Fatalf("Create falhou: %v", err)
	}
	if res.Provider.ACPCredentialEnv["OPENAI_API_KEY"] != "api.openai.com" {
		t.Errorf("credenciais do cofre = %#v", res.Provider.ACPCredentialEnv)
	}
	if len(spy.registrados) != 0 {
		t.Errorf("o cofre ganhou entrada por causa da referência: %v", spy.registrados)
	}
	if res.Provider.CredentialPattern != "" {
		t.Errorf("credential_pattern = %q, esperado vazio: o agente não faz chamada HTTP", res.Provider.CredentialPattern)
	}
}

// Provedor HTTP não tem processo onde a variável pudesse existir, e guardar o
// par em silêncio faria alguém achar que configurou algo.
func TestCredencialDoCofrePorVariavelExigeAgente(t *testing.T) {
	svc, _ := acpService(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, CreateRequest{
		ID: "openai", Name: "OpenAI", Type: string(llm.ProviderOpenAI),
		BaseURL:          "https://api.openai.com/v1",
		ACPCredentialEnv: map[string]string{"OPENAI_API_KEY": "api.openai.com"},
	})
	if err == nil || !strings.Contains(err.Error(), "api_format") {
		t.Fatalf("erro = %v, esperado cobrar o formato acp", err)
	}

	if _, err := svc.Create(ctx, CreateRequest{
		ID: "openai", Name: "OpenAI", Type: string(llm.ProviderOpenAI),
		BaseURL: "https://api.openai.com/v1",
	}); err != nil {
		t.Fatalf("Create falhou: %v", err)
	}
	cofre := map[string]string{"OPENAI_API_KEY": "api.openai.com"}
	if _, err := svc.Update(ctx, "openai", UpdateRequest{ACPCredentialEnv: &cofre}); err == nil {
		t.Fatal("esperava recusa de credencial por variável em provedor HTTP")
	}
}

// Voltar um agente para HTTP tem que ser edição, e não apagar e recriar: o
// comando antigo sai junto com o formato, senão a própria validação — que
// recusa comando em provedor HTTP — travaria a mudança.
func TestAgentePodeVoltarASerProvedorHTTP(t *testing.T) {
	svc, _ := acpService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, CreateRequest{
		ID: "cursor", Name: "Cursor", Type: string(llm.ProviderOpenAI),
		APIFormat:  string(llm.APIFormatACP),
		ACPCommand: "cursor-agent", ACPArgs: []string{"acp"},
		ACPEnv:           map[string]string{"CURSOR_LOG": "debug"},
		ACPCredentialEnv: map[string]string{"CURSOR_API_KEY": "api.cursor.com"},
	}); err != nil {
		t.Fatalf("Create falhou: %v", err)
	}

	// Sem mexer no tipo: é a edição mínima que troca o formato, e a que
	// deixaria o modo de autenticação do agente para trás.
	res, err := svc.Update(ctx, "cursor", UpdateRequest{
		APIFormat: string(llm.APIFormatOpenAI),
		BaseURL:   "https://api.openai.com/v1",
	})
	if err != nil {
		t.Fatalf("Update falhou: %v", err)
	}
	if res.Provider.ACPCommand != "" || len(res.Provider.ACPArgs) != 0 || len(res.Provider.ACPEnv) != 0 ||
		len(res.Provider.ACPCredentialEnv) != 0 {
		t.Errorf("sobrou configuração de agente: %+v", res.Provider)
	}
	if res.Provider.CredentialPattern != "api.openai.com" {
		t.Errorf("credential_pattern = %q, esperado o hostname novo", res.Provider.CredentialPattern)
	}
	// O "sem autenticação" era decisão do agente. Se ficasse, o provedor HTTP
	// chamaria a API sem a chave que acabou de ganhar.
	if res.Provider.EffectiveAuthMode() == llm.AuthModeNone {
		t.Error("o provedor HTTP herdou o 'sem autenticação' do agente")
	}
}

// Formato e URL chegam de formulário e de linha de comando: espaço nas pontas
// não pode mudar o caminho da criação nem o que vai para o banco.
func TestEspacoNasPontasNaoMudaOCaminhoDaCriacao(t *testing.T) {
	t.Run("agente", func(t *testing.T) {
		svc, _ := acpService(t)
		res, err := svc.Create(context.Background(), CreateRequest{
			ID: "cursor", Name: "Cursor", APIFormat: "  acp  ",
			ACPCommand: "cursor-agent",
		})
		if err != nil {
			t.Fatalf("Create falhou: %v", err)
		}
		if !res.Provider.IsACP() {
			t.Errorf("api_format = %q, esperado agente", res.Provider.APIFormat)
		}
	})

	t.Run("http", func(t *testing.T) {
		svc, _ := acpService(t)
		res, err := svc.Create(context.Background(), CreateRequest{
			ID: "openai", Name: "OpenAI", Type: string(llm.ProviderOpenAI),
			BaseURL: "  https://api.openai.com/v1  ",
		})
		if err != nil {
			t.Fatalf("Create falhou: %v", err)
		}
		if res.CredentialPattern != "api.openai.com" {
			t.Errorf("hostname = %q", res.CredentialPattern)
		}
		if res.Provider.BaseURL != "https://api.openai.com/v1" {
			t.Errorf("base_url = %q, esperado sem espaços", res.Provider.BaseURL)
		}
	})
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

	// Só espaços não é edição nenhuma: não vira comando nem erro.
	if _, err := svc.Update(ctx, "openai", UpdateRequest{ACPCommand: "   "}); err != nil {
		t.Errorf("campo em branco não deveria virar tentativa de configurar agente: %v", err)
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
