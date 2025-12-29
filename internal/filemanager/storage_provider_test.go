package filemanager

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewStorageManager(t *testing.T) {
	sm := NewStorageManager()
	
	if sm == nil {
		t.Fatal("NewStorageManager returned nil")
	}

	// Local provider deve estar sempre disponível
	if sm.local == nil {
		t.Error("Local provider is nil")
	}
}

func TestStorageManager_ParsePath(t *testing.T) {
	sm := NewStorageManager()

	tests := []struct {
		input           string
		expectedProvider string
		expectedPath    string
	}{
		// Caminhos locais
		{`C:\Users\test\file.txt`, "local", `C:\Users\test\file.txt`},
		{`/home/user/file.txt`, "local", `/home/user/file.txt`},
		{`./relative/path.txt`, "local", `./relative/path.txt`},
		{`file.txt`, "local", `file.txt`},
		
		// Google Drive pseudo-schemes
		{"gdrive://abc123", "gdrive", "abc123"},
		{"gdocs://document-id", "gdrive", "document-id"},
		{"gsheet://spreadsheet-id", "gdrive", "spreadsheet-id"},
		{"gslides://presentation-id", "gdrive", "presentation-id"},
		
		// URLs do Google
		{"https://docs.google.com/document/d/1BxiMVs0XRA5/edit", "gdrive", "1BxiMVs0XRA5"},
		{"https://docs.google.com/spreadsheets/d/abc123xyz/edit", "gdrive", "abc123xyz"},
		{"https://drive.google.com/file/d/fileid123/view", "gdrive", "fileid123"},
		
		// Outros providers (futuros)
		{"onedrive://path/to/file", "onedrive", "path/to/file"},
		{"dropbox://folder/file.txt", "dropbox", "folder/file.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			provider, path := sm.ParsePath(tt.input)
			if provider != tt.expectedProvider {
				t.Errorf("ParsePath(%q) provider = %q, want %q", tt.input, provider, tt.expectedProvider)
			}
			if path != tt.expectedPath {
				t.Errorf("ParsePath(%q) path = %q, want %q", tt.input, path, tt.expectedPath)
			}
		})
	}
}

func TestStorageManager_IsCloudPath(t *testing.T) {
	sm := NewStorageManager()

	cloudPaths := []string{
		"gdrive://abc123",
		"gdocs://document",
		"https://docs.google.com/document/d/abc/edit",
		"onedrive://path",
		"dropbox://file",
	}

	for _, path := range cloudPaths {
		t.Run(path+"_is_cloud", func(t *testing.T) {
			if !sm.IsCloudPath(path) {
				t.Errorf("IsCloudPath(%q) = false, want true", path)
			}
		})
	}

	localPaths := []string{
		`C:\Users\test\file.txt`,
		`/home/user/file.txt`,
		`./relative/path.txt`,
		`file.txt`,
	}

	for _, path := range localPaths {
		t.Run(path+"_is_local", func(t *testing.T) {
			if sm.IsCloudPath(path) {
				t.Errorf("IsCloudPath(%q) = true, want false", path)
			}
		})
	}
}

func TestStorageManager_GetProvider_Local(t *testing.T) {
	sm := NewStorageManager()

	provider, cleanPath, err := sm.GetProvider(`C:\test\file.txt`)
	if err != nil {
		t.Fatalf("GetProvider failed: %v", err)
	}

	if provider.Name() != "local" {
		t.Errorf("Provider name = %q, want %q", provider.Name(), "local")
	}

	if cleanPath != `C:\test\file.txt` {
		t.Errorf("Clean path = %q, want %q", cleanPath, `C:\test\file.txt`)
	}
}

func TestStorageManager_GetProvider_NotConfigured(t *testing.T) {
	sm := NewStorageManager()

	// Google Drive não está configurado (sem token)
	_, _, err := sm.GetProvider("gdrive://abc123")
	if err == nil {
		t.Error("Expected error for unconfigured gdrive provider, got nil")
	}
}

func TestStorageManager_RegisterProvider(t *testing.T) {
	sm := NewStorageManager()

	// Registra provider mock
	mock := &mockStorageProvider{
		name:      "mock",
		scheme:    "mock://",
		available: true,
	}

	sm.RegisterProvider(mock)

	// Verifica que foi registrado
	providers := sm.GetAvailableProviders()
	found := false
	for _, p := range providers {
		if p == "mock" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Mock provider not found in available providers")
	}
}

func TestStorageManager_GetAvailableProviders(t *testing.T) {
	sm := NewStorageManager()

	providers := sm.GetAvailableProviders()

	// Deve ter pelo menos o provider local
	found := false
	for _, p := range providers {
		if p == "local" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Local provider not found in available providers")
	}
}

func TestStorageManager_ReadFile_Local(t *testing.T) {
	sm := NewStorageManager()
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "Test content for StorageManager"
	os.WriteFile(testFile, []byte(content), 0644)

	result, err := sm.ReadFile(context.Background(), testFile, ReadOptions{})
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if result.Text != content {
		t.Errorf("Text = %q, want %q", result.Text, content)
	}
}

func TestStorageManager_ListDirectory_Local(t *testing.T) {
	sm := NewStorageManager()
	tmpDir := t.TempDir()

	// Cria alguns arquivos
	os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("1"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("2"), 0644)
	os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755)

	entries, err := sm.ListDirectory(context.Background(), tmpDir, ListOptions{})
	if err != nil {
		t.Fatalf("ListDirectory failed: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("Expected 3 entries, got %d", len(entries))
	}

	// Verifica que todos têm provider "local"
	for _, e := range entries {
		if e.Provider != "local" {
			t.Errorf("Entry provider = %q, want %q", e.Provider, "local")
		}
	}
}

func TestStorageManager_SearchByName_Local(t *testing.T) {
	sm := NewStorageManager()
	tmpDir := t.TempDir()

	// Cria arquivos
	os.WriteFile(filepath.Join(tmpDir, "test1.txt"), []byte("1"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "test2.txt"), []byte("2"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "other.go"), []byte("3"), 0644)

	results, err := sm.SearchByName(context.Background(), tmpDir, "*.txt", SearchOptions{})
	if err != nil {
		t.Fatalf("SearchByName failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

func TestStorageManager_SearchByContent_Local(t *testing.T) {
	sm := NewStorageManager()
	tmpDir := t.TempDir()

	// Cria arquivos com conteúdo
	os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("Hello World"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("Goodbye World"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file3.txt"), []byte("No match here"), 0644)

	results, err := sm.SearchByContent(context.Background(), tmpDir, "World", SearchOptions{})
	if err != nil {
		t.Fatalf("SearchByContent failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

func TestStorageManager_WriteFile_Local(t *testing.T) {
	sm := NewStorageManager()
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "write_test.txt")

	content := &Content{Text: "Written via StorageManager"}

	err := sm.WriteFile(context.Background(), testFile, content, WriteOptions{})
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Verifica conteúdo
	data, _ := os.ReadFile(testFile)
	if string(data) != content.Text {
		t.Errorf("Written content = %q, want %q", string(data), content.Text)
	}
}

func TestStorageManager_CreateDirectory_Local(t *testing.T) {
	sm := NewStorageManager()
	tmpDir := t.TempDir()
	newDir := filepath.Join(tmpDir, "new", "nested", "dir")

	err := sm.CreateDirectory(context.Background(), newDir)
	if err != nil {
		t.Fatalf("CreateDirectory failed: %v", err)
	}

	// Verifica que foi criado
	info, err := os.Stat(newDir)
	if err != nil {
		t.Fatalf("Directory was not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("Created path is not a directory")
	}
}

func TestStorageManager_DeleteFile_Local(t *testing.T) {
	sm := NewStorageManager()
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "to_delete.txt")
	os.WriteFile(testFile, []byte("delete me"), 0644)

	// Sem autorização, delete deve falhar
	err := sm.DeleteFile(context.Background(), testFile)
	if err == nil {
		t.Error("Expected error for delete without authorization")
	} else {
		t.Logf("Delete without authorization correctly failed: %v", err)
	}

	// Arquivo ainda deve existir
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Error("File should not have been deleted without authorization")
	}
}

// Mock storage provider for testing
type mockStorageProvider struct {
	name      string
	scheme    string
	available bool
}

func (m *mockStorageProvider) Name() string       { return m.name }
func (m *mockStorageProvider) Scheme() string     { return m.scheme }
func (m *mockStorageProvider) IsAvailable() bool  { return m.available }
func (m *mockStorageProvider) CanWrite() bool     { return true }
func (m *mockStorageProvider) CanDelete() bool    { return true }

func (m *mockStorageProvider) ReadFile(ctx context.Context, path string, opts ReadOptions) (*Content, error) {
	return &Content{Text: "mock content"}, nil
}

func (m *mockStorageProvider) GetFileInfo(ctx context.Context, path string) (*RemoteFileInfo, error) {
	return &RemoteFileInfo{Name: "mock", Provider: m.name}, nil
}

func (m *mockStorageProvider) ListDirectory(ctx context.Context, path string, opts ListOptions) ([]RemoteFileInfo, error) {
	return []RemoteFileInfo{}, nil
}

func (m *mockStorageProvider) SearchByName(ctx context.Context, basePath string, pattern string, opts SearchOptions) ([]RemoteFileInfo, error) {
	return []RemoteFileInfo{}, nil
}

func (m *mockStorageProvider) SearchByContent(ctx context.Context, basePath string, query string, opts SearchOptions) ([]SearchResult, error) {
	return []SearchResult{}, nil
}

func (m *mockStorageProvider) WriteFile(ctx context.Context, path string, content *Content, opts WriteOptions) error {
	return nil
}

func (m *mockStorageProvider) CreateDirectory(ctx context.Context, path string) error {
	return nil
}

func (m *mockStorageProvider) DeleteFile(ctx context.Context, path string) error {
	return nil
}

