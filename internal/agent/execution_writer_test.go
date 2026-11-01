package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
)

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
		svc.persistNativeMCPCalls(ctx, conv, turn, []llm.MCPToolEvent{{ID: "shared-call", Name: "unknown", ServerLabel: "Imported", Arguments: `{}`, Output: mode, IsCompleted: true}}, 0)
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
	window, err := reader.ReadWindow(ctx, database.MessageWindowQuery{ConversationID: conv, Anchor: "end", Direction: "before", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Response.Representative.Media != `[{"type":"image","data":"base"}]` || window.Responses[turn].Representative.Media != terminal.Response.Representative.Media {
		t.Fatal("janela e terminal perderam referência de mídia")
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
	svc, ctx, conv, turn, em := setupTurnRuntime(t)
	original, err := svc.EnsureTurnResponse(ctx, conv, turn)
	if err != nil {
		t.Fatal(err)
	}
	if !svc.SaveAndFinish(ctx, conv, turn, original, AgenticResult{FullResponse: "prefixo", Reasoning: "raciocínio-base"}, "", nil, nil) {
		t.Fatal("base não persistida")
	}
	svc.EndUnfinishedTurn(ctx, conv, turn, nil)
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
			h.OnChunk(" intermediário")
			h.OnThinking(" novo-1")
			h.OnThinkingDone(" novo-1")
			h.OnToolCalls([]llm.ToolCall{{ID: "call", Type: "function", Function: llm.FunctionCall{Name: "ok_tool", Arguments: `{}`}}}, "", llm.Usage{}, "modelo")
		},
		func(h llm.StreamHandler) {
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
	if strings.Count(texts, "prefixo") != 1 || strings.Count(thoughts, "raciocínio-base") != 1 || !strings.Contains(thoughts, "novo-1") || !strings.Contains(thoughts, "novo-2") {
		t.Fatalf("prefixo perdido/duplicado: text=%q reasoning=%q", texts, thoughts)
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
}
