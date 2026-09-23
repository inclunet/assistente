package commandcontract

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"time"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandjson"
)

var fingerprintVersionPattern = regexp.MustCompile(`^v[1-9][0-9]*$`)

var (
	// ErrFingerprintKeyUnavailable não expõe o erro do provider, que pode
	// conter caminho de cofre ou material sensível.
	ErrFingerprintKeyUnavailable = errors.New("chave de fingerprint indisponível")
	ErrFingerprintInput          = errors.New("entrada de fingerprint inválida")
)

// SemanticProjection devolve uma cópia da projeção semântica antes do HMAC.
// Timestamps de provider, received_at, client_requested_at, auth_generation,
// authorization_decision_id, foreground_snapshot e fingerprints existentes
// ficam deliberadamente fora. source_occurred_at e source_replay_deadline são
// as exceções de tempo definidas em D2.1. A projeção não deve ser persistida,
// pois arguments/provenance podem conter dados transitórios; o envelope
// persistido deve usar resumos redigidos.
func (e Envelope) SemanticProjection() (map[string]any, error) {
	return e.semanticProjection(ResolutionExecute)
}

// SemanticProjectionFor é usada quando o ledger precisa projetar uma negativa
// ou supressão sem anexar a Definition do catálogo.
func (e Envelope) SemanticProjectionFor(mode ResolutionMode) (map[string]any, error) {
	return e.semanticProjection(mode)
}

func (e Envelope) semanticProjection(mode ResolutionMode) (map[string]any, error) {
	if err := validateEnvelope(&e, true, mode); err != nil {
		return nil, err
	}
	projection := map[string]any{
		"version":             EnvelopeVersion,
		"invocation_id":       e.InvocationID,
		"auth_context_type":   e.AuthContextType,
		"auth_context_id":     e.AuthContextID,
		"security_generation": e.SecurityGeneration,
		"actor_type":          e.ActorType,
		"actor_id":            e.ActorID,
		"registry_version":    e.RegistryVersion,
		"binding_ids":         slices.Clone(e.BindingIDs),
		"correlation_id":      e.CorrelationID,
	}
	if projection["binding_ids"] == nil {
		projection["binding_ids"] = []string{}
	}
	putString(projection, "command_id", e.CommandID)
	if e.Arguments == nil {
		projection["arguments"] = json.RawMessage(`{}`)
	} else {
		putDocument(projection, "arguments", e.Arguments)
	}
	putString(projection, "observed_trigger_type", e.ObservedTriggerType)
	putString(projection, "trigger_type", e.TriggerType)
	putDocument(projection, "trigger_spec", e.TriggerSpec)
	putString(projection, "user_id", e.UserID)
	putString(projection, "session_id", e.SessionID)
	putSource(projection, e.SourceType)
	putString(projection, "observer_type", e.ObserverType)
	// source_instance_id identifica uma geração do adapter, mas não uma
	// ocorrência. Em evento a ocorrência é a identidade estável e domina.
	if e.SourceEventID == nil {
		putString(projection, "source_instance_id", e.SourceInstanceID)
	}
	putString(projection, "source_event_id", e.SourceEventID)
	putTime(projection, "source_occurred_at", e.SourceOccurredAt)
	putString(projection, "source_replay_policy_generation", e.SourceReplayPolicyGeneration)
	putTime(projection, "source_replay_deadline", e.SourceReplayDeadline)
	putString(projection, "workspace_id", e.WorkspaceID)
	putString(projection, "global_config_generation", e.GlobalConfigGeneration)
	putString(projection, "workspace_config_generation", e.WorkspaceConfigGeneration)
	putString(projection, "active_layers_generation", e.ActiveLayersGeneration)
	putString(projection, "conversation_id", e.ConversationID)
	putString(projection, "turn_id", e.TurnID)
	putString(projection, "surface_type", e.SurfaceType)
	putString(projection, "surface_id", e.SurfaceID)
	putString(projection, "surface_snapshot_version", e.SurfaceSnapshotVersion)
	putString(projection, "context_version", e.ContextVersion)
	putString(projection, "source_profile_slug", e.SourceProfileSlug)
	putString(projection, "target_profile_slug", e.TargetProfileSlug)
	putString(projection, "delegation_fingerprint", e.DelegationFingerprint)
	putString(projection, "grant_generation", e.GrantGeneration)
	putString(projection, "job_id", e.JobID)
	putString(projection, "job_slug", e.JobSlug)
	putString(projection, "job_definition_fingerprint", e.JobDefinitionFingerprint)
	putString(projection, "run_id", e.RunID)
	putDocument(projection, "provenance", e.Provenance)
	return projection, nil
}

// SemanticProjectionWithDefinition acrescenta à projeção os dados de
// catálogo que não pertencem ao wire envelope: effect, decision e
// context_policy. A Definition deve vir do Registry confiável; Presentation é
// deliberadamente ignorada. O catálogo é validado novamente para impedir que
// um signer aceite context_policy=none como bypass de um comando mutável.
func (e Envelope) SemanticProjectionWithDefinition(definition commandcatalog.Definition) (map[string]any, error) {
	if err := validateDefinition(definition); err != nil {
		return nil, err
	}
	if err := validateEnvelopePolicy(e, definition, ResolutionExecute); err != nil {
		return nil, err
	}
	projection, err := e.semanticProjection(ResolutionExecute)
	if err != nil {
		return nil, err
	}
	projection["effect"] = string(definition.Effect)
	projection["decision"] = string(definition.Decision)
	projection["context_policy"] = contextPolicyProjection(definition.Context)
	return projection, nil
}

func (e Envelope) semanticProjectionWithDefinitionAndMode(definition commandcatalog.Definition, mode ResolutionMode) (map[string]any, error) {
	if err := validateDefinition(definition); err != nil {
		return nil, err
	}
	if err := validateEnvelopePolicy(e, definition, mode); err != nil {
		return nil, err
	}
	projection, err := e.semanticProjection(mode)
	if err != nil {
		return nil, err
	}
	projection["effect"] = string(definition.Effect)
	projection["decision"] = string(definition.Decision)
	projection["context_policy"] = contextPolicyProjection(definition.Context)
	projection["resolution"] = string(mode)
	return projection, nil
}

// CanonicalSemanticProjectionWithDefinition é a entrada canônica usada pelo
// signer I04.
func (e Envelope) CanonicalSemanticProjectionWithDefinition(definition commandcatalog.Definition) ([]byte, error) {
	projection, err := e.SemanticProjectionWithDefinition(definition)
	if err != nil {
		return nil, err
	}
	return commandjson.Marshal(projection)
}

// CanonicalSemanticProjection serializa a projeção com commandjson.Marshal.
// Não usa encoding/json diretamente: RFC 8785, números, Unicode, duplicatas
// e documentos JSON aninhados são responsabilidade do pacote compartilhado.
func (e Envelope) CanonicalSemanticProjection() ([]byte, error) {
	projection, err := e.semanticProjection(ResolutionExecute)
	if err != nil {
		return nil, err
	}
	return commandjson.Marshal(projection)
}

// CanonicalSemanticProjectionFor serializa a projeção estrutural do modo
// indicado. A projeção com Definition, usada para assinar, é produzida por
// CanonicalSemanticProjectionWithDefinition dentro de SignResolved.
func (e Envelope) CanonicalSemanticProjectionFor(mode ResolutionMode) ([]byte, error) {
	projection, err := e.semanticProjection(mode)
	if err != nil {
		return nil, err
	}
	return commandjson.Marshal(projection)
}

// SignResolved calcula o request fingerprint para um envelope já resolvido e
// devolve uma cópia com request_fingerprint_version/request_fingerprint. A
// versão vem do host e só é usada para selecionar a chave; não é lida do
// payload. O signer não autentica nem autoriza.
func (e Envelope) SignResolved(ctx context.Context, mode ResolutionMode, definition commandcatalog.Definition, keyVersion string, keys FingerprintKeyProvider) (Envelope, error) {
	if ctx == nil || keys == nil || !fingerprintVersionPattern.MatchString(keyVersion) {
		return Envelope{}, ErrFingerprintInput
	}
	if err := e.ValidateResolved(mode); err != nil {
		return Envelope{}, err
	}
	if e.RequestFingerprintVersion != nil || e.RequestFingerprint != nil {
		return Envelope{}, fmt.Errorf("%w: fingerprint já preenchido", ErrFingerprintInput)
	}
	if e.CommandID != nil && *e.CommandID != definition.ID {
		return Envelope{}, fmt.Errorf("%w: definição não corresponde ao comando", ErrFingerprintInput)
	}
	if mode == ResolutionExecute && (e.CommandID == nil || *e.CommandID != definition.ID) {
		return Envelope{}, fmt.Errorf("%w: execução exige command_id da definição", ErrFingerprintInput)
	}
	if err := ctx.Err(); err != nil {
		return Envelope{}, err
	}
	projection, err := e.semanticProjectionWithDefinitionAndMode(definition, mode)
	if err != nil {
		return Envelope{}, err
	}
	payload, err := commandjson.Marshal(projection)
	if err != nil {
		return Envelope{}, err
	}
	digest, err := hmacDocument(ctx, keyVersion, RequestFingerprintDomain, payload, keys)
	if err != nil {
		return Envelope{}, err
	}
	result := cloneEnvelope(e)
	result.RequestFingerprintVersion = stringPtr(keyVersion)
	result.RequestFingerprint = stringPtr(digest)
	return result, nil
}

// SignRefusal assina uma resolução denied sem Definition. É restrito a um
// envelope com ou sem command_id solicitado e inclui marcadores que tornam a
// identidade explicitamente não executável. O resultado continua sendo apenas um
// fingerprint; não é autorização nem receipt.
func (e Envelope) SignRefusal(ctx context.Context, keyVersion string, keys FingerprintKeyProvider) (Envelope, error) {
	if ctx == nil || keys == nil || !fingerprintVersionPattern.MatchString(keyVersion) || e.RequestFingerprintVersion != nil || e.RequestFingerprint != nil {
		return Envelope{}, ErrFingerprintInput
	}
	if err := e.ValidateResolved(ResolutionDenied); err != nil {
		return Envelope{}, err
	}
	projection, err := e.semanticProjection(ResolutionDenied)
	if err != nil {
		return Envelope{}, err
	}
	projection["resolution"] = string(ResolutionDenied)
	projection["effect_policy"] = "unresolved"
	payload, err := commandjson.Marshal(projection)
	if err != nil {
		return Envelope{}, err
	}
	digest, err := hmacDocument(ctx, keyVersion, RequestFingerprintDomain, payload, keys)
	if err != nil {
		return Envelope{}, err
	}
	result := cloneEnvelope(e)
	result.RequestFingerprintVersion = stringPtr(keyVersion)
	result.RequestFingerprint = stringPtr(digest)
	return result, nil
}

// ArgumentsHMAC calcula o fingerprint separado dos argumentos. A ausência de
// arguments representa o documento sem argumentos `{}` para preservar a
// identidade de comandos read sem parâmetros. O retorno não é persistência.
func (e Envelope) ArgumentsHMAC(ctx context.Context, keyVersion string, keys FingerprintKeyProvider) (string, error) {
	if ctx == nil || keys == nil || !fingerprintVersionPattern.MatchString(keyVersion) {
		return "", ErrFingerprintInput
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	document := []byte(`{}`)
	if e.Arguments != nil {
		canonical, err := commandjson.Canonicalize([]byte(*e.Arguments))
		if err != nil || len(canonical) == 0 || canonical[0] != '{' {
			return "", ErrFingerprintInput
		}
		document = canonical
	}
	return hmacDocument(ctx, keyVersion, ArgumentsFingerprintDomain, document, keys)
}

func hmacDocument(ctx context.Context, version, domain string, payload []byte, keys FingerprintKeyProvider) (string, error) {
	material, err := keys(ctx, "command-request-hmac:"+version)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}
	if err != nil || len(material) < 32 {
		return "", ErrFingerprintKeyUnavailable
	}
	key := append([]byte(nil), material...)
	defer clear(key)
	return commandjson.HMAC(key, domain, payload)
}

func putString(out map[string]any, name string, value *string) {
	if value != nil {
		out[name] = *value
	}
}

func putSource(out map[string]any, value *SourceType) {
	if value != nil {
		out["source_type"] = *value
	}
}

func putDocument(out map[string]any, name string, value *json.RawMessage) {
	if value == nil {
		return
	}
	canonical, err := commandjson.Canonicalize([]byte(*value))
	if err == nil {
		out[name] = json.RawMessage(canonical)
	}
}

func putTime(out map[string]any, name string, value *time.Time) {
	if value != nil {
		out[name] = value.UTC()
	}
}

func stringPtr(value string) *string { return &value }

// Clone devolve uma cópia independente, sem validar nem autorizar o envelope.
// Também preserva o valor zero usado por markers do ledger sem auditoria.
// A validação de ingressos continua pertencendo ao contrato de decode/sign.
func (e Envelope) Clone() Envelope { return cloneEnvelope(e) }

func cloneEnvelope(e Envelope) Envelope {
	result := e
	result.BindingIDs = slices.Clone(e.BindingIDs)
	cloneString := func(value *string) *string {
		if value == nil {
			return nil
		}
		copy := *value
		return &copy
	}
	result.CommandID = cloneString(e.CommandID)
	result.ObservedTriggerType = cloneString(e.ObservedTriggerType)
	result.TriggerType = cloneString(e.TriggerType)
	result.UserID = cloneString(e.UserID)
	result.AuthContextID = e.AuthContextID
	result.SessionID = cloneString(e.SessionID)
	result.SourceType = nil
	if e.SourceType != nil {
		source := *e.SourceType
		result.SourceType = &source
	}
	result.ObserverType = cloneString(e.ObserverType)
	result.SourceInstanceID = cloneString(e.SourceInstanceID)
	result.SourceEventID = cloneString(e.SourceEventID)
	result.SourceReplayPolicyGeneration = cloneString(e.SourceReplayPolicyGeneration)
	result.WorkspaceID = cloneString(e.WorkspaceID)
	result.GlobalConfigGeneration = cloneString(e.GlobalConfigGeneration)
	result.WorkspaceConfigGeneration = cloneString(e.WorkspaceConfigGeneration)
	result.ActiveLayersGeneration = cloneString(e.ActiveLayersGeneration)
	result.ConversationID = cloneString(e.ConversationID)
	result.TurnID = cloneString(e.TurnID)
	result.SurfaceType = cloneString(e.SurfaceType)
	result.SurfaceID = cloneString(e.SurfaceID)
	result.SurfaceSnapshotVersion = cloneString(e.SurfaceSnapshotVersion)
	result.ContextVersion = cloneString(e.ContextVersion)
	result.SourceProfileSlug = cloneString(e.SourceProfileSlug)
	result.TargetProfileSlug = cloneString(e.TargetProfileSlug)
	result.AuthorizationDecisionID = cloneString(e.AuthorizationDecisionID)
	result.DelegationFingerprint = cloneString(e.DelegationFingerprint)
	result.GrantGeneration = cloneString(e.GrantGeneration)
	result.JobID = cloneString(e.JobID)
	result.JobSlug = cloneString(e.JobSlug)
	result.JobDefinitionFingerprint = cloneString(e.JobDefinitionFingerprint)
	result.RunID = cloneString(e.RunID)
	result.RequestFingerprintVersion = cloneString(e.RequestFingerprintVersion)
	result.RequestFingerprint = cloneString(e.RequestFingerprint)
	cloneRaw := func(value *json.RawMessage) *json.RawMessage {
		if value == nil {
			return nil
		}
		copy := append(json.RawMessage(nil), (*value)...)
		return &copy
	}
	result.Arguments = cloneRaw(e.Arguments)
	result.TriggerSpec = cloneRaw(e.TriggerSpec)
	result.ForegroundSnapshot = cloneRaw(e.ForegroundSnapshot)
	result.Provenance = cloneRaw(e.Provenance)
	if e.ContextCapturedAtByProvider != nil {
		copy := make(map[string]time.Time, len(*e.ContextCapturedAtByProvider))
		for key, value := range *e.ContextCapturedAtByProvider {
			copy[key] = value
		}
		result.ContextCapturedAtByProvider = &copy
	}
	if e.SourceOccurredAt != nil {
		value := *e.SourceOccurredAt
		result.SourceOccurredAt = &value
	}
	if e.SourceReplayDeadline != nil {
		value := *e.SourceReplayDeadline
		result.SourceReplayDeadline = &value
	}
	if e.ClientRequestedAt != nil {
		value := *e.ClientRequestedAt
		result.ClientRequestedAt = &value
	}
	return result
}

func validateDefinition(definition commandcatalog.Definition) error {
	if definition.ID == "" {
		return fmt.Errorf("%w: definição ausente", ErrFingerprintInput)
	}
	registry, err := commandcatalog.New([]commandcatalog.Registration{{
		Definition: definition,
		Handler: commandcatalog.HandlerContract{
			Effect:                     definition.Effect,
			MutatesEffectiveCapability: definition.MutatesEffectiveCapability,
		},
	}})
	if err != nil || registry == nil {
		return fmt.Errorf("%w: definição inválida", ErrFingerprintInput)
	}
	return nil
}

func validateEnvelopePolicy(e Envelope, definition commandcatalog.Definition, mode ResolutionMode) error {
	if definition.Context.None {
		if e.ContextVersion != nil || e.ContextCapturedAtByProvider != nil || e.ForegroundSnapshot != nil {
			return fmt.Errorf("%w: context_policy none não aceita snapshot", ErrFingerprintInput)
		}
		return nil
	}
	// A denial still needs a stable request identity even when the missing
	// provider is precisely the reason for denying it. The policy itself enters
	// the projection; runtime freshness is an executor gate, not a signer gate.
	if mode == ResolutionDenied {
		return nil
	}
	if e.ContextVersion == nil || !validText(*e.ContextVersion, 4096) {
		return fmt.Errorf("%w: policy com providers exige context_version", ErrFingerprintInput)
	}
	for _, fact := range definition.Context.Facts {
		if fact.Mode != commandcatalog.MaxAge && fact.Mode != commandcatalog.EventSnapshot {
			continue
		}
		if e.ContextCapturedAtByProvider == nil {
			return fmt.Errorf("%w: policy temporal exige timestamps de provider", ErrFingerprintInput)
		}
		captured, ok := (*e.ContextCapturedAtByProvider)[fact.Provider]
		if !ok || !validTime(captured) {
			return fmt.Errorf("%w: timestamp ausente para provider %s", ErrFingerprintInput, fact.Provider)
		}
	}
	if hasForegroundFact(definition.Context) && e.ForegroundSnapshot == nil {
		return fmt.Errorf("%w: policy de foreground exige snapshot", ErrFingerprintInput)
	}
	return nil
}

func hasForegroundFact(policy commandcatalog.ContextPolicy) bool {
	for _, fact := range policy.Facts {
		if fact.Provider == "foreground" {
			return true
		}
	}
	return false
}

func contextPolicyProjection(policy commandcatalog.ContextPolicy) map[string]any {
	if policy.None {
		return map[string]any{"none": true}
	}
	facts := slices.Clone(policy.Facts)
	slices.SortFunc(facts, func(a, b commandcatalog.ContextFact) int {
		if a.Provider != b.Provider {
			if a.Provider < b.Provider {
				return -1
			}
			return 1
		}
		if a.Fact != b.Fact {
			if a.Fact < b.Fact {
				return -1
			}
			return 1
		}
		if a.Mode < b.Mode {
			return -1
		}
		if a.Mode > b.Mode {
			return 1
		}
		if a.MaxAgeMS < b.MaxAgeMS {
			return -1
		}
		if a.MaxAgeMS > b.MaxAgeMS {
			return 1
		}
		return 0
	})
	encoded := make([]map[string]any, len(facts))
	for i, fact := range facts {
		encoded[i] = map[string]any{"provider": fact.Provider, "fact": fact.Fact, "mode": fact.Mode}
		if fact.MaxAgeMS != 0 {
			encoded[i]["max_age_ms"] = fact.MaxAgeMS
		}
	}
	return map[string]any{"facts": encoded}
}
