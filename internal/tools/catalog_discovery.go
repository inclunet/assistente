package tools

import (
	"sort"
	"strings"
	"unicode"
)

const (
	// MaxCatalogSearchCandidates limita a busca ampla anterior ao ranking.
	MaxCatalogSearchCandidates = 200
	// MaxCatalogWildcardMatches impede que um único seletor despeje um catálogo
	// inteiro no contexto. O ToolPlanner continua aplicando o budget de schema.
	MaxCatalogWildcardMatches = 20
	// MaxCatalogAutoPreloadTools mantém o preload automático pequeno.
	MaxCatalogAutoPreloadTools = 3
)

type CatalogDiscoveryOptions struct {
	Query             string
	PreferredPackages []string
	RecentNames       []string
	ReadOnlyOnly      bool
	Limit             int
}

// RankCatalogEntries aplica uma ordem total e determinística:
// relevância textual, ordem de PreferredPackages, recência por conversa,
// origem e nome. ReadOnlyOnly aceita exclusivamente risco "read".
func RankCatalogEntries(entries []ToolCatalogEntry, opts CatalogDiscoveryOptions) []ToolCatalogEntry {
	preferred := orderedRank(opts.PreferredPackages)
	recent := orderedRank(opts.RecentNames)
	queryTokens := catalogQueryTokens(opts.Query)
	type rankedEntry struct {
		entry         ToolCatalogEntry
		relevance     int
		preferredRank int
		recentRank    int
		originRank    int
	}
	ranked := make([]rankedEntry, 0, len(entries))
	for _, entry := range entries {
		if opts.ReadOnlyOnly && strings.TrimSpace(entry.Risk) != "read" {
			continue
		}
		relevance, matches := catalogRelevance(entry, queryTokens)
		if len(queryTokens) > 0 && !matches {
			continue
		}
		ranked = append(ranked, rankedEntry{
			entry:         entry,
			relevance:     relevance,
			preferredRank: rankOrLast(preferred, entry.Package),
			recentRank:    rankOrLast(recent, entry.Name),
			originRank:    catalogOriginRank(entry.Origin),
		})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if a.relevance != b.relevance {
			return a.relevance > b.relevance
		}
		if a.preferredRank != b.preferredRank {
			return a.preferredRank < b.preferredRank
		}
		if a.recentRank != b.recentRank {
			return a.recentRank < b.recentRank
		}
		if a.originRank != b.originRank {
			return a.originRank < b.originRank
		}
		return a.entry.Name < b.entry.Name
	})
	limit := opts.Limit
	if limit > 0 && len(ranked) > limit {
		ranked = ranked[:limit]
	}
	out := make([]ToolCatalogEntry, len(ranked))
	for i := range ranked {
		out[i] = ranked[i].entry
	}
	return out
}

func orderedRank(values []string) map[string]int {
	ranks := make(map[string]int, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := ranks[value]; !exists {
			ranks[value] = len(ranks)
		}
	}
	return ranks
}

func rankOrLast(ranks map[string]int, value string) int {
	if rank, ok := ranks[strings.TrimSpace(value)]; ok {
		return rank
	}
	return len(ranks) + 1
}

func catalogOriginRank(origin string) int {
	switch origin {
	case ToolOriginBuiltin:
		return 0
	case ToolOriginMCPBridge:
		return 1
	case ToolOriginMCPNative:
		return 2
	default:
		return 3
	}
}

func catalogRelevance(entry ToolCatalogEntry, tokens []string) (int, bool) {
	if len(tokens) == 0 {
		return 0, true
	}
	name := searchableCatalogText(entry.Name)
	metadata := searchableCatalogText(strings.Join([]string{
		entry.DisplayName, entry.Description, entry.Category, entry.Class,
		entry.Package, strings.Join(entry.Tags, " "),
	}, " "))
	score := 0
	for _, token := range tokens {
		switch {
		case strings.Contains(name, token):
			score += 4
		case strings.Contains(metadata, token):
			score++
		}
	}
	return score, score > 0
}

func catalogQueryTokens(query string) []string {
	aliases := map[string][]string{
		"arquivo": {"file", "filesystem"}, "arquivos": {"file", "filesystem"},
		"carpeta": {"file", "filesystem"}, "fichero": {"file", "filesystem"},
		"ler": {"read"}, "leer": {"read"}, "buscar": {"search"},
		"pesquisar": {"search"}, "procurar": {"search"},
		"conversa": {"conversation", "history"}, "conversas": {"conversation", "history"},
		"histórico": {"history"}, "historico": {"history"},
	}
	raw := strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != ':'
	})
	seen := map[string]struct{}{}
	out := make([]string, 0, len(raw))
	add := func(value string) {
		if len([]rune(value)) < 3 {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	for _, token := range raw {
		add(token)
		for _, alias := range aliases[token] {
			add(alias)
		}
	}
	return out
}

func searchableCatalogText(value string) string {
	return strings.ToLower(strings.ReplaceAll(value, "-", "_"))
}
