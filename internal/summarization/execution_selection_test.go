package summarization

import (
	"assistente/internal/chat"
	"assistente/internal/database"
	"assistente/internal/toolinvocations"
	"strings"
	"testing"
)

func TestSummarizationExecutionIdentitySurvivesRepeatedCallIDs(t *testing.T) {
	turn := "turn"
	messages := []chat.Message{{UUIDModel: database.UUIDModel{ID: "base-message"}, TurnID: &turn, Role: "assistant", Content: "base"}, {UUIDModel: database.UUIDModel{ID: "current-message"}, TurnID: &turn, Role: "assistant", Content: "atual"}}
	calls := map[string][]toolinvocations.ChatToolInvocationDisplay{turn: {
		{LedgerID: "base-call", ID: "same", ExecutionID: "base", AssistantMessageID: "base-message", ModelResult: "resultado anterior"},
		{LedgerID: "current-call", ID: "same", ExecutionID: "current", AssistantMessageID: "current-message", ModelResult: "resultado atual"},
		{LedgerID: "base-unbound", ID: "same", ExecutionID: "base", ExecutionDepth: 1, ModelResult: "base sem fala"},
		{LedgerID: "unbound-call", ID: "same", ExecutionID: "current", ModelResult: "em andamento"},
	}}
	results := assignUnscopedSummarizationInvocationResults(messages, summarizationInvocationResultsFromDisplays(calls))
	if len(results[turn]) != 4 {
		t.Fatalf("identidades colidiram: %+v", results)
	}
	if results[turn]["base-call"].AssistantMessageID != "base-message" || results[turn]["current-call"].AssistantMessageID != "current-message" {
		t.Fatalf("vínculos explícitos alterados: %+v", results)
	}
	if results[turn]["unbound-call"].AssistantMessageID != "" {
		t.Fatalf("inferiu vínculo por iteração: %+v", results)
	}

	prompt := buildSummarizationUserPrompt("", messages, results)
	for _, text := range []string{"resultado anterior", "resultado atual", "base sem fala", "em andamento"} {
		if strings.Count(prompt, text) != 1 {
			t.Fatalf("resultado perdido/duplicado: %s", prompt)
		}
	}
	if strings.Index(prompt, "base sem fala") > strings.Index(prompt, "em andamento") {
		t.Fatalf("ordem das execuções alterada: %s", prompt)
	}
	want := EstimateTokens("resultado anterior") + EstimateTokens("resultado atual") + EstimateTokens("base sem fala") + EstimateTokens("em andamento")
	repeated := append(append([]chat.Message{}, messages...), messages...)
	if got := estimateHydratedToolResultTokens(repeated, results); got != want {
		t.Fatalf("estimativa %d; esperado %d", got, want)
	}
}
