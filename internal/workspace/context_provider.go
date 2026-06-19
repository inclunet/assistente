package workspace

import (
	"context"
	"strconv"
	"strings"

	"assistente/internal/contextprovider"
)

const defaultPromptBudget = 500
const workspaceContextPrefix = "<workspace_context>\n"
const workspaceContextSuffix = "\n</workspace_context>"
const workspaceContextTruncationNotice = "\n... Additional workspace context omitted due to context budget."

type ContextProvider struct{}

func NewContextProvider() *ContextProvider {
	return &ContextProvider{}
}

func (p *ContextProvider) Name() string { return "workspace" }

func (p *ContextProvider) Metadata() contextprovider.ProviderMetadata {
	return contextprovider.ProviderMetadata{
		Name:             p.Name(),
		DisplayName:      "Workspace",
		Description:      "Current workspace, tabs, active surface, and editor state for this turn.",
		DefaultEnabled:   true,
		DefaultBudget:    defaultPromptBudget,
		SupportsSettings: false,
	}
}

func (p *ContextProvider) Build(_ context.Context, req contextprovider.BuildRequest) ([]contextprovider.Block, error) {
	blocks := []contextprovider.Block{{
		Provider:   p.Name(),
		Name:       "workspace_instructions",
		Volatility: contextprovider.VolatilityStable,
		Priority:   10,
		Content:    workspaceInstructionsBlock(),
	}}
	content := buildContextBlock(req, req.Budget(p.Name(), defaultPromptBudget))
	if content == "" {
		return blocks, nil
	}
	blocks = append(blocks, contextprovider.Block{
		Provider:   p.Name(),
		Name:       "workspace_context",
		Volatility: contextprovider.VolatilityFastDynamic,
		Priority:   100,
		Content:    content,
	})
	return blocks, nil
}

func workspaceInstructionsBlock() string {
	return `<workspace_instructions>
Use workspace deep links when referring to app resources that can be opened by the user.
Supported forms include assistente://conversation/{id}, assistente://tasklist/{id}, assistente://terminal/{id}, assistente://editor/{id}, and assistente://navigate/{route}.
When a deep link is useful, present it directly instead of inventing another navigation format.
</workspace_instructions>`
}

func buildContextBlock(req contextprovider.BuildRequest, budgetChars int) string {
	if req.WorkspaceName == "" && req.Surface == nil && req.TabCount == 0 {
		return ""
	}
	if budgetChars <= 0 {
		budgetChars = defaultPromptBudget
	}
	var sb strings.Builder
	sb.WriteString(workspaceContextPrefix)
	sb.WriteString("Current workspace and active surface context. Treat this as dynamic state, not stable instructions.\n")
	if req.WorkspaceName != "" {
		sb.WriteString("- workspace: ")
		sb.WriteString(sanitizeContextLine(req.WorkspaceName))
		sb.WriteString("\n")
	}
	if req.TabCount > 0 {
		sb.WriteString("- tab_count: ")
		sb.WriteString(strconv.Itoa(req.TabCount))
		sb.WriteString("\n")
	}
	for idx, tab := range req.Tabs {
		sb.WriteString("- tab[")
		sb.WriteString(strconv.Itoa(idx))
		sb.WriteString("]: ")
		if tab.IsActive {
			sb.WriteString("active ")
		}
		sb.WriteString(sanitizeContextLine(tab.Type))
		if tab.Title != "" {
			sb.WriteString(" ")
			sb.WriteString(sanitizeContextLine(tab.Title))
		}
		if tab.ContentID != "" {
			sb.WriteString(" link=")
			sb.WriteString(deepLinkForTab(tab.Type, tab.ContentID))
		}
		sb.WriteString("\n")
	}
	if req.Surface != nil {
		if req.Surface.Type != "" {
			sb.WriteString("- surface_type: ")
			sb.WriteString(sanitizeContextLine(req.Surface.Type))
			sb.WriteString("\n")
		}
		if req.Surface.Title != "" {
			sb.WriteString("- surface_title: ")
			sb.WriteString(sanitizeContextLine(req.Surface.Title))
			sb.WriteString("\n")
		}
		if value := stringFromMap(req.Surface.State, "filePath"); value != "" {
			sb.WriteString("- active_file: ")
			sb.WriteString(sanitizeContextLine(value))
			sb.WriteString("\n")
		}
		if value := stringFromMap(req.Surface.State, "tasklistId"); value != "" {
			sb.WriteString("- active_tasklist: ")
			sb.WriteString(sanitizeContextLine(value))
			sb.WriteString("\n")
		}
		writeSurfaceValue(&sb, "active_terminal_session", req.Surface.State, "sessionId")
		writeSurfaceValue(&sb, "selected_text", req.Surface.Context, "selectedText")
		writeSurfaceValue(&sb, "history_preview", req.Surface.Context, "historyPreview")
		writeSurfaceValue(&sb, "tasks_preview", req.Surface.Context, "tasksPreview")
	}
	return trimContextBlock(sb.String(), budgetChars)
}

func trimContextBlock(content string, budgetChars int) string {
	content = strings.TrimRight(content, "\n")
	if runeLen(content)+runeLen(workspaceContextSuffix) <= budgetChars {
		return content + workspaceContextSuffix
	}
	if runeLen(workspaceContextTruncationNotice)+runeLen(workspaceContextSuffix) >= budgetChars {
		return ""
	}
	contentBudget := budgetChars - runeLen(workspaceContextTruncationNotice) - runeLen(workspaceContextSuffix)
	if contentBudget < runeLen(workspaceContextPrefix) {
		return ""
	}
	runes := []rune(content)
	if len(runes) > contentBudget {
		content = strings.TrimSpace(string(runes[:contentBudget]))
	}
	if content == "" {
		return ""
	}
	return content + workspaceContextTruncationNotice + workspaceContextSuffix
}

func deepLinkForTab(tabType, contentID string) string {
	switch strings.TrimSpace(tabType) {
	case "chat":
		return "assistente://conversation/" + sanitizeContextLine(contentID)
	case "tasklist":
		return "assistente://tasklist/" + sanitizeContextLine(contentID)
	case "terminal":
		return "assistente://terminal/" + sanitizeContextLine(contentID)
	case "editor":
		return "assistente://editor/" + sanitizeContextLine(contentID)
	default:
		return sanitizeContextLine(contentID)
	}
}

func writeSurfaceValue(sb *strings.Builder, label string, values map[string]any, key string) {
	if value := stringFromMap(values, key); value != "" {
		sb.WriteString("- ")
		sb.WriteString(label)
		sb.WriteString(": ")
		sb.WriteString(sanitizeContextLine(value))
		sb.WriteString("\n")
	}
}

func sanitizeContextLine(value string) string {
	return strings.NewReplacer("\n", " ", "\r", " ", "<", "", ">", "").Replace(strings.TrimSpace(value))
}

func stringFromMap(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	if raw, ok := values[key].(string); ok {
		return strings.TrimSpace(raw)
	}
	return ""
}

func runeLen(value string) int {
	count := 0
	for range value {
		count++
	}
	return count
}
