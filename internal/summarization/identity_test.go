package summarization

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/database"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestSummarizationPreservesOpaqueIDsFromLedgerToPrompt(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	previous := database.DB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(previous) })
	if err := db.AutoMigrate(&database.Conversation{}, &database.ChatMessage{}, &database.ToolCatalog{}, &database.ToolInvocation{}); err != nil {
		t.Fatal(err)
	}
	if err := database.MigrateChatTurns(db); err != nil {
		t.Fatal(err)
	}
	if err := database.MigrateChatExecutionContent(db); err != nil {
		t.Fatal(err)
	}
	catalog := database.ToolCatalog{Name: "echo", DisplayName: "Echo", Origin: "builtin"}
	if err := db.Create(&catalog).Error; err != nil {
		t.Fatal(err)
	}
	messages := []chat.Message{}
	markers := []string{}
	wantTokens := 0
	for index, turn := range []string{"turn", " turn "} {
		assistant := []string{"assistant", " assistant "}[index]
		messages = append(messages, chat.Message{UUIDModel: database.UUIDModel{ID: assistant}, Role: "assistant", TurnID: &turn, Content: fmt.Sprintf("RESPOSTA-%d", index)})
		for callIndex, call := range []string{"call", " call "} {
			marker := fmt.Sprintf("RESULTADO-%d-%d", index, callIndex)
			markers = append(markers, marker)
			wantTokens += EstimateTokens(marker)
			output, _ := json.Marshal(map[string]string{"content": marker})
			metadata, _ := json.Marshal(map[string]any{"display": map[string]string{"assistant_message_id": assistant}})
			inv := database.ToolInvocation{UserID: "user", ToolCatalogID: catalog.ID, OriginType: "chat", OriginID: turn,
				TurnID: &turn, ToolCallID: call, Status: "succeeded", Output: string(output), Metadata: string(metadata)}
			if err := db.Create(&inv).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	ctx := database.WithUserID(context.Background(), "user")
	results := loadSummarizationToolInvocationResults(ctx, messages)
	if len(results) != 2 || len(results["turn"]) != 2 || len(results[" turn "]) != 2 {
		t.Fatalf("identidades colidiram: %+v", results)
	}
	prompt := buildSummarizationUserPrompt("", messages, results)
	boundary := strings.Index(prompt, "RESPOSTA-1")
	for index, marker := range markers {
		position := strings.Index(prompt, marker)
		if strings.Count(prompt, marker) != 1 || (index < 2 && position >= boundary) || (index >= 2 && position < boundary) {
			t.Fatalf("resultado perdido, duplicado ou associado à resposta errada: %s\n%s", marker, prompt)
		}
	}
	// Repetir a mensagem não duplica resultados na estimativa nem no prompt.
	repeated := append(append([]chat.Message(nil), messages...), messages...)
	if got := estimateHydratedToolResultTokens(repeated, results); got != wantTokens {
		t.Fatalf("estimativa = %d, esperado %d", got, wantTokens)
	}
	for _, result := range results[" turn "] {
		if summarizationInvocationResultMatchesMessage(messages[0], result) {
			t.Fatal("mensagens com IDs distintos foram tratadas como iguais")
		}
	}
	// Sem vínculo explícito, a atribuição por turno também mantém as identidades.
	for turn, calls := range results {
		for call, result := range calls {
			result.AssistantMessageID = ""
			results[turn][call] = result
		}
	}
	if unscoped := buildSummarizationUserPrompt("", messages, results); unscoped != prompt {
		t.Fatalf("atribuição por turno perdeu identidade:\n%s", unscoped)
	}
}
