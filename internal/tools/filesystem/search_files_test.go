package filesystem

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// search_files localiza paths; nenhum documento inválido deve fazê-la tentar
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

func TestSearchFilesLimitUsesAnnotationWithoutPollutingContent(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}

	result, err := NewSearchFiles(dir).Execute(context.Background(), json.RawMessage(`{
		"pattern": "*.txt",
		"max_results": 2
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Annotations == nil {
		t.Fatalf("busca limitada sem anotações: %+v", result)
	}
	window := result.Annotations.OutputWindow
	if window == nil || !window.HasMore || window.Unit != "results" || window.Returned != 2 {
		t.Fatalf("janela de busca inválida: %+v", result.Annotations)
	}
	if strings.Contains(strings.ToUpper(result.Content), "TRUNCAD") || strings.Contains(result.Content, "continu") {
		t.Fatalf("aviso de truncamento poluiu o conteúdo: %q", result.Content)
	}
}
