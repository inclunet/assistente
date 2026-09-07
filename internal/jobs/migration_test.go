package jobs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImportLegacyDefinitionsImportsDefinitionsOnlyIdempotently(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	dir := t.TempDir()
	mgr := NewManager(ManagerConfig{BaseDir: dir, Repository: repo})

	writeFile := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	writeFile("sync.yaml", `
id: sync
name: Sync
enabled: true
tool: test_tool
triggers:
  - type: manual
`)
	writeFile("catalog.yaml", `
- name: ignored
`)
	writeFile("broken.yaml", `id: broken`)
	if err := os.Mkdir(filepath.Join(dir, "runs"), 0o700); err != nil {
		t.Fatalf("mkdir runs: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "events"), 0o700); err != nil {
		t.Fatalf("mkdir events: %v", err)
	}
	writeNested := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, path), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	writeNested(filepath.Join("runs", "sync.json"), `{"ignored":true}`)
	writeNested(filepath.Join("events", "2026-01-01.jsonl"), `{"ignored":true}`)

	result, err := mgr.ImportLegacyDefinitions(userA)
	if err != nil {
		t.Fatalf("import legacy definitions: %v", err)
	}
	if result.Imported != 1 || result.Failed != 1 || result.Skipped != 0 {
		t.Fatalf("unexpected result: imported=%d failed=%d skipped=%d errors=%v", result.Imported, result.Failed, result.Skipped, result.Errors)
	}
	if _, err := repo.GetJob(userA, "sync"); err != nil {
		t.Fatalf("expected sync job imported: %v", err)
	}
	if _, err := repo.GetJob(userA, "ignored"); err == nil {
		t.Fatalf("catalog.yaml should not be imported")
	}

	result, err = mgr.ImportLegacyDefinitions(userA)
	if err != nil {
		t.Fatalf("second import legacy definitions: %v", err)
	}
	if result.Imported != 0 || result.Skipped != 1 || result.Failed != 1 {
		t.Fatalf("unexpected idempotent result: imported=%d failed=%d skipped=%d errors=%v", result.Imported, result.Failed, result.Skipped, result.Errors)
	}
}
