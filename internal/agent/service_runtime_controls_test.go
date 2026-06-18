package agent

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/core/ports"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
)

type loadSkillRuntimeTestTool struct{}

func (loadSkillRuntimeTestTool) Name() string        { return tools.LoadSkillName }
func (loadSkillRuntimeTestTool) Description() string { return "load skill" }
func (loadSkillRuntimeTestTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (loadSkillRuntimeTestTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{
		Content: "loaded",
		Metadata: map[string]any{
			"skill_slug":            "review",
			"skill_name":            "Review",
			"filesystem_read":       []string{"src/**"},
			"tools_allowed":         []string{"scope_probe"},
			"tools_denied":          []string{"danger_tool"},
			"bash_commands_allowed": []string{"go test ./..."},
			"bash_commands_denied":  []string{"rm -rf /"},
			"network_allowed_hosts": []string{"api.example.com"},
			"network_denied_hosts":  []string{"metadata.google.internal"},
		},
	}, nil
}

type scopeProbeTool struct{}

func (scopeProbeTool) Name() string                { return "scope_probe" }
func (scopeProbeTool) Description() string         { return "scope probe" }
func (scopeProbeTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (scopeProbeTool) Execute(ctx context.Context, _ json.RawMessage) (tools.ToolResult, error) {
	ec, ok := tools.GetExecutionContext(ctx)
	if !ok || ec.Filesystem == nil || len(ec.Filesystem.Read) == 0 {
		return tools.ToolResult{Content: "missing scope", IsError: true}, nil
	}
	if len(ec.AllowedTools) == 0 || ec.AllowedTools[0] != "scope_probe" ||
		len(ec.DeniedTools) == 0 || ec.DeniedTools[0] != "danger_tool" ||
		len(ec.AllowedBash) == 0 || ec.AllowedBash[0] != "go test ./..." ||
		len(ec.DeniedBash) == 0 || ec.DeniedBash[0] != "rm -rf /" ||
		len(ec.NetworkAllowedHost) == 0 || ec.NetworkAllowedHost[0] != "api.example.com" ||
		len(ec.NetworkDeniedHost) == 0 || ec.NetworkDeniedHost[0] != "metadata.google.internal" {
		return tools.ToolResult{Content: "missing non-filesystem permissions", IsError: true}, nil
	}
	return tools.ToolResult{Content: ec.Filesystem.Read[0]}, nil
}

type runtimeControlEmitter struct {
	events []string
	data   []any
}

func (e *runtimeControlEmitter) Emit(event string, data any) {
	e.events = append(e.events, event)
	e.data = append(e.data, data)
}

func TestExecuteToolCallsWithRuntimeControlsAppliesLoadSkillScopeBeforeRegularTools(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister(loadSkillRuntimeTestTool{})
	registry.MustRegister(scopeProbeTool{})
	emitter := &runtimeControlEmitter{}
	svc := &Service{toolExecutor: tools.NewExecutor(registry, tools.DefaultExecutorConfig()), emitter: emitter}

	batch := svc.executeToolCallsWithRuntimeControls(context.Background(), []tools.ToolCall{
		{ID: "regular", Type: "function", Function: tools.FunctionCall{Name: "scope_probe", Arguments: `{}`}},
		{ID: "load", Type: "function", Function: tools.FunctionCall{Name: tools.LoadSkillName, Arguments: `{"skill":"review"}`}},
	}, toolinvocations.Origin{Type: toolinvocations.OriginChat, ID: "turn-1"}, "conv-1", "turn-1", nil)

	if len(batch.Executions) != 2 {
		t.Fatalf("expected two executions, got %#v", batch.Executions)
	}
	if batch.Executions[0].CallID != "regular" || batch.Executions[1].CallID != "load" {
		t.Fatalf("result order must match original call order, got %#v", batch.Executions)
	}
	if batch.Executions[0].Result.IsError || batch.Executions[0].Result.Content != "src/**" {
		t.Fatalf("regular tool did not receive loaded skill scope: %#v", batch.Executions[0].Result)
	}
	if len(emitter.events) != 1 || emitter.events[0] != "chat:skill_loaded" {
		t.Fatalf("expected one chat:skill_loaded event, got %#v", emitter.events)
	}
	ev, ok := emitter.data[0].(ports.SkillLoadedEvent)
	if !ok {
		t.Fatalf("expected SkillLoadedEvent, got %T", emitter.data[0])
	}
	if ev.ConversationID != "conv-1" || ev.TurnID != "turn-1" || ev.Slug != "review" || ev.DisplayName != "Review" || ev.Mode != "on_demand" {
		t.Fatalf("unexpected skill loaded event: %+v", ev)
	}
}
