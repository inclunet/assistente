package acp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"assistente/internal/logging"

	sdk "github.com/coder/acp-go-sdk"
)

// cancelGrace é quanto esperamos o agente confirmar o fim de um turno
// cancelado. Estourar esse prazo não é detalhe: significa que um agente de
// código continua solto, possivelmente editando arquivos, e quem chamou
// precisa saber disso (AEP-0084 D10).
const cancelGrace = 30 * time.Second

// closeTimeout limita o session/close. Encerrar a conversa não pode segurar a
// saída do app por causa de um agente que não responde.
const closeTimeout = 5 * time.Second

type session struct {
	id  string
	cwd string
	cn  *conn

	// turnSlot tem capacidade 1 e serializa os turnos da sessão. O slot só
	// volta quando a chamada ao agente termina de verdade — não quando quem
	// pediu desiste de esperar —, porque dois session/prompt no mesmo
	// sessionId se atropelariam do lado do agente (AEP-0084 D10).
	turnSlot chan struct{}

	// grace é o prazo de confirmação do cancelamento; campo, e não constante,
	// para que o teste possa encurtá-lo sem esperar meio minuto.
	grace time.Duration
	// closeWait é quanto o encerramento espera pela despedida do agente. Campo
	// pelo mesmo motivo de grace.
	closeWait time.Duration

	// sinkMu protege a entrega, e não só a leitura do sink. Segurar a trava
	// durante a chamada é o que faz o fim do turno esperar a entrega em
	// andamento terminar: sem isso, uma atualização já lida escaparia para um
	// sink que quem chamou considera fechado. Quem desliga é sempre o Prompt,
	// e nunca o Close, para que um sink possa encerrar a própria conversa sem
	// esperar por si mesmo. É trava própria porque o sink é código de fora, que
	// pode consultar a sessão enquanto renderiza.
	sinkMu sync.RWMutex
	sink   UpdateSink

	mu      sync.Mutex
	options []ConfigOption
	closed  bool

	// unconfirmedSig fecha quando o turno em andamento foi cancelado e o agente
	// não confirmou no prazo. O slot continua ocupado nesse caso — dois
	// session/prompt no mesmo sessionId se atropelariam —, mas quem espera é
	// acordado com o motivo em vez de ficar no escuro. É um canal, e não uma
	// marca, justamente por causa de quem já está na fila: uma marca só seria
	// vista por quem chegasse depois.
	unconfirmedSig chan struct{}

	// closedSig fecha quando a sessão é encerrada, e é o que tira da fila quem
	// espera a vez. Sem ele, um turno enfileirado com contexto sem prazo
	// esperaria para sempre por uma conversa que já não existe.
	closedSig chan struct{}

	// turnSeq numera os turnos. O prazo pode estourar no mesmo instante em que o
	// turno enfim responde, e sem o número a marca de um turno morto derrubaria
	// o turno seguinte, que está saudável.
	turnSeq uint64

	// cancelSig fecha quando o turno em curso é cancelado, e é renovado a cada
	// turno novo. Quem manda session/cancel precisa responder "cancelado" aos
	// pedidos de permissão ainda pendentes (exigência do ACP), e é por aqui que
	// o transporte fica sabendo.
	cancelSig chan struct{}
}

func (s *session) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func newSession(id, cwd string, cn *conn, options []ConfigOption) *session {
	s := &session{
		id:             id,
		cwd:            cwd,
		cn:             cn,
		turnSlot:       make(chan struct{}, 1),
		grace:          cancelGrace,
		closeWait:      closeTimeout,
		options:        copyOptions(options),
		cancelSig:      make(chan struct{}),
		unconfirmedSig: make(chan struct{}),
		closedSig:      make(chan struct{}),
	}
	s.turnSlot <- struct{}{}
	return s
}

func (s *session) ID() string { return s.id }

// ConfigOptions devolve uma cópia funda. A rasa deixaria quem chamou mexendo no
// slice de valores de dentro da sessão — estado compartilhado sem trava, que é
// corrida garantida quando o agente troca de modelo sozinho no meio do turno.
func (s *session) ConfigOptions() []ConfigOption {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyOptions(s.options)
}

// setConfigOptions guarda uma cópia funda pelo mesmo motivo que ConfigOptions
// devolve uma: o slice que chega aqui costuma ser o mesmo que segue no Update
// para quem escuta a sessão. Guardar a referência deixaria o estado da conversa
// a um passo de qualquer código que retenha esse Update — e o caminho inverso
// também, já que a troca de modo mexe nas opções no lugar.
func (s *session) setConfigOptions(options []ConfigOption) {
	if len(options) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.options = copyOptions(options)
}

// mergeConfigOptions funde o conjunto que o agente mandou com o que a sessão já
// conhecia, guarda o resultado e devolve a cópia dele — tudo sob a mesma trava.
// Ler, fundir e gravar em passos separados abriria janela entre dois caminhos
// que escrevem aqui ao mesmo tempo: a troca de modelo que a pessoa pediu e o
// anúncio que o agente manda por conta própria, previsto no meio do turno. O
// bool diz se sobrou algo aproveitável para guardar.
func (s *session) mergeConfigOptions(fresh []ConfigOption) ([]ConfigOption, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	merged := withKnownMode(fresh, s.options)
	if len(merged) == 0 {
		return copyOptions(s.options), false
	}
	s.options = copyOptions(merged)
	return copyOptions(s.options), true
}

func copyOptions(options []ConfigOption) []ConfigOption {
	out := make([]ConfigOption, len(options))
	for i, option := range options {
		option.Values = append([]ConfigValue(nil), option.Values...)
		out[i] = option
	}
	return out
}

// setCurrentMode acompanha a troca de modo que o agente anuncia pelo formato
// legado. Sem isso o seletor continuaria mostrando o modo anterior, e a pessoa
// acharia que está em "agente" quando o agente já passou para "plano".
func (s *session) setCurrentMode(mode string) {
	if mode == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.options {
		if s.options[i].Category == modeCategory {
			s.options[i].CurrentValue = mode
			return
		}
	}
}

// setCurrentValue anota o valor de uma opção pelo identificador dela e devolve
// o estado resultante junto com a notícia de ter achado a opção — na mesma
// trava, para que quem chamou receba o que acabou de escrever. Não inventa a
// opção que não existe: sem nome nem lista de valores, o que apareceria no
// seletor é um controle mudo, e as opções que este pacote ainda não modela são
// justamente as que ninguém sabe desenhar.
func (s *session) setCurrentValue(id, value string) ([]ConfigOption, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.options {
		if s.options[i].ID == id {
			s.options[i].CurrentValue = value
			return copyOptions(s.options), true
		}
	}
	return copyOptions(s.options), false
}

func (s *session) setSink(sink UpdateSink) {
	s.sinkMu.Lock()
	defer s.sinkMu.Unlock()
	s.sink = sink
}

// deliver entrega uma atualização ao turno corrente. A entrega vale enquanto o
// turno não retornou — o que inclui o rastro que o agente ainda emite depois de
// um cancelamento, porque é ali que ele conta o que chegou a fazer no disco.
// Fora de turno o evento é descartado: não há a quem entregar.
//
// O estado da sessão é exceção e se atualiza de qualquer forma. O agente troca
// de modelo por conta própria, inclusive entre turnos, e esquecer isso deixaria
// a pessoa achando que fala com outro modelo.
func (s *session) deliver(update Update) {
	switch update.Kind {
	case UpdateConfigOptions:
		// Quem escuta recebe o mesmo conjunto que passamos a guardar, e não o
		// que veio do agente. O evento se anuncia como o conjunto completo, e
		// um agente que manda só os modelos faria a UI esconder o seletor de
		// modo no meio da conversa — enquanto o estado da sessão ainda diria
		// que o modo existe.
		update.ConfigOptions, _ = s.mergeConfigOptions(update.ConfigOptions)
	case UpdateMode:
		s.setCurrentMode(update.Mode)
	}

	s.sinkMu.RLock()
	defer s.sinkMu.RUnlock()
	if s.sink == nil {
		return
	}
	s.emit(update)
}

// emit isola o consumidor do protocolo. Quem quebra ao renderizar um pedaço da
// resposta perde aquele pedaço, e só: sem isso o pânico subiria como falha ao
// tratar a notificação do agente, misturando defeito de quem exibe com defeito
// de quem conversa — e é o segundo que o diagnóstico precisa achar.
func (s *session) emit(update Update) {
	defer func() {
		if r := recover(); r != nil {
			logging.Errorf(context.Background(), logComponent,
				"[ACP] quem escuta a sessão %q quebrou ao receber %q: %v", s.id, update.Kind, r)
		}
	}()
	s.sink(update)
}

func (s *session) acquireTurn(ctx context.Context) error {
	// Fila livre não dispensa a conferência: quem chega com o contexto já morto
	// não pode pôr um agente de código para trabalhar e só então descobrir que
	// desistiu.
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-s.turnSlot:
		return s.takeTurn(ctx)
	default:
	}
	// O canal é lido antes da espera: quem já está na fila quando o prazo de
	// cancelamento estoura precisa ser acordado, não descobrir só na próxima vez
	// que tentar.
	return s.waitForTurn(ctx, s.unconfirmedCancel(), s.closedSignal())
}

// takeTurn confirma a vez recém-pegada, e é por onde passam todos os caminhos
// que pegam uma. Quem desistiu no meio do caminho devolve a vez em vez de
// começar o turno: a desistência pode ficar pronta no mesmo instante em que a
// vez, e aí o select escolheria ao acaso — o acaso aqui põe um agente de código
// para editar arquivo depois que a pessoa mandou parar. Devolver deixa a fila
// andar para quem ainda quer.
func (s *session) takeTurn(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		s.releaseTurn()
		return err
	}
	return nil
}

// waitForTurn é a espera na fila.
func (s *session) waitForTurn(ctx context.Context, unconfirmed, closed <-chan struct{}) error {
	select {
	case <-s.turnSlot:
		return s.takeTurn(ctx)
	case <-unconfirmed:
		// A vez pode ter ficado livre no mesmo instante, e aí ela vale mais: o
		// turno velho terminou de verdade, não há mais o que confirmar.
		select {
		case <-s.turnSlot:
			return s.takeTurn(ctx)
		default:
			return ErrCancelNotConfirmed
		}
	case <-closed:
		return ErrSessionClosed
	case <-s.cn.dead:
		return ErrSessionLost
	case <-s.cn.rpc.Done():
		return ErrSessionLost
	case <-ctx.Done():
		return ctx.Err()
	}
}

// cancelSignal devolve o canal que fecha quando o turno corrente é cancelado.
func (s *session) cancelSignal() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancelSig
}

// startTurn prepara o turno recém-admitido: sinal de cancelamento limpo, para
// que o cancelamento do turno anterior não derrube um pedido de permissão deste,
// e um número próprio, que é como a sessão distingue a marca de um turno velho
// da do turno em andamento.
func (s *session) startTurn() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.cancelSig:
		s.cancelSig = make(chan struct{})
	default:
	}
	select {
	case <-s.unconfirmedSig:
		s.unconfirmedSig = make(chan struct{})
	default:
	}
	s.turnSeq++
	return s.turnSeq
}

func (s *session) signalCancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.cancelSig:
	default:
		close(s.cancelSig)
	}
}

// turnInFlight diz se o slot está tomado. É uma leitura sem trava, e de
// propósito: no pior caso manda um session/cancel a mais para uma sessão ociosa,
// que o agente ignora — o contrário, deixar de cancelar, é que custa caro.
func (s *session) turnInFlight() bool {
	return len(s.turnSlot) == 0
}

func (s *session) closedSignal() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closedSig
}

func (s *session) unconfirmedCancel() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.unconfirmedSig
}

// markCancelUnconfirmed só vale para o turno em andamento: o prazo pode estourar
// no instante em que o turno enfim responde, e aí a marca chegaria atrasada para
// derrubar o turno seguinte, que está saudável.
func (s *session) markCancelUnconfirmed(seq uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.turnSeq != seq {
		return
	}
	select {
	case <-s.unconfirmedSig:
	default:
		close(s.unconfirmedSig)
	}
}

func (s *session) releaseTurn() {
	select {
	case s.turnSlot <- struct{}{}:
	default:
	}
}

type promptOutcome struct {
	stop StopReason
	err  error
}

func (s *session) Prompt(ctx context.Context, content []Content, sink UpdateSink) (StopReason, error) {
	blocks, err := promptBlocks(content)
	if err != nil {
		return "", &PromptError{Err: err}
	}
	if s.isClosed() {
		return "", &PromptError{Err: ErrSessionClosed}
	}
	if s.cn.isDead() {
		return "", &PromptError{Err: ErrSessionLost}
	}
	if err := s.acquireTurn(ctx); err != nil {
		return "", &PromptError{Err: err}
	}
	// A sessão pode ter sido encerrada enquanto este turno esperava a vez na
	// fila; mandá-lo agora seria falar com uma conversa que já não existe.
	if s.isClosed() {
		s.releaseTurn()
		return "", &PromptError{Err: ErrSessionClosed}
	}
	seq := s.startTurn()

	// O que estiver pendente com quem decide caduca no instante do "pare", e
	// não quando esta função acordar para tratar o cancelamento: nessa brecha,
	// uma permissão respondida — por uma política de aprovação automática, por
	// exemplo — ainda chegaria ao agente como autorização. Autorizar depois do
	// "pare" é o que o D9 não admite.
	stopWatching := context.AfterFunc(ctx, s.signalCancel)
	defer stopWatching()

	s.setSink(sink)
	defer s.setSink(nil)

	done := make(chan promptOutcome, 1)
	go func() {
		defer s.releaseTurn()
		// Última conferência antes de falar com o agente. Entre pegar a vez e
		// esta goroutine ser escalonada pode passar tempo suficiente para a
		// conversa ser excluída, e um agente de código não deve começar a
		// trabalhar numa conversa que já não existe. Se o encerramento cair
		// exatamente entre esta linha e a escrita, o session/cancel do Close
		// chega antes do prompt e não cancela nada — janela que este transporte
		// não consegue fechar sozinho, porque o SDK não separa o envio da espera
		// pela resposta.
		if s.isClosed() {
			done <- promptOutcome{err: ErrSessionClosed}
			return
		}
		// A chamada ignora o cancelamento do ctx de quem pediu (mantendo os
		// valores de correlação): desistir da espera não é a mesma coisa que
		// encerrar o turno. Quem encerra é session/cancel, e a resposta do
		// agente ainda precisa ser aguardada.
		resp, err := sdk.SendRequest[sdk.PromptResponse](s.cn.rpc, context.WithoutCancel(ctx),
			sdk.AgentMethodSessionPrompt, sdk.PromptRequest{
				SessionId: sdk.SessionId(s.id),
				Prompt:    blocks,
			})
		done <- promptOutcome{stop: StopReason(resp.StopReason), err: err}
	}()

	select {
	case out := <-done:
		return s.finishTurn(out)
	case <-s.closedSig:
		return s.closedOutcome(done)
	case <-ctx.Done():
		// A entrega continua ligada durante o prazo de graça, de propósito. O
		// que o agente emite enquanto se recolhe é justamente o desfecho do que
		// ele já tinha começado — a ferramenta que terminou de gravar o arquivo,
		// o comando que ainda rodou. Calar isso aqui deixaria a lista de
		// ferramentas parada em "em andamento" e esconderia da pessoa uma
		// escrita em disco que aconteceu de verdade.
		//
		// O envio do cancelamento não pode segurar o prazo: escrever para o
		// agente é I/O que pode travar, e travaria quem chamou justamente na
		// hora em que ele pediu para parar. Contra um agente vivo que parou de
		// ler a entrada, essa goroutine fica parada até o cano quebrar — o que
		// acontece quando o processo morre, no Close do cliente ou por conta
		// dele mesmo. Prazo não resolveria: o SDK confere o contexto antes de
		// escrever e depois entra num Write que não olha mais nada.
		go func() {
			if err := s.Cancel(context.Background()); err != nil {
				logging.Warnf(context.Background(), logComponent,
					"[ACP] falha ao cancelar turno da sessão %q: %v", s.id, err)
			}
		}()
		timer := time.NewTimer(s.grace)
		defer timer.Stop()
		return s.awaitCancelled(seq, done, timer.C)
	}
}

// closedOutcome resolve um turno cuja conversa foi excluída. Se a goroutine já
// deixou o desfecho pronto, é ele que vale: só ela sabe dizer se o pedido
// chegou a sair para o agente, e é isso que decide se uma retentativa
// automática é segura. Sem essa conferência, o mesmo turno seria dado como
// aceito ou não conforme quem ganhasse a corrida.
//
// Não esperamos pelo desfecho: o Close já mandou o agente parar e já desligou a
// entrega, então segurar quem chamou não mostraria nada a ninguém e o prenderia
// num agente que talvez nunca responda. Quem cuida do agente solto é o Close.
func (s *session) closedOutcome(done <-chan promptOutcome) (StopReason, error) {
	if out, ok := tryReceive(done); ok {
		return s.finishTurn(out)
	}
	return StopCancelled, &PromptError{Accepted: true, Err: ErrSessionClosed}
}

// awaitCancelled espera o desfecho de um turno que foi mandado parar. A
// resposta do agente tem preferência sobre o prazo e sobre o encerramento da
// conversa: com dois casos prontos o select escolhe ao acaso, e aqui o acaso
// diria que o agente não confirmou o cancelamento quando ele acabou de
// confirmar — deixando a sessão marcada e recusando o próximo turno por um
// motivo que não existe.
func (s *session) awaitCancelled(seq uint64, done <-chan promptOutcome, expired <-chan time.Time) (StopReason, error) {
	select {
	case out := <-done:
		return s.finishTurn(out)
	case <-s.closedSig:
		return s.closedOutcome(done)
	case <-expired:
		if out, ok := tryReceive(done); ok {
			return s.finishTurn(out)
		}
		// O turno saiu e não voltou: pode haver um agente de código ainda
		// mexendo no disco, e quem chamou precisa saber que é esse o estado.
		// A sessão fica marcada para que o próximo turno seja recusado com
		// esse mesmo motivo, em vez de esperar calado na fila.
		s.markCancelUnconfirmed(seq)
		return StopCancelled, &PromptError{Accepted: true, Err: ErrCancelNotConfirmed}
	}
}

func (s *session) finishTurn(out promptOutcome) (StopReason, error) {
	if out.err == nil {
		return out.stop, nil
	}
	// O turno nem chegou a sair: a conversa foi encerrada antes do envio.
	if errors.Is(out.err, ErrSessionClosed) {
		return "", &PromptError{Err: ErrSessionClosed}
	}
	if s.cn.isDead() {
		return "", &PromptError{Accepted: turnAccepted(out.err), Err: ErrSessionLost}
	}
	return "", &PromptError{
		Accepted: turnAccepted(out.err),
		Err:      wrapCallError(fmt.Sprintf("turno na sessão %q", s.id), out.err),
	}
}

func (s *session) Cancel(ctx context.Context) error {
	// O sinal vem antes do envio: um pedido de permissão pendente precisa ser
	// respondido como cancelado, e a pessoa não pode ficar com um diálogo na
	// tela decidindo sobre um turno que ela mesma acabou de abortar.
	s.signalCancel()

	if s.cn.isDead() {
		return ErrSessionLost
	}
	err := s.cn.rpc.SendNotification(ctx, sdk.AgentMethodSessionCancel, sdk.CancelNotification{
		SessionId: sdk.SessionId(s.id),
	})
	if err != nil {
		return fmt.Errorf("cancelar turno da sessão %q: %w", s.id, err)
	}
	return nil
}

// Close solta a sessão. O registro local some sempre — sem isso, uma conversa
// excluída continuaria recebendo atualizações e ocupando memória —, e o
// session/close só vai ao agente quando ele anuncia suportar o método.
//
// O registro local sai antes de qualquer conversa com o agente, então o retorno
// não muda o que já é fato: a sessão morreu para o app. O erro só conta se o
// agente confirmou a despedida — e a espera por ela é limitada, porque agente
// travado não pode segurar a saída do app.
//
// Um turno em andamento é cancelado antes: excluir a conversa não pode deixar
// um agente de código solto no disco, nem um pedido de permissão pendente sem
// resposta — o agente esperaria para sempre e a pessoa ficaria decidindo sobre
// uma conversa que já não existe (AEP-0084 D9).
func (s *session) Close(ctx context.Context) error {
	s.mu.Lock()
	already := s.closed
	s.closed = true
	if !already && s.closedSig != nil {
		close(s.closedSig)
	}
	s.mu.Unlock()
	if already {
		return nil
	}

	// Não desligamos a entrega aqui de propósito. Quem desliga é o Prompt, que
	// este encerramento acaba de acordar, e ele espera a entrega em andamento
	// terminar antes de retornar. Fazer isso aqui também travaria um sink que
	// encerra a própria conversa ao receber um evento: ele ficaria esperando a
	// entrega que ele mesmo é.
	s.cn.removeSession(s.id)
	if s.cn.isDead() {
		return nil
	}

	// A despedida sai da frente de quem fechou. Falar com o agente é escrita no
	// stdin dele, e essa escrita não olha contexto nenhum: o SDK confere o
	// cancelamento antes de escrever e depois entra num Write que só volta
	// quando o cano aceita os bytes. Um agente vivo que parou de ler penduraria
	// o encerramento do app aqui para sempre.
	done := make(chan error, 1)
	go func() { done <- s.farewell(context.WithoutCancel(ctx)) }()

	// A espera é maior que o prazo do próprio pedido, e é isso que separa os
	// dois desfechos: um agente que só demora a responder volta por aqui com o
	// erro do prazo interno.
	timer := time.NewTimer(2 * s.closeWait)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		// Nem o prazo interno voltou: o que travou não foi a resposta, foi a
		// escrita para o agente. Um processo que parou de aceitar entrada não
		// serve mais a ninguém — as outras conversas dele só descobririam isso
		// pendurando cada chamada até o contexto de quem chamou morrer, e a
		// goroutine da despedida ficaria presa no Write. Derrubar devolve o app
		// ao caminho de recuperação: a próxima chamada sobe um agente novo.
		s.cn.shutdown()
		return fmt.Errorf("encerrar a sessão %q: o agente parou de aceitar pedidos e foi derrubado", s.id)
	}
}

// farewell avisa o agente que a conversa acabou: para o turno em andamento e,
// se o agente souber do método, encerra a sessão do lado dele.
func (s *session) farewell(ctx context.Context) error {
	if s.turnInFlight() {
		if err := s.Cancel(ctx); err != nil {
			logging.Warnf(ctx, logComponent,
				"[ACP] falha ao cancelar o turno da sessão %q ao encerrá-la: %v", s.id, err)
		}
	}
	if !s.cn.caps.CloseSession {
		return nil
	}
	cctx, cancel := context.WithTimeout(ctx, s.closeWait)
	defer cancel()
	_, err := sdk.SendRequest[sdk.CloseSessionResponse](s.cn.rpc, cctx, sdk.AgentMethodSessionClose,
		sdk.CloseSessionRequest{SessionId: sdk.SessionId(s.id)})
	if err != nil {
		return wrapCallError(fmt.Sprintf("encerrar a sessão %q", s.id), err)
	}
	return nil
}

func (s *session) SetConfigOption(ctx context.Context, id, value string) ([]ConfigOption, error) {
	// Conversa encerrada não troca de modelo: quem ainda segura a sessão não
	// deve conseguir falar com o agente sobre ela.
	if s.isClosed() {
		return nil, ErrSessionClosed
	}
	if s.cn.isDead() {
		return nil, ErrSessionLost
	}
	resp, err := sdk.SendRequest[sdk.SetSessionConfigOptionResponse](s.cn.rpc, ctx,
		sdk.AgentMethodSessionSetConfigOption, sdk.SetSessionConfigOptionRequest{
			ValueId: &sdk.SetSessionConfigOptionValueId{
				SessionId: sdk.SessionId(s.id),
				ConfigId:  sdk.SessionConfigId(id),
				Value:     sdk.SessionConfigValueId(value),
			},
		})
	if err != nil {
		return nil, wrapCallError(fmt.Sprintf("trocar a opção %q da sessão", id), err)
	}

	// Mesmo cuidado do deliver: o agente no formato legado responde só com o
	// que ele conhece como configOptions, e o modo — que ele anuncia por outro
	// campo — sumiria do seletor só porque a pessoa trocou de modelo. O que
	// volta é sempre cópia: entregar o slice guardado deixaria quem chamou
	// mexendo no estado da sessão por fora.
	if novas, ok := s.mergeConfigOptions(configOptionsFrom(resp.ConfigOptions)); ok {
		return novas, nil
	}

	// A resposta do agente não trouxe nada aproveitável — só opções de tipos
	// que ainda não modelamos, por exemplo —, então o valor pedido é guardado
	// à mão: sem isso a tela anunciaria o modelo antigo para uma troca que
	// aconteceu de verdade.
	novas, achou := s.setCurrentValue(id, value)
	if !achou {
		// Nem isso deu, porque a opção trocada também é uma que não
		// acompanhamos. Fica registrado: é o que explica uma tela que não
		// mudou depois de uma troca bem-sucedida.
		logging.Debugf(ctx, logComponent,
			"[ACP] o agente confirmou a troca de %q na sessão %q, opção que não acompanhamos", id, s.id)
	}
	return novas, nil
}

// PromptError diz, além do que falhou, se o agente chegou a aceitar o turno.
// Essa é a informação que impede a auto-recuperação de repetir em silêncio um
// pedido que já virou arquivo editado e comando executado na máquina
// (AEP-0084 D4).
type PromptError struct {
	// Accepted verdadeiro significa que o pedido saiu para o agente sem falhar.
	// Repetir o turno automaticamente é inseguro.
	Accepted bool
	Err      error
}

func (e *PromptError) Error() string {
	if e == nil || e.Err == nil {
		return "falha no turno ACP"
	}
	return e.Err.Error()
}

func (e *PromptError) Unwrap() error { return e.Err }

// turnAccepted classifica a falha de forma deliberadamente conservadora: só é
// "não aceito" o que o agente recusou sem começar, respondendo o próprio
// JSON-RPC com erro. No escuro, assume-se aceito, porque parar e devolver o
// controle para a pessoa é mais barato do que repetir uma edição em arquivo.
func turnAccepted(err error) bool {
	if err == nil {
		return true
	}
	var reqErr *sdk.RequestError
	if errors.As(err, &reqErr) {
		switch reqErr.Code {
		case -32700, -32600, -32601, -32602, authRequiredCode:
			return false
		}
	}
	return true
}
