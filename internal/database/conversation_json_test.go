package database

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestConversationParentConversationIDJSONCamelCase trava o contrato JSON do
// campo ParentConversationID em camelCase (AEP-0068), consistente com
// SubAgentRun.parentConversationId e o padrão camelCase do projeto.
func TestConversationParentConversationIDJSONCamelCase(t *testing.T) {
	b, err := json.Marshal(Conversation{ParentConversationID: "conv-1"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, `"parentConversationId":"conv-1"`) {
		t.Fatalf("esperava chave camelCase parentConversationId, veio: %s", s)
	}
	if strings.Contains(s, "parent_conversation_id") {
		t.Fatalf("não deveria conter a chave snake_case parent_conversation_id: %s", s)
	}
}
