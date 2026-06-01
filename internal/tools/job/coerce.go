package job

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// jsonKind descreve o tipo JSON esperado para um campo tipado das tools de job.
// É usado pela tolerância defensiva (coerceArgs) para "desembrulhar" valores que
// alguns LLMs em tool-calling enviam stringificados (ex.: "true", "1000",
// "{...}") em vez do literal correto.
type jsonKind int

const (
	kindObject jsonKind = iota
	kindArray
	kindBool
	// kindInteger cobre os campos numéricos das tools de job, que sem exceção
	// fazem unmarshal em `int`/`*int` (limit, offset, max_runs_per_hour). Aceitar
	// floats/notação científica aqui apenas adiaria a falha para o unmarshal
	// estrito com um erro cru — então a coerção exige um inteiro JSON.
	kindInteger
)

// jobTypedFields lista os campos da tool `job` cujo tipo NÃO é string. Campos
// legitimamente string (job_id, run_id, name, description, datas, etc.) são
// deliberadamente omitidos para que nunca sejam desembrulhados por engano.
var jobTypedFields = map[string]jsonKind{
	"delete":            kindBool,
	"run":               kindBool,
	"dry_run":           kindBool,
	"list_runs":         kindBool,
	"list_events":       kindBool,
	"include_dry_run":   kindBool,
	"enabled":           kindBool,
	"limit":             kindInteger,
	"offset":            kindInteger,
	"max_runs_per_hour": kindInteger,
	"status":            kindArray,
	"tags":              kindArray,
	"triggers":          kindArray,
	"inputs":            kindObject,
	"output":            kindObject,
	"events":            kindObject,
	"error_policy":      kindObject,
	"dry_run_config":    kindObject,
}

// pipelineTypedFields lista os campos não-string da tool `job_pipeline`.
var pipelineTypedFields = map[string]jsonKind{
	"delete":   kindBool,
	"enabled":  kindBool,
	"metadata": kindObject,
}

// coerceArgs normaliza, de forma defensiva, um payload de argumentos antes do
// json.Unmarshal estrito. Para cada campo tipado (object/array/bool/number) que
// chegue como uma string JSON, tenta interpretar o conteúdo da string como o
// tipo esperado e o substitui pelo literal correspondente. Campos string
// legítimos nunca são tocados (não estão no mapa de tipos).
//
// Se um campo chega stringificado mas o conteúdo não é um valor válido para o
// tipo esperado, retorna um erro EXPLICATIVO (com o tipo esperado e um exemplo)
// em vez de deixar o encoding/json emitir um erro cru e pouco acionável.
func coerceArgs(raw json.RawMessage, typed map[string]jsonKind) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "{}" {
		return raw, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		// Não é um objeto JSON (ou é inválido): deixa o Unmarshal estrito reportar.
		return raw, nil
	}
	changed := false
	for name, kind := range typed {
		value, ok := fields[name]
		if !ok {
			continue
		}
		// Só agimos quando o valor chegou como uma string JSON literal.
		if !isJSONStringLiteral(value) {
			continue
		}
		var inner string
		if err := json.Unmarshal(value, &inner); err != nil {
			// Improvável dado isJSONStringLiteral; deixa o estrito reportar.
			continue
		}
		coerced, err := coerceStringValue(name, inner, kind)
		if err != nil {
			return nil, err
		}
		fields[name] = coerced
		changed = true
	}
	if !changed {
		return raw, nil
	}
	out, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(out), nil
}

// coerceStringValue tenta converter o conteúdo `inner` de uma string para o
// literal JSON do tipo esperado, retornando erro explicativo em caso de falha.
func coerceStringValue(field, inner string, kind jsonKind) (json.RawMessage, error) {
	value := strings.TrimSpace(inner)
	switch kind {
	case kindBool:
		switch strings.ToLower(value) {
		case "true":
			return json.RawMessage("true"), nil
		case "false":
			return json.RawMessage("false"), nil
		}
	case kindInteger:
		if isJSONInteger(value) {
			return json.RawMessage(value), nil
		}
	case kindObject:
		if isJSONObject(value) {
			return json.RawMessage(value), nil
		}
	case kindArray:
		if isJSONArray(value) {
			return json.RawMessage(value), nil
		}
	}
	return nil, coercionError(field, kind, inner)
}

func coercionError(field string, kind jsonKind, got string) error {
	return fmt.Errorf(
		"field %q must be %s, not a string; received %q which is not valid for that type. Example of a correct value: %s",
		field, kind.label(), got, kind.example(field),
	)
}

func (k jsonKind) label() string {
	switch k {
	case kindObject:
		return "a JSON object"
	case kindArray:
		return "a JSON array"
	case kindBool:
		return "a boolean"
	case kindInteger:
		return "an integer"
	default:
		return "a value"
	}
}

func (k jsonKind) example(field string) string {
	switch k {
	case kindObject:
		return fmt.Sprintf(`%q: {"key": "value"}`, field)
	case kindArray:
		return fmt.Sprintf(`%q: ["value"]`, field)
	case kindBool:
		return fmt.Sprintf(`%q: true`, field)
	case kindInteger:
		return fmt.Sprintf(`%q: 1000`, field)
	default:
		return ""
	}
}

func isJSONStringLiteral(raw json.RawMessage) bool {
	t := strings.TrimSpace(string(raw))
	return len(t) > 0 && t[0] == '"'
}

func isJSONObject(s string) bool {
	t := strings.TrimSpace(s)
	return len(t) > 0 && t[0] == '{' && json.Valid([]byte(t))
}

func isJSONArray(s string) bool {
	t := strings.TrimSpace(s)
	return len(t) > 0 && t[0] == '[' && json.Valid([]byte(t))
}

// isJSONInteger só aceita inteiros em base 10 (com sinal opcional), recusando
// floats e notação científica ("1.5", "1e3"). Os campos numéricos das tools de
// job fazem unmarshal em `int`; aceitar um float aqui apenas trocaria o erro cru
// do encoding/json por outro mais adiante, derrotando a tolerância defensiva.
func isJSONInteger(s string) bool {
	t := strings.TrimSpace(s)
	if t == "" {
		return false
	}
	if _, err := strconv.ParseInt(t, 10, 64); err != nil {
		return false
	}
	// Garante que o literal também é um número JSON válido (sem zeros à esquerda,
	// "+", espaços internos, etc. que ParseInt toleraria mas o JSON não).
	return json.Valid([]byte(t))
}
