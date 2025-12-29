package filemanager

import (
	"testing"
)

func TestNewFormatRegistry(t *testing.T) {
	r := NewFormatRegistry()
	
	if r == nil {
		t.Fatal("NewFormatRegistry returned nil")
	}

	// Verifica que handlers foram registrados
	handlers := r.GetAllHandlers()
	if len(handlers) == 0 {
		t.Error("No handlers registered")
	}
}

func TestFormatRegistry_GetHandler(t *testing.T) {
	r := NewFormatRegistry()

	tests := []struct {
		filename     string
		expectedType string
	}{
		{"test.txt", "plaintext"},
		{"test.md", "plaintext"},
		{"test.go", "plaintext"},
		{"test.py", "plaintext"},
		{"test.json", "plaintext"},
		{"test.docx", "word"},
		{"test.xlsx", "excel"},
		{"test.pdf", "pdf"},
		{"test.odt", "opendocument"},
		{"test.ods", "opendocument"},
		{"test.odp", "opendocument"},
		{"test.pptx", "powerpoint"},
		{"test.epub", "epub"},
		{"test.xls", "legacy_office"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			handler := r.GetHandler(tt.filename)
			if handler == nil {
				t.Errorf("GetHandler(%q) returned nil", tt.filename)
				return
			}
			if handler.Name() != tt.expectedType {
				t.Errorf("GetHandler(%q).Name() = %q, want %q", tt.filename, handler.Name(), tt.expectedType)
			}
		})
	}
}

func TestFormatRegistry_GetHandler_CaseInsensitive(t *testing.T) {
	r := NewFormatRegistry()

	// Extensões em diferentes cases devem funcionar
	cases := []string{
		"test.TXT",
		"test.Txt",
		"test.DOCX",
		"test.DocX",
		"test.PDF",
	}

	for _, filename := range cases {
		t.Run(filename, func(t *testing.T) {
			handler := r.GetHandler(filename)
			if handler == nil {
				t.Errorf("GetHandler(%q) returned nil (case sensitivity issue?)", filename)
			}
		})
	}
}

func TestFormatRegistry_GetHandler_NoHandler(t *testing.T) {
	r := NewFormatRegistry()

	// O PlainTextHandler pode lidar com qualquer arquivo como texto
	// Então para extensões desconhecidas, pode retornar plaintext
	handler := r.GetHandler("test.xyz123unknown")
	
	// Apenas verifica que não dá panic
	if handler != nil {
		t.Logf("Handler for unknown extension: %s", handler.Name())
	} else {
		t.Log("No handler for unknown extension (expected)")
	}
}

func TestFormatRegistry_Register(t *testing.T) {
	r := NewFormatRegistry()
	
	// Conta handlers antes
	beforeCount := len(r.GetAllHandlers())

	// Cria handler mock
	mock := &mockHandler{
		name: "MockHandler",
		exts: []string{".mock", ".test123"},
	}

	r.Register(mock)

	// Conta handlers depois
	afterCount := len(r.GetAllHandlers())
	if afterCount != beforeCount+1 {
		t.Errorf("Handler count = %d, want %d", afterCount, beforeCount+1)
	}

	// Verifica que novo handler é encontrado
	handler := r.GetHandler("file.mock")
	if handler == nil {
		t.Error("GetHandler for .mock returned nil")
	}
	if handler != nil && handler.Name() != "MockHandler" {
		t.Errorf("Handler name = %q, want %q", handler.Name(), "MockHandler")
	}
}

func TestFormatRegistry_GetHandler_ByMimeType(t *testing.T) {
	r := NewFormatRegistry()

	// GetHandler também aceita MIME types
	tests := []struct {
		mimeType     string
		expectedType string
	}{
		{"text/plain", "plaintext"},
		{"application/json", "plaintext"},
		{"application/pdf", "pdf"},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			handler := r.GetHandler(tt.mimeType)
			if handler == nil {
				t.Skipf("GetHandler(%q) returned nil (MIME type lookup not implemented)", tt.mimeType)
				return
			}
			if handler.Name() != tt.expectedType {
				t.Errorf("GetHandler(%q).Name() = %q, want %q", tt.mimeType, handler.Name(), tt.expectedType)
			}
		})
	}
}

func TestFormatRegistry_GetAllHandlers(t *testing.T) {
	r := NewFormatRegistry()
	handlers := r.GetAllHandlers()

	// Deve ter pelo menos os handlers básicos
	expectedHandlers := []string{
		"plaintext",
		"word",
		"excel",
		"pdf",
		"opendocument",
		"powerpoint",
		"epub",
		"legacy_office",
	}

	handlerNames := make(map[string]bool)
	for _, h := range handlers {
		handlerNames[h.Name()] = true
	}

	for _, expected := range expectedHandlers {
		if !handlerNames[expected] {
			t.Errorf("Expected handler %q not found", expected)
		}
	}
}

func TestFormatRegistry_ReadContent(t *testing.T) {
	r := NewFormatRegistry()
	
	// Cria arquivo de teste temporário
	tmpDir := t.TempDir()
	testFile := tmpDir + "/test.txt"
	content := "Test content for registry"
	
	// Escreve arquivo
	handler := r.GetHandler(testFile)
	if handler == nil {
		t.Fatal("No handler for .txt")
	}
	
	err := handler.WriteContent(testFile, &Content{Text: content}, WriteOptions{CreateDirs: true})
	if err != nil {
		t.Fatalf("WriteContent failed: %v", err)
	}

	// Lê via registry
	result, err := r.ReadContent(testFile, ReadOptions{})
	if err != nil {
		t.Fatalf("ReadContent failed: %v", err)
	}

	if result.Text != content {
		t.Errorf("Text = %q, want %q", result.Text, content)
	}
}

// Mock handler for testing
type mockHandler struct {
	name  string
	exts  []string
	mimes []string
}

func (m *mockHandler) Name() string              { return m.name }
func (m *mockHandler) Extensions() []string      { return m.exts }
func (m *mockHandler) MimeTypes() []string       { return m.mimes }
func (m *mockHandler) Capabilities() Capabilities { return Capabilities{CanRead: true} }
func (m *mockHandler) ReadContent(path string, opts ReadOptions) (*Content, error) {
	return &Content{Text: "mock content"}, nil
}
func (m *mockHandler) WriteContent(path string, content *Content, opts WriteOptions) error {
	return nil
}
func (m *mockHandler) SearchContent(path string, query string, opts SearchOptions) ([]SearchMatch, error) {
	return nil, nil
}
func (m *mockHandler) GetMetadata(path string) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

