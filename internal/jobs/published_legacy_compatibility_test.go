package jobs

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestAcceptedLegacyJobCorpusImportsIdempotentlyAndPreservesSources(t *testing.T) {
	repo, userCtx, _ := setupJobsRepositoryTest(t)
	dir := t.TempDir()
	valid := readPublishedJobFixture(t, "pre-release-v1.yaml")
	invalid := readPublishedJobFixture(t, "invalid.yaml")
	writeJobFixture(t, dir, "corpus.yaml", valid)
	writeJobFixture(t, dir, "invalid.yaml", invalid)

	manager := NewManager(ManagerConfig{BaseDir: dir, Repository: repo})
	first, err := manager.ImportLegacyDefinitions(userCtx)
	if err != nil {
		t.Fatalf("importação direta de jobs: %v", err)
	}
	if first.Imported != 1 || first.Failed != 1 || first.Skipped != 0 {
		t.Fatalf("resultado inesperado: %+v", first)
	}
	job, err := repo.GetJob(userCtx, "corpus-job")
	if err != nil {
		t.Fatalf("carregar job importado: %v", err)
	}
	if job.Pipeline != "corpus" || job.Inputs["query"] != "valor-sintetico" ||
		len(job.Triggers) != 2 || job.Triggers[0].Every != "30m" ||
		job.Triggers[1].Type != TriggerManual {
		t.Fatalf("dados do job não foram preservados: %+v", job)
	}

	second, err := manager.ImportLegacyDefinitions(userCtx)
	if err != nil {
		t.Fatalf("segunda importação de jobs: %v", err)
	}
	if second.Imported != 0 || second.Skipped != 1 || second.Failed != 1 {
		t.Fatalf("reimportação de jobs não foi idempotente: %+v", second)
	}
	assertJobFixtureUnchanged(t, filepath.Join(dir, "corpus.yaml"), valid)
	assertJobFixtureUnchanged(t, filepath.Join(dir, "invalid.yaml"), invalid)
}

func readPublishedJobFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "published", "legacy", name))
	if err != nil {
		t.Fatalf("ler fixture %s: %v", name, err)
	}
	return data
}

func writeJobFixture(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatalf("escrever fixture %s: %v", name, err)
	}
}

func assertJobFixtureUnchanged(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("fonte %s deixou de existir: %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("fonte %s foi alterada", path)
	}
}
