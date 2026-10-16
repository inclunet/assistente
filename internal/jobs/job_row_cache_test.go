package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/database"

	"gorm.io/gorm"
)

func seedCacheJob(t *testing.T, repo *DBRepository, ctx context.Context, slug, name string) {
	t.Helper()
	job := &Job{
		ID:       slug,
		Name:     name,
		Enabled:  true,
		Tool:     "test_tool",
		Triggers: []Trigger{{Type: TriggerManual}},
	}
	if err := repo.SaveJob(ctx, job); err != nil {
		t.Fatalf("SaveJob(%s): %v", slug, err)
	}
}

// deleteJobRowDireto remove a linha SEM passar por DeleteJob, portanto SEM
// invalidar o cache — usado para provar que a segunda leitura veio do cache.
func deleteJobRowDireto(t *testing.T, repo *DBRepository, ctx context.Context, slug string) {
	t.Helper()
	userID, _ := database.UserIDFromContext(ctx)
	if err := repo.db.WithContext(ctx).
		Where("user_id = ? AND slug = ?", userID, slug).
		Delete(&database.Job{}).Error; err != nil {
		t.Fatalf("delete direto: %v", err)
	}
}

// TestJobRowBySlug_ServeDoCache prova que a segunda resolução vem do cache: a
// linha é apagada direto no banco (sem invalidar) e ainda assim é resolvida.
func TestJobRowBySlug_ServeDoCache(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	seedCacheJob(t, repo, userA, "meu-job", "Meu Job")

	first, err := repo.jobRowBySlug(userA, "meu-job")
	if err != nil {
		t.Fatalf("primeira resolução: %v", err)
	}
	deleteJobRowDireto(t, repo, userA, "meu-job")

	second, err := repo.jobRowBySlug(userA, "meu-job")
	if err != nil {
		t.Fatalf("segunda resolução deveria vir do cache, veio erro: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("cache retornou ID %q; esperado %q", second.ID, first.ID)
	}
}

// TestJobRowBySlug_InvalidaEmMutacao prova que DeleteJob invalida o cache: após
// resolver (cachear) e deletar via repositório, a próxima resolução vai ao banco
// e não encontra (em vez de servir a entrada obsoleta).
func TestJobRowBySlug_InvalidaEmMutacao(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	seedCacheJob(t, repo, userA, "meu-job", "Meu Job")

	if _, err := repo.jobRowBySlug(userA, "meu-job"); err != nil {
		t.Fatalf("resolução inicial: %v", err)
	}
	if err := repo.DeleteJob(userA, "meu-job"); err != nil {
		t.Fatalf("DeleteJob: %v", err)
	}
	if _, err := repo.jobRowBySlug(userA, "meu-job"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("após DeleteJob esperava ErrRecordNotFound (cache invalidado), veio: %v", err)
	}
}

// TestJobRowBySlug_TTLExpira prova a rede de segurança: passado o TTL, a entrada
// expira e a resolução volta ao banco.
func TestJobRowBySlug_TTLExpira(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	current := time.Unix(1_000_000, 0)
	repo.now = func() time.Time { return current }

	seedCacheJob(t, repo, userA, "meu-job", "Meu Job")
	if _, err := repo.jobRowBySlug(userA, "meu-job"); err != nil {
		t.Fatalf("resolução inicial: %v", err)
	}
	deleteJobRowDireto(t, repo, userA, "meu-job")

	// Ainda dentro do TTL: cache serve.
	current = current.Add(jobRowResolveCacheTTL - time.Second)
	if _, err := repo.jobRowBySlug(userA, "meu-job"); err != nil {
		t.Fatalf("dentro do TTL o cache deveria servir, veio: %v", err)
	}

	// Passado o TTL: expira e vai ao banco (vazio) -> not found.
	current = current.Add(2 * time.Second)
	if _, err := repo.jobRowBySlug(userA, "meu-job"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("após o TTL esperava ErrRecordNotFound, veio: %v", err)
	}
}

// TestJobRowBySlug_IsolaPorUsuario garante que a entrada de um usuário nunca é
// servida a outro, mesmo com slug idêntico.
func TestJobRowBySlug_IsolaPorUsuario(t *testing.T) {
	repo, userA, userB := setupJobsRepositoryTest(t)
	seedCacheJob(t, repo, userA, "compartilhado", "Job A")
	seedCacheJob(t, repo, userB, "compartilhado", "Job B")

	rowA, err := repo.jobRowBySlug(userA, "compartilhado")
	if err != nil {
		t.Fatalf("resolução user-a: %v", err)
	}
	rowB, err := repo.jobRowBySlug(userB, "compartilhado")
	if err != nil {
		t.Fatalf("resolução user-b: %v", err)
	}
	if rowA.Name != "Job A" {
		t.Fatalf("user-a resolveu %q; esperado \"Job A\"", rowA.Name)
	}
	if rowB.Name != "Job B" {
		t.Fatalf("user-b resolveu %q; esperado \"Job B\" (vazamento entre usuários)", rowB.Name)
	}
	if rowA.UserID == rowB.UserID {
		t.Fatalf("linhas de usuários distintos com o mesmo user_id %q", rowA.UserID)
	}
}
