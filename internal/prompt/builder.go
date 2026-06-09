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
//
// AEP-0072 (Fase 4b): a descoberta (Nível 1) é servida diretamente do catálogo
// compacto persistido (ListCatalog) — sem recarregar o corpo das skills. Apenas
// as skills classificadas como autoload têm o corpo carregado sob demanda (Get),
// pois precisam ser injetadas inteiras no system prompt.
type SkillReader interface {
	// ListCatalog devolve o índice compacto de skills (Nível 1, descoberta).
	ListCatalog() ([]skills.SkillCatalogEntry, error)
	// Get carrega o corpo completo de uma skill (usado só no autoload, Nível 2).
	Get(slug string) (*skills.Skill, error)
	GetSkillFiles(slug string) ([]string, error)
	// MaterializeSkill devolve um caminho em disco legível para o corpo da skill
	// (AEP-0072 D2). Em modo DB materializa um cache; em filesystem retorna o path.
	// Usado como fallback quando o catálogo não traz um Path pré-materializado.
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

	pool, err := b.collectCatalogPool(enabledSkills)
	if err != nil {
		log.Printf("[prompt] Erro ao carregar catálogo de skills: %v", err)
		return ""
	}

	// AEP-0072 D1: a SkillSelectionPolicy é a fonte única de verdade para a
	// visibilidade de cada skill (autoload / sob demanda / oculta). O builder
	// apenas monta o contexto a partir do runtime/perfil e renderiza o resultado;
	// nenhuma regra de gating é reimplementada aqui.
	toolsEnabled := !templateToolCallingDisabled(tplData)
	selCtx := skills.SkillSelectionContext{
		ToolsEnabled:      toolsEnabled,
		FilesystemEnabled: toolsEnabled,
		NetworkEnabled:    toolsEnabled,
		MCPEnabled:        toolsEnabled,
		// Sem tool calling o modelo não consegue read_file, então o catálogo sob
		// demanda é inútil: rebaixa tudo para oculto, mantendo só autoload.
		DisableOnDemand:       disableOnDemand || !toolsEnabled,
		AutoloadAllowlist:     enabledSkills,
		RequireAutoloadReason: enabledSkills == nil,
	}

	// AEP-0072 Fase 4b: classificação feita sobre o catálogo compacto (Nível 1),
	// sem corpo. Só as skills de autoload terão o corpo carregado depois (Nível 2).
	policy := skills.NewSkillSelectionPolicy()
	sel := policy.DecideAllCatalog(pool, selCtx)
	bySlug := make(map[string]skills.SkillCatalogEntry, len(pool))
	for _, e := range pool {
		bySlug[e.Slug] = e
	}

	if len(sel.Autoload) == 0 && len(sel.OnDemand) == 0 {
		return ""
	}

	var sb strings.Builder

	// <auto_skills>: corpo completo injetado no system prompt. O corpo é carregado
	// sob demanda (por slug) a partir da fonte canônica — apenas para autoload.
	autoBlocks := make([]string, 0, len(sel.Autoload))
	for _, d := range sel.Autoload {
		full, err := b.Skills.Get(d.Slug)
		if err != nil || full == nil {
			log.Printf("[prompt] autoload: skill %q indisponível: %v", d.Slug, err)
			continue
		}
		autoBlocks = append(autoBlocks, b.renderAutoSkill(*full, tplData))
	}
	if len(autoBlocks) > 0 {
		sb.WriteString("<auto_skills>\n")
		sb.WriteString(strings.Join(autoBlocks, "\n"))
		sb.WriteString("</auto_skills>")
	}

	// <available_skills>: catálogo compacto (Nível 1) para leitura lazy pelo modelo,
	// montado direto das entradas do catálogo (sem corpo).
	var modelInvocable []skills.SkillCatalogEntry
	for _, d := range sel.OnDemand {
		if e, ok := bySlug[d.Slug]; ok && e.ModelInvocable {
			modelInvocable = append(modelInvocable, e)
		}
	}

	// AEP-0072 D3: orçamento de contexto no bloco de descoberta. Encurta
	// descrições e, se ainda exceder, omite skills de menor prioridade (ordem),
	// sinalizando a omissão de forma observável.
	kept, descs, omitted := planAvailableCatalogBudget(modelInvocable, skillCatalogCharBudget)
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
		for i, e := range kept {
			sb.WriteString("- **")
			sb.WriteString(e.GetDisplayName())
			sb.WriteString("** (`")
			sb.WriteString(e.Slug)
			sb.WriteString("`)")
			if e.Type != "" {
				sb.WriteString(" [")
				sb.WriteString(e.Type)
				sb.WriteString("]")
			}
			sb.WriteString(": ")
			sb.WriteString(descs[i])

			sb.WriteString("\n  Path: `")
			sb.WriteString(b.catalogPath(e))
			sb.WriteString("`\n")

			supplementary, _ := b.Skills.GetSkillFiles(e.Slug)
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

// renderAutoSkill monta o bloco de uma skill de autoload (corpo completo) para a
// seção <auto_skills>, aplicando template e pré-processamento de comandos.
func (b *Builder) renderAutoSkill(s skills.Skill, tplData any) string {
	var sb strings.Builder
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
	return sb.String()
}

// catalogPath devolve o caminho legível do corpo de uma entrada de catálogo. Usa
// o Path pré-materializado (gravado no rebuild do catálogo); como fallback raro
// (catálogo sem path), carrega o corpo e materializa sob demanda.
func (b *Builder) catalogPath(e skills.SkillCatalogEntry) string {
	if e.Path != "" {
		return e.Path
	}
	full, err := b.Skills.Get(e.Slug)
	if err != nil || full == nil {
		return ""
	}
	if path, err := b.Skills.MaterializeSkill(*full); err == nil {
		return path
	}
	return full.Path
}

// skillCatalogCharBudget é o cap (em caracteres) do bloco de descoberta do
// Nível 1 (AEP-0072 D3), no espírito do limite do Codex (~8k chars).
const skillCatalogCharBudget = 8000

// skillCatalogShortDescLen é o comprimento máximo das descrições quando o
// catálogo precisa ser encurtado para caber no budget.
const skillCatalogShortDescLen = 100

// collectCatalogPool carrega o índice compacto de skills (catálogo, Nível 1),
// preservando a ordem esperada por cada modo. No modo lista-explícita do perfil,
// as entradas da allowlist vêm primeiro (na ordem do perfil) seguidas das demais;
// no modo metadata-driven, usa a ordem do catálogo (nome). A classificação de
// visibilidade é feita depois pela SkillSelectionPolicy.
func (b *Builder) collectCatalogPool(enabledSkills []string) ([]skills.SkillCatalogEntry, error) {
	all, err := b.Skills.ListCatalog()
	if err != nil {
		return nil, err
	}
	if enabledSkills != nil {
		ordered := skills.CatalogByNamesOrdered(all, enabledSkills)
		return append(ordered, skills.CatalogExcludeNames(all, enabledSkills)...), nil
	}
	return all, nil
}

// planAvailableCatalogBudget aplica o orçamento de contexto ao catálogo do Nível
// 1. Retorna as entradas mantidas, suas descrições (possivelmente encurtadas) e a
// quantidade omitida. Estratégia: (1) cabe com descrição cheia → mantém;
// (2) encurta descrições; (3) omite skills do fim (menor prioridade).
func planAvailableCatalogBudget(list []skills.SkillCatalogEntry, budget int) (kept []skills.SkillCatalogEntry, descs []string, omitted int) {
	if len(list) == 0 {
		return nil, nil, 0
	}

	fullDescs := make([]string, len(list))
	total := 0
	for i, e := range list {
		fullDescs[i] = e.Description
		total += catalogEntryCost(e, e.Description)
	}

	chosen := fullDescs
	if budget > 0 && total > budget {
		chosen = make([]string, len(list))
		total = 0
		for i, e := range list {
			chosen[i] = truncateDescription(e.Description, skillCatalogShortDescLen)
			total += catalogEntryCost(e, chosen[i])
		}
	}

	end := len(list)
	if budget > 0 {
		for end > 0 && total > budget {
			end--
			total -= catalogEntryCost(list[end], chosen[end])
			omitted++
		}
	}

	return list[:end], chosen[:end], omitted
}

// catalogEntryCost estima o custo em caracteres da linha de catálogo de uma skill.
func catalogEntryCost(e skills.SkillCatalogEntry, desc string) int {
	// nome + slug + descrição + path + overhead de formatação/markdown.
	return len(e.GetDisplayName()) + len(e.Slug) + len(desc) + len(e.Path) + 32
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
