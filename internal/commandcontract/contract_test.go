package commandcontract

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"assistente/internal/commandcatalog"
)

const (
	testInvocation = "01890f00-0000-7000-8000-000000000001"
	testUser       = "01890f00-0000-7000-8000-000000000002"
	testSession    = "01890f00-0000-7000-8000-000000000003"
	testWorkspace  = "ws-20260914-01"
	testJobID      = "01890f00-0000-7000-8000-000000000004"
	testEvent      = "01890f00-0000-7000-8000-000000000005"
	testInstance   = "01890f00-0000-7000-8000-000000000006"
)

func stringPointer(value string) *string         { return &value }
func sourcePointer(value SourceType) *SourceType { return &value }
func rawPointer(value string) *json.RawMessage   { raw := json.RawMessage(value); return &raw }
func timePointer(value time.Time) *time.Time     { return &value }

func resolvedDirectFixture() Envelope {
	return Envelope{
		Version: 1, InvocationID: testInvocation,
		CommandID: stringPointer("workspace.tab.new"),
		UserID:    stringPointer(testUser), AuthContextType: AuthLocalSession,
		AuthContextID: testSession, AuthGeneration: "auth:1", SessionID: stringPointer(testSession),
		SecurityGeneration: "security:1", ActorType: ActorUser, ActorID: testUser,
		SourceType: sourcePointer(SourcePalette), BindingIDs: []string{}, RegistryVersion: "registry:1",
		GlobalConfigGeneration: stringPointer("global:1"), ActiveLayersGeneration: stringPointer("layers:1"),
		WorkspaceID: stringPointer(testWorkspace), WorkspaceConfigGeneration: stringPointer("workspace:1"),
		CorrelationID: "correlation:1", ReceivedAt: time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC),
	}
}

func readDefinition() commandcatalog.Definition {
	return commandcatalog.Definition{
		ID: "workspace.tab.new", Effect: commandcatalog.Read, Decision: commandcatalog.NoDecision,
		AllowedSources: []commandcatalog.Source{commandcatalog.Palette},
		Context:        commandcatalog.ContextPolicy{None: true},
	}
}

func testKeyProvider(_ context.Context, name string) ([]byte, error) {
	if name != "command-request-hmac:v1" {
		return nil, ErrFingerprintKeyUnavailable
	}
	return bytes.Repeat([]byte{'k'}, 32), nil
}

func TestValidateResolvedFieldGroups(t *testing.T) {
	base := resolvedDirectFixture()
	cases := map[string]func(*Envelope){
		"surface parcial":           func(e *Envelope) { e.SurfaceID = stringPointer("surface") },
		"conversation parcial":      func(e *Envelope) { e.ConversationID = stringPointer(testWorkspace) },
		"workspace sem geração":     func(e *Envelope) { e.WorkspaceConfigGeneration = nil },
		"binding nil pós resolução": func(e *Envelope) { e.BindingIDs = nil },
		"user actor divergente":     func(e *Envelope) { e.ActorID = testSession },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			value := base
			mutate(&value)
			if err := value.ValidateResolved(ResolutionExecute); err == nil {
				t.Fatal("envelope estruturalmente inválido foi aceito")
			}
		})
	}
	base.JobID = stringPointer(testJobID)
	base.JobSlug = stringPointer("daily-report")
	base.JobDefinitionFingerprint = stringPointer("job-fp")
	base.RunID = stringPointer("run_1")
	if err := base.ValidateResolved(ResolutionExecute); err != nil {
		t.Fatalf("job UUID + slug válidos rejeitados: %v", err)
	}
}

func TestValidateIngressPhysicalTriggerBeforeResolution(t *testing.T) {
	e := resolvedDirectFixture()
	e.CommandID = nil
	e.SourceType = nil
	e.TriggerType = nil
	e.ObservedTriggerType = stringPointer(string(SourceKeyboardGlobal))
	e.TriggerSpec = rawPointer(`{"version":1,"code":"KeyK","modifiers":[]}`)
	e.SourceInstanceID = stringPointer(testInstance)
	e.SourceEventID = stringPointer(testEvent)
	e.ObserverType = stringPointer("os.keyboard")
	e.BindingIDs = nil
	e.ReceivedAt = time.Time{}
	if err := e.ValidateIngress(); err != nil {
		t.Fatalf("trigger físico de ingresso rejeitado: %v", err)
	}

	bad := e
	bad.RequestFingerprint = stringPointer("old")
	if err := bad.ValidateIngress(); err == nil {
		t.Fatal("fingerprint de ingresso aceito")
	}
	bad = e
	bad.SourceReplayPolicyGeneration = stringPointer("epoch:1")
	if err := bad.ValidateIngress(); err == nil {
		t.Fatal("epoch de replay de cliente aceito")
	}
}

func TestValidateDeniedWithoutResolvedSource(t *testing.T) {
	e := resolvedDirectFixture()
	e.CommandID = nil
	e.SourceType = nil
	e.TriggerType = nil
	e.ObservedTriggerType = stringPointer(string(SourcePalette))
	e.TriggerSpec = rawPointer(`{"version":1,"selection":"missing"}`)
	if err := e.ValidateResolved(ResolutionDenied); err != nil {
		t.Fatalf("denied sem vencedor deve manter source nulo: %v", err)
	}
}

func TestValidateDeniedEventWithoutResolvedSourceRetainsReplayFields(t *testing.T) {
	e := resolvedDirectFixture()
	e.CommandID = nil
	e.SourceType = nil
	e.TriggerType = nil
	e.ObservedTriggerType = stringPointer(string(SourceEvent))
	e.TriggerSpec = rawPointer(`{"version":1,"event":"done"}`)
	e.SourceInstanceID = stringPointer(testInstance)
	e.SourceEventID = stringPointer(testEvent)
	e.ObserverType = stringPointer("job.outbox")
	e.SourceOccurredAt = timePointer(e.ReceivedAt.Add(-time.Minute))
	e.Provenance = rawPointer(`{"version":1,"_source":"job"}`)
	e.SourceReplayPolicyGeneration = stringPointer("epoch:1")
	deadline := e.ReceivedAt.Add(time.Hour)
	e.SourceReplayDeadline = &deadline
	if err := e.ValidateResolved(ResolutionDenied); err != nil {
		t.Fatalf("evento denied sem vencedor válido rejeitado: %v", err)
	}

	missingEpoch := e
	missingEpoch.SourceReplayPolicyGeneration = nil
	if err := missingEpoch.ValidateResolved(ResolutionDenied); err == nil {
		t.Fatal("evento denied sem epoch de replay foi aceito")
	}

	missingDeadline := e
	missingDeadline.SourceReplayDeadline = nil
	if err := missingDeadline.ValidateResolved(ResolutionDenied); err == nil {
		t.Fatal("evento denied sem deadline de replay foi aceito")
	}
}

func TestSuppressOnlyEmptyArguments(t *testing.T) {
	e := resolvedDirectFixture()
	e.CommandID = nil
	e.TriggerType = stringPointer(string(SourcePalette))
	e.TriggerSpec = rawPointer(`{"version":1,"selection":"workspace.tab.new"}`)
	e.BindingIDs = []string{"builtin.workspace-tab-new"}
	if err := e.ValidateResolved(ResolutionSuppress); err != nil {
		t.Fatalf("suppress válido rejeitado: %v", err)
	}
	e.Arguments = rawPointer(`{"secret":"não pode"}`)
	if err := e.ValidateResolved(ResolutionSuppress); err == nil {
		t.Fatal("suppress aceitou argumentos")
	}
}

func TestStrictEnvelopeJSONNamesAndNull(t *testing.T) {
	e := resolvedDirectFixture()
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Envelope
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("JSON canônico válido rejeitado: %v", err)
	}
	for _, mutation := range []string{
		strings.Replace(string(raw), `"version":1`, `"Version":1`, 1),
		strings.Replace(string(raw), `"command_id":"workspace.tab.new"`, `"command_id":null`, 1),
		strings.TrimSuffix(string(raw), "}") + `,"unknown":true}`,
	} {
		if err := json.Unmarshal([]byte(mutation), &decoded); err == nil {
			t.Fatalf("JSON inválido aceito: %s", mutation)
		}
	}
}

func TestUnmarshalPreservesVersionedDocumentLexeme(t *testing.T) {
	for _, version := range []string{"1.0", "1e0"} {
		t.Run(version, func(t *testing.T) {
			e := resolvedDirectFixture()
			e.TriggerType = stringPointer(string(SourcePalette))
			e.TriggerSpec = rawPointer(`{"version":` + version + `,"selection":"workspace.tab.new"}`)
			raw, err := json.Marshal(e)
			if err != nil {
				t.Fatal(err)
			}
			var decoded Envelope
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatalf("envelope com documento lexicalmente válido não decodificou: %v", err)
			}
			if err := decoded.ValidateResolved(ResolutionExecute); err == nil {
				t.Fatalf("version %s de documento versionado foi reinterpretada como 1", version)
			}
		})
	}
}

func TestSharedLexicalCorpusForArguments(t *testing.T) {
	data, err := os.ReadFile("../commandjson/testdata/lexical.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name  string `json:"name"`
		Raw   string `json:"raw"`
		Valid bool   `json:"valid"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			e := resolvedDirectFixture()
			e.Arguments = rawPointer(tc.Raw)
			got := e.ValidateResolved(ResolutionExecute)
			if (got == nil) != tc.Valid {
				t.Fatalf("corpus lexical divergente: valid=%v err=%v", tc.Valid, got)
			}
		})
	}
}

func TestVersionedDocumentsRequireVersionOne(t *testing.T) {
	base := resolvedDirectFixture()
	for name, set := range map[string]func(*Envelope){
		"trigger_spec": func(e *Envelope) { e.TriggerSpec = rawPointer(`{"selection":"workspace.tab.new"}`) },
		"provenance":   func(e *Envelope) { e.Provenance = rawPointer(`{"_source":"job"}`) },
	} {
		t.Run(name+" sem version", func(t *testing.T) {
			e := base
			set(&e)
			if err := e.ValidateResolved(ResolutionExecute); err == nil {
				t.Fatal("documento versionado sem version foi aceito")
			}
			for _, version := range []string{`2`, `1.0`, `1e0`, `"1"`, `null`} {
				value := base
				if name == "trigger_spec" {
					value.TriggerSpec = rawPointer(`{"version":` + version + `,"selection":"workspace.tab.new"}`)
				} else {
					value.Provenance = rawPointer(`{"version":` + version + `,"_source":"job"}`)
				}
				if err := value.ValidateResolved(ResolutionExecute); err == nil {
					t.Fatalf("version %s foi aceita", version)
				}
			}
		})
	}

	arguments := base
	arguments.Arguments = rawPointer(`{"foo":"bar"}`)
	if err := arguments.ValidateResolved(ResolutionExecute); err != nil {
		t.Fatalf("arguments sem version foi incorretamente rejeitado: %v", err)
	}
}

func TestSemanticProjectionRulesAndDeepClone(t *testing.T) {
	base := resolvedDirectFixture()
	base.Arguments = nil
	base.ForegroundSnapshot = rawPointer(`{"title":"texto transitório","captured_at":"2026-09-14T12:00:00Z"}`)
	base.ContextCapturedAtByProvider = &map[string]time.Time{"workspace": base.ReceivedAt}
	projection, err := base.CanonicalSemanticProjection()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(projection, []byte("foreground_snapshot")) || bytes.Contains(projection, []byte("captured_at")) {
		t.Fatalf("snapshot/timestamp de provider vazou na identidade: %s", projection)
	}
	base.Arguments = rawPointer(`{}`)
	other, err := base.CanonicalSemanticProjection()
	if err != nil || !bytes.Equal(projection, other) {
		t.Fatalf("arguments ausente e {} não normalizaram igual: %s / %s", projection, other)
	}

	base.ContextCapturedAtByProvider = nil
	base.ForegroundSnapshot = nil
	keys := testKeyProvider
	signed, err := base.SignResolved(context.Background(), ResolutionExecute, readDefinition(), "v1", keys)
	if err != nil {
		t.Fatal(err)
	}
	if signed.RequestFingerprint == nil || signed.RequestFingerprintVersion == nil {
		t.Fatal("par de fingerprint ausente")
	}
	*signed.CommandID = "mutated-after-sign"
	if *base.CommandID != "workspace.tab.new" {
		t.Fatal("signer alterou envelope de entrada")
	}
	changed := base
	occurred := base.ReceivedAt.Add(time.Minute)
	changed.SourceType = sourcePointer(SourceEvent)
	changed.TriggerType = stringPointer(string(SourceEvent))
	changed.TriggerSpec = rawPointer(`{"version":1,"event":"done"}`)
	changed.SourceInstanceID = stringPointer(testInstance)
	changed.SourceEventID = stringPointer(testEvent)
	changed.ObserverType = stringPointer("job.outbox")
	changed.SourceOccurredAt = &occurred
	deadline := occurred.Add(time.Hour)
	changed.SourceReplayDeadline = &deadline
	changed.SourceReplayPolicyGeneration = stringPointer("epoch:2")
	changed.Provenance = rawPointer(`{"version":1,"_source":"job"}`)
	changed.SourceType = sourcePointer(SourceEvent)
	changed.BindingIDs = []string{"builtin.workspace-tab-new"}
	changed.CommandID = stringPointer("workspace.tab.new")
	eventDefinition := readDefinition()
	eventDefinition.AllowedSources = []commandcatalog.Source{commandcatalog.Event}
	mutated, err := changed.SignResolved(context.Background(), ResolutionExecute, eventDefinition, "v1", keys)
	if err != nil {
		t.Fatal(err)
	}
	if *mutated.RequestFingerprint == *signed.RequestFingerprint {
		t.Fatal("source ocorrido/deadline não influenciaram fingerprint")
	}
}

func TestSignResolvedNãoAutorizaESignRefusalNãoÉExecutável(t *testing.T) {
	e := resolvedDirectFixture()
	definition := readDefinition()
	definition.AllowedSources = []commandcatalog.Source{commandcatalog.CLI}
	if _, err := e.SignResolved(context.Background(), ResolutionExecute, definition, "v1", testKeyProvider); err != nil {
		// AllowsSource não é autorização: a assinatura deve ser possível para o
		// executor negar depois da reserva.
		t.Fatalf("signer confundiu fingerprint com autorização: %v", err)
	}
	refusal := e
	refusal.CommandID = stringPointer("workspace.unknown")
	refusal.SourceType = nil
	refusal.TriggerType = stringPointer(string(SourcePalette))
	refusal.TriggerSpec = rawPointer(`{"version":1,"selection":"missing"}`)
	signed, err := refusal.SignRefusal(context.Background(), "v1", testKeyProvider)
	if err != nil {
		t.Fatal(err)
	}
	if signed.RequestFingerprint == nil {
		t.Fatal("refusal não recebeu fingerprint")
	}
	if signed.CommandID == nil || *signed.CommandID != "workspace.unknown" {
		t.Fatal("refusal perdeu a identidade do comando desconhecido")
	}
	if _, err := signed.SignResolved(context.Background(), ResolutionExecute, definition, "v1", testKeyProvider); err == nil {
		t.Fatal("fingerprint de refusal pôde ser reutilizado como execução")
	}
}

func TestSigningPolicyCannotBypassContextNone(t *testing.T) {
	e := resolvedDirectFixture()
	definition := readDefinition()
	definition.Effect = commandcatalog.Write
	definition.Context = commandcatalog.ContextPolicy{None: true}
	if _, err := e.SignResolved(context.Background(), ResolutionExecute, definition, "v1", testKeyProvider); err == nil {
		t.Fatal("policy none mutável foi assinada")
	}
}

func TestDefinitionPolicyEffectAndDecisionEnterFingerprint(t *testing.T) {
	e := resolvedDirectFixture()
	read := readDefinition()
	readFingerprint, err := e.SignResolved(context.Background(), ResolutionExecute, read, "v1", testKeyProvider)
	if err != nil {
		t.Fatal(err)
	}
	write := read
	write.Effect = commandcatalog.Write
	write.Decision = commandcatalog.Interactive
	write.HasMutableTarget = true
	write.AllowedSources = []commandcatalog.Source{commandcatalog.Palette}
	write.Context = commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{Provider: "workspace", Fact: "active", Mode: commandcatalog.ExactVersion}}}
	e.ContextVersion = stringPointer("context:1")
	writeFingerprint, err := e.SignResolved(context.Background(), ResolutionExecute, write, "v1", testKeyProvider)
	if err != nil {
		t.Fatal(err)
	}
	if *readFingerprint.RequestFingerprint == *writeFingerprint.RequestFingerprint {
		t.Fatal("effect/decision/context_policy não alteraram fingerprint")
	}
	presented := read
	presented.Presentation = &commandcatalog.Presentation{Version: "1", Locales: map[string]commandcatalog.LocalizedMetadata{
		"pt-BR": {Name: "Abrir", Description: "Abrir", Category: "Workspace"},
		"en":    {Name: "Open", Description: "Open", Category: "Workspace"},
		"es":    {Name: "Abrir", Description: "Abrir", Category: "Workspace"},
	}}
	presentationEnvelope := resolvedDirectFixture()
	presentedFingerprint, err := presentationEnvelope.SignResolved(context.Background(), ResolutionExecute, presented, "v1", testKeyProvider)
	if err != nil {
		t.Fatal(err)
	}
	if *readFingerprint.RequestFingerprint != *presentedFingerprint.RequestFingerprint {
		t.Fatal("presentation visual alterou fingerprint semântico")
	}
}

func TestDeniedMissingProviderAindaRecebeFingerprint(t *testing.T) {
	e := resolvedDirectFixture()
	definition := readDefinition()
	definition.Context = commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{Provider: "workspace", Fact: "active", Mode: commandcatalog.MaxAge, MaxAgeMS: 1000}}}
	// Ausência de context_version/captured_at é a causa operacional da recusa;
	// não pode impedir a identidade que será reservada para a negativa.
	signed, err := e.SignResolved(context.Background(), ResolutionDenied, definition, "v1", testKeyProvider)
	if err != nil {
		t.Fatalf("denied por provider ausente não foi fingerprintado: %v", err)
	}
	if signed.RequestFingerprint == nil || signed.RequestFingerprintVersion == nil {
		t.Fatal("fingerprint de denied ausente")
	}
}
