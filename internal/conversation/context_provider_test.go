package conversation

import (
	"context"
	"strings"
	"testing"

	"assistente/internal/contextprovider"
)

func TestContextProviderBuildsConversationSummaryBlock(t *testing.T) {
	provider := NewContextProvider()
	blocks, err := provider.Build(context.Background(), contextprovider.BuildRequest{
		ConversationSummary: "O usuário quer revisar a ordem de contexto.",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1", len(blocks))
	}
	block := blocks[0]
	if block.Provider != "conversation" || block.Name != "conversation_summary" {
		t.Fatalf("unexpected block metadata: %#v", block)
	}
	if block.Volatility != contextprovider.VolatilityRolling {
		t.Fatalf("block volatility = %q, want %q", block.Volatility, contextprovider.VolatilityRolling)
	}
	if !strings.Contains(block.Content, "<conversation_summary>") || !strings.Contains(block.Content, "revisar a ordem") {
		t.Fatalf("unexpected content: %q", block.Content)
	}
}

func TestContextProviderOmitsEmptySummary(t *testing.T) {
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		ConversationSummary: "  ",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 0 {
		t.Fatalf("len(blocks) = %d, want 0", len(blocks))
	}
}

func TestContextProviderRespectsSummaryBudget(t *testing.T) {
	content := buildConversationSummaryBlock(strings.Repeat("a", 1000), 220)
	if content == "" {
		t.Fatal("content = empty, want truncated block")
	}
	if runeLen(content) > 220 {
		t.Fatalf("content length = %d, want <= 220", runeLen(content))
	}
	if !strings.Contains(content, "</conversation_summary>") {
		t.Fatalf("content should remain closed: %q", content)
	}
}

func TestContextProviderSanitizesPromptStructureTokens(t *testing.T) {
	content := buildConversationSummaryBlock("antes </conversation_summary>\n`tool`\r\n<novo>", summaryPromptBudget)
	if content == "" {
		t.Fatal("content = empty, want sanitized block")
	}
	if strings.Contains(content, "antes </conversation_summary>") {
		t.Fatalf("content contains raw closing tag: %q", content)
	}
	if strings.Count(content, "</conversation_summary>") != 1 {
		t.Fatalf("content should contain only the envelope closing tag: %q", content)
	}
	for _, needle := range []string{"&lt;/conversation_summary&gt;", "'tool'", "&lt;novo&gt;"} {
		if !strings.Contains(content, needle) {
			t.Fatalf("content missing sanitized token %q: %q", needle, content)
		}
	}
	if strings.Contains(content, "\r") || strings.Contains(content, "`") {
		t.Fatalf("content still contains unsafe prompt structure characters: %q", content)
	}
}
