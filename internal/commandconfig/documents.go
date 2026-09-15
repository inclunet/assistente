package commandconfig

import (
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"assistente/internal/commandbindings"
	"assistente/internal/commandjson"
)

const maxDocumentBytes = 64 * 1024

// strictObject lê exatamente um objeto JSON e rejeita chaves repetidas. O
// Decoder normaliza escapes de chaves antes de as colocarmos no mapa, então
// "code" e "\u0063ode" também são tratados como a mesma chave.
func strictObject(raw string) (map[string]json.RawMessage, error) {
	if len(raw) > maxDocumentBytes || !utf8.ValidString(raw) || !validUnicodeEscapes(raw) {
		return nil, ErrInvalid
	}
	// Compartilha o contrato lexical com ingresso e fingerprints: inclusive
	// duplicatas aninhadas, limites de profundidade e números não representáveis.
	_, err := commandjson.Canonicalize([]byte(raw))
	if err != nil {
		return nil, ErrInvalid
	}

	decoder := json.NewDecoder(strings.NewReader(raw))
	first, err := decoder.Token()
	if err != nil {
		return nil, ErrInvalid
	}
	delimiter, ok := first.(json.Delim)
	if !ok || delimiter != '{' {
		return nil, ErrInvalid
	}

	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, ErrInvalid
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, ErrInvalid
		}
		if _, exists := fields[key]; exists {
			return nil, ErrInvalid
		}

		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, ErrInvalid
		}
		fields[key] = value
	}
	last, err := decoder.Token()
	if err != nil {
		return nil, ErrInvalid
	}
	lastDelimiter, ok := last.(json.Delim)
	if !ok || lastDelimiter != '}' {
		return nil, ErrInvalid
	}

	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, ErrInvalid
	}
	return fields, nil
}

func hexDigit(value byte) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', true
	case value >= 'a' && value <= 'f':
		return value - 'a' + 10, true
	case value >= 'A' && value <= 'F':
		return value - 'A' + 10, true
	default:
		return 0, false
	}
}

func unicodeEscapeValue(raw string, start int) (uint16, bool) {
	if start+6 > len(raw) || raw[start] != '\\' || raw[start+1] != 'u' {
		return 0, false
	}
	var value uint16
	for i := start + 2; i < start+6; i++ {
		digit, ok := hexDigit(raw[i])
		if !ok {
			return 0, false
		}
		value = value<<4 | uint16(digit)
	}
	return value, true
}

// validUnicodeEscapes recusa surrogates UTF-16 isolados. encoding/json os
// substitui por U+FFFD, tornando documentos distintos iguais após o decode.
// Pares válidos de surrogates escapados são aceitos.
func validUnicodeEscapes(raw string) bool {
	inString := false
	for i := 0; i < len(raw); i++ {
		if !inString {
			if raw[i] == '"' {
				inString = true
			}
			continue
		}
		switch raw[i] {
		case '"':
			inString = false
		case '\\':
			if i+1 >= len(raw) {
				return false
			}
			if raw[i+1] != 'u' {
				i++
				continue
			}
			value, ok := unicodeEscapeValue(raw, i)
			if !ok {
				return false
			}
			i += 5
			switch {
			case value >= 0xDC00 && value <= 0xDFFF:
				return false
			case value >= 0xD800 && value <= 0xDBFF:
				low, ok := unicodeEscapeValue(raw, i+1)
				if !ok || low < 0xDC00 || low > 0xDFFF {
					return false
				}
				i += 6
			}
		}
	}
	return !inString
}

func exactFields(fields map[string]json.RawMessage, names ...string) bool {
	if len(fields) != len(names) {
		return false
	}
	for _, name := range names {
		if _, ok := fields[name]; !ok {
			return false
		}
	}
	return true
}

func versionOne(fields map[string]json.RawMessage) bool {
	raw, ok := fields["version"]
	if !ok {
		return false
	}
	var version int
	if err := json.Unmarshal(raw, &version); err != nil {
		return false
	}
	return version == 1
}

func jsonString(raw json.RawMessage) (string, bool) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	valueString, ok := value.(string)
	return valueString, ok
}

func jsonBool(raw json.RawMessage) (bool, bool) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, false
	}
	valueBool, ok := value.(bool)
	return valueBool, ok
}

func jsonArray(raw json.RawMessage) ([]json.RawMessage, bool) {
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil || values == nil {
		return nil, false
	}
	return values, true
}

func validKeyboardCode(code string) bool {
	switch {
	case len(code) == len("KeyA") && strings.HasPrefix(code, "Key") && code[3] >= 'A' && code[3] <= 'Z':
		return true
	case len(code) == len("Digit0") && strings.HasPrefix(code, "Digit") && code[5] >= '0' && code[5] <= '9':
		return true
	case len(code) >= 2 && len(code) <= 3 && code[0] == 'F':
		for i := 1; i < len(code); i++ {
			if code[i] < '0' || code[i] > '9' {
				return false
			}
		}
		number, err := strconv.Atoi(code[1:])
		if err != nil {
			return false
		}
		return number >= 1 && number <= 24 && code[1] != '0'
	default:
		switch code {
		case "Enter", "Escape", "Tab", "Space", "ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight", "Home", "End", "PageUp", "PageDown", "Delete", "Backspace", "Insert":
			return true
		default:
			return false
		}
	}
}

func decodeKeyboard(typeName, raw string) (string, error) {
	if typeName != "keyboard.local" {
		return "", ErrInvalid
	}
	fields, err := strictObject(raw)
	if err != nil || !exactFields(fields, "version", "code", "modifiers") || !versionOne(fields) {
		return "", ErrInvalid
	}

	code, ok := jsonString(fields["code"])
	if !ok || !validKeyboardCode(code) {
		return "", ErrInvalid
	}
	modifiers, ok := jsonArray(fields["modifiers"])
	if !ok {
		return "", ErrInvalid
	}

	const (
		control = "Control"
		alt     = "Alt"
		shift   = "Shift"
		meta    = "Meta"
	)
	allowed := map[string]bool{control: true, alt: true, shift: true, meta: true}
	present := make(map[string]bool, len(modifiers))
	for _, modifierRaw := range modifiers {
		modifier, ok := jsonString(modifierRaw)
		if !ok || !allowed[modifier] || present[modifier] {
			return "", ErrInvalid
		}
		present[modifier] = true
	}

	var identity strings.Builder
	identity.WriteString("keyboard.local:")
	for _, modifier := range []string{control, alt, shift, meta} {
		if present[modifier] {
			identity.WriteString(modifier)
			identity.WriteByte('+')
		}
	}
	identity.WriteString(code)
	return identity.String(), nil
}

func decodeCondition(raw string) (commandbindings.Facts, error) {
	fields, err := strictObject(raw)
	if err != nil || !exactFields(fields, "version", "clauses") || !versionOne(fields) {
		return nil, ErrInvalid
	}
	clauses, ok := jsonArray(fields["clauses"])
	if !ok || len(clauses) > 32 {
		return nil, ErrInvalid
	}

	facts := make(commandbindings.Facts, len(clauses))
	for _, clauseRaw := range clauses {
		clause, err := strictObject(string(clauseRaw))
		if err != nil || !exactFields(clause, "field", "op", "value") {
			return nil, ErrInvalid
		}
		fieldName, ok := jsonString(clause["field"])
		if !ok {
			return nil, ErrInvalid
		}
		op, ok := jsonString(clause["op"])
		if !ok || op != "eq" {
			return nil, ErrInvalid
		}

		field := commandbindings.Field(fieldName)
		if _, exists := facts[field]; exists {
			return nil, ErrInvalid
		}
		var value any
		switch field {
		case commandbindings.AppFocused:
			boolean, ok := jsonBool(clause["value"])
			if !ok {
				return nil, ErrInvalid
			}
			value = boolean
		case commandbindings.SurfaceType, commandbindings.SurfaceID, commandbindings.Profile, commandbindings.Device, commandbindings.Process:
			stringValue, ok := jsonString(clause["value"])
			if !ok || strings.TrimSpace(stringValue) == "" {
				return nil, ErrInvalid
			}
			value = stringValue
		default:
			return nil, ErrInvalid
		}
		facts[field] = value
	}
	return facts, nil
}

func emptyDocument(raw string) bool {
	fields, err := strictObject(raw)
	return err == nil && len(fields) == 0
}
