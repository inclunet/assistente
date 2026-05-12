package app

import (
	"context"
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
	ids, err := resolveConversationIDs(context.Background(), ExportRequest{
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

func TestResolveConversationIDsAcceptsUUIDStrings(t *testing.T) {
	const id = "01926b90-7a5a-7c4e-8d3f-000000000001"
	ids, err := resolveConversationIDs(context.Background(), ExportRequest{
		ConversationIDs: []string{id},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 1 || ids[0] != id {
		t.Fatalf("expected UUID string to be preserved, got %#v", ids)
	}
}

func TestResolveTaskListIDsAcceptsUUIDStrings(t *testing.T) {
	const id = "01926b90-7a5a-7c4e-8d3f-000000000003"
	ids, err := resolveTaskListIDs(context.Background(), ExportRequest{
		TaskListIDs: []string{id},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 1 || ids[0] != id {
		t.Fatalf("expected UUID string to be preserved, got %#v", ids)
	}
}

func TestValidateDBOnlyExportRequestRejectsUnsupportedResources(t *testing.T) {
	err := validateDBOnlyExportRequest(ExportRequest{
		ProfileSlugs:     []string{"default"},
		SkillSlugs:       []string{"writer"},
		AllowlistSlugs:   []string{"safe"},
		JobIDs:           []string{"job-1"},
		ChannelNames:     []string{"telegram"},
		IncludeContacts:  true,
		IncludeWorkspace: true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{
		"profiles",
		"skills",
		"allowlists",
		"jobs",
		"channels",
		"contacts",
		"workspace",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing unsupported resource %q", err.Error(), want)
		}
	}
}

func TestValidateDBOnlyExportRequestAllowsSupportedDBOnlyResources(t *testing.T) {
	err := validateDBOnlyExportRequest(ExportRequest{
		ExplicitSelection:        true,
		ConversationIDs:          []string{"01926b90-7a5a-7c4e-8d3f-000000000001"},
		ProviderIDs:              []string{"01926b90-7a5a-7c4e-8d3f-000000000002"},
		MCPServerSlugs:           []string{"github"},
		TaskListIDs:              []string{"01926b90-7a5a-7c4e-8d3f-000000000003"},
		IncludeCredentials:       true,
		CredentialExportPassword: "segredo",
		IncludeAudio:             true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
