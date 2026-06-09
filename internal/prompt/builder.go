// Package prompt constrói o system prompt completo para o pipeline de chat.
// É puro — sem dependência de Wails, sem acesso a banco, sem I/O direto.
// As dependências externas (skills, workspace) são injetadas via interfaces.
package prompt

import (
	"log"
	"reflect"
	"strings"

	"assistente/internal/chat"
	"assistente/internal/llm"
	"assistente/internal/profiles"
	"assistente/internal/skills"
	"assistente/internal/tools"
	"assistente/internal/workspace"
)

// SkillReader é o subconjunto de skills.Manager que o Builder precisa.
// Permite mockar em testes sem instanciar o manager completo.
type SkillReader interface {
	GetAutoSkills() ([]skills.Skill, error)
	GetAvailableSkills() ([]skills.Skill, error)
	GetAllSkillsFull() ([]skills.Skill, error)
	GetSkillFiles(slug string) ([]string, error)
	// MaterializeSkill devolve um caminho em disco legível para o corpo da skill
	// (AEP-0072 D2). Em modo DB materializa um cache; em filesystem retorna o path.
	MaterializeSkill(s skills.Skill) (string, error)
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

	// OpenEditorPaths retorna os caminhos absolutos de arquivos abertos em abas de editor.
	// Se definido e não-vazio, o Build() adiciona uma seção ao system prompt
	// informando ao modelo que esses arquivos podem ser lidos/editados via tools.
	OpenEditorPaths func() []string
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
					ContentID: tab.ContentID,
					IsActive:  isActive,
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

// Build compõe o system prompt completo e o injeta na lista de mensagens.
//
//   - enabledSkills: nil = todos os auto_load, [] = skills desabilitados, ["slug1"] = lista explícita
//   - disableOnDemand: quando true, omite a seção <available_skills>
//   - tplData: contexto disponível nos templates dos skills
//   - slashSkillContent: conteúdo de um skill invocado via /slash (pode ser "")
//   - conversationSummary: resumo de mensagens antigas (rolling context)
func (b *Builder) Build(
	messages []llm.Message,
	enabledSkills []string,
	disableOnDemand bool,
	tplData any,
	slashSkillContent string,
	conversationSummary string,
) []llm.Message {
	var parts []string

	// 1. Base prompt — só inclui se há skills ou slash skill
	if len(enabledSkills) > 0 || slashSkillContent != "" {
		parts = append(parts, chat.DefaultSystemPrompt)
	}

	// 1b. Protocolo catalog-first (AEP-0049, D16): incluído SEMPRE que o gating por
	// catálogo está ativo (tool_catalog é a única tool inicial), independentemente de
	// haver skills ou slash skill, para forçar a ordem "consultar catálogo → usar tools".
	if catalogFirstActive(tplData) {
		parts = append(parts, joinPrefix(parts)+chat.CatalogFirstToolPrompt)
	}

	// 2. Seção de skills (auto_load + disponíveis)
	skillsSection := b.BuildSkillsSection(enabledSkills, disableOnDemand, tplData)
	if skillsSection != "" {
		parts = append(parts, "\n\n"+skillsSection)
	}

	// 3. Skill invocado via /slash
	if slashSkillContent != "" {
		parts = append(parts, "\n\n"+slashSkillContent)
	}

	// 4. Resumo da conversa (rolling context)
	if conversationSummary != "" {
		parts = append(parts, "\n\n<conversation_summary>\nSummary of earlier messages in this conversation (these messages are no longer in the context window but their content is captured below):\n\n"+conversationSummary+"\n</conversation_summary>")
	}

	// 5. Arquivos abertos em abas de editor (acessíveis via filesystem tools)
	if b.OpenEditorPaths != nil {
		if paths := b.OpenEditorPaths(); len(paths) > 0 {
			var sb strings.Builder
			sb.WriteString("\n\n<open_editor_files>\n")
			sb.WriteString("The following files are currently open in the user's editor tabs. ")
			sb.WriteString("You MAY use read_file, write_file, edit_file, and grep_search on ONLY these exact file paths, ")
			sb.WriteString("even if one of the listed files is outside the working directory. ")
			sb.WriteString("This exception applies ONLY to the exact full paths listed below — not to their parent directories, sibling files, or any other related paths. ")
			sb.WriteString("Structural operations (move_file, copy_file, delete_file, list_directory) are NOT allowed on these files outside the workspace. ")
			sb.WriteString("Normal tool policies still apply: denylisted or sensitive files (e.g. .env) may still be blocked even if listed here. ")
			sb.WriteString("If the active skill restricts filesystem access, those restrictions still apply on top of this exception. ")
			sb.WriteString("Any other path remains subject to the normal workspace roots and filesystem access policies:\n")
			for _, p := range paths {
				// Sanitize to prevent prompt injection via filenames while keeping
				// the path usable by filesystem tools (which need the real path).
				// Strip chars that could break XML-like prompt structure or confuse
				// LLM markdown parsing; do NOT use html.EscapeString which corrupts
				// chars like & and " making the path unusable for tool calls.
				safe := strings.NewReplacer(
					"<", "", ">", "", "`", "",
					"\n", "_", "\r", "_",
				).Replace(p)
				sb.WriteString("- ")
				sb.WriteString(safe)
				sb.WriteString("\n")
			}
			sb.WriteString("</open_editor_files>")
			parts = append(parts, sb.String())
		}
	}

	return chat.InjectSystemPrompt(messages, strings.Join(parts, ""))
}

// BuildSkillsSection constrói as seções <auto_skills> e <available_skills>.
func (b *Builder) BuildSkillsSection(enabledSkills []string, disableOnDemand bool, tplData any) string {
	if b.Skills == nil {
		return ""
	}

	// Slice vazio (não nil) = skills explicitamente desabilitados pelo perfil
	if enabledSkills != nil && len(enabledSkills) == 0 {
		return ""
	}

	var autoSkills []skills.Skill
	var availableSkills []skills.Skill

	if enabledSkills != nil {
		// Lista explícita do perfil: respeita a ordem definida
		allSkills, err := b.Skills.GetAllSkillsFull()
		if err != nil {
			log.Printf("[prompt] Erro ao carregar skills: %v", err)
			return ""
		}
		autoSkills = skills.FilterByNamesOrdered(allSkills, enabledSkills)
		if !disableOnDemand {
			availableSkills = skills.FilterExcludeNames(allSkills, enabledSkills)
		}
	} else {
		// Sem lista: usa auto_load do próprio skill (backward compat)
		var err error
		autoSkills, err = b.Skills.GetAutoSkills()
		if err != nil {
			log.Printf("[prompt] Erro ao carregar auto skills: %v", err)
		}
		if !disableOnDemand {
			availableSkills, err = b.Skills.GetAvailableSkills()
			if err != nil {
				log.Printf("[prompt] Erro ao carregar available skills: %v", err)
			}
		}
	}

	// AEP-0072 D5: autoload é exceção. No modo metadata-driven (sem lista
	// explícita do perfil), só permanecem em <auto_skills> as skills que
	// declaram autoload_reason; as demais são rebaixadas para sob demanda.
	// No modo lista-explícita a escolha do perfil é respeitada como está.
	if enabledSkills == nil {
		var demoted []skills.Skill
		autoSkills, demoted = splitAutoloadByReason(autoSkills)
		if !disableOnDemand && len(demoted) > 0 {
			availableSkills = append(availableSkills, demoted...)
		}
	}

	if templateToolCallingDisabled(tplData) {
		autoSkills = filterSkillsWithoutToolDependencies(autoSkills)
		availableSkills = nil
	}

	if len(autoSkills) == 0 && len(availableSkills) == 0 {
		return ""
	}

	var sb strings.Builder

	// <auto_skills>: conteúdo completo injetado no system prompt
	if len(autoSkills) > 0 {
		sb.WriteString("<auto_skills>\n")
		for i, s := range autoSkills {
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

			content := skills.ProcessTemplate(s.Content, tplData)
			var allowedBash []string
			if s.Tools != nil && s.Tools.BashCommands != nil {
				allowedBash = s.Tools.BashCommands.Allowed
			}
			content = skills.PreprocessCommands(content, allowedBash)
			sb.WriteString(content)
			sb.WriteString("\n")

			supplementary, _ := b.Skills.GetSkillFiles(s.Slug)
			if len(supplementary) > 0 {
				sb.WriteString("\nSupporting files (use read_file to access when needed):\n")
				for _, f := range supplementary {
					sb.WriteString("- `")
					sb.WriteString(f)
					sb.WriteString("`\n")
				}
			}
		}
		sb.WriteString("</auto_skills>")
	}

	// <available_skills>: catálogo compacto (Nível 1) para leitura lazy pelo modelo.
	var modelInvocable []skills.Skill
	for _, s := range availableSkills {
		if s.IsModelInvocable() {
			modelInvocable = append(modelInvocable, s)
		}
	}

	// AEP-0072 D3: orçamento de contexto no bloco de descoberta. Encurta
	// descrições e, se ainda exceder, omite skills de menor prioridade (ordem),
	// sinalizando a omissão de forma observável.
	kept, descs, omitted := planAvailableSkillsBudget(modelInvocable, skillCatalogCharBudget)
	if omitted > 0 {
		log.Printf("[prompt] catálogo de skills excedeu o budget (%d chars): %d skill(s) omitida(s) do Nível 1", skillCatalogCharBudget, omitted)
	}

	if len(kept) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString("<available_skills>\n")
		sb.WriteString("You have skills available that provide specialized instructions for specific tasks.\n")
		sb.WriteString("To use a skill, read its file using the read_file tool with the path indicated below.\n")
		sb.WriteString("Only read a skill when it's relevant to the current task.\n\n")
		for i, s := range kept {
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
			sb.WriteString(descs[i])

			path := s.Path
			if materialized, err := b.Skills.MaterializeSkill(s); err == nil && materialized != "" {
				path = materialized
			}
			sb.WriteString("\n  Path: `")
			sb.WriteString(path)
			sb.WriteString("`\n")

			supplementary, _ := b.Skills.GetSkillFiles(s.Slug)
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

// skillCatalogCharBudget é o cap (em caracteres) do bloco de descoberta do
// Nível 1 (AEP-0072 D3), no espírito do limite do Codex (~8k chars).
const skillCatalogCharBudget = 8000

// skillCatalogShortDescLen é o comprimento máximo das descrições quando o
// catálogo precisa ser encurtado para caber no budget.
const skillCatalogShortDescLen = 100

// splitAutoloadByReason separa skills autoload que declaram autoload_reason
// (kept) das que não declaram (demoted), conforme AEP-0072 D5.
func splitAutoloadByReason(input []skills.Skill) (kept, demoted []skills.Skill) {
	for _, s := range input {
		if strings.TrimSpace(s.AutoloadReason) != "" {
			kept = append(kept, s)
		} else {
			demoted = append(demoted, s)
		}
	}
	return kept, demoted
}

// planAvailableSkillsBudget aplica o orçamento de contexto ao catálogo do Nível
// 1. Retorna as skills mantidas, suas descrições (possivelmente encurtadas) e a
// quantidade omitida. Estratégia: (1) cabe com descrição cheia → mantém;
// (2) encurta descrições; (3) omite skills do fim (menor prioridade).
func planAvailableSkillsBudget(list []skills.Skill, budget int) (kept []skills.Skill, descs []string, omitted int) {
	if len(list) == 0 {
		return nil, nil, 0
	}

	fullDescs := make([]string, len(list))
	total := 0
	for i, s := range list {
		fullDescs[i] = s.Description
		total += skillEntryCost(s, s.Description)
	}

	chosen := fullDescs
	if budget > 0 && total > budget {
		chosen = make([]string, len(list))
		total = 0
		for i, s := range list {
			chosen[i] = truncateDescription(s.Description, skillCatalogShortDescLen)
			total += skillEntryCost(s, chosen[i])
		}
	}

	end := len(list)
	if budget > 0 {
		for end > 0 && total > budget {
			end--
			total -= skillEntryCost(list[end], chosen[end])
			omitted++
		}
	}

	return list[:end], chosen[:end], omitted
}

// skillEntryCost estima o custo em caracteres da linha de catálogo de uma skill.
func skillEntryCost(s skills.Skill, desc string) int {
	// nome + slug + descrição + path + overhead de formatação/markdown.
	return len(s.GetDisplayName()) + len(s.Slug) + len(desc) + len(s.Path) + 32
}

// truncateDescription encurta uma descrição para no máximo max runes, anexando "…".
func truncateDescription(desc string, max int) string {
	if max <= 0 {
		return desc
	}
	runes := []rune(desc)
	if len(runes) <= max {
		return desc
	}
	return strings.TrimSpace(string(runes[:max])) + "…"
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
// tool calling está habilitado e "tool_catalog" é a ÚNICA tool inicial exposta ao
// modelo. Só nesse caso o protocolo catalog-first deve ser instruído: o texto de
// CatalogFirstToolPrompt afirma que a única tool disponível inicialmente é a
// "tool_catalog". Quando o perfil fixa EnabledTools com tool_catalog + outras
// tools, essas outras já ficam disponíveis de imediato (ResolveInitialEnabledTools
// devolve a lista intacta), então o gating não restringe ao catálogo e o protocolo
// não deve ser injetado para não enganar o modelo.
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
		// Qualquer outra tool inicial significa que o gating não restringe o
		// modelo a apenas o catálogo — o protocolo catalog-first seria enganoso.
		return false
	}
	return hasCatalog
}

func templateToolCallingDisabled(tplData any) bool {
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

// skillDependsOnTools indica se a skill exige alguma capacidade (tools,
// filesystem, network ou MCP), seja por declaração explícita (requires_*,
// AEP-0072 D4) ou inferida das permissões. Usado para omitir skills incompatíveis
// quando o tool calling está desabilitado.
func skillDependsOnTools(s skills.Skill) bool {
	return s.RequiresAnyCapability()
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
	if activeProfile != nil {
		initialEnabledTools := chat.ResolveInitialEnabledTools(b.Tools, activeProfile.Chat.EnabledTools, activeProfile.Chat.DisableTools)
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
