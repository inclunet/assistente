package commandbindings

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
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
	// LayerConditions are activation gates owned by the layer/rule domain.
	// They are OR-ed and do not participate in binding specificity.
	LayerConditions []Facts
	ReviewStatus    ReviewStatus
	LayerPriority   int
	BindingPriority int
	LayerRef        string
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
	validUntil               time.Time
	defaults                 map[string]Default
	deltas                   map[string][]Delta
	byTrigger                map[string][]string
	custom                   *Resolver
	presentation             *PresentationSnapshot
	layerPresentationTargets map[string]LayerPresentationState
	adjustments              []Adjustment
	layerProvenance          map[string][]LayerProvenance
	persistedBaseline        string
}

// WithValidityDeadline attaches the host's earliest activation deadline to an
// immutable projection. Adapters must not keep using this snapshot after it.
func (c *Configuration) WithValidityDeadline(deadline time.Time) *Configuration {
	if c == nil {
		return nil
	}
	clone := *c
	clone.validUntil = deadline
	return &clone
}

func (c *Configuration) ValidUntil() time.Time {
	if c == nil {
		return time.Time{}
	}
	return c.validUntil
}

// WithPresentation associa uma projeção textual imutável sem alterar a
// identidade/execução da configuração. O snapshot é clonado para que nem o
// objeto recebido possa ser reutilizado como alias interno.
func (c *Configuration) WithPresentation(presentation *PresentationSnapshot) *Configuration {
	if c == nil {
		return nil
	}
	clone := *c
	clone.presentation = presentation.clone()
	return &clone
}

// Presentation devolve um clone independente do snapshot textual.
func (c *Configuration) Presentation() *PresentationSnapshot {
	if c == nil {
		return nil
	}
	return c.presentation.clone()
}

// TitleForBindings consulta a apresentação somente para os IDs já retornados
// pela resolução. IDs inelegíveis nunca chegam a este contrato.
func (c *Configuration) TitleForBindings(bindingIDs []string, locale string) (string, bool) {
	if c == nil {
		return "", false
	}
	return c.presentation.TitleForBindings(bindingIDs, locale)
}

// TriggerIdentities retorna cópia ordenada do índice para publicação de mapas
// de adapters. Não resolve nem autoriza comandos; não é usado por keydown.
func (c *Configuration) TriggerIdentities() []string {
	if c == nil {
		return nil
	}
	identities := make(map[string]bool, len(c.byTrigger))
	for identity := range c.byTrigger {
		identities[identity] = true
	}
	if c.custom != nil {
		for identity := range c.custom.byTrigger {
			identities[identity] = true
		}
	}
	result := make([]string, 0, len(identities))
	for identity := range identities {
		result = append(result, identity)
	}
	slices.Sort(result)
	return result
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
		if err := validateLayerConditions(d.Candidate.LayerConditions); err != nil {
			return nil, err
		}
		d.Candidate.LayerConditions = cloneLayerConditions(d.Candidate.LayerConditions)
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
		if err := validateLayerConditions(delta.LayerConditions); err != nil {
			return nil, err
		}
		delta.LayerConditions = cloneLayerConditions(delta.LayerConditions)
		base, exists := c.defaults[delta.DefaultID]
		if exists && base.Invariant {
			return nil, fmt.Errorf("default invariante não admite delta: %s", delta.DefaultID)
		}
		// Mesmo quando o fingerprint mudou, a forma do delta continua sendo
		// entrada de configuração e precisa ser validada antes de virar uma
		// pendência. Classificar como needs_review não é um bypass para campos
		// impossíveis de materializar.
		if exists {
			if _, err := withDelta(base.Candidate, delta); err != nil {
				return nil, err
			}
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
		default:
			if base.Version != delta.DefaultVersion {
				// O status original é parte do ajuste para que o repository
				// possa reaplicar somente a versão sem reativar uma pendência.
				c.adjustments = append(c.adjustments, Adjustment{delta.ID, delta.ReviewStatus, base.Version, "version_advanced"})
				delta.DefaultVersion = base.Version
			}
			// Uma pendência explícita permanece pendente, mas não exige um
			// segundo ajuste: o avanço de versão acima já é reutilizável.
			if delta.ReviewStatus == NeedsReview {
				reason = "pending_review"
			}
		}
		if reason != "" {
			delta.ReviewStatus = NeedsReview
			if reason != "pending_review" {
				c.adjustments = append(c.adjustments, Adjustment{delta.ID, NeedsReview, delta.DefaultVersion, reason})
			}
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

// RequiredFacts retorna os fatos que podem alterar a decisão para trigger.
// Considera o bucket já composto de candidatos, defaults e deltas. Deltas de
// um default também entram quando o trigger é o acionador original, pois uma
// pendência pode continuar bloqueando esse acionador mesmo quando o delta
// aponta para outro trigger.
func (c *Configuration) RequiredFacts(trigger string) []Field {
	fields := make(map[Field]struct{})
	add := func(condition Facts) {
		for field := range condition {
			fields[field] = struct{}{}
		}
	}
	addLayer := func(conditions []Facts) {
		for _, condition := range conditions {
			add(condition)
		}
	}
	for _, candidate := range c.custom.byTrigger[trigger] {
		if candidate.Enabled && candidate.LayerActive {
			add(candidate.Condition)
			addLayer(candidate.LayerConditions)
		}
	}
	for _, id := range c.byTrigger[trigger] {
		base, exists := c.defaults[id]
		baseActive := exists && base.Candidate.Enabled && base.Candidate.LayerActive
		if baseActive && base.Candidate.Trigger == trigger {
			add(base.Candidate.Condition)
			addLayer(base.Candidate.LayerConditions)
		}
		for _, delta := range c.deltas[id] {
			if delta.ReviewStatus == NeedsReview && (delta.Trigger == trigger || exists && base.Candidate.Trigger == trigger) {
				// NeedsReview continua bloqueando conservadoramente conforme
				// Resolve, inclusive se a configuração efetiva estiver inativa.
				if exists {
					add(base.Candidate.Condition)
					addLayer(base.Candidate.LayerConditions)
				}
				add(delta.Condition)
				continue
			}
			if delta.ReviewStatus == Active && baseActive && base.Candidate.Trigger == trigger && delta.Trigger == trigger && delta.Enabled && delta.LayerActive {
				add(delta.Condition)
				addLayer(delta.LayerConditions)
			}
		}
	}
	result := make([]Field, 0, len(fields))
	for field := range fields {
		result = append(result, field)
	}
	slices.Sort(result)
	return result
}

// FieldValues retorna os valores literais de um fato que participam da decisão
// do acionador. O resultado inclui condições de bindings, gates de camada e
// pendências de revisão, mas nunca inventa um valor ausente.
func (c *Configuration) FieldValues(trigger string, field Field) []string {
	if c == nil {
		return nil
	}
	values := map[string]struct{}{}
	add := func(facts Facts) {
		if value, ok := facts[field].(string); ok && value != "" {
			values[value] = struct{}{}
		}
	}
	addLayer := func(conditions []Facts) {
		for _, condition := range conditions {
			add(condition)
		}
	}
	for _, candidate := range c.custom.byTrigger[trigger] {
		if candidate.Enabled && candidate.LayerActive {
			add(candidate.Condition)
			addLayer(candidate.LayerConditions)
		}
	}
	for _, id := range c.byTrigger[trigger] {
		base, exists := c.defaults[id]
		if exists && base.Candidate.Trigger == trigger {
			add(base.Candidate.Condition)
			addLayer(base.Candidate.LayerConditions)
		}
		for _, delta := range c.deltas[id] {
			if delta.Trigger == trigger || exists && base.Candidate.Trigger == trigger {
				if exists && delta.Trigger == trigger && base.Candidate.Trigger != trigger {
					add(base.Candidate.Condition)
					addLayer(base.Candidate.LayerConditions)
				}
				add(delta.Condition)
				addLayer(delta.LayerConditions)
			}
		}
	}
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}

// SurfaceValues retorna os valores literais de surface.type que participam da
// decisão do acionador. Mantém a API histórica como atalho para FieldValues.
func (c *Configuration) SurfaceValues(trigger string) []string {
	return c.FieldValues(trigger, SurfaceType)
}

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
	if delta.LayerRef != "" {
		base.LayerRef = delta.LayerRef
	}
	if delta.Effect == Execute {
		base.CommandID, base.ArgumentsKey = delta.CommandID, delta.ArgumentsKey
	}
	// Enabled/LayerActive são compostos com os do default, nunca ampliados.
	base.Enabled = base.Enabled && delta.Enabled
	base.LayerActive = base.LayerActive && delta.LayerActive
	if delta.LayerConditions != nil {
		base.LayerConditions = cloneLayerConditions(delta.LayerConditions)
	}
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
	suppressionCandidates := []Candidate{}
	review := []string{}
	for _, id := range c.byTrigger[trigger] {
		base, exists := c.defaults[id]
		removed := false
		for _, delta := range c.deltas[id] {
			if trigger != delta.Trigger && (!exists || trigger != base.Candidate.Trigger) {
				continue
			}
			if delta.ReviewStatus == NeedsReview {
				// A pendência bloqueia apenas a conjunção efetiva do default e
				// do estreitamento do delta. Não transforma uma revisão de uma
				// surface/perfil em bloqueio global do acionador.
				if exists && trigger != base.Candidate.Trigger && trigger != delta.Trigger {
					continue
				}
				if exists && !matches(base.Candidate.Condition, facts) {
					continue
				}
				if !matches(delta.Condition, facts) {
					continue
				}
				// Pendência nunca é ativação implícita. Fora de diálogo, bloqueia
				// conservadoramente o acionador antigo/atual. Durante diálogo só
				// o default do topmost pode bloquear sua própria combinação.
				if dialog == nil || exists && eligible(base.Candidate, facts, dialog) {
					review = append(review, delta.ID)
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
				suppressionCandidates = append(suppressionCandidates, candidate)
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
		return Result{Status: ReviewRequired, BindingIDs: review, LayerRefs: layerRefsForDeltas(c.deltaEntriesForTrigger(trigger), review)}, nil
	}
	// Somente o bucket do acionador é materializado; não varre o catálogo.
	r, err := New(candidates)
	if err != nil {
		return Result{}, err
	}
	result, err := r.Resolve(trigger, facts, dialog)
	if err == nil && len(suppressed) > 0 && result.Status == Selected && suppressionWins(suppressionCandidates, candidates, result.BindingIDs) {
		slices.Sort(suppressed)
		return Result{Status: Suppressed, BindingIDs: suppressed, LayerRefs: layerRefsForDeltas(c.deltaEntriesForTrigger(trigger), suppressed)}, nil
	}
	if err == nil && len(suppressed) > 0 && (result.Status == NoMatch || result.Status == Blocked) {
		slices.Sort(suppressed)
		return Result{Status: Suppressed, BindingIDs: suppressed, LayerRefs: layerRefsForDeltas(c.deltaEntriesForTrigger(trigger), suppressed)}, nil
	}
	return result, err
}

func suppressionWins(suppressions, candidates []Candidate, selectedIDs []string) bool {
	selected := make([]Candidate, 0, len(selectedIDs))
	for _, id := range selectedIDs {
		for _, candidate := range candidates {
			if candidate.ID == id {
				selected = append(selected, candidate)
				break
			}
		}
	}
	for _, suppression := range suppressions {
		wins := true
		compared := false
		for _, candidate := range selected {
			if sameTarget(suppression, candidate) {
				continue
			}
			compared = true
			// candidatePrecedes compara primeiro o escopo D7 e depois a
			// especificidade. Um tombstone só sombreia quando nenhum candidato
			// diferente do mesmo alvo o precede; não deixe a condição do tombstone
			// pular a precedência de Surface sobre Global.
			if candidatePrecedes(candidate, suppression) {
				wins = false
				break
			}
		}
		if wins && compared {
			return true
		}
	}
	return false
}

func candidatePrecedes(a, b Candidate) bool {
	if a.Scope != b.Scope {
		return a.Scope < b.Scope
	}
	if dominates(a.Condition, b.Condition) {
		return true
	}
	if dominates(b.Condition, a.Condition) {
		return false
	}
	return a.LayerPriority > b.LayerPriority ||
		(a.LayerPriority == b.LayerPriority && a.BindingPriority > b.BindingPriority)
}

func (c *Configuration) deltaEntriesForTrigger(trigger string) [][]Delta {
	entries := make([][]Delta, 0, len(c.byTrigger[trigger]))
	for _, id := range c.byTrigger[trigger] {
		if deltas := c.deltas[id]; len(deltas) > 0 {
			entries = append(entries, deltas)
		}
	}
	return entries
}

func layerRefsForDeltas(entries [][]Delta, bindingIDs []string) []string {
	byID := make(map[string]string, len(bindingIDs))
	for _, deltas := range entries {
		for _, delta := range deltas {
			byID[delta.ID] = delta.LayerRef
		}
	}
	refs := make(map[string]struct{})
	for _, id := range bindingIDs {
		if ref := byID[id]; ref != "" {
			refs[ref] = struct{}{}
		}
	}
	result := make([]string, 0, len(refs))
	if len(refs) == 0 {
		return nil
	}
	for ref := range refs {
		result = append(result, ref)
	}
	slices.Sort(result)
	return result
}
