package skills

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"assistente/internal/logging"
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

// InvokeContext prepara a skill invocada dentro do contexto do turno.
func InvokeContext(ctx context.Context, userContent string, mgr InvokerManager, sessionID string, policy SelectionPolicy) (*InvocationResult, bool, error) {
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

	logging.Logger(ctx, "skills.invocation").Info("Slash command detectado", "skill", slug)

	var content string
	if mode != SkillModeBase {
		turn, _ := ctx.Value(commandTurnKey{}).(*commandTurn)
		var run SkillCommandRunner
		budget := -1
		if turn != nil {
			run = turn.run
			budget = turn.budget
		}
		doc, err := planCommandLines(ctx, skill, skill.Content, false, run, argumentExpander(args, parseArgs(args), map[string]string{"CLAUDE_SESSION_ID": sessionID}), args)
		if err != nil {
			return nil, true, err
		}
		files, err := invocationFiles(mgr, slug, skill.Path)
		if err != nil {
			warnSupportingFiles(ctx, slug, err, false)
			if len(doc.commands) > 0 {
				return nil, true, fmt.Errorf("não foi possível listar os arquivos de apoio da skill %q", slug)
			}
			files = nil
		}
		before, after := invocationFrame(skill, files)
		after = argumentFallback(skill.Content, args) + after
		quota := -1
		if len(doc.commands) > 0 && budget >= 0 {
			static := utf8.RuneCountInString(before) + utf8.RuneCountInString(strings.Join(doc.lines, "\n")) + utf8.RuneCountInString(after)
			quota = budget - static
			if quota/len(doc.commands) < 2 {
				return nil, true, fmt.Errorf("orçamento insuficiente para as instruções e os resultados da skill %q", slug)
			}
		}
		body, err := doc.render(ctx, skill, run, false, quota, nil)
		if err != nil {
			return nil, true, err
		}
		content = before + body + after
	}
	result := &InvocationResult{
		SkillSlug:   slug,
		DisplayName: skill.GetDisplayName(),
		Content:     content,
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

func invocationFiles(mgr InvokerManager, slug, manifestPath string) ([]string, error) {
	if source, ok := mgr.(supportingFileLister); ok && manifestPath != "" {
		return source.listSupportingFiles(manifestPath)
	}
	// Adaptadores que só expõem o catálogo por slug conservam sua própria
	// resolução de origem; não é possível fixar o manifesto nesse contrato.
	return mgr.GetSkillFiles(slug)
}

func invocationFrame(skill *Skill, supplementary []string) (string, string) {
	var sb strings.Builder
	sb.WriteString("<invoked_skill>\n## ")
	sb.WriteString(skill.GetDisplayName())
	if skill.Type != "" {
		sb.WriteString(" [")
		sb.WriteString(skill.Type)
		sb.WriteString("]")
	}
	sb.WriteString("\n")
	before := sb.String()
	sb.Reset()
	sb.WriteString("\n")

	// Progressive file loading: lista arquivos complementares do skill
	if len(supplementary) > 0 {
		sb.WriteString("\nSupporting files (use read_file to access when needed):\n")
		for _, f := range supplementary {
			sb.WriteString("- `")
			sb.WriteString(f)
			sb.WriteString("`\n")
		}
	}
	sb.WriteString("</invoked_skill>")

	return before, sb.String()
}
