package controllers

import (
	"assistente/internal/chat"
	"assistente/internal/toolinvocations"
	"testing"
)

func TestHistoricoPropagaPosicaoACP(t *testing.T) {
	offset := 0
	converted := chat.ToolInvocationSummariesToTurnSegments(map[string][]toolinvocations.Summary{"turn": {{Origin: "acp_agent", AssistantMessageID: "assistant", ACPTextOffset: &offset}}})
	call := converted["turn"][0]
	if call.ACPTextOffset == nil || *call.ACPTextOffset != 0 || call.AssistantMessageID != "assistant" {
		t.Fatalf("posição perdida: %+v", call)
	}
}
