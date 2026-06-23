// Package prompt ordena blocos de prompt e constrói o system prompt final.
// É puro — sem dependência de Wails, sem acesso a banco, sem I/O direto.
// Fontes de conteúdo estável/dinâmico entram por Context Providers.
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

// Builder monta o system prompt final a partir dos blocos já resolvidos.
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
	hasBaseSkill := hasContextBlock(contextBlocks, "skills", "base_skill")
	stableContext, dynamicContext := splitRenderedContextBlocks(contextBlocks)
	return b.build(messages, enabledSkills, disableSkills, disableOnDemand, tplData, slashSkillContent, stableContext, dynamicContext, !hasBaseSkill)
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
	includeBaseFallback bool,
) []llm.Message {
	var parts []string

	// Fallback mínimo: no caminho normal, a identidade/instrução base vem do
	// Context Provider de skills (`base_skill`).
	if includeBaseFallback {
		parts = append(parts, chat.DefaultSystemPrompt)
	}

	// 1. Context Providers estáveis (base skill, catálogos, protocolos e instruções)
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

	// 2. Context Providers dinâmicos (workspace, tasklists, memória, summary, etc.)
	for _, contextBlock := range dynamicContext {
		if strings.TrimSpace(contextBlock) == "" {
			continue
		}
		parts = append(parts, "\n\n"+strings.TrimSpace(contextBlock))
	}

	// 3. Skill invocado via /slash é conteúdo específico do turno e fica no fim.
	if slashSkillContent != "" {
		parts = append(parts, "\n\n"+slashSkillContent)
	}

	return chat.InjectSystemPromptWithCachePrefix(messages, strings.Join(parts, ""), stablePromptLen)
}

func sortContextBlocks(blocks []contextprovider.Block) {
	sort.SliceStable(blocks, func(i, j int) bool {
		if blocks[i].Volatility != blocks[j].Volatility {
			return contextprovider.VolatilityRank(blocks[i].Volatility) < contextprovider.VolatilityRank(blocks[j].Volatility)
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

func hasContextBlock(blocks []contextprovider.Block, provider string, name string) bool {
	for _, block := range blocks {
		if block.Provider == provider && block.Name == name && strings.TrimSpace(block.Content) != "" {
			return true
		}
	}
	return false
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
