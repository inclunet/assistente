// Package commandbindings contém o protótipo puro de seleção de candidatos
// da Fase 0 da AEP-0103. Não autoriza nem executa comandos. O chamador futuro
// deve autenticar, materializar deltas, validar schemas e revalidar contexto.
package commandbindings

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Scope é a precedência de D7; valores menores têm preferência.
type Scope uint8

const (
	Dialog Scope = iota
	Control
	Surface
	Workspace
	ExplicitLayer
	Foreground
	Application
	Global
)

// Field identifica fatos normalizados do protótipo. A lista não é ainda o
// schema público de contexto; os providers autoritativos serão integrados depois.
type Field string

const (
	SurfaceType Field = "surface.type"
	SurfaceID   Field = "surface.id"
	Profile     Field = "profile"
	Device      Field = "device"
	Process     Field = "foreground.process"
	AppFocused  Field = "app.focused"
)

// Facts aceita igualdade exata: strings para identidades e bool para foco.
// Ausência significa fato desconhecido, nunca false nem string vazia.
type Facts map[Field]any

func (f Facts) validate() error {
	for key, value := range f {
		switch key {
		case AppFocused:
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("%s exige bool", key)
			}
		case SurfaceType, SurfaceID, Profile, Device, Process:
			if s, ok := value.(string); !ok || strings.TrimSpace(s) == "" {
				return fmt.Errorf("%s exige string não vazia", key)
			}
		default:
			return fmt.Errorf("campo desconhecido: %s", key)
		}
	}
	return nil
}

// Candidate já representa um binding executável após materialização de deltas.
// ArgumentsKey é a identidade dos argumentos normalizados; não é JSON bruto.
// ExecutionScopeKey identifica o alvo, distinguindo instâncias de uma surface.
// Nenhuma dessas chaves é calculada ou aceita de cliente por este protótipo.
type Candidate struct {
	ID                string
	Trigger           string
	CommandID         string
	ArgumentsKey      string
	ExecutionScopeKey string
	Scope             Scope
	Condition         Facts
	LayerPriority     int
	BindingPriority   int
	Enabled           bool
	LayerActive       bool
	DialogID          string
}

// DialogScope representa exclusivamente o diálogo topmost, já derivado pelo
// host. Uma allowlist vazia bloqueia tudo. Invariantes de D7 devem ser reservados
// pelo futuro dispatcher antes de chegar a este seletor.
type DialogScope struct {
	ID                string
	AllowedCommandIDs []string
	AllowedTriggers   []string
}

type Status string

const (
	Selected Status = "selected"
	NoMatch  Status = "no_match"
	Blocked  Status = "blocked_by_dialog"
	Conflict Status = "conflict"
)

// Result não é uma autorização. Em conflito, CommandID e chaves ficam vazios;
// BindingIDs contém os candidatos empatados para diagnóstico, em ordem estável.
type Result struct {
	Status            Status
	CommandID         string
	ArgumentsKey      string
	ExecutionScopeKey string
	BindingIDs        []string
}

// Resolver é um snapshot imutável, indexado por acionador, sem banco ou handlers.
type Resolver struct{ byTrigger map[string][]Candidate }

func New(candidates []Candidate) (*Resolver, error) {
	r := &Resolver{byTrigger: make(map[string][]Candidate)}
	ids := make(map[string]bool)
	for _, c := range candidates {
		if strings.TrimSpace(c.ID) == "" || strings.TrimSpace(c.Trigger) == "" ||
			strings.TrimSpace(c.CommandID) == "" || strings.TrimSpace(c.ArgumentsKey) == "" ||
			strings.TrimSpace(c.ExecutionScopeKey) == "" {
			return nil, fmt.Errorf("candidato exige ID, acionador, comando e identidades normalizadas")
		}
		if ids[c.ID] {
			return nil, fmt.Errorf("binding duplicado: %s", c.ID)
		}
		ids[c.ID] = true
		if c.Scope > Global {
			return nil, fmt.Errorf("escopo inválido: %d", c.Scope)
		}
		if (c.Scope == Dialog && strings.TrimSpace(c.DialogID) == "") ||
			(c.Scope != Dialog && c.DialogID != "") {
			return nil, fmt.Errorf("binding %s: identidade de diálogo incoerente com escopo", c.ID)
		}
		if err := c.Condition.validate(); err != nil {
			return nil, fmt.Errorf("binding %s: %w", c.ID, err)
		}
		// O normalizador futuro deve obter o tipo da identidade no provider.
		// Exigir ambos mantém identidade > tipo sob a regra de inclusão de D7,
		// sem inventar uma ordem entre campos independentes.
		if _, hasID := c.Condition[SurfaceID]; hasID {
			if _, hasType := c.Condition[SurfaceType]; !hasType {
				return nil, fmt.Errorf("binding %s: identidade de surface exige tipo normalizado", c.ID)
			}
		}
		c.Condition = maps.Clone(c.Condition)
		r.byTrigger[c.Trigger] = append(r.byTrigger[c.Trigger], c)
	}
	return r, nil
}

func matches(condition, facts Facts) bool {
	for key, value := range condition {
		if actual, ok := facts[key]; !ok || actual != value {
			return false
		}
	}
	return true
}

// dominates só estabelece especificidade quando há inclusão estrita de cláusulas.
func dominates(a, b Facts) bool { return len(a) > len(b) && matches(b, a) }

func sameTarget(a, b Candidate) bool {
	return a.CommandID == b.CommandID && a.ArgumentsKey == b.ArgumentsKey &&
		a.ExecutionScopeKey == b.ExecutionScopeKey
}

func eligible(c Candidate, facts Facts, dialog *DialogScope) bool {
	if !c.Enabled || !c.LayerActive || !matches(c.Condition, facts) {
		return false
	}
	if c.Scope == Foreground && facts[AppFocused] != false {
		return false
	}
	if dialog != nil {
		return c.Scope == Dialog && c.DialogID == dialog.ID &&
			slices.Contains(dialog.AllowedCommandIDs, c.CommandID) &&
			slices.Contains(dialog.AllowedTriggers, c.Trigger)
	}
	return c.Scope != Dialog
}

// Resolve escolhe entre candidatos ativos do acionador. Primeiro elimina escopos
// inferiores, depois condições estritamente menos específicas e finalmente
// aplica prioridades aos máximos incomparáveis. Não usa sort com comparador de
// especificidade parcial (que não forma uma ordem total).
func (r *Resolver) Resolve(trigger string, facts Facts, dialog *DialogScope) (Result, error) {
	if err := facts.validate(); err != nil {
		return Result{}, err
	}
	if dialog != nil && strings.TrimSpace(dialog.ID) == "" {
		return Result{}, fmt.Errorf("diálogo topmost exige identidade")
	}
	empty := Result{Status: NoMatch, BindingIDs: []string{}}
	if dialog != nil {
		empty.Status = Blocked
	}
	var candidates []Candidate
	bestScope := Global
	for _, c := range r.byTrigger[trigger] {
		if !eligible(c, facts, dialog) {
			continue
		}
		if len(candidates) == 0 || c.Scope < bestScope {
			candidates = candidates[:0]
			bestScope = c.Scope
		}
		if c.Scope == bestScope {
			candidates = append(candidates, c)
		}
	}
	if len(candidates) == 0 {
		return empty, nil
	}
	var maximal []Candidate
	for i, c := range candidates {
		dominated := false
		for j, other := range candidates {
			if i != j && dominates(other.Condition, c.Condition) {
				dominated = true
				break
			}
		}
		if !dominated {
			maximal = append(maximal, c)
		}
	}
	winners := []Candidate{maximal[0]}
	for _, c := range maximal[1:] {
		best := winners[0]
		if c.LayerPriority > best.LayerPriority ||
			(c.LayerPriority == best.LayerPriority && c.BindingPriority > best.BindingPriority) {
			winners = []Candidate{c}
		} else if c.LayerPriority == best.LayerPriority && c.BindingPriority == best.BindingPriority {
			winners = append(winners, c)
		}
	}
	best := winners[0]
	result := Result{Status: Selected, CommandID: best.CommandID,
		ArgumentsKey: best.ArgumentsKey, ExecutionScopeKey: best.ExecutionScopeKey,
		BindingIDs: []string{}}
	for _, c := range winners {
		if !sameTarget(c, best) {
			result.Status = Conflict
		}
		result.BindingIDs = append(result.BindingIDs, c.ID)
	}
	if result.Status == Conflict {
		result.CommandID, result.ArgumentsKey, result.ExecutionScopeKey = "", "", ""
	} else {
		// Preserva também a proveniência dos equivalentes elegíveis que perderam
		// precedência. Deltas/supressões já devem ter sido materializados antes.
		for _, c := range r.byTrigger[trigger] {
			if slices.Contains(result.BindingIDs, c.ID) || !sameTarget(c, best) || !eligible(c, facts, dialog) {
				continue
			}
			result.BindingIDs = append(result.BindingIDs, c.ID)
		}
	}
	slices.Sort(result.BindingIDs)
	return result, nil
}
