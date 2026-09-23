package app

import (
	"context"
	"testing"

	"assistente/internal/commanddecision"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
	"assistente/internal/tasklist"
	"assistente/internal/workspace"
)

func TestCommandTaskListClearDecisionAndCommit(t *testing.T) {
	for _, scenario := range []string{"apply", "decline", "concurrent-task", "cancel"} {
		t.Run(scenario, func(t *testing.T) {
			a, events, _ := clearCommandFixture(t, workspace.TabTypeTasklist)
			if err := database.DB().AutoMigrate(&database.TaskList{}, &database.TaskListWorkflow{}, &database.Task{}, &database.TaskNote{}); err != nil {
				t.Fatal(err)
			}
			a.taskSvc = tasklist.NewService(tasklist.ServiceConfig{Store: tasklist.NewDBStore()})
			ctx := database.WithUserID(context.Background(), a.commandProduct.Load().principal.UserID)
			list, err := database.CreateTaskListWithContext(ctx, "Keep this list", "Keep metadata", nil, "")
			if err != nil {
				t.Fatal(err)
			}
			task, err := database.CreateTaskWithContext(ctx, list.ID, "Original", "", "", "", nil)
			if err != nil {
				t.Fatal(err)
			}
			target, err := a.ReadTaskListCommandTarget(list.ID)
			if err != nil {
				t.Fatal(err)
			}
			r, err := a.BeginUICommand("tasklists.clear")
			if err != nil {
				t.Fatal(err)
			}
			if err = a.PreparePageMutationCommand(r.Ticket, CommandPageMutationRequest{TargetID: list.ID, ExpectedFingerprint: target.Fingerprint}); err != nil {
				t.Fatal(err)
			}
			payload := receiveCommandDecisionEvent(t, events)
			if _, err = database.GetTaskWithContext(ctx, task.ID); err != nil {
				t.Fatal("cleared before decision", err)
			}
			action := commanddecision.ApplyAction
			if scenario == "decline" {
				action = commanddecision.DenyAction
			}
			finishCommandDecision(t, a.questionnaireMgr, payload, map[string]any{questionnaire.AnswerActionID: action}, false)
			h, takeErr := a.TakeUICommand(r.Ticket)
			if scenario == "decline" {
				if takeErr == nil {
					t.Fatal("declined clear received handoff")
				}
			} else {
				if takeErr != nil {
					t.Fatal(takeErr)
				}
				if scenario == "concurrent-task" {
					if _, err = database.CreateTaskWithContext(ctx, list.ID, "Created after approval", "", "", "", nil); err != nil {
						t.Fatal(err)
					}
				}
				if scenario == "cancel" {
					if err = a.CancelUICommand(r.Ticket); err != nil {
						t.Fatal(err)
					}
				}
				err = a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID)
				if scenario == "apply" {
					if err != nil {
						t.Fatal(err)
					}
					if got := getUIResultEventually(t, a, r.Ticket); got.Status != "succeeded" {
						t.Fatalf("result: %+v", got)
					}
					result, err := a.GetPageMutationCommandResult(r.Ticket)
					if err != nil || result.ID != list.ID {
						t.Fatalf("result: %+v %v", result, err)
					}
					if err = a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err == nil {
						t.Fatal("replay committed")
					}
				} else if err == nil {
					t.Fatal("stale/cancelled clear committed")
				}
			}
			current, err := database.GetTaskListMetadataWithContext(ctx, list.ID)
			if err != nil || current.Title != list.Title || current.Description != list.Description || current.Workflow == nil {
				t.Fatalf("list/workflow lost: %+v %v", current, err)
			}
			_, err = database.GetTaskWithContext(ctx, task.ID)
			if scenario == "apply" && err == nil {
				t.Fatal("task not cleared")
			}
			if scenario != "apply" && err != nil {
				t.Fatal("task lost despite rejected commit", err)
			}
		})
	}
}
