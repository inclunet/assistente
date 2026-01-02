package filemanager

import (
	"testing"
)

func TestPowerPointHandler_Name(t *testing.T) {
	h := NewPowerPointHandler()
	if h.Name() != "powerpoint" {
		t.Errorf("Name() = %q, want %q", h.Name(), "powerpoint")
	}
}

func TestPowerPointHandler_Extensions(t *testing.T) {
	h := NewPowerPointHandler()
	exts := h.Extensions()

	if len(exts) == 0 {
		t.Error("Extensions() returned empty list")
	}

	// Deve suportar .pptx
	found := false
	for _, ext := range exts {
		if ext == ".pptx" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected .pptx in extensions")
	}
}

func TestPowerPointHandler_Capabilities(t *testing.T) {
	h := NewPowerPointHandler()
	caps := h.Capabilities()

	if !caps.CanRead {
		t.Error("Expected CanRead = true")
	}
}

func TestPowerPointHandler_ReadContent_FileNotFound(t *testing.T) {
	h := NewPowerPointHandler()

	_, err := h.ReadContent("/nonexistent/file.pptx", ReadOptions{})
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestPowerPointHandler_GetMetadata_FileNotFound(t *testing.T) {
	h := NewPowerPointHandler()

	_, err := h.GetMetadata("/nonexistent/file.pptx")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestPowerPointHandler_MimeTypes(t *testing.T) {
	h := NewPowerPointHandler()
	mimes := h.MimeTypes()

	if len(mimes) == 0 {
		t.Error("MimeTypes() returned empty list")
	}
}



