package agent

import (
	"context"
	"reflect"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/toolinvocations"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestACPCronologiaNoTerminalEReabertura(t *testing.T) {
	for _, outcome := range []string{"completed", "error", "cancelled", "empty_tail"} {
		t.Run(outcome, func(t *testing.T) {
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
			for _, row := range []any{
				&database.User{UUIDModel: database.UUIDModel{ID: "user"}, Username: "acp", PasswordHash: "x", Role: database.UserRoleUser, IsActive: true},
				&database.Conversation{UUIDModel: database.UUIDModel{ID: "conv"}, UserID: "user", Title: "ACP"},
				&database.ChatMessage{UUIDModel: database.UUIDModel{ID: "turn"}, ConversationID: "conv", Role: "user", Content: "teste"},
			} {
				if err := db.Create(row).Error; err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(database.WithUserID(context.Background(), "user"))
			defer cancel()
			emitter := &mockEmitter{}
			svc := NewService(ServiceConfig{Emitter: emitter, MsgRepo: chat.NewDBMessageStore(), ToolInvocations: toolinvocations.NewService(toolinvocations.NewDBRepository(db), nil)})
			texts := []string{"Vou ler ação. ", "Agora executar. ", "Conclusão."}
			if outcome == "empty_tail" {
				texts[2] = ""
			}
			streamer := acpHistoryStreamer{run: func(handler llm.StreamHandler) {
				h := handler.(*SimpleStreamHandler)
				h.OnChunk(texts[0])
				h.OnSegmentDone()
				h.OnAgentToolEvent(llm.AgentToolEvent{ID: "X", Kind: "read", Status: llm.AgentToolRunning})
				h.OnAgentToolEvent(llm.AgentToolEvent{ID: "X", Kind: "read", Status: llm.AgentToolCompleted})
				h.OnChunk(texts[1])
				h.OnSegmentDone()
				h.OnAgentToolEvent(llm.AgentToolEvent{ID: "Y", Kind: "execute", Status: llm.AgentToolRunning})
				h.OnChunk(texts[2])
				if outcome == "cancelled" {
					cancel()
					return
				}
				if outcome == "error" {
					h.OnError("interrompido")
					return
				}
				h.OnAgentToolEvent(llm.AgentToolEvent{ID: "Y", Kind: "execute", Status: llm.AgentToolCompleted})
				h.OnDone(texts[0]+texts[1]+texts[2], llm.Usage{}, "acp")
			}}
			svc.StreamSimpleWithRecovery(ctx, streamer, nil, llm.ChatParams{}, "conv", "turn", "", nil, true, 3)
			doneEvents := eventosPorNome(emitter, "chat:done")
			if len(doneEvents) != 1 {
				t.Fatalf("terminais: %d", len(doneEvents))
			}
			done := doneEvents[0].(ports.DoneEvent)
			if done.TurnPatch == nil {
				t.Fatal("patch ausente")
			}
			assertOrder := func(patch *ports.TurnPatchEvent) {
				t.Helper()
				if patch == nil {
					t.Fatal("histórico ausente")
				}
				var order []string
				for _, segment := range patch.Message.TurnSegments {
					if segment.Type == "text" {
						order = append(order, segment.Content)
					}
					for _, call := range segment.ToolInvocations {
						order = append(order, call.CallID)
					}
				}
				want := []string{texts[0], "X", texts[1], "Y"}
				if texts[2] != "" {
					want = append(want, texts[2])
				}
				if !reflect.DeepEqual(order, want) {
					t.Fatalf("ordem=%q; esperada=%q", order, want)
				}
				if patch.Message.Content != texts[0]+texts[1]+texts[2] {
					t.Fatalf("texto integral alterado: %q", patch.Message.Content)
				}
			}
			assertOrder(done.TurnPatch)
			// Outra instância, sem estado do streaming: só mensagens e ledger.
			reopened := NewService(ServiceConfig{MsgRepo: chat.NewDBMessageStore()})
			patch, err := reopened.buildTurnPatch(database.WithUserID(context.Background(), "user"), "conv", "turn")
			if err != nil {
				t.Fatal(err)
			}
			assertOrder(patch)
		})
	}
}
