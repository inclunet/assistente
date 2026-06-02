package jobs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"
)

// SecretResolver busca secrets pelo nome (integra com credentials.Store).
type SecretResolver func(key string) (string, error)

// TemplateContext e o contexto disponivel durante a resolucao de templates.
type TemplateContext struct {
	Event   map[string]any
	Output  map[string]any
	Secrets SecretResolver
	Now     time.Time
}

// templateFuncs retorna as funcoes custom disponiveis em templates.
func templateFuncs(secrets SecretResolver) template.FuncMap {
	return template.FuncMap{
		"pluck":        tplPluck,
		"any":          tplAny,
		"date":         tplDate,
		"now":          func() time.Time { return time.Now() },
		"join":         tplJoin,
		"secret":       makeSecretFunc(secrets),
		"json":         tplJSON,
		"default":      tplDefault,
		"adf_markdown": tplADFMarkdown,
		"adf_text":     tplADFText,
	}
}

// ResolveInputs resolve templates em todos os valores do mapa de inputs.
// Valores sem {{ }} sao retornados como estao.
func ResolveInputs(inputs map[string]any, ctx *TemplateContext) (map[string]any, error) {
	if len(inputs) == 0 {
		return map[string]any{}, nil
	}

	resolved := make(map[string]any, len(inputs))
	for k, v := range inputs {
		val, err := resolveValue(v, ctx)
		if err != nil {
			return nil, fmt.Errorf("input %q: %w", k, err)
		}
		resolved[k] = val
	}
	return resolved, nil
}

// ResolveOutputMap aplica o mapeamento de output, transformando campos.
func ResolveOutputMap(outputMap map[string]string, ctx *TemplateContext) (map[string]any, error) {
	if len(outputMap) == 0 {
		return ctx.Output, nil
	}

	resolved := make(map[string]any, len(outputMap))
	for k, tmplStr := range outputMap {
		val, err := resolveTemplate(tmplStr, ctx)
		if err != nil {
			return nil, fmt.Errorf("output map %q: %w", k, err)
		}
		resolved[k] = val
	}
	return resolved, nil
}

func resolveValue(v any, ctx *TemplateContext) (any, error) {
	switch val := v.(type) {
	case string:
		if !containsTemplate(val) {
			return val, nil
		}
		return resolveTemplate(val, ctx)
	case map[string]any:
		resolved := make(map[string]any, len(val))
		for k, inner := range val {
			r, err := resolveValue(inner, ctx)
			if err != nil {
				return nil, err
			}
			resolved[k] = r
		}
		return resolved, nil
	case []any:
		resolved := make([]any, len(val))
		for i, inner := range val {
			r, err := resolveValue(inner, ctx)
			if err != nil {
				return nil, err
			}
			resolved[i] = r
		}
		return resolved, nil
	default:
		return v, nil
	}
}

func containsTemplate(s string) bool {
	return strings.Contains(s, "{{")
}

// missingDotRe matches Go template expressions where root variables (event, output, now)
// are used without the required leading dot: {{ event.x }} instead of {{ .event.x }}.
var missingDotRe = regexp.MustCompile(`(\{\{-?\s*)(event|output|now)\b`)

// fixTemplateDots adds the missing dot prefix for known root variables.
// {{ event.x }} -> {{ .event.x }}. Already-correct {{ .event.x }} are untouched.
func fixTemplateDots(tmpl string) string {
	return missingDotRe.ReplaceAllString(tmpl, "${1}.${2}")
}

// templateBlockRe matches individual {{ ... }} blocks.
var templateBlockRe = regexp.MustCompile(`\{\{.*?\}\}`)

// numericIndexRe matches dot-notation array access: .field.0 where 0 is a numeric index.
// Go templates don't support numeric indexing via dot notation — needs (index .field 0).
var numericIndexRe = regexp.MustCompile(`((?:\.[\w]+)+)\.([\d]+)`)

// fixArrayAccess converts JS-style numeric dot notation to Go template (index ...) calls.
// {{ .event.content.0.id }} -> {{ (index .event.content 0).id }}
func fixArrayAccess(tmpl string) string {
	return templateBlockRe.ReplaceAllStringFunc(tmpl, func(block string) string {
		for numericIndexRe.MatchString(block) {
			block = numericIndexRe.ReplaceAllString(block, "(index $1 $2)")
		}
		return block
	})
}

func resolveTemplate(tmplStr string, ctx *TemplateContext) (any, error) {
	tmplStr = fixTemplateDots(tmplStr)
	tmplStr = fixArrayAccess(tmplStr)

	funcs := templateFuncs(ctx.Secrets)

	t, err := template.New("").Funcs(funcs).Parse(tmplStr)
	if err != nil {
		return nil, fmt.Errorf("template parse error: %w", err)
	}

	data := map[string]any{
		"event":  ctx.Event,
		"output": ctx.Output,
		"now":    ctx.Now,
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("template exec error (template=%q): %w", tmplStr, err)
	}

	result := strings.TrimSpace(buf.String())
	return result, nil
}

// RenderWithRoot renderiza um template Go contra uma raiz de dados arbitrária
// (ex.: {"task": {...}, "now": time.Time}), reusando as mesmas funções e
// correções de sintaxe dos templates de jobs. Usado por custom actions de
// tasklists (AEP-0067), onde a raiz é `.task`.
func RenderWithRoot(tmplStr string, data map[string]any) (string, error) {
	tmplStr = fixTemplateDots(tmplStr)
	tmplStr = fixArrayAccess(tmplStr)

	t, err := template.New("").Funcs(templateFuncs(nil)).Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("template parse error: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("template exec error (template=%q): %w", tmplStr, err)
	}
	return strings.TrimSpace(buf.String()), nil
}

// EvaluateConditionWithRoot avalia uma condição (template truthy) contra uma raiz
// arbitrária. Condição vazia é sempre verdadeira (sem filtragem).
func EvaluateConditionWithRoot(condition string, data map[string]any) (bool, error) {
	if strings.TrimSpace(condition) == "" {
		return true, nil
	}
	result, err := RenderWithRoot(condition, data)
	if err != nil {
		return false, fmt.Errorf("condition eval: %w", err)
	}
	s := strings.TrimSpace(result)
	return s != "" && s != "false" && s != "<no value>", nil
}

// --- Funcoes de template ---

// pluck extrai um campo de cada item de uma slice de maps.
// Uso: {{ pluck .output.issues "key" }}
func tplPluck(items any, field string) ([]any, error) {
	rv := reflect.ValueOf(items)
	if rv.Kind() != reflect.Slice {
		return nil, fmt.Errorf("pluck: expected slice, got %T", items)
	}

	result := make([]any, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		item := rv.Index(i).Interface()
		val, ok := navigatePath(item, field)
		if ok {
			result = append(result, val)
		}
	}
	return result, nil
}

// any verifica se algum item de uma slice tem campo == valor.
// Uso: {{ any .output.issues "fields.priority.name" "Critical" }}
func tplAny(items any, path string, value any) (bool, error) {
	rv := reflect.ValueOf(items)
	if rv.Kind() != reflect.Slice {
		return false, fmt.Errorf("any: expected slice, got %T", items)
	}

	for i := 0; i < rv.Len(); i++ {
		item := rv.Index(i).Interface()
		val, ok := navigatePath(item, path)
		if ok && fmt.Sprintf("%v", val) == fmt.Sprintf("%v", value) {
			return true, nil
		}
	}
	return false, nil
}

// date formata um time.Time.
// Uso: {{ date .now "2006-01-02" }}
func tplDate(t any, layout string) (string, error) {
	switch v := t.(type) {
	case time.Time:
		return v.Format(layout), nil
	case string:
		parsed, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return "", fmt.Errorf("date: cannot parse %q: %w", v, err)
		}
		return parsed.Format(layout), nil
	default:
		return "", fmt.Errorf("date: expected time or string, got %T", t)
	}
}

// join concatena uma slice com separador.
// Uso: {{ join .output.keys ", " }}
func tplJoin(items any, sep string) (string, error) {
	rv := reflect.ValueOf(items)
	if rv.Kind() != reflect.Slice {
		return "", fmt.Errorf("join: expected slice, got %T", items)
	}

	parts := make([]string, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		parts[i] = fmt.Sprintf("%v", rv.Index(i).Interface())
	}
	return strings.Join(parts, sep), nil
}

func makeSecretFunc(resolver SecretResolver) func(string) (string, error) {
	return func(key string) (string, error) {
		if resolver == nil {
			return "", fmt.Errorf("secret: no secret resolver configured")
		}
		return resolver(key)
	}
}

// json serializa valor para string JSON (util em content templates).
func tplJSON(v any) (string, error) {
	b, err := encodeJSON(v)
	if err != nil {
		return "", fmt.Errorf("json: %w", err)
	}
	return string(b), nil
}

// default retorna fallback se valor for zero/vazio.
// Uso: {{ default .event.limit 50 }}
func tplDefault(fallback, value any) any {
	if value == nil {
		return fallback
	}
	rv := reflect.ValueOf(value)
	if rv.IsZero() {
		return fallback
	}
	return value
}

// navigatePath navega campos aninhados em maps usando notacao de ponto.
// Ex: "fields.priority.name" navega map["fields"]["priority"]["name"]
func navigatePath(data any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	current := data

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]any:
			val, ok := v[part]
			if !ok {
				return nil, false
			}
			current = val
		case map[any]any:
			val, ok := v[part]
			if !ok {
				return nil, false
			}
			current = val
		default:
			return nil, false
		}
	}

	return current, true
}

// CoerceInputs converte valores string para os tipos corretos de acordo com o JSON Schema
// da tool. Necessario porque o TemplateEditor (Monaco) e a resolucao de Go templates
// sempre produzem strings, mas servidores MCP esperam tipos nativos (number, boolean, etc).
func CoerceInputs(inputs map[string]any, schemaJSON json.RawMessage) map[string]any {
	if len(schemaJSON) == 0 || len(inputs) == 0 {
		return inputs
	}

	var schema struct {
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		return inputs
	}

	result := make(map[string]any, len(inputs))
	for k, v := range inputs {
		prop, hasProp := schema.Properties[k]
		if !hasProp {
			result[k] = v
			continue
		}

		str, isString := v.(string)
		if !isString {
			result[k] = v
			continue
		}

		switch prop.Type {
		case "integer":
			if n, err := strconv.ParseInt(str, 10, 64); err == nil {
				result[k] = n
				continue
			}
		case "number":
			if f, err := strconv.ParseFloat(str, 64); err == nil {
				result[k] = f
				continue
			}
		case "boolean":
			if b, err := strconv.ParseBool(str); err == nil {
				result[k] = b
				continue
			}
		case "array":
			var arr []any
			if err := json.Unmarshal([]byte(str), &arr); err == nil {
				result[k] = arr
				continue
			}
		case "object":
			var obj map[string]any
			if err := json.Unmarshal([]byte(str), &obj); err == nil {
				result[k] = obj
				continue
			}
		}

		result[k] = v
	}
	return result
}

// EvaluateCondition renders a Go template condition against the given context
// and returns whether the result is truthy (non-empty and not "false").
// An empty condition string is always truthy (no filtering).
func EvaluateCondition(condition string, ctx *TemplateContext) (bool, error) {
	if condition == "" {
		return true, nil
	}

	result, err := resolveTemplate(condition, ctx)
	if err != nil {
		return false, fmt.Errorf("condition eval: %w", err)
	}

	s := strings.TrimSpace(fmt.Sprintf("%v", result))
	return s != "" && s != "false" && s != "<no value>", nil
}

func encodeJSON(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return b, nil
}
