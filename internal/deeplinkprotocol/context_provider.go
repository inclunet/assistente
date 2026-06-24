package deeplinkprotocol

import (
	"context"

	"assistente/internal/contextprovider"
)

const defaultPromptBudget = 1200
const protocolTruncationNotice = "... Additional deeplink protocol content omitted due to context budget."

const protocolBlock = `<deeplink_protocol>
link= values and assistente://... URIs are app deep links that identify Assistente resources, such as conversations, task lists, editor or terminal tabs, app routes, and editable resources. Reuse exact link= values as stable resource references in responses, including Markdown links when useful. Treat deep link URIs as opaque app identifiers, not filesystem paths; opening them navigates, opens, sends, or creates according to the URI, but does not grant content access by itself. When the ` + "`open_deep_link`" + ` tool is available and the user asks to open or navigate to a resource, call it with the exact assistente:// URI.
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
