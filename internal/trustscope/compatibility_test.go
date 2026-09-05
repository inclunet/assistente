package trustscope_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/fstrust"
	"assistente/internal/nettrust"
	"assistente/internal/trustscope"
)

func TestConsumerScopesUseSharedContract(t *testing.T) {
	var networkScope trustscope.Scope = nettrust.ScopeWorkspace
	var filesystemScope trustscope.Scope = fstrust.ScopeWorkspace
	if networkScope != filesystemScope || !networkScope.IsPersistent() {
		t.Fatalf("escopos dos consumidores divergiram: rede=%q fs=%q", networkScope, filesystemScope)
	}
	if fstrust.ValidScope(trustscope.Scope("unknown")) || nettrust.ValidScope(trustscope.Scope("unknown")) {
		t.Fatal("consumidores devem compartilhar validação fail-closed de escopo")
	}
}

func TestNettrustReadsAndRewritesExistingFormat(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "network-allowlist", "global.json")
	writeFixture(t, path, `{
  "version": 1,
  "entries": [
    {
      "host": "legacy.internal",
      "scope": "global",
      "category": "private-rfc1918",
      "resolved_ips": ["10.0.0.10"],
      "created_by": "user",
      "created_at": "2026-01-02T03:04:05Z",
      "reason": "fixture anterior à extração"
    }
  ]
}`)

	manager := nettrust.NewManagerWithDirs(home, home)
	if decision := manager.Match(context.Background(), "legacy.internal", "443"); !decision.Allowed {
		t.Fatal("entrada de rede no formato existente deveria continuar válida")
	}
	if err := manager.Add(context.Background(), nettrust.AllowlistEntry{
		Host:  "new.internal",
		Scope: nettrust.ScopeGlobal,
	}); err != nil {
		t.Fatalf("reescrita compatível: %v", err)
	}

	var stored struct {
		Version int                       `json:"version"`
		Entries []nettrust.AllowlistEntry `json:"entries"`
	}
	readJSON(t, path, &stored)
	if stored.Version != 1 || len(stored.Entries) != 2 {
		t.Fatalf("formato de rede alterado: version=%d entries=%d", stored.Version, len(stored.Entries))
	}
	legacy := stored.Entries[0]
	if legacy.Host != "legacy.internal" || legacy.Category != "private-rfc1918" ||
		len(legacy.ResolvedIPs) != 1 || legacy.ResolvedIPs[0] != "10.0.0.10" {
		t.Fatalf("metadados legados de rede não foram preservados: %+v", legacy)
	}
}

func TestFstrustReadsLegacyAllowAndPreservesFormat(t *testing.T) {
	home := t.TempDir()
	legacyPath := filepath.Join(home, "legacy.txt")
	path := filepath.Join(home, "path-allowlist", "global.json")
	fixture := map[string]any{
		"version": 1,
		"entries": []map[string]any{{
			"path":       legacyPath,
			"kind":       "file",
			"operation":  "read",
			"scope":      "global",
			"created_by": "user",
			"created_at": "2026-01-02T03:04:05Z",
			"reason":     "allow legado sem effect",
		}},
	}
	data, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, path, string(data))

	manager := fstrust.NewManagerWithDirs(home, home)
	if decision := manager.Match(context.Background(), legacyPath, "read"); !decision.Allowed {
		t.Fatal("effect omitido no formato legado deve continuar significando allow")
	}
	if err := manager.Add(context.Background(), fstrust.AllowlistEntry{
		Path:      filepath.Join(home, "new.txt"),
		Kind:      fstrust.KindFile,
		Operation: "read",
		Scope:     fstrust.ScopeGlobal,
	}); err != nil {
		t.Fatalf("reescrita compatível: %v", err)
	}

	var stored struct {
		Version int                      `json:"version"`
		Entries []fstrust.AllowlistEntry `json:"entries"`
	}
	readJSON(t, path, &stored)
	if stored.Version != 1 || len(stored.Entries) != 2 {
		t.Fatalf("formato de filesystem alterado: version=%d entries=%d", stored.Version, len(stored.Entries))
	}
	if stored.Entries[0].Effect != "" || stored.Entries[0].Reason != "allow legado sem effect" {
		t.Fatalf("entrada legada de filesystem não foi preservada: %+v", stored.Entries[0])
	}
}

func TestCorruptPersistentStoreFailsClosedWithoutOverwrite(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "network-allowlist", "global.json")
	const corrupt = "{invalid"
	writeFixture(t, path, corrupt)

	manager := nettrust.NewManagerWithDirs(home, home)
	if decision := manager.Match(context.Background(), "blocked.internal", "443"); decision.Allowed {
		t.Fatal("arquivo inválido jamais pode conceder trust")
	}
	if err := manager.Add(context.Background(), nettrust.AllowlistEntry{
		Host:  "blocked.internal",
		Scope: nettrust.ScopeGlobal,
	}); err == nil {
		t.Fatal("arquivo inválido deve impedir sobrescrita")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != corrupt {
		t.Fatalf("conteúdo inválido foi sobrescrito: %q", data)
	}
}

func writeFixture(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readJSON(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}
