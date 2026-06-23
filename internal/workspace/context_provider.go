package workspace

import (
	"context"
	"path/filepath"
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
Use link= values as app deep links for any workspace resource. open_editor_file[...] paths may be used with filesystem tools outside the workspace, subject to normal restrictions.
</workspace_instructions>`
}

func buildContextBlock(req contextprovider.BuildRequest, budgetChars int) string {
	if req.WorkspaceName == "" && req.Surface == nil && req.TabCount == 0 {
		return ""
	}
	if budgetChars <= 0 {
		budgetChars = defaultPromptBudget
	}
	var required strings.Builder
	required.WriteString(workspaceContextPrefix)
	required.WriteString("Current workspace and active surface context. Treat this as dynamic state, not stable instructions.\n")
	if req.WorkspaceName != "" {
		required.WriteString("- workspace: ")
		required.WriteString(sanitizeContextLine(req.WorkspaceName))
		required.WriteString("\n")
	}
	if req.TabCount > 0 {
		required.WriteString("- tab_count: ")
		required.WriteString(strconv.Itoa(req.TabCount))
		required.WriteString("\n")
	}
	openEditorFileCount := writeOpenEditorFiles(&required, req.Tabs)
	writeSurfaceIdentity(&required, req.Surface)

	var optional strings.Builder
	writeSurfaceTransientContext(&optional, req.Surface)
	for idx, tab := range req.Tabs {
		optional.WriteString("- tab[")
		optional.WriteString(strconv.Itoa(idx))
		optional.WriteString("]: ")
		if tab.IsActive {
			optional.WriteString("active ")
		}
		optional.WriteString(sanitizeContextLine(tab.Type))
		if tab.Title != "" {
			optional.WriteString(" ")
			optional.WriteString(sanitizeContextLine(tab.Title))
		}
		if tab.ContentID != "" {
			optional.WriteString(" link=")
			optional.WriteString(deepLinkForTab(tab.Type, tab.ContentID))
		}
		if label, value := tabStateReference(tab); label != "" && value != "" {
			optional.WriteString(" ")
			optional.WriteString(label)
			optional.WriteString("=")
			optional.WriteString(sanitizeContextLine(value))
		}
		optional.WriteString("\n")
	}
	return trimContextBlockWithRequiredPrefix(required.String(), optional.String(), budgetChars, openEditorFileCount > 0)
}

func writeOpenEditorFiles(sb *strings.Builder, tabs []contextprovider.Tab) int {
	idx := 0
	for _, tab := range tabs {
		if strings.TrimSpace(tab.Type) != "editor" {
			continue
		}
		filePath := firstNonEmpty(stringFromMap(tab.State, "filePath"), tab.ContentID)
		if filePath == "" {
			continue
		}
		filePath = filepath.Clean(filePath)
		if !filepath.IsAbs(filePath) {
			continue
		}
		sb.WriteString("- open_editor_file[")
		sb.WriteString(strconv.Itoa(idx))
		sb.WriteString("]: ")
		sb.WriteString(sanitizeContextLine(filePath))
		sb.WriteString("\n")
		idx++
	}
	return idx
}

func writeSurfaceIdentity(sb *strings.Builder, surface *contextprovider.Surface) {
	if surface == nil {
		return
	}
	if surface.Type != "" {
		sb.WriteString("- surface_type: ")
		sb.WriteString(sanitizeContextLine(surface.Type))
		sb.WriteString("\n")
	}
	if surface.Title != "" {
		sb.WriteString("- surface_title: ")
		sb.WriteString(sanitizeContextLine(surface.Title))
		sb.WriteString("\n")
	}
	if value := stringFromMap(surface.State, "filePath"); value != "" {
		sb.WriteString("- active_file: ")
		sb.WriteString(sanitizeContextLine(value))
		sb.WriteString("\n")
	}
	if value := stringFromMap(surface.State, "tasklistId"); value != "" {
		sb.WriteString("- active_tasklist: ")
		sb.WriteString(sanitizeContextLine(value))
		sb.WriteString("\n")
	}
	writeSurfaceValue(sb, "active_terminal_session", surface.State, "sessionId")
}

func writeSurfaceTransientContext(sb *strings.Builder, surface *contextprovider.Surface) {
	if surface == nil {
		return
	}
	writeSurfaceValue(sb, "selected_text", surface.Context, "selectedText")
	writeSurfaceValue(sb, "history_preview", surface.Context, "historyPreview")
	writeSurfaceValue(sb, "tasks_preview", surface.Context, "tasksPreview")
}

func trimContextBlock(content string, budgetChars int) string {
	return trimContextBlockWithRequiredPrefix(content, "", budgetChars, false)
}

func trimContextBlockWithRequiredPrefix(requiredContent string, optionalContent string, budgetChars int, preserveRequired bool) string {
	content := requiredContent + optionalContent
	content = strings.TrimRight(content, "\n")
	if runeLen(content)+runeLen(workspaceContextSuffix) <= budgetChars {
		return content + workspaceContextSuffix
	}
	requiredContent = strings.TrimRight(requiredContent, "\n")
	requiredLen := runeLen(requiredContent)
	suffixLen := runeLen(workspaceContextTruncationNotice) + runeLen(workspaceContextSuffix)
	if preserveRequired && requiredLen+suffixLen >= budgetChars {
		return requiredContent + workspaceContextTruncationNotice + workspaceContextSuffix
	}
	if suffixLen >= budgetChars {
		return ""
	}
	contentBudget := budgetChars - suffixLen
	if contentBudget <= runeLen(workspaceContextPrefix) {
		return ""
	}
	if !preserveRequired && requiredLen >= contentBudget {
		runes := []rune(content)
		if len(runes) > contentBudget {
			content = strings.TrimSpace(string(runes[:contentBudget]))
		}
		if content == "" {
			return ""
		}
		return content + workspaceContextTruncationNotice + workspaceContextSuffix
	}
	optionalBudget := contentBudget - requiredLen
	if optionalBudget > 0 {
		optionalRunes := []rune(optionalContent)
		if len(optionalRunes) > optionalBudget {
			optionalContent = string(optionalRunes[:optionalBudget])
		}
		content = strings.TrimSpace(requiredContent + optionalContent)
	} else {
		content = requiredContent
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

func tabStateReference(tab contextprovider.Tab) (string, string) {
	switch strings.TrimSpace(tab.Type) {
	case "chat":
		return "conversation", strings.TrimSpace(tab.ContentID)
	case "editor":
		return "file", firstNonEmpty(stringFromMap(tab.State, "filePath"), tab.ContentID)
	case "terminal":
		return "session", firstNonEmpty(stringFromMap(tab.State, "sessionId"), tab.ContentID)
	case "tasklist":
		return "tasklist", firstNonEmpty(stringFromMap(tab.State, "tasklistId"), tab.ContentID)
	default:
		return "", ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
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
	return strings.NewReplacer("\n", " ", "\r", " ", "<", "", ">", "", "`", "").Replace(strings.TrimSpace(value))
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
