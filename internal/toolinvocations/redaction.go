package toolinvocations

import (
	"encoding/json"
	"strconv"
	"strings"

	"assistente/internal/tools"
)

const redactedValue = "[redacted]"

// redactArgumentsJSONWithPaths combina a redação genérica histórica com os
// JSON Pointers declarados pelo contrato do comando. Qualquer erro de parse ou
// de pointer retorna um documento seguro, nunca o argumento original.
func redactArgumentsJSONWithPaths(raw string, paths []string) string {
	redacted, ok := redactJSONWithPaths(raw, paths, true)
	if !ok {
		return `{"_redacted":true}`
	}
	return redacted
}

// redactJSONWithPaths redige somente a cópia em memória usada para auditoria.
// generic aplica as heurísticas de segredo já usadas pelas invocações de chat;
// paths acrescenta a política tipada do catálogo do comando.
func redactJSONWithPaths(raw string, paths []string, generic bool) (string, bool) {
	if len(raw) == 0 || !json.Valid([]byte(raw)) {
		return `{"_redacted":true}`, false
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return `{"_redacted":true}`, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return `{"_redacted":true}`, false
	}
	if generic {
		value = redactAny("", value)
	}
	for _, pointer := range paths {
		segments, ok := parseJSONPointer(pointer)
		if !ok {
			return `{"_redacted":true}`, false
		}
		value = redactJSONPointerValue(value, segments)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return `{"_redacted":true}`, false
	}
	return string(encoded), true
}

func parseJSONPointer(pointer string) ([]string, bool) {
	if pointer == "" {
		return nil, true
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, false
	}
	parts := strings.Split(pointer[1:], "/")
	for index, part := range parts {
		var decoded strings.Builder
		for offset := 0; offset < len(part); offset++ {
			if part[offset] != '~' {
				decoded.WriteByte(part[offset])
				continue
			}
			if offset+1 >= len(part) {
				return nil, false
			}
			switch part[offset+1] {
			case '0':
				decoded.WriteByte('~')
			case '1':
				decoded.WriteByte('/')
			default:
				return nil, false
			}
			offset++
		}
		parts[index] = decoded.String()
	}
	return parts, true
}

func redactJSONPointerValue(value any, segments []string) any {
	if len(segments) == 0 {
		return redactedValue
	}
	switch current := value.(type) {
	case map[string]any:
		child, exists := current[segments[0]]
		if !exists {
			return value
		}
		current[segments[0]] = redactJSONPointerValue(child, segments[1:])
	case []any:
		index, err := strconv.Atoi(segments[0])
		if err != nil || index < 0 || index >= len(current) || (len(segments[0]) > 1 && segments[0][0] == '0') {
			return value
		}
		current[index] = redactJSONPointerValue(current[index], segments[1:])
	}
	return value
}

func redactToolResultForPersistence(result tools.ToolResult, paths []string) tools.ToolResult {
	if len(paths) == 0 {
		return result
	}
	redacted, ok := redactJSONWithPaths(result.Content, paths, false)
	if !ok {
		// Um contrato que declara paths de saída não pode persistir conteúdo cujo
		// formato não permita provar que os paths foram redigidos.
		result.Content = `"[redacted]"`
	} else {
		result.Content = redacted
	}
	// Metadata, annotations e failure são payloads persistidos fora de
	// result.Content. Podem carregar URL, resource_id, janela/truncation,
	// headers ou mensagens derivadas do mesmo segredo; paths tipados exigem
	// uma política fail-closed para todos eles.
	result.Metadata = nil
	result.Annotations = nil
	result.Failure = nil
	return result
}

// RedactJSONAtPaths expõe a mesma operação para bridges que precisam preparar
// snapshots de auditoria antes de delegar. O retorno nunca contém o documento
// original quando o JSON ou um pointer é inválido.
func RedactJSONAtPaths(raw []byte, paths []string) ([]byte, error) {
	redacted, ok := redactJSONWithPaths(string(raw), paths, false)
	if !ok {
		return []byte(`{"_redacted":true}`), nil
	}
	return []byte(redacted), nil
}
