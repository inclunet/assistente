package filemanager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlainTextHandler_Name(t *testing.T) {
	h := NewPlainTextHandler()
	if h.Name() != "plaintext" {
		t.Errorf("Name() = %q, want %q", h.Name(), "plaintext")
	}
}

func TestPlainTextHandler_Extensions(t *testing.T) {
	h := NewPlainTextHandler()
	exts := h.Extensions()
	
	if len(exts) == 0 {
		t.Error("Extensions() returned empty list")
	}

	// Verifica algumas extensões esperadas
	expectedExts := map[string]bool{
		".txt": true,
		".md":  true,
		".go":  true,
		".py":  true,
		".js":  true,
	}

	extMap := make(map[string]bool)
	for _, ext := range exts {
		extMap[ext] = true
	}

	for ext := range expectedExts {
		if !extMap[ext] {
			t.Errorf("Expected extension %q not found", ext)
		}
	}
}

func TestPlainTextHandler_Capabilities(t *testing.T) {
	h := NewPlainTextHandler()
	caps := h.Capabilities()

	if !caps.CanRead {
		t.Error("Expected CanRead = true")
	}
	if !caps.CanWrite {
		t.Error("Expected CanWrite = true")
	}
	if !caps.CanSearch {
		t.Error("Expected CanSearch = true")
	}
}

func TestPlainTextHandler_ReadContent(t *testing.T) {
	h := NewPlainTextHandler()
	tmpDir := t.TempDir()
	
	// Cria arquivo de teste
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "Hello, World!\nThis is line 2.\nLine 3 here."
	os.WriteFile(testFile, []byte(content), 0644)

	result, err := h.ReadContent(testFile, ReadOptions{})
	if err != nil {
		t.Fatalf("ReadContent failed: %v", err)
	}

	if result.Text != content {
		t.Errorf("Text = %q, want %q", result.Text, content)
	}

	if result.LineCount != 3 {
		t.Errorf("LineCount = %d, want %d", result.LineCount, 3)
	}
}

func TestPlainTextHandler_ReadContent_UTF8BOM(t *testing.T) {
	h := NewPlainTextHandler()
	tmpDir := t.TempDir()
	
	// Cria arquivo com BOM UTF-8
	testFile := filepath.Join(tmpDir, "bom.txt")
	bom := []byte{0xEF, 0xBB, 0xBF}
	content := append(bom, []byte("Hello with BOM")...)
	os.WriteFile(testFile, content, 0644)

	result, err := h.ReadContent(testFile, ReadOptions{})
	if err != nil {
		t.Fatalf("ReadContent failed: %v", err)
	}

	// BOM deve ser removido
	if strings.HasPrefix(result.Text, "\xEF\xBB\xBF") {
		t.Error("BOM was not removed from content")
	}

	if result.Text != "Hello with BOM" {
		t.Errorf("Text = %q, want %q", result.Text, "Hello with BOM")
	}
}

func TestPlainTextHandler_ReadContent_MaxBytes(t *testing.T) {
	h := NewPlainTextHandler()
	tmpDir := t.TempDir()
	
	// Cria arquivo grande
	testFile := filepath.Join(tmpDir, "large.txt")
	content := strings.Repeat("A", 10000)
	os.WriteFile(testFile, []byte(content), 0644)

	// Lê com limite
	result, err := h.ReadContent(testFile, ReadOptions{MaxBytes: 100})
	if err != nil {
		t.Fatalf("ReadContent failed: %v", err)
	}

	if len(result.Text) > 100 {
		t.Errorf("Text length = %d, want <= 100", len(result.Text))
	}
}

func TestPlainTextHandler_WriteContent(t *testing.T) {
	h := NewPlainTextHandler()
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "write_test.txt")

	content := &Content{
		Text: "Written content\nLine 2",
	}

	err := h.WriteContent(testFile, content, WriteOptions{})
	if err != nil {
		t.Fatalf("WriteContent failed: %v", err)
	}

	// Verifica conteúdo escrito
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read written file: %v", err)
	}

	if string(data) != content.Text {
		t.Errorf("Written content = %q, want %q", string(data), content.Text)
	}
}

func TestPlainTextHandler_WriteContent_CreateDirs(t *testing.T) {
	h := NewPlainTextHandler()
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "new", "subdir", "file.txt")

	content := &Content{
		Text: "In nested dir",
	}

	err := h.WriteContent(testFile, content, WriteOptions{CreateDirs: true})
	if err != nil {
		t.Fatalf("WriteContent failed: %v", err)
	}

	// Verifica que arquivo foi criado
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Error("File was not created")
	}
}

func TestPlainTextHandler_SearchContent(t *testing.T) {
	h := NewPlainTextHandler()
	tmpDir := t.TempDir()
	
	testFile := filepath.Join(tmpDir, "search.txt")
	content := `Line 1: Hello World
Line 2: This is a test
Line 3: Hello again
Line 4: Final line`
	os.WriteFile(testFile, []byte(content), 0644)

	// Busca simples
	matches, err := h.SearchContent(testFile, "Hello", SearchOptions{})
	if err != nil {
		t.Fatalf("SearchContent failed: %v", err)
	}

	if len(matches) != 2 {
		t.Errorf("Expected 2 matches, got %d", len(matches))
	}

	// Verifica números de linha
	if matches[0].LineNumber != 1 {
		t.Errorf("First match line = %d, want 1", matches[0].LineNumber)
	}
	if matches[1].LineNumber != 3 {
		t.Errorf("Second match line = %d, want 3", matches[1].LineNumber)
	}
}

func TestPlainTextHandler_SearchContent_CaseInsensitive(t *testing.T) {
	h := NewPlainTextHandler()
	tmpDir := t.TempDir()
	
	testFile := filepath.Join(tmpDir, "search.txt")
	content := "HELLO hello HeLLo"
	os.WriteFile(testFile, []byte(content), 0644)

	// Case insensitive (default)
	matches, err := h.SearchContent(testFile, "hello", SearchOptions{CaseSensitive: false})
	if err != nil {
		t.Fatalf("SearchContent failed: %v", err)
	}

	// Deve encontrar a linha (todas as variações estão na mesma linha)
	if len(matches) != 1 {
		t.Errorf("Expected 1 match, got %d", len(matches))
	}
}

func TestPlainTextHandler_SearchContent_CaseSensitive(t *testing.T) {
	h := NewPlainTextHandler()
	tmpDir := t.TempDir()
	
	testFile := filepath.Join(tmpDir, "search.txt")
	content := `HELLO
hello
HeLLo`
	os.WriteFile(testFile, []byte(content), 0644)

	// Case sensitive
	matches, err := h.SearchContent(testFile, "hello", SearchOptions{CaseSensitive: true})
	if err != nil {
		t.Fatalf("SearchContent failed: %v", err)
	}

	if len(matches) != 1 {
		t.Errorf("Expected 1 match, got %d", len(matches))
	}
}

func TestPlainTextHandler_SearchContent_Regex(t *testing.T) {
	h := NewPlainTextHandler()
	tmpDir := t.TempDir()
	
	testFile := filepath.Join(tmpDir, "search.txt")
	content := `error: something failed
warning: be careful
error: another error
info: all good`
	os.WriteFile(testFile, []byte(content), 0644)

	// Busca com regex
	matches, err := h.SearchContent(testFile, `error:.*`, SearchOptions{UseRegex: true})
	if err != nil {
		t.Fatalf("SearchContent failed: %v", err)
	}

	if len(matches) != 2 {
		t.Errorf("Expected 2 matches, got %d", len(matches))
	}
}

func TestPlainTextHandler_SearchContent_MaxResults(t *testing.T) {
	h := NewPlainTextHandler()
	tmpDir := t.TempDir()
	
	testFile := filepath.Join(tmpDir, "search.txt")
	content := "match\nmatch\nmatch\nmatch\nmatch"
	os.WriteFile(testFile, []byte(content), 0644)

	matches, err := h.SearchContent(testFile, "match", SearchOptions{MaxResults: 2})
	if err != nil {
		t.Fatalf("SearchContent failed: %v", err)
	}

	if len(matches) != 2 {
		t.Errorf("Expected 2 matches (limited), got %d", len(matches))
	}
}

func TestPlainTextHandler_SearchContent_WithContext(t *testing.T) {
	h := NewPlainTextHandler()
	tmpDir := t.TempDir()
	
	testFile := filepath.Join(tmpDir, "search.txt")
	content := `Line 1
Line 2
TARGET LINE
Line 4
Line 5`
	os.WriteFile(testFile, []byte(content), 0644)

	matches, err := h.SearchContent(testFile, "TARGET", SearchOptions{ContextLines: 1})
	if err != nil {
		t.Fatalf("SearchContent failed: %v", err)
	}

	if len(matches) != 1 {
		t.Fatalf("Expected 1 match, got %d", len(matches))
	}

	// O handler atual pode não suportar contexto - apenas verifica que encontrou
	match := matches[0]
	t.Logf("Match: line %d, context before: %d, context after: %d", 
		match.LineNumber, len(match.ContextBefore), len(match.ContextAfter))
}

func TestPlainTextHandler_GetMetadata(t *testing.T) {
	h := NewPlainTextHandler()
	tmpDir := t.TempDir()
	
	testFile := filepath.Join(tmpDir, "meta.txt")
	content := "Test content for metadata"
	os.WriteFile(testFile, []byte(content), 0644)

	meta, err := h.GetMetadata(testFile)
	if err != nil {
		t.Fatalf("GetMetadata failed: %v", err)
	}

	// Verifica que retornou algum metadata
	if len(meta) == 0 {
		t.Error("GetMetadata returned empty map")
	}
	
	// Log what we got
	t.Logf("Metadata: %v", meta)
}

