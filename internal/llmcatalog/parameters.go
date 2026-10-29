package llmcatalog

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
)

// Parameter describes a canonical chat field. An absent bound or default is
// unknown; zero is a value, not a sentinel. Exposure overrides replace the
// entire field so constraints from different routes are never mixed.
type Parameter struct {
	Support string   `json:"support"`
	Minimum *float64 `json:"minimum,omitempty"`
	Maximum *float64 `json:"maximum,omitempty"`
	Choices []string `json:"choices,omitempty"`
	Default any      `json:"default,omitempty"`
}

// ParameterValueError is a stable diagnostic translated by the error presenter.
type ParameterValueError struct {
	Field string
	Code  string
	Limit *float64
}

func (e *ParameterValueError) Error() string {
	marker := "parameter_error:v1:" + e.Field + ":" + e.Code
	if e.Limit != nil {
		marker += fmt.Sprintf(":%g", *e.Limit)
	}
	return marker
}

// WithDomain intersects catalog constraints with the parameter's numeric domain.
// The editor receives this same contract; it does not repeat transport bounds.
func (p Parameter) WithDomain(name string) Parameter {
	if p.Support == "unsupported" {
		return p
	}
	var minimum, maximum float64
	switch name {
	case "temperature", "context_window":
		minimum, maximum = 0, math.Inf(1)
	case "top_p":
		minimum, maximum = 0, 1
	case "max_output_tokens":
		minimum, maximum = 1, math.MaxInt32
	default:
		return p
	}
	if p.Minimum == nil || *p.Minimum < minimum {
		p.Minimum = &minimum
	}
	if !math.IsInf(maximum, 1) && (p.Maximum == nil || *p.Maximum > maximum) {
		p.Maximum = &maximum
	}
	return p
}

// NumericDefault returns a copy of a validated numeric default.
func (p Parameter) NumericDefault() *float64 {
	var value float64
	switch number := p.Default.(type) {
	case float64:
		value = number
	case json.Number:
		parsed, err := number.Float64()
		if err != nil {
			return nil
		}
		value = parsed
	default:
		return nil
	}
	return &value
}

func cloneParameters(source map[string]Parameter) map[string]Parameter {
	result := cloneMap(source)
	for name, field := range result {
		if field.Minimum != nil {
			value := *field.Minimum
			field.Minimum = &value
		}
		if field.Maximum != nil {
			value := *field.Maximum
			field.Maximum = &value
		}
		field.Choices = cloneSlice(field.Choices)
		result[name] = field
	}
	return result
}

// Callers own the map. A declared default does not erase a known output limit.
func withKnownOutputLimit(fields map[string]Parameter, limit int64) map[string]Parameter {
	field, declared := fields["max_output_tokens"]
	if limit <= 0 || field.Support == "unsupported" || (field.Maximum != nil && *field.Maximum <= float64(limit)) {
		return fields
	}
	if fields == nil {
		fields = make(map[string]Parameter)
	}
	if !declared {
		field.Support = "unknown"
	}
	maximum := float64(limit)
	field.Maximum = &maximum
	fields["max_output_tokens"] = field
	return fields
}

func validateParameters(path string, fields map[string]Parameter) error {
	for name, field := range fields {
		fieldPath := path + "." + name
		if !oneOf(name, "temperature", "top_p", "max_output_tokens", "reasoning_effort") {
			return invalid(fieldPath, "parâmetro de chat desconhecido")
		}
		if !oneOf(field.Support, "supported", "unsupported", "unknown") {
			return invalid(fieldPath+".support", "estado inválido")
		}
		if field.Support == "unsupported" && (field.Minimum != nil || field.Maximum != nil || field.Choices != nil || field.Default != nil) {
			return invalid(fieldPath, "campo não suportado não aceita restrições ou default")
		}
		if field.Support == "unknown" && field.Default != nil {
			return invalid(fieldPath, "default exige suporte conhecido")
		}
		for _, bound := range []*float64{field.Minimum, field.Maximum} {
			if bound != nil && (math.IsNaN(*bound) || math.IsInf(*bound, 0)) {
				return invalid(fieldPath, "limites devem ser finitos")
			}
			if name == "max_output_tokens" && bound != nil && (math.Trunc(*bound) != *bound || *bound < 1 || *bound > math.MaxInt32) {
				return invalid(fieldPath, "limite de tokens não representável pelo transporte")
			}
			if bound != nil && name != "reasoning_effort" {
				if err := field.ValidateValue(name, *bound); err != nil {
					return invalid(fieldPath, err.Error())
				}
			}
		}
		if field.Minimum != nil && field.Maximum != nil && *field.Minimum > *field.Maximum {
			return invalid(fieldPath, "mínimo maior que máximo")
		}
		if name == "reasoning_effort" {
			if field.Minimum != nil || field.Maximum != nil {
				return invalid(fieldPath, "parâmetro textual não aceita limites numéricos")
			}
			if field.Choices != nil && len(field.Choices) == 0 {
				return invalid(fieldPath, "opções não podem ser vazias")
			}
			seen := make(map[string]bool, len(field.Choices))
			for _, choice := range field.Choices {
				if !lengthBetween(choice, 1, 64) || seen[choice] {
					return invalid(fieldPath, "opção inválida ou repetida")
				}
				seen[choice] = true
			}
		} else if field.Choices != nil {
			return invalid(fieldPath, "parâmetro numérico não aceita opções textuais")
		}
		if field.Default != nil {
			if err := field.ValidateValue(name, field.Default); err != nil {
				return invalid(fieldPath+".default", err.Error())
			}
		}
	}
	return nil
}

// ValidateValue checks an explicit value without changing it. Only declared
// constraints apply; unsupported fields are omitted by the caller.
func (p Parameter) ValidateValue(name string, value any) error {
	if p.Support == "unsupported" {
		return nil
	}
	if name == "reasoning_effort" {
		text, ok := value.(string)
		if !ok || !lengthBetween(text, 0, 64) || (p.Choices != nil && !slices.Contains(p.Choices, text)) {
			return &ParameterValueError{Field: name, Code: "choice"}
		}
		return nil
	}
	var number float64
	switch typed := value.(type) {
	case float64:
		number = typed
	case int:
		number = float64(typed)
	case json.Number:
		var err error
		number, err = typed.Float64()
		if err != nil {
			return &ParameterValueError{Field: name, Code: "number"}
		}
	default:
		return &ParameterValueError{Field: name, Code: "number"}
	}
	if math.IsNaN(number) || math.IsInf(number, 0) || ((name == "max_output_tokens" || name == "context_window") && math.Trunc(number) != number) {
		return &ParameterValueError{Field: name, Code: "number"}
	}
	p = p.WithDomain(name)
	if p.Minimum != nil && number < *p.Minimum {
		return &ParameterValueError{Field: name, Code: "minimum", Limit: p.Minimum}
	}
	if p.Maximum != nil && number > *p.Maximum {
		return &ParameterValueError{Field: name, Code: "maximum", Limit: p.Maximum}
	}
	return nil
}
