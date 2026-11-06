package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/chat"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
)

func TestExecutionWriterToolFirstTerminalUsesExplicitExecutionMode(t *testing.T) {
	for _, tc := range []struct{ mode, reason, base string }{
		{database.TurnModeRetry, "completed", "base"}, {database.TurnModeRetry, "error", "base"},
		{database.TurnModeRetry, "cancelled", "base"},
		{database.TurnModeContinue, "completed", "base"}, {database.TurnModeContinue, "error", "base"},
		{database.TurnModeContinue, "cancelled", "base"}, {database.TurnModeContinue, "output_limit", "base"},
		{database.TurnModeContinue, "error", ""}, {database.TurnModeContinue, "cancelled", ""},
	} {
		t.Run(tc.mode+"/"+tc.reason+"/"+tc.base, func(t *testing.T) {
			svc, ctx, conv, turn, em := setupTurnRuntime(t)
			original, err := svc.EnsureTurnResponse(ctx, conv, turn)
			if err != nil {
				t.Fatal(err)
			}
			svc.toolInvocations = toolinvocations.NewService(toolinvocations.NewDBRepository(database.DB()), nil)
			if err := svc.persistNativeMCPCalls(ctx, conv, turn, []llm.MCPToolEvent{{ID: "old-call", Name: "old-tool", ServerLabel: "fixture", IsCompleted: true}}, 0); err != nil {
				t.Fatal(err)
			}
			svc.tagChatToolInvocationsWithAssistantMessage(ctx, conv, turn, []tools.ToolExecutionResult{{CallID: "old-call"}}, original)
			baseReasoning := ""
			if tc.base != "" {
				baseReasoning = "pensamento-base"
			}
			if !svc.SaveAndFinish(ctx, conv, turn, original, AgenticResult{FullResponse: tc.base, Reasoning: baseReasoning}, "", nil, nil) {
				t.Fatal("base não persistida")
			}
			svc.EndUnfinishedTurn(ctx, conv, turn, nil)
			ctx, err = svc.BeginTurn(ctx, conv, turn, tc.mode)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			t.Cleanup(func() { svc.EndUnfinishedTurn(ctx, conv, turn, nil) })
			if err := database.DB().Create(&database.ToolCatalog{Name: "ok_tool", DisplayName: "ok_tool", Origin: tools.ToolOriginBuiltin, AvailabilityStatus: tools.ToolAvailabilityAvailable}).Error; err != nil {
				t.Fatal(err)
			}
			registry := tools.NewRegistry()
			registry.MustRegister(okTool{})
			svc.toolInvocations = toolinvocations.NewService(toolinvocations.NewDBRepository(database.DB()), tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
			streamer := &recoveryStreamer{steps: []func(llm.StreamHandler){
				func(h llm.StreamHandler) {
					h.OnToolCalls([]llm.ToolCall{{ID: "new-call", Type: "function", Function: llm.FunctionCall{Name: "ok_tool", Arguments: `{}`}}}, "", llm.Usage{}, "modelo")
				},
				func(h llm.StreamHandler) {
					stored, err := chat.NewDBTimelineReader(database.DB()).ReadTurn(ctx, conv, turn)
					if err != nil {
						t.Fatal(err)
					}
					storedText, storedReasoning := "", ""
					for _, segment := range stored.Response.Segments {
						if segment.Kind == "text" {
							storedText += segment.Text
						}
						if segment.Kind == "reasoning" {
							storedReasoning += segment.Text
						}
					}
					if tc.mode == database.TurnModeContinue && (storedText != tc.base || storedReasoning != baseReasoning) {
						t.Fatalf("fontes inconsistentes entre iterações: text=%q reasoning=%q", storedText, storedReasoning)
					}
					switch tc.reason {
					case "error":
						h.OnError("erro do provedor")
					case "cancelled":
						cancel()
						h.OnError("cancelado")
					case "output_limit":
						llm.ReportFinishReason(h, llm.FinishInfo{Reason: llm.FinishReasonMaxTokens})
						h.OnDone("", llm.Usage{}, "modelo")
					default:
						h.OnDone("", llm.Usage{}, "modelo")
					}
				},
			}}
			svc.RunAgenticLoop(ctx, nil, llm.ChatParams{MaxAgenticIterations: 3, ContinueViaUserMessage: tc.mode == database.TurnModeContinue}, conv, turn, nil, streamer, nil,
				func(c string, i int) IterationHandler { return NewAgenticStreamHandler(em, c, i, nil, turn) }, nil, false, 1)
			svc.EndUnfinishedTurn(ctx, conv, turn, nil)
			window, err := svc.projection.ReadCanonicalWindow(context.WithoutCancel(ctx), database.MessageWindowQuery{ConversationID: conv, Anchor: "end", Direction: "before", Limit: 20}, chat.NewDBTimelineReader(database.DB()))
			if err != nil {
				t.Fatal(err)
			}
			response := window.Responses[turn]
			if response == nil {
				t.Fatal("resposta terminal ausente")
			}
			text, reasoning, calls := "", "", []string{}
			for _, segment := range response.Segments {
				if segment.Kind == "text" {
					text += segment.Text
				}
				if segment.Kind == "reasoning" {
					reasoning += segment.Text
				}
				for _, invocation := range segment.Invocations {
					calls = append(calls, invocation.ID)
				}
			}
			wantText, wantCalls := "", []string{"new-call"}
			if tc.mode == database.TurnModeContinue {
				wantText, wantCalls = tc.base, []string{"old-call", "new-call"}
			}
			if text != wantText || strings.Join(calls, ",") != strings.Join(wantCalls, ",") {
				t.Fatalf("modo/diagnóstico misturou fontes: text=%q calls=%v", text, calls)
			}
			wantReasoning := ""
			if tc.mode == database.TurnModeContinue {
				wantReasoning = baseReasoning
			}
			if reasoning != wantReasoning {
				t.Fatalf("reasoning duplicado/perdido: %q", reasoning)
			}
			reopened, err := chat.NewDBTimelineReader(database.DB()).ReadTurn(context.WithoutCancel(ctx), conv, turn)
			if err != nil {
				t.Fatal(err)
			}
			reopenedText, reopenedReasoning := "", ""
			for _, segment := range reopened.Response.Segments {
				if segment.Kind == "text" {
					reopenedText += segment.Text
				}
				if segment.Kind == "reasoning" {
					reopenedReasoning += segment.Text
				}
			}
			if reopenedText != wantText || reopenedReasoning != wantReasoning {
				t.Fatalf("reabertura divergente: text=%q reasoning=%q", reopenedText, reopenedReasoning)
			}
			wantState := database.TurnCompleted
			if tc.reason == "error" {
				wantState = database.TurnFailed
			} else if tc.reason != "completed" {
				wantState = database.TurnInterrupted
			}
			if response.State != wantState {
				t.Fatalf("terminal incorreto: %+v", response)
			}
		})
	}
}

func TestExecutionWriterChronologySurvivesTerminalAndPersistenceFailure(t *testing.T) {
	for _, mode := range []string{database.TurnModeRetry, database.TurnModeContinue} {
		for _, reason := range []string{"completed", "terminal_only", "error", "cancelled", "output_limit", "persistence_error", "terminal_only_persistence_error"} {
			t.Run(mode+"/"+reason, func(t *testing.T) {
				svc, ctx, conv, turn, em := setupTurnRuntime(t)
				original, err := svc.EnsureTurnResponse(ctx, conv, turn)
				if err != nil {
					t.Fatal(err)
				}
				if !svc.SaveAndFinish(ctx, conv, turn, original, AgenticResult{FullResponse: "base", Reasoning: "pensamento"}, "", nil, nil) {
					t.Fatal("base não persistida")
				}
				svc.EndUnfinishedTurn(ctx, conv, turn, nil)
				ctx, err = svc.BeginTurn(ctx, conv, turn, mode)
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithCancel(ctx)
				defer cancel()
				t.Cleanup(func() { svc.EndUnfinishedTurn(ctx, conv, turn, nil) })
				if err := database.DB().Create(&database.ToolCatalog{Name: "ok_tool", DisplayName: "ok_tool", Origin: tools.ToolOriginBuiltin, AvailabilityStatus: tools.ToolAvailabilityAvailable}).Error; err != nil {
					t.Fatal(err)
				}
				registry := tools.NewRegistry()
				registry.MustRegister(okTool{})
				svc.toolInvocations = toolinvocations.NewService(toolinvocations.NewDBRepository(database.DB()), tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
				streamer := &recoveryStreamer{steps: []func(llm.StreamHandler){
					func(h llm.StreamHandler) {
						h.OnChunk(" primeiro")
						h.OnToolCalls([]llm.ToolCall{{ID: "first", Type: "function", Function: llm.FunctionCall{Name: "ok_tool", Arguments: `{}`}}}, "", llm.Usage{}, "modelo")
					},
					func(h llm.StreamHandler) {
						h.OnToolCalls([]llm.ToolCall{{ID: "second", Type: "function", Function: llm.FunctionCall{Name: "ok_tool", Arguments: `{}`}}}, "", llm.Usage{}, "modelo")
					},
					func(h llm.StreamHandler) {
						if strings.Contains(reason, "persistence_error") {
							if err := database.DB().Exec(`CREATE TRIGGER reject_terminal_content BEFORE UPDATE ON chat_messages BEGIN SELECT RAISE(ABORT,'disco indisponível'); END`).Error; err != nil {
								t.Fatal(err)
							}
						}
						if !strings.HasPrefix(reason, "terminal_only") {
							h.OnChunk(" final")
						}
						switch reason {
						case "error":
							h.OnError("erro do provedor")
						case "cancelled":
							cancel()
							h.OnError("cancelado")
						case "output_limit":
							llm.ReportFinishReason(h, llm.FinishInfo{Reason: llm.FinishReasonMaxTokens})
							h.OnDone(" final", llm.Usage{}, "modelo")
						default:
							h.OnDone(" final", llm.Usage{}, "modelo")
						}
					},
				}}
				svc.RunAgenticLoop(ctx, nil, llm.ChatParams{MaxAgenticIterations: 4, ContinueViaUserMessage: mode == database.TurnModeContinue}, conv, turn, nil, streamer, nil,
					func(c string, i int) IterationHandler { return NewAgenticStreamHandler(em, c, i, nil, turn) }, nil, false, 1)
				svc.EndUnfinishedTurn(ctx, conv, turn, nil)
				window, err := svc.projection.ReadCanonicalWindow(context.WithoutCancel(ctx), database.MessageWindowQuery{ConversationID: conv, Anchor: "end", Direction: "before", Limit: 20}, chat.NewDBTimelineReader(database.DB()))
				if err != nil {
					t.Fatal(err)
				}
				response := window.Responses[turn]
				if response == nil {
					t.Fatal("resposta ausente")
				}
				text, reasoning, calls := "", "", []string{}
				chronology := []string{}
				for _, segment := range response.Segments {
					if segment.Kind == "text" {
						text += segment.Text
						chronology = append(chronology, "text:"+segment.Text)
					}
					if segment.Kind == "reasoning" {
						reasoning += segment.Text
					}
					for _, invocation := range segment.Invocations {
						calls = append(calls, invocation.ID)
						chronology = append(chronology, "tool:"+invocation.ID)
					}
				}
				wantText, wantReasoning := " primeiro final", ""
				if mode == database.TurnModeContinue {
					wantText, wantReasoning = "base"+wantText, "pensamento"
				}
				if text != wantText || reasoning != wantReasoning || strings.Join(calls, ",") != "first,second" {
					t.Fatalf("cronologia perdida: text=%q reasoning=%q calls=%v", text, reasoning, calls)
				}
				firstText := " primeiro"
				if mode == database.TurnModeContinue {
					firstText = "base" + firstText
				}
				wantChronology := []string{"text:" + firstText, "tool:first", "tool:second", "text: final"}
				if strings.Join(chronology, "|") != strings.Join(wantChronology, "|") {
					t.Fatalf("ordem intercalada incorreta: %v", chronology)
				}
				if strings.Contains(reason, "persistence_error") && (response.State != database.TurnFailed || response.Durability != "volatile") {
					t.Fatalf("falha não recuperável: %+v", response)
				}
			})
		}
	}
}

func TestExecutionWriterSecondContinuationKeepsExecutionChain(t *testing.T) {
	svc, ctx, conv, turn, em := setupTurnRuntime(t)
	original, err := svc.EnsureTurnResponse(ctx, conv, turn)
	if err != nil {
		t.Fatal(err)
	}
	svc.toolInvocations = toolinvocations.NewService(toolinvocations.NewDBRepository(database.DB()), nil)
	if err := svc.persistNativeMCPCalls(ctx, conv, turn, []llm.MCPToolEvent{{ID: "original", Name: "old-tool", ServerLabel: "fixture", IsCompleted: true}}, 0); err != nil {
		t.Fatal(err)
	}
	svc.tagChatToolInvocationsWithAssistantMessage(ctx, conv, turn, []tools.ToolExecutionResult{{CallID: "original"}}, original)
	if !svc.SaveAndFinish(ctx, conv, turn, original, AgenticResult{FullResponse: "base", Reasoning: "pensamento"}, "", nil, nil) {
		t.Fatal("base não persistida")
	}
	svc.EndUnfinishedTurn(ctx, conv, turn, nil)
	if err := database.DB().Create(&database.ToolCatalog{Name: "ok_tool", DisplayName: "ok_tool", Origin: tools.ToolOriginBuiltin, AvailabilityStatus: tools.ToolAvailabilityAvailable}).Error; err != nil {
		t.Fatal(err)
	}
	registry := tools.NewRegistry()
	registry.MustRegister(okTool{})
	svc.toolInvocations = toolinvocations.NewService(toolinvocations.NewDBRepository(database.DB()), tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
	wantText := "base"
	for _, suffix := range []string{" primeira", " segunda"} {
		ctx, err = svc.BeginTurn(ctx, conv, turn, database.TurnModeContinue)
		if err != nil {
			t.Fatal(err)
		}
		streamer := &recoveryStreamer{steps: []func(llm.StreamHandler){
			func(h llm.StreamHandler) {
				h.OnToolCalls([]llm.ToolCall{{ID: strings.TrimSpace(suffix), Type: "function", Function: llm.FunctionCall{Name: "ok_tool", Arguments: `{}`}}}, "", llm.Usage{}, "modelo")
			},
			func(h llm.StreamHandler) { h.OnChunk(suffix); h.OnDone("", llm.Usage{}, "modelo") },
		}}
		svc.RunAgenticLoop(ctx, nil, llm.ChatParams{MaxAgenticIterations: 3, ContinueViaUserMessage: true}, conv, turn, nil, streamer, nil,
			func(c string, i int) IterationHandler { return NewAgenticStreamHandler(em, c, i, nil, turn) }, nil, false, 1)
		svc.EndUnfinishedTurn(ctx, conv, turn, nil)
		wantText += suffix
		stored, err := chat.NewDBTimelineReader(database.DB()).ReadTurn(ctx, conv, turn)
		if err != nil {
			t.Fatal(err)
		}
		text, reasoning, calls := "", "", []string{}
		for _, segment := range stored.Response.Segments {
			if segment.Kind == "text" {
				text += segment.Text
			}
			if segment.Kind == "reasoning" {
				reasoning += segment.Text
			}
			for _, invocation := range segment.Invocations {
				calls = append(calls, invocation.ID)
			}
		}
		wantCalls := "original,primeira"
		if suffix == " segunda" {
			wantCalls += ",segunda"
		}
		if text != wantText || reasoning != "pensamento" || strings.Join(calls, ",") != wantCalls {
			t.Fatalf("cadeia divergente: text=%q reasoning=%q calls=%v", text, reasoning, calls)
		}
	}
}

type executionWriterBlockingTool struct {
	okTool
	entered chan struct{}
}

func (tool executionWriterBlockingTool) Execute(ctx context.Context, _ json.RawMessage) (tools.ToolResult, error) {
	close(tool.entered)
	<-ctx.Done()
	return tools.ToolResult{}, ctx.Err()
}

func TestExecutionWriterCancellationPreservesRunningToolChronology(t *testing.T) {
	for _, failWrite := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancelled", true: "write_failure"}[failWrite], func(t *testing.T) {
			svc, ctx, conv, turn, em := setupTurnRuntime(t)
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			if err := database.DB().Create(&database.ToolCatalog{Name: "ok_tool", DisplayName: "ok_tool", Origin: tools.ToolOriginBuiltin, AvailabilityStatus: tools.ToolAvailabilityAvailable}).Error; err != nil {
				t.Fatal(err)
			}
			entered := make(chan struct{})
			registry := tools.NewRegistry()
			registry.MustRegister(executionWriterBlockingTool{entered: entered})
			svc.toolInvocations = toolinvocations.NewService(toolinvocations.NewDBRepository(database.DB()), tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
			streamer := &recoveryStreamer{steps: []func(llm.StreamHandler){func(h llm.StreamHandler) {
				h.OnChunk("antes da ferramenta")
				h.OnToolCalls([]llm.ToolCall{{ID: "pending", Type: "function", Function: llm.FunctionCall{Name: "ok_tool", Arguments: `{}`}}}, "", llm.Usage{}, "modelo")
			}}}
			done := make(chan struct{})
			go func() {
				defer close(done)
				svc.RunAgenticLoop(ctx, nil, llm.ChatParams{MaxAgenticIterations: 3}, conv, turn, nil, streamer, nil, func(c string, i int) IterationHandler { return NewAgenticStreamHandler(em, c, i, nil, turn) }, nil, false, 1)
				svc.EndUnfinishedTurn(ctx, conv, turn, nil)
			}()
			t.Cleanup(func() {
				cancel()
				select {
				case <-done:
				case <-time.After(10 * time.Second):
					t.Error("runtime não encerrou")
				}
			})
			select {
			case <-entered:
			case <-time.After(10 * time.Second):
				t.Fatal("ferramenta não iniciou")
			}
			read := func() *chat.TimelineItem {
				window, err := svc.projection.ReadCanonicalWindow(context.WithoutCancel(ctx), database.MessageWindowQuery{ConversationID: conv, Anchor: "end", Direction: "before", Limit: 20}, chat.NewDBTimelineReader(database.DB()))
				if err != nil {
					t.Fatal(err)
				}
				response := window.Responses[turn]
				if response == nil {
					t.Fatal("resposta ausente")
				}
				return response
			}
			assertChain := func(response *chat.TimelineItem, wantStatus string) string {
				order, invocationID := []string{}, ""
				for _, segment := range response.Segments {
					if segment.Kind == "text" {
						order = append(order, "text:"+segment.Text)
					}
					for _, call := range segment.Invocations {
						order = append(order, "tool:"+call.ID)
						invocationID = call.InvocationID
						if call.Status != wantStatus {
							t.Fatalf("estado da chamada: %+v", call)
						}
					}
				}
				if strings.Join(order, "|") != "text:antes da ferramenta|tool:pending" || invocationID == "" {
					t.Fatalf("cronologia divergente: %v", order)
				}
				return invocationID
			}
			identity := assertChain(read(), "running")
			if failWrite {
				if err := database.DB().Exec(`CREATE TRIGGER reject_cancelled_content BEFORE UPDATE ON chat_messages BEGIN SELECT RAISE(ABORT,'disco indisponível'); END`).Error; err != nil {
					t.Fatal(err)
				}
			}
			cancel()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Fatal("cancelamento não encerrou")
			}
			if finalIdentity := assertChain(read(), "cancelled"); finalIdentity != identity {
				t.Fatalf("identidade alterada: %s -> %s", identity, finalIdentity)
			}
			if !failWrite {
				assertRuntimeTerminal(t, context.WithoutCancel(ctx), conv, turn, database.TurnInterrupted, "cancelled", em)
			} else {
				assertRuntimeTerminal(t, context.WithoutCancel(ctx), conv, turn, database.TurnFailed, "persistence_error", em)
			}
		})
	}
}

type executionWriterObservedTool struct {
	okTool
	calls     *atomic.Int32
	retryable bool
}

func (tool executionWriterObservedTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	tool.calls.Add(1)
	if tool.retryable {
		return tools.ToolResult{Content: "retryable failure", IsError: true, Failure: &tools.ToolFailure{Code: "timeout", Kind: tools.ErrorKindTimeout, Retryable: true}}, nil
	}
	return tools.ToolResult{Content: "resultado"}, nil
}

func TestExecutionWriterLedgerFailureCannotSilentlyLoseDiagnostic(t *testing.T) {
	for _, scenario := range []string{"admission", "result", "admission_and_partial", "load_skill", "native_final", "native_tools", "native_error", "retry_batch", "load_skill_result", "intermediate"} {
		t.Run(scenario, func(t *testing.T) {
			svc, ctx, conv, turn, em := setupTurnRuntime(t)
			if err := database.DB().Create(&database.ToolCatalog{Name: "ok_tool", DisplayName: "ok_tool", Origin: tools.ToolOriginBuiltin, AvailabilityStatus: tools.ToolAvailabilityAvailable}).Error; err != nil {
				t.Fatal(err)
			}
			calls := &atomic.Int32{}
			registry := tools.NewRegistry()
			registry.MustRegister(executionWriterObservedTool{calls: calls, retryable: scenario == "retry_batch"})
			registry.MustRegister(loadSkillRuntimeTestTool{})
			svc.toolInvocations = toolinvocations.NewService(toolinvocations.NewDBRepository(database.DB()), tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
			operation := "INSERT"
			condition := ""
			if scenario == "result" || scenario == "load_skill_result" {
				operation = "UPDATE"
				condition = " WHEN NEW.status='succeeded'"
			}
			if scenario == "retry_batch" {
				condition = " WHEN NEW.attempt>1"
			}
			if err := database.DB().Exec("CREATE TRIGGER reject_invocation BEFORE " + operation + " ON tool_invocations" + condition + " BEGIN SELECT RAISE(ABORT,'ledger unavailable'); END").Error; err != nil {
				t.Fatal(err)
			}
			if scenario == "intermediate" {
				database.DB().Exec("DROP TRIGGER reject_invocation")
				if err := database.DB().Exec("CREATE TRIGGER reject_intermediate BEFORE INSERT ON chat_messages WHEN NEW.role='assistant' AND NEW.content <> '' BEGIN SELECT RAISE(ABORT,'write unavailable'); END").Error; err != nil {
					t.Fatal(err)
				}
			}
			streamer := &recoveryStreamer{steps: []func(llm.StreamHandler){func(h llm.StreamHandler) {
				h.OnChunk("parcial recebido")
				if scenario == "admission_and_partial" {
					if err := database.DB().Exec("CREATE TRIGGER reject_partial BEFORE UPDATE OF content ON chat_messages BEGIN SELECT RAISE(ABORT,'write unavailable'); END").Error; err != nil {
						t.Fatal(err)
					}
				}
				if strings.HasPrefix(scenario, "native_") {
					h.(interface{ OnMCPToolEvent(llm.MCPToolEvent) }).OnMCPToolEvent(llm.MCPToolEvent{ID: "native", Name: "remote", ServerLabel: "fixture", IsCompleted: true, Output: "executado externamente"})
					if scenario == "native_final" {
						h.OnDone("parcial recebido", llm.Usage{}, "modelo")
						return
					}
					if scenario == "native_error" {
						h.OnError("provider failure")
						return
					}
				}
				if scenario == "retry_batch" {
					h.OnToolCalls([]llm.ToolCall{{ID: "first", Type: "function", Function: llm.FunctionCall{Name: "ok_tool", Arguments: `{}`}}, {ID: "second", Type: "function", Function: llm.FunctionCall{Name: "ok_tool", Arguments: `{}`}}}, "", llm.Usage{}, "modelo")
					return
				}
				if strings.HasPrefix(scenario, "load_skill") {
					h.OnToolCalls([]llm.ToolCall{
						{ID: "load", Type: "function", Function: llm.FunctionCall{Name: tools.LoadSkillName, Arguments: `{}`}},
						{ID: "regular", Type: "function", Function: llm.FunctionCall{Name: "ok_tool", Arguments: `{}`}},
					}, "", llm.Usage{}, "modelo")
					return
				}

				h.OnToolCalls([]llm.ToolCall{{ID: "unavailable", Type: "function", Function: llm.FunctionCall{Name: "ok_tool", Arguments: `{}`}}}, "", llm.Usage{}, "modelo")
			}}}
			svc.RunAgenticLoop(ctx, nil, llm.ChatParams{MaxAgenticIterations: 3}, conv, turn, nil, streamer, nil,
				func(c string, i int) IterationHandler { return NewAgenticStreamHandler(em, c, i, nil, turn) }, nil, false, 1)
			svc.EndUnfinishedTurn(ctx, conv, turn, nil)
			done := assertRuntimeTerminal(t, ctx, conv, turn, database.TurnFailed, "persistence_error", em)
			if done.ErrorMessage != ports.ChatErrorPersistence || streamer.calls != 1 {
				t.Fatalf("terminal=%+v model calls=%d", done, streamer.calls)
			}
			wantCalls := int32(0)
			if scenario == "result" || scenario == "intermediate" {
				wantCalls = 1
			}
			if scenario == "retry_batch" {
				wantCalls = 2
			}
			if strings.HasPrefix(scenario, "load_skill") && len(em.find("chat:skill_loaded")) != 0 {
				t.Fatal("applied unconfirmed skill")
			}
			if strings.HasPrefix(scenario, "load_skill") {
				starts := em.find("chat:tool_start")
				if len(starts) != 1 || starts[0].data.(ports.ToolStartEvent).CallID != "load" {
					t.Fatalf("started blocked group: %+v", starts)
				}
			}

			if calls.Load() != wantCalls {
				t.Fatalf("tool calls=%d want=%d", calls.Load(), wantCalls)
			}
			window, err := svc.projection.ReadCanonicalWindow(ctx, database.MessageWindowQuery{ConversationID: conv, Anchor: "end", Direction: "before", Limit: 20}, chat.NewDBTimelineReader(database.DB()))
			if err != nil {
				t.Fatal(err)
			}
			response := window.Responses[turn]
			if response == nil || response.State != database.TurnFailed || response.TerminalReason != "persistence_error" {
				t.Fatalf("canonical terminal=%+v", response)
			}
			text := ""
			for _, segment := range response.Segments {
				if segment.Kind == "text" {
					text += segment.Text
				}
			}
			if text != "parcial recebido" {
				t.Fatalf("partial lost: %q", text)
			}
			if scenario == "admission_and_partial" || scenario == "intermediate" {
				if response.Durability != "volatile" {
					t.Fatalf("durability=%s", response.Durability)
				}
			} else {
				reopened, err := chat.NewDBTimelineReader(database.DB()).ReadTurn(ctx, conv, turn)
				if err != nil {
					t.Fatal(err)
				}
				if reopened.Response.TerminalReason != "persistence_error" {
					t.Fatalf("reopened=%+v", reopened)
				}
				if scenario == "result" {
					order := []string{}
					for _, segment := range reopened.Response.Segments {
						if segment.Kind == "text" && segment.Text != "" {
							order = append(order, "text:"+segment.Text)
						}
						for range segment.Invocations {
							order = append(order, "tool")
						}
					}
					if strings.Join(order, ",") != "text:parcial recebido,tool" {
						t.Fatalf("reopened chronology=%v", order)
					}
				}

			}
		})
	}
}

func TestExecutionWriterNativeCallsRetryAndReferencedMedia(t *testing.T) {
	svc, ctx, conv, turn, _ := setupTurnRuntime(t)
	svc.toolInvocations = toolinvocations.NewService(toolinvocations.NewDBRepository(database.DB()), nil)
	previousResponse := ""
	for _, mode := range []string{database.TurnModeNew, database.TurnModeRetry, database.TurnModeContinue} {
		if mode != database.TurnModeNew {
			var err error
			ctx, err = svc.BeginTurn(ctx, conv, turn, mode)
			if err != nil {
				t.Fatal(err)
			}
		}
		runCtx := ctx
		t.Cleanup(func() { svc.EndUnfinishedTurn(runCtx, conv, turn, nil) })
		response, err := svc.EnsureTurnResponse(ctx, conv, turn)
		if err != nil {
			t.Fatal(err)
		}
		if response == previousResponse {
			t.Fatal("representante reutilizado")
		}
		if mode == database.TurnModeContinue {
			var stored database.ChatMessage
			if err := database.DB().First(&stored, "id=?", response).Error; err != nil {
				t.Fatal(err)
			}
			if stored.Media != "" || stored.Content != database.TurnModeRetry {
				t.Fatalf("continuação duplicou blob ou perdeu prefixo: %+v", stored)
			}
			presented, err := chat.NewDBMessageStore().GetMessage(ctx, response)
			if err != nil || presented.Media != `[{"type":"image","data":"base"}]` {
				t.Fatalf("referência de mídia perdida: %+v %v", presented, err)
			}
		}
		if err := svc.persistNativeMCPCalls(ctx, conv, turn, []llm.MCPToolEvent{{ID: "shared-call", Name: "unknown", ServerLabel: "Imported", Arguments: `{}`, Output: mode, IsCompleted: true}}, 0); err != nil {
			t.Fatal(err)
		}
		svc.tagChatToolInvocationsWithAssistantMessage(ctx, conv, turn, []tools.ToolExecutionResult{{CallID: "shared-call"}}, response)
		if !svc.SaveAndFinish(ctx, conv, turn, response, AgenticResult{FullResponse: mode}, "", nil, nil) {
			t.Fatal("terminal não persistido")
		}
		if mode == database.TurnModeRetry {
			if err := database.DB().Model(&database.ChatMessage{}).Where("id=?", response).Update("media", `[{"type":"image","data":"base"}]`).Error; err != nil {
				t.Fatal(err)
			}
		}
		svc.EndUnfinishedTurn(ctx, conv, turn, nil)
		previousResponse = response
	}
	var rows []database.ToolInvocation
	if err := database.DB().Where("conversation_id=?", conv).Order("created_at,id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("ledger incompleto: %+v", rows)
	}
	seen := map[string]bool{}
	for _, row := range rows {
		if row.ChatExecutionID == nil || seen[*row.ChatExecutionID] || row.Attempt != 1 {
			t.Fatalf("proveniência inválida: %+v", rows)
		}
		seen[*row.ChatExecutionID] = true
		var meta struct {
			Display struct {
				AssistantMessageID string `json:"assistant_message_id"`
			}
		}
		if err := json.Unmarshal([]byte(row.Metadata), &meta); err != nil {
			t.Fatal(err)
		}
		var message database.ChatMessage
		if err := database.DB().First(&message, "id=?", meta.Display.AssistantMessageID).Error; err != nil {
			t.Fatal(err)
		}
		if message.ChatExecutionID == nil || *message.ChatExecutionID != *row.ChatExecutionID {
			t.Fatal("tag da nova execução alterou a anterior")
		}
	}
	reader := chat.NewDBTimelineReader(database.DB())
	terminal, err := reader.ReadTurn(ctx, conv, turn)
	if err != nil {
		t.Fatal(err)
	}
	window, err := reader.ReadCanonicalWindow(ctx, database.MessageWindowQuery{ConversationID: conv, Anchor: "end", Direction: "before", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Response.Representative.Media != `[{"type":"image","data":"base"}]` || window.Responses[turn].Representative.Media != "" {
		t.Fatal("terminal perdeu mídia herdada ou janela leve materializou o blob")
	}
}

func TestExecutionWriterContinuationPreservesReasoningThroughRuntime(t *testing.T) {
	for _, added := range []string{"", " novo"} {
		t.Run("reasoning="+added, func(t *testing.T) {
			svc, ctx, conv, turn, _ := setupTurnRuntime(t)
			original, err := svc.EnsureTurnResponse(ctx, conv, turn)
			if err != nil {
				t.Fatal(err)
			}
			if !svc.SaveAndFinish(ctx, conv, turn, original, AgenticResult{FullResponse: "prefixo", Reasoning: "anterior"}, "", nil, nil) {
				t.Fatal("base não persistida")
			}
			svc.EndUnfinishedTurn(ctx, conv, turn, nil)
			ctx, err = svc.BeginTurn(ctx, conv, turn, database.TurnModeContinue)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { svc.EndUnfinishedTurn(ctx, conv, turn, nil) })
			streamer := &recoveryStreamer{steps: []func(llm.StreamHandler){func(h llm.StreamHandler) {
				if added != "" {
					h.OnThinking(added)
					h.OnThinkingDone(added)
				}
				h.OnChunk(" sufixo")
				h.OnDone("", llm.Usage{}, "modelo")
			}}}
			svc.StreamSimpleWithRecovery(ctx, streamer, nil, llm.ChatParams{ContinueViaUserMessage: true}, conv, turn, "", nil, false, 1)
			svc.EndUnfinishedTurn(ctx, conv, turn, nil)
			snapshot, err := chat.NewDBTimelineReader(database.DB()).ReadTurn(ctx, conv, turn)
			if err != nil {
				t.Fatal(err)
			}
			representative := snapshot.Response.Representative
			if representative.Content != "prefixo sufixo" || representative.Reasoning != "anterior"+added {
				t.Fatalf("prefixo perdido/duplicado: %+v", representative)
			}
			stored, err := database.GetMessageWithContext(ctx, representative.ID)
			if err != nil || stored.Reasoning != representative.Reasoning {
				t.Fatalf("persistência divergiu: %+v %v", stored, err)
			}
			base, err := database.GetMessageWithContext(ctx, original)
			if err != nil || base.Reasoning != "anterior" {
				t.Fatalf("base alterada: %+v %v", base, err)
			}
		})
	}
}

func TestExecutionWriterContinuationPrefixOccursOnceAcrossToolIterations(t *testing.T) {
	for _, tc := range []struct {
		name      string
		toolFirst bool
		edited    bool
	}{
		{name: "text-first"},
		{name: "tool-first", toolFirst: true},
		{name: "edited-base", edited: true},
		{name: "edited-base-tool-first", toolFirst: true, edited: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, ctx, conv, turn, em := setupTurnRuntime(t)
			original, err := svc.EnsureTurnResponse(ctx, conv, turn)
			if err != nil {
				t.Fatal(err)
			}
			if !svc.SaveAndFinish(ctx, conv, turn, original, AgenticResult{FullResponse: "prefixo", Reasoning: "raciocínio-base"}, "", nil, nil) {
				t.Fatal("base não persistida")
			}
			svc.EndUnfinishedTurn(ctx, conv, turn, nil)
			prefix := "prefixo"
			if tc.edited {
				prefix = "base editada divergente"
				if err := database.UpdateMessageTextWithContext(ctx, original, prefix); err != nil {
					t.Fatal(err)
				}
			}
			ctx, err = svc.BeginTurn(ctx, conv, turn, database.TurnModeContinue)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { svc.EndUnfinishedTurn(ctx, conv, turn, nil) })
			if err := database.DB().Create(&database.ToolCatalog{Name: "ok_tool", DisplayName: "ok_tool", Origin: tools.ToolOriginBuiltin, AvailabilityStatus: tools.ToolAvailabilityAvailable}).Error; err != nil {
				t.Fatal(err)
			}
			registry := tools.NewRegistry()
			registry.MustRegister(okTool{})
			executor := tools.NewExecutor(registry, tools.DefaultExecutorConfig())
			svc.toolInvocations = toolinvocations.NewService(toolinvocations.NewDBRepository(database.DB()), executor)
			streamer := &recoveryStreamer{steps: []func(llm.StreamHandler){
				func(h llm.StreamHandler) {
					if !tc.toolFirst {
						h.OnChunk(" intermediário")
					}
					h.OnThinking(" novo-1")
					h.OnThinkingDone(" novo-1")
					h.OnToolCalls([]llm.ToolCall{{ID: "call", Type: "function", Function: llm.FunctionCall{Name: "ok_tool", Arguments: `{}`}}}, "", llm.Usage{}, "modelo")
				},
				func(h llm.StreamHandler) {
					if tc.toolFirst {
						during, readErr := svc.projection.ReadCanonicalWindow(ctx,
							database.MessageWindowQuery{ConversationID: conv, Anchor: "end", Direction: "before", Limit: 20},
							chat.NewDBTimelineReader(database.DB()))
						if readErr != nil {
							t.Fatal(readErr)
						}
						text, calls := "", 0
						response := during.Responses[turn]
						if response == nil {
							t.Fatal("resposta ausente da janela canônica")
						}
						for _, segment := range response.Segments {
							if segment.Kind == "text" {
								text += segment.Text
							}
							calls += len(segment.Invocations)
						}
						if strings.Count(text, prefix) != 1 || calls != 1 {
							t.Fatalf("tool-first perdeu base ou ferramenta antes do delta: text=%q calls=%d", text, calls)
						}
					}
					h.OnChunk("final")
					h.OnThinking("novo-2")
					h.OnThinkingDone("novo-2")
					h.OnDone("", llm.Usage{}, "modelo")
				},
			}}
			svc.RunAgenticLoop(ctx, nil, llm.ChatParams{MaxAgenticIterations: 3, ContinueViaUserMessage: true}, conv, turn, nil, streamer, nil, func(c string, i int) IterationHandler { return NewAgenticStreamHandler(em, c, i, nil, turn) }, nil, false, 1)
			svc.EndUnfinishedTurn(ctx, conv, turn, nil)
			snapshot, err := chat.NewDBTimelineReader(database.DB()).ReadTurn(ctx, conv, turn)
			if err != nil {
				t.Fatal(err)
			}
			texts, thoughts := "", ""
			for _, segment := range snapshot.Response.Segments {
				if segment.Kind == "text" {
					texts += segment.Text
				}
				if segment.Kind == "reasoning" {
					thoughts += segment.Text
				}
			}
			if strings.Count(texts, prefix) != 1 || strings.Count(thoughts, "raciocínio-base") != 1 || !strings.Contains(thoughts, "novo-1") || !strings.Contains(thoughts, "novo-2") {
				t.Fatalf("prefixo perdido/duplicado: text=%q reasoning=%q", texts, thoughts)
			}
			if tc.edited && strings.Contains(texts, "prefixo") {
				t.Fatalf("continuação ressuscitou base anterior à edição: %q", texts)
			}
			history, err := chat.NewDBMessageStore().LoadHistoryWindow(ctx, conv, 100)
			if err != nil {
				t.Fatal(err)
			}
			thoughts = ""
			for _, message := range history.Messages {
				thoughts += message.Reasoning
			}
			if strings.Count(thoughts, "raciocínio-base") != 1 {
				t.Fatalf("contexto duplicou prefixo: %q", thoughts)
			}
		})
	}
}

func TestExecutionWriterRetryWithoutActivityThenContinueExcludesDiagnostic(t *testing.T) {
	for _, reason := range []string{"internal_error", "output_limit"} {
		t.Run(reason, func(t *testing.T) {
			svc, ctx, conv, turn, _ := setupTurnRuntime(t)
			original, err := svc.EnsureTurnResponse(ctx, conv, turn)
			if err != nil {
				t.Fatal(err)
			}
			if !svc.SaveAndFinish(ctx, conv, turn, original, AgenticResult{FullResponse: "execução anterior"}, "", nil, nil) {
				t.Fatal("base não persistida")
			}
			svc.EndUnfinishedTurn(ctx, conv, turn, nil)
			ctx, err = svc.BeginTurn(ctx, conv, turn, database.TurnModeRetry)
			if err != nil {
				t.Fatal(err)
			}
			failedResponse, err := svc.EnsureTurnResponse(ctx, conv, turn)
			if err != nil {
				t.Fatal(err)
			}
			if reason == "output_limit" {
				result := AgenticResult{}
				result.Finish.Reason = llm.FinishReasonMaxTokens
				if !svc.SaveAndFinish(ctx, conv, turn, failedResponse, result, "", nil, nil) {
					t.Fatal("limite não persistido")
				}
			}
			svc.EndUnfinishedTurn(ctx, conv, turn, nil)
			failed, err := chat.NewDBTimelineReader(database.DB()).ReadTurn(ctx, conv, turn)
			if err != nil {
				t.Fatal(err)
			}
			if failed.Response.Representative.Content != "" || failed.Response.TerminalReason != reason {
				t.Fatalf("diagnóstico virou conteúdo: %+v", failed.Response)
			}
			ctx, err = svc.BeginTurn(ctx, conv, turn, database.TurnModeContinue)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { svc.EndUnfinishedTurn(ctx, conv, turn, nil) })
			streamer := &recoveryStreamer{steps: []func(llm.StreamHandler){func(h llm.StreamHandler) {
				h.OnChunk("conteúdo novo")
				h.OnDone("", llm.Usage{}, "modelo")
			}}}
			svc.StreamSimpleWithRecovery(ctx, streamer, nil, llm.ChatParams{ContinueViaUserMessage: true}, conv, turn, "", nil, false, 1)
			svc.EndUnfinishedTurn(ctx, conv, turn, nil)
			finished, err := chat.NewDBTimelineReader(database.DB()).ReadTurn(ctx, conv, turn)
			if err != nil {
				t.Fatal(err)
			}
			text := ""
			for _, segment := range finished.Response.Segments {
				if segment.Kind == "text" {
					text += segment.Text
				}
			}
			if text != "conteúdo novo" || finished.Response.Representative.Content != text {
				t.Fatalf("continuação herdou diagnóstico ou execução substituída: %+v", finished.Response)
			}
			previous, err := database.GetMessageWithContext(ctx, original)
			if err != nil || previous.Content != "execução anterior" {
				t.Fatalf("histórico físico alterado: %+v %v", previous, err)
			}
		})
	}
}

func TestSimpleNativeLedgerPersistsCompletedBatchDespiteFailure(t *testing.T) {
	for _, terminal := range []string{"done", "error"} {
		t.Run(terminal, func(t *testing.T) {
			svc, ctx, conv, turn, em := setupTurnRuntime(t)
			svc.toolInvocations = toolinvocations.NewService(toolinvocations.NewDBRepository(database.DB()), nil)
			if err := database.DB().Exec("CREATE TRIGGER reject_first_native BEFORE INSERT ON tool_invocations WHEN NEW.tool_call_id='first' BEGIN SELECT RAISE(ABORT,'first unavailable'); END").Error; err != nil {
				t.Fatal(err)
			}
			streamer := &recoveryStreamer{steps: []func(llm.StreamHandler){func(h llm.StreamHandler) {
				h.OnChunk("parcial nativo")
				for _, id := range []string{"first", "second"} {
					h.OnMCPToolEvent(llm.MCPToolEvent{ID: id, Name: "remote", ServerLabel: "fixture", IsCompleted: true, Output: "executado"})
				}
				if terminal == "done" {
					h.OnDone("parcial nativo", llm.Usage{}, "modelo")
				} else {
					h.OnError("provider error")
				}
			}}}
			svc.StreamSimpleWithRecovery(ctx, streamer, nil, llm.ChatParams{}, conv, turn, "", nil, true, 3)
			svc.EndUnfinishedTurn(ctx, conv, turn, nil)
			assertRuntimeTerminal(t, ctx, conv, turn, database.TurnFailed, "persistence_error", em)
			if streamer.calls != 1 {
				t.Fatalf("repeated native effects: %d", streamer.calls)
			}
			var count int64
			if err := database.DB().Table("tool_invocations").Where("tool_call_id = ? AND status = ?", "second", "succeeded").Count(&count).Error; err != nil || count != 1 {
				t.Fatalf("remaining completed call lost: %d %v", count, err)
			}
			reopened, err := chat.NewDBTimelineReader(database.DB()).ReadTurn(ctx, conv, turn)
			if err != nil {
				t.Fatal(err)
			}
			text := ""
			for _, segment := range reopened.Response.Segments {
				if segment.Kind == "text" {
					text += segment.Text
				}
			}
			if text != "parcial nativo" {
				t.Fatalf("partial lost: %q", text)
			}
		})
	}
}

func TestSimpleNativeCompletedCallSurvivesCancellation(t *testing.T) {
	svc, ctx, conv, turn, em := setupTurnRuntime(t)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	svc.toolInvocations = toolinvocations.NewService(toolinvocations.NewDBRepository(database.DB()), nil)
	streamer := &recoveryStreamer{steps: []func(llm.StreamHandler){func(h llm.StreamHandler) {
		h.OnChunk("antes do cancelamento")
		h.OnMCPToolEvent(llm.MCPToolEvent{ID: "completed", Name: "remote", ServerLabel: "fixture", IsCompleted: true, Output: "feito"})
		cancel()
	}}}
	svc.StreamSimpleWithRecovery(ctx, streamer, nil, llm.ChatParams{}, conv, turn, "", nil, true, 3)
	ctx = context.WithoutCancel(ctx)
	svc.EndUnfinishedTurn(ctx, conv, turn, nil)
	assertRuntimeTerminal(t, ctx, conv, turn, database.TurnInterrupted, "cancelled", em)
	var count int64
	if err := database.DB().Table("tool_invocations").Where("tool_call_id = ? AND status = ?", "completed", "succeeded").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("completed effect lost: %d %v", count, err)
	}
	if streamer.calls != 1 {
		t.Fatalf("repeated native effects: %d", streamer.calls)
	}
}
