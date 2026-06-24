package deeplinkprotocol

import (
	"context"
	"strings"

	"assistente/internal/contextprovider"
)

const defaultPromptBudget = 1200
const protocolTruncationNotice = "... Additional deeplink protocol content omitted due to context budget."

const protocolBlock = `<deeplink_protocol>
link= values are app deep links for navigating or opening Assistente resources. Treat assistente://... URIs as opaque app resource identifiers, not filesystem paths. When the ` + "`open_deep_link`" + ` tool is available and the user asks to open a resource, call it with the exact URI.
</deeplink_protocol>`

type ContextProvider struct{}

func NewContextProvider() *ContextProvider {
	return &ContextProvider{}
}

func (p *ContextProvider) Name() string { return "deeplink_protocol" }

func (p *ContextProvider) Metadata() contextprovider.ProviderMetadata {
	return contextprovider.ProviderMetadata{
		Name:             p.Name(),
		DisplayName:      "Deeplink Protocol",
		Description:      "Stable instructions for app deep links.",
		DefaultEnabled:   true,
		DefaultBudget:    defaultPromptBudget,
		SupportsSettings: false,
	}
}

func (p *ContextProvider) Build(_ context.Context, req contextprovider.BuildRequest) ([]contextprovider.Block, error) {
	content := trimTaggedBlockToBudget(protocolBlock, "deeplink_protocol", protocolTruncationNotice, req.Budget(p.Name(), defaultPromptBudget))
	if content == "" {
		return nil, nil
	}
	return []contextprovider.Block{{
		Provider:   p.Name(),
		Name:       "deeplink_protocol",
		Volatility: contextprovider.VolatilityStable,
		Priority:   9,
		Content:    content,
	}}, nil
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
		return ""
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
