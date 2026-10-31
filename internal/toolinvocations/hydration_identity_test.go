package toolinvocations

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"assistente/internal/database"
)

func TestHydrationPreservesOpaqueIDsAndRetryIsolation(t *testing.T) {
	db := setupProjectionReadTest(t)
	catalog := database.ToolCatalog{Name: "echo", DisplayName: "Echo", Origin: "builtin"}
	if err := db.Create(&catalog).Error; err != nil {
		t.Fatal(err)
	}
	turns := []string{"turn", " turn "}
	calls := []string{"call", " call "}
	for turnIndex, turn := range turns {
		for callIndex, call := range calls {
			for attempt := 1; attempt <= 2; attempt++ {
				output, _ := json.Marshal(map[string]string{"content": fmt.Sprintf("%d/%d/%d", turnIndex, callIndex, attempt)})
				invocation := database.ToolInvocation{
					UserID: "user-a", ToolCatalogID: catalog.ID, OriginType: OriginChat, OriginID: turn, TurnID: &turn,
					ToolCallID: call, Attempt: attempt, Status: StatusSucceeded, Output: string(output),
					Metadata: `{"display":{"assistant_message_id":" assistant "}}`,
					// A tentativa mais nova é cronologicamente anterior: Attempt é autoritativo.
					QueuedAt: time.Unix(int64(3-attempt), 0),
				}
				if err := db.Create(&invocation).Error; err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	// Repetição atravessa o limite de lote sem reconstruir estado de tentativas.
	requested := []string{"", " "}
	for range 401 {
		requested = append(requested, turns...)
	}
	displays, err := LoadChatToolInvocationDisplaysForTurnIDsWithUser(context.Background(), "user-a", requested)
	if err != nil || len(displays) != 2 {
		t.Fatalf("turnos alterados: %+v, %v", displays, err)
	}
	results := make(map[string]map[string]string)
	for turn, calls := range displays {
		results[turn] = make(map[string]string)
		for _, call := range calls {
			results[turn][call.ID] = call.ModelResult
		}
	}
	for turnIndex, turn := range turns {
		if len(displays[turn]) != 2 || len(results[turn]) != 2 {
			t.Fatalf("chamadas colidiram no turno %q: %+v / %+v", turn, displays[turn], results[turn])
		}
		for _, display := range displays[turn] {
			if display.AssistantMessageID != " assistant " {
				t.Fatalf("referência da mensagem alterada: %+v", display)
			}
		}
		for callIndex, call := range calls {
			want := fmt.Sprintf("%d/%d/2", turnIndex, callIndex)
			if results[turn][call] != want {
				t.Fatalf("resultado de %q/%q = %q; esperado %q", turn, call, results[turn][call], want)
			}
		}
	}
	other, err := LoadChatToolInvocationDisplaysForTurnIDsWithUser(context.Background(), "other-user", requested)
	if err != nil || len(other) != 0 {
		t.Fatalf("vazamento entre usuários: %+v, %v", other, err)
	}
}
