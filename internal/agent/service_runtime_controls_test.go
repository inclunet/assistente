package agent

import (
	"context"
	"encoding/json"
	"testing"

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
			"skill_slug":      "review",
			"filesystem_read": []string{"src/**"},
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
	return tools.ToolResult{Content: ec.Filesystem.Read[0]}, nil
}

func TestExecuteToolCallsWithRuntimeControlsAppliesLoadSkillScopeBeforeRegularTools(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister(loadSkillRuntimeTestTool{})
	registry.MustRegister(scopeProbeTool{})
	svc := &Service{toolExecutor: tools.NewExecutor(registry, tools.DefaultExecutorConfig())}

	batch := svc.executeToolCallsWithRuntimeControls(context.Background(), []tools.ToolCall{
		{ID: "regular", Type: "function", Function: tools.FunctionCall{Name: "scope_probe", Arguments: `{}`}},
		{ID: "load", Type: "function", Function: tools.FunctionCall{Name: tools.LoadSkillName, Arguments: `{"skill":"review"}`}},
	}, toolinvocations.Origin{Type: toolinvocations.OriginChat, ID: "turn-1"})

	if len(batch.Executions) != 2 {
		t.Fatalf("expected two executions, got %#v", batch.Executions)
	}
	if batch.Executions[0].CallID != "regular" || batch.Executions[1].CallID != "load" {
		t.Fatalf("result order must match original call order, got %#v", batch.Executions)
	}
	if batch.Executions[0].Result.IsError || batch.Executions[0].Result.Content != "src/**" {
		t.Fatalf("regular tool did not receive loaded skill scope: %#v", batch.Executions[0].Result)
	}
}
