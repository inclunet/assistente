package commandpolicy

import "strings"

// Operator descreve como um comando atomico foi encadeado ao comando anterior.
type Operator string

const (
	OperatorNone       Operator = ""
	OperatorSequence   Operator = ";"
	OperatorAnd        Operator = "&&"
	OperatorOr         Operator = "||"
	OperatorPipe       Operator = "|"
	OperatorBackground Operator = "&"
	OperatorNewline    Operator = "\n"
)

// Feature marca recursos de shell que afetam a decisao de seguranca.
type Feature string

const (
	FeaturePipe                 Feature = "pipe"
	FeatureBackground           Feature = "background"
	FeatureRedirectOutput       Feature = "redirect_output"
	FeatureRedirectAppend       Feature = "redirect_append"
	FeatureRedirectError        Feature = "redirect_error"
	FeatureRedirectErrorAppend  Feature = "redirect_error_append"
	FeatureRedirectInput        Feature = "redirect_input"
	FeatureHeredoc              Feature = "heredoc"
	FeatureCommandSubstitution  Feature = "command_substitution"
	FeatureBacktickSubstitution Feature = "backtick_substitution"
	FeatureAmbiguousSyntax      Feature = "ambiguous_syntax"
)

// Command e a unidade atomica avaliada pela politica.
type Command struct {
	Program        string
	Args           []string
	OperatorBefore Operator
	Features       []Feature
}

// String devolve o comando atomico em formato "shell-like" sem reaplicar
// aspas: o programa e os args sao concatenados por espaco. E a forma usada
// para matching de patterns legados que nao continham aspas (a vasta maioria
// dos casos: "git status", "git diff *", "npm *", etc).
func (c Command) String() string {
	if c.Program == "" {
		return ""
	}
	if len(c.Args) == 0 {
		return c.Program
	}
	out := c.Program
	for _, arg := range c.Args {
		out += " " + arg
	}
	return out
}

// QuotedString devolve o comando atomico re-aplicando aspas em args que
// originalmente exigiam aspas (espaco em branco, aspas, "$", crase, "\").
// E a forma usada para matching de patterns legados que dependiam da forma
// quotada do comando (ex.: pattern legado `echo "a b"` deve continuar
// casando com o input `echo "a b"`, mesmo que o parser tenha removido as
// aspas no Args). evaluateAtom tenta os dois formatos para preservar
// compatibilidade — qualquer pattern que casava antes do AEP-0060 continua
// casando, e novos patterns podem usar a forma sem aspas.
func (c Command) QuotedString() string {
	if c.Program == "" {
		return ""
	}
	if len(c.Args) == 0 {
		return c.Program
	}
	parts := make([]string, 0, len(c.Args)+1)
	parts = append(parts, c.Program)
	for _, arg := range c.Args {
		parts = append(parts, shellQuoteIfNeeded(arg))
	}
	return strings.Join(parts, " ")
}

// shellQuoteIfNeeded envolve s em aspas duplas e escapa "\" e `"` quando o
// argumento contem caracteres que normalmente exigem quoting em uma shell.
// Retorna o proprio s quando nao ha necessidade de aspas (caso comum).
func shellQuoteIfNeeded(s string) string {
	if s == "" {
		return `""`
	}
	if !strings.ContainsAny(s, " \t\"'`$\\") {
		return s
	}
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

// ParseResult contem os comandos atomicos e avisos conservadores detectados.
type ParseResult struct {
	Commands []Command
	Features []Feature
	Errors   []string
}

func (r ParseResult) RequiresConfirmation() bool {
	return len(r.Features) > 0 || len(r.Errors) > 0
}
