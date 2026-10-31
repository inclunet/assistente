package chat

import (
	"reflect"
	"testing"
	"time"

	"assistente/internal/database"
)

func TestPersistedResponseUsesExplicitRepresentativeAndStableSegments(t *testing.T) {
	turnID, messages, _ := canonicalTurnFixture()
	// A primeira mensagem é a resposta persistida; a segunda tem usage e edição
	// posterior suficientes para vencer a heurística antiga.
	messages[0].Reasoning = "raciocínio"
	messages[1].TotalTokens = 900
	messages[1].UpdatedAt = time.Now().Add(time.Hour)
	var calls []TurnSegmentToolCall
	turn := &database.ChatTurn{UserMessageID: turnID, ConversationID: "conv-1", ResponseMessageID: &messages[0].ID, State: database.TurnFailed, Origin: "runtime"}
	first, err := projectPersistedResponse("conv-1", turnID, nil, messages, calls, turn)
	if err != nil {
		t.Fatal(err)
	}
	if first.Representative.ID != messages[0].ID || first.State != database.TurnFailed || first.Segments[len(first.Segments)-1].Text != messages[0].Content {
		t.Fatalf("autoridade persistida perdida: %+v", first)
	}
	messages[0].Content += " e mais texto"
	second, err := projectPersistedResponse("conv-1", turnID, nil, messages, calls, turn)
	if err != nil {
		t.Fatal(err)
	}
	for i, segment := range first.Segments {
		if segment.SegmentID == "" || segment.SegmentID != second.Segments[i].SegmentID {
			t.Fatalf("identidade mudou ao crescer texto: %+v", second)
		}
	}
	if first.ItemID != second.ItemID || first.Segments[1].Kind != "reasoning" {
		t.Fatalf("identidade/raciocínio: %+v", first)
	}
}

func TestPersistedResponseNullIsNotLegacyOrCompleted(t *testing.T) {
	turnID, messages, calls := canonicalTurnFixture()
	turn := &database.ChatTurn{UserMessageID: turnID, ConversationID: "conv-1", State: database.TurnInterrupted, Origin: "runtime"}
	projection, err := projectPersistedResponse("conv-1", turnID, nil, messages, calls, turn)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Representative != nil || projection.State != database.TurnInterrupted || len(projection.Segments) == 0 {
		t.Fatalf("NULL promovido: %+v", projection)
	}
	visual, _ := projection.legacyMessage(messages, calls)
	if visual.ID == "" || projection.Representative != nil {
		t.Fatal("adaptação contaminou a projeção")
	}
	legacy, err := projectPersistedResponse("conv-1", turnID, nil, messages, calls, nil)
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Representative == nil || legacy.State != database.TurnHistoricalUnknown {
		t.Fatalf("legado: %+v", legacy)
	}
	empty, err := projectPersistedResponse("conv-1", turnID, nil, nil, nil, turn)
	if err != nil || empty.Representative != nil || empty.State != database.TurnInterrupted {
		t.Fatalf("turno sem conteúdo: %+v %v", empty, err)
	}
}

func TestPersistedResponseRejectsInvalidReferences(t *testing.T) {
	turnID, messages, calls := canonicalTurnFixture()
	missing := "missing"
	turn := &database.ChatTurn{UserMessageID: turnID, ConversationID: "conv-1", ResponseMessageID: &missing, State: database.TurnCompleted}
	if _, err := projectPersistedResponse("conv-1", turnID, nil, messages, calls, turn); err == nil {
		t.Fatal("representante ausente aceito")
	}
	turn.ResponseMessageID = &messages[0].ID
	for _, field := range []string{"conversation", "scope", "turn", "role", "invocation"} {
		t.Run(field, func(t *testing.T) {
			rows := append([]Message(nil), messages...)
			tools := append([]TurnSegmentToolCall(nil), calls...)
			switch field {
			case "conversation":
				rows[0].ConversationID = "other"
			case "scope":
				rows[0].ParentID = &missing
			case "turn":
				rows[0].TurnID = &missing
			case "role":
				rows[0].Role = "system"
			case "invocation":
				tools[0].InvocationID = ""
			}
			if _, err := projectPersistedResponse("conv-1", turnID, nil, rows, tools, turn); err == nil {
				t.Fatal("referência inválida aceita")
			}
		})
	}
}

func TestPersistedResponseSimpleSegmentsAndInvocationIdentity(t *testing.T) {
	turnID, messages, calls := canonicalTurnFixture()
	projection, err := projectPersistedResponse("conv-1", turnID, nil, messages[:1], nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Segments) != 1 || projection.Segments[0].Text != messages[0].Content {
		t.Fatal("segmento simples omitido")
	}
	_, legacy := projection.legacyMessage(messages[:1], nil)
	if len(legacy) != 0 {
		t.Fatal("adaptador antigo perdeu compatibilidade")
	}
	first := responseToolSegment(calls)
	calls[0].ID = "provider-id-changed"
	calls[0].OutputPreview = "resultado atualizado"
	second := responseToolSegment(calls)
	if first.SegmentID != second.SegmentID {
		t.Fatal("ID depende do provedor/conteúdo")
	}
	if responseTextSegment("text", messages[0], 0, "igual").SegmentID == responseTextSegment("text", messages[1], 0, "igual").SegmentID {
		t.Fatal("textos iguais colidiram")
	}
	if responseTextSegment("text", messages[0], 0, "igual").SegmentID == responseTextSegment("reasoning", messages[0], 0, "igual").SegmentID {
		t.Fatal("tipos colidiram")
	}
}

func TestTimelineSnapshotProjectsSameAuthoritativeResponse(t *testing.T) {
	f := newSnapshotFixture(t)
	intermediate, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, TurnID: &f.question, Role: "assistant", Content: "intermediário editado depois"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.Model(&database.ChatMessage{}).Where("id = ?", intermediate.ID).Updates(map[string]any{"total_tokens": 9999, "updated_at": time.Now().Add(time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	window := f.window(t)
	terminal, err := f.reader.ReadTurn(f.ctx, f.conversation, f.question)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(window.Responses[f.question], terminal.Response) {
		t.Fatal("janela e terminal divergem")
	}
	for _, nodes := range [][]MessageNode{window.Window.Nodes, terminal.Nodes} {
		if responseNode(t, nodes, f.question).Message.ID != f.answer {
			t.Fatal("adaptador ignorou representante persistido")
		}
	}
	// Paginação pode carregar só a pergunta; a resposta fora da janela não é erro.
	_, err = f.reader.ReadWindow(f.ctx, database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "start", Direction: "after", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.DeleteMessageWithContext(f.ctx, f.answer); err != nil {
		t.Fatal(err)
	}
	window = f.window(t)
	terminal, err = f.reader.ReadTurn(f.ctx, f.conversation, f.question)
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Response.Representative != nil || !reflect.DeepEqual(window.Responses[f.question], terminal.Response) {
		t.Fatal("exclusão reelegeu representante ou quebrou paridade")
	}
}

func TestPersistedResponsePreservesOpaqueMessageLinks(t *testing.T) {
	turnID, messages, calls := canonicalTurnFixture()
	messages[0].ID = " m "
	messages[1].ID = "m"
	calls[0].AssistantMessageID = messages[0].ID
	turn := &database.ChatTurn{UserMessageID: turnID, ConversationID: "conv-1", ResponseMessageID: &messages[1].ID, State: database.TurnCompleted}
	projection, err := projectPersistedResponse("conv-1", turnID, nil, messages, calls, turn)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Segments) != 3 || projection.Segments[0].Text != messages[0].Content || projection.Segments[1].Kind != "tool_invocations" || projection.Segments[2].Text != messages[1].Content {
		t.Fatalf("vínculo opaco alterou ordem: %+v", projection.Segments)
	}
	messages[0].UpdatedAt = time.Now().Add(time.Hour)
	legacy := legacyTurnRepresentative(orderedTurnMessages(messages), calls)
	if legacy.ID != "m" {
		t.Fatalf("intermediário com ID opaco promovido: %q", legacy.ID)
	}
}

func TestSnapshotResponseLeavesBlankTurnAsIndependentMessage(t *testing.T) {
	blank := "  "
	rows := []Message{{UUIDModel: database.UUIDModel{ID: "legacy"}, ConversationID: "conv", TurnID: &blank, Role: "assistant", Content: "legado"}}
	responses, err := projectSnapshotResponses("conv", nil, rows, timelineFacts{})
	if err != nil || len(responses) != 0 {
		t.Fatalf("mensagem independente promovida: %+v %v", responses, err)
	}
}
