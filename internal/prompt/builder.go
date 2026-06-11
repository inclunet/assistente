// Package prompt constrói o system prompt completo para o pipeline de chat.
// É puro — sem dependência de Wails, sem acesso a banco, sem I/O direto.
// As dependências externas (skills, workspace) são injetadas via interfaces.
package prompt

import (
	"log"
	"reflect"
	"strings"
	"unicode/utf8"

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
	// ListCatalog devolve o índice compacto de skills (Nível 1, descoberta). As
	// entradas já trazem o Path pré-materializado (alvo do read_file, AEP-0072 D2),
	// gravado no rebuild do catálogo — a descoberta não recarrega o corpo.
	ListCatalog() ([]skills.SkillCatalogEntry, error)
	// Get carrega o corpo completo de uma skill (usado só no autoload, Nível 2).
	Get(slug string) (*skills.Skill, error)
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

	// OpenEditorPaths retorna os caminhos absolutos de arquivos abertos em abas de editor.
	// Se definido e não-vazio, o Build() adiciona uma seção ao system prompt
	// informando ao modelo que esses arquivos podem ser lidos/editados via tools.
	OpenEditorPaths func() []string

	// SkillCatalogBudgetPercent é o cap do bloco de descoberta (Nível 1) como
	// percentual da janela de contexto do modelo (AEP-0072 D3, estilo Codex ~2%).
	// 0 = usa o default. Quando a janela do modelo é desconhecida, cai no
	// fallback fixo em caracteres (skillCatalogCharBudget).
	SkillCatalogBudgetPercent float64
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

	pool, allowlistSlugs, err := b.collectCatalogPool(enabledSkills)
	if err != nil {
		log.Printf("[prompt] Erro ao carregar catálogo de skills: %v", err)
		return ""
	}

	// AEP-0072 D1: a SkillSelectionPolicy é a fonte única de verdade para a
	// visibilidade de cada skill (autoload / sob demanda / oculta). O builder
	// apenas monta o contexto a partir do runtime/perfil e renderiza o resultado;
	// nenhuma regra de gating é reimplementada aqui.
	caps := resolveSkillCapabilities(tplData)
	selCtx := skills.SkillSelectionContext{
		ToolsEnabled:      caps.tools,
		FilesystemEnabled: caps.filesystem,
		NetworkEnabled:    caps.network,
		MCPEnabled:        caps.mcp,
		// O catálogo sob demanda só serve se o modelo conseguir read_file (ativação
		// por leitura, Nível 2). Quando read_file não é alcançável pelo perfil
		// (tool calling off, ou EnabledTools fixo sem read_file/tool_catalog),
		// rebaixa tudo para oculto, mantendo só autoload.
		DisableOnDemand: disableOnDemand || !caps.onDemand,
		// Allowlist já resolvida para slugs canônicos (determinística, slug-first):
		// a política só precisa de membership por slug, sem ambiguidade slug/nome.
		AutoloadAllowlist:     allowlistSlugs,
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
	// montado direto das entradas do catálogo (sem corpo). Uma entrada sem Path
	// materializado não tem como ser ativada via read_file: é omitida (anomalia
	// observável), nunca renderizada com path vazio.
	var modelInvocable []skills.SkillCatalogEntry
	for _, d := range sel.OnDemand {
		e, ok := bySlug[d.Slug]
		if !ok || !e.ModelInvocable {
			continue
		}
		if e.Path == "" {
			log.Printf("[prompt] skill %q sem path materializado no catálogo; omitida da descoberta", e.Slug)
			continue
		}
		// Slug e Path são renderizados entre backticks; caracteres inseguros (que
		// fecham backticks/tags ou quebram o Markdown) poderiam injetar/quebrar a
		// estrutura do system prompt. Omite a entry (e loga) nesse caso.
		if hasUnsafePathChars(e.Slug) || hasUnsafePathChars(e.Path) {
			log.Printf("[prompt] skill com slug/path inseguro omitida da descoberta: slug=%q path=%q", e.Slug, e.Path)
			continue
		}
		modelInvocable = append(modelInvocable, e)
	}

	// AEP-0072 D3: orçamento de contexto no bloco de descoberta. O cap é um
	// percentual da janela do modelo (fallback fixo quando desconhecida). Encurta
	// descrições e, se ainda exceder, omite skills de menor prioridade (ordem),
	// sinalizando a omissão de forma observável.
	budget := resolveSkillCatalogBudget(b.SkillCatalogBudgetPercent, templateContextWindow(tplData))
	kept, descs, omitted := planAvailableCatalogBudget(modelInvocable, budget)
	if omitted > 0 {
		log.Printf("[prompt] catálogo de skills excedeu o budget (%d chars): %d skill(s) omitida(s) do Nível 1", budget, omitted)
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
			sb.WriteString(sanitizeSkillText(e.GetDisplayName()))
			sb.WriteString("** (`")
			sb.WriteString(e.Slug)
			sb.WriteString("`)")
			if e.Type != "" {
				sb.WriteString(" [")
				sb.WriteString(sanitizeSkillText(e.Type))
				sb.WriteString("]")
			}
			sb.WriteString(": ")
			sb.WriteString(sanitizeSkillText(descs[i]))

			sb.WriteString("\n  Path: `")
			sb.WriteString(e.Path)
			sb.WriteString("`\n")

			rawSupp, _ := b.Skills.GetSkillFiles(e.Slug)
			supplementary := safeSkillPaths(e.Slug, rawSupp)
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

// unsafeSkillPathChars são caracteres que poderiam fechar backticks/tags ou
// quebrar a estrutura/indentação Markdown do system prompt se aparecessem em um
// nome de supporting file. Inclui tab (\t), válido em filenames mas capaz de
// desalinhar o bloco — mantém a política consistente com sanitizeSkillText.
const unsafeSkillPathChars = "\n\r\t`<>"

// hasUnsafePathChars informa se s contém algum caractere que poderia fechar
// backticks/tags ou quebrar a estrutura Markdown/XML do system prompt.
func hasUnsafePathChars(s string) bool {
	return strings.ContainsAny(s, unsafeSkillPathChars)
}

// skillTextSanitizer neutraliza os caracteres que poderiam quebrar a estrutura
// Markdown/tags do bloco <available_skills>: quebras de linha/tab viram espaço,
// crases viram aspas simples e os delimitadores de tag < > viram aspas angulares.
var skillTextSanitizer = strings.NewReplacer(
	"\n", " ",
	"\r", " ",
	"\t", " ",
	"`", "'",
	"<", "‹",
	">", "›",
)

// sanitizeSkillText neutraliza texto exibido (nome, tipo, descrição) antes de
// renderizá-lo no <available_skills>. Diferente de Slug/Path (que omitem a entrada
// inteira quando inseguros), o texto é preservado de forma legível, apenas com os
// caracteres estruturais neutralizados — evitando injeção acidental/maliciosa
// (ex.: descrição contendo "</available_skills>").
func sanitizeSkillText(s string) string {
	return strings.TrimSpace(skillTextSanitizer.Replace(s))
}

// safeSkillPaths filtra paths de supporting files cujo nome contenha caracteres
// inseguros (ver unsafeSkillPathChars). Como GetSkillFiles lista o diretório do
// skill, um nome inesperado/malicioso não pode injetar conteúdo no prompt; paths
// omitidos são logados para diagnóstico.
func safeSkillPaths(slug string, paths []string) []string {
	if len(paths) == 0 {
		return paths
	}
	safe := make([]string, 0, len(paths))
	for _, p := range paths {
		if hasUnsafePathChars(p) {
			log.Printf("[prompt] supporting file omitido da skill %q: caractere inseguro no path %q", slug, p)
			continue
		}
		safe = append(safe, p)
	}
	return safe
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

	rawSupp, _ := b.Skills.GetSkillFiles(s.Slug)
	supplementary := safeSkillPaths(s.Slug, rawSupp)
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

// skillCatalogCharBudget é o cap (em caracteres) do bloco de descoberta do Nível
// 1 (AEP-0072 D3) usado como FALLBACK quando a janela do modelo é desconhecida,
// no espírito do limite do Codex (~8k chars).
const skillCatalogCharBudget = 8000

// defaultSkillCatalogBudgetPercent é o percentual default da janela do modelo
// reservado ao bloco de descoberta (Codex usa ~2%).
const defaultSkillCatalogBudgetPercent = 2.0

// skillCatalogCharsPerToken converte tokens (janela do modelo) em caracteres
// (unidade do planner de budget). Heurística ~4 chars/token.
const skillCatalogCharsPerToken = 4

// skillCatalogShortDescLen é o comprimento máximo das descrições quando o
// catálogo precisa ser encurtado para caber no budget.
const skillCatalogShortDescLen = 100

// resolveSkillCatalogBudget calcula o cap em caracteres do bloco de descoberta.
// Com a janela do modelo conhecida (tokens), usa percent% dela (convertida em
// chars); caso contrário, cai no fallback fixo skillCatalogCharBudget. percent<=0
// usa o default.
func resolveSkillCatalogBudget(percent float64, windowTokens int) int {
	if percent <= 0 {
		percent = defaultSkillCatalogBudgetPercent
	}
	if windowTokens <= 0 {
		return skillCatalogCharBudget
	}
	chars := int(float64(windowTokens) * (percent / 100.0) * skillCatalogCharsPerToken)
	if chars < 1 {
		return 1
	}
	return chars
}

// templateContextWindow extrai a janela de contexto do modelo (em tokens) do
// perfil ativo no tplData; 0 quando indisponível (cai no fallback de budget).
func templateContextWindow(tplData any) int {
	data, ok := asTemplateData(tplData)
	if !ok || data.Profile == nil {
		return 0
	}
	if w := data.Profile.Chat.ContextWindow; w > 0 {
		return w
	}
	return 0
}

// asTemplateData extrai um chat.TemplateData (por valor) do tplData genérico.
// ok=false quando o tipo não é TemplateData (ex.: nil ou tipos de teste simples),
// caso em que os callers devem assumir o comportamento default (sem restrição).
func asTemplateData(tplData any) (chat.TemplateData, bool) {
	switch d := tplData.(type) {
	case chat.TemplateData:
		return d, true
	case *chat.TemplateData:
		if d == nil {
			return chat.TemplateData{}, false
		}
		return *d, true
	default:
		return chat.TemplateData{}, false
	}
}

// skillCapabilities resume as capacidades que o modelo realmente alcança segundo o
// perfil (tplData), usadas no gating de skills do Nível 1 (AEP-0072 D4).
type skillCapabilities struct {
	tools      bool // tool calling utilizável (alguma tool real no universo do perfil)
	filesystem bool
	network    bool
	mcp        bool
	onDemand   bool // read_file alcançável → ativação por leitura (Nível 2) viável
}

func allSkillCapabilities() skillCapabilities {
	return skillCapabilities{tools: true, filesystem: true, network: true, mcp: true, onDemand: true}
}

// resolveSkillCapabilities deriva, de forma confiável, as capacidades que uma skill
// pode exigir contra o UNIVERSO de tools realmente disponível ao perfil.
//
// Princípio (AEP-0072 D4): o tool_catalog só ESCONDE tools inicialmente, não reduz
// o universo. Uma tool não desativada para o perfil torna a skill disponível mesmo
// que comece escondida no catálogo; uma tool ausente do perfil deve ocultar a skill.
//
//   - Sem tool calling → nada alcançável.
//   - Perfil sem allowlist (EnabledTools == nil) → universo completo → tudo
//     alcançável (catalog-first revela qualquer tool, ou todas já estão diretas).
//   - Perfil com allowlist fixa → o universo é exatamente essa lista (o catálogo
//     não excede a allowlist). Cada capacidade é derivada das tools presentes; o
//     próprio tool_catalog não concede capacidade.
func resolveSkillCapabilities(tplData any) skillCapabilities {
	data, ok := asTemplateData(tplData)
	if !ok {
		// Sem TemplateData tipado (ex.: chamadas simples/testes): assume tudo
		// habilitado, preservando o comportamento histórico.
		return allSkillCapabilities()
	}
	if !data.ToolCallingEnabled {
		return skillCapabilities{}
	}
	universe, restricted := profileToolUniverse(data)
	if !restricted {
		return allSkillCapabilities()
	}
	caps := skillCapabilities{}
	for _, n := range universe {
		if n == tools.ToolCatalogName {
			continue // meta-tool: não concede capacidade própria
		}
		caps.tools = true
		if n == "read_file" {
			caps.onDemand = true
		}
		switch tools.ToolCapabilityKind(n) {
		case tools.ToolCapabilityFilesystem:
			caps.filesystem = true
		case tools.ToolCapabilityNetwork:
			caps.network = true
		case tools.ToolCapabilityMCP:
			caps.mcp = true
		}
	}
	return caps
}

// profileToolUniverse devolve o universo de tools do perfil e se há restrição
// (allowlist). restricted=false significa universo completo (sem allowlist).
//
// A fonte confiável é o perfil (data.Profile.Chat): EnabledTools == nil = sem
// allowlist; DisableTools cobre o caso sem tools (já barrado por ToolCallingEnabled).
// Sem Profile tipado (testes/callers simples), interpreta data.EnabledTools como a
// allowlist — com [tool_catalog] sozinho equivalendo a catalog-first (sem restrição).
func profileToolUniverse(data chat.TemplateData) (names []string, restricted bool) {
	if data.Profile != nil {
		if data.Profile.Chat.DisableTools {
			return nil, true
		}
		if data.Profile.Chat.EnabledTools == nil {
			return nil, false
		}
		return data.Profile.Chat.EnabledTools, true
	}
	if data.EnabledTools == nil {
		return nil, false
	}
	if len(data.EnabledTools) == 1 && data.EnabledTools[0] == tools.ToolCatalogName {
		return nil, false
	}
	return data.EnabledTools, true
}

// collectCatalogPool carrega o índice compacto de skills (catálogo, Nível 1),
// preservando a ordem esperada por cada modo: no modo lista-explícita do perfil,
// as entradas da allowlist vêm primeiro (na ordem do perfil) seguidas das demais;
// no modo metadata-driven, usa a ordem do catálogo (nome). A classificação de
// visibilidade é feita depois pela SkillSelectionPolicy.
//
// Também devolve a allowlist já RESOLVIDA para slugs canônicos via
// CatalogByNamesOrdered (resolução determinística slug-first): é esse conjunto que
// vai para o SkillSelectionContext.AutoloadAllowlist, evitando matches ambíguos
// por nome.
func (b *Builder) collectCatalogPool(enabledSkills []string) (pool []skills.SkillCatalogEntry, allowlistSlugs []string, err error) {
	all, err := b.Skills.ListCatalog()
	if err != nil {
		return nil, nil, err
	}
	if enabledSkills == nil {
		return all, nil, nil
	}

	// Resolve a allowlist para entradas canônicas (slug-first) e deduplica por
	// slug — uma allowlist que repita o identificador (ou traga slug + nome da
	// mesma entrada) não pode render/carregar a skill duas vezes.
	ordered := skills.CatalogByNamesOrdered(all, enabledSkills)
	inAllowlist := make(map[string]bool, len(ordered))
	pool = make([]skills.SkillCatalogEntry, 0, len(all))
	allowlistSlugs = make([]string, 0, len(ordered))
	for _, e := range ordered {
		if inAllowlist[e.Slug] {
			continue
		}
		inAllowlist[e.Slug] = true
		allowlistSlugs = append(allowlistSlugs, e.Slug)
		pool = append(pool, e)
	}

	// As "demais" skills são excluídas por SLUG das entradas resolvidas (não pelos
	// nomes crus de enabledSkills): assim uma skill cujo NOME colida com um slug da
	// allowlist não é removida por engano e continua disponível como sob demanda.
	for _, e := range all {
		if !inAllowlist[e.Slug] {
			pool = append(pool, e)
		}
	}
	return pool, allowlistSlugs, nil
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
	// nome + slug + descrição + path + overhead de formatação/markdown. Conta runes
	// (não bytes) para manter a unidade consistente com a documentação ("caracteres")
	// e com truncateDescription, evitando subestimar descrições com runes multibyte.
	return utf8.RuneCountInString(e.GetDisplayName()) +
		utf8.RuneCountInString(e.Slug) +
		utf8.RuneCountInString(desc) +
		utf8.RuneCountInString(e.Path) + 32
}

// truncateDescription encurta uma descrição para no máximo max runes (contando o
// "…" anexado), de modo a não estourar o budget planejado.
func truncateDescription(desc string, max int) string {
	if max <= 0 {
		return desc
	}
	runes := []rune(desc)
	if len(runes) <= max {
		return desc
	}
	// Reserva 1 rune para o "…" para que o resultado tenha no máximo max runes.
	return strings.TrimSpace(string(runes[:max-1])) + "…"
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
