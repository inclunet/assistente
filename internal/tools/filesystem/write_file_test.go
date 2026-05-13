package filesystem

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFile_Name(t *testing.T) {
	tool := NewWriteFile("/tmp")
	if tool.Name() != "write_file" {
		t.Errorf("expected 'write_file', got '%s'", tool.Name())
	}
}

func TestWriteFile_CreateNew(t *testing.T) {
	dir := t.TempDir()
	tool := NewWriteFile(dir)

	args := `{"path": "hello.txt", "content": "Hello, World!\nLine 2"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}

	// Verifica que arquivo foi criado
	data, err := os.ReadFile(filepath.Join(dir, "hello.txt"))
	if err != nil {
		t.Fatalf("arquivo não foi criado: %v", err)
	}
	if string(data) != "Hello, World!\nLine 2" {
		t.Errorf("conteúdo incorreto: %s", string(data))
	}

	// Verifica relatório
	if !containsString(result.Content, "Criado") {
		t.Error("resultado deve indicar que foi criado")
	}
}

func TestWriteFile_Overwrite(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "existing.txt")
	_ = os.WriteFile(filePath, []byte("old content"), 0644)

	tool := NewWriteFile(dir)
	args := `{"path": "existing.txt", "content": "new content"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}

	data, _ := os.ReadFile(filePath)
	if string(data) != "new content" {
		t.Errorf("conteúdo não foi sobrescrito: %s", string(data))
	}

	if !containsString(result.Content, "Sobrescrito") {
		t.Error("resultado deve indicar que foi sobrescrito")
	}
}

func TestWriteFile_CreatesDirs(t *testing.T) {
	dir := t.TempDir()
	tool := NewWriteFile(dir)

	args := `{"path": "sub/dir/deep/file.txt", "content": "nested content"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}

	data, err := os.ReadFile(filepath.Join(dir, "sub", "dir", "deep", "file.txt"))
	if err != nil {
		t.Fatalf("arquivo em subdiretório não foi criado: %v", err)
	}
	if string(data) != "nested content" {
		t.Errorf("conteúdo incorreto: %s", string(data))
	}
}

func TestWriteFile_MissingPath(t *testing.T) {
	tool := NewWriteFile(t.TempDir())
	args := `{"content": "test"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Error("deve ser erro sem path")
	}
}

func TestWriteFile_BlocksSensitiveFiles(t *testing.T) {
	dir := t.TempDir()
	tool := NewWriteFile(dir)

	sensitiveFiles := []string{".env", "id_rsa", "server.key", "cert.pem"}
	for _, name := range sensitiveFiles {
		args, _ := json.Marshal(map[string]string{"path": name, "content": "test"})
		result, err := tool.Execute(context.Background(), json.RawMessage(args))
		if err != nil {
			t.Fatalf("Execute retornou erro para %s: %v", name, err)
		}
		if !result.IsError {
			t.Errorf("deve bloquear escrita em arquivo sensível: %s", name)
		}
	}
}

func TestWriteFile_ContentTooLarge(t *testing.T) {
	dir := t.TempDir()
	tool := NewWriteFile(dir)

	// Cria conteúdo maior que 5MB
	large := make([]byte, writeMaxContentSize+1)
	for i := range large {
		large[i] = 'x'
	}

	args, _ := json.Marshal(map[string]string{"path": "big.txt", "content": string(large)})
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Error("deve rejeitar conteúdo muito grande")
	}
}

func TestWriteFile_CannotOverwriteDir(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "subdir"), 0755)

	tool := NewWriteFile(dir)
	args := `{"path": "subdir", "content": "test"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Error("deve rejeitar sobrescrever diretório")
	}
}

func TestIsSensitiveFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/home/user/.env", true},
		{"/home/user/.env.local", true},
		{"/home/user/id_rsa", true},
		{"/home/user/server.key", true},
		{"/home/user/cert.pem", true},
		{"/home/user/main.go", false},
		{"/home/user/config.yaml", false},
		{"/home/user/data.json", false},
	}

	for _, tt := range tests {
		result := isSensitiveFile(tt.path)
		if result != tt.expected {
			t.Errorf("isSensitiveFile(%q) = %v, want %v", tt.path, result, tt.expected)
		}
	}
}
