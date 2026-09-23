package app

import (
	"context"
	"testing"
	"time"

	"assistente/controllers"
	"assistente/internal/commandcatalog"
	"assistente/internal/commanddecision"
	"assistente/internal/commanddeck"
	"assistente/internal/commandui"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

func messageCommandSeed(t *testing.T, cid string) database.ChatMessage {
	t.Helper()
	m := database.ChatMessage{ConversationID: cid, Role: "user", Content: "private-message-batch91", Media: "private-media-batch91"}
	if err := database.DB().Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	return m
}

func TestCommandChatMessagePreparedEffects(t *testing.T) {
	for _, id := range commandChatMessageIDs {
		t.Run(id, func(t *testing.T) {
			a, events, cid := clearCommandFixture(t, workspace.TabTypeChat)
			m := messageCommandSeed(t, cid)
			r := beginUICommand(t, a, id)
			if commandInvocationCount(t, id) != 0 {
				t.Fatal("admitted before message preparation")
			}
			prepare := func() error {
				if id == commandMessageSendEditorID {
					_, err := a.PrepareChatEditorCommand(r.Ticket, m.ID, m.Content, "")
					return err
				}
				if id == commandMessageEditSaveID {
					return a.PrepareChatMessageEditCommand(r.Ticket, m.ID, m.Content, "private-edited-text-batch92")
				}
				return a.PrepareChatMessageCommand(r.Ticket, m.ID)
			}
			if err := prepare(); err != nil {
				t.Fatal(err)
			}
			if err := prepare(); err == nil {
				t.Fatal("preparation replay accepted")
			}
			if id == commandMessageDeleteID {
				payload := receiveCommandDecisionEvent(t, events)
				finishCommandDecision(t, a.questionnaireMgr, payload, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
			}
			h := takeUICommandFor(t, a, r.Ticket, id)
			if _, err := a.TakeUICommand(r.Ticket); err == nil {
				t.Fatal("Take replay accepted")
			}
			if isChatMessageBackendCommand(id) {
				if err := a.CompleteUICommand(r.Ticket, h.HandoffID, "succeeded"); err == nil {
					t.Fatal("backend bypass")
				}
				if err := a.CommitChatMessageCommand(r.Ticket, h.HandoffID); err != nil {
					t.Fatal(err)
				}
				if err := a.CommitChatMessageCommand(r.Ticket, h.HandoffID); err == nil {
					t.Fatal("commit replay")
				}
			} else {
				if err := a.CommitChatMessageCommand(r.Ticket, h.HandoffID); err == nil {
					t.Fatal("UI routed to durable commit")
				}
				if id == commandMessageSendEditorID {
					if _, err := a.OpenChatEditorCommand(r.Ticket, h.HandoffID, "Message"); err != nil {
						t.Fatal(err)
					}
				}
				if err := a.CompleteUICommand(r.Ticket, h.HandoffID, "succeeded"); err != nil {
					t.Fatal(err)
				}
			}
			if got := getUIResultEventually(t, a, r.Ticket); got.Status != "succeeded" {
				t.Fatalf("%+v", got)
			}
			var stored database.ChatMessage
			err := database.DB().First(&stored, "id = ?", m.ID).Error
			if id == commandMessageDeleteID {
				if err == nil {
					t.Fatal("message not deleted")
				}
			} else if err != nil || (id == commandMessagePinID && !stored.Pinned) {
				t.Fatalf("effect: %+v %v", stored, err)
			}
			assertChatActionLedgerRedacted(t, r.InvocationID, m.Content)
			assertChatActionLedgerRedacted(t, r.InvocationID, m.Media)
			assertChatActionLedgerRedacted(t, r.InvocationID, m.ID)
		})
	}
}

func TestCommandChatMessageStaleAndCancel(t *testing.T) {
	for _, scenario := range []string{"changed-before-take", "changed-after-take", "pin-aba", "decision-change", "decision-cancel", "cancel-before-prepare", "wrong-conversation", "foreign-owner", "source-generation", "tab-aba", "binding-change"} {
		t.Run(scenario, func(t *testing.T) {
			a, events, cid := clearCommandFixture(t, workspace.TabTypeChat)
			m := messageCommandSeed(t, cid)
			id := commandMessagePinID
			if scenario == "decision-change" || scenario == "decision-cancel" {
				id = commandMessageDeleteID
			}
			r := beginUICommand(t, a, id)
			if scenario == "cancel-before-prepare" {
				_, run, err := a.commandUIRun(r.Ticket)
				if err != nil {
					t.Fatal(err)
				}
				if err := a.CancelUICommand(r.Ticket); err != nil {
					t.Fatal(err)
				}
				if err := a.PrepareChatMessageCommand(r.Ticket, m.ID); err == nil {
					t.Fatal("cancelled preparation")
				}
				select {
				case <-run.done:
				case <-time.After(time.Second):
					t.Fatal("prepare waiter leaked")
				}
				return
			}
			if scenario == "wrong-conversation" || scenario == "foreign-owner" {
				owner := a.currentUserID
				if scenario == "foreign-owner" {
					owner = "foreign-user"
				}
				conv, err := database.CreateConversationWithContext(database.WithUserID(context.Background(), owner), "other", "")
				if err != nil {
					t.Fatal(err)
				}
				other := messageCommandSeed(t, conv.ID)
				if err := a.PrepareChatMessageCommand(r.Ticket, other.ID); err == nil {
					t.Fatal("foreign target accepted")
				}
				if err := a.PrepareChatMessageCommand(r.Ticket, m.ID); err == nil {
					t.Fatal("failed claim reusable")
				}
				return
			}
			if err := a.PrepareChatMessageCommand(r.Ticket, m.ID); err != nil {
				t.Fatal(err)
			}
			if scenario == "decision-change" || scenario == "decision-cancel" {
				payload := receiveCommandDecisionEvent(t, events)
				if scenario == "decision-change" {
					if err := database.DB().Model(&m).Update("content", "new-private-content").Error; err != nil {
						t.Fatal(err)
					}
				}
				finishCommandDecision(t, a.questionnaireMgr, payload, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, scenario == "decision-cancel")
				if _, err := a.TakeUICommand(r.Ticket); err == nil {
					t.Fatal("changed/cancelled decision delivered target")
				}
				return
			}
			if scenario == "changed-before-take" {
				if err := database.DB().Model(&m).Update("content", "new-content").Error; err != nil {
					t.Fatal(err)
				}
				if _, err := a.TakeUICommand(r.Ticket); err == nil {
					t.Fatal("stale take")
				}
				return
			}
			h := takeUICommandFor(t, a, r.Ticket, id)
			switch scenario {
			case "pin-aba":
				original := m.UpdatedAt
				for _, value := range []bool{true, false} {
					// Bypass GORM's timestamp update: rejection must come from
					// the durable revision, even with an identical final payload.
					if err := database.DB().Model(&m).UpdateColumn("pinned", value).Error; err != nil {
						t.Fatal(err)
					}
				}
				var current database.ChatMessage
				if err := database.DB().First(&current, "id = ?", m.ID).Error; err != nil {
					t.Fatal(err)
				}
				if current.Pinned || !current.UpdatedAt.Equal(original) {
					t.Fatal("ABA fixture must preserve pinned and updated_at")
				}
			case "source-generation":
				if err := a.commandHost.SetActiveLayers(context.Background(), a.currentUserID, []string{"changed"}); err != nil {
					t.Fatal(err)
				}
			case "binding-change":
				if err := a.workspaceMgr.UpdateTab(a.workspaceMgr.Active().Tabs.Active, map[string]any{"conversation_id": uuid.NewString()}); err != nil {
					t.Fatal(err)
				}
			case "tab-aba":
				original := a.workspaceMgr.Active().Tabs.Active
				other := uuid.NewString()
				if err := a.workspaceMgr.AddTab(workspace.Tab{ID: other, Type: workspace.TabTypeChat}); err != nil {
					t.Fatal(err)
				}
				if err := a.workspaceMgr.SetActiveTab(other); err != nil {
					t.Fatal(err)
				}
				if err := a.workspaceMgr.SetActiveTab(original); err != nil {
					t.Fatal(err)
				}
			default:
				if err := database.DB().Model(&m).Update("content", "changed").Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := a.CommitChatMessageCommand(r.Ticket, h.HandoffID); err == nil {
				t.Fatal("stale commit")
			}
		})
	}
}

func TestCommandChatMessageDeckPreservesSource(t *testing.T) {
	a := deckChatPickerFixture(t, commandMessageCopyID)
	if err := database.DB().AutoMigrate(&database.Conversation{}, &database.ChatMessage{}); err != nil {
		t.Fatal(err)
	}
	if err := database.MigrateMessageRevisions(database.DB()); err != nil {
		t.Fatal(err)
	}
	ctx := database.WithUserID(context.Background(), a.currentUserID)
	conversation, err := database.CreateConversationWithContext(ctx, "deck-chat", "")
	if err != nil {
		t.Fatal(err)
	}
	tabID := uuid.NewString()
	if err := a.workspaceMgr.AddTab(workspace.Tab{ID: tabID, Type: workspace.TabTypeChat, ConversationID: conversation.ID}); err != nil {
		t.Fatal(err)
	}
	if err := a.workspaceMgr.SetActiveTab(tabID); err != nil {
		t.Fatal(err)
	}
	a.conversationsCtrl = controllers.NewConversationsController(controllers.ConversationsControllerConfig{})
	if _, err := a.GetLocalCommandKeyboardMap(); err != nil {
		t.Fatal(err)
	}
	reservations := make(chan commandui.Reservation, 1)
	a.emitter = commandOSBootstrapEmitter(func(name string, value any) {
		if name == "command:deck-ui-reservation" {
			reservations <- value.(commandui.Reservation)
		}
	})
	driver := &appDeckDriver{opened: make(chan *appDeckHandle, 1)}
	deckCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.commandProduct.Load().startDeck(deckCtx, driver)
	var handle *appDeckHandle
	select {
	case handle = <-driver.opened:
	case <-time.After(4 * time.Second):
		t.Fatal("deck open")
	}
	if err := drainDeckStartupFrames(handle); err != nil {
		t.Fatal(err)
	}
	handle.events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: true}
	var r commandui.Reservation
	select {
	case r = <-reservations:
	case <-time.After(4 * time.Second):
		t.Fatal("deck reservation")
	}
	m := messageCommandSeed(t, conversation.ID)
	if err := a.PrepareChatMessageCommand(r.Ticket, m.ID); err != nil {
		t.Fatal(err)
	}
	h := takeUICommandFor(t, a, r.Ticket, commandMessageCopyID)
	if err := a.CompleteUICommand(r.Ticket, h.HandoffID, "succeeded"); err != nil {
		t.Fatal(err)
	}
	if result := getUIResultEventually(t, a, r.Ticket); result.Status != "succeeded" {
		t.Fatalf("%+v", result)
	}
	var source string
	if err := database.DB().Table("command_invocations").Select("source_type").Where("invocation_id = ?", r.InvocationID).Scan(&source).Error; err != nil || source != "streamdeck.key" {
		t.Fatalf("source=%s %v", source, err)
	}
}

func TestCommandChatMessageCatalog(t *testing.T) {
	a, _, _ := clearCommandFixture(t, workspace.TabTypeChat)
	p := a.commandProduct.Load()
	if len(p.registry.List()) != 149 {
		t.Fatal("catalog count")
	}
	for _, id := range commandChatMessageIDs {
		d, ok := p.registry.Lookup(id)
		if !ok || isLocalUICommand(id) || !localKeyboardCommandAllowed(id) || !commandDeckLedgerCommand(d) || d.Persistence.Arguments != commandcatalog.PersistenceNever || d.Persistence.Result != commandcatalog.PersistenceNever || !d.HasMutableTarget || d.Context.Facts[0].Mode != commandcatalog.ExactVersion {
			t.Fatalf("contract %s: %+v", id, d)
		}
		for _, locale := range []string{"pt-BR", "en", "es"} {
			if d.Presentation.Locales[locale].Name == "" {
				t.Fatal("missing label")
			}
		}
	}
	if commandUIRunTimeout(commandMessageDeleteID) != 5*time.Minute || commandUITakeTimeout(commandMessageDeleteID) != 5*time.Minute {
		t.Fatal("decision timeout")
	}
	if isChatMessageCommand(commandMessageEditID) || !isLocalUICommand(commandMessageEditID) {
		t.Fatal("edit must be local")
	}
	_, handlers, err := a.commandProductCatalog()
	if err != nil || handlers[commandMessageDeleteID].ExecutionTimeout != 5*time.Minute {
		t.Fatal("handler decision timeout", err)
	}
}

func TestCommandChatMessageAtomicMutationGuard(t *testing.T) {
	for _, deleting := range []bool{false, true} {
		t.Run(map[bool]string{false: "pin", true: "delete"}[deleting], func(t *testing.T) {
			a, _, cid := clearCommandFixture(t, workspace.TabTypeChat)
			m := messageCommandSeed(t, cid)
			ctx := database.WithUserID(context.Background(), a.currentUserID)
			fingerprint, err := database.MessageCommandSnapshotWithContext(ctx, cid, m.ID, deleting)
			if err != nil {
				t.Fatal(err)
			}
			if err := database.DB().Model(&m).Update("content", "changed-before-transaction").Error; err != nil {
				t.Fatal(err)
			}
			err = a.conversationsCtrl.CommitMessageCommandGuarded(ctx, cid, m.ID, fingerprint, deleting, func(commit func() error) error { return commit() })
			if err == nil {
				t.Fatal("stale transactional compare accepted")
			}
			var current database.ChatMessage
			if err := database.DB().First(&current, "id = ?", m.ID).Error; err != nil || current.Pinned || current.Content != "changed-before-transaction" {
				t.Fatalf("mutation leaked: %+v %v", current, err)
			}
		})
	}
}

func TestCommandChatMessageDeleteRejectsNewReplyAndActiveGeneration(t *testing.T) {
	for _, active := range []bool{false, true} {
		t.Run(map[bool]string{false: "new-reply", true: "active-generation"}[active], func(t *testing.T) {
			a, events, cid := clearCommandFixture(t, workspace.TabTypeChat)
			m := messageCommandSeed(t, cid)
			r := beginUICommand(t, a, commandMessageDeleteID)
			if err := a.PrepareChatMessageCommand(r.Ticket, m.ID); err != nil {
				t.Fatal(err)
			}
			payload := receiveCommandDecisionEvent(t, events)
			finishCommandDecision(t, a.questionnaireMgr, payload, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
			h := takeUICommandFor(t, a, r.Ticket, commandMessageDeleteID)
			if active {
				release, ok := a.streamMgr.ReserveConversation(cid)
				if !ok {
					t.Fatal("reserve")
				}
				defer release()
			} else {
				if err := database.DB().Create(&database.ChatMessage{ConversationID: cid, ParentID: &m.ID, Role: "assistant", Content: "new reply"}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := a.CommitChatMessageCommand(r.Ticket, h.HandoffID); err == nil {
				t.Fatal("unsafe deletion")
			}
			var count int64
			if err := database.DB().Model(&database.ChatMessage{}).Where("id = ?", m.ID).Count(&count).Error; err != nil || count != 1 {
				t.Fatal("target lost", err)
			}
		})
	}
}
