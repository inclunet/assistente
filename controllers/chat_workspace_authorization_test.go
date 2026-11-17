package controllers

import (
	"context"
	"testing"

	"assistente/internal/core/usecases"
	"assistente/internal/llm"
	"assistente/internal/tools/invocationctx"
)

type workspaceCaptureExecutor struct{ requests []usecases.SendMessageRequest }

func (s *workspaceCaptureExecutor) Execute(req usecases.SendMessageRequest) (string, error) {
	s.requests = append(s.requests, req)
	return "message", nil
}

func TestChatCapturaWorkspaceAntesDeEnfileirar(t *testing.T) {
	active := "workspace-a"
	executor := &workspaceCaptureExecutor{}
	c := &ChatController{sendMsgUC: executor, workspacePath: func() string { return active }}
	if _, err := c.SendMessage(context.Background(), "conversation", "message", "", llm.ChatParams{}); err != nil {
		t.Fatal(err)
	}
	active = "workspace-b"
	if root, ok := invocationctx.Workspace(executor.requests[0].Ctx); !ok || root != "workspace-a" {
		t.Fatalf("origem mudou: %q, %v", root, ok)
	}
	if _, err := c.RetryMessage(context.Background(), "conversation", "message", llm.ChatParams{}); err != nil {
		t.Fatal(err)
	}
	if root, ok := invocationctx.Workspace(executor.requests[1].Ctx); !ok || root != "workspace-b" {
		t.Fatalf("retry: %q, %v", root, ok)
	}
	if _, err := c.SendMessageFromChannel(context.Background(), "conversation", "message", "", llm.ChatParams{}, "telegram"); err != nil {
		t.Fatal(err)
	}
	if root, ok := invocationctx.Workspace(executor.requests[2].Ctx); !ok || root != "" {
		t.Fatalf("canal herdou workspace desktop: %q, %v", root, ok)
	}
	for _, parent := range []int{0, 2} {
		ctx := context.WithoutCancel(executor.requests[parent].Ctx)
		if _, err := c.SendForSubagent(ctx, "child", "task", "", "", ""); err != nil {
			t.Fatal(err)
		}
		want, _ := invocationctx.Workspace(ctx)
		last := executor.requests[len(executor.requests)-1]
		if root, ok := invocationctx.Workspace(last.Ctx); !ok || root != want {
			t.Fatalf("subagente perdeu origem %q: %q %v", want, root, ok)
		}
	}
}
