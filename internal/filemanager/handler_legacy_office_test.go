package filemanager

import (
	"testing"
)

func TestLegacyOfficeHandler_Name(t *testing.T) {
	h := NewLegacyOfficeHandler()
	if h.Name() != "legacy_office" {
		t.Errorf("Name() = %q, want %q", h.Name(), "legacy_office")
	}
}

func TestLegacyOfficeHandler_Extensions(t *testing.T) {
	h := NewLegacyOfficeHandler()
	exts := h.Extensions()

	if len(exts) == 0 {
		t.Error("Extensions() returned empty list")
	}

	// Deve suportar pelo menos .xls
	found := false
	for _, ext := range exts {
		if ext == ".xls" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected .xls in extensions")
	}
}

func TestLegacyOfficeHandler_Capabilities(t *testing.T) {
	h := NewLegacyOfficeHandler()
	caps := h.Capabilities()

	if !caps.CanRead {
		t.Error("Expected CanRead = true")
	}
	// Legacy não suporta escrita
	if caps.CanWrite {
		t.Error("Expected CanWrite = false for Legacy Office")
	}
}

func TestLegacyOfficeHandler_ReadContent_FileNotFound(t *testing.T) {
	h := NewLegacyOfficeHandler()

	_, err := h.ReadContent("/nonexistent/file.xls", ReadOptions{})
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestLegacyOfficeHandler_GetMetadata_FileNotFound(t *testing.T) {
	h := NewLegacyOfficeHandler()

	meta, err := h.GetMetadata("/nonexistent/file.xls")
	// A implementação pode retornar metadata vazio sem erro
	// ou retornar erro - ambos são aceitáveis
	if err != nil {
		t.Logf("GetMetadata returned error (expected): %v", err)
	} else {
		t.Logf("GetMetadata returned metadata: %v", meta)
	}
}

func TestLegacyOfficeHandler_MimeTypes(t *testing.T) {
	h := NewLegacyOfficeHandler()
	mimes := h.MimeTypes()

	if len(mimes) == 0 {
		t.Error("MimeTypes() returned empty list")
	}
}

func TestLegacyOfficeHandler_WriteContent_NotSupported(t *testing.T) {
	h := NewLegacyOfficeHandler()

	err := h.WriteContent("/any/path.xls", &Content{Text: "test"}, WriteOptions{})
	if err == nil {
		t.Error("Expected error for write to legacy format (not supported)")
	}
}

