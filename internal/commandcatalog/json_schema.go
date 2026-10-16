package commandcatalog

import "slices"

// JSONSchema projeta o contrato tipado para descoberta nas bordas de chat/CLI.
// A validação autoritativa continua no registry, não no schema apresentado.
func JSONSchema(s *Schema) map[string]any {
	if s == nil {
		return nil
	}
	out := map[string]any{"type": string(s.Type)}
	if s.Nullable && s.Type != SchemaNull {
		out["type"] = []string{string(s.Type), "null"}
	}
	if len(s.Enum) > 0 {
		out["enum"] = cloneSchema(s).Enum
	}
	if s.Type == SchemaObject {
		properties := make(map[string]any, len(s.Properties))
		required := make([]string, 0)
		for name, child := range s.Properties {
			properties[name] = JSONSchema(&child)
			if s.Required != nil && slices.Contains(s.Required, name) || s.Required == nil && !child.Optional {
				required = append(required, name)
			}
		}
		slices.Sort(required)
		out["properties"], out["additionalProperties"], out["required"] = properties, false, required
	}
	if s.Items != nil {
		out["items"] = JSONSchema(s.Items)
	}
	if s.Minimum != nil {
		out["minimum"] = *s.Minimum
	}
	if s.Maximum != nil {
		out["maximum"] = *s.Maximum
	}
	if s.MinLength != nil {
		out["minLength"] = *s.MinLength
	}
	if s.MaxLength != nil {
		out["maxLength"] = *s.MaxLength
	}
	if s.MinItems != nil {
		out["minItems"] = *s.MinItems
	}
	if s.MaxItems != nil {
		out["maxItems"] = *s.MaxItems
	}
	return out
}
