package chat

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"assistente/internal/database"
)

func TestPublicationSourceSelectsOnlyRequestedMessagesAndValidTurnSources(t *testing.T) {
	f := newSnapshotFixture(t)
	other, err := database.CreateConversationWithContext(f.ctx, "other", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []Message{
		{UUIDModel: database.UUIDModel{ID: "wrong-conversation"}, ConversationID: other.ID, TurnID: &f.question, Role: "assistant"},
		{UUIDModel: database.UUIDModel{ID: "wrong-scope"}, ConversationID: f.conversation, ParentID: &f.answer, TurnID: &f.question, Role: "assistant"},
		{UUIDModel: database.UUIDModel{ID: "user-with-turn"}, ConversationID: f.conversation, TurnID: &f.question, Role: "user"},
	} {
		if err := f.db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	// A resposta aparece nas duas seleções; UNION deve contá-la uma única vez.
	messages := publicationIDSet([]string{f.question, f.answer})
	turns := publicationIDSet([]string{f.question, "missing-question"})
	var ids []string
	if err := f.db.Raw("SELECT chat_messages.id FROM "+publicationMessageSourceSQL+" ORDER BY chat_messages.id", messages, turns).Scan(&ids).Error; err != nil {
		t.Fatal(err)
	}
	want := []string{f.question, f.answer}
	slices.Sort(want)
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("fontes fora da seleção ou duplicadas: %v != %v", ids, want)
	}
}

func TestPublicationBudgetPlanStartsFromSelectedTurnMessages(t *testing.T) {
	f := newSnapshotFixture(t)
	var rows []struct{ Detail string }
	turns := publicationIDSet([]string{f.question})
	if err := f.db.Raw("EXPLAIN QUERY PLAN "+publicationReadBudgetSQL, turns, f.conversation, f.conversation, turns, turns, turns, f.conversation).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows {
		t.Log(row.Detail)
		if strings.Contains(row.Detail, "SEARCH m USING") && strings.Contains(row.Detail, "(turn_id=?)") {
			found = true
		}
		if strings.Contains(row.Detail, "SCAN m") || (strings.Contains(row.Detail, "SEARCH m USING") && !strings.Contains(row.Detail, "(turn_id=?)")) {
			t.Fatalf("orçamento varre fontes fora dos turnos selecionados: %s", row.Detail)
		}
	}
	if !found {
		t.Fatal("plano não usa busca indexada pelas identidades de turno")
	}
}
