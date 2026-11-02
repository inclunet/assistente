package speech_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/database"
	"assistente/internal/speech"
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
			restore := database.SetDB(database.DB())
			if err := database.InitPath(filepath.Join(t.TempDir(), "audio.db")); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = database.Close(); restore() })
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
			message := database.ChatMessage{ConversationID: conv.ID, Role: "assistant", Content: "texto", Audio: strings.Repeat("x", 2*1024*1024), AudioMimeType: "audio/wav"}
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
			if err := database.NewChatTurnRepository(db).ReconcileImportedConversationInTransaction(ctx, conv.ID); err != nil {
				t.Fatal(err)
			}
			r := &audioTimelineReader{DBTimelineReader: chat.NewDBTimelineReader(db)}
			p := chat.NewLiveProjection(r)
			sub, _, err := p.SubscribeTimeline(ctx, audioTimelineSession{ctx}, "audio", database.MessageWindowQuery{ConversationID: conv.ID, Anchor: "end", Direction: "before", Limit: 20}, r)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			preRead := false
			if err := db.Callback().Query().After("gorm:query").Register("audio_identity", func(tx *gorm.DB) {
				if m, ok := tx.Statement.Dest.(*database.ChatMessage); ok && m.ID == message.ID && len(tx.Statement.Selects) == 3 {
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
			if err := store.SaveMessageAudio(ctx, message.ID, "novo-áudio", "audio/mpeg"); err != nil {
				t.Fatal(err)
			}
			if !preRead || calls != 1 || r.reads != 1 || len(r.selected.Items) != 1 {
				t.Fatal("áudio não reconciliado antes do retorno")
			}
			selected := r.selected.Items[0]
			expected := "message:" + message.ID
			if mode == "turn" {
				expected = "turn:" + question.ID
			}
			if selected.ItemID != expected || selected.Representative == nil || selected.Representative.Audio != "" {
				t.Fatal("identidade ou omissão de áudio alteradas")
			}
			audio, mime, err := store.GetMessageAudio(ctx, message.ID)
			if err != nil || audio != "novo-áudio" || mime != "audio/mpeg" {
				t.Fatal("áudio não persistido", err)
			}
			if _, err := sub.Refresh(ctx); err != nil {
				t.Fatal("inscrição bloqueada após gravação", err)
			}
			callsBefore := calls
			if err := store.SaveMessageAudio(database.WithUserID(context.Background(), "other"), message.ID, "indevido", "audio/wav"); err == nil || calls != callsBefore {
				t.Fatal("recusa de ownership chegou ao coordenador")
			}
			failure := errors.New("escrita de áudio recusada")
			if err := db.Callback().Update().Before("gorm:update").Register("audio_failure", func(tx *gorm.DB) {
				if tx.Statement.Table == "chat_messages" {
					_ = tx.AddError(failure)
				}
			}); err != nil {
				t.Fatal(err)
			}
			if err := store.SaveMessageAudio(ctx, message.ID, "não confirmar", "audio/wav"); !errors.Is(err, failure) {
				t.Fatal("erro original perdido", err)
			}
			if err := db.Callback().Update().Remove("audio_failure"); err != nil {
				t.Fatal(err)
			}
			audio, _, err = store.GetMessageAudio(ctx, message.ID)
			if err != nil || audio != "novo-áudio" {
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
