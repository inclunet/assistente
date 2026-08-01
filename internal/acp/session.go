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

	mu      sync.Mutex
	sink    UpdateSink
	options []ConfigOption
	closed  bool

	// turnSeq numera os turnos e awaitingCancelSeq guarda aquele cujo
	// cancelamento o agente não confirmou. O slot continua ocupado nesse caso —
	// dois session/prompt no mesmo sessionId se atropelariam —, mas quem chegar
	// depois merece ouvir o motivo em vez de esperar no escuro.
	//
	// São dois números, e não uma marca simples, porque o prazo pode estourar no
	// mesmo instante em que o turno enfim responde: aí a marca é cravada depois
	// de o turno ter acabado, e sem o número ela recusaria o turno seguinte, que
	// está saudável.
	turnSeq           uint64
	awaitingCancelSeq uint64

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
		id:        id,
		cwd:       cwd,
		cn:        cn,
		turnSlot:  make(chan struct{}, 1),
		grace:     cancelGrace,
		options:   options,
		cancelSig: make(chan struct{}),
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
	options := make([]ConfigOption, len(s.options))
	for i, option := range s.options {
		option.Values = append([]ConfigValue(nil), option.Values...)
		options[i] = option
	}
	return options
}

func (s *session) setConfigOptions(options []ConfigOption) {
	if len(options) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.options = options
}

func (s *session) setSink(sink UpdateSink) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sink = sink
}

// deliver entrega uma atualização ao turno corrente. Fora de turno (ou depois
// que quem pediu desistiu) o evento não é entregue: renderizar texto num turno
// que a pessoa considera fechado seria pior do que perdê-lo.
//
// O estado da sessão é exceção e se atualiza de qualquer forma. O agente troca
// de modelo por conta própria, inclusive entre turnos, e esquecer isso deixaria
// a pessoa achando que fala com outro modelo.
func (s *session) deliver(update Update) {
	if update.Kind == UpdateConfigOptions {
		s.setConfigOptions(update.ConfigOptions)
	}

	s.mu.Lock()
	sink := s.sink
	s.mu.Unlock()
	if sink == nil {
		return
	}
	sink(update)
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
		return nil
	default:
	}
	if s.awaitingCancelConfirmation() {
		return ErrCancelNotConfirmed
	}
	select {
	case <-s.turnSlot:
		return nil
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

// awaitingCancelConfirmation só vale para o turno em andamento: a marca de um
// turno que já terminou não recusa quem vem depois.
func (s *session) awaitingCancelConfirmation() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.awaitingCancelSeq != 0 && s.awaitingCancelSeq == s.turnSeq
}

func (s *session) markCancelUnconfirmed(seq uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.awaitingCancelSeq = seq
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

	s.setSink(sink)
	defer s.setSink(nil)

	done := make(chan promptOutcome, 1)
	go func() {
		defer s.releaseTurn()
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
	case <-ctx.Done():
		// O envio do cancelamento não pode segurar o prazo: escrever para o
		// agente é I/O que pode travar, e travaria quem chamou justamente na
		// hora em que ele pediu para parar. A goroutine não fica presa para
		// sempre: um agente que não lê mais a entrada acaba morto pelo
		// encerramento da conexão, e a escrita falha quando o cano fecha.
		go func() {
			if err := s.Cancel(context.Background()); err != nil {
				logging.Warnf(context.Background(), logComponent,
					"[ACP] falha ao cancelar turno da sessão %s: %v", s.id, err)
			}
		}()
		select {
		case out := <-done:
			return s.finishTurn(out)
		case <-time.After(s.grace):
			// O turno saiu e não voltou: pode haver um agente de código ainda
			// mexendo no disco, e quem chamou precisa saber que é esse o estado.
			// A sessão fica marcada para que o próximo turno seja recusado com
			// esse mesmo motivo, em vez de esperar calado na fila.
			s.markCancelUnconfirmed(seq)
			return StopCancelled, &PromptError{Accepted: true, Err: ErrCancelNotConfirmed}
		}
	}
}

func (s *session) finishTurn(out promptOutcome) (StopReason, error) {
	if out.err == nil {
		return out.stop, nil
	}
	if s.cn.isDead() {
		return "", &PromptError{Accepted: turnAccepted(out.err), Err: ErrSessionLost}
	}
	return "", &PromptError{
		Accepted: turnAccepted(out.err),
		Err:      wrapCallError(fmt.Sprintf("turno na sessão %s", s.id), out.err),
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
		return fmt.Errorf("cancelar turno da sessão %s: %w", s.id, err)
	}
	return nil
}

// Close solta a sessão. O registro local some sempre — sem isso, uma conversa
// excluída continuaria recebendo atualizações e ocupando memória —, e o
// session/close só vai ao agente quando ele anuncia suportar o método.
//
// Um turno em andamento é cancelado antes: excluir a conversa não pode deixar
// um agente de código solto no disco, nem um pedido de permissão pendente sem
// resposta — o agente esperaria para sempre e a pessoa ficaria decidindo sobre
// uma conversa que já não existe (AEP-0084 D9).
func (s *session) Close(ctx context.Context) error {
	s.mu.Lock()
	already := s.closed
	s.closed = true
	s.sink = nil
	s.mu.Unlock()
	if already {
		return nil
	}

	s.cn.removeSession(s.id)
	if s.cn.isDead() {
		return nil
	}
	if s.turnInFlight() {
		// Mandar o agente parar não pode depender do contexto de quem fechou:
		// no encerramento do app ele costuma já estar morto, e a notificação
		// nem sairia — deixando um agente de código editando arquivos de uma
		// conversa que já não existe.
		if err := s.Cancel(context.WithoutCancel(ctx)); err != nil {
			logging.Warnf(ctx, logComponent,
				"[ACP] falha ao cancelar o turno da sessão %s ao encerrá-la: %v", s.id, err)
		}
	}
	if !s.cn.caps.CloseSession {
		return nil
	}
	_, err := sdk.SendRequest[sdk.CloseSessionResponse](s.cn.rpc, ctx, sdk.AgentMethodSessionClose,
		sdk.CloseSessionRequest{SessionId: sdk.SessionId(s.id)})
	if err != nil {
		return wrapCallError(fmt.Sprintf("encerrar a sessão %s", s.id), err)
	}
	return nil
}

func (s *session) SetConfigOption(ctx context.Context, id, value string) ([]ConfigOption, error) {
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

	options := configOptionsFrom(resp.ConfigOptions)
	s.setConfigOptions(options)
	return options, nil
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
