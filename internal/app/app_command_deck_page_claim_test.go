package app

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/workspace"
)

// Install only after Take has completed. The wrapper deliberately keeps the
// successful optimistic answer even after the source changes: the final private
// source guard, not a second call to this predicate, must close the frontier.
func pageDeckChangeAfterPreflight(t *testing.T, a *App, ticket string, change func() error) func() {
	t.Helper()
	p := a.commandProduct.Load()
	p.mu.Lock()
	run := p.uiRuns[ticket]
	p.mu.Unlock()
	if run == nil || run.sourceValid == nil || !run.sourceValid() {
		t.Fatal("fixture did not reach a valid physical handoff")
	}
	checked := make(chan struct{})
	changed := make(chan struct{})
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	var once sync.Once
	var changeErr error
	var optimisticValidated bool
	go func() {
		select {
		case <-checked:
		case <-stop:
			return
		}
		changeErr = change()
		close(changed)
	}()
	original := run.sourceValid
	run.sourceValid = func() bool {
		once.Do(func() {
			optimisticValidated = original()
			close(checked)
			<-changed
		})
		return true
	}
	return func() {
		t.Helper()
		select {
		case <-changed:
			if !optimisticValidated {
				t.Fatal("source was already stale before injected change")
			}
			if changeErr != nil {
				t.Fatal(changeErr)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("commit did not reach the optimistic source check")
		}
	}
}

func TestContextualDeckPageClaimRechecksSourceAfterPreflight(t *testing.T) {
	for _, domain := range []string{"tasklists", "profiles"} {
		for _, scenario := range []string{"unchanged", "reset", "replaced", "expired", "disconnect", "occurrence-removed"} {
			t.Run(domain+"/"+scenario, func(t *testing.T) {
				var a *App
				var decisions <-chan map[string]any
				var taskCtx context.Context
				var targetID, fingerprint, originalActive string
				id := "tasklists.duplicate"
				if domain == "profiles" {
					id = "profiles.activate"
					a, decisions, targetID = profileCommandFixture(t)
					originalActive = a.profileManager.GetActiveSlug()
				} else {
					var list *database.TaskList
					a, decisions, taskCtx, list = pagePaletteTaskFixture(t, workspace.TabTypeChat)
					targetID = list.ID
				}
				bindingID := configurePageDeckCommand(t, a, decisions, id, domain)
				if domain == "profiles" {
					target, err := a.ReadProfileCommandTarget(targetID)
					if err != nil {
						t.Fatal(err)
					}
					fingerprint = target.Fingerprint
				} else {
					target, err := a.ReadTaskListCommandTarget(targetID)
					if err != nil {
						t.Fatal(err)
					}
					fingerprint = target.Fingerprint
				}
				event, controller := pageDeckOffer(t, a)
				r := beginPageDeckCommand(t, a, event, id, domain)
				request := CommandPageMutationRequest{TargetID: targetID, ExpectedFingerprint: fingerprint}
				if domain == "tasklists" {
					request.Title = "Claim boundary copy"
				}
				if err := a.PreparePageMutationCommand(r.Ticket, request); err != nil {
					t.Fatal(err)
				}
				h := takeUICommandFor(t, a, r.Ticket, id)
				p := a.commandProduct.Load()
				var verifyChange func()
				if scenario != "unchanged" {
					verifyChange = pageDeckChangeAfterPreflight(t, a, r.Ticket, func() error {
						switch scenario {
						case "reset":
							p.clearLocalCommandKeyboard(event.Generation)
						case "replaced":
							p.clearLocalCommandKeyboard(event.Generation)
							fresh, err := a.GetLocalCommandKeyboardMap()
							if err != nil {
								return err
							}
							if fresh.Generation == event.Generation {
								return fmt.Errorf("replacement reused retired generation")
							}
						case "expired":
							p.keyboardMu.Lock()
							p.keyboardMap.view.ValidUntil = time.Now().Add(-time.Second).UnixMilli()
							p.keyboardMu.Unlock()
						case "disconnect":
							controller.reset("test-deck")
						case "occurrence-removed":
							p.deckExecution.remove(r.InvocationID)
						}
						return nil
					})
				}
				err := a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID)
				if verifyChange != nil {
					verifyChange()
				}
				if scenario == "unchanged" {
					if err != nil {
						t.Fatal(err)
					}
					assertPageDeckSucceeded(t, a, controller, r, bindingID)
					if domain == "profiles" {
						if a.profileManager.GetActiveSlug() != targetID {
							t.Fatal("valid profile was not activated")
						}
						fresh, err := a.GetLocalCommandKeyboardMap()
						if err != nil || fresh.Generation == "" || fresh.Generation == event.Generation {
							t.Fatalf("self publication failed: %+v %v", fresh, err)
						}
					} else {
						lists, err := database.GetAllTaskListsWithContext(taskCtx)
						if err != nil || len(lists) != 2 {
							t.Fatalf("valid clone missing: %v %v", lists, err)
						}
					}
					return
				}
				if err == nil {
					t.Fatal("stale source crossed final claim/admission")
				}
				_ = a.CancelUICommand(r.Ticket)
				assertPageDeckCleanup(t, a, r.Ticket)
				var succeeded int64
				if err := database.DB().Table("command_invocations").Where("invocation_id = ? AND status = ?", r.InvocationID, "succeeded").Count(&succeeded).Error; err != nil || succeeded != 0 {
					t.Fatalf("stale source reported success: %d %v", succeeded, err)
				}
				if domain == "profiles" {
					if a.profileManager.GetActiveSlug() != originalActive {
						t.Fatal("stale claim persisted profile activation")
					}
					current, err := a.ReadProfileCommandTarget(targetID)
					if err != nil || current.Fingerprint != fingerprint {
						t.Fatalf("stale claim changed profile: %+v %v", current, err)
					}
				} else {
					lists, err := database.GetAllTaskListsWithContext(taskCtx)
					if err != nil || len(lists) != 1 || lists[0].ID != targetID {
						t.Fatalf("stale admission persisted clone: %v %v", lists, err)
					}
				}
			})
		}
	}
}
