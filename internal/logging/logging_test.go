package logging

import (
	"context"
	"log/slog"
	"testing"

	"assistente/internal/eventctx"
	"assistente/internal/toolctx"
	"assistente/internal/tools/invocationctx"
	"assistente/internal/userctx"
)

func TestContextAttrsIncludesCorrelationFields(t *testing.T) {
	ctx := userctx.WithUserID(context.Background(), "user-1")
	ctx = invocationctx.With(ctx, invocationctx.InvocationContext{
		ConversationID: "conv-1",
		TurnID:         "turn-1",
		ProfileSlug:    "profile-1",
		TabType:        "chat",
	})
	ctx = eventctx.With(ctx, eventctx.Provenance{
		Source:       "job",
		SourceJobID:  "job-1",
		ChainID:      "chain-1",
		ChainHistory: []string{"job-1", "job-2"},
	})
	ctx = toolctx.WithCurrentInvocationID(ctx, "inv-1")
	ctx = toolctx.WithParentInvocationID(ctx, "parent-inv-1")
	ctx = WithAttrs(ctx, slog.String("run_id", "run-1"))

	got := attrsByKey(ContextAttrs(ctx))

	assertAttr(t, got, "user_id", "user-1")
	assertAttr(t, got, "conversation_id", "conv-1")
	assertAttr(t, got, "turn_id", "turn-1")
	assertAttr(t, got, "profile_slug", "profile-1")
	assertAttr(t, got, "surface_type", "chat")
	assertAttr(t, got, "source", "job")
	assertAttr(t, got, "source_job_id", "job-1")
	assertAttr(t, got, "chain_id", "chain-1")
	assertAttr(t, got, "chain_depth", int64(2))
	assertAttr(t, got, "tool_invocation_id", "inv-1")
	assertAttr(t, got, "parent_tool_invocation_id", "parent-inv-1")
	assertAttr(t, got, "run_id", "run-1")
}

func TestWithAttrsKeepsExistingAttributes(t *testing.T) {
	ctx := WithAttrs(context.Background(), slog.String("job_id", "job-1"))
	ctx = WithAttrs(ctx, slog.String("run_id", "run-1"))

	got := attrsByKey(ContextAttrs(ctx))
	assertAttr(t, got, "job_id", "job-1")
	assertAttr(t, got, "run_id", "run-1")
}

func TestNormalizeLegacyMessageRemovesPrefixAndSymbols(t *testing.T) {
	got := normalizeLegacyMessage("[Updater] ✅ Atualização aplicada com sucesso")
	if got != "Atualização aplicada com sucesso" {
		t.Fatalf("normalizeLegacyMessage() = %q", got)
	}
}

func attrsByKey(attrs []slog.Attr) map[string]any {
	out := make(map[string]any, len(attrs))
	for _, attr := range attrs {
		out[attr.Key] = attr.Value.Any()
	}
	return out
}

func assertAttr(t *testing.T, attrs map[string]any, key string, want any) {
	t.Helper()
	if got, ok := attrs[key]; !ok || got != want {
		t.Fatalf("attr %q = %v, want %v (present=%v)", key, got, want, ok)
	}
}
