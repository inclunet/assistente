package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"
	"time"

	"assistente/controllers"
	"assistente/internal/apidto"
	"assistente/internal/chat"
	"assistente/internal/commandcatalog"
	"assistente/internal/commanddecision"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
	"assistente/internal/wailsapi"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

func clearCommandFixture(t *testing.T, kind workspace.TabType) (*App, <-chan map[string]any, string) {
	t.Helper()
	a, _ := appLifecycleProductMountFixture(t)
	manager, events := newCommandDecisionManager(t)
	a.questionnaireMgr = manager
	a.streamMgr = chat.NewStreamingManager(nil)
	a.conversationsCtrl = controllers.NewConversationsController(controllers.ConversationsControllerConfig{PrepareBatchDelete: a.prepareConversationDeletion})
	ctx := context.Background()
	if err := database.DB().AutoMigrate(&database.Conversation{}, &database.ChatMessage{}); err != nil {
		t.Fatal(err)
	}
	if err := database.MigrateMessageRevisions(database.DB()); err != nil {
		t.Fatal(err)
	}
	conv, err := database.CreateConversationWithContext(database.WithUserID(ctx, a.currentUserID), "clear", "")
	if err != nil {
		t.Fatal(err)
	}
	tabID := uuid.NewString()
	if err := a.workspaceMgr.AddTab(workspace.Tab{ID: tabID, Type: kind, ConversationID: conv.ID}); err != nil {
		t.Fatal(err)
	}
	if err := a.workspaceMgr.SetActiveTab(tabID); err != nil {
		t.Fatal(err)
	}
	if err := database.DB().Model(&database.Conversation{}).Where("id = ?", conv.ID).Update("summary", "original").Error; err != nil {
		t.Fatal(err)
	}
	if err := a.credMgr.RegisterInstanceSecret("internal-auth:command-request-hmac:v1", base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{42}, 32))); err != nil {
		t.Fatal(err)
	}
	if err := a.ensureCommandLifecycleMountedForCurrentUser(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = ShutdownCommandLifecycle(ctx, a)
		_ = a.drainCommandExecutors(ctx)
		_ = a.shutdownCommandBridgeIfConfigured(ctx)
	})
	if err := a.commandHost.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	if err := a.rebuildCommandLifecyclePersistedConfiguration(ctx); err != nil {
		t.Fatal(err)
	}
	if err := BootstrapCommandLifecycle(ctx, a); err != nil {
		t.Fatal(err)
	}
	return a, events, conv.ID
}

func TestCommandConversationClearDecisionCommitAndReplay(t *testing.T) {
	for _, kind := range []workspace.TabType{workspace.TabTypeChat, workspace.TabTypeEditor, workspace.TabTypeTerminal, workspace.TabTypeTasklist} {
		t.Run(string(kind), func(t *testing.T) {
			a, events, cid := clearCommandFixture(t, kind)
			r := beginUICommand(t, a, commandConversationClearID)
			payload := receiveCommandDecisionEvent(t, events)
			finishCommandDecision(t, a.questionnaireMgr, payload, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
			h := takeUICommandFor(t, a, r.Ticket, commandConversationClearID)
			if err := a.CompleteUICommand(r.Ticket, h.HandoffID, "succeeded"); err == nil {
				t.Fatal("UI completion bypassed backend")
			}
			if err := a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err != nil {
				t.Fatal(err)
			}
			if got := getUIResultEventually(t, a, r.Ticket); got.Status != "succeeded" {
				t.Fatalf("%+v", got)
			}
			var conv database.Conversation
			if err := database.DB().Where("id = ?", cid).First(&conv).Error; err != nil || conv.Summary != "" {
				t.Fatalf("not cleared: %+v %v", conv, err)
			}
			if err := a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err == nil {
				t.Fatal("replay accepted")
			}
			var status string
			if err := database.DB().Table("command_decision_receipts").Select("status").Where("subject_id = ?", r.InvocationID).Scan(&status).Error; err != nil || status != "consumed" {
				t.Fatalf("receipt %s %v", status, err)
			}
		})
	}
}

func TestCommandConversationClearRejectsStaleAndActiveGeneration(t *testing.T) {
	for _, scenario := range []string{"cancel", "content", "new-message", "binding", "aba", "active", "generation"} {
		t.Run(scenario, func(t *testing.T) {
			a, events, cid := clearCommandFixture(t, workspace.TabTypeEditor)
			r := beginUICommand(t, a, commandConversationClearID)
			payload := receiveCommandDecisionEvent(t, events)
			if scenario == "cancel" {
				finishCommandDecision(t, a.questionnaireMgr, payload, nil, true)
				if _, err := a.TakeUICommand(r.Ticket); err == nil {
					t.Fatal("cancel delivered handoff")
				}
			} else {
				finishCommandDecision(t, a.questionnaireMgr, payload, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
				h := takeUICommandFor(t, a, r.Ticket, commandConversationClearID)
				switch scenario {
				case "new-message":
					if err := database.DB().Create(&database.ChatMessage{ConversationID: cid, Role: "user", Content: "new message"}).Error; err != nil {
						t.Fatal(err)
					}
				case "content":
					if err := database.DB().Model(&database.Conversation{}).Where("id = ?", cid).Update("summary", "new content").Error; err != nil {
						t.Fatal(err)
					}
				case "binding":
					if err := a.workspaceMgr.UpdateTab(a.workspaceMgr.Active().Tabs.Active, map[string]any{"conversation_id": uuid.NewString()}); err != nil {
						t.Fatal(err)
					}
				case "aba":
					original := a.workspaceMgr.Active().Tabs.Active
					other := uuid.NewString()
					if err := a.workspaceMgr.AddTab(workspace.Tab{ID: other, Type: workspace.TabTypeEditor}); err != nil {
						t.Fatal(err)
					}
					if err := a.workspaceMgr.SetActiveTab(other); err != nil {
						t.Fatal(err)
					}
					if err := a.workspaceMgr.SetActiveTab(original); err != nil {
						t.Fatal(err)
					}
				case "active":
					release, ok := a.streamMgr.ReserveConversation(cid)
					if !ok {
						t.Fatal("reserve failed")
					}
					defer release()
				case "generation":
					if err := a.commandHost.SetActiveLayers(context.Background(), a.currentUserID, []string{"changed"}); err != nil {
						t.Fatal(err)
					}
				}
				if err := a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err == nil {
					t.Fatal("unsafe clear accepted")
				}
			}
			var conv database.Conversation
			if err := database.DB().Where("id = ?", cid).First(&conv).Error; err != nil || conv.Summary == "" {
				t.Fatalf("content erased: %v", err)
			}
		})
	}
}

func TestCommandConversationClearContractsAndForeignOwner(t *testing.T) {
	a, _, cid := clearCommandFixture(t, workspace.TabTypeChat)
	d, h := commandConversationClearRegistration()
	if d.Effect != commandcatalog.Destructive || d.Decision != commandcatalog.Interactive || h.Classification != commandcatalog.HandlerBackend || !d.HasMutableTarget || len(d.Context.Facts) != 1 || d.Context.Facts[0].Mode != commandcatalog.ExactVersion {
		t.Fatalf("unsafe contract: %+v", d)
	}
	if commandUIRunTimeout(d.ID) != 5*time.Minute || isLocalUICommand(d.ID) || !commandDeckLedgerCommand(d) {
		t.Fatal("unsafe execution class/timeout")
	}
	_, handlers, err := a.commandProductCatalog()
	if err != nil || handlers[d.ID].ExecutionTimeout != 5*time.Minute || commandUITakeTimeout(d.ID) != 5*time.Minute || commandUIResultTTL(d.ID) != 6*time.Minute || commandUITakeTimeout("editor.format.bold") != 35*time.Second || commandUIRunTimeout("editor.format.bold") != 30*time.Second {
		t.Fatalf("dialog deadlines: %v", err)
	}
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	binding := commandKeyboardBindingFor(t, view, LocalCommandShortcut{Version: 1, Code: "KeyL", Modifiers: []string{"Control"}})
	if binding.CommandID != d.ID || binding.Handler != "contextual" {
		t.Fatalf("%+v", binding)
	}
	if r, err := a.BeginLocalCommandUIKey(view.Generation, binding.Shortcut, true); err == nil || r != nil {
		t.Fatal("repeat accepted")
	}
	if err := database.DB().Model(&database.Conversation{}).Where("id = ?", cid).Update("user_id", "foreign").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := a.BeginUICommand(d.ID); err == nil {
		t.Fatal("foreign owner accepted")
	}
}

func TestCommandConversationClearKeyboardDecisionPreservesTarget(t *testing.T) {
	a, events, _ := clearCommandFixture(t, workspace.TabTypeChat)
	p := a.commandProduct.Load()
	before, err := p.host.Snapshot(context.Background(), p.principal)
	if err != nil {
		t.Fatal(err)
	}
	target, err := a.workspaceMgr.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	key := LocalCommandShortcut{Version: 1, Code: "KeyL", Modifiers: []string{"Control"}}
	r, err := a.BeginLocalCommandUIKey(view.Generation, key, false)
	if err != nil || r == nil {
		t.Fatalf("begin: %+v %v", r, err)
	}
	payload := receiveCommandDecisionEvent(t, events)
	// Modal/focus facts are frontend-only: the real presenter must not rotate
	// authoritative workspace or host versions while its decision is pending.
	after, err := p.host.Snapshot(context.Background(), p.principal)
	if err != nil || before != after {
		t.Fatalf("dialog changed host: %v", err)
	}
	current, err := a.workspaceMgr.CommandSnapshot()
	if err != nil || current != target {
		t.Fatalf("dialog changed target: %v", err)
	}
	finishCommandDecision(t, a.questionnaireMgr, payload, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
	h := takeUICommandFor(t, a, r.Ticket, commandConversationClearID)
	if err := a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err != nil {
		t.Fatal(err)
	}
	if got := getUIResultEventually(t, a, r.Ticket); got.Status != "succeeded" {
		t.Fatalf("%+v", got)
	}
}

func TestCommandConversationClearReadinessRequiresControllerAndBinding(t *testing.T) {
	a, _, _ := clearCommandFixture(t, workspace.TabTypeChat)
	a.commandCatalogAPI = wailsapi.NewCommandCatalog()
	a.wireCommandCatalog()
	check := func(want bool) {
		t.Helper()
		items, err := a.commandCatalogAPI.ListCommands(apidto.CommandCatalogFilter{Source: "palette", Locale: "pt-BR"})
		if err != nil || len(items) != 150 {
			t.Fatalf("catalog: %d %v", len(items), err)
		}
		for _, item := range items {
			if item.ID == commandConversationClearID {
				if item.Available != want || (!want && item.ReadinessReason == "") {
					t.Fatalf("readiness: %+v", item)
				}
				return
			}
		}
		t.Fatal("missing clear")
	}
	check(true)
	controller := a.conversationsCtrl
	a.conversationsCtrl = nil
	check(false)
	a.conversationsCtrl = controller
	if err := a.workspaceMgr.UpdateTab(a.workspaceMgr.Active().Tabs.Active, map[string]any{"conversation_id": ""}); err != nil {
		t.Fatal(err)
	}
	check(false)
}
