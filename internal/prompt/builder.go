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
func (b *Builder) BuildTemplateData(activeProfile *profiles.Profile, params llm.ChatParams, conversationID uint) TemplateData {
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
			sb.WriteString("You CAN read and edit these files using the filesystem tools (read_file, write_file, edit_file), ")
			sb.WriteString("even if they are outside the working directory:\n")
			for _, p := range paths {
				sb.WriteString("- ")
				sb.WriteString(p)
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
		sb.WriteString("You have skills available that provide specialized instructions for specific tasks.\n")
		sb.WriteString("To use a skill, read its file using the read_file tool with the path indicated below.\n")
		sb.WriteString("Only read a skill when it's relevant to the current task.\n\n")
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
			sb.WriteString("\n  Path: `")
			sb.WriteString(s.Path)
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

// ComputeEnabledToolNames retorna a lista de nomes de tools habilitadas pelo perfil.
func (b *Builder) ComputeEnabledToolNames(activeProfile *profiles.Profile) []string {
	if activeProfile != nil && activeProfile.Chat.DisableTools {
		return nil
	}
	if b.Tools == nil || b.Tools.Count() == 0 {
		return nil
	}

	var defs []tools.ToolDefinition
	if activeProfile != nil && activeProfile.Chat.EnabledTools != nil {
		defs = b.Tools.FilterByNames(activeProfile.Chat.EnabledTools)
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
