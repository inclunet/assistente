package commandbindings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"assistente/internal/commandjson"
)

// LayerProvenance é a prova estrutural associada a uma referência de camada.
// Provenance nil significa ausência de prova; não é convertido em JSON vazio.
type LayerProvenance struct {
	SourceID   string
	Provenance json.RawMessage
}

// WithLayerProvenance publica um snapshot independente da configuração atual.
// Registros idênticos são deduplicados, mas conflitos do mesmo SourceID são
// preservados para que o consumidor possa recusá-los.
func (c *Configuration) WithLayerProvenance(input map[string][]LayerProvenance) (*Configuration, error) {
	if c == nil {
		return nil, fmt.Errorf("configuração ausente")
	}
	clone := *c
	if len(input) == 0 {
		clone.layerProvenance = nil
		return &clone, nil
	}
	clone.layerProvenance = make(map[string][]LayerProvenance, len(input))
	for ref, entries := range input {
		if !validLayerProvenanceRef(ref) {
			return nil, fmt.Errorf("referência de camada inválida: %q", ref)
		}
		normalized := make([]LayerProvenance, 0, len(entries))
		for _, entry := range entries {
			if !validLayerProvenanceRef(entry.SourceID) {
				return nil, fmt.Errorf("sourceID de proveniência inválido: %q", entry.SourceID)
			}
			canonical := json.RawMessage(nil)
			if entry.Provenance != nil {
				var err error
				canonical, err = commandjson.Canonicalize(entry.Provenance)
				if err != nil {
					return nil, fmt.Errorf("proveniência JSON inválida para %q", entry.SourceID)
				}
			}
			copyEntry := LayerProvenance{SourceID: entry.SourceID}
			if canonical != nil {
				copyEntry.Provenance = append(json.RawMessage(nil), canonical...)
			}
			if !containsExactLayerProvenance(normalized, copyEntry) {
				normalized = append(normalized, copyEntry)
			}
		}
		sortLayerProvenance(normalized)
		clone.layerProvenance[ref] = normalized
	}
	return &clone, nil
}

// LayerProvenance retorna somente as provas das referências pedidas, em ordem
// determinística, com todos os conflitos preservados e sem aliases mutáveis.
func (c *Configuration) LayerProvenance(refs []string) []LayerProvenance {
	if c == nil || len(refs) == 0 {
		return nil
	}
	selected := make([]LayerProvenance, 0)
	seenRefs := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if _, seen := seenRefs[ref]; seen {
			continue
		}
		seenRefs[ref] = struct{}{}
		for _, entry := range c.layerProvenance[ref] {
			if !containsExactLayerProvenance(selected, entry) {
				copyEntry := LayerProvenance{SourceID: entry.SourceID}
				if entry.Provenance != nil {
					copyEntry.Provenance = append(json.RawMessage(nil), entry.Provenance...)
				}
				selected = append(selected, copyEntry)
			}
		}
	}
	if len(selected) == 0 {
		return nil
	}
	sortLayerProvenance(selected)
	return selected
}

func validLayerProvenanceRef(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && !strings.ContainsRune(value, '\x00')
}

func containsExactLayerProvenance(entries []LayerProvenance, candidate LayerProvenance) bool {
	for _, entry := range entries {
		if entry.SourceID == candidate.SourceID && bytes.Equal(entry.Provenance, candidate.Provenance) {
			return true
		}
	}
	return false
}

func sortLayerProvenance(entries []LayerProvenance) {
	slices.SortStableFunc(entries, func(a, b LayerProvenance) int {
		if result := strings.Compare(a.SourceID, b.SourceID); result != 0 {
			return result
		}
		return bytes.Compare(a.Provenance, b.Provenance)
	})
}
