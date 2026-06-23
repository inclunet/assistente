package workspace

import (
	"context"
	"net/url"
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
		Volatility: contextprovider.VolatilityLowDynamic,
		Priority:   100,
		Content:    content,
	})
	return blocks, nil
}

func workspaceInstructionsBlock() string {
	return `<workspace_instructions>
Use link= values as app deep links for any workspace resource. open_editor_file[...] entries are exact editor-open files: only read_file, write_file, edit_file, and grep_search may use those exact paths outside the workspace; structural operations, sensitive files, denylisted files, and active skill restrictions still apply.
</workspace_instructions>`
}

func buildContextBlock(req contextprovider.BuildRequest, budgetChars int) string {
	if req.WorkspaceName == "" && req.Surface == nil && req.TabCount == 0 && len(req.Tabs) == 0 {
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
	writeOpenEditorFiles(&required, req.Tabs)
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
		if linkTarget := tabLinkTarget(tab); linkTarget != "" {
			optional.WriteString(" link=")
			optional.WriteString(deepLinkForTab(tab.Type, linkTarget))
		}
		if label, value := tabStateReference(tab); label != "" && value != "" {
			writeSafeMachineReference(&optional, label, value)
		}
		optional.WriteString("\n")
	}
	return trimContextBlock(required.String(), optional.String(), budgetChars)
}

func writeOpenEditorFiles(sb *strings.Builder, tabs []contextprovider.Tab) {
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
		if containsPromptStructureChars(filePath) {
			continue
		}
		sb.WriteString("- open_editor_file[")
		sb.WriteString(strconv.Itoa(idx))
		sb.WriteString("]: ")
		sb.WriteString(filePath)
		sb.WriteString("\n")
		idx++
	}
}

func containsPromptStructureChars(value string) bool {
	return strings.ContainsAny(value, "<>`\n\r")
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

func writeSafeMachineReference(sb *strings.Builder, label string, value string) {
	value = strings.TrimSpace(value)
	if value == "" || containsPromptStructureChars(value) {
		return
	}
	sb.WriteString(" ")
	sb.WriteString(label)
	sb.WriteString("=")
	sb.WriteString(value)
}

func trimContextBlock(requiredContent string, optionalContent string, budgetChars int) string {
	requiredContent = strings.TrimRight(requiredContent, "\n")
	optionalContent = strings.TrimSpace(optionalContent)
	content := requiredContent
	if optionalContent != "" {
		content += "\n" + optionalContent
	}
	content = strings.TrimRight(content, "\n")
	if runeLen(content)+runeLen(workspaceContextSuffix) <= budgetChars {
		return content + workspaceContextSuffix
	}
	suffixLen := runeLen(workspaceContextTruncationNotice) + runeLen(workspaceContextSuffix)
	if suffixLen >= budgetChars {
		return ""
	}
	contentBudget := budgetChars - suffixLen
	if contentBudget <= runeLen(workspaceContextPrefix) {
		return ""
	}
	content = trimToWholeLines(content, contentBudget)
	if content == "" {
		return ""
	}
	if !hasWorkspaceContextLine(content) {
		return ""
	}
	return content + workspaceContextTruncationNotice + workspaceContextSuffix
}

func hasWorkspaceContextLine(content string) bool {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, strings.TrimSpace(workspaceContextPrefix)) {
		return false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(content, strings.TrimSpace(workspaceContextPrefix)))
	return rest != ""
}

func trimToWholeLines(content string, budgetChars int) string {
	content = strings.TrimRight(content, "\n")
	for content != "" && runeLen(content) > budgetChars {
		idx := strings.LastIndex(content, "\n")
		if idx < 0 {
			return ""
		}
		content = strings.TrimRight(content[:idx], "\n")
	}
	return strings.TrimSpace(content)
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
		return "assistente://editor/open?file=" + url.QueryEscape(strings.TrimSpace(contentID))
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

func tabLinkTarget(tab contextprovider.Tab) string {
	if trimmed := strings.TrimSpace(tab.ContentID); trimmed != "" {
		return trimmed
	}
	_, value := tabStateReference(tab)
	return value
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
