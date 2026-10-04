// Package llmcapabilities define o vocabulário controlado e a resolução local
// de fatos de capabilities. Não consulta rede nem acessa o banco.
package llmcapabilities

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type Capability string
type FieldKey string
type SupportState string
type Source string
type Scope string
type ValueType string

const (
	Supported   SupportState = "supported"
	Unsupported SupportState = "unsupported"
	Unknown     SupportState = "unknown"

	SourceExecutionObservation Source = "execution_observation"
	SourceEndpointDiscovery    Source = "endpoint_discovery"
	SourceAppCuration          Source = "app_curation"
	SourceOfficialCatalog      Source = "official_catalog"
	SourceThirdPartyCatalog    Source = "third_party_catalog"

	ScopeConnection      Scope = "connection"
	ScopeExternalBinding Scope = "external_binding"
	ScopeGeneric         Scope = "generic"

	TypeNumber  ValueType = "number"
	TypeInteger ValueType = "integer"
	TypeBoolean ValueType = "boolean"
	TypeString  ValueType = "string"
	TypeEnum    ValueType = "enum"
)

const (
	CapabilityChat            Capability = "chat"
	CapabilityReasoning       Capability = "reasoning"
	CapabilityTools           Capability = "tools"
	CapabilityInputText       Capability = "input_text"
	CapabilityInputImage      Capability = "input_image"
	CapabilityInputAudio      Capability = "input_audio"
	CapabilityInputVideo      Capability = "input_video"
	CapabilityOutputText      Capability = "output_text"
	CapabilityOutputAudio     Capability = "output_audio"
	CapabilityOutputImage     Capability = "output_image"
	CapabilityOutputVideo     Capability = "output_video"
	CapabilityTTS             Capability = "tts"
	CapabilitySTT             Capability = "stt"
	CapabilityImageGeneration Capability = "image_generation"
	CapabilityMusicGeneration Capability = "music_generation"
)

const (
	FieldMaxOutputTokens    FieldKey = "max_output_tokens"
	FieldTemperature        FieldKey = "temperature"
	FieldTopP               FieldKey = "top_p"
	FieldFrequencyPenalty   FieldKey = "frequency_penalty"
	FieldPresencePenalty    FieldKey = "presence_penalty"
	FieldSeed               FieldKey = "seed"
	FieldReasoningEffort    FieldKey = "reasoning_effort"
	FieldMaxReasoningTokens FieldKey = "max_reasoning_tokens"
	FieldParallelToolCalls  FieldKey = "parallel_tool_calls"
	FieldVoice              FieldKey = "voice"
	FieldSpeed              FieldKey = "speed"
	FieldAudioFormat        FieldKey = "audio_format"
	FieldLanguage           FieldKey = "language"
)

type FieldSpec struct {
	Capability Capability
	Key        FieldKey
	ValueType  ValueType
	Unit       string
}

// catalog é fechado em código: importadores só poderão gravar chaves e tipos
// que a aplicação conhece explicitamente.
var catalog = map[Capability]map[FieldKey]FieldSpec{
	CapabilityChat: {
		FieldMaxOutputTokens:  {CapabilityChat, FieldMaxOutputTokens, TypeInteger, "tokens"},
		FieldTemperature:      {CapabilityChat, FieldTemperature, TypeNumber, ""},
		FieldTopP:             {CapabilityChat, FieldTopP, TypeNumber, ""},
		FieldFrequencyPenalty: {CapabilityChat, FieldFrequencyPenalty, TypeNumber, ""},
		FieldPresencePenalty:  {CapabilityChat, FieldPresencePenalty, TypeNumber, ""},
		FieldSeed:             {CapabilityChat, FieldSeed, TypeInteger, ""},
	},
	CapabilityReasoning: {
		FieldReasoningEffort:    {CapabilityReasoning, FieldReasoningEffort, TypeEnum, ""},
		FieldMaxReasoningTokens: {CapabilityReasoning, FieldMaxReasoningTokens, TypeInteger, "tokens"},
	},
	CapabilityTools: {
		FieldParallelToolCalls: {CapabilityTools, FieldParallelToolCalls, TypeBoolean, ""},
	},
	CapabilityTTS: {
		FieldVoice:       {CapabilityTTS, FieldVoice, TypeEnum, ""},
		FieldSpeed:       {CapabilityTTS, FieldSpeed, TypeNumber, ""},
		FieldAudioFormat: {CapabilityTTS, FieldAudioFormat, TypeEnum, ""},
	},
	CapabilitySTT: {
		FieldLanguage:    {CapabilitySTT, FieldLanguage, TypeString, ""},
		FieldAudioFormat: {CapabilitySTT, FieldAudioFormat, TypeEnum, ""},
	},
}

var capabilityKeys = []Capability{
	CapabilityChat, CapabilityReasoning, CapabilityTools,
	CapabilityInputText, CapabilityInputImage, CapabilityInputAudio,
	CapabilityInputVideo, CapabilityOutputText, CapabilityOutputAudio,
	CapabilityOutputImage, CapabilityOutputVideo, CapabilityTTS,
	CapabilitySTT, CapabilityImageGeneration, CapabilityMusicGeneration,
}

func Capabilities() []Capability {
	result := append([]Capability(nil), capabilityKeys...)
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func Fields() []FieldSpec {
	result := make([]FieldSpec, 0)
	for _, fields := range catalog {
		for _, field := range fields {
			result = append(result, field)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Capability == result[j].Capability {
			return result[i].Key < result[j].Key
		}
		return result[i].Capability < result[j].Capability
	})
	return result
}

func HasCapability(capability Capability) bool {
	for _, known := range capabilityKeys {
		if known == capability {
			return true
		}
	}
	return false
}

func Field(capability Capability, key FieldKey) (FieldSpec, bool) {
	field, ok := catalog[capability][key]
	return field, ok
}

type CapabilityAssertion struct {
	ID               string
	Capability       Capability
	State            SupportState
	Source           Source
	Scope            Scope
	ProviderRevision int
	BindingVerified  bool
	BindingRevision  int
	ObservedAt       time.Time
	ValidUntil       *time.Time
}

type FieldOption struct {
	Value string
	Label string
	State SupportState
}

type FieldAssertion struct {
	ID               string
	Capability       Capability
	Field            FieldKey
	State            SupportState
	Source           Source
	Scope            Scope
	ProviderRevision int
	BindingVerified  bool
	BindingRevision  int
	Minimum          *float64
	Maximum          *float64
	Step             *float64
	Options          []FieldOption
	ObservedAt       time.Time
	ValidUntil       *time.Time
}

type EffectiveFact struct {
	State       SupportState
	Source      Source
	Scope       Scope
	ObservedAt  time.Time
	Conflict    bool
	EvidenceIDs []string
}

type EffectiveField struct {
	EffectiveFact
	Minimum *float64
	Maximum *float64
	Step    *float64
	Options []FieldOption
}

type Resolution struct {
	Capabilities map[Capability]EffectiveFact
	Fields       map[Capability]map[FieldKey]EffectiveField
}

var (
	ErrUnknownCapability = errors.New("capability desconhecida")
	ErrUnknownField      = errors.New("campo desconhecido para capability")
	ErrInvalidAssertion  = errors.New("afirmação de capability inválida")
)

func ValidateCapabilityAssertion(a CapabilityAssertion) error {
	if !HasCapability(a.Capability) || !validState(a.State) || !validSourceScope(a.Source, a.Scope) ||
		a.ProviderRevision < 1 || a.ObservedAt.IsZero() ||
		!validBindingScope(a.Scope, a.BindingVerified, a.BindingRevision, a.ProviderRevision) || !validExpiry(a.ObservedAt, a.ValidUntil) {
		return ErrInvalidAssertion
	}
	return nil
}

func ValidateFieldAssertion(a FieldAssertion) error {
	spec, ok := Field(a.Capability, a.Field)
	if !ok {
		return ErrUnknownField
	}
	if !validState(a.State) || !validSourceScope(a.Source, a.Scope) || a.ProviderRevision < 1 || a.ObservedAt.IsZero() ||
		!validBindingScope(a.Scope, a.BindingVerified, a.BindingRevision, a.ProviderRevision) || !validExpiry(a.ObservedAt, a.ValidUntil) {
		return ErrInvalidAssertion
	}
	if a.State != Supported && (a.Minimum != nil || a.Maximum != nil || a.Step != nil || len(a.Options) != 0) {
		return fmt.Errorf("%w: estado %q não pode declarar restrições", ErrInvalidAssertion, a.State)
	}
	numeric := spec.ValueType == TypeNumber || spec.ValueType == TypeInteger
	if !numeric && (a.Minimum != nil || a.Maximum != nil || a.Step != nil) {
		return fmt.Errorf("%w: campo %q não aceita limites numéricos", ErrInvalidAssertion, a.Field)
	}
	if spec.ValueType != TypeEnum && len(a.Options) > 0 {
		return fmt.Errorf("%w: campo %q não aceita opções", ErrInvalidAssertion, a.Field)
	}
	if a.Minimum != nil && !finite(*a.Minimum) || a.Maximum != nil && !finite(*a.Maximum) || a.Step != nil && (!finite(*a.Step) || *a.Step <= 0) {
		return fmt.Errorf("%w: limite não finito ou passo não positivo", ErrInvalidAssertion)
	}
	if a.Minimum != nil && a.Maximum != nil && *a.Minimum > *a.Maximum {
		return fmt.Errorf("%w: mínimo maior que máximo", ErrInvalidAssertion)
	}
	if spec.ValueType == TypeInteger {
		// float64 arredonda MaxInt64 para 2^63, portanto o teto é exclusivo.
		const integerUpperBound = float64(1 << 63)
		for _, value := range []*float64{a.Minimum, a.Maximum, a.Step} {
			if value != nil && math.Trunc(*value) != *value {
				return fmt.Errorf("%w: campo inteiro recebeu limite fracionário", ErrInvalidAssertion)
			}
			if value != nil && (*value < -integerUpperBound || *value >= integerUpperBound) {
				return fmt.Errorf("%w: limite fora do intervalo de inteiro de 64 bits", ErrInvalidAssertion)
			}
		}
	}
	seen := make(map[string]struct{}, len(a.Options))
	for _, option := range a.Options {
		if strings.TrimSpace(option.Value) == "" || strings.Trim(option.Value, " ") != option.Value || utf8.RuneCountInString(option.Value) > 512 || strings.ContainsRune(option.Value, '\x00') ||
			utf8.RuneCountInString(option.Label) > 512 || strings.ContainsRune(option.Label, '\x00') || !validState(option.State) {
			return fmt.Errorf("%w: opção enumerada inválida", ErrInvalidAssertion)
		}
		if _, exists := seen[option.Value]; exists {
			return fmt.Errorf("%w: opção enumerada repetida", ErrInvalidAssertion)
		}
		seen[option.Value] = struct{}{}
	}
	return nil
}

func validState(state SupportState) bool {
	return state == Supported || state == Unsupported || state == Unknown
}

func validSourceScope(source Source, scope Scope) bool {
	switch source {
	case SourceExecutionObservation, SourceEndpointDiscovery:
		return scope == ScopeConnection
	case SourceAppCuration:
		return scope == ScopeConnection || scope == ScopeExternalBinding
	case SourceOfficialCatalog, SourceThirdPartyCatalog:
		return scope == ScopeExternalBinding || scope == ScopeGeneric
	default:
		return false
	}
}

func validBindingScope(scope Scope, verified bool, bindingRevision, providerRevision int) bool {
	switch scope {
	case ScopeConnection, ScopeGeneric:
		return !verified && bindingRevision == 0
	case ScopeExternalBinding:
		return verified && bindingRevision == providerRevision
	default:
		return false
	}
}

func validExpiry(observedAt time.Time, validUntil *time.Time) bool {
	return validUntil == nil || validUntil.After(observedAt)
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func Resolve(now time.Time, providerRevision int, capabilities []CapabilityAssertion, fields []FieldAssertion) Resolution {
	result := Resolution{Capabilities: make(map[Capability]EffectiveFact), Fields: make(map[Capability]map[FieldKey]EffectiveField)}
	for _, capability := range capabilityKeys {
		result.Capabilities[capability] = EffectiveFact{State: Unknown}
	}
	for capability, fieldsForCapability := range catalog {
		result.Fields[capability] = make(map[FieldKey]EffectiveField, len(fieldsForCapability))
		for field := range fieldsForCapability {
			result.Fields[capability][field] = EffectiveField{EffectiveFact: EffectiveFact{State: Unknown}}
		}
	}

	byCapability := make(map[Capability][]CapabilityAssertion)
	for _, assertion := range capabilities {
		if ValidateCapabilityAssertion(assertion) == nil && applicable(now, providerRevision, assertion.Scope, assertion.ProviderRevision, assertion.BindingVerified, assertion.BindingRevision, assertion.ObservedAt, assertion.ValidUntil) {
			byCapability[assertion.Capability] = append(byCapability[assertion.Capability], assertion)
		}
	}
	for capability, assertions := range byCapability {
		result.Capabilities[capability] = resolveFacts(assertions, func(a CapabilityAssertion) SupportState { return a.State }, func(a CapabilityAssertion) string { return a.ID })
	}

	byField := make(map[Capability]map[FieldKey][]FieldAssertion)
	for _, assertion := range fields {
		if ValidateFieldAssertion(assertion) != nil || !applicable(now, providerRevision, assertion.Scope, assertion.ProviderRevision, assertion.BindingVerified, assertion.BindingRevision, assertion.ObservedAt, assertion.ValidUntil) {
			continue
		}
		if byField[assertion.Capability] == nil {
			byField[assertion.Capability] = make(map[FieldKey][]FieldAssertion)
		}
		byField[assertion.Capability][assertion.Field] = append(byField[assertion.Capability][assertion.Field], assertion)
	}
	for capability, fieldAssertions := range byField {
		for field, assertions := range fieldAssertions {
			result.Fields[capability][field] = resolveField(assertions)
		}
	}
	return result
}

func applicable(now time.Time, currentRevision int, scope Scope, revision int, verified bool, bindingRevision int, observedAt time.Time, expiry *time.Time) bool {
	if revision != currentRevision || observedAt.After(now) || (expiry != nil && !expiry.After(now)) {
		return false
	}
	switch scope {
	case ScopeConnection:
		return !verified
	case ScopeExternalBinding:
		return verified && bindingRevision == currentRevision
	default:
		return false
	}
}

func resolveFacts[T interface {
	getSource() Source
	getScope() Scope
	getObservedAt() time.Time
}](assertions []T, state func(T) SupportState, id func(T) string) EffectiveFact {
	sort.Slice(assertions, func(i, j int) bool { return stronger(assertions[i], assertions[j]) })
	if len(assertions) == 0 {
		return EffectiveFact{State: Unknown}
	}
	winner := assertions[0]
	base := state(winner)
	ids := make([]string, 0, len(assertions))
	conflict := false
	for _, candidate := range assertions {
		if !sameRankAndTime(winner, candidate) {
			break
		}
		ids = append(ids, id(candidate))
		if state(candidate) != base {
			conflict = true
		}
	}
	effective := factMeta(winner, base)
	if conflict {
		effective.State, effective.Conflict = Unknown, true
	}
	sort.Strings(ids)
	effective.EvidenceIDs = ids
	return effective
}

func resolveField(assertions []FieldAssertion) EffectiveField {
	sort.Slice(assertions, func(i, j int) bool { return stronger(assertions[i], assertions[j]) })
	winner := assertions[0]
	result := EffectiveField{EffectiveFact: factMeta(winner, winner.State), Minimum: copyFloat(winner.Minimum), Maximum: copyFloat(winner.Maximum), Step: copyFloat(winner.Step), Options: append([]FieldOption(nil), winner.Options...)}
	ids := make([]string, 0, len(assertions))
	for _, candidate := range assertions {
		if !sameRankAndTime(winner, candidate) {
			break
		}
		ids = append(ids, candidate.ID)
		if !sameFieldFact(winner, candidate) {
			result.State, result.Conflict = Unknown, true
		}
	}
	if result.Conflict {
		result.Minimum, result.Maximum, result.Step, result.Options = nil, nil, nil, nil
	}
	sort.Strings(ids)
	result.EvidenceIDs = ids
	sort.Slice(result.Options, func(i, j int) bool { return result.Options[i].Value < result.Options[j].Value })
	return result
}

type rankedAssertion interface {
	getSource() Source
	getScope() Scope
	getObservedAt() time.Time
}

func stronger(a, b rankedAssertion) bool {
	if scopeRank(a.getScope()) != scopeRank(b.getScope()) {
		return scopeRank(a.getScope()) > scopeRank(b.getScope())
	}
	if sourceRank(a.getSource()) != sourceRank(b.getSource()) {
		return sourceRank(a.getSource()) > sourceRank(b.getSource())
	}
	return a.getObservedAt().After(b.getObservedAt())
}

func (a CapabilityAssertion) getSource() Source        { return a.Source }
func (a CapabilityAssertion) getScope() Scope          { return a.Scope }
func (a CapabilityAssertion) getObservedAt() time.Time { return a.ObservedAt }
func (a FieldAssertion) getSource() Source             { return a.Source }
func (a FieldAssertion) getScope() Scope               { return a.Scope }
func (a FieldAssertion) getObservedAt() time.Time      { return a.ObservedAt }

func sameRankAndTime(a, b rankedAssertion) bool {
	return scopeRank(a.getScope()) == scopeRank(b.getScope()) && sourceRank(a.getSource()) == sourceRank(b.getSource()) && a.getObservedAt().Equal(b.getObservedAt())
}

func scopeRank(scope Scope) int {
	switch scope {
	case ScopeConnection:
		return 2
	case ScopeExternalBinding:
		return 1
	default:
		return 0
	}
}

func sourceRank(source Source) int {
	switch source {
	case SourceExecutionObservation:
		return 5
	case SourceEndpointDiscovery:
		return 4
	case SourceAppCuration:
		return 3
	case SourceOfficialCatalog:
		return 2
	case SourceThirdPartyCatalog:
		return 1
	default:
		return 0
	}
}

func factMeta(a rankedAssertion, state SupportState) EffectiveFact {
	return EffectiveFact{State: state, Source: a.getSource(), Scope: a.getScope(), ObservedAt: a.getObservedAt()}
}

func sameFieldFact(a, b FieldAssertion) bool {
	return a.State == b.State && equalFloat(a.Minimum, b.Minimum) && equalFloat(a.Maximum, b.Maximum) && equalFloat(a.Step, b.Step) && equalOptions(a.Options, b.Options)
}

func equalFloat(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func equalOptions(a, b []FieldOption) bool {
	if len(a) != len(b) {
		return false
	}
	left, right := append([]FieldOption(nil), a...), append([]FieldOption(nil), b...)
	sort.Slice(left, func(i, j int) bool { return left[i].Value < left[j].Value })
	sort.Slice(right, func(i, j int) bool { return right[i].Value < right[j].Value })
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func copyFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
