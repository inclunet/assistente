package app

import (
	"context"
	"testing"

	"assistente/controllers"
	"assistente/internal/commandautomation"
	"assistente/internal/commandcatalog"
	"assistente/internal/commanddecision"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
	"assistente/internal/terminal"
	"assistente/internal/workspace"
)

func surfacePaletteFixture(t *testing.T, kind workspace.TabType) (*App, <-chan map[string]any, string) {
	t.Helper()
	// Install the decision presenter before the executor captures it, and
	// include the settings schema required by confirmed configuration writes.
	a, events, cid := clearCommandFixture(t, kind)
	if err := commandautomation.Migrate(context.Background(), database.DB()); err != nil {
		t.Fatal(err)
	}
	return a, events, cid
}

// Configure through the same confirmed settings API used by the application.
func configureSurfacePaletteCommand(t *testing.T, a *App, events <-chan map[string]any, commandID string) (LocalCommandKeyboardMap, LocalCommandKeyboardContext) {
	t.Helper()
	if err := a.workspaceMgr.SetProfile("dev"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := a.workspaceMgr.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	observed := LocalCommandKeyboardContext{SurfaceID: snapshot.Tab.ID, SurfaceType: string(snapshot.Tab.Type), Profile: "dev"}
	layer := settingsContractApply(t, a, events, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Contextual " + commandID, Enabled: true}})
	settingsContractApply(t, a, events, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "always", Lifecycle: "persistent", Enabled: true}})
	settingsContractApply(t, a, events, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &CommandSettingsBindingInput{LayerID: layer.ID, CommandID: commandID, TriggerType: "palette", TriggerSpec: `{"version":1,"selection":"` + commandID + `"}`, Arguments: map[string]any{}, Effect: "execute", Enabled: true, Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{
		{Field: "app.focused", Value: true}, {Field: "surface.type", Value: observed.SurfaceType}, {Field: "surface.id", Value: observed.SurfaceID}, {Field: "profile", Value: observed.Profile},
	}}}})
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	return view, observed
}

func assertSurfacePaletteSucceeded(t *testing.T, a *App, ticket, invocationID string) {
	t.Helper()
	if result := getUIResultEventually(t, a, ticket); result.Status != "succeeded" {
		t.Fatalf("result: %+v", result)
	}
	var count int64
	if err := database.DB().Table("command_invocations").Where("invocation_id = ? AND source_type = ? AND status = ?", invocationID, "palette", "succeeded").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("palette ledger: %d %v", count, err)
	}
}

func TestContextualPaletteSurfacesClosedCommandSet(t *testing.T) {
	a := readyCommandProduct(t)
	ids := []string{commandConversationClearID, commandTerminalInterruptID, commandTerminalSessionCreateID, commandTerminalSessionCloseID, commandChatSendID, commandChatRetryID, commandChatCancelID, commandEditorFileOpenID, commandEditorFileSaveID, commandEditorFileSaveCopyID}
	ids = append(ids, commandChatMessageIDs...)
	ids = append(ids, commandEditorFormatIDs...)
	for _, id := range ids {
		d, ok := a.commandProduct.Load().registry.Lookup(id)
		class := commandExecutionClassForDefinition(d)
		if !ok || !isContextualPaletteWorkspaceCommand(id) || !d.AllowsSource(commandcatalog.Palette) || (class != commandExecutionDurable && class != commandExecutionAuditedUI) {
			t.Fatalf("unsupported contract for %s: %+v", id, d)
		}
	}
	for _, d := range a.commandProduct.Load().registry.List() {
		if isPageMutationCommand(d.ID) || isCommandLayerAction(d.ID) || commandExecutionClassForDefinition(d) == commandExecutionLocalUI {
			if isContextualPaletteWorkspaceCommand(d.ID) {
				t.Fatalf("expanded into page/layer/local UI: %s", d.ID)
			}
		}
	}
}

func TestContextualPaletteSurfacesChatCancelAndMessageEffects(t *testing.T) {
	for _, id := range []string{commandChatCancelID, commandMessagePinID, commandMessageCopyID} {
		t.Run(id, func(t *testing.T) {
			a, events, cid := surfacePaletteFixture(t, workspace.TabTypeChat)
			a.chatCtrl = controllers.NewChatController(controllers.ChatControllerConfig{})
			message := messageCommandSeed(t, cid)
			view, observed := configureSurfacePaletteCommand(t, a, events, id)
			cancelled := false
			if id == commandChatCancelID {
				a.streamMgr.Register(cid, func() { cancelled = true })
			}
			r, err := a.BeginContextualPaletteUICommand(view.Generation, id, observed)
			if err != nil {
				t.Fatal(err)
			}
			if isChatMessageCommand(id) {
				if commandInvocationCount(t, id) != 0 {
					t.Fatal("admitted before message preparation")
				}
				if err := a.PrepareChatMessageCommand(r.Ticket, message.ID); err != nil {
					t.Fatal(err)
				}
			}
			h := takeUICommandFor(t, a, r.Ticket, id)
			switch id {
			case commandChatCancelID:
				err = a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID)
			case commandMessagePinID:
				err = a.CommitChatMessageCommand(r.Ticket, h.HandoffID)
			default:
				err = a.CompleteUICommand(r.Ticket, h.HandoffID, "succeeded")
			}
			if err != nil {
				t.Fatal(err)
			}
			assertSurfacePaletteSucceeded(t, a, r.Ticket, r.InvocationID)
			if id == commandChatCancelID && !cancelled {
				t.Fatal("stream was not cancelled")
			}
			if id == commandMessagePinID {
				var stored database.ChatMessage
				if err := database.DB().First(&stored, "id = ?", message.ID).Error; err != nil || !stored.Pinned {
					t.Fatalf("message not pinned: %+v %v", stored, err)
				}
			}
			assertChatActionLedgerRedacted(t, r.InvocationID, message.Content)
		})
	}
}

func TestContextualPaletteSurfacesAuditedEditorOldIngressAndABA(t *testing.T) {
	for _, phase := range []string{"success", "old-ingress", "before-take", "wrong-profile"} {
		t.Run(phase, func(t *testing.T) {
			a, events, _ := surfacePaletteFixture(t, workspace.TabTypeEditor)
			view, observed := configureSurfacePaletteCommand(t, a, events, commandEditorFormatBoldID)
			if phase == "wrong-profile" {
				observed.Profile = "prod"
				if r, err := a.BeginContextualPaletteUICommand(view.Generation, commandEditorFormatBoldID, observed); err == nil || r.Ticket != "" {
					t.Fatal("forged profile admitted")
				}
				return
			}
			if phase == "old-ingress" {
				r, err := a.BeginUICommand(commandEditorFormatBoldID)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := a.TakeUICommand(r.Ticket); err == nil {
					t.Fatal("ordinary ingress bypassed visual conditions")
				}
				return
			}
			r, err := a.BeginContextualPaletteUICommand(view.Generation, commandEditorFormatBoldID, observed)
			if err != nil {
				t.Fatal(err)
			}
			if phase == "before-take" {
				if err := a.workspaceMgr.SetProfile("other"); err != nil {
					t.Fatal(err)
				}
				if err := a.workspaceMgr.SetProfile("dev"); err != nil {
					t.Fatal(err)
				}
				if _, err := a.TakeUICommand(r.Ticket); err == nil {
					t.Fatal("ABA delivered audited handoff")
				}
				return
			}
			h := takeUICommandFor(t, a, r.Ticket, commandEditorFormatBoldID)
			if err := a.CompleteUICommand(r.Ticket, h.HandoffID, "succeeded"); err != nil {
				t.Fatal(err)
			}
			assertSurfacePaletteSucceeded(t, a, r.Ticket, r.InvocationID)
		})
	}
}

func TestContextualPaletteSurfacesDestructiveDecisionAndCommitABA(t *testing.T) {
	for _, phase := range []string{"confirm", "cancel", "commit-aba"} {
		t.Run(phase, func(t *testing.T) {
			a, events, cid := surfacePaletteFixture(t, workspace.TabTypeChat)
			view, observed := configureSurfacePaletteCommand(t, a, events, commandConversationClearID)
			r, err := a.BeginContextualPaletteUICommand(view.Generation, commandConversationClearID, observed)
			if err != nil {
				t.Fatal(err)
			}
			decision := receiveCommandDecisionEvent(t, events)
			var before database.Conversation
			if err := database.DB().First(&before, "id = ?", cid).Error; err != nil || before.Summary != "original" {
				t.Fatal("effect before decision", err)
			}
			if phase == "cancel" {
				finishCommandDecision(t, a.questionnaireMgr, decision, nil, true)
				if _, err := a.TakeUICommand(r.Ticket); err == nil {
					t.Fatal("cancelled decision delivered handoff")
				}
			} else {
				finishCommandDecision(t, a.questionnaireMgr, decision, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
				h := takeUICommandFor(t, a, r.Ticket, commandConversationClearID)
				if err := a.CompleteUICommand(r.Ticket, h.HandoffID, "succeeded"); err == nil {
					t.Fatal("UI bypassed destructive commit")
				}
				if phase == "commit-aba" {
					if err := a.workspaceMgr.SetProfile("other"); err != nil {
						t.Fatal(err)
					}
					if err := a.workspaceMgr.SetProfile("dev"); err != nil {
						t.Fatal(err)
					}
				}
				err := a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID)
				if (err == nil) != (phase == "confirm") {
					t.Fatalf("commit phase=%s: %v", phase, err)
				}
				if phase == "confirm" {
					assertSurfacePaletteSucceeded(t, a, r.Ticket, r.InvocationID)
				}
			}
			var stored database.Conversation
			if err := database.DB().First(&stored, "id = ?", cid).Error; err != nil || (stored.Summary == "") != (phase == "confirm") {
				t.Fatalf("unexpected destructive effect: %+v %v", stored, err)
			}
		})
	}
}

func TestContextualPaletteSurfacesTerminalPreparationAndStaleCommit(t *testing.T) {
	a, events, _ := surfacePaletteFixture(t, workspace.TabTypeTerminal)
	a.terminalMgr = terminal.NewManager(terminal.DefaultManagerConfig(), nil)
	t.Cleanup(a.terminalMgr.CloseAll)
	view, observed := configureSurfacePaletteCommand(t, a, events, commandTerminalSessionCreateID)
	r, err := a.BeginContextualPaletteUICommand(view.Generation, commandTerminalSessionCreateID, observed)
	if err != nil {
		t.Fatal(err)
	}
	if commandInvocationCount(t, commandTerminalSessionCreateID) != 0 {
		t.Fatal("terminal admitted before preparation")
	}
	if err := a.PrepareTerminalSessionCommand(r.Ticket, view.WorkspaceID, observed.SurfaceID, ""); err != nil {
		t.Fatal(err)
	}
	h := takeUICommandFor(t, a, r.Ticket, commandTerminalSessionCreateID)
	if len(a.terminalMgr.List()) != 0 {
		t.Fatal("Begin/Take started terminal")
	}
	if err := a.workspaceMgr.SetProfile("other"); err != nil {
		t.Fatal(err)
	}
	if err := a.workspaceMgr.SetProfile("dev"); err != nil {
		t.Fatal(err)
	}
	if err := a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err == nil {
		t.Fatal("stale terminal commit admitted")
	}
	if len(a.terminalMgr.List()) != 0 {
		t.Fatal("stale commit created terminal session")
	}
	for _, id := range []string{commandTerminalInterruptID, commandTerminalSessionCloseID} {
		view, observed = configureSurfacePaletteCommand(t, a, events, id)
		if r, err := a.BeginContextualPaletteUICommand(view.Generation, id, observed); err == nil || r.Ticket != "" {
			t.Fatalf("missing terminal target accepted: %s", id)
		}
	}
}
