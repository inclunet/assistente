// Package llmcatalog decodes and validates the public provider/model catalog.
package llmcatalog

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Document is the version 1 catalog envelope. Treat a decoded Document as
// immutable when sharing it: its maps and slices are reference types. Clone
// makes an independent copy for callers that need to mutate a document.
type Document struct {
	SchemaVersion  int        `json:"schema_version"`
	CatalogVersion string     `json:"catalog_version"`
	UpdatedAt      string     `json:"updated_at"`
	Servers        []Server   `json:"servers"`
	Models         []Model    `json:"models"`
	Exposures      []Exposure `json:"exposures"`
}

type Server struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	Group          string                 `json:"group"`
	ProviderFamily string                 `json:"provider_family"`
	Status         string                 `json:"status"`
	ReplacedBy     string                 `json:"replaced_by,omitempty"`
	APIFormat      string                 `json:"api_format"`
	AdapterID      string                 `json:"adapter_id"`
	BaseURL        string                 `json:"base_url,omitempty"`
	DefaultPort    *int                   `json:"default_port,omitempty"`
	Auth           Auth                   `json:"auth"`
	SignupURL      string                 `json:"signup_url,omitempty"`
	APIKeyURL      string                 `json:"api_key_url,omitempty"`
	DefaultModelID string                 `json:"default_model_id,omitempty"`
	Endpoints      map[string]Endpoint    `json:"endpoints,omitempty"`
	Fields         map[string]FieldPolicy `json:"fields"`
}

type Auth struct {
	Mode     string `json:"mode"`
	Required bool   `json:"required"`
}

// UnmarshalJSON preserves the distinction between required:false and a
// missing required member in the catalog contract.
func (a *Auth) UnmarshalJSON(data []byte) error {
	var raw struct {
		Mode     string `json:"mode"`
		Required *bool  `json:"required"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.Required == nil {
		return fmt.Errorf("campo obrigatório auth.required ausente")
	}
	a.Mode = raw.Mode
	a.Required = *raw.Required
	return nil
}

type Endpoint struct {
	Path string `json:"path"`
}

type FieldPolicy struct {
	Policy         string `json:"policy"`
	Default        any    `json:"default,omitempty"`
	Required       bool   `json:"required,omitempty"`
	Choices        []any  `json:"choices,omitempty"`
	defaultPresent bool
}

// UnmarshalJSON keeps JSON values primitive-typed for Wails while retaining
// the difference between a missing default and an explicitly null default.
func (p *FieldPolicy) UnmarshalJSON(data []byte) error {
	var raw struct {
		Policy   string            `json:"policy"`
		Default  json.RawMessage   `json:"default"`
		Required bool              `json:"required"`
		Choices  []json.RawMessage `json:"choices"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	p.Policy = raw.Policy
	p.Required = raw.Required
	p.defaultPresent = len(raw.Default) > 0
	if p.defaultPresent {
		value, err := decodeJSONValue(raw.Default)
		if err != nil {
			return err
		}
		p.Default = value
	} else {
		p.Default = nil
	}
	if raw.Choices != nil {
		p.Choices = make([]any, len(raw.Choices))
		for i, choice := range raw.Choices {
			value, err := decodeJSONValue(choice)
			if err != nil {
				return err
			}
			p.Choices[i] = value
		}
	} else {
		p.Choices = nil
	}
	return nil
}

// MarshalJSON preserves explicit null defaults and omits absent defaults.
func (p FieldPolicy) MarshalJSON() ([]byte, error) {
	value := map[string]any{"policy": p.Policy}
	if p.defaultPresent {
		value["default"] = p.Default
	}
	if p.Required {
		value["required"] = true
	}
	if p.Choices != nil {
		value["choices"] = p.Choices
	}
	return json.Marshal(value)
}

func decodeJSONValue(raw json.RawMessage) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

type Model struct {
	ID           string            `json:"id"`
	Vendor       string            `json:"vendor"`
	Name         string            `json:"name"`
	Status       string            `json:"status"`
	Capabilities map[string]string `json:"capabilities"`
	Limits       map[string]int64  `json:"limits,omitempty"`
}

type Voice struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Language string `json:"language,omitempty"`
}

type VoiceList struct {
	Completeness string  `json:"completeness"`
	Items        []Voice `json:"items"`
}

type Exposure struct {
	ServerProfileID     string            `json:"server_profile_id"`
	APIFormat           string            `json:"api_format"`
	ExposedModelID      string            `json:"exposed_model_id"`
	CanonicalModelID    string            `json:"canonical_model_id"`
	CapabilityOverrides map[string]string `json:"capability_overrides,omitempty"`
	SelectionMode       string            `json:"selection_mode,omitempty"`
	Voices              *VoiceList        `json:"voices,omitempty"`
}

// Clone returns an independent deep copy.
func (d Document) Clone() Document {
	clone := d
	clone.Servers = make([]Server, len(d.Servers))
	for i, server := range d.Servers {
		clone.Servers[i] = server
		if server.DefaultPort != nil {
			port := *server.DefaultPort
			clone.Servers[i].DefaultPort = &port
		}
		clone.Servers[i].Endpoints = cloneMap(server.Endpoints)
		clone.Servers[i].Fields = make(map[string]FieldPolicy, len(server.Fields))
		for key, policy := range server.Fields {
			policy.Choices = append([]any(nil), policy.Choices...)
			clone.Servers[i].Fields[key] = policy
		}
	}
	clone.Models = make([]Model, len(d.Models))
	for i, model := range d.Models {
		clone.Models[i] = model
		clone.Models[i].Capabilities = cloneMap(model.Capabilities)
		clone.Models[i].Limits = cloneMap(model.Limits)
	}
	clone.Exposures = make([]Exposure, len(d.Exposures))
	for i, exposure := range d.Exposures {
		clone.Exposures[i] = exposure
		clone.Exposures[i].CapabilityOverrides = cloneMap(exposure.CapabilityOverrides)
		if exposure.Voices != nil {
			voices := *exposure.Voices
			voices.Items = append([]Voice(nil), exposure.Voices.Items...)
			clone.Exposures[i].Voices = &voices
		}
	}
	return clone
}

func cloneMap[K comparable, V any](source map[K]V) map[K]V {
	if source == nil {
		return nil
	}
	clone := make(map[K]V, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}
