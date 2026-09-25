// Package commandportability define o formato seguro e o planejamento puro
// de portabilidade de command_layers. Ele não persiste, concede claims, cria
// grants ou executa comandos.
package commandportability

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"assistente/internal/commandactivation"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"github.com/google/uuid"
)

const ExportVersion = 2

type ScopeKind string

const (
	GlobalScope    ScopeKind = "global"
	WorkspaceScope ScopeKind = "workspace"
)

type PortableScope struct {
	Kind        ScopeKind `json:"kind"`
	WorkspaceID string    `json:"workspaceId,omitempty"`
}

// LayerExport não transporta owner, claims, grants, histórico ou defaults
// puros. O destino deriva o owner do contexto autenticado.
type LayerExport struct {
	ID                 string                 `json:"id"`
	Scope              PortableScope          `json:"scope"`
	Name               string                 `json:"name"`
	Description        string                 `json:"description,omitempty"`
	Enabled            bool                   `json:"enabled"`
	ResolutionPriority int                    `json:"resolutionPriority"`
	ActivationRules    []ActivationRuleExport `json:"activationRules,omitempty"`
	Bindings           []BindingExport        `json:"bindings,omitempty"`
	// BuiltinDeltas são personalizações do usuário que apontam para uma
	// camada builtin. Não são defaults puros e não carregam grants/claims.
	BuiltinDeltas     []BindingExport        `json:"builtinDeltas,omitempty"`
	BuiltinRuleDeltas []ActivationRuleExport `json:"builtinRuleDeltas,omitempty"`
	// DeltaOnly é o discriminador explícito do contêiner canônico por escopo.
	// Quando true, o objeto não é uma camada: ID/nome/descrição ficam vazios,
	// Enabled é true, prioridade é zero e somente os dois campos de deltas são
	// aceitos. Sem esse discriminador, ID ausente é inválido e não vira
	// silenciosamente um contêiner.
	DeltaOnly bool `json:"deltaOnly,omitempty"`
}

// ActivationRuleExport exclui authorization_decision_id, automation_grant_*,
// claims e estado de ativação. Regras event-driven são desabilitadas no plano.
type ActivationRuleExport struct {
	ID                           string  `json:"id"`
	LayerRefKind                 string  `json:"layerRefKind"`
	LayerRef                     string  `json:"layerRef"`
	RuleRefKind                  string  `json:"ruleRefKind"`
	RuleRef                      string  `json:"ruleRef"`
	Mode                         string  `json:"mode"`
	Condition                    string  `json:"condition"`
	Lifecycle                    string  `json:"lifecycle"`
	EventName                    *string `json:"eventName,omitempty"`
	AllowedInternalProducerTypes *string `json:"allowedInternalProducerTypes,omitempty"`
	Enabled                      bool    `json:"enabled"`
	ReplacesDefaultID            *string `json:"replacesDefaultId,omitempty"`
	ReplacesDefaultVersion       *string `json:"replacesDefaultVersion,omitempty"`
	ReplacesDefaultFingerprint   *string `json:"replacesDefaultFingerprint,omitempty"`
	ReviewStatus                 string  `json:"reviewStatus"`
}

// BindingExport não inclui Source/UserID. Source é sempre "user" no destino.
type BindingExport struct {
	ID                         string  `json:"id"`
	LayerRefKind               string  `json:"layerRefKind"`
	LayerRef                   string  `json:"layerRef"`
	TriggerType                string  `json:"triggerType"`
	TriggerSpec                string  `json:"triggerSpec"`
	CommandID                  *string `json:"commandId,omitempty"`
	Arguments                  string  `json:"arguments"`
	Condition                  string  `json:"condition"`
	Effect                     string  `json:"effect"`
	Enabled                    bool    `json:"enabled"`
	ResolutionPriority         int     `json:"resolutionPriority"`
	ReplacesDefaultID          *string `json:"replacesDefaultId,omitempty"`
	ReplacesDefaultVersion     *string `json:"replacesDefaultVersion,omitempty"`
	ReplacesDefaultFingerprint *string `json:"replacesDefaultFingerprint,omitempty"`
	ReviewStatus               string  `json:"reviewStatus"`
	Presentation               string  `json:"presentation"`
}

type PlanMode string

const (
	KeepMode    PlanMode = "keep"
	ReplaceMode PlanMode = "replace"
	CopyMode    PlanMode = "copy"
)

// PlanOptions contém somente decisões explícitas do chamador confiável.
// WorkspaceMap é uma intenção; cada destino ainda precisa ser autorizado
// por ReferencePort.Workspace.
type PlanOptions struct {
	Mode          PlanMode
	WorkspaceMap  map[string]string
	RenameByLayer map[string]string
	Name          NamePort
}

type Ownership string

const (
	AbsentOwner        Ownership = "absent"
	CurrentUserOwner   Ownership = "current_user"
	ForeignUserOwner   Ownership = "foreign_owner"
	AmbiguousOwnership Ownership = "ambiguous"
)

// OwnershipPort retorna somente posse; em foreign_owner nenhum conteúdo da
// linha estrangeira é carregado ou colocado no diagnóstico.
type OwnershipPort func(context.Context, string, string) (Ownership, error)

// NamePort consulta a chave natural no escopo de destino sem revelar a linha
// que causou o conflito. Cópia/renomeação só prossegue com escolha explícita.
type NamePort func(context.Context, string, string) (Ownership, error)

type CredentialStatus string

const (
	CredentialAvailable CredentialStatus = "available"
	CredentialMissing   CredentialStatus = "missing"
	CredentialForeign   CredentialStatus = "foreign_owner"
	CredentialAmbiguous CredentialStatus = "ambiguous"
)

// ReferencePort é composto por portas confiáveis do destino. Não é montado a
// partir do arquivo ou da UI.
type ReferencePort struct {
	Catalog *commandcatalog.Registry
	Command func(context.Context, string) error
	// BuiltinLayer valida a identidade estável da camada builtin referenciada
	// pelo delta (por exemplo, application.defaults).
	BuiltinLayer func(context.Context, string) error
	// BuiltinDefault valida o ID do default builtin substituído; ele não é um
	// ID de camada nem um ID de binding persistido.
	BuiltinDefault func(context.Context, string) error
	// BuiltinRuleReference valida a identidade estável de uma regra builtin.
	BuiltinRuleReference func(context.Context, string) error
	// BuiltinRule informa se a chave natural da regra builtin já existe no
	// destino autenticado. Cópia sem esta prova falha fechado.
	BuiltinRule       func(context.Context, string, string, string) (bool, error)
	CredentialPattern func(context.Context, string) (CredentialStatus, error)
	// Workspace resolve um ID apresentado à porta e retorna o mesmo ID
	// canônico/autorizado. Um WorkspaceMap não substitui esta prova.
	Workspace func(context.Context, string) (string, error)
	// Trigger normaliza o documento através do adapter registrado. O retorno
	// não é confiado pelo plano; ele só comprova que a identidade veio da porta.
	Trigger func(context.Context, string, string) (string, error)
}

type Warning struct {
	Code       string
	Identifier string
}

type PlannedLayer struct {
	Layer       LayerExport
	TargetID    string
	TargetScope PortableScope
	Action      PlanMode
	Enabled     bool
}

// Plan é um plano puro e mutável, sem autorização implícita ou efeito de
// persistência. O serviço deve revalidá-lo dentro do seu fluxo transacional.
type Plan struct {
	Version  int
	Layers   []PlannedLayer
	Warnings []Warning
}

var (
	ErrInvalid             = errors.New("exportação de command_layers inválida")
	ErrForeignOwner        = errors.New("foreign_owner em command_layers")
	ErrWorkspaceResolution = errors.New("workspace de command_layers sem resolução autorizada")
	ErrNameConflict        = errors.New("nome de command_layer exige escolha explícita")
	ErrMissingReference    = errors.New("referência de command_layer ausente")
	ErrSensitiveValue      = errors.New("valor sensível bruto em command_layer")
	ErrUnsupported         = errors.New("operação de command_layers exige pipeline confiável do aplicativo")
	ErrBuiltinRuleConflict = errors.New("regra builtin já existe no escopo de destino")
)

func (s PortableScope) validate() error {
	switch s.Kind {
	case GlobalScope:
		if s.WorkspaceID != "" {
			return ErrInvalid
		}
	case WorkspaceScope:
		if s.WorkspaceID == "" || strings.TrimSpace(s.WorkspaceID) != s.WorkspaceID || strings.ContainsRune(s.WorkspaceID, '\x00') {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

func (l LayerExport) validate() error {
	if err := l.Scope.validate(); err != nil {
		return err
	}
	if l.DeltaOnly {
		if l.ID != "" || l.Name != "" || l.Description != "" || l.ResolutionPriority != 0 || !l.Enabled || len(l.Bindings) != 0 || len(l.ActivationRules) != 0 || len(l.BuiltinDeltas) == 0 && len(l.BuiltinRuleDeltas) == 0 {
			return ErrInvalid
		}
	} else if !validUUID7(l.ID) || l.Name == "" || strings.TrimSpace(l.Name) != l.Name || l.Description != strings.TrimSpace(l.Description) || l.ResolutionPriority < 0 || len(l.BuiltinDeltas) != 0 || len(l.BuiltinRuleDeltas) != 0 {
		return ErrInvalid
	}
	seen := map[string]struct{}{}
	for _, binding := range l.Bindings {
		if err := binding.validate(); err != nil {
			return err
		}
		if binding.LayerRefKind != "user" {
			return ErrInvalid
		}
		if _, ok := seen[binding.ID]; ok {
			return ErrInvalid
		}
		seen[binding.ID] = struct{}{}
	}
	for _, binding := range l.BuiltinDeltas {
		if err := binding.validate(); err != nil {
			return err
		}
		if binding.LayerRefKind != "builtin" {
			return ErrInvalid
		}
		if _, ok := seen[binding.ID]; ok {
			return ErrInvalid
		}
		seen[binding.ID] = struct{}{}
	}
	seenRules := map[string]struct{}{}
	for _, rule := range l.ActivationRules {
		if err := rule.validate(); err != nil {
			return err
		}
		if rule.LayerRefKind != "user" {
			return ErrInvalid
		}
		if _, ok := seenRules[rule.ID]; ok {
			return ErrInvalid
		}
		seenRules[rule.ID] = struct{}{}
	}
	for _, rule := range l.BuiltinRuleDeltas {
		if err := rule.validate(); err != nil {
			return err
		}
		if rule.LayerRefKind != "builtin" {
			return ErrInvalid
		}
		if _, ok := seenRules[rule.ID]; ok {
			return ErrInvalid
		}
		seenRules[rule.ID] = struct{}{}
	}
	return nil
}

func (b BindingExport) validate() error {
	if !validUUID7(b.ID) || (b.LayerRefKind != "user" && b.LayerRefKind != "builtin") || b.LayerRef == "" || b.TriggerType == "" || b.TriggerSpec == "" || b.Arguments == "" || b.Condition == "" || b.Presentation == "" || b.ReviewStatus == "" {
		return ErrInvalid
	}
	if b.LayerRefKind == "builtin" && !validBuiltinReference(b.LayerRef) {
		return ErrInvalid
	}
	if b.Effect != "execute" && b.Effect != "suppress" {
		return ErrInvalid
	}
	if b.Effect == "suppress" && (b.CommandID != nil || b.Arguments != "{}") {
		return ErrInvalid
	}
	if b.Effect == "execute" && (b.CommandID == nil || strings.TrimSpace(*b.CommandID) == "") {
		return ErrInvalid
	}
	if (b.ReplacesDefaultID == nil) != (b.ReplacesDefaultVersion == nil) || (b.ReplacesDefaultID == nil) != (b.ReplacesDefaultFingerprint == nil) {
		return ErrInvalid
	}
	if b.LayerRefKind == "builtin" && (b.ReplacesDefaultID == nil || !validBuiltinReference(*b.ReplacesDefaultID)) {
		return ErrInvalid
	}
	// Configuração antiga pode conter segredo também em metadados/condições.
	// Use a mesma política recursiva da fronteira de argumentos, inclusive
	// para suppress e para deltas builtin, antes de exportar ou planejar.
	for _, document := range []string{b.Arguments, b.Condition, b.Presentation} {
		if err := validateNoSecretDocuments(document); err != nil {
			return err
		}
	}
	return nil
}

func (r ActivationRuleExport) validate() error {
	if !validUUID7(r.ID) || (r.LayerRefKind != "user" && r.LayerRefKind != "builtin") || r.LayerRef == "" || (r.RuleRefKind != "user" && r.RuleRefKind != "builtin") || r.RuleRef == "" || r.Mode == "" || r.Condition == "" || r.Lifecycle == "" || r.ReviewStatus == "" {
		return ErrInvalid
	}
	if r.LayerRefKind == "builtin" && !validBuiltinReference(r.LayerRef) {
		return ErrInvalid
	}
	if r.RuleRefKind == "builtin" && !validBuiltinReference(r.RuleRef) {
		return ErrInvalid
	}
	if (r.ReplacesDefaultID == nil) != (r.ReplacesDefaultVersion == nil) || (r.ReplacesDefaultID == nil) != (r.ReplacesDefaultFingerprint == nil) {
		return ErrInvalid
	}
	return validateNoSecretDocuments(r.Condition)
}

func validUUID7(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.Version() == uuid.Version(7) && id.Variant() == uuid.RFC4122 && id.String() == value
}

func validBuiltinReference(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) < 2 || strings.TrimSpace(value) != value {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for i, char := range part {
			if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' && i > 0 || char == '_' && i > 0 {
				continue
			}
			return false
		}
	}
	return true
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneLayerExport(source LayerExport) LayerExport {
	clone := source
	clone.Scope = source.Scope
	clone.Bindings = make([]BindingExport, len(source.Bindings))
	for i, binding := range source.Bindings {
		clone.Bindings[i] = binding
		clone.Bindings[i].CommandID = cloneString(binding.CommandID)
		clone.Bindings[i].ReplacesDefaultID = cloneString(binding.ReplacesDefaultID)
		clone.Bindings[i].ReplacesDefaultVersion = cloneString(binding.ReplacesDefaultVersion)
		clone.Bindings[i].ReplacesDefaultFingerprint = cloneString(binding.ReplacesDefaultFingerprint)
	}
	clone.ActivationRules = make([]ActivationRuleExport, len(source.ActivationRules))
	for i, rule := range source.ActivationRules {
		clone.ActivationRules[i] = rule
		clone.ActivationRules[i].EventName = cloneString(rule.EventName)
		clone.ActivationRules[i].AllowedInternalProducerTypes = cloneString(rule.AllowedInternalProducerTypes)
		clone.ActivationRules[i].ReplacesDefaultID = cloneString(rule.ReplacesDefaultID)
		clone.ActivationRules[i].ReplacesDefaultVersion = cloneString(rule.ReplacesDefaultVersion)
		clone.ActivationRules[i].ReplacesDefaultFingerprint = cloneString(rule.ReplacesDefaultFingerprint)
	}
	clone.BuiltinDeltas = make([]BindingExport, len(source.BuiltinDeltas))
	for i, binding := range source.BuiltinDeltas {
		clone.BuiltinDeltas[i] = binding
		clone.BuiltinDeltas[i].CommandID = cloneString(binding.CommandID)
		clone.BuiltinDeltas[i].ReplacesDefaultID = cloneString(binding.ReplacesDefaultID)
		clone.BuiltinDeltas[i].ReplacesDefaultVersion = cloneString(binding.ReplacesDefaultVersion)
		clone.BuiltinDeltas[i].ReplacesDefaultFingerprint = cloneString(binding.ReplacesDefaultFingerprint)
	}
	clone.BuiltinRuleDeltas = make([]ActivationRuleExport, len(source.BuiltinRuleDeltas))
	for i, rule := range source.BuiltinRuleDeltas {
		clone.BuiltinRuleDeltas[i] = rule
		clone.BuiltinRuleDeltas[i].EventName = cloneString(rule.EventName)
		clone.BuiltinRuleDeltas[i].AllowedInternalProducerTypes = cloneString(rule.AllowedInternalProducerTypes)
		clone.BuiltinRuleDeltas[i].ReplacesDefaultID = cloneString(rule.ReplacesDefaultID)
		clone.BuiltinRuleDeltas[i].ReplacesDefaultVersion = cloneString(rule.ReplacesDefaultVersion)
		clone.BuiltinRuleDeltas[i].ReplacesDefaultFingerprint = cloneString(rule.ReplacesDefaultFingerprint)
	}
	return clone
}

// FromSnapshot strips owner/source metadata and creates the portable DTO.
func FromSnapshot(snapshot commandconfig.Snapshot) ([]LayerExport, error) {
	if snapshot.Scope.UserID == "" {
		return nil, ErrInvalid
	}
	layers := make(map[string]LayerExport, len(snapshot.Layers))
	deltas := make(map[string]*LayerExport)
	for _, row := range snapshot.Layers {
		scope := PortableScope{Kind: GlobalScope}
		if row.WorkspaceID != nil {
			scope = PortableScope{Kind: WorkspaceScope, WorkspaceID: *row.WorkspaceID}
		}
		layer := LayerExport{ID: row.ID, Scope: scope, Name: row.Name, Description: row.Description, Enabled: row.Enabled, ResolutionPriority: row.ResolutionPriority}
		if err := layer.validate(); err != nil {
			return nil, err
		}
		if _, exists := layers[row.ID]; exists {
			return nil, ErrInvalid
		}
		layers[row.ID] = layer
	}
	seenBindings := map[string]struct{}{}
	for _, row := range snapshot.Bindings {
		binding := BindingExport{ID: row.ID, LayerRefKind: row.LayerRefKind, LayerRef: row.LayerRef, TriggerType: row.TriggerType, TriggerSpec: row.TriggerSpec, CommandID: cloneString(row.CommandID), Arguments: row.Arguments, Condition: row.Condition, Effect: row.Effect, Enabled: row.Enabled, ResolutionPriority: row.ResolutionPriority, ReplacesDefaultID: cloneString(row.ReplacesDefaultID), ReplacesDefaultVersion: cloneString(row.ReplacesDefaultVersion), ReplacesDefaultFingerprint: cloneString(row.ReplacesDefaultFingerprint), ReviewStatus: row.ReviewStatus, Presentation: row.Presentation}
		if err := binding.validate(); err != nil {
			return nil, err
		}
		if _, exists := seenBindings[binding.ID]; exists {
			return nil, ErrInvalid
		}
		seenBindings[binding.ID] = struct{}{}
		if row.LayerRefKind == "user" {
			layer, ok := layers[row.LayerRef]
			if !ok || !samePortableWorkspace(row.WorkspaceID, layer.Scope) {
				return nil, ErrInvalid
			}
			layer.Bindings = append(layer.Bindings, binding)
			layers[row.LayerRef] = layer
			continue
		}
		group := deltaGroup(deltas, portableScopeFromWorkspace(row.WorkspaceID))
		group.BuiltinDeltas = append(group.BuiltinDeltas, binding)
	}
	seenRules := map[string]struct{}{}
	for _, row := range snapshot.ActivationRules {
		rule := ActivationRuleExport{
			ID:                           row.ID,
			LayerRefKind:                 string(row.LayerRefKind),
			LayerRef:                     row.LayerRef,
			RuleRefKind:                  string(row.RuleRefKind),
			RuleRef:                      row.RuleRef,
			Mode:                         string(row.Mode),
			Condition:                    row.Condition,
			Lifecycle:                    string(row.Lifecycle),
			EventName:                    cloneString(row.EventName),
			AllowedInternalProducerTypes: cloneString(row.AllowedInternalProducerTypes),
			Enabled:                      row.Enabled,
			ReplacesDefaultID:            cloneString(row.ReplacesDefaultID),
			ReplacesDefaultVersion:       cloneString(row.ReplacesDefaultVersion),
			ReplacesDefaultFingerprint:   cloneString(row.ReplacesDefaultFingerprint),
			ReviewStatus:                 row.ReviewStatus,
		}
		if err := rule.validate(); err != nil {
			return nil, err
		}
		if _, exists := seenRules[rule.ID]; exists {
			return nil, ErrInvalid
		}
		seenRules[rule.ID] = struct{}{}
		if row.LayerRefKind == commandactivation.UserRef {
			layer, ok := layers[row.LayerRef]
			if !ok || !samePortableWorkspace(row.WorkspaceID, layer.Scope) {
				return nil, ErrInvalid
			}
			layer.ActivationRules = append(layer.ActivationRules, rule)
			layers[row.LayerRef] = layer
			continue
		}
		group := deltaGroup(deltas, portableScopeFromWorkspace(row.WorkspaceID))
		group.BuiltinRuleDeltas = append(group.BuiltinRuleDeltas, rule)
	}
	result := make([]LayerExport, 0, len(layers))
	for _, layer := range layers {
		slices.SortFunc(layer.Bindings, func(a, b BindingExport) int { return strings.Compare(a.ID, b.ID) })
		slices.SortFunc(layer.ActivationRules, func(a, b ActivationRuleExport) int { return strings.Compare(a.ID, b.ID) })
		result = append(result, layer)
	}
	for _, group := range deltas {
		slices.SortFunc(group.BuiltinDeltas, func(a, b BindingExport) int { return strings.Compare(a.ID, b.ID) })
		slices.SortFunc(group.BuiltinRuleDeltas, func(a, b ActivationRuleExport) int { return strings.Compare(a.ID, b.ID) })
		result = append(result, *group)
	}
	slices.SortFunc(result, compareLayerExport)
	return result, nil
}

func portableScopeFromWorkspace(workspace *string) PortableScope {
	if workspace == nil {
		return PortableScope{Kind: GlobalScope}
	}
	return PortableScope{Kind: WorkspaceScope, WorkspaceID: *workspace}
}

func deltaGroup(groups map[string]*LayerExport, scope PortableScope) *LayerExport {
	key := string(scope.Kind) + ":" + scope.WorkspaceID
	group, ok := groups[key]
	if !ok {
		group = &LayerExport{Scope: scope, Enabled: true, DeltaOnly: true}
		groups[key] = group
	}
	return group
}

func compareLayerExport(a, b LayerExport) int {
	if a.ID != "" || b.ID != "" {
		if a.ID != b.ID {
			return strings.Compare(a.ID, b.ID)
		}
	}
	if a.Scope.Kind != b.Scope.Kind {
		return strings.Compare(string(a.Scope.Kind), string(b.Scope.Kind))
	}
	return strings.Compare(a.Scope.WorkspaceID, b.Scope.WorkspaceID)
}

func samePortableWorkspace(workspace *string, scope PortableScope) bool {
	if scope.Kind == GlobalScope {
		return workspace == nil
	}
	return workspace != nil && *workspace == scope.WorkspaceID
}

// PlanImport é puro: valida o DTO e retorna ações explícitas. Não escreve
// SQLite, não altera a entrada e não carrega conteúdo de owner estrangeiro.
func PlanImport(ctx context.Context, layers []LayerExport, options PlanOptions, ownership OwnershipPort, refs ReferencePort) (Plan, error) {
	if ctx == nil || ownership == nil || refs.Catalog == nil || !refs.Catalog.Complete() || refs.Trigger == nil || (options.Mode != KeepMode && options.Mode != ReplaceMode && options.Mode != CopyMode) {
		return Plan{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return Plan{}, err
	}
	plan := Plan{Version: ExportVersion, Layers: make([]PlannedLayer, 0, len(layers)), Warnings: make([]Warning, 0)}
	remap := make(map[string]string, len(layers))
	ruleRemap := make(map[string]string)
	seen := make(map[string]struct{}, len(layers))
	seenRules := make(map[string]struct{})
	seenBindings := make(map[string]struct{})
	seenBuiltinRuleKeys := make(map[string]struct{})
	for _, input := range layers {
		source := cloneLayerExport(input)
		if err := source.validate(); err != nil {
			return Plan{}, err
		}
		deltaOnly := source.DeltaOnly
		if !deltaOnly {
			if _, ok := seen[source.ID]; ok {
				return Plan{}, ErrInvalid
			}
			seen[source.ID] = struct{}{}
		}
		for _, binding := range append(slices.Clone(source.Bindings), source.BuiltinDeltas...) {
			if _, ok := seenBindings[binding.ID]; ok {
				return Plan{}, ErrInvalid
			}
			seenBindings[binding.ID] = struct{}{}
		}
		allRules := append(slices.Clone(source.ActivationRules), source.BuiltinRuleDeltas...)
		for _, rule := range allRules {
			if _, ok := seenRules[rule.ID]; ok {
				return Plan{}, ErrInvalid
			}
			seenRules[rule.ID] = struct{}{}
		}
		targetScope, enabled, err := resolveScope(ctx, source.Scope, options, refs, &plan)
		if err != nil {
			return Plan{}, err
		}
		for _, rule := range allRules {
			if rule.LayerRefKind != string(commandactivation.BuiltinRef) {
				continue
			}
			key := builtinRuleNaturalKey(targetScope, rule)
			if _, ok := seenBuiltinRuleKeys[key]; ok {
				return Plan{}, ErrInvalid
			}
			seenBuiltinRuleKeys[key] = struct{}{}
		}
		if options.Mode == CopyMode {
			for _, rule := range allRules {
				if rule.LayerRefKind != string(commandactivation.BuiltinRef) {
					continue
				}
				if refs.BuiltinRule == nil {
					return Plan{}, ErrMissingReference
				}
				exists, err := refs.BuiltinRule(ctx, targetScope.WorkspaceID, rule.LayerRef, rule.RuleRef)
				if err != nil {
					return Plan{}, err
				}
				if exists {
					return Plan{}, ErrBuiltinRuleConflict
				}
			}
		}
		action, targetID := options.Mode, source.ID
		owner := AbsentOwner
		if deltaOnly {
			if err := validateDeltaOwnership(ctx, targetScope.WorkspaceID, source.BuiltinDeltas, source.BuiltinRuleDeltas, options.Mode, ownership); err != nil {
				return Plan{}, err
			}
			targetID = ""
		} else if options.Mode == CopyMode {
			id, err := uuid.NewV7()
			if err != nil {
				return Plan{}, err
			}
			targetID = id.String()
		} else {
			owner, err = ownership(ctx, targetScope.WorkspaceID, source.ID)
			if err != nil {
				return Plan{}, err
			}
			if owner == ForeignUserOwner || owner == AmbiguousOwnership {
				return Plan{}, ErrForeignOwner
			}
			if owner == CurrentUserOwner && options.Mode == KeepMode {
				action = KeepMode
			}
		}
		if deltaOnly {
			// Deltas-only não possuem nome nem camada persistente para renomear.
		} else if rename := options.RenameByLayer[source.ID]; rename != "" {
			if strings.TrimSpace(rename) != rename {
				return Plan{}, ErrNameConflict
			}
			source.Name = rename
		} else if options.Name != nil && (options.Mode == CopyMode || owner == AbsentOwner) {
			nameOwner, err := options.Name(ctx, targetScope.WorkspaceID, source.Name)
			if err != nil {
				return Plan{}, err
			}
			if nameOwner != AbsentOwner {
				return Plan{}, ErrNameConflict
			}
		}
		if err := validateReferences(ctx, &source, refs, &plan); err != nil {
			return Plan{}, err
		}
		if !deltaOnly {
			remap[source.ID] = targetID
		}
		if options.Mode == CopyMode {
			for _, rule := range allRules {
				id, err := uuid.NewV7()
				if err != nil {
					return Plan{}, err
				}
				ruleRemap[rule.ID] = id.String()
			}
		}
		plan.Layers = append(plan.Layers, PlannedLayer{Layer: source, TargetID: targetID, TargetScope: targetScope, Action: action, Enabled: enabled})
	}
	for i := range plan.Layers {
		item := &plan.Layers[i]
		if item.Layer.DeltaOnly {
			for j := range item.Layer.BuiltinDeltas {
				if options.Mode == CopyMode {
					id, err := uuid.NewV7()
					if err != nil {
						return Plan{}, err
					}
					item.Layer.BuiltinDeltas[j].ID = id.String()
				}
			}
			for j := range item.Layer.BuiltinRuleDeltas {
				rule := &item.Layer.BuiltinRuleDeltas[j]
				if options.Mode == CopyMode {
					mapped, ok := ruleRemap[rule.ID]
					if !ok {
						return Plan{}, ErrMissingReference
					}
					rule.ID = mapped
				}
				if rule.RuleRefKind == "user" {
					mapped, ok := mapRuleReference(rule.RuleRef, ruleRemap, seenRules, options.Mode)
					if !ok {
						return Plan{}, ErrMissingReference
					}
					rule.RuleRef = mapped
				}
			}
			continue
		}
		item.Layer.ID = item.TargetID
		for j := range item.Layer.Bindings {
			binding := &item.Layer.Bindings[j]
			if options.Mode == CopyMode {
				id, err := uuid.NewV7()
				if err != nil {
					return Plan{}, err
				}
				binding.ID = id.String()
			}
			if binding.LayerRefKind != "user" {
				return Plan{}, ErrInvalid
			}
			mapped, ok := remap[binding.LayerRef]
			if !ok {
				return Plan{}, ErrMissingReference
			}
			binding.LayerRef = mapped
		}
		for j := range item.Layer.BuiltinDeltas {
			if options.Mode == CopyMode {
				id, err := uuid.NewV7()
				if err != nil {
					return Plan{}, err
				}
				item.Layer.BuiltinDeltas[j].ID = id.String()
			}
		}
		for j := range item.Layer.ActivationRules {
			rule := &item.Layer.ActivationRules[j]
			if options.Mode == CopyMode {
				mapped, ok := ruleRemap[rule.ID]
				if !ok {
					return Plan{}, ErrMissingReference
				}
				rule.ID = mapped
			}
			mappedLayer, ok := remap[rule.LayerRef]
			if !ok {
				return Plan{}, ErrMissingReference
			}
			rule.LayerRef = mappedLayer
			if rule.RuleRefKind == "user" {
				mappedRule, ok := mapRuleReference(rule.RuleRef, ruleRemap, seenRules, options.Mode)
				if !ok {
					return Plan{}, ErrMissingReference
				}
				rule.RuleRef = mappedRule
			}
		}
		for j := range item.Layer.BuiltinRuleDeltas {
			rule := &item.Layer.BuiltinRuleDeltas[j]
			if options.Mode == CopyMode {
				mapped, ok := ruleRemap[rule.ID]
				if !ok {
					return Plan{}, ErrMissingReference
				}
				rule.ID = mapped
			}
			if rule.RuleRefKind == "user" {
				mapped, ok := mapRuleReference(rule.RuleRef, ruleRemap, seenRules, options.Mode)
				if !ok {
					return Plan{}, ErrMissingReference
				}
				rule.RuleRef = mapped
			}
		}
	}
	return plan, nil
}

func validateDeltaOwnership(ctx context.Context, workspace string, bindings []BindingExport, rules []ActivationRuleExport, mode PlanMode, ownership OwnershipPort) error {
	if mode == CopyMode {
		return nil
	}
	for _, id := range append(deltaBindingIDs(bindings), deltaRuleIDs(rules)...) {
		owner, err := ownership(ctx, workspace, id)
		if err != nil {
			return err
		}
		if owner == ForeignUserOwner || owner == AmbiguousOwnership {
			return ErrForeignOwner
		}
	}
	return nil
}

func deltaBindingIDs(bindings []BindingExport) []string {
	ids := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		ids = append(ids, binding.ID)
	}
	return ids
}

func deltaRuleIDs(rules []ActivationRuleExport) []string {
	ids := make([]string, 0, len(rules))
	for _, rule := range rules {
		ids = append(ids, rule.ID)
	}
	return ids
}

func mapRuleReference(id string, remap map[string]string, seen map[string]struct{}, mode PlanMode) (string, bool) {
	if mode == CopyMode {
		mapped, ok := remap[id]
		return mapped, ok
	}
	_, ok := seen[id]
	return id, ok
}

func builtinRuleNaturalKey(scope PortableScope, rule ActivationRuleExport) string {
	return string(scope.Kind) + "\x00" + scope.WorkspaceID + "\x00" + rule.LayerRef + "\x00" + rule.RuleRefKind + "\x00" + rule.RuleRef
}

func resolveScope(ctx context.Context, source PortableScope, options PlanOptions, refs ReferencePort, plan *Plan) (PortableScope, bool, error) {
	if source.Kind == GlobalScope {
		return source, true, nil
	}
	destination := options.WorkspaceMap[source.WorkspaceID]
	if destination != "" {
		if refs.Workspace == nil {
			return PortableScope{}, false, ErrWorkspaceResolution
		}
		authorized, err := refs.Workspace(ctx, destination)
		if err != nil || authorized != destination {
			return PortableScope{}, false, ErrWorkspaceResolution
		}
	} else if refs.Workspace != nil {
		var err error
		destination, err = refs.Workspace(ctx, source.WorkspaceID)
		if err != nil {
			return PortableScope{}, false, err
		}
	}
	if destination == "" {
		// O ID de origem é somente uma referência portátil. Sem uma prova
		// canônica do destino, ele não pode virar workspace persistido nem
		// autoridade de escrita. Falhar antes de criar PlannedLayer também
		// impede que Snapshot/Apply transportem esse escopo por acidente.
		plan.Warnings = append(plan.Warnings, Warning{Code: "workspace_unresolved", Identifier: source.WorkspaceID})
		return PortableScope{}, false, ErrWorkspaceResolution
	}
	target := PortableScope{Kind: WorkspaceScope, WorkspaceID: destination}
	if err := target.validate(); err != nil {
		return PortableScope{}, false, err
	}
	return target, true, nil
}

func validateReferences(ctx context.Context, layer *LayerExport, refs ReferencePort, plan *Plan) error {
	bindings := append(slices.Clone(layer.Bindings), layer.BuiltinDeltas...)
	for i := range bindings {
		binding := &bindings[i]
		if err := validateCatalogBinding(ctx, binding, refs); err != nil {
			return err
		}
		if binding.LayerRefKind == "builtin" {
			if refs.BuiltinLayer == nil || refs.BuiltinLayer(ctx, binding.LayerRef) != nil {
				return ErrMissingReference
			}
		}
		if binding.CommandID != nil && refs.Command != nil {
			if err := refs.Command(ctx, *binding.CommandID); err != nil {
				binding.Enabled = false
				plan.Warnings = append(plan.Warnings, Warning{Code: "command_unavailable", Identifier: *binding.CommandID})
			}
		}
		if binding.ReplacesDefaultID != nil {
			if refs.BuiltinDefault == nil {
				return ErrMissingReference
			}
			if err := refs.BuiltinDefault(ctx, *binding.ReplacesDefaultID); err != nil {
				binding.Enabled = false
				plan.Warnings = append(plan.Warnings, Warning{Code: "default_unavailable", Identifier: *binding.ReplacesDefaultID})
			}
		}
		credentialOK, err := validateCredentialReferences(ctx, binding.Arguments, refs.CredentialPattern, plan)
		if err != nil {
			return err
		}
		if !credentialOK {
			binding.Enabled = false
		}
	}
	rules := append(slices.Clone(layer.ActivationRules), layer.BuiltinRuleDeltas...)
	for i := range rules {
		rule := &rules[i]
		if rule.LayerRefKind == "builtin" {
			if refs.BuiltinLayer == nil || refs.BuiltinLayer(ctx, rule.LayerRef) != nil {
				return ErrMissingReference
			}
		}
		if rule.RuleRefKind == "builtin" {
			if refs.BuiltinRuleReference == nil || refs.BuiltinRuleReference(ctx, rule.RuleRef) != nil {
				return ErrMissingReference
			}
		}
		if rule.ReplacesDefaultID != nil {
			if refs.BuiltinDefault == nil {
				return ErrMissingReference
			}
			if err := refs.BuiltinDefault(ctx, *rule.ReplacesDefaultID); err != nil {
				rule.Enabled = false
				plan.Warnings = append(plan.Warnings, Warning{Code: "default_unavailable", Identifier: *rule.ReplacesDefaultID})
			}
		}
		if rule.EventName != nil {
			rule.Enabled = false
			plan.Warnings = append(plan.Warnings, Warning{Code: "event_rule_disabled", Identifier: rule.ID})
		}
	}
	copy(layer.Bindings, bindings[:len(layer.Bindings)])
	copy(layer.BuiltinDeltas, bindings[len(layer.Bindings):])
	copy(layer.ActivationRules, rules[:len(layer.ActivationRules)])
	copy(layer.BuiltinRuleDeltas, rules[len(layer.ActivationRules):])
	return nil
}

func validateCatalogBinding(ctx context.Context, binding *BindingExport, refs ReferencePort) error {
	source, ok := commandSource(binding.TriggerType)
	if !ok {
		return ErrInvalid
	}
	identity, err := refs.Trigger(ctx, binding.TriggerType, binding.TriggerSpec)
	if err != nil || identity == "" || strings.TrimSpace(identity) != identity || !strings.HasPrefix(identity, string(source)+":") {
		return ErrInvalid
	}
	if binding.Effect == "suppress" {
		return nil
	}
	if binding.CommandID == nil {
		return ErrInvalid
	}
	definition, ok := refs.Catalog.Lookup(*binding.CommandID)
	if !ok || !definition.IsComplete() || !definition.AllowsSource(source) {
		return ErrMissingReference
	}
	canonical, err := refs.Catalog.ValidateArguments(*binding.CommandID, []byte(binding.Arguments))
	if err != nil || (definition.Persistence.Arguments == commandcatalog.PersistenceNever && string(canonical) != "{}") {
		return ErrInvalid
	}
	if err := validateSensitiveArgumentPaths(canonical, definition.SensitivePaths.Input); err != nil {
		return err
	}
	binding.Arguments = string(canonical)
	return nil
}

func commandSource(triggerType string) (commandcatalog.Source, bool) {
	source := commandcatalog.Source(triggerType)
	switch source {
	case commandcatalog.KeyboardLocal, commandcatalog.KeyboardGlobal, commandcatalog.StreamDeck,
		commandcatalog.Palette, commandcatalog.UI, commandcatalog.Chat, commandcatalog.CLI, commandcatalog.Event:
		return source, true
	default:
		return "", false
	}
}

func validateSensitiveArgumentPaths(raw []byte, paths []string) error {
	for _, path := range paths {
		value, present, err := jsonPointerValue(raw, path)
		if err != nil {
			return ErrInvalid
		}
		if !present {
			continue
		}
		if !isExactCredentialReference(value) {
			return ErrSensitiveValue
		}
	}
	return nil
}

func jsonPointerValue(raw []byte, pointer string) (any, bool, error) {
	var current any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&current); err != nil {
		return nil, false, err
	}
	if pointer == "" {
		return current, true, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, false, ErrInvalid
	}
	for _, segment := range strings.Split(pointer[1:], "/") {
		segment = strings.ReplaceAll(strings.ReplaceAll(segment, "~1", "/"), "~0", "~")
		switch node := current.(type) {
		case map[string]any:
			var ok bool
			current, ok = node[segment]
			if !ok {
				return nil, false, nil
			}
		case []any:
			if segment == "" {
				return nil, false, nil
			}
			index := 0
			for _, char := range segment {
				if char < '0' || char > '9' {
					return nil, false, nil
				}
				index = index*10 + int(char-'0')
			}
			if index >= len(node) {
				return nil, false, nil
			}
			current = node[index]
		default:
			return nil, false, nil
		}
	}
	return current, true, nil
}

func isExactCredentialReference(value any) bool {
	object, ok := value.(map[string]any)
	if !ok || len(object) != 2 || object["kind"] != "credential" {
		return false
	}
	pattern, ok := object["pattern"].(string)
	return ok && pattern != "" && strings.TrimSpace(pattern) == pattern && !strings.ContainsRune(pattern, '\x00')
}

func validateCredentialReferences(ctx context.Context, raw string, port func(context.Context, string) (CredentialStatus, error), plan *Plan) (bool, error) {
	var root any
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		return false, ErrInvalid
	}
	available := true
	var visit func(any) error
	visit = func(node any) error {
		switch value := node.(type) {
		case map[string]any:
			for key, child := range value {
				if isSecretKey(key) {
					return ErrSensitiveValue
				}
				if err := visit(child); err != nil {
					return err
				}
			}
			if kind, ok := value["kind"].(string); ok && kind == "credential" {
				if len(value) != 2 {
					return ErrInvalid
				}
				pattern, ok := value["pattern"].(string)
				if !ok || pattern == "" || strings.TrimSpace(pattern) != pattern {
					return ErrInvalid
				}
				if port == nil {
					return ErrMissingReference
				}
				status, err := port(ctx, pattern)
				if err != nil {
					return err
				}
				if status != CredentialAvailable {
					available = false
					code := "credential_missing"
					switch status {
					case CredentialForeign:
						code = "credential_foreign_owner"
					case CredentialAmbiguous:
						code = "credential_ambiguous"
					}
					plan.Warnings = append(plan.Warnings, Warning{Code: code, Identifier: pattern})
				}
			}
		case []any:
			for _, child := range value {
				if err := visit(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := visit(root); err != nil {
		return false, err
	}
	return available, nil
}

func isSecretKey(key string) bool {
	key = strings.ToLower(strings.ReplaceAll(key, "_", ""))
	for _, part := range []string{"token", "password", "secret", "refresh", "headers"} {
		if strings.Contains(key, part) {
			return true
		}
	}
	return false
}

func validateNoSecretDocuments(raw string) error {
	var root any
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		return ErrInvalid
	}
	var visit func(any) error
	visit = func(node any) error {
		switch value := node.(type) {
		case map[string]any:
			for key, child := range value {
				if isSecretKey(key) {
					return ErrSensitiveValue
				}
				if err := visit(child); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range value {
				if err := visit(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return visit(root)
}

// Snapshot converte o plano para os tipos canônicos de commandconfig. Regras
// de ativação são configuração (não claims/grants) e seguem para a mesma
// validação/mutação do chamador. O retorno continua um DTO mutável: não é
// autorização nem prova de escopo; o commit deve rederivar owner, escopo,
// referências, catálogo e CAS dentro da própria transação.
func (p Plan) Snapshot(userID string) (commandconfig.Snapshot, error) {
	if userID == "" || p.Version != ExportVersion {
		return commandconfig.Snapshot{}, ErrInvalid
	}
	snapshot := commandconfig.Snapshot{Scope: commandconfig.Scope{UserID: userID}}
	for _, item := range p.Layers {
		workspace := item.TargetScope.WorkspaceID
		var workspacePtr *string
		if item.TargetScope.Kind == WorkspaceScope {
			workspacePtr = &workspace
		}
		if !item.Layer.DeltaOnly && item.TargetID != "" {
			snapshot.Layers = append(snapshot.Layers, commandconfig.Layer{ID: item.TargetID, UserID: userID, WorkspaceID: cloneString(workspacePtr), Name: item.Layer.Name, Description: item.Layer.Description, Enabled: item.Enabled && item.Layer.Enabled, Source: "user", ResolutionPriority: item.Layer.ResolutionPriority})
		}
		for _, binding := range item.Layer.Bindings {
			if binding.LayerRefKind != "user" || item.TargetID == "" {
				return commandconfig.Snapshot{}, ErrInvalid
			}
			snapshot.Bindings = append(snapshot.Bindings, portableBindingSnapshot(binding, userID, workspacePtr, item.TargetID, item.Enabled))
		}
		for _, exportedRule := range item.Layer.ActivationRules {
			if exportedRule.LayerRefKind != "user" || item.TargetID == "" {
				return commandconfig.Snapshot{}, ErrInvalid
			}
			snapshot.ActivationRules = append(snapshot.ActivationRules, portableRuleSnapshot(exportedRule, userID, workspacePtr, item.TargetID, item.Enabled))
		}
		for _, binding := range item.Layer.BuiltinDeltas {
			if binding.LayerRefKind != "builtin" {
				return commandconfig.Snapshot{}, ErrInvalid
			}
			snapshot.Bindings = append(snapshot.Bindings, portableBindingSnapshot(binding, userID, workspacePtr, binding.LayerRef, item.Enabled))
		}
		for _, exportedRule := range item.Layer.BuiltinRuleDeltas {
			if exportedRule.LayerRefKind != "builtin" {
				return commandconfig.Snapshot{}, ErrInvalid
			}
			snapshot.ActivationRules = append(snapshot.ActivationRules, portableRuleSnapshot(exportedRule, userID, workspacePtr, exportedRule.LayerRef, item.Enabled))
		}
	}
	return snapshot, nil
}

func portableBindingSnapshot(binding BindingExport, userID string, workspace *string, layerRef string, layerEnabled bool) commandconfig.Binding {
	return commandconfig.Binding{ID: binding.ID, UserID: userID, WorkspaceID: cloneString(workspace), LayerRefKind: binding.LayerRefKind, LayerRef: layerRef, TriggerType: binding.TriggerType, TriggerSpec: binding.TriggerSpec, CommandID: cloneString(binding.CommandID), Arguments: binding.Arguments, Condition: binding.Condition, Effect: binding.Effect, Enabled: binding.Enabled && layerEnabled, Source: "user", ResolutionPriority: binding.ResolutionPriority, ReplacesDefaultID: cloneString(binding.ReplacesDefaultID), ReplacesDefaultVersion: cloneString(binding.ReplacesDefaultVersion), ReplacesDefaultFingerprint: cloneString(binding.ReplacesDefaultFingerprint), ReviewStatus: binding.ReviewStatus, Presentation: binding.Presentation}
}

func portableRuleSnapshot(rule ActivationRuleExport, userID string, workspace *string, layerRef string, layerEnabled bool) commandactivation.Rule {
	return commandactivation.Rule{ID: rule.ID, UserID: userID, WorkspaceID: cloneString(workspace), LayerRefKind: commandactivation.RefKind(rule.LayerRefKind), LayerRef: layerRef, RuleRefKind: commandactivation.RefKind(rule.RuleRefKind), RuleRef: rule.RuleRef, Mode: commandactivation.Mode(rule.Mode), Condition: rule.Condition, Lifecycle: commandactivation.Lifecycle(rule.Lifecycle), EventName: cloneString(rule.EventName), AllowedInternalProducerTypes: cloneString(rule.AllowedInternalProducerTypes), Enabled: rule.Enabled && layerEnabled, Source: "user", ReplacesDefaultID: cloneString(rule.ReplacesDefaultID), ReplacesDefaultVersion: cloneString(rule.ReplacesDefaultVersion), ReplacesDefaultFingerprint: cloneString(rule.ReplacesDefaultFingerprint), ReviewStatus: rule.ReviewStatus}
}

func (p Plan) ValidateWithCompleteProjection(ctx context.Context, userID string, projection commandconfig.CompleteProjection) error {
	snapshot, err := p.Snapshot(userID)
	if err != nil {
		return err
	}
	_, err = commandconfig.ProjectComplete(ctx, snapshot, projection)
	return err
}
