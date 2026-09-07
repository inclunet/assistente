package skills

import (
	"assistente/internal/logging"
	"context"
	"fmt"
	"strings"
)

// InvokerManager é a interface mínima necessária para invocar um skill via slash command.
type InvokerManager interface {
	Get(slug string) (*Skill, error)
	GetSkillFiles(slug string) ([]string, error)
}

// InvocationResult contém o resultado de uma invocação de skill via /slash command.
type InvocationResult struct {
	// SkillSlug é o identificador do skill invocado.
	SkillSlug string
	// DisplayName é o nome amigável do skill invocado.
	DisplayName string
	// Content é o bloco XML pronto para injetar no system prompt.
	Content string
	// Arguments contém os argumentos passados após o slash command.
	Arguments string
	// Mode é o modo efetivo do skill no perfil ativo.
	Mode SkillMode
	// Filesystem contém as permissões de filesystem declaradas pelo skill (nil se não definido).
	Filesystem *FilesystemPermissions
	// Tools contém as permissões de tools declaradas pelo skill (nil se não definido).
	Tools *ToolPermissions
	// Network contém as permissões de rede declaradas pelo skill (nil se não definido).
	Network *NetworkPermissions
}

// ParseSlashCommand detecta se uma mensagem é um slash command para invocar um skill.
// Formato: /skill-slug [argumentos...]
// Retorna (slug, args, true) se for um slash command válido, ("", "", false) caso contrário.
func ParseSlashCommand(content string) (slug string, args string, ok bool) {
	content = strings.TrimSpace(content)

	if !strings.HasPrefix(content, "/") {
		return "", "", false
	}

	rest := content[1:]
	if rest == "" {
		return "", "", false
	}

	// Evita confusão com paths como "/ algo"
	if rest[0] == ' ' {
		return "", "", false
	}

	parts := strings.SplitN(rest, " ", 2)
	slug = strings.ToLower(parts[0])

	// Valida que o slug parece um nome de skill (letras minúsculas, números, hifens)
	for _, ch := range slug {
		if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') && ch != '-' {
			return "", "", false
		}
	}

	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}

	return slug, args, true
}

// Invoke tenta invocar um skill via slash command no conteúdo da mensagem do usuário.
//
//   - mgr: repositório de skills para busca e listagem de arquivos complementares.
//   - tplData: parâmetro legado mantido para compatibilidade de assinatura; templates não são executados no runtime novo.
//   - sessionID: ID numérico da conversa, disponibilizado como variável $CLAUDE_SESSION_ID.
//
// Retorna (resultado, encontrado, erro).
// found=false significa que o conteúdo não é um slash command ou o skill não existe.
func Invoke(userContent string, mgr InvokerManager, tplData any, sessionID string, policy SelectionPolicy) (*InvocationResult, bool, error) {
	slug, args, ok := ParseSlashCommand(userContent)
	if !ok || mgr == nil {
		return nil, false, nil
	}

	skill, err := mgr.Get(slug)
	if err != nil {
		logging.Errorf(context.Background(), "skills.invocation", "[Skills] Skill /%s não encontrado: %v", slug, err)
		return nil, false, nil
	}

	if !skill.IsUserInvocable() {
		return nil, false, nil
	}
	mode := policy.ModeFor(slug)
	if mode == SkillModeDisabled {
		return nil, true, fmt.Errorf("skill /%s está desabilitada no perfil ativo", slug)
	}

	logging.Infof(context.Background(), "skills.invocation", "[Skills] Slash command detectado: /%s args=%q", slug, args)

	// Substitui $ARGUMENTS, $N e variáveis de sessão no conteúdo
	sessionVars := map[string]string{
		"CLAUDE_SESSION_ID": sessionID,
	}
	processedContent := SubstituteArguments(skill.Content, args, sessionVars)

	// Preprocessa !commands (respeita permissões de bash do skill)
	var allowedBashCmds []string
	if skill.Tools != nil && skill.Tools.BashCommands != nil {
		allowedBashCmds = skill.Tools.BashCommands.Allowed
	}
	processedContent = PreprocessCommands(processedContent, allowedBashCmds)

	// Monta seção XML do skill invocado
	var sb strings.Builder
	sb.WriteString("<invoked_skill>\n## ")
	sb.WriteString(skill.GetDisplayName())
	if skill.Type != "" {
		sb.WriteString(" [")
		sb.WriteString(skill.Type)
		sb.WriteString("]")
	}
	sb.WriteString("\n")
	sb.WriteString(processedContent)
	sb.WriteString("\n")

	// Progressive file loading: lista arquivos complementares do skill
	supplementary, _ := mgr.GetSkillFiles(slug)
	if len(supplementary) > 0 {
		sb.WriteString("\nSupporting files (use read_file to access when needed):\n")
		for _, f := range supplementary {
			sb.WriteString("- `")
			sb.WriteString(f)
			sb.WriteString("`\n")
		}
	}
	sb.WriteString("</invoked_skill>")

	result := &InvocationResult{
		SkillSlug:   slug,
		DisplayName: skill.GetDisplayName(),
		Content:     sb.String(),
		Arguments:   args,
		Mode:        mode,
	}
	if skill.Filesystem != nil {
		result.Filesystem = skill.Filesystem
	}
	if skill.Tools != nil {
		result.Tools = skill.Tools
	}
	if skill.Network != nil {
		result.Network = skill.Network
	}

	return result, true, nil
}
