package commandcatalog

import (
	"slices"
	"strings"
)

var supportedLocales = map[string]struct{}{"pt-BR": {}, "en": {}, "es": {}}

// Search encontra comandos pela metadata do locale solicitado. A ordem é
// sempre a ordem canônica dos IDs, independentemente da ordem de registro.
func (r *Registry) Search(locale, query string) []Definition {
	if r == nil {
		return []Definition{}
	}
	if _, ok := supportedLocales[locale]; !ok {
		return []Definition{}
	}
	query = normalize(query)
	if query == "" {
		return []Definition{}
	}
	ids := make([]string, 0, len(r.definitions))
	for id := range r.definitions {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	result := make([]Definition, 0)
	for _, id := range ids {
		d := r.definitions[id]
		if d.Presentation == nil {
			continue
		}
		metadata := d.Presentation.Locales[locale]
		fields := []string{d.ID, metadata.Name, metadata.Description, metadata.Category}
		matched := false
		for _, field := range fields {
			if strings.Contains(normalize(field), query) {
				matched = true
				break
			}
		}
		if !matched {
			for _, alias := range metadata.Aliases {
				if strings.Contains(normalize(alias), query) {
					matched = true
					break
				}
			}
		}
		if matched {
			result = append(result, clone(d))
		}
	}
	return result
}

func normalize(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}
