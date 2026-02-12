package filesystem

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEditFile_Name(t *testing.T) {
	tool := NewEditFile("/tmp")
	if tool.Name() != "edit_file" {
		t.Errorf("expected 'edit_file', got '%s'", tool.Name())
	}
}

func TestEditFile_BasicReplace(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.go")
	os.WriteFile(filePath, []byte(`package main

func main() {
	fmt.Println("Hello")
}
`), 0644)

	tool := NewEditFile(dir)
	args := `{"path": "test.go", "old_string": "fmt.Println(\"Hello\")", "new_string": "fmt.Println(\"World\")"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}

	data, _ := os.ReadFile(filePath)
	if !containsString(string(data), `fmt.Println("World")`) {
		t.Error("substituição não foi aplicada")
	}
	if containsString(string(data), `fmt.Println("Hello")`) {
		t.Error("texto antigo ainda presente")
	}
}

func TestEditFile_MultilineReplace(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "multi.go")
	os.WriteFile(filePath, []byte(`func old() {
	return 1
}

func other() {
	return 2
}
`), 0644)

	tool := NewEditFile(dir)
	args := `{"path": "multi.go", "old_string": "func old() {\n\treturn 1\n}", "new_string": "func new() {\n\treturn 42\n}"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}

	data, _ := os.ReadFile(filePath)
	if !containsString(string(data), "func new()") {
		t.Error("substituição multilinha não aplicada")
	}
}

func TestEditFile_AmbiguousMatch(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "dup.txt")
	os.WriteFile(filePath, []byte("hello world\nhello again\nhello final"), 0644)

	tool := NewEditFile(dir)
	args := `{"path": "dup.txt", "old_string": "hello", "new_string": "bye"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Error("deve ser erro quando old_string não é única")
	}
	if !containsString(result.Content, "3 vezes") {
		t.Errorf("deve informar quantas vezes foi encontrada: %s", result.Content)
	}
}

func TestEditFile_ReplaceAll(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "all.txt")
	os.WriteFile(filePath, []byte("foo bar foo baz foo"), 0644)

	tool := NewEditFile(dir)
	args := `{"path": "all.txt", "old_string": "foo", "new_string": "qux", "replace_all": true}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}

	data, _ := os.ReadFile(filePath)
	if containsString(string(data), "foo") {
		t.Error("ainda contém 'foo' após replace_all")
	}
	if string(data) != "qux bar qux baz qux" {
		t.Errorf("conteúdo inesperado: %s", string(data))
	}
	if !containsString(result.Content, "3 substituição") {
		t.Errorf("deve indicar 3 substituições: %s", result.Content)
	}
}

func TestEditFile_NotFound(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	os.WriteFile(filePath, []byte("some content"), 0644)

	tool := NewEditFile(dir)
	args := `{"path": "test.txt", "old_string": "nonexistent text", "new_string": "replacement"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Error("deve ser erro quando old_string não é encontrada")
	}
}

func TestEditFile_WhitespaceHint(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "ws.txt")
	// Arquivo com tabs — old_string com espaços (whitespace diferente)
	os.WriteFile(filePath, []byte("\tindented text here"), 0644)

	tool := NewEditFile(dir)
	// old_string com espaços em vez de tab — NÃO é substring exata
	args := `{"path": "ws.txt", "old_string": "  indented text here", "new_string": "new text"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Error("deve ser erro (indentação diferente)")
	}
	if !containsString(result.Content, "indentação") {
		t.Errorf("deve dar dica sobre indentação: %s", result.Content)
	}
}

func TestEditFile_FileNotExists(t *testing.T) {
	tool := NewEditFile(t.TempDir())
	args := `{"path": "ghost.txt", "old_string": "a", "new_string": "b"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Error("deve ser erro para arquivo inexistente")
	}
}

func TestEditFile_IdenticalStrings(t *testing.T) {
	tool := NewEditFile(t.TempDir())
	args := `{"path": "test.txt", "old_string": "same", "new_string": "same"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Error("deve ser erro quando old_string == new_string")
	}
}

func TestEditFile_EmptyOldString(t *testing.T) {
	tool := NewEditFile(t.TempDir())
	args := `{"path": "test.txt", "old_string": "", "new_string": "new"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Error("deve ser erro com old_string vazia")
	}
}

func TestEditFile_BlocksSensitive(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	os.WriteFile(envPath, []byte("SECRET=123"), 0644)

	tool := NewEditFile(dir)
	args := `{"path": ".env", "old_string": "SECRET=123", "new_string": "SECRET=456"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Error("deve bloquear edição de .env")
	}
}

func TestFindOccurrenceLines(t *testing.T) {
	content := "line1\nfoo\nline3\nfoo\nline5"
	lines := findOccurrenceLines(content, "foo")
	if len(lines) != 2 {
		t.Fatalf("expected 2 occurrences, got %d", len(lines))
	}
	if lines[0] != 2 || lines[1] != 4 {
		t.Errorf("expected lines [2, 4], got %v", lines)
	}
}
