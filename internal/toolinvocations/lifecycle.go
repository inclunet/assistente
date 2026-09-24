package toolinvocations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"assistente/internal/logging"
	"assistente/internal/tools"

	"gorm.io/gorm"
)

var errChatLedgerUnavailable = errors.New("chat ledger unavailable")

// A UI recebe erros seguros e genéricos; o diagnóstico operacional não pode
// desaparecer quando Execute converte o erro em Persisted=false.
func logInvocationPersistenceFailure(ctx context.Context, stage string, origin Origin, id string, err error) {
	logging.Logger(ctx, "toolinvocations.service").Error("tool invocation persistence failed",
		"stage", stage, "origin_type", origin.Type, "origin_id", origin.ID,
		"invocation_id", id, "error", err)
}

// As entradas adaptam seus resultados; somente este núcleo grava o ciclo de vida.
type invocationStart struct {
	call               tools.ToolCall
	persistedArguments *string
	origin             Origin
	parentID           string
	dryRun, external   bool
	iteration          int
	observation        *ExternalObservation
}

type invocationOutcome struct {
	status, message              string
	result                       tools.ToolResult
	errorKind                    tools.ErrorKind
	errorCode                    string
	retryable, retryabilityKnown bool
	durationMs                   int64
}

func (r invocationStart) persistenceCall() tools.ToolCall {
	call := r.call
	if r.persistedArguments != nil {
		call.Function.Arguments = *r.persistedArguments
	}
	return call
}

func (s *Service) resolveInvocationCatalog(ctx context.Context, req invocationStart) (string, error) {
	archivalName := ""
	if req.observation != nil {
		archivalName = strings.TrimSpace(req.observation.CatalogName)
		if archivalName == "" {
			return "", fmt.Errorf("archival catalog name is required")
		}
	} else {
		// A identidade canônica vem do nome no escopo do usuário, nunca do
		// ID sugerido pelo chamador. Não consultar um ID que não será usado.
		opCtx, cancel := s.persistOpCtx(ctx)
		id, err := s.repo.ResolveToolCatalogID(opCtx, req.call.Function.Name)
		cancel()
		if err == nil {
			if strings.TrimSpace(id) == "" {
				return "", fmt.Errorf("tool_catalog_id is required")
			}
			return id, nil
		}
		if !errors.Is(err, ErrToolCatalogNotFound) {
			return "", err
		}
		archivalName = req.call.Function.Name
	}
	repo, ok := s.repo.(interface {
		ResolveOrCreateArchivalToolCatalogID(context.Context, string) (string, error)
	})
	if !ok {
		return "", fmt.Errorf("archival repository required: %w", ErrToolCatalogNotFound)
	}
	opCtx, cancel := s.persistOpCtx(ctx)
	defer cancel()
	id, err := repo.ResolveOrCreateArchivalToolCatalogID(opCtx, archivalName)
	if err == nil && strings.TrimSpace(id) == "" {
		err = fmt.Errorf("tool_catalog_id is required")
	}
	return id, err
}

func (s *Service) beginInvocation(ctx context.Context, req invocationStart) (Invocation, error) {
	if s == nil || s.repo == nil {
		s.recordPersistenceFailure()
		return Invocation{}, fmt.Errorf("tool invocation repository not configured")
	}
	persistCtx := s.persistCtx(ctx)
	var observationMetadata json.RawMessage
	if observation := req.observation; observation != nil {
		// Limitar antes de decodificar e antes de criar qualquer entrada archival.
		if len(observation.DisplayMetadata) > tools.DefaultMaxResultSize {
			s.recordPersistenceFailure()
			return Invocation{}, fmt.Errorf("observation display exceeds size limit")
		}
		var display map[string]json.RawMessage
		if err := json.Unmarshal(observation.DisplayMetadata, &display); err != nil || display == nil {
			s.recordPersistenceFailure()
			return Invocation{}, fmt.Errorf("observation display must be a JSON object")
		}
		observationMetadata, _ = json.Marshal(map[string]any{"external": true, "display": display})
	}
	origin := req.origin
	origin.Type, origin.ID = strings.TrimSpace(origin.Type), strings.TrimSpace(origin.ID)
	if origin.Type == "" {
		origin.Type = OriginChat
	}
	// Execução exige pré-validação fail-closed. Observação já ocorreu e usa a
	// validação transacional de Create, sem uma leitura preliminar redundante.
	if origin.Type == OriginChat && !req.external {
		opCtx, cancel := s.persistOpCtx(persistCtx)
		err := s.repo.ValidateChatOrigin(opCtx, origin.ID)
		cancel()
		if err != nil {
			s.recordPersistenceFailure()
			logInvocationPersistenceFailure(ctx, "validate_origin", origin, "", err)
			return Invocation{}, fmt.Errorf("%w: %w", errChatLedgerUnavailable, err)
		}
	}
	queuedAt := s.now()
	catalogID, err := s.resolveInvocationCatalog(persistCtx, req)
	if err != nil {
		s.recordPersistenceFailure()
		logInvocationPersistenceFailure(ctx, "resolve_catalog", origin, "", err)
		return Invocation{}, err
	}
	parentID := req.parentID
	if parentID == "" {
		parentID = ParentInvocationIDFromContext(ctx)
	}
	call := req.persistenceCall()
	inv := Invocation{
		ToolCatalogID: catalogID, OriginType: origin.Type, OriginID: origin.ID,
		ConversationID: origin.ConversationID, TurnID: origin.TurnID,
		ParentInvocationID: parentID, ToolCallID: req.call.ID, Attempt: 1,
		Status: StatusQueued, DryRun: req.dryRun, ModelIteration: req.iteration,
		External: req.external, DisplayName: req.call.Function.Name,
		ResultAvailability: "pending", QueuedAt: queuedAt,
		Metadata: s.buildInvocationDisplayMetadata(call, req.iteration, 0, req.external),
	}
	if observation := req.observation; observation != nil {
		inv.Metadata = observationMetadata
		inv.ResultAvailability = "unavailable"
		if !observation.StartedAt.IsZero() {
			inv.QueuedAt = observation.StartedAt
		}
	} else {
		inv.Input = s.buildInvocationInput(call)
		populateInputProjection(&inv)
	}
	opCtx, cancel := s.persistOpCtx(persistCtx)
	err = s.repo.Create(opCtx, &inv)
	cancel()
	if err != nil {
		s.recordPersistenceFailure()
		logInvocationPersistenceFailure(ctx, "create", origin, inv.ID, err)
		if origin.Type == OriginChat {
			return inv, fmt.Errorf("%w: %w", errChatLedgerUnavailable, err)
		}
		return inv, err
	}
	if inv.ID == "" {
		s.recordPersistenceFailure()
		return inv, fmt.Errorf("repository returned an empty invocation ID")
	}
	startedAt := s.now()
	if req.observation != nil {
		startedAt = inv.QueuedAt
	}
	opCtx, cancel = s.persistOpCtx(persistCtx)
	err = s.repo.MarkRunning(opCtx, inv.ID, startedAt)
	cancel()
	if err != nil {
		s.recordPersistenceFailure()
		// Uma observação já aconteceu: Complete ainda pode salvar o resultado.
		// Uma execução local ainda não aconteceu: falha fechada, sem executar.
		if !req.external {
			deleteCtx, deleteCancel := s.persistOpCtx(persistCtx)
			deleteErr := s.repo.Delete(deleteCtx, inv.ID)
			deleteCancel()
			if deleteErr != nil {
				s.recordPersistenceFailure()
				logInvocationPersistenceFailure(ctx, "delete_after_start_failure", origin, inv.ID, deleteErr)
			}
			logInvocationPersistenceFailure(ctx, "mark_running", origin, inv.ID, err)
			return inv, errors.Join(err, deleteErr)
		}
		logging.Errorf(ctx, "toolinvocations.service", "failed to mark observed invocation running (id=%s): %v", inv.ID, err)
	} else {
		inv.StartedAt = &startedAt
	}
	return inv, nil
}

func (s *Service) finishInvocation(ctx context.Context, inv *Invocation, req invocationStart, outcome invocationOutcome) error {
	persistCtx := s.persistCtx(ctx)
	// A origem pode desaparecer enquanto a tool roda; não deixar órfãos.
	if inv.OriginType == OriginChat && inv.OriginID != "" {
		opCtx, cancel := s.persistOpCtx(persistCtx)
		err := s.repo.ValidateChatOrigin(opCtx, inv.OriginID)
		cancel()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			opCtx, cancel = s.persistOpCtx(persistCtx)
			deleteErr := s.repo.Delete(opCtx, inv.ID)
			cancel()
			if deleteErr != nil {
				s.recordPersistenceFailure()
				logInvocationPersistenceFailure(ctx, "delete_orphan", Origin{Type: inv.OriginType, ID: inv.OriginID}, inv.ID, deleteErr)
			}
			return errors.Join(err, deleteErr)
		}
		if err != nil {
			logging.Warnf(ctx, "toolinvocations.service", "failed to revalidate chat origin %s; completing invocation: %v", inv.OriginID, err)
		}
	}
	completedAt := s.now()
	inv.Status, inv.DurationMs = outcome.status, outcome.durationMs
	inv.ErrorKind, inv.ErrorCode = string(outcome.errorKind), outcome.errorCode
	inv.ErrorMessage = s.truncateErrorForPersistence(outcome.message)
	inv.Retryable, inv.RetryabilityKnown = outcome.retryable, outcome.retryabilityKnown
	if req.observation != nil {
		completedAt = inv.QueuedAt.Add(time.Duration(outcome.durationMs) * time.Millisecond)
		inv.OutputPreview = truncateUTF8Safe(req.observation.Summary, 512)
		// Nenhum payload foi observado; não inventar detalhes técnicos.
		inv.ResultAvailability = "unavailable"
	} else {
		inv.Output = s.outputForPersistence(outcome.result)
		populateOutputProjection(inv)
		inv.Metadata = s.buildInvocationMetadata(req.persistenceCall(), req.iteration, outcome.durationMs, req.external, outcome.result)
	}
	inv.CompletedAt = &completedAt
	opCtx, cancel := s.persistOpCtx(persistCtx)
	err := s.repo.Complete(opCtx, inv.ID, inv)
	cancel()
	if err != nil {
		s.recordPersistenceFailure()
		logging.Errorf(ctx, "toolinvocations.service", "failed to complete invocation (id=%s): %v", inv.ID, err)
	}
	return err
}
