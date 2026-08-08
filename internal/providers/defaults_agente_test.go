package providers

import (
	"slices"
	"strings"
	"testing"

	"assistente/internal/acp"
	"assistente/internal/llm"
)

func TestAgentProviderGuardaComandoEDispensaURLECredencial(t *testing.T) {
	install := acp.Install{
		Found:   true,
		Command: `C:\cursor-agent\versions\2026.07.23-e383d2b\node.exe`,
		Args:    []string{`C:\cursor-agent\versions\2026.07.23-e383d2b\index.js`, "acp"},
		Version: "2026.07.23-e383d2b",
	}

	p := AgentProvider("cursor-agent", "Cursor", "cursor", install)

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
		t.Errorf("provedor inválido: %v", err)
	}
}

func TestTodoAgenteTemOMesmoTipoEDizQualEleENoCampoProprio(t *testing.T) {
	// O que distingue um agente do outro é dado, e não vocabulário do app
	// (AEP-0086 D11): o tipo é o mesmo para os 38, e qual deles é aquele
	// provedor está no identificador do registro.
	cursor := AgentProvider("cursor-agent", "Cursor", "cursor", acp.Install{Command: "cursor-agent"})
	gemini := AgentProvider("gemini-agent", "Gemini CLI", "gemini-cli", acp.Install{Command: "gemini"})

	if cursor.Type != llm.ProviderACP || gemini.Type != llm.ProviderACP {
		t.Fatalf("tipos = %q e %q, queria acp nos dois", cursor.Type, gemini.Type)
	}
	if cursor.ACPAgentID != "cursor" {
		t.Errorf("agente = %q, queria cursor", cursor.ACPAgentID)
	}
	if gemini.ACPAgentID != "gemini-cli" {
		t.Errorf("agente = %q, queria gemini-cli", gemini.ACPAgentID)
	}
}

func TestAgentProviderNaoCompartilhaAListaDeArgumentosComADeteccao(t *testing.T) {
	install := acp.Install{Found: true, Command: "cursor-agent", Args: []string{"acp"}}

	p := AgentProvider("cursor-agent", "Cursor", "cursor", install)
	install.Args[0] = "outro-modo"

	if p.ACPArgs[0] != "acp" {
		t.Fatalf("o provider seguiu a lista de quem detectou: %q", p.ACPArgs)
	}
}

func TestOAgenteLocalNaoSeConfundeComOProvedorHTTPDaMesmaMarca(t *testing.T) {
	// São dois provedores diferentes com a mesma marca: um é a API por HTTP, o
	// outro é o agente local. Compartilhar identificador faria um sobrescrever o
	// outro no registro.
	agente := AgentProvider("claude-code-agent", "Claude Code", "claude-acp", acp.Install{Command: "claude-agent-acp"})
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

func TestNenhumAgenteTemTemplateEscritoAMao(t *testing.T) {
	// Enquanto Cursor e Claude Code tinham template aqui, acrescentar um agente
	// era acrescentar código, que é o que a Fase 6 do AEP-0086 apagou. Qual
	// agente subir é escolha de quem configura, feita no catálogo.
	for _, tipo := range []string{"cursor", "claude-code", "acp", "gemini-cli"} {
		p, err := BuiltinTemplate(tipo)
		if err == nil {
			t.Errorf("%q ainda tem template builtin: %+v", tipo, p)
			continue
		}
		if !strings.Contains(err.Error(), "inválido") {
			t.Errorf("%q falhou por outro motivo: %v", tipo, err)
		}
	}
}
