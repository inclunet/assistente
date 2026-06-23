package toolprotocol

import (
	"context"
	"strings"

	"assistente/internal/chat"
	"assistente/internal/contextprovider"
	"assistente/internal/tools"
)

const defaultPromptBudget = 4000
const protocolTruncationNotice = "... Additional tool selection protocol content omitted due to context budget."

type ContextProvider struct{}

func NewContextProvider() *ContextProvider {
	return &ContextProvider{}
}

func (p *ContextProvider) Name() string { return "tool_protocol" }

func (p *ContextProvider) Metadata() contextprovider.ProviderMetadata {
	return contextprovider.ProviderMetadata{
		Name:             p.Name(),
		DisplayName:      "Tool Protocol",
		Description:      "Stable instructions for catalog-first tool selection.",
		DefaultEnabled:   true,
		DefaultBudget:    defaultPromptBudget,
		SupportsSettings: false,
	}
}

func (p *ContextProvider) Build(_ context.Context, req contextprovider.BuildRequest) ([]contextprovider.Block, error) {
	if !catalogFirstActive(req) {
		return nil, nil
	}
	content := chat.CatalogFirstToolPrompt
	content = trimTaggedBlockToBudget(content, "tool_selection_protocol", protocolTruncationNotice, req.Budget(p.Name(), defaultPromptBudget))
	if content == "" {
		return nil, nil
	}
	return []contextprovider.Block{{
		Provider:   p.Name(),
		Name:       "tool_selection_protocol",
		Volatility: contextprovider.VolatilityStable,
		Priority:   8,
		Content:    content,
	}}, nil
}

func catalogFirstActive(req contextprovider.BuildRequest) bool {
	if !req.ToolCallingEnabled {
		return false
	}
	hasCatalog := false
	for _, name := range req.EnabledTools {
		if name == tools.ToolCatalogName {
			hasCatalog = true
			continue
		}
		if name == tools.LoadSkillName {
			continue
		}
		return false
	}
	return hasCatalog
}

func trimTaggedBlockToBudget(content string, tag string, truncationNotice string, budgetChars int) string {
	if budgetChars <= 0 {
		return ""
	}
	if runeLen(content) <= budgetChars {
		return content
	}
	prefix := "<" + tag + ">\n"
	suffix := "\n</" + tag + ">"
	if !strings.HasPrefix(content, prefix) || !strings.HasSuffix(content, suffix) {
		return truncateRunes(content, budgetChars)
	}
	notice := "\n" + truncationNotice
	minimal := prefix + truncationNotice + suffix
	if runeLen(minimal) > budgetChars {
		return truncateRunes(truncationNotice, budgetChars)
	}
	bodyBudget := budgetChars - runeLen(prefix) - runeLen(notice) - runeLen(suffix)
	if bodyBudget <= 0 {
		return minimal
	}
	body := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(content, prefix), suffix))
	bodyRunes := []rune(body)
	if len(bodyRunes) > bodyBudget {
		body = strings.TrimSpace(string(bodyRunes[:bodyBudget]))
	}
	if body == "" {
		return minimal
	}
	return prefix + body + notice + suffix
}

func truncateRunes(value string, budgetChars int) string {
	if budgetChars <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= budgetChars {
		return value
	}
	return strings.TrimSpace(string(runes[:budgetChars]))
}

func runeLen(value string) int {
	count := 0
	for range value {
		count++
	}
	return count
}
