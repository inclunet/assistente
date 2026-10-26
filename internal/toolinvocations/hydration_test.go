package toolinvocations

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/tools"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestLoadChatToolInvocationDisplaysForTurnIDsPreservesCompletenessOrderAndLatestRetry(t *testing.T) {
	testDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := testDB.AutoMigrate(&database.ToolCatalog{}, &database.ToolInvocation{}); err != nil {
		t.Fatal(err)
	}
	previous := database.DB()
	database.SetDB(testDB)
	t.Cleanup(func() { database.SetDB(previous) })

	catalog := database.ToolCatalog{
		Name:               "echo",
		DisplayName:        "Echo",
		Origin:             tools.ToolOriginBuiltin,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
	}
	if err := testDB.Create(&catalog).Error; err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	turnIDs := make([]string, 61)
	for turn := range turnIDs {
		turnID := fmt.Sprintf("turn-%02d", turn)
		turnIDs[turn] = turnID
		for call := 0; call < 3; call++ {
			invocation := database.ToolInvocation{
				UserID:        "user-a",
				ToolCatalogID: catalog.ID,
				OriginType:    OriginChat,
				OriginID:      turnID,
				TurnID:        &turnID,
				ToolCallID:    fmt.Sprintf("call-%d", call),
				Attempt:       1,
				Status:        StatusSucceeded,
				Output:        fmt.Sprintf(`{"content":"%s-%d"}`, turnID, call),
				QueuedAt:      base.Add(time.Duration(turn*10+call) * time.Millisecond),
			}
			completed := invocation.QueuedAt.Add(time.Millisecond)
			invocation.CompletedAt = &completed
			if err := testDB.Create(&invocation).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	retry := database.ToolInvocation{
		UserID:        "user-a",
		ToolCatalogID: catalog.ID,
		OriginType:    OriginChat,
		OriginID:      turnIDs[0],
		TurnID:        &turnIDs[0],
		ToolCallID:    "call-1",
		Status:        StatusSucceeded,
		Attempt:       2,
		Output:        `{"content":"retry-mais-recente"}`,
		// Timestamp anterior à tentativa 1 de call-1: Attempt deve prevalecer.
		QueuedAt: base.Add(500 * time.Microsecond),
	}
	retryCompleted := retry.QueuedAt.Add(time.Millisecond)
	retry.CompletedAt = &retryCompleted
	if err := testDB.Create(&retry).Error; err != nil {
		t.Fatal(err)
	}
	blankCallID := retry
	blankCallID.ID = ""
	blankCallID.ToolCallID = "   "
	blankCallID.Output = `{"content":"não deve entrar"}`
	blankCallID.QueuedAt = base.Add(750 * time.Microsecond)
	if err := testDB.Create(&blankCallID).Error; err != nil {
		t.Fatal(err)
	}

	got, err := LoadChatToolInvocationDisplaysForTurnIDsWithUser(context.Background(), "user-a", turnIDs)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(turnIDs) {
		t.Fatalf("hidratação incompleta: %d turns de %d", len(got), len(turnIDs))
	}
	for _, turnID := range turnIDs {
		if len(got[turnID]) != 3 {
			t.Fatalf("turn %s: esperado 3 calls, recebido %+v", turnID, got[turnID])
		}
		for index, call := range got[turnID] {
			if call.ID != fmt.Sprintf("call-%d", index) {
				t.Fatalf("turn %s perdeu ordem cronológica: %+v", turnID, got[turnID])
			}
		}
	}
	if got[turnIDs[0]][1].Result != "retry-mais-recente" {
		t.Fatalf("retry mais recente não substituiu a tentativa anterior: %+v", got[turnIDs[0]][1])
	}
}

func TestLoadChatToolInvocationDisplaysUsesCanonicalTurnWithAssistantOrigin(t *testing.T) {
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

	conv := database.Conversation{UUIDModel: database.UUIDModel{ID: "conv-legacy"}, UserID: "user-a"}
	if err := testDB.Create(&conv).Error; err != nil {
		t.Fatal(err)
	}
	turnID := "turn-legacy"
	assistantID := "assistant-legacy"
	if err := testDB.Create(&[]database.ChatMessage{
		{UUIDModel: database.UUIDModel{ID: turnID}, ConversationID: conv.ID, Role: "user"},
		{UUIDModel: database.UUIDModel{ID: assistantID}, ConversationID: conv.ID, TurnID: &turnID, Role: "assistant"},
	}).Error; err != nil {
		t.Fatal(err)
	}
	catalog := database.ToolCatalog{Name: "legacy", DisplayName: "Legacy", Origin: tools.ToolOriginBuiltin}
	if err := testDB.Create(&catalog).Error; err != nil {
		t.Fatal(err)
	}
	completed := time.Now().UTC()
	if err := testDB.Create(&database.ToolInvocation{
		UserID: "user-a", ToolCatalogID: catalog.ID, OriginType: OriginChat,
		ConversationID: &conv.ID, TurnID: &turnID,
		OriginID: assistantID, ToolCallID: "call-legacy", Status: StatusSucceeded,
		Output: `{"content":"resultado"}`, QueuedAt: completed.Add(-time.Second), CompletedAt: &completed,
	}).Error; err != nil {
		t.Fatal(err)
	}

	got, err := LoadChatToolInvocationDisplaysForTurnIDsWithUser(context.Background(), "user-a", []string{turnID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got[turnID]) != 1 || got[turnID][0].Result != "resultado" {
		t.Fatalf("vínculo canônico não foi carregado: %+v", got)
	}
}

func TestCanonicalHistoryReadsUseScopedIndex(t *testing.T) {
	testDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := testDB.AutoMigrate(&database.ToolCatalog{}, &database.ToolInvocation{}); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`CREATE INDEX idx_tool_invocations_chat_turn ON tool_invocations(user_id,origin_type,turn_id,queued_at,id)`,
		`CREATE INDEX idx_tool_invocations_orphan_scan ON tool_invocations(user_id,origin_type,id)`,
	} {
		if err := testDB.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	log := &orphanPlanLogger{Interface: logger.Default}
	previous := database.DB()
	database.SetDB(testDB.Session(&gorm.Session{Logger: log}))
	t.Cleanup(func() { database.SetDB(previous) })
	if _, err := LoadChatToolInvocationDisplaysForTurnIDsWithUser(context.Background(), "user", []string{"turn-a", "turn-b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSummariesForTurnIDsWithUser(context.Background(), "user", []string{"turn-a", "turn-b"}); err != nil {
		t.Fatal(err)
	}
	queries := 0
	for _, sql := range log.statements {
		if !strings.Contains(sql, "resolved_turn_id") {
			continue
		}
		queries++
		if strings.Contains(sql, "legacy_message") {
			t.Fatalf("consulta ainda interpreta legado: %s", sql)
		}
		var plan []struct{ Detail string }
		if err := testDB.Raw("EXPLAIN QUERY PLAN " + sql).Scan(&plan).Error; err != nil {
			t.Fatal(err)
		}
		found := false
		for _, step := range plan {
			if strings.Contains(step.Detail, "idx_tool_invocations_chat_turn") && strings.Contains(step.Detail, "user_id=? AND origin_type=? AND turn_id=?") {
				found = true
			}
		}
		if !found {
			t.Fatalf("consulta varre fora dos turnos solicitados: %+v", plan)
		}
	}
	if queries != 2 {
		t.Fatalf("esperadas duas consultas produtivas, recebido %d", queries)
	}
}

func TestCanonicalHistoryPaginationPreservesEveryCallAndTurnOrder(t *testing.T) {
	testDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := testDB.AutoMigrate(&database.ToolCatalog{}, &database.ToolInvocation{}); err != nil {
		t.Fatal(err)
	}
	previous := database.DB()
	database.SetDB(testDB)
	t.Cleanup(func() { database.SetDB(previous) })
	base := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	// A ordem temporal global intercala os turnos; a página termina dentro do segundo.
	for _, turn := range []string{"turn-a", "turn-b"} {
		rows := make([]database.ToolInvocation, 1101)
		for i := range rows {
			rows[i] = database.ToolInvocation{
				UUIDModel: database.UUIDModel{ID: fmt.Sprintf("%s-%04d", turn, i)},
				UserID:    "user", OriginType: OriginChat, OriginID: turn, TurnID: &turn,
				ToolCallID: fmt.Sprintf("call-%04d", i), Status: StatusSucceeded,
				QueuedAt: base.Add(time.Duration(i/2) * time.Second), Output: `{"content":"preservado"}`,
			}
		}
		if err := testDB.CreateInBatches(rows, 20).Error; err != nil {
			t.Fatal(err)
		}
	}
	got, err := LoadChatToolInvocationDisplaysForTurnIDsWithUser(context.Background(), "user", []string{"turn-b", "turn-a"})
	if err != nil {
		t.Fatal(err)
	}
	for _, turn := range []string{"turn-a", "turn-b"} {
		calls := got[turn]
		if len(calls) != 1101 {
			t.Fatalf("%s: %d chamadas, esperado 1101", turn, len(calls))
		}
		for i, call := range calls {
			if call.ID != fmt.Sprintf("call-%04d", i) || call.Result != "preservado" {
				t.Fatalf("ordem/conteúdo perdido %s[%d]: %+v", turn, i, call)
			}
		}
	}
}
