package filemanager

import (
	"testing"
)

func TestOpenDocumentHandler_Name(t *testing.T) {
	h := NewOpenDocumentHandler()
	if h.Name() != "opendocument" {
		t.Errorf("Name() = %q, want %q", h.Name(), "opendocument")
	}
}

func TestOpenDocumentHandler_Extensions(t *testing.T) {
	h := NewOpenDocumentHandler()
	exts := h.Extensions()

	if len(exts) == 0 {
		t.Error("Extensions() returned empty list")
	}

	// Deve suportar .odt, .ods, .odp
	expectedExts := map[string]bool{
		".odt": true,
		".ods": true,
		".odp": true,
	}

	for _, ext := range exts {
		delete(expectedExts, ext)
	}

	if len(expectedExts) > 0 {
		t.Errorf("Missing extensions: %v", expectedExts)
	}
}

func TestOpenDocumentHandler_Capabilities(t *testing.T) {
	h := NewOpenDocumentHandler()
	caps := h.Capabilities()

	if !caps.CanRead {
		t.Error("Expected CanRead = true")
	}
}

func TestOpenDocumentHandler_ReadContent_FileNotFound(t *testing.T) {
	h := NewOpenDocumentHandler()

	_, err := h.ReadContent("/nonexistent/file.odt", ReadOptions{})
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestOpenDocumentHandler_GetMetadata_FileNotFound(t *testing.T) {
	h := NewOpenDocumentHandler()

	_, err := h.GetMetadata("/nonexistent/file.odt")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestOpenDocumentHandler_MimeTypes(t *testing.T) {
	h := NewOpenDocumentHandler()
	mimes := h.MimeTypes()

	if len(mimes) == 0 {
		t.Error("MimeTypes() returned empty list")
	}
}

