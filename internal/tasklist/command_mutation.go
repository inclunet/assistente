package tasklist

import (
	"context"
	"errors"
	"strings"
	"sync"

	"assistente/internal/database"
)

const (
	CommandMutationCreate = "create"
	CommandMutationUpdate = "update"
	CommandMutationClone  = "clone"
	CommandMutationClear  = "clear"
	CommandMutationDelete = "delete"
)

var (
	ErrCommandMutationStale        = database.ErrTaskListCommandStale
	ErrCommandMutationUnavailable  = errors.New("tasklist command mutation store unavailable")
	ErrCommandMutationConsumed     = errors.New("tasklist command mutation already consumed")
	ErrCommandMutationNotCommitted = errors.New("tasklist command guard did not commit")
	ErrCommandMutationOwner        = errors.New("tasklist command owner changed")
	ErrCommandMutationInvalidTitle = errors.New("tasklist command title is required")
)

const (
	maxCommandMutationTitle       = 4096
	maxCommandMutationDescription = 1 << 20
	maxCommandMutationFingerprint = 256
	maxCommandMutationID          = 256
)

// CommandMutation é um preparo selado de uma única operação de tasklist.
// Seus parâmetros não são reextraídos de UI/store no Commit: o CAS final
// compara o fingerprint capturado e persiste exatamente este payload.
type CommandMutation struct {
	service             *Service
	operation           string
	id                  string
	title               string
	description         string
	expectedFingerprint string
	ownerID             string

	mu       sync.Mutex
	consumed bool
}

// PrepareCommandMutation valida o alvo e sela os parâmetros antes do gate de
// execução. Criação exige fingerprint vazio; as outras operações exigem o
// fingerprint do ReadCommandTarget correspondente.
func (s *Service) PrepareCommandMutation(ctx context.Context, operation, id, title, description, expectedFingerprint string) (*CommandMutation, error) {
	ownerID, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	operation = strings.ToLower(strings.TrimSpace(operation))
	if !validCommandMutationOperation(operation) {
		return nil, errors.New("operação de mutação de tasklist inválida")
	}
	if operation == CommandMutationCreate {
		if id != "" || expectedFingerprint != "" {
			return nil, ErrCommandMutationStale
		}
		if err := validateCommandMutationPayload(operation, id, title, description, expectedFingerprint); err != nil {
			return nil, err
		}
		return &CommandMutation{service: s, operation: operation, title: title, description: description, ownerID: ownerID}, nil
	}
	if id == "" || expectedFingerprint == "" {
		return nil, ErrCommandMutationStale
	}
	if err := validateCommandMutationPayload(operation, id, title, description, expectedFingerprint); err != nil {
		return nil, err
	}
	_, actual, err := s.ReadCommandTarget(ctx, id)
	if err != nil {
		return nil, err
	}
	if actual != expectedFingerprint {
		return nil, ErrCommandMutationStale
	}
	return &CommandMutation{service: s, operation: operation, id: id, title: title, description: description, expectedFingerprint: expectedFingerprint, ownerID: ownerID}, nil
}

// CommitGuarded deixa auth/workspace/versão final envolver somente a
// transação final de CAS e escrita. Essa transação pode reler tasks/notas
// para recalcular o fingerprint; o guard deve chamar commit exatamente uma
// vez e a emissão de eventos acontece depois que ele retorna.
func (m *CommandMutation) CommitGuarded(ctx context.Context, guard func(func() error) error) (*database.TaskList, error) {
	if m == nil || m.service == nil {
		return nil, ErrCommandMutationUnavailable
	}
	if guard == nil {
		return nil, errors.New("guard de mutação ausente")
	}
	if current, ok := database.UserIDFromContext(ctx); ok && current != m.ownerID {
		return nil, ErrCommandMutationOwner
	}
	m.mu.Lock()
	if m.consumed {
		m.mu.Unlock()
		return nil, ErrCommandMutationConsumed
	}
	m.consumed = true
	m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	effectCtx := database.WithUserID(ctx, m.ownerID)
	request := database.TaskListCommandMutationRequest{Operation: m.operation, ID: m.id, Title: m.title, Description: m.description, ExpectedFingerprint: m.expectedFingerprint}
	// O CAS e toda a leitura necessária para revalidar o fingerprint vivem na
	// transação final. Ela usa busy_timeout=0 e não faz retry; o guard, quando
	// fornecido pelo caller, cobre essa transação final sem prometer que a
	// hidratação de tasks/notas seja apenas um COMMIT curto.
	var result *database.TaskList
	called := false
	err := guard(func() error {
		if called {
			return ErrCommandMutationConsumed
		}
		called = true
		var err error
		result, err = m.service.commitCommandMutation(effectCtx, request)
		return err
	})
	if err != nil {
		return nil, err
	}
	if !called {
		return nil, ErrCommandMutationNotCommitted
	}
	m.service.emitCommandMutation(effectCtx, m.operation, m.id, result)
	return result, nil
}

func (s *Service) commitCommandMutation(ctx context.Context, request database.TaskListCommandMutationRequest) (*database.TaskList, error) {
	store, ok := s.store.(commandMutationStore)
	if !ok {
		return nil, ErrCommandMutationUnavailable
	}
	return store.CommitCommandMutation(ctx, request)
}

func (s *Service) emitCommandMutation(ctx context.Context, operation, sourceID string, list *database.TaskList) {
	if s.emitter != nil {
		switch operation {
		case CommandMutationCreate, CommandMutationClone:
			s.emitter.Emit("taskList:created", list)
		case CommandMutationUpdate:
			s.emitter.Emit("taskList:updated", list)
		case CommandMutationDelete:
			if list != nil {
				s.emitter.Emit("taskList:deleted", list.ID)
			} else {
				s.emitter.Emit("taskList:deleted", sourceID)
			}
		case CommandMutationClear:
			s.emitter.Emit("taskList:cleared", sourceID)
		}
	}
	switch operation {
	case CommandMutationCreate:
		if s.wantsDomain("tasklist.list.created") {
			s.publishDomain(ctx, "tasklist.list.created", listPayload(list))
		}
	case CommandMutationClone:
		if s.wantsDomain("tasklist.list.cloned") {
			payload := listPayload(list)
			payload["source_task_list_id"] = sourceID
			s.publishDomain(ctx, "tasklist.list.cloned", payload)
		}
	case CommandMutationUpdate:
		if s.wantsDomain("tasklist.list.updated") {
			s.publishDomain(ctx, "tasklist.list.updated", listPayload(list))
		}
	case CommandMutationDelete:
		if s.wantsDomain("tasklist.list.deleted") {
			payload := listPayload(list)
			if list == nil {
				payload = map[string]any{"task_list_id": sourceID}
			}
			s.publishDomain(ctx, "tasklist.list.deleted", payload)
		}
	case CommandMutationClear:
		if s.wantsDomain("tasklist.list.cleared") {
			s.publishDomain(ctx, "tasklist.list.cleared", map[string]any{
				"task_list_id":   sourceID,
				"task_list_slug": s.taskListSlug(ctx, sourceID),
			})
		}
	}
}

func validCommandMutationOperation(operation string) bool {
	switch operation {
	case CommandMutationCreate, CommandMutationUpdate, CommandMutationClone, CommandMutationClear, CommandMutationDelete:
		return true
	default:
		return false
	}
}

func validateCommandMutationPayload(operation, id, title, description, fingerprint string) error {
	if operation != CommandMutationDelete && operation != CommandMutationClear && strings.TrimSpace(title) == "" {
		return ErrCommandMutationInvalidTitle
	}
	if len(title) > maxCommandMutationTitle || len(description) > maxCommandMutationDescription ||
		len(fingerprint) > maxCommandMutationFingerprint || len(id) > maxCommandMutationID {
		return errors.New("payload de mutação de tasklist excede o limite")
	}
	return nil
}
