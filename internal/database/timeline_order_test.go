package database

import (
	"reflect"
	"testing"
	"time"
)

func TestTimelineOrderAnchorsResponsesAndPaginatesEveryBoundary(t *testing.T) {
	setupOrderingTestDB(t)
	conv, err := CreateConversationWithContext(testCtx(), "ordem", "")
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 31, 1, 0, 0, 123456789, time.UTC)
	questions := []string{" a ", "B", "a", "b", "nanosecond"}
	want := make([]string, 0, 2*len(questions))
	for i, id := range questions {
		created := base
		if i == len(questions)-1 {
			created = created.Add(time.Nanosecond)
		}
		question := ChatMessage{UUIDModel: UUIDModel{ID: id, CreatedAt: created}, ConversationID: conv.ID, Role: "user"}
		// Dado legado: a própria pergunta não pode entrar na resposta consolidada.
		question.TurnID = &id
		if err := db.Create(&question).Error; err != nil {
			t.Fatal(err)
		}
		for j := range 2 {
			response := ChatMessage{UUIDModel: UUIDModel{ID: id + string(rune('0'+j)), CreatedAt: base.Add(time.Duration(j*2-1) * time.Hour)}, ConversationID: conv.ID, Role: "assistant", TurnID: &id, Content: "resposta"}
			if err := db.Create(&response).Error; err != nil {
				t.Fatal(err)
			}
		}
		want = append(want, "message:"+id, "turn:"+id)
	}
	read := func(q MessageWindowQuery) *MessageWindowResult {
		t.Helper()
		q.ConversationID = conv.ID
		window, err := GetMessageWindowWithContext(testCtx(), q)
		if err != nil {
			t.Fatal(err)
		}
		if window.TotalCount != len(want) {
			t.Fatalf("contagem: %+v", window)
		}
		for i, item := range window.Items {
			if item.OriginalIndex != window.StartIndex+i {
				t.Fatal("índice inconsistente")
			}
		}
		return window
	}
	all := read(MessageWindowQuery{Anchor: "start", Direction: "after", Limit: 100})
	keys := func(items []MessageWindowItem) []string {
		result := make([]string, 0, len(items))
		for _, item := range items {
			prefix := "message:"
			if item.Kind == MessageWindowItemKindTurn {
				prefix = "turn:"
			}
			result = append(result, prefix+item.ID)
		}
		return result
	}
	if !reflect.DeepEqual(keys(all.Items), want) {
		t.Fatalf("ordem: %q; esperada %q", keys(all.Items), want)
	}
	for index, item := range all.Items {
		// Qualquer mensagem interna precisa apontar para o mesmo item/índice.
		anchors := []string{item.MessageID}
		if item.Kind == MessageWindowItemKindTurn {
			anchors = append(anchors, item.TurnID+"1")
		}
		for _, anchor := range anchors {
			before := read(MessageWindowQuery{AnchorMessageID: anchor, Direction: "before", Limit: 100})
			after := read(MessageWindowQuery{AnchorMessageID: anchor, Direction: "after", Limit: 100})
			around := read(MessageWindowQuery{AnchorMessageID: anchor, Direction: "around", Limit: 1})
			if !reflect.DeepEqual(keys(before.Items), want[:index]) || !reflect.DeepEqual(keys(after.Items), want[index+1:]) || !reflect.DeepEqual(keys(around.Items), want[index:index+1]) {
				t.Fatalf("paginação divergiu na âncora %q", anchor)
			}
			if around.StartIndex != index || around.EndIndex != index || around.HasBefore != (index > 0) || around.HasAfter != (index < len(want)-1) {
				t.Fatalf("metadados: %+v", around)
			}
			if item.Kind == MessageWindowItemKindTurn {
				if len(around.Messages) != 2 {
					t.Fatalf("resposta trouxe pergunta ou perdeu segmento: %+v", around.Messages)
				}
				for _, message := range around.Messages {
					if message.Role == "user" {
						t.Fatal("pergunta incluída na resposta")
					}
				}
			}
		}
	}
	// Alterar criação/edição de qualquer resposta não move seu item.
	if err := db.Model(&ChatMessage{}).Where("role = ?", "assistant").Updates(map[string]any{"created_at": base.Add(48 * time.Hour), "updated_at": base.Add(72 * time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	if got := keys(read(MessageWindowQuery{Anchor: "end", Direction: "before", Limit: 100}).Items); !reflect.DeepEqual(got, want) {
		t.Fatalf("respostas moveram: %q", got)
	}
}

func TestTimelineOrderInvalidLinksRemainIndependentWithoutStorageRepair(t *testing.T) {
	setupOrderingTestDB(t)
	conv, err := CreateConversationWithContext(testCtx(), "legado", "")
	if err != nil {
		t.Fatal(err)
	}
	other, err := CreateConversationWithContext(testCtx(), "outra", "")
	if err != nil {
		t.Fatal(err)
	}
	parent, otherParent := "parent", "other-parent"
	base := time.Now().UTC()
	seeds := []ChatMessage{
		{UUIDModel: UUIDModel{ID: parent, CreatedAt: base}, ConversationID: conv.ID, Role: "user"},
		{UUIDModel: UUIDModel{ID: otherParent, CreatedAt: base}, ConversationID: conv.ID, Role: "user"},
		{UUIDModel: UUIDModel{ID: "foreign", CreatedAt: base}, ConversationID: other.ID, Role: "user"},
		{UUIDModel: UUIDModel{ID: "wrong-role", CreatedAt: base}, ConversationID: conv.ID, Role: "assistant"},
		{UUIDModel: UUIDModel{ID: "thread-question", CreatedAt: base}, ConversationID: conv.ID, ParentID: &parent, Role: "user"},
	}
	if err := db.Create(&seeds).Error; err != nil {
		t.Fatal(err)
	}
	links := []string{"missing", "foreign", "wrong-role", "thread-question", "  "}
	for _, link := range links {
		message := ChatMessage{UUIDModel: UUIDModel{ID: "orphan:" + link, CreatedAt: base}, ConversationID: conv.ID, Role: "assistant", TurnID: &link}
		if err := db.Create(&message).Error; err != nil {
			t.Fatal(err)
		}
	}
	window, err := GetMessageWindowWithContext(testCtx(), MessageWindowQuery{ConversationID: conv.ID, Anchor: "start", Direction: "after", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if window.InvalidTurnReferences != len(links) || window.TotalCount != len(links)+3 {
		t.Fatalf("classificação/diagnóstico: %+v", window)
	}
	for _, item := range window.Items {
		if item.Kind != MessageWindowItemKindMessage {
			t.Fatalf("vínculo inválido agrupado: %+v", item)
		}
	}
	for _, message := range window.Messages {
		if message.Role == "assistant" && message.TurnID != nil {
			t.Fatalf("adaptador reagruparia %s", message.ID)
		}
	}
	for _, link := range links {
		var stored ChatMessage
		if err := db.First(&stored, "id = ?", "orphan:"+link).Error; err != nil {
			t.Fatal(err)
		}
		if stored.TurnID == nil || *stored.TurnID != link {
			t.Fatal("leitura reparou o banco")
		}
	}
	// A mesma pergunta é válida só dentro do próprio escopo de thread.
	question := "thread-question"
	threadRows := []ChatMessage{
		{UUIDModel: UUIDModel{ID: "valid-thread"}, ConversationID: conv.ID, ParentID: &parent, Role: "assistant", TurnID: &question},
		{UUIDModel: UUIDModel{ID: "wrong-thread"}, ConversationID: conv.ID, ParentID: &otherParent, Role: "assistant", TurnID: &question},
	}
	if err := db.Create(&threadRows).Error; err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{parent, otherParent} {
		w, err := GetMessageWindowWithContext(testCtx(), MessageWindowQuery{ConversationID: conv.ID, ParentID: &scope, Anchor: "start", Direction: "after", Limit: 100})
		if err != nil {
			t.Fatal(err)
		}
		if scope == parent && (w.TotalCount != 2 || w.InvalidTurnReferences != 0 || w.Items[1].Kind != MessageWindowItemKindTurn) {
			t.Fatalf("thread válida: %+v", w)
		}
		if scope == otherParent && (w.TotalCount != 1 || w.InvalidTurnReferences != 1 || w.Items[0].Kind != MessageWindowItemKindMessage) {
			t.Fatalf("thread inválida: %+v", w)
		}
	}
	for _, anchor := range []string{"foreign", "thread-question", "deleted"} {
		if _, err := GetMessageWindowWithContext(testCtx(), MessageWindowQuery{ConversationID: conv.ID, AnchorMessageID: anchor, Direction: "around", Limit: 1}); err == nil {
			t.Fatalf("âncora inválida aceita: %s", anchor)
		}
	}
}

func TestTimelineOrderDeletedAnchorDoesNotLookLikeEmptySuccess(t *testing.T) {
	setupOrderingTestDB(t)
	conv, err := CreateConversationWithContext(testCtx(), "vazia", "")
	if err != nil {
		t.Fatal(err)
	}
	message, err := AddMessageWithContext(testCtx(), conv.ID, "user", "apagada")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&ChatMessage{}, "id = ?", message.ID).Error; err != nil {
		t.Fatal(err)
	}
	for _, limit := range []int{0, 10} {
		if _, err := GetMessageWindowWithContext(testCtx(), MessageWindowQuery{ConversationID: conv.ID, AnchorMessageID: message.ID, Direction: "before", Limit: limit}); err == nil {
			t.Fatal("âncora excluída virou sucesso vazio")
		}
	}
}
