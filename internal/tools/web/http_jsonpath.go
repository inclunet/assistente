package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type restrictedJSONPathError struct {
	code string
	msg  string
}

func (e *restrictedJSONPathError) Error() string { return e.msg }

type jsonPathToken struct {
	name      string
	recursive bool
}

// extractRestrictedJSONPath implementa somente o subconjunto seguro usado
// para localizar campos: $.metadata.name e $..metadata.name. Filtros, scripts,
// expressões e operadores não fazem parte do contrato.
func extractRestrictedJSONPath(content, expression string) (string, error) {
	tokens, err := parseRestrictedJSONPath(expression)
	if err != nil {
		return "", err
	}
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return "", &restrictedJSONPathError{code: "jsonpath_invalid_json", msg: fmt.Sprintf("resposta não é JSON válido para jsonpath: %v", err)}
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return "", &restrictedJSONPathError{code: "jsonpath_invalid_json", msg: "resposta contém mais de um documento JSON"}
		}
		return "", &restrictedJSONPathError{code: "jsonpath_invalid_json", msg: fmt.Sprintf("resposta não é JSON válido para jsonpath: %v", err)}
	}

	matches := make([]any, 0)
	collectJSONPathMatches([]any{document}, tokens, 0, &matches)
	result, err := json.MarshalIndent(matches, "", "  ")
	if err != nil {
		return "", &restrictedJSONPathError{code: "jsonpath_result_invalid", msg: fmt.Sprintf("não foi possível serializar o resultado de jsonpath: %v", err)}
	}
	return string(result), nil
}

func parseRestrictedJSONPath(expression string) ([]jsonPathToken, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" || expression[0] != '$' {
		return nil, &restrictedJSONPathError{code: "jsonpath_invalid", msg: "jsonpath inválido: a expressão deve começar com '$'"}
	}
	var tokens []jsonPathToken
	for i := 1; i < len(expression); {
		recursive := false
		switch {
		case strings.HasPrefix(expression[i:], ".."):
			recursive = true
			i += 2
		case expression[i] == '.':
			i++
		case expression[i] == '[':
			name, next, ok := parseBracketField(expression, i)
			if !ok {
				return nil, &restrictedJSONPathError{code: "jsonpath_invalid", msg: "jsonpath inválido: use apenas campos, por exemplo '$..metadata.name'"}
			}
			tokens = append(tokens, jsonPathToken{name: name, recursive: false})
			i = next
			continue
		default:
			return nil, &restrictedJSONPathError{code: "jsonpath_invalid", msg: "jsonpath inválido: use apenas campos, por exemplo '$..metadata.name'"}
		}

		start := i
		for i < len(expression) && isJSONPathFieldChar(expression[i]) {
			i++
		}
		if start == i {
			return nil, &restrictedJSONPathError{code: "jsonpath_invalid", msg: "jsonpath inválido: cada segmento deve ser um nome de campo"}
		}
		tokens = append(tokens, jsonPathToken{name: expression[start:i], recursive: recursive})
	}
	if len(tokens) == 0 {
		return nil, &restrictedJSONPathError{code: "jsonpath_invalid", msg: "jsonpath inválido: informe pelo menos um campo"}
	}
	return tokens, nil
}

func parseBracketField(expression string, start int) (string, int, bool) {
	if start >= len(expression) || expression[start] != '[' || start+3 >= len(expression) {
		return "", 0, false
	}
	quote := expression[start+1]
	if quote != '\'' && quote != '"' {
		return "", 0, false
	}
	end := strings.IndexByte(expression[start+2:], quote)
	if end < 0 {
		return "", 0, false
	}
	end += start + 2
	if end+1 >= len(expression) || expression[end+1] != ']' {
		return "", 0, false
	}
	name := expression[start+2 : end]
	if name == "" {
		return "", 0, false
	}
	for i := 0; i < len(name); i++ {
		if !isJSONPathFieldChar(name[i]) {
			return "", 0, false
		}
	}
	return name, end + 2, true
}

func isJSONPathFieldChar(ch byte) bool {
	return ch == '_' || ch == '-' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9'
}

func collectJSONPathMatches(nodes []any, tokens []jsonPathToken, index int, matches *[]any) {
	if index >= len(tokens) {
		*matches = append(*matches, nodes...)
		return
	}
	token := tokens[index]
	for _, node := range nodes {
		if token.recursive {
			collectRecursiveField(node, token.name, tokens, index+1, matches)
			continue
		}
		if object, ok := node.(map[string]any); ok {
			if value, found := object[token.name]; found {
				collectJSONPathMatches([]any{value}, tokens, index+1, matches)
			}
		}
	}
}

func collectRecursiveField(node any, name string, tokens []jsonPathToken, next int, matches *[]any) {
	if object, ok := node.(map[string]any); ok {
		if value, found := object[name]; found {
			collectJSONPathMatches([]any{value}, tokens, next, matches)
		}
		for _, value := range object {
			collectRecursiveField(value, name, tokens, next, matches)
		}
		return
	}
	if array, ok := node.([]any); ok {
		for _, value := range array {
			collectRecursiveField(value, name, tokens, next, matches)
		}
	}
}

func jsonPathErrorCode(err error) string {
	var pathErr *restrictedJSONPathError
	if errors.As(err, &pathErr) {
		return pathErr.code
	}
	return "jsonpath_error"
}
