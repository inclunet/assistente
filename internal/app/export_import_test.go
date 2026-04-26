package app

import (
	"strings"
	"testing"

	"assistente/internal/portability"
)

func TestNormalizeRichConversationExportRequestRejectsCredentials(t *testing.T) {
	_, err := normalizeRichConversationExportRequest(ExportRequest{
		OutputFormat:       portability.FormatHTML,
		IncludeCredentials: true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "não suporta credenciais") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeRichConversationExportRequestRejectsCredentialPassword(t *testing.T) {
	_, err := normalizeRichConversationExportRequest(ExportRequest{
		OutputFormat:             portability.FormatPDF,
		CredentialExportPassword: "segredo",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "não usa senha de credenciais") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeRichConversationExportRequestClearsCredentialFields(t *testing.T) {
	req, err := normalizeRichConversationExportRequest(ExportRequest{
		OutputFormat: portability.FormatHTML,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.IncludeCredentials {
		t.Fatal("expected credentials to remain disabled")
	}
	if req.CredentialExportPassword != "" {
		t.Fatalf("expected empty credential password, got %q", req.CredentialExportPassword)
	}
}

func TestResolveConversationIDsRespectsExplicitSelection(t *testing.T) {
	ids, err := resolveConversationIDs(ExportRequest{
		ExplicitSelection:  true,
		IncludeCredentials: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ids != nil {
		t.Fatalf("expected nil conversation ids, got %#v", ids)
	}
}
