package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"assistente/internal/commandautomation"
	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/commandui"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
	"assistente/internal/workspace"
)

func configurePageDeckCommand(t *testing.T, a *App, decisions <-chan map[string]any, id, surface string) string {
	t.Helper()
	if err := commandautomation.Migrate(context.Background(), database.DB()); err != nil {
		t.Fatal(err)
	}
	if err := a.workspaceMgr.SetProfile("dev"); err != nil {
		t.Fatal(err)
	}
	layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Physical page " + id, Enabled: true}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "always", Lifecycle: "persistent", Enabled: true}})
	binding := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &CommandSettingsBindingInput{LayerID: layer.ID, CommandID: id, TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"test-deck","key":3}`, Arguments: map[string]any{}, Effect: "execute", Enabled: true, Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{
		{Field: "app.focused", Value: true}, {Field: "surface.type", Value: surface}, {Field: "profile", Value: "dev"},
	}}}})
	if _, err := a.GetLocalCommandKeyboardMap(); err != nil {
		t.Fatal(err)
	}
	return binding.ID
}

func pageDeckOffer(t *testing.T, a *App) (CommandDeckContextualUIEvent, *commandDeckController) {
	t.Helper()
	// The shared helper drives native Input down/up; it does not mint a ticket
	// or manufacture an invocation/envelope.
	event, controller := physicalDeckLayerOffer(t, contextualLayerPaletteFixture{a: a}, 3)
	p := a.commandProduct.Load()
	snapshot, err := a.workspaceMgr.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	p.deckExecution.mu.Lock()
	offer, exists := p.deckExecution.offers[event.OfferID]
	p.deckExecution.mu.Unlock()
	if !exists || offer.snapshot != snapshot || offer.serial != "test-deck" || offer.key != 3 {
		t.Fatalf("offer lost original workspace/physical key: %+v", offer)
	}
	return event, controller
}

func beginPageDeckCommand(t *testing.T, a *App, event CommandDeckContextualUIEvent, id, surface string) commandui.Reservation {
	t.Helper()
	r, err := a.BeginContextualDeckPageUICommand(event.OfferID, event.Generation, surface, "dev")
	if err != nil || r.Ticket == "" || r.CommandID != id {
		t.Fatalf("physical page Begin: %+v %v", r, err)
	}
	p := a.commandProduct.Load()
	occurrence, exists := commandDeckOccurrenceFor(p, r.InvocationID)
	identity, normalizeErr := (commandconfig.StreamDeckTriggerPort{}).Normalize(context.Background(), json.RawMessage(`{"version":1,"device":"test-deck","key":3}`))
	if !exists || normalizeErr != nil || identity != occurrence.identity || occurrence.serial != "test-deck" {
		t.Fatalf("reservation lost physical occurrence: %+v %v", occurrence, normalizeErr)
	}
	if commandInvocationCount(t, id) != 0 {
		t.Fatal("Begin wrote an invocation before page preparation")
	}
	if replay, err := a.BeginContextualDeckPageUICommand(event.OfferID, event.Generation, surface, "dev"); err == nil || replay.Ticket != "" {
		t.Fatalf("physical offer replay admitted: %+v %v", replay, err)
	}
	return r
}

func assertPageDeckCleanup(t *testing.T, a *App, ticket string) {
	t.Helper()
	p := a.commandProduct.Load()
	p.mu.Lock()
	run := p.uiRuns[ticket]
	p.mu.Unlock()
	if run == nil {
		t.Fatal("page reservation missing")
	}
	select {
	case <-run.done:
	case <-time.After(5 * time.Second):
		t.Fatal("page run did not terminate")
	}
	p.deckExecution.mu.Lock()
	remaining := len(p.deckExecution.occurrences)
	p.deckExecution.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("page run retained %d physical occurrences", remaining)
	}
}

func assertPageDeckSucceeded(t *testing.T, a *App, c *commandDeckController, r commandui.Reservation, bindingID string) {
	t.Helper()
	assertContextualDeckLedger(t, a, r.Ticket, r.InvocationID, r.CommandID)
	c.mu.Lock()
	instanceID := c.instances["test-deck"].id
	c.mu.Unlock()
	var audit struct{ SourceInstanceID, SourceEventID, TriggerType, ObservedTriggerType, ObserverType, BindingIDs string }
	if err := database.DB().Table("command_invocations").Where("invocation_id = ?", r.InvocationID).Take(&audit).Error; err != nil {
		t.Fatal(err)
	}
	var bindings []string
	if err := json.Unmarshal([]byte(audit.BindingIDs), &bindings); err != nil || len(bindings) != 1 || bindings[0] != bindingID {
		t.Fatalf("wrong physical key binding: %+v %v", bindings, err)
	}
	if instanceID == "" || audit.SourceInstanceID != instanceID || audit.SourceEventID != r.InvocationID || audit.TriggerType != "streamdeck.key" || audit.ObservedTriggerType != "streamdeck.key" || audit.ObserverType != "streamdeck.key" {
		t.Fatalf("page ledger lost native identity: %+v; expected instance=%s", audit, instanceID)
	}
	assertPageDeckCleanup(t, a, r.Ticket)
}

func TestContextualDeckPageTaskOperations(t *testing.T) {
	for _, scenario := range []struct {
		id, surface string
		cancel      bool
	}{
		{"tasklists.duplicate", "tasklists", false}, {"tasklists.delete", "tasklists", false}, {"tasklists.clear", "tasklists", false},
		{"tasklists.duplicate", "tasklist", false}, {"tasklists.clear", "tasklist", false},
		{"tasklists.delete", "tasklists", true}, {"tasklists.clear", "tasklists", true},
	} {
		t.Run(scenario.id+"/"+scenario.surface+map[bool]string{true: "/cancel", false: "/apply"}[scenario.cancel], func(t *testing.T) {
			kind := workspace.TabTypeChat
			if scenario.surface == "tasklist" {
				kind = workspace.TabTypeTasklist
			}
			a, decisions, ctx, list := pagePaletteTaskFixture(t, kind)
			bindingID := configurePageDeckCommand(t, a, decisions, scenario.id, scenario.surface)
			target, err := a.ReadTaskListCommandTarget(list.ID)
			if err != nil {
				t.Fatal(err)
			}
			event, controller := pageDeckOffer(t, a)
			r := beginPageDeckCommand(t, a, event, scenario.id, scenario.surface)
			request := CommandPageMutationRequest{TargetID: list.ID, ExpectedFingerprint: target.Fingerprint}
			if scenario.id == "tasklists.duplicate" {
				request.Title = "Physical copy"
			}
			if err = a.PreparePageMutationCommand(r.Ticket, request); err != nil {
				t.Fatal(err)
			}
			if pageMutationDestructive(scenario.id) {
				payload := receiveCommandDecisionEvent(t, decisions)
				tasks, err := database.GetTasksByTaskListIDWithContext(ctx, list.ID)
				if err != nil || len(tasks) != 1 {
					t.Fatalf("effect before approval: %v %v", tasks, err)
				}
				if scenario.cancel {
					finishCommandDecision(t, a.questionnaireMgr, payload, nil, true)
					if got := getUIResultEventually(t, a, r.Ticket); got.Status == "succeeded" {
						t.Fatal("cancel succeeded")
					}
					tasks, err = database.GetTasksByTaskListIDWithContext(ctx, list.ID)
					if err != nil || len(tasks) != 1 {
						t.Fatalf("cancel changed target: %v %v", tasks, err)
					}
					assertPageDeckCleanup(t, a, r.Ticket)
					return
				}
				finishCommandDecision(t, a.questionnaireMgr, payload, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
			}
			h := takeUICommandFor(t, a, r.Ticket, scenario.id)
			if err = a.CompleteUICommand(r.Ticket, h.HandoffID, "succeeded"); err == nil {
				t.Fatal("UI forged backend success")
			}
			if err = a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err != nil {
				t.Fatal(err)
			}
			assertPageDeckSucceeded(t, a, controller, r, bindingID)
			if err = a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err == nil {
				t.Fatal("commit replay succeeded")
			}
			switch scenario.id {
			case "tasklists.delete":
				if _, err = database.GetTaskListMetadataWithContext(ctx, list.ID); err == nil {
					t.Fatal("list not deleted")
				}
			case "tasklists.clear":
				current, err := database.GetTaskListMetadataWithContext(ctx, list.ID)
				if err != nil || current.Description != list.Description {
					t.Fatalf("list metadata lost: %+v %v", current, err)
				}
				tasks, err := database.GetTasksByTaskListIDWithContext(ctx, list.ID)
				if err != nil || len(tasks) != 0 {
					t.Fatalf("tasks not cleared: %v %v", tasks, err)
				}
			case "tasklists.duplicate":
				result, err := a.GetPageMutationCommandResult(r.Ticket)
				if err != nil || result.ID == "" || result.ID == list.ID {
					t.Fatalf("invalid clone: %+v %v", result, err)
				}
				copy, err := database.GetTaskListWithContext(ctx, result.ID)
				if err != nil || copy.Title != request.Title || copy.Description != list.Description || copy.Workflow == nil {
					t.Fatalf("clone metadata/workflow: %+v %v", copy, err)
				}
			}
		})
	}
}

func TestContextualDeckPageProfileOperations(t *testing.T) {
	for _, operation := range []string{"duplicate", "delete", "activate", "delete-cancel"} {
		t.Run(operation, func(t *testing.T) {
			a, decisions, slug := profileCommandFixture(t)
			id := "profiles." + operation
			if operation == "delete-cancel" {
				id = "profiles.delete"
			}
			bindingID := configurePageDeckCommand(t, a, decisions, id, "profiles")
			target, err := a.ReadProfileCommandTarget(slug)
			if err != nil {
				t.Fatal(err)
			}
			event, controller := pageDeckOffer(t, a)
			r := beginPageDeckCommand(t, a, event, id, "profiles")
			if err = a.PreparePageMutationCommand(r.Ticket, CommandPageMutationRequest{TargetID: slug, ExpectedFingerprint: target.Fingerprint}); err != nil {
				t.Fatal(err)
			}
			if id == "profiles.delete" {
				payload := receiveCommandDecisionEvent(t, decisions)
				if _, err = a.profileManager.Get(slug); err != nil {
					t.Fatal("profile changed before approval", err)
				}
				if operation == "delete-cancel" {
					finishCommandDecision(t, a.questionnaireMgr, payload, nil, true)
					if got := getUIResultEventually(t, a, r.Ticket); got.Status == "succeeded" {
						t.Fatal("cancel succeeded")
					}
					if _, err = a.profileManager.Get(slug); err != nil {
						t.Fatal("cancel deleted profile", err)
					}
					assertPageDeckCleanup(t, a, r.Ticket)
					return
				}
				finishCommandDecision(t, a.questionnaireMgr, payload, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
			}
			h := takeUICommandFor(t, a, r.Ticket, id)
			if err = a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err != nil {
				t.Fatal(err)
			}
			assertPageDeckSucceeded(t, a, controller, r, bindingID)
			result, err := a.GetPageMutationCommandResult(r.Ticket)
			if err != nil || result.ID == "" {
				t.Fatalf("missing result: %+v %v", result, err)
			}
			switch operation {
			case "activate":
				if a.profileManager.GetActiveSlug() != slug {
					t.Fatal("profile not activated")
				}
			case "duplicate":
				if result.ID == slug {
					t.Fatal("duplicate reused source")
				}
				if _, err = a.profileManager.Get(result.ID); err != nil {
					t.Fatal("clone not persisted", err)
				}
			case "delete":
				if _, err = a.profileManager.Get(slug); err == nil {
					t.Fatal("profile not deleted")
				}
			}
			fresh, err := a.GetLocalCommandKeyboardMap()
			if err != nil || fresh.Generation == "" || fresh.Generation == event.Generation {
				t.Fatalf("self mutation did not republish: %+v %v", fresh, err)
			}
			if err = a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err == nil {
				t.Fatal("profile commit replay succeeded")
			}
		})
	}
}

func TestContextualDeckPageRejectsInvalidOffers(t *testing.T) {
	for _, scenario := range []string{"forged", "expiry", "wrong-page", "wrong-profile", "missing-profile", "stale-generation", "expired-map", "profile-ABA", "background-tab-not-page", "disconnect", "workspace-ingress"} {
		t.Run(scenario, func(t *testing.T) {
			a, decisions, _, _ := pagePaletteTaskFixture(t, workspace.TabTypeChat)
			surface := "tasklists"
			if scenario == "background-tab-not-page" {
				surface = "tasklist"
			}
			configurePageDeckCommand(t, a, decisions, "tasklists.duplicate", surface)
			event, controller := pageDeckOffer(t, a)
			p := a.commandProduct.Load()
			profile := "dev"
			switch scenario {
			case "forged":
				event.OfferID = "forged"
			case "expiry":
				p.deckExecution.mu.Lock()
				offer := p.deckExecution.offers[event.OfferID]
				offer.expiresAt = time.Now().Add(-time.Second)
				p.deckExecution.offers[event.OfferID] = offer
				p.deckExecution.mu.Unlock()
			case "wrong-page":
				surface = "profiles"
			case "wrong-profile":
				profile = "other"
			case "missing-profile":
				profile = ""
			case "stale-generation":
				a.ResetLocalCommandKeyboard(event.Generation)
				if _, err := a.GetLocalCommandKeyboardMap(); err != nil {
					t.Fatal(err)
				}
			case "expired-map":
				p.keyboardMu.Lock()
				p.keyboardMap.view.ValidUntil = time.Now().Add(-time.Second).UnixMilli()
				p.keyboardMu.Unlock()
			case "profile-ABA":
				if err := a.workspaceMgr.SetProfile("other"); err != nil {
					t.Fatal(err)
				}
				if err := a.workspaceMgr.SetProfile("dev"); err != nil {
					t.Fatal(err)
				}
			case "disconnect":
				controller.reset("test-deck")
			case "workspace-ingress":
				snapshot, err := a.workspaceMgr.CommandSnapshot()
				if err != nil {
					t.Fatal(err)
				}
				if r, err := a.BeginContextualDeckUICommand(event.OfferID, event.Generation, LocalCommandKeyboardContext{SurfaceType: string(snapshot.Tab.Type), SurfaceID: snapshot.Tab.ID, Profile: "dev"}); err == nil || r.Ticket != "" {
					t.Fatalf("page entered workspace ingress: %+v %v", r, err)
				}
			}
			if r, err := a.BeginContextualDeckPageUICommand(event.OfferID, event.Generation, surface, profile); err == nil || r.Ticket != "" {
				t.Fatalf("invalid offer admitted: %+v %v", r, err)
			}
			if commandInvocationCount(t, "tasklists.duplicate") != 0 {
				t.Fatal("invalid offer wrote ledger")
			}
			assertDeckLayerNoUIReservation(t, a)
		})
	}
}

func TestContextualDeckPageTargetAndSourceGuards(t *testing.T) {
	for _, scenario := range []string{"foreign-target", "changed-before-prepare", "changed-before-commit", "cancel-before-prepare", "disconnect-before-take", "disconnect-before-commit", "ABA-before-commit"} {
		t.Run(scenario, func(t *testing.T) {
			a, decisions, ctx, list := pagePaletteTaskFixture(t, workspace.TabTypeChat)
			configurePageDeckCommand(t, a, decisions, "tasklists.duplicate", "tasklists")
			target, err := a.ReadTaskListCommandTarget(list.ID)
			if err != nil {
				t.Fatal(err)
			}
			event, controller := pageDeckOffer(t, a)
			r := beginPageDeckCommand(t, a, event, "tasklists.duplicate", "tasklists")
			if scenario == "cancel-before-prepare" {
				if err = a.CancelUICommand(r.Ticket); err != nil {
					t.Fatal(err)
				}
				assertPageDeckCleanup(t, a, r.Ticket)
				if commandInvocationCount(t, r.CommandID) != 0 {
					t.Fatal("cancel before Prepare wrote ledger")
				}
				return
			}
			request := CommandPageMutationRequest{TargetID: list.ID, ExpectedFingerprint: target.Fingerprint, Title: "Physical copy"}
			if scenario == "foreign-target" {
				foreign, err := database.CreateTaskListWithContext(database.WithUserID(context.Background(), "foreign-owner"), "Foreign", "", nil, "")
				if err != nil {
					t.Fatal(err)
				}
				request.TargetID = foreign.ID
			}
			if scenario == "changed-before-prepare" {
				if err = database.UpdateTaskListWithContext(ctx, list.ID, "Concurrent", ""); err != nil {
					t.Fatal(err)
				}
			}
			err = a.PreparePageMutationCommand(r.Ticket, request)
			if scenario == "foreign-target" || scenario == "changed-before-prepare" {
				if err == nil {
					t.Fatal("invalid target prepared")
				}
				_ = a.CancelUICommand(r.Ticket)
				assertPageDeckCleanup(t, a, r.Ticket)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "disconnect-before-take" {
				controller.reset("test-deck")
				if _, err = a.TakeUICommand(r.Ticket); err == nil {
					t.Fatal("disconnected source handed off")
				}
			} else {
				h := takeUICommandFor(t, a, r.Ticket, r.CommandID)
				switch scenario {
				case "changed-before-commit":
					err = database.UpdateTaskListWithContext(ctx, list.ID, "Concurrent", "")
				case "disconnect-before-commit":
					controller.reset("test-deck")
				case "ABA-before-commit":
					if err = a.workspaceMgr.SetProfile("other"); err == nil {
						err = a.workspaceMgr.SetProfile("dev")
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				if err = a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err == nil {
					t.Fatal("stale source/target committed")
				}
			}
			_ = a.CancelUICommand(r.Ticket)
			assertPageDeckCleanup(t, a, r.Ticket)
			lists, err := database.GetAllTaskListsWithContext(ctx)
			if err != nil || len(lists) != 1 {
				t.Fatalf("invalid clone persisted: %v %v", lists, err)
			}
		})
	}
}
