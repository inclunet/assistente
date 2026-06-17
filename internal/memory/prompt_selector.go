package memory

import (
	"fmt"
	"sort"
	"strings"

	"assistente/internal/contextprovider"
	"assistente/internal/database"
)

type PromptLine struct {
	Record database.MemoryRecord
	Line   string
}

type PromptSelector struct{}

func NewPromptSelector() PromptSelector {
	return PromptSelector{}
}

func (s PromptSelector) Select(records []database.MemoryRecord, req contextprovider.BuildRequest, budgetChars int) []PromptLine {
	if budgetChars <= 0 || len(records) == 0 {
		return nil
	}
	ordered := append([]database.MemoryRecord(nil), records...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].LoadPolicy != ordered[j].LoadPolicy {
			return policyRank(ordered[i].LoadPolicy) < policyRank(ordered[j].LoadPolicy)
		}
		if ordered[i].Importance != ordered[j].Importance {
			return ordered[i].Importance > ordered[j].Importance
		}
		return ordered[i].UpdatedAt.After(ordered[j].UpdatedAt)
	})

	var selected []PromptLine
	used := 0
	for _, record := range ordered {
		if !matchesScope(record, req) {
			continue
		}
		if record.LoadPolicy == LoadPolicyAuto && autoRelevanceScore(record, req.CurrentUserText) == 0 {
			continue
		}
		line := promptLine(record)
		if line == "" {
			continue
		}
		lineChars := len([]rune(line))
		if used+lineChars > budgetChars {
			if record.LoadPolicy == LoadPolicyCore || record.LoadPolicy == LoadPolicyPinned {
				truncated := truncatePromptLine(line, budgetChars-used)
				if truncated != "" {
					selected = append(selected, PromptLine{Record: record, Line: truncated})
					used += len([]rune(truncated))
				}
			}
			if used > 0 {
				break
			}
			continue
		}
		selected = append(selected, PromptLine{Record: record, Line: line})
		used += lineChars
	}
	return selected
}

func truncatePromptLine(line string, budget int) string {
	if budget <= 0 {
		return ""
	}
	runes := []rune(line)
	if len(runes) <= budget {
		return line
	}
	if budget <= 4 {
		return strings.TrimSpace(string(runes[:budget]))
	}
	return strings.TrimSpace(string(runes[:budget-4])) + "...\n"
}

func promptLine(record database.MemoryRecord) string {
	text := strings.TrimSpace(record.Summary)
	if text == "" {
		text = strings.TrimSpace(record.Content)
	}
	if text == "" {
		return ""
	}
	return fmt.Sprintf("- [%s/%s/%s] %s\n", record.LoadPolicy, record.Kind, record.Scope, sanitizePromptLine(text))
}

func autoRelevanceScore(record database.MemoryRecord, currentUserText string) int {
	tokens := relevanceTokens(currentUserText)
	if len(tokens) == 0 {
		return 0
	}
	haystack := strings.ToLower(strings.Join([]string{
		record.Content,
		record.Summary,
		record.Tags,
		record.Kind,
	}, " "))
	score := 0
	for _, token := range tokens {
		if strings.Contains(haystack, token) {
			score++
		}
	}
	return score
}

func relevanceTokens(value string) []string {
	cleaned := strings.NewReplacer(
		".", " ",
		",", " ",
		";", " ",
		":", " ",
		"?", " ",
		"!", " ",
		"(", " ",
		")", " ",
		"[", " ",
		"]", " ",
		"{", " ",
		"}", " ",
		"\"", " ",
		"'", " ",
		"\n", " ",
		"\r", " ",
	).Replace(strings.ToLower(value))
	seen := map[string]bool{}
	var tokens []string
	for _, field := range strings.Fields(cleaned) {
		if len(field) < 4 || seen[field] {
			continue
		}
		seen[field] = true
		tokens = append(tokens, field)
	}
	return tokens
}

func matchesScope(record database.MemoryRecord, req contextprovider.BuildRequest) bool {
	scope := strings.TrimSpace(record.Scope)
	scopeRef := strings.TrimSpace(record.ScopeRef)
	switch scope {
	case "", database.MemoryScopeGlobal, database.MemoryScopeUser:
		return true
	case database.MemoryScopeConversation:
		return req.ConversationID != "" && scopeRef == req.ConversationID
	case database.MemoryScopeWorkspace:
		return scopeRef != "" && (scopeRef == req.WorkspaceID || scopeRef == surfaceString(req.Surface, "workspaceId"))
	case database.MemoryScopeProject:
		return scopeRef != "" && (scopeRef == req.ProjectID || scopeRef == surfaceString(req.Surface, "projectId"))
	default:
		return false
	}
}

func surfaceString(surface *contextprovider.Surface, key string) string {
	if surface == nil {
		return ""
	}
	if raw, ok := surface.Context[key].(string); ok && strings.TrimSpace(raw) != "" {
		return strings.TrimSpace(raw)
	}
	if raw, ok := surface.State[key].(string); ok && strings.TrimSpace(raw) != "" {
		return strings.TrimSpace(raw)
	}
	return ""
}

func sanitizePromptLine(value string) string {
	replacer := strings.NewReplacer("\n", " ", "\r", " ", "<", "", ">", "")
	return strings.TrimSpace(replacer.Replace(value))
}

func policyRank(policy string) int {
	switch policy {
	case LoadPolicyCore:
		return 0
	case LoadPolicyPinned:
		return 1
	case LoadPolicyAuto:
		return 2
	default:
		return 9
	}
}
