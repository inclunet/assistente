package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"assistente/controllers"
	"assistente/internal/chat"
	"assistente/internal/commandcatalog"
	"assistente/internal/commanddeck"
	"assistente/internal/commandui"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

func chatActionFixture(t *testing.T) (*App, string, context.Context) {
	t.Helper()
	a, _, cid := clearCommandFixture(t, workspace.TabTypeChat)
	a.chatCtrl = controllers.NewChatController(controllers.ChatControllerConfig{})
	a.chatInteractor = chat.NewInteractor(chat.InteractorConfig{Repo: chat.NewDBMessageStore()})
	return a, cid, database.WithUserID(context.Background(), a.currentUserID)
}

func chatActionMetadata(t *testing.T, a *App, id string) (*llm.ChatCommandMetadata, string) {
	t.Helper()
	r := beginUICommand(t, a, id)
	h := takeUICommandFor(t, a, r.Ticket, id)
	return &llm.ChatCommandMetadata{Ticket: r.Ticket, HandoffID: h.HandoffID}, r.InvocationID
}

func TestCommandChatActionsAcceptanceLifetimeReplayAndRedaction(t *testing.T) {
	for _, id := range []string{commandChatSendID, commandChatRetryID} {
		t.Run(id, func(t *testing.T) {
			a, cid, ctx := chatActionFixture(t)
			retryID := ""
			if id == commandChatRetryID {
				message := database.ChatMessage{ConversationID: cid, Role: "user", Content: "private draft"}
				if err := database.DB().Create(&message).Error; err != nil {
					t.Fatal(err)
				}
				retryID = message.ID
			}
			metadata, invocationID := chatActionMetadata(t, a, id)
			calls := 0
			var turn context.Context
			next := func(got context.Context) (string, error) { calls++; turn = got; return cid, nil }
			if got, err := a.commitChatSubmission(ctx, metadata, cid, retryID, next); err != nil || got != cid {
				t.Fatalf("submit: %s %v", got, err)
			}
			if result := getUIResultEventually(t, a, metadata.Ticket); result.Status != "succeeded" {
				t.Fatalf("%+v", result)
			}
			if turn != ctx || turn.Err() != nil {
				t.Fatal("command finalization cancelled/replaced turn context")
			}
			if _, err := a.commitChatSubmission(ctx, metadata, cid, retryID, next); err == nil || calls != 1 {
				t.Fatal("replay")
			}
			assertChatActionLedgerRedacted(t, invocationID, "private draft")
		})
	}
}

func assertChatActionLedgerRedacted(t *testing.T, invocationID, private string) {
	t.Helper()
	for _, table := range []string{"command_invocations", "command_idempotency_keys"} {
		var rows []map[string]any
		if err := database.DB().Table(table).Where("invocation_id = ?", invocationID).Find(&rows).Error; err != nil {
			t.Fatal(err)
		}
		if len(rows) == 0 {
			t.Fatalf("missing audit %s", table)
		}
		for _, row := range rows {
			for _, value := range row {
				var text string
				switch v := value.(type) {
				case string:
					text = v
				case []byte:
					text = string(v)
				}
				if strings.Contains(text, private) {
					t.Fatalf("content in %s", table)
				}
			}
		}
	}
}

func TestCommandChatActionsPartialEffectIsOutcomeUnknown(t *testing.T) {
	a, cid, ctx := chatActionFixture(t)
	metadata, invocationID := chatActionMetadata(t, a, commandChatSendID)
	private := "private-content-provider-failure"
	effects := 0
	_, err := a.commitChatSubmission(ctx, metadata, cid, "", func(context.Context) (string, error) {
		effects++
		if err := database.DB().Create(&database.ChatMessage{ConversationID: cid, Role: "user", Content: private}).Error; err != nil {
			t.Fatal(err)
		}
		return "", errors.New(private)
	})
	if !errors.Is(err, commandui.ErrOutcomeUnknown) || strings.Contains(err.Error(), private) || effects != 1 {
		t.Fatalf("unsafe error: %v effects=%d", err, effects)
	}
	if result := getUIResultEventually(t, a, metadata.Ticket); result.Status != "outcome_unknown" {
		t.Fatalf("%+v", result)
	}
	assertChatActionLedgerRedacted(t, invocationID, private)
}

func TestCommandChatActionsKeyboardLoadingPreservesAdmission(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	if err := database.DB().AutoMigrate(&database.Conversation{}, &database.ChatMessage{}); err != nil {
		t.Fatal(err)
	}
	conversation, err := database.CreateConversationWithContext(database.WithUserID(context.Background(), a.currentUserID), "chat", "")
	if err != nil {
		t.Fatal(err)
	}
	cid := conversation.ID
	tabID := uuid.NewString()
	if err := a.workspaceMgr.AddTab(workspace.Tab{ID: tabID, Type: workspace.TabTypeChat, ConversationID: cid}); err != nil {
		t.Fatal(err)
	}
	if err := a.workspaceMgr.SetActiveTab(tabID); err != nil {
		t.Fatal(err)
	}
	a.streamMgr = chat.NewStreamingManager(nil)
	a.chatCtrl = controllers.NewChatController(controllers.ChatControllerConfig{})
	layer, _ := settingsActivationSecurityLayerAndRule(t, a, decisions)
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandBinding(CommandBindingEdit{LayerID: layer, CommandID: commandChatSendID, TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyK","modifiers":["Control","Shift"]}`, Enabled: true})
	})
	if _, err := a.SetCommandLayerActive(layer, true); err != nil {
		t.Fatal(err)
	}
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	key := LocalCommandShortcut{Version: 1, Code: "KeyK", Modifiers: []string{"Control", "Shift"}}
	if r, err := a.BeginLocalCommandUIKey(view.Generation, key, true); err == nil || r != nil {
		t.Fatal("repeat admitted")
	}
	r, err := a.BeginLocalCommandUIKey(view.Generation, key, false)
	if err != nil || r == nil {
		t.Fatalf("begin key: %+v %v", r, err)
	}
	// Loading is frontend presentation state. Re-publication/reading the map and
	// workspace snapshots must not rotate authority, unlike an actual tab change.
	a.workspaceMgr.Active()
	refreshed, err := a.GetLocalCommandKeyboardMap()
	if err != nil || refreshed.Generation != view.Generation {
		t.Fatalf("presentation read invalidated keyboard: %v", err)
	}
	h := takeUICommandFor(t, a, r.Ticket, commandChatSendID)
	ctx := database.WithUserID(context.Background(), a.currentUserID)
	if _, err := a.commitChatSubmission(ctx, &llm.ChatCommandMetadata{Ticket: r.Ticket, HandoffID: h.HandoffID}, cid, "", func(context.Context) (string, error) { return cid, nil }); err != nil {
		t.Fatal(err)
	}
	if result := getUIResultEventually(t, a, r.Ticket); result.Status != "succeeded" {
		t.Fatalf("%+v", result)
	}
	var source string
	if err := database.DB().Table("command_invocations").Select("source_type").Where("invocation_id = ?", r.InvocationID).Scan(&source).Error; err != nil || source != "keyboard.local" {
		t.Fatalf("source=%s %v", source, err)
	}
}

func TestCommandChatActionsRejectStaleAndForeignTargets(t *testing.T) {
	for _, scenario := range []string{"binding", "aba", "foreign", "conversation", "session", "retry-assistant", "retry-other-conversation", "forged", "missing"} {
		t.Run(scenario, func(t *testing.T) {
			a, cid, ctx := chatActionFixture(t)
			id, retryID := commandChatSendID, ""
			if strings.HasPrefix(scenario, "retry-") {
				id = commandChatRetryID
				m := database.ChatMessage{ConversationID: cid, Role: "assistant"}
				if scenario == "retry-other-conversation" {
					m.ConversationID = uuid.NewString()
					m.Role = "user"
				}
				if err := database.DB().Create(&m).Error; err != nil {
					t.Fatal(err)
				}
				retryID = m.ID
			}
			metadata, _ := chatActionMetadata(t, a, id)
			switch scenario {
			case "binding":
				if err := a.workspaceMgr.UpdateTab(a.workspaceMgr.Active().Tabs.Active, map[string]any{"conversation_id": uuid.NewString()}); err != nil {
					t.Fatal(err)
				}
			case "aba":
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
			case "foreign":
				if err := database.DB().Model(&database.Conversation{}).Where("id = ?", cid).Update("user_id", "foreign").Error; err != nil {
					t.Fatal(err)
				}
			case "conversation":
				cid = uuid.NewString()
			case "session":
				a.authMu.Lock()
				a.currentAuthUser.SessionID = uuid.NewString()
				a.authMu.Unlock()
			case "forged":
				metadata.HandoffID = uuid.NewString()
			case "missing":
				metadata = nil
			}
			called := false
			if _, err := a.commitChatSubmission(ctx, metadata, cid, retryID, func(context.Context) (string, error) { called = true; return cid, nil }); err == nil || called {
				t.Fatal("unsafe submission")
			}
		})
	}
}

func TestCommandChatCancelGenerationAndSerializationAbsence(t *testing.T) {
	for _, scenario := range []string{"active", "absent", "new-after-absent", "replacement"} {
		t.Run(scenario, func(t *testing.T) {
			a, cid, _ := chatActionFixture(t)
			cancelled := false
			if scenario == "active" || scenario == "replacement" {
				a.streamMgr.Register(cid, func() { cancelled = true })
			}
			metadata, _ := chatActionMetadata(t, a, commandChatCancelID)
			newCancelled := false
			if scenario == "replacement" || scenario == "new-after-absent" {
				a.streamMgr.Register(cid, func() { newCancelled = true })
			}
			err := a.CommitWorkspaceTabCommand(metadata.Ticket, metadata.HandoffID)
			wantOK := scenario == "active" || scenario == "absent"
			if (err == nil) != wantOK || newCancelled || (scenario == "active" && !cancelled) {
				t.Fatalf("cancel: %v old=%v new=%v", err, cancelled, newCancelled)
			}
			if wantOK {
				if result := getUIResultEventually(t, a, metadata.Ticket); result.Status != "succeeded" {
					t.Fatalf("%+v", result)
				}
			}
		})
	}
}

func TestCommandChatActionsContracts(t *testing.T) {
	a, _, _ := chatActionFixture(t)
	if len(a.commandProduct.Load().registry.List()) != 150 {
		t.Fatal("catalog count")
	}
	for _, id := range []string{commandChatSendID, commandChatRetryID, commandChatCancelID} {
		d, ok := a.commandProduct.Load().registry.Lookup(id)
		if !ok || d.Effect != commandcatalog.Write || d.Decision != commandcatalog.NoDecision || d.HandlerClassification != commandcatalog.HandlerBackend || !d.HasMutableTarget || isLocalUICommand(id) || !commandDeckLedgerCommand(d) || !localKeyboardCommandAllowed(id) || d.Persistence.Arguments != commandcatalog.PersistenceNever || d.Persistence.Result != commandcatalog.PersistenceNever || d.Context.Facts[0].Mode != commandcatalog.ExactVersion {
			t.Fatalf("%+v", d)
		}
		for _, locale := range []string{"pt-BR", "en", "es"} {
			if d.Presentation.Locales[locale].Name == "" {
				t.Fatal("label")
			}
		}
	}
}

func TestCommandChatActionsDeckPreservesSource(t *testing.T) {
	a := deckChatPickerFixture(t, commandChatSendID)
	if err := database.DB().AutoMigrate(&database.Conversation{}, &database.ChatMessage{}); err != nil {
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
	a.chatCtrl = controllers.NewChatController(controllers.ChatControllerConfig{})
	a.streamMgr = chat.NewStreamingManager(nil)
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
	h := takeUICommandFor(t, a, r.Ticket, commandChatSendID)
	if _, err := a.commitChatSubmission(ctx, &llm.ChatCommandMetadata{Ticket: r.Ticket, HandoffID: h.HandoffID}, conversation.ID, "", func(context.Context) (string, error) { return conversation.ID, nil }); err != nil {
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

func TestCommandChatActionsMissingConversationFeedback(t *testing.T) {
	for _, scenario := range []string{"missing", "foreign"} {
		t.Run(scenario, func(t *testing.T) {
			a, cid, _ := chatActionFixture(t)
			var err error
			if scenario == "missing" {
				err = database.DB().Where("id = ?", cid).Delete(&database.Conversation{}).Error
			} else {
				err = database.DB().Model(&database.Conversation{}).Where("id = ?", cid).Update("user_id", "another-owner").Error
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, command := range []string{commandChatSendID, commandChatRetryID} {
				reservation, err := a.BeginUICommand(command)
				if !errors.Is(err, errChatConversationUnavailable) || err.Error() != "chat_conversation_unavailable" || reservation.Ticket != "" {
					t.Fatalf("diagnóstico seguro ausente: reservation=%+v err=%v", reservation, err)
				}
			}
		})
	}
}
