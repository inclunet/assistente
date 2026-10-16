package jobs

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/database"

	"github.com/google/uuid"
)

func TestPrepareCommandJobRequiresExplicitMatchingOwner(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("prepared-command", "Prepared command")
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	m := mustNewManager(t, ManagerConfig{
		Repository: repo,
		ContextProvider: func() context.Context {
			return userA
		},
	})

	if _, err := m.PrepareCommandJob(context.Background(), job.DatabaseID); !errors.Is(err, ErrCommandJobDenied) {
		t.Fatalf("contexto sem usuário deveria ser recusado, erro=%v", err)
	}
	if _, err := m.PrepareCommandJob(database.WithUserID(context.Background(), "user-b"), job.DatabaseID); !errors.Is(err, ErrCommandJobDenied) {
		t.Fatalf("usuário estrangeiro deveria ser recusado, erro=%v", err)
	}
}

func TestPrepareCommandJobRequiresCanonicalUUIDv7AndExistingJob(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	m := mustNewManager(t, ManagerConfig{
		Repository: repo,
		ContextProvider: func() context.Context {
			return userA
		},
	})

	for _, id := range []string{"not-a-uuid", uuid.New().String()} {
		if _, err := m.PrepareCommandJob(userA, id); !errors.Is(err, ErrCommandJobDenied) {
			t.Errorf("ID inválido %q deveria ser recusado, erro=%v", id, err)
		}
	}

	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("new uuidv7: %v", err)
	}
	if _, err := m.PrepareCommandJob(userA, id.String()); err == nil {
		t.Fatal("job inexistente deveria falhar fechado")
	}
}

func TestPrepareCommandJobReturnsDetachedSnapshotWithDatabaseID(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("detached-command", "Detached command")
	job.Inputs = map[string]any{
		"nested": map[string]any{"value": "original"},
	}
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	m := mustNewManager(t, ManagerConfig{
		Repository: repo,
		ContextProvider: func() context.Context {
			return userA
		},
	})
	snapshot, err := m.PrepareCommandJob(userA, job.DatabaseID)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if snapshot.DatabaseID != job.DatabaseID {
		t.Fatalf("DatabaseID = %q, esperado %q", snapshot.DatabaseID, job.DatabaseID)
	}

	snapshot.Inputs["nested"].(map[string]any)["value"] = "mutated"
	fresh, err := m.PrepareCommandJob(userA, job.DatabaseID)
	if err != nil {
		t.Fatalf("prepare após mutação: %v", err)
	}
	if fresh.Inputs["nested"].(map[string]any)["value"] != "original" {
		t.Fatal("snapshot não deveria compartilhar estado mutável com a definição persistida")
	}
}

func TestPrepareCommandJobPreservesEmptyInputsFingerprint(t *testing.T) {
	repo, owner, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("empty-inputs-command", "Empty inputs command")
	job.Inputs = map[string]any{}
	if err := repo.SaveJob(owner, job); err != nil {
		t.Fatal(err)
	}
	persisted, err := repo.GetJobByID(owner, job.DatabaseID)
	if err != nil {
		t.Fatal(err)
	}
	m := mustNewManager(t, ManagerConfig{Repository: repo, ContextProvider: func() context.Context { return owner }})
	snapshot, err := m.PrepareCommandJob(owner, job.DatabaseID)
	if err != nil {
		t.Fatal(err)
	}
	want, err := DefinitionFingerprint(persisted)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DefinitionFingerprint(snapshot)
	if err != nil || got != want {
		t.Fatalf("detached definition fingerprint changed: got %q want %q err=%v", got, want, err)
	}
	if snapshot.Inputs == nil {
		t.Fatal("empty inputs object became null")
	}
	snapshot.Inputs["mutated"] = true
	if len(persisted.Inputs) != 0 {
		t.Fatal("snapshot shares persisted inputs")
	}
}
