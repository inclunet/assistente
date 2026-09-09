package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/chat"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/llm"
)

// TestSaveAndFinish_DoneEvent_WithLoopStats verifica que chat:done carrega
// os campos IterationCount, ToolCallCount, ToolsUsed e tokens quando loopStats é fornecido.
func TestSaveAndFinish_DoneEvent_WithLoopStats(t *testing.T) {
	emitter := &mockEmitter{}
	repo := &mockMsgRepo{}

	svc := NewService(ServiceConfig{
		Emitter: emitter,
		MsgRepo: repo,
	})

	stats := &LoopStats{
		IterationCount: 3,
		ToolCallCount:  5,
		ToolsUsed: map[string]struct{}{
			"search":    {},
			"read_file": {},
			"write":     {},
		},
		LastUsage: llm.Usage{
			PromptTokens:     1200,
			CompletionTokens: 300,
			CacheReadTokens:  450,
			CacheWriteTokens: 80,
			CacheMissTokens:  750,
		},
	}

	svc.SaveAndFinish(context.Background(), "42", "1", "", AgenticResult{
		FullResponse: "Resposta final",
		Model:        "gpt-4",
	}, "", stats, nil)

	evts := emitter.getEvents()
	var done *ports.DoneEvent
	for _, e := range evts {
		if e.name == "chat:done" {
			d := e.data.(ports.DoneEvent)
			done = &d
			break
		}
	}

	if done == nil {
		t.Fatal("chat:done não emitido")
	}

	if done.Reason != "completed" {
		t.Errorf("Reason=%q, esperava 'completed'", done.Reason)
	}
	if done.IterationCount != 3 {
		t.Errorf("IterationCount=%d, esperava 3", done.IterationCount)
	}
	if done.ToolCallCount != 5 {
		t.Errorf("ToolCallCount=%d, esperava 5", done.ToolCallCount)
	}
	if done.PromptTokens != 1200 {
		t.Errorf("PromptTokens=%d, esperava 1200", done.PromptTokens)
	}
	if done.CompletionTokens != 300 {
		t.Errorf("CompletionTokens=%d, esperava 300", done.CompletionTokens)
	}
	if done.CacheReadTokens != 450 {
		t.Errorf("CacheReadTokens=%d, esperava 450", done.CacheReadTokens)
	}
	if done.CacheWriteTokens != 80 {
		t.Errorf("CacheWriteTokens=%d, esperava 80", done.CacheWriteTokens)
	}
	if done.CacheMissTokens != 750 {
		t.Errorf("CacheMissTokens=%d, esperava 750", done.CacheMissTokens)
	}
	if !done.HadToolCalls {
		t.Error("HadToolCalls deveria ser true quando ToolCallCount > 0")
	}
	// ToolsUsed deve estar ordenado
	if len(done.ToolsUsed) != 3 {
		t.Fatalf("ToolsUsed len=%d, esperava 3", len(done.ToolsUsed))
	}
	for i := 1; i < len(done.ToolsUsed); i++ {
		if done.ToolsUsed[i] < done.ToolsUsed[i-1] {
			t.Errorf("ToolsUsed não está ordenado: %v", done.ToolsUsed)
			break
		}
	}
}

func TestSaveAndFinish_DoneEvent_CarregaPatchAutoritativoMultiTool(t *testing.T) {
	turnID := "turn-1"
	base := time.Date(2026, 9, 8, 20, 0, 0, 0, time.UTC)
	repo := &mockMsgRepo{turnMessages: []chat.Message{
		{UUIDModel: database.UUIDModel{ID: "assistant-placeholder", CreatedAt: base}, ConversationID: "conv-1", Role: "assistant", TurnID: &turnID, Content: "resposta final", PromptTokens: 50, CompletionTokens: 12, TotalTokens: 62},
		{UUIDModel: database.UUIDModel{ID: "assistant-1", CreatedAt: base.Add(time.Second)}, ConversationID: "conv-1", Role: "assistant", TurnID: &turnID, Content: "vou atualizar o plano", ToolCalls: `[{"id":"call-plan","type":"function","function":{"name":"update_plan","arguments":"{}"}}]`},
		{UUIDModel: database.UUIDModel{ID: "tool-1", CreatedAt: base.Add(2 * time.Second)}, ConversationID: "conv-1", Role: "tool", TurnID: &turnID, ToolCallID: "call-plan", Content: `{"updated":true}`},
		{UUIDModel: database.UUIDModel{ID: "assistant-2", CreatedAt: base.Add(3 * time.Second)}, ConversationID: "conv-1", Role: "assistant", TurnID: &turnID, Content: "agora vou consultar", ToolCalls: `[{"id":"call-read","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"a\"}"}}]`},
		{UUIDModel: database.UUIDModel{ID: "tool-2", CreatedAt: base.Add(4 * time.Second)}, ConversationID: "conv-1", Role: "tool", TurnID: &turnID, ToolCallID: "call-read", Content: "conteúdo"},
	}}
	emitter := &mockEmitter{}
	svc := NewService(ServiceConfig{Emitter: emitter, MsgRepo: repo})

	svc.SaveAndFinish(context.Background(), "conv-1", turnID, "assistant-placeholder", AgenticResult{
		FullResponse: "resposta final",
		Finish:       llm.FinishInfo{Reason: llm.FinishReasonMaxTokens},
	}, "", &LoopStats{IterationCount: 3, ToolCallCount: 2}, nil)

	var done ports.DoneEvent
	for _, event := range emitter.getEvents() {
		if event.name == "chat:done" {
			done = event.data.(ports.DoneEvent)
		}
	}
	if done.Reason != "output_limit" || done.TurnPatch == nil {
		t.Fatalf("esperava output_limit com patch, recebeu %+v", done)
	}
	if done.TurnPatch.Message.TurnID != turnID || done.TurnPatch.Message.Content != "resposta final" {
		t.Fatalf("mensagem final incorreta no patch: %+v", done.TurnPatch.Message)
	}
	if len(done.TurnPatch.Message.TurnSegments) != 5 {
		t.Fatalf("esperava texto/tool/texto/tool/texto, recebeu %+v", done.TurnPatch.Message.TurnSegments)
	}
	if got := done.TurnPatch.Message.TurnSegments[1].ToolCalls[0].Function.Name; got != "update_plan" {
		t.Fatalf("esperava update_plan no primeiro segmento de tool, recebeu %q", got)
	}
	if got := done.TurnPatch.Message.TurnSegments[3].ToolCalls[0].Result; got != "conteúdo" {
		t.Fatalf("resultado da segunda tool não hidratado: %q", got)
	}
}

func TestSaveAndFinish_PreservaDesfechoQuandoPatchFalha(t *testing.T) {
	emitter := &mockEmitter{}
	svc := NewService(ServiceConfig{
		Emitter: emitter,
		MsgRepo: &mockMsgRepo{turnMessagesError: errors.New("db indisponível")},
	})

	svc.SaveAndFinish(context.Background(), "conv-1", "turn-1", "assistant-1", AgenticResult{
		FullResponse: "resposta salva",
	}, "", nil, nil)

	for _, event := range emitter.getEvents() {
		if event.name != "chat:done" {
			continue
		}
		done := event.data.(ports.DoneEvent)
		if done.Reason != "completed" || done.ErrorMessage != "" || done.TurnPatch != nil {
			t.Fatalf("falha opcional do patch alterou o desfecho: %+v", done)
		}
		return
	}
	t.Fatal("chat:done não emitido")
}

func TestBuildTurnPatchSobreviveAoCancelamentoDoTurno(t *testing.T) {
	turnID := "turn-cancelado"
	repo := &mockMsgRepo{turnMessages: []chat.Message{{
		UUIDModel:      database.UUIDModel{ID: "assistant-1", CreatedAt: time.Now()},
		ConversationID: "conv-1",
		Role:           "assistant",
		TurnID:         &turnID,
		Content:        "conteúdo parcial persistido",
	}}}
	svc := NewService(ServiceConfig{Emitter: &mockEmitter{}, MsgRepo: repo})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	patch, err := svc.buildTurnPatch(ctx, "conv-1", turnID)
	if err != nil {
		t.Fatalf("patch não deve herdar cancelamento: %v", err)
	}
	if repo.turnMessagesContextErr != nil {
		t.Fatalf("repository recebeu contexto cancelado: %v", repo.turnMessagesContextErr)
	}
	if patch == nil || patch.Message.Content != "conteúdo parcial persistido" {
		t.Fatalf("patch parcial ausente após cancelamento: %+v", patch)
	}
}

func TestBuildTurnPatchPreservaEscopoDeThread(t *testing.T) {
	turnID := "turn-thread"
	parentID := "thread-root"
	repo := &mockMsgRepo{
		messagesByID: map[string]*chat.Message{
			turnID: {
				UUIDModel:      database.UUIDModel{ID: turnID},
				ConversationID: "conv-1",
				Role:           "user",
				ParentID:       &parentID,
			},
		},
		turnMessages: []chat.Message{{
			UUIDModel:      database.UUIDModel{ID: "assistant-thread", CreatedAt: time.Now()},
			ConversationID: "conv-1",
			ParentID:       &parentID,
			Role:           "assistant",
			TurnID:         &turnID,
			Content:        "resposta na thread",
		}},
	}
	svc := NewService(ServiceConfig{Emitter: &mockEmitter{}, MsgRepo: repo})

	patch, err := svc.buildTurnPatch(context.Background(), "conv-1", turnID)
	if err != nil {
		t.Fatalf("montar patch de thread: %v", err)
	}
	if repo.turnMessagesParentID == nil || *repo.turnMessagesParentID != parentID {
		t.Fatalf("consulta perdeu parentId: %v", repo.turnMessagesParentID)
	}
	if patch == nil || patch.Message.ParentID == nil || *patch.Message.ParentID != parentID {
		t.Fatalf("schema perdeu parentId: %+v", patch)
	}
}

// TestSaveAndFinish_DoneEvent_NilLoopStats verifica que chat:done funciona
// sem loopStats (chamada simples sem agentic loop).
func TestSaveAndFinish_DoneEvent_NilLoopStats(t *testing.T) {
	emitter := &mockEmitter{}
	repo := &mockMsgRepo{}

	svc := NewService(ServiceConfig{
		Emitter: emitter,
		MsgRepo: repo,
	})

	svc.SaveAndFinish(context.Background(), "1", "turn-1", "", AgenticResult{
		FullResponse: "Resposta direta",
		Model:        "test",
		Usage: llm.Usage{
			PromptTokens:     500,
			CompletionTokens: 100,
			CacheReadTokens:  200,
			CacheWriteTokens: 50,
			CacheMissTokens:  300,
		},
	}, "", nil, nil)

	evts := emitter.getEvents()
	var done *ports.DoneEvent
	for _, e := range evts {
		if e.name == "chat:done" {
			d := e.data.(ports.DoneEvent)
			done = &d
			break
		}
	}

	if done == nil {
		t.Fatal("chat:done não emitido")
	}

	if done.Reason != "completed" {
		t.Errorf("Reason=%q, esperava 'completed'", done.Reason)
	}
	if done.IterationCount != 0 {
		t.Errorf("IterationCount=%d, esperava 0 (nil stats)", done.IterationCount)
	}
	if done.ToolCallCount != 0 {
		t.Errorf("ToolCallCount=%d, esperava 0 (nil stats)", done.ToolCallCount)
	}
	if done.HadToolCalls {
		t.Error("HadToolCalls deveria ser false quando loopStats=nil, mesmo com turnID")
	}
	// Tokens devem vir do result.Usage como fallback
	if done.PromptTokens != 500 {
		t.Errorf("PromptTokens=%d, esperava 500 (fallback de result.Usage)", done.PromptTokens)
	}
	if done.CompletionTokens != 100 {
		t.Errorf("CompletionTokens=%d, esperava 100 (fallback de result.Usage)", done.CompletionTokens)
	}
	if done.CacheReadTokens != 200 {
		t.Errorf("CacheReadTokens=%d, esperava 200 (fallback de result.Usage)", done.CacheReadTokens)
	}
	if done.CacheWriteTokens != 50 {
		t.Errorf("CacheWriteTokens=%d, esperava 50 (fallback de result.Usage)", done.CacheWriteTokens)
	}
	if done.CacheMissTokens != 300 {
		t.Errorf("CacheMissTokens=%d, esperava 300 (fallback de result.Usage)", done.CacheMissTokens)
	}
}

// TestSaveAndFinish_DoneEvent_ZeroToolCalls verifica HadToolCalls=false
// quando loopStats tem zero tool calls.
func TestSaveAndFinish_DoneEvent_ZeroToolCalls(t *testing.T) {
	emitter := &mockEmitter{}
	repo := &mockMsgRepo{}

	svc := NewService(ServiceConfig{
		Emitter: emitter,
		MsgRepo: repo,
	})

	stats := &LoopStats{
		IterationCount: 1,
		ToolCallCount:  0,
		ToolsUsed:      map[string]struct{}{},
	}

	svc.SaveAndFinish(context.Background(), "1", "1", "", AgenticResult{
		FullResponse: "Sem tools",
		Model:        "test",
	}, "", stats, nil)

	evts := emitter.getEvents()
	for _, e := range evts {
		if e.name == "chat:done" {
			done := e.data.(ports.DoneEvent)
			if done.HadToolCalls {
				t.Error("HadToolCalls deveria ser false quando ToolCallCount=0")
			}
			if done.ToolCallCount != 0 {
				t.Errorf("ToolCallCount=%d, esperava 0", done.ToolCallCount)
			}
			return
		}
	}
	t.Fatal("chat:done não emitido")
}
