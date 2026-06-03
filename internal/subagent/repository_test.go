package subagent

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/database"

	"gorm.io/gorm"
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

// TestRepositoryUpdateNonexistentReturnsNotFound garante que um Update que não
// casa nenhuma linha (run inexistente) retorna gorm.ErrRecordNotFound em vez de
// um no-op silencioso "bem-sucedido".
func TestRepositoryUpdateNonexistentReturnsNotFound(t *testing.T) {
	repo, ctx := setupManagerTest(t) // ctx => user-a
	run := &database.SubAgentRun{
		UUIDModel:           database.UUIDModel{ID: "00000000-0000-7000-8000-00000000abcd"},
		ChildConversationID: "child-x",
		Status:              database.SubAgentRunStatusRunning,
	}
	if err := repo.Update(ctx, run); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("esperava gorm.ErrRecordNotFound para run inexistente, veio %v", err)
	}
}

// TestRepositoryUpdateOtherUsersRunReturnsNotFound garante que tentar atualizar
// um run de outro usuário (UserID vazio normalizado para o usuário do contexto;
// o WHERE de escopo não casa a linha do outro dono) retorna ErrRecordNotFound e
// não altera o registro alheio (AEP-0052).
func TestRepositoryUpdateOtherUsersRunReturnsNotFound(t *testing.T) {
	repo, ctxA := setupManagerTest(t) // ctx => user-a
	ctxB := database.WithUserID(context.Background(), "user-b")

	runB := &database.SubAgentRun{ChildConversationID: "child-b", Status: database.SubAgentRunStatusQueued}
	if err := repo.Create(ctxB, runB); err != nil {
		t.Fatalf("Create user-b: %v", err)
	}

	upd := &database.SubAgentRun{
		UUIDModel:           database.UUIDModel{ID: runB.ID},
		ChildConversationID: "child-b",
		Status:              database.SubAgentRunStatusRunning,
	}
	if err := repo.Update(ctxA, upd); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("esperava gorm.ErrRecordNotFound ao atualizar run de outro usuário, veio %v", err)
	}

	got, err := repo.Get(ctxB, runB.ID)
	if err != nil {
		t.Fatalf("Get user-b: %v", err)
	}
	if got.Status != database.SubAgentRunStatusQueued {
		t.Fatalf("status de user-b não deveria mudar, veio %q", got.Status)
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
