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

func TestCursorTemplateGuardaComandoEDispensaURLECredencial(t *testing.T) {
	install := acp.Install{
		Found:   true,
		Command: `C:\cursor-agent\versions\2026.07.23-e383d2b\node.exe`,
		Args:    []string{`C:\cursor-agent\versions\2026.07.23-e383d2b\index.js`, "acp"},
		Version: "2026.07.23-e383d2b",
	}

	p := CursorTemplate(install)

	if p.GetAPIFormat() != llm.APIFormatACP || !p.IsACP() {
		t.Fatalf("formato = %q, queria acp", p.GetAPIFormat())
	}
	if p.ACPCommand != install.Command {
		t.Errorf("comando = %q, queria %q", p.ACPCommand, install.Command)
	}
	if !slices.Equal(p.ACPArgs, install.Args) {
		t.Errorf("argumentos = %q, queria %q", p.ACPArgs, install.Args)
	}
	if p.BaseURL != "" || p.CredentialPattern != "" {
		t.Errorf("agente não tem endereço nem credencial no app: base_url=%q pattern=%q", p.BaseURL, p.CredentialPattern)
	}
	if p.AuthMode != llm.AuthModeNone {
		t.Errorf("auth_mode = %q, queria none: o login é feito no CLI do agente", p.AuthMode)
	}
	// Sem comando não há como subir o agente, e é isso que a validação cobra em
	// lugar da URL.
	if err := p.Validate(); err != nil {
		t.Errorf("template inválido: %v", err)
	}
}

func TestCursorTemplateNaoCompartilhaAListaDeArgumentosComADeteccao(t *testing.T) {
	install := acp.Install{Found: true, Command: "cursor-agent", Args: []string{"acp"}}

	p := CursorTemplate(install)
	install.Args[0] = "outro-modo"

	if p.ACPArgs[0] != "acp" {
		t.Fatalf("o provider seguiu a lista de quem detectou: %q", p.ACPArgs)
	}
}

// cursorFalsoNoPath faz esta execução ver um Cursor instalado, sem depender de
// como está a máquina de quem roda o teste. Sem isso os testes de template
// seriam pulados no CI, onde o CLI não existe — justamente onde a cobertura
// precisa valer. O arquivo não precisa funcionar: a detecção resolve caminho e
// não executa nada.
func cursorFalsoNoPath(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	nome := "cursor-agent"
	if runtime.GOOS == "windows" {
		// No Windows a detecção só aceita executável, e o LookPath procura pelas
		// extensões do PATHEXT: um arquivo sem extensão não seria encontrado.
		nome += ".exe"
	}
	caminho := filepath.Join(dir, nome)
	if err := os.WriteFile(caminho, []byte("agente falso"), 0o755); err != nil {
		t.Fatalf("não deu para criar o agente falso: %v", err)
	}

	t.Setenv("PATH", dir)
	// Um LOCALAPPDATA vazio evita que a instalação real desta máquina responda
	// antes do PATH e mude o comando esperado.
	t.Setenv("LOCALAPPDATA", filepath.Join(t.TempDir(), "sem-cursor"))
	t.Setenv("HOME", filepath.Join(t.TempDir(), "sem-cursor"))
	return caminho
}

// maquinaSemCursor deixa esta execução sem nenhum lugar onde achar o CLI.
func maquinaSemCursor(t *testing.T) {
	t.Helper()

	vazio := t.TempDir()
	t.Setenv("PATH", vazio)
	t.Setenv("LOCALAPPDATA", filepath.Join(vazio, "sem-cursor"))
	t.Setenv("HOME", filepath.Join(vazio, "sem-cursor"))
}

func TestBuiltinTemplateCursorNaoDizTipoInvalido(t *testing.T) {
	// Sem instalação, o motivo tem de ser "agente não encontrado" — e não "tipo
	// inválido", que mandaria a pessoa mexer no formulário em vez de instalar o
	// CLI.
	maquinaSemCursor(t)

	_, err := BuiltinTemplate("cursor")

	if !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("erro = %v, queria ErrAgentNotFound", err)
	}
}

func TestBuiltinTemplateCursorUsaOComandoDetectado(t *testing.T) {
	caminho := cursorFalsoNoPath(t)

	p, err := BuiltinTemplate("cursor")

	if err != nil {
		t.Fatalf("BuiltinTemplate falhou com o CLI no PATH: %v", err)
	}
	if p.ACPCommand != caminho {
		t.Errorf("comando = %q, queria o do PATH (%q)", p.ACPCommand, caminho)
	}
	if err := p.Validate(); err != nil {
		t.Errorf("template inválido: %v", err)
	}
}

func TestCreateFromTemplateRecusaChaveParaAgente(t *testing.T) {
	cursorFalsoNoPath(t)
	svc, spy := acpService(t)

	err := svc.CreateFromTemplate(context.Background(), "cursor", "sk-uma-chave")

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

func TestCreateFromTemplateCursorRegistraOAgente(t *testing.T) {
	caminho := cursorFalsoNoPath(t)
	svc, _ := acpService(t)

	if err := svc.CreateFromTemplate(context.Background(), "cursor", ""); err != nil {
		t.Fatalf("CreateFromTemplate falhou: %v", err)
	}

	registrado := svc.registry.Get("cursor-agent")
	if registrado == nil {
		t.Fatal("provedor do Cursor não ficou registrado")
	}
	if !registrado.IsACP() || registrado.ACPCommand != caminho {
		t.Errorf("provedor registrado não é o agente detectado: %+v", registrado)
	}
}

func TestCreateFromTemplateCursorSemInstalacaoExplicaOQueFalta(t *testing.T) {
	maquinaSemCursor(t)
	svc, _ := acpService(t)

	err := svc.CreateFromTemplate(context.Background(), "cursor", "")

	if !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("erro = %v, queria ErrAgentNotFound", err)
	}
	if svc.registry.Get("cursor-agent") != nil {
		t.Error("registrou um agente que não existe nesta máquina")
	}
}
