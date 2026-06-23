// Package prompt constrói o system prompt completo para o pipeline de chat.
// É puro — sem dependência de Wails, sem acesso a banco, sem I/O direto.
// As dependências externas (skills, workspace) são injetadas via interfaces.
package prompt

import (
	"log"
	"reflect"
	"sort"
	"strings"

	"assistente/internal/chat"
	"assistente/internal/contextprovider"
	"assistente/internal/llm"
	"assistente/internal/profiles"
	"assistente/internal/skills"
	"assistente/internal/tools"
	"assistente/internal/workspace"
)

// SkillReader é o subconjunto de skills.Manager que o Builder precisa.
// Permite mockar em testes sem instanciar o manager completo.
type SkillReader interface {
	GetAllSkillsFull() ([]skills.Skill, error)
	GetSkillFiles(slug string) ([]string, error)
}

// WorkspaceReader é o subconjunto de workspace.Manager que o Builder precisa.
type WorkspaceReader interface {
	Active() *workspace.Workspace
}

// workspaceReaderIsUsable evita typed nil: var m *Manager = nil; WorkspaceReader = m →
// r != nil mas chamar Active() panica. Ref: https://go.dev/doc/faq#nil_error
func workspaceReaderIsUsable(r WorkspaceReader) bool {
	if r == nil {
		return false
	}
	v := reflect.ValueOf(r)
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface:
		return !v.IsNil()
	default:
		return true
	}
}

// Builder monta o system prompt final a partir de skills, resumo e contexto de workspace.
type Builder struct {
	Skills    SkillReader
	Workspace WorkspaceReader
	Tools     *tools.Registry
}

// TemplateData é um alias para chat.TemplateData — a definição canônica vive em internal/chat
// para evitar import circular (prompt importa chat).
type TemplateData = chat.TemplateData

// TabInfo é um alias para chat.TabInfo.
type TabInfo = chat.TabInfo

// BuildTemplateData monta o TemplateData a partir do perfil ativo e do workspace.
func (b *Builder) BuildTemplateData(activeProfile *profiles.Profile, params llm.ChatParams, conversationID string) TemplateData {
	enabledToolNames := b.ComputeEnabledToolNames(activeProfile)
	data := TemplateData{
		Profile:            activeProfile,
		ProfileSlug:        params.ProfileSlug,
		ToolCallingEnabled: len(enabledToolNames) > 0,
		EnabledTools:       enabledToolNames,
		EnabledToolCount:   len(enabledToolNames),
		ConversationID:     conversationID,
	}

	var activeTab *workspace.Tab
	if workspaceReaderIsUsable(b.Workspace) {
		if ws := b.Workspace.Active(); ws != nil {
			data.WorkspaceID = ws.ID
			data.WorkspaceName = ws.Name
			data.WorkspaceProfile = ws.Profile
			data.TabCount = len(ws.Tabs.Items)
			data.Tabs = make([]TabInfo, 0, len(ws.Tabs.Items))
			for idx := range ws.Tabs.Items {
				tab := ws.Tabs.Items[idx]
				isActive := tab.ID == ws.Tabs.Active
				info := TabInfo{
					Title:     tab.Title,
					Type:      string(tab.Type),
					ContentID: tabContentReference(tab),
					IsActive:  isActive,
					State:     cloneStringAnyMap(tab.State),
				}
				data.Tabs = append(data.Tabs, info)
				if isActive {
					activeTab = &ws.Tabs.Items[idx]
					data.ActiveTabTitle = tab.Title
					data.ActiveTabType = string(tab.Type)
				}
			}
		}
	}

	surfaceType := strings.TrimSpace(params.TabType)
	if surfaceType == "" {
		surfaceType = data.ActiveTabType
	}
	activeTabMatchesSurface := activeTab != nil && (surfaceType == "" || string(activeTab.Type) == surfaceType)
	surfaceTitle := ""
	if activeTabMatchesSurface {
		surfaceTitle = data.ActiveTabTitle
	}
	surfaceState := chat.DecodeSurfaceJSONMap(params.SurfaceStateJSON, "[prompt] surface state json")
	if surfaceState == nil && activeTabMatchesSurface && len(activeTab.State) > 0 {
		surfaceState = activeTab.State
	}
	surfaceContext := chat.DecodeSurfaceJSONMap(params.SurfaceContextJSON, "[prompt] surface context json")
	data.ProjectID = firstNonEmpty(stringFromMap(surfaceContext, "projectId"), stringFromMap(surfaceState, "projectId"))

	if surfaceType != "" || surfaceTitle != "" || surfaceState != nil || surfaceContext != nil {
		data.Surface = &chat.SurfaceInfo{
			Type:    surfaceType,
			Title:   surfaceTitle,
			State:   surfaceState,
			Context: surfaceContext,
		}
	}

	return data
}

func cloneStringAnyMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func tabContentReference(tab workspace.Tab) string {
	if tab.ContentID != "" {
		return tab.ContentID
	}
	switch tab.Type {
	case workspace.TabTypeChat:
		return tab.ConversationID
	case workspace.TabTypeEditor:
		return stringFromAny(tab.State["filePath"])
	case workspace.TabTypeTerminal:
		return stringFromAny(tab.State["sessionId"])
	case workspace.TabTypeTasklist:
		return stringFromAny(tab.State["tasklistId"])
	default:
		return ""
	}
}

func stringFromAny(value any) string {
	if raw, ok := value.(string); ok {
		return strings.TrimSpace(raw)
	}
	return ""
}

func stringFromMap(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	return stringFromAny(values[key])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (b *Builder) BuildWithContextBlocks(
	messages []llm.Message,
	enabledSkills []string,
	disableSkills bool,
	disableOnDemand bool,
	tplData any,
	slashSkillContent string,
	contextBlocks []contextprovider.Block,
) []llm.Message {
	contextBlocks = append([]contextprovider.Block(nil), contextBlocks...)
	sortContextBlocks(contextBlocks)
	stableContext, dynamicContext := splitRenderedContextBlocks(contextBlocks)
	return b.build(messages, enabledSkills, disableSkills, disableOnDemand, tplData, slashSkillContent, stableContext, dynamicContext)
}

func (b *Builder) build(
	messages []llm.Message,
	enabledSkills []string,
	disableSkills bool,
	disableOnDemand bool,
	tplData any,
	slashSkillContent string,
	stableContext []string,
	dynamicContext []string,
) []llm.Message {
	var parts []string

	// 1. Base prompt — independente de skills. Context providers substituem
	// algumas skills legadas, mas não substituem a identidade base do assistente.
	parts = append(parts, chat.DefaultSystemPrompt)

	// 1b. Protocolo catalog-first (AEP-0049, D16): incluído SEMPRE que o gating por
	// catálogo está ativo (tool_catalog é a única tool inicial), independentemente de
	// haver skills ou slash skill, para forçar a ordem "consultar catálogo → usar tools".
	if catalogFirstActive(tplData) {
		parts = append(parts, joinPrefix(parts)+chat.CatalogFirstToolPrompt)
	}

	// 2. Seção de skills (base + catálogo on-demand)
	skillsSection := b.buildSkillsSection(enabledSkills, disableSkills, disableOnDemand, tplData)
	if skillsSection != "" {
		parts = append(parts, "\n\n"+skillsSection)
	}

	// 3. Context Providers estáveis (instruções cacheáveis)
	for _, contextBlock := range stableContext {
		if strings.TrimSpace(contextBlock) == "" {
			continue
		}
		parts = append(parts, "\n\n"+strings.TrimSpace(contextBlock))
	}
	stablePromptLen := 0
	for _, part := range parts {
		stablePromptLen += len(part)
	}

	// 4. Context Providers dinâmicos (workspace, tasklists, memória, summary, etc.)
	for _, contextBlock := range dynamicContext {
		if strings.TrimSpace(contextBlock) == "" {
			continue
		}
		parts = append(parts, "\n\n"+strings.TrimSpace(contextBlock))
	}

	// 5. Skill invocado via /slash é conteúdo específico do turno e fica no fim.
	if slashSkillContent != "" {
		parts = append(parts, "\n\n"+slashSkillContent)
	}

	return chat.InjectSystemPromptWithCachePrefix(messages, strings.Join(parts, ""), stablePromptLen)
}

func sortContextBlocks(blocks []contextprovider.Block) {
	sort.SliceStable(blocks, func(i, j int) bool {
		if blocks[i].Volatility != blocks[j].Volatility {
			return contextVolatilityRank(blocks[i].Volatility) < contextVolatilityRank(blocks[j].Volatility)
		}
		if blocks[i].Priority != blocks[j].Priority {
			return blocks[i].Priority < blocks[j].Priority
		}
		if blocks[i].Provider != blocks[j].Provider {
			return blocks[i].Provider < blocks[j].Provider
		}
		if blocks[i].Name != blocks[j].Name {
			return blocks[i].Name < blocks[j].Name
		}
		return blocks[i].Content < blocks[j].Content
	})
}

func contextVolatilityRank(value contextprovider.Volatility) int {
	switch value {
	case contextprovider.VolatilityStable:
		return 0
	case contextprovider.VolatilityLowDynamic:
		return 1
	case contextprovider.VolatilitySlowDynamic:
		return 2
	case contextprovider.VolatilityMidDynamic:
		return 3
	case contextprovider.VolatilityRolling:
		return 4
	case contextprovider.VolatilityFastDynamic:
		return 5
	case contextprovider.VolatilityTurnDynamic:
		return 6
	default:
		return 9
	}
}

func sortedStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func splitRenderedContextBlocks(blocks []contextprovider.Block) ([]string, []string) {
	stable := make([]string, 0)
	dynamic := make([]string, 0)
	for _, block := range blocks {
		content := strings.TrimSpace(block.Content)
		if content == "" {
			continue
		}
		if block.Volatility == contextprovider.VolatilityStable {
			stable = append(stable, content)
			continue
		}
		dynamic = append(dynamic, content)
	}
	return stable, dynamic
}

func (b *Builder) buildSkillsSection(enabledSkills []string, disableSkills bool, disableOnDemand bool, tplData any) string {
	if b.Skills == nil {
		return ""
	}

	allSkills, err := b.Skills.GetAllSkillsFull()
	if err != nil {
		log.Printf("[prompt] Erro ao carregar skills: %v", err)
		return ""
	}
	policy := skills.ResolveSelectionPolicy(allSkills, enabledSkills, disableSkills, disableOnDemand)
	baseSkills := policy.Base
	availableSkills := policy.OnDemand

	if toolCallingDisabled(tplData) {
		if enabledSkills == nil {
			compatible := filterSkillsWithoutToolDependencies(append(append([]skills.Skill{}, baseSkills...), availableSkills...))
			baseSkills = nil
			if len(compatible) > 0 {
				baseSkills = compatible[:1]
			}
		} else {
			baseSkills = filterSkillsWithoutToolDependencies(baseSkills)
		}
		availableSkills = nil
	}
	if len(baseSkills) == 0 && len(availableSkills) == 0 {
		return ""
	}

	var sb strings.Builder

	// <base_skills>: conteúdo completo da skill base do perfil.
	if len(baseSkills) > 0 {
		sb.WriteString("<base_skills>\n")
		for i, s := range baseSkills {
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString("## ")
			sb.WriteString(s.GetDisplayName())
			if s.Type != "" {
				sb.WriteString(" [")
				sb.WriteString(s.Type)
				sb.WriteString("]")
			}
			sb.WriteString("\n")

			content := s.Content
			var allowedBash []string
			if s.Tools != nil && s.Tools.BashCommands != nil {
				allowedBash = s.Tools.BashCommands.Allowed
			}
			content = skills.PreprocessCommands(content, allowedBash)
			sb.WriteString(content)
			sb.WriteString("\n")

			supplementary, _ := b.Skills.GetSkillFiles(s.Slug)
			supplementary = sortedStrings(supplementary)
			if len(supplementary) > 0 {
				sb.WriteString("\nSupporting files (use read_file to access when needed):\n")
				for _, f := range supplementary {
					sb.WriteString("- `")
					sb.WriteString(f)
					sb.WriteString("`\n")
				}
			}
		}
		sb.WriteString("</base_skills>")
	}

	// <available_skills>: referências para leitura lazy pelo modelo
	var modelInvocable []skills.Skill
	for _, s := range availableSkills {
		if s.IsModelInvocable() {
			modelInvocable = append(modelInvocable, s)
		}
	}

	if len(modelInvocable) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString("<available_skills>\n")
		sb.WriteString("The user can invoke these on-demand skills with slash commands; you can invoke them by calling `load_skill` when tool calling is available.\n")
		sb.WriteString("Treat this as a lightweight catalog of available workflows; do not assume the full instructions are loaded until a skill is invoked or `load_skill` succeeds.\n")
		sb.WriteString("Do not assume disabled or unlisted skills are available.\n\n")
		for _, s := range modelInvocable {
			sb.WriteString("- **")
			sb.WriteString(s.GetDisplayName())
			sb.WriteString("** (`")
			sb.WriteString(s.Slug)
			sb.WriteString("`)")
			if s.Type != "" {
				sb.WriteString(" [")
				sb.WriteString(s.Type)
				sb.WriteString("]")
			}
			sb.WriteString(": ")
			sb.WriteString(s.Description)
			sb.WriteString("\n  Identifier: `")
			sb.WriteString(s.Slug)
			sb.WriteString("`\n")

			supplementary, _ := b.Skills.GetSkillFiles(s.Slug)
			supplementary = sortedStrings(supplementary)
			if len(supplementary) > 0 {
				sb.WriteString("  Supporting files:\n")
				for _, f := range supplementary {
					sb.WriteString("    - `")
					sb.WriteString(f)
					sb.WriteString("`\n")
				}
			}
		}
		sb.WriteString("</available_skills>")
	}

	return sb.String()
}

// joinPrefix retorna o separador adequado para anexar uma nova seção: vazio quando
// ainda não há partes (a seção será a primeira) e "\n\n" caso contrário.
func joinPrefix(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	return "\n\n"
}

// catalogFirstActive informa se o gating por catálogo está ativo, ou seja, se o
// tool calling está habilitado e as únicas tools iniciais expostas ao modelo são
// tools de controle do runtime (`tool_catalog` e, opcionalmente, `load_skill`).
// Quando o perfil fixa EnabledTools com tool_catalog + outras tools, essas outras
// já ficam disponíveis de imediato, então o protocolo catalog-first não deve ser
// injetado para não enganar o modelo.
func catalogFirstActive(tplData any) bool {
	var data chat.TemplateData
	switch d := tplData.(type) {
	case chat.TemplateData:
		data = d
	case *chat.TemplateData:
		if d == nil {
			return false
		}
		data = *d
	default:
		return false
	}
	if !data.ToolCallingEnabled {
		return false
	}
	hasCatalog := false
	for _, name := range data.EnabledTools {
		if name == tools.ToolCatalogName {
			hasCatalog = true
			continue
		}
		if name == tools.LoadSkillName {
			continue
		}
		// Qualquer outra tool inicial significa que o gating não restringe o modelo
		// apenas a tools de controle — o protocolo catalog-first seria enganoso.
		return false
	}
	return hasCatalog
}

func toolCallingDisabled(tplData any) bool {
	switch data := tplData.(type) {
	case chat.TemplateData:
		return !data.ToolCallingEnabled
	case *chat.TemplateData:
		return data != nil && !data.ToolCallingEnabled
	default:
		return false
	}
}

func filterSkillsWithoutToolDependencies(input []skills.Skill) []skills.Skill {
	if len(input) == 0 {
		return input
	}
	filtered := make([]skills.Skill, 0, len(input))
	for _, s := range input {
		if skillDependsOnTools(s) {
			continue
		}
		filtered = append(filtered, s)
	}
	return filtered
}

func skillDependsOnTools(s skills.Skill) bool {
	if s.Tools != nil {
		if len(s.Tools.Allowed) > 0 || len(s.Tools.Denied) > 0 || s.Tools.BashCommands != nil {
			return true
		}
	}
	if s.Filesystem != nil {
		if len(s.Filesystem.Read) > 0 || len(s.Filesystem.Write) > 0 || len(s.Filesystem.Deny) > 0 {
			return true
		}
	}
	if s.Network != nil {
		if len(s.Network.AllowedHosts) > 0 || len(s.Network.DeniedHosts) > 0 {
			return true
		}
	}
	return s.MCP != nil
}

// ComputeEnabledToolNames retorna a lista de nomes de tools habilitadas pelo perfil.
func (b *Builder) ComputeEnabledToolNames(activeProfile *profiles.Profile) []string {
	if activeProfile != nil && activeProfile.Chat.DisableTools {
		return nil
	}
	if b.Tools == nil || b.Tools.Count() == 0 {
		return nil
	}

	var defs []tools.ToolDefinition
	var runtimeTools []string
	if b.modelOnDemandSkillAvailable(activeProfile) {
		runtimeTools = append(runtimeTools, tools.LoadSkillName)
	}
	if activeProfile != nil {
		initialEnabledTools := chat.ResolveInitialEnabledToolsWithRuntime(b.Tools, activeProfile.Chat.EnabledTools, activeProfile.Chat.DisableTools, runtimeTools)
		if initialEnabledTools != nil {
			defs = b.Tools.FilterByNames(initialEnabledTools)
		} else {
			defs = b.Tools.ToDefinitions()
		}
	} else {
		defs = b.Tools.ToDefinitions()
	}
	if len(defs) == 0 {
		return nil
	}

	names := make([]string, 0, len(defs))
	for _, d := range defs {
		names = append(names, d.Function.Name)
	}
	return names
}

func (b *Builder) modelOnDemandSkillAvailable(activeProfile *profiles.Profile) bool {
	if b.Skills == nil {
		return false
	}
	allSkills, err := b.Skills.GetAllSkillsFull()
	if err != nil {
		log.Printf("[prompt] Erro ao carregar política de skills para runtime tools: %v", err)
		return false
	}
	var enabledSkills []string
	var disableSkills bool
	var disableOnDemand bool
	if activeProfile != nil {
		enabledSkills = activeProfile.Chat.EnabledSkills
		disableSkills = activeProfile.Chat.DisableSkills
		disableOnDemand = activeProfile.Chat.DisableOnDemandSkills
	}
	return skills.ResolveSelectionPolicy(allSkills, enabledSkills, disableSkills, disableOnDemand).HasModelOnDemandSkill()
}
