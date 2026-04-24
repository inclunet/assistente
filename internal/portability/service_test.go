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
	if err := db.AutoMigrate(&database.Conversation{}, &database.ChatMessage{}); err != nil {
		t.Fatalf("falha ao migrar tabelas: %v", err)
	}
	database.SetDB(db)
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

	if len(analysis.UnsupportedResourceTypes) != 2 {
		t.Fatalf("unsupported resource types = %v, want 2 entries", analysis.UnsupportedResourceTypes)
	}
	if analysis.UnsupportedResourceTypes[0] != "profiles" || analysis.UnsupportedResourceTypes[1] != "taskLists" {
		t.Fatalf("unexpected unsupported resource types: %v", analysis.UnsupportedResourceTypes)
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
