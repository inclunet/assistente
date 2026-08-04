package providers

import (
	"context"
	"errors"
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

func TestBuiltinTemplateCursorNaoDizTipoInvalido(t *testing.T) {
	// A máquina que roda o teste pode ou não ter o Cursor instalado, e o teste
	// não decide isso. O que precisa valer nos dois casos: quando não há
	// instalação, o motivo é "agente não encontrado" — e não "tipo inválido",
	// que mandaria a pessoa mexer no formulário em vez de instalar o CLI.
	p, err := BuiltinTemplate("cursor")
	if err != nil {
		if !errors.Is(err, ErrAgentNotFound) {
			t.Fatalf("erro = %v, queria ErrAgentNotFound", err)
		}
		return
	}
	if p.ACPCommand == "" {
		t.Fatalf("template devolvido sem comando do agente: %+v", p)
	}
	if err := p.Validate(); err != nil {
		t.Errorf("template inválido: %v", err)
	}
}

func TestCreateFromTemplateRecusaChaveParaAgente(t *testing.T) {
	if _, err := BuiltinTemplate("cursor"); err != nil {
		t.Skipf("Cursor não instalado nesta máquina: %v", err)
	}
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
	if _, err := BuiltinTemplate("cursor"); err != nil {
		t.Skipf("Cursor não instalado nesta máquina: %v", err)
	}
	svc, _ := acpService(t)

	if err := svc.CreateFromTemplate(context.Background(), "cursor", ""); err != nil {
		t.Fatalf("CreateFromTemplate falhou: %v", err)
	}

	registrado := svc.registry.Get("cursor-agent")
	if registrado == nil {
		t.Fatal("provedor do Cursor não ficou registrado")
	}
	if !registrado.IsACP() || registrado.ACPCommand == "" {
		t.Errorf("provedor registrado não é um agente utilizável: %+v", registrado)
	}
}
