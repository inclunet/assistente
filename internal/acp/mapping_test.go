package acp

import (
	"errors"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
)

func TestNormalizeIDLimpaIdentificadoresDoAgente(t *testing.T) {
	casos := []struct {
		nome     string
		entrada  string
		esperado string
	}{
		{"quebra de linha vira espaço", "call-abc\nfc_123", "call-abc fc_123"},
		{"retorno de carro também", "call\r\nfc", "call fc"},
		{"controle é removido", "call\x00fc", "callfc"},
		{"espaços em excesso colapsam", "  call   fc  ", "call fc"},
		{"identificador simples não muda", "call-1", "call-1"},
		{"vazio continua vazio", "", ""},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			if got := normalizeID(caso.entrada); got != caso.esperado {
				t.Errorf("normalizeID(%q) = %q, esperado %q", caso.entrada, got, caso.esperado)
			}
		})
	}
}

func TestBackoffCresceAteOTeto(t *testing.T) {
	if got := backoffFor(1); got != backoffBase {
		t.Errorf("primeira falha = %v, esperado %v", got, backoffBase)
	}
	if got := backoffFor(3); got != 4*time.Second {
		t.Errorf("terceira falha = %v, esperado 4s", got)
	}
	if got := backoffFor(50); got != backoffMax {
		t.Errorf("falha persistente = %v, esperado teto %v", got, backoffMax)
	}
}

func TestTurnAceitoEhClassificadoDeFormaConservadora(t *testing.T) {
	casos := []struct {
		nome     string
		err      error
		esperado bool
	}{
		{"sucesso conta como aceito", nil, true},
		{"agente recusou o método", sdk.NewMethodNotFound("session/prompt"), false},
		{"parâmetros inválidos", sdk.NewInvalidParams(nil), false},
		{"falta de autenticação", sdk.NewAuthRequired(nil), false},
		// Queda depois do envio é o caso ambíguo: o pedido pode já estar
		// editando arquivos, então assume-se aceito para ninguém repetir sozinho.
		{"queda depois do envio", sdk.NewInternalError(nil), true},
		{"erro desconhecido", errors.New("timeout"), true},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			if got := turnAccepted(caso.err); got != caso.esperado {
				t.Errorf("turnAccepted = %t, esperado %t", got, caso.esperado)
			}
		})
	}
}

func TestDesfechoDePermissaoPrefereRecusaPontual(t *testing.T) {
	oferecidas := []sdk.PermissionOption{
		{OptionId: "sim", Kind: sdk.PermissionOptionKindAllowOnce},
		{OptionId: "sempre", Kind: sdk.PermissionOptionKindAllowAlways},
		{OptionId: "nao", Kind: sdk.PermissionOptionKindRejectOnce},
		{OptionId: "nunca", Kind: sdk.PermissionOptionKindRejectAlways},
	}

	escolhido := permissionOutcomeToSDK(PermissionOutcome{OptionID: "sim"}, oferecidas)
	if escolhido.Selected == nil || escolhido.Selected.OptionId != "sim" {
		t.Fatalf("escolha explícita não foi respeitada: %+v", escolhido)
	}

	semDecisao := permissionOutcomeToSDK(PermissionOutcome{}, oferecidas)
	if semDecisao.Selected == nil || semDecisao.Selected.OptionId != "nao" {
		t.Fatalf("sem decisão deveria negar pontualmente, obtive: %+v", semDecisao)
	}

	// Sem recusa pontual oferecida, resta o cancelamento — nunca a recusa
	// permanente, que calaria o agente sem ninguém ter decidido isso.
	semRecusa := permissionOutcomeToSDK(PermissionOutcome{}, []sdk.PermissionOption{
		{OptionId: "sim", Kind: sdk.PermissionOptionKindAllowOnce},
		{OptionId: "nunca", Kind: sdk.PermissionOptionKindRejectAlways},
	})
	if semRecusa.Cancelled == nil || semRecusa.Selected != nil {
		t.Fatalf("esperava desfecho cancelado, obtive: %+v", semRecusa)
	}
}

func TestBlocosDoPromptIgnoramConteudoVazio(t *testing.T) {
	if _, err := promptBlocks(nil); err == nil {
		t.Error("turno sem conteúdo deveria falhar antes de ir ao agente")
	}
	if _, err := promptBlocks([]Content{{Text: ""}}); err == nil {
		t.Error("turno só com texto vazio deveria falhar")
	}
	blocos, err := promptBlocks([]Content{TextContent("oi"), ImageContent("ZmFrZQ==", "image/png")})
	if err != nil {
		t.Fatalf("blocos válidos: %v", err)
	}
	if len(blocos) != 2 || blocos[0].Text == nil || blocos[1].Image == nil {
		t.Fatalf("blocos inesperados: %+v", blocos)
	}
}

func TestModoLegadoNaoDuplicaOFormatoEstavel(t *testing.T) {
	estado := &sdk.SessionModeState{
		CurrentModeId:  "agent",
		AvailableModes: []sdk.SessionMode{{Id: "agent", Name: "Agente"}},
	}

	semModo := withModeOption([]ConfigOption{{ID: "model", Category: "model"}}, estado)
	if len(semModo) != 2 || semModo[1].Category != "mode" {
		t.Fatalf("modo legado deveria ter sido acrescentado: %+v", semModo)
	}

	// O Cursor manda os dois formatos no mesmo payload; a lista não pode vir
	// com o modo duas vezes.
	comModo := withModeOption([]ConfigOption{{ID: "mode", Category: "mode"}}, estado)
	if len(comModo) != 1 {
		t.Fatalf("modo não deveria ser duplicado: %+v", comModo)
	}

	if got := withModeOption(nil, nil); got != nil {
		t.Errorf("sem modos, nada a acrescentar: %+v", got)
	}
}
