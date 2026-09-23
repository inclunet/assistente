package commandbindings

import "strings"

// BindingPresentation contém somente a apresentação textual persistida de um
// binding. Ela é deliberadamente separada de Candidate: apresentação não
// participa da identidade, resolução ou execução.
type BindingPresentation struct {
	TitleByLocale map[string]string
}

// PresentationSnapshot é uma projeção imutável de apresentações por ID de
// binding materializado. O mapa interno nunca é exposto.
type PresentationSnapshot struct {
	byBindingID map[string]BindingPresentation
}

// NewPresentationSnapshot copia profundamente a entrada. O chamador pode
// reutilizar ou modificar seus mapas depois sem alterar o snapshot.
func NewPresentationSnapshot(entries map[string]BindingPresentation) *PresentationSnapshot {
	snapshot := &PresentationSnapshot{byBindingID: make(map[string]BindingPresentation, len(entries))}
	for bindingID, presentation := range entries {
		if strings.TrimSpace(bindingID) == "" {
			continue
		}
		snapshot.byBindingID[bindingID] = BindingPresentation{TitleByLocale: cloneStringMap(presentation.TitleByLocale)}
	}
	return snapshot
}

func (p *PresentationSnapshot) clone() *PresentationSnapshot {
	if p == nil {
		return nil
	}
	return NewPresentationSnapshot(p.byBindingID)
}

// Binding devolve uma cópia profunda da apresentação do binding.
func (p *PresentationSnapshot) Binding(bindingID string) (BindingPresentation, bool) {
	if p == nil {
		return BindingPresentation{}, false
	}
	presentation, ok := p.byBindingID[bindingID]
	if !ok {
		return BindingPresentation{}, false
	}
	return BindingPresentation{TitleByLocale: cloneStringMap(presentation.TitleByLocale)}, true
}

// TitleForBindings só aceita um título quando todos os IDs fornecidos têm a
// mesma apresentação textual para o locale exato. Isso evita escolher uma
// apresentação arbitrária em empate e não faz fallback entre locales.
func (p *PresentationSnapshot) TitleForBindings(bindingIDs []string, locale string) (string, bool) {
	if p == nil || len(bindingIDs) == 0 || strings.TrimSpace(locale) == "" {
		return "", false
	}
	var title string
	for _, bindingID := range bindingIDs {
		presentation, ok := p.byBindingID[bindingID]
		if !ok {
			return "", false
		}
		candidate, ok := presentation.TitleByLocale[locale]
		if !ok || strings.TrimSpace(candidate) == "" {
			return "", false
		}
		if title == "" {
			title = candidate
			continue
		}
		if title != candidate {
			return "", false
		}
	}
	return title, title != ""
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
