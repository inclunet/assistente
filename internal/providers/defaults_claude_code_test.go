package providers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"assistente/internal/acp"
	"assistente/internal/llm"
)

func TestClaudeCodeTemplateGuardaOAdaptadorEDispensaURLECredencial(t *testing.T) {
	install := acp.Install{
		Found:   true,
		Command: `C:\Program Files\nodejs\node.exe`,
		Args: []string{
			`C:\Program Files\nodejs\node_modules\@agentclientprotocol\claude-agent-acp\dist\index.js`,
		},
		Version:      "0.65.0",
		LoginCommand: "claude",
	}

	p := ClaudeCodeTemplate(install)

	if p.GetAPIFormat() != llm.APIFormatACP || !p.IsACP() {
		t.Fatalf("formato = %q, queria acp", p.GetAPIFormat())
	}
	if p.Type != llm.ProviderClaudeCode {
		t.Errorf("tipo = %q, queria o rótulo do Claude Code", p.Type)
	}
	if p.ACPCommand != install.Command {
		t.Errorf("comando = %q, queria %q", p.ACPCommand, install.Command)
	}
	// Sem subcomando: o adaptador sobe em ACP como está, e um `acp` a mais seria
	// argumento que ele não entende.
	if !slices.Equal(p.ACPArgs, install.Args) {
		t.Errorf("argumentos = %q, queria %q", p.ACPArgs, install.Args)
	}
	if p.BaseURL != "" || p.CredentialPattern != "" {
		t.Errorf("agente não tem endereço nem credencial no app: base_url=%q pattern=%q", p.BaseURL, p.CredentialPattern)
	}
	if p.AuthMode != llm.AuthModeNone {
		t.Errorf("auth_mode = %q, queria none: o login é feito no CLI do agente", p.AuthMode)
	}
	if err := p.Validate(); err != nil {
		t.Errorf("template inválido: %v", err)
	}
}

func TestClaudeCodeTemplateNaoSeConfundeComOProvedorHTTPDaAnthropic(t *testing.T) {
	// São dois provedores diferentes com a mesma marca: um é a API por HTTP, o
	// outro é o agente local. Compartilhar identificador faria um sobrescrever o
	// outro no registro.
	agente := ClaudeCodeTemplate(acp.Install{Found: true, Command: "claude-agent-acp"})
	http, err := BuiltinTemplate("claude")
	if err != nil {
		t.Fatalf("template da Anthropic falhou: %v", err)
	}

	if agente.ID == http.ID {
		t.Fatalf("os dois provedores da mesma marca dividem o identificador %q", agente.ID)
	}
	if agente.Type == http.Type {
		t.Errorf("tipo = %q nos dois; o agente local não é a API por HTTP", agente.Type)
	}
}

func TestClaudeCodeTemplateNaoCompartilhaAListaDeArgumentosComADeteccao(t *testing.T) {
	install := acp.Install{Found: true, Command: "node", Args: []string{"/opt/claude-agent-acp/dist/index.js"}}

	p := ClaudeCodeTemplate(install)
	install.Args[0] = "/outro/index.js"

	if p.ACPArgs[0] != "/opt/claude-agent-acp/dist/index.js" {
		t.Fatalf("o provider seguiu a lista de quem detectou: %q", p.ACPArgs)
	}
}

// adaptadorFalsoNoPath faz esta execução ver o adaptador do Claude Code
// instalado, sem depender de como está a máquina de quem roda o teste — no CI
// ele não existe, e é justamente lá que a cobertura precisa valer. O arquivo não
// precisa funcionar: a detecção resolve caminho e não executa nada.
func adaptadorFalsoNoPath(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	nome := "claude-agent-acp"
	if runtime.GOOS == "windows" {
		// No Windows a detecção só aceita executável, e o LookPath procura pelas
		// extensões do PATHEXT: um arquivo sem extensão não seria encontrado.
		nome += ".exe"
	}
	caminho := filepath.Join(dir, nome)
	if err := os.WriteFile(caminho, []byte("adaptador falso"), 0o755); err != nil {
		t.Fatalf("não deu para criar o adaptador falso: %v", err)
	}

	t.Setenv("PATH", dir)
	maquinaSemPacotesGlobais(t)
	return caminho
}

// maquinaSemPacotesGlobais tira do caminho as instalações globais do npm desta
// máquina, que responderiam antes do PATH e mudariam o comando esperado.
func maquinaSemPacotesGlobais(t *testing.T) {
	t.Helper()

	vazio := t.TempDir()
	t.Setenv("ProgramFiles", filepath.Join(vazio, "sem-node"))
	t.Setenv("APPDATA", filepath.Join(vazio, "sem-npm"))
	t.Setenv("HOME", filepath.Join(vazio, "sem-nada"))
}

// maquinaSemClaudeCode deixa esta execução sem nenhum lugar onde achar o
// adaptador.
func maquinaSemClaudeCode(t *testing.T) {
	t.Helper()

	t.Setenv("PATH", t.TempDir())
	maquinaSemPacotesGlobais(t)
}

func TestBuiltinTemplateClaudeCodeNaoDizTipoInvalido(t *testing.T) {
	// Sem adaptador, o motivo tem de ser "agente não encontrado" — e não "tipo
	// inválido", que mandaria a pessoa mexer no formulário em vez de instalar o
	// pacote.
	maquinaSemClaudeCode(t)

	_, err := BuiltinTemplate("claude-code")

	if !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("erro = %v, queria ErrAgentNotFound", err)
	}
	if !strings.Contains(err.Error(), "claude-code") {
		t.Errorf("erro não nomeia o agente procurado: %v", err)
	}
}

func TestBuiltinTemplateClaudeCodeUsaOComandoDetectado(t *testing.T) {
	caminho := adaptadorFalsoNoPath(t)

	p, err := BuiltinTemplate("claude-code")

	if err != nil {
		t.Fatalf("BuiltinTemplate falhou com o adaptador no PATH: %v", err)
	}
	if p.ACPCommand != caminho {
		t.Errorf("comando = %q, queria o do PATH (%q)", p.ACPCommand, caminho)
	}
	if err := p.Validate(); err != nil {
		t.Errorf("template inválido: %v", err)
	}
}

func TestCreateFromTemplateClaudeCodeRegistraOAgente(t *testing.T) {
	caminho := adaptadorFalsoNoPath(t)
	svc, _ := acpService(t)

	if err := svc.CreateFromTemplate(context.Background(), "claude-code", ""); err != nil {
		t.Fatalf("CreateFromTemplate falhou: %v", err)
	}

	registrado := svc.registry.Get("claude-code-agent")
	if registrado == nil {
		t.Fatal("provedor do Claude Code não ficou registrado")
	}
	if !registrado.IsACP() || registrado.ACPCommand != caminho {
		t.Errorf("provedor registrado não é o adaptador detectado: %+v", registrado)
	}
}

func TestCreateFromTemplateClaudeCodeRecusaChave(t *testing.T) {
	// A conta do Claude Code é do CLI dele: uma chave informada aqui não
	// autenticaria nada, e guardá-la em silêncio deixaria quem a informou
	// achando que autenticou.
	adaptadorFalsoNoPath(t)
	svc, spy := acpService(t)

	err := svc.CreateFromTemplate(context.Background(), "claude-code", "sk-uma-chave")

	if err == nil {
		t.Fatal("aceitou uma chave para um provedor que não tem onde usá-la")
	}
	if !strings.Contains(err.Error(), "CLI") {
		t.Errorf("erro não diz onde autenticar: %v", err)
	}
	if len(spy.registrados) != 0 {
		t.Errorf("a chave recusada foi para o cofre: %v", spy.registrados)
	}
}

func TestCreateFromTemplateClaudeCodeSemInstalacaoExplicaOQueFalta(t *testing.T) {
	maquinaSemClaudeCode(t)
	svc, _ := acpService(t)

	err := svc.CreateFromTemplate(context.Background(), "claude-code", "")

	if !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("erro = %v, queria ErrAgentNotFound", err)
	}
	if svc.registry.Get("claude-code-agent") != nil {
		t.Error("registrou um agente que não existe nesta máquina")
	}
}
