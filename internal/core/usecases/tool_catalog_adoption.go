package usecases

import (
	"context"
	"strings"

	"assistente/internal/chat"
	"assistente/internal/llm"
	"assistente/internal/tools"
)

type catalogDiscoveryStore interface {
	ListTools(context.Context, tools.ToolCatalogFilter) ([]tools.ToolCatalogEntry, error)
}

func isFirstConversationTurn(messages []llm.Message) bool {
	userMessages := 0
	for _, message := range messages {
		if message.Role == "user" {
			userMessages++
		}
	}
	return userMessages <= 1
}

// autoDiscoverReadOnlyTools executa a busca interna do primeiro turno. Ela só
// considera candidatas on_demand, disponíveis, de risco read e já visíveis
// para a política efetiva. Logo, não concede autorização nem contorna budget.
func autoDiscoverReadOnlyTools(
	ctx context.Context,
	store catalogDiscoveryStore,
	policy chat.EffectiveToolPolicy,
	query string,
	preferredPackages []string,
	recentNames []string,
) ([]string, error) {
	if store == nil || strings.TrimSpace(query) == "" {
		return nil, nil
	}
	visible := policy.CatalogVisibleNames()
	if len(visible) == 0 {
		return nil, nil
	}
	entries, err := store.ListTools(ctx, tools.ToolCatalogFilter{
		NameIn:             visible,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
		Limit:              tools.MaxCatalogSearchCandidates + 1,
	})
	if err != nil {
		return nil, err
	}
	eligible := entries[:0]
	for _, entry := range entries {
		if policy.State(entry.Name) == chat.ToolPolicyOnDemand {
			eligible = append(eligible, entry)
		}
	}
	ranked := tools.RankCatalogEntries(eligible, tools.CatalogDiscoveryOptions{
		Query:             query,
		PreferredPackages: preferredPackages,
		RecentNames:       recentNames,
		ReadOnlyOnly:      true,
		Limit:             tools.MaxCatalogAutoPreloadTools,
	})
	names := make([]string, 0, len(ranked))
	for _, entry := range ranked {
		names = append(names, entry.Name)
	}
	return names, nil
}

func canonicalToolSelectorMatcher(raw string, entry tools.ToolCatalogEntry) (bool, bool) {
	selector, ok := chat.ParseToolPolicySelector(raw)
	if !ok {
		return false, false
	}
	wildcard := selector.Kind != chat.ToolPolicySelectorLiteral
	return selector.Matches(chat.ToolPolicyTarget{Name: entry.Name, Package: entry.Package}), wildcard
}
