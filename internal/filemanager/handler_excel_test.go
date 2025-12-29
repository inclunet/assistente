package filemanager

import (
	"testing"
)

func TestExcelHandler_Name(t *testing.T) {
	h := NewExcelHandler()
	if h.Name() != "excel" {
		t.Errorf("Name() = %q, want %q", h.Name(), "excel")
	}
}

func TestExcelHandler_Extensions(t *testing.T) {
	h := NewExcelHandler()
	exts := h.Extensions()

	if len(exts) == 0 {
		t.Error("Extensions() returned empty list")
	}

	// Deve suportar pelo menos .xlsx
	found := false
	for _, ext := range exts {
		if ext == ".xlsx" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected .xlsx in extensions")
	}
}

func TestExcelHandler_Capabilities(t *testing.T) {
	h := NewExcelHandler()
	caps := h.Capabilities()

	if !caps.CanRead {
		t.Error("Expected CanRead = true")
	}
}

func TestExcelHandler_ReadContent_FileNotFound(t *testing.T) {
	h := NewExcelHandler()

	_, err := h.ReadContent("/nonexistent/file.xlsx", ReadOptions{})
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestExcelHandler_GetMetadata_FileNotFound(t *testing.T) {
	h := NewExcelHandler()

	_, err := h.GetMetadata("/nonexistent/file.xlsx")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

// Teste com arquivo XLSX real requer criar um arquivo válido
// que é complexo sem a biblioteca excelize já em uso
func TestExcelHandler_MimeTypes(t *testing.T) {
	h := NewExcelHandler()
	mimes := h.MimeTypes()

	if len(mimes) == 0 {
		t.Error("MimeTypes() returned empty list")
	}
}

