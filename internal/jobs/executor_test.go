package jobs

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"assistente/internal/tools"
)

// --- resolveForEachItems ---

func TestResolveForEachItems_SimpleKey(t *testing.T) {
	output := map[string]any{
		"results": []any{
			map[string]any{"id": "a"},
			map[string]any{"id": "b"},
		},
	}

	items := resolveForEachItems(output, "results")
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}

func TestResolveForEachItems_NestedPath(t *testing.T) {
	output := map[string]any{
		"data": map[string]any{
			"rows": []any{"x", "y", "z"},
		},
	}

	items := resolveForEachItems(output, "data.rows")
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
}

func TestResolveForEachItems_MissingKey(t *testing.T) {
	output := map[string]any{"other": "value"}
	items := resolveForEachItems(output, "missing")
	if items != nil {
		t.Fatalf("expected nil, got %v", items)
	}
}

func TestResolveForEachItems_NotArray(t *testing.T) {
	output := map[string]any{"field": "string-value"}
	items := resolveForEachItems(output, "field")
	if items != nil {
		t.Fatalf("expected nil for non-array, got %v", items)
	}
}

func TestResolveForEachItems_ContentArray(t *testing.T) {
	output := map[string]any{
		"content": []any{
			map[string]any{"id": "tenant-1", "name": "acme"},
			map[string]any{"id": "tenant-2", "name": "globex"},
		},
	}

	items := resolveForEachItems(output, "content")
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	first := items[0].(map[string]any)
	if first["id"] != "tenant-1" {
		t.Errorf("expected 'tenant-1', got %v", first["id"])
	}
}

// collectEvents subscribes to an event and collects payloads in a thread-safe way.
type eventCollector struct {
	mu       sync.Mutex
	events   []map[string]any
	wg       sync.WaitGroup
}

func newEventCollector(eb *EventBus, eventName string, expectedCount int) *eventCollector {
	c := &eventCollector{}
	c.wg.Add(expectedCount)
	eb.Subscribe(eventName, "test-collector", func(_ context.Context, _ string, payload map[string]any) {
		cp := make(map[string]any, len(payload))
		for k, v := range payload {
			cp[k] = v
		}
		c.mu.Lock()
		c.events = append(c.events, cp)
		c.mu.Unlock()
		c.wg.Done()
	})
	return c
}

func (c *eventCollector) wait(t *testing.T, timeout time.Duration) []map[string]any {
	t.Helper()
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatal("timeout waiting for events")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.events
}

// --- Fan-out payload via emitSuccess ---

func TestEmitSuccess_FanOut_MapItems_FlattenedPayload(t *testing.T) {
	eb := NewEventBus()
	logger := NewLogger(t.TempDir())

	executor := NewJobExecutor(ExecutorConfig{
		EventBus:       eb,
		Logger:         logger,
		CircuitBreaker: NewCircuitBreaker(),
	})

	job := &Job{
		ID: "upstream-job",
		Events: EventsConfig{
			OnSuccess: "job.upstream.success",
			ForEach:   "content",
		},
	}

	rl := &RunLog{
		RunID: "run-001",
		JobID: "upstream-job",
		Output: map[string]any{
			"content": []any{
				map[string]any{"key": "PROJ-100", "summary": "Fix bug", "priority": "high"},
				map[string]any{"key": "PROJ-200", "summary": "Add feature", "priority": "low"},
			},
		},
	}

	collector := newEventCollector(eb, "job.upstream.success", 2)
	executor.emitSuccess(context.Background(), job, rl, &TriggerContext{Type: TriggerManual})
	received := collector.wait(t, 2*time.Second)

	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}

	// Build a map by _fan_out_index to avoid ordering issues
	byIndex := make(map[int]map[string]any)
	for _, ev := range received {
		idx := ev["_fan_out_index"].(int)
		byIndex[idx] = ev
	}

	ev0 := byIndex[0]
	if ev0["key"] != "PROJ-100" {
		t.Errorf("event[0] key: got %v, want PROJ-100", ev0["key"])
	}
	if ev0["summary"] != "Fix bug" {
		t.Errorf("event[0] summary: got %v, want 'Fix bug'", ev0["summary"])
	}
	if ev0["priority"] != "high" {
		t.Errorf("event[0] priority: got %v, want 'high'", ev0["priority"])
	}
	if ev0["_fan_out_total"] != 2 {
		t.Errorf("event[0] _fan_out_total: got %v, want 2", ev0["_fan_out_total"])
	}
	if _, hasItem := ev0["item"]; hasItem {
		t.Error("event[0] should NOT have 'item' wrapper key")
	}

	ev1 := byIndex[1]
	if ev1["key"] != "PROJ-200" {
		t.Errorf("event[1] key: got %v, want PROJ-200", ev1["key"])
	}
	if ev1["summary"] != "Add feature" {
		t.Errorf("event[1] summary: got %v, want 'Add feature'", ev1["summary"])
	}
}

func TestEmitSuccess_FanOut_ScalarItems_WrappedInContent(t *testing.T) {
	eb := NewEventBus()
	logger := NewLogger(t.TempDir())

	executor := NewJobExecutor(ExecutorConfig{
		EventBus:       eb,
		Logger:         logger,
		CircuitBreaker: NewCircuitBreaker(),
	})

	job := &Job{
		ID: "scalar-job",
		Events: EventsConfig{
			OnSuccess: "job.scalar.success",
			ForEach:   "tags",
		},
	}

	rl := &RunLog{
		RunID: "run-002",
		JobID: "scalar-job",
		Output: map[string]any{
			"tags": []any{"alpha", "beta", "gamma"},
		},
	}

	collector := newEventCollector(eb, "job.scalar.success", 3)
	executor.emitSuccess(context.Background(), job, rl, &TriggerContext{Type: TriggerManual})
	received := collector.wait(t, 2*time.Second)

	if len(received) != 3 {
		t.Fatalf("expected 3 events, got %d", len(received))
	}

	byIndex := make(map[int]map[string]any)
	for _, ev := range received {
		idx := ev["_fan_out_index"].(int)
		byIndex[idx] = ev
	}

	expected := []string{"alpha", "beta", "gamma"}
	for i, want := range expected {
		ev := byIndex[i]
		if ev["content"] != want {
			t.Errorf("event[%d] content: got %v, want %q", i, ev["content"], want)
		}
		if ev["_fan_out_total"] != 3 {
			t.Errorf("event[%d] _fan_out_total: got %v, want 3", i, ev["_fan_out_total"])
		}
	}
}

func TestEmitSuccess_NoFanOut_SingleEvent(t *testing.T) {
	eb := NewEventBus()
	logger := NewLogger(t.TempDir())

	executor := NewJobExecutor(ExecutorConfig{
		EventBus:       eb,
		Logger:         logger,
		CircuitBreaker: NewCircuitBreaker(),
	})

	job := &Job{
		ID: "simple-job",
		Events: EventsConfig{
			OnSuccess: "job.simple.success",
		},
	}

	rl := &RunLog{
		RunID:  "run-003",
		JobID:  "simple-job",
		Output: map[string]any{"status": "ok", "count": 42},
	}

	collector := newEventCollector(eb, "job.simple.success", 1)
	executor.emitSuccess(context.Background(), job, rl, &TriggerContext{Type: TriggerManual})
	received := collector.wait(t, 2*time.Second)

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}

	ev := received[0]
	if ev["status"] != "ok" {
		t.Errorf("status: got %v, want 'ok'", ev["status"])
	}
	if ev["count"] != 42 {
		t.Errorf("count: got %v, want 42", ev["count"])
	}
}

func TestEmitSuccess_FanOut_ForEachMissing_FallsBackToSingle(t *testing.T) {
	eb := NewEventBus()
	logger := NewLogger(t.TempDir())

	executor := NewJobExecutor(ExecutorConfig{
		EventBus:       eb,
		Logger:         logger,
		CircuitBreaker: NewCircuitBreaker(),
	})

	job := &Job{
		ID: "fallback-job",
		Events: EventsConfig{
			OnSuccess: "job.fallback.success",
			ForEach:   "nonexistent",
		},
	}

	rl := &RunLog{
		RunID:  "run-004",
		JobID:  "fallback-job",
		Output: map[string]any{"data": "hello"},
	}

	collector := newEventCollector(eb, "job.fallback.success", 1)
	executor.emitSuccess(context.Background(), job, rl, &TriggerContext{Type: TriggerManual})
	received := collector.wait(t, 2*time.Second)

	if len(received) != 1 {
		t.Fatalf("expected 1 fallback event, got %d", len(received))
	}
	if received[0]["data"] != "hello" {
		t.Errorf("data: got %v, want 'hello'", received[0]["data"])
	}
}

// --- End-to-end: fan-out + template resolution in downstream ---

func TestFanOut_DownstreamTemplateResolution(t *testing.T) {
	output := map[string]any{
		"issues": []any{
			map[string]any{"key": "TASK-1", "title": "First", "assignee": "alice"},
			map[string]any{"key": "TASK-2", "title": "Second", "assignee": "bob"},
		},
	}

	items := resolveForEachItems(output, "issues")
	if len(items) != 2 {
		t.Fatalf("expected 2, got %d", len(items))
	}

	for i, item := range items {
		payload := make(map[string]any)
		if m, ok := item.(map[string]any); ok {
			for k, v := range m {
				payload[k] = v
			}
		}
		payload["_fan_out_index"] = i
		payload["_fan_out_total"] = len(items)

		ctx := &TemplateContext{Event: payload}

		key, err := resolveTemplate("{{ .event.key }}", ctx)
		if err != nil {
			t.Fatalf("item[%d] resolve key error: %v", i, err)
		}

		title, err := resolveTemplate("{{ .event.title }}", ctx)
		if err != nil {
			t.Fatalf("item[%d] resolve title error: %v", i, err)
		}

		if i == 0 {
			if key != "TASK-1" {
				t.Errorf("item[0] key = %q, want TASK-1", key)
			}
			if title != "First" {
				t.Errorf("item[0] title = %q, want First", title)
			}
		}
		if i == 1 {
			if key != "TASK-2" {
				t.Errorf("item[1] key = %q, want TASK-2", key)
			}
			if title != "Second" {
				t.Errorf("item[1] title = %q, want Second", title)
			}
		}
	}
}

// --- fakeTool for Execute tests ---

type fakeTool struct {
	name       string
	params     json.RawMessage
	response   string
	callCount  int
	lastArgs   json.RawMessage
}

func (f *fakeTool) Name() string                 { return f.name }
func (f *fakeTool) Description() string           { return "fake tool for testing" }
func (f *fakeTool) Parameters() json.RawMessage   { return f.params }
func (f *fakeTool) Execute(_ context.Context, args json.RawMessage) (tools.ToolResult, error) {
	f.callCount++
	f.lastArgs = args
	return tools.ToolResult{Content: f.response}, nil
}

// --- Execute captures ToolName and ResolvedInputs ---

func TestExecute_CapturesToolNameAndResolvedInputs(t *testing.T) {
	registry := tools.NewRegistry()
	ft := &fakeTool{
		name:     "test_greet",
		params:   json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"count":{"type":"integer"}}}`),
		response: `{"greeting":"hello world"}`,
	}
	registry.MustRegister(ft)

	logger := NewLogger(t.TempDir())
	executor := NewJobExecutor(ExecutorConfig{
		ToolRegistry:   registry,
		EventBus:       NewEventBus(),
		Logger:         logger,
		CircuitBreaker: NewCircuitBreaker(),
	})

	job := &Job{
		ID:   "greet-job",
		Tool: "test_greet",
		Inputs: map[string]any{
			"name":  "Alice",
			"count": "5",
		},
	}

	rl := executor.Execute(context.Background(), job, &TriggerContext{Type: TriggerManual})

	if rl.Status != "completed" {
		t.Fatalf("expected completed, got %s (error: %s)", rl.Status, rl.Error)
	}
	if rl.ToolName != "test_greet" {
		t.Errorf("ToolName: got %q, want %q", rl.ToolName, "test_greet")
	}
	if rl.ResolvedInputs == nil {
		t.Fatal("ResolvedInputs is nil, expected non-nil")
	}
	if rl.ResolvedInputs["name"] != "Alice" {
		t.Errorf("ResolvedInputs[name]: got %v, want 'Alice'", rl.ResolvedInputs["name"])
	}
}

func TestExecute_CapturesResolvedInputsWithTemplates(t *testing.T) {
	registry := tools.NewRegistry()
	ft := &fakeTool{
		name:     "test_search",
		params:   json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"integer"}}}`),
		response: `{"results":[]}`,
	}
	registry.MustRegister(ft)

	logger := NewLogger(t.TempDir())
	executor := NewJobExecutor(ExecutorConfig{
		ToolRegistry:   registry,
		EventBus:       NewEventBus(),
		Logger:         logger,
		CircuitBreaker: NewCircuitBreaker(),
	})

	job := &Job{
		ID:   "search-job",
		Tool: "test_search",
		Inputs: map[string]any{
			"query": "{{ .event.keyword }}",
			"limit": "10",
		},
	}

	trigCtx := &TriggerContext{
		Type:         TriggerEvent,
		EventName:    "job.upstream.success",
		EventPayload: map[string]any{"keyword": "golang"},
	}

	rl := executor.Execute(context.Background(), job, trigCtx)

	if rl.Status != "completed" {
		t.Fatalf("expected completed, got %s (error: %s)", rl.Status, rl.Error)
	}
	if rl.ResolvedInputs["query"] != "golang" {
		t.Errorf("ResolvedInputs[query]: got %v, want 'golang'", rl.ResolvedInputs["query"])
	}
}

func TestExecute_CapturesInputsEvenOnFailure(t *testing.T) {
	registry := tools.NewRegistry()
	ft := &fakeTool{
		name:     "test_fail",
		params:   json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}}}`),
		response: `{"error":true,"message":"not found"}`,
	}
	ft.response = "" // force empty response to test error handling
	registry.MustRegister(ft)

	logger := NewLogger(t.TempDir())
	executor := NewJobExecutor(ExecutorConfig{
		ToolRegistry:   registry,
		EventBus:       NewEventBus(),
		Logger:         logger,
		CircuitBreaker: NewCircuitBreaker(),
	})

	// Use a tool that returns IsError
	failTool := &fakeErrorTool{name: "test_fail_tool"}
	registry.Unregister("test_fail")
	registry.MustRegister(failTool)

	job := &Job{
		ID:   "fail-job",
		Tool: "test_fail_tool",
		Inputs: map[string]any{
			"id": "item-999",
		},
	}

	rl := executor.Execute(context.Background(), job, &TriggerContext{Type: TriggerManual})

	if rl.Status != "failed" {
		t.Fatalf("expected failed, got %s", rl.Status)
	}
	if rl.ToolName != "test_fail_tool" {
		t.Errorf("ToolName: got %q, want %q", rl.ToolName, "test_fail_tool")
	}
	if rl.ResolvedInputs == nil {
		t.Fatal("ResolvedInputs should be captured even on failure")
	}
	if rl.ResolvedInputs["id"] != "item-999" {
		t.Errorf("ResolvedInputs[id]: got %v, want 'item-999'", rl.ResolvedInputs["id"])
	}
}

type fakeErrorTool struct {
	name string
}

func (f *fakeErrorTool) Name() string               { return f.name }
func (f *fakeErrorTool) Description() string         { return "always fails" }
func (f *fakeErrorTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}}}`) }
func (f *fakeErrorTool) Execute(_ context.Context, _ json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{Content: "something went wrong", IsError: true}, nil
}

// --- Execute persists RunLog to disk ---

func TestExecute_RunLogPersistedWithInputs(t *testing.T) {
	registry := tools.NewRegistry()
	ft := &fakeTool{
		name:     "test_persist",
		params:   json.RawMessage(`{"type":"object","properties":{"x":{"type":"string"}}}`),
		response: `{"ok":true}`,
	}
	registry.MustRegister(ft)

	logger := NewLogger(t.TempDir())
	executor := NewJobExecutor(ExecutorConfig{
		ToolRegistry:   registry,
		EventBus:       NewEventBus(),
		Logger:         logger,
		CircuitBreaker: NewCircuitBreaker(),
	})

	job := &Job{
		ID:   "persist-job",
		Tool: "test_persist",
		Inputs: map[string]any{
			"x": "hello",
		},
	}

	rl := executor.Execute(context.Background(), job, &TriggerContext{Type: TriggerManual})
	if rl.Status != "completed" {
		t.Fatalf("expected completed, got %s", rl.Status)
	}

	// Recover from disk via GetRun
	recovered, err := logger.GetRun("persist-job", rl.RunID)
	if err != nil {
		t.Fatalf("GetRun error: %v", err)
	}
	if recovered.ToolName != "test_persist" {
		t.Errorf("recovered ToolName: got %q, want %q", recovered.ToolName, "test_persist")
	}
	if recovered.ResolvedInputs["x"] != "hello" {
		t.Errorf("recovered ResolvedInputs[x]: got %v, want 'hello'", recovered.ResolvedInputs["x"])
	}
	if recovered.Output["ok"] != true {
		t.Errorf("recovered Output[ok]: got %v, want true", recovered.Output["ok"])
	}
}
