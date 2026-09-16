package database

import (
	"bytes"
	"errors"
	"fmt"
	stdlog "log"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type legacyMigrationChatMessage struct {
	UUIDModel
	ConversationID   string
	ParentID         *string
	TurnID           *string
	Role             string
	Content          string
	Reasoning        string
	Media            string
	Audio            string
	AudioMimeType    string
	ToolCalls        string
	ToolCallID       string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CacheReadTokens  int
	CacheWriteTokens int
	CacheMissTokens  int
	Model            string
	Source           string
	Pinned           bool
}

func (legacyMigrationChatMessage) TableName() string { return "chat_messages" }

type legacyMigrationJobRun struct {
	UUIDModel
	UserID        string
	JobID         string
	TriggerID     string
	Status        string
	QueuedAt      time.Time
	StartedAt     time.Time
	CompletedAt   *time.Time
	DurationMs    int64
	Error         string
	RetryCount    int
	IsDryRun      bool
	TriggerData   string
	EventsEmitted string
	ToolName      string
	Inputs        string
	Output        string
}

func (legacyMigrationJobRun) TableName() string { return "job_runs" }

func setupToolLedgerMigrationTest(t *testing.T) (*User, *User, *ToolCatalog) {
	t.Helper()
	database := newMigratorTestDB(t)
	fullAutoMigrate(t, database)
	for _, statement := range []string{
		`ALTER TABLE chat_messages ADD COLUMN tool_calls text`,
		`ALTER TABLE chat_messages ADD COLUMN tool_call_id text`,
		`ALTER TABLE job_runs ADD COLUMN tool_name text`,
		`ALTER TABLE job_runs ADD COLUMN inputs text`,
		`ALTER TABLE job_runs ADD COLUMN output text`,
	} {
		if err := database.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	previous := DB()
	SetDB(database)
	t.Cleanup(func() { SetDB(previous) })

	userA := &User{UUIDModel: UUIDModel{ID: "ledger-user-a"}, Username: "ledger-a", PasswordHash: "x", Role: UserRoleUser, IsActive: true}
	userB := &User{UUIDModel: UUIDModel{ID: "ledger-user-b"}, Username: "ledger-b", PasswordHash: "x", Role: UserRoleUser, IsActive: true}
	if err := database.Create([]*User{userA, userB}).Error; err != nil {
		t.Fatal(err)
	}
	catalog := &ToolCatalog{
		UUIDModel:          UUIDModel{ID: "ledger-tool-search"},
		Name:               "search",
		DisplayName:        "Search",
		Origin:             "builtin",
		AvailabilityStatus: "available",
	}
	if err := database.Create(catalog).Error; err != nil {
		t.Fatal(err)
	}
	return userA, userB, catalog
}

func TestToolLedgerBackfillChatIsoladoIdempotenteESemPerda(t *testing.T) {
	userA, userB, _ := setupToolLedgerMigrationTest(t)
	database := DB()
	now := time.Now().UTC()
	for index, user := range []*User{userA, userB} {
		conversationID := "ledger-conversation-" + user.ID
		turnID := "ledger-turn-" + user.ID
		conversation := Conversation{UUIDModel: UUIDModel{ID: conversationID}, UserID: user.ID, Title: "Legacy"}
		if err := database.Create(&conversation).Error; err != nil {
			t.Fatal(err)
		}
		assistant := legacyMigrationChatMessage{
			UUIDModel:      UUIDModel{ID: "ledger-assistant-" + user.ID, CreatedAt: now.Add(time.Duration(index) * time.Second)},
			ConversationID: conversationID,
			TurnID:         &turnID,
			Role:           "assistant",
			ToolCalls:      `[{"id":"call-` + user.ID + `","type":"function","function":{"name":"search","arguments":"{\"token\":\"segredo-` + user.ID + `\",\"query\":\"x\"}"}}]`,
		}
		result := legacyMigrationChatMessage{
			UUIDModel:      UUIDModel{ID: "ledger-result-" + user.ID, CreatedAt: assistant.CreatedAt.Add(time.Millisecond)},
			ConversationID: conversationID,
			TurnID:         &turnID,
			Role:           "tool",
			Content:        "resultado-" + user.ID,
			ToolCallID:     "call-" + user.ID,
		}
		if err := database.Create([]*legacyMigrationChatMessage{&assistant, &result}).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := migrateToolLedgerBackfill(database); err != nil {
		t.Fatalf("primeiro backfill: %v", err)
	}
	assertMigratedChatInvocations(t, 2)
	var states []ToolLedgerMigrationState
	if err := database.Order("user_id").Find(&states).Error; err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 {
		t.Fatalf("states=%d, esperado 2", len(states))
	}
	for _, state := range states {
		if state.State != toolLedgerStateBackfilled || state.LegacyRows != 1 || state.LedgerRows != 1 ||
			state.LegacyInputDigest == "" || state.LegacyOutputDigest == "" ||
			state.LegacyInputDigest != state.LedgerInputDigest ||
			state.LegacyOutputDigest != state.LedgerOutputDigest ||
			state.AmbiguousCount != 0 {
			t.Fatalf("estado inválido: %+v", state)
		}
	}

	if err := migrateToolLedgerBackfill(database); err != nil {
		t.Fatalf("segundo backfill: %v", err)
	}
	assertMigratedChatInvocations(t, 2)
	var legacyRows int64
	if err := database.Model(&ChatMessage{}).Where("role = 'tool'").Count(&legacyRows).Error; err != nil {
		t.Fatal(err)
	}
	if legacyRows != 2 {
		t.Fatalf("fase aditiva alterou legado: %d", legacyRows)
	}
}

func assertMigratedChatInvocations(t *testing.T, expected int64) {
	t.Helper()
	var rows []ToolInvocation
	if err := DB().Where("origin_type = ?", "chat").Order("user_id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if int64(len(rows)) != expected {
		t.Fatalf("invocações=%d, esperado %d", len(rows), expected)
	}
	for _, row := range rows {
		if row.ConversationID == nil || row.TurnID == nil || row.MigrationSourceKey == "" ||
			row.InputHash == "" || row.OutputHash == "" || row.ResultAvailability != "available" {
			t.Fatalf("invocação incompleta: %+v", row)
		}
		if strings.Contains(row.InputPreview, "segredo") || strings.Contains(row.OutputPreview, "resultado") {
			t.Fatalf("preview vazou payload: input=%q output=%q", row.InputPreview, row.OutputPreview)
		}
	}
}

func TestToolLedgerBackfillJobAdotaInvocacaoPublicada(t *testing.T) {
	userA, _, catalog := setupToolLedgerMigrationTest(t)
	database := DB()
	job := Job{
		UUIDModel:     UUIDModel{ID: "ledger-job"},
		UserID:        userA.ID,
		Slug:          "ledger-job",
		Name:          "Ledger job",
		Enabled:       true,
		ToolCatalogID: catalog.ID,
		ToolName:      "search",
		Inputs:        `{"query":"definition"}`,
	}
	trigger := JobTrigger{UUIDModel: UUIDModel{ID: "ledger-trigger"}, UserID: userA.ID, JobID: job.ID, Type: "manual", Enabled: true}
	now := time.Now().UTC()
	run := legacyMigrationJobRun{
		UUIDModel:   UUIDModel{ID: "ledger-run"},
		UserID:      userA.ID,
		JobID:       job.ID,
		TriggerID:   trigger.ID,
		Status:      "completed",
		QueuedAt:    now,
		StartedAt:   now,
		CompletedAt: &now,
		ToolName:    "search",
		Inputs:      `{"query":"runtime"}`,
		Output:      `{"items":[1]}`,
	}
	oldInvocation := ToolInvocation{
		UUIDModel:     UUIDModel{ID: "ledger-old-job-invocation"},
		UserID:        userA.ID,
		ToolCatalogID: catalog.ID,
		OriginType:    "job",
		OriginID:      job.ID,
		Status:        "completed",
		Input:         run.Inputs,
		Output:        run.Output,
		QueuedAt:      now,
	}
	if err := database.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&trigger).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&oldInvocation).Error; err != nil {
		t.Fatal(err)
	}

	if err := migrateToolLedgerBackfill(database); err != nil {
		t.Fatal(err)
	}
	var rows []ToolInvocation
	if err := database.Where("user_id = ?", userA.ID).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != oldInvocation.ID || rows[0].OriginType != "job_run" ||
		rows[0].OriginID != run.ID || rows[0].MigrationSourceKey != "job-run:"+run.ID+":1" {
		t.Fatalf("invocação publicada não foi adotada: %+v", rows)
	}
}

func TestToolLedgerBackfillJobLegadoComVariosRunsBloqueiaAssociacao(t *testing.T) {
	userA, _, catalog := setupToolLedgerMigrationTest(t)
	database := DB()
	job := Job{
		UUIDModel:     UUIDModel{ID: "ledger-ambiguous-job"},
		UserID:        userA.ID,
		Slug:          "ledger-ambiguous-job",
		Name:          "Ambiguous job",
		Enabled:       true,
		ToolCatalogID: catalog.ID,
		ToolName:      "search",
	}
	trigger := JobTrigger{UUIDModel: UUIDModel{ID: "ledger-ambiguous-trigger"}, UserID: userA.ID, JobID: job.ID, Type: "manual", Enabled: true}
	now := time.Now().UTC()
	runs := []legacyMigrationJobRun{
		{UUIDModel: UUIDModel{ID: "ledger-ambiguous-run-a"}, UserID: userA.ID, JobID: job.ID, TriggerID: trigger.ID, Status: "completed", QueuedAt: now, StartedAt: now, ToolName: "search", Inputs: `{}`, Output: `{"run":"a"}`},
		{UUIDModel: UUIDModel{ID: "ledger-ambiguous-run-b"}, UserID: userA.ID, JobID: job.ID, TriggerID: trigger.ID, Status: "completed", QueuedAt: now.Add(time.Second), StartedAt: now.Add(time.Second), ToolName: "search", Inputs: `{}`, Output: `{"run":"b"}`},
	}
	invocation := ToolInvocation{
		UUIDModel:     UUIDModel{ID: "ledger-ambiguous-job-invocation"},
		UserID:        userA.ID,
		ToolCatalogID: catalog.ID,
		OriginType:    "job",
		OriginID:      job.ID,
		Status:        "completed",
		Input:         `{}`,
		Output:        `{"legacy":true}`,
		QueuedAt:      now,
	}
	if err := database.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&trigger).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&runs).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&invocation).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateToolLedgerBackfill(database); !errors.Is(err, errMigrationDeferred) {
		t.Fatalf("origem job ambígua deveria adiar: %v", err)
	}
	var persisted ToolInvocation
	if err := database.First(&persisted, "id = ?", invocation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.OriginType != "job" || persisted.OriginID != job.ID {
		t.Fatalf("backfill associou run arbitrariamente: %+v", persisted)
	}
	if got := queryCount(t, database, `
		SELECT COUNT(*) FROM tool_ledger_migration_states
		 WHERE resource_type = 'job_run'
		   AND state = 'pending'
		   AND last_error_code = 'ambiguous_legacy_job_origin'`); got != 2 {
		t.Fatalf("states ambíguos=%d, esperado 2", got)
	}
}

func TestToolLedgerBackfillAmbiguidadeBloqueiaSemPerda(t *testing.T) {
	userA, _, _ := setupToolLedgerMigrationTest(t)
	database := DB()
	conversation := Conversation{UUIDModel: UUIDModel{ID: "ledger-ambiguous-conversation"}, UserID: userA.ID, Title: "Ambiguous"}
	turnID := "ledger-ambiguous-turn"
	assistant := legacyMigrationChatMessage{
		UUIDModel:      UUIDModel{ID: "ledger-ambiguous-assistant"},
		ConversationID: conversation.ID,
		TurnID:         &turnID,
		Role:           "assistant",
		ToolCalls:      `[{"id":"duplicate-call","function":{"name":"search","arguments":"{}"}}]`,
	}
	resultA := legacyMigrationChatMessage{UUIDModel: UUIDModel{ID: "ledger-result-a"}, ConversationID: conversation.ID, TurnID: &turnID, Role: "tool", ToolCallID: "duplicate-call", Content: "A"}
	resultB := legacyMigrationChatMessage{UUIDModel: UUIDModel{ID: "ledger-result-b"}, ConversationID: conversation.ID, TurnID: &turnID, Role: "tool", ToolCallID: "duplicate-call", Content: "B"}
	if err := database.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create([]*legacyMigrationChatMessage{&assistant, &resultA, &resultB}).Error; err != nil {
		t.Fatal(err)
	}

	err := migrateToolLedgerBackfill(database)
	if !errors.Is(err, errMigrationDeferred) {
		t.Fatalf("ambiguidade deveria adiar: %v", err)
	}
	var state ToolLedgerMigrationState
	if err := database.Where("resource_id = ?", conversation.ID).First(&state).Error; err != nil {
		t.Fatal(err)
	}
	if state.State != toolLedgerStatePending || state.AmbiguousCount != 1 || state.LastErrorCode != "duplicate_tool_result" {
		t.Fatalf("estado ambíguo inválido: %+v", state)
	}
	var invocationCount int64
	if err := database.Model(&ToolInvocation{}).Where("origin_type = 'chat'").Count(&invocationCount).Error; err != nil {
		t.Fatal(err)
	}
	if invocationCount != 0 {
		t.Fatalf("ambiguidade persistiu associação parcial: %d", invocationCount)
	}
}

func TestToolLedgerBackfillJSONInvalidoPermanecePending(t *testing.T) {
	userA, _, _ := setupToolLedgerMigrationTest(t)
	database := DB()
	conversation := Conversation{UUIDModel: UUIDModel{ID: "ledger-invalid-json-conversation"}, UserID: userA.ID, Title: "Invalid"}
	message := legacyMigrationChatMessage{
		UUIDModel:      UUIDModel{ID: "ledger-invalid-json-message"},
		ConversationID: conversation.ID,
		Role:           "assistant",
		ToolCalls:      `{json-invalido`,
	}
	if err := database.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateToolLedgerBackfill(database); !errors.Is(err, errMigrationDeferred) {
		t.Fatalf("JSON inválido deveria adiar: %v", err)
	}
	var state ToolLedgerMigrationState
	if err := database.Where("resource_id = ?", conversation.ID).First(&state).Error; err != nil {
		t.Fatal(err)
	}
	if state.State != toolLedgerStatePending || state.LastErrorCode != "invalid_tool_calls" {
		t.Fatalf("estado de JSON inválido: %+v", state)
	}
}

func TestToolLedgerBackfillRoleToolSemCallIDPermanecePending(t *testing.T) {
	userA, _, _ := setupToolLedgerMigrationTest(t)
	database := DB()
	conversation := Conversation{UUIDModel: UUIDModel{ID: "ledger-missing-call-conversation"}, UserID: userA.ID, Title: "Missing call"}
	message := legacyMigrationChatMessage{
		UUIDModel:      UUIDModel{ID: "ledger-missing-call-message"},
		ConversationID: conversation.ID,
		Role:           "tool",
		Content:        "resultado sem identidade",
	}
	if err := database.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateToolLedgerBackfill(database); !errors.Is(err, errMigrationDeferred) {
		t.Fatalf("resultado sem call id deveria adiar: %v", err)
	}
	var state ToolLedgerMigrationState
	if err := database.Where("resource_id = ?", conversation.ID).First(&state).Error; err != nil {
		t.Fatal(err)
	}
	if state.State != toolLedgerStatePending || state.LegacyRows != 1 ||
		state.LastErrorCode != "missing_call_identity" {
		t.Fatalf("resultado sem identidade foi dado como coberto: %+v", state)
	}
}

func TestToolLedgerBackfillNaoAssociaResultadoDeOutroTurno(t *testing.T) {
	userA, _, _ := setupToolLedgerMigrationTest(t)
	database := DB()
	conversation := Conversation{UUIDModel: UUIDModel{ID: "ledger-turn-mismatch-conversation"}, UserID: userA.ID, Title: "Turn mismatch"}
	assistantTurn := "ledger-assistant-turn"
	resultTurn := "ledger-result-turn"
	assistant := legacyMigrationChatMessage{
		UUIDModel:      UUIDModel{ID: "ledger-turn-mismatch-assistant"},
		ConversationID: conversation.ID,
		TurnID:         &assistantTurn,
		Role:           "assistant",
		ToolCalls:      `[{"id":"same-call","function":{"name":"search","arguments":"{}"}}]`,
	}
	result := legacyMigrationChatMessage{
		UUIDModel:      UUIDModel{ID: "ledger-turn-mismatch-result"},
		ConversationID: conversation.ID,
		TurnID:         &resultTurn,
		Role:           "tool",
		ToolCallID:     "same-call",
		Content:        "não associar",
	}
	if err := database.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create([]*legacyMigrationChatMessage{&assistant, &result}).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateToolLedgerBackfill(database); !errors.Is(err, errMigrationDeferred) {
		t.Fatalf("turno divergente deveria adiar: %v", err)
	}
	var state ToolLedgerMigrationState
	if err := database.Where("resource_id = ?", conversation.ID).First(&state).Error; err != nil {
		t.Fatal(err)
	}
	if state.State != toolLedgerStatePending || state.LastErrorCode != "tool_result_turn_mismatch" {
		t.Fatalf("resultado de outro turno foi associado: %+v", state)
	}
}

func TestToolLedgerBackfillRemoveCheckpointDeRecursoExcluido(t *testing.T) {
	userA, _, _ := setupToolLedgerMigrationTest(t)
	database := DB()
	state := ToolLedgerMigrationState{
		UserID:       userA.ID,
		ResourceType: toolLedgerResourceConversation,
		ResourceID:   "conversation-already-deleted",
		State:        toolLedgerStatePending,
	}
	if err := database.Create(&state).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateToolLedgerBackfill(database); err != nil {
		t.Fatalf("checkpoint órfão não deveria bloquear boot: %v", err)
	}
	if got := queryCount(t, database, "SELECT COUNT(*) FROM tool_ledger_migration_states WHERE id = ?", state.ID); got != 0 {
		t.Fatalf("checkpoint órfão permaneceu: %d", got)
	}
}

func TestToolLedgerBackfillDivergenciaDeHashConcluiSemSobrescreverLedger(t *testing.T) {
	userA, _, catalog := setupToolLedgerMigrationTest(t)
	database := DB()
	conversation := Conversation{UUIDModel: UUIDModel{ID: "ledger-hash-conversation"}, UserID: userA.ID, Title: "Hash"}
	turnID := "ledger-hash-turn"
	message := legacyMigrationChatMessage{
		UUIDModel:      UUIDModel{ID: "ledger-hash-message"},
		ConversationID: conversation.ID,
		TurnID:         &turnID,
		Role:           "assistant",
		ToolCalls:      `[{"id":"hash-call","function":{"name":"search","arguments":"{}"},"result":"LEGADO"}]`,
	}
	now := time.Now().UTC()
	invocation := ToolInvocation{
		UUIDModel:     UUIDModel{ID: "ledger-hash-existing"},
		UserID:        userA.ID,
		ToolCatalogID: catalog.ID,
		OriginType:    "chat",
		OriginID:      turnID,
		ToolCallID:    "hash-call",
		Status:        "succeeded",
		Input:         `{}`,
		Output:        migratedOutput("CANONICO"),
		QueuedAt:      now,
	}
	if err := database.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&invocation).Error; err != nil {
		t.Fatal(err)
	}
	// Dados presentes com divergência apenas de hash: a reconciliação conclui
	// (backfilled) preservando o código para auditoria, sem adiar o boot.
	if err := migrateToolLedgerBackfill(database); err != nil {
		t.Fatalf("hash_mismatch benigno não deveria bloquear: %v", err)
	}
	var state ToolLedgerMigrationState
	if err := database.Where("resource_id = ?", conversation.ID).First(&state).Error; err != nil {
		t.Fatal(err)
	}
	if state.State != toolLedgerStateBackfilled || state.LastErrorCode != toolLedgerErrorHashMismatch ||
		state.LegacyOutputDigest == state.LedgerOutputDigest {
		t.Fatalf("hash_mismatch não concluído com aviso preservado: %+v", state)
	}
	// O gate do cutover não pode bloquear por divergência apenas de hash.
	if err := verifyToolLedgerCutoverGate(database); err != nil {
		t.Fatalf("gate do cutover bloqueou por hash_mismatch benigno: %v", err)
	}
	var persisted ToolInvocation
	if err := database.First(&persisted, "id = ?", invocation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Output != invocation.Output {
		t.Fatalf("backfill sobrescreveu ledger existente: %q", persisted.Output)
	}
}

// TestToolLedgerBackfillCountMismatchContinuaBloqueando garante que a perda
// real de linhas (count_mismatch) segue mantendo o recurso pendente e
// bloqueando o gate do cutover, mesmo após a flexibilização do hash_mismatch.
func TestToolLedgerBackfillCountMismatchContinuaBloqueando(t *testing.T) {
	userA, _, _ := setupToolLedgerMigrationTest(t)
	database := DB()
	blocking := ToolLedgerMigrationState{
		UserID:             userA.ID,
		ResourceType:       toolLedgerResourceConversation,
		ResourceID:         "ledger-count-mismatch",
		State:              toolLedgerStatePending,
		LegacyRows:         3,
		LedgerRows:         2,
		LastErrorCode:      toolLedgerErrorCountMismatch,
		LegacyInputDigest:  "in-legacy",
		LedgerInputDigest:  "in-legacy",
		LegacyOutputDigest: "out-legacy",
		LedgerOutputDigest: "out-legacy",
	}
	if err := database.Create(&blocking).Error; err != nil {
		t.Fatal(err)
	}
	var pending int64
	if err := database.Model(&ToolLedgerMigrationState{}).
		Where("state <> ? OR ambiguous_count > 0 OR last_error_code = ?",
			toolLedgerStateBackfilled, toolLedgerErrorCountMismatch).
		Count(&pending).Error; err != nil {
		t.Fatal(err)
	}
	if pending != 1 {
		t.Fatalf("count_mismatch deveria contar como pendente: %d", pending)
	}
	// Mesmo marcado como backfilled, count_mismatch precisa bloquear o gate:
	// representa perda real de linhas, não divergência benigna de hash.
	if err := database.Model(&blocking).Update("state", toolLedgerStateBackfilled).Error; err != nil {
		t.Fatal(err)
	}
	if err := verifyToolLedgerCutoverGate(database); err == nil {
		t.Fatal("gate do cutover deveria bloquear por count_mismatch")
	}
}

func TestToolLedgerBackfillOwnerVazioEhRetomavel(t *testing.T) {
	_, _, _ = setupToolLedgerMigrationTest(t)
	database := DB()
	conversation := Conversation{UUIDModel: UUIDModel{ID: "ledger-owner-pending"}, UserID: "", Title: "Owner pending"}
	assistant := legacyMigrationChatMessage{
		UUIDModel:      UUIDModel{ID: "ledger-owner-assistant"},
		ConversationID: conversation.ID,
		Role:           "assistant",
		ToolCalls:      `[{"id":"owner-call","function":{"name":"search","arguments":"{}"},"result":"ok"}]`,
	}
	if err := database.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&assistant).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateToolLedgerBackfill(database); !errors.Is(err, errMigrationDeferred) {
		t.Fatalf("owner vazio deveria adiar: %v", err)
	}
	if err := database.Model(&conversation).Update("user_id", "ledger-user-a").Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateToolLedgerBackfill(database); err != nil {
		t.Fatalf("retomada após owner: %v", err)
	}
	assertMigratedChatInvocations(t, 1)
}

func TestToolLedgerBackfillProcessaMaisDeUmLote(t *testing.T) {
	userA, _, _ := setupToolLedgerMigrationTest(t)
	database := DB()
	for index := 0; index < 101; index++ {
		conversationID := fmt.Sprintf("ledger-batch-conversation-%03d", index)
		messageID := fmt.Sprintf("ledger-batch-message-%03d", index)
		conversation := Conversation{UUIDModel: UUIDModel{ID: conversationID}, UserID: userA.ID, Title: "Batch"}
		message := legacyMigrationChatMessage{
			UUIDModel:      UUIDModel{ID: messageID},
			ConversationID: conversationID,
			Role:           "assistant",
			ToolCalls:      fmt.Sprintf(`[{"id":"call-%03d","function":{"name":"search","arguments":"{}"},"result":"ok"}]`, index),
		}
		if err := database.Create(&conversation).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.Create(&message).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := migrateToolLedgerBackfill(database); err != nil {
		t.Fatal(err)
	}
	assertMigratedChatInvocations(t, 101)
	if got := queryCount(t, database, "SELECT COUNT(*) FROM tool_ledger_migration_states WHERE state = 'backfilled'"); got != 101 {
		t.Fatalf("states concluídos=%d, esperado 101", got)
	}
}

func TestToolLedgerBackfillRetomaDepoisDeFalhaEntreRecursos(t *testing.T) {
	userA, _, _ := setupToolLedgerMigrationTest(t)
	database := DB()
	for _, suffix := range []string{"a", "b"} {
		conversationID := "ledger-crash-conversation-" + suffix
		conversation := Conversation{UUIDModel: UUIDModel{ID: conversationID}, UserID: userA.ID, Title: "Crash " + suffix}
		message := legacyMigrationChatMessage{
			UUIDModel:      UUIDModel{ID: "ledger-crash-message-" + suffix},
			ConversationID: conversationID,
			Role:           "assistant",
			ToolCalls:      `[{"id":"crash-` + suffix + `","function":{"name":"search","arguments":"{}"},"result":"ok"}]`,
		}
		if err := database.Create(&conversation).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.Create(&message).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := seedToolLedgerMigrationStates(database); err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&ToolLedgerMigrationState{}).
		Where("resource_id = ?", "ledger-crash-conversation-a").
		Update("id", "00000000-0000-7000-8000-000000000001").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&ToolLedgerMigrationState{}).
		Where("resource_id = ?", "ledger-crash-conversation-b").
		Update("id", "00000000-0000-7000-8000-000000000002").Error; err != nil {
		t.Fatal(err)
	}

	callbackName := "test:tool_ledger_crash"
	if err := database.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		invocation, ok := tx.Statement.Dest.(*ToolInvocation)
		if ok && strings.Contains(invocation.MigrationSourceKey, "crash-b") {
			_ = tx.AddError(errors.New("falha sintética entre recursos"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	err := migrateToolLedgerBackfill(database)
	if removeErr := database.Callback().Create().Remove(callbackName); removeErr != nil {
		t.Fatal(removeErr)
	}
	if err == nil || !strings.Contains(err.Error(), "falha sintética") {
		t.Fatalf("falha sintética não propagada: %v", err)
	}
	if got := queryCount(t, database, "SELECT COUNT(*) FROM tool_invocations WHERE origin_type = 'chat'"); got != 1 {
		t.Fatalf("primeiro recurso transacionado não foi preservado: %d", got)
	}

	if err := migrateToolLedgerBackfill(database); err != nil {
		t.Fatalf("retomada: %v", err)
	}
	assertMigratedChatInvocations(t, 2)
}

func TestToolLedgerBackfillNaoRegistraPayloadEmErroDePersistencia(t *testing.T) {
	userA, _, _ := setupToolLedgerMigrationTest(t)
	database := DB()
	conversation := Conversation{UUIDModel: UUIDModel{ID: "ledger-safe-log-conversation"}, UserID: userA.ID, Title: "Safe log"}
	message := legacyMigrationChatMessage{
		UUIDModel:      UUIDModel{ID: "ledger-safe-log-message"},
		ConversationID: conversation.ID,
		Role:           "assistant",
		ToolCalls:      `[{"id":"safe-log-call","function":{"name":"search","arguments":"{\"token\":\"NAO-PODE-VAZAR\"}"},"result":"RESULTADO-NAO-PODE-VAZAR"}]`,
	}
	if err := database.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&message).Error; err != nil {
		t.Fatal(err)
	}

	var logs bytes.Buffer
	migrationDB := database.Session(&gorm.Session{Logger: logger.New(
		stdlog.New(&logs, "", 0),
		logger.Config{LogLevel: logger.Info, ParameterizedQueries: false},
	)})
	callbackName := "test:tool_ledger_payload_error"
	if err := migrationDB.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*ToolInvocation); ok {
			_ = tx.AddError(errors.New("falha sintética sem payload"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	err := migrateToolLedgerBackfill(migrationDB)
	if removeErr := migrationDB.Callback().Create().Remove(callbackName); removeErr != nil {
		t.Fatal(removeErr)
	}
	if err == nil {
		t.Fatal("falha sintética não propagada")
	}
	if output := logs.String(); strings.Contains(output, "NAO-PODE-VAZAR") {
		t.Fatalf("log expôs payload técnico: %s", output)
	}
}

func TestToolLedgerBackfillCompletaSnapshotDeMCPDryRunExistente(t *testing.T) {
	userA, _, catalog := setupToolLedgerMigrationTest(t)
	database := DB()
	now := time.Now().UTC()
	invocation := ToolInvocation{
		UUIDModel:     UUIDModel{ID: "ledger-existing-dry-run"},
		UserID:        userA.ID,
		ToolCatalogID: catalog.ID,
		OriginType:    "tool_catalog",
		OriginID:      catalog.ID,
		Status:        "succeeded",
		DryRun:        true,
		Input:         `{"token":"valor-sensivel"}`,
		Output:        `{"content":"resultado-sensivel","is_error":false}`,
		QueuedAt:      now,
	}
	if err := database.Create(&invocation).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateToolLedgerBackfill(database); err != nil {
		t.Fatal(err)
	}
	var migrated ToolInvocation
	if err := database.First(&migrated, "id = ?", invocation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if migrated.Attempt != 1 || migrated.DisplayName != catalog.DisplayName ||
		migrated.InputHash == "" || migrated.OutputHash == "" ||
		migrated.InputBytes == 0 || migrated.OutputBytes == 0 ||
		migrated.ResultAvailability != "available" {
		t.Fatalf("snapshot de invocação existente incompleto: %+v", migrated)
	}
	if strings.Contains(migrated.InputPreview, "valor-sensivel") ||
		strings.Contains(migrated.OutputPreview, "resultado-sensivel") {
		t.Fatalf("snapshot expôs conteúdo: input=%q output=%q", migrated.InputPreview, migrated.OutputPreview)
	}
}

func TestToolLedgerBackfillNaoMarcaInvocacaoNovaComoMigrada(t *testing.T) {
	userA, _, catalog := setupToolLedgerMigrationTest(t)
	database := DB()
	invocation := ToolInvocation{
		UUIDModel:          UUIDModel{ID: "ledger-runtime-complete"},
		UserID:             userA.ID,
		ToolCatalogID:      catalog.ID,
		OriginType:         "job_run",
		OriginID:           "runtime-run",
		ToolCallID:         "runtime-call",
		Attempt:            1,
		Status:             "succeeded",
		Input:              `{}`,
		Output:             `{}`,
		InputHash:          "sha256:runtime-input",
		OutputHash:         "sha256:runtime-output",
		ResultAvailability: "available",
		QueuedAt:           time.Now().UTC(),
	}
	if err := database.Create(&invocation).Error; err != nil {
		t.Fatal(err)
	}

	updated, err := backfillExistingInvocationMetadata(database)
	if err != nil {
		t.Fatal(err)
	}
	if updated != 0 {
		t.Fatalf("invocação canônica foi tratada como legado: updated=%d", updated)
	}
	var persisted ToolInvocation
	if err := database.First(&persisted, "id = ?", invocation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.MigrationProvenance != "" {
		t.Fatalf("invocação nova recebeu proveniência de migração: %q", persisted.MigrationProvenance)
	}
}

func TestToolLedgerMigrationStateRejeitaEstadoERecursoInvalidos(t *testing.T) {
	userA, _, _ := setupToolLedgerMigrationTest(t)
	database := DB()
	valid := ToolLedgerMigrationState{
		UserID:       userA.ID,
		ResourceType: toolLedgerResourceConversation,
		ResourceID:   "resource-unique",
		State:        toolLedgerStatePending,
	}
	if err := database.Create(&valid).Error; err != nil {
		t.Fatal(err)
	}
	duplicate := valid
	duplicate.UUIDModel = UUIDModel{}
	if err := database.Create(&duplicate).Error; err == nil {
		t.Fatal("índice deveria rejeitar estado duplicado por usuário/recurso")
	}
	invalidState := ToolLedgerMigrationState{
		UserID:       userA.ID,
		ResourceType: toolLedgerResourceConversation,
		ResourceID:   "resource-invalid-state",
		State:        "desconhecido",
	}
	if err := database.Create(&invalidState).Error; err == nil {
		t.Fatal("check deveria rejeitar estado desconhecido")
	}
	invalidResource := ToolLedgerMigrationState{
		UserID:       userA.ID,
		ResourceType: "desconhecido",
		ResourceID:   "resource-invalid-type",
		State:        toolLedgerStatePending,
	}
	if err := database.Create(&invalidResource).Error; err == nil {
		t.Fatal("check deveria rejeitar tipo de recurso desconhecido")
	}
}

func TestToolLedgerBackfillContinuoCapturaLegadoCriadoAposV18(t *testing.T) {
	userA, _, _ := setupToolLedgerMigrationTest(t)
	database := DB()
	var migrationV18 migration
	for _, candidate := range schemaMigrations {
		if candidate.Version == 18 {
			migrationV18 = candidate
			break
		}
	}
	if migrationV18.Name == "" {
		t.Fatal("migração v18 não encontrada")
	}
	if err := runMigrationList(database, phasePostAutoMigrate, []migration{migrationV18}); err != nil {
		t.Fatal(err)
	}

	conversation := Conversation{UUIDModel: UUIDModel{ID: "ledger-after-v18-conversation"}, UserID: userA.ID, Title: "After v18"}
	message := legacyMigrationChatMessage{
		UUIDModel:      UUIDModel{ID: "ledger-after-v18-message"},
		ConversationID: conversation.ID,
		Role:           "assistant",
		ToolCalls:      `[{"id":"after-v18-call","function":{"name":"search","arguments":"{}"},"result":"ok"}]`,
	}
	if err := database.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	if err := runMigrationList(database, phasePostAutoMigrate, []migration{migrationV18}); err != nil {
		t.Fatal(err)
	}
	if got := queryCount(t, database, "SELECT COUNT(*) FROM tool_invocations WHERE tool_call_id = 'after-v18-call'"); got != 0 {
		t.Fatalf("registro da v18 não deveria reexecutar sozinho: %d", got)
	}
	if err := migrateToolLedgerBackfill(database); err != nil {
		t.Fatalf("backfill contínuo: %v", err)
	}
	if got := queryCount(t, database, "SELECT COUNT(*) FROM tool_invocations WHERE tool_call_id = 'after-v18-call'"); got != 1 {
		t.Fatalf("backfill contínuo não capturou legado novo: %d", got)
	}
	later := time.Now().UTC().Add(time.Minute)
	secondMessage := legacyMigrationChatMessage{
		UUIDModel: UUIDModel{
			ID:        "ledger-after-v18-message-2",
			CreatedAt: later,
			UpdatedAt: later,
		},
		ConversationID: conversation.ID,
		Role:           "assistant",
		ToolCalls:      `[{"id":"after-v18-call-2","function":{"name":"search","arguments":"{}"},"result":"ok"}]`,
	}
	if err := database.Create(&secondMessage).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateToolLedgerBackfill(database); err != nil {
		t.Fatalf("backfill contínuo da mesma conversa: %v", err)
	}
	if got := queryCount(t, database, "SELECT COUNT(*) FROM tool_invocations WHERE conversation_id = ?", conversation.ID); got != 2 {
		t.Fatalf("backfill contínuo não reabriu conversa alterada: %d", got)
	}
	if err := migrateToolLedgerBackfill(database); err != nil {
		t.Fatalf("terceiro boot sem mudanças: %v", err)
	}
	if got := queryCount(t, database, "SELECT COUNT(*) FROM tool_invocations WHERE conversation_id = ?", conversation.ID); got != 2 {
		t.Fatalf("terceiro boot duplicou ledger: %d", got)
	}
	turnID := secondMessage.ID
	resultTime := later.Add(time.Minute)
	results := []legacyMigrationChatMessage{
		{UUIDModel: UUIDModel{ID: "ledger-after-v18-result-a", CreatedAt: resultTime, UpdatedAt: resultTime}, ConversationID: conversation.ID, TurnID: &turnID, Role: "tool", ToolCallID: "after-v18-call-2", Content: "A"},
		{UUIDModel: UUIDModel{ID: "ledger-after-v18-result-b", CreatedAt: resultTime, UpdatedAt: resultTime}, ConversationID: conversation.ID, TurnID: &turnID, Role: "tool", ToolCallID: "after-v18-call-2", Content: "B"},
	}
	if err := database.Create(&results).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateToolLedgerBackfill(database); !errors.Is(err, errMigrationDeferred) {
		t.Fatalf("novo legado ambíguo deveria reabrir estado: %v", err)
	}
	var state ToolLedgerMigrationState
	if err := database.Where("resource_id = ?", conversation.ID).First(&state).Error; err != nil {
		t.Fatal(err)
	}
	if state.State != toolLedgerStatePending || state.LastErrorCode != "duplicate_tool_result" {
		t.Fatalf("prova anterior não foi invalidada: %+v", state)
	}
}
