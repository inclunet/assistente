// Package commandjson fornece a serialização JCS usada pelas identidades de
// comandos. A implementação mantém o parser local para não herdar as
// permissões permissivas de encoding/json (chaves duplicadas e surrogates).
package commandjson

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	maxDocumentSize   = 64 * 1024
	maxDefinitionSize = 1024 * 1024
	maxDepth          = 64
)

var (
	ErrInvalidJSON      = errors.New("commandjson: JSON inválido")
	ErrDocumentTooLarge = errors.New("commandjson: documento excede o limite de tamanho")
	ErrDepthExceeded    = errors.New("commandjson: profundidade excede 64")
	ErrDuplicateKey     = errors.New("commandjson: chave de objeto duplicada")
	ErrInvalidUnicode   = errors.New("commandjson: Unicode inválido")
	ErrNumberRange      = errors.New("commandjson: número fora do intervalo IEEE 754")
	ErrInvalidValue     = errors.New("commandjson: valor não serializável")
	ErrInvalidHMAC      = errors.New("commandjson: parâmetros HMAC inválidos")
)

// Canonicalize parses raw JSON and returns its RFC 8785 canonical form.
// It rejects duplicate names, invalid Unicode, non-I-JSON numbers, trailing
// data, documents larger than 64 KiB, and nesting deeper than 64 containers.
func Canonicalize(raw []byte) ([]byte, error) {
	return canonicalize(raw, maxDocumentSize)
}

func canonicalize(raw []byte, maxSize int) ([]byte, error) {
	if len(raw) > maxSize {
		return nil, ErrDocumentTooLarge
	}
	if len(raw) == 0 {
		return nil, ErrInvalidJSON
	}

	p := parser{raw: raw}
	value, err := p.parseValue(0)
	if err != nil {
		return nil, err
	}
	p.skipSpace()
	if p.pos != len(p.raw) {
		return nil, ErrInvalidJSON
	}
	if len(value) > maxSize {
		return nil, ErrDocumentTooLarge
	}
	return value, nil
}

// Marshal serializes value without HTML escaping and canonicalizes the
// resulting JSON document. It follows encoding/json's supported Go values,
// while applying the JCS validation and limits of Canonicalize.
func Marshal(value any) ([]byte, error) {
	return marshal(value, maxDocumentSize)
}

// MarshalDefinition serializa uma definição persistida para cálculo de digest,
// com limite próprio de 1 MiB. Não usar para envelopes, argumentos ou HMAC do
// protocolo de comandos: esses continuam limitados a 64 KiB por Marshal.
// As demais validações JCS e a profundidade máxima são idênticas.
func MarshalDefinition(value any) ([]byte, error) {
	return marshal(value, maxDefinitionSize)
}

func marshal(value any, maxSize int) ([]byte, error) {
	if err := validateValue(reflect.ValueOf(value), 0, make(map[visit]struct{})); err != nil {
		return nil, err
	}

	out := boundedBuffer{maxSize: maxSize}
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		if errors.Is(err, ErrDocumentTooLarge) {
			return nil, err
		}
		return nil, ErrInvalidValue
	}
	data := out.Bytes()
	if len(data) == 0 || data[len(data)-1] != '\n' {
		return nil, ErrInvalidValue
	}
	return canonicalize(data[:len(data)-1], maxSize)
}

// HMAC canonicalizes raw and computes HMAC-SHA256 over a framed, domain-
// separated input. The returned digest is lowercase hexadecimal.
func HMAC(key []byte, domain string, raw []byte) (string, error) {
	if len(key) < 32 || domain == "" {
		return "", ErrInvalidHMAC
	}
	canonical, err := Canonicalize(raw)
	if err != nil {
		return "", err
	}

	// Fixed protocol prefix plus length framing makes both domain and payload
	// boundaries unambiguous, including for domains containing NUL bytes.
	var framed bytes.Buffer
	framed.WriteString("assistente:commandjson:hmac:v1\x00")
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(domain)))
	framed.Write(length[:])
	framed.WriteString(domain)
	binary.BigEndian.PutUint64(length[:], uint64(len(canonical)))
	framed.Write(length[:])
	framed.Write(canonical)

	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(framed.Bytes())
	return hex.EncodeToString(mac.Sum(nil)), nil
}

type parser struct {
	raw []byte
	pos int
}

type member struct {
	name  string
	value []byte
}

func (p *parser) parseValue(depth int) ([]byte, error) {
	p.skipSpace()
	if p.pos >= len(p.raw) {
		return nil, ErrInvalidJSON
	}

	switch p.raw[p.pos] {
	case 'n':
		if p.takeLiteral("null") {
			return []byte("null"), nil
		}
	case 't':
		if p.takeLiteral("true") {
			return []byte("true"), nil
		}
	case 'f':
		if p.takeLiteral("false") {
			return []byte("false"), nil
		}
	case '"':
		value, err := p.parseString()
		if err != nil {
			return nil, err
		}
		return appendJSONString(nil, value), nil
	case '[':
		return p.parseArray(depth)
	case '{':
		return p.parseObject(depth)
	default:
		if p.raw[p.pos] == '-' || (p.raw[p.pos] >= '0' && p.raw[p.pos] <= '9') {
			return p.parseNumber()
		}
	}
	return nil, ErrInvalidJSON
}

func (p *parser) parseArray(depth int) ([]byte, error) {
	if depth+1 > maxDepth {
		return nil, ErrDepthExceeded
	}
	p.pos++
	p.skipSpace()
	if p.pos < len(p.raw) && p.raw[p.pos] == ']' {
		p.pos++
		return []byte("[]"), nil
	}

	values := make([][]byte, 0, 4)
	for {
		value, err := p.parseValue(depth + 1)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
		p.skipSpace()
		if p.pos >= len(p.raw) {
			return nil, ErrInvalidJSON
		}
		switch p.raw[p.pos] {
		case ',':
			p.pos++
			continue
		case ']':
			p.pos++
			return joinArray(values), nil
		default:
			return nil, ErrInvalidJSON
		}
	}
}

func (p *parser) parseObject(depth int) ([]byte, error) {
	if depth+1 > maxDepth {
		return nil, ErrDepthExceeded
	}
	p.pos++
	p.skipSpace()
	if p.pos < len(p.raw) && p.raw[p.pos] == '}' {
		p.pos++
		return []byte("{}"), nil
	}

	members := make([]member, 0, 4)
	names := make(map[string]struct{})
	for {
		if p.pos >= len(p.raw) || p.raw[p.pos] != '"' {
			return nil, ErrInvalidJSON
		}
		name, err := p.parseString()
		if err != nil {
			return nil, err
		}
		if _, exists := names[name]; exists {
			return nil, ErrDuplicateKey
		}
		names[name] = struct{}{}
		p.skipSpace()
		if p.pos >= len(p.raw) || p.raw[p.pos] != ':' {
			return nil, ErrInvalidJSON
		}
		p.pos++
		value, err := p.parseValue(depth + 1)
		if err != nil {
			return nil, err
		}
		members = append(members, member{name: name, value: value})
		p.skipSpace()
		if p.pos >= len(p.raw) {
			return nil, ErrInvalidJSON
		}
		switch p.raw[p.pos] {
		case ',':
			p.pos++
			p.skipSpace()
			continue
		case '}':
			p.pos++
			sort.Slice(members, func(i, j int) bool {
				return utf16Less(members[i].name, members[j].name)
			})
			return joinObject(members), nil
		default:
			return nil, ErrInvalidJSON
		}
	}
}

func (p *parser) parseString() (string, error) {
	if p.pos >= len(p.raw) || p.raw[p.pos] != '"' {
		return "", ErrInvalidJSON
	}
	p.pos++
	var out []byte
	for p.pos < len(p.raw) {
		c := p.raw[p.pos]
		switch {
		case c == '"':
			p.pos++
			return string(out), nil
		case c == '\\':
			p.pos++
			if p.pos >= len(p.raw) {
				return "", ErrInvalidJSON
			}
			switch p.raw[p.pos] {
			case '"', '\\', '/':
				out = append(out, p.raw[p.pos])
				p.pos++
			case 'b':
				out = append(out, '\b')
				p.pos++
			case 'f':
				out = append(out, '\f')
				p.pos++
			case 'n':
				out = append(out, '\n')
				p.pos++
			case 'r':
				out = append(out, '\r')
				p.pos++
			case 't':
				out = append(out, '\t')
				p.pos++
			case 'u':
				r, err := p.parseUnicodeEscape()
				if err != nil {
					return "", err
				}
				var encoded [utf8.UTFMax]byte
				n := utf8.EncodeRune(encoded[:], r)
				out = append(out, encoded[:n]...)
			default:
				return "", ErrInvalidJSON
			}
		case c < 0x20:
			return "", ErrInvalidJSON
		case c < utf8.RuneSelf:
			out = append(out, c)
			p.pos++
		default:
			r, size := utf8.DecodeRune(p.raw[p.pos:])
			if r == utf8.RuneError && size == 1 {
				return "", ErrInvalidUnicode
			}
			out = append(out, p.raw[p.pos:p.pos+size]...)
			p.pos += size
		}
	}
	return "", ErrInvalidJSON
}

func (p *parser) parseUnicodeEscape() (rune, error) {
	// Current position is the 'u' in the escape.
	if p.pos+5 > len(p.raw) {
		return 0, ErrInvalidUnicode
	}
	high, ok := fourHex(p.raw[p.pos+1 : p.pos+5])
	if !ok {
		return 0, ErrInvalidUnicode
	}
	p.pos += 5
	if high >= 0xd800 && high <= 0xdbff {
		if p.pos+6 > len(p.raw) || p.raw[p.pos] != '\\' || p.raw[p.pos+1] != 'u' {
			return 0, ErrInvalidUnicode
		}
		low, ok := fourHex(p.raw[p.pos+2 : p.pos+6])
		if !ok || low < 0xdc00 || low > 0xdfff {
			return 0, ErrInvalidUnicode
		}
		p.pos += 6
		return utf16.DecodeRune(rune(high), rune(low)), nil
	}
	if high >= 0xdc00 && high <= 0xdfff {
		return 0, ErrInvalidUnicode
	}
	return rune(high), nil
}

func fourHex(value []byte) (uint16, bool) {
	if len(value) != 4 {
		return 0, false
	}
	var out uint16
	for _, c := range value {
		out <<= 4
		switch {
		case c >= '0' && c <= '9':
			out += uint16(c - '0')
		case c >= 'a' && c <= 'f':
			out += uint16(c-'a') + 10
		case c >= 'A' && c <= 'F':
			out += uint16(c-'A') + 10
		default:
			return 0, false
		}
	}
	return out, true
}

func (p *parser) parseNumber() ([]byte, error) {
	start := p.pos
	if p.raw[p.pos] == '-' {
		p.pos++
		if p.pos >= len(p.raw) {
			return nil, ErrInvalidJSON
		}
	}
	if p.raw[p.pos] == '0' {
		p.pos++
		if p.pos < len(p.raw) && p.raw[p.pos] >= '0' && p.raw[p.pos] <= '9' {
			return nil, ErrInvalidJSON
		}
	} else if p.raw[p.pos] >= '1' && p.raw[p.pos] <= '9' {
		for p.pos < len(p.raw) && p.raw[p.pos] >= '0' && p.raw[p.pos] <= '9' {
			p.pos++
		}
	} else {
		return nil, ErrInvalidJSON
	}
	if p.pos < len(p.raw) && p.raw[p.pos] == '.' {
		p.pos++
		fractionStart := p.pos
		for p.pos < len(p.raw) && p.raw[p.pos] >= '0' && p.raw[p.pos] <= '9' {
			p.pos++
		}
		if p.pos == fractionStart {
			return nil, ErrInvalidJSON
		}
	}
	if p.pos < len(p.raw) && (p.raw[p.pos] == 'e' || p.raw[p.pos] == 'E') {
		p.pos++
		if p.pos < len(p.raw) && (p.raw[p.pos] == '+' || p.raw[p.pos] == '-') {
			p.pos++
		}
		exponentStart := p.pos
		for p.pos < len(p.raw) && p.raw[p.pos] >= '0' && p.raw[p.pos] <= '9' {
			p.pos++
		}
		if p.pos == exponentStart {
			return nil, ErrInvalidJSON
		}
	}

	token := string(p.raw[start:p.pos])
	value, err := strconv.ParseFloat(token, 64)
	if err != nil || math.IsInf(value, 0) || math.IsNaN(value) {
		return nil, ErrNumberRange
	}
	mantissaEnd := start
	for mantissaEnd < p.pos && p.raw[mantissaEnd] != 'e' && p.raw[mantissaEnd] != 'E' {
		mantissaEnd++
	}
	// Um decimal não nulo que arredonda para zero não é representável como
	// valor IEEE-754 recebido pelo contrato; zeros lexicais continuam válidos.
	if value == 0 && hasNonZeroDigit(p.raw[start:mantissaEnd]) {
		return nil, ErrNumberRange
	}
	return []byte(formatNumber(value)), nil
}

func hasNonZeroDigit(value []byte) bool {
	for _, c := range value {
		if c >= '1' && c <= '9' {
			return true
		}
	}
	return false
}

func formatNumber(value float64) string {
	if value == 0 {
		return "0"
	}
	negative := math.Signbit(value)
	if negative {
		value = -value
	}

	formatted := strconv.FormatFloat(value, 'e', -1, 64)
	eIndex := 0
	for eIndex < len(formatted) && formatted[eIndex] != 'e' {
		eIndex++
	}
	mantissa := formatted[:eIndex]
	exponent, _ := strconv.Atoi(formatted[eIndex+1:])
	digits := make([]byte, 0, len(mantissa)-1)
	for i := 0; i < len(mantissa); i++ {
		if mantissa[i] != '.' {
			digits = append(digits, mantissa[i])
		}
	}

	var out []byte
	if value >= 1e-6 && value < 1e21 {
		point := exponent + 1
		switch {
		case point <= 0:
			out = append(out, "0."...)
			out = appendZeros(out, -point)
			out = append(out, digits...)
		case point >= len(digits):
			out = append(out, digits...)
			out = appendZeros(out, point-len(digits))
		default:
			out = append(out, digits[:point]...)
			out = append(out, '.')
			out = append(out, digits[point:]...)
		}
	} else {
		out = append(out, digits[0])
		if len(digits) > 1 {
			out = append(out, '.')
			out = append(out, digits[1:]...)
		}
		out = append(out, 'e')
		if exponent >= 0 {
			out = append(out, '+')
		}
		out = strconv.AppendInt(out, int64(exponent), 10)
	}
	if negative {
		return "-" + string(out)
	}
	return string(out)
}

func appendZeros(out []byte, count int) []byte {
	for i := 0; i < count; i++ {
		out = append(out, '0')
	}
	return out
}

func appendJSONString(out []byte, value string) []byte {
	const hexDigits = "0123456789abcdef"
	out = append(out, '"')
	for i := 0; i < len(value); {
		r, size := utf8.DecodeRuneInString(value[i:])
		i += size
		switch r {
		case '"', '\\':
			out = append(out, '\\', byte(r))
		case '\b':
			out = append(out, '\\', 'b')
		case '\t':
			out = append(out, '\\', 't')
		case '\n':
			out = append(out, '\\', 'n')
		case '\f':
			out = append(out, '\\', 'f')
		case '\r':
			out = append(out, '\\', 'r')
		default:
			if r < 0x20 {
				out = append(out, '\\', 'u', '0', '0', hexDigits[byte(r)>>4], hexDigits[byte(r)&0xf])
			} else {
				out = append(out, value[i-size:i]...)
			}
		}
	}
	return append(out, '"')
}

func utf16Less(a, b string) bool {
	aa := utf16.Encode([]rune(a))
	bb := utf16.Encode([]rune(b))
	for i := 0; i < len(aa) && i < len(bb); i++ {
		if aa[i] != bb[i] {
			return aa[i] < bb[i]
		}
	}
	return len(aa) < len(bb)
}

func joinArray(values [][]byte) []byte {
	out := make([]byte, 0, 2)
	out = append(out, '[')
	for i, value := range values {
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, value...)
	}
	return append(out, ']')
}

func joinObject(members []member) []byte {
	out := make([]byte, 0, 2)
	out = append(out, '{')
	for i, value := range members {
		if i > 0 {
			out = append(out, ',')
		}
		out = appendJSONString(out, value.name)
		out = append(out, ':')
		out = append(out, value.value...)
	}
	return append(out, '}')
}

func (p *parser) skipSpace() {
	for p.pos < len(p.raw) {
		switch p.raw[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *parser) takeLiteral(literal string) bool {
	if len(p.raw)-p.pos < len(literal) || string(p.raw[p.pos:p.pos+len(literal)]) != literal {
		return false
	}
	p.pos += len(literal)
	return true
}

type boundedBuffer struct {
	bytes.Buffer
	maxSize int
}

func (b *boundedBuffer) Write(value []byte) (int, error) {
	if b.Len()+len(value) > b.maxSize+1 {
		return 0, ErrDocumentTooLarge
	}
	return b.Buffer.Write(value)
}

type visit struct {
	typ  reflect.Type
	ptr  uintptr
	kind reflect.Kind
}

func validateValue(value reflect.Value, depth int, active map[visit]struct{}) error {
	if !value.IsValid() {
		return nil
	}
	for value.Kind() == reflect.Interface {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
	}

	switch value.Kind() {
	case reflect.String:
		if !utf8.ValidString(value.String()) {
			return ErrInvalidUnicode
		}
	case reflect.Float32, reflect.Float64:
		f := value.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return ErrNumberRange
		}
	case reflect.Ptr:
		if value.IsNil() {
			return nil
		}
		id := visit{typ: value.Type(), ptr: value.Pointer(), kind: value.Kind()}
		if _, exists := active[id]; exists {
			return ErrInvalidValue
		}
		active[id] = struct{}{}
		defer delete(active, id)
		return validateValue(value.Elem(), depth, active)
	case reflect.Map, reflect.Slice, reflect.Array, reflect.Struct:
		if depth+1 > maxDepth {
			return ErrDepthExceeded
		}
		if value.Kind() == reflect.Map && value.IsNil() {
			return nil
		}
		if value.Kind() == reflect.Slice && value.IsNil() {
			return nil
		}
		var id visit
		if value.Kind() != reflect.Array && value.Kind() != reflect.Struct {
			id = visit{typ: value.Type(), ptr: value.Pointer(), kind: value.Kind()}
			if id.ptr != 0 {
				if _, exists := active[id]; exists {
					return ErrInvalidValue
				}
				active[id] = struct{}{}
				defer delete(active, id)
			}
		}
		switch value.Kind() {
		case reflect.Map:
			for iter := value.MapRange(); iter.Next(); {
				if err := validateValue(iter.Key(), depth+1, active); err != nil {
					return err
				}
				if err := validateValue(iter.Value(), depth+1, active); err != nil {
					return err
				}
			}
		case reflect.Array, reflect.Slice:
			for i := 0; i < value.Len(); i++ {
				if err := validateValue(value.Index(i), depth+1, active); err != nil {
					return err
				}
			}
		case reflect.Struct:
			for i := 0; i < value.NumField(); i++ {
				field := value.Type().Field(i)
				if field.PkgPath != "" {
					continue
				}
				jsonName, _, _ := strings.Cut(field.Tag.Get("json"), ",")
				if jsonName == "-" {
					continue
				}
				if err := validateValue(value.Field(i), depth+1, active); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return nil
}
