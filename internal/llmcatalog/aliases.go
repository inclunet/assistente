package llmcatalog

import "strings"

// ValidateAliases checks identifiers without normalizing their wire values.
func ValidateAliases(aliases map[string]string) error {
	for remoteID, canonicalID := range aliases {
		if strings.TrimSpace(remoteID) == "" || strings.TrimSpace(canonicalID) == "" {
			return invalid("model_aliases", "IDs de modelo não podem ser vazios")
		}
	}
	return nil
}

// ValidateAliasChanges accepts missing targets only for unchanged saved bindings.
// Catalog availability must never prevent removing a binding or editing a name.
func (d Document) ValidateAliasChanges(aliases, previous map[string]string) error {
	if err := ValidateAliases(aliases); err != nil {
		return err
	}
	known := make(map[string]bool, len(d.Models))
	for _, model := range d.Models {
		known[model.ID] = true
	}
	for remoteID, canonicalID := range aliases {
		if !known[canonicalID] && previous[remoteID] != canonicalID {
			return invalid("model_aliases", "modelo canônico ausente do catálogo: "+canonicalID)
		}
	}
	return nil
}
