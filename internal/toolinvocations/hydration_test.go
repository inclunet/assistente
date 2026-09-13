package toolinvocations

import (
	"context"
	"fmt"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/tools"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
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
				ToolCallID:    fmt.Sprintf("call-%d", call),
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
		ToolCallID:    "call-1",
		Status:        StatusSucceeded,
		Output:        `{"content":"retry-mais-recente"}`,
		QueuedAt:      base.Add(time.Hour),
	}
	retryCompleted := retry.QueuedAt.Add(time.Millisecond)
	retry.CompletedAt = &retryCompleted
	if err := testDB.Create(&retry).Error; err != nil {
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
