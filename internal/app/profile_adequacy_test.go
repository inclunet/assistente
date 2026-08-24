package app

import (
	"testing"

	"assistente/internal/workspace"
)

func TestWorkspaceTabMatchesConversation(t *testing.T) {
	ws := &workspace.Workspace{
		Tabs: workspace.TabsState{
			Items: []workspace.Tab{{
				ID:             "tab-1",
				Type:           workspace.TabTypeChat,
				ConversationID: "conversation-1",
			}},
		},
	}

	if !workspaceTabMatchesConversation(ws, "tab-1", "conversation-1") {
		t.Fatal("aba vinculada deveria ser aceita")
	}
	for _, test := range []struct {
		tabID          string
		conversationID string
	}{
		{tabID: "tab-inexistente", conversationID: "conversation-1"},
		{tabID: "tab-1", conversationID: "outra-conversa"},
		{tabID: "tab-1", conversationID: ""},
	} {
		if workspaceTabMatchesConversation(ws, test.tabID, test.conversationID) {
			t.Fatalf("vínculo inválido aceito: %#v", test)
		}
	}
}
