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
		// Peela atribuicoes inline KEY=VALUE antes do programa real. Em POSIX
		// shells, "TOKEN=secret cmd" exporta TOKEN apenas para cmd. Sem isso,
		// o parser tomaria "TOKEN=secret" como Program e o valor secreto
		// vazaria no log, em Reasons e no Content devolvido ao LLM.
		envAssignments, programIdx := splitEnvAssignments(words)
		if programIdx >= len(words) {
			// Linha so com env assignments e sem comando real. Isso nao e um
			// atomo executavel — tratamos como sintaxe ambigua para forcar
			// confirmacao em vez de aceitar silenciosamente, e nao registramos
			// um Command (nao tem Program). A reason fica generica para nao
			// vazar o nome da variavel que pode ser tao sensivel quanto o valor.
			result.Errors = append(result.Errors, "atribuicao de env sem comando")
			words = nil
			commandFeatures = nil
			pendingOperator = OperatorSequence
			lastWasOperator = false
			return
		}
		if len(envAssignments) > 0 {
			commandFeatures = append(commandFeatures, FeatureEnvAssignment)
			result.Features = appendFeature(result.Features, FeatureEnvAssignment)
		}
		programWords := words[programIdx:]
		result.Commands = append(result.Commands, Command{
			Program:        programWords[0],
			Args:           append([]string(nil), programWords[1:]...),
			EnvAssignments: append([]string(nil), envAssignments...),
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
			// Em POSIX, aspas simples nao expandem nada, mas mantemos a agregacao por consistencia.
			for _, f := range next.features {
				features = appendFeature(features, f)
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
			// Aspas duplas em POSIX continuam expandindo $(...) e crases;
			// agregamos as features detectadas pelo readQuoted para forcar confirmacao.
			for _, f := range next.features {
				features = appendFeature(features, f)
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
			// "\" fora de aspas e LITERAL nesta subset, nao escape POSIX.
			// Motivos:
			//   - Windows usa "\" como separador de path (C:\Windows,
			//     del /s /q C:\). Tratar como escape POSIX engole a barra
			//     e quebra paths reais e patterns da allowlist (o
			//     AlwaysDeny default "del /s /q C:\\" deixa de bater).
			//   - "\" no fim de linha em POSIX e continuacao de linha,
			//     mas no contexto deste parser (uma unica linha de
			//     comando vinda de a.Command) e mais provavel ser path
			//     Windows terminando em "\" do que continuacao de linha.
			//   - Quem precisar de escape POSIX (\$, \&, \") pode usar
			//     aspas duplas — readQuoted preserva o escape \ dentro
			//     de aspas duplas para os casos legitimos.
			current.WriteByte(ch)

		default:
			current.WriteByte(ch)
		}
	}

	flushWord()
	return tokens, uniqueFeatures(features), errors
}

type quotedValue struct {
	value    string
	end      int
	features []Feature
}

func readQuoted(input string, start int, quote byte) (quotedValue, bool) {
	var out strings.Builder
	var features []Feature
	for i := start; i < len(input); i++ {
		ch := input[i]
		if quote == '"' && ch == '\\' && i+1 < len(input) {
			i++
			out.WriteByte(input[i])
			continue
		}
		if ch == quote {
			return quotedValue{value: out.String(), end: i, features: features}, true
		}
		// Em aspas duplas, $(...) e crases continuam ativando substituicao em
		// shells POSIX. Marcamos as features para que o caller force confirmacao.
		if quote == '"' {
			if ch == '$' && i+1 < len(input) && input[i+1] == '(' {
				features = appendFeature(features, FeatureCommandSubstitution)
			} else if ch == '`' {
				features = appendFeature(features, FeatureBacktickSubstitution)
			}
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

// splitEnvAssignments pega os tokens iniciais no formato KEY=VALUE (atribuicoes
// inline de env do shell) e devolve a lista deles + o indice do primeiro
// token que e um programa real. Quando nenhum prefixo casa, devolve nil + 0.
//
// Regra POSIX: o nome da variavel deve comecar com letra/underscore e conter
// apenas letras, digitos ou underscore. Tokens como "=foo" ou "1A=x" nao
// sao atribuicoes — sao argumentos comuns (ou nomes de programa esquisitos)
// e devem ser tratados como Program para nao falhar matching.
func splitEnvAssignments(words []string) (assignments []string, programIdx int) {
	for i, w := range words {
		if !looksLikeEnvAssignment(w) {
			return assignments, i
		}
		assignments = append(assignments, w)
	}
	return assignments, len(words)
}

func looksLikeEnvAssignment(token string) bool {
	eq := strings.IndexByte(token, '=')
	if eq <= 0 {
		return false
	}
	name := token[:eq]
	for i := 0; i < len(name); i++ {
		ch := name[i]
		if ch == '_' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
			continue
		}
		if i == 0 {
			return false
		}
		if ch >= '0' && ch <= '9' {
			continue
		}
		return false
	}
	return true
}
