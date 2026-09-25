package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"assistente/controllers"
	"assistente/internal/chat"
	"assistente/internal/commanddeck"
	"assistente/internal/commandui"
	"assistente/internal/database"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

func TestCommandChatEditorPlanOpenAckAndReplay(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "existing"}[existing], func(t *testing.T) {
			a, _, cid := clearCommandFixture(t, workspace.TabTypeChat)
			source := a.workspaceMgr.Active().Tabs.Active
			target := ""
			if existing {
				target = uuid.NewString()
				if err := a.workspaceMgr.AddTab(workspace.Tab{ID: target, Type: workspace.TabTypeEditor, State: map[string]any{"draftId": "existing-draft"}}); err != nil {
					t.Fatal(err)
				}
				if err := a.workspaceMgr.SetActiveTab(source); err != nil {
					t.Fatal(err)
				}
			}
			m := messageCommandSeed(t, cid)
			m.Role = "assistant"
			if err := database.DB().Save(&m).Error; err != nil {
				t.Fatal(err)
			}
			count := len(a.workspaceMgr.Active().Tabs.Items)
			events := make(chan *workspace.Workspace, 3)
			a.emitter = commandOSBootstrapEmitter(func(name string, value any) {
				if name == "workspace:tab_activated" {
					events <- value.(*workspace.Workspace)
				}
			})
			r := beginUICommand(t, a, commandMessageSendEditorID)
			plan, err := a.PrepareChatEditorCommand(r.Ticket, m.ID, m.Content, target)
			if err != nil {
				t.Fatal(err)
			}
			if plan.TabID == "" || plan.DraftID == "" || len(a.workspaceMgr.Active().Tabs.Items) != count || a.workspaceMgr.Active().Tabs.Active != source {
				t.Fatal("Prepare mutated workspace or omitted identity")
			}
			if existing && plan.TabID != target {
				t.Fatal("retargeted plan")
			}
			h := takeUICommandFor(t, a, r.Ticket, commandMessageSendEditorID)
			if err := a.CompleteUICommand(r.Ticket, h.HandoffID, "succeeded"); err == nil {
				t.Fatal("completed before Open")
			}
			if err := a.ValidateChatEditorCommand(r.Ticket, h.HandoffID); err == nil {
				t.Fatal("validated before Open")
			}
			const retries = 4
			results := make(chan *ChatEditorCommandTarget, retries)
			failures := make(chan error, retries)
			var workers sync.WaitGroup
			for i := 0; i < retries; i++ {
				workers.Add(1)
				go func() {
					defer workers.Done()
					result, err := a.OpenChatEditorCommand(r.Ticket, h.HandoffID, "Message")
					results <- result
					failures <- err
				}()
			}
			workers.Wait()
			for i := 0; i < retries; i++ {
				if err := <-failures; err != nil {
					t.Fatal(err)
				}
				result := <-results
				if result.TabID != plan.TabID || result.DraftID != plan.DraftID || result.Workspace.Tabs.Active != plan.TabID {
					t.Fatal("Open diverged from plan")
				}
			}
			want := count
			if !existing {
				want++
			}
			if len(a.workspaceMgr.Active().Tabs.Items) != want {
				t.Fatal("duplicate tab")
			}
			if len(events) != 1 {
				t.Fatal("transition emitted more than once")
			}
			event := <-events
			if event.Tabs.Active != plan.TabID || event.SnapshotEpoch == "" || event.SnapshotSequence == "" {
				t.Fatal("invalid workspace event")
			}
			_, run, err := a.commandUIRun(r.Ticket)
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-run.done:
				t.Fatal("Open terminated command before UI ack")
			default:
			}
			if _, err := a.GetLocalCommandKeyboardMap(); err != nil {
				t.Fatal(err)
			}
			if err := a.ValidateChatEditorCommand(r.Ticket, h.HandoffID); err != nil {
				t.Fatal("own transition invalidated execution", err)
			}
			if err := a.CompleteUICommand(r.Ticket, h.HandoffID, "succeeded"); err != nil {
				t.Fatal(err)
			}
			if got := getUIResultEventually(t, a, r.Ticket); got.Status != "succeeded" {
				t.Fatalf("%+v", got)
			}
			assertChatActionLedgerRedacted(t, r.InvocationID, m.Content)
			assertChatActionLedgerRedacted(t, r.InvocationID, m.Media)
		})
	}
}

func TestCommandChatEditorRejectsStaleAndReportsPartialUnknown(t *testing.T) {
	for _, scenario := range []string{"cancelled", "failed", "cancel", "aba", "source-content", "configuration", "session", "source-device"} {
		t.Run(scenario, func(t *testing.T) {
			a, _, cid := clearCommandFixture(t, workspace.TabTypeChat)
			m := messageCommandSeed(t, cid)
			r := beginUICommand(t, a, commandMessageSendEditorID)
			if _, err := a.PrepareChatEditorCommand(r.Ticket, m.ID, m.Content, ""); err != nil {
				t.Fatal(err)
			}
			h := takeUICommandFor(t, a, r.Ticket, commandMessageSendEditorID)
			result, err := a.OpenChatEditorCommand(r.Ticket, h.HandoffID, "Message")
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "cancelled", "failed":
				if err := a.CompleteUICommand(r.Ticket, h.HandoffID, scenario); err != nil {
					t.Fatal(err)
				}
			case "cancel":
				if err := a.CancelUICommand(r.Ticket); err != nil {
					t.Fatal(err)
				}
			default:
				switch scenario {
				case "aba":
					source := a.commandProduct.Load().uiRuns[r.Ticket].snapshot.ActiveTabID
					if err := a.workspaceMgr.SetActiveTab(source); err != nil {
						t.Fatal(err)
					}
					if err := a.workspaceMgr.SetActiveTab(result.TabID); err != nil {
						t.Fatal(err)
					}
				case "source-content":
					if err := database.DB().Model(&m).Update("content", "changed").Error; err != nil {
						t.Fatal(err)
					}
				case "configuration":
					if err := a.commandHost.SetActiveLayers(context.Background(), a.currentUserID, []string{"changed"}); err != nil {
						t.Fatal(err)
					}
				case "session":
					a.authMu.Lock()
					old := a.currentAuthUser.SessionID
					a.currentAuthUser.SessionID = uuid.NewString()
					a.authMu.Unlock()
					defer func() { a.authMu.Lock(); a.currentAuthUser.SessionID = old; a.authMu.Unlock() }()
				case "source-device":
					_, run, err := a.commandUIRun(r.Ticket)
					if err != nil {
						t.Fatal(err)
					}
					run.sourceValid = func() bool { return false }
				}
				if err := a.ValidateChatEditorCommand(r.Ticket, h.HandoffID); err == nil {
					t.Fatal("stale target validated")
				}
				if err := a.CompleteUICommand(r.Ticket, h.HandoffID, "succeeded"); err == nil {
					t.Fatal("stale completion")
				}
				if scenario == "session" {
					return
				}
			}
			if got := getUIResultEventually(t, a, r.Ticket); got.Status != "outcome_unknown" {
				t.Fatalf("partial effect misreported: %+v", got)
			}
		})
	}
}

func TestCommandChatEditorPreparationBoundSourceAndTarget(t *testing.T) {
	for _, scenario := range []string{"original", "foreign", "target", "readonly", "streaming", "target-changed"} {
		t.Run(scenario, func(t *testing.T) {
			a, _, cid := clearCommandFixture(t, workspace.TabTypeChat)
			m := messageCommandSeed(t, cid)
			target, original := "", m.Content
			switch scenario {
			case "original":
				original = "stale"
			case "foreign":
				if err := database.DB().Model(&database.Conversation{}).Where("id = ?", cid).Update("user_id", "another").Error; err != nil {
					t.Fatal(err)
				}
			case "target":
				target = a.workspaceMgr.Active().Tabs.Active
			case "readonly":
				source := a.workspaceMgr.Active().Tabs.Active
				target = uuid.NewString()
				if err := a.workspaceMgr.AddTab(workspace.Tab{ID: target, Type: workspace.TabTypeEditor, State: map[string]any{"readOnly": true}}); err != nil {
					t.Fatal(err)
				}
				if err := a.workspaceMgr.SetActiveTab(source); err != nil {
					t.Fatal(err)
				}
			case "streaming":
				generation := a.streamMgr.Register(cid, func() {})
				defer a.streamMgr.UnregisterIfCurrent(cid, generation)
			}
			r, err := a.BeginUICommand(commandMessageSendEditorID)
			if scenario == "foreign" {
				if err == nil {
					t.Fatal("foreign conversation admitted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			_, err = a.PrepareChatEditorCommand(r.Ticket, m.ID, original, target)
			if scenario != "target-changed" {
				if err == nil {
					t.Fatal("invalid prepare accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			h := takeUICommandFor(t, a, r.Ticket, commandMessageSendEditorID)
			if err := a.workspaceMgr.UpdateTab(a.workspaceMgr.Active().Tabs.Active, map[string]any{"state": map[string]any{"changed": true}}); err != nil {
				t.Fatal(err)
			}
			if _, err := a.OpenChatEditorCommand(r.Ticket, h.HandoffID, "Message"); err == nil {
				t.Fatal("stale source opened editor")
			}
		})
	}
}

func TestCommandChatEditorKeyboardSurvivesOwnMapReset(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	if err := database.DB().AutoMigrate(&database.Conversation{}, &database.ChatMessage{}); err != nil {
		t.Fatal(err)
	}
	if err := database.MigrateMessageRevisions(database.DB()); err != nil {
		t.Fatal(err)
	}
	conv, err := database.CreateConversationWithContext(database.WithUserID(context.Background(), a.currentUserID), "source", "")
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	if err := a.workspaceMgr.AddTab(workspace.Tab{ID: id, Type: workspace.TabTypeChat, ConversationID: conv.ID}); err != nil {
		t.Fatal(err)
	}
	if err := a.workspaceMgr.SetActiveTab(id); err != nil {
		t.Fatal(err)
	}
	a.conversationsCtrl = controllers.NewConversationsController(controllers.ConversationsControllerConfig{})
	a.streamMgr = chat.NewStreamingManager(nil)
	layer, _ := settingsActivationSecurityLayerAndRule(t, a, decisions)
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandBinding(CommandBindingEdit{LayerID: layer, CommandID: commandMessageSendEditorID, TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyK","modifiers":["Control","Shift"]}`, Enabled: true})
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
	if err != nil {
		t.Fatal(err)
	}
	m := messageCommandSeed(t, conv.ID)
	if _, err := a.PrepareChatEditorCommand(r.Ticket, m.ID, m.Content, ""); err != nil {
		t.Fatal(err)
	}
	h := takeUICommandFor(t, a, r.Ticket, commandMessageSendEditorID)
	if _, err := a.OpenChatEditorCommand(r.Ticket, h.HandoffID, "Message"); err != nil {
		t.Fatal(err)
	}
	a.ResetLocalCommandKeyboard(view.Generation)
	if err := a.ValidateChatEditorCommand(r.Ticket, h.HandoffID); err != nil {
		t.Fatal(err)
	}
	if err := a.CompleteUICommand(r.Ticket, h.HandoffID, "succeeded"); err != nil {
		t.Fatal(err)
	}
	if got := getUIResultEventually(t, a, r.Ticket); got.Status != "succeeded" {
		t.Fatalf("%+v", got)
	}
	var source string
	if err := database.DB().Table("command_invocations").Select("source_type").Where("invocation_id = ?", r.InvocationID).Scan(&source).Error; err != nil || source != "keyboard.local" {
		t.Fatal("source changed", source, err)
	}
}

func TestCommandChatEditorDeckPreservesSource(t *testing.T) {
	a := deckChatPickerFixture(t, commandMessageSendEditorID)
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
	if _, err := a.PrepareChatEditorCommand(r.Ticket, m.ID, m.Content, ""); err != nil {
		t.Fatal(err)
	}
	h := takeUICommandFor(t, a, r.Ticket, commandMessageSendEditorID)
	if _, err := a.OpenChatEditorCommand(r.Ticket, h.HandoffID, "Message"); err != nil {
		t.Fatal(err)
	}
	if err := a.ValidateChatEditorCommand(r.Ticket, h.HandoffID); err != nil {
		t.Fatal(err)
	}
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
