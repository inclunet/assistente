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

func jsonInt(raw json.RawMessage) (int, bool) {
	var value int
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, false
	}
	return value, true
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
	if err != nil {
		return "", ErrInvalid
	}
	version, ok := jsonInt(fields["version"])
	if !ok {
		return "", ErrInvalid
	}
	switch version {
	case 1:
		return decodeKeyboardV1(fields)
	case 2:
		return decodeKeyboardV2(fields)
	default:
		return "", ErrInvalid
	}
}

func decodeKeyboardGlobal(raw string) (string, error) {
	fields, err := strictObject(raw)
	if err != nil {
		return "", ErrInvalid
	}
	version, ok := jsonInt(fields["version"])
	if !ok || version != 1 {
		return "", ErrInvalid
	}
	identity, err := decodeKeyboardV1(fields)
	if err != nil {
		return "", err
	}
	return "keyboard.global:" + strings.TrimPrefix(identity, "keyboard.local:"), nil
}

func decodeKeyboardV1(fields map[string]json.RawMessage) (string, error) {
	return decodeKeyboardV1WithPrefix("keyboard.local", fields)
}

func decodeKeyboardV1WithPrefix(prefix string, fields map[string]json.RawMessage) (string, error) {
	if !exactFields(fields, "version", "code", "modifiers") || !versionOne(fields) {
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
	identity.WriteString(prefix)
	identity.WriteByte(':')
	for _, modifier := range []string{control, alt, shift, meta} {
		if present[modifier] {
			identity.WriteString(modifier)
			identity.WriteByte('+')
		}
	}
	identity.WriteString(code)
	return identity.String(), nil
}

type KeyboardLocalStep struct {
	Code      string   `json:"code"`
	Modifiers []string `json:"modifiers"`
}

type KeyboardLocalSequence struct {
	Version int                 `json:"version"`
	Steps   []KeyboardLocalStep `json:"steps"`
}

func decodeKeyboardV2(fields map[string]json.RawMessage) (string, error) {
	if !exactFields(fields, "version", "steps") {
		return "", ErrInvalid
	}
	steps, ok := jsonArray(fields["steps"])
	if !ok || len(steps) != 2 {
		return "", ErrInvalid
	}
	parsed := make([]KeyboardLocalStep, 2)
	for i, raw := range steps {
		stepFields, err := strictObject(string(raw))
		if err != nil || !exactFields(stepFields, "code", "modifiers") {
			return "", ErrInvalid
		}
		code, ok := jsonString(stepFields["code"])
		if !ok || !validKeyboardCode(code) || (i == 1 && code == "Escape") {
			return "", ErrInvalid
		}
		modifiers, ok := canonicalKeyboardModifiers(stepFields["modifiers"], i == 0, i == 1)
		if !ok {
			return "", ErrInvalid
		}
		parsed[i] = KeyboardLocalStep{Code: code, Modifiers: modifiers}
	}
	return keyboardSequenceIdentity(parsed[0], parsed[1]), nil
}

func canonicalKeyboardModifiers(raw json.RawMessage, requirePrimary, requireEmpty bool) ([]string, bool) {
	modifiers, ok := jsonArray(raw)
	if !ok {
		return nil, false
	}
	values := make([]string, len(modifiers))
	for i, rawModifier := range modifiers {
		value, ok := jsonString(rawModifier)
		if !ok {
			return nil, false
		}
		values[i] = value
	}
	return canonicalKeyboardModifierValues(values, requirePrimary, requireEmpty)
}

func canonicalKeyboardModifierValues(modifiers []string, requirePrimary, requireEmpty bool) ([]string, bool) {
	const (
		control = "Control"
		alt     = "Alt"
		shift   = "Shift"
		meta    = "Meta"
	)
	allowed := map[string]bool{control: true, alt: true, shift: true, meta: true}
	present := make(map[string]bool, len(modifiers))
	for _, modifier := range modifiers {
		if !allowed[modifier] || present[modifier] {
			return nil, false
		}
		present[modifier] = true
	}
	if requirePrimary && !present[control] && !present[alt] && !present[meta] {
		return nil, false
	}
	if requireEmpty && len(modifiers) != 0 {
		return nil, false
	}
	ordered := make([]string, 0, len(present))
	for _, modifier := range []string{control, alt, shift, meta} {
		if present[modifier] {
			ordered = append(ordered, modifier)
		}
	}
	return ordered, true
}

func keyboardSequenceIdentity(first, second KeyboardLocalStep) string {
	var identity strings.Builder
	identity.WriteString("keyboard.local:")
	for _, modifier := range first.Modifiers {
		identity.WriteString(modifier)
		identity.WriteByte('+')
	}
	identity.WriteString(first.Code)
	identity.WriteByte(' ')
	identity.WriteString(second.Code)
	return identity.String()
}

// EncodeKeyboardLocalIdentity devolve o documento JSON canônico de uma
// identidade keyboard.local v1 ou v2. É o inverso explícito de Normalize e
// serve para round-trip sem aceitar aliases ou ordens alternativas.
func EncodeKeyboardLocalIdentity(identity string) ([]byte, error) {
	const prefix = "keyboard.local:"
	if !strings.HasPrefix(identity, prefix) {
		return nil, ErrInvalid
	}
	rest := strings.TrimPrefix(identity, prefix)
	if strings.Contains(rest, " ") {
		parts := strings.Split(rest, " ")
		if len(parts) != 2 {
			return nil, ErrInvalid
		}
		firstParts := strings.Split(parts[0], "+")
		if len(firstParts) < 2 {
			return nil, ErrInvalid
		}
		modifiers := firstParts[:len(firstParts)-1]
		first := KeyboardLocalStep{Code: firstParts[len(firstParts)-1], Modifiers: modifiers}
		second := KeyboardLocalStep{Code: parts[1]}
		canonicalModifiers, ok := canonicalKeyboardModifierValues(modifiers, true, false)
		if !ok || !validKeyboardCode(first.Code) || !validKeyboardCode(second.Code) || second.Code == "Escape" || !slicesEqual(canonicalModifiers, modifiers) || keyboardSequenceIdentity(first, second) != identity {
			return nil, ErrInvalid
		}
		return json.Marshal(KeyboardLocalSequence{Version: 2, Steps: []KeyboardLocalStep{
			{Code: first.Code, Modifiers: first.Modifiers},
			{Code: second.Code, Modifiers: []string{}},
		}})
	}
	parts := strings.Split(rest, "+")
	if len(parts) == 0 || !validKeyboardCode(parts[len(parts)-1]) {
		return nil, ErrInvalid
	}
	modifiers := parts[:len(parts)-1]
	ordered, ok := canonicalKeyboardModifierValues(modifiers, false, false)
	if !ok || !slicesEqual(ordered, modifiers) {
		return nil, ErrInvalid
	}
	return json.Marshal(struct {
		Version   int      `json:"version"`
		Code      string   `json:"code"`
		Modifiers []string `json:"modifiers"`
	}{Version: 1, Code: parts[len(parts)-1], Modifiers: modifiers})
}

func slicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// MatchCondition usa a mesma gramática estrita da configuração persistida.
// Fatos ausentes não satisfazem cláusulas, inclusive comparações com false.
func MatchCondition(raw string, facts commandbindings.Facts) (bool, error) {
	condition, err := decodeCondition(raw)
	if err != nil {
		return false, err
	}
	return commandbindings.MatchCondition(condition, facts)
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
		case commandbindings.AppPage:
			stringValue, ok := jsonString(clause["value"])
			if !ok || !commandbindings.IsAppPage(stringValue) {
				return nil, ErrInvalid
			}
			value = stringValue
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
