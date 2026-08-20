package filesystem

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// search_files localiza paths; nem um documento inválido deve fazê-la tentar
// abrir/extrair conteúdo (AEP-0093, D5).
func TestSearchFilesFindsDocumentsWithoutExtracting(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"valido.docx", "quebrado.docx", "manual.pdf"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("não é um documento válido"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	result, err := NewSearchFiles(dir).Execute(context.Background(), json.RawMessage(`{
		"pattern": "**/*"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("search_files tentou interpretar conteúdo: %s", result.Content)
	}
	for _, name := range []string{"valido.docx", "quebrado.docx", "manual.pdf"} {
		if !strings.Contains(result.Content, name) {
			t.Errorf("path %s ausente: %s", name, result.Content)
		}
	}
}
