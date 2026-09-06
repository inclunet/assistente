package portability

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/database"
)

func TestPublished019MCPImportsDirectlyIdempotentlyAndPreservesSources(t *testing.T) {
	setupPortabilityTestDB(t)
	valid := readPublishedLegacyFixture(t, "mcp", "0.1.9.json")
	invalid := readPublishedLegacyFixture(t, "mcp", "invalid.json")
	source := &memoryLegacyImportSource{
		files: []LegacyImportFile{
			{Name: "invalid", Filename: "invalid.json", Path: "/fixture/invalid.json", Source: "0.1.9"},
			{Name: "corpus-mcp", Filename: "0.1.9.json", Path: "/fixture/0.1.9.json", Source: "0.1.9"},
		},
		data: map[string][]byte{
			"invalid.json": invalid,
			"0.1.9.json":   valid,
		},
	}
	originalValid := append([]byte(nil), valid...)
	originalInvalid := append([]byte(nil), invalid...)

	first, err := ImportLegacyMCPServersWithContext(portabilityTestCtx(), source, nil)
	if err != nil {
		t.Fatalf("importação direta MCP da 0.1.9: %v", err)
	}
	if first.Imported != 1 || first.Failed != 1 || first.Skipped != 0 {
		t.Fatalf("resultado inesperado: %+v", first)
	}
	var row database.MCPServer
	if err := database.DB().Where("user_id = ? AND slug = ?", portabilityTestUserID, "corpus-mcp").
		First(&row).Error; err != nil {
		t.Fatalf("carregar MCP importado: %v", err)
	}
	if row.Name != "Servidor sintético 0.1.9" || row.Command != "fixture-mcp" ||
		row.Args != `["--modo","teste"]` || row.Env != `{"FIXTURE_MODE":"synthetic"}` {
		t.Fatalf("dados MCP não foram preservados: %+v", row)
	}

	second, err := ImportLegacyMCPServersWithContext(portabilityTestCtx(), source, nil)
	if err != nil {
		t.Fatalf("segunda importação MCP: %v", err)
	}
	if second.Imported != 0 || second.Skipped != 1 || second.Failed != 1 {
		t.Fatalf("reimportação MCP não foi idempotente: %+v", second)
	}
	if !bytes.Equal(source.data["0.1.9.json"], originalValid) ||
		!bytes.Equal(source.data["invalid.json"], originalInvalid) {
		t.Fatal("importador MCP alterou uma fonte legada")
	}
}

func readPublishedLegacyFixture(t *testing.T, parts ...string) []byte {
	t.Helper()
	pathParts := append([]string{"testdata", "published", "legacy"}, parts...)
	data, err := os.ReadFile(filepath.Join(pathParts...))
	if err != nil {
		t.Fatalf("ler fixture %s: %v", filepath.Join(parts...), err)
	}
	return data
}
