package commandbindings

import "strings"

// BindingPresentation contém somente a apresentação persistida de um binding.
// Ela é deliberadamente separada de Candidate: apresentação não participa da
// identidade, resolução ou execução.
type BindingPresentation struct {
	TitleByLocale map[string]string
	Icon          string
	ImageRef      string
	States        map[string]BindingPresentation
}

var presentationStates = [...]string{"on", "off", "waiting", "running", "succeeded", "failed", "denied", "cancelled", "timed_out", "outcome_unknown"}

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
		snapshot.byBindingID[bindingID] = BindingPresentation{
			TitleByLocale: cloneStringMap(presentation.TitleByLocale),
			Icon:          presentation.Icon,
			ImageRef:      presentation.ImageRef,
			States:        clonePresentationStates(presentation.States),
		}
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
	return BindingPresentation{
		TitleByLocale: cloneStringMap(presentation.TitleByLocale),
		Icon:          presentation.Icon,
		ImageRef:      presentation.ImageRef,
		States:        clonePresentationStates(presentation.States),
	}, true
}

func clonePresentationStates(input map[string]BindingPresentation) map[string]BindingPresentation {
	if input == nil {
		return nil
	}
	output := make(map[string]BindingPresentation, len(input))
	for state, presentation := range input {
		output[state] = BindingPresentation{
			TitleByLocale: cloneStringMap(presentation.TitleByLocale),
			Icon:          presentation.Icon,
			ImageRef:      presentation.ImageRef,
		}
	}
	return output
}

// PresentationForState compõe cada binding com a variante solicitada e só
// devolve campos que concordam em todos os bindings. Campos ausentes herdam
// do binding base; títulos herdam locale por locale, sem fallback de locale.
func (p *PresentationSnapshot) PresentationForState(bindingIDs []string, state string) BindingPresentation {
	if p == nil || len(bindingIDs) == 0 || !validPresentationState(state) {
		return BindingPresentation{}
	}
	effective := make([]BindingPresentation, 0, len(bindingIDs))
	for _, bindingID := range bindingIDs {
		base, ok := p.byBindingID[bindingID]
		if !ok {
			return BindingPresentation{}
		}
		variant, exists := base.States[state]
		if !exists {
			variant = BindingPresentation{}
		}
		if strings.TrimSpace(variant.Icon) == "" {
			variant.Icon = base.Icon
		}
		if strings.TrimSpace(variant.ImageRef) == "" {
			variant.ImageRef = base.ImageRef
		}
		effective = append(effective, variant)
	}

	result := BindingPresentation{}
	locales := make(map[string]struct{})
	for _, bindingID := range bindingIDs {
		base := p.byBindingID[bindingID]
		for locale := range base.TitleByLocale {
			locales[locale] = struct{}{}
		}
		for locale := range base.States[state].TitleByLocale {
			locales[locale] = struct{}{}
		}
	}
	for locale := range locales {
		title, present := effectivePresentationTitle(p, bindingIDs[0], state, locale)
		if !present || strings.TrimSpace(title) == "" {
			continue
		}
		unanimous := true
		for _, bindingID := range bindingIDs[1:] {
			if other, ok := effectivePresentationTitle(p, bindingID, state, locale); !ok || other != title {
				unanimous = false
				break
			}
		}
		if unanimous {
			if result.TitleByLocale == nil {
				result.TitleByLocale = make(map[string]string)
			}
			result.TitleByLocale[locale] = title
		}
	}
	result.Icon = unanimousString(effective, func(p BindingPresentation) string { return p.Icon })
	result.ImageRef = unanimousString(effective, func(p BindingPresentation) string { return p.ImageRef })
	return result
}

func effectivePresentationTitle(p *PresentationSnapshot, bindingID, state, locale string) (string, bool) {
	base, ok := p.byBindingID[bindingID]
	if !ok {
		return "", false
	}
	if variant, ok := base.States[state]; ok {
		if title, ok := variant.TitleByLocale[locale]; ok {
			return title, true
		}
	}
	title, ok := base.TitleByLocale[locale]
	return title, ok
}

func unanimousString(presentations []BindingPresentation, value func(BindingPresentation) string) string {
	want := value(presentations[0])
	if strings.TrimSpace(want) == "" {
		return ""
	}
	for _, presentation := range presentations[1:] {
		if value(presentation) != want {
			return ""
		}
	}
	return want
}

func validPresentationState(state string) bool {
	return IsPresentationState(state)
}

// IsPresentationState reports whether state is part of the versioned variant contract.
func IsPresentationState(state string) bool {
	for _, allowed := range presentationStates {
		if state == allowed {
			return true
		}
	}
	return false
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

// IconForBindings só aceita um ícone quando todos os IDs fornecidos têm a
// mesma apresentação. IDs ausentes, ícones ausentes ou divergentes não
// produzem fallback arbitrário.
func (p *PresentationSnapshot) IconForBindings(bindingIDs []string) string {
	if p == nil || len(bindingIDs) == 0 {
		return ""
	}
	var icon string
	for _, bindingID := range bindingIDs {
		presentation, ok := p.byBindingID[bindingID]
		if !ok || strings.TrimSpace(presentation.Icon) == "" {
			return ""
		}
		if icon == "" {
			icon = presentation.Icon
			continue
		}
		if icon != presentation.Icon {
			return ""
		}
	}
	return icon
}

// ImageForBindings só aceita uma referência de imagem quando todos os IDs
// fornecidos têm a mesma referência não vazia. IDs ausentes, referências
// ausentes ou divergentes não produzem fallback arbitrário.
func (p *PresentationSnapshot) ImageForBindings(bindingIDs []string) string {
	if p == nil || len(bindingIDs) == 0 {
		return ""
	}
	var imageRef string
	for _, bindingID := range bindingIDs {
		presentation, ok := p.byBindingID[bindingID]
		if !ok || strings.TrimSpace(presentation.ImageRef) == "" {
			return ""
		}
		if imageRef == "" {
			imageRef = presentation.ImageRef
			continue
		}
		if imageRef != presentation.ImageRef {
			return ""
		}
	}
	return imageRef
}

// IconForBindings consulta a apresentação somente para os IDs já retornados
// pela resolução. IDs inelegíveis nunca chegam a este contrato.
func (c *Configuration) IconForBindings(bindingIDs []string) string {
	if c == nil {
		return ""
	}
	return c.presentation.IconForBindings(bindingIDs)
}

// ImageForBindings consulta a apresentação somente para os IDs já retornados
// pela resolução. IDs inelegíveis nunca chegam a este contrato.
func (c *Configuration) ImageForBindings(bindingIDs []string) string {
	if c == nil {
		return ""
	}
	return c.presentation.ImageForBindings(bindingIDs)
}

// PresentationForState consulta variantes somente para os IDs resolvidos.
func (c *Configuration) PresentationForState(bindingIDs []string, state string) BindingPresentation {
	if c == nil {
		return BindingPresentation{}
	}
	return c.presentation.PresentationForState(bindingIDs, state)
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
