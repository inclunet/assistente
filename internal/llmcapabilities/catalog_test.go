package llmcapabilities

import (
	"math"
	"reflect"
	"testing"
	"time"
)

func TestValidateFieldAssertionRejectsUnknownOrUntypedData(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	minimum, maximum, step := 0.0, 2.0, 0.5
	base := FieldAssertion{
		ID: "valid", Capability: CapabilityChat, Field: FieldTemperature,
		State: Supported, Source: SourceExecutionObservation, Scope: ScopeConnection,
		ProviderRevision: 1, Minimum: &minimum, Maximum: &maximum, Step: &step, ObservedAt: now,
	}
	if err := ValidateFieldAssertion(base); err != nil {
		t.Fatalf("afirmação válida recusada: %v", err)
	}

	cases := []struct {
		name string
		edit func(*FieldAssertion)
	}{
		{"capability desconhecida", func(a *FieldAssertion) { a.Capability = "arbitrary" }},
		{"campo não cadastrado", func(a *FieldAssertion) { a.Field = "prompt" }},
		{"campo na capability errada", func(a *FieldAssertion) { a.Capability, a.Field = CapabilityChat, FieldVoice }},
		{"tipo textual com limite numérico", func(a *FieldAssertion) { a.Capability, a.Field, a.Minimum = CapabilitySTT, FieldLanguage, &minimum }},
		{"enum com NaN", func(a *FieldAssertion) {
			a.Capability, a.Field, a.Minimum = CapabilityTTS, FieldVoice, floatPointer(math.NaN())
		}},
		{"inteiro com fração", func(a *FieldAssertion) { a.Field, a.Minimum = FieldMaxOutputTokens, floatPointer(0.5) }},
		{"range invertido", func(a *FieldAssertion) { a.Minimum, a.Maximum = floatPointer(3), floatPointer(2) }},
		{"passo zero", func(a *FieldAssertion) { a.Step = floatPointer(0) }},
		{"unsupported com opções", func(a *FieldAssertion) {
			a.Capability, a.Field, a.State, a.Options = CapabilityTTS, FieldVoice, Unsupported, []FieldOption{{Value: "alloy", State: Supported}}
		}},
		{"enum sem chave única", func(a *FieldAssertion) {
			a.Capability, a.Field, a.Options = CapabilityTTS, FieldVoice, []FieldOption{{Value: "alloy", State: Supported}, {Value: "alloy", State: Unsupported}}
		}},
		{"scope não verificado", func(a *FieldAssertion) { a.Scope = ScopeExternalBinding }},
		{"expiração anterior à observação", func(a *FieldAssertion) { expiry := now; a.ValidUntil = &expiry }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			claim := base
			test.edit(&claim)
			if err := ValidateFieldAssertion(claim); err == nil {
				t.Fatal("afirmação inválida foi aceita")
			}
		})
	}
}

func TestResolveCapabilityHonorsRevisionScopePrecedenceExpiryAndConflicts(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	old := now.Add(-time.Hour)
	expired := now.Add(-time.Minute)
	future := now.Add(time.Hour)
	claims := []CapabilityAssertion{
		{ID: "stale", Capability: CapabilityChat, State: Unsupported, Source: SourceExecutionObservation, Scope: ScopeConnection, ProviderRevision: 1, ObservedAt: now},
		{ID: "expired", Capability: CapabilityChat, State: Unsupported, Source: SourceExecutionObservation, Scope: ScopeConnection, ProviderRevision: 2, ObservedAt: old, ValidUntil: &expired},
		{ID: "generic", Capability: CapabilityChat, State: Unsupported, Source: SourceOfficialCatalog, Scope: ScopeGeneric, ProviderRevision: 2, ObservedAt: now},
		{ID: "external", Capability: CapabilityChat, State: Unsupported, Source: SourceOfficialCatalog, Scope: ScopeExternalBinding, ProviderRevision: 2, BindingVerified: true, BindingRevision: 2, ObservedAt: now},
		{ID: "runtime", Capability: CapabilityChat, State: Supported, Source: SourceExecutionObservation, Scope: ScopeConnection, ProviderRevision: 2, ObservedAt: now},
		{ID: "tie-conflict", Capability: CapabilityChat, State: Unsupported, Source: SourceExecutionObservation, Scope: ScopeConnection, ProviderRevision: 2, ObservedAt: now},
		{ID: "newer", Capability: CapabilityTools, State: Unsupported, Source: SourceEndpointDiscovery, Scope: ScopeConnection, ProviderRevision: 2, ObservedAt: future},
	}
	result := Resolve(now, 2, claims, nil)
	if got := result.Capabilities[CapabilityChat]; got.State != Unknown || !got.Conflict || !reflect.DeepEqual(got.EvidenceIDs, []string{"runtime", "tie-conflict"}) {
		t.Fatalf("conflito no topo deveria resolver como unknown: %+v", got)
	}
	if got := result.Capabilities[CapabilityTools]; got.State != Unknown {
		t.Fatalf("observação futura não deveria governar a resolução: %+v", got)
	}
	if got := result.Capabilities[CapabilityReasoning]; got.State != Unknown {
		t.Fatalf("ausência de fato precisa permanecer unknown: %+v", got)
	}
}

func TestResolveFieldsDoesNotMergeOptionsAcrossSources(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	minimum, maximum := 0.25, 4.0
	fields := []FieldAssertion{
		{ID: "curated", Capability: CapabilityTTS, Field: FieldVoice, State: Supported, Source: SourceAppCuration, Scope: ScopeConnection, ProviderRevision: 4, ObservedAt: now, Options: []FieldOption{{Value: "nova", Label: "Nova", State: Supported}}},
		{ID: "endpoint", Capability: CapabilityTTS, Field: FieldVoice, State: Supported, Source: SourceEndpointDiscovery, Scope: ScopeConnection, ProviderRevision: 4, ObservedAt: now.Add(-time.Minute), Options: []FieldOption{{Value: "alloy", Label: "Alloy", State: Supported}}},
		{ID: "temperature", Capability: CapabilityChat, Field: FieldTemperature, State: Supported, Source: SourceEndpointDiscovery, Scope: ScopeConnection, ProviderRevision: 4, ObservedAt: now, Minimum: &minimum, Maximum: &maximum},
	}
	resolved := Resolve(now, 4, nil, fields)
	voices := resolved.Fields[CapabilityTTS][FieldVoice]
	if voices.State != Supported || !reflect.DeepEqual(voices.EvidenceIDs, []string{"endpoint"}) || len(voices.Options) != 1 || voices.Options[0].Value != "alloy" {
		t.Fatalf("opções de fontes diferentes foram misturadas: %+v", voices)
	}
	temperature := resolved.Fields[CapabilityChat][FieldTemperature]
	if temperature.Minimum == nil || *temperature.Minimum != minimum || temperature.Maximum == nil || *temperature.Maximum != maximum {
		t.Fatalf("limites tipados não foram preservados: %+v", temperature)
	}
}

func TestResolveExternalFactsHonorsApplicationCurationPrecedence(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	claims := []CapabilityAssertion{
		{ID: "official", Capability: CapabilityReasoning, State: Unsupported, Source: SourceOfficialCatalog, Scope: ScopeExternalBinding, ProviderRevision: 7, BindingVerified: true, BindingRevision: 7, ObservedAt: now},
		{ID: "curated", Capability: CapabilityReasoning, State: Supported, Source: SourceAppCuration, Scope: ScopeExternalBinding, ProviderRevision: 7, BindingVerified: true, BindingRevision: 7, ObservedAt: now},
		{ID: "third-party", Capability: CapabilityReasoning, State: Unsupported, Source: SourceThirdPartyCatalog, Scope: ScopeExternalBinding, ProviderRevision: 7, BindingVerified: true, BindingRevision: 7, ObservedAt: now},
	}
	got := Resolve(now, 7, claims, nil).Capabilities[CapabilityReasoning]
	if got.State != Supported || got.Source != SourceAppCuration || !reflect.DeepEqual(got.EvidenceIDs, []string{"curated"}) {
		t.Fatalf("curadoria versionada não venceu as fontes externas: %+v", got)
	}
}

func TestResolveIgnoresFutureCapabilityAndFieldObservations(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Minute)
	capabilities := []CapabilityAssertion{{
		ID: "future-capability", Capability: CapabilityTTS, State: Unsupported,
		Source: SourceEndpointDiscovery, Scope: ScopeConnection, ProviderRevision: 1, ObservedAt: future,
	}}
	fields := []FieldAssertion{{
		ID: "future-field", Capability: CapabilityTTS, Field: FieldVoice, State: Supported,
		Source: SourceEndpointDiscovery, Scope: ScopeConnection, ProviderRevision: 1,
		ObservedAt: future,
	}}
	resolved := Resolve(now, 1, capabilities, fields)
	if got := resolved.Capabilities[CapabilityTTS]; got.State != Unknown {
		t.Fatalf("capability futura foi aceita: %+v", got)
	}
	if got := resolved.Fields[CapabilityTTS][FieldVoice]; got.State != Unknown {
		t.Fatalf("campo futuro foi aceito: %+v", got)
	}
}

func TestResolveFieldTieWithDifferentConstraintsBecomesUnknown(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	left, right := 0.1, 0.2
	claims := []FieldAssertion{
		{ID: "a", Capability: CapabilityChat, Field: FieldTemperature, State: Supported, Source: SourceEndpointDiscovery, Scope: ScopeConnection, ProviderRevision: 1, ObservedAt: now, Minimum: &left},
		{ID: "b", Capability: CapabilityChat, Field: FieldTemperature, State: Supported, Source: SourceEndpointDiscovery, Scope: ScopeConnection, ProviderRevision: 1, ObservedAt: now, Minimum: &right},
	}
	got := Resolve(now, 1, nil, claims).Fields[CapabilityChat][FieldTemperature]
	if got.State != Unknown || !got.Conflict {
		t.Fatalf("limites conflitantes deveriam produzir unknown: %+v", got)
	}
}

func floatPointer(value float64) *float64 { return &value }
