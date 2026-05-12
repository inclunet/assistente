package filesystem

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGrepSearch_Name(t *testing.T) {
	tool := NewGrepSearch("/tmp")
	if tool.Name() != "grep_search" {
		t.Errorf("expected 'grep_search', got '%s'", tool.Name())
	}
}

func TestGrepSearch_Parameters(t *testing.T) {
	tool := NewGrepSearch("/tmp")
	var schema map[string]interface{}
	if err := json.Unmarshal(tool.Parameters(), &schema); err != nil {
		t.Fatalf("Parameters() deve retornar JSON válido: %v", err)
	}
	if schema["type"] != "object" {
		t.Error("schema deve ter type=object")
	}
	props := schema["properties"].(map[string]interface{})
	if _, ok := props["pattern"]; !ok {
		t.Error("schema deve ter propriedade 'pattern'")
	}
}

func TestGrepSearch_LiteralSearch(t *testing.T) {
	dir := t.TempDir()

	// Cria arquivo de teste
	content := `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
	fmt.Println("Goodbye, World!")
}
`
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte(content), 0644)

	tool := NewGrepSearch(dir)
	args := `{"pattern": "Hello"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}
	if !containsString(result.Content, "Hello") {
		t.Error("resultado deve conter 'Hello'")
	}
	if !containsString(result.Content, "main.go") {
		t.Error("resultado deve conter nome do arquivo")
	}
}

func TestGrepSearch_RegexSearch(t *testing.T) {
	dir := t.TempDir()

	content := `func Calculate(a, b int) int {
	return a + b
}

func Validate(input string) bool {
	return len(input) > 0
}
`
	_ = os.WriteFile(filepath.Join(dir, "util.go"), []byte(content), 0644)

	tool := NewGrepSearch(dir)
	args := `{"pattern": "func \\w+\\("}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}
	// Deve encontrar ambas as funções
	if !containsString(result.Content, "Calculate") {
		t.Error("deve encontrar Calculate")
	}
	if !containsString(result.Content, "Validate") {
		t.Error("deve encontrar Validate")
	}
}

func TestGrepSearch_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()

	content := `Hello World
hello world
HELLO WORLD
`
	_ = os.WriteFile(filepath.Join(dir, "test.txt"), []byte(content), 0644)

	tool := NewGrepSearch(dir)
	args := `{"pattern": "hello", "case_sensitive": false}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}
	// Deve encontrar todas as 3 ocorrências
	if !containsString(result.Content, "Hello World") {
		t.Error("deve encontrar 'Hello World'")
	}
	if !containsString(result.Content, "HELLO WORLD") {
		t.Error("deve encontrar 'HELLO WORLD'")
	}
}

func TestGrepSearch_IncludeFilter(t *testing.T) {
	dir := t.TempDir()

	// Cria arquivos de tipos diferentes
	_ = os.WriteFile(filepath.Join(dir, "code.go"), []byte("func main() {}"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "code.py"), []byte("def main(): pass"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("main note"), 0644)

	tool := NewGrepSearch(dir)
	args := `{"pattern": "main", "include": "*.go"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}
	// Deve encontrar apenas no .go
	if !containsString(result.Content, "code.go") {
		t.Error("deve encontrar em code.go")
	}
	if containsString(result.Content, "code.py") {
		t.Error("não deve encontrar em code.py")
	}
	if containsString(result.Content, "notes.txt") {
		t.Error("não deve encontrar em notes.txt")
	}
}

func TestGrepSearch_NoMatch(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello world"), 0644)

	tool := NewGrepSearch(dir)
	args := `{"pattern": "xyz_not_found"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Error("não deve ser erro, apenas zero resultados")
	}
	if !containsString(result.Content, "Nenhuma") {
		t.Error("deve indicar que não encontrou")
	}
}

func TestGrepSearch_SingleFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "single.txt")
	_ = os.WriteFile(filePath, []byte("line1\nfind_me\nline3"), 0644)

	tool := NewGrepSearch(dir)
	args := `{"pattern": "find_me", "path": "single.txt"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}
	if !containsString(result.Content, "find_me") {
		t.Error("deve encontrar 'find_me'")
	}
}

func TestGrepSearch_MissingPattern(t *testing.T) {
	tool := NewGrepSearch(t.TempDir())
	args := `{}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Error("deve ser erro sem pattern")
	}
}

func TestGrepSearch_SkipsBinaryFiles(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "image.png"), []byte("fake png with searchterm"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "code.go"), []byte("searchterm in code"), 0644)

	tool := NewGrepSearch(dir)
	args := `{"pattern": "searchterm"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	// Deve encontrar em code.go mas não em image.png
	if !containsString(result.Content, "code.go") {
		t.Error("deve encontrar em code.go")
	}
	if containsString(result.Content, "image.png") {
		t.Error("não deve buscar em image.png (binário)")
	}
}

func TestMatchIncludePattern(t *testing.T) {
	tests := []struct {
		filename string
		pattern  string
		expected bool
	}{
		{"main.go", "*.go", true},
		{"main.py", "*.go", false},
		{"app.tsx", "*.{ts,tsx}", true},
		{"app.ts", "*.{ts,tsx}", true},
		{"app.js", "*.{ts,tsx}", false},
		{"test_main.py", "test_*", true},
		{"main.py", "test_*", false},
	}

	for _, tt := range tests {
		result := matchIncludePattern(tt.filename, tt.pattern)
		if result != tt.expected {
			t.Errorf("matchIncludePattern(%q, %q) = %v, want %v", tt.filename, tt.pattern, result, tt.expected)
		}
	}
}

func TestIsBinaryExtension(t *testing.T) {
	tests := []struct {
		filename string
		expected bool
	}{
		{"main.go", false},
		{"photo.png", true},
		{"doc.pdf", true},
		{"data.json", false},
		{"archive.zip", true},
		{"script.py", false},
	}

	for _, tt := range tests {
		result := isBinaryExtension(tt.filename)
		if result != tt.expected {
			t.Errorf("isBinaryExtension(%q) = %v, want %v", tt.filename, result, tt.expected)
		}
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
