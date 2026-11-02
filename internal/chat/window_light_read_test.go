package chat

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"assistente/internal/database"
	"gorm.io/gorm"
)

func TestCanonicalWindowDoesNotMaterializeMediaOrParentPayload(t *testing.T) {
	for _, thread := range []bool{false, true} {
		t.Run(map[bool]string{false: "root", true: "thread"}[thread], func(t *testing.T) {
			f := newSnapshotFixture(t)
			query := database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}
			selected := f.answer
			if thread {
				child, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, ParentID: &f.question, Role: "user", Content: "filha"})
				if err != nil {
					t.Fatal(err)
				}
				selected, query.ParentID = child.ID, &f.question
			}
			before, err := f.reader.ReadCanonicalWindow(f.ctx, query)
			if err != nil {
				t.Fatal(err)
			}
			large := strings.Repeat("x", 2*1024*1024)
			media := `[{"type":"audio","data":"` + large + `"}]`
			for _, id := range []string{f.question, selected} {
				if err := f.db.Model(&database.ChatMessage{}).Where("id=?", id).Updates(map[string]any{"audio": large, "media": media, "audio_mime_type": "audio/wav"}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if thread {
				if err := f.db.Model(&database.ChatMessage{}).Where("id=?", f.question).Update("content", large).Error; err != nil {
					t.Fatal(err)
				}
			}
			var payloadSQL []string
			parents := 0
			if err := f.db.Callback().Query().After("gorm:query").Register("window_light_columns", func(tx *gorm.DB) {
				if strings.Contains(tx.Statement.SQL.String(), "selected_payload") {
					payloadSQL = append(payloadSQL, tx.Statement.SQL.String())
				}
				if m, ok := tx.Statement.Dest.(*database.MessageScope); ok && thread {
					parents++
					if m.ConversationID != f.conversation || m.HasParent || strings.Join(tx.Statement.Selects, ",") != "chat_messages.conversation_id, chat_messages.parent_id IS NOT NULL AS has_parent" {
						t.Error("validação do pai carregou payload")
					}
				}
			}); err != nil {
				t.Fatal(err)
			}
			after, err := f.reader.ReadCanonicalWindow(f.ctx, query)
			if err != nil {
				t.Fatal(err)
			}
			if err := f.db.Callback().Query().Remove("window_light_columns"); err != nil {
				t.Fatal(err)
			}
			if len(payloadSQL) == 0 || (thread && parents == 0) {
				t.Fatal("consultas não verificadas", parents, payloadSQL)
			}
			for _, sql := range payloadSQL {
				if strings.Contains(sql, "audio") || strings.Contains(sql, "media") {
					t.Fatal("SQL materializa blobs", sql)
				}
			}
			if len(after.Items) != len(before.Items) {
				t.Fatal("leitura leve mudou participação")
			}
			for i, item := range after.Items {
				if !reflect.DeepEqual(canonicalItem(before.Items[i]), canonicalItem(item)) {
					t.Fatal("leitura leve mudou o DTO", item.ItemID)
				}
				for _, m := range item.messages {
					if m.Audio != "" || m.Media != "" || m.AudioMimeType != "" {
						t.Fatal("mensagem auxiliar reteve blobs")
					}
				}
				if item.Representative != nil && (item.Representative.Audio != "" || item.Representative.Media != "") {
					t.Fatal("representante reteve blobs")
				}
			}
			legacy, err := f.reader.ReadWindow(f.ctx, query)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, item := range legacy.Items {
				if item.Representative != nil && item.Representative.ID == selected {
					found = true
					if item.Representative.Audio != large || item.Representative.Media != media {
						t.Fatal("caminho legado perdeu mídia")
					}
				}
			}
			if !found {
				t.Fatal("representante legado não encontrado")
			}
			stored, err := database.GetMessageWithContext(f.ctx, selected)
			if err != nil || stored.Audio != large || stored.Media != media {
				t.Fatal("leitura alterou armazenamento", err)
			}
			if thread {
				nested := query
				nested.ParentID = &selected
				if _, err := f.reader.ReadCanonicalWindow(f.ctx, nested); err == nil {
					t.Fatal("metadados incompletos aceitaram pai que não é raiz")
				}
			}
			foreign := database.WithUserID(context.Background(), "other-user")
			if _, err := f.reader.ReadCanonicalWindow(foreign, query); err == nil {
				t.Fatal("leitura leve ignorou ownership")
			}
		})
	}
}

func TestCanonicalWindowSkipsContinuationMediaResolution(t *testing.T) {
	f := newSnapshotFixture(t)
	media := `[{"type":"image","url":"data:image/png;base64,` + strings.Repeat("x", 2*1024*1024) + `"}]`
	if err := f.db.Model(&database.ChatMessage{}).Where("id=?", f.answer).Update("media", media).Error; err != nil {
		t.Fatal(err)
	}
	turns := database.NewChatTurnRepository(f.db)
	execution, err := turns.Begin(f.ctx, f.conversation, f.question, f.answer, database.TurnModeContinue)
	if err != nil {
		t.Fatal(err)
	}
	answer := "continued-response"
	if err := f.db.Table("chat_messages").Create(map[string]any{"id": answer, "conversation_id": f.conversation, "turn_id": f.question, "role": "assistant", "content": "continuação", "chat_execution_id": execution.ExecutionID, "created_at": time.Now(), "updated_at": time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec("UPDATE chat_turn_attempts SET response_message_id=? WHERE execution_id=?", answer, execution.ExecutionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec("UPDATE chat_turns SET response_message_id=? WHERE user_message_id=?", answer, f.question).Error; err != nil {
		t.Fatal(err)
	}
	if err := turns.Finish(f.ctx, f.conversation, f.question, execution.ExecutionID, database.TurnCompleted, "completed"); err != nil {
		t.Fatal(err)
	}
	resolutions := 0
	if err := f.db.Callback().Row().After("gorm:row").Register("window_continuation_media", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "WITH RECURSIVE bases") {
			resolutions++
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.db.Callback().Row().Remove("window_continuation_media") })
	query := database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}
	canonical, err := f.reader.ReadCanonicalWindow(f.ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if resolutions != 0 || canonical.Responses[f.question].Representative.Media != "" {
		t.Fatal("janela canônica resolveu mídia herdada")
	}
	legacy, err := f.reader.ReadWindow(f.ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if resolutions == 0 || legacy.Responses[f.question].Representative.Media != media {
		t.Fatal("janela legada perdeu mídia de continuação")
	}
	if !reflect.DeepEqual(canonicalItem(canonical.Responses[f.question]), canonicalItem(legacy.Responses[f.question])) {
		t.Fatal("exclusão de mídia alterou DTO canônico")
	}
}

func BenchmarkCanonicalWindowLargeMedia(b *testing.B) {
	f := newSnapshotFixture(b)
	large := strings.Repeat("x", 2*1024*1024)
	if err := f.db.Model(&database.ChatMessage{}).Where("conversation_id=?", f.conversation).Updates(map[string]any{"audio": large, "media": large}).Error; err != nil {
		b.Fatal(err)
	}
	query := database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}
	for _, canonical := range []bool{false, true} {
		b.Run(map[bool]string{false: "legacy", true: "canonical"}[canonical], func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				snapshot, err := f.reader.readWindow(f.ctx, query, canonical)
				if err != nil {
					b.Fatal(err)
				}
				views := make([]CanonicalItem, 0, len(snapshot.Items))
				for _, item := range snapshot.Items {
					views = append(views, canonicalItem(item))
				}
				payload, err := json.Marshal(views)
				if err != nil {
					b.Fatal(err)
				}
				b.ReportMetric(float64(len(payload)), "DTO-bytes")
			}
		})
	}
}
