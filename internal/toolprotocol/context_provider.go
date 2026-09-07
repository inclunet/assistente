package toolprotocol

import (
	"context"

	"assistente/internal/chat"
	"assistente/internal/contextprovider"
	"assistente/internal/tools"
)

const defaultPromptBudget = 4000
const protocolTruncationNotice = "... Additional tool selection protocol content omitted due to context budget."
const unavailableStatusTruncationNotice = "... Additional tool availability status omitted due to context budget."

const unavailableStatusPrompt = `<tool_selection_status>
Dynamic tool discovery is unavailable because tool_catalog is unavailable.
No implicit tools were exposed.
The user can explicitly select tools in the profile editor.
</tool_selection_status>`

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
	if req.ImplicitToolSelectionUnavailable {
		content := contextprovider.TrimTaggedBlockToBudget(
			unavailableStatusPrompt,
			"tool_selection_status",
			unavailableStatusTruncationNotice,
			req.Budget(p.Name(), defaultPromptBudget),
		)
		if content == "" {
			return nil, nil
		}
		return []contextprovider.Block{{
			Provider:   p.Name(),
			Name:       "tool_selection_status",
			Volatility: contextprovider.VolatilityStable,
			Priority:   8,
			Content:    content,
		}}, nil
	}
	if !catalogFirstActive(req) {
		return nil, nil
	}
	content := chat.CatalogFirstToolPrompt
	content = contextprovider.TrimTaggedBlockToBudget(content, "tool_selection_protocol", protocolTruncationNotice, req.Budget(p.Name(), defaultPromptBudget))
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
