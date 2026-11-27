package tools

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"assistente/internal/tools/invocationctx"
)

func TestBeforeToolCancellationDuringGrantRecheck(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx = invocationctx.WithVerifiedWorkspaceID(ctx, "w")
	var checks atomic.Int32
	ctx = invocationctx.WithAgentCheck(ctx, func(context.Context) error {
		if checks.Add(1) == 2 {
			cancel()
		}
		return nil
	})
	var calls atomic.Int32
	tool := &mockTool{name: "run_command", exec: func(context.Context, json.RawMessage) (ToolResult, error) { calls.Add(1); return ToolResult{}, nil }}
	cfg := DefaultExecutorConfig()
	cfg.BeforeTool = func(context.Context, ToolCall) error { return nil }
	r := NewExecutor(newRegistry(tool), cfg).ExecuteOne(ctx, ToolCall{Function: FunctionCall{Name: tool.name, Arguments: "{}"}})
	if calls.Load() != 0 || r.ErrorCode != "plugin_hook_blocked" || checks.Load() != 2 {
		t.Fatalf("resultado=%+v chamadas=%d verificações=%d", r, calls.Load(), checks.Load())
	}
}

func TestBeforeToolInVerifiedWorkspace(t *testing.T) {
	for _, tc := range []struct {
		name            string
		workspace       bool
		mode            string
		hooks, dispatch int32
	}{
		{"workspace libera e ferramenta recusa", true, "allow", 1, 1},
		{"veto", true, "deny", 1, 0},
		{"falha interna", true, "panic", 1, 0},
		{"prazo com resposta atrasada", true, "timeout", 1, 0},
		{"sem origem de workspace", false, "deny", 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var seen, called atomic.Int32
				release := make(chan struct{})
				defer close(release)
				cfg := DefaultExecutorConfig()
				cfg.ToolTimeout = time.Second
				cfg.BeforeTool = func(ctx context.Context, call ToolCall) error {
					seen.Add(1)
					if call.ID != "1" || call.Function.Name != "run_command" || call.Function.Arguments != "{}" || invocationctx.VerifiedWorkspaceID(ctx) != "w" {
						t.Error("evento ou origem incorretos")
					}
					switch tc.mode {
					case "deny":
						return errors.New("detalhes privados")
					case "panic":
						panic("detalhes privados")
					case "timeout":
						<-release
					}
					return nil
				}
				tool := &mockTool{name: "run_command", exec: func(context.Context, json.RawMessage) (ToolResult, error) {
					called.Add(1)
					return ToolResult{IsError: true, Content: "recusa da ferramenta", Failure: &ToolFailure{Code: "tool_denied", Kind: ErrorKindAuthorization}}, nil
				}}
				ctx := t.Context()
				if tc.workspace {
					ctx = invocationctx.WithVerifiedWorkspaceID(ctx, "w")
				}
				result := NewExecutor(newRegistry(tool), cfg).ExecuteOne(ctx, ToolCall{ID: "1", Function: FunctionCall{Name: tool.name, Arguments: "{}"}})
				if seen.Load() != tc.hooks || called.Load() != tc.dispatch {
					t.Fatalf("hooks=%d despachos=%d", seen.Load(), called.Load())
				}
				want := "plugin_hook_blocked"
				if tc.dispatch != 0 {
					want = "tool_denied"
				}
				if result.ErrorCode != want || result.Retryable || !result.RetryabilityKnown {
					t.Fatalf("resultado: %+v", result)
				}
			})
		})
	}
}
