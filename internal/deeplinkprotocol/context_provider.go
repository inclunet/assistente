package deeplinkprotocol

import (
	"context"

	"assistente/internal/contextprovider"
)

const defaultPromptBudget = 1200
const protocolTruncationNotice = "... Additional deeplink protocol content omitted due to context budget."

const protocolBlock = `<deeplink_protocol>
link= values and assistente://... URIs are app deep links that identify Assistente resources. Reuse exact link= values as stable references in responses, including Markdown links like [label](assistente://...) when useful. If a link= value is available, reuse it exactly instead of reconstructing it.

Common forms: assistente://conversation/{id}, assistente://conversation/new?message=..., assistente://conversation/{id}/send?message=..., assistente://tasklist/{id}, assistente://editor/{id}, assistente://terminal/{id}, assistente://editor/open?file=..., assistente://navigate/{route}, assistente://{resource}/new, assistente://{resource}/edit/{id}. Conversation links may include profile={slug}. URL-encode query parameters and path IDs when generating links.

Treat deep link URIs as opaque app identifiers, not filesystem paths; opening them navigates, opens, sends, or creates according to the URI, but does not grant content access by itself. Generate a new deep link only when you have the required target ID or parameters; do not invent resource IDs. When the ` + "`open_deep_link`" + ` tool is available and the user asks to open or navigate to a resource, call it with the exact assistente:// URI.
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
	content := contextprovider.TrimTaggedBlockToBudget(protocolBlock, "deeplink_protocol", protocolTruncationNotice, req.Budget(p.Name(), defaultPromptBudget))
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
