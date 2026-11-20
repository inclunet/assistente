package skills

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"assistente/internal/allowlist"
	"assistente/internal/commandpolicy"
	"assistente/internal/contextprovider"
	"assistente/internal/logging"
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

// A preparação do prompt e os providers acessam este estado sequencialmente.
type commandTurn struct {
	budget int
	run    SkillCommandRunner
	base   map[string]string
	err    error
}

// WithCommandTurn scopes prepared base content to one prompt construction.
func WithCommandTurn(ctx context.Context, run SkillCommandRunner) context.Context {
	return context.WithValue(ctx, commandTurnKey{}, &commandTurn{budget: -1, run: run, base: make(map[string]string)})
}

// WithSlashCommandTurn reserves the provider's budget for this invocation.
func WithSlashCommandTurn(ctx context.Context, run SkillCommandRunner, budget int) context.Context {
	return context.WithValue(ctx, commandTurnKey{}, &commandTurn{run: run, budget: budget, base: make(map[string]string)})
}

func CommandTurnError(ctx context.Context) error {
	turn, _ := ctx.Value(commandTurnKey{}).(*commandTurn)
	if turn == nil {
		return nil
	}
	return turn.err
}

func prepareBaseCommands(ctx context.Context, skill *Skill) (string, error) {
	turn, _ := ctx.Value(commandTurnKey{}).(*commandTurn)
	var run SkillCommandRunner
	if turn != nil {
		if turn.err != nil {
			return "", turn.err
		}
		if content, ok := turn.base[skill.Slug]; ok {
			return content, nil
		}
		run = turn.run
	}
	content, err := renderCommands(ctx, skill, skill.Content, true, run, nil)
	if turn != nil {
		if err != nil {
			turn.err = err
		} else {
			turn.base[skill.Slug] = content
		}
	}
	return content, err
}

func renderCommands(ctx context.Context, skill *Skill, content string, base bool, run SkillCommandRunner, expand func(string) string) (string, error) {
	doc, err := planCommandLines(ctx, skill, content, base, run, expand, "")
	if err != nil {
		return "", err
	}
	return doc.render(ctx, skill, run, base, -1)
}

type commandLine struct {
	index   int
	command string
}
type commandDocument struct {
	lines    []string
	commands []commandLine
}

var shellArgumentReference = regexp.MustCompile(`\$(?:ARGUMENTS|[0-9]+)`)
var shellArgumentText = regexp.MustCompile(`^[\pL\pN ._/@:+,=-]*$`)

func planCommandLines(ctx context.Context, skill *Skill, content string, base bool, run SkillCommandRunner, expand func(string) string, rawArgs string) (commandDocument, error) {
	var commands []commandLine
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
	shellToolAllowed := skill.Tools == nil || (!slices.Contains(skill.Tools.Denied, "run_command") &&
		(len(skill.Tools.Allowed) == 0 || slices.Contains(skill.Tools.Allowed, "run_command")))
	var fence string
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		indented := len(indent) >= 4 || strings.Contains(indent, "\t")
		literal := indented || fence != ""
		if !indented && len(trimmed) > 0 && (trimmed[0] == '~' || trimmed[0] == '`') {
			n := len(trimmed) - len(strings.TrimLeft(trimmed, string(trimmed[0])))
			if n >= 3 && fence == "" {
				fence = trimmed[:n]
				literal = true
			} else if n >= len(fence) && fence != "" && trimmed[0] == fence[0] && strings.TrimSpace(trimmed[n:]) == "" {
				fence = ""
			}
		}
		if literal || len(trimmed) < 2 || trimmed[0] != '!' || strings.HasPrefix(trimmed, "![") {
			if expand != nil {
				lines[idx] = expand(line)
			}
			continue
		}
		command := stripBackticks(strings.TrimSpace(trimmed[1:]))
		// A forma explícita é sempre uma diretiva. A forma legada só é
		// reconhecida quando o primeiro programa foi declarado pela skill.
		explicit := strings.HasPrefix(strings.TrimSpace(trimmed[1:]), "`")
		words := strings.Fields(command)
		declared := len(words) > 0 && slices.ContainsFunc(allowed, func(name string) bool { return strings.EqualFold(name, words[0]) })
		if !explicit && !declared {
			if expand != nil {
				lines[idx] = expand(line)
			}
			continue
		}
		if expand != nil {
			expanded := expand(command)
			if expanded != command && shellArgumentReference.MatchString(command) && !shellArgumentText.MatchString(rawArgs) {
				return commandDocument{}, fmt.Errorf("argumentos incompatíveis com uma linha de comando da skill %q", skill.Slug)
			}
			command = expanded
		}
		if strings.ContainsAny(command, "\r\n") {
			return commandDocument{}, fmt.Errorf("comando de skill deve ocupar uma linha")
		}
		allowed, reason := evaluateSkillCommand(command, policy)
		if base && (!allowed || !shellToolAllowed || run == nil) {
			logging.Warnf(ctx, "skills.preprocess", "Comando omitido da skill base %q por incompatibilidade com as permissões disponíveis", skill.Slug)
			lines[idx] = ""
			continue
		}
		if !allowed {
			return commandDocument{}, fmt.Errorf("comando bloqueado para a skill %q: %s", skill.Slug, reason)
		}
		if run == nil {
			return commandDocument{}, fmt.Errorf("executor de comandos indisponível para a skill %q", skill.Slug)
		}
		lines[idx] = ""
		commands = append(commands, commandLine{idx, command})
	}
	return commandDocument{lines, commands}, nil
}

func (d commandDocument) render(ctx context.Context, skill *Skill, run SkillCommandRunner, base bool, quota int) (string, error) {
	for _, line := range d.commands {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		output, err := run(ctx, skill, line.command)
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		// Protege também os runners injetados que não usam o serviço de invocação.
		if err == nil && len(output) > MaxCommandOutputSize {
			err = fmt.Errorf("saída excedeu o limite da skill %q", skill.Slug)
		}
		if err != nil {
			if !base || errors.Is(err, contextprovider.ErrCommandRefused) {
				return "", err
			}
			logging.Logger(ctx, "skills.preprocess").Warn("Linha omitida após falha do comando da skill base", "skill", skill.Slug)
			d.lines[line.index] = ""
			continue
		}
		output = strings.TrimRight(output, "\r\n")
		if quota >= 0 && utf8.RuneCountInString(output) > quota {
			output = string([]rune(output)[:quota-1]) + "…"
		}
		d.lines[line.index] = output
	}
	return strings.Join(d.lines, "\n"), nil
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
// sem patterns/descrições da allowlist) para a mensagem de erro do usuário.
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
