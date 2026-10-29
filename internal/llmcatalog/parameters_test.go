package llmcatalog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeRejectsInvalidParameterContracts(t *testing.T) {
	for _, extra := range []string{
		`"parameters":{"temperature":{"support":"supported","minimum":null}}`,
		`"parameters":{"temperature":{"support":"supported","default":null}}`,
		`"parameters":{"top_p":{"support":"supported","maximum":2}}`,
		`"parameters":{"max_output_tokens":{"support":"supported","default":2147483648}}`,
		`"limits":{"max_output_tokens":128},"parameters":{"max_output_tokens":{"support":"supported","default":256}}`,
		`"limits":{"max_output_tokens":128},"parameters":{"max_output_tokens":{"maximum":64}}`,
	} {
		input := strings.Replace(minimalCatalog, `"capabilities":{}`, `"capabilities":{},`+extra, 1)
		if _, err := Decode([]byte(input)); err == nil {
			t.Errorf("invalid contract accepted: %s", extra)
		}
	}
}

func TestParameterDefaultPreservesZeroAndRejectsInvalidValues(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		valid bool
	}{
		{"zero", `{"temperature":{"support":"supported","minimum":0,"maximum":1,"default":0}}`, true},
		{"outside_range", `{"temperature":{"support":"supported","minimum":0,"maximum":1,"default":2}}`, false},
		{"inverted_bounds", `{"temperature":{"support":"supported","minimum":2,"maximum":1}}`, false},
		{"fractional_tokens", `{"max_output_tokens":{"support":"supported","default":1.5}}`, false},
		{"unknown_default", `{"temperature":{"support":"unknown","default":0}}`, false},
		{"unsupported_default", `{"temperature":{"support":"unsupported","default":0}}`, false},
		{"reasoning_choice", `{"reasoning_effort":{"support":"supported","choices":["low","high"],"default":"low"}}`, true},
		{"reasoning_invalid_default", `{"reasoning_effort":{"support":"supported","choices":["low","high"],"default":"medium"}}`, false},
		{"reasoning_numeric_default", `{"reasoning_effort":{"support":"supported","default":0}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var fields map[string]Parameter
			if err := json.Unmarshal([]byte(tc.input), &fields); err != nil {
				t.Fatal(err)
			}
			if err := validateParameters("parameters", fields); (err == nil) != tc.valid {
				t.Fatalf("valid=%t, error=%v", tc.valid, err)
			}
			if tc.name == "zero" {
				field := fields["temperature"]
				if field.Minimum == nil || *field.Minimum != 0 || field.Default != float64(0) {
					t.Fatalf("lost zero: %+v", field)
				}
			}
		})
	}
}

func TestResolveParameterOverridesReplaceWholeFieldAndDoNotMutateSnapshot(t *testing.T) {
	minimum, maximum := 0.0, 2.0
	doc := Document{
		Models: []Model{{ID: "model", Parameters: map[string]Parameter{
			"temperature":      {Support: "supported", Minimum: &minimum, Maximum: &maximum, Default: 0.7},
			"reasoning_effort": {Support: "supported", Choices: []string{"low", "high"}},
		}}},
		Exposures: []Exposure{{ServerProfileID: "proxy", APIFormat: "openai", ExposedModelID: "alias", CanonicalModelID: "model", ParameterOverrides: map[string]Parameter{
			"temperature": {Support: "unknown"},
		}}},
	}
	model := doc.ResolveModels("proxy", "openai", []string{"alias"})["alias"]
	field := model.Parameters["temperature"]
	if field.Support != "unknown" || field.Minimum != nil || field.Maximum != nil || field.Default != nil {
		t.Fatalf("mixed constraints: %+v", field)
	}
	model.Parameters["reasoning_effort"].Choices[0] = "changed"
	copy := doc.Clone()
	*copy.Models[0].Parameters["temperature"].Minimum = 1
	if *doc.Models[0].Parameters["temperature"].Minimum != 0 || doc.Models[0].Parameters["reasoning_effort"].Choices[0] != "low" {
		t.Fatal("snapshot mutated")
	}
}
