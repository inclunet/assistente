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

// O agente que só fala o formato de antes é o caso que o formato estável não
// cobre sozinho: sem ler o `models` da abertura da sessão, ele chegaria à tela
// sem modelo nenhum para escolher, e a troca não teria por onde começar.
func TestOAgenteQueSoAnunciaModelosNoFormatoAnteriorTemModelosParaEscolher(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptSoLegado, nil)
	sess := startSession(t, client, ctx)

	option := findOption(sess.ConfigOptions(), CategoryModel)
	if option == nil {
		t.Fatalf("nenhuma opção de modelo: %+v", sess.ConfigOptions())
	}
	if option.CurrentValue != "modelo-a" {
		t.Errorf("modelo corrente = %q, queria modelo-a", option.CurrentValue)
	}
	// O nome vem do agente e é o que a pessoa lê; o identificador é o que
	// volta para ele na troca.
	if len(option.Values) != 2 || option.Values[0].Value != "modelo-a" || option.Values[0].Name != "Modelo A" {
		t.Fatalf("valores inesperados: %+v", option.Values)
	}
	if option.Values[1].Value != "modelo-b" {
		t.Errorf("segundo modelo = %q, queria modelo-b", option.Values[1].Value)
	}
}

// Escolher um modelo tem de chegar ao agente, e não só à tela: é o turno
// seguinte que prova que a leitura do formato de antes serviu para alguma coisa.
func TestOModeloDoFormatoAnteriorPodeSerTrocadoEValeParaOTurno(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptSoLegado, nil)
	sess := startSession(t, client, ctx)

	options, err := sess.SetConfigOption(ctx, "model", "modelo-b")
	if err != nil {
		t.Fatalf("trocar modelo: %v", err)
	}
	if got := findOption(options, CategoryModel); got == nil || got.CurrentValue != "modelo-b" {
		t.Fatalf("estado devolvido inesperado: %+v", options)
	}

	col := &collector{}
	if _, err := sess.Prompt(ctx, []Content{TextContent("oi")}, col.sink); err != nil {
		t.Fatalf("turno falhou: %v", err)
	}
	if got := col.textOfKind(UpdateText); got != "modelo=modelo-b" {
		t.Errorf("o agente não trocou de modelo de verdade: %q", got)
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
