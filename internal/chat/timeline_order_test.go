package chat

import (
	"fmt"
	"reflect"
	"testing"
	"time"
	"unicode"

	"assistente/internal/database"
)

func TestTimelineOrderSnapshotKeepsSQLClassificationAndIndexes(t *testing.T) {
	f := newSnapshotFixture(t)
	base := time.Date(2026, 10, 31, 1, 0, 0, 123456789, time.UTC)
	if err := f.db.Model(&database.ChatMessage{}).Where("id = ?", f.question).Updates(map[string]any{"created_at": base, "turn_id": f.question}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Model(&database.ChatMessage{}).Where("id = ?", f.answer).Update("created_at", base.Add(-time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	missing := "missing-question"
	orphan := database.ChatMessage{UUIDModel: database.UUIDModel{ID: "orphan", CreatedAt: base.Add(time.Hour)}, ConversationID: f.conversation, Role: "assistant", TurnID: &missing, Content: "legado preservado"}
	if err := f.db.Create(&orphan).Error; err != nil {
		t.Fatal(err)
	}
	before := f.window(t)
	want := []string{"message:" + f.question, "turn:" + f.question, "message:" + orphan.ID}
	got := make([]string, 0, len(before.Items))
	for _, node := range before.Items {
		got = append(got, node.ItemID)
	}
	if !reflect.DeepEqual(got, want) || before.Window.TotalCount != 3 || before.InvalidTurnReferences != 1 {
		t.Fatalf("projeção divergiu da classificação: %q %+v", got, before)
	}
	if before.Items[2].Representative.TurnID != nil || before.Responses[missing] != nil {
		t.Fatal("órfã reagrupada pelo adaptador")
	}
	onlyResponse, err := f.reader.ReadCanonicalWindow(f.ctx, database.MessageWindowQuery{ConversationID: f.conversation, AnchorMessageID: f.question, Direction: "after", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(onlyResponse.Items) != 1 || onlyResponse.Items[0].ItemID != want[1] || onlyResponse.Window.StartIndex != 1 || onlyResponse.Window.EndIndex != 1 {
		t.Fatalf("janela parcial: %+v", onlyResponse.Window)
	}
	onlyQuestion, err := f.reader.ReadCanonicalWindow(f.ctx, database.MessageWindowQuery{ConversationID: f.conversation, AnchorMessageID: f.answer, Direction: "before", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(onlyQuestion.Items) != 1 || onlyQuestion.Items[0].ItemID != want[0] {
		t.Fatalf("âncora da resposta: %+v", onlyQuestion.Window)
	}
	after := f.window(t)
	if before.Version != after.Version {
		t.Fatal("leitura/classificação alterou a versão persistida")
	}
	var stored database.ChatMessage
	if err := f.db.First(&stored, "id = ?", orphan.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.TurnID == nil || *stored.TurnID != missing {
		t.Fatal("leitura alterou vínculo legado")
	}
}

func TestTimelineOrderSnapshotWhitespaceLinksMatchGoClassification(t *testing.T) {
	f := newSnapshotFixture(t)
	count := 0
	// Exercita a tabela Unicode inteira usada por strings.TrimSpace, inclusive
	// tab/NBSP, com a pergunta referenciada realmente existente no banco.
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if !unicode.IsSpace(r) {
			continue
		}
		id := string(r)
		rows := []database.ChatMessage{
			{UUIDModel: database.UUIDModel{ID: id}, ConversationID: f.conversation, Role: "user"},
			{UUIDModel: database.UUIDModel{ID: fmt.Sprintf("blank-link-%d", r)}, ConversationID: f.conversation, Role: "assistant", TurnID: &id, Content: "preservada"},
		}
		if err := f.db.Create(&rows).Error; err != nil {
			t.Fatal(err)
		}
		count++
	}
	snapshot, err := f.reader.ReadCanonicalWindow(f.ctx, database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "start", Direction: "after", Limit: 240})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Items) != 2+2*count || snapshot.Window.TotalCount != len(snapshot.Items) || snapshot.InvalidTurnReferences != count {
		t.Fatalf("classificações SQL/Go divergiram: nodes=%d total=%d inválidos=%d", len(snapshot.Items), snapshot.Window.TotalCount, snapshot.InvalidTurnReferences)
	}
	for _, node := range snapshot.Items {
		if node.Representative.Content == "preservada" && (node.ItemID != "message:"+node.Representative.ID || node.Representative.TurnID != nil) {
			t.Fatalf("vínculo vazio agrupado: %+v", node)
		}
	}
}
