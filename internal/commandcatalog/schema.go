package commandcatalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"assistente/internal/commandjson"
)

// SchemaType é o subconjunto fechado de tipos aceito pelo catálogo. O
// catálogo não interpreta palavras-chave JSON Schema fora deste conjunto.
type SchemaType string

const (
	SchemaObject  SchemaType = "object"
	SchemaArray   SchemaType = "array"
	SchemaString  SchemaType = "string"
	SchemaNumber  SchemaType = "number"
	SchemaInteger SchemaType = "integer"
	SchemaBoolean SchemaType = "boolean"
	SchemaNull    SchemaType = "null"
)

// Schema é um schema JSON tipado e fechado. A ausência de uma propriedade é
// controlada por Optional; Nullable controla somente a aceitação de null.
// Portanto, uma propriedade pode ser opcional sem ser nullable e vice-versa.
//
// Required é aceito como forma explícita de declarar propriedades obrigatórias
// para facilitar a construção por callers. Em um schema válido ele deve ser
// consistente com Optional. AdditionalProperties não existe de propósito:
// objetos deste subconjunto são sempre fechados.
type Schema struct {
	Type       SchemaType
	Optional   bool
	Nullable   bool
	Properties map[string]Schema
	Required   []string
	Items      *Schema
	Enum       []any

	Minimum   *float64
	Maximum   *float64
	MinLength *int
	MaxLength *int
	MinItems  *int
	MaxItems  *int
}

const (
	// commandjson impõe o limite de entrada do envelope canônico em 64 KiB.
	// O catálogo mantém o mesmo teto para não validar algo que não pode ser
	// recebido pelo caminho real.
	maxSchemaDocumentBytes = 64 << 10
	maxSchemaDepth         = 64
)

func cloneSchema(s *Schema) *Schema {
	return cloneSchemaAt(s, 0, make(map[*Schema]bool))
}

func cloneSchemaAt(s *Schema, depth int, active map[*Schema]bool) *Schema {
	if s == nil {
		return nil
	}
	if depth >= maxSchemaDepth || active[s] {
		// Schemas cíclicos são inválidos no modo completo. O construtor
		// compatível também precisa permanecer seguro ao receber um bootstrap
		// defeituoso, por isso corta o ramo em vez de recursar indefinidamente.
		return nil
	}
	active[s] = true
	defer delete(active, s)
	out := *s
	out.Required = slices.Clone(s.Required)
	out.Enum = cloneJSONValuesAt(s.Enum, 0, make(map[visit]bool))
	if s.Properties != nil {
		out.Properties = make(map[string]Schema, len(s.Properties))
		for name, property := range s.Properties {
			out.Properties[name] = cloneSchemaValueAt(property, depth+1, active)
		}
	}
	out.Items = cloneSchemaAt(s.Items, depth+1, active)
	out.Minimum = cloneFloat(s.Minimum)
	out.Maximum = cloneFloat(s.Maximum)
	out.MinLength = cloneInt(s.MinLength)
	out.MaxLength = cloneInt(s.MaxLength)
	out.MinItems = cloneInt(s.MinItems)
	out.MaxItems = cloneInt(s.MaxItems)
	return &out
}

func cloneSchemaValueAt(s Schema, depth int, active map[*Schema]bool) Schema {
	cloned := cloneSchemaAt(&s, depth, active)
	if cloned == nil {
		return Schema{}
	}
	return *cloned
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

type visit struct {
	kind reflect.Kind
	ptr  uintptr
}

func cloneJSONValuesAt(values []any, depth int, active map[visit]bool) []any {
	if values == nil {
		return nil
	}
	out := make([]any, len(values))
	for i, value := range values {
		out[i] = cloneJSONValueAt(value, depth, active)
	}
	return out
}

func cloneJSONValueAt(value any, depth int, active map[visit]bool) any {
	if depth >= maxSchemaDepth {
		return nil
	}
	switch value := value.(type) {
	case map[string]any:
		key := visit{kind: reflect.Map, ptr: reflect.ValueOf(value).Pointer()}
		if active[key] {
			return nil
		}
		active[key] = true
		defer delete(active, key)
		out := make(map[string]any, len(value))
		for key, item := range value {
			out[key] = cloneJSONValueAt(item, depth+1, active)
		}
		return out
	case []any:
		key := visit{kind: reflect.Slice, ptr: reflect.ValueOf(value).Pointer()}
		if active[key] {
			return nil
		}
		active[key] = true
		defer delete(active, key)
		out := make([]any, len(value))
		for i, item := range value {
			out[i] = cloneJSONValueAt(item, depth+1, active)
		}
		return out
	default:
		return value
	}
}

func validateSchemaDefinition(schema *Schema, path string, root bool) error {
	return validateSchemaDefinitionAt(schema, path, root, 0, make(map[*Schema]bool))
}

func validateSchemaDefinitionAt(schema *Schema, path string, root bool, depth int, active map[*Schema]bool) error {
	if schema == nil {
		return fmt.Errorf("%s: schema ausente", path)
	}
	if root && schema.Optional {
		return fmt.Errorf("%s: schema raiz não pode ser opcional", path)
	}
	if depth >= maxSchemaDepth {
		return fmt.Errorf("%s: schema excede a profundidade máxima", path)
	}
	if active[schema] {
		return fmt.Errorf("%s: schema cíclico", path)
	}
	active[schema] = true
	defer delete(active, schema)
	if !validSchemaType(schema.Type) {
		return fmt.Errorf("%s: tipo %q não suportado", path, schema.Type)
	}
	if schema.Minimum != nil && (math.IsNaN(*schema.Minimum) || math.IsInf(*schema.Minimum, 0)) {
		return fmt.Errorf("%s: minimum inválido", path)
	}
	if schema.Maximum != nil && (math.IsNaN(*schema.Maximum) || math.IsInf(*schema.Maximum, 0)) {
		return fmt.Errorf("%s: maximum inválido", path)
	}
	if schema.Minimum != nil && schema.Maximum != nil && *schema.Minimum > *schema.Maximum {
		return fmt.Errorf("%s: minimum maior que maximum", path)
	}
	if schema.MinLength != nil && *schema.MinLength < 0 || schema.MaxLength != nil && *schema.MaxLength < 0 {
		return fmt.Errorf("%s: limite de string negativo", path)
	}
	if schema.MinItems != nil && *schema.MinItems < 0 || schema.MaxItems != nil && *schema.MaxItems < 0 {
		return fmt.Errorf("%s: limite de array negativo", path)
	}
	if schema.MinLength != nil && schema.MaxLength != nil && *schema.MinLength > *schema.MaxLength {
		return fmt.Errorf("%s: minLength maior que maxLength", path)
	}
	if schema.MinItems != nil && schema.MaxItems != nil && *schema.MinItems > *schema.MaxItems {
		return fmt.Errorf("%s: minItems maior que maxItems", path)
	}

	if schema.Type != SchemaString && (schema.MinLength != nil || schema.MaxLength != nil) {
		return fmt.Errorf("%s: limites de string em tipo não textual", path)
	}
	if schema.Type != SchemaArray && (schema.MinItems != nil || schema.MaxItems != nil || schema.Items != nil) {
		return fmt.Errorf("%s: limites/items em tipo não array", path)
	}
	if schema.Type != SchemaObject && (len(schema.Properties) != 0 || len(schema.Required) != 0) {
		return fmt.Errorf("%s: propriedades em tipo não objeto", path)
	}
	if schema.Type != SchemaNumber && schema.Type != SchemaInteger && (schema.Minimum != nil || schema.Maximum != nil) {
		return fmt.Errorf("%s: limites numéricos em tipo não numérico", path)
	}
	switch schema.Type {
	case SchemaObject:
		if schema.Items != nil {
			return fmt.Errorf("%s: objeto não possui items", path)
		}
		seenRequired := make(map[string]struct{}, len(schema.Required))
		for _, name := range schema.Required {
			if _, duplicate := seenRequired[name]; duplicate {
				return fmt.Errorf("%s: propriedade obrigatória repetida %q", path, name)
			}
			seenRequired[name] = struct{}{}
			property, exists := schema.Properties[name]
			if !exists {
				return fmt.Errorf("%s: propriedade obrigatória ausente %q", path, name)
			}
			if property.Optional {
				return fmt.Errorf("%s: propriedade %q é optional e required", path, name)
			}
		}
		for name, property := range schema.Properties {
			if name == "" || !utf8.ValidString(name) {
				return fmt.Errorf("%s: nome de propriedade inválido", path)
			}
			if err := validateSchemaDefinitionAt(&property, path+"."+name, false, depth+1, active); err != nil {
				return err
			}
		}
	case SchemaArray:
		if err := validateSchemaDefinitionAt(schema.Items, path+"[]", false, depth+1, active); err != nil {
			return err
		}
	}
	if schema.Type != SchemaObject && schema.Type != SchemaArray && len(schema.Properties) == 0 && len(schema.Required) == 0 && schema.Items != nil {
		return fmt.Errorf("%s: items em tipo escalar", path)
	}
	for i, enumValue := range schema.Enum {
		if err := validateEnumValueAt(enumValue, path+fmt.Sprintf(".enum[%d]", i), 0, make(map[visit]bool)); err != nil {
			return err
		}
		if err := validateSchemaValue(enumValue, schema, path+fmt.Sprintf(".enum[%d]", i), false); err != nil {
			return fmt.Errorf("%s: enum[%d] inválido", path, i)
		}
	}
	for i := 0; i < len(schema.Enum); i++ {
		for j := i + 1; j < len(schema.Enum); j++ {
			if jsonValuesEqual(schema.Enum[i], schema.Enum[j]) {
				return fmt.Errorf("%s: enum contém valores duplicados", path)
			}
		}
	}
	return nil
}

func validSchemaType(value SchemaType) bool {
	switch value {
	case SchemaObject, SchemaArray, SchemaString, SchemaNumber, SchemaInteger, SchemaBoolean, SchemaNull:
		return true
	default:
		return false
	}
}

func validateEnumValueAt(value any, path string, depth int, active map[visit]bool) error {
	if depth >= maxSchemaDepth {
		return fmt.Errorf("%s: enum excede a profundidade máxima", path)
	}
	switch value := value.(type) {
	case nil, string, bool, json.Number, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		if number, ok := numericValue(value); ok && (math.IsNaN(number) || math.IsInf(number, 0)) {
			return fmt.Errorf("%s: número inválido", path)
		}
		if number, ok := value.(json.Number); ok {
			if _, valid := numberRat(number); !valid {
				return fmt.Errorf("%s: número inválido", path)
			}
		}
		return nil
	case map[string]any:
		key := visit{kind: reflect.Map, ptr: reflect.ValueOf(value).Pointer()}
		if active[key] {
			return fmt.Errorf("%s: enum cíclico", path)
		}
		active[key] = true
		defer delete(active, key)
		for key, item := range value {
			if key == "" || !utf8.ValidString(key) {
				return fmt.Errorf("%s: chave inválida", path)
			}
			if err := validateEnumValueAt(item, path+"."+key, depth+1, active); err != nil {
				return err
			}
		}
		return nil
	case []any:
		key := visit{kind: reflect.Slice, ptr: reflect.ValueOf(value).Pointer()}
		if active[key] {
			return fmt.Errorf("%s: enum cíclico", path)
		}
		active[key] = true
		defer delete(active, key)
		for i, item := range value {
			if err := validateEnumValueAt(item, fmt.Sprintf("%s[%d]", path, i), depth+1, active); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("%s: enum não é JSON", path)
	}
}

func validateDocument(raw []byte, schema *Schema) ([]byte, error) {
	if schema == nil {
		return nil, fmt.Errorf("schema ausente")
	}
	if len(raw) == 0 || len(raw) > maxSchemaDocumentBytes {
		return nil, fmt.Errorf("documento JSON ausente ou excede o limite")
	}
	if err := validateSchemaDefinition(schema, "$", true); err != nil {
		return nil, err
	}
	canonical, err := commandjson.Canonicalize(raw)
	if err != nil {
		// O erro do canonicalizer pode carregar trechos do documento. O
		// catálogo expõe apenas um diagnóstico estrutural seguro.
		return nil, fmt.Errorf("JSON não canônico")
	}
	if len(canonical) == 0 || len(canonical) > maxSchemaDocumentBytes {
		return nil, fmt.Errorf("JSON canônico excede o limite")
	}
	decoder := json.NewDecoder(bytes.NewReader(canonical))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("JSON inválido: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return nil, fmt.Errorf("JSON contém mais de um valor")
	}
	if err := validateSchemaValue(value, schema, "$", true); err != nil {
		return nil, err
	}
	return slices.Clone(canonical), nil
}

func validateSchemaValue(value any, schema *Schema, path string, root bool) error {
	if value == nil {
		if schema.Nullable || schema.Type == SchemaNull {
			if len(schema.Enum) != 0 {
				for _, candidate := range schema.Enum {
					if candidate == nil {
						return nil
					}
				}
				return fmt.Errorf("%s: valor fora do enum", path)
			}
			return nil
		}
		return fmt.Errorf("%s: null não permitido", path)
	}
	if schema.Type == SchemaNull {
		return fmt.Errorf("%s: esperava null", path)
	}
	if !valueMatchesSchema(value, schema) {
		return fmt.Errorf("%s: tipo incompatível, esperado %s", path, schema.Type)
	}
	switch schema.Type {
	case SchemaObject:
		object := value.(map[string]any)
		for name := range object {
			if _, ok := schema.Properties[name]; !ok {
				return fmt.Errorf("%s: propriedade desconhecida", path)
			}
		}
		for name, property := range schema.Properties {
			propertyValue, exists := object[name]
			if !exists {
				if !propertyIsRequired(schema, name, property) {
					continue
				}
				return fmt.Errorf("%s: propriedade obrigatória ausente %q", path, name)
			}
			if err := validateSchemaValue(propertyValue, &property, path+"."+name, false); err != nil {
				return err
			}
		}
	case SchemaArray:
		array := value.([]any)
		if schema.MinItems != nil && len(array) < *schema.MinItems || schema.MaxItems != nil && len(array) > *schema.MaxItems {
			return fmt.Errorf("%s: quantidade de itens fora dos limites", path)
		}
		for i, item := range array {
			if err := validateSchemaValue(item, schema.Items, fmt.Sprintf("%s[%d]", path, i), false); err != nil {
				return err
			}
		}
	case SchemaString:
		text := value.(string)
		length := len([]rune(text))
		if schema.MinLength != nil && length < *schema.MinLength || schema.MaxLength != nil && length > *schema.MaxLength {
			return fmt.Errorf("%s: tamanho fora dos limites", path)
		}
	case SchemaNumber, SchemaInteger:
		number, valid := exactNumber(value)
		if !valid {
			return fmt.Errorf("%s: número inválido", path)
		}
		if schema.Minimum != nil && number.Cmp(floatRat(*schema.Minimum)) < 0 || schema.Maximum != nil && number.Cmp(floatRat(*schema.Maximum)) > 0 {
			return fmt.Errorf("%s: número fora dos limites", path)
		}
	}
	if len(schema.Enum) != 0 {
		matched := false
		for _, candidate := range schema.Enum {
			if jsonValuesEqual(value, candidate) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%s: valor fora do enum", path)
		}
	}
	_ = root
	return nil
}

func propertyIsRequired(schema *Schema, name string, property Schema) bool {
	if schema.Required != nil {
		return slices.Contains(schema.Required, name)
	}
	return !property.Optional
}

func valueMatchesSchema(value any, schema *Schema) bool {
	switch schema.Type {
	case SchemaObject:
		_, ok := value.(map[string]any)
		return ok
	case SchemaArray:
		_, ok := value.([]any)
		return ok
	case SchemaString:
		_, ok := value.(string)
		return ok
	case SchemaNumber:
		_, ok := exactNumber(value)
		return ok
	case SchemaInteger:
		number, ok := value.(json.Number)
		if !ok {
			number, ok := exactNumber(value)
			return ok && number.IsInt()
		}
		rational, valid := numberRat(number)
		return valid && rational.IsInt()
	case SchemaBoolean:
		_, ok := value.(bool)
		return ok
	case SchemaNull:
		return value == nil
	default:
		return false
	}
}

func numericValue(value any) (float64, bool) {
	switch value := value.(type) {
	case json.Number:
		parsed, err := value.Float64()
		return parsed, err == nil
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int8:
		return float64(value), true
	case int16:
		return float64(value), true
	case int32:
		return float64(value), true
	case int64:
		return float64(value), true
	case uint:
		return float64(value), true
	case uint8:
		return float64(value), true
	case uint16:
		return float64(value), true
	case uint32:
		return float64(value), true
	case uint64:
		return float64(value), true
	default:
		return 0, false
	}
}

func jsonValuesEqual(left, right any) bool {
	// Igualdade JSON, inclusive números dentro de objetos/arrays. A definição
	// e os argumentos usam a mesma representação numérica JCS suportada.
	l, le := commandjson.Marshal(left)
	r, re := commandjson.Marshal(right)
	return le == nil && re == nil && bytes.Equal(l, r)
}

func exactNumber(value any) (*big.Rat, bool) {
	switch value := value.(type) {
	case json.Number:
		return numberRat(value)
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, false
		}
		return numberRat(json.Number(strconv.FormatFloat(value, 'g', -1, 64)))
	case float32:
		return exactNumber(float64(value))
	case int:
		return new(big.Rat).SetInt64(int64(value)), true
	case int8:
		return new(big.Rat).SetInt64(int64(value)), true
	case int16:
		return new(big.Rat).SetInt64(int64(value)), true
	case int32:
		return new(big.Rat).SetInt64(int64(value)), true
	case int64:
		return new(big.Rat).SetInt64(value), true
	case uint, uint8, uint16, uint32, uint64:
		return numberRat(json.Number(strconv.FormatUint(reflect.ValueOf(value).Uint(), 10)))
	default:
		return nil, false
	}
}

func numberRat(value json.Number) (*big.Rat, bool) {
	text := value.String()
	if text == "" {
		return nil, false
	}
	exponent := 0
	if position := strings.IndexAny(text, "eE"); position >= 0 {
		parsed, err := strconv.Atoi(text[position+1:])
		if err != nil {
			return nil, false
		}
		exponent = parsed
		if exponent > maxSchemaDocumentBytes || exponent < -maxSchemaDocumentBytes {
			return nil, false
		}
		text = text[:position]
	}
	decimals := 0
	if position := strings.IndexByte(text, '.'); position >= 0 {
		decimals = len(text) - position - 1
		text = text[:position] + text[position+1:]
	}
	if text == "" || text == "-" {
		return nil, false
	}
	coefficient := new(big.Int)
	if _, ok := coefficient.SetString(text, 10); !ok {
		return nil, false
	}
	shift := exponent - decimals
	if shift >= 0 {
		coefficient.Mul(coefficient, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(shift)), nil))
		return new(big.Rat).SetInt(coefficient), true
	}
	denominator := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-shift)), nil)
	return new(big.Rat).SetFrac(coefficient, denominator), true
}

func floatRat(value float64) *big.Rat {
	rational, _ := numberRat(json.Number(strconv.FormatFloat(value, 'g', -1, 64)))
	return rational
}

// ValidateArguments valida o JSON contra o schema do comando identificado por
// ID exato e retorna a representação produzida pelo canonicalizer confiável.
// Não aplica defaults, transforma valores nem executa handler.
func (r *Registry) ValidateArguments(id string, raw []byte) ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("registry ausente")
	}
	d, ok := r.Lookup(id)
	if !ok {
		return nil, fmt.Errorf("comando %q não encontrado por ID exato", id)
	}
	return d.ValidateArguments(raw)
}

// ValidateResult é o par de ValidateArguments para o payload produzido pelo
// handler. A validação ainda é somente estrutural e não faz despacho.
func (r *Registry) ValidateResult(id string, raw []byte) ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("registry ausente")
	}
	d, ok := r.Lookup(id)
	if !ok {
		return nil, fmt.Errorf("comando %q não encontrado por ID exato", id)
	}
	return d.ValidateResult(raw)
}

func (d Definition) ValidateArguments(raw []byte) ([]byte, error) {
	return validateDocument(raw, d.ArgumentsSchema)
}

func (d Definition) ValidateResult(raw []byte) ([]byte, error) {
	return validateDocument(raw, d.ResultSchema)
}
