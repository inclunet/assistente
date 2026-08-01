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

	mu      sync.Mutex
	sink    UpdateSink
	options []ConfigOption
	closed  bool
}

func (s *session) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func newSession(id, cwd string, cn *conn, options []ConfigOption) *session {
	s := &session{
		id:       id,
		cwd:      cwd,
		cn:       cn,
		turnSlot: make(chan struct{}, 1),
		options:  options,
	}
	s.turnSlot <- struct{}{}
	return s
}

func (s *session) ID() string { return s.id }

func (s *session) ConfigOptions() []ConfigOption {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ConfigOption(nil), s.options...)
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
	select {
	case <-s.turnSlot:
		return nil
	default:
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
		case <-time.After(cancelGrace):
			// O turno saiu e não voltou: pode haver um agente de código ainda
			// mexendo no disco, e quem chamou precisa saber que é esse o estado.
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
	if s.cn.isDead() || !s.cn.caps.CloseSession {
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
