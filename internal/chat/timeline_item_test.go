package chat

import (
	"reflect"
	"testing"

	"assistente/internal/database"
)

func TestTimelineItemsShareWindowAndTerminalProjection(t *testing.T) {
	f := newSnapshotFixture(t)
	window := f.window(t)
	if len(window.Items) != 2 || window.Items[0].ItemID != "message:"+f.question || window.Items[1].ItemID != "turn:"+f.question {
		t.Fatalf("ordem dos itens: %+v", window.Items)
	}
	response := window.Items[1]
	if response != window.Responses[f.question] || response.Kind != "response" || response.Durability != "persisted" || response.ReplyToItemID != window.Items[0].ItemID {
		t.Fatalf("resposta não compartilhada: %+v", response)
	}
	turn, err := f.reader.ReadTurn(f.ctx, f.conversation, f.question)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(response, turn.Response) {
		t.Fatalf("janela e turno divergiram:\n%+v\n%+v", response, turn.Response)
	}
	if !reflect.DeepEqual(response.MessageIDs, []string{f.answer}) || response.Position.AnchorID != f.question {
		t.Fatalf("referências incompletas: %+v", response)
	}
	if window.Window.Nodes[1].ItemID != response.ItemID || window.Window.Nodes[1].Message.ID != response.Representative.ID {
		t.Fatal("adaptador alterou identidade")
	}
}

func TestTimelineItemsPreserveNullRepresentativeAtLegacyBoundary(t *testing.T) {
	f := newSnapshotFixture(t)
	if err := f.db.Model(&database.ChatTurn{}).Where("user_message_id = ?", f.question).Update("response_message_id", nil).Error; err != nil {
		t.Fatal(err)
	}
	window := f.window(t)
	response := window.Items[1]
	if response.Representative != nil || response.TerminalPatch() != nil || len(response.Segments) == 0 {
		t.Fatalf("NULL promovido a representante: %+v", response)
	}
	if window.Window.Nodes[1].Message.ID != f.answer || response.State != database.TurnCompleted {
		t.Fatal("compatibilidade visual alterou conteúdo ou estado persistido")
	}
}

func TestTimelineItemPositionPreservesSQLitePrecision(t *testing.T) {
	row := database.MessageWindowItem{Kind: "turn", ID: "question", TurnID: "question", AnchorID: "question", AnchorCreatedAt: "2026-01-01 01:02:03.123456789+03:00"}
	position := itemPosition(row)
	if position != (TimelinePosition{row.AnchorCreatedAt, "question", 1, "turn:question"}) {
		t.Fatalf("chave reinterpretada: %+v", position)
	}
}

func TestTimelineItemsRejectMissingMaterialization(t *testing.T) {
	_, err := projectWindowItems("conversation", nil, []database.MessageWindowItem{{Kind: "turn", ID: "question", TurnID: "question"}}, nil, nil)
	if err == nil {
		t.Fatal("item ausente foi descartado silenciosamente")
	}
}

func TestTimelineContentAcknowledgementKeepsPersistedIdentity(t *testing.T) {
	p, ctx, execution := liveFixture(t)
	stream := execution.StartStream(0)
	stream.Append("same text")
	if err := execution.CommitContent(0, "same text", "", func() (string, error) { return "stored-one", nil }); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := p.Snapshot(ctx, "conversation", "question")
	if !snapshot.Streams[0].Confirmed || snapshot.Streams[0].PersistedMessageID != "stored-one" {
		t.Fatalf("ack perdido: %+v", snapshot.Streams)
	}
	stream.Append(" and more")
	snapshot, _ = p.Snapshot(ctx, "conversation", "question")
	if snapshot.Streams[0].Confirmed || snapshot.Streams[0].PersistedMessageID != "stored-one" {
		t.Fatalf("delta perdeu proveniência: %+v", snapshot.Streams)
	}
	execution.StartStream(0).Append("same text")
	snapshot, _ = p.Snapshot(ctx, "conversation", "question")
	if snapshot.Streams[0].Confirmed || snapshot.Streams[0].PersistedMessageID != "" {
		t.Fatal("nova tentativa herdou confirmação por igualdade de texto")
	}
	if err := execution.CommitContent(0, "same text", "", func() (string, error) { return "", nil }); err == nil {
		t.Fatal("ack sem identidade aceito")
	}
}
