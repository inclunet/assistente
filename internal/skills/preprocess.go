package skills

import (
	"context"
	"fmt"
	"strings"
	"time"

	"assistente/internal/allowlist"
	"assistente/internal/commandpolicy"
)

const (
	// DefaultCommandTimeout é o timeout padrão para execução de !commands (5 segundos).
	DefaultCommandTimeout = 5 * time.Second
	// MaxCommandOutputSize é o tamanho máximo da saída de um comando (100KB).
	MaxCommandOutputSize = 100 * 1024
)

// SkillCommandRunner uses the application's authenticated shell execution path.
type SkillCommandRunner func(context.Context, *Skill, string) (string, error)

type commandTurnKey struct{}
type commandTurn struct {
	run  SkillCommandRunner
	base map[string]string
	err  error
}

// WithCommandTurn scopes prepared base content to one prompt construction.
func WithCommandTurn(ctx context.Context, run SkillCommandRunner) context.Context {
	return context.WithValue(ctx, commandTurnKey{}, &commandTurn{run: run, base: make(map[string]string)})
}

func CommandTurnError(ctx context.Context) error {
	turn, _ := ctx.Value(commandTurnKey{}).(*commandTurn)
	if turn == nil {
		return nil
	}
	return turn.err
}

func prepareCommands(ctx context.Context, skill *Skill, content string, base bool, expand func(string) string) (string, error) {
	turn, _ := ctx.Value(commandTurnKey{}).(*commandTurn)
	if turn != nil && turn.err != nil {
		return "", turn.err
	}
	if base && turn != nil {
		if value, ok := turn.base[skill.Slug]; ok {
			return value, nil
		}
	}
	var run SkillCommandRunner
	if turn != nil {
		run = turn.run
	}
	result, err := renderCommands(ctx, skill, content, run, expand)
	if turn != nil {
		if err != nil {
			turn.err = err
		} else if base {
			turn.base[skill.Slug] = result
		}
	}
	return result, err
}

func renderCommands(ctx context.Context, skill *Skill, content string, run SkillCommandRunner, expand func(string) string) (string, error) {
	if skill.Source == "plugin" && strings.HasPrefix(skill.Slug, PluginPrefix) {
		if skill.PackageFormat == "claude-code" {
			return renderClaudeCommands(ctx, skill, content, run, expand)
		}
		if expand != nil {
			content = expand(content)
		}
		return content, nil
	}
	lines := strings.Split(content, "\n")
	var allowed []string
	if skill.Tools != nil && skill.Tools.BashCommands != nil {
		allowed = skill.Tools.BashCommands.Allowed
	}
	policy := buildSkillPolicy(allowed)
	if policy != nil && skill.Tools != nil && skill.Tools.BashCommands != nil {
		policy.AlwaysDeny = append(policy.AlwaysDeny, skill.Tools.BashCommands.Denied...)
		for _, name := range skill.Tools.BashCommands.Denied {
			policy.CommandRules = append(policy.CommandRules, allowlist.CommandRule{Program: name, Subcommands: []string{"*"}, Decision: allowlist.DecisionDeny.String()})
		}
	}
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !commandLine(trimmed) {
			if expand != nil {
				lines[idx] = expand(line)
			}
			continue
		}
		command := stripBackticks(strings.TrimSpace(trimmed[1:]))
		if expand != nil {
			command = expand(command)
		}
		if strings.ContainsAny(command, "\r\n") {
			return "", fmt.Errorf("comando de skill deve ocupar uma linha")
		}
		if ok, reason := evaluateSkillCommand(command, policy); !ok {
			return "", fmt.Errorf("comando bloqueado para a skill %q: %s", skill.Slug, sanitizeHTMLCommentText(reason))
		}
		if run == nil {
			return "", fmt.Errorf("executor de comandos indisponível para a skill %q", skill.Slug)
		}
		output, err := run(ctx, skill, command)
		if err != nil {
			return "", err
		}
		if len(output) > MaxCommandOutputSize {
			return "", fmt.Errorf("saída excedeu o limite da skill %q", skill.Slug)
		}
		lines[idx] = strings.TrimRight(output, "\r\n")
	}
	return strings.Join(lines, "\n"), nil
}

// stripBackticks remove backticks envolvendo o comando.
// Suporta: `command` → command, “command“ → command
func stripBackticks(cmd string) string {
	// Remove backticks do início e fim
	if len(cmd) >= 2 && cmd[0] == '`' && cmd[len(cmd)-1] == '`' {
		cmd = cmd[1 : len(cmd)-1]
	}
	return strings.TrimSpace(cmd)
}

// buildSkillPolicy monta uma allowlist sintética default-deny a partir da
// lista de executáveis permitidos declarada pelo skill (tools.bashCommands.allowed).
// Cada executável vira uma regra estruturada `approve` com wildcard de cauda
// (qualquer subcomando/args), preservando a semântica anterior de restrição
// por executável (case-insensitive). Retorna nil quando a lista é nil/vazia,
// sinalizando bloqueio total (default-deny).
func buildSkillPolicy(allowedCommands []string) *allowlist.Allowlist {
	rules := make([]allowlist.CommandRule, 0, len(allowedCommands))
	for _, prog := range allowedCommands {
		prog = strings.TrimSpace(prog)
		if prog == "" {
			continue
		}
		rules = append(rules, allowlist.CommandRule{
			Program:     prog,
			Subcommands: []string{"*"},
			Decision:    allowlist.DecisionApprove.String(),
			Description: "permitido por tools.bashCommands do skill",
		})
	}
	if len(rules) == 0 {
		return nil
	}
	return &allowlist.Allowlist{
		Name:          "skill-preprocess",
		CommandRules:  rules,
		DefaultAction: allowlist.DecisionDeny.String(),
	}
}

// evaluateSkillCommand decide se um !command de skill pode ser executado.
//
// Default-deny: policy nil (skill sem allowlist de bash) bloqueia tudo.
// Com policy, o comando inteiro passa por commandpolicy.Evaluate: comandos
// compostos são decompostos e cada átomo é avaliado individualmente; features
// de shell não suportadas (pipe, redirecionamento, substituição de comando,
// heredoc, background, env inline, sintaxe ambígua) resultam em confirm.
// Como o preprocess não tem fluxo de confirmação, só approve executa.
//
// O reason retornado usa exclusivamente EvaluationResult.Reasons (canal safe,
// sem patterns/descrições da allowlist), pois o comentário de bloqueio é
// injetado no system prompt enviado ao LLM.
func evaluateSkillCommand(cmd string, policy *allowlist.Allowlist) (allowed bool, reason string) {
	if policy == nil {
		return false, "skill não declara comandos bash permitidos"
	}

	result := commandpolicy.Evaluate(cmd, policy)
	if result.Decision == allowlist.DecisionApprove {
		return true, ""
	}

	if len(result.Reasons) == 0 {
		return false, "não permitido pela política de comandos"
	}
	return false, strings.Join(result.Reasons, "; ")
}

// sanitizeHTMLCommentText evita que texto vindo do parser/policy feche ou
// quebre o comentário HTML injetado no prompt do LLM.
func sanitizeHTMLCommentText(text string) string {
	for strings.Contains(text, "--") {
		text = strings.ReplaceAll(text, "--", "-")
	}
	text = strings.ReplaceAll(text, ">", "&gt;")
	return text
}

func commandLine(trimmed string) bool { return len(trimmed) >= 2 && trimmed[0] == '!' }
func ContainsShellCommands(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		if commandLine(strings.TrimSpace(line)) {
			return true
		}
	}
	return false
}
