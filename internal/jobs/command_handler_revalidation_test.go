package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"assistente/internal/tools"
)

type commandHandlerRevalidationProbeTool struct {
	calls atomic.Int32
}

func (t *commandHandlerRevalidationProbeTool) Name() string {
	return "command-handler-revalidation-probe"
}

func (t *commandHandlerRevalidationProbeTool) Description() string { return "revalidation test probe" }

func (t *commandHandlerRevalidationProbeTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}

func (t *commandHandlerRevalidationProbeTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	t.calls.Add(1)
	return tools.ToolResult{Content: `{"ok":true}`}, nil
}

func executeCommandHandlerRevalidationProbe(t *testing.T, revalidate func(context.Context) error) (error, *commandHandlerRevalidationProbeTool) {
	t.Helper()
	probe := &commandHandlerRevalidationProbeTool{}
	registry := tools.NewRegistry()
	registry.MustRegister(probe)
	executor := mustNewJobExecutor(t, ExecutorConfig{ToolRegistry: registry})
	ctx := context.WithValue(context.Background(), commandJobDispatchKey{}, commandJobDispatch{
		jobID:      "job-db-id",
		revalidate: revalidate,
	})
	_, err := executor.executeSingle(ctx, &Job{DatabaseID: "job-db-id", Tool: probe.Name()}, &TriggerContext{}, nil)
	return err, probe
}

func requireAttemptFailure(t *testing.T, err error) *attemptFailure {
	t.Helper()
	if err == nil {
		t.Fatal("executeSingle returned nil error")
	}
	var failure *attemptFailure
	if !errors.As(err, &failure) {
		t.Fatalf("error = %T %v, want attemptFailure", err, err)
	}
	return failure
}

func TestExecuteSinglePreservesCommandJobRevalidationUnavailable(t *testing.T) {
	err, probe := executeCommandHandlerRevalidationProbe(t, func(context.Context) error {
		return permanentAttemptFailure(errors.New("revalidation backend unavailable"), tools.ErrorKindUnavailable, "command_job_revalidation_unavailable")
	})

	failure := requireAttemptFailure(t, err)
	if failure.kind != tools.ErrorKindUnavailable {
		t.Fatalf("failure kind = %q, want %q", failure.kind, tools.ErrorKindUnavailable)
	}
	if failure.code != "command_job_revalidation_unavailable" {
		t.Fatalf("failure code = %q, want command_job_revalidation_unavailable", failure.code)
	}
	if failure.retryable {
		t.Fatal("revalidation unavailability must not be retryable")
	}
	if !failure.retryabilityKnown {
		t.Fatal("revalidation unavailability must have known retryability")
	}
	if got := probe.calls.Load(); got != 0 {
		t.Fatalf("tool calls = %d, want 0", got)
	}
}

func TestExecuteSingleCommandJobDeniedUsesAuthorizationFailure(t *testing.T) {
	err, probe := executeCommandHandlerRevalidationProbe(t, func(context.Context) error {
		return ErrCommandJobDenied
	})

	failure := requireAttemptFailure(t, err)
	if failure.kind != tools.ErrorKindAuthorization {
		t.Fatalf("failure kind = %q, want %q", failure.kind, tools.ErrorKindAuthorization)
	}
	if failure.code != "command_job_authorization_changed" {
		t.Fatalf("failure code = %q, want command_job_authorization_changed", failure.code)
	}
	if failure.retryable {
		t.Fatal("authorization change must not be retryable")
	}
	if got := probe.calls.Load(); got != 0 {
		t.Fatalf("tool calls = %d, want 0", got)
	}
}

func TestExecuteSingleRejectsMissingRevalidationAndCancellationBeforeTool(t *testing.T) {
	t.Run("missing revalidation", func(t *testing.T) {
		probe := &commandHandlerRevalidationProbeTool{}
		registry := tools.NewRegistry()
		registry.MustRegister(probe)
		executor := mustNewJobExecutor(t, ExecutorConfig{ToolRegistry: registry})
		ctx := context.WithValue(context.Background(), commandJobDispatchKey{}, commandJobDispatch{jobID: "job-db-id"})
		_, err := executor.executeSingle(ctx, &Job{DatabaseID: "job-db-id", Tool: probe.Name()}, &TriggerContext{}, nil)

		failure := requireAttemptFailure(t, err)
		if failure.kind != tools.ErrorKindAuthorization || failure.code != "command_job_authorization_changed" {
			t.Fatalf("failure = (%q, %q), want authorization/command_job_authorization_changed", failure.kind, failure.code)
		}
		if probe.calls.Load() != 0 {
			t.Fatal("tool executed with missing revalidation")
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		err, probe := executeCommandHandlerRevalidationProbe(t, func(context.Context) error {
			return context.Canceled
		})
		if err == nil {
			t.Fatal("canceled revalidation returned nil error")
		}
		if probe.calls.Load() != 0 {
			t.Fatal("tool executed after canceled revalidation")
		}
	})
}
