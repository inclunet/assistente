package toolinvocations

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupProjectionReadTest(t *testing.T) *gorm.DB {
	t.Helper()
	testDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := testDB.AutoMigrate(
		&database.Conversation{},
		&database.ChatMessage{},
		&database.ToolCatalog{},
		&database.ToolInvocation{},
	); err != nil {
		t.Fatal(err)
	}
	previous := database.DB()
	database.SetDB(testDB)
	t.Cleanup(func() { database.SetDB(previous) })
	return testDB
}

func TestProjectionSummaryLeveEDetailsUserScoped(t *testing.T) {
	testDB := setupProjectionReadTest(t)
	convA := database.Conversation{UUIDModel: database.UUIDModel{ID: "conv-a"}, UserID: "user-a"}
	convB := database.Conversation{UUIDModel: database.UUIDModel{ID: "conv-b"}, UserID: "user-b"}
	if err := testDB.Create(&[]database.Conversation{convA, convB}).Error; err != nil {
		t.Fatal(err)
	}
	turnA := "turn-a"
	turnB := "turn-b"
	if err := testDB.Create(&[]database.ChatMessage{
		{UUIDModel: database.UUIDModel{ID: turnA}, ConversationID: convA.ID, Role: "user"},
		{UUIDModel: database.UUIDModel{ID: turnB}, ConversationID: convB.ID, Role: "user"},
	}).Error; err != nil {
		t.Fatal(err)
	}
	catalog := database.ToolCatalog{Name: "search", DisplayName: "Search", Origin: "builtin"}
	if err := testDB.Create(&catalog).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	invocation := database.ToolInvocation{
		UUIDModel: database.UUIDModel{ID: "inv-a"}, UserID: "user-a", ToolCatalogID: catalog.ID,
		OriginType: OriginChat, OriginID: turnA, ConversationID: &convA.ID, TurnID: &turnA,
		ToolCallID: "call-a", Status: StatusSucceeded, Attempt: 1,
		Input: `{"secret":"input-integral"}`, Output: `{"content":"output-integral"}`,
		Metadata:     `{"display":{"name":"Search","origin":"builtin","iteration":2}}`,
		InputPreview: `{"secret":"[redacted]"}`, OutputPreview: `{"content":"[redacted]"}`,
		InputBytes: 27, OutputBytes: 29, ResultAvailability: "available", QueuedAt: now,
	}
	spoofed := invocation
	spoofed.ID = "inv-spoofed"
	spoofed.ConversationID = &convB.ID
	spoofed.TurnID = &turnB
	spoofed.OriginID = turnB
	if err := testDB.Create(&[]database.ToolInvocation{invocation, spoofed}).Error; err != nil {
		t.Fatal(err)
	}

	var detailQueries atomic.Int32
	if err := testDB.Callback().Query().Before("gorm:query").
		Register("projection_details_query_count", func(tx *gorm.DB) {
			if tx.Statement != nil && tx.Statement.Table == "tool_invocations" {
				detailQueries.Add(1)
			}
		}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := testDB.Callback().Query().Remove("projection_details_query_count"); err != nil {
			t.Fatal(err)
		}
	})
	summaries, err := LoadSummariesForTurnIDsWithUser(context.Background(), "user-a", []string{turnA})
	if err != nil {
		t.Fatal(err)
	}
	if detailQueries.Load() != 1 {
		t.Fatalf("resumo executou %d queries", detailQueries.Load())
	}
	if len(summaries[turnA]) != 1 {
		t.Fatalf("resumos = %+v", summaries)
	}
	summaryJSON, err := json.Marshal(summaries[turnA][0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(summaryJSON), "input-integral") || strings.Contains(string(summaryJSON), "output-integral") {
		t.Fatalf("summary vazou payload integral: %s", summaryJSON)
	}
	if summaries[turnA][0].InvocationID != invocation.ID || !summaries[turnA][0].HasDetails {
		t.Fatalf("summary incompleto: %+v", summaries[turnA][0])
	}

	detailQueries.Store(0)
	details, err := LoadDetailsWithUser(context.Background(), "user-a", []string{invocation.ID, spoofed.ID})
	if err != nil {
		t.Fatal(err)
	}
	if detailQueries.Load() != 1 {
		t.Fatalf("detalhes executaram %d queries", detailQueries.Load())
	}
	if len(details) != 1 || details[0].InvocationID != invocation.ID {
		t.Fatalf("ownership não filtrou vínculo inválido: %+v", details)
	}
	if string(details[0].Input) != invocation.Input || string(details[0].Output) != invocation.Output {
		t.Fatalf("detalhes perderam payload: %+v", details[0])
	}
}

func TestProjectionDetailsValidaLimite(t *testing.T) {
	setupProjectionReadTest(t)
	ids := make([]string, MaxDetailBatchSize+1)
	for index := range ids {
		ids[index] = "inv-" + strings.Repeat("x", index+1)
	}
	if _, err := LoadDetailsWithUser(context.Background(), "user-a", ids); err == nil {
		t.Fatal("lote acima do limite deveria falhar")
	}
	duplicates := make([]string, MaxDetailBatchSize+1)
	for index := range duplicates {
		duplicates[index] = "inv-repetida"
	}
	if _, err := LoadDetailsWithUser(context.Background(), "user-a", duplicates); err == nil {
		t.Fatal("limite deve considerar também IDs repetidos recebidos")
	}
}

func TestProjectionSecurityOutcomeUsesBlockedPrecedence(t *testing.T) {
	testDB := setupProjectionReadTest(t)
	conv := database.Conversation{UUIDModel: database.UUIDModel{ID: "conv-security"}, UserID: "user-security"}
	if err := testDB.Create(&conv).Error; err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, metadata, expected string
	}{
		{"blocked-first", `{"security_signals":[{"version":1,"outcome":"blocked"},{"version":1,"outcome":"approved"}]}`, "blocked"},
		{"approved-only", `{"security_signals":[{"version":1,"outcome":"approved"}]}`, "approved"},
		{"null", `{"security_signals":null}`, ""},
		{"string", `{"security_signals":"oops"}`, ""},
		{"object", `{"security_signals":{"version":1,"outcome":"blocked"}}`, ""},
		{"scalar-array", `{"security_signals":["oops",null,2]}`, ""},
		{"version-2", `{"security_signals":[{"version":2,"outcome":"blocked"}]}`, ""},
		{"version-string", `{"security_signals":[{"version":"1","outcome":"blocked"}]}`, ""},
		{"version-string-junk", `{"security_signals":[{"version":"1junk","outcome":"blocked"}]}`, ""},
		{"version-float", `{"security_signals":[{"version":1.5,"outcome":"blocked"}]}`, ""},
		{"version-bool", `{"security_signals":[{"version":true,"outcome":"blocked"}]}`, ""},
	}
	turnIDs := make([]string, 0, len(cases))
	for index, tc := range cases {
		turn := "turn-security-" + tc.name
		turnIDs = append(turnIDs, turn)
		if err := testDB.Create(&database.ChatMessage{UUIDModel: database.UUIDModel{ID: turn}, ConversationID: conv.ID, Role: "user"}).Error; err != nil {
			t.Fatal(err)
		}
		if err := testDB.Create(&database.ToolInvocation{
			UUIDModel: database.UUIDModel{ID: "inv-security-" + tc.name}, UserID: conv.UserID,
			OriginType: OriginChat, OriginID: turn, ConversationID: &conv.ID, TurnID: &turn,
			ToolCallID: "call-security-" + tc.name, Status: StatusSucceeded, ResultAvailability: "available",
			Metadata: tc.metadata, QueuedAt: time.Now().UTC().Add(time.Duration(index) * time.Millisecond),
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	summaries, err := LoadSummariesForTurnIDsWithUser(context.Background(), conv.UserID, turnIDs)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		turn := "turn-security-" + tc.name
		if got := summaries[turn][0].SecurityOutcome; got != tc.expected {
			t.Errorf("%s: outcome=%q, esperado %q", tc.name, got, tc.expected)
		}
	}
}

func TestProjectionSummaryDeduplicaRetriesPeloCallID(t *testing.T) {
	testDB := setupProjectionReadTest(t)
	conv := database.Conversation{UUIDModel: database.UUIDModel{ID: "conv-retry"}, UserID: "user-retry"}
	turn := "turn-retry"
	if err := testDB.Create(&conv).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Create(&database.ChatMessage{UUIDModel: database.UUIDModel{ID: turn}, ConversationID: conv.ID, Role: "user"}).Error; err != nil {
		t.Fatal(err)
	}
	catalog := database.ToolCatalog{Name: "search", DisplayName: "Search", Origin: "builtin"}
	if err := testDB.Create(&catalog).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	rows := []database.ToolInvocation{
		// A tentativa terminal tem timestamp anterior de propósito: Attempt, e não
		// a cronologia reconstruída, deve decidir qual linha prevalece.
		{UUIDModel: database.UUIDModel{ID: "inv-retry-2"}, UserID: conv.UserID, ToolCatalogID: catalog.ID, OriginType: OriginChat, OriginID: turn, ConversationID: &conv.ID, TurnID: &turn, ToolCallID: "call-retry", Status: StatusSucceeded, Attempt: 2, OutputPreview: "concluiu", QueuedAt: now},
		{UUIDModel: database.UUIDModel{ID: "inv-retry-1"}, UserID: conv.UserID, ToolCatalogID: catalog.ID, OriginType: OriginChat, OriginID: turn, ConversationID: &conv.ID, TurnID: &turn, ToolCallID: "call-retry", Status: StatusFailed, Attempt: 1, OutputPreview: "falhou", QueuedAt: now.Add(time.Millisecond)},
		// Tentativa abandonada/incompleta não pode substituir a terminal, assim
		// como já ocorre na hidratação completa.
		{UUIDModel: database.UUIDModel{ID: "inv-retry-3"}, UserID: conv.UserID, ToolCatalogID: catalog.ID, OriginType: OriginChat, OriginID: turn, ConversationID: &conv.ID, TurnID: &turn, ToolCallID: "call-retry", Status: "legacy_unknown", Attempt: 3, OutputPreview: "incompleto", QueuedAt: now.Add(2 * time.Millisecond)},
	}
	if err := testDB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	summaries, err := LoadSummariesForTurnIDsWithUser(context.Background(), conv.UserID, []string{turn})
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries[turn]) != 1 || summaries[turn][0].InvocationID != "inv-retry-2" || summaries[turn][0].Status != StatusSucceeded {
		t.Fatalf("retry não foi projetado pela tentativa terminal: %+v", summaries[turn])
	}
}
