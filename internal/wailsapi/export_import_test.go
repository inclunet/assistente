package wailsapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"assistente/internal/apidto"
	"assistente/internal/core/ports"
	"assistente/internal/portability"
)

func TestExportImportNotWired(t *testing.T) {
	t.Parallel()
	api := NewExportImport()
	if _, err := api.ExportConversations(nil); !errors.Is(err, ErrExportImportNotWired) {
		t.Fatalf("ExportConversations: got %v", err)
	}
	if _, err := api.ExportData(portability.ExportRequest{}); !errors.Is(err, ErrExportImportNotWired) {
		t.Fatalf("ExportData: got %v", err)
	}
	if _, err := api.ExportConversationsToFile(nil, portability.FormatHTML, portability.ContentExportOptions{}, apidto.FileDialogLabels{}); !errors.Is(err, ErrExportImportNotWired) {
		t.Fatalf("ExportConversationsToFile: got %v", err)
	}
	if _, err := api.ExportDataToFile(portability.ExportRequest{}, "x"); !errors.Is(err, ErrExportImportNotWired) {
		t.Fatalf("ExportDataToFile: got %v", err)
	}
	if _, err := api.ImportConversations(""); !errors.Is(err, ErrExportImportNotWired) {
		t.Fatalf("ImportConversations: got %v", err)
	}
	if _, err := api.ImportData("", ""); !errors.Is(err, ErrExportImportNotWired) {
		t.Fatalf("ImportData: got %v", err)
	}
	if _, err := api.ImportDataWithResolutions(portability.ImportRequest{}); !errors.Is(err, ErrExportImportNotWired) {
		t.Fatalf("ImportDataWithResolutions: got %v", err)
	}
	if _, err := api.AnalyzeImportData("", ""); !errors.Is(err, ErrExportImportNotWired) {
		t.Fatalf("AnalyzeImportData: got %v", err)
	}
}

func TestExportImportAttachNilDialogStillNotWired(t *testing.T) {
	t.Parallel()
	api := NewExportImport()
	AttachExportImport(api, stubSession{}, nil, nil, "dev")
	if _, err := api.ExportData(portability.ExportRequest{}); !errors.Is(err, ErrExportImportNotWired) {
		t.Fatalf("dialog nil: got %v", err)
	}
}

func TestExportImportUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	semAuth := errors.New("sessão não autenticada")
	api := NewExportImport()
	AttachExportImport(api, stubSession{err: semAuth}, nil, func() ports.SystemDialogPort { return nil }, "dev")

	casos := []struct {
		nome string
		fn   func() error
	}{
		{"ExportConversations", func() error {
			_, err := api.ExportConversations(nil)
			return err
		}},
		{"ExportData", func() error {
			_, err := api.ExportData(portability.ExportRequest{})
			return err
		}},
		{"ExportConversationsToFile", func() error {
			_, err := api.ExportConversationsToFile(nil, portability.FormatHTML, portability.ContentExportOptions{}, apidto.FileDialogLabels{})
			return err
		}},
		{"ExportDataToFile", func() error {
			_, err := api.ExportDataToFile(portability.ExportRequest{}, "x.json")
			return err
		}},
		{"ImportConversations", func() error {
			_, err := api.ImportConversations("")
			return err
		}},
		{"ImportData", func() error {
			_, err := api.ImportData("", "")
			return err
		}},
		{"ImportDataWithResolutions", func() error {
			_, err := api.ImportDataWithResolutions(portability.ImportRequest{})
			return err
		}},
		{"AnalyzeImportData", func() error {
			_, err := api.AnalyzeImportData("", "")
			return err
		}},
	}
	for _, c := range casos {
		c := c
		t.Run(c.nome, func(t *testing.T) {
			t.Parallel()
			if err := c.fn(); !errors.Is(err, semAuth) {
				t.Fatalf("erro = %v, quer o da sessão", err)
			}
		})
	}
}

func TestExportImportUsesWithUserNotRequireAuthSource(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "export_import.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("export_import.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if strings.Contains(body, "importExportContext(") {
		t.Fatal("export_import.go não deve chamar importExportContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(session,") {
		t.Fatal("export_import.go deve chamar WithUser(session,")
	}
	if strings.Contains(body, "BuildConversationExportFile(") && !strings.Contains(body, "BuildConversationExportFileWithContext(") {
		t.Fatal("ExportDataToFile deve usar BuildConversationExportFileWithContext")
	}
	// Garante que o caminho rich (HTML/PDF/MD) usa WithContext, não o helper sem ctx.
	if strings.Contains(body, "BuildConversationExportFile(conversationIDs") {
		t.Fatal("não chame BuildConversationExportFile sem ctx autenticado")
	}
}

func TestNormalizeRichConversationExportRequestRejectsCredentials(t *testing.T) {
	t.Parallel()
	_, err := normalizeRichConversationExportRequest(portability.ExportRequest{
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
	t.Parallel()
	_, err := normalizeRichConversationExportRequest(portability.ExportRequest{
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
	t.Parallel()
	req, err := normalizeRichConversationExportRequest(portability.ExportRequest{
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

func TestRichConversationExportAllowsAllWithoutExplicitUnsupportedSelections(t *testing.T) {
	t.Parallel()
	if hasUnsupportedRichConversationSelections(portability.ExportRequest{
		OutputFormat: portability.FormatHTML,
		All:          true,
	}) {
		t.Fatal("expected All to be handled as all conversations in rich exports")
	}
}

func TestRichConversationExportRejectsExplicitMemorySelections(t *testing.T) {
	t.Parallel()
	if !hasUnsupportedRichConversationSelections(portability.ExportRequest{
		OutputFormat:    portability.FormatHTML,
		MemoryRecordIDs: []string{"mem-1"},
	}) {
		t.Fatal("expected explicit memory selections to be unsupported in rich exports")
	}
}

func TestResolveConversationIDsRespectsExplicitSelection(t *testing.T) {
	t.Parallel()
	ids, err := resolveConversationIDs(context.Background(), portability.ExportRequest{
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
	t.Parallel()
	const id = "01926b90-7a5a-7c4e-8d3f-000000000001"
	ids, err := resolveConversationIDs(context.Background(), portability.ExportRequest{
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
	t.Parallel()
	const id = "01926b90-7a5a-7c4e-8d3f-000000000003"
	ids, err := resolveTaskListIDs(context.Background(), portability.ExportRequest{
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
	t.Parallel()
	err := validateDBOnlyExportRequest(portability.ExportRequest{
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
	t.Parallel()
	err := validateDBOnlyExportRequest(portability.ExportRequest{
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

func TestValidateMCPJSONExportRequestAllowsAllMCPServers(t *testing.T) {
	t.Parallel()
	err := validateMCPJSONExportRequest(portability.ExportRequest{
		OutputFormat: portability.FormatMCPJSON,
		All:          true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateMCPJSONExportRequestRejectsOtherResources(t *testing.T) {
	t.Parallel()
	err := validateMCPJSONExportRequest(portability.ExportRequest{
		OutputFormat:             portability.FormatMCPJSON,
		ConversationIDs:          []string{"01926b90-7a5a-7c4e-8d3f-000000000001"},
		ProfileSlugs:             []string{"default"},
		SkillSlugs:               []string{"writer"},
		AllowlistSlugs:           []string{"safe"},
		JobIDs:                   []string{"job-1"},
		ChannelNames:             []string{"telegram"},
		IncludeContacts:          true,
		IncludeWorkspace:         true,
		IncludeAudio:             true,
		IncludeCredentials:       true,
		CredentialExportPassword: "secret",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "mcp-json suporta apenas servidores MCP") {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"conversations", "profiles", "skills", "allowlists", "jobs", "channels", "contacts", "workspace", "audio", "credentials"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err.Error(), want)
		}
	}
}
