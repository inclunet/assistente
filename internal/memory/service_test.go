package memory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"assistente/internal/configdir"
	"assistente/internal/contextprovider"
	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupMemoryService(t *testing.T) (*Service, context.Context, context.Context) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&database.User{}, &database.MemoryRecord{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	svc := NewService(NewDBStore(db))
	return svc, database.WithUserID(context.Background(), "user-a"), database.WithUserID(context.Background(), "user-b")
}

func TestServiceCreateDefaultsAndPromptBlock(t *testing.T) {
	svc, ctx, _ := setupMemoryService(t)

	record, err := svc.Create(ctx, RecordInput{
		Content:    "Usuário prefere respostas curtas.",
		LoadPolicy: LoadPolicyCore,
		Kind:       database.MemoryKindUserPreference,
		Tags:       []string{"style", "prefs"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if record.UserID != "user-a" {
		t.Fatalf("UserID = %q", record.UserID)
	}
	if record.Scope != database.MemoryScopeUser {
		t.Fatalf("Scope default = %q", record.Scope)
	}
	if record.Importance != 3 || record.Confidence != 80 {
		t.Fatalf("defaults importance/confidence = %d/%d", record.Importance, record.Confidence)
	}

	block, err := svc.PromptBlock(ctx, 500)
	if err != nil {
		t.Fatalf("prompt block: %v", err)
	}
	if block == "" || !containsAll(block, "<user_memory>", "Usuário prefere respostas curtas.") {
		t.Fatalf("prompt block inesperado: %q", block)
	}
}

func TestServiceImplementsContextProvider(t *testing.T) {
	svc, ctx, _ := setupMemoryService(t)
	if _, err := svc.Create(ctx, RecordInput{
		Content:    "Sempre responder em pt-BR.",
		LoadPolicy: LoadPolicyCore,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	blocks, err := svc.Build(ctx, contextprovider.BuildRequest{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	block := findMemoryBlock(blocks, "user_memory")
	if block == nil {
		t.Fatalf("user_memory block not found: %+v", blocks)
	}
	if block.Provider != "memory" || block.Volatility != contextprovider.VolatilitySlowDynamic {
		t.Fatalf("unexpected block metadata: %+v", block)
	}
	if !strings.Contains(block.Content, "Sempre responder em pt-BR.") {
		t.Fatalf("memory block missing record: %s", block.Content)
	}
}

func TestServiceBuildReturnsInstructionsAndErrorWhenPromptFails(t *testing.T) {
	svc := NewService(promptErrorStore{err: errors.New("db offline")})

	blocks, err := svc.Build(database.WithUserID(context.Background(), "user-a"), contextprovider.BuildRequest{})
	if err == nil {
		t.Fatal("expected prompt error")
	}
	block := findMemoryBlock(blocks, "memory_instructions")
	if block == nil {
		t.Fatalf("memory_instructions block not found: %+v", blocks)
	}
	if !strings.Contains(block.Content, "<memory_instructions>") {
		t.Fatalf("unexpected instructions block: %s", block.Content)
	}
}

func TestLegacyMemoryBlockIsSkippedForAuthenticatedUser(t *testing.T) {
	svc, ctx, _ := setupMemoryService(t)
	restore := setupLegacyMemoryFile(t, "memória legada global")
	defer restore()

	block, err := svc.PromptBlock(ctx, 500)
	if err != nil {
		t.Fatalf("PromptBlock authenticated: %v", err)
	}
	if block != "" {
		t.Fatalf("expected no authenticated legacy block, got %q", block)
	}

	block, err = svc.PromptBlock(context.Background(), 500)
	if err != nil {
		t.Fatalf("PromptBlock legacy: %v", err)
	}
	if !strings.Contains(block, "memória legada global") {
		t.Fatalf("expected unauthenticated legacy block, got %q", block)
	}
}

func TestServiceBuildUsesLegacyInstructionsWithoutAuthenticatedUser(t *testing.T) {
	svc, _, _ := setupMemoryService(t)
	restore := setupLegacyMemoryFile(t, "memória legada global")
	defer restore()

	blocks, err := svc.Build(context.Background(), contextprovider.BuildRequest{})
	if err != nil {
		t.Fatalf("Build legacy: %v", err)
	}
	instructions := findMemoryBlock(blocks, "memory_instructions")
	if instructions == nil {
		t.Fatalf("memory_instructions block not found: %+v", blocks)
	}
	if !containsAll(instructions.Content, "legacy compatibility mode", "Do not call the memory tool") {
		t.Fatalf("unexpected legacy instructions: %s", instructions.Content)
	}
	if block := findMemoryBlock(blocks, "user_memory"); block == nil || !strings.Contains(block.Content, "memória legada global") {
		t.Fatalf("expected legacy memory block, got: %+v", blocks)
	}
}

func TestServiceScopesRecordsByUser(t *testing.T) {
	svc, ctxA, ctxB := setupMemoryService(t)

	if _, err := svc.Create(ctxA, RecordInput{Content: "segredo do user a", LoadPolicy: LoadPolicyCore}); err != nil {
		t.Fatalf("create a: %v", err)
	}
	if _, err := svc.Create(ctxB, RecordInput{Content: "segredo do user b", LoadPolicy: LoadPolicyCore}); err != nil {
		t.Fatalf("create b: %v", err)
	}

	listA, err := svc.List(ctxA, Filter{IncludeArchived: true})
	if err != nil {
		t.Fatalf("list a: %v", err)
	}
	if len(listA.Records) != 1 || listA.Records[0].Content != "segredo do user a" {
		t.Fatalf("list a vazou dados: %+v", listA.Records)
	}
}

func TestUpdatePreservesOmittedFields(t *testing.T) {
	svc, ctx, _ := setupMemoryService(t)
	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	record, err := svc.Create(ctx, RecordInput{
		Content:    "conteúdo original",
		Summary:    "resumo original",
		LoadPolicy: LoadPolicyPinned,
		Kind:       database.MemoryKindDecision,
		Scope:      database.MemoryScopeWorkspace,
		ScopeRef:   "workspace-a",
		Tags:       []string{"aep", "cache"},
		Importance: 5,
		Confidence: 90,
		SourceType: "legacy_file",
		SourceID:   "memories.md",
		ExpiresAt:  &expiresAt,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	updated, err := svc.Patch(ctx, record.ID, RecordPatch{Content: stringPtr("conteúdo atualizado")})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Content != "conteúdo atualizado" {
		t.Fatalf("content = %q", updated.Content)
	}
	if updated.LoadPolicy != LoadPolicyPinned || updated.Kind != database.MemoryKindDecision || updated.Scope != database.MemoryScopeWorkspace {
		t.Fatalf("metadata resetada: %+v", updated)
	}
	if updated.ScopeRef != "workspace-a" || updated.SourceType != "legacy_file" || updated.SourceID != "memories.md" {
		t.Fatalf("campos opcionais resetados: %+v", updated)
	}
	if updated.Importance != 5 || updated.Confidence != 90 {
		t.Fatalf("scores resetados: %d/%d", updated.Importance, updated.Confidence)
	}
	if updated.ExpiresAt == nil || !updated.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expires_at resetado: %+v", updated.ExpiresAt)
	}
	if !containsAll(updated.Tags, "aep", "cache") {
		t.Fatalf("tags resetadas: %q", updated.Tags)
	}
}

func TestCompleteUpdateCanClearOptionalFields(t *testing.T) {
	svc, ctx, _ := setupMemoryService(t)
	record, err := svc.Create(ctx, RecordInput{
		Content:    "conteúdo original",
		Summary:    "resumo removível",
		LoadPolicy: LoadPolicyPinned,
		Kind:       database.MemoryKindDecision,
		Scope:      database.MemoryScopeWorkspace,
		ScopeRef:   "workspace-a",
		Tags:       []string{"remove"},
		Importance: 4,
		Confidence: 90,
		SourceType: "legacy_file",
		SourceID:   "memories.md",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	updated, err := svc.Update(ctx, record.ID, RecordInput{
		Content:    "conteúdo editado",
		Summary:    "",
		LoadPolicy: LoadPolicyPinned,
		Kind:       database.MemoryKindDecision,
		Scope:      database.MemoryScopeUser,
		ScopeRef:   "",
		Tags:       []string{},
		Importance: 4,
		Confidence: 90,
		SourceType: "",
		SourceID:   "",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Summary != "" || updated.ScopeRef != "" || updated.Tags != "" || updated.SourceType != "" || updated.SourceID != "" {
		t.Fatalf("campos opcionais não foram limpos: %+v", updated)
	}
}

func TestContextProviderFiltersScopedRecords(t *testing.T) {
	svc, ctx, _ := setupMemoryService(t)
	cases := []RecordInput{
		{Content: "memória de usuário", LoadPolicy: LoadPolicyCore},
		{Content: "memória da conversa certa", LoadPolicy: LoadPolicyPinned, Scope: database.MemoryScopeConversation, ScopeRef: "conv-a"},
		{Content: "memória de outra conversa", LoadPolicy: LoadPolicyPinned, Scope: database.MemoryScopeConversation, ScopeRef: "conv-b"},
		{Content: "memória do workspace", LoadPolicy: LoadPolicyAuto, Scope: database.MemoryScopeWorkspace, ScopeRef: "workspace-id-a"},
		{Content: "memória de outro workspace", LoadPolicy: LoadPolicyAuto, Scope: database.MemoryScopeWorkspace, ScopeRef: "workspace-b"},
		{Content: "memória do projeto", LoadPolicy: LoadPolicyPinned, Scope: database.MemoryScopeProject, ScopeRef: "project-a"},
		{Content: "memória de outro projeto", LoadPolicy: LoadPolicyPinned, Scope: database.MemoryScopeProject, ScopeRef: "project-b"},
	}
	for _, input := range cases {
		if _, err := svc.Create(ctx, input); err != nil {
			t.Fatalf("create %q: %v", input.Content, err)
		}
	}

	blocks, err := svc.Build(ctx, contextprovider.BuildRequest{
		ConversationID:  "conv-a",
		WorkspaceID:     "workspace-id-a",
		ProjectID:       "project-a",
		WorkspaceName:   "nome que não é id",
		CurrentUserText: "vamos falar sobre workspace",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	block := findMemoryBlock(blocks, "user_memory")
	if block == nil {
		t.Fatalf("user_memory block not found: %+v", blocks)
	}
	content := block.Content
	if !containsAll(content, "memória de usuário", "memória da conversa certa", "memória do workspace", "memória do projeto") {
		t.Fatalf("faltou memória esperada: %s", content)
	}
	if strings.Contains(content, "memória de outra conversa") || strings.Contains(content, "memória de outro workspace") || strings.Contains(content, "memória de outro projeto") {
		t.Fatalf("vazou memória fora de escopo: %s", content)
	}
}

func TestContextProviderFiltersAutoByRelevance(t *testing.T) {
	svc, ctx, _ := setupMemoryService(t)
	if _, err := svc.Create(ctx, RecordInput{Content: "Preferência sobre relatórios financeiros", LoadPolicy: LoadPolicyAuto}); err != nil {
		t.Fatalf("create relevant: %v", err)
	}
	if _, err := svc.Create(ctx, RecordInput{Content: "Convenção sobre deploy", LoadPolicy: LoadPolicyAuto}); err != nil {
		t.Fatalf("create unrelated: %v", err)
	}

	blocks, err := svc.Build(ctx, contextprovider.BuildRequest{CurrentUserText: "preciso revisar relatórios"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	block := findMemoryBlock(blocks, "user_memory")
	if block == nil || !strings.Contains(block.Content, "relatórios financeiros") {
		t.Fatalf("auto relevante não entrou: %+v", blocks)
	}
	if strings.Contains(block.Content, "deploy") {
		t.Fatalf("auto irrelevante entrou: %s", block.Content)
	}

	blocks, err = svc.Build(ctx, contextprovider.BuildRequest{CurrentUserText: "oi"})
	if err != nil {
		t.Fatalf("Build short query: %v", err)
	}
	if block := findMemoryBlock(blocks, "user_memory"); block != nil {
		t.Fatalf("auto sem relevância entrou: %+v", blocks)
	}
}

func TestPromptCandidatesEscapesLikeWildcardsInRelevanceText(t *testing.T) {
	svc, ctx, _ := setupMemoryService(t)
	if _, err := svc.Create(ctx, RecordInput{Content: "taxa de 100 percent", LoadPolicy: LoadPolicyAuto}); err != nil {
		t.Fatalf("create wildcard false positive: %v", err)
	}
	if _, err := svc.Create(ctx, RecordInput{Content: "taxa de 100% literal", LoadPolicy: LoadPolicyAuto}); err != nil {
		t.Fatalf("create literal match: %v", err)
	}

	records, err := svc.store.PromptCandidates(ctx, PromptCandidateFilter{
		LoadPolicies:  []string{LoadPolicyAuto},
		RelevanceText: "100%",
		Limit:         10,
	})
	if err != nil {
		t.Fatalf("PromptCandidates: %v", err)
	}
	if len(records) != 1 || !strings.Contains(records[0].Content, "100% literal") {
		t.Fatalf("LIKE wildcard was not escaped, got: %+v", records)
	}
}

func TestContextProviderDoesNotDropScopedPinnedAfterManyCandidates(t *testing.T) {
	svc, ctx, _ := setupMemoryService(t)
	for i := 0; i < 60; i++ {
		if _, err := svc.Create(ctx, RecordInput{
			Content:    "memória global irrelevante",
			LoadPolicy: LoadPolicyAuto,
			Importance: 5,
		}); err != nil {
			t.Fatalf("create auto %d: %v", i, err)
		}
	}
	if _, err := svc.Create(ctx, RecordInput{
		Content:    "memória da conversa atual",
		LoadPolicy: LoadPolicyPinned,
		Scope:      database.MemoryScopeConversation,
		ScopeRef:   "conv-atual",
		Importance: 1,
	}); err != nil {
		t.Fatalf("create scoped pinned: %v", err)
	}

	blocks, err := svc.Build(ctx, contextprovider.BuildRequest{ConversationID: "conv-atual"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if block := findMemoryBlock(blocks, "user_memory"); block == nil || !strings.Contains(block.Content, "memória da conversa atual") {
		t.Fatalf("scoped pinned was dropped: %+v", blocks)
	}
}

func TestContextProviderDoesNotDropCoreBehindManyAutoCandidates(t *testing.T) {
	svc, ctx, _ := setupMemoryService(t)
	for i := 0; i < 220; i++ {
		if _, err := svc.Create(ctx, RecordInput{
			Content:    "auto relevante sobre orçamento",
			LoadPolicy: LoadPolicyAuto,
			Importance: 5,
		}); err != nil {
			t.Fatalf("create auto %d: %v", i, err)
		}
	}
	if _, err := svc.Create(ctx, RecordInput{
		Content:    "memória core prioritária",
		LoadPolicy: LoadPolicyCore,
		Importance: 1,
	}); err != nil {
		t.Fatalf("create core: %v", err)
	}

	blocks, err := svc.Build(ctx, contextprovider.BuildRequest{CurrentUserText: "orçamento"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if block := findMemoryBlock(blocks, "user_memory"); block == nil || !strings.Contains(block.Content, "memória core prioritária") {
		t.Fatalf("core memory was dropped behind auto records: %+v", blocks)
	}
}

func TestPromptBlockTruncatesOversizedCore(t *testing.T) {
	svc, ctx, _ := setupMemoryService(t)
	if _, err := svc.Create(ctx, RecordInput{Content: strings.Repeat("x", 200), LoadPolicy: LoadPolicyCore}); err != nil {
		t.Fatalf("create: %v", err)
	}
	block, err := svc.PromptBlock(ctx, 80)
	if err != nil {
		t.Fatalf("PromptBlock: %v", err)
	}
	if block == "" || !strings.Contains(block, "...") {
		t.Fatalf("core memory should be truncated, got: %q", block)
	}
}

func TestPromptBlockTruncatesOversizedPinned(t *testing.T) {
	svc, ctx, _ := setupMemoryService(t)
	if _, err := svc.Create(ctx, RecordInput{Content: strings.Repeat("p", 200), LoadPolicy: LoadPolicyPinned}); err != nil {
		t.Fatalf("create: %v", err)
	}
	block, err := svc.PromptBlock(ctx, 80)
	if err != nil {
		t.Fatalf("PromptBlock: %v", err)
	}
	if block == "" || !strings.Contains(block, "...") {
		t.Fatalf("pinned memory should be truncated, got: %q", block)
	}
}

func TestPromptSelectorSkipsOversizedAutoAndKeepsSmallerAuto(t *testing.T) {
	now := time.Now()
	lines := NewPromptSelector().Select([]database.MemoryRecord{
		{
			Content:    "budget " + strings.Repeat("x", 300),
			LoadPolicy: LoadPolicyAuto,
			Kind:       database.MemoryKindHistoricalNote,
			Scope:      database.MemoryScopeUser,
			Importance: 5,
			UUIDModel:  database.UUIDModel{UpdatedAt: now},
		},
		{
			Content:    "budget pequeno",
			LoadPolicy: LoadPolicyAuto,
			Kind:       database.MemoryKindHistoricalNote,
			Scope:      database.MemoryScopeUser,
			Importance: 4,
			UUIDModel:  database.UUIDModel{UpdatedAt: now.Add(-time.Second)},
		},
	}, contextprovider.BuildRequest{CurrentUserText: "budget"}, 120)
	if len(lines) != 1 || !strings.Contains(lines[0].Line, "budget pequeno") {
		t.Fatalf("expected smaller auto to fit after oversized auto, got: %+v", lines)
	}
}

func TestScopedMemoryRequiresScopeRef(t *testing.T) {
	svc, ctx, _ := setupMemoryService(t)
	_, err := svc.Create(ctx, RecordInput{
		Content:    "memória sem referência",
		LoadPolicy: LoadPolicyPinned,
		Scope:      database.MemoryScopeConversation,
	})
	if err == nil {
		t.Fatal("expected scopeRef validation error")
	}
}

func TestArchiveRemovesFromDefaultList(t *testing.T) {
	svc, ctx, _ := setupMemoryService(t)
	record, err := svc.Create(ctx, RecordInput{Content: "lembrança arquivável", LoadPolicy: LoadPolicyPinned})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Archive(ctx, record.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	list, err := svc.List(ctx, Filter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.Records) != 0 {
		t.Fatalf("archived record apareceu na lista padrão: %+v", list.Records)
	}
	withArchived, err := svc.List(ctx, Filter{IncludeArchived: true})
	if err != nil {
		t.Fatalf("list archived: %v", err)
	}
	if len(withArchived.Records) != 1 || withArchived.Records[0].LoadPolicy != LoadPolicyArchived {
		t.Fatalf("archived record ausente: %+v", withArchived.Records)
	}
}

func TestUnarchiveRestoresPreviousLoadPolicy(t *testing.T) {
	svc, ctx, _ := setupMemoryService(t)
	record, err := svc.Create(ctx, RecordInput{Content: "lembrança fixa", LoadPolicy: LoadPolicyPinned})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	archived, err := svc.Archive(ctx, record.ID)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if archived.LoadPolicy != LoadPolicyArchived || archived.ArchivedFromPolicy != LoadPolicyPinned {
		t.Fatalf("archive did not persist previous policy: %+v", archived)
	}
	restored, err := svc.Unarchive(ctx, record.ID, "")
	if err != nil {
		t.Fatalf("unarchive: %v", err)
	}
	if restored.LoadPolicy != LoadPolicyPinned || restored.ArchivedFromPolicy != "" {
		t.Fatalf("unarchive did not restore previous policy: %+v", restored)
	}
}

func TestUpdateToArchivedStoresPreviousLoadPolicy(t *testing.T) {
	svc, ctx, _ := setupMemoryService(t)
	record, err := svc.Create(ctx, RecordInput{Content: "lembrança fixa", LoadPolicy: LoadPolicyPinned})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	updated, err := svc.Update(ctx, record.ID, RecordInput{
		Content:    record.Content,
		LoadPolicy: LoadPolicyArchived,
		Kind:       record.Kind,
		Scope:      record.Scope,
		Importance: record.Importance,
		Confidence: record.Confidence,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.ArchivedFromPolicy != LoadPolicyPinned {
		t.Fatalf("archivedFromPolicy = %q, want pinned", updated.ArchivedFromPolicy)
	}
}

func TestPatchPreservesArchivedFromPolicy(t *testing.T) {
	svc, ctx, _ := setupMemoryService(t)
	record, err := svc.Create(ctx, RecordInput{Content: "lembrança fixa", LoadPolicy: LoadPolicyPinned})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	archived, err := svc.Archive(ctx, record.ID)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	patched, err := svc.Patch(ctx, archived.ID, RecordPatch{Content: stringPtr("lembrança editada")})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if patched.LoadPolicy != LoadPolicyArchived || patched.ArchivedFromPolicy != LoadPolicyPinned {
		t.Fatalf("patch cleared archive metadata: %+v", patched)
	}
}

func TestListCanFilterArchivedExplicitly(t *testing.T) {
	svc, ctx, _ := setupMemoryService(t)
	record, err := svc.Create(ctx, RecordInput{Content: "lembrança arquivada", LoadPolicy: LoadPolicyPinned})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Archive(ctx, record.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	list, err := svc.List(ctx, Filter{LoadPolicies: []string{LoadPolicyArchived}})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.Records) != 1 || list.Records[0].ID != record.ID {
		t.Fatalf("archived explícito não retornou registro: %+v", list.Records)
	}
}

func containsAll(value string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(value, needle) {
			return false
		}
	}
	return true
}

func findMemoryBlock(blocks []contextprovider.Block, name string) *contextprovider.Block {
	for i := range blocks {
		if blocks[i].Name == name {
			return &blocks[i]
		}
	}
	return nil
}

func stringPtr(value string) *string {
	return &value
}

func setupLegacyMemoryFile(t *testing.T, content string) func() {
	t.Helper()
	previousWorkDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tempDir := t.TempDir()
	memoryDir := filepath.Join(tempDir, ".assistente", "memory")
	if err := os.MkdirAll(memoryDir, 0700); err != nil {
		t.Fatalf("mkdir memory dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(memoryDir, "memory.md"), []byte(content), 0600); err != nil {
		t.Fatalf("write memory.md: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	configdir.ResetForTests()
	return func() {
		_ = os.Chdir(previousWorkDir)
		configdir.ResetForTests()
	}
}

type promptErrorStore struct {
	err error
}

func (s promptErrorStore) List(context.Context, Filter) (ListResult, error) {
	return ListResult{}, s.err
}

func (s promptErrorStore) PromptCandidates(context.Context, PromptCandidateFilter) ([]database.MemoryRecord, error) {
	return nil, s.err
}

func (s promptErrorStore) Get(context.Context, string) (*database.MemoryRecord, error) {
	return nil, s.err
}

func (s promptErrorStore) Create(context.Context, *database.MemoryRecord) (*database.MemoryRecord, error) {
	return nil, s.err
}

func (s promptErrorStore) Upsert(context.Context, *database.MemoryRecord) (*database.MemoryRecord, error) {
	return nil, s.err
}

func (s promptErrorStore) Update(context.Context, string, map[string]any) (*database.MemoryRecord, error) {
	return nil, s.err
}

func (s promptErrorStore) Delete(context.Context, string) error {
	return s.err
}

func (s promptErrorStore) PolicySummary(context.Context) (PolicySummary, error) {
	return PolicySummary{}, s.err
}
