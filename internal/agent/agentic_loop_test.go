package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/events"
	"assistente/internal/llm"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
	"assistente/internal/tools/invocationctx"
)

// newSeamRunner cria um runner mínimo para testar seams isoladamente.
func newSeamRunner(svc *Service, conversationID, turnID string) *agenticLoopRunner {
	return &agenticLoopRunner{
		svc:            svc,
		conversationID: conversationID,
		turnID:         turnID,
		toolsUsedSet:   map[string]struct{}{},
	}
}

// --- Helpers puros ---

func TestNormalizeRecoveryMaxAttempts(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 3}, {-5, 3}, {1, 1}, {2, 2}, {10, 10},
	}
	for _, c := range cases {
		if got := normalizeRecoveryMaxAttempts(c.in); got != c.want {
			t.Errorf("normalizeRecoveryMaxAttempts(%d)=%d, want %d", c.in, got, c.want)
		}
	}
}

func TestResolveAgenticMaxIterations(t *testing.T) {
	// params positivo: não consulta o executor (pode ser nil sem panic).
	if got := resolveAgenticMaxIterations(llm.ChatParams{MaxAgenticIterations: 7}, nil); got != 7 {
		t.Fatalf("params positivo: got %d, want 7", got)
	}
	// params <= 0: cai no config do executor.
	exec := tools.NewExecutor(tools.NewRegistry(), tools.DefaultExecutorConfig())
	want := exec.Config().MaxIterations
	if got := resolveAgenticMaxIterations(llm.ChatParams{}, exec); got != want {
		t.Fatalf("fallback executor: got %d, want %d", got, want)
	}
}

func TestSortedToolNames(t *testing.T) {
	got := sortedToolNames(map[string]struct{}{"b": {}, "a": {}, "c": {}})
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("sortedToolNames=%v", got)
	}
	empty := sortedToolNames(map[string]struct{}{})
	if empty == nil {
		t.Fatal("esperava slice não-nil para set vazio")
	}
	if len(empty) != 0 {
		t.Fatalf("esperava slice vazia, got %#v", empty)
	}
}

func TestBuildEnrichedToolCalls(t *testing.T) {
	calls := []llm.ToolCall{
		{ID: "c1", Type: "function", Function: llm.FunctionCall{Name: "read_file", Arguments: `{"p":1}`}},
		{ID: "c2", Type: "function", Function: llm.FunctionCall{Name: "mcp_github__search", Arguments: `{}`}},
	}
	execs := []tools.ToolExecutionResult{
		{CallID: "c1", DurationMs: 11},
		{CallID: "c2", DurationMs: 22},
	}
	got := buildEnrichedToolCalls(calls, execs, 3)
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}
	if got[0].Function.Name != "read_file" || got[0].Origin != OriginBuiltin || got[0].Iteration != 3 || got[0].DurationMs != 11 {
		t.Fatalf("builtin enriched inesperado: %#v", got[0])
	}
	if got[1].Function.Name != "search" || got[1].Origin != OriginMCPBridge || got[1].ServerLabel != "github" || got[1].DurationMs != 22 {
		t.Fatalf("bridge enriched inesperado: %#v", got[1])
	}
}

func TestBuildEnrichedToolCalls_MoreCallsThanExecs(t *testing.T) {
	calls := []llm.ToolCall{
		{ID: "c1", Function: llm.FunctionCall{Name: "read_file"}},
		{ID: "c2", Function: llm.FunctionCall{Name: "grep_search"}},
	}
	// Apenas uma execução — a segunda call não deve estourar índice.
	got := buildEnrichedToolCalls(calls, []tools.ToolExecutionResult{{CallID: "c1", DurationMs: 9}}, 0)
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}
	if got[0].DurationMs != 9 || got[1].DurationMs != 0 {
		t.Fatalf("durations inesperadas: %#v", got)
	}
}

func TestBuildAgenticInvocationContext(t *testing.T) {
	params := llm.ChatParams{TabType: "chat", ActiveFilePath: "a.go", ProfileSlug: "dev"}
	ctx := buildAgenticInvocationContext(context.Background(), params, "conv", "turn")
	ic, ok := invocationctx.Get(ctx)
	if !ok {
		t.Fatal("invocation context não propagado")
	}
	if ic.ConversationID != "conv" || ic.TurnID != "turn" || ic.ProfileSlug != "dev" || ic.TabType != "chat" || ic.ActiveFilePath != "a.go" {
		t.Fatalf("invocation context inesperado: %+v", ic)
	}
}

// --- Seams (métodos do runner) ---

func TestAgenticLoopRunner_TurnStillValid(t *testing.T) {
	// repo nil → segue o fluxo (não há como validar).
	svcNil := NewService(ServiceConfig{Emitter: events.NoopEmitter{}})
	if !newSeamRunner(svcNil, "c1", "t1").turnStillValid(context.Background()) {
		t.Fatal("repo nil deveria seguir o fluxo")
	}

	// conversa correspondente → válido.
	svcMatch := NewService(ServiceConfig{Emitter: events.NoopEmitter{}, MsgRepo: msgRepoStub{conversationID: "c1"}})
	if !newSeamRunner(svcMatch, "c1", "t1").turnStillValid(context.Background()) {
		t.Fatal("conversa correspondente deveria ser válida")
	}

	// conversa divergente → inválido (aborta).
	svcMismatch := NewService(ServiceConfig{Emitter: events.NoopEmitter{}, MsgRepo: msgRepoStub{conversationID: "outra"}})
	if newSeamRunner(svcMismatch, "c1", "t1").turnStillValid(context.Background()) {
		t.Fatal("conversa divergente deveria abortar")
	}

	// erro ao buscar a mensagem do turno → inválido (aborta).
	repoErr := &inMemoryMsgRepo{rejectCanceledCtx: true}
	svcErr := NewService(ServiceConfig{Emitter: events.NoopEmitter{}, MsgRepo: repoErr})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if newSeamRunner(svcErr, "c1", "t1").turnStillValid(ctx) {
		t.Fatal("erro no repo deveria abortar")
	}
}

func TestAgenticLoopRunner_EmitToolEndsAndAccount(t *testing.T) {
	em := &captureEmitter{}
	svc := NewService(ServiceConfig{Emitter: em, MsgRepo: &mockMsgRepo{}})
	r := newSeamRunner(svc, "c1", "t1")

	execResults := []tools.ToolExecutionResult{
		{CallID: "a", ToolName: "read_file", Result: tools.ToolResult{Content: "ok"}},
		{CallID: "b", ToolName: "mcp_github__search", Result: tools.ToolResult{Content: "boom", IsError: true}, ErrorKind: tools.ErrorKindUnknown, DurationMs: 5},
	}
	retried := map[string]struct{}{"b": {}}

	summaries := r.emitToolEndsAndAccount(execResults, retried)

	ends := em.find("chat:tool_end")
	if len(ends) != 2 {
		t.Fatalf("tool_end=%d, want 2", len(ends))
	}
	e0 := ends[0].data.(ports.ToolEndEvent)
	if e0.Name != "read_file" || e0.Status != "ok" || e0.Origin != OriginBuiltin || e0.Attempt != 0 {
		t.Fatalf("tool_end[0] inesperado: %+v", e0)
	}
	e1 := ends[1].data.(ports.ToolEndEvent)
	if e1.Name != "search" || e1.Status != "error" || e1.Origin != OriginMCPBridge || e1.ServerLabel != "github" || e1.Attempt != 1 {
		t.Fatalf("tool_end[1] inesperado: %+v", e1)
	}

	fails := em.find("chat:tool_failure")
	if len(fails) != 1 {
		t.Fatalf("tool_failure=%d, want 1", len(fails))
	}
	f := fails[0].data.(ports.ToolFailureEvent)
	if f.Name != "search" || f.ErrorKind != "unknown" {
		t.Fatalf("tool_failure inesperado: %+v", f)
	}

	if r.totalToolCallCount != 2 {
		t.Fatalf("totalToolCallCount=%d, want 2", r.totalToolCallCount)
	}
	if _, ok := r.toolsUsedSet["read_file"]; !ok {
		t.Fatal("read_file deveria constar em toolsUsedSet")
	}
	if _, ok := r.toolsUsedSet["search"]; !ok {
		t.Fatal("search deveria constar em toolsUsedSet")
	}
	if len(summaries) != 2 {
		t.Fatalf("summaries=%d, want 2", len(summaries))
	}
}

func TestAgenticLoopRunner_RetryRetryableTools(t *testing.T) {
	em := &captureEmitter{}
	reg := tools.NewRegistry()
	reg.MustRegister(okTool{})
	exec := tools.NewExecutor(reg, tools.DefaultExecutorConfig())
	svc := NewService(ServiceConfig{Emitter: em, ToolExecutor: exec})
	r := newSeamRunner(svc, "c1", "t1")
	r.maxIterations = 3

	toolCalls := []tools.ToolCall{
		{ID: "x", Type: "function", Function: tools.FunctionCall{Name: "ok_tool", Arguments: `{}`}},
	}
	execResults := []tools.ToolExecutionResult{
		{CallID: "x", ToolName: "ok_tool", Result: tools.ToolResult{Content: "boom", IsError: true}, Retryable: true, ErrorKind: tools.ErrorKindTimeout, DurationMs: 1},
	}
	persisted := map[string]bool{}

	retried := r.retryRetryableTools(context.Background(), toolCalls, execResults, persisted, 0)

	if _, ok := retried["x"]; !ok {
		t.Fatal("call x deveria ter sido re-tentada")
	}
	if execResults[0].Result.IsError {
		t.Fatalf("retry deveria substituir o resultado por sucesso: %#v", execResults[0])
	}
	// Sequência de eventos: tool_end(falha) → tool_failure(willRetry) → tool_start(attempt=1).
	if got := len(em.find("chat:tool_end")); got != 1 {
		t.Fatalf("tool_end=%d, want 1", got)
	}
	ff := em.find("chat:tool_failure")
	if len(ff) != 1 || !ff[0].data.(ports.ToolFailureEvent).WillRetry {
		t.Fatalf("tool_failure willRetry inesperado: %#v", ff)
	}
	ts := em.find("chat:tool_start")
	if len(ts) != 1 || ts[0].data.(ports.ToolStartEvent).Attempt != 1 {
		t.Fatalf("tool_start retry inesperado: %#v", ts)
	}
}

func TestAgenticLoopRunner_RetryRetryableTools_LastIterationNoRetry(t *testing.T) {
	em := &captureEmitter{}
	reg := tools.NewRegistry()
	reg.MustRegister(okTool{})
	exec := tools.NewExecutor(reg, tools.DefaultExecutorConfig())
	svc := NewService(ServiceConfig{Emitter: em, ToolExecutor: exec})
	r := newSeamRunner(svc, "c1", "t1")
	r.maxIterations = 1 // iteração 0 é a última → sem retry.

	toolCalls := []tools.ToolCall{{ID: "x", Function: tools.FunctionCall{Name: "ok_tool", Arguments: `{}`}}}
	execResults := []tools.ToolExecutionResult{
		{CallID: "x", ToolName: "ok_tool", Result: tools.ToolResult{Content: "boom", IsError: true}, Retryable: true, ErrorKind: tools.ErrorKindTimeout},
	}
	retried := r.retryRetryableTools(context.Background(), toolCalls, execResults, map[string]bool{}, 0)

	if len(retried) != 0 {
		t.Fatalf("não deveria re-tentar na última iteração: %#v", retried)
	}
	if !execResults[0].Result.IsError {
		t.Fatal("resultado não deveria ser substituído sem retry")
	}
	if len(em.find("chat:tool_start")) != 0 {
		t.Fatal("não deveria emitir tool_start de retry na última iteração")
	}
}

func TestAgenticLoopRunner_PersistAndAccountNativeMCP(t *testing.T) {
	em := &captureEmitter{}
	msgRepo := &capturingMsgRepo{conversationID: "c1"}
	svc := NewService(ServiceConfig{Emitter: em, MsgRepo: msgRepo})
	r := newSeamRunner(svc, "c1", "t1")
	ctx := database.WithUserID(context.Background(), "u1")

	summaries := r.persistAndAccountNativeMCP(ctx, AgenticResult{NativeMCPEvents: []llm.MCPToolEvent{
		{ID: "n1", Name: "ping", ServerLabel: "Srv", Output: "ok", IsCompleted: true},
		{ID: "n2", Name: "boom", ServerLabel: "Srv", Error: "e", IsCompleted: true},
		{ID: "n3", Name: "pending", IsCompleted: false},
	}}, 0)

	if len(summaries) != 2 {
		t.Fatalf("summaries=%d, want 2 (só completas)", len(summaries))
	}
	if r.totalToolCallCount != 2 {
		t.Fatalf("totalToolCallCount=%d, want 2", r.totalToolCallCount)
	}
	if summaries[0].Status != "ok" || summaries[0].Origin != OriginMCPNative {
		t.Fatalf("summary[0] inesperado: %+v", summaries[0])
	}
	if summaries[1].Status != "error" {
		t.Fatalf("summary[1] inesperado: %+v", summaries[1])
	}
}

func TestAgenticLoopRunner_BuildErrorDoneEvent(t *testing.T) {
	svc := NewService(ServiceConfig{Emitter: events.NoopEmitter{}})
	r := newSeamRunner(svc, "c1", "t1")
	r.assistantMessageID = "am"
	r.surfaceOrigin = nil
	r.totalToolCallCount = 2
	r.toolsUsedSet = map[string]struct{}{"b": {}, "a": {}}
	r.lastUsage = llm.Usage{PromptTokens: 100, CompletionTokens: 20, CacheReadTokens: 5, CacheWriteTokens: 3, CacheMissTokens: 7}

	ev := r.buildErrorDoneEvent("boom", 1)
	if ev.Reason != "error" || ev.ErrorMessage != "boom" {
		t.Fatalf("reason/error inesperados: %+v", ev)
	}
	if ev.AssistantMessageID != "am" {
		t.Fatalf("assistantMessageID=%q", ev.AssistantMessageID)
	}
	if ev.IterationCount != 2 || ev.ToolCallCount != 2 || !ev.HadToolCalls {
		t.Fatalf("contadores inesperados: %+v", ev)
	}
	if len(ev.ToolsUsed) != 2 || ev.ToolsUsed[0] != "a" || ev.ToolsUsed[1] != "b" {
		t.Fatalf("toolsUsed não ordenado: %v", ev.ToolsUsed)
	}
	if ev.PromptTokens != 100 || ev.CompletionTokens != 20 || ev.CacheReadTokens != 5 || ev.CacheWriteTokens != 3 || ev.CacheMissTokens != 7 {
		t.Fatalf("tokens inesperados: %+v", ev)
	}
}

func TestAgenticLoopRunner_FinishLimitReached(t *testing.T) {
	em := &captureEmitter{}
	svc := NewService(ServiceConfig{Emitter: em, MsgRepo: &mockMsgRepo{}})
	r := newSeamRunner(svc, "c1", "t1")
	r.maxIterations = 4
	r.totalToolCallCount = 1
	r.toolsUsedSet = map[string]struct{}{"x": {}}

	r.finishLimitReached(context.Background())

	streamEvents := em.find("chat:stream")
	if len(streamEvents) != 1 {
		t.Fatalf("chat:stream=%d, want 1", len(streamEvents))
	}
	if !streamEvents[0].data.(events.StreamEvent).Done {
		t.Fatal("chat:stream de limite deveria ser terminal (Done=true)")
	}
	done := em.find("chat:done")
	if len(done) != 1 {
		t.Fatalf("chat:done=%d, want 1", len(done))
	}
	d := done[0].data.(ports.DoneEvent)
	if d.Reason != "limit_reached" || d.IterationCount != 4 || d.ToolCallCount != 1 {
		t.Fatalf("chat:done de limite inesperado: %+v", d)
	}
}

// --- Cenários de loop completo ---

// multiIterToolStreamer emite tool_calls em toolIters iterações e depois conclui.
type multiIterToolStreamer struct {
	toolIters int
	calls     int
}

func (s *multiIterToolStreamer) StreamChat(_ context.Context, _ []llm.Message, _ llm.ChatParams, handler llm.StreamHandler, _ ...llm.ToolDefinition) {
	if s.calls < s.toolIters {
		id := fmt.Sprintf("call-%d", s.calls)
		handler.OnToolCalls([]llm.ToolCall{{ID: id, Type: "function", Function: llm.FunctionCall{Name: "ok_tool", Arguments: `{}`}}}, "", llm.Usage{}, "m")
		s.calls++
		return
	}
	handler.OnDone("final", llm.Usage{}, "m")
	s.calls++
}

func TestRunAgenticLoop_MultipleToolIterations_EmitsEventsInOrder(t *testing.T) {
	db, cleanup := setupAgenticToolCallDB(t)
	t.Cleanup(cleanup)

	ctx := database.WithUserID(context.Background(), "user-1")
	conv, err := database.CreateConversationWithContext(ctx, "t", "")
	if err != nil {
		t.Fatalf("create conv: %v", err)
	}
	turn, err := database.AddMessageWithContext(ctx, conv.ID, "user", "hi")
	if err != nil {
		t.Fatalf("create turn: %v", err)
	}
	if err := db.Create(&database.ToolCatalog{
		Name:               "ok_tool",
		DisplayName:        "ok_tool",
		Origin:             tools.ToolOriginBuiltin,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
	}).Error; err != nil {
		t.Fatalf("seed catalog: %v", err)
	}

	repo := toolinvocations.NewDBRepository(db)
	reg := tools.NewRegistry()
	reg.MustRegister(okTool{})
	exec := tools.NewExecutor(reg, tools.DefaultExecutorConfig())
	inv := toolinvocations.NewService(repo, exec)

	em := &captureEmitter{}
	msgRepo := &toolMsgRepo{conversationID: conv.ID}
	svc := NewService(ServiceConfig{Emitter: em, MsgRepo: msgRepo, ToolExecutor: exec, ToolInvocations: inv})

	streamer := &multiIterToolStreamer{toolIters: 2}
	svc.RunAgenticLoop(ctx, []llm.Message{{Role: "user", Content: "hi"}}, llm.ChatParams{MaxAgenticIterations: 5}, conv.ID, turn.ID, nil, streamer, nil, func(string, int) IterationHandler {
		return &testIterationHandler{}
	}, nil, false, 0)

	if got := len(em.find("chat:tool_start")); got != 2 {
		t.Fatalf("tool_start=%d, want 2", got)
	}
	if got := len(em.find("chat:tool_end")); got != 2 {
		t.Fatalf("tool_end=%d, want 2", got)
	}

	seg := em.find("chat:segment_done")
	if len(seg) != 3 {
		t.Fatalf("segment_done=%d, want 3", len(seg))
	}
	hasMore := 0
	for _, e := range seg {
		if e.data.(ports.SegmentDoneEvent).HasMore {
			hasMore++
		}
	}
	if hasMore != 2 {
		t.Fatalf("segment_done HasMore=%d, want 2", hasMore)
	}

	done := em.find("chat:done")
	if len(done) != 1 {
		t.Fatalf("chat:done=%d, want 1", len(done))
	}
	d := done[0].data.(ports.DoneEvent)
	if d.Reason != "completed" {
		t.Fatalf("reason=%q, want completed", d.Reason)
	}
	if d.IterationCount != 3 {
		t.Fatalf("IterationCount=%d, want 3", d.IterationCount)
	}
	if d.ToolCallCount != 2 {
		t.Fatalf("ToolCallCount=%d, want 2", d.ToolCallCount)
	}
	if len(d.ToolsUsed) != 1 || d.ToolsUsed[0] != "ok_tool" {
		t.Fatalf("ToolsUsed=%v", d.ToolsUsed)
	}

	// Trava a ordem/semântica dos eventos relevantes do protocolo (AEP-0040).
	var seq []string
	for _, e := range em.events {
		switch e.name {
		case "chat:tool_start", "chat:tool_end", "chat:segment_done", "chat:done":
			seq = append(seq, e.name)
		}
	}
	want := []string{
		"chat:tool_start", "chat:tool_end", "chat:segment_done",
		"chat:tool_start", "chat:tool_end", "chat:segment_done",
		"chat:segment_done", "chat:done",
	}
	if len(seq) != len(want) {
		t.Fatalf("sequência de eventos=%v, want %v", seq, want)
	}
	for i := range want {
		if seq[i] != want[i] {
			t.Fatalf("sequência de eventos[%d]=%q, want %q (seq=%v)", i, seq[i], want[i], seq)
		}
	}
}

// errTool retorna sempre um resultado de erro não classificado (sem ErrorKind).
type errTool struct{}

func (errTool) Name() string                { return "err_tool" }
func (errTool) Description() string         { return "always fails" }
func (errTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (errTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{Content: "falhou", IsError: true}, nil
}

// twoToolsThenDoneStreamer emite duas tool calls (uma ok, uma com erro) na primeira
// iteração e conclui na segunda.
type twoToolsThenDoneStreamer struct{ calls int }

func (s *twoToolsThenDoneStreamer) StreamChat(_ context.Context, _ []llm.Message, _ llm.ChatParams, handler llm.StreamHandler, _ ...llm.ToolDefinition) {
	if s.calls == 0 {
		handler.OnToolCalls([]llm.ToolCall{
			{ID: "ok-1", Type: "function", Function: llm.FunctionCall{Name: "ok_tool", Arguments: `{}`}},
			{ID: "err-1", Type: "function", Function: llm.FunctionCall{Name: "err_tool", Arguments: `{}`}},
		}, "", llm.Usage{}, "m")
		s.calls++
		return
	}
	handler.OnDone("final", llm.Usage{}, "m")
	s.calls++
}

func TestRunAgenticLoop_PartialToolError_ContinuesAndCompletes(t *testing.T) {
	db, cleanup := setupAgenticToolCallDB(t)
	t.Cleanup(cleanup)

	ctx := database.WithUserID(context.Background(), "user-1")
	conv, err := database.CreateConversationWithContext(ctx, "t", "")
	if err != nil {
		t.Fatalf("create conv: %v", err)
	}
	turn, err := database.AddMessageWithContext(ctx, conv.ID, "user", "hi")
	if err != nil {
		t.Fatalf("create turn: %v", err)
	}
	for _, name := range []string{"ok_tool", "err_tool"} {
		if err := db.Create(&database.ToolCatalog{
			Name:               name,
			DisplayName:        name,
			Origin:             tools.ToolOriginBuiltin,
			AvailabilityStatus: tools.ToolAvailabilityAvailable,
		}).Error; err != nil {
			t.Fatalf("seed catalog %s: %v", name, err)
		}
	}

	repo := toolinvocations.NewDBRepository(db)
	reg := tools.NewRegistry()
	reg.MustRegister(okTool{})
	reg.MustRegister(errTool{})
	exec := tools.NewExecutor(reg, tools.DefaultExecutorConfig())
	inv := toolinvocations.NewService(repo, exec)

	em := &captureEmitter{}
	msgRepo := &toolMsgRepo{conversationID: conv.ID}
	svc := NewService(ServiceConfig{Emitter: em, MsgRepo: msgRepo, ToolExecutor: exec, ToolInvocations: inv})

	streamer := &twoToolsThenDoneStreamer{}
	svc.RunAgenticLoop(ctx, []llm.Message{{Role: "user", Content: "hi"}}, llm.ChatParams{MaxAgenticIterations: 5}, conv.ID, turn.ID, nil, streamer, nil, func(string, int) IterationHandler {
		return &testIterationHandler{}
	}, nil, false, 0)

	ends := em.find("chat:tool_end")
	if len(ends) != 2 {
		t.Fatalf("tool_end=%d, want 2", len(ends))
	}
	statusByName := map[string]string{}
	for _, e := range ends {
		ev := e.data.(ports.ToolEndEvent)
		statusByName[ev.Name] = ev.Status
	}
	if statusByName["ok_tool"] != "ok" {
		t.Fatalf("ok_tool status=%q, want ok", statusByName["ok_tool"])
	}
	if statusByName["err_tool"] != "error" {
		t.Fatalf("err_tool status=%q, want error", statusByName["err_tool"])
	}

	done := em.find("chat:done")
	if len(done) != 1 {
		t.Fatalf("chat:done=%d, want 1", len(done))
	}
	d := done[0].data.(ports.DoneEvent)
	if d.Reason != "completed" {
		t.Fatalf("reason=%q, want completed (erro parcial não deve abortar)", d.Reason)
	}
	if d.ToolCallCount != 2 {
		t.Fatalf("ToolCallCount=%d, want 2", d.ToolCallCount)
	}
}
