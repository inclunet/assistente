package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandjobactivation"
	"assistente/internal/commandjobevents"
	"assistente/internal/database"
)

func TestCommandRuntimeIdentityFromFactUsesPersistedRunProof(t *testing.T) {
	repo, _, _ := setupJobsRepositoryTest(t)
	userCtx := database.WithUserID(context.Background(), uuid7ForTest(t))
	userID, err := database.RequireUserID(userCtx)
	if err != nil {
		t.Fatal(err)
	}
	job := testRepositoryJob("command-runtime-job", "Command Runtime Job")
	if err := repo.SaveJob(userCtx, job); err != nil {
		t.Fatal(err)
	}
	runID := uuid7ForTest(t)
	eventID := uuid7ForTest(t)
	provenance, err := CommandRuntimeIdentityProvenance(commandjobactivation.RuntimeIdentity{
		Generation: "runtime-1", UserID: userID, AuthContextType: "local_session",
		AuthContextID: "session-1", AuthGeneration: "auth-1", SecurityGeneration: "sec-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.LogRun(userCtx, &RunLog{
		RunID: runID, JobID: job.ID, JobDatabaseID: job.DatabaseID, Status: RunStatusRunning,
		Trigger: TriggerInfo{Type: TriggerManual}, QueuedAt: time.Now().UTC(), StartedAt: time.Now().UTC(),
		RootOriginType: "manual", RootOriginID: runID, Provenance: provenance,
		RunEvents: []RunEvent{{ID: eventID, RunID: runID, Sequence: 1, Timestamp: time.Now().UTC(), Type: RunStatusRunning}},
	}); err != nil {
		t.Fatal(err)
	}
	fact := commandjobevents.Fact{UserID: userID, JobDatabaseID: job.DatabaseID, JobSlug: job.ID, RunID: runID, State: commandjobevents.StateStarted, RootOriginType: "manual", RootOriginID: runID}
	identity, err := CommandPersistedIdentityFromFact(context.Background(), repo.db, fact)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Generation != "runtime-1" || identity.UserID != userID || identity.AuthContextID != "session-1" || identity.SecurityGeneration != "sec-1" {
		t.Fatalf("identity = %+v", identity)
	}
	m := &Manager{commandRuntime: make(map[string]commandRuntimeEntry), commandRuntimeAccepting: true}
	watchCtx, cancelWatch := context.WithCancel(context.Background())
	if !m.registerCommandRuntime(runID, identity, watchCtx, 0) {
		t.Fatal("runtime não foi registrado")
	}
	if got, err := m.CommandRuntimeIdentity(context.Background(), repo.db, fact); err != nil || got != identity {
		t.Fatalf("runtime vivo = %+v, err=%v", got, err)
	}
	cancelWatch()
	if _, err := m.CommandRuntimeIdentity(context.Background(), repo.db, fact); !errors.Is(err, ErrCommandMaintenanceUnavailable) {
		t.Fatalf("runtime revogado aceito: %v", err)
	}
}

func TestCommandRuntimeIdentityFromFactFailsClosedForLegacyOrStaleRuns(t *testing.T) {
	repo, _, _ := setupJobsRepositoryTest(t)
	userCtx := database.WithUserID(context.Background(), uuid7ForTest(t))
	userID, err := database.RequireUserID(userCtx)
	if err != nil {
		t.Fatal(err)
	}
	job := testRepositoryJob("command-runtime-stale", "Command Runtime Stale")
	if err := repo.SaveJob(userCtx, job); err != nil {
		t.Fatal(err)
	}
	runID := uuid7ForTest(t)
	eventID := uuid7ForTest(t)
	if err := repo.LogRun(userCtx, &RunLog{
		RunID: runID, JobID: job.ID, JobDatabaseID: job.DatabaseID, Status: RunStatusRunning,
		Trigger: TriggerInfo{Type: TriggerManual}, QueuedAt: time.Now().UTC(), StartedAt: time.Now().UTC(),
		RootOriginType: "manual", RootOriginID: runID,
		RunEvents: []RunEvent{{ID: eventID, RunID: runID, Sequence: 1, Timestamp: time.Now().UTC(), Type: RunStatusRunning}},
	}); err != nil {
		t.Fatal(err)
	}
	fact := commandjobevents.Fact{UserID: userID, JobDatabaseID: job.DatabaseID, JobSlug: job.ID, RunID: runID, State: commandjobevents.StateStarted, RootOriginType: "manual", RootOriginID: runID}
	if _, err := CommandRuntimeIdentityFromFact(context.Background(), repo.db, fact); !errors.Is(err, ErrCommandMaintenanceUnavailable) {
		t.Fatalf("legacy run error = %v", err)
	}

	proof, err := CommandRuntimeIdentityProvenance(commandjobactivation.RuntimeIdentity{
		Generation: "runtime-1", UserID: userID, AuthContextType: "local_session",
		AuthContextID: "session-1", AuthGeneration: "auth-1", SecurityGeneration: "sec-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	provenance, _ := json.Marshal(proof)
	if err := repo.db.Model(&database.JobRun{}).Where("id = ?", runID).Updates(map[string]any{"provenance": string(provenance), "status": RunStatusCompleted}).Error; err != nil {
		t.Fatal(err)
	}
	fact.State = commandjobevents.StateCompleted
	if _, err := CommandRuntimeIdentityFromFact(context.Background(), repo.db, fact); err != nil {
		t.Fatalf("persisted terminal identity error = %v", err)
	}
	if _, err := (&Manager{}).CommandRuntimeIdentity(context.Background(), repo.db, fact); !errors.Is(err, ErrCommandMaintenanceUnavailable) {
		t.Fatalf("terminal run accepted as live runtime: %v", err)
	}
}
