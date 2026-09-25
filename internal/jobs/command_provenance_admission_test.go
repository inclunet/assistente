package jobs

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/commandjobevents"
	"assistente/internal/database"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
)

type provenanceAdmissionTool struct{ calls int }

func (t *provenanceAdmissionTool) Name() string { return "test_tool" }

func (t *provenanceAdmissionTool) Description() string { return "tool de admissão de proveniência" }

func (t *provenanceAdmissionTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}

func (t *provenanceAdmissionTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	t.calls++
	return tools.ToolResult{Content: `{"called":true}`}, nil
}

func TestCommandProvenanceInvalidFailsBeforeQueuedWithRealRepository(t *testing.T) {
	repo, userCtx, _ := setupJobsRepositoryTest(t)
	if err := repo.db.AutoMigrate(commandjobevents.Models()...); err != nil {
		t.Fatalf("migrate command events: %v", err)
	}

	tool := &provenanceAdmissionTool{}
	registry := tools.NewRegistry()
	registry.MustRegister(tool)
	invocations := toolinvocations.NewService(
		toolinvocations.NewDBRepository(repo.db),
		tools.NewExecutor(registry, tools.DefaultExecutorConfig()),
	)
	executor := mustNewJobExecutor(t, ExecutorConfig{
		ToolRegistry:    registry,
		ToolInvocations: invocations,
		Repository:      repo,
	})
	job := testRepositoryJob("invalid-command-provenance", "Invalid command provenance")

	firstInvocation := uuid7ForTest(t)
	cases := []struct {
		name  string
		chain []any
	}{
		{
			name: "command id sem namespace",
			chain: []any{map[string]any{
				"command_id":    "step",
				"invocation_id": firstInvocation,
				"layer_refs":    []string{"layer.a"},
			}},
		},
		{
			name: "invocation duplicada",
			chain: []any{
				map[string]any{"command_id": "job.first", "invocation_id": firstInvocation, "layer_refs": []string{"layer.a"}},
				map[string]any{"command_id": "job.second", "invocation_id": firstInvocation, "layer_refs": []string{"layer.b"}},
			},
		},
		{
			name: "layer duplicada",
			chain: []any{map[string]any{
				"command_id":    "job.first",
				"invocation_id": uuid7ForTest(t),
				"layer_refs":    []string{"layer.a", "layer.a"},
			}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beforeRuns, beforeEvents, beforeOutbox := countProvenanceAdmissionRows(t, repo)
			run := executor.Execute(userCtx, job, &TriggerContext{
				Type:         TriggerManual,
				EventPayload: map[string]any{},
				Provenance:   map[string]any{"command_chain_history": tc.chain},
			})
			if run == nil || run.Status != RunStatusFailed || run.Error != "invalid command provenance" {
				t.Fatalf("invalid provenance result=%+v", run)
			}
			afterRuns, afterEvents, afterOutbox := countProvenanceAdmissionRows(t, repo)
			if afterRuns != beforeRuns || afterEvents != beforeEvents || afterOutbox != beforeOutbox {
				t.Fatalf("invalid provenance crossed queued boundary: before=(%d,%d,%d) after=(%d,%d,%d)", beforeRuns, beforeEvents, beforeOutbox, afterRuns, afterEvents, afterOutbox)
			}
			if tool.calls != 0 {
				t.Fatalf("tool executed for invalid provenance: calls=%d", tool.calls)
			}
		})
	}
}

func countProvenanceAdmissionRows(t *testing.T, repo *DBRepository) (runs, events, outbox int64) {
	t.Helper()
	if err := repo.db.Model(&database.JobRun{}).Count(&runs).Error; err != nil {
		t.Fatalf("count job_runs: %v", err)
	}
	if err := repo.db.Model(&database.JobRunEvent{}).Count(&events).Error; err != nil {
		t.Fatalf("count job_run_events: %v", err)
	}
	if err := repo.db.Model(&commandjobevents.ActivationOutbox{}).Count(&outbox).Error; err != nil {
		t.Fatalf("count activation outbox: %v", err)
	}
	return runs, events, outbox
}
