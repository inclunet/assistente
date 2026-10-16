package logging

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
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

func TestWithAttrScopeReplacesOnlyScopedAttributes(t *testing.T) {
	base := WithAttrs(context.Background(),
		slog.String("job_id", "parent-job"),
		slog.String("run_id", "parent-run"),
		slog.String("trigger_type", "manual"),
		slog.String("trigger_event", "parent.done"),
		slog.String("trigger_chain_id", "parent-chain"),
		slog.String("request_id", "request-1"),
	)

	child := WithAttrScope(base,
		[]string{"job_id", "run_id", "trigger_type", "trigger_event", "trigger_chain_id"},
		slog.String("job_id", "child-job"),
		slog.String("run_id", "child-run"),
		slog.String("trigger_type", "event"),
		slog.String("trigger_event", "parent.done"),
	)

	got := ContextAttrs(child)
	assertSingleAttr(t, got, "job_id", "child-job")
	assertSingleAttr(t, got, "run_id", "child-run")
	assertSingleAttr(t, got, "trigger_type", "event")
	assertSingleAttr(t, got, "trigger_event", "parent.done")
	assertAttr(t, attrsByKey(got), "request_id", "request-1")
	if countAttrs(got, "trigger_chain_id") != 0 {
		t.Fatal("trigger_chain_id herdado deveria ter sido removido")
	}

	baseAttrs := attrsByKey(ContextAttrs(base))
	assertAttr(t, baseAttrs, "job_id", "parent-job")
	assertAttr(t, baseAttrs, "run_id", "parent-run")
}

func TestWithAttrScopeSupportsConcurrentDerivedContexts(t *testing.T) {
	base := WithAttrs(context.Background(),
		slog.String("job_id", "parent-job"),
		slog.String("run_id", "parent-run"),
		slog.String("request_id", "request-1"),
	)

	const workers = 32
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		i := i
		go func() {
			defer wg.Done()
			id := strconv.Itoa(i)
			ctx := WithAttrScope(base,
				[]string{"job_id", "run_id"},
				slog.String("job_id", "job-"+id),
				slog.String("run_id", "run-"+id),
			)
			got := ContextAttrs(ctx)
			assertSingleAttr(t, got, "job_id", "job-"+id)
			assertSingleAttr(t, got, "run_id", "run-"+id)
			assertSingleAttr(t, got, "request_id", "request-1")
		}()
	}
	wg.Wait()

	got := ContextAttrs(base)
	assertSingleAttr(t, got, "job_id", "parent-job")
	assertSingleAttr(t, got, "run_id", "parent-run")
}

func TestNormalizeLegacyMessageRemovesPrefixAndSymbols(t *testing.T) {
	cases := map[string]string{
		"[Updater] ✅ Atualização aplicada com sucesso":       "Atualização aplicada com sucesso",
		"[RunCommand] Comando: git status, decisão: approve": "Comando: git status, decisão: approve",
	}
	for input, want := range cases {
		if got := normalizeLegacyMessage(input); got != want {
			t.Errorf("normalizeLegacyMessage(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeLegacyMessagePreservesDiagnosticTags(t *testing.T) {
	cases := map[string]string{
		"[MCP-DEGRADE] attempt=1":            "[MCP-DEGRADE] attempt=1",
		"[MCP Native] event persisted":       "[MCP Native] event persisted",
		"🔴 [PANIC RECOVERED] handler failed": "[PANIC RECOVERED] handler failed",
	}

	for input, want := range cases {
		if got := normalizeLegacyMessage(input); got != want {
			t.Fatalf("normalizeLegacyMessage(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestLogfSkipsFormattingWhenLevelDisabled(t *testing.T) {
	defaultLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(discardWriter{}, &slog.HandlerOptions{Level: slog.LevelInfo})))
	defer slog.SetDefault(defaultLogger)

	Debugf(context.Background(), "logging.test", "disabled %s", panicStringer{})
}

type panicStringer struct{}

func (panicStringer) String() string {
	panic("disabled log should not format arguments")
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

func attrsByKey(attrs []slog.Attr) map[string]any {
	out := make(map[string]any, len(attrs))
	for _, attr := range attrs {
		out[attr.Key] = attr.Value.Any()
	}
	return out
}

func countAttrs(attrs []slog.Attr, key string) int {
	count := 0
	for _, attr := range attrs {
		if attr.Key == key {
			count++
		}
	}
	return count
}

func assertSingleAttr(t *testing.T, attrs []slog.Attr, key string, want any) {
	t.Helper()
	if count := countAttrs(attrs, key); count != 1 {
		t.Fatalf("attr %q ocorre %d vezes, want 1", key, count)
	}
	assertAttr(t, attrsByKey(attrs), key, want)
}

func assertAttr(t *testing.T, attrs map[string]any, key string, want any) {
	t.Helper()
	if got, ok := attrs[key]; !ok || got != want {
		t.Fatalf("attr %q = %v, want %v (present=%v)", key, got, want, ok)
	}
}
