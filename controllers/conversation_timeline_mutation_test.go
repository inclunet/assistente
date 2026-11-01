package controllers

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/database"
	"gorm.io/gorm"
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
			var firstMessageColumns []string
			if err := db.Callback().Query().After("gorm:query").Register("mutation_identity_columns", func(tx *gorm.DB) {
				if firstMessageColumns == nil && tx.Statement.Table == "chat_messages" {
					firstMessageColumns = append([]string{}, tx.Statement.Selects...)
				}
			}); err != nil {
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
			if !strings.HasPrefix(mode, "command-") && strings.Join(firstMessageColumns, ",") != "chat_messages.id,chat_messages.conversation_id,chat_messages.turn_id" {
				t.Fatal("leitura de coordenação materializou payload", firstMessageColumns)
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

func TestLegacyPinReadsOnlyAuthorizedIdentityBeforeCoordination(t *testing.T) {
	restore := database.SetDB(database.DB())
	if err := database.InitPath(filepath.Join(t.TempDir(), "pin-identity.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close(); restore() })
	db := database.DB()
	if err := db.Create(&database.User{UUIDModel: database.UUIDModel{ID: "pin-owner"}, Username: "pin-owner"}).Error; err != nil {
		t.Fatal(err)
	}
	ctx := database.WithUserID(context.Background(), "pin-owner")
	conv, err := database.CreateConversationWithContext(ctx, "pin", "")
	if err != nil {
		t.Fatal(err)
	}
	question, err := database.CreateMessageWithContext(ctx, database.MessageOptions{ConversationID: conv.ID, Role: "user", Content: "pergunta"})
	if err != nil {
		t.Fatal(err)
	}
	answer, err := database.CreateMessageWithContext(ctx, database.MessageOptions{ConversationID: conv.ID, Role: "assistant", TurnID: &question.ID, Content: "resposta"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&database.ChatMessage{}).Where("id=?", answer.ID).Update("audio", strings.Repeat("x", 2<<20)).Error; err != nil {
		t.Fatal(err)
	}
	coordinated := false
	var columns []string
	if err := db.Callback().Query().After("gorm:query").Register("pin_identity_columns", func(tx *gorm.DB) {
		if !coordinated && tx.Statement.Table == "chat_messages" {
			columns = append(columns, tx.Statement.Selects...)
		}
	}); err != nil {
		t.Fatal(err)
	}
	c := NewConversationsController(ConversationsControllerConfig{MutateTimeline: func(_ context.Context, id string, known []string, write func() ([]string, error)) error {
		coordinated = true
		if id != conv.ID || len(known) != 2 || known[0] != "message:"+answer.ID || known[1] != "turn:"+question.ID {
			t.Fatal("vínculos perdidos", id, known)
		}
		_, err := write()
		return err
	}})
	pinned, err := c.ToggleMessagePin(ctx, answer.ID)
	if err != nil || !pinned.Pinned {
		t.Fatal("pin falhou", err)
	}
	if strings.Join(columns, ",") != "chat_messages.id,chat_messages.conversation_id,chat_messages.turn_id" {
		t.Fatal("pré-leitura carregou payload", columns)
	}
	for _, denied := range []context.Context{context.Background(), database.WithUserID(context.Background(), "other")} {
		coordinated = false
		if _, err := c.ToggleMessagePin(denied, answer.ID); err == nil || coordinated {
			t.Fatal("pré-leitura ultrapassou ownership")
		}
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

func TestMessageMutationPreservesOpaqueTurnIdentityAndIgnoresWhitespace(t *testing.T) {
	for _, link := range []string{" \t ", "\u2003\u00a0", " turno opaco "} {
		t.Run(link, func(t *testing.T) {
			restore := database.SetDB(database.DB())
			if err := database.InitPath(filepath.Join(t.TempDir(), "turn-identity.db")); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = database.Close(); restore() })
			db := database.DB()
			if err := db.Create(&database.User{UUIDModel: database.UUIDModel{ID: "owner"}, Username: "owner"}).Error; err != nil {
				t.Fatal(err)
			}
			ctx := database.WithUserID(context.Background(), "owner")
			conv, err := database.CreateConversationWithContext(ctx, "identidade", "")
			if err != nil {
				t.Fatal(err)
			}
			opaque := strings.TrimSpace(link) != ""
			if opaque {
				if err := db.Create(&database.ChatMessage{UUIDModel: database.UUIDModel{ID: link}, ConversationID: conv.ID, Role: "user", Content: "pergunta"}).Error; err != nil {
					t.Fatal(err)
				}
			}
			message := database.ChatMessage{ConversationID: conv.ID, Role: "assistant", TurnID: &link, Content: "antes"}
			if err := db.Create(&message).Error; err != nil {
				t.Fatal(err)
			}
			r := &commandTimelineReader{DBTimelineReader: chat.NewDBTimelineReader(db)}
			p := chat.NewLiveProjection(r)
			sub, _, err := p.SubscribeTimeline(ctx, commandTimelineSession{ctx}, "identity", database.MessageWindowQuery{ConversationID: conv.ID, Anchor: "end", Direction: "before", Limit: 20}, r)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			c := NewConversationsController(ConversationsControllerConfig{MutateTimeline: p.ApplyStorageMutation})
			if err := c.UpdateMessage(ctx, message.ID, "depois"); err != nil {
				t.Fatal(err)
			}
			if r.readErr != nil {
				t.Fatal("identidade inválida bloqueou reconciliação", r.readErr)
			}
			revision, err := database.MessageCommandSnapshotWithContext(ctx, conv.ID, message.ID, false)
			if err != nil {
				t.Fatal(err)
			}
			if err := c.CommitMessageCommandGuarded(ctx, conv.ID, message.ID, revision, false, func(write func() error) error { return write() }); err != nil {
				t.Fatal(err)
			}
			if r.readErr != nil {
				t.Fatal("fixação bloqueou reconciliação", r.readErr)
			}
			snapshot, err := sub.Refresh(ctx)
			if err != nil {
				t.Fatal("vínculo bloqueou publicação", err)
			}
			wanted := "message:" + message.ID
			if opaque {
				wanted = "turn:" + link
			}
			found := false
			for _, item := range snapshot.Items {
				if item.ItemID == wanted {
					var text strings.Builder
					for _, segment := range item.Segments {
						text.WriteString(segment.Text)
					}
					found = text.String() == "depois" && item.Metadata.Pinned
				}
			}
			if !found {
				t.Fatal("identidade/conteúdo/fixação não preservados", wanted)
			}
		})
	}
}
