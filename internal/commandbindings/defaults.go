package commandbindings

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Default é um binding versionado fornecido pelo aplicativo. Fingerprint é
// produzido pelo futuro catálogo canônico (D6), não calculado a partir desta
// projeção parcial de Candidate. Uma instância de Configuration pertence a um
// único escopo autenticado, previamente resolvido pelo chamador.
type Default struct {
	Candidate   Candidate
	Version     string
	Fingerprint string
	Invariant   bool
}

type DeltaEffect string

const (
	Execute  DeltaEffect = "execute"
	Suppress DeltaEffect = "suppress"
)

type ReviewStatus string

const (
	Active      ReviewStatus = "active"
	NeedsReview ReviewStatus = "needs_review"
)

// Delta representa uma personalização do mesmo acionador de um default.
// Condition adiciona restrições à condição base; não pode contradizê-la.
// O alvo, escopo e identidade de diálogo são herdados do default. Remapeamento
// para outro acionador ainda não é suportado por esta API.
type Delta struct {
	ID                 string
	DefaultID          string
	DefaultVersion     string
	DefaultFingerprint string
	Trigger            string
	Effect             DeltaEffect
	CommandID          string
	ArgumentsKey       string
	Condition          Facts
	Enabled            bool
	LayerActive        bool
	ReviewStatus       ReviewStatus
	LayerPriority      int
	BindingPriority    int
}

// Adjustment comunica ao repository futuro o avanço de versão ou uma pendência.
// NewConfiguration não altera a entrada nem persiste nada.
type Adjustment struct {
	DeltaID        string
	ReviewStatus   ReviewStatus
	DefaultVersion string
	Reason         string
}

const (
	Suppressed     Status = "suppressed"
	ReviewRequired Status = "needs_review"
)

// Configuration contém defaults, personalizações e bindings novos, indexados.
// Supressão/recusa aqui são apenas decisões puras. O futuro executor precisa
// revalidar contexto e reservar o ledger antes de consumir o acionamento (D4).
type Configuration struct {
	defaults    map[string]Default
	deltas      map[string][]Delta
	byTrigger   map[string][]string
	custom      *Resolver
	adjustments []Adjustment
}

func NewConfiguration(defaults []Default, deltas []Delta, custom []Candidate) (*Configuration, error) {
	r, err := New(custom)
	if err != nil {
		return nil, err
	}
	c := &Configuration{defaults: map[string]Default{}, deltas: map[string][]Delta{}, byTrigger: map[string][]string{}, custom: r}
	ids := make(map[string]bool)
	for _, candidate := range custom {
		ids[candidate.ID] = true
	}
	for _, d := range defaults {
		if _, err := New([]Candidate{d.Candidate}); err != nil {
			return nil, err
		}
		if strings.TrimSpace(d.Version) == "" || strings.TrimSpace(d.Fingerprint) == "" {
			return nil, fmt.Errorf("default exige versão e fingerprint")
		}
		if ids[d.Candidate.ID] {
			return nil, fmt.Errorf("binding duplicado: %s", d.Candidate.ID)
		}
		ids[d.Candidate.ID] = true
		d.Candidate.Condition = maps.Clone(d.Candidate.Condition)
		c.defaults[d.Candidate.ID] = d
		c.byTrigger[d.Candidate.Trigger] = append(c.byTrigger[d.Candidate.Trigger], d.Candidate.ID)
	}
	for _, delta := range deltas {
		if strings.TrimSpace(delta.ID) == "" || ids[delta.ID] {
			return nil, fmt.Errorf("ID de delta ausente ou duplicado")
		}
		ids[delta.ID] = true
		if strings.TrimSpace(delta.DefaultID) == "" || strings.TrimSpace(delta.DefaultVersion) == "" || strings.TrimSpace(delta.DefaultFingerprint) == "" || strings.TrimSpace(delta.Trigger) == "" {
			return nil, fmt.Errorf("delta exige referência completa ao default e acionador")
		}
		if delta.ReviewStatus != Active && delta.ReviewStatus != NeedsReview {
			return nil, fmt.Errorf("review_status inválido")
		}
		if delta.Effect != Execute && delta.Effect != Suppress {
			return nil, fmt.Errorf("efeito de delta inválido")
		}
		if delta.Effect == Suppress {
			if delta.CommandID != "" || delta.ArgumentsKey != "" {
				return nil, fmt.Errorf("supressão não pode declarar comando ou argumentos")
			}
		} else if strings.TrimSpace(delta.CommandID) == "" || strings.TrimSpace(delta.ArgumentsKey) == "" {
			return nil, fmt.Errorf("override exige comando e argumentos normalizados")
		}
		if err := delta.Condition.validate(); err != nil {
			return nil, err
		}
		delta.Condition = maps.Clone(delta.Condition)
		base, exists := c.defaults[delta.DefaultID]
		if exists && base.Invariant {
			return nil, fmt.Errorf("default invariante não admite delta: %s", delta.DefaultID)
		}
		reason := ""
		switch {
		case !exists:
			reason = "missing_default"
		case base.Fingerprint != delta.DefaultFingerprint:
			reason = "changed_default"
		case base.Candidate.Trigger != delta.Trigger:
			// Mesmo fingerprint com acionador distinto viola o contrato do
			// produtor confiável; não reinterpretar como remapeamento.
			return nil, fmt.Errorf("acionador diverge do default sem mudança de fingerprint")
		case delta.ReviewStatus == NeedsReview:
			if _, err := withDelta(base.Candidate, delta); err != nil {
				return nil, err
			}
			reason = "pending_review"
		default:
			if _, err := withDelta(base.Candidate, delta); err != nil {
				return nil, err
			}
			if base.Version != delta.DefaultVersion {
				c.adjustments = append(c.adjustments, Adjustment{delta.ID, Active, base.Version, "version_advanced"})
				delta.DefaultVersion = base.Version
			}
		}
		if reason != "" {
			delta.ReviewStatus = NeedsReview
			c.adjustments = append(c.adjustments, Adjustment{delta.ID, NeedsReview, delta.DefaultVersion, reason})
		}
		c.deltas[delta.DefaultID] = append(c.deltas[delta.DefaultID], delta)
		// Uma referência órfã ou default cujo acionador mudou deve bloquear
		// também o acionador antigo, sem permitir fallback silencioso.
		if !exists || base.Candidate.Trigger != delta.Trigger {
			if !slices.Contains(c.byTrigger[delta.Trigger], delta.DefaultID) {
				c.byTrigger[delta.Trigger] = append(c.byTrigger[delta.Trigger], delta.DefaultID)
			}
		}
	}
	slices.SortFunc(c.adjustments, func(a, b Adjustment) int { return strings.Compare(a.DeltaID, b.DeltaID) })
	return c, nil
}

func (c *Configuration) Adjustments() []Adjustment { return slices.Clone(c.adjustments) }

func withDelta(base Candidate, delta Delta) (Candidate, error) {
	condition := maps.Clone(base.Condition)
	if condition == nil {
		condition = Facts{}
	}
	for key, value := range delta.Condition {
		if previous, exists := condition[key]; exists && previous != value {
			return Candidate{}, fmt.Errorf("delta contradiz condição base: %s", key)
		}
		condition[key] = value
	}
	base.ID, base.Condition = delta.ID, condition
	base.LayerPriority, base.BindingPriority = delta.LayerPriority, delta.BindingPriority
	if delta.Effect == Execute {
		base.CommandID, base.ArgumentsKey = delta.CommandID, delta.ArgumentsKey
	}
	// Enabled/LayerActive são compostos com os do default, nunca ampliados.
	base.Enabled = base.Enabled && delta.Enabled
	base.LayerActive = base.LayerActive && delta.LayerActive
	if _, err := New([]Candidate{base}); err != nil {
		return Candidate{}, err
	}
	return base, nil
}

func (c *Configuration) Resolve(trigger string, facts Facts, dialog *DialogScope) (Result, error) {
	if err := facts.validate(); err != nil {
		return Result{}, err
	}
	if dialog != nil && strings.TrimSpace(dialog.ID) == "" {
		return Result{}, fmt.Errorf("diálogo topmost exige identidade")
	}
	candidates := slices.Clone(c.custom.byTrigger[trigger])
	suppressed := []string{}
	review := []string{}
	for _, id := range c.byTrigger[trigger] {
		base, exists := c.defaults[id]
		removed := false
		for _, delta := range c.deltas[id] {
			if trigger != delta.Trigger && (!exists || trigger != base.Candidate.Trigger) {
				continue
			}
			if delta.ReviewStatus == NeedsReview {
				// Com semântica preservada conhecemos exatamente o contexto
				// herdado. Uma pendência não deve bloquear outra surface/perfil.
				if exists && base.Fingerprint == delta.DefaultFingerprint &&
					(!matches(base.Candidate.Condition, facts) || !matches(delta.Condition, facts)) {
					continue
				}
				// Pendência nunca é ativação implícita. Fora de diálogo, bloqueia
				// conservadoramente o acionador antigo/atual. Durante diálogo só
				// o default do topmost pode bloquear sua própria combinação.
				if dialog == nil || exists && eligible(base.Candidate, facts, dialog) {
					if !exists || matches(delta.Condition, facts) || matches(base.Candidate.Condition, facts) {
						review = append(review, delta.ID)
					}
				}
				continue
			}
			if !exists || trigger != base.Candidate.Trigger {
				continue
			}
			candidate, err := withDelta(base.Candidate, delta)
			if err != nil {
				return Result{}, err
			}
			if !eligible(candidate, facts, dialog) {
				continue
			}
			removed = true
			if delta.Effect == Suppress {
				suppressed = append(suppressed, delta.ID)
			} else {
				candidates = append(candidates, candidate)
			}
		}
		if exists && trigger == base.Candidate.Trigger && !removed {
			candidates = append(candidates, base.Candidate)
		}
	}
	if len(review) > 0 {
		slices.Sort(review)
		return Result{Status: ReviewRequired, BindingIDs: review}, nil
	}
	// Somente o bucket do acionador é materializado; não varre o catálogo.
	r, err := New(candidates)
	if err != nil {
		return Result{}, err
	}
	result, err := r.Resolve(trigger, facts, dialog)
	if err == nil && len(suppressed) > 0 && (result.Status == NoMatch || result.Status == Blocked) {
		slices.Sort(suppressed)
		return Result{Status: Suppressed, BindingIDs: suppressed}, nil
	}
	return result, err
}
