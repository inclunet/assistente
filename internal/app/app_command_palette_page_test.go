package app

import (
	"context"
	"testing"
	"time"

	"assistente/internal/commandautomation"
	"assistente/internal/commanddecision"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
	"assistente/internal/tasklist"
	"assistente/internal/workspace"
)

func configurePagePaletteCommand(t *testing.T, a *App, events <-chan map[string]any, id, surface string, extra ...CommandSettingsConditionClause) LocalCommandKeyboardMap {
	t.Helper()
	if err := commandautomation.Migrate(context.Background(), database.DB()); err != nil {
		t.Fatal(err)
	}
	if err := a.workspaceMgr.SetProfile("dev"); err != nil {
		t.Fatal(err)
	}
	layer := settingsContractApply(t, a, events, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Page " + id, Enabled: true}})
	settingsContractApply(t, a, events, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "always", Lifecycle: "persistent", Enabled: true}})
	clauses := append([]CommandSettingsConditionClause{{Field: "app.focused", Value: true}, {Field: "surface.type", Value: surface}, {Field: "profile", Value: "dev"}}, extra...)
	settingsContractApply(t, a, events, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &CommandSettingsBindingInput{LayerID: layer.ID, CommandID: id, TriggerType: "palette", TriggerSpec: `{"version":1,"selection":"` + id + `"}`, Arguments: map[string]any{}, Effect: "execute", Enabled: true, Condition: &CommandSettingsCondition{Version: 1, Clauses: clauses}}})
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func pagePaletteTaskFixture(t *testing.T, kind workspace.TabType) (*App, <-chan map[string]any, context.Context, *database.TaskList) {
	t.Helper()
	a, events, _ := clearCommandFixture(t, kind)
	if err := database.DB().AutoMigrate(&database.TaskList{}, &database.TaskListWorkflow{}, &database.Task{}, &database.TaskNote{}); err != nil {
		t.Fatal(err)
	}
	a.taskSvc = tasklist.NewService(tasklist.ServiceConfig{Store: tasklist.NewDBStore()})
	ctx := database.WithUserID(context.Background(), a.commandProduct.Load().principal.UserID)
	list, err := database.CreateTaskListWithContext(ctx, "Captured page target", "Keep metadata", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.CreateTaskWithContext(ctx, list.ID, "Original task", "", "", "", nil); err != nil {
		t.Fatal(err)
	}
	return a, events, ctx, list
}

func TestContextualPagePaletteTaskOperations(t *testing.T) {
	for _, scenario := range []struct {
		id, surface string
		cancel      bool
	}{
		{"tasklists.duplicate", "tasklists", false}, {"tasklists.delete", "tasklists", false}, {"tasklists.clear", "tasklists", false},
		{"tasklists.delete", "tasklists", true}, {"tasklists.clear", "tasklists", true},
		{"tasklists.duplicate", "tasklist", false}, {"tasklists.clear", "tasklist", false},
	} {
		t.Run(scenario.id+"/"+scenario.surface+map[bool]string{true: "/cancel", false: "/apply"}[scenario.cancel], func(t *testing.T) {
			kind := workspace.TabTypeChat
			if scenario.surface == "tasklist" {
				kind = workspace.TabTypeTasklist
			}
			a, events, ctx, list := pagePaletteTaskFixture(t, kind)
			view := configurePagePaletteCommand(t, a, events, scenario.id, scenario.surface)
			target, err := a.ReadTaskListCommandTarget(list.ID)
			if err != nil {
				t.Fatal(err)
			}
			r, err := a.BeginContextualPagePaletteUICommand(view.Generation, scenario.id, LocalCommandKeyboardContext{SurfaceType: scenario.surface, Profile: "dev"})
			if err != nil {
				t.Fatal(err)
			}
			request := CommandPageMutationRequest{TargetID: list.ID, ExpectedFingerprint: target.Fingerprint}
			if scenario.id == "tasklists.duplicate" {
				request.Title = "Captured copy"
			}
			if err = a.PreparePageMutationCommand(r.Ticket, request); err != nil {
				t.Fatal(err)
			}
			if pageMutationDestructive(scenario.id) {
				payload := receiveCommandDecisionEvent(t, events)
				tasks, err := database.GetTasksByTaskListIDWithContext(ctx, list.ID)
				if err != nil || len(tasks) != 1 {
					t.Fatalf("effect before decision: %v %v", tasks, err)
				}
				if scenario.cancel {
					finishCommandDecision(t, a.questionnaireMgr, payload, nil, true)
					if result := getUIResultEventually(t, a, r.Ticket); result.Status == "succeeded" {
						t.Fatal("cancel succeeded")
					}
					tasks, err = database.GetTasksByTaskListIDWithContext(ctx, list.ID)
					if err != nil || len(tasks) != 1 {
						t.Fatalf("cancel changed target: %v %v", tasks, err)
					}
					return
				}
				finishCommandDecision(t, a.questionnaireMgr, payload, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
			}
			h := takeUICommandFor(t, a, r.Ticket, scenario.id)
			if err = a.CompleteUICommand(r.Ticket, h.HandoffID, "succeeded"); err == nil {
				t.Fatal("frontend forged backend success")
			}
			if err = a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err != nil {
				t.Fatal(err)
			}
			assertSurfacePaletteSucceeded(t, a, r.Ticket, r.InvocationID)
			if err = a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err == nil {
				t.Fatal("replay accepted")
			}
			switch scenario.id {
			case "tasklists.delete":
				if _, err = database.GetTaskListMetadataWithContext(ctx, list.ID); err == nil {
					t.Fatal("target not deleted")
				}
			case "tasklists.clear":
				current, err := database.GetTaskListMetadataWithContext(ctx, list.ID)
				if err != nil || current.Description != list.Description {
					t.Fatalf("list metadata lost: %v %v", current, err)
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
				tasks, err := database.GetTasksByTaskListIDWithContext(ctx, result.ID)
				// Domain duplication copies metadata/workflow, deliberately not tasks.
				if err != nil || len(tasks) != 0 {
					t.Fatalf("clone unexpectedly copied tasks: %v %v", tasks, err)
				}
				copy, err := database.GetTaskListWithContext(ctx, result.ID)
				if err != nil || copy.Title != "Captured copy" || copy.Description != list.Description || copy.Workflow == nil {
					t.Fatalf("clone metadata/workflow missing: %+v %v", copy, err)
				}
			}
		})
	}
}

func TestContextualPagePaletteTaskRefusesStaleAndForeignTargets(t *testing.T) {
	for _, scenario := range []string{"fingerprint-prepare", "fingerprint-commit", "foreign-owner", "profile-ABA", "map", "cancel", "ordinary", "ABA-before-take", "deadline", "session"} {
		t.Run(scenario, func(t *testing.T) {
			a, events, ctx, list := pagePaletteTaskFixture(t, workspace.TabTypeChat)
			view := configurePagePaletteCommand(t, a, events, "tasklists.duplicate", "tasklists")
			target, err := a.ReadTaskListCommandTarget(list.ID)
			if err != nil {
				t.Fatal(err)
			}
			r, err := a.BeginContextualPagePaletteUICommand(view.Generation, "tasklists.duplicate", LocalCommandKeyboardContext{SurfaceType: "tasklists", Profile: "dev"})
			if scenario == "ordinary" {
				if err == nil {
					_ = a.CancelUICommand(r.Ticket)
				}
				r, err = a.BeginUICommand("tasklists.duplicate")
			}
			if err != nil {
				if scenario == "ordinary" {
					return
				}
				t.Fatal(err)
			}
			request := CommandPageMutationRequest{TargetID: list.ID, ExpectedFingerprint: target.Fingerprint, Title: "Captured copy"}
			if scenario == "foreign-owner" {
				foreign, err := database.CreateTaskListWithContext(database.WithUserID(context.Background(), "foreign-owner"), "Foreign", "", nil, "")
				if err != nil {
					t.Fatal(err)
				}
				request.TargetID = foreign.ID
			}
			if scenario == "fingerprint-prepare" {
				if err = database.UpdateTaskListWithContext(ctx, list.ID, "Concurrent", ""); err != nil {
					t.Fatal(err)
				}
			}
			err = a.PreparePageMutationCommand(r.Ticket, request)
			if scenario == "foreign-owner" || scenario == "fingerprint-prepare" {
				if err == nil {
					t.Fatal("invalid target prepared")
				}
				return
			}
			if err != nil {
				if scenario == "ordinary" {
					return
				}
				t.Fatal(err)
			}
			if scenario == "ordinary" {
				if _, err = a.TakeUICommand(r.Ticket); err == nil {
					t.Fatal("ordinary ingress supplied visual facts")
				}
				return
			}
			if scenario == "ABA-before-take" {
				if err = a.workspaceMgr.SetProfile("other"); err != nil {
					t.Fatal(err)
				}
				if err = a.workspaceMgr.SetProfile("dev"); err != nil {
					t.Fatal(err)
				}
				if _, err = a.TakeUICommand(r.Ticket); err == nil {
					t.Fatal("ABA handed off")
				}
				return
			}
			h := takeUICommandFor(t, a, r.Ticket, r.CommandID)
			switch scenario {
			case "fingerprint-commit":
				err = database.UpdateTaskListWithContext(ctx, list.ID, "Concurrent", "")
			case "profile-ABA":
				if err = a.workspaceMgr.SetProfile("other"); err == nil {
					err = a.workspaceMgr.SetProfile("dev")
				}
			case "map":
				err = a.resetCommandLifecycleIfConfigured(context.Background(), "page-test-stale")
			case "cancel":
				err = a.CancelUICommand(r.Ticket)
			case "deadline":
				p := a.commandProduct.Load()
				p.keyboardMu.Lock()
				p.keyboardMap.view.ValidUntil = time.Now().Add(-time.Second).UnixMilli()
				p.keyboardMu.Unlock()
			case "session":
				err = database.DB().Exec("UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", a.commandProduct.Load().principal.SessionID).Error
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err == nil {
				t.Fatal("stale/cancelled effect committed")
			}
			lists, err := database.GetAllTaskListsWithContext(ctx)
			if err != nil || len(lists) != 1 {
				t.Fatalf("invalid clone persisted: %v %v", lists, err)
			}
		})
	}
}

func TestContextualPagePaletteAdmissionClosed(t *testing.T) {
	a, events, _, _ := pagePaletteTaskFixture(t, workspace.TabTypeChat)
	view := configurePagePaletteCommand(t, a, events, "tasklists.duplicate", "tasklists")
	for _, c := range []struct{ id, surface, profile, generation string }{
		{"tasklists.duplicate", "profiles", "dev", view.Generation}, {"tasklists.duplicate", "tasklist", "dev", view.Generation},
		{"tasklists.delete", "tasklist", "dev", view.Generation}, {"profiles.activate", "tasklists", "dev", view.Generation},
		{"tasklists.create", "tasklists", "dev", view.Generation}, {"profiles.update", "profiles", "dev", view.Generation},
		{"layer.back", "tasklists", "dev", view.Generation}, {"tasklists.duplicate", "tasklists", "wrong", view.Generation},
		{"tasklists.duplicate", "tasklists", "", view.Generation}, {"tasklists.duplicate", "tasklists", "dev", "stale"},
	} {
		if r, err := a.BeginContextualPagePaletteUICommand(c.generation, c.id, LocalCommandKeyboardContext{SurfaceType: c.surface, Profile: c.profile}); err == nil || r.Ticket != "" {
			t.Fatalf("invalid admission %+v: %+v %v", c, r, err)
		}
	}
}

func TestContextualPagePaletteRejectsSurfaceIDFact(t *testing.T) {
	a, events, _, _ := pagePaletteTaskFixture(t, workspace.TabTypeChat)
	view := configurePagePaletteCommand(t, a, events, "tasklists.duplicate", "tasklists", CommandSettingsConditionClause{Field: "surface.id", Value: "target-id"})
	if r, err := a.BeginContextualPagePaletteUICommand(view.Generation, "tasklists.duplicate", LocalCommandKeyboardContext{SurfaceType: "tasklists", Profile: "dev"}); err == nil || r.Ticket != "" {
		t.Fatalf("page accepted DOM target fact: %+v %v", r, err)
	}
}

func TestContextualPagePaletteProfileOperations(t *testing.T) {
	for _, operation := range []string{"duplicate", "delete", "activate", "delete-cancel"} {
		t.Run(operation, func(t *testing.T) {
			a, events, slug := profileCommandFixture(t)
			id := "profiles." + operation
			if operation == "delete-cancel" {
				id = "profiles.delete"
			}
			view := configurePagePaletteCommand(t, a, events, id, "profiles")
			target, err := a.ReadProfileCommandTarget(slug)
			if err != nil {
				t.Fatal(err)
			}
			r, err := a.BeginContextualPagePaletteUICommand(view.Generation, id, LocalCommandKeyboardContext{SurfaceType: "profiles", Profile: "dev"})
			if err != nil {
				t.Fatal(err)
			}
			if err = a.PreparePageMutationCommand(r.Ticket, CommandPageMutationRequest{TargetID: slug, ExpectedFingerprint: target.Fingerprint}); err != nil {
				t.Fatal(err)
			}
			if id == "profiles.delete" {
				payload := receiveCommandDecisionEvent(t, events)
				if _, err = a.profileManager.Get(slug); err != nil {
					t.Fatal("effect before approval", err)
				}
				if operation == "delete-cancel" {
					finishCommandDecision(t, a.questionnaireMgr, payload, nil, true)
					if got := getUIResultEventually(t, a, r.Ticket); got.Status == "succeeded" {
						t.Fatal("cancel succeeded")
					}
					if _, err = a.profileManager.Get(slug); err != nil {
						t.Fatal("cancel deleted profile", err)
					}
					return
				}
				finishCommandDecision(t, a.questionnaireMgr, payload, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
			}
			h := takeUICommandFor(t, a, r.Ticket, id)
			if err = a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err != nil {
				t.Fatal(err)
			}
			assertSurfacePaletteSucceeded(t, a, r.Ticket, r.InvocationID)
			result, err := a.GetPageMutationCommandResult(r.Ticket)
			if err != nil || result.ID == "" {
				t.Fatalf("missing result: %+v %v", result, err)
			}
			if operation == "activate" && a.profileManager.GetActiveSlug() != slug {
				t.Fatal("profile not activated")
			}
			if operation == "duplicate" && result.ID == slug {
				t.Fatal("duplicate reused source")
			}
			if operation == "delete" {
				if _, err = a.profileManager.Get(slug); err == nil {
					t.Fatal("profile not deleted")
				}
			}
			fresh, err := a.GetLocalCommandKeyboardMap()
			if err != nil || fresh.Generation == view.Generation || fresh.Generation == "" {
				t.Fatalf("own invalidation did not publish fresh map: %+v %v", fresh, err)
			}
			if err = a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err == nil {
				t.Fatal("profile replay accepted")
			}
		})
	}
}
