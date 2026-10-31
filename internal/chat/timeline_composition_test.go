package chat

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"assistente/internal/database"
)

func compositionItem(t *testing.T, messages []Message, calls []TurnSegmentToolCall) *TimelineItem {
	t.Helper()
	response := "response"
	for i := range messages {
		messages[i].ConversationID = "conversation"
		messages[i].TurnID = refString("question")
		messages[i].Role = "assistant"
	}
	item, err := ProjectPersistedResponse("conversation", "question", nil, messages, calls, &database.ChatTurn{UserMessageID: "question", ConversationID: "conversation", ResponseMessageID: &response})
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func refString(value string) *string { return &value }

func TestCompositionReplacesConfirmedPartialBySourceThenReturnsToStorage(t *testing.T) {
	p, ctx, e := liveFixture(t)
	_, err := e.AssignResponse(func() (string, error) { return "response", nil })
	if err != nil {
		t.Fatal(err)
	}
	s := e.StartStream(0)
	s.Append("parcial")
	if err := e.CommitContent(0, "parcial", "", func() (string, error) { return "response", nil }); err != nil {
		t.Fatal(err)
	}
	s.Append(" novo")
	item := compositionItem(t, []Message{{UUIDModel: database.UUIDModel{ID: "response"}, Content: "parcial"}}, nil)
	stored := recoveryStored("execution", 2)
	stored.Response = item
	r, err := p.ReadRecovery(ctx, "conversation", "question", recoveryReader{func() (*TimelineTurnSnapshot, error) { return stored, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if r.Item.Durability != "volatile" || len(r.Item.Segments) != 1 || r.Item.Segments[0].Text != "parcial novo" {
		t.Fatalf("parcial duplicado: %+v", r.Item)
	}
	if item.Segments[0].Text != "parcial" {
		t.Fatal("snapshot persistido mutado")
	}
	s.Seal("parcial novo", "")
	if err := e.CommitContent(0, "parcial novo", "", func() (string, error) { return "response", nil }); err != nil {
		t.Fatal(err)
	}
	item = compositionItem(t, []Message{{UUIDModel: database.UUIDModel{ID: "response"}, Content: "edição externa"}}, nil)
	live, _ := p.Snapshot(ctx, "conversation", "question")
	composed, err := composeResponse(item, live)
	if err != nil || !reflect.DeepEqual(composed.Segments, item.Segments) {
		t.Fatalf("buffer confirmado sobrescreveu storage: %+v %v", composed, err)
	}
}

func TestCompositionKeepsConfirmedPrefixOnceAndScopesRepeatedIterations(t *testing.T) {
	item := compositionItem(t, []Message{{UUIDModel: database.UUIDModel{ID: "base-middle"}, Content: "base"}, {UUIDModel: database.UUIDModel{ID: "middle"}, Content: "prefixo intermediário"}, {UUIDModel: database.UUIDModel{ID: "response"}, Content: "prefixo"}}, []TurnSegmentToolCall{{InvocationID: "old", ID: "same", ExecutionID: "base", ExecutionDepth: 1, AssistantMessageID: "base-middle"}, {InvocationID: "new", ID: "same", ExecutionID: "execution", AssistantMessageID: "middle"}})
	live := &LiveTurnSnapshot{ItemID: item.ItemID, TurnID: "question", ExecutionID: "execution", ResponseID: "response", State: database.TurnStreaming, Streams: []LiveStreamSnapshot{{Iteration: 0, Confirmed: true, PersistedMessageID: "middle"}, {Iteration: 1, Generation: 2, Segments: []ResponseSegment{{SegmentID: "draft:new", Kind: "text", Text: "final"}}}}}
	composed, err := composeResponse(item, live)
	if err != nil {
		t.Fatal(err)
	}
	var text string
	var ids []string
	for _, segment := range composed.Segments {
		text += segment.Text
		for _, call := range segment.Invocations {
			ids = append(ids, call.InvocationID)
		}
	}
	if strings.Count(text, "prefixo") != 1 || !strings.HasSuffix(text, "final") || !reflect.DeepEqual(ids, []string{"old", "new"}) {
		t.Fatalf("fontes misturadas: %q %+v", text, ids)
	}
	live.Streams[0].PersistedMessageID = "missing"
	if _, err := composeResponse(item, live); !errors.Is(err, ErrProjectionRecoveryRequired) {
		t.Fatalf("fonte ausente aceita: %v", err)
	}
}

func TestCompositionRejectsOtherAttemptAndPreservesGenerationIdentity(t *testing.T) {
	p, ctx, e := liveFixture(t)
	item := compositionItem(t, []Message{{UUIDModel: database.UUIDModel{ID: "response"}}}, nil)
	e.StartStream(0).Append("igual")
	first, _ := p.Snapshot(ctx, "conversation", "question")
	a, err := composeResponse(item, first)
	if err != nil {
		t.Fatal(err)
	}
	e.StartStream(0).Append("igual")
	second, _ := p.Snapshot(ctx, "conversation", "question")
	b, err := composeResponse(item, second)
	if err != nil {
		t.Fatal(err)
	}
	if a.Segments[0].SegmentID == b.Segments[0].SegmentID {
		t.Fatal("reset reutilizou segmento draft")
	}
	second.Detached = true
	if _, err := composeResponse(item, second); !errors.Is(err, ErrProjectionRecoveryRequired) {
		t.Fatal("tentativa retida foi composta")
	}
}

func TestCompositionPreservesLiveCutsAndTextIdentityDuringReasoning(t *testing.T) {
	p, ctx, e := liveFixture(t)
	if _, err := e.AssignResponse(func() (string, error) { return "response", nil }); err != nil {
		t.Fatal(err)
	}
	item := compositionItem(t, []Message{{UUIDModel: database.UUIDModel{ID: "response"}}}, nil)
	stream := e.StartStream(0)
	stream.Append("texto A")
	first, _ := p.Snapshot(ctx, "conversation", "question")
	a, err := composeResponse(item, first)
	if err != nil {
		t.Fatal(err)
	}
	textID := a.Segments[0].SegmentID
	stream.Reasoning("raciocínio A", false)
	stream.Cut()
	stream.Reasoning("raciocínio B", false)
	stream.Append("texto B")
	second, _ := p.Snapshot(ctx, "conversation", "question")
	b, err := composeResponse(item, second)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.Segments, second.Streams[0].Segments) || len(b.Segments) != 4 || b.Segments[1].SegmentID != textID {
		t.Fatalf("fragmentos/identidade alterados: %+v", b.Segments)
	}
	stream.Reasoning("", true)
	third, _ := p.Snapshot(ctx, "conversation", "question")
	c, err := composeResponse(item, third)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Segments) != 2 || c.Segments[0].SegmentID != textID || c.Segments[1].SegmentID != b.Segments[3].SegmentID {
		t.Fatalf("descarte reidentificou texto: %+v", c.Segments)
	}
}
