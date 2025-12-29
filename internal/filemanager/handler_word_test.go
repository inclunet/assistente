package filemanager

import (
	"testing"
)

func TestWordHandler_Name(t *testing.T) {
	h := NewWordHandler()
	if h.Name() != "word" {
		t.Errorf("Name() = %q, want %q", h.Name(), "word")
	}
}

func TestWordHandler_Extensions(t *testing.T) {
	h := NewWordHandler()
	exts := h.Extensions()

	if len(exts) == 0 {
		t.Error("Extensions() returned empty list")
	}

	// Deve suportar .docx
	found := false
	for _, ext := range exts {
		if ext == ".docx" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected .docx in extensions")
	}
}

func TestWordHandler_Capabilities(t *testing.T) {
	h := NewWordHandler()
	caps := h.Capabilities()

	if !caps.CanRead {
		t.Error("Expected CanRead = true")
	}
}

func TestWordHandler_ReadContent_FileNotFound(t *testing.T) {
	h := NewWordHandler()

	_, err := h.ReadContent("/nonexistent/file.docx", ReadOptions{})
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestWordHandler_GetMetadata_FileNotFound(t *testing.T) {
	h := NewWordHandler()

	_, err := h.GetMetadata("/nonexistent/file.docx")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestWordHandler_MimeTypes(t *testing.T) {
	h := NewWordHandler()
	mimes := h.MimeTypes()

	if len(mimes) == 0 {
		t.Error("MimeTypes() returned empty list")
	}
}

