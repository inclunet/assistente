package skills

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/configdir"
)

func writeSkillFile(t *testing.T, base, slug, content string) string {
	t.Helper()
	dir := filepath.Join(base, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	p := filepath.Join(dir, skillFile)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

func TestImportLegacySkills(t *testing.T) {
	repo, ctx := setupRepo(t)
	tmp := t.TempDir()
	p1 := writeSkillFile(t, tmp, "alpha", buildSkillMD("alpha", "1.0.0", "skill alpha legado para teste", "corpo alpha"))
	writeSkillFile(t, tmp, "beta", buildSkillMD("beta", "1.0.0", "skill beta legado para teste", "corpo beta"))

	m := &Manager{resolver: configdir.NewResolverWithBase(tmp)}
	m.SetRepository(repo)

	res, err := m.ImportLegacySkills(ctx)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.Imported != 2 {
		t.Errorf("imported: got %d want 2", res.Imported)
	}

	got, err := repo.Get(ctx, "alpha")
	if err != nil {
		t.Fatalf("get alpha: %v", err)
	}
	if got.Content != "corpo alpha" {
		t.Errorf("alpha content: %q", got.Content)
	}

	// Não-destrutivo: o arquivo original permanece intacto.
	if _, err := os.Stat(p1); err != nil {
		t.Errorf("arquivo original removido/alterado: %v", err)
	}

	// Idempotente: re-import não duplica nem altera.
	res2, err := m.ImportLegacySkills(ctx)
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if res2.Imported != 0 || res2.Skipped != 2 {
		t.Errorf("re-import: imported=%d skipped=%d want 0/2", res2.Imported, res2.Skipped)
	}
}

func TestImportLegacySkillsRequiresRepo(t *testing.T) {
	m := &Manager{resolver: configdir.NewResolverWithBase(t.TempDir())}
	if _, err := m.ImportLegacySkills(context.Background()); err == nil {
		t.Errorf("esperava erro sem repository configurado")
	}
}
