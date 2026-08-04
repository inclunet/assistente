package acp

import (
	"strings"
	"testing"
)

func TestTrocaDeModeloValeParaOTurnoSeguinte(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptModelo, nil)
	sess := startSession(t, client, ctx)

	if _, err := sess.SetConfigOption(ctx, "model", "modelo-b"); err != nil {
		t.Fatalf("trocar modelo: %v", err)
	}

	col := &collector{}
	if _, err := sess.Prompt(ctx, []Content{TextContent("em que modelo você está?")}, col.sink); err != nil {
		t.Fatalf("turno falhou: %v", err)
	}
	// Quem responde é o agente, dizendo em que modelo ele está. Conferir o
	// estado guardado no app provaria só que o app anotou o que quis.
	if got := col.textOfKind(UpdateText); got != "modelo=modelo-b" {
		t.Errorf("o turno seguinte não foi no modelo escolhido: %q", got)
	}
}

func TestTrocaDeModeloUsaOSeletorAnteriorQuandoOAgenteNaoConheceOFormatoEstavel(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptLegado, nil)
	sess := startSession(t, client, ctx)

	options, err := sess.SetConfigOption(ctx, "model", "modelo-b")
	if err != nil {
		t.Fatalf("trocar modelo pelo seletor anterior: %v", err)
	}
	// O seletor anterior não devolve estado nenhum: o app anota o que pediu, ou
	// a tela seguiria anunciando o modelo antigo depois de uma troca que valeu.
	if got := findOption(options, "model"); got == nil || got.CurrentValue != "modelo-b" {
		t.Fatalf("estado devolvido inesperado: %+v", options)
	}

	col := &collector{}
	if _, err := sess.Prompt(ctx, []Content{TextContent("oi")}, col.sink); err != nil {
		t.Fatalf("turno falhou: %v", err)
	}
	if got := col.textOfKind(UpdateText); got != "modelo=modelo-b" {
		t.Errorf("o agente legado não trocou de modelo de verdade: %q", got)
	}
}

func TestTrocaDeModoUsaOSeletorAnteriorQuandoOAgenteNaoConheceOFormatoEstavel(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptLegado, nil)
	sess := startSession(t, client, ctx)

	options, err := sess.SetConfigOption(ctx, "mode", "plan")
	if err != nil {
		t.Fatalf("trocar modo pelo seletor anterior: %v", err)
	}
	if got := findOption(options, modeCategory); got == nil || got.CurrentValue != "plan" {
		t.Errorf("o modo não acompanhou a troca: %+v", options)
	}
}

func TestTrocaRecusadaPeloAgenteNaoViraTentativaNoSeletorAnterior(t *testing.T) {
	ctx := testContext(t)
	// Este agente conhece o formato estável e recusa o valor: repetir o pedido
	// pelo caminho de antes não o faria aceitar, e ainda esconderia o motivo.
	client := newTestClient(t, scriptTurn, nil)
	sess := startSession(t, client, ctx)

	if _, err := sess.SetConfigOption(ctx, "model", "modelo-b"); err != nil {
		t.Fatalf("este agente aceita a troca no formato estável: %v", err)
	}

	if _, err := sess.SetConfigOption(ctx, "inexistente", "x"); err == nil {
		t.Fatal("trocar uma opção que o agente recusa deveria falhar")
	} else if strings.Contains(err.Error(), legacySetModelMethod) {
		t.Errorf("a recusa virou tentativa no seletor anterior: %v", err)
	}
}
