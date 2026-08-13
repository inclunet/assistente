package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"assistente/internal/app"
	"assistente/internal/database"
	"assistente/internal/llm"
	mcpmgr "assistente/internal/mcp"
	"assistente/internal/portability"
)

type mockDataBackend struct {
	exportOutput      string
	exportErr         error
	exportedToFile    string
	exportToFileErr   error
	analysis          *app.ImportAnalysis
	analysisErr       error
	importResult      *app.ImportResult
	importErr         error
	lastExportReq     app.ExportRequest
	lastExportFileReq app.ExportRequest
	lastAnalyzeJSON   string
	lastAnalyzePwd    string
	lastImportJSON    string
	lastImportPwd     string
	conversations     []app.Conversation
	conversationsErr  error
	providers         []*llm.ProviderConfig
	taskLists         []app.TaskList
	taskListsErr      error
}

func (m *mockDataBackend) ExportData(req app.ExportRequest) (string, error) {
	m.lastExportReq = req
	return m.exportOutput, m.exportErr
}

func (m *mockDataBackend) ExportDataToFile(req app.ExportRequest, path string) (string, error) {
	m.lastExportFileReq = req
	m.exportedToFile = path
	if m.exportToFileErr != nil {
		return "", m.exportToFileErr
	}
	return path, nil
}

func (m *mockDataBackend) AnalyzeImportData(jsonData string, credentialExportPassword string) (*app.ImportAnalysis, error) {
	m.lastAnalyzeJSON = jsonData
	m.lastAnalyzePwd = credentialExportPassword
	return m.analysis, m.analysisErr
}

func (m *mockDataBackend) ImportData(jsonData string, credentialExportPassword string) (*app.ImportResult, error) {
	m.lastImportJSON = jsonData
	m.lastImportPwd = credentialExportPassword
	return m.importResult, m.importErr
}

func (m *mockDataBackend) GetConversations() ([]app.Conversation, error) {
	return m.conversations, m.conversationsErr
}

func (m *mockDataBackend) GetLLMProviders() []*llm.ProviderConfig {
	return m.providers
}

func (m *mockDataBackend) GetAllTaskLists() ([]app.TaskList, error) {
	return m.taskLists, m.taskListsErr
}

func (m *mockDataBackend) ListMCPServers() []mcpmgr.ServerInfo {
	return []mcpmgr.ServerInfo{{Slug: "github"}, {Slug: "filesystem"}}
}

func TestRunDataExport_Stdout(t *testing.T) {
	mock := &mockDataBackend{
		exportOutput: `{"version":2}`,
	}

	var out bytes.Buffer
	err := runDataExport(mock, &out, app.ExportRequest{
		OutputFormat:       portability.FormatJSON,
		ConversationIDs:    []string{"12"},
		IncludeCredentials: true,
		IncludeAudio:       true,
	}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out.String() != `{"version":2}` {
		t.Fatalf("unexpected output: %q", out.String())
	}
	if mock.lastExportReq.ConversationIDs[0] != "12" {
		t.Fatalf("expected conversation id 12, got %#v", mock.lastExportReq.ConversationIDs)
	}
	if !mock.lastExportReq.IncludeCredentials {
		t.Fatal("expected credentials export flag to be forwarded")
	}
	if !mock.lastExportReq.IncludeAudio {
		t.Fatal("expected audio export flag to be forwarded")
	}
}

func TestDataExportIncludeAudioHelpIsExplicit(t *testing.T) {
	flag := dataExportCmd.Flags().Lookup("include-audio")
	if flag == nil {
		t.Fatal("expected include-audio flag to exist")
	}
	if !strings.Contains(flag.Usage, "conteúdo de áudio") {
		t.Fatalf("include-audio help should mention audio content, got %q", flag.Usage)
	}
	if !strings.Contains(flag.Usage, "audioMimeType") {
		t.Fatalf("include-audio help should mention audioMimeType, got %q", flag.Usage)
	}
}

func TestRunDataExport_ToFile(t *testing.T) {
	mock := &mockDataBackend{}

	var out bytes.Buffer
	err := runDataExport(mock, &out, app.ExportRequest{
		OutputFormat:    portability.FormatPDF,
		ConversationIDs: []string{"7"},
	}, "backup.pdf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.exportedToFile != "backup.pdf" {
		t.Fatalf("expected file path backup.pdf, got %q", mock.exportedToFile)
	}
	if mock.lastExportFileReq.OutputFormat != portability.FormatPDF {
		t.Fatalf("expected pdf format, got %q", mock.lastExportFileReq.OutputFormat)
	}
	if !strings.Contains(out.String(), "backup.pdf") {
		t.Fatalf("expected confirmation message, got %q", out.String())
	}
}

func TestRunDataAnalyze_PrintsConflictsAndWarnings(t *testing.T) {
	mock := &mockDataBackend{
		analysis: &app.ImportAnalysis{
			Version:                    portability.ExportVersion,
			AppVersion:                 "dev",
			ConversationCount:          2,
			MessageCount:               8,
			ProviderCount:              1,
			TaskListCount:              1,
			TaskCount:                  3,
			TaskNoteCount:              1,
			IncludesCredentials:        true,
			RequiresCredentialPassword: true,
			CredentialCount:            1,
			ConflictCount:              2,
			ProviderConflicts: []portability.ImportConflict{
				{
					ResourceType:        "provider",
					Identifier:          "openai-custom",
					Reason:              portability.LocalizedMessage{Message: "provider já existe"},
					SupportedStrategies: []portability.ConflictResolutionStrategy{portability.ConflictResolutionSkip, portability.ConflictResolutionRename},
				},
			},
			CredentialConflicts: []portability.ImportConflict{
				{
					ResourceType:        "credential",
					Identifier:          "api.openai.com",
					Reason:              portability.LocalizedMessage{Code: portability.CodeConflictCredentialPattern, Message: "credencial já existe"},
					SupportedStrategies: []portability.ConflictResolutionStrategy{portability.ConflictResolutionSkip, portability.ConflictResolutionOverwrite},
				},
			},
			Warnings:                 []portability.LocalizedMessage{{Message: "arquivo contém metadados extras"}},
			UnsupportedResourceTypes: []string{"channels"},
		},
	}

	readFile := func(path string) ([]byte, error) {
		if path != "import.json" {
			t.Fatalf("unexpected path: %s", path)
		}
		return []byte(`{"version":2}`), nil
	}

	var out bytes.Buffer
	err := runDataAnalyze(mock, &out, readFile, "import.json", "segredo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if mock.lastAnalyzeJSON != `{"version":2}` {
		t.Fatalf("unexpected analyze payload: %q", mock.lastAnalyzeJSON)
	}
	if mock.lastAnalyzePwd != "segredo" {
		t.Fatalf("unexpected analyze password: %q", mock.lastAnalyzePwd)
	}
	if !strings.Contains(output, "Conflitos de providers") {
		t.Fatalf("expected provider conflicts section, got %q", output)
	}
	if !strings.Contains(output, "skip, rename") {
		t.Fatalf("expected supported strategies in output, got %q", output)
	}
	if !strings.Contains(output, "Recursos fora do escopo atual: channels") {
		t.Fatalf("expected unsupported resource warning, got %q", output)
	}
	if !strings.Contains(output, "Aviso: arquivo contém metadados extras") {
		t.Fatalf("expected warning, got %q", output)
	}
}

func TestRunDataImport_ForwardsPayload(t *testing.T) {
	mock := &mockDataBackend{
		importResult: &app.ImportResult{
			Success:  true,
			Imported: 4,
			Skipped:  1,
			Message:  "Importação concluída.",
		},
	}

	readFile := func(path string) ([]byte, error) {
		if path != "backup.json" {
			t.Fatalf("unexpected path: %s", path)
		}
		return []byte(`{"version":2}`), nil
	}

	var out bytes.Buffer
	err := runDataImport(mock, &out, readFile, "backup.json", "segredo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.lastImportJSON != `{"version":2}` {
		t.Fatalf("unexpected import payload: %q", mock.lastImportJSON)
	}
	if mock.lastImportPwd != "segredo" {
		t.Fatalf("unexpected import password: %q", mock.lastImportPwd)
	}
	if !strings.Contains(out.String(), "Importação concluída.") {
		t.Fatalf("expected import message in output, got %q", out.String())
	}
}

func TestRunDataImport_ReturnsErrorWhenImportFails(t *testing.T) {
	mock := &mockDataBackend{
		importResult: &app.ImportResult{
			Success: false,
			Failed:  1,
			Errors:  []portability.LocalizedMessage{{Code: "provider.missingBaseUrl", Message: "provider inválido"}},
		},
	}

	readFile := func(path string) ([]byte, error) {
		return []byte(`{"version":2}`), nil
	}

	var out bytes.Buffer
	err := runDataImport(mock, &out, readFile, "backup.json", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "importação concluída com falhas") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "Erro: provider inválido") {
		t.Fatalf("expected import error details in output, got %q", out.String())
	}
}

func TestRunDataAnalyze_ReadError(t *testing.T) {
	mock := &mockDataBackend{}
	readFile := func(path string) ([]byte, error) {
		return nil, fmt.Errorf("arquivo ausente")
	}

	var out bytes.Buffer
	err := runDataAnalyze(mock, &out, readFile, "missing.json", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "erro ao ler arquivo de importação") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepareDataExportRequest_ExpandsTypeSelections(t *testing.T) {
	mock := &mockDataBackend{
		conversations: []app.Conversation{
			{UUIDModel: database.UUIDModel{ID: "12"}},
			{UUIDModel: database.UUIDModel{ID: "34"}},
		},
		providers: []*llm.ProviderConfig{
			{ID: "openai-custom"},
			{ID: "anthropic-main"},
		},
		taskLists: []app.TaskList{
			{UUIDModel: database.UUIDModel{ID: "7"}},
		},
	}

	req, err := prepareDataExportRequest(mock, app.ExportRequest{}, dataExportSelection{
		Conversations: true,
		Providers:     true,
		TaskLists:     true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !req.ExplicitSelection {
		t.Fatal("expected explicit selection to be enabled")
	}
	if strings.Join(req.ConversationIDs, ",") != "12,34" {
		t.Fatalf("unexpected conversation ids: %#v", req.ConversationIDs)
	}
	if strings.Join(req.ProviderIDs, ",") != "openai-custom,anthropic-main" {
		t.Fatalf("unexpected provider ids: %#v", req.ProviderIDs)
	}
	if strings.Join(req.TaskListIDs, ",") != "7" {
		t.Fatalf("unexpected task list ids: %#v", req.TaskListIDs)
	}
}

func TestPrepareDataExportRequest_OnlyCredentials(t *testing.T) {
	req, err := prepareDataExportRequest(&mockDataBackend{}, app.ExportRequest{}, dataExportSelection{
		CredentialsOnly: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !req.ExplicitSelection {
		t.Fatal("expected explicit selection to be enabled")
	}
	if !req.IncludeCredentials {
		t.Fatal("expected credentials export to be enabled")
	}
	if len(req.ConversationIDs) != 0 || len(req.ProviderIDs) != 0 || len(req.TaskListIDs) != 0 {
		t.Fatalf("expected credentials-only export without resource ids, got %#v", req)
	}
}

func TestPrepareDataExportRequest_RejectsConflictingSelections(t *testing.T) {
	_, err := prepareDataExportRequest(&mockDataBackend{}, app.ExportRequest{All: true}, dataExportSelection{
		Providers: true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--all não pode ser combinado") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepareDataExportRequest_MergesSpecificAndExpandedIDs(t *testing.T) {
	mock := &mockDataBackend{
		providers: []*llm.ProviderConfig{
			{ID: "openai-custom"},
			{ID: "anthropic-main"},
		},
	}

	req, err := prepareDataExportRequest(mock, app.ExportRequest{
		ProviderIDs: []string{"anthropic-main", "manual-provider"},
	}, dataExportSelection{
		Providers: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Join(req.ProviderIDs, ",") != "anthropic-main,manual-provider,openai-custom" {
		t.Fatalf("unexpected merged provider ids: %#v", req.ProviderIDs)
	}
}
