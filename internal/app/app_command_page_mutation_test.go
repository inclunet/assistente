package app

import (
	"assistente/internal/commandcatalog"
	"assistente/internal/commanddecision"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
	"assistente/internal/tasklist"
	"assistente/internal/workspace"
	"context"
	"testing"
)

func TestCommandPageMutationCatalogRequiresTaskService(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	p := a.commandProduct.Load()
	for _, id := range commandPageMutationIDs {
		d, ok := p.registry.Lookup(id)
		if !ok || !isWorkspaceMutationCommand(id) || isLocalUICommand(id) || !d.HasMutableTarget || d.Persistence.Arguments != commandcatalog.PersistenceNever || d.Persistence.Result != commandcatalog.PersistenceNever {
			t.Fatalf("invalid command contract: %+v", d)
		}
		if pageMutationDestructive(id) != (d.Decision == commandcatalog.Interactive && d.Effect == commandcatalog.Destructive) {
			t.Fatalf("wrong destructive policy: %+v", d)
		}
		if r, err := a.BeginUICommand(id); err == nil || r.Ticket != "" {
			t.Fatalf("accepted missing service: %s %+v %v", id, r, err)
		}
	}
}

func TestCommandPageMutationDeleteRequiresDecisionAndDeletesOnce(t *testing.T) {
	a, events, _ := clearCommandFixture(t, workspace.TabTypeChat)
	if err := database.DB().AutoMigrate(&database.TaskList{}, &database.TaskListWorkflow{}, &database.Task{}, &database.TaskNote{}); err != nil {
		t.Fatal(err)
	}
	a.taskSvc = tasklist.NewService(tasklist.ServiceConfig{Store: tasklist.NewDBStore()})
	p := a.commandProduct.Load()
	ctx := database.WithUserID(context.Background(), p.principal.UserID)
	list, err := database.CreateTaskListWithContext(ctx, "Delete captured list", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	target, err := a.ReadTaskListCommandTarget(list.ID)
	if err != nil {
		t.Fatal(err)
	}
	reservation, err := a.BeginUICommand("tasklists.delete")
	if err != nil {
		t.Fatal(err)
	}
	if err = a.PreparePageMutationCommand(reservation.Ticket, CommandPageMutationRequest{TargetID: list.ID, ExpectedFingerprint: target.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	payload := receiveCommandDecisionEvent(t, events)
	if _, err = database.GetTaskListMetadataWithContext(ctx, list.ID); err != nil {
		t.Fatal("deleted before confirmation", err)
	}
	finishCommandDecision(t, a.questionnaireMgr, payload, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
	handoff, err := a.TakeUICommand(reservation.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.CompleteUICommand(reservation.Ticket, handoff.HandoffID, "succeeded"); err == nil {
		t.Fatal("frontend bypassed backend effect")
	}
	if err = a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
		t.Fatal(err)
	}
	if got := getUIResultEventually(t, a, reservation.Ticket); got.Status != "succeeded" {
		t.Fatalf("unexpected result: %+v", got)
	}
	if _, err = database.GetTaskListMetadataWithContext(ctx, list.ID); err == nil {
		t.Fatal("list still present")
	}
	if err = a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err == nil {
		t.Fatal("replay accepted")
	}
}

func TestCommandPageMutationCreateAndUpdateRealDatabase(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	if err := database.DB().AutoMigrate(&database.TaskList{}, &database.TaskListWorkflow{}, &database.Task{}, &database.TaskNote{}); err != nil {
		t.Fatal(err)
	}
	a.taskSvc = tasklist.NewService(tasklist.ServiceConfig{Store: tasklist.NewDBStore()})
	p := a.commandProduct.Load()
	ctx := database.WithUserID(context.Background(), p.principal.UserID)
	execute := func(id string, request CommandPageMutationRequest) CommandPageMutationResult {
		t.Helper()
		reservation, err := a.BeginUICommand(id)
		if err != nil {
			t.Fatal(err)
		}
		if err = a.PreparePageMutationCommand(reservation.Ticket, request); err != nil {
			t.Fatal(err)
		}
		handoff, err := a.TakeUICommand(reservation.Ticket)
		if err != nil {
			t.Fatal(err)
		}
		if err = a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
			t.Fatal(err)
		}
		result, err := a.GetPageMutationCommandResult(reservation.Ticket)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	created := execute("tasklists.create", CommandPageMutationRequest{Title: "Captured list", Description: "original"})
	target, err := a.ReadTaskListCommandTarget(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	execute("tasklists.update", CommandPageMutationRequest{TargetID: created.ID, ExpectedFingerprint: target.Fingerprint, Title: "Updated", Description: "sealed"})
	list, err := database.GetTaskListMetadataWithContext(ctx, created.ID)
	if err != nil || list.Title != "Updated" || list.Description != "sealed" {
		t.Fatalf("unexpected persisted result: %+v %v", list, err)
	}
	target, err = a.ReadTaskListCommandTarget(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	clone := execute("tasklists.duplicate", CommandPageMutationRequest{TargetID: created.ID, ExpectedFingerprint: target.Fingerprint, Title: "Copy"})
	if clone.ID == created.ID || clone.Title != "Copy" {
		t.Fatalf("invalid duplicate: %+v", clone)
	}
}

func TestCommandPageMutationRechecksDatabaseAtCommit(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	if err := database.DB().AutoMigrate(&database.TaskList{}, &database.TaskListWorkflow{}, &database.Task{}, &database.TaskNote{}); err != nil {
		t.Fatal(err)
	}
	a.taskSvc = tasklist.NewService(tasklist.ServiceConfig{Store: tasklist.NewDBStore()})
	p := a.commandProduct.Load()
	ctx := database.WithUserID(context.Background(), p.principal.UserID)
	list, err := database.CreateTaskListWithContext(ctx, "Before", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	target, err := a.ReadTaskListCommandTarget(list.ID)
	if err != nil {
		t.Fatal(err)
	}
	reservation, err := a.BeginUICommand("tasklists.update")
	if err != nil {
		t.Fatal(err)
	}
	if err = a.PreparePageMutationCommand(reservation.Ticket, CommandPageMutationRequest{TargetID: list.ID, ExpectedFingerprint: target.Fingerprint, Title: "Stale"}); err != nil {
		t.Fatal(err)
	}
	handoff, err := a.TakeUICommand(reservation.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	if err = database.UpdateTaskListWithContext(ctx, list.ID, "Concurrent", ""); err != nil {
		t.Fatal(err)
	}
	if err = a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err == nil {
		t.Fatal("stale target committed")
	}
	current, err := database.GetTaskListMetadataWithContext(ctx, list.ID)
	if err != nil || current.Title != "Concurrent" {
		t.Fatalf("concurrent value overwritten: %+v %v", current, err)
	}
}

func TestCommandPageMutationStaleDraftCannotOverwrite(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	if err := database.DB().AutoMigrate(&database.TaskList{}, &database.TaskListWorkflow{}, &database.Task{}, &database.TaskNote{}); err != nil {
		t.Fatal(err)
	}
	a.taskSvc = tasklist.NewService(tasklist.ServiceConfig{Store: tasklist.NewDBStore()})
	p := a.commandProduct.Load()
	ctx := database.WithUserID(context.Background(), p.principal.UserID)
	list, err := database.CreateTaskListWithContext(ctx, "Before", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	target, err := a.ReadTaskListCommandTarget(list.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = database.UpdateTaskListWithContext(ctx, list.ID, "Concurrent", ""); err != nil {
		t.Fatal(err)
	}
	reservation, err := a.BeginUICommand("tasklists.update")
	if err != nil {
		t.Fatal(err)
	}
	if err = a.PreparePageMutationCommand(reservation.Ticket, CommandPageMutationRequest{TargetID: list.ID, ExpectedFingerprint: target.Fingerprint, Title: "Stale"}); err == nil {
		t.Fatal("stale draft accepted")
	}
	current, err := database.GetTaskListMetadataWithContext(ctx, list.ID)
	if err != nil || current.Title != "Concurrent" {
		t.Fatalf("concurrent value overwritten: %+v %v", current, err)
	}
}
