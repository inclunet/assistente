package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"assistente/controllers"
	"assistente/internal/commandcatalog"
	"assistente/internal/commanddecision"
	"assistente/internal/commandexecution"
	"assistente/internal/commandruntime"
	"assistente/internal/database"
	"assistente/internal/jobprofilegrant"
	"assistente/internal/profiles"
	"assistente/internal/questionnaire"
	"assistente/internal/workspace"
)

func profileCommandFixture(t *testing.T) (*App, <-chan map[string]any, string) {
	t.Helper()
	a, events, _ := clearCommandFixture(t, workspace.TabTypeChat)
	prepareProfileMutationAppFixture(t, a)
	if err := database.DB().AutoMigrate(&database.ToolCatalog{}, &database.Job{}, &database.JobProfileGrant{}, &database.JobProfileGrantEpoch{}, &database.ProfileGrantRevocationIntent{}); err != nil {
		t.Fatal(err)
	}
	a.jobGrantStore = jobprofilegrant.NewStore(database.DB())
	a.profileAccess.WithJobGrants(a.jobGrantStore)
	base := profiles.DefaultProfile()
	base.Name, base.Active = "Base profile", true
	if _, err := a.profileManager.Create(base); err != nil {
		t.Fatal(err)
	}
	target := profiles.DefaultProfile()
	target.Name, target.Active = "Captured profile", false
	slug, err := a.profileManager.Create(target)
	if err != nil {
		t.Fatal(err)
	}
	a.profilesCtrl = controllers.NewProfilesController(controllers.ProfilesControllerConfig{ProfileMgr: a.profileManager, CommitProfileMutation: a.commitProfileMutation})
	return a, events, slug
}

func TestCommandProfileMutationClaimRechecksChangesAfterPreflight(t *testing.T) {
	for _, scenario := range []string{"host", "workspace", "source"} {
		t.Run(scenario, func(t *testing.T) {
			a, _, slug := profileCommandFixture(t)
			target, err := a.ReadProfileCommandTarget(slug)
			if err != nil {
				t.Fatal(err)
			}
			r := beginUICommand(t, a, "profiles.activate")
			if err := a.PreparePageMutationCommand(r.Ticket, CommandPageMutationRequest{TargetID: slug, ExpectedFingerprint: target.Fingerprint}); err != nil {
				t.Fatal(err)
			}
			takeUICommandFor(t, a, r.Ticket, r.CommandID)
			p := a.commandProduct.Load()
			p.mu.Lock()
			run := p.uiRuns[r.Ticket]
			epoch := run.admission.epoch
			p.mu.Unlock()
			ctx := context.Background()
			finish, err := p.epochs.BeginTransitionFromSnapshot(ctx, epoch, func(ctx context.Context) error {
				if err := a.validatePageMutationOrigin(ctx, p, run); err != nil {
					return err
				}
				switch scenario {
				case "host":
					return p.host.SetVaultUnlocked(ctx, false)
				case "workspace":
					return p.workspaceMgr.AddTab(workspace.Tab{ID: "changed-before-claim", Type: workspace.TabTypeChat})
				case "source":
					run.sourceValid = func() bool { return false }
				}
				return nil
			}, func(ctx context.Context) error { return a.claimProfileCommand(ctx, p, run) })
			if finish != nil {
				finish()
				t.Fatal("invalid local state claimed transition")
			}
			if err == nil {
				t.Fatal("accepted change after preflight")
			}
			// A recusa não pode consumir ownership: prova sem iniciar escrita.
			if err := run.pageMutation.ownership.Claim(ctx); err != nil && !errors.Is(err, commandexecution.ErrCommitOwnershipAborted) {
				t.Fatalf("claim was consumed before validation: %v", err)
			}
			run.cancel()
			_ = p.ui.Cancel(p.uiOwner(), r.Ticket)
			if a.profileManager.GetActiveSlug() == slug {
				t.Fatal("invalid transition changed profile")
			}
		})
	}
}

func TestCommandProfileMutationUnknownPublicationKeepsMapClosedAndResult(t *testing.T) {
	a, _, slug := profileCommandFixture(t)
	target, err := a.ReadProfileCommandTarget(slug)
	if err != nil {
		t.Fatal(err)
	}
	target.Profile.Description = "Persisted before publication failure"
	r := beginUICommand(t, a, "profiles.update")
	if err := a.PreparePageMutationCommand(r.Ticket, CommandPageMutationRequest{TargetID: slug, ExpectedFingerprint: target.Fingerprint, Profile: target.Profile}); err != nil {
		t.Fatal(err)
	}
	handoff := takeUICommandFor(t, a, r.Ticket, r.CommandID)
	p := a.commandProduct.Load()
	p.mu.Lock()
	p.uiRuns[r.Ticket].pageMutation.profileMutation.Publish = func(string) error { return errors.New("publication failed") }
	p.mu.Unlock()
	if err := a.CommitWorkspaceTabCommand(r.Ticket, handoff.HandoffID); err == nil {
		t.Fatal("publication failure concealed")
	}
	if got := getUIResultEventually(t, a, r.Ticket); got.Status != "outcome_unknown" {
		t.Fatalf("wrong outcome: %+v", got)
	}
	value, err := a.profileManager.Get(slug)
	if err != nil || value.Description != "Persisted before publication failure" {
		t.Fatalf("write missing: %+v %v", value, err)
	}
	if _, err := a.GetPageMutationCommandResult(r.Ticket); err == nil {
		t.Fatal("unknown exposed as success")
	}
	state, err := CommandLifecycleSnapshot(a)
	if err != nil || state.Published || state.State == commandruntime.StateReady {
		t.Fatalf("unknown reopened map: %+v %v", state, err)
	}
}

func TestCommandProfileMutationDeleteDeclinedDoesNotWrite(t *testing.T) {
	a, events, slug := profileCommandFixture(t)
	target, err := a.ReadProfileCommandTarget(slug)
	if err != nil {
		t.Fatal(err)
	}
	r := beginUICommand(t, a, "profiles.delete")
	if err := a.PreparePageMutationCommand(r.Ticket, CommandPageMutationRequest{TargetID: slug, ExpectedFingerprint: target.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	payload := receiveCommandDecisionEvent(t, events)
	finishCommandDecision(t, a.questionnaireMgr, payload, nil, true)
	if got := getUIResultEventually(t, a, r.Ticket); got.Status == "succeeded" {
		t.Fatal("declined delete succeeded")
	}
	if _, err := a.profileManager.Get(slug); err != nil {
		t.Fatal("declined delete wrote", err)
	}
}

func TestCommandProfileMutationFiveOperationsCommitAndReportSuccess(t *testing.T) {
	for _, operation := range []string{"create", "update", "duplicate", "delete", "activate"} {
		t.Run(operation, func(t *testing.T) {
			a, events, slug := profileCommandFixture(t)
			id := "profiles." + operation
			d, ok := a.commandProduct.Load().registry.Lookup(id)
			if !ok || !d.MutatesEffectiveCapability || d.HandlerClassification != commandcatalog.HandlerBackend || d.Persistence.Result != commandcatalog.PersistenceNever {
				t.Fatalf("wrong contract: %+v", d)
			}
			for _, source := range []commandcatalog.Source{commandcatalog.Palette, commandcatalog.KeyboardLocal, commandcatalog.StreamDeck} {
				if !d.AllowsSource(source) {
					t.Fatalf("missing source %s", source)
				}
			}
			if operation == "create" {
				slug = ""
			}
			target, err := a.ReadProfileCommandTarget(slug)
			if err != nil {
				t.Fatal(err)
			}
			request := CommandPageMutationRequest{TargetID: slug, ExpectedFingerprint: target.Fingerprint}
			if operation == "create" {
				request.Profile = profiles.DefaultProfile()
				request.Profile.Name = "Created by command"
				request.Profile.Active = false
			}
			if operation == "update" {
				request.Profile = target.Profile
				request.Profile.Description = "Saved by command"
			}
			reservation := beginUICommand(t, a, id)
			if err := a.PreparePageMutationCommand(reservation.Ticket, request); err != nil {
				t.Fatal(err)
			}
			if request.Profile != nil {
				request.Profile.Name = "Changed after preparation"
			}
			if operation == "delete" {
				payload := receiveCommandDecisionEvent(t, events)
				if _, err := a.profileManager.Get(slug); err != nil {
					t.Fatal("effect before decision", err)
				}
				finishCommandDecision(t, a.questionnaireMgr, payload, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
			}
			handoff := takeUICommandFor(t, a, reservation.Ticket, id)
			if err := a.CompleteUICommand(reservation.Ticket, handoff.HandoffID, "succeeded"); err == nil {
				t.Fatal("frontend forged success")
			}
			if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
				t.Fatal(err)
			}
			status := getUIResultEventually(t, a, reservation.Ticket)
			if status.Status != "succeeded" {
				t.Fatalf("own invalidation lost committed result: %+v", status)
			}
			var audit struct {
				ArgumentsSummary string
				ResultSummary    string
			}
			if err := database.DB().Table("command_invocations").Select("arguments_summary, result_summary").Where("invocation_id = ?", reservation.InvocationID).Take(&audit).Error; err != nil {
				t.Fatal(err)
			}
			if audit.ArgumentsSummary != `{"version":1,"redacted":true}` || strings.Contains(audit.ResultSummary, "Captured profile") || strings.Contains(audit.ResultSummary, "Saved by command") {
				t.Fatalf("profile content leaked to ledger: %+v", audit)
			}
			result, err := a.GetPageMutationCommandResult(reservation.Ticket)
			if err != nil || result.ID == "" {
				t.Fatalf("result missing: %+v %v", result, err)
			}
			if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err == nil {
				t.Fatal("replay accepted")
			}
			switch operation {
			case "delete":
				if _, err := a.profileManager.Get(slug); err == nil {
					t.Fatal("not deleted")
				}
			case "activate":
				if a.profileManager.GetActiveSlug() != slug {
					t.Fatal("not activated")
				}
			default:
				value, err := a.profileManager.Get(result.ID)
				if err != nil || value.Name == "Changed after preparation" {
					t.Fatalf("unsealed content: %+v %v", value, err)
				}
				if operation == "update" && value.Description != "Saved by command" {
					t.Fatal("update missing")
				}
				if operation == "duplicate" && result.ID == slug {
					t.Fatal("duplicate reused source")
				}
			}
			snapshot, err := CommandLifecycleSnapshot(a)
			if err != nil || !snapshot.Published || snapshot.State != commandruntime.StateReady {
				t.Fatalf("map not rebuilt: %+v %v", snapshot, err)
			}
		})
	}
}

func TestCommandProfileMutationRefusesStaleOrCancelledEffect(t *testing.T) {
	for _, scenario := range []string{"stale-file", "cancelled", "revoked-session"} {
		t.Run(scenario, func(t *testing.T) {
			a, _, slug := profileCommandFixture(t)
			target, err := a.ReadProfileCommandTarget(slug)
			if err != nil {
				t.Fatal(err)
			}
			target.Profile.Description = "Must not be saved"
			r := beginUICommand(t, a, "profiles.update")
			if err := a.PreparePageMutationCommand(r.Ticket, CommandPageMutationRequest{TargetID: slug, ExpectedFingerprint: target.Fingerprint, Profile: target.Profile}); err != nil {
				t.Fatal(err)
			}
			handoff := takeUICommandFor(t, a, r.Ticket, r.CommandID)
			switch scenario {
			case "stale-file":
				value, err := a.profileManager.Get(slug)
				if err != nil {
					t.Fatal(err)
				}
				value.Description = "Concurrent change"
				if err := a.profileManager.Update(slug, value); err != nil {
					t.Fatal(err)
				}
			case "cancelled":
				if err := a.CancelUICommand(r.Ticket); err != nil {
					t.Fatal(err)
				}
			case "revoked-session":
				if err := database.DB().Exec("UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", a.commandProduct.Load().principal.SessionID).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := a.CommitWorkspaceTabCommand(r.Ticket, handoff.HandoffID); err == nil {
				t.Fatal("invalid effect accepted")
			}
			value, err := a.profileManager.Get(slug)
			if err != nil || value.Description == "Must not be saved" {
				t.Fatalf("invalid write: %+v %v", value, err)
			}
			if scenario == "revoked-session" {
				if _, err := a.GetUICommandResult(r.Ticket); err == nil {
					t.Fatal("result exposed to revoked owner")
				}
			}
		})
	}
}

func TestCommandProfileMutationResultReadableWithoutPublishedMap(t *testing.T) {
	a, _, slug := profileCommandFixture(t)
	target, err := a.ReadProfileCommandTarget(slug)
	if err != nil {
		t.Fatal(err)
	}
	r := beginUICommand(t, a, "profiles.activate")
	if err := a.PreparePageMutationCommand(r.Ticket, CommandPageMutationRequest{TargetID: slug, ExpectedFingerprint: target.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	handoff := takeUICommandFor(t, a, r.Ticket, r.CommandID)
	if err := a.CommitWorkspaceTabCommand(r.Ticket, handoff.HandoffID); err != nil {
		t.Fatal(err)
	}
	if got := getUIResultEventually(t, a, r.Ticket); got.Status != "succeeded" {
		t.Fatalf("%+v", got)
	}
	if err := a.resetCommandLifecycleIfConfigured(context.Background(), "test-result-only"); err != nil {
		t.Fatal(err)
	}
	if got, err := a.GetUICommandResult(r.Ticket); err != nil || got.Status != "succeeded" {
		t.Fatalf("confirmed result inaccessible: %+v %v", got, err)
	}
	if _, err := a.BeginUICommand("profiles.activate"); err == nil {
		t.Fatal("result exception admitted effect")
	}
}
