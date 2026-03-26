package jobs

import (
	"strings"
	"testing"
	"time"
)

func TestGetRun_Found(t *testing.T) {
	logger := NewLogger(t.TempDir())

	rl := &RunLog{
		RunID:     "run-abc-123",
		JobID:     "my-job",
		ToolName:  "some_tool",
		Status:    "completed",
		StartedAt: time.Now(),
		ResolvedInputs: map[string]any{
			"key": "value",
		},
		Output: map[string]any{
			"result": "ok",
		},
	}

	if err := logger.LogRun(rl); err != nil {
		t.Fatalf("LogRun error: %v", err)
	}

	recovered, err := logger.GetRun("my-job", "run-abc-123")
	if err != nil {
		t.Fatalf("GetRun error: %v", err)
	}
	if recovered.RunID != "run-abc-123" {
		t.Errorf("RunID: got %q, want %q", recovered.RunID, "run-abc-123")
	}
	if recovered.ToolName != "some_tool" {
		t.Errorf("ToolName: got %q, want %q", recovered.ToolName, "some_tool")
	}
	if recovered.ResolvedInputs["key"] != "value" {
		t.Errorf("ResolvedInputs[key]: got %v, want 'value'", recovered.ResolvedInputs["key"])
	}
	if recovered.Output["result"] != "ok" {
		t.Errorf("Output[result]: got %v, want 'ok'", recovered.Output["result"])
	}
}

func TestGetRun_NotFound(t *testing.T) {
	logger := NewLogger(t.TempDir())

	rl := &RunLog{
		RunID:     "run-exists",
		JobID:     "my-job",
		Status:    "completed",
		StartedAt: time.Now(),
	}
	if err := logger.LogRun(rl); err != nil {
		t.Fatalf("LogRun error: %v", err)
	}

	_, err := logger.GetRun("my-job", "run-does-not-exist")
	if err == nil {
		t.Fatal("expected error for non-existent run, got nil")
	}
	if !strings.Contains(err.Error(), "run not found") {
		t.Errorf("expected 'run not found' in error, got: %v", err)
	}
}

func TestGetRun_JobNotFound(t *testing.T) {
	logger := NewLogger(t.TempDir())

	_, err := logger.GetRun("nonexistent-job", "run-123")
	if err == nil {
		t.Fatal("expected error for non-existent job, got nil")
	}
	if !strings.Contains(err.Error(), "run not found") {
		t.Errorf("expected 'run not found' in error, got: %v", err)
	}
}

func TestGetRun_MultipleRuns_FindsCorrectOne(t *testing.T) {
	logger := NewLogger(t.TempDir())

	runs := []RunLog{
		{RunID: "run-001", JobID: "multi-job", Status: "completed", StartedAt: time.Now().Add(-2 * time.Second), ToolName: "tool-a"},
		{RunID: "run-002", JobID: "multi-job", Status: "failed", StartedAt: time.Now().Add(-1 * time.Second), ToolName: "tool-b"},
		{RunID: "run-003", JobID: "multi-job", Status: "completed", StartedAt: time.Now(), ToolName: "tool-c"},
	}

	for i := range runs {
		if err := logger.LogRun(&runs[i]); err != nil {
			t.Fatalf("LogRun error: %v", err)
		}
	}

	got, err := logger.GetRun("multi-job", "run-002")
	if err != nil {
		t.Fatalf("GetRun error: %v", err)
	}
	if got.RunID != "run-002" {
		t.Errorf("RunID: got %q, want %q", got.RunID, "run-002")
	}
	if got.ToolName != "tool-b" {
		t.Errorf("ToolName: got %q, want %q", got.ToolName, "tool-b")
	}
	if got.Status != "failed" {
		t.Errorf("Status: got %q, want 'failed'", got.Status)
	}
}

func TestLogRun_PreservesResolvedInputs(t *testing.T) {
	logger := NewLogger(t.TempDir())

	rl := &RunLog{
		RunID:     "run-inputs",
		JobID:     "inputs-job",
		ToolName:  "my_tool",
		Status:    "completed",
		StartedAt: time.Now(),
		ResolvedInputs: map[string]any{
			"query":    "SELECT * FROM users",
			"limit":    float64(50),
			"nested":   map[string]any{"a": "b"},
		},
	}

	if err := logger.LogRun(rl); err != nil {
		t.Fatalf("LogRun error: %v", err)
	}

	recovered, err := logger.GetRun("inputs-job", "run-inputs")
	if err != nil {
		t.Fatalf("GetRun error: %v", err)
	}

	if recovered.ResolvedInputs["query"] != "SELECT * FROM users" {
		t.Errorf("query: got %v", recovered.ResolvedInputs["query"])
	}
	if recovered.ResolvedInputs["limit"] != float64(50) {
		t.Errorf("limit: got %v (type %T)", recovered.ResolvedInputs["limit"], recovered.ResolvedInputs["limit"])
	}
	nested, ok := recovered.ResolvedInputs["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested: expected map, got %T", recovered.ResolvedInputs["nested"])
	}
	if nested["a"] != "b" {
		t.Errorf("nested.a: got %v", nested["a"])
	}
}
