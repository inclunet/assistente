package conversation

import (
	"context"
	"strings"

	"assistente/internal/contextprovider"
)

const summaryPromptBudget = 12000

var summaryContentReplacer = strings.NewReplacer(
	"<", "&lt;",
	">", "&gt;",
	"`", "'",
	"\r", "",
)

type ContextProvider struct{}

func NewContextProvider() *ContextProvider {
	return &ContextProvider{}
}

func (p *ContextProvider) Name() string { return "conversation" }

func (p *ContextProvider) Metadata() contextprovider.ProviderMetadata {
	return contextprovider.ProviderMetadata{
		Name:             p.Name(),
		DisplayName:      "Conversation",
		Description:      "Rolling summary of earlier messages in the current conversation.",
		DefaultEnabled:   true,
		DefaultBudget:    summaryPromptBudget,
		SupportsSettings: false,
	}
}

func (p *ContextProvider) Build(_ context.Context, req contextprovider.BuildRequest) ([]contextprovider.Block, error) {
	content := buildConversationSummaryBlock(req.ConversationSummary, req.Budget(p.Name(), summaryPromptBudget))
	if content == "" {
		return nil, nil
	}
	return []contextprovider.Block{{
		Provider:   p.Name(),
		Name:       "conversation_summary",
		Volatility: contextprovider.VolatilityRolling,
		Priority:   100,
		Content:    content,
	}}, nil
}

func buildConversationSummaryBlock(summary string, budgetChars int) string {
	summary = sanitizeConversationSummary(summary)
	if summary == "" {
		return ""
	}
	if budgetChars <= 0 {
		budgetChars = summaryPromptBudget
	}
	const prefix = "<conversation_summary>\nSummary of earlier messages in this conversation (these messages are no longer in the context window but their content is captured below):\n\n"
	const suffix = "\n</conversation_summary>"
	contentBudget := budgetChars - runeLen(prefix) - runeLen(suffix)
	if contentBudget <= 0 {
		return ""
	}
	runes := []rune(summary)
	if len(runes) > contentBudget {
		summary = strings.TrimSpace(string(runes[:contentBudget]))
	}
	if summary == "" {
		return ""
	}
	return prefix + summary + suffix
}

func sanitizeConversationSummary(summary string) string {
	return strings.TrimSpace(summaryContentReplacer.Replace(strings.TrimSpace(summary)))
}

func runeLen(value string) int {
	count := 0
	for range value {
		count++
	}
	return count
}
