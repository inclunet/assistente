package acp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
)

// Decisão que já chegou não pode ser atropelada pelo fim do prazo: com os dois
// prontos, o select escolhe ao acaso, e o acaso aqui responde "negado" a quem
// acabou de autorizar. O laço existe porque um único sorteio poderia acertar
// por sorte.
func TestDecisaoQueJaChegouVenceOFimDoPrazo(t *testing.T) {
	morto, cancelar := context.WithCancel(context.Background())
	cancelar()

	for i := 0; i < 50; i++ {
		decidido := make(chan string, 1)
		decidido <- "permitir"
		got, decided := awaitDecision(morto, decidido, "recusar")
		if !decided || got != "permitir" {
			t.Fatalf("a decisão foi descartada pelo fim do prazo: %q (decidiu=%v)", got, decided)
		}
	}

	// Sem decisão, o prazo manda: o agente precisa de resposta.
	vazio := make(chan string)
	if got, decided := awaitDecision(morto, vazio, "recusar"); decided || got != "recusar" {
		t.Errorf("sem decisão o prazo deveria valer: %q (decidiu=%v)", got, decided)
	}
}

// O evento de opções é o conjunto completo, então entregar vazio significa "não
// há mais opção nenhuma" — e o seletor de modelo sumiria da tela. Agente que só
// manda o que ainda não consumimos não está dizendo isso.
func TestConjuntoDeOpcoesSemNadaMapeavelNaoViraEventoVazio(t *testing.T) {
	soBooleana := sdk.SessionUpdate{
		ConfigOptionUpdate: &sdk.SessionConfigOptionUpdate{
			ConfigOptions: []sdk.SessionConfigOption{
				{Boolean: &sdk.SessionConfigOptionBoolean{Id: "telemetria", Name: "Telemetria", CurrentValue: true}},
			},
		},
	}
	if update, ok := updateFrom(soBooleana); ok {
		t.Errorf("opção que não sabemos mapear virou conjunto vazio: %+v", update)
	}

	// Com algo mapeável, o evento sai normalmente.
	categoria := sdk.SessionConfigOptionCategory("model")
	comModelo := sdk.SessionUpdate{
		ConfigOptionUpdate: &sdk.SessionConfigOptionUpdate{
			ConfigOptions: []sdk.SessionConfigOption{
				{Select: &sdk.SessionConfigOptionSelect{
					Id:           "model",
					Name:         "Modelo",
					Category:     &categoria,
					CurrentValue: "modelo-a",
				}},
			},
		},
	}
	update, ok := updateFrom(comModelo)
	if !ok || update.Kind != UpdateConfigOptions || len(update.ConfigOptions) != 1 {
		t.Errorf("conjunto com modelo deveria ser entregue: %+v (ok=%v)", update, ok)
	}
}

// Uma linha gigante no stderr do agente não pode calar o leitor: é justamente
// quando o agente despeja um stack trace que o diagnóstico seguinte importa.
func TestLinhaGiganteNoStderrNaoCalaODiagnosticoSeguinte(t *testing.T) {
	registradas := make(chan string, 8)
	writer := newStderrLoggerTo(func(line string) { registradas <- line })

	go func() {
		_, _ = fmt.Fprintln(writer, "antes")
		_, _ = fmt.Fprintln(writer, strings.Repeat("x", stderrLineLimit*2))
		_, _ = fmt.Fprintln(writer, "depois")
		_ = writer.Close()
	}()

	var vistas []string
	for range 3 {
		select {
		case line := <-registradas:
			vistas = append(vistas, line)
		case <-time.After(5 * time.Second):
			t.Fatalf("o leitor parou depois de %v", vistas)
		}
	}

	if vistas[0] != "antes" {
		t.Errorf("primeira linha = %q", vistas[0])
	}
	if !strings.HasSuffix(vistas[1], "linha truncada]") {
		t.Errorf("a linha gigante deveria constar como truncada, veio com %d caracteres", len(vistas[1]))
	}
	// O que vem depois da linha gigante é o que se perderia por completo.
	if vistas[2] != "depois" {
		t.Errorf("o diagnóstico seguinte se perdeu: %q", vistas[2])
	}
	// Os pedaços da linha gigante não podem virar linhas soltas no log.
	select {
	case sobra := <-registradas:
		t.Errorf("pedaço da linha gigante virou diagnóstico solto: %.40q", sobra)
	case <-time.After(200 * time.Millisecond):
	}
}

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

	// Uma opção que o agente não ofereceu viraria resposta inválida e
	// derrubaria o turno; vale a recusa pontual.
	inventada := permissionOutcomeToSDK(PermissionOutcome{OptionID: "talvez"}, oferecidas)
	if inventada.Selected == nil || inventada.Selected.OptionId != "nao" {
		t.Fatalf("opção inexistente deveria virar recusa, obtive: %+v", inventada)
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

// Quem lê as opções não pode alcançar o estado de dentro da sessão: o agente
// troca de modelo sozinho no meio do turno, e o slice compartilhado seria
// corrida.
func TestOpcoesDaSessaoSaoEntreguesComoCopia(t *testing.T) {
	s := &session{
		turnSlot: make(chan struct{}, 1),
		options: []ConfigOption{{
			ID:     "model",
			Values: []ConfigValue{{Value: "modelo-a", Name: "Modelo A"}},
		}},
	}

	copia := s.ConfigOptions()
	copia[0].Values[0].Name = "adulterado"

	if s.ConfigOptions()[0].Values[0].Name != "Modelo A" {
		t.Error("mexer na cópia alterou o estado da sessão")
	}
}

// O prazo de cancelamento pode estourar no mesmo instante em que o turno enfim
// responde, cravando a marca depois de o turno ter acabado. O turno seguinte
// está saudável e não pode pagar por isso.
func TestMarcaDeCancelamentoAntigoNaoRecusaTurnoSaudavel(t *testing.T) {
	s := &session{
		turnSlot:       make(chan struct{}, 1),
		cancelSig:      make(chan struct{}),
		unconfirmedSig: make(chan struct{}),
	}

	cancelado := s.startTurn()
	s.markCancelUnconfirmed(cancelado)
	if !signalFired(s.unconfirmedCancel()) {
		t.Fatal("o turno cancelado sem confirmação deveria recusar o próximo")
	}

	s.startTurn()
	if signalFired(s.unconfirmedCancel()) {
		t.Error("a marca do turno anterior recusaria um turno saudável")
	}

	// A marca atrasada de um turno que já acabou não pode derrubar o atual.
	s.markCancelUnconfirmed(cancelado)
	if signalFired(s.unconfirmedCancel()) {
		t.Error("marca atrasada de turno morto recusou o turno em andamento")
	}
}

func TestBlocosDoPromptIgnoramConteudoVazio(t *testing.T) {
	if _, err := promptBlocks(nil); err == nil {
		t.Error("turno sem conteúdo deveria falhar antes de ir ao agente")
	}
	if _, err := promptBlocks([]Content{{Text: ""}}); err == nil {
		t.Error("turno só com texto vazio deveria falhar")
	}
	if _, err := promptBlocks([]Content{ImageContent("ZmFrZQ==", "")}); err == nil {
		t.Error("imagem sem tipo MIME deveria falhar aqui, e não no agente")
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
