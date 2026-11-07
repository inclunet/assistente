package chat

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"assistente/internal/database"
	"gorm.io/gorm"
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
		{UUIDModel: database.UUIDModel{ID: "wrong-question-role"}, ConversationID: f.conversation, TurnID: &f.answer, Role: "assistant"},
		{UUIDModel: database.UUIDModel{ID: "user-with-turn"}, ConversationID: f.conversation, TurnID: &f.question, Role: "user"},
	} {
		if err := f.db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	// A resposta aparece nas duas seleções; UNION deve contá-la uma única vez.
	sources, err := database.ReadBoundedTimelineSources(f.db.WithContext(f.ctx), f.conversation, []string{f.question, f.answer}, []string{f.question, f.answer, "missing-question"})
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(sources))
	for _, source := range sources {
		ids = append(ids, source.ID)
	}
	slices.Sort(ids)
	want := []string{f.question, f.answer}
	slices.Sort(want)
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("fontes fora da seleção ou duplicadas: %v != %v", ids, want)
	}
}

func TestPublicationBudgetPlanStartsFromSelectedTurnMessages(t *testing.T) {
	f := newSnapshotFixture(t)
	var rows []struct{ Detail string }
	var query string
	var args []interface{}
	if err := f.db.Callback().Row().After("gorm:row").Register("source_budget_plan", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "admitted_sources") {
			query = tx.Statement.SQL.String()
			args = append([]interface{}(nil), tx.Statement.Vars...)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ReadBoundedTimelineSources(f.db.WithContext(f.ctx), f.conversation, nil, []string{f.question}); err != nil {
		t.Fatal(err)
	}
	if query == "" {
		t.Fatal("leitura limitada não capturada")
	}
	if err := f.db.Raw("EXPLAIN QUERY PLAN "+query, args...).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows {
		t.Log(row.Detail)
		if strings.Contains(row.Detail, "SEARCH m USING") && strings.Contains(row.Detail, "(turn_id=?)") {
			found = true
		}
		if (row.Detail == "SCAN m" || strings.HasPrefix(row.Detail, "SCAN m ")) || (strings.Contains(row.Detail, "SEARCH m USING") && !strings.Contains(row.Detail, "(turn_id=?)")) {
			t.Fatalf("orçamento varre fontes fora dos turnos selecionados: %s", row.Detail)
		}
	}
	if !found {
		t.Fatal("plano não usa busca indexada pelas identidades de turno")
	}
}

func TestPublicationBudgetCombinesMessagesAndToolsBeforeLoading(t *testing.T) {
	f := newSnapshotFixture(t)
	large := strings.Repeat("x", canonicalSourceBytes/2)
	for _, test := range []struct {
		name, content, preview string
		exceeds                bool
	}{
		{"mensagem", large, "curto", false},
		{"ferramenta", "curto", large, false},
		{"soma", large, large, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := f.db.Model(&database.ChatMessage{}).Where("id=?", f.answer).Update("content", test.content).Error; err != nil {
				t.Fatal(err)
			}
			if err := f.db.Model(&database.ToolInvocation{}).Where("id=?", f.invocation).Update("output_preview", test.preview).Error; err != nil {
				t.Fatal(err)
			}
			_, err := database.ReadBoundedTimelineSources(f.db.WithContext(f.ctx), f.conversation, nil, []string{f.question})
			if test.exceeds {
				if !errors.Is(err, ErrTimelineSourceLimit) {
					t.Fatal("soma acima do teto foi admitida", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBoundedSourcesKeepLedgerBudgetWithoutMessageRows(t *testing.T) {
	f := newSnapshotFixture(t)
	if err := f.db.Exec("DELETE FROM chat_messages WHERE id=?", f.answer).Error; err != nil {
		t.Fatal(err)
	}
	tx := f.db.WithContext(f.ctx)
	if messages, err := database.ReadBoundedTimelineSources(tx, f.conversation, nil, []string{f.question}); err != nil || len(messages) != 0 {
		t.Fatal(len(messages), err)
	}
	if err := f.db.Model(&database.ToolInvocation{}).Where("id=?", f.invocation).Update("output_preview", strings.Repeat("x", canonicalSourceBytes+1)).Error; err != nil {
		t.Fatal(err)
	}
	if messages, err := database.ReadBoundedTimelineSources(tx, f.conversation, nil, []string{f.question}); messages != nil || !errors.Is(err, ErrTimelineSourceLimit) {
		t.Fatal("ledger sem mensagens escapou da cota", len(messages), err)
	}
}

func TestBoundedSourceLedgerBudgetUsesTurnConversationAndKeepsLegacyCalls(t *testing.T) {
	f := newSnapshotFixture(t)
	other, err := database.CreateConversationWithContext(f.ctx, "other", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.Model(&database.ToolInvocation{}).Where("id=?", f.invocation).Update("output_preview", strings.Repeat("x", canonicalSourceBytes+1)).Error; err != nil {
		t.Fatal(err)
	}
	tx := f.db.WithContext(f.ctx)
	if sources, err := database.ReadBoundedTimelineSources(tx, other.ID, nil, []string{f.question}); err != nil || len(sources) != 0 {
		t.Fatal("turno estrangeiro consumiu orçamento", len(sources), err)
	}
	if err := f.db.Model(&database.ToolInvocation{}).Where("id=?", f.invocation).Update("conversation_id", nil).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := database.ReadBoundedTimelineSources(tx, f.conversation, nil, []string{f.question}); !errors.Is(err, ErrTimelineSourceLimit) {
		t.Fatal("ledger legado do turno local escapou da cota", err)
	}
}

func TestSelectionSourceBudgetCoversAllBatchesAndScopesBeforeMetadata(t *testing.T) {
	f := newSnapshotFixture(t)
	rows := make([]database.ChatMessage, 401)
	for i := range rows {
		rows[i] = database.ChatMessage{ConversationID: f.conversation, Role: "user", Content: strings.Repeat("x", 21000)}
		if i%2 != 0 {
			rows[i].ParentID = &f.question
		}
	}
	if err := f.db.CreateInBatches(&rows, 100).Error; err != nil {
		t.Fatal(err)
	}
	refs := make([]string, len(rows))
	for i, row := range rows {
		refs[i] = "message:" + row.ID
	}
	if _, err := f.reader.ReadSelection(f.ctx, f.conversation, refs[:200]); err != nil {
		t.Fatal("metade deveria caber", err)
	}
	metadataReads := 0
	if err := f.db.Callback().Query().After("gorm:query").Register("bounded_source_metadata", func(tx *gorm.DB) {
		if strings.Join(tx.Statement.Selects, ",") == "id,parent_id" {
			metadataReads++
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.reader.ReadSelection(f.ctx, f.conversation, refs); !errors.Is(err, ErrTimelineSourceLimit) || metadataReads != 0 {
		t.Fatal("cota dividida por lote/escopo", err, metadataReads)
	}
}
