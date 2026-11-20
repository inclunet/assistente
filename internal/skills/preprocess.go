package skills

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

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
	budget   int
	run      SkillCommandRunner
	prepared *preparedSkillContext
	err      error
}

// WithCommandTurn scopes prepared base content to one prompt construction.
func WithCommandTurn(ctx context.Context, run SkillCommandRunner) context.Context {
	return context.WithValue(ctx, commandTurnKey{}, &commandTurn{budget: -1, run: run})
}

// WithSlashCommandTurn reserves the provider's budget for this invocation.
func WithSlashCommandTurn(ctx context.Context, run SkillCommandRunner, budget int) context.Context {
	return context.WithValue(ctx, commandTurnKey{}, &commandTurn{run: run, budget: budget})
}

func CommandTurnError(ctx context.Context) error {
	turn, _ := ctx.Value(commandTurnKey{}).(*commandTurn)
	if turn == nil {
		return nil
	}
	return turn.err
}

type commandLine struct {
	index   int
	command string
}
type commandDocument struct {
	lines    []string
	commands []commandLine
}

var preformattedHTML = regexp.MustCompile(`(?i)^[ \t]*<pre(?:[ \t\r\n>]|$)`)
var shellArgumentReference = regexp.MustCompile(`\$(?:ARGUMENTS|[0-9]+)`)
var shellPosition = regexp.MustCompile(`\$(?:ARGUMENTS\[([0-9]+)\]|([0-9]+))`)
var parentArgumentSegment = regexp.MustCompile(`(?:^|[ /\\=:"'])\.\.(?:[ /\\:"']|$)`)
var shellArgumentText = regexp.MustCompile(`^(?:[\pL\pN._][\pL\pN._/-]*(?: +[\pL\pN._][\pL\pN._/-]*)*)?$`)

// O parser CommonMark resolve o recuo relativo de cercas em listas e citações.
func codeLineStarts(content string) map[int]bool {
	if !strings.Contains(content, "!") {
		return nil
	}
	source := []byte(content)
	root := goldmark.DefaultParser().Parse(text.NewReader(source))
	starts := make(map[int]bool)
	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		literal := node.Kind() == ast.KindFencedCodeBlock || node.Kind() == ast.KindCodeBlock
		if block, ok := node.(*ast.HTMLBlock); ok && block.HTMLBlockType == ast.HTMLBlockType1 && block.Lines().Len() > 0 {
			first := block.Lines().At(0)
			literal = preformattedHTML.Match(first.Value(source))
			if literal && block.HasClosure() {
				starts[bytes.LastIndexByte(source[:block.ClosureLine.Start], '\n')+1] = true
			}
		}
		if literal {
			for i := 0; i < node.Lines().Len(); i++ {
				pos := node.Lines().At(i).Start
				starts[bytes.LastIndexByte(source[:pos], '\n')+1] = true
			}
		}
		return ast.WalkContinue, nil
	})
	return starts
}

// Inspecione só os operandos que recebem dados da invocação. A análise da
// política sobre a linha inteira continua obrigatória depois da substituição.
func validInterpolatedCommand(rawArgs, template string, expand func(string) string) bool {
	if !shellArgumentText.MatchString(strings.Trim(rawArgs, " ")) || parentArgumentSegment.MatchString(rawArgs) {
		return false
	}
	if positions := shellPosition.FindAllStringSubmatch(template, -1); len(positions) > 0 {
		count := len(parseArgs(rawArgs))
		for _, pos := range positions {
			index, err := strconv.Atoi(pos[1] + pos[2])
			if err != nil || index >= count {
				return false
			}
		}
	}
	// Para verificação de componentes, ambos os separadores de caminho têm o
	// mesmo significado. O texto enviado ao executor conserva a grafia original.
	parsed := commandpolicy.Parse(strings.ReplaceAll(template, "\\", "/"))
	if len(parsed.Commands) == 0 || len(parsed.Errors) > 0 {
		return false
	}
	for _, cmd := range parsed.Commands {
		if shellArgumentReference.MatchString(cmd.Program) {
			return false
		}
		for _, operand := range cmd.Args {
			if shellArgumentReference.MatchString(operand) && parentArgumentSegment.MatchString(expand(operand)) {
				return false
			}
		}
	}
	return true
}

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
	fenced := codeLineStarts(content)
	offset := 0
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		literal := fenced[offset]
		offset += len(line) + 1
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
			if shellArgumentReference.MatchString(command) && !validInterpolatedCommand(rawArgs, command, expand) {
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
		if !shellToolAllowed {
			return commandDocument{}, fmt.Errorf("skill %q sem permissão para run_command", skill.Slug)
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

func (d commandDocument) render(ctx context.Context, skill *Skill, run SkillCommandRunner, base bool, remaining int) (string, error) {
	return d.renderLines(ctx, skill, base, remaining, func(line commandLine) (string, error) {
		output, err := run(ctx, skill, line.command)
		// Protege também runners injetados que não usam o serviço de invocação.
		if err == nil && len(output) > MaxCommandOutputSize {
			return "", fmt.Errorf("saída excedeu o limite da skill %q", skill.Slug)
		}
		return strings.TrimRight(output, "\r\n"), err
	})
}

func (d commandDocument) renderLines(ctx context.Context, skill *Skill, base bool, remaining int, result func(commandLine) (string, error)) (string, error) {
	for idx, line := range d.commands {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		output, err := result(line)
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if err != nil {
			if !base || errors.Is(err, contextprovider.ErrCommandRefused) {
				return "", err
			}
			logging.Logger(ctx, "skills.preprocess").Warn("Linha omitida após falha do comando da skill base", "skill", skill.Slug)
			d.lines[line.index] = ""
			continue
		}
		if remaining >= 0 {
			quota := remaining / (len(d.commands) - idx)
			size := utf8.RuneCountInString(output)
			if size > quota {
				output = string([]rune(output)[:quota-1]) + "…"
				size = quota
			}
			remaining -= size
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
