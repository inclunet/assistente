package llmcatalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Decode parses a JSON v1 catalog and validates its structure and cross-record
// references before returning it. Unknown additive properties are ignored, in
// keeping with the schema's additionalProperties contract.
func Decode(data []byte) (Document, error) {
	var doc Document
	// The public schema permits additive properties throughout the document.
	if err := rejectNullProperties(data); err != nil {
		return Document{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&doc); err != nil {
		return Document{}, fmt.Errorf("decodificar catálogo: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return Document{}, fmt.Errorf("decodificar catálogo: há conteúdo após o JSON")
	} else if !errors.Is(err, io.EOF) {
		return Document{}, fmt.Errorf("decodificar catálogo: conteúdo final inválido: %w", err)
	}
	if err := Validate(doc); err != nil {
		return Document{}, err
	}
	return doc, nil
}

// rejectNullProperties preserves JSON Schema's non-null types for optional
// members, which encoding/json otherwise maps to the same zero value as an
// absent property. FieldPolicy.default is intentionally nullable.
func rejectNullProperties(data []byte) error {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil || root == nil {
		return nil // Decode reports malformed or non-object roots with context.
	}
	if err := rejectNullMembers(root, "$", "schema_version", "catalog_version", "updated_at", "servers", "models", "exposures"); err != nil {
		return err
	}
	if err := rejectNullArrayObjects(root["servers"], "servers", func(index int, server map[string]json.RawMessage) error {
		path := fmt.Sprintf("servers[%d]", index)
		if err := rejectNullMembers(server, path, "id", "name", "group", "provider_family", "status", "replaced_by", "api_format", "adapter_id", "base_url", "default_port", "auth", "signup_url", "api_key_url", "default_model_id", "endpoints", "fields"); err != nil {
			return err
		}
		if auth, ok := rawObject(server["auth"]); ok {
			if err := rejectNullMembers(auth, path+".auth", "mode", "required"); err != nil {
				return err
			}
		}
		if endpoints, ok := rawObject(server["endpoints"]); ok {
			for name, rawEndpoint := range endpoints {
				if endpoint, ok := rawObject(rawEndpoint); ok {
					if err := rejectNullMembers(endpoint, path+".endpoints."+name, "path"); err != nil {
						return err
					}
				}
			}
		}
		if fields, ok := rawObject(server["fields"]); ok {
			for name, rawPolicy := range fields {
				if policy, ok := rawObject(rawPolicy); ok {
					if err := rejectNullMembers(policy, path+".fields."+name, "policy", "required", "choices"); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := rejectNullArrayObjects(root["models"], "models", func(index int, model map[string]json.RawMessage) error {
		path := fmt.Sprintf("models[%d]", index)
		if err := rejectNullMembers(model, path, "id", "vendor", "name", "status", "capabilities", "limits", "parameters"); err != nil {
			return err
		}
		return rejectNullParameters(model["parameters"], path+".parameters")
	}); err != nil {
		return err
	}
	return rejectNullArrayObjects(root["exposures"], "exposures", func(index int, exposure map[string]json.RawMessage) error {
		path := fmt.Sprintf("exposures[%d]", index)
		if err := rejectNullMembers(exposure, path, "server_profile_id", "api_format", "exposed_model_id", "canonical_model_id", "capability_overrides", "parameter_overrides", "selection_mode", "voices"); err != nil {
			return err
		}
		if err := rejectNullParameters(exposure["parameter_overrides"], path+".parameter_overrides"); err != nil {
			return err
		}
		if voices, ok := rawObject(exposure["voices"]); ok {
			if err := rejectNullMembers(voices, path+".voices", "completeness", "items"); err != nil {
				return err
			}
			if err := rejectNullArrayObjects(voices["items"], path+".voices.items", func(index int, voice map[string]json.RawMessage) error {
				return rejectNullMembers(voice, fmt.Sprintf("%s.voices.items[%d]", path, index), "id", "name", "language")
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func rejectNullParameters(raw json.RawMessage, path string) error {
	fields, _ := rawObject(raw)
	for name, fieldRaw := range fields {
		if field, ok := rawObject(fieldRaw); ok {
			if err := rejectNullMembers(field, path+"."+name, "support", "minimum", "maximum", "choices", "default"); err != nil {
				return err
			}
		}
	}
	return nil
}

func rejectNullMembers(object map[string]json.RawMessage, path string, names ...string) error {
	for _, name := range names {
		if raw, exists := object[name]; exists && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("decodificar catálogo: %s.%s não pode ser null", path, name)
		}
	}
	return nil
}

func rejectNullArrayObjects(raw json.RawMessage, path string, visit func(int, map[string]json.RawMessage) error) error {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil // Typed decoding reports a structural mismatch.
	}
	for index, item := range items {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(item, &object); err != nil || object == nil {
			continue // Typed decoding reports a structural mismatch.
		}
		if err := visit(index, object); err != nil {
			return err
		}
	}
	return nil
}

func rawObject(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	var object map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &object) != nil || object == nil {
		return nil, false
	}
	return object, true
}
