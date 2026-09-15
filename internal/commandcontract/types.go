// Package commandcontract define o envelope versionado da AEP-0103 D2.1.
//
// Este pacote valida forma, nulabilidade e coerência estrutural. Ele não
// autentica principal, origem, workspace, decisão, proveniência ou qualquer
// outro valor recebido: essas decisões pertencem à borda confiável e ao
// executor. O envelope pode transportar documentos JSON transitórios, mas
// não é um formato de persistência e não deve ser gravado com segredos.
package commandcontract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandjson"
)

const (
	// EnvelopeVersion é a única versão de envelope aceita nesta implementação.
	EnvelopeVersion = 1

	// RequestFingerprintDomain separa o HMAC de request dos demais HMACs.
	// O domínio é passado ao framing de HMAC de commandjson.
	RequestFingerprintDomain = "assistente.command.request.v1"
	// ArgumentsFingerprintDomain separa o HMAC dos argumentos do request.
	ArgumentsFingerprintDomain = "assistente.command.arguments.v1"
)

// SourceType é a taxonomia fechada de origens da AEP-0103 D3.
type SourceType string

const (
	SourceKeyboardLocal  SourceType = "keyboard.local"
	SourceKeyboardGlobal SourceType = "keyboard.global"
	SourceStreamDeck     SourceType = "streamdeck.key"
	SourcePalette        SourceType = "palette"
	SourceUI             SourceType = "ui.action"
	SourceChat           SourceType = "chat"
	SourceCLI            SourceType = "cli"
	SourceEvent          SourceType = "event"
	SourceSystem         SourceType = "system"
)

// AuthContextType identifica a classe de contexto que o host autenticou.
// O valor não concede autenticação e nunca deve ser escolhido por um adapter.
type AuthContextType string

const (
	AuthLocalSession  AuthContextType = "local_session"
	AuthExternalToken AuthContextType = "external_token"
	AuthJobService    AuthContextType = "job_service"
	AuthSystem        AuthContextType = "system"
)

// ActorType é ortogonal a SourceType.
type ActorType string

const (
	ActorUser       ActorType = "user"
	ActorAgent      ActorType = "agent"
	ActorAutomation ActorType = "automation"
)

// ResolutionMode informa o resultado do resolvedor ao validar o envelope
// interno. Suppress é terminal e não cria CommandInvocation.
type ResolutionMode string

const (
	ResolutionExecute  ResolutionMode = "execute"
	ResolutionSuppress ResolutionMode = "suppress"
	ResolutionDenied   ResolutionMode = "denied"
)

// Envelope é o CommandInvocation completo da AEP-0103 D2.1.
//
// Campos opcionais são ponteiros para distinguir ausência de zero. Documentos
// JSON são RawMessage somente para transporte; antes de qualquer uso semântico
// eles são canonicalizados por internal/commandjson. BindingIDs é uma lista
// não-nula: no ingresso pode aguardar resolução, mas depois dela deve ser [] ou
// conter todos os bindings contribuintes em ordem determinística.
type Envelope struct {
	Version      int    `json:"version"`
	InvocationID string `json:"invocation_id"`

	CommandID *string          `json:"command_id,omitempty"`
	Arguments *json.RawMessage `json:"arguments,omitempty"`

	ObservedTriggerType *string          `json:"observed_trigger_type,omitempty"`
	TriggerType         *string          `json:"trigger_type,omitempty"`
	TriggerSpec         *json.RawMessage `json:"trigger_spec,omitempty"`

	UserID             *string         `json:"user_id,omitempty"`
	AuthContextType    AuthContextType `json:"auth_context_type"`
	AuthContextID      string          `json:"auth_context_id"`
	AuthGeneration     string          `json:"auth_generation"`
	SessionID          *string         `json:"session_id,omitempty"`
	SecurityGeneration string          `json:"security_generation"`

	ActorType ActorType `json:"actor_type"`
	ActorID   string    `json:"actor_id"`

	SourceType       *SourceType `json:"source_type,omitempty"`
	ObserverType     *string     `json:"observer_type,omitempty"`
	SourceInstanceID *string     `json:"source_instance_id,omitempty"`
	SourceEventID    *string     `json:"source_event_id,omitempty"`

	SourceOccurredAt             *time.Time `json:"source_occurred_at,omitempty"`
	SourceReplayPolicyGeneration *string    `json:"source_replay_policy_generation,omitempty"`
	SourceReplayDeadline         *time.Time `json:"source_replay_deadline,omitempty"`

	WorkspaceID               *string  `json:"workspace_id,omitempty"`
	BindingIDs                []string `json:"binding_ids"`
	RegistryVersion           string   `json:"registry_version"`
	GlobalConfigGeneration    *string  `json:"global_config_generation,omitempty"`
	WorkspaceConfigGeneration *string  `json:"workspace_config_generation,omitempty"`
	ActiveLayersGeneration    *string  `json:"active_layers_generation,omitempty"`

	ForegroundSnapshot          *json.RawMessage      `json:"foreground_snapshot,omitempty"`
	ConversationID              *string               `json:"conversation_id,omitempty"`
	TurnID                      *string               `json:"turn_id,omitempty"`
	SurfaceType                 *string               `json:"surface_type,omitempty"`
	SurfaceID                   *string               `json:"surface_id,omitempty"`
	SurfaceSnapshotVersion      *string               `json:"surface_snapshot_version,omitempty"`
	ContextVersion              *string               `json:"context_version,omitempty"`
	ContextCapturedAtByProvider *map[string]time.Time `json:"context_captured_at_by_provider,omitempty"`

	SourceProfileSlug *string `json:"source_profile_slug,omitempty"`
	TargetProfileSlug *string `json:"target_profile_slug,omitempty"`

	AuthorizationDecisionID *string `json:"authorization_decision_id,omitempty"`
	DelegationFingerprint   *string `json:"delegation_fingerprint,omitempty"`
	GrantGeneration         *string `json:"grant_generation,omitempty"`

	JobID                    *string `json:"job_id,omitempty"`
	JobSlug                  *string `json:"job_slug,omitempty"`
	JobDefinitionFingerprint *string `json:"job_definition_fingerprint,omitempty"`
	RunID                    *string `json:"run_id,omitempty"`

	Provenance                *json.RawMessage `json:"provenance,omitempty"`
	CorrelationID             string           `json:"correlation_id"`
	RequestFingerprintVersion *string          `json:"request_fingerprint_version,omitempty"`
	RequestFingerprint        *string          `json:"request_fingerprint,omitempty"`
	ClientRequestedAt         *time.Time       `json:"client_requested_at,omitempty"`
	ReceivedAt                time.Time        `json:"received_at"`
}

// SignedRequest mantém juntos o envelope e a Definition confiável que
// completam sua semântica. Definition não é serializada no wire envelope.
type SignedRequest struct {
	Envelope   Envelope
	Definition commandcatalog.Definition
}

// UnmarshalJSON faz o decode de entrada com nomes exatos. encoding/json aceita
// nomes de campos sem distinção de maiúsculas/minúsculas e sobrescreve chaves
// repetidas; ambos os comportamentos são inadequados para um contrato
// canônico. commandjson rejeita duplicatas/JSON fora do conjunto suportado e o
// mapa abaixo fecha o conjunto de nomes antes de DisallowUnknownFields.
func (e *Envelope) UnmarshalJSON(data []byte) error {
	if e == nil {
		return fmt.Errorf("%w: receptor nil", ErrInvalidEnvelope)
	}
	canonical, err := commandjson.Canonicalize(data)
	if err != nil || len(canonical) == 0 || canonical[0] != '{' {
		return ErrInvalidEnvelope
	}
	var fields map[string]json.RawMessage
	// Keep the original raw values for nested documents. Canonicalization is
	// validation/output framing, not a lossy decode: 1.0 and 1e0 must remain
	// distinguishable from the integer 1 for versioned document validation.
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return ErrInvalidEnvelope
	}
	for name, raw := range fields {
		if !isEnvelopeJSONField(name) {
			return fmt.Errorf("%w: campo desconhecido", ErrInvalidEnvelope)
		}
		if isNullJSON(raw) {
			return fmt.Errorf("%w: campo %s não aceita null explícito", ErrInvalidEnvelope, name)
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var value envelopeJSONAlias
	if err := decoder.Decode(&value); err != nil {
		return ErrInvalidEnvelope
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return ErrInvalidEnvelope
	}
	*e = Envelope(value)
	return nil
}

type envelopeJSONAlias Envelope

func (e *envelopeJSONAlias) UnmarshalJSON(data []byte) error {
	type plain envelopeJSONAlias
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*e = envelopeJSONAlias(value)
	return nil
}

func isNullJSON(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}

func isEnvelopeJSONField(name string) bool {
	switch name {
	case "version", "invocation_id", "command_id", "arguments", "observed_trigger_type", "trigger_type", "trigger_spec", "user_id", "auth_context_type", "auth_context_id", "auth_generation", "session_id", "security_generation", "actor_type", "actor_id", "source_type", "observer_type", "source_instance_id", "source_event_id", "source_occurred_at", "source_replay_policy_generation", "source_replay_deadline", "workspace_id", "binding_ids", "registry_version", "global_config_generation", "workspace_config_generation", "active_layers_generation", "foreground_snapshot", "conversation_id", "turn_id", "surface_type", "surface_id", "surface_snapshot_version", "context_version", "context_captured_at_by_provider", "source_profile_slug", "target_profile_slug", "authorization_decision_id", "delegation_fingerprint", "grant_generation", "job_id", "job_slug", "job_definition_fingerprint", "run_id", "provenance", "correlation_id", "request_fingerprint_version", "request_fingerprint", "client_requested_at", "received_at":
		return true
	default:
		return false
	}
}

// FingerprintKeyProvider é fornecido pelo host e recebe o nome lógico da
// versão, por exemplo command-request-hmac:v2. O provider não é autorizado a
// escolher uma versão derivada do payload.
type FingerprintKeyProvider func(context.Context, string) ([]byte, error)
