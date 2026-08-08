package acp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
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

// Se o agente confirmou o fim do turno no mesmo instante em que o prazo
// estourou, vale o que ele disse. O contrário marcaria a sessão como
// "cancelamento não confirmado" e recusaria o próximo turno por um motivo que
// não existe — e o agente, que parou direitinho, levaria a fama de solto.
func TestRespostaDoAgenteVenceOPrazoDeGraca(t *testing.T) {
	for i := 0; i < 50; i++ {
		s := &session{
			id:             "sess-teste",
			turnSlot:       make(chan struct{}, 1),
			unconfirmedSig: make(chan struct{}),
			closedSig:      make(chan struct{}),
		}
		done := make(chan promptOutcome, 1)
		done <- promptOutcome{stop: StopCancelled}
		expirado := make(chan time.Time, 1)
		expirado <- time.Now()

		stop, err := s.awaitCancelled(1, done, expirado)
		if err != nil || stop != StopCancelled {
			t.Fatalf("a confirmação do agente foi ignorada: stop=%q err=%v", stop, err)
		}
		if signalFired(s.unconfirmedSig) {
			t.Fatal("a sessão foi marcada como não confirmada mesmo com o agente tendo confirmado")
		}
	}

	// Sem resposta, o prazo manda: quem chamou precisa saber que o agente pode
	// ter ficado solto.
	s := &session{
		id:             "sess-teste",
		turnSlot:       make(chan struct{}, 1),
		unconfirmedSig: make(chan struct{}),
		closedSig:      make(chan struct{}),
	}
	expirado := make(chan time.Time, 1)
	expirado <- time.Now()
	if _, err := s.awaitCancelled(1, make(chan promptOutcome), expirado); !errors.Is(err, ErrCancelNotConfirmed) {
		t.Errorf("sem resposta o prazo deveria valer, obtive: %v", err)
	}
}

// Quando a conversa é excluída, dizer se o turno chegou a sair para o agente
// não pode depender de quem ganhou a corrida: é esse sinal que decide se uma
// retentativa automática mexeria no disco de novo.
func TestConversaExcluidaNaoInventaQueOTurnoSaiu(t *testing.T) {
	novaSessao := func() *session {
		return &session{id: "sess-teste", turnSlot: make(chan struct{}, 1), closedSig: make(chan struct{})}
	}

	// A goroutine já disse que o pedido nem saiu: retentar é seguro.
	naoSaiu := make(chan promptOutcome, 1)
	naoSaiu <- promptOutcome{err: ErrSessionClosed}
	_, err := novaSessao().closedOutcome(naoSaiu)
	var falha *PromptError
	if !errors.As(err, &falha) || !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("erro inesperado: %v", err)
	}
	if falha.Accepted {
		t.Error("o turno não chegou a sair e não deveria constar como aceito")
	}

	// Sem desfecho pronto, o pedido está em voo: retentar mexeria no disco de
	// novo.
	_, err = novaSessao().closedOutcome(make(chan promptOutcome))
	if !errors.As(err, &falha) || !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !falha.Accepted {
		t.Error("o turno estava em voo e deveria constar como aceito")
	}
}

// O evento de opções se anuncia como o conjunto completo, então quem escuta
// precisa receber o mesmo conjunto que a sessão passou a guardar. Um agente que
// manda só os modelos não pode fazer o seletor de modo sumir da tela no meio da
// conversa enquanto a sessão ainda diz que o modo está lá.
// sessaoSolta monta uma sessão fora de qualquer processo, para os testes que
// olham só o estado dela. A conexão é vazia, mas existe: a entrega de uma
// atualização de opções mexe no cache de descoberta do processo (AEP-0084 D6), e
// uma sessão sem conexão nenhuma é estado que a produção não constrói.
func sessaoSolta(options []ConfigOption) *session {
	return newSession("sess-teste", "/tmp", &conn{}, options)
}

func TestOModoPreservadoTambemChegaAQuemEscuta(t *testing.T) {
	s := sessaoSolta([]ConfigOption{
		{ID: "mode", Category: modeCategory, CurrentValue: "plan", Values: []ConfigValue{{Value: "plan"}}},
		{ID: "model", Category: "model", CurrentValue: "antigo", Values: []ConfigValue{{Value: "antigo"}}},
	})

	var entregue Update
	s.setSink(func(u Update) { entregue = u })
	s.deliver(Update{Kind: UpdateConfigOptions, ConfigOptions: []ConfigOption{
		{ID: "model", Category: "model", CurrentValue: "novo", Values: []ConfigValue{{Value: "novo"}}},
	}})

	if findOption(entregue.ConfigOptions, modeCategory) == nil {
		t.Error("o modo sumiu do evento entregue a quem escuta")
	}
	if findOption(s.ConfigOptions(), modeCategory) == nil {
		t.Error("o modo sumiu do estado da sessão")
	}
	if len(entregue.ConfigOptions) != len(s.ConfigOptions()) {
		t.Errorf("evento e estado divergem: %d opções entregues, %d guardadas",
			len(entregue.ConfigOptions), len(s.ConfigOptions()))
	}

	// O que foi entregue continua sendo de quem escuta: mexer nele não pode
	// reescrever o estado da conversa.
	entregue.ConfigOptions[0].CurrentValue = "adulterado"
	if findOption(s.ConfigOptions(), "model").CurrentValue != "novo" {
		t.Error("quem escuta conseguiu trocar o modelo da sessão sem passar pelo agente")
	}
}

// Dois caminhos escrevem nas opções ao mesmo tempo: o anúncio que o agente
// manda por conta própria e a troca que a pessoa pede — que o ACP admite no
// meio do turno. Se a fusão ler, juntar e gravar em passos separados, o anúncio
// que começou antes grava por cima do que foi trocado depois, e o modo volta
// sozinho para o anterior na tela de quem acabou de mudá-lo.
func TestOModoTrocadoNaoVoltaSozinhoComAnuncioConcorrente(t *testing.T) {
	s := sessaoSolta([]ConfigOption{
		{ID: "mode", Category: modeCategory, CurrentValue: "agente", Values: []ConfigValue{{Value: "agente"}, {Value: "plano"}}},
		{ID: "model", Category: "model", CurrentValue: "antigo", Values: []ConfigValue{{Value: "antigo"}}},
	})

	const rodadas = 2000
	var wg sync.WaitGroup
	parar := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-parar:
				return
			default:
			}
			s.mergeConfigOptions([]ConfigOption{
				{ID: "model", Category: "model", CurrentValue: "novo", Values: []ConfigValue{{Value: "novo"}}},
			})
		}
	}()

	// Cada rodada troca por um modo novo e confere logo em seguida: o valor lido
	// nunca pode ser mais antigo do que o que acabou de ser gravado.
	for i := 1; i <= rodadas; i++ {
		s.setCurrentMode(fmt.Sprintf("modo-%d", i))
		lido := findOption(s.ConfigOptions(), modeCategory).CurrentValue
		var visto int
		if _, err := fmt.Sscanf(lido, "modo-%d", &visto); err != nil || visto < i {
			close(parar)
			wg.Wait()
			t.Fatalf("o modo voltou para %q depois de trocado para modo-%d", lido, i)
		}
	}
	close(parar)
	wg.Wait()
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

// O comando do agente vem da configuração do provider, que a pessoa edita e
// que pode chegar importada. Uma quebra de linha ali forjaria linha de log e
// atrapalharia quem lê a mensagem de erro no leitor de telas.
func TestDescricaoDoAgenteNaoForjaLinhaDeLog(t *testing.T) {
	cfg := Config{
		Command: "cursor-agent\n2026-01-01 ERROR falso",
		Args:    []string{"--acp", "--flag\rinjetada"},
	}
	got := describeAgent(cfg)
	if strings.ContainsAny(got, "\n\r") {
		t.Errorf("a descrição carrega quebra de linha: %q", got)
	}
	if got != "cursor-agent 2026-01-01 ERROR falso --acp --flag injetada" {
		t.Errorf("descrição inesperada: %q", got)
	}
	// Caminho comum segue legível: no Windows a barra invertida do caminho não
	// pode virar ruído no meio do erro.
	simples := Config{Command: `C:\Program Files\cursor\cursor-agent.exe`, Args: []string{"--acp"}}
	if got := describeAgent(simples); got != `C:\Program Files\cursor\cursor-agent.exe --acp` {
		t.Errorf("o comando comum foi desfigurado: %q", got)
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

func TestModeloLegadoNaoDuplicaOFormatoEstavel(t *testing.T) {
	estado := &legacyModelState{
		CurrentModelID:  "auto",
		AvailableModels: []legacyModel{{ModelID: "auto", Name: "Auto"}},
	}

	semModelo := withModelOption([]ConfigOption{{ID: "mode", Category: "mode"}}, estado)
	if len(semModelo) != 2 || semModelo[1].Category != CategoryModel {
		t.Fatalf("modelo legado deveria ter sido acrescentado: %+v", semModelo)
	}
	if semModelo[1].CurrentValue != "auto" {
		t.Errorf("modelo corrente = %q, queria auto", semModelo[1].CurrentValue)
	}

	comModelo := withModelOption([]ConfigOption{{ID: "model", Category: CategoryModel}}, estado)
	if len(comModelo) != 1 {
		t.Fatalf("modelo não deveria ser duplicado: %+v", comModelo)
	}

	if got := withModelOption(nil, nil); got != nil {
		t.Errorf("sem modelos, nada a acrescentar: %+v", got)
	}

	// Lista só de linhas sem identificador não vira seletor: ele diria que a
	// escolha existe e não deixaria escolher nada.
	vazio := &legacyModelState{AvailableModels: []legacyModel{{Name: "sem id"}}}
	if got := withModelOption(nil, vazio); got != nil {
		t.Errorf("modelo sem identificador não deveria virar opção: %+v", got)
	}
}

// O agente que anuncia modelo ou modo pelo formato de antes só o faz na
// abertura da sessão. O conjunto que ele manda depois fala do que ele guarda em
// configOptions, e sem preservar o que a sessão já sabia o seletor sumiria da
// tela no meio da conversa.
func TestOQueVeioPeloFormatoAnteriorSobreviveAoConjuntoSeguinte(t *testing.T) {
	conhecido := []ConfigOption{
		{ID: "model", Category: CategoryModel, CurrentValue: "auto"},
		{ID: "mode", Category: modeCategory, CurrentValue: "agent"},
	}

	soOutraCoisa := withKnownLegacy([]ConfigOption{{ID: "verbosidade", Category: "outra"}}, conhecido)
	if len(soOutraCoisa) != 3 {
		t.Fatalf("modelo e modo deveriam ter sobrevivido: %+v", soOutraCoisa)
	}

	// O que o conjunto novo traz é o que vale: preservar aqui devolveria o
	// valor antigo por cima de uma troca que acabou de acontecer.
	trocouModelo := withKnownLegacy([]ConfigOption{{ID: "model", Category: CategoryModel, CurrentValue: "claude"}}, conhecido)
	if got := findOption(trocouModelo, CategoryModel); got == nil || got.CurrentValue != "claude" {
		t.Fatalf("o modelo novo deveria ter prevalecido: %+v", trocouModelo)
	}

	// Conjunto vazio é o agente que não mandou nada, e não o agente que
	// esvaziou as opções: quem chama trata isso guardando o que já tinha.
	if got := withKnownLegacy(nil, conhecido); got != nil {
		t.Errorf("sem conjunto novo, nada a preservar: %+v", got)
	}
}
