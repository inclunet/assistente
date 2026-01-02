package filemanager

import (
	"testing"
)

func TestPDFHandler_Name(t *testing.T) {
	h := NewPDFHandler()
	if h.Name() != "pdf" {
		t.Errorf("Name() = %q, want %q", h.Name(), "pdf")
	}
}

func TestPDFHandler_Extensions(t *testing.T) {
	h := NewPDFHandler()
	exts := h.Extensions()

	if len(exts) == 0 {
		t.Error("Extensions() returned empty list")
	}

	// Deve suportar .pdf
	found := false
	for _, ext := range exts {
		if ext == ".pdf" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected .pdf in extensions")
	}
}

func TestPDFHandler_Capabilities(t *testing.T) {
	h := NewPDFHandler()
	caps := h.Capabilities()

	if !caps.CanRead {
		t.Error("Expected CanRead = true")
	}
	// PDF não suporta escrita
	if caps.CanWrite {
		t.Error("Expected CanWrite = false for PDF")
	}
}

func TestPDFHandler_ReadContent_FileNotFound(t *testing.T) {
	h := NewPDFHandler()

	_, err := h.ReadContent("/nonexistent/file.pdf", ReadOptions{})
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestPDFHandler_GetMetadata_FileNotFound(t *testing.T) {
	h := NewPDFHandler()

	_, err := h.GetMetadata("/nonexistent/file.pdf")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestPDFHandler_MimeTypes(t *testing.T) {
	h := NewPDFHandler()
	mimes := h.MimeTypes()

	if len(mimes) == 0 {
		t.Error("MimeTypes() returned empty list")
	}

	// Deve incluir application/pdf
	found := false
	for _, mime := range mimes {
		if mime == "application/pdf" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected application/pdf in MimeTypes")
	}
}

func TestPDFHandler_WriteContent_NotSupported(t *testing.T) {
	h := NewPDFHandler()

	err := h.WriteContent("/any/path.pdf", &Content{Text: "test"}, WriteOptions{})
	if err == nil {
		t.Error("Expected error for write to PDF (not supported)")
	}
}



