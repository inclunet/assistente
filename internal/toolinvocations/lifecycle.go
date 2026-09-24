package toolinvocations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"assistente/internal/database"
	"assistente/internal/logging"
	"assistente/internal/tools"

	"gorm.io/gorm"
)

var errChatLedgerUnavailable = errors.New("chat ledger unavailable")

// As entradas adaptam seus resultados; somente este núcleo grava o ciclo de vida.
type invocationStart struct {
	call                tools.ToolCall
	persistedArguments  *string
	origin              Origin
	parentID, catalogID string
	dryRun, external    bool
	iteration           int
	observation         *ExternalObservation
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
		// Nunca confiar no ID fornecido sem conferir usuário e identidade.
		if supplied := strings.TrimSpace(req.catalogID); supplied != "" {
			opCtx, cancel := s.persistOpCtx(ctx)
			_, err := s.repo.IsToolCatalogIDVisible(opCtx, supplied)
			cancel()
			if err != nil {
				return "", err
			}
		}
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
	origin := req.origin
	origin.Type, origin.ID = strings.TrimSpace(origin.Type), strings.TrimSpace(origin.ID)
	if origin.Type == "" {
		origin.Type = OriginChat
	}
	if origin.Type == OriginChat {
		opCtx, cancel := s.persistOpCtx(persistCtx)
		err := s.repo.ValidateChatOrigin(opCtx, origin.ID)
		cancel()
		if err != nil {
			s.recordPersistenceFailure()
			return Invocation{}, fmt.Errorf("%w: %w", errChatLedgerUnavailable, err)
		}
	}
	queuedAt := s.now()
	catalogID, err := s.resolveInvocationCatalog(persistCtx, req)
	if err != nil {
		s.recordPersistenceFailure()
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
		var display map[string]json.RawMessage
		if err := json.Unmarshal(observation.DisplayMetadata, &display); err != nil || display == nil {
			return Invocation{}, fmt.Errorf("observation display must be a JSON object")
		}
		if len(observation.DisplayMetadata) > tools.DefaultMaxResultSize {
			return Invocation{}, fmt.Errorf("observation display exceeds size limit")
		}
		inv.Metadata, _ = json.Marshal(map[string]any{"external": true, "display": display})
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
		if db := database.DB(); db != nil && db.Migrator().HasTable(&database.ChatMessage{}) {
			opCtx, cancel := s.persistOpCtx(persistCtx)
			_, err := database.GetMessageWithContext(opCtx, inv.OriginID)
			cancel()
			if errors.Is(err, gorm.ErrRecordNotFound) {
				opCtx, cancel = s.persistOpCtx(persistCtx)
				deleteErr := s.repo.Delete(opCtx, inv.ID)
				cancel()
				if deleteErr != nil {
					s.recordPersistenceFailure()
				}
				return errors.Join(err, deleteErr)
			}
			if err != nil {
				logging.Warnf(ctx, "toolinvocations.service", "failed to revalidate chat origin %s; completing invocation: %v", inv.OriginID, err)
			}
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
