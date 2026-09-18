package filesystem

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"assistente/internal/tools"
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

func TestSearchFilesEmitsStructuredPresentationForSafeFileTargets(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "relatorio.txt")
	if err := os.WriteFile(file, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := NewSearchFiles(dir).Execute(context.Background(), json.RawMessage(`{"pattern":"*.txt"}`))
	if err != nil || result.IsError {
		t.Fatalf("busca falhou: err=%v result=%+v", err, result)
	}
	presentation, ok := result.Metadata[tools.SearchResultPresentationMetadataKey].(tools.SearchResultPresentation)
	if !ok || presentation.Version != 1 || presentation.Total != 1 || len(presentation.Items) != 1 {
		t.Fatalf("apresentação inválida: %#v", result.Metadata[tools.SearchResultPresentationMetadataKey])
	}
	if target := presentation.Items[0].Target; target == nil || target.Kind != "file" || target.Path != file {
		t.Fatalf("target inseguro ou ausente: %#v", target)
	}
}

func TestSearchFilesNonRecursiveDoesNotAnnounceHiddenContinuation(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "z.key"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	result, err := NewSearchFiles(dir).Execute(context.Background(), json.RawMessage(`{
		"pattern": "*",
		"max_results": 1
	}`))
	if err != nil || result.IsError {
		t.Fatalf("busca falhou: err=%v result=%+v", err, result)
	}
	if !strings.Contains(result.Content, "a.txt") || strings.Contains(result.Content, "z.key") {
		t.Fatalf("resultado visível incorreto: %q", result.Content)
	}
	if result.Annotations != nil && result.Annotations.OutputWindow != nil &&
		result.Annotations.OutputWindow.HasMore {
		t.Fatalf("entrada sensível gerou continuação falsa: %+v", result.Annotations)
	}
}
