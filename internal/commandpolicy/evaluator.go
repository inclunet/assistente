package commandpolicy

import (
	"fmt"
	"strings"

	"assistente/internal/allowlist"
)

// EvaluationResult descreve a decisao final para uma linha de comando.
//
// Contrato de seguranca: Reasons e o unico campo seguro para enviar ao LLM
// (ou qualquer canal externo, como telemetria). DetailedReasons existe para
// uso LOCAL apenas (logs do app, UI dentro do desktop) — pode replicar
// patterns legados, subcommands/args/descricoes de regras estruturadas e
// outros conteudos que o usuario configurou na allowlist e que podem incluir
// dados sensiveis (URLs internas, hostnames, descricoes com identificadores).
//
// Quando uma reason nao envolve conteudo controlado pelo usuario (ex.: "comando
// atomico vazio", "feature conservadora detectada: pipe"), o mesmo texto vai
// para os dois slices — eles tem sempre o mesmo tamanho e a mesma ordem, para
// que callers possam correlacionar uma posicao na UI com seu detalhe.
type EvaluationResult struct {
	Decision        allowlist.Decision
	Parse           ParseResult
	Reasons         []string
	DetailedReasons []string
}

// Evaluate parseia e avalia uma linha de comando contra a allowlist.
func Evaluate(commandLine string, al *allowlist.Allowlist) EvaluationResult {
	trimmed := strings.TrimSpace(commandLine)
	if trimmed == "" {
		return EvaluationResult{
			Decision:        allowlist.DecisionDeny,
			Reasons:         []string{"comando vazio"},
			DetailedReasons: []string{"comando vazio"},
		}
	}

	parsed := Parse(commandLine)
	result := EvaluationResult{
		Decision: allowlist.DecisionApprove,
		Parse:    parsed,
	}

	for _, cmd := range parsed.Commands {
		decision, safe, detailed := evaluateAtom(cmd, al)
		result.Reasons = append(result.Reasons, safe)
		result.DetailedReasons = append(result.DetailedReasons, detailed)
		result.Decision = combineDecision(result.Decision, decision)
	}

	if len(parsed.Commands) == 0 {
		result.Decision = combineDecision(result.Decision, allowlist.DecisionConfirm)
		// Evita duplicar com parsed.Errors, que ja contem a mesma mensagem quando
		// o parser nao reconhece nenhum atomo (e que sera reformatada como
		// "sintaxe ambigua" abaixo).
		if !containsString(parsed.Errors, "nenhum comando atomico reconhecido") {
			result.appendReason("nenhum comando atomico reconhecido")
		}
	}

	if parsed.RequiresConfirmation() && result.Decision != allowlist.DecisionDeny {
		result.Decision = combineDecision(result.Decision, allowlist.DecisionConfirm)
		// Features sao um enum fechado em types.go — nomes sao seguros.
		// Erros do parser sao mensagens estaticas ou interpolam apenas tokens
		// de operador (`;`, `&&`, etc.), tambem seguros.
		for _, feature := range parsed.Features {
			result.appendReason(fmt.Sprintf("feature conservadora detectada: %s", feature))
		}
		for _, err := range parsed.Errors {
			result.appendReason(fmt.Sprintf("sintaxe ambigua: %s", err))
		}
	}

	return result
}

// appendReason adiciona o mesmo texto a Reasons e DetailedReasons. Use apenas
// quando o conteudo nao envolve dados controlados pelo usuario (patterns,
// subcommands, args, description). Caso contrario, append explicito nos dois
// slices com versoes diferentes.
func (r *EvaluationResult) appendReason(reason string) {
	r.Reasons = append(r.Reasons, reason)
	r.DetailedReasons = append(r.DetailedReasons, reason)
}

// evaluateAtom avalia um unico comando atomico contra a allowlist e devolve
// uma decisao + duas reasons:
//
//   - safe: para envio externo (LLM, telemetria). Inclui apenas cmd.Program,
//     o tipo da regra e um indice de correlacao (rule[N], always_deny[N],
//     auto_approve[N]). Nunca interpola pattern bruto, subcommands, args
//     ou description — todos campos controlados pelo usuario que podem
//     conter dados sensiveis (URLs internas, hostnames, identificadores).
//   - detailed: para uso LOCAL (logs do app, UI dentro do desktop). Mantem
//     a forma verbosa anterior, citando o pattern legado completo ou o
//     describeRule(rule). O usuario ja tem visibilidade desses valores na
//     UI da allowlist, entao expor localmente nao introduz risco novo.
//
// Quando a reason nao envolve conteudo do usuario (ex.: "comando atomico vazio",
// "sem allowlist ativa", default_action), os dois textos sao iguais.
//
// Precedencia interna (fail-closed) quando varias regras casam o mesmo atomo:
//
//  1. structured deny
//  2. legacy always_deny
//  3. structured confirm     // antes de approve para preservar excecoes restritivas
//  4. structured approve
//  5. legacy auto_approve
//  6. default_action da allowlist
//
// Esta ordem é diferente da agregacao entre atomos compostos
// (deny > confirm > approve, em ParseResult/Evaluate): aqui ela existe para
// que um usuario consiga adicionar uma regra restritiva (ex.:
// "kubectl get secret confirm") sobre uma regra mais permissiva
// ("kubectl get * approve") sem precisar reordenar. Documentado tambem em
// aep/0060-command-policy-parser.md.
func evaluateAtom(cmd Command, al *allowlist.Allowlist) (decision allowlist.Decision, safe, detailed string) {
	// safeProgram protege contra qualquer Program que tenha escapado da
	// limpeza do parser e ainda contenha "=" (env inline nao consumido por
	// algum motivo, perfis legados, configuracoes manuais). Defesa em
	// profundidade: as Reasons jamais devem replicar o lado direito do "=".
	prog := redactProgramForReason(cmd.Program)

	if al == nil {
		msg := fmt.Sprintf("%q exige confirmacao: sem allowlist ativa", prog)
		return allowlist.DecisionConfirm, msg, msg
	}

	if cmd.Program == "" {
		return allowlist.DecisionDeny, "comando atomico vazio", "comando atomico vazio"
	}

	if rule, idx, ok := matchStructuredRule(cmd, al.CommandRules, allowlist.DecisionDeny); ok {
		return allowlist.DecisionDeny,
			fmt.Sprintf("%q bloqueado por regra estruturada (rule[%d])", prog, idx),
			fmt.Sprintf("%q bloqueado por regra estruturada: %s", prog, describeRule(rule))
	}

	for i, pattern := range al.AlwaysDeny {
		if matchesLegacyPattern(cmd, pattern) {
			return allowlist.DecisionDeny,
				fmt.Sprintf("%q bloqueado por always_deny[%d]", prog, i),
				fmt.Sprintf("%q bloqueado por always_deny: %s", prog, pattern)
		}
	}

	if rule, idx, ok := matchStructuredRule(cmd, al.CommandRules, allowlist.DecisionConfirm); ok {
		return allowlist.DecisionConfirm,
			fmt.Sprintf("%q exige confirmacao por regra estruturada (rule[%d])", prog, idx),
			fmt.Sprintf("%q exige confirmacao por regra estruturada: %s", prog, describeRule(rule))
	}

	if rule, idx, ok := matchStructuredRule(cmd, al.CommandRules, allowlist.DecisionApprove); ok {
		return allowlist.DecisionApprove,
			fmt.Sprintf("%q aprovado por regra estruturada (rule[%d])", prog, idx),
			fmt.Sprintf("%q aprovado por regra estruturada: %s", prog, describeRule(rule))
	}

	for i, pattern := range al.AutoApprove {
		if matchesLegacyPattern(cmd, pattern) {
			return allowlist.DecisionApprove,
				fmt.Sprintf("%q aprovado por auto_approve[%d]", prog, i),
				fmt.Sprintf("%q aprovado por auto_approve: %s", prog, pattern)
		}
	}

	switch strings.ToLower(al.DefaultAction) {
	case "deny":
		msg := fmt.Sprintf("%q bloqueado por default_action=deny", prog)
		return allowlist.DecisionDeny, msg, msg
	default:
		msg := fmt.Sprintf("%q exige confirmacao por default_action=confirm", prog)
		return allowlist.DecisionConfirm, msg, msg
	}
}

// redactProgramForReason garante que o nome do programa interpolado em
// reasons jamais carregue o valor de uma atribuicao inline. O parser ja
// consome KEY=VALUE em Command.EnvAssignments, mas mantemos esta funcao
// como defesa em profundidade para perfis legados/manuais e para qualquer
// caminho futuro que crie Commands fora do parser. A regra: se Program tem
// "=", devolvemos so a parte ANTES do primeiro "=", anexando "=<redacted>"
// para sinalizar a redaction sem revelar o valor.
func redactProgramForReason(program string) string {
	eq := strings.IndexByte(program, '=')
	if eq < 0 {
		return program
	}
	return program[:eq] + "=<redacted>"
}

func containsString(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

// matchesLegacyPattern testa um pattern legado contra ate tres representacoes
// do comando atomico:
//
//  1. cmd.String()             — forma "shell-like" sem aspas (caso comum).
//  2. cmd.QuotedString()       — args com espaco/aspas re-quotados em "...".
//  3. cmd.SingleQuotedString() — variante com aspas simples '...'.
//
// O parser remove as aspas do Args durante o lexing, entao patterns legados
// que dependiam da forma quotada (ex.: `auto_approve: ["echo 'a b'"]` ou
// `auto_approve: ["echo \"a b\""]`) deixariam silenciosamente de casar caso
// so a forma sem aspas fosse testada. Tentar as tres formas cobre os perfis
// pre-AEP-0060 mais comuns; o unico caso nao coberto e arg que contem aspa
// simples literal — raro o suficiente para nao justificar quoting hibrido.
func matchesLegacyPattern(cmd Command, pattern string) bool {
	candidates := []string{cmd.String()}
	if quoted := cmd.QuotedString(); quoted != cmd.String() {
		candidates = append(candidates, quoted)
	}
	if singleQuoted := cmd.SingleQuotedString(); singleQuoted != cmd.String() && singleQuoted != cmd.QuotedString() {
		candidates = append(candidates, singleQuoted)
	}
	for _, candidate := range candidates {
		if allowlist.MatchPattern(candidate, pattern) {
			return true
		}
	}
	return false
}

func combineDecision(current, next allowlist.Decision) allowlist.Decision {
	if current == allowlist.DecisionDeny || next == allowlist.DecisionDeny {
		return allowlist.DecisionDeny
	}
	if current == allowlist.DecisionConfirm || next == allowlist.DecisionConfirm {
		return allowlist.DecisionConfirm
	}
	return allowlist.DecisionApprove
}

// matchStructuredRule procura a primeira regra estruturada que case com cmd
// e tenha a decision pedida. Devolve a regra encontrada, seu indice no slice
// rules (para correlacao em reasons safe — ex.: "rule[2]") e ok=true.
func matchStructuredRule(cmd Command, rules []allowlist.CommandRule, decision allowlist.Decision) (allowlist.CommandRule, int, bool) {
	for i, rule := range rules {
		if parseRuleDecision(rule.Decision) != decision {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(rule.Program), cmd.Program) {
			continue
		}
		consumed, ok := matchSequence(cmd.Args, rule.Subcommands)
		if !ok {
			continue
		}
		// Quando "*" e o ultimo Subcommand, ele consome todo o restante de
		// cmd.Args. Aplicar Args sobre algo que ja foi consumido pelo "*"
		// nao faz sentido; a validacao em allowlist.Validate() rejeita esse
		// combo, mas para perfis legados/editados manualmente, ignoramos
		// rule.Args nesse caso para evitar matching contra-intuitivo.
		if consumed >= len(cmd.Args) {
			if len(rule.Args) == 0 || (len(rule.Args) == 1 && rule.Args[0] == "*") {
				return rule, i, true
			}
			continue
		}
		remainingArgs := cmd.Args[consumed:]
		if _, ok := matchSequence(remainingArgs, rule.Args); !ok {
			continue
		}
		return rule, i, true
	}
	return allowlist.CommandRule{}, -1, false
}

// matchSequence faz casamento posicional de patterns contra values e devolve
// quantos elementos de values foram consumidos.
//
// Regras:
//   - patterns vazio       -> match sem consumir nada (consumed=0).
//   - "*" como ultimo      -> consome todo o restante de values (wildcard de cauda).
//   - "*" em outra posicao -> tratado como literal (a validacao em
//     allowlist.Validate() rejeita esse caso no save; aqui mantemos a
//     interpretacao literal como defesa em profundidade para perfis legados).
//   - Demais patterns      -> casamento case-insensitive posicao a posicao.
//
// O retorno consumido e usado por matchStructuredRule para calcular o sufixo
// que sera testado contra rule.Args, evitando o bug onde len(rule.Subcommands)
// era usado direto e descartava um arg real quando o ultimo pattern era "*".
func matchSequence(values, patterns []string) (consumed int, ok bool) {
	if len(patterns) == 0 {
		return 0, true
	}
	if len(patterns) == 1 && patterns[0] == "*" {
		return len(values), true
	}
	if len(values) < len(patterns) {
		return 0, false
	}
	for i, pattern := range patterns {
		if i == len(patterns)-1 && pattern == "*" {
			return len(values), true
		}
		if !strings.EqualFold(values[i], pattern) {
			return 0, false
		}
	}
	return len(patterns), true
}

func parseRuleDecision(value string) allowlist.Decision {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "deny":
		return allowlist.DecisionDeny
	case "approve":
		return allowlist.DecisionApprove
	default:
		return allowlist.DecisionConfirm
	}
}

func describeRule(rule allowlist.CommandRule) string {
	parts := []string{rule.Program}
	parts = append(parts, rule.Subcommands...)
	parts = append(parts, rule.Args...)
	if rule.Description != "" {
		return strings.Join(parts, " ") + " (" + rule.Description + ")"
	}
	return strings.Join(parts, " ")
}

