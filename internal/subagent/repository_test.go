package subagent

import (
	"context"
	"errors"
	"testing"
	"time"

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

// TestRepositoryUpdateEmptyIDDoesNotMassUpdate garante que um Update com run.ID
// vazio falha fechado (ErrRecordNotFound) em vez de virar UPDATE em massa e
// corromper todos os runs do usuário.
func TestRepositoryUpdateEmptyIDDoesNotMassUpdate(t *testing.T) {
	repo, ctx := setupManagerTest(t) // ctx => user-a

	// Dois runs existentes do mesmo usuário.
	r1 := &database.SubAgentRun{ChildConversationID: "child-1", Status: database.SubAgentRunStatusQueued}
	r2 := &database.SubAgentRun{ChildConversationID: "child-2", Status: database.SubAgentRunStatusQueued}
	if err := repo.Create(ctx, r1); err != nil {
		t.Fatalf("Create r1: %v", err)
	}
	if err := repo.Create(ctx, r2); err != nil {
		t.Fatalf("Create r2: %v", err)
	}

	// Update sem ID (struct parcial): deve recusar, não escrever nada.
	partial := &database.SubAgentRun{Status: database.SubAgentRunStatusRunning}
	if err := repo.Update(ctx, partial); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("esperava gorm.ErrRecordNotFound para run sem id, veio %v", err)
	}

	// Nenhum dos runs existentes pode ter sido alterado (sem mass-update).
	for _, id := range []string{r1.ID, r2.ID} {
		got, err := repo.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get %s: %v", id, err)
		}
		if got.Status != database.SubAgentRunStatusQueued {
			t.Fatalf("run %s não deveria ter mudado (mass-update), status=%q", id, got.Status)
		}
	}
}

// TestRepositoryUpdatePreservesImmutableFields garante que o Update não altera
// os campos imutáveis id/created_at (Select("*").Omit("id","created_at")).
func TestRepositoryUpdatePreservesImmutableFields(t *testing.T) {
	repo, ctx := setupManagerTest(t) // ctx => user-a
	run := &database.SubAgentRun{ChildConversationID: "child-1", Status: database.SubAgentRunStatusQueued}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}
	original, err := repo.Get(ctx, run.ID)
	if err != nil {
		t.Fatalf("Get original: %v", err)
	}

	// Tenta sobrescrever created_at junto com uma mudança legítima de status.
	run.CreatedAt = original.CreatedAt.Add(-72 * time.Hour)
	run.Status = database.SubAgentRunStatusRunning
	if err := repo.Update(ctx, run); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.Get(ctx, run.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != original.ID {
		t.Fatalf("id não deveria mudar: era %q, veio %q", original.ID, got.ID)
	}
	if !got.CreatedAt.Equal(original.CreatedAt) {
		t.Fatalf("created_at não deveria mudar: era %v, veio %v", original.CreatedAt, got.CreatedAt)
	}
	if got.Status != database.SubAgentRunStatusRunning {
		t.Fatalf("status deveria ter mudado para running, veio %q", got.Status)
	}
}

// TestRepositoryListRecentNaoVazaTituloDeOutroUsuario garante que o LEFT JOIN
// com conversations casa também o user_id (AEP-0052): um run cujo
// child_conversation_id aponte para conversa de outro dono vem sem título, em
// vez de expor o título alheio na UI, enquanto o run com sub-conversa própria
// continua trazendo o título.
func TestRepositoryListRecentNaoVazaTituloDeOutroUsuario(t *testing.T) {
	repo, ctxA := setupManagerTest(t) // ctx => user-a

	convB := &database.Conversation{
		UUIDModel: database.UUIDModel{ID: "conv-de-user-b"},
		UserID:    "user-b",
		Title:     "titulo secreto do user-b",
	}
	if err := repo.db.Create(convB).Error; err != nil {
		t.Fatalf("criar conversa de user-b: %v", err)
	}
	convA := &database.Conversation{
		UUIDModel: database.UUIDModel{ID: "conv-de-user-a"},
		UserID:    "user-a",
		Title:     "titulo do user-a",
	}
	if err := repo.db.Create(convA).Error; err != nil {
		t.Fatalf("criar conversa de user-a: %v", err)
	}

	// Run inconsistente: pertence a user-a, mas aponta para a sub-conversa alheia.
	runVazado := &database.SubAgentRun{ChildConversationID: convB.ID, Status: database.SubAgentRunStatusQueued}
	if err := repo.Create(ctxA, runVazado); err != nil {
		t.Fatalf("Create run vazado: %v", err)
	}
	runProprio := &database.SubAgentRun{ChildConversationID: convA.ID, Status: database.SubAgentRunStatusQueued}
	if err := repo.Create(ctxA, runProprio); err != nil {
		t.Fatalf("Create run próprio: %v", err)
	}

	items, err := repo.ListRecent(ctxA, 10)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("esperava 2 runs, veio %d", len(items))
	}
	titles := make(map[string]string, len(items))
	for _, item := range items {
		titles[item.RunID] = item.Title
	}
	if got := titles[runVazado.ID]; got != "" {
		t.Fatalf("título de conversa de outro usuário vazou na listagem: %q", got)
	}
	if got := titles[runProprio.ID]; got != convA.Title {
		t.Fatalf("título da própria sub-conversa esperado %q, veio %q", convA.Title, got)
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
