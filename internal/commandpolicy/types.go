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

// QuotedString devolve o comando atomico re-aplicando aspas duplas em args
// que originalmente exigiam aspas (espaco em branco, aspas, "$", crase,
// "\"). E uma das formas usadas para matching de patterns legados que
// dependiam da forma quotada do comando (ex.: pattern legado `echo "a b"`
// deve continuar casando com o input `echo "a b"`, mesmo que o parser tenha
// removido as aspas no Args). Veja tambem SingleQuotedString para a variante
// com aspas simples.
func (c Command) QuotedString() string {
	return c.quoted(shellQuoteWithDouble)
}

// SingleQuotedString devolve o comando atomico re-aplicando aspas simples
// em args com caracteres especiais. E usada como tentativa adicional de
// matching legacy: alguns perfis pre-AEP-0060 podem ter usado aspas simples
// em vez de duplas (ex.: pattern `echo 'a b'`). Como aspas simples em POSIX
// nao expandem nada, a unica opcao de "escape" para um arg que contenha
// aspa simples e fechar a aspa, escapar a aspa via aspas duplas e reabrir
// — para esse caso de borda, devolvemos o argumento sem aspas (matching
// pode falhar, mas evitamos uma string mal-formada).
func (c Command) SingleQuotedString() string {
	return c.quoted(shellQuoteWithSingle)
}

func (c Command) quoted(quoter func(string) string) string {
	if c.Program == "" {
		return ""
	}
	if len(c.Args) == 0 {
		return c.Program
	}
	parts := make([]string, 0, len(c.Args)+1)
	parts = append(parts, c.Program)
	for _, arg := range c.Args {
		parts = append(parts, quoter(arg))
	}
	return strings.Join(parts, " ")
}

// shellQuoteWithDouble envolve s em aspas duplas e escapa "\" e `"` quando
// o argumento contem caracteres que normalmente exigem quoting em uma shell.
// Retorna o proprio s quando nao ha necessidade de aspas (caso comum).
func shellQuoteWithDouble(s string) string {
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

// shellQuoteWithSingle envolve s em aspas simples quando precisa de quoting.
// Em POSIX, aspas simples sao literais e nao expandem nada — perfeito para
// matching legacy, mas nao conseguem conter o proprio caractere "'".
// Quando s contem aspas simples, devolvemos s sem aspas (a melhor coisa que
// podemos fazer sem produzir uma string mal-formada que confundiria o
// MatchPattern). Isso e raro o suficiente para nao justificar a complexidade
// extra de quoting hibrido.
func shellQuoteWithSingle(s string) string {
	if s == "" {
		return `''`
	}
	if !strings.ContainsAny(s, " \t\"'`$\\") {
		return s
	}
	if strings.ContainsRune(s, '\'') {
		return s
	}
	return `'` + s + `'`
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
