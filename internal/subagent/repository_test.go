package subagent

import (
	"context"
	"testing"

	"assistente/internal/database"
)

// TestRepositoryCreateNormalizesEmptyUserID garante que o Create preenche o
// UserID com o usuário do contexto quando o campo vem vazio (AEP-0052).
func TestRepositoryCreateNormalizesEmptyUserID(t *testing.T) {
	repo, ctx := setupManagerTest(t) // ctx => user-a
	run := &database.SubAgentRun{
		ChildConversationID: "child-1",
		Status:              database.SubAgentRunStatusQueued,
	}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create erro inesperado: %v", err)
	}
	got, err := repo.Get(ctx, run.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.UserID != "user-a" {
		t.Fatalf("UserID esperado user-a, veio %q", got.UserID)
	}
}

// TestRepositoryCreateRejectsDivergentUserID impede inserir um run sob outro
// user_id diferente do usuário autenticado do contexto (AEP-0052).
func TestRepositoryCreateRejectsDivergentUserID(t *testing.T) {
	repo, ctx := setupManagerTest(t) // ctx => user-a
	run := &database.SubAgentRun{
		UserID:              "user-b",
		ChildConversationID: "child-1",
		Status:              database.SubAgentRunStatusQueued,
	}
	if err := repo.Create(ctx, run); err == nil {
		t.Fatal("esperava erro ao criar run sob outro user_id")
	}
	// Nada deve ter sido persistido para user-b sob a ótica de user-a.
	if run.ID != "" {
		if _, err := repo.Get(ctx, run.ID); err == nil {
			t.Fatal("run divergente não deveria existir/visível para user-a")
		}
	}
}

// TestRepositoryUpdateRejectsOwnershipTransfer impede que um Save com UserID
// alterado troque o dono do registro (AEP-0052).
func TestRepositoryUpdateRejectsOwnershipTransfer(t *testing.T) {
	repo, ctx := setupManagerTest(t) // ctx => user-a
	run := &database.SubAgentRun{
		ChildConversationID: "child-1",
		Status:              database.SubAgentRunStatusQueued,
	}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}

	run.UserID = "user-b" // tentativa de transferência de posse
	run.Status = database.SubAgentRunStatusRunning
	if err := repo.Update(ctx, run); err == nil {
		t.Fatal("esperava erro ao tentar transferir ownership no Update")
	}

	// O registro continua pertencendo a user-a e não mudou de dono nem status.
	got, err := repo.Get(ctx, run.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.UserID != "user-a" {
		t.Fatalf("UserID deveria permanecer user-a, veio %q", got.UserID)
	}
	if got.Status != database.SubAgentRunStatusQueued {
		t.Fatalf("status não deveria ter mudado, veio %q", got.Status)
	}
}

// TestRepositoryUpdateNormalizesEmptyUserID garante que o Update preenche o
// UserID com o usuário do contexto quando o campo vem vazio, mantendo o escopo.
func TestRepositoryUpdateNormalizesEmptyUserID(t *testing.T) {
	repo, ctx := setupManagerTest(t) // ctx => user-a
	run := &database.SubAgentRun{
		ChildConversationID: "child-1",
		Status:              database.SubAgentRunStatusQueued,
	}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}

	run.UserID = ""
	run.Status = database.SubAgentRunStatusRunning
	if err := repo.Update(ctx, run); err != nil {
		t.Fatalf("Update erro inesperado: %v", err)
	}

	got, err := repo.Get(ctx, run.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.UserID != "user-a" || got.Status != database.SubAgentRunStatusRunning {
		t.Fatalf("esperava user-a/running, veio %q/%q", got.UserID, got.Status)
	}
}

// TestRepositoryCreateRequiresUserScope rejeita escrita sem usuário no contexto.
func TestRepositoryCreateRequiresUserScope(t *testing.T) {
	repo, _ := setupManagerTest(t)
	run := &database.SubAgentRun{ChildConversationID: "child-1", Status: database.SubAgentRunStatusQueued}
	if err := repo.Create(context.Background(), run); err == nil {
		t.Fatal("esperava erro de escopo de usuário ausente")
	}
}
