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
const surfaceContextPrefix = "<surface_context"
const surfaceContextSuffix = "\n</surface_context>"
const surfaceContextTruncationNotice = "\n... Additional surface context omitted due to context budget."
const surfaceFieldTruncationNotice = "\n... field truncated ..."
const maxSurfaceTextFieldChars = 1600
const maxTerminalTextFieldChars = 900

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
	budget := req.Budget(p.Name(), defaultPromptBudget)
	content := buildContextBlock(req, budget)
	if content != "" {
		blocks = append(blocks, contextprovider.Block{
			Provider:   p.Name(),
			Name:       "workspace_context",
			Volatility: contextprovider.VolatilityLowDynamic,
			Priority:   100,
			Content:    content,
		})
	}
	surfaceBudget := budget - runeLen(content)
	if surfaceBudget < 0 {
		surfaceBudget = 0
	}
	surfaceContent := buildSurfaceContextBlock(req.Surface, surfaceBudget)
	if surfaceContent != "" {
		blocks = append(blocks, contextprovider.Block{
			Provider:   p.Name(),
			Name:       "surface_context",
			Volatility: contextprovider.VolatilityTurnDynamic,
			Priority:   100,
			Content:    surfaceContent,
		})
	}
	return blocks, nil
}

func workspaceInstructionsBlock() string {
	return `<workspace_instructions>
open_editor_file[...] entries are exact editor-open files: only read_file, write_file, edit_file, and grep_search may use those exact paths outside the workspace; structural operations, sensitive files, denylisted files, and active skill restrictions still apply.
</workspace_instructions>`
}

func buildContextBlock(req contextprovider.BuildRequest, budgetChars int) string {
	if req.WorkspaceName == "" && req.TabCount == 0 && len(req.Tabs) == 0 {
		return ""
	}
	if budgetChars <= 0 {
		budgetChars = defaultPromptBudget
	}
	var required strings.Builder
	required.WriteString(workspaceContextPrefix)
	required.WriteString("Current workspace and tab context. Treat this as dynamic state, not stable instructions.\n")
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

	var optional strings.Builder
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

func buildSurfaceContextBlock(surface *contextprovider.Surface, budgetChars int) string {
	if surface == nil {
		return ""
	}
	if budgetChars <= 0 {
		return ""
	}
	normalized := normalizeSurfaceContext(surface)
	if normalized == nil {
		return ""
	}
	var body strings.Builder
	writeStructuredSelection(&body, normalized)
	writeStructuredFocus(&body, normalized)
	writeStructuredContent(&body, normalized)
	writeStructuredMetadata(&body, normalized)
	if strings.TrimSpace(body.String()) == "" && normalized.Incomplete {
		body.WriteString("<notice>surface context is incomplete and must not be used as a trusted mutation target</notice>\n")
	}
	content := buildSurfaceOpenTag(normalized) + "\nCurrent active surface context. Treat this as turn-specific dynamic state.\n" + strings.TrimRight(body.String(), "\n")
	return trimSurfaceContextBlock(content, budgetChars)
}

type normalizedSurfaceContext struct {
	SurfaceType     string
	SurfaceID       string
	Title           string
	Mode            string
	Selection       map[string]any
	Focus           map[string]any
	Content         map[string]any
	Metadata        map[string]any
	SnapshotVersion string
	CapturedAt      string
	StaleAfterMs    string
	Incomplete      bool
}

func normalizeSurfaceContext(surface *contextprovider.Surface) *normalizedSurfaceContext {
	if surface == nil {
		return nil
	}
	ctx := surface.Context
	surfaceType := firstNonEmpty(stringFromMap(ctx, "surfaceType"), surface.Type)
	surfaceID := stringFromMap(ctx, "surfaceId")
	snapshotVersion := stringFromMap(ctx, "snapshotVersion")
	incomplete := surfaceType == "" || surfaceID == "" || snapshotVersion == ""

	if surfaceType == "" {
		return nil
	}
	if surfaceID == "" {
		surfaceID = firstNonEmpty(
			stringFromMap(surface.State, "sessionId"),
			stringFromMap(surface.State, "tasklistId"),
			stringFromMap(surface.State, "draftId"),
			stringFromMap(surface.State, "filePath"),
			surfaceType,
		)
	}
	if snapshotVersion == "" {
		snapshotVersion = "legacy:" + surfaceType + ":" + surfaceID
	}

	normalized := &normalizedSurfaceContext{
		SurfaceType:     surfaceType,
		SurfaceID:       surfaceID,
		Title:           firstNonEmpty(stringFromMap(ctx, "title"), surface.Title),
		Mode:            stringFromMap(ctx, "mode"),
		Selection:       mapFromMap(ctx, "selection"),
		Focus:           mapFromMap(ctx, "focus"),
		Content:         mapFromMap(ctx, "content"),
		Metadata:        mapFromMap(ctx, "metadata"),
		SnapshotVersion: snapshotVersion,
		CapturedAt:      stringFromMap(ctx, "capturedAt"),
		StaleAfterMs:    numberStringFromMap(ctx, "staleAfterMs"),
		Incomplete:      incomplete,
	}

	if incomplete {
		adaptLegacySurfaceContext(surface, normalized)
	}
	return normalized
}

func adaptLegacySurfaceContext(surface *contextprovider.Surface, normalized *normalizedSurfaceContext) {
	if normalized == nil || surface == nil {
		return
	}
	if normalized.Mode == "" {
		normalized.Mode = stringFromMap(surface.Context, "mode")
	}
	if normalized.Selection == nil {
		if selectedText := stringFromMap(surface.Context, "selectedText"); selectedText != "" {
			normalized.Selection = map[string]any{
				"kind":     "text",
				"text":     selectedText,
				"explicit": true,
			}
		}
	}
	if normalized.Focus == nil {
		if cursorContext := stringFromMap(surface.Context, "cursorContext"); cursorContext != "" {
			normalized.Focus = map[string]any{
				"kind": "cursor",
				"text": cursorContext,
			}
		}
	}
	if normalized.Content == nil {
		switch {
		case stringFromMap(surface.Context, "historyPreview") != "":
			normalized.Content = map[string]any{
				"kind":         "terminal_output",
				"recentOutput": stringFromMap(surface.Context, "historyPreview"),
			}
		case stringFromMap(surface.Context, "tasksPreview") != "":
			normalized.Content = map[string]any{
				"kind":    "tasklist_summary",
				"summary": stringFromMap(surface.Context, "tasksPreview"),
			}
		}
	}
	if normalized.Metadata == nil {
		normalized.Metadata = map[string]any{}
	}
	for _, key := range []string{"filePath", "draftId", "tasklistId", "sessionId"} {
		if value := stringFromMap(surface.State, key); value != "" {
			normalized.Metadata[key] = value
		}
	}
	normalized.Metadata["legacySurfaceContext"] = true
}

func buildSurfaceOpenTag(surface *normalizedSurfaceContext) string {
	attrs := []string{
		xmlAttr("surface_type", surface.SurfaceType),
		xmlAttr("surface_id", surface.SurfaceID),
		xmlAttr("snapshot_version", surface.SnapshotVersion),
	}
	if surface.Title != "" {
		attrs = append(attrs, xmlAttr("title", surface.Title))
	}
	if surface.Mode != "" {
		attrs = append(attrs, xmlAttr("mode", surface.Mode))
	}
	if surface.CapturedAt != "" {
		attrs = append(attrs, xmlAttr("captured_at", surface.CapturedAt))
	}
	if surface.StaleAfterMs != "" {
		attrs = append(attrs, xmlAttr("stale_after_ms", surface.StaleAfterMs))
	}
	if surface.Incomplete {
		attrs = append(attrs, `incomplete="true"`)
	}
	return "<surface_context\n  " + strings.Join(attrs, "\n  ") + "\n>"
}

func writeStructuredSelection(sb *strings.Builder, surface *normalizedSurfaceContext) {
	if surface == nil || len(surface.Selection) == 0 {
		return
	}
	attrs := []string{xmlAttr("kind", firstNonEmpty(stringFromMap(surface.Selection, "kind"), "unknown"))}
	if value := boolStringFromMap(surface.Selection, "explicit"); value != "" {
		attrs = append(attrs, xmlAttr("explicit", value))
	}
	if value := boolStringFromMap(surface.Selection, "isEmpty"); value != "" {
		attrs = append(attrs, xmlAttr("is_empty", value))
	}
	if value := rangeString(mapFromMap(surface.Selection, "range")); value != "" {
		attrs = append(attrs, xmlAttr("range", value))
	}
	text := firstNonEmpty(stringFromMap(surface.Selection, "markdown"), stringFromMap(surface.Selection, "text"))
	if text == "" {
		if items := arraySummary(surface.Selection["items"]); items != "" {
			text = items
		}
	}
	writeXMLTextElement(sb, "selection", attrs, text, surfaceTextLimit(surface.SurfaceType))
}

func writeStructuredFocus(sb *strings.Builder, surface *normalizedSurfaceContext) {
	if surface == nil || len(surface.Focus) == 0 {
		return
	}
	attrs := []string{xmlAttr("kind", firstNonEmpty(stringFromMap(surface.Focus, "kind"), "unknown"))}
	if label := stringFromMap(surface.Focus, "label"); label != "" {
		attrs = append(attrs, xmlAttr("label", label))
	}
	if value := rangeString(mapFromMap(surface.Focus, "range")); value != "" {
		attrs = append(attrs, xmlAttr("range", value))
	}
	if value := cursorString(mapFromMap(surface.Focus, "cursor")); value != "" {
		attrs = append(attrs, xmlAttr("cursor", value))
	}
	if entity := mapFromMap(surface.Focus, "entity"); len(entity) > 0 {
		for _, key := range []string{"slideIndex", "taskListId", "taskId", "statusId", "statusLabel", "sessionId", "cwd"} {
			if value := scalarString(entity[key]); value != "" {
				attrs = append(attrs, xmlAttr(toSnakeCase(key), value))
			}
		}
	}
	writeXMLTextElement(sb, "focus", attrs, stringFromMap(surface.Focus, "text"), surfaceTextLimit(surface.SurfaceType))
}

func writeStructuredContent(sb *strings.Builder, surface *normalizedSurfaceContext) {
	if surface == nil || len(surface.Content) == 0 {
		return
	}
	attrs := []string{xmlAttr("kind", firstNonEmpty(stringFromMap(surface.Content, "kind"), "unknown"))}
	if value := boolStringFromMap(surface.Content, "truncated"); value != "" {
		attrs = append(attrs, xmlAttr("truncated", value))
	}
	text := firstNonEmpty(
		stringFromMap(surface.Content, "markdown"),
		stringFromMap(surface.Content, "recentOutput"),
		stringFromMap(surface.Content, "summary"),
		stringFromMap(surface.Content, "text"),
	)
	if currentInput := stringFromMap(surface.Content, "currentInput"); currentInput != "" {
		attrs = append(attrs, xmlAttr("current_input", truncateText(currentInput, 240)))
	}
	writeXMLTextElement(sb, "content", attrs, text, surfaceTextLimit(surface.SurfaceType))
}

func writeStructuredMetadata(sb *strings.Builder, surface *normalizedSurfaceContext) {
	if surface == nil || len(surface.Metadata) == 0 {
		return
	}
	for _, key := range metadataAllowlist(surface.SurfaceType) {
		raw, ok := surface.Metadata[key]
		if !ok {
			continue
		}
		value := scalarString(raw)
		if value == "" {
			value = arraySummary(raw)
		}
		if value == "" {
			continue
		}
		writeXMLTextElement(sb, "metadata", []string{xmlAttr("key", toSnakeCase(key))}, value, 500)
	}
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

func trimSurfaceContextBlock(content string, budgetChars int) string {
	content = strings.TrimRight(content, "\n")
	if runeLen(content)+runeLen(surfaceContextSuffix) <= budgetChars {
		return content + surfaceContextSuffix
	}
	suffixLen := runeLen(surfaceContextTruncationNotice) + runeLen(surfaceContextSuffix)
	if suffixLen >= budgetChars {
		return ""
	}
	contentBudget := budgetChars - suffixLen
	if contentBudget <= runeLen(surfaceContextPrefix) {
		return ""
	}
	content = trimToWholeLines(content, contentBudget)
	content = closeSurfaceOpenTagForTruncation(content, contentBudget)
	if content == "" || !hasSurfaceContextLine(content) {
		return ""
	}
	return content + surfaceContextTruncationNotice + surfaceContextSuffix
}

func closeSurfaceOpenTagForTruncation(content string, budgetChars int) string {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, surfaceContextPrefix) || strings.Contains(content, ">") {
		return content
	}
	closed := content + "\n>"
	if runeLen(closed) <= budgetChars {
		return closed
	}
	return content
}

func hasWorkspaceContextLine(content string) bool {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, strings.TrimSpace(workspaceContextPrefix)) {
		return false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(content, strings.TrimSpace(workspaceContextPrefix)))
	return rest != ""
}

func hasSurfaceContextLine(content string) bool {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, strings.TrimSpace(surfaceContextPrefix)) {
		return false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(content, strings.TrimSpace(surfaceContextPrefix)))
	return rest != "" && rest != "Current active surface context. Treat this as turn-specific dynamic state."
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

func mapFromMap(values map[string]any, key string) map[string]any {
	if values == nil {
		return nil
	}
	raw, ok := values[key]
	if !ok {
		return nil
	}
	if typed, ok := raw.(map[string]any); ok {
		return typed
	}
	return nil
}

func scalarString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case int32:
		return strconv.FormatInt(int64(typed), 10)
	default:
		return ""
	}
}

func numberStringFromMap(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	return scalarString(values[key])
}

func boolStringFromMap(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	if raw, ok := values[key].(bool); ok {
		if raw {
			return "true"
		}
		return "false"
	}
	return ""
}

func rangeString(values map[string]any) string {
	if len(values) == 0 {
		return ""
	}
	startLine := scalarString(values["startLine"])
	startColumn := scalarString(values["startColumn"])
	endLine := scalarString(values["endLine"])
	endColumn := scalarString(values["endColumn"])
	startOffset := scalarString(values["startOffset"])
	endOffset := scalarString(values["endOffset"])
	if startLine != "" || startColumn != "" || endLine != "" || endColumn != "" {
		return startLine + ":" + startColumn + "-" + endLine + ":" + endColumn
	}
	if startOffset != "" || endOffset != "" {
		return "offset:" + startOffset + "-" + endOffset
	}
	return ""
}

func cursorString(values map[string]any) string {
	if len(values) == 0 {
		return ""
	}
	line := scalarString(values["line"])
	column := scalarString(values["column"])
	offset := scalarString(values["offset"])
	parts := make([]string, 0, 3)
	if line != "" || column != "" {
		parts = append(parts, line+":"+column)
	}
	if offset != "" {
		parts = append(parts, "offset:"+offset)
	}
	return strings.Join(parts, " ")
}

func arraySummary(value any) string {
	values, ok := value.([]any)
	if !ok || len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for idx, item := range values {
		if idx >= 12 {
			parts = append(parts, "... additional items omitted")
			break
		}
		if itemMap, ok := item.(map[string]any); ok {
			label := firstNonEmpty(scalarString(itemMap["title"]), scalarString(itemMap["label"]), scalarString(itemMap["taskId"]))
			if label != "" {
				parts = append(parts, label)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func writeXMLTextElement(sb *strings.Builder, name string, attrs []string, text string, limit int) {
	sb.WriteString("<")
	sb.WriteString(name)
	if len(attrs) > 0 {
		sb.WriteString(" ")
		sb.WriteString(strings.Join(attrs, " "))
	}
	if strings.TrimSpace(text) == "" {
		sb.WriteString(" />\n")
		return
	}
	sb.WriteString(">")
	sb.WriteString(escapeXMLText(truncateText(text, limit)))
	sb.WriteString("</")
	sb.WriteString(name)
	sb.WriteString(">\n")
}

func xmlAttr(name string, value string) string {
	return name + `="` + escapeXMLAttr(value) + `"`
}

func escapeXMLText(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return replacer.Replace(value)
}

func escapeXMLAttr(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", `"`, "&quot;", "<", "&lt;", ">", "&gt;", "\n", " ", "\r", " ")
	return replacer.Replace(strings.TrimSpace(value))
}

func truncateText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || runeLen(value) <= limit {
		return value
	}
	runes := []rune(value)
	if limit <= runeLen(surfaceFieldTruncationNotice) {
		return string(runes[:limit])
	}
	return strings.TrimRight(string(runes[:limit-runeLen(surfaceFieldTruncationNotice)]), "\n ") + surfaceFieldTruncationNotice
}

func surfaceTextLimit(surfaceType string) int {
	if strings.TrimSpace(surfaceType) == "terminal" {
		return maxTerminalTextFieldChars
	}
	return maxSurfaceTextFieldChars
}

func metadataAllowlist(surfaceType string) []string {
	switch strings.TrimSpace(surfaceType) {
	case "editor":
		return []string{"documentId", "filePath", "draftId", "language", "presentationDetection", "slideCount", "currentSlideIndex", "currentSlideLabel", "projectId", "legacySurfaceContext"}
	case "tasklist":
		return []string{"taskListId", "slug", "taskCount", "statuses", "projectId", "legacySurfaceContext"}
	case "terminal":
		return []string{"sessionId", "cwd", "shell", "historyEntryCount", "lastExitCode", "projectId", "legacySurfaceContext"}
	default:
		return []string{"projectId", "legacySurfaceContext"}
	}
}

func toSnakeCase(value string) string {
	var sb strings.Builder
	for idx, r := range value {
		if r >= 'A' && r <= 'Z' {
			if idx > 0 {
				sb.WriteRune('_')
			}
			sb.WriteRune(r + ('a' - 'A'))
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

func runeLen(value string) int {
	count := 0
	for range value {
		count++
	}
	return count
}
