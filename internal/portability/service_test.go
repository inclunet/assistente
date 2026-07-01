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

func TestExportConversationPreservesStableIDs(t *testing.T) {
	parentID := "10"
	turnID := "10"
	assistantID := "20"

	conv := &database.Conversation{
		Title: "Teste",
		Messages: []database.ChatMessage{
			{UUIDModel: database.UUIDModel{ID: parentID, CreatedAt: time.Unix(100, 0)}, Role: "user", Content: "Oi"},
			{UUIDModel: database.UUIDModel{ID: assistantID, CreatedAt: time.Unix(101, 0)}, Role: "assistant", Content: "Ola", ParentID: &parentID, TurnID: &turnID},
		},
	}

	exported := exportConversation(conv, false)
	if len(exported.Messages) != 2 {
		t.Fatalf("len(Messages) = %d, want 2", len(exported.Messages))
	}
	if exported.Messages[0].ID != parentID {
		t.Fatalf("root ID = %q, want %q", exported.Messages[0].ID, parentID)
	}
	if exported.Messages[1].ParentID != parentID {
		t.Fatalf("assistant ParentID = %q, want %q", exported.Messages[1].ParentID, parentID)
	}
	if exported.Messages[1].TurnID != turnID {
		t.Fatalf("assistant TurnID = %q, want %q", exported.Messages[1].TurnID, turnID)
	}
}

func TestExportConversationOmitsAudioByDefault(t *testing.T) {
	conv := &database.Conversation{
		Title: "Audio",
		Messages: []database.ChatMessage{
			{UUIDModel: database.UUIDModel{ID: "1"}, Role: "assistant", Content: "fala", Audio: "base64-audio", AudioMimeType: "audio/mpeg"},
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

func TestExportPortableDataIncludesMCPServers(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()
	server := database.MCPServer{
		UserID:      portabilityTestUserID,
		Slug:        "github",
		Name:        "GitHub",
		Transport:   "streamable",
		URL:         "https://github.example/mcp",
		Args:        `["--verbose"]`,
		Env:         `{"TOKEN":"x"}`,
		Enabled:     true,
		AutoConnect: true,
	}
	if err := database.DB().Create(&server).Error; err != nil {
		t.Fatalf("create mcp server: %v", err)
	}

	file, err := BuildExportFileWithContext(ctx, nil, nil, nil, nil, ExportRequest{
		ExplicitSelection: true,
		MCPServerSlugs:    []string{"github"},
	}, "test")
	if err != nil {
		t.Fatalf("BuildExportFileWithContext: %v", err)
	}
	if len(file.Resources.MCPServers) != 1 {
		t.Fatalf("MCPServers len = %d, want 1", len(file.Resources.MCPServers))
	}
	got := file.Resources.MCPServers[0]
	if got.Slug != "github" || got.URL != "https://github.example/mcp" {
		t.Fatalf("unexpected mcp export: %#v", got)
	}
	if len(got.Env) != 0 {
		t.Fatalf("env should be omitted from portable export, got %#v", got.Env)
	}
}

func TestExportPortableDataIncludesMemoryRecordsWhenAll(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()
	record := database.MemoryRecord{
		UserID:             portabilityTestUserID,
		Content:            "Preferir respostas em pt-BR.",
		LoadPolicy:         database.MemoryLoadPolicyPinned,
		ArchivedFromPolicy: database.MemoryLoadPolicyCore,
		Kind:               database.MemoryKindUserPreference,
		Scope:              database.MemoryScopeUser,
		Importance:         5,
		Confidence:         90,
	}
	if err := database.DB().Create(&record).Error; err != nil {
		t.Fatalf("create memory: %v", err)
	}
	expired := database.MemoryRecord{
		UserID:     portabilityTestUserID,
		Content:    "Memória expirada",
		LoadPolicy: database.MemoryLoadPolicyPinned,
		Kind:       database.MemoryKindUserPreference,
		Scope:      database.MemoryScopeUser,
		ExpiresAt:  timePtr(time.Now().Add(-time.Hour)),
	}
	if err := database.DB().Create(&expired).Error; err != nil {
		t.Fatalf("create expired memory: %v", err)
	}

	file, err := BuildExportFileWithContext(ctx, nil, nil, nil, nil, ExportRequest{All: true}, "test")
	if err != nil {
		t.Fatalf("BuildExportFileWithContext: %v", err)
	}
	if len(file.Resources.MemoryRecords) != 1 {
		t.Fatalf("MemoryRecords len = %d, want 1", len(file.Resources.MemoryRecords))
	}
	got := file.Resources.MemoryRecords[0]
	if got.ID != record.ID || got.Content != record.Content || got.ArchivedFromPolicy != database.MemoryLoadPolicyCore {
		t.Fatalf("unexpected memory export: %#v", got)
	}
}

func TestImportPortableMemoryRecordUpsertsByID(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()
	exported := ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Now().UTC(),
		Options:    ExportOptions{},
		Resources: ExportResources{
			MemoryRecords: []MemoryRecordExport{{
				ID:                 "mem-1",
				Content:            "Memória importada",
				LoadPolicy:         database.MemoryLoadPolicyArchived,
				ArchivedFromPolicy: database.MemoryLoadPolicyPinned,
				Kind:               database.MemoryKindDecision,
				Scope:              database.MemoryScopeUser,
				Importance:         4,
				Confidence:         80,
				CreatedAt:          time.Unix(100, 0),
			}},
		},
	}
	raw, err := json.Marshal(exported)
	if err != nil {
		t.Fatalf("marshal export: %v", err)
	}

	result, err := ImportConversationsWithContext(ctx, string(raw), nil, "")
	if err != nil {
		t.Fatalf("ImportConversationsWithContext: %v", err)
	}
	if !result.Success || result.Imported != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	var record database.MemoryRecord
	if err := database.ScopeByUser(ctx, database.DB(), "user_id").First(&record, "id = ?", "mem-1").Error; err != nil {
		t.Fatalf("load memory: %v", err)
	}
	if record.ArchivedFromPolicy != database.MemoryLoadPolicyPinned || record.LoadPolicy != database.MemoryLoadPolicyArchived {
		t.Fatalf("memory metadata not imported: %+v", record)
	}

	exported.Resources.MemoryRecords[0].Content = "Memória atualizada"
	raw, err = json.Marshal(exported)
	if err != nil {
		t.Fatalf("marshal second export: %v", err)
	}
	result, err = ImportConversationsWithContext(ctx, string(raw), nil, "")
	if err != nil {
		t.Fatalf("ImportConversationsWithContext second: %v", err)
	}
	if !result.Success || result.Imported != 1 {
		t.Fatalf("unexpected second result: %+v", result)
	}
	if err := database.ScopeByUser(ctx, database.DB(), "user_id").First(&record, "id = ?", "mem-1").Error; err != nil {
		t.Fatalf("reload memory: %v", err)
	}
	if record.Content != "Memória atualizada" {
		t.Fatalf("memory not upserted: %+v", record)
	}
}

func TestImportPortableMemoryRecordValidatesRecord(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()
	exported := ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Now().UTC(),
		Options:    ExportOptions{},
		Resources: ExportResources{
			MemoryRecords: []MemoryRecordExport{{
				ID:         "mem-invalid",
				Content:    "sem referência de escopo",
				LoadPolicy: database.MemoryLoadPolicyPinned,
				Kind:       database.MemoryKindDecision,
				Scope:      database.MemoryScopeWorkspace,
				Importance: 4,
				Confidence: 80,
				CreatedAt:  time.Unix(100, 0),
			}},
		},
	}
	raw, err := json.Marshal(exported)
	if err != nil {
		t.Fatalf("marshal export: %v", err)
	}

	result, err := ImportConversationsWithContext(ctx, string(raw), nil, "")
	if err != nil {
		t.Fatalf("ImportConversationsWithContext: %v", err)
	}
	if result.Success || result.Failed != 1 {
		t.Fatalf("invalid memory import should fail: %+v", result)
	}
	var count int64
	if err := database.ScopeByUser(ctx, database.DB().Model(&database.MemoryRecord{}), "user_id").Count(&count).Error; err != nil {
		t.Fatalf("count memories: %v", err)
	}
	if count != 0 {
		t.Fatalf("invalid memory was persisted, count=%d", count)
	}
}

func TestExportExternalMCPServersOmitsEnv(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()
	server := database.MCPServer{
		UserID:      portabilityTestUserID,
		Slug:        "filesystem",
		Name:        "Filesystem",
		Transport:   "stdio",
		Command:     "npx",
		Args:        `["-y","@modelcontextprotocol/server-filesystem"]`,
		Env:         `{"TOKEN":"x"}`,
		Enabled:     true,
		AutoConnect: true,
	}
	if err := database.DB().Create(&server).Error; err != nil {
		t.Fatalf("create mcp server: %v", err)
	}

	raw, err := ExportMCPServersExternalJSONWithContext(ctx, []string{"filesystem"})
	if err != nil {
		t.Fatalf("ExportMCPServersExternalJSONWithContext: %v", err)
	}
	var decoded externalMCPExportFile
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("unmarshal external export: %v", err)
	}
	got := decoded.MCPServers["filesystem"]
	if got.Command != "npx" || len(got.Args) != 2 {
		t.Fatalf("unexpected external mcp export: %#v", got)
	}
	if len(got.Env) != 0 {
		t.Fatalf("env should be omitted from external export, got %#v", got.Env)
	}
}

func TestImportPortableMCPServerIsIdempotent(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()
	server := MCPServerExport{
		Slug:        "github",
		Name:        "GitHub",
		Transport:   "streamable",
		URL:         "https://github.example/mcp",
		Enabled:     true,
		AutoConnect: true,
	}

	imported, err := ImportMCPServerWithContext(ctx, server)
	if err != nil {
		t.Fatalf("ImportMCPServerWithContext first: %v", err)
	}
	if !imported {
		t.Fatal("first import should insert")
	}
	imported, err = ImportMCPServerWithContext(ctx, MCPServerExport{
		Slug:        "github",
		Name:        "Changed",
		Transport:   "streamable",
		URL:         "https://changed.example/mcp",
		Enabled:     true,
		AutoConnect: true,
	})
	if err != nil {
		t.Fatalf("ImportMCPServerWithContext second: %v", err)
	}
	if imported {
		t.Fatal("second import should skip existing slug")
	}

	var row database.MCPServer
	if err := database.DB().Where("user_id = ? AND slug = ?", portabilityTestUserID, "github").First(&row).Error; err != nil {
		t.Fatalf("load mcp server: %v", err)
	}
	if row.Name != "GitHub" || row.URL != "https://github.example/mcp" {
		t.Fatalf("existing server was overwritten: %#v", row)
	}
}

func TestImportDataAcceptsExternalMCPServersJSON(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()
	payload := `{"mcpServers":{"filesystem":{"command":"npx","args":["-y","@modelcontextprotocol/server-filesystem"],"env":{"ROOT":"/tmp"}}}}`

	result, err := ImportConversationsWithContext(ctx, payload, nil, "")
	if err != nil {
		t.Fatalf("ImportConversationsWithContext: %v", err)
	}
	if !result.Success || result.Imported != 1 {
		t.Fatalf("result = %#v", result)
	}
	var row database.MCPServer
	if err := database.DB().Where("user_id = ? AND slug = ?", portabilityTestUserID, "filesystem").First(&row).Error; err != nil {
		t.Fatalf("load mcp server: %v", err)
	}
	if row.Transport != "stdio" || row.Command != "npx" {
		t.Fatalf("unexpected imported server: %#v", row)
	}
}

func TestImportMCPServersJSONContinuesAfterInvalidServer(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()
	payload := []byte(`{"mcpServers":{"good":{"url":"https://good.example/mcp"},"broken":{"name":"Broken"}}}`)

	result, err := ImportMCPServersJSONWithContext(ctx, payload, nil)
	if err != nil {
		t.Fatalf("ImportMCPServersJSONWithContext: %v", err)
	}
	if result.Imported != 1 || result.Failed != 1 || len(result.Errors) != 1 {
		t.Fatalf("result = %#v", result)
	}
	var row database.MCPServer
	if err := database.DB().Where("user_id = ? AND slug = ?", portabilityTestUserID, "good").First(&row).Error; err != nil {
		t.Fatalf("load imported mcp server: %v", err)
	}
	if row.URL != "https://good.example/mcp" {
		t.Fatalf("unexpected imported server: %#v", row)
	}
}

func TestImportDataAcceptsEmptyExternalMCPServersJSON(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()

	result, err := ImportConversationsWithContext(ctx, `{"mcpServers":{}}`, nil, "")
	if err != nil {
		t.Fatalf("ImportConversationsWithContext: %v", err)
	}
	if !result.Success || result.Imported != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestParseExternalMCPServersRejectsUnrelatedFlatObject(t *testing.T) {
	servers, ok, err := parseExternalMCPServers([]byte(`{"foo":{"bar":"baz"}}`))
	if err != nil {
		t.Fatalf("parseExternalMCPServers: %v", err)
	}
	if ok || len(servers) != 0 {
		t.Fatalf("unrelated flat object should not be MCP JSON, ok=%v servers=%#v", ok, servers)
	}
}

func TestParseExternalMCPServersRejectsMixedFlatObject(t *testing.T) {
	payload := []byte(`{"api":{"url":"https://api.example.com"},"metadata":{"name":"not an mcp server"}}`)
	servers, ok, err := parseExternalMCPServers(payload)
	if err != nil {
		t.Fatalf("parseExternalMCPServers: %v", err)
	}
	if ok || len(servers) != 0 {
		t.Fatalf("mixed flat object should not be MCP JSON, ok=%v servers=%#v", ok, servers)
	}
}

func TestParseExternalMCPServersAcceptsFlatObjectWhenAllEntriesAreServers(t *testing.T) {
	payload := []byte(`{"filesystem":{"command":"npx"},"github":{"url":"https://api.githubcopilot.com/mcp/"}}`)
	servers, ok, err := parseExternalMCPServers(payload)
	if err != nil {
		t.Fatalf("parseExternalMCPServers: %v", err)
	}
	if !ok || len(servers) != 2 {
		t.Fatalf("flat MCP object should be accepted, ok=%v servers=%#v", ok, servers)
	}
}

func TestImportMCPServersJSONRejectsUnrelatedJSON(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()

	result, err := ImportMCPServersJSONWithContext(ctx, []byte(`{"foo":{"bar":"baz"}}`), nil)
	if err == nil {
		t.Fatal("expected unrelated JSON to fail")
	}
	if !strings.Contains(err.Error(), "servidores MCP") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Failed != 1 || len(result.Errors) != 1 {
		t.Fatalf("result = %#v, want one failure", result)
	}
}

func TestImportDataExternalMCPServersImportsBearerCredential(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()
	credMgr := credentials.NewManagerWithStore([]byte("test-key-exactly-32-bytes-long!!"), credentials.NewDBStore(), true)
	payload := `{"mcpServers":{"github":{"url":"https://api.githubcopilot.com/mcp/","requestInit":{"headers":{"Authorization":"Bearer ghp_imported"}}}}}`

	result, err := ImportConversationsWithContext(ctx, payload, credMgr, "")
	if err != nil {
		t.Fatalf("ImportConversationsWithContext: %v", err)
	}
	if !result.Success || result.Imported != 1 {
		t.Fatalf("result = %#v", result)
	}
	auth, err := credMgr.GetByPatternWithContext(ctx, "api.githubcopilot.com")
	if err != nil {
		t.Fatalf("GetByPatternWithContext: %v", err)
	}
	if auth == nil || auth.Token != "ghp_imported" {
		t.Fatalf("imported auth = %#v", auth)
	}
}

func TestImportMCPServerRejectsIncompleteTransportConfig(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()

	_, err := ImportMCPServerWithContext(ctx, MCPServerExport{
		Slug: "broken",
		Name: "Broken",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), `transport inválido ou ausente`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestImportLegacyMCPServersIsReusableAndIdempotent(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()
	source := &memoryLegacyImportSource{
		files: []LegacyImportFile{{Name: "github", Filename: "github.json", Path: "/legacy/github.json", Source: "home"}},
		data: map[string][]byte{
			"github.json": []byte(`{"name":"GitHub","transport":"streamable","url":"https://github.example/mcp","enabled":true,"auto_connect":true}`),
		},
	}
	original := string(source.data["github.json"])

	result, err := ImportLegacyMCPServersWithContext(ctx, source, nil)
	if err != nil {
		t.Fatalf("ImportLegacyMCPServersWithContext first: %v", err)
	}
	if result.Imported != 1 || result.Skipped != 0 {
		t.Fatalf("first result = %#v", result)
	}
	if string(source.data["github.json"]) != original {
		t.Fatal("legacy source should remain untouched")
	}

	source.data["github.json"] = []byte(`{"name":"Changed","transport":"streamable","url":"https://changed.example/mcp","enabled":true,"auto_connect":true}`)
	result, err = ImportLegacyMCPServersWithContext(ctx, source, nil)
	if err != nil {
		t.Fatalf("ImportLegacyMCPServersWithContext second: %v", err)
	}
	if result.Imported != 0 || result.Skipped != 1 {
		t.Fatalf("second result = %#v", result)
	}

	var row database.MCPServer
	if err := database.DB().Where("user_id = ? AND slug = ?", portabilityTestUserID, "github").First(&row).Error; err != nil {
		t.Fatalf("load mcp server: %v", err)
	}
	if row.Name != "GitHub" || row.URL != "https://github.example/mcp" {
		t.Fatalf("legacy import overwrote existing server: %#v", row)
	}
}

func TestImportLegacyMCPServersContinuesAfterInvalidFile(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()
	source := &memoryLegacyImportSource{
		files: []LegacyImportFile{
			{Name: "broken", Filename: "broken.json", Path: "/legacy/broken.json", Source: "home"},
			{Name: "github", Filename: "github.json", Path: "/legacy/github.json", Source: "home"},
		},
		data: map[string][]byte{
			"broken.json": []byte(`{"name":`),
			"github.json": []byte(`{"name":"GitHub","transport":"streamable","url":"https://github.example/mcp","enabled":true,"auto_connect":true}`),
		},
	}

	result, err := ImportLegacyMCPServersWithContext(ctx, source, nil)
	if err != nil {
		t.Fatalf("ImportLegacyMCPServersWithContext: %v", err)
	}
	if result.Imported != 1 || result.Failed != 1 || len(result.Errors) != 1 {
		t.Fatalf("result = %#v", result)
	}
	var row database.MCPServer
	if err := database.DB().Where("user_id = ? AND slug = ?", portabilityTestUserID, "github").First(&row).Error; err != nil {
		t.Fatalf("load mcp server: %v", err)
	}
}

type memoryLegacyImportSource struct {
	files []LegacyImportFile
	data  map[string][]byte
}

func (s *memoryLegacyImportSource) ListLegacyImportFiles(context.Context) ([]LegacyImportFile, error) {
	return append([]LegacyImportFile(nil), s.files...), nil
}

func (s *memoryLegacyImportSource) ReadLegacyImportFile(_ context.Context, filename string) ([]byte, error) {
	return append([]byte(nil), s.data[filename]...), nil
}

func TestBuildExportFileLoadsConversationsInBatchPreservingRequestedOrder(t *testing.T) {
	setupPortabilityTestDB(t)

	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.UTC)
	conversations := []database.Conversation{
		{
			UUIDModel: database.UUIDModel{
				ID:        "01926b90-0000-7000-8000-000000000901",
				CreatedAt: now,
				UpdatedAt: now,
			},
			Title:  "Primeira",
			UserID: portabilityTestUserID,
		},
		{
			UUIDModel: database.UUIDModel{
				ID:        "01926b90-0000-7000-8000-000000000902",
				CreatedAt: now.Add(time.Minute),
				UpdatedAt: now.Add(time.Minute),
			},
			Title:  "Segunda",
			UserID: portabilityTestUserID,
		},
	}
	if err := database.DB().Create(&conversations).Error; err != nil {
		t.Fatalf("Create(conversations) error = %v", err)
	}

	messages := []database.ChatMessage{
		{
			UUIDModel:      database.UUIDModel{ID: "01926b90-0000-7000-8000-000000000903", CreatedAt: now.Add(2 * time.Minute)},
			ConversationID: conversations[1].ID,
			Role:           "assistant",
			Content:        "segunda mensagem 2",
		},
		{
			UUIDModel:      database.UUIDModel{ID: "01926b90-0000-7000-8000-000000000904", CreatedAt: now.Add(time.Minute)},
			ConversationID: conversations[1].ID,
			Role:           "user",
			Content:        "segunda mensagem 1",
		},
		{
			UUIDModel:      database.UUIDModel{ID: "01926b90-0000-7000-8000-000000000905", CreatedAt: now},
			ConversationID: conversations[0].ID,
			Role:           "user",
			Content:        "primeira mensagem",
		},
	}
	if err := database.DB().Create(&messages).Error; err != nil {
		t.Fatalf("Create(messages) error = %v", err)
	}

	file, err := BuildExportFileWithContext(portabilityTestCtx(), []string{conversations[1].ID, conversations[0].ID, conversations[1].ID}, nil, nil, nil, ExportRequest{}, "test")
	if err != nil {
		t.Fatalf("BuildExportFile() error = %v", err)
	}
	if len(file.Resources.Conversations) != 3 {
		t.Fatalf("len(Conversations) = %d, want 3", len(file.Resources.Conversations))
	}
	if file.Resources.Conversations[0].ID != conversations[1].ID || file.Resources.Conversations[1].ID != conversations[0].ID || file.Resources.Conversations[2].ID != conversations[1].ID {
		t.Fatalf("unexpected conversation order: %+v", file.Resources.Conversations)
	}
	if got := file.Resources.Conversations[0].Messages; len(got) != 2 || got[0].Content != "segunda mensagem 1" || got[1].Content != "segunda mensagem 2" {
		t.Fatalf("messages not ordered by created_at: %+v", got)
	}
}

func TestBuildExportFileReturnsClearErrorForMissingConversation(t *testing.T) {
	setupPortabilityTestDB(t)

	missingID := "01926b90-0000-7000-8000-000000000999"
	_, err := BuildExportFileWithContext(portabilityTestCtx(), []string{missingID}, nil, nil, nil, ExportRequest{}, "test")
	if err == nil {
		t.Fatal("BuildExportFile() error = nil, want missing conversation error")
	}
	if !strings.Contains(err.Error(), missingID) {
		t.Fatalf("BuildExportFile() error = %v, want missing ID in error", err)
	}
}

func TestExportConversationHydratesToolCallResultsFromToolInvocations(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()

	turnID := "turn-1"
	assistantID := "assistant-1"
	callID := "call-1"
	convID := "conv-1"

	conv := &database.Conversation{UUIDModel: database.UUIDModel{ID: convID}, UserID: portabilityTestUserID, Title: "Hydrate"}
	if err := database.DB().Create(conv).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if err := database.DB().Create(&database.ChatMessage{UUIDModel: database.UUIDModel{ID: turnID}, ConversationID: convID, Role: "user", Content: "hi"}).Error; err != nil {
		t.Fatalf("create turn message: %v", err)
	}
	toolCalls := `[{"id":"` + callID + `","type":"function","function":{"name":"x","arguments":"{}"}}]`
	if err := database.DB().Create(&database.ChatMessage{UUIDModel: database.UUIDModel{ID: assistantID}, ConversationID: convID, Role: "assistant", Content: "", ToolCalls: toolCalls, TurnID: &turnID}).Error; err != nil {
		t.Fatalf("create assistant tool_calls: %v", err)
	}
	if err := database.DB().Create(&database.ToolInvocation{UserID: portabilityTestUserID, ToolCatalogID: "tool-1", OriginType: "chat", OriginID: turnID, ToolCallID: callID, Status: "succeeded", DryRun: false, Output: `{"content":"RESULT"}`}).Error; err != nil {
		t.Fatalf("create tool invocation: %v", err)
	}

	file, err := BuildExportFileWithContext(ctx, []string{convID}, nil, nil, nil, ExportRequest{ExplicitSelection: true, ConversationIDs: []string{convID}}, "test")
	if err != nil {
		t.Fatalf("BuildExportFileWithContext: %v", err)
	}
	if len(file.Resources.Conversations) != 1 {
		t.Fatalf("expected 1 conversation export, got %d", len(file.Resources.Conversations))
	}
	msgs := file.Resources.Conversations[0].Messages
	if len(msgs) != 2 {
		t.Fatalf("expected 2 exported messages, got %d", len(msgs))
	}
	var decoded []map[string]any
	if err := json.Unmarshal([]byte(msgs[1].ToolCalls), &decoded); err != nil {
		t.Fatalf("unmarshal exported toolCalls: %v", err)
	}
	if len(decoded) != 1 {
		t.Fatalf("expected 1 tool call, got %#v", decoded)
	}
	if got, _ := decoded[0]["result"].(string); got != "RESULT" {
		t.Fatalf("hydrated result = %q, want RESULT", got)
	}
}

func TestExportConversationBuildsToolCallsFromToolInvocationsWithoutMessageToolCalls(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()

	turnID := "turn-new"
	assistantID := "assistant-new"
	callID := "call-new"
	convID := "conv-new"

	conv := &database.Conversation{UUIDModel: database.UUIDModel{ID: convID}, UserID: portabilityTestUserID, Title: "L3 free"}
	if err := database.DB().Create(conv).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if err := database.DB().Create(&database.ChatMessage{UUIDModel: database.UUIDModel{ID: turnID}, ConversationID: convID, Role: "user", Content: "hi"}).Error; err != nil {
		t.Fatalf("create turn message: %v", err)
	}
	if err := database.DB().Create(&database.ChatMessage{UUIDModel: database.UUIDModel{ID: assistantID}, ConversationID: convID, Role: "assistant", Content: "vou buscar", TurnID: &turnID}).Error; err != nil {
		t.Fatalf("create assistant without tool_calls: %v", err)
	}
	if err := database.DB().Create(&database.ToolInvocation{
		UserID:        portabilityTestUserID,
		ToolCatalogID: "tool-1",
		OriginType:    "chat",
		OriginID:      turnID,
		ToolCallID:    callID,
		Status:        "succeeded",
		DryRun:        false,
		Output:        `{"content":"RESULT-NEW"}`,
		Metadata:      `{"display":{"version":1,"type":"function","name":"search","arguments":"{\"q\":\"x\"}","origin":"builtin","iteration":2,"duration_ms":12}}`,
	}).Error; err != nil {
		t.Fatalf("create tool invocation: %v", err)
	}

	file, err := BuildExportFileWithContext(ctx, []string{convID}, nil, nil, nil, ExportRequest{ExplicitSelection: true, ConversationIDs: []string{convID}}, "test")
	if err != nil {
		t.Fatalf("BuildExportFileWithContext: %v", err)
	}
	msgs := file.Resources.Conversations[0].Messages
	var decoded []map[string]any
	if err := json.Unmarshal([]byte(msgs[1].ToolCalls), &decoded); err != nil {
		t.Fatalf("unmarshal synthesized toolCalls: %v", err)
	}
	if len(decoded) != 1 {
		t.Fatalf("expected 1 synthesized tool call, got %#v", decoded)
	}
	fn, _ := decoded[0]["function"].(map[string]any)
	if decoded[0]["id"] != callID || decoded[0]["result"] != "RESULT-NEW" || fn["name"] != "search" {
		t.Fatalf("unexpected synthesized tool call: %#v", decoded[0])
	}
}

func TestImportOverwriteClearsChatToolInvocationsToAvoidStaleExportHydration(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()

	turnID := "turn-1"
	assistantID := "assistant-1"
	callID := "call-1"
	convID := "conv-1"

	// Existing conversation with stale invocation.
	conv := &database.Conversation{UUIDModel: database.UUIDModel{ID: convID}, UserID: portabilityTestUserID, Title: "Original"}
	if err := database.DB().Create(conv).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if err := database.DB().Create(&database.ChatMessage{UUIDModel: database.UUIDModel{ID: turnID}, ConversationID: convID, Role: "user", Content: "hi"}).Error; err != nil {
		t.Fatalf("create turn message: %v", err)
	}
	toolCalls := `[{"id":"` + callID + `","type":"function","function":{"name":"x","arguments":"{}"}}]`
	if err := database.DB().Create(&database.ChatMessage{UUIDModel: database.UUIDModel{ID: assistantID}, ConversationID: convID, Role: "assistant", Content: "", ToolCalls: toolCalls, TurnID: &turnID}).Error; err != nil {
		t.Fatalf("create assistant tool_calls: %v", err)
	}
	if err := database.DB().Create(&database.ToolInvocation{UserID: portabilityTestUserID, ToolCatalogID: "tool-1", OriginType: "chat", OriginID: turnID, ToolCallID: callID, Status: "succeeded", DryRun: false, Output: `{"content":"OLD"}`}).Error; err != nil {
		t.Fatalf("create stale tool invocation: %v", err)
	}

	// Import overwrite with the same IDs but without creating tool_invocations.
	importFile := ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Now().UTC(),
		Options:    ExportOptions{},
		Resources: ExportResources{Conversations: []ConversationExport{{
			ID:        convID,
			Title:     "Replaced",
			CreatedAt: time.Now().UTC(),
			Messages: []MessageExport{{
				ID:        turnID,
				Role:      "user",
				Content:   "hi",
				CreatedAt: time.Now().UTC(),
			}, {
				ID:        assistantID,
				Role:      "assistant",
				Content:   "",
				ToolCalls: toolCalls,
				TurnID:    turnID,
				CreatedAt: time.Now().UTC(),
			}}}},
		},
	}
	raw, err := json.Marshal(importFile)
	if err != nil {
		t.Fatalf("marshal import file: %v", err)
	}
	res, err := ImportConversationsWithContext(ctx, string(raw), nil, "")
	if err != nil {
		t.Fatalf("ImportConversationsWithContext: %v", err)
	}
	if !res.Success {
		t.Fatalf("import result = %#v", res)
	}

	// Export again; should not hydrate OLD from stale invocations.
	file, err := BuildExportFileWithContext(ctx, []string{convID}, nil, nil, nil, ExportRequest{ExplicitSelection: true, ConversationIDs: []string{convID}}, "test")
	if err != nil {
		t.Fatalf("BuildExportFileWithContext: %v", err)
	}
	msgs := file.Resources.Conversations[0].Messages
	var decoded []map[string]any
	if err := json.Unmarshal([]byte(msgs[1].ToolCalls), &decoded); err != nil {
		t.Fatalf("unmarshal exported toolCalls: %v", err)
	}
	if got, _ := decoded[0]["result"].(string); got == "OLD" {
		t.Fatal("export hydrated stale tool result after overwrite")
	}
}

func TestExportConversationPrefersFallbackToolMessageOverInvocationHydration(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()

	turnID := "turn-1"
	assistantID := "assistant-1"
	callID := "call-1"
	convID := "conv-1"

	conv := &database.Conversation{UUIDModel: database.UUIDModel{ID: convID}, UserID: portabilityTestUserID, Title: "Hydrate"}
	if err := database.DB().Create(conv).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if err := database.DB().Create(&database.ChatMessage{UUIDModel: database.UUIDModel{ID: turnID}, ConversationID: convID, Role: "user", Content: "hi"}).Error; err != nil {
		t.Fatalf("create turn message: %v", err)
	}
	toolCalls := `[{"id":"` + callID + `","type":"function","function":{"name":"x","arguments":"{}"}}]`
	if err := database.DB().Create(&database.ChatMessage{UUIDModel: database.UUIDModel{ID: assistantID}, ConversationID: convID, Role: "assistant", Content: "", ToolCalls: toolCalls, TurnID: &turnID}).Error; err != nil {
		t.Fatalf("create assistant tool_calls: %v", err)
	}

	// Existe um tool_invocations "stale" (ex.: falha anterior), mas o resultado real
	// do turno atual caiu em fallback role=tool (persistência falhou) e deve vencer.
	if err := database.DB().Create(&database.ToolInvocation{UserID: portabilityTestUserID, ToolCatalogID: "tool-1", OriginType: "chat", OriginID: turnID, ToolCallID: callID, Status: "succeeded", DryRun: false, Output: `{"content":"STALE"}`}).Error; err != nil {
		t.Fatalf("create tool invocation: %v", err)
	}
	if err := database.DB().Create(&database.ChatMessage{ConversationID: convID, Role: "tool", Content: "FALLBACK", ToolCallID: callID, TurnID: &turnID}).Error; err != nil {
		t.Fatalf("create tool fallback message: %v", err)
	}

	file, err := BuildExportFileWithContext(ctx, []string{convID}, nil, nil, nil, ExportRequest{ExplicitSelection: true, ConversationIDs: []string{convID}}, "test")
	if err != nil {
		t.Fatalf("BuildExportFileWithContext: %v", err)
	}
	msgs := file.Resources.Conversations[0].Messages
	var decoded []map[string]any
	if err := json.Unmarshal([]byte(msgs[1].ToolCalls), &decoded); err != nil {
		t.Fatalf("unmarshal exported toolCalls: %v", err)
	}
	if got, _ := decoded[0]["result"].(string); got != "FALLBACK" {
		t.Fatalf("hydrated result = %q, want FALLBACK", got)
	}
}

func TestExportConversationIgnoresEmptyFallbackToolMessage(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()

	turnID := "turn-1"
	assistantID := "assistant-1"
	callID := "call-1"
	convID := "conv-1"

	conv := &database.Conversation{UUIDModel: database.UUIDModel{ID: convID}, UserID: portabilityTestUserID, Title: "Hydrate"}
	if err := database.DB().Create(conv).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if err := database.DB().Create(&database.ChatMessage{UUIDModel: database.UUIDModel{ID: turnID}, ConversationID: convID, Role: "user", Content: "hi"}).Error; err != nil {
		t.Fatalf("create turn message: %v", err)
	}
	toolCalls := `[{"id":"` + callID + `","type":"function","function":{"name":"x","arguments":"{}"}}]`
	if err := database.DB().Create(&database.ChatMessage{UUIDModel: database.UUIDModel{ID: assistantID}, ConversationID: convID, Role: "assistant", Content: "", ToolCalls: toolCalls, TurnID: &turnID}).Error; err != nil {
		t.Fatalf("create assistant tool_calls: %v", err)
	}
	if err := database.DB().Create(&database.ToolInvocation{UserID: portabilityTestUserID, ToolCatalogID: "tool-1", OriginType: "chat", OriginID: turnID, ToolCallID: callID, Status: "succeeded", DryRun: false, Output: `{"content":"REAL"}`}).Error; err != nil {
		t.Fatalf("create tool invocation: %v", err)
	}
	// Placeholder/empty tool message deve ser ignorada.
	if err := database.DB().Create(&database.ChatMessage{ConversationID: convID, Role: "tool", Content: "", ToolCallID: callID, TurnID: &turnID}).Error; err != nil {
		t.Fatalf("create empty tool message: %v", err)
	}

	file, err := BuildExportFileWithContext(ctx, []string{convID}, nil, nil, nil, ExportRequest{ExplicitSelection: true, ConversationIDs: []string{convID}}, "test")
	if err != nil {
		t.Fatalf("BuildExportFileWithContext: %v", err)
	}
	msgs := file.Resources.Conversations[0].Messages
	var decoded []map[string]any
	if err := json.Unmarshal([]byte(msgs[1].ToolCalls), &decoded); err != nil {
		t.Fatalf("unmarshal exported toolCalls: %v", err)
	}
	if got, _ := decoded[0]["result"].(string); got != "REAL" {
		t.Fatalf("hydrated result = %q, want REAL", got)
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
		&database.ToolInvocation{},
		&database.TaskListWorkflow{},
		&database.TaskList{},
		&database.Task{},
		&database.TaskNote{},
		&database.MemoryRecord{},
		&database.CredentialEntry{},
		&database.MCPServer{},
		&database.SubAgentRun{},
	); err != nil {
		t.Fatalf("falha ao migrar tabelas: %v", err)
	}
	database.SetDB(db)
}

// portabilityTestUserID is the implicit user-id used by the legacy/test fixtures
// in this package. Tests that previously relied on no-context wrappers (which
// silently dropped the user_id filter) now scope explicitly via this id.
const portabilityTestUserID = "portability-test-user"

func portabilityTestCtx() context.Context {
	return database.WithUserID(context.Background(), portabilityTestUserID)
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
	if err := database.SaveLLMProviderWithContext(portabilityTestCtx(), provider); err != nil {
		t.Fatalf("SaveLLMProvider() error = %v", err)
	}
	return provider
}

func createPortableTaskListFixture(t *testing.T) *database.TaskList {
	t.Helper()

	ctx := portabilityTestCtx()
	taskList, err := database.CreateTaskListWithContext(ctx, "Sprint 42", "Implementar portability", nil, "sprint-42")
	if err != nil {
		t.Fatalf("CreateTaskList() error = %v", err)
	}
	if err := database.SetTaskListViewModeWithContext(ctx, taskList.ID, "kanban"); err != nil {
		t.Fatalf("SetTaskListViewMode() error = %v", err)
	}

	policy := `{"task_code_regex":"^TASK-[0-9]+$","allowed_note_sources":["jira"]}`
	if err := database.SetTaskListValidationPolicyWithContext(ctx, taskList.ID, policy); err != nil {
		t.Fatalf("SetTaskListValidationPolicy() error = %v", err)
	}

	root, err := database.CreateTaskFullWithContext(
		ctx,
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
	if err := database.UpdateTaskStatusWithContext(ctx, root.ID, 2); err != nil {
		t.Fatalf("UpdateTaskStatus(root) error = %v", err)
	}

	child, err := database.CreateTaskFullWithContext(
		ctx,
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

	note, err := database.CreateTaskNoteWithContext(ctx, root.ID, database.TaskNoteAgent, "Primeira nota", "Assistente", "agent")
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

	out, err := database.GetTaskListWithContext(ctx, taskList.ID)
	if err != nil {
		t.Fatalf("GetTaskList() error = %v", err)
	}
	return out
}

func TestAnalyzeImportDataDoesNotDetectNaturalConversationConflicts(t *testing.T) {
	setupPortabilityTestDB(t)

	existingCreatedAt := time.Date(2025, 4, 24, 10, 0, 0, 0, time.UTC)
	existingConv := &database.Conversation{
		UUIDModel: database.UUIDModel{
			CreatedAt: existingCreatedAt,
			UpdatedAt: existingCreatedAt,
		},
		UserID:  portabilityTestUserID,
		Title:   "Conversa importada",
		Channel: "telegram",
	}
	if err := database.DB().Create(existingConv).Error; err != nil {
		t.Fatalf("falha ao criar conversa existente: %v", err)
	}

	credMgr := credentials.NewManagerWithStoreAndPersistence([]byte("test-key-exactly-32-bytes-long!!"), credentials.NewDBStore(), true)
	if err := credMgr.RegisterPatternWithContext(portabilityTestCtx(), "api.openai.com", &credentials.AuthConfig{
		Type:  "bearer",
		Token: "secret",
	}); err != nil {
		t.Fatalf("falha ao registrar credencial existente: %v", err)
	}
	existingCreds, err := credMgr.ListCredentials()
	if err != nil || len(existingCreds) != 1 {
		t.Fatalf("ListCredentials() error = %v, len = %d", err, len(existingCreds))
	}

	file := &ExportFile{
		Version:    ExportVersion,
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
		{ID: existingCreds[0].ID, Pattern: "api.openai.com", AuthType: "bearer", Token: "secret"},
	})
	if err != nil {
		t.Fatalf("falha ao criptografar credenciais de teste: %v", err)
	}
	file.Resources.Credentials = blob

	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("falha ao serializar export file: %v", err)
	}

	analysis, err := AnalyzeImportDataWithContext(portabilityTestCtx(), string(raw), credMgr, "senha-teste")
	if err != nil {
		t.Fatalf("AnalyzeImportData() error = %v", err)
	}

	if analysis.ConversationCount != 1 || analysis.MessageCount != 1 {
		t.Fatalf("counts inesperados: %+v", analysis)
	}
	if analysis.ConflictCount != 0 {
		t.Fatalf("ConflictCount = %d, want 0", analysis.ConflictCount)
	}
	if len(analysis.ConversationConflicts) != 0 {
		t.Fatalf("conversation conflicts = %d, want 0", len(analysis.ConversationConflicts))
	}
	if len(analysis.CredentialConflicts) != 0 {
		t.Fatalf("credential conflicts = %d, want 0 for idempotent credential import", len(analysis.CredentialConflicts))
	}
}

func TestImportConversationRestoresCreatedAt(t *testing.T) {
	setupPortabilityTestDB(t)

	ctx := portabilityTestCtx()
	createdAt := time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)
	imported, err := importConversation(ctx, ConversationExport{
		ID:        "01926b90-0000-7000-8000-000000000101",
		Title:     "Conversa antiga",
		CreatedAt: createdAt,
		Messages: []MessageExport{
			{ID: "01926b90-0000-7000-8000-000000000102", Role: "user", Content: "Oi", CreatedAt: createdAt},
		},
	}, false)
	if err != nil {
		t.Fatalf("importConversation() error = %v", err)
	}
	if !imported {
		t.Fatal("importConversation() = false, want true")
	}

	conversations, err := database.GetConversationsWithContext(ctx)
	if err != nil {
		t.Fatalf("GetConversations() error = %v", err)
	}
	if len(conversations) != 1 {
		t.Fatalf("len(conversations) = %d, want 1", len(conversations))
	}
	if !conversations[0].CreatedAt.Equal(createdAt) {
		t.Fatalf("CreatedAt = %s, want %s", conversations[0].CreatedAt, createdAt)
	}

	conv, err := database.GetConversationWithContext(ctx, conversations[0].ID)
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

	ctx := portabilityTestCtx()
	_, err := importConversation(ctx, ConversationExport{
		ID:        "01926b90-0000-7000-8000-000000000111",
		Title:     "Conversa inválida",
		CreatedAt: time.Now().UTC(),
		Messages: []MessageExport{
			{ID: "01926b90-0000-7000-8000-000000000112", Role: "user", Content: "Oi", CreatedAt: time.Now().UTC()},
			{ID: "01926b90-0000-7000-8000-000000000113", Role: "assistant", Content: "Resposta", ParentIndex: intPtr(99), CreatedAt: time.Now().UTC()},
		},
	}, false)
	if err == nil {
		t.Fatal("importConversation() error = nil, want invalid reference error")
	}

	conversations, err := database.GetConversationsWithContext(ctx)
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
		Version:    ExportVersion,
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

	analysis, err := AnalyzeImportDataWithContext(portabilityTestCtx(), string(raw), nil, "")
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
		"version": 2,
		"resources": {
			"conversations": [],
			"profiles": [{"slug":"perfil-demo"}],
			"taskLists": [{"title":"Sprint 42"}],
			"credentials": null
		}
	}`

	analysis, err := AnalyzeImportDataWithContext(portabilityTestCtx(), raw, nil, "")
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

	file, err := BuildExportFileWithContext(portabilityTestCtx(), nil, nil, []string{taskList.ID}, nil, ExportRequest{}, "test")
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

	// Usa o ctx escopado por usuário porque GetLLMProviderWithContext agora
	// é fail-closed (B11 / AEP-0052): a versão sem ctx só funciona se o
	// caller passar bootstrap explícito, fora do escopo deste teste.
	file, err := BuildExportFileWithContext(portabilityTestCtx(), nil, []string{provider.ID}, nil, nil, ExportRequest{}, "test")
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

	// Mesmo motivo do teste acima: GetLLMProviderWithContext fail-closed
	// exige ctx escopado por usuário (B11 / AEP-0052).
	file, err := BuildExportFileWithContext(portabilityTestCtx(), nil, []string{provider.ID}, nil, nil, ExportRequest{}, "test")
	if err != nil {
		t.Fatalf("BuildExportFile() error = %v", err)
	}
	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	analysis, err := AnalyzeImportDataWithContext(portabilityTestCtx(), string(raw), nil, "")
	if err != nil {
		t.Fatalf("AnalyzeImportData() error = %v", err)
	}

	if analysis.ProviderCount != 1 {
		t.Fatalf("ProviderCount = %d, want 1", analysis.ProviderCount)
	}
	if len(analysis.ProviderConflicts) != 0 {
		t.Fatalf("len(ProviderConflicts) = %d, want 0 for idempotent upsert by id", len(analysis.ProviderConflicts))
	}
}

func TestAnalyzeImportDataDetectsMCPServerConflicts(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()
	if err := database.DB().Create(&database.MCPServer{
		UserID:      portabilityTestUserID,
		Slug:        "github",
		Name:        "GitHub",
		Transport:   "streamable",
		URL:         "https://github.example/mcp",
		Enabled:     true,
		AutoConnect: true,
	}).Error; err != nil {
		t.Fatalf("create mcp server: %v", err)
	}
	file := &ExportFile{
		Version: ExportVersion,
		Resources: ExportResources{
			MCPServers: []MCPServerExport{{
				Slug:      "github",
				Name:      "GitHub Import",
				Transport: "streamable",
				URL:       "https://import.example/mcp",
			}},
		},
	}
	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	analysis, err := AnalyzeImportDataWithContext(ctx, string(raw), nil, "")
	if err != nil {
		t.Fatalf("AnalyzeImportData() error = %v", err)
	}
	if analysis.ConflictCount != 1 || len(analysis.MCPServerConflicts) != 1 {
		t.Fatalf("MCP conflicts not detected: %+v", analysis)
	}
	conflict := analysis.MCPServerConflicts[0]
	if conflict.Identifier != "github" || conflict.ResourceType != "mcpServer" {
		t.Fatalf("unexpected conflict: %+v", conflict)
	}
}

func TestAnalyzeImportDataDetectsTaskListConflicts(t *testing.T) {
	setupPortabilityTestDB(t)

	taskList := createPortableTaskListFixture(t)

	file, err := BuildExportFileWithContext(portabilityTestCtx(), nil, nil, []string{taskList.ID}, nil, ExportRequest{}, "test")
	if err != nil {
		t.Fatalf("BuildExportFile() error = %v", err)
	}
	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	analysis, err := AnalyzeImportDataWithContext(portabilityTestCtx(), string(raw), nil, "")
	if err != nil {
		t.Fatalf("AnalyzeImportData() error = %v", err)
	}

	if analysis.TaskListCount != 1 {
		t.Fatalf("TaskListCount = %d, want 1", analysis.TaskListCount)
	}
	if analysis.TaskCount != 2 || analysis.TaskNoteCount != 1 {
		t.Fatalf("TaskCount/TaskNoteCount = %d/%d, want 2/1", analysis.TaskCount, analysis.TaskNoteCount)
	}
	if len(analysis.TaskListConflicts) != 0 {
		t.Fatalf("len(TaskListConflicts) = %d, want 0 for idempotent upsert by id", len(analysis.TaskListConflicts))
	}
}

func TestAnalyzeImportDataDetectsTaskListConflictsWithNormalizedSlug(t *testing.T) {
	setupPortabilityTestDB(t)

	taskList := createPortableTaskListFixture(t)

	file, err := BuildExportFileWithContext(portabilityTestCtx(), nil, nil, []string{taskList.ID}, nil, ExportRequest{}, "test")
	if err != nil {
		t.Fatalf("BuildExportFile() error = %v", err)
	}
	file.Resources.TaskLists[0].Slug = "  Sprint-42  "

	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	analysis, err := AnalyzeImportDataWithContext(portabilityTestCtx(), string(raw), nil, "")
	if err != nil {
		t.Fatalf("AnalyzeImportData() error = %v", err)
	}

	if len(analysis.TaskListConflicts) != 0 {
		t.Fatalf("len(TaskListConflicts) = %d, want 0 for idempotent upsert by id", len(analysis.TaskListConflicts))
	}
}

func TestImportConversationsImportsProviders(t *testing.T) {
	setupPortabilityTestDB(t)

	now := time.Now().UTC()
	file := &ExportFile{
		Version:    ExportVersion,
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

	result, err := ImportConversationsWithContext(portabilityTestCtx(), string(raw), nil, "")
	if err != nil {
		t.Fatalf("ImportConversations() error = %v", err)
	}
	if result.Imported != 1 || result.Skipped != 0 || result.Failed != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	imported, err := database.GetLLMProviderWithContext(portabilityTestCtx(), "ollama-local")
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

func TestImportConversationsRejectsProviderMissingRequiredFields(t *testing.T) {
	setupPortabilityTestDB(t)

	testCases := []struct {
		name     string
		provider ProviderExport
		want     string
	}{
		{
			name:     "name",
			provider: ProviderExport{ID: "provider-missing-name", Type: "openai", BaseURL: "https://api.example/v1"},
			want:     "sem name",
		},
		{
			name:     "type",
			provider: ProviderExport{ID: "provider-missing-type", Name: "Provider", BaseURL: "https://api.example/v1"},
			want:     "sem type",
		},
		{
			name:     "base url",
			provider: ProviderExport{ID: "provider-missing-base-url", Name: "Provider", Type: "openai"},
			want:     "sem baseUrl",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			file := &ExportFile{
				Version:    ExportVersion,
				ExportedAt: time.Now().UTC(),
				Resources: ExportResources{
					Providers: []ProviderExport{tc.provider},
				},
			}
			raw, err := json.Marshal(file)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}

			result, err := ImportConversationsWithContext(portabilityTestCtx(), string(raw), nil, "")
			if err != nil {
				t.Fatalf("ImportConversations() error = %v", err)
			}
			if result.Failed != 1 || len(result.Errors) != 1 || !strings.Contains(result.Errors[0], tc.want) {
				t.Fatalf("unexpected result: %+v", result)
			}
		})
	}
}

func TestImportConversationsPreservesProviderCreatedAtWhenOverwriteOmitsTimestamp(t *testing.T) {
	setupPortabilityTestDB(t)

	provider := createPortableProviderFixture(t)
	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Now().UTC(),
		Resources: ExportResources{
			Providers: []ProviderExport{
				{
					ID:                provider.ID,
					Name:              "OpenAI Custom Atualizado",
					Type:              provider.Type,
					APIFormat:         provider.APIFormat,
					BaseURL:           "https://updated.example/v1",
					Model:             provider.Model,
					DefaultModel:      provider.DefaultModel,
					IsDefault:         provider.IsDefault,
					Timeout:           provider.Timeout,
					CredentialPattern: provider.CredentialPattern,
				},
			},
		},
	}
	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	result, err := ImportConversationsWithContext(portabilityTestCtx(), string(raw), nil, "")
	if err != nil {
		t.Fatalf("ImportConversations() error = %v", err)
	}
	if result.Imported != 1 || result.Failed != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	imported, err := database.GetLLMProviderWithContext(portabilityTestCtx(), provider.ID)
	if err != nil {
		t.Fatalf("GetLLMProvider() error = %v", err)
	}
	if !imported.CreatedAt.Equal(provider.CreatedAt) {
		t.Fatalf("CreatedAt = %s, want %s", imported.CreatedAt, provider.CreatedAt)
	}
}

func TestImportConversationsImportsTaskLists(t *testing.T) {
	setupPortabilityTestDB(t)

	now := time.Now().UTC()
	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: now,
		Resources: ExportResources{
			TaskLists: []TaskListExport{
				{
					ID:                "01926b90-0000-7000-8000-000000000201",
					Title:             "Sprint 99",
					Slug:              "sprint-99",
					Description:       "Fechar lote pre-migracao",
					PreferredViewMode: "kanban",
					ValidationPolicy:  `{"task_code_regex":"^TASK-[0-9]+$"}`,
					CreatedAt:         now.Add(-48 * time.Hour),
					Workflow: TaskListWorkflowExport{
						ID: "01926b90-0000-7000-8000-000000000202",
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
							ID:           "01926b90-0000-7000-8000-000000000203",
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
									ID:                "01926b90-0000-7000-8000-000000000204",
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
									ID:          "01926b90-0000-7000-8000-000000000205",
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

	result, err := ImportConversationsWithContext(portabilityTestCtx(), string(raw), nil, "")
	if err != nil {
		t.Fatalf("ImportConversations() error = %v", err)
	}
	if result.Imported != 1 || result.Skipped != 0 || result.Failed != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	taskLists, err := database.GetAllTaskListsWithContext(portabilityTestCtx())
	if err != nil {
		t.Fatalf("GetAllTaskLists() error = %v", err)
	}
	if len(taskLists) != 1 {
		t.Fatalf("len(taskLists) = %d, want 1", len(taskLists))
	}
	importedTaskList, err := database.GetTaskListWithHierarchyWithContext(portabilityTestCtx(), taskLists[0].ID)
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
	notes, err := database.GetTaskNotesWithContext(portabilityTestCtx(), importedTaskList.Tasks[0].ID)
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

func TestImportConversationsUsesUTCFallbackForTaskListTimestamps(t *testing.T) {
	setupPortabilityTestDB(t)

	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Now().UTC(),
		Resources: ExportResources{
			TaskLists: []TaskListExport{
				{
					ID:                "01926b90-0000-7000-8000-000000000211",
					Title:             "Sem timestamps",
					Slug:              "sem-timestamps",
					PreferredViewMode: "list",
					Workflow: TaskListWorkflowExport{
						ID: "01926b90-0000-7000-8000-000000000212",
						Statuses: []TaskListWorkflowStatusExport{
							{ID: 1, Order: 0, Label: "Todo"},
						},
						AllowedTransitions: map[int][]int{},
						InitialStatusID:    1,
					},
					Tasks: []TaskExport{
						{ID: "01926b90-0000-7000-8000-000000000213", Title: "Task sem timestamp", StatusID: 1},
					},
				},
			},
		},
	}

	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	result, err := ImportConversationsWithContext(portabilityTestCtx(), string(raw), nil, "")
	if err != nil {
		t.Fatalf("ImportConversations() error = %v", err)
	}
	if result.Imported != 1 || result.Failed != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	taskLists, err := database.GetAllTaskListsWithContext(portabilityTestCtx())
	if err != nil {
		t.Fatalf("GetAllTaskLists() error = %v", err)
	}
	if len(taskLists) != 1 {
		t.Fatalf("len(taskLists) = %d, want 1", len(taskLists))
	}
	importedTaskList, err := database.GetTaskListWithHierarchyWithContext(portabilityTestCtx(), taskLists[0].ID)
	if err != nil {
		t.Fatalf("GetTaskListWithHierarchy() error = %v", err)
	}
	if importedTaskList.CreatedAt.Location() != time.UTC {
		t.Fatalf("tasklist CreatedAt location = %s, want UTC", importedTaskList.CreatedAt.Location())
	}
	if len(importedTaskList.Tasks) != 1 {
		t.Fatalf("len(Tasks) = %d, want 1", len(importedTaskList.Tasks))
	}
	if importedTaskList.Tasks[0].CreatedAt.Location() != time.UTC {
		t.Fatalf("task CreatedAt location = %s, want UTC", importedTaskList.Tasks[0].CreatedAt.Location())
	}
}

func TestImportConversationsPreservesTaskListCreatedAtWhenOverwriteOmitsTimestamp(t *testing.T) {
	setupPortabilityTestDB(t)

	taskList := createPortableTaskListFixture(t)
	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Now().UTC(),
		Resources: ExportResources{
			TaskLists: []TaskListExport{
				{
					ID:                taskList.ID,
					Title:             "Sprint 42 Atualizada",
					Slug:              "sprint-42",
					Description:       "Sem timestamp no arquivo",
					PreferredViewMode: "list",
					Workflow: TaskListWorkflowExport{
						ID: "01926b90-0000-7000-8000-000000000811",
						Statuses: []TaskListWorkflowStatusExport{
							{ID: 1, Order: 0, Label: "Todo"},
						},
						InitialStatusID: 1,
					},
					Tasks: []TaskExport{
						{ID: "01926b90-0000-7000-8000-000000000812", Title: "Nova task", StatusID: 1},
					},
				},
			},
		},
	}
	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	result, err := ImportConversationsWithContext(portabilityTestCtx(), string(raw), nil, "")
	if err != nil {
		t.Fatalf("ImportConversations() error = %v", err)
	}
	if result.Imported != 1 || result.Failed != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	imported, err := database.GetTaskListWithHierarchyWithContext(portabilityTestCtx(), taskList.ID)
	if err != nil {
		t.Fatalf("GetTaskListWithHierarchy() error = %v", err)
	}
	if !imported.CreatedAt.Equal(taskList.CreatedAt) {
		t.Fatalf("CreatedAt = %s, want %s", imported.CreatedAt, taskList.CreatedAt)
	}
}

func TestImportConversationsOverwritesConversationByID(t *testing.T) {
	setupPortabilityTestDB(t)

	createdAt := time.Date(2025, 4, 24, 10, 0, 0, 0, time.UTC)
	existing := &database.Conversation{
		UUIDModel: database.UUIDModel{
			CreatedAt: createdAt,
			UpdatedAt: createdAt,
		},
		UserID:  portabilityTestUserID,
		Title:   "Conversa importada",
		Channel: "telegram",
		Summary: "Resumo antigo",
	}
	if err := database.DB().Create(existing).Error; err != nil {
		t.Fatalf("Create(existing conversation) error = %v", err)
	}
	if err := database.DB().Create(&database.ChatMessage{
		UUIDModel: database.UUIDModel{
			CreatedAt: createdAt,
		},
		ConversationID: existing.ID,
		Role:           "user",
		Content:        "Mensagem antiga",
	}).Error; err != nil {
		t.Fatalf("Create(existing message) error = %v", err)
	}

	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Now().UTC(),
		Resources: ExportResources{
			Conversations: []ConversationExport{
				{
					ID:        existing.ID,
					Title:     "Conversa importada",
					Channel:   "telegram",
					Summary:   "Resumo novo",
					CreatedAt: createdAt,
					Messages: []MessageExport{
						{ID: "01926b90-0000-7000-8000-000000000301", Role: "user", Content: "Nova mensagem", CreatedAt: createdAt.Add(1 * time.Minute)},
						{ID: "01926b90-0000-7000-8000-000000000302", Role: "assistant", Content: "Resposta nova", CreatedAt: createdAt.Add(2 * time.Minute)},
					},
				},
			},
		},
	}

	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	result, err := ImportConversationsWithResolutions(portabilityTestCtx(), string(raw), nil, "", nil)
	if err != nil {
		t.Fatalf("ImportConversationsWithResolutions() error = %v", err)
	}
	if result.Imported != 1 || result.Skipped != 0 || result.Failed != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	conversations, err := database.GetConversationsWithContext(portabilityTestCtx())
	if err != nil {
		t.Fatalf("GetConversations() error = %v", err)
	}
	if len(conversations) != 1 {
		t.Fatalf("len(conversations) = %d, want 1", len(conversations))
	}

	imported, err := database.GetConversationWithContext(portabilityTestCtx(), conversations[0].ID)
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
		Version:    ExportVersion,
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

	result, err := ImportConversationsWithResolutions(portabilityTestCtx(), string(raw), nil, "", []ImportResolution{
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

	providers, err := database.GetLLMProvidersWithContext(portabilityTestCtx())
	if err != nil {
		t.Fatalf("GetLLMProviders() error = %v", err)
	}
	if len(providers) != 1 {
		t.Fatalf("len(providers) = %d, want 1 after idempotent overwrite by id", len(providers))
	}

	renamed, err := database.GetLLMProviderWithContext(portabilityTestCtx(), provider.ID)
	if err != nil {
		t.Fatalf("GetLLMProvider(renamed) error = %v", err)
	}
	if renamed.Name != "OpenAI Renamed Copy" || renamed.BaseURL != "https://renamed.example/v1" {
		t.Fatalf("unexpected renamed provider: %+v", renamed)
	}
}

func TestImportConversationsOverwritesTaskListByID(t *testing.T) {
	setupPortabilityTestDB(t)

	taskList := createPortableTaskListFixture(t)
	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Now().UTC(),
		Resources: ExportResources{
			TaskLists: []TaskListExport{
				{
					ID:                taskList.ID,
					Title:             "Sprint 42",
					Slug:              "sprint-42",
					Description:       "Workflow substituído",
					PreferredViewMode: "list",
					ValidationPolicy:  `{"task_code_regex":"^NEW-[0-9]+$"}`,
					CreatedAt:         taskList.CreatedAt.Add(2 * time.Hour),
					Workflow: TaskListWorkflowExport{
						ID: "01926b90-0000-7000-8000-000000000401",
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
							ID:        "01926b90-0000-7000-8000-000000000402",
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

	result, err := ImportConversationsWithResolutions(portabilityTestCtx(), string(raw), nil, "", nil)
	if err != nil {
		t.Fatalf("ImportConversationsWithResolutions() error = %v", err)
	}
	if result.Imported != 1 || result.Skipped != 0 || result.Failed != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	taskLists, err := database.GetAllTaskListsWithContext(portabilityTestCtx())
	if err != nil {
		t.Fatalf("GetAllTaskLists() error = %v", err)
	}
	if len(taskLists) != 1 {
		t.Fatalf("len(taskLists) = %d, want 1", len(taskLists))
	}

	importedTaskList, err := database.GetTaskListWithHierarchyWithContext(portabilityTestCtx(), taskLists[0].ID)
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

	taskList, err := database.CreateTaskListWithContext(portabilityTestCtx(), "Deep tree", "", nil, "deep-tree")
	if err != nil {
		t.Fatalf("CreateTaskList() error = %v", err)
	}

	ctx := portabilityTestCtx()
	root, err := database.CreateTaskFullWithContext(ctx, taskList.ID, "Root", "", "ROOT-1", "", "", "", "", "", nil)
	if err != nil {
		t.Fatalf("CreateTaskFull(root) error = %v", err)
	}
	child, err := database.CreateTaskFullWithContext(ctx, taskList.ID, "Child", "", "CHILD-1", "", "", "", "", "", &root.ID)
	if err != nil {
		t.Fatalf("CreateTaskFull(child) error = %v", err)
	}
	_, err = database.CreateTaskFullWithContext(ctx, taskList.ID, "Grandchild", "", "GRAND-1", "", "", "", "", "", &child.ID)
	if err != nil {
		t.Fatalf("CreateTaskFull(grandchild) error = %v", err)
	}

	hierarchy, err := database.GetTaskListWithHierarchyWithContext(portabilityTestCtx(), taskList.ID)
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

	_, err := AnalyzeImportData(`{"version":1,"resources":{"conversations":[]}}`, nil, "")
	if err == nil {
		t.Fatal("AnalyzeImportData() error = nil, want unsupported version error")
	}
	if !strings.Contains(err.Error(), "versão de exportação não suportada: 1") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestImportConversationsSkipsEmptyConversations(t *testing.T) {
	setupPortabilityTestDB(t)

	now := time.Now().UTC()
	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: now,
		Resources: ExportResources{
			Conversations: []ConversationExport{
				{Title: "Vazia", CreatedAt: now},
				{
					ID:        "01926b90-0000-7000-8000-000000000501",
					Title:     "Com mensagens",
					CreatedAt: now,
					Messages: []MessageExport{
						{ID: "01926b90-0000-7000-8000-000000000502", Role: "user", Content: "Oi", CreatedAt: now},
					},
				},
			},
		},
	}

	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("falha ao serializar export file: %v", err)
	}

	result, err := ImportConversationsWithContext(portabilityTestCtx(), string(raw), nil, "")
	if err != nil {
		t.Fatalf("ImportConversations() error = %v", err)
	}
	if result.Imported != 1 || result.Skipped != 1 {
		t.Fatalf("got imported=%d skipped=%d, want 1/1", result.Imported, result.Skipped)
	}
	if result.SkippedEmptyConversations != 1 {
		t.Fatalf("SkippedEmptyConversations = %d, want 1", result.SkippedEmptyConversations)
	}

	conversations, err := database.GetConversationsWithContext(portabilityTestCtx())
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
		Version:    ExportVersion,
		ExportedAt: now,
		Resources: ExportResources{
			Conversations: []ConversationExport{
				{
					ID:        "01926b90-0000-7000-8000-000000000511",
					Title:     "Com mensagens",
					CreatedAt: now,
					Messages: []MessageExport{
						{ID: "01926b90-0000-7000-8000-000000000512", Role: "user", Content: "Oi", CreatedAt: now},
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
	result, err := ImportConversationsWithContext(portabilityTestCtx(), rawString, nil, "")
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

	_, err := ImportConversationsWithContext(portabilityTestCtx(), `{"version":1,"resources":{"conversations":[]}}`, nil, "")
	if err == nil {
		t.Fatal("ImportConversations() error = nil, want unsupported version error")
	}
	if !strings.Contains(err.Error(), "versão de exportação não suportada: 1") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAnalyzeImportDataRejectsMissingCredentialBlock(t *testing.T) {
	setupPortabilityTestDB(t)

	_, err := AnalyzeImportData(`{"version":2,"options":{"includeCredentials":true},"resources":{"conversations":[]}}`, nil, "")
	if err == nil {
		t.Fatal("AnalyzeImportData() error = nil, want missing credential block error")
	}
	if !strings.Contains(err.Error(), "resources.credentials está ausente") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestImportConversationsRejectsMissingCredentialBlock(t *testing.T) {
	setupPortabilityTestDB(t)

	_, err := ImportConversationsWithContext(portabilityTestCtx(), `{"version":2,"options":{"includeCredentials":true},"resources":{"conversations":[]}}`, nil, "")
	if err == nil {
		t.Fatal("ImportConversations() error = nil, want missing credential block error")
	}
	if !strings.Contains(err.Error(), "resources.credentials está ausente") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestImportConversationsRejectsMissingStableConversationID(t *testing.T) {
	setupPortabilityTestDB(t)

	now := time.Now().UTC()
	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: now,
		Resources: ExportResources{
			Conversations: []ConversationExport{
				{
					Title:     "Sem id",
					CreatedAt: now,
					Messages: []MessageExport{
						{ID: "01926b90-0000-7000-8000-000000000701", Role: "user", Content: "Oi", CreatedAt: now},
					},
				},
			},
		},
	}
	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	result, err := ImportConversationsWithContext(portabilityTestCtx(), string(raw), nil, "")
	if err != nil {
		t.Fatalf("ImportConversations() error = %v", err)
	}
	if result.Failed != 1 || len(result.Errors) != 1 || !strings.Contains(result.Errors[0], "sem id") {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestImportConversationsRejectsMissingStableMessageID(t *testing.T) {
	setupPortabilityTestDB(t)

	now := time.Now().UTC()
	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: now,
		Resources: ExportResources{
			Conversations: []ConversationExport{
				{
					ID:        "01926b90-0000-7000-8000-000000000711",
					Title:     "Mensagem sem id",
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
		t.Fatalf("json.Marshal() error = %v", err)
	}

	result, err := ImportConversationsWithContext(portabilityTestCtx(), string(raw), nil, "")
	if err != nil {
		t.Fatalf("ImportConversations() error = %v", err)
	}
	if result.Failed != 1 || len(result.Errors) != 1 || !strings.Contains(result.Errors[0], "mensagem 0") {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestImportConversationsRejectsMissingStableTaskListIDs(t *testing.T) {
	setupPortabilityTestDB(t)

	testCases := []struct {
		name     string
		taskList TaskListExport
		want     string
	}{
		{
			name: "tasklist id",
			taskList: TaskListExport{
				Title: "Tasklist sem id",
				Workflow: TaskListWorkflowExport{
					ID: "01926b90-0000-7000-8000-000000000721",
					Statuses: []TaskListWorkflowStatusExport{
						{ID: 1, Label: "Todo"},
					},
					InitialStatusID: 1,
				},
			},
			want: "tasklist",
		},
		{
			name: "workflow id",
			taskList: TaskListExport{
				ID:    "01926b90-0000-7000-8000-000000000722",
				Title: "Workflow sem id",
				Workflow: TaskListWorkflowExport{
					Statuses: []TaskListWorkflowStatusExport{
						{ID: 1, Label: "Todo"},
					},
					InitialStatusID: 1,
				},
			},
			want: "workflow",
		},
		{
			name: "task id",
			taskList: TaskListExport{
				ID:    "01926b90-0000-7000-8000-000000000723",
				Title: "Task sem id",
				Workflow: TaskListWorkflowExport{
					ID: "01926b90-0000-7000-8000-000000000724",
					Statuses: []TaskListWorkflowStatusExport{
						{ID: 1, Label: "Todo"},
					},
					InitialStatusID: 1,
				},
				Tasks: []TaskExport{{Title: "Sem id", StatusID: 1}},
			},
			want: "task",
		},
		{
			name: "note id",
			taskList: TaskListExport{
				ID:    "01926b90-0000-7000-8000-000000000725",
				Title: "Nota sem id",
				Workflow: TaskListWorkflowExport{
					ID: "01926b90-0000-7000-8000-000000000726",
					Statuses: []TaskListWorkflowStatusExport{
						{ID: 1, Label: "Todo"},
					},
					InitialStatusID: 1,
				},
				Tasks: []TaskExport{
					{
						ID:       "01926b90-0000-7000-8000-000000000727",
						Title:    "Com nota",
						StatusID: 1,
						Notes:    []TaskNoteExport{{Type: int(database.TaskNoteAgent), Content: "sem id"}},
					},
				},
			},
			want: "nota",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			setupPortabilityTestDB(t)
			file := &ExportFile{
				Version:    ExportVersion,
				ExportedAt: time.Now().UTC(),
				Resources: ExportResources{
					TaskLists: []TaskListExport{tc.taskList},
				},
			}
			raw, err := json.Marshal(file)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			result, err := ImportConversationsWithContext(portabilityTestCtx(), string(raw), nil, "")
			if err != nil {
				t.Fatalf("ImportConversations() error = %v", err)
			}
			if result.Failed != 1 || len(result.Errors) != 1 || !strings.Contains(result.Errors[0], tc.want) {
				t.Fatalf("unexpected result: %+v", result)
			}
		})
	}
}

func TestImportConversationsReturnsDetailedSkipBreakdown(t *testing.T) {
	setupPortabilityTestDB(t)

	now := time.Now().UTC()
	existingConv := &database.Conversation{
		UUIDModel: database.UUIDModel{
			CreatedAt: now,
			UpdatedAt: now,
		},
		UserID:  portabilityTestUserID,
		Title:   "Duplicada",
		Channel: "telegram",
	}
	if err := database.DB().Create(existingConv).Error; err != nil {
		t.Fatalf("falha ao criar conversa existente: %v", err)
	}

	credMgr := credentials.NewManagerWithStoreAndPersistence([]byte("test-key-exactly-32-bytes-long!!"), credentials.NewDBStore(), true)
	if err := credMgr.RegisterPatternWithContext(portabilityTestCtx(), "api.openai.com", &credentials.AuthConfig{
		Type:  "bearer",
		Token: "secret",
	}); err != nil {
		t.Fatalf("falha ao registrar credencial existente: %v", err)
	}
	existingCreds, err := credMgr.ListCredentials()
	if err != nil || len(existingCreds) != 1 {
		t.Fatalf("ListCredentials() error = %v, len = %d", err, len(existingCreds))
	}

	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: now,
		Options: ExportOptions{
			IncludeCredentials: true,
		},
		Resources: ExportResources{
			Conversations: []ConversationExport{
				{Title: "Vazia", CreatedAt: now},
				{
					ID:        existingConv.ID,
					Title:     "Duplicada",
					Channel:   "telegram",
					CreatedAt: now,
					Messages: []MessageExport{
						{ID: "01926b90-0000-7000-8000-000000000601", Role: "user", Content: "Oi", CreatedAt: now},
					},
				},
				{
					ID:        "01926b90-0000-7000-8000-000000000602",
					Title:     "Nova",
					CreatedAt: now.Add(time.Second),
					Messages: []MessageExport{
						{ID: "01926b90-0000-7000-8000-000000000603", Role: "user", Content: "Mensagem", CreatedAt: now.Add(time.Second)},
					},
				},
			},
		},
	}

	blob, err := EncryptCredentialsPayload("senha-teste", []CredentialExport{
		{ID: existingCreds[0].ID, Pattern: "api.openai.com", AuthType: "bearer", Token: "secret"},
	})
	if err != nil {
		t.Fatalf("falha ao criptografar credenciais de teste: %v", err)
	}
	file.Resources.Credentials = blob

	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("falha ao serializar export file: %v", err)
	}

	result, err := ImportConversationsWithContext(portabilityTestCtx(), string(raw), credMgr, "senha-teste")
	if err != nil {
		t.Fatalf("ImportConversations() error = %v", err)
	}

	if result.Imported != 3 {
		t.Fatalf("Imported = %d, want 3", result.Imported)
	}
	if result.Skipped != 1 {
		t.Fatalf("Skipped = %d, want 1", result.Skipped)
	}
	if result.Failed != 0 {
		t.Fatalf("Failed = %d, want 0", result.Failed)
	}
	if result.SkippedEmptyConversations != 1 {
		t.Fatalf("SkippedEmptyConversations = %d, want 1", result.SkippedEmptyConversations)
	}
	if result.SkippedConversationConflict != 0 {
		t.Fatalf("SkippedConversationConflict = %d, want 0", result.SkippedConversationConflict)
	}
	if result.SkippedCredentialConflict != 0 {
		t.Fatalf("SkippedCredentialConflict = %d, want 0", result.SkippedCredentialConflict)
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
		Version:    ExportVersion,
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
	ctx := database.WithUserID(context.WithValue(context.Background(), ctxKey("source"), "test-import"), portabilityTestUserID)
	_, err = ImportConversationsWithContext(ctx, string(raw), credMgr, "senha-teste")
	if err != nil {
		t.Fatalf("ImportConversationsWithContext() error = %v", err)
	}

	if got := store.lastCtx.Value(ctxKey("source")); got != "test-import" {
		t.Fatalf("credential persistence ctx value = %v, want test-import", got)
	}
}

func TestExportCredentialsSkipsManagedAndInternalSecrets(t *testing.T) {
	setupPortabilityTestDB(t)

	credMgr := credentials.NewManagerWithStoreAndPersistence([]byte("test-key-exactly-32-bytes-long!!"), credentials.NewDBStore(), true)
	if err := credMgr.RegisterPatternWithContext(portabilityTestCtx(), "api.openai.com", &credentials.AuthConfig{
		Type:  "bearer",
		Token: "portable",
	}); err != nil {
		t.Fatalf("register portable credential: %v", err)
	}
	if err := credMgr.RegisterPatternWithContext(portabilityTestCtx(), "mcp-client:github", &credentials.AuthConfig{
		Type:         "oauth2_client_credentials",
		ClientID:     "managed-client",
		ClientSecret: "managed-secret",
	}); err != nil {
		t.Fatalf("register managed credential: %v", err)
	}
	if err := credMgr.RegisterInstanceSecret(credentials.InstanceSecretJWTSigningKey, "jwt-secret"); err != nil {
		t.Fatalf("register instance secret: %v", err)
	}

	file, err := BuildExportFileWithContext(portabilityTestCtx(), nil, nil, nil, credMgr, ExportRequest{
		IncludeCredentials:       true,
		CredentialExportPassword: "senha-teste",
	}, "test")
	if err != nil {
		t.Fatalf("BuildExportFileWithContext() error = %v", err)
	}
	exports, err := decodeCredentialExports(file.Resources.Credentials, "senha-teste")
	if err != nil {
		t.Fatalf("decode credentials: %v", err)
	}
	if len(exports) != 1 {
		t.Fatalf("exported credentials = %d, want 1: %#v", len(exports), exports)
	}
	if exports[0].Pattern != "api.openai.com" || exports[0].Token != "portable" {
		t.Fatalf("unexpected exported credential: %#v", exports[0])
	}
}

func TestImportCredentialsRejectsManagedPatterns(t *testing.T) {
	setupPortabilityTestDB(t)

	credMgr := credentials.NewManagerWithStoreAndPersistence([]byte("test-key-exactly-32-bytes-long!!"), credentials.NewDBStore(), true)
	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Now().UTC(),
		Options: ExportOptions{
			IncludeCredentials: true,
		},
	}
	blob, err := EncryptCredentialsPayload("senha-teste", []CredentialExport{
		{Pattern: credentials.InstanceSecretJWTSigningKey, AuthType: "bearer", Token: "jwt-secret"},
	})
	if err != nil {
		t.Fatalf("EncryptCredentialsPayload() error = %v", err)
	}
	file.Resources.Credentials = blob
	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	result, err := ImportConversationsWithContext(portabilityTestCtx(), string(raw), credMgr, "senha-teste")
	if err != nil {
		t.Fatalf("ImportConversations() returned unexpected top-level error: %v", err)
	}
	if result.Success || result.Failed != 1 {
		t.Fatalf("import result should fail managed credential import, got %+v", result)
	}
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0], "gerenciada/interna") {
		t.Fatalf("import errors = %#v, want managed/internal rejection", result.Errors)
	}
}

func TestImportConversationsOverwritesCredentialsByID(t *testing.T) {
	setupPortabilityTestDB(t)

	credMgr := credentials.NewManagerWithStoreAndPersistence([]byte("test-key-exactly-32-bytes-long!!"), credentials.NewDBStore(), true)
	if err := credMgr.RegisterPatternWithContext(portabilityTestCtx(), "api.openai.com", &credentials.AuthConfig{
		Type:  "bearer",
		Token: "token-antigo",
	}); err != nil {
		t.Fatalf("RegisterPatternWithContext(existing) error = %v", err)
	}
	existingCreds, err := credMgr.ListCredentials()
	if err != nil || len(existingCreds) != 1 {
		t.Fatalf("ListCredentials() error = %v, len = %d", err, len(existingCreds))
	}

	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Now().UTC(),
		Options: ExportOptions{
			IncludeCredentials: true,
		},
	}

	blob, err := EncryptCredentialsPayload("senha-teste", []CredentialExport{
		{ID: existingCreds[0].ID, Pattern: "api.openai.com", AuthType: "bearer", Token: "token-novo"},
	})
	if err != nil {
		t.Fatalf("EncryptCredentialsPayload() error = %v", err)
	}
	file.Resources.Credentials = blob

	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	result, err := ImportConversationsWithContext(portabilityTestCtx(), string(raw), credMgr, "senha-teste")
	if err != nil {
		t.Fatalf("ImportConversations() error = %v", err)
	}
	if result.Imported != 1 || result.Skipped != 0 || result.Failed != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	auth, err := credMgr.ResolveForURLWithContext(portabilityTestCtx(), "https://api.openai.com/v1/models")
	if err != nil {
		t.Fatalf("ResolveForURL() error = %v", err)
	}
	if auth == nil || auth.Token != "token-novo" {
		t.Fatalf("credential token = %v, want token-novo", auth)
	}
}

func TestImportConversationsSkipsCredentialConflictByPattern(t *testing.T) {
	setupPortabilityTestDB(t)

	credMgr := credentials.NewManagerWithStoreAndPersistence([]byte("test-key-exactly-32-bytes-long!!"), credentials.NewDBStore(), true)
	if err := credMgr.RegisterPatternWithContext(portabilityTestCtx(), "api.openai.com", &credentials.AuthConfig{
		Type:  "bearer",
		Token: "token-antigo",
	}); err != nil {
		t.Fatalf("RegisterPatternWithContext(existing) error = %v", err)
	}

	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Now().UTC(),
		Options: ExportOptions{
			IncludeCredentials: true,
		},
	}
	blob, err := EncryptCredentialsPayload("senha-teste", []CredentialExport{
		{ID: "different-id", Pattern: "api.openai.com", AuthType: "bearer", Token: "token-novo"},
	})
	if err != nil {
		t.Fatalf("EncryptCredentialsPayload() error = %v", err)
	}
	file.Resources.Credentials = blob

	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	analysis, err := AnalyzeImportDataWithContext(portabilityTestCtx(), string(raw), credMgr, "senha-teste")
	if err != nil {
		t.Fatalf("AnalyzeImportData() error = %v", err)
	}
	if len(analysis.CredentialConflicts) != 1 {
		t.Fatalf("CredentialConflicts len = %d, want 1: %+v", len(analysis.CredentialConflicts), analysis.CredentialConflicts)
	}
	if analysis.CredentialConflicts[0].Identifier != "api.openai.com" {
		t.Fatalf("Credential conflict identifier = %q, want pattern", analysis.CredentialConflicts[0].Identifier)
	}

	result, err := ImportConversationsWithContext(portabilityTestCtx(), string(raw), credMgr, "senha-teste")
	if err != nil {
		t.Fatalf("ImportConversations() error = %v", err)
	}
	if result.Imported != 0 || result.SkippedCredentialConflict != 1 || result.Failed != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	auth, err := credMgr.ResolveForURLWithContext(portabilityTestCtx(), "https://api.openai.com/v1/models")
	if err != nil {
		t.Fatalf("ResolveForURL() error = %v", err)
	}
	if auth == nil || auth.Token != "token-antigo" {
		t.Fatalf("credential token = %v, want token-antigo", auth)
	}
}

func TestAnalyzeImportDataScopesCredentialConflictsByUser(t *testing.T) {
	setupPortabilityTestDB(t)

	userA := database.WithUserID(context.Background(), "user-a")
	userB := database.WithUserID(context.Background(), "user-b")
	credMgr := credentials.NewManagerWithStoreAndPersistence([]byte("test-key-exactly-32-bytes-long!!"), credentials.NewDBStore(), true)
	if err := credMgr.RegisterPatternWithContext(userB, "api.openai.com", &credentials.AuthConfig{
		Type:  "bearer",
		Token: "token-b",
	}); err != nil {
		t.Fatalf("RegisterPatternWithContext(existing) error = %v", err)
	}

	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Now().UTC(),
		Options: ExportOptions{
			IncludeCredentials: true,
		},
	}
	blob, err := EncryptCredentialsPayload("senha-teste", []CredentialExport{
		{ID: "different-id", Pattern: "api.openai.com", AuthType: "bearer", Token: "token-importado"},
	})
	if err != nil {
		t.Fatalf("EncryptCredentialsPayload() error = %v", err)
	}
	file.Resources.Credentials = blob
	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	analysis, err := AnalyzeImportDataWithContext(userA, string(raw), credMgr, "senha-teste")
	if err != nil {
		t.Fatalf("AnalyzeImportDataWithContext(userA) error = %v", err)
	}
	if len(analysis.CredentialConflicts) != 0 {
		t.Fatalf("userA should not see userB credential conflicts: %+v", analysis.CredentialConflicts)
	}

	analysis, err = AnalyzeImportDataWithContext(userB, string(raw), credMgr, "senha-teste")
	if err != nil {
		t.Fatalf("AnalyzeImportDataWithContext(userB) error = %v", err)
	}
	if len(analysis.CredentialConflicts) != 1 {
		t.Fatalf("userB should see own credential conflict: %+v", analysis.CredentialConflicts)
	}
}

func TestImportConversationsOverwritesCredentialConflictByPattern(t *testing.T) {
	setupPortabilityTestDB(t)

	credMgr := credentials.NewManagerWithStoreAndPersistence([]byte("test-key-exactly-32-bytes-long!!"), credentials.NewDBStore(), true)
	if err := credMgr.RegisterPatternWithContext(portabilityTestCtx(), "api.openai.com", &credentials.AuthConfig{
		Type:  "bearer",
		Token: "token-antigo",
	}); err != nil {
		t.Fatalf("RegisterPatternWithContext(existing) error = %v", err)
	}

	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Now().UTC(),
		Options: ExportOptions{
			IncludeCredentials: true,
		},
	}
	blob, err := EncryptCredentialsPayload("senha-teste", []CredentialExport{
		{ID: "different-id", Pattern: "api.openai.com", AuthType: "bearer", Token: "token-novo"},
	})
	if err != nil {
		t.Fatalf("EncryptCredentialsPayload() error = %v", err)
	}
	file.Resources.Credentials = blob

	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	result, err := ImportConversationsWithResolutions(portabilityTestCtx(), string(raw), credMgr, "senha-teste", []ImportResolution{
		{
			ResourceType: "credential",
			Identifier:   "api.openai.com",
			Strategy:     ConflictResolutionOverwrite,
		},
	})
	if err != nil {
		t.Fatalf("ImportConversationsWithResolutions() error = %v", err)
	}
	if result.Imported != 1 || result.SkippedCredentialConflict != 0 || result.Failed != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	auth, err := credMgr.ResolveForURLWithContext(portabilityTestCtx(), "https://api.openai.com/v1/models")
	if err != nil {
		t.Fatalf("ResolveForURL() error = %v", err)
	}
	if auth == nil || auth.Token != "token-novo" {
		t.Fatalf("credential token = %v, want token-novo", auth)
	}

	var count int64
	if err := database.DB().Model(&database.CredentialEntry{}).Where("pattern = ?", "api.openai.com").Count(&count).Error; err != nil {
		t.Fatalf("Count(CredentialEntry) error = %v", err)
	}
	if count != 1 {
		t.Fatalf("credential entries with pattern = %d, want 1", count)
	}
}
