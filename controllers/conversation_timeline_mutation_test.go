package controllers

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/database"
)

type commandTimelineReader struct {
	*chat.DBTimelineReader
	selected *chat.TimelineSelectionSnapshot
	readErr  error
	reads    int
}

func (r *commandTimelineReader) ReadSelection(ctx context.Context, conversationID string, ids []string) (*chat.TimelineSelectionSnapshot, error) {
	r.reads++
	r.selected, r.readErr = r.DBTimelineReader.ReadSelection(ctx, conversationID, ids)
	return r.selected, r.readErr
}

type commandTimelineSession struct{ ctx context.Context }

func (s commandTimelineSession) AuthorizeTimeline(context.Context) (context.Context, string, error) {
	return s.ctx, "command-test", nil
}

type commandTimelineEmitter struct{ emit func(string) }

func (e commandTimelineEmitter) Emit(name string, _ any) { e.emit(name) }

func TestMessageMutationsReconcileBeforeEvents(t *testing.T) {
	for _, mode := range []string{"command-edit", "command-pin", "command-delete", "legacy-edit", "legacy-pin", "legacy-delete", "batch-partial"} {
		t.Run(mode, func(t *testing.T) {
			restore := database.SetDB(database.DB())
			if err := database.InitPath(filepath.Join(t.TempDir(), "message-timeline.db")); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = database.Close(); restore() })
			db := database.DB()
			if err := db.Create(&database.User{UUIDModel: database.UUIDModel{ID: "owner"}, Username: "owner"}).Error; err != nil {
				t.Fatal(err)
			}
			ctx := database.WithUserID(context.Background(), "owner")
			conv, err := database.CreateConversationWithContext(ctx, "timeline", "")
			if err != nil {
				t.Fatal(err)
			}
			question, err := database.CreateMessageWithContext(ctx, database.MessageOptions{ConversationID: conv.ID, Role: "user", Content: "pergunta"})
			if err != nil {
				t.Fatal(err)
			}
			answer, err := database.CreateMessageWithContext(ctx, database.MessageOptions{ConversationID: conv.ID, TurnID: &question.ID, Role: "assistant", Content: "resposta"})
			if err != nil {
				t.Fatal(err)
			}
			if err := database.NewChatTurnRepository(db).ReconcileImportedConversationInTransaction(ctx, conv.ID); err != nil {
				t.Fatal(err)
			}
			r := &commandTimelineReader{DBTimelineReader: chat.NewDBTimelineReader(db)}
			p := chat.NewLiveProjection(r)
			sub, _, err := p.SubscribeTimeline(ctx, commandTimelineSession{ctx}, "history", database.MessageWindowQuery{ConversationID: conv.ID, Anchor: "end", Direction: "before", Limit: 20}, r)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			insideMutation, insideMaintenance := false, false
			mutations, events := 0, 0
			controller := NewConversationsController(ConversationsControllerConfig{
				WithMaintenance: func(ctx context.Context, write func() error) error {
					return database.WithSQLiteMaintenance(ctx, func() error { insideMaintenance = true; defer func() { insideMaintenance = false }(); return write() })
				},
				MutateTimeline: func(ctx context.Context, id string, known []string, write func() ([]string, error)) error {
					if strings.HasPrefix(mode, "command-") && !insideMaintenance {
						t.Error("projetor adquirido antes da manutenção")
					}
					insideMutation = true
					mutations++
					defer func() { insideMutation = false }()
					return p.ApplyStorageMutation(ctx, id, known, write)
				},
				Emitter: commandTimelineEmitter{func(string) {
					events++
					if insideMutation || r.reads != 1 || r.readErr != nil {
						t.Error("evento antes de concluir reconciliação", r.readErr)
					}
				}},
			})
			guard := func(write func() error) error { return write() }
			target := question.ID
			if mode == "command-pin" {
				target = answer.ID
			}
			revision, err := database.MessageCommandSnapshotWithContext(ctx, conv.ID, target, mode == "command-delete")
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "command-edit":
				err = controller.UpdateMessageCommandGuarded(ctx, conv.ID, question.ID, revision, "editada", guard)
			case "command-pin":
				err = controller.CommitMessageCommandGuarded(ctx, conv.ID, answer.ID, revision, false, guard)
			case "command-delete":
				err = controller.CommitMessageCommandGuarded(ctx, conv.ID, question.ID, revision, true, guard)
			case "legacy-edit":
				err = controller.UpdateMessage(ctx, answer.ID, "editada")
			case "legacy-pin":
				_, err = controller.ToggleMessagePin(ctx, answer.ID)
			case "legacy-delete":
				err = controller.DeleteMessage(ctx, question.ID)
			case "batch-partial":
				err = controller.DeleteMessages(ctx, conv.ID, []string{question.ID, "missing"})
			}
			if mode == "batch-partial" {
				if err == nil || events != 0 {
					t.Fatal("erro parcial não preservado", err)
				}
			} else if err != nil || events != 1 {
				t.Fatal("resultado/evento", events, err)
			}
			if mutations != 1 || r.reads != 1 || r.readErr != nil || r.selected == nil {
				t.Fatal("mutação não reconciliada", mutations, r.reads, r.readErr)
			}
			deleting := strings.Contains(mode, "delete") || mode == "batch-partial"
			if deleting {
				if len(r.selected.Items) != 0 {
					t.Fatal("exclusão não visível na seleção")
				}
				return
			}
			itemID := "turn:" + question.ID
			if mode == "command-edit" {
				itemID = "message:" + question.ID
			}
			var item *chat.TimelineItem
			for _, candidate := range r.selected.Items {
				if candidate.ItemID == itemID {
					item = candidate
				}
			}
			if item == nil {
				t.Fatal("item visual afetado não selecionado")
			}
			if strings.Contains(mode, "pin") {
				if item.Representative == nil || !item.Representative.Pinned {
					t.Fatal("fixação não projetada")
				}
			} else {
				var text strings.Builder
				for _, segment := range item.Segments {
					if segment.Kind == "text" {
						text.WriteString(segment.Text)
					}
				}
				if text.String() != "editada" {
					t.Fatal("edição não projetada", text.String())
				}
			}
		})
	}
}

func TestMessageMutationDeniedGuardDoesNotEnterProjector(t *testing.T) {
	_, ctx, conversationID, messageID := clearControllerFixture(t)
	denied := errors.New("guard denied")
	controller := NewConversationsController(ConversationsControllerConfig{MutateTimeline: func(context.Context, string, []string, func() ([]string, error)) error {
		t.Fatal("guard recusado chegou ao projetor")
		return nil
	}})
	err := controller.CommitMessageCommandGuarded(ctx, conversationID, messageID, "unused", false, func(func() error) error { return denied })
	if !errors.Is(err, denied) {
		t.Fatal(err)
	}
}
