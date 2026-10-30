package llmcatalog

import (
	"net/url"
	"strings"
)

// AliasError crosses the bridge as a stable marker, like ParameterValueError.
type AliasError struct {
	Code    string
	ModelID string
}

func (e *AliasError) Error() string {
	return "model_alias_error:v1:" + e.Code + ":" + url.PathEscape(e.ModelID)
}

// ValidateAliases checks identifiers without normalizing their wire values.
func ValidateAliases(aliases map[string]string) error {
	for remoteID, canonicalID := range aliases {
		if strings.TrimSpace(remoteID) == "" || strings.TrimSpace(canonicalID) == "" {
			return &AliasError{Code: "empty_id"}
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
			return &AliasError{Code: "missing_target", ModelID: canonicalID}
		}
	}
	return nil
}
