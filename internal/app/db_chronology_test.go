package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"assistente/internal/chat"
	"assistente/internal/database"
)

func TestMessageWindowPreservaCronologiaComFinalPreCriadoSemUsage(t *testing.T) {
	setupMessageWindowAppTestDB(t)
	ctx := database.WithUserID(context.Background(), messageWindowTestUserID)
	conv := createMessageWindowTestConversation(t, "Cronologia")
	user, err := database.AddMessageWithContext(ctx, conv.ID, "user", "pergunta")
	if err != nil {
		t.Fatal(err)
	}
	// Runtime cria o registro final antes de executar as rodadas. O provedor
	// pode omitir usage; tokens não podem ser a identidade da resposta final.
	final := addMessageWindowAssistant(t, ctx, conv.ID, user.ID, "resposta final")
	intermediate := addMessageWindowAssistant(t, ctx, conv.ID, user.ID, "vou consultar")
	base := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	for _, update := range []struct {
		id               string
		created, updated time.Time
	}{
		{user.ID, base.Add(-time.Second), base.Add(-time.Second)},
		{final.ID, base, base.Add(10 * time.Second)},
		{intermediate.ID, base.Add(time.Second), base.Add(time.Second)},
	} {
		if err := database.DB().Model(&database.ChatMessage{}).Where("id = ?", update.id).
			UpdateColumns(map[string]any{"created_at": update.created, "updated_at": update.updated}).Error; err != nil {
			t.Fatal(err)
		}
	}
	var catalog database.ToolCatalog
	if err := database.DB().WithContext(ctx).First(&catalog, "name = ?", "search").Error; err != nil {
		t.Fatal(err)
	}
	// A rodada 1 não tem fala: nenhum assistant_message_id é persistido.
	for iteration := 0; iteration < 2; iteration++ {
		messageID := ""
		if iteration == 0 {
			messageID = intermediate.ID
		}
		invocation := database.ToolInvocation{
			UUIDModel: database.UUIDModel{ID: fmt.Sprintf("chronology-inv-%d", iteration)},
			UserID:    messageWindowTestUserID, ToolCatalogID: catalog.ID,
			OriginType: "chat", OriginID: user.ID,
			ToolCallID: fmt.Sprintf("call-%d", iteration), Status: "succeeded",
			Metadata: fmt.Sprintf(`{"display":{"assistant_message_id":%q,"iteration":%d}}`, messageID, iteration),
			QueuedAt: base.Add(time.Duration(iteration+1) * time.Second),
		}
		if err := database.DB().WithContext(ctx).Create(&invocation).Error; err != nil {
			t.Fatal(err)
		}
	}
	window, err := newMessageWindowTestController().GetConversationMessageWindow(messageWindowTestCtx(), chat.MessageWindowRequest{
		ConversationID: conv.ID, Scope: chat.MessageWindowScopeConversation,
		Anchor: chat.MessageWindowAnchorEnd, Direction: chat.MessageWindowDirectionBefore, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(window.Nodes) != 2 {
		t.Fatalf("esperava usuário e um único turno: %+v", window.Nodes)
	}
	message := window.Nodes[1].Message
	if message.ID != final.ID || message.Content != "resposta final" {
		t.Fatalf("representante final sem usage perdido: %+v", message)
	}
	segments := message.TurnSegments
	if len(segments) != 4 || segments[0].Content != "vou consultar" || segments[3].Content != "resposta final" {
		t.Fatalf("textos fora de ordem: %+v", segments)
	}
	for index := 0; index < 2; index++ {
		if len(segments[index+1].ToolCalls) != 1 || segments[index+1].ToolCalls[0].ID != fmt.Sprintf("call-%d", index) {
			t.Fatalf("ferramentas fora de ordem: %+v", segments)
		}
	}
}
