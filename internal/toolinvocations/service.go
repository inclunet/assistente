package toolinvocations

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"assistente/internal/tools"
)

type Service struct {
	repo     Repository
	executor *tools.Executor
	now      func() time.Time
}

func NewService(repo Repository, executor *tools.Executor) *Service {
	return &Service{repo: repo, executor: executor, now: time.Now}
}

func (s *Service) CleanOld(ctx context.Context, maxAge time.Duration) (int, error) {
	if s == nil || s.repo == nil {
		return 0, nil
	}
	return s.repo.CleanOld(ctx, maxAge)
}

func (s *Service) Execute(ctx context.Context, req ExecuteRequest) ExecuteResult {
	if s == nil || s.executor == nil {
		return ExecuteResult{Execution: executionError(req.Call, "tool invocation service not configured")}
	}

	queuedAt := s.now()
	toolCatalogID := req.ToolCatalogID
	if toolCatalogID == "" && s.repo != nil {
		if id, err := s.repo.ResolveToolCatalogID(ctx, req.Call.Function.Name); err == nil {
			toolCatalogID = id
		}
	}
	input, _ := json.Marshal(map[string]any{
		"tool_call": req.Call,
	})
	inv := Invocation{
		ToolCatalogID:      toolCatalogID,
		OriginType:         req.Origin.Type,
		OriginID:           req.Origin.ID,
		ParentInvocationID: req.ParentInvocationID,
		ToolCallID:         req.Call.ID,
		Status:             StatusQueued,
		DryRun:             req.DryRun,
		Input:              input,
		QueuedAt:           queuedAt,
	}
	if inv.OriginType == "" {
		inv.OriginType = OriginChat
	}

	if s.repo != nil && toolCatalogID != "" {
		if err := s.repo.Create(ctx, &inv); err != nil {
			exec := executionError(req.Call, fmt.Sprintf("failed to create tool invocation: %v", err))
			return ExecuteResult{Invocation: inv, Execution: exec}
		}
		_ = s.repo.MarkRunning(ctx, inv.ID, s.now())
	}

	exec := s.executor.ExecuteOne(ctx, req.Call)
	if s.repo != nil && inv.ID != "" {
		status := StatusCompleted
		errorMessage := ""
		if exec.Result.IsError || exec.Error != nil {
			status = StatusFailed
			if exec.Error != nil {
				errorMessage = exec.Error.Error()
			} else {
				errorMessage = exec.Result.Content
			}
		}
		completedAt := s.now()
		inv.Status = status
		inv.Output = resultOutput(exec.Result)
		inv.ErrorKind = string(exec.ErrorKind)
		inv.ErrorMessage = errorMessage
		inv.Retryable = exec.Retryable
		inv.CompletedAt = &completedAt
		inv.DurationMs = exec.DurationMs
		metadata, _ := json.Marshal(map[string]any{
			"tool_name": exec.ToolName,
			"call_id":   exec.CallID,
			"dry_run":   req.DryRun,
		})
		inv.Metadata = metadata
		_ = s.repo.Complete(ctx, inv.ID, &inv)
	}

	return ExecuteResult{Invocation: inv, Execution: exec}
}

func (s *Service) ExecuteAll(ctx context.Context, calls []tools.ToolCall, origin Origin) []ExecuteResult {
	results := make([]ExecuteResult, len(calls))
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func(idx int, tc tools.ToolCall) {
			defer wg.Done()
			results[idx] = s.Execute(ctx, ExecuteRequest{Call: tc, Origin: origin})
		}(i, call)
	}
	wg.Wait()
	return results
}

func executionError(call tools.ToolCall, message string) tools.ToolExecutionResult {
	return tools.ToolExecutionResult{
		CallID:   call.ID,
		ToolName: call.Function.Name,
		Result: tools.ToolResult{
			Content: message,
			IsError: true,
		},
		ErrorKind:  tools.ErrorKindUnknown,
		Retryable:  false,
		DurationMs: 0,
	}
}
