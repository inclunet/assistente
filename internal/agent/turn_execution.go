package agent

import (
	"context"
	"errors"
	"sync"

	"assistente/internal/chat"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"gorm.io/gorm"
)

// TurnStore é obrigatório na composição. Implementações de teste substituem
// explicitamente a persistência; produção sempre usa ChatTurnRepository.
type TurnStore interface {
	Begin(context.Context, string, string, string, string) (*database.ChatTurnAttempt, error)
	EnsureResponse(context.Context, string, string, string) (string, error)
	MarkStreaming(context.Context, string, string, string) error
	WritePartial(context.Context, string, string, string, string, bool) error
	FinishResponse(context.Context, string, string, string, string, string, string, *database.MessageOptions) error
}

type turnExecutionKey struct{}
type pendingTurnKey struct{ userID, conversationID, turnID string }

func terminalKey(ctx context.Context, conversationID, turnID string) pendingTurnKey {
	userID, _ := database.UserIDFromContext(ctx)
	return pendingTurnKey{userID, conversationID, turnID}
}

type turnExecution struct {
	mu       sync.Mutex
	id       string
	finished bool
}

func executionFrom(ctx context.Context) *turnExecution {
	execution, _ := ctx.Value(turnExecutionKey{}).(*turnExecution)
	return execution
}

func executionID(ctx context.Context) string {
	if execution := executionFrom(ctx); execution != nil {
		return execution.id
	}
	return ""
}

// BeginTurn precede criação da resposta, prompt e chamada ao provider. Erros
// posteriores à pergunta confirmada podem assim encerrar a tentativa correta.
func (s *Service) BeginTurn(ctx context.Context, conversationID, turnID, mode string) (context.Context, error) {
	key := terminalKey(ctx, conversationID, turnID)
	if pending, ok := s.pendingTerminals.Load(key); ok {
		// Só recupera a execução que este Service já encerrou localmente.
		// CAS impede que a recuperação altere uma tentativa posterior ou viva.
		err := s.turns.FinishResponse(ctx, conversationID, turnID, pending.(string), database.TurnFailed, "persistence_error", "", nil)
		if err != nil && !errors.Is(err, database.ErrChatTurnStale) {
			return ctx, err
		}
		s.pendingTerminals.CompareAndDelete(key, pending)
	}
	attempt, err := s.turns.Begin(ctx, conversationID, turnID, "", mode)
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, turnExecutionKey{}, &turnExecution{id: attempt.ExecutionID}), nil
}

func (s *Service) EnsureTurnResponse(ctx context.Context, conversationID, turnID string) (string, error) {
	id, err := s.turns.EnsureResponse(ctx, conversationID, turnID, executionID(ctx))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", chat.ErrConversationGone
	}
	return id, err
}

func (s *Service) startTurnStreaming(ctx context.Context, conversationID, turnID string, origin *ports.ChatSurfaceOrigin) bool {
	if err := s.turns.MarkStreaming(ctx, conversationID, turnID, executionID(ctx)); err != nil {
		if errors.Is(err, database.ErrChatTurnStale) {
			return false
		}
		if ctx.Err() != nil {
			s.emitFinalizationCancelledDone(ctx, conversationID, turnID, "", origin)
			return false
		}
		s.emitFinalizationErrorDone(ctx, conversationID, turnID, "", origin)
		return false
	}
	return true
}

func (s *Service) finishTurn(ctx context.Context, conversationID, turnID, state, reason, responseID string, response *database.MessageOptions) error {
	execution := executionFrom(ctx)
	if execution != nil {
		execution.mu.Lock()
		defer execution.mu.Unlock()
		if execution.finished {
			return database.ErrChatTurnStale
		}
	}
	err := s.turns.FinishResponse(ctx, conversationID, turnID, executionID(ctx), state, reason, responseID, response)
	if err == nil && execution != nil {
		execution.finished = true
	}
	return err
}

// persistTerminal mantém falha de persistência explícita e nunca publica patch
// antigo como sucesso. Um terminal de execução antiga não é publicado.
func (s *Service) persistTerminal(ctx context.Context, event *ports.DoneEvent, state, reason string) bool {
	execution := executionFrom(ctx)
	if execution != nil {
		execution.mu.Lock()
		defer execution.mu.Unlock()
		if execution.finished {
			return false
		}
	}
	err := s.turns.FinishResponse(context.WithoutCancel(ctx), event.ConversationID, event.TurnID, executionID(ctx), state, reason, "", nil)
	if errors.Is(err, database.ErrChatTurnStale) {
		return false
	}
	if err != nil {
		event.Reason = "error"
		event.ErrorMessage = ports.ChatErrorPersistence
		event.TurnPatch = nil
		s.pendingTerminals.Store(terminalKey(ctx, event.ConversationID, event.TurnID), executionID(ctx))
	}
	// Mesmo se o banco falhar, o terminal volátil já foi entregue. O próximo
	// boot recupera apenas o estado confirmado, sem inventar sucesso.
	if execution != nil {
		execution.finished = true
	}
	return true
}

// EndUnfinishedTurn cobre falhas de preparação e provider que retorna sem
// terminal. Não infere sucesso nem substitui resultado confirmado.
func (s *Service) EndUnfinishedTurn(ctx context.Context, conversationID, turnID string, origin *ports.ChatSurfaceOrigin) {
	execution := executionFrom(ctx)
	if execution == nil {
		return
	}
	execution.mu.Lock()
	finished := execution.finished
	execution.mu.Unlock()
	if finished {
		return
	}
	state, reason := database.TurnFailed, "internal_error"
	event := ports.DoneEvent{ConversationID: conversationID, TurnID: turnID, ItemID: ports.ResponseItemID(turnID), SurfaceOrigin: origin, Reason: "error", ErrorMessage: ports.ChatErrorInternal}
	if ctx.Err() != nil {
		state, reason = database.TurnInterrupted, "cancelled"
		event.Reason, event.ErrorMessage = "cancelled", ""
	}
	if s.persistTerminal(ctx, &event, state, reason) {
		s.emitter.Emit("chat:done", event)
	}
}
