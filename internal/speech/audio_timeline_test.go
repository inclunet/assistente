package speech_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/database"
	"assistente/internal/speech"
	"assistente/internal/testutil/dbfixture"
	"gorm.io/gorm"
)

type audioTimelineReader struct {
	*chat.DBTimelineReader
	reads    int
	selected *chat.TimelineSelectionSnapshot
}

func (r *audioTimelineReader) ReadSelection(ctx context.Context, id string, ids []string) (*chat.TimelineSelectionSnapshot, error) {
	r.reads++
	var err error
	r.selected, err = r.DBTimelineReader.ReadSelection(ctx, id, ids)
	return r.selected, err
}

type audioTimelineSession struct{ ctx context.Context }

func (s audioTimelineSession) AuthorizeTimeline(context.Context) (context.Context, string, error) {
	return s.ctx, "audio-test", nil
}

func TestAudioPersistenceCoordinatesTimelineWithoutReadingPayload(t *testing.T) {
	for _, mode := range []string{"standalone", "turn", "blank-link"} {
		t.Run(mode, func(t *testing.T) {
			dbfixture.Open(t)
			db := database.DB()
			if err := db.Create(&database.User{UUIDModel: database.UUIDModel{ID: "owner"}, Username: "owner"}).Error; err != nil {
				t.Fatal(err)
			}
			ctx := database.WithUserID(context.Background(), "owner")
			conv, err := database.CreateConversationWithContext(ctx, "áudio", "")
			if err != nil {
				t.Fatal(err)
			}
			question, err := database.CreateMessageWithContext(ctx, database.MessageOptions{ConversationID: conv.ID, Role: "user", Content: "pergunta"})
			if err != nil {
				t.Fatal(err)
			}
			message := database.ChatMessage{ConversationID: conv.ID, Role: "assistant", Content: "texto"}
			if mode == "turn" {
				message.TurnID = &question.ID
			}
			if mode == "blank-link" {
				blank := " \t "
				message.TurnID = &blank
			}
			if err := db.Create(&message).Error; err != nil {
				t.Fatal(err)
			}
			if err := database.SaveMessageAudioWithContext(ctx, message.ID, strings.Repeat("x", 2*1024*1024), "audio/wav"); err != nil {
				t.Fatal(err)
			}
			if err := database.NewChatTurnRepository(db).ReconcileImportedConversationInTransaction(ctx, conv.ID); err != nil {
				t.Fatal(err)
			}
			r := &audioTimelineReader{DBTimelineReader: chat.NewDBTimelineReader(db)}
			p := chat.NewLiveProjection(r)
			sub, before, err := p.SubscribeTimeline(ctx, audioTimelineSession{ctx}, "audio", database.MessageWindowQuery{ConversationID: conv.ID, Anchor: "end", Direction: "before", Limit: 20}, r)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			preRead := false
			if err := db.Callback().Query().After("gorm:query").Register("audio_identity", func(tx *gorm.DB) {
				if m, ok := tx.Statement.Dest.(*database.ChatMessage); ok && m.ID == message.ID && strings.Join(tx.Statement.Selects, ",") == "chat_messages.id,chat_messages.conversation_id,chat_messages.turn_id,chat_messages.parent_id" {
					preRead = true
					if m.Audio != "" || m.Content != "" || m.Media != "" {
						t.Error("pré-leitura carregou payload")
					}
				}
			}); err != nil {
				t.Fatal(err)
			}
			calls := 0
			store := speech.NewDBAudioStore(func(ctx context.Context, id string, ids []string, write func() ([]string, error)) error {
				calls++
				return p.ApplyStorageMutation(ctx, id, ids, write)
			})
			if err := store.SaveMessageAudio(ctx, message.ID, "bm92by3DoXVkaW8=", "audio/mpeg"); err != nil {
				t.Fatal(err)
			}
			if !preRead || calls != 1 || r.reads != 1 || r.selected == nil || len(r.selected.Items) != len(before.Items) {
				t.Fatal("áudio não reconciliado antes do retorno")
			}
			expected := "message:" + message.ID
			if mode == "turn" {
				expected = "turn:" + question.ID
			}
			// Todos os participantes são reconciliados na mesma versão, sem áudio.
			var selected *chat.TimelineItem
			seen := map[string]bool{}
			for _, item := range r.selected.Items {
				seen[item.ItemID] = true
				if item.Representative != nil && item.Representative.Audio != "" {
					t.Fatal("participante carregou áudio")
				}
				if item.ItemID == expected {
					selected = item
				}
			}
			for _, item := range before.Items {
				if !seen[item.ItemID] {
					t.Fatal("participante omitido")
				}
			}
			if selected == nil || selected.Representative == nil || selected.Representative.Audio != "" {
				t.Fatal("identidade ou omissão de áudio alteradas")
			}
			audio, mime, err := store.GetMessageAudio(ctx, message.ID)
			if err != nil || audio != "bm92by3DoXVkaW8=" || mime != "audio/mpeg" {
				t.Fatal("áudio não persistido", err)
			}
			if _, err := sub.Refresh(ctx, "1", nil); err != nil {
				t.Fatal("inscrição bloqueada após gravação", err)
			}
			callsBefore := calls
			if err := store.SaveMessageAudio(database.WithUserID(context.Background(), "other"), message.ID, "indevido", "audio/wav"); err == nil || calls != callsBefore {
				t.Fatal("recusa de ownership chegou ao coordenador")
			}
			failure := errors.New("escrita de áudio recusada")
			if err := db.Callback().Raw().Before("gorm:raw").Register("audio_failure", func(tx *gorm.DB) {
				if strings.Contains(tx.Statement.SQL.String(), "INSERT INTO chat_message_attachments") {
					_ = tx.AddError(failure)
				}
			}); err != nil {
				t.Fatal(err)
			}
			if err := store.SaveMessageAudio(ctx, message.ID, "bmFvIGNvbmZpcm1hcg==", "audio/wav"); !errors.Is(err, failure) {
				t.Fatal("erro original perdido", err)
			}
			if err := db.Callback().Raw().Remove("audio_failure"); err != nil {
				t.Fatal(err)
			}
			audio, _, err = store.GetMessageAudio(ctx, message.ID)
			if err != nil || audio != "bm92by3DoXVkaW8=" {
				t.Fatal("falha alterou áudio confirmado", err)
			}
		})
	}
}

func TestAudioPersistenceRequiresCoordinator(t *testing.T) {
	if err := speech.NewDBAudioStore(nil).SaveMessageAudio(context.Background(), "message", "audio", "audio/wav"); err == nil {
		t.Fatal("gravação sem coordenador aceita")
	}
}
