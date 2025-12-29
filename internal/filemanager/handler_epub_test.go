package filemanager

import (
	"testing"
)

func TestEpubHandler_Name(t *testing.T) {
	h := NewEPUBHandler()
	if h.Name() != "epub" {
		t.Errorf("Name() = %q, want %q", h.Name(), "epub")
	}
}

func TestEpubHandler_Extensions(t *testing.T) {
	h := NewEPUBHandler()
	exts := h.Extensions()

	if len(exts) == 0 {
		t.Error("Extensions() returned empty list")
	}

	// Deve suportar .epub
	found := false
	for _, ext := range exts {
		if ext == ".epub" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected .epub in extensions")
	}
}

func TestEpubHandler_Capabilities(t *testing.T) {
	h := NewEPUBHandler()
	caps := h.Capabilities()

	if !caps.CanRead {
		t.Error("Expected CanRead = true")
	}
	// EPUB não suporta escrita
	if caps.CanWrite {
		t.Error("Expected CanWrite = false for EPUB")
	}
}

func TestEpubHandler_ReadContent_FileNotFound(t *testing.T) {
	h := NewEPUBHandler()

	_, err := h.ReadContent("/nonexistent/file.epub", ReadOptions{})
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestEpubHandler_GetMetadata_FileNotFound(t *testing.T) {
	h := NewEPUBHandler()

	_, err := h.GetMetadata("/nonexistent/file.epub")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestEpubHandler_MimeTypes(t *testing.T) {
	h := NewEPUBHandler()
	mimes := h.MimeTypes()

	if len(mimes) == 0 {
		t.Error("MimeTypes() returned empty list")
	}
}

func TestEpubHandler_WriteContent_NotSupported(t *testing.T) {
	h := NewEPUBHandler()

	err := h.WriteContent("/any/path.epub", &Content{Text: "test"}, WriteOptions{})
	if err == nil {
		t.Error("Expected error for write to EPUB (not supported)")
	}
}

