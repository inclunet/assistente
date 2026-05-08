package commandpolicy

import (
	"fmt"
	"strings"
)

type tokenKind int

const (
	tokenWord tokenKind = iota
	tokenOperator
	tokenRedirect
)

type token struct {
	kind     tokenKind
	value    string
	operator Operator
	feature  Feature
}

// Parse divide uma linha de comando em comandos atomicos usando uma subset
// conservadora comum entre shells. Sintaxe fora da subset nunca deve aprovar.
func Parse(input string) ParseResult {
	tokens, features, errors := lex(input)
	result := ParseResult{
		Features: uniqueFeatures(features),
		Errors:   errors,
	}

	var words []string
	var commandFeatures []Feature
	pendingOperator := OperatorNone
	skipRedirectTarget := false
	lastWasOperator := false

	flush := func() {
		if len(words) == 0 && len(commandFeatures) == 0 {
			return
		}
		if len(words) == 0 {
			result.Errors = append(result.Errors, "redirecionamento sem comando")
			commandFeatures = nil
			return
		}
		result.Commands = append(result.Commands, Command{
			Program:        words[0],
			Args:           append([]string(nil), words[1:]...),
			OperatorBefore: pendingOperator,
			Features:       uniqueFeatures(commandFeatures),
		})
		words = nil
		commandFeatures = nil
		pendingOperator = OperatorSequence
		lastWasOperator = false
	}

	for _, tok := range tokens {
		switch tok.kind {
		case tokenWord:
			if skipRedirectTarget {
				skipRedirectTarget = false
				continue
			}
			words = append(words, tok.value)
			lastWasOperator = false

		case tokenRedirect:
			commandFeatures = append(commandFeatures, tok.feature)
			result.Features = appendFeature(result.Features, tok.feature)
			skipRedirectTarget = true
			lastWasOperator = false
			if tok.feature == FeatureHeredoc {
				result.Errors = append(result.Errors, "heredoc detectado; corpo nao e parseado")
				flush()
				return result
			}

		case tokenOperator:
			if tok.operator == OperatorPipe {
				result.Features = appendFeature(result.Features, FeaturePipe)
			}
			if tok.operator == OperatorBackground {
				result.Features = appendFeature(result.Features, FeatureBackground)
			}
			if len(words) == 0 && len(commandFeatures) == 0 {
				result.Errors = append(result.Errors, fmt.Sprintf("operador %q sem comando anterior", tok.value))
			}
			flush()
			pendingOperator = tok.operator
			lastWasOperator = true
		}
	}

	if skipRedirectTarget {
		result.Errors = append(result.Errors, "redirecionamento sem alvo")
	}
	if lastWasOperator {
		result.Errors = append(result.Errors, "linha termina com operador")
	}
	flush()

	if len(result.Commands) == 0 && strings.TrimSpace(input) != "" {
		result.Errors = append(result.Errors, "nenhum comando atomico reconhecido")
	}

	return result
}

func lex(input string) ([]token, []Feature, []string) {
	var tokens []token
	var features []Feature
	var errors []string
	var current strings.Builder

	flushWord := func() {
		if current.Len() == 0 {
			return
		}
		tokens = append(tokens, token{kind: tokenWord, value: current.String()})
		current.Reset()
	}

	for i := 0; i < len(input); i++ {
		ch := input[i]

		switch ch {
		case ' ', '\t', '\r':
			flushWord()

		case '\n':
			flushWord()
			tokens = append(tokens, token{kind: tokenOperator, value: "\n", operator: OperatorNewline})

		case ';':
			flushWord()
			tokens = append(tokens, token{kind: tokenOperator, value: ";", operator: OperatorSequence})

		case '&':
			flushWord()
			if i+1 < len(input) && input[i+1] == '&' {
				tokens = append(tokens, token{kind: tokenOperator, value: "&&", operator: OperatorAnd})
				i++
				continue
			}
			features = appendFeature(features, FeatureBackground)
			tokens = append(tokens, token{kind: tokenOperator, value: "&", operator: OperatorBackground})

		case '|':
			flushWord()
			if i+1 < len(input) && input[i+1] == '|' {
				tokens = append(tokens, token{kind: tokenOperator, value: "||", operator: OperatorOr})
				i++
				continue
			}
			features = appendFeature(features, FeaturePipe)
			tokens = append(tokens, token{kind: tokenOperator, value: "|", operator: OperatorPipe})

		case '>':
			flushWord()
			if i+1 < len(input) && input[i+1] == '>' {
				tokens = append(tokens, token{kind: tokenRedirect, value: ">>", feature: FeatureRedirectAppend})
				i++
				continue
			}
			tokens = append(tokens, token{kind: tokenRedirect, value: ">", feature: FeatureRedirectOutput})

		case '<':
			flushWord()
			if i+1 < len(input) && input[i+1] == '<' {
				tokens = append(tokens, token{kind: tokenRedirect, value: "<<", feature: FeatureHeredoc})
				i++
				continue
			}
			tokens = append(tokens, token{kind: tokenRedirect, value: "<", feature: FeatureRedirectInput})

		case '2':
			if current.Len() == 0 && i+1 < len(input) && input[i+1] == '>' {
				if i+2 < len(input) && input[i+2] == '>' {
					tokens = append(tokens, token{kind: tokenRedirect, value: "2>>", feature: FeatureRedirectErrorAppend})
					i += 2
					continue
				}
				tokens = append(tokens, token{kind: tokenRedirect, value: "2>", feature: FeatureRedirectError})
				i++
				continue
			}
			current.WriteByte(ch)

		case '\'':
			next, ok := readQuoted(input, i+1, '\'')
			if !ok {
				errors = append(errors, "aspas simples sem fechamento")
				features = appendFeature(features, FeatureAmbiguousSyntax)
				current.WriteString(input[i+1:])
				i = len(input)
				continue
			}
			current.WriteString(next.value)
			i = next.end

		case '"':
			next, ok := readQuoted(input, i+1, '"')
			if !ok {
				errors = append(errors, "aspas duplas sem fechamento")
				features = appendFeature(features, FeatureAmbiguousSyntax)
				current.WriteString(input[i+1:])
				i = len(input)
				continue
			}
			current.WriteString(next.value)
			i = next.end

		case '`':
			features = appendFeature(features, FeatureBacktickSubstitution)
			next, ok := readQuoted(input, i+1, '`')
			if !ok {
				errors = append(errors, "substituicao por crase sem fechamento")
				features = appendFeature(features, FeatureAmbiguousSyntax)
				i = len(input)
				continue
			}
			current.WriteString(next.value)
			i = next.end

		case '$':
			if i+1 < len(input) && input[i+1] == '(' {
				features = appendFeature(features, FeatureCommandSubstitution)
				current.WriteString("$(")
				i++
				continue
			}
			current.WriteByte(ch)

		case '\\':
			if i+1 < len(input) {
				i++
				current.WriteByte(input[i])
				continue
			}
			current.WriteByte(ch)

		default:
			current.WriteByte(ch)
		}
	}

	flushWord()
	return tokens, uniqueFeatures(features), errors
}

type quotedValue struct {
	value string
	end   int
}

func readQuoted(input string, start int, quote byte) (quotedValue, bool) {
	var out strings.Builder
	for i := start; i < len(input); i++ {
		ch := input[i]
		if quote == '"' && ch == '\\' && i+1 < len(input) {
			i++
			out.WriteByte(input[i])
			continue
		}
		if ch == quote {
			return quotedValue{value: out.String(), end: i}, true
		}
		out.WriteByte(ch)
	}
	return quotedValue{}, false
}

func appendFeature(features []Feature, feature Feature) []Feature {
	for _, existing := range features {
		if existing == feature {
			return features
		}
	}
	return append(features, feature)
}

func uniqueFeatures(features []Feature) []Feature {
	var out []Feature
	for _, feature := range features {
		out = appendFeature(out, feature)
	}
	return out
}
