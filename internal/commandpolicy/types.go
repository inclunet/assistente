package commandpolicy

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

// String normaliza o comando atomico para matching legado.
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

// ParseResult contem os comandos atomicos e avisos conservadores detectados.
type ParseResult struct {
	Commands []Command
	Features []Feature
	Errors   []string
}

func (r ParseResult) RequiresConfirmation() bool {
	return len(r.Features) > 0 || len(r.Errors) > 0
}
