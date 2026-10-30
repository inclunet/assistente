package llmcatalog

// ResolveModels enriches only IDs actually offered by a connection. Identity is
// the exact server profile, API format and exposed ID; aliases are never parsed.
// Missing entries or capability keys mean unknown, not unsupported.
func (d Document) ResolveModels(serverProfileID, apiFormat string, ids []string, aliases map[string]string) map[string]Model {
	resolved := make(map[string]Model)
	if apiFormat == "" || len(ids) == 0 {
		return resolved
	}
	requested := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		requested[id] = struct{}{}
	}
	canonical := make(map[string]Model, len(d.Models))
	for _, model := range d.Models {
		canonical[model.ID] = model
	}
	for _, exposure := range d.Exposures {
		if _, explicit := aliases[exposure.ExposedModelID]; explicit {
			continue
		}
		if exposure.ServerProfileID != serverProfileID || exposure.APIFormat != apiFormat {
			continue
		}
		if _, ok := requested[exposure.ExposedModelID]; !ok {
			continue
		}
		model, ok := canonical[exposure.CanonicalModelID]
		if !ok {
			continue
		}
		model.Capabilities = make(map[string]string, len(model.Capabilities)+len(exposure.CapabilityOverrides))
		for key, value := range canonical[model.ID].Capabilities {
			model.Capabilities[key] = value
		}
		for key, value := range exposure.CapabilityOverrides {
			model.Capabilities[key] = value
		}
		model.Limits = cloneMap(model.Limits)
		model.Parameters = withKnownOutputLimit(cloneParameters(model.Parameters), model.Limits["max_output_tokens"])
		if len(exposure.ParameterOverrides) > 0 {
			if model.Parameters == nil {
				model.Parameters = make(map[string]Parameter)
			}
			for name, field := range cloneParameters(exposure.ParameterOverrides) {
				model.Parameters[name] = field
			}
		}
		resolved[exposure.ExposedModelID] = model
	}
	for remoteID, canonicalID := range aliases {
		if _, offered := requested[remoteID]; !offered {
			continue
		}
		model, known := canonical[canonicalID]
		if !known {
			continue
		}
		model.Capabilities = cloneMap(model.Capabilities)
		model.Limits = cloneMap(model.Limits)
		model.Parameters = withKnownOutputLimit(cloneParameters(model.Parameters), model.Limits["max_output_tokens"])
		resolved[remoteID] = model
	}

	return resolved
}
