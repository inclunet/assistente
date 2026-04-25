package portability

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"assistente/internal/credentials"
	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type memoryCredentialStore struct {
	credentials map[string]credentials.StoredCredential
}

type trackingCredentialStore struct {
	*memoryCredentialStore
	lastCtx context.Context
}

func newMemoryCredentialStore() *memoryCredentialStore {
	return &memoryCredentialStore{
		credentials: make(map[string]credentials.StoredCredential),
	}
}

func newTrackingCredentialStore() *trackingCredentialStore {
	return &trackingCredentialStore{
		memoryCredentialStore: newMemoryCredentialStore(),
	}
}

func (s *memoryCredentialStore) SaveCredential(_ context.Context, cred credentials.StoredCredential) error {
	s.credentials[cred.Pattern] = cred
	return nil
}

func (s *trackingCredentialStore) SaveCredential(ctx context.Context, cred credentials.StoredCredential) error {
	s.lastCtx = ctx
	return s.memoryCredentialStore.SaveCredential(ctx, cred)
}

func (s *memoryCredentialStore) ListCredentials(context.Context) ([]credentials.StoredCredential, error) {
	result := make([]credentials.StoredCredential, 0, len(s.credentials))
	for _, cred := range s.credentials {
		result = append(result, cred)
	}
	return result, nil
}

func (s *memoryCredentialStore) DeleteCredential(_ context.Context, pattern string) error {
	delete(s.credentials, pattern)
	return nil
}

func (s *memoryCredentialStore) SaveKeyWrap(context.Context, credentials.KeyWrap) error {
	return nil
}

func (s *memoryCredentialStore) GetKeyWrap(context.Context, string) (*credentials.KeyWrap, error) {
	return nil, nil
}

func (s *memoryCredentialStore) HasKeyWrap(context.Context, string) (bool, error) {
	return false, nil
}

func timePtr(v time.Time) *time.Time {
	return &v
}

func TestExportConversationUsesIndexesInsteadOfIDs(t *testing.T) {
	parentID := uint(10)
	turnID := uint(10)
	assistantID := uint(20)

	conv := &database.Conversation{
		Title: "Teste",
		Messages: []database.ChatMessage{
			{ID: parentID, Role: "user", Content: "Oi", CreatedAt: time.Unix(100, 0)},
			{ID: assistantID, Role: "assistant", Content: "Ola", ParentID: &parentID, TurnID: &turnID, CreatedAt: time.Unix(101, 0)},
		},
	}

	exported := exportConversation(conv, false)
	if len(exported.Messages) != 2 {
		t.Fatalf("len(Messages) = %d, want 2", len(exported.Messages))
	}
	if exported.Messages[0].ParentIndex != nil {
		t.Fatalf("root ParentIndex = %v, want nil", *exported.Messages[0].ParentIndex)
	}
	if exported.Messages[1].ParentIndex == nil || *exported.Messages[1].ParentIndex != 0 {
		t.Fatalf("assistant ParentIndex = %v, want 0", exported.Messages[1].ParentIndex)
	}
	if exported.Messages[1].TurnIndex == nil || *exported.Messages[1].TurnIndex != 0 {
		t.Fatalf("assistant TurnIndex = %v, want 0", exported.Messages[1].TurnIndex)
	}
}

func TestExportConversationOmitsAudioByDefault(t *testing.T) {
	conv := &database.Conversation{
		Title: "Audio",
		Messages: []database.ChatMessage{
			{ID: 1, Role: "assistant", Content: "fala", Audio: "base64-audio", AudioMimeType: "audio/mpeg"},
		},
	}

	exported := exportConversation(conv, false)
	if exported.Messages[0].Audio != "" {
		t.Fatalf("Audio = %q, want empty", exported.Messages[0].Audio)
	}
	if exported.Messages[0].AudioMimeType != "audio/mpeg" {
		t.Fatalf("AudioMimeType = %q, want audio/mpeg", exported.Messages[0].AudioMimeType)
	}
}

func setupPortabilityTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("falha ao criar banco em memória: %v", err)
	}
	if err := db.AutoMigrate(
		&database.LLMProvider{},
		&database.Conversation{},
		&database.ChatMessage{},
		&database.TaskListWorkflow{},
		&database.TaskList{},
		&database.Task{},
		&database.TaskNote{},
	); err != nil {
		t.Fatalf("falha ao migrar tabelas: %v", err)
	}
	database.SetDB(db)
}

func createPortableProviderFixture(t *testing.T) *database.LLMProvider {
	t.Helper()

	provider := &database.LLMProvider{
		ID:                "openai-custom",
		Name:              "OpenAI Custom",
		Type:              "openai",
		APIFormat:         "openai",
		BaseURL:           "https://api.openai.example/v1",
		Model:             "gpt-4.1",
		DefaultModel:      "gpt-4.1-mini",
		IsDefault:         true,
		Timeout:           45,
		CredentialPattern: "api.openai.example",
		CreatedAt:         time.Date(2025, 4, 2, 9, 0, 0, 0, time.UTC),
		UpdatedAt:         time.Date(2025, 4, 2, 9, 0, 0, 0, time.UTC),
	}
	if err := database.SaveLLMProvider(provider); err != nil {
		t.Fatalf("SaveLLMProvider() error = %v", err)
	}
	return provider
}

func createPortableTaskListFixture(t *testing.T) *database.TaskList {
	t.Helper()

	taskList, err := database.CreateTaskList("Sprint 42", "Implementar portability", nil, "sprint-42")
	if err != nil {
		t.Fatalf("CreateTaskList() error = %v", err)
	}
	if err := database.SetTaskListViewMode(taskList.ID, "kanban"); err != nil {
		t.Fatalf("SetTaskListViewMode() error = %v", err)
	}

	policy := `{"task_code_regex":"^TASK-[0-9]+$","allowed_note_sources":["jira"]}`
	if err := database.SetTaskListValidationPolicy(taskList.ID, policy); err != nil {
		t.Fatalf("SetTaskListValidationPolicy() error = %v", err)
	}

	root, err := database.CreateTaskFull(
		taskList.ID,
		"Exportar tasklists",
		"Fechar export/import canônico",
		"TASK-1",
		"https://example.invalid/tasks/1",
		"Leonardo",
		"leo",
		"Assistente",
		"agent",
		nil,
	)
	if err != nil {
		t.Fatalf("CreateTaskFull(root) error = %v", err)
	}
	if err := database.UpdateTaskStatus(root.ID, 2); err != nil {
		t.Fatalf("UpdateTaskStatus(root) error = %v", err)
	}

	child, err := database.CreateTaskFull(
		taskList.ID,
		"Persistir notas",
		"Importar notas externas também",
		"TASK-2",
		"",
		"",
		"",
		"",
		"",
		&root.ID,
	)
	if err != nil {
		t.Fatalf("CreateTaskFull(child) error = %v", err)
	}

	note, err := database.CreateTaskNote(root.ID, database.TaskNoteAgent, "Primeira nota", "Assistente", "agent")
	if err != nil {
		t.Fatalf("CreateTaskNote() error = %v", err)
	}

	taskListCreatedAt := time.Date(2025, 4, 1, 10, 0, 0, 0, time.UTC)
	rootCreatedAt := taskListCreatedAt.Add(2 * time.Hour)
	childCreatedAt := rootCreatedAt.Add(30 * time.Minute)
	noteCreatedAt := rootCreatedAt.Add(15 * time.Minute)
	noteExternalUpdatedAt := noteCreatedAt.Add(10 * time.Minute)

	if err := database.DB().Model(&database.TaskList{}).Where("id = ?", taskList.ID).Updates(map[string]interface{}{
		"created_at":          taskListCreatedAt,
		"updated_at":          taskListCreatedAt,
		"validation_policy":   policy,
		"preferred_view_mode": "kanban",
	}).Error; err != nil {
		t.Fatalf("update tasklist fixture timestamps error = %v", err)
	}
	if err := database.DB().Model(&database.TaskListWorkflow{}).Where("task_list_id = ?", taskList.ID).Updates(map[string]interface{}{
		"created_at": taskListCreatedAt,
		"updated_at": taskListCreatedAt,
	}).Error; err != nil {
		t.Fatalf("update workflow fixture timestamps error = %v", err)
	}
	if err := database.DB().Model(&database.Task{}).Where("id = ?", root.ID).Updates(map[string]interface{}{
		"created_at":   rootCreatedAt,
		"updated_at":   rootCreatedAt,
		"completed_at": nil,
	}).Error; err != nil {
		t.Fatalf("update root task timestamps error = %v", err)
	}
	if err := database.DB().Model(&database.Task{}).Where("id = ?", child.ID).Updates(map[string]interface{}{
		"created_at": childCreatedAt,
		"updated_at": childCreatedAt,
	}).Error; err != nil {
		t.Fatalf("update child task timestamps error = %v", err)
	}
	if err := database.DB().Model(&database.TaskNote{}).Where("id = ?", note.ID).Updates(map[string]interface{}{
		"created_at":          noteCreatedAt,
		"updated_at":          noteCreatedAt,
		"external_source":     "jira",
		"external_id":         "NOTE-1",
		"external_parent_id":  "TASK-1",
		"external_updated_at": noteExternalUpdatedAt,
	}).Error; err != nil {
		t.Fatalf("update task note fixture error = %v", err)
	}

	out, err := database.GetTaskList(taskList.ID)
	if err != nil {
		t.Fatalf("GetTaskList() error = %v", err)
	}
	return out
}

func TestAnalyzeImportDataDetectsConversationAndCredentialConflicts(t *testing.T) {
	setupPortabilityTestDB(t)

	existingCreatedAt := time.Date(2025, 4, 24, 10, 0, 0, 0, time.UTC)
	existingConv := &database.Conversation{
		Title:     "Conversa importada",
		Channel:   "telegram",
		CreatedAt: existingCreatedAt,
		UpdatedAt: existingCreatedAt,
	}
	if err := database.DB().Create(existingConv).Error; err != nil {
		t.Fatalf("falha ao criar conversa existente: %v", err)
	}

	credMgr := credentials.NewManager([]byte("test-key-exactly-32-bytes-long!!"))
	if err := credMgr.RegisterPatternWithContext(t.Context(), "api.openai.com", &credentials.AuthConfig{
		Type:  "bearer",
		Token: "secret",
	}); err != nil {
		t.Fatalf("falha ao registrar credencial existente: %v", err)
	}

	file := &ExportFile{
		Version:    1,
		ExportedAt: time.Now().UTC(),
		Options: ExportOptions{
			IncludeCredentials: true,
		},
		Resources: ExportResources{
			Conversations: []ConversationExport{
				{
					Title:     "Conversa importada",
					Channel:   "telegram",
					CreatedAt: existingCreatedAt,
					Messages: []MessageExport{
						{Role: "user", Content: "Oi", CreatedAt: existingCreatedAt},
					},
				},
			},
		},
	}

	blob, err := EncryptCredentialsPayload("senha-teste", []CredentialExport{
		{Pattern: "api.openai.com", AuthType: "bearer", Token: "secret"},
	})
	if err != nil {
		t.Fatalf("falha ao criptografar credenciais de teste: %v", err)
	}
	file.Resources.Credentials = blob

	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("falha ao serializar export file: %v", err)
	}

	analysis, err := AnalyzeImportData(string(raw), credMgr, "senha-teste")
	if err != nil {
		t.Fatalf("AnalyzeImportData() error = %v", err)
	}

	if analysis.ConversationCount != 1 || analysis.MessageCount != 1 {
		t.Fatalf("counts inesperados: %+v", analysis)
	}
	if analysis.ConflictCount != 2 {
		t.Fatalf("ConflictCount = %d, want 2", analysis.ConflictCount)
	}
	if len(analysis.ConversationConflicts) != 1 {
		t.Fatalf("conversation conflicts = %d, want 1", len(analysis.ConversationConflicts))
	}
	if len(analysis.ConversationConflicts[0].SupportedStrategies) != 3 {
		t.Fatalf("ConversationConflicts[0].SupportedStrategies = %v, want 3 opções", analysis.ConversationConflicts[0].SupportedStrategies)
	}
	if len(analysis.CredentialConflicts) != 1 {
		t.Fatalf("credential conflicts = %d, want 1", len(analysis.CredentialConflicts))
	}
}

func TestImportConversationRestoresCreatedAt(t *testing.T) {
	setupPortabilityTestDB(t)

	createdAt := time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)
	imported, err := importConversation(ConversationExport{
		Title:     "Conversa antiga",
		CreatedAt: createdAt,
		Messages: []MessageExport{
			{Role: "user", Content: "Oi", CreatedAt: createdAt},
		},
	}, false)
	if err != nil {
		t.Fatalf("importConversation() error = %v", err)
	}
	if !imported {
		t.Fatal("importConversation() = false, want true")
	}

	conversations, err := database.GetConversations()
	if err != nil {
		t.Fatalf("GetConversations() error = %v", err)
	}
	if len(conversations) != 1 {
		t.Fatalf("len(conversations) = %d, want 1", len(conversations))
	}
	if !conversations[0].CreatedAt.Equal(createdAt) {
		t.Fatalf("CreatedAt = %s, want %s", conversations[0].CreatedAt, createdAt)
	}

	conv, err := database.GetConversation(conversations[0].ID)
	if err != nil {
		t.Fatalf("GetConversation() error = %v", err)
	}
	if len(conv.Messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(conv.Messages))
	}
	if !conv.Messages[0].CreatedAt.Equal(createdAt) {
		t.Fatalf("message CreatedAt = %s, want %s", conv.Messages[0].CreatedAt, createdAt)
	}
}

func TestImportConversationRollsBackOnInvalidMessageReference(t *testing.T) {
	setupPortabilityTestDB(t)

	_, err := importConversation(ConversationExport{
		Title:     "Conversa inválida",
		CreatedAt: time.Now().UTC(),
		Messages: []MessageExport{
			{Role: "user", Content: "Oi", CreatedAt: time.Now().UTC()},
			{Role: "assistant", Content: "Resposta", ParentIndex: intPtr(99), CreatedAt: time.Now().UTC()},
		},
	}, false)
	if err == nil {
		t.Fatal("importConversation() error = nil, want invalid reference error")
	}

	conversations, err := database.GetConversations()
	if err != nil {
		t.Fatalf("GetConversations() error = %v", err)
	}
	if len(conversations) != 0 {
		t.Fatalf("len(conversations) = %d, want 0 after rollback", len(conversations))
	}
}

func TestAnalyzeImportDataWarnsAboutEmptyConversations(t *testing.T) {
	setupPortabilityTestDB(t)

	file := &ExportFile{
		Version:    1,
		ExportedAt: time.Now().UTC(),
		Resources: ExportResources{
			Conversations: []ConversationExport{
				{Title: "Vazia", CreatedAt: time.Now().UTC()},
				{
					Title:     "Com mensagens",
					CreatedAt: time.Now().UTC(),
					Messages: []MessageExport{
						{Role: "user", Content: "Oi", CreatedAt: time.Now().UTC()},
					},
				},
			},
		},
	}

	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("falha ao serializar export file: %v", err)
	}

	analysis, err := AnalyzeImportData(string(raw), nil, "")
	if err != nil {
		t.Fatalf("AnalyzeImportData() error = %v", err)
	}
	if len(analysis.Warnings) == 0 {
		t.Fatal("expected warning about empty conversations")
	}
	if analysis.Warnings[0] != "1 conversa(s) vazia(s) serão descartadas na importação." {
		t.Fatalf("unexpected warning: %q", analysis.Warnings[0])
	}
}

func TestAnalyzeImportDataReportsUnsupportedResourceTypes(t *testing.T) {
	setupPortabilityTestDB(t)

	raw := `{
		"version": 1,
		"resources": {
			"conversations": [],
			"profiles": [{"slug":"perfil-demo"}],
			"taskLists": [{"title":"Sprint 42"}],
			"credentials": null
		}
	}`

	analysis, err := AnalyzeImportData(raw, nil, "")
	if err != nil {
		t.Fatalf("AnalyzeImportData() error = %v", err)
	}

	if len(analysis.UnsupportedResourceTypes) != 1 {
		t.Fatalf("unsupported resource types = %v, want 1 entry", analysis.UnsupportedResourceTypes)
	}
	if analysis.UnsupportedResourceTypes[0] != "profiles" {
		t.Fatalf("unexpected unsupported resource types: %v", analysis.UnsupportedResourceTypes)
	}
}

func TestBuildExportFileIncludesTaskLists(t *testing.T) {
	setupPortabilityTestDB(t)

	taskList := createPortableTaskListFixture(t)

	file, err := BuildExportFile(nil, nil, []uint{taskList.ID}, nil, ExportRequest{}, "test")
	if err != nil {
		t.Fatalf("BuildExportFile() error = %v", err)
	}

	if len(file.Resources.TaskLists) != 1 {
		t.Fatalf("len(taskLists) = %d, want 1", len(file.Resources.TaskLists))
	}
	exported := file.Resources.TaskLists[0]
	if exported.Slug != "sprint-42" {
		t.Fatalf("Slug = %q, want sprint-42", exported.Slug)
	}
	if exported.PreferredViewMode != "kanban" {
		t.Fatalf("PreferredViewMode = %q, want kanban", exported.PreferredViewMode)
	}
	if exported.ValidationPolicy == "" {
		t.Fatal("ValidationPolicy vazio, want policy JSON")
	}
	if len(exported.Workflow.Statuses) != 3 {
		t.Fatalf("len(workflow.statuses) = %d, want 3", len(exported.Workflow.Statuses))
	}
	if len(exported.Tasks) != 1 {
		t.Fatalf("len(root tasks) = %d, want 1", len(exported.Tasks))
	}
	root := exported.Tasks[0]
	if root.Code != "TASK-1" || root.StatusID != 2 {
		t.Fatalf("unexpected root task export: %+v", root)
	}
	if len(root.Children) != 1 {
		t.Fatalf("len(children) = %d, want 1", len(root.Children))
	}
	if len(root.Notes) != 1 {
		t.Fatalf("len(notes) = %d, want 1", len(root.Notes))
	}
	if root.Notes[0].Source != "jira" || root.Notes[0].ExternalID != "NOTE-1" {
		t.Fatalf("unexpected note export: %+v", root.Notes[0])
	}
}

func TestBuildExportFileIncludesProviders(t *testing.T) {
	setupPortabilityTestDB(t)

	provider := createPortableProviderFixture(t)

	file, err := BuildExportFile(nil, []string{provider.ID}, nil, nil, ExportRequest{}, "test")
	if err != nil {
		t.Fatalf("BuildExportFile() error = %v", err)
	}

	if len(file.Resources.Providers) != 1 {
		t.Fatalf("len(providers) = %d, want 1", len(file.Resources.Providers))
	}
	exported := file.Resources.Providers[0]
	if exported.ID != provider.ID || exported.CredentialPattern != provider.CredentialPattern {
		t.Fatalf("unexpected provider export: %+v", exported)
	}
	if !exported.CreatedAt.Equal(provider.CreatedAt) {
		t.Fatalf("CreatedAt = %s, want %s", exported.CreatedAt, provider.CreatedAt)
	}
}

func TestAnalyzeImportDataDetectsProviderConflicts(t *testing.T) {
	setupPortabilityTestDB(t)

	provider := createPortableProviderFixture(t)

	file, err := BuildExportFile(nil, []string{provider.ID}, nil, nil, ExportRequest{}, "test")
	if err != nil {
		t.Fatalf("BuildExportFile() error = %v", err)
	}
	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	analysis, err := AnalyzeImportData(string(raw), nil, "")
	if err != nil {
		t.Fatalf("AnalyzeImportData() error = %v", err)
	}

	if analysis.ProviderCount != 1 {
		t.Fatalf("ProviderCount = %d, want 1", analysis.ProviderCount)
	}
	if len(analysis.ProviderConflicts) != 1 {
		t.Fatalf("len(ProviderConflicts) = %d, want 1", len(analysis.ProviderConflicts))
	}
	if analysis.ProviderConflicts[0].Identifier != provider.ID {
		t.Fatalf("unexpected provider conflict: %+v", analysis.ProviderConflicts[0])
	}
	if len(analysis.ProviderConflicts[0].SupportedStrategies) != 3 {
		t.Fatalf("ProviderConflicts[0].SupportedStrategies = %v, want 3 opções", analysis.ProviderConflicts[0].SupportedStrategies)
	}
}

func TestAnalyzeImportDataDetectsTaskListConflicts(t *testing.T) {
	setupPortabilityTestDB(t)

	taskList := createPortableTaskListFixture(t)

	file, err := BuildExportFile(nil, nil, []uint{taskList.ID}, nil, ExportRequest{}, "test")
	if err != nil {
		t.Fatalf("BuildExportFile() error = %v", err)
	}
	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	analysis, err := AnalyzeImportData(string(raw), nil, "")
	if err != nil {
		t.Fatalf("AnalyzeImportData() error = %v", err)
	}

	if analysis.TaskListCount != 1 {
		t.Fatalf("TaskListCount = %d, want 1", analysis.TaskListCount)
	}
	if analysis.TaskCount != 2 || analysis.TaskNoteCount != 1 {
		t.Fatalf("TaskCount/TaskNoteCount = %d/%d, want 2/1", analysis.TaskCount, analysis.TaskNoteCount)
	}
	if len(analysis.TaskListConflicts) != 1 {
		t.Fatalf("len(TaskListConflicts) = %d, want 1", len(analysis.TaskListConflicts))
	}
	if analysis.TaskListConflicts[0].Identifier != "sprint-42" {
		t.Fatalf("unexpected tasklist conflict identifier: %+v", analysis.TaskListConflicts[0])
	}
	if len(analysis.TaskListConflicts[0].SupportedStrategies) != 3 {
		t.Fatalf("TaskListConflicts[0].SupportedStrategies = %v, want 3 opções", analysis.TaskListConflicts[0].SupportedStrategies)
	}
}

func TestAnalyzeImportDataDetectsTaskListConflictsWithNormalizedSlug(t *testing.T) {
	setupPortabilityTestDB(t)

	taskList := createPortableTaskListFixture(t)

	file, err := BuildExportFile(nil, nil, []uint{taskList.ID}, nil, ExportRequest{}, "test")
	if err != nil {
		t.Fatalf("BuildExportFile() error = %v", err)
	}
	file.Resources.TaskLists[0].Slug = "  Sprint-42  "

	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	analysis, err := AnalyzeImportData(string(raw), nil, "")
	if err != nil {
		t.Fatalf("AnalyzeImportData() error = %v", err)
	}

	if len(analysis.TaskListConflicts) != 1 {
		t.Fatalf("len(TaskListConflicts) = %d, want 1", len(analysis.TaskListConflicts))
	}
	if analysis.TaskListConflicts[0].Identifier != "sprint-42" {
		t.Fatalf("unexpected normalized tasklist conflict identifier: %+v", analysis.TaskListConflicts[0])
	}
}

func TestImportConversationsImportsProviders(t *testing.T) {
	setupPortabilityTestDB(t)

	now := time.Now().UTC()
	file := &ExportFile{
		Version:    1,
		ExportedAt: now,
		Resources: ExportResources{
			Providers: []ProviderExport{
				{
					ID:                "ollama-local",
					Name:              "Ollama Local",
					Type:              "ollama",
					APIFormat:         "openai",
					BaseURL:           "http://localhost:11434/v1",
					Model:             "llama3.1",
					DefaultModel:      "llama3.1",
					IsDefault:         true,
					Timeout:           30,
					CredentialPattern: "localhost",
					CreatedAt:         now.Add(-24 * time.Hour),
				},
			},
		},
	}

	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	result, err := ImportConversations(string(raw), nil, "")
	if err != nil {
		t.Fatalf("ImportConversations() error = %v", err)
	}
	if result.Imported != 1 || result.Skipped != 0 || result.Failed != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	imported, err := database.GetLLMProvider("ollama-local")
	if err != nil {
		t.Fatalf("GetLLMProvider() error = %v", err)
	}
	if imported.Name != "Ollama Local" || imported.BaseURL != "http://localhost:11434/v1" {
		t.Fatalf("unexpected imported provider: %+v", imported)
	}
	if !imported.CreatedAt.Equal(now.Add(-24 * time.Hour)) {
		t.Fatalf("CreatedAt = %s, want %s", imported.CreatedAt, now.Add(-24*time.Hour))
	}
}

func TestImportConversationsImportsTaskLists(t *testing.T) {
	setupPortabilityTestDB(t)

	now := time.Now().UTC()
	file := &ExportFile{
		Version:    1,
		ExportedAt: now,
		Resources: ExportResources{
			TaskLists: []TaskListExport{
				{
					Title:             "Sprint 99",
					Slug:              "sprint-99",
					Description:       "Fechar lote pre-migracao",
					PreferredViewMode: "kanban",
					ValidationPolicy:  `{"task_code_regex":"^TASK-[0-9]+$"}`,
					CreatedAt:         now.Add(-48 * time.Hour),
					Workflow: TaskListWorkflowExport{
						Statuses: []TaskListWorkflowStatusExport{
							{ID: 1, Order: 0, Label: "A Fazer", Color: "var(--color-warning)", Icon: "⌛"},
							{ID: 2, Order: 1, Label: "Em Progresso", Color: "var(--color-info)", Icon: "▶️"},
							{ID: 3, Order: 2, Label: "Concluído", Color: "var(--color-success)", Icon: "✅"},
						},
						AllowedTransitions: map[int][]int{
							1: {2, 3},
							2: {1, 3},
							3: {1, 2},
						},
						InitialStatusID: 1,
					},
					Tasks: []TaskExport{
						{
							Title:        "Implementar export",
							Description:  "Levar tasklists para o JSON",
							Code:         "TASK-10",
							StatusID:     2,
							Order:        0,
							AssigneeName: "Leonardo",
							AssigneeID:   "leo",
							CreatorName:  "Assistente",
							CreatorID:    "agent",
							CreatedAt:    now.Add(-47 * time.Hour),
							Notes: []TaskNoteExport{
								{
									Type:              int(database.TaskNoteAgent),
									Content:           "Nota sincronizada",
									AuthorName:        "Assistente",
									AuthorID:          "agent",
									Source:            "jira",
									ExternalID:        "NOTE-99",
									ExternalParentID:  "TASK-10",
									ExternalUpdatedAt: timePtr(now.Add(-46 * time.Hour)),
									CreatedAt:         now.Add(-47 * time.Hour).Add(15 * time.Minute),
								},
							},
							Children: []TaskExport{
								{
									Title:       "Validar import",
									Code:        "TASK-11",
									StatusID:    1,
									Order:       0,
									CreatedAt:   now.Add(-46 * time.Hour),
									CompletedAt: nil,
								},
							},
						},
					},
				},
			},
		},
	}

	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	result, err := ImportConversations(string(raw), nil, "")
	if err != nil {
		t.Fatalf("ImportConversations() error = %v", err)
	}
	if result.Imported != 1 || result.Skipped != 0 || result.Failed != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	taskLists, err := database.GetAllTaskLists()
	if err != nil {
		t.Fatalf("GetAllTaskLists() error = %v", err)
	}
	if len(taskLists) != 1 {
		t.Fatalf("len(taskLists) = %d, want 1", len(taskLists))
	}
	importedTaskList, err := database.GetTaskListWithHierarchy(taskLists[0].ID)
	if err != nil {
		t.Fatalf("GetTaskListWithHierarchy() error = %v", err)
	}
	if importedTaskList.Slug != "sprint-99" {
		t.Fatalf("Slug = %q, want sprint-99", importedTaskList.Slug)
	}
	if importedTaskList.PreferredViewMode != "kanban" {
		t.Fatalf("PreferredViewMode = %q, want kanban", importedTaskList.PreferredViewMode)
	}
	if !importedTaskList.CreatedAt.Equal(now.Add(-48 * time.Hour)) {
		t.Fatalf("CreatedAt = %s, want %s", importedTaskList.CreatedAt, now.Add(-48*time.Hour))
	}
	if len(importedTaskList.Tasks) != 1 || len(importedTaskList.Tasks[0].Subtasks) != 1 {
		t.Fatalf("unexpected imported task hierarchy: %+v", importedTaskList.Tasks)
	}
	if importedTaskList.Tasks[0].StatusID != 2 {
		t.Fatalf("root StatusID = %d, want 2", importedTaskList.Tasks[0].StatusID)
	}
	notes, err := database.GetTaskNotes(importedTaskList.Tasks[0].ID)
	if err != nil {
		t.Fatalf("GetTaskNotes() error = %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("len(notes) = %d, want 1", len(notes))
	}
	if notes[0].ExternalSource != "jira" || notes[0].ExternalID != "NOTE-99" {
		t.Fatalf("unexpected imported note: %+v", notes[0])
	}
}

func TestImportConversationsWithResolutionsOverwritesConversation(t *testing.T) {
	setupPortabilityTestDB(t)

	createdAt := time.Date(2025, 4, 24, 10, 0, 0, 0, time.UTC)
	existing := &database.Conversation{
		Title:     "Conversa importada",
		Channel:   "telegram",
		Summary:   "Resumo antigo",
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}
	if err := database.DB().Create(existing).Error; err != nil {
		t.Fatalf("Create(existing conversation) error = %v", err)
	}
	if err := database.DB().Create(&database.ChatMessage{
		ConversationID: existing.ID,
		Role:           "user",
		Content:        "Mensagem antiga",
		CreatedAt:      createdAt,
	}).Error; err != nil {
		t.Fatalf("Create(existing message) error = %v", err)
	}

	file := &ExportFile{
		Version:    1,
		ExportedAt: time.Now().UTC(),
		Resources: ExportResources{
			Conversations: []ConversationExport{
				{
					Title:     "Conversa importada",
					Channel:   "telegram",
					Summary:   "Resumo novo",
					CreatedAt: createdAt,
					Messages: []MessageExport{
						{Role: "user", Content: "Nova mensagem", CreatedAt: createdAt.Add(1 * time.Minute)},
						{Role: "assistant", Content: "Resposta nova", CreatedAt: createdAt.Add(2 * time.Minute)},
					},
				},
			},
		},
	}

	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	result, err := ImportConversationsWithResolutions(t.Context(), string(raw), nil, "", []ImportResolution{
		{
			ResourceType: "conversation",
			Identifier:   conversationConflictIdentifier(file.Resources.Conversations[0]),
			Strategy:     ConflictResolutionOverwrite,
		},
	})
	if err != nil {
		t.Fatalf("ImportConversationsWithResolutions() error = %v", err)
	}
	if result.Imported != 1 || result.Skipped != 0 || result.Failed != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	conversations, err := database.GetConversations()
	if err != nil {
		t.Fatalf("GetConversations() error = %v", err)
	}
	if len(conversations) != 1 {
		t.Fatalf("len(conversations) = %d, want 1", len(conversations))
	}

	imported, err := database.GetConversation(conversations[0].ID)
	if err != nil {
		t.Fatalf("GetConversation() error = %v", err)
	}
	if imported.Summary != "Resumo novo" {
		t.Fatalf("Summary = %q, want Resumo novo", imported.Summary)
	}
	if len(imported.Messages) != 2 {
		t.Fatalf("len(Messages) = %d, want 2", len(imported.Messages))
	}
	if imported.Messages[0].Content != "Nova mensagem" || imported.Messages[1].Content != "Resposta nova" {
		t.Fatalf("unexpected messages after overwrite: %+v", imported.Messages)
	}
}

func TestImportConversationsWithResolutionsRenamesProvider(t *testing.T) {
	setupPortabilityTestDB(t)

	provider := createPortableProviderFixture(t)
	file := &ExportFile{
		Version:    1,
		ExportedAt: time.Now().UTC(),
		Resources: ExportResources{
			Providers: []ProviderExport{
				{
					ID:                provider.ID,
					Name:              "OpenAI Renamed Copy",
					Type:              provider.Type,
					APIFormat:         provider.APIFormat,
					BaseURL:           "https://renamed.example/v1",
					Model:             "gpt-4.1",
					DefaultModel:      "gpt-4.1",
					IsDefault:         false,
					Timeout:           90,
					CredentialPattern: "api.openai.renamed",
					CreatedAt:         provider.CreatedAt.Add(1 * time.Hour),
				},
			},
		},
	}

	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	result, err := ImportConversationsWithResolutions(t.Context(), string(raw), nil, "", []ImportResolution{
		{
			ResourceType: "provider",
			Identifier:   provider.ID,
			Strategy:     ConflictResolutionRename,
			RenameValue:  "openai-custom-copy",
		},
	})
	if err != nil {
		t.Fatalf("ImportConversationsWithResolutions() error = %v", err)
	}
	if result.Imported != 1 || result.Skipped != 0 || result.Failed != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	providers, err := database.GetLLMProviders()
	if err != nil {
		t.Fatalf("GetLLMProviders() error = %v", err)
	}
	if len(providers) != 2 {
		t.Fatalf("len(providers) = %d, want 2", len(providers))
	}

	renamed, err := database.GetLLMProvider("openai-custom-copy")
	if err != nil {
		t.Fatalf("GetLLMProvider(renamed) error = %v", err)
	}
	if renamed.Name != "OpenAI Renamed Copy" || renamed.BaseURL != "https://renamed.example/v1" {
		t.Fatalf("unexpected renamed provider: %+v", renamed)
	}
}

func TestImportConversationsWithResolutionsOverwritesTaskList(t *testing.T) {
	setupPortabilityTestDB(t)

	taskList := createPortableTaskListFixture(t)
	file := &ExportFile{
		Version:    1,
		ExportedAt: time.Now().UTC(),
		Resources: ExportResources{
			TaskLists: []TaskListExport{
				{
					Title:             "Sprint 42",
					Slug:              "sprint-42",
					Description:       "Workflow substituído",
					PreferredViewMode: "list",
					ValidationPolicy:  `{"task_code_regex":"^NEW-[0-9]+$"}`,
					CreatedAt:         taskList.CreatedAt.Add(2 * time.Hour),
					Workflow: TaskListWorkflowExport{
						Statuses: []TaskListWorkflowStatusExport{
							{ID: 1, Order: 0, Label: "Backlog", Color: "var(--color-warning)", Icon: "B"},
							{ID: 2, Order: 1, Label: "Done", Color: "var(--color-success)", Icon: "D"},
						},
						AllowedTransitions: map[int][]int{
							1: {2},
							2: {1},
						},
						InitialStatusID: 1,
					},
					Tasks: []TaskExport{
						{
							Title:     "Task nova",
							Code:      "NEW-1",
							StatusID:  1,
							Order:     0,
							CreatedAt: taskList.CreatedAt.Add(3 * time.Hour),
						},
					},
				},
			},
		},
	}

	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	result, err := ImportConversationsWithResolutions(t.Context(), string(raw), nil, "", []ImportResolution{
		{
			ResourceType: "taskList",
			Identifier:   "sprint-42",
			Strategy:     ConflictResolutionOverwrite,
		},
	})
	if err != nil {
		t.Fatalf("ImportConversationsWithResolutions() error = %v", err)
	}
	if result.Imported != 1 || result.Skipped != 0 || result.Failed != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	taskLists, err := database.GetAllTaskLists()
	if err != nil {
		t.Fatalf("GetAllTaskLists() error = %v", err)
	}
	if len(taskLists) != 1 {
		t.Fatalf("len(taskLists) = %d, want 1", len(taskLists))
	}

	importedTaskList, err := database.GetTaskListWithHierarchy(taskLists[0].ID)
	if err != nil {
		t.Fatalf("GetTaskListWithHierarchy() error = %v", err)
	}
	if importedTaskList.Description != "Workflow substituído" {
		t.Fatalf("Description = %q, want Workflow substituído", importedTaskList.Description)
	}
	if importedTaskList.PreferredViewMode != "list" {
		t.Fatalf("PreferredViewMode = %q, want list", importedTaskList.PreferredViewMode)
	}
	if len(importedTaskList.Tasks) != 1 || importedTaskList.Tasks[0].Code != "NEW-1" {
		t.Fatalf("unexpected tasks after overwrite: %+v", importedTaskList.Tasks)
	}
}

func TestGetTaskListWithHierarchyPreservesDeepHierarchy(t *testing.T) {
	setupPortabilityTestDB(t)

	taskList, err := database.CreateTaskList("Deep tree", "", nil, "deep-tree")
	if err != nil {
		t.Fatalf("CreateTaskList() error = %v", err)
	}

	root, err := database.CreateTaskFull(taskList.ID, "Root", "", "ROOT-1", "", "", "", "", "", nil)
	if err != nil {
		t.Fatalf("CreateTaskFull(root) error = %v", err)
	}
	child, err := database.CreateTaskFull(taskList.ID, "Child", "", "CHILD-1", "", "", "", "", "", &root.ID)
	if err != nil {
		t.Fatalf("CreateTaskFull(child) error = %v", err)
	}
	_, err = database.CreateTaskFull(taskList.ID, "Grandchild", "", "GRAND-1", "", "", "", "", "", &child.ID)
	if err != nil {
		t.Fatalf("CreateTaskFull(grandchild) error = %v", err)
	}

	hierarchy, err := database.GetTaskListWithHierarchy(taskList.ID)
	if err != nil {
		t.Fatalf("GetTaskListWithHierarchy() error = %v", err)
	}

	if len(hierarchy.Tasks) != 1 {
		t.Fatalf("len(Tasks) = %d, want 1", len(hierarchy.Tasks))
	}
	if len(hierarchy.Tasks[0].Subtasks) != 1 {
		t.Fatalf("len(root.Subtasks) = %d, want 1", len(hierarchy.Tasks[0].Subtasks))
	}
	if len(hierarchy.Tasks[0].Subtasks[0].Subtasks) != 1 {
		t.Fatalf("len(child.Subtasks) = %d, want 1", len(hierarchy.Tasks[0].Subtasks[0].Subtasks))
	}
	if hierarchy.Tasks[0].Subtasks[0].Subtasks[0].Code != "GRAND-1" {
		t.Fatalf("grandchild code = %q, want GRAND-1", hierarchy.Tasks[0].Subtasks[0].Subtasks[0].Code)
	}
}

func TestAnalyzeImportDataRejectsUnsupportedVersion(t *testing.T) {
	setupPortabilityTestDB(t)

	_, err := AnalyzeImportData(`{"version":2,"resources":{"conversations":[]}}`, nil, "")
	if err == nil {
		t.Fatal("AnalyzeImportData() error = nil, want unsupported version error")
	}
	if !strings.Contains(err.Error(), "versão de exportação não suportada: 2") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestImportConversationsSkipsEmptyConversations(t *testing.T) {
	setupPortabilityTestDB(t)

	now := time.Now().UTC()
	file := &ExportFile{
		Version:    1,
		ExportedAt: now,
		Resources: ExportResources{
			Conversations: []ConversationExport{
				{Title: "Vazia", CreatedAt: now},
				{
					Title:     "Com mensagens",
					CreatedAt: now,
					Messages: []MessageExport{
						{Role: "user", Content: "Oi", CreatedAt: now},
					},
				},
			},
		},
	}

	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("falha ao serializar export file: %v", err)
	}

	result, err := ImportConversations(string(raw), nil, "")
	if err != nil {
		t.Fatalf("ImportConversations() error = %v", err)
	}
	if result.Imported != 1 || result.Skipped != 1 {
		t.Fatalf("got imported=%d skipped=%d, want 1/1", result.Imported, result.Skipped)
	}
	if result.SkippedEmptyConversations != 1 {
		t.Fatalf("SkippedEmptyConversations = %d, want 1", result.SkippedEmptyConversations)
	}

	conversations, err := database.GetConversations()
	if err != nil {
		t.Fatalf("GetConversations() error = %v", err)
	}
	if len(conversations) != 1 {
		t.Fatalf("len(conversations) = %d, want 1", len(conversations))
	}
	if conversations[0].Title != "Com mensagens" {
		t.Fatalf("unexpected imported conversation: %q", conversations[0].Title)
	}
}

func TestImportConversationsWarnsAboutUnsupportedResourceTypes(t *testing.T) {
	setupPortabilityTestDB(t)

	now := time.Now().UTC()
	file := &ExportFile{
		Version:    1,
		ExportedAt: now,
		Resources: ExportResources{
			Conversations: []ConversationExport{
				{
					Title:     "Com mensagens",
					CreatedAt: now,
					Messages: []MessageExport{
						{Role: "user", Content: "Oi", CreatedAt: now},
					},
				},
			},
		},
	}

	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("falha ao serializar export file: %v", err)
	}

	rawString := strings.Replace(string(raw), `"resources":{"conversations":[`, `"resources":{"profiles":[{"slug":"perfil-demo"}],"conversations":[`, 1)
	result, err := ImportConversations(rawString, nil, "")
	if err != nil {
		t.Fatalf("ImportConversations() error = %v", err)
	}
	if len(result.UnsupportedResourceTypes) != 1 || result.UnsupportedResourceTypes[0] != "profiles" {
		t.Fatalf("unexpected unsupported resource types: %v", result.UnsupportedResourceTypes)
	}
	if len(result.Warnings) == 0 || !strings.Contains(result.Warnings[0], "fora do escopo atual (profiles)") {
		t.Fatalf("unexpected warnings: %v", result.Warnings)
	}
}

func TestImportConversationsRejectsUnsupportedVersion(t *testing.T) {
	setupPortabilityTestDB(t)

	_, err := ImportConversations(`{"version":2,"resources":{"conversations":[]}}`, nil, "")
	if err == nil {
		t.Fatal("ImportConversations() error = nil, want unsupported version error")
	}
	if !strings.Contains(err.Error(), "versão de exportação não suportada: 2") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestImportConversationsReturnsDetailedSkipBreakdown(t *testing.T) {
	setupPortabilityTestDB(t)

	now := time.Now().UTC()
	existingConv := &database.Conversation{
		Title:     "Duplicada",
		Channel:   "telegram",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := database.DB().Create(existingConv).Error; err != nil {
		t.Fatalf("falha ao criar conversa existente: %v", err)
	}

	credStore := newMemoryCredentialStore()
	credMgr := credentials.NewManagerWithStoreAndPersistence([]byte("test-key-exactly-32-bytes-long!!"), credStore, true)
	if err := credMgr.RegisterPatternWithContext(t.Context(), "api.openai.com", &credentials.AuthConfig{
		Type:  "bearer",
		Token: "secret",
	}); err != nil {
		t.Fatalf("falha ao registrar credencial existente: %v", err)
	}

	file := &ExportFile{
		Version:    1,
		ExportedAt: now,
		Options: ExportOptions{
			IncludeCredentials: true,
		},
		Resources: ExportResources{
			Conversations: []ConversationExport{
				{Title: "Vazia", CreatedAt: now},
				{
					Title:     "Duplicada",
					Channel:   "telegram",
					CreatedAt: now,
					Messages: []MessageExport{
						{Role: "user", Content: "Oi", CreatedAt: now},
					},
				},
				{
					Title:     "Nova",
					CreatedAt: now.Add(time.Second),
					Messages: []MessageExport{
						{Role: "user", Content: "Mensagem", CreatedAt: now.Add(time.Second)},
					},
				},
			},
		},
	}

	blob, err := EncryptCredentialsPayload("senha-teste", []CredentialExport{
		{Pattern: "api.openai.com", AuthType: "bearer", Token: "secret"},
	})
	if err != nil {
		t.Fatalf("falha ao criptografar credenciais de teste: %v", err)
	}
	file.Resources.Credentials = blob

	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("falha ao serializar export file: %v", err)
	}

	result, err := ImportConversations(string(raw), credMgr, "senha-teste")
	if err != nil {
		t.Fatalf("ImportConversations() error = %v", err)
	}

	if result.Imported != 1 {
		t.Fatalf("Imported = %d, want 1", result.Imported)
	}
	if result.Skipped != 3 {
		t.Fatalf("Skipped = %d, want 3", result.Skipped)
	}
	if result.Failed != 0 {
		t.Fatalf("Failed = %d, want 0", result.Failed)
	}
	if result.SkippedEmptyConversations != 1 {
		t.Fatalf("SkippedEmptyConversations = %d, want 1", result.SkippedEmptyConversations)
	}
	if result.SkippedConversationConflict != 1 {
		t.Fatalf("SkippedConversationConflict = %d, want 1", result.SkippedConversationConflict)
	}
	if result.SkippedCredentialConflict != 1 {
		t.Fatalf("SkippedCredentialConflict = %d, want 1", result.SkippedCredentialConflict)
	}
	if result.SkippedOther != 0 {
		t.Fatalf("SkippedOther = %d, want 0", result.SkippedOther)
	}
}

func TestImportConversationsPropagatesContextToCredentialPersistence(t *testing.T) {
	setupPortabilityTestDB(t)

	store := newTrackingCredentialStore()
	credMgr := credentials.NewManagerWithStoreAndPersistence([]byte("test-key-exactly-32-bytes-long!!"), store, true)

	now := time.Now().UTC()
	file := &ExportFile{
		Version:    1,
		ExportedAt: now,
		Options: ExportOptions{
			IncludeCredentials: true,
		},
	}

	blob, err := EncryptCredentialsPayload("senha-teste", []CredentialExport{
		{Pattern: "api.openai.com", AuthType: "bearer", Token: "secret"},
	})
	if err != nil {
		t.Fatalf("falha ao criptografar credenciais de teste: %v", err)
	}
	file.Resources.Credentials = blob

	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("falha ao serializar export file: %v", err)
	}

	type ctxKey string
	ctx := context.WithValue(context.Background(), ctxKey("source"), "test-import")
	_, err = ImportConversationsWithContext(ctx, string(raw), credMgr, "senha-teste")
	if err != nil {
		t.Fatalf("ImportConversationsWithContext() error = %v", err)
	}

	if got := store.lastCtx.Value(ctxKey("source")); got != "test-import" {
		t.Fatalf("credential persistence ctx value = %v, want test-import", got)
	}
}
