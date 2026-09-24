package agent

import (
	"assistente/internal/chat"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/toolinvocations"
	"context"
	"encoding/json"
	"errors"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestACPAtividadePersisteNoPatchEHistorico(t *testing.T) {
	for _, status := range []string{llm.AgentToolCompleted, llm.AgentToolFailed, llm.AgentToolCancelled} {
		t.Run(string(status), func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			sqlDB.SetMaxOpenConns(1)
			previous := database.DB()
			database.SetDB(db)
			t.Cleanup(func() {
				database.SetDB(previous)
				if err := sqlDB.Close(); err != nil {
					t.Error(err)
				}
			})
			if err := db.AutoMigrate(&database.User{}, &database.Conversation{}, &database.ChatMessage{}, &database.ToolCatalog{}, &database.ToolInvocation{}); err != nil {
				t.Fatal(err)
			}
			turnID := "turn-acp"
			for _, row := range []any{
				&database.User{UUIDModel: database.UUIDModel{ID: "user-a"}, Username: "acp-test", PasswordHash: "x", Role: database.UserRoleUser, IsActive: true},
				&database.Conversation{UUIDModel: database.UUIDModel{ID: "conv-acp"}, UserID: "user-a", Title: "ACP"},
				&database.ChatMessage{UUIDModel: database.UUIDModel{ID: turnID}, ConversationID: "conv-acp", Role: "user", Content: "execute"},
			} {
				if err := db.Create(row).Error; err != nil {
					t.Fatal(err)
				}
			}
			repo := &mockMsgRepo{turnMessages: []chat.Message{{UUIDModel: database.UUIDModel{ID: "assistant-acp", CreatedAt: time.Now()}, ConversationID: "conv-acp", Role: "assistant", TurnID: &turnID, Content: "conclusão"}}}
			emitter := &mockEmitter{}
			svc := NewService(ServiceConfig{Emitter: emitter, MsgRepo: repo, ToolInvocations: toolinvocations.NewService(toolinvocations.NewDBRepository(db), nil)})
			ctx, cancel := context.WithCancel(database.WithUserID(context.Background(), "user-a"))
			defer cancel()
			attempts := 0
			var assistantID string
			streamer := acpHistoryStreamer{run: func(handler llm.StreamHandler) {
				attempts++
				h := handler.(*SimpleStreamHandler)
				assistantID = h.AssistantMessageID
				h.OnChunk("resposta parcial")
				h.OnAgentToolEvent(llm.AgentToolEvent{ID: "call-acp", Kind: "execute", Title: "Executando testes", Status: llm.AgentToolRunning})
				if status == llm.AgentToolCancelled {
					cancel()
					return
				}
				h.OnAgentToolEvent(llm.AgentToolEvent{ID: "call-acp", Kind: "execute", Status: status})
				h.OnAgentToolEvent(llm.AgentToolEvent{ID: "call-acp", Kind: "execute", Status: status})
				if status == llm.AgentToolFailed {
					h.OnFinishReason(llm.FinishInfo{Provider: "acp", Model: "modelo", RawReason: "interrupted", OutputLimit: 100})
					h.OnUsage(llm.Usage{OutputTokensReported: true})
					h.OnError("processo interrompido")
					return
				}
				h.OnDone("conclusão", llm.Usage{}, "acp")
			}}
			svc.StreamSimpleWithRecovery(ctx, streamer, nil, llm.ChatParams{}, "conv-acp", turnID, "", nil, true, 3)
			if attempts != 1 {
				t.Fatalf("agente repetido: %d", attempts)
			}
			doneEvents := eventosPorNome(emitter, "chat:done")
			if len(doneEvents) != 1 {
				t.Fatalf("terminais: %d", len(doneEvents))
			}
			done := doneEvents[0].(ports.DoneEvent)
			if status == llm.AgentToolFailed && (done.Provider != "acp" || done.Model != "modelo" || done.RawReason != "interrupted" || done.EffectiveOutputLimit != 100 || done.ResponseBytes == nil || *done.ResponseBytes != 0 || done.OutputTokens == nil || *done.OutputTokens != 0 || done.ReasoningTokens != nil) {
				t.Fatalf("diagnóstico perdido: %+v", done)
			}
			if done.TurnPatch == nil || len(done.TurnPatch.Message.TurnSegments) == 0 {
				t.Fatalf("terminal sem histórico: %+v", done)
			}
			var count int64
			db.Model(&database.ToolInvocation{}).Count(&count)
			if count != 1 {
				t.Fatalf("duplicação: %d", count)
			}
			history, err := toolinvocations.LoadSummariesForTurnIDsWithUser(context.Background(), "user-a", []string{turnID})
			if err != nil {
				t.Fatal(err)
			}
			if len(history[turnID]) != 1 || history[turnID][0].Origin != "acp_agent" {
				t.Fatalf("histórico: %+v", history)
			}
			if history[turnID][0].HasDetails || history[turnID][0].ResultAvailability != "unavailable" {
				t.Fatalf("detalhes fictícios: %+v", history[turnID][0])
			}
			if history[turnID][0].OutputPreview != "Executando testes" {
				t.Fatal("resumo perdido")
			}
			var saved database.ToolInvocation
			if err := db.First(&saved).Error; err != nil {
				t.Fatal(err)
			}
			expectedStatus := map[string]string{llm.AgentToolCompleted: "succeeded", llm.AgentToolFailed: "failed", llm.AgentToolCancelled: "cancelled"}[status]
			if saved.Status != expectedStatus || !saved.External || saved.InputBytes != 0 {
				t.Fatalf("registro incorreto: %+v", saved)
			}
			var metadata struct {
				External bool `json:"external"`
				Display  struct {
					Version     int    `json:"version"`
					Name        string `json:"name"`
					Origin      string `json:"origin"`
					Iteration   *int   `json:"iteration"`
					DurationMs  *int64 `json:"duration_ms"`
					TextOffset  *int   `json:"acp_text_offset"`
					AssistantID string `json:"assistant_message_id"`
				} `json:"display"`
			}
			if err := json.Unmarshal([]byte(saved.Metadata), &metadata); err != nil {
				t.Fatal(err)
			}
			display := metadata.Display
			if !metadata.External || display.Version != 1 || display.Name != "execute" || display.Origin != "acp_agent" ||
				display.Iteration == nil || *display.Iteration != saved.ModelIteration ||
				display.DurationMs == nil || *display.DurationMs != saved.DurationMs ||
				display.TextOffset == nil || *display.TextOffset != len("resposta parcial") ||
				assistantID == "" || display.AssistantID != assistantID {
				t.Fatalf("metadata ACP persistida incorreta: %s", saved.Metadata)
			}
			other, err := toolinvocations.LoadSummariesForTurnIDsWithUser(context.Background(), "user-b", []string{turnID})
			if err != nil || len(other[turnID]) != 0 {
				t.Fatalf("isolamento: %+v, %v", other, err)
			}
			patch, err := svc.buildTurnPatch(ctx, "conv-acp", turnID)
			if err != nil {
				t.Fatal(err)
			}
			if patch == nil {
				t.Fatal("patch ausente")
			}
			found := false
			for _, segment := range patch.Message.TurnSegments {
				for _, call := range segment.ToolInvocations {
					if call.Origin == "acp_agent" && call.InvocationID != "" {
						found = true
					}
				}
			}
			if !found {
				t.Fatalf("patch perdeu tools: %+v", patch)
			}
		})
	}
}

type acpHistoryStreamer struct{ run func(llm.StreamHandler) }

func (s acpHistoryStreamer) StreamChat(_ context.Context, _ []llm.Message, _ llm.ChatParams, h llm.StreamHandler, _ ...llm.ToolDefinition) {
	s.run(h)
}

func TestACPFalhaAoPersistirNaoRepeteAgente(t *testing.T) {
	h := novoHandlerDeAgente(t, &mockEmitter{}, nil)
	h.SuppressTerminalError(true)
	h.OnChunk("resposta já recebida")
	h.activity.archiveError = errors.New("ledger indisponível")
	h.OnDone("resposta já recebida", llm.Usage{}, "acp")
	if !h.ErrorNotRetryable() || h.LastError() == "" || h.TerminalEmitted() {
		t.Fatal("falha de persistência deve encerrar sem retry")
	}
}
