package commandcontract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"assistente/internal/commandjson"
	"github.com/google/uuid"
)

var (
	// ErrInvalidEnvelope é retornado para qualquer falha estrutural. A
	// validação não deve ser confundida com autenticação ou autorização.
	ErrInvalidEnvelope     = errors.New("envelope de comando inválido")
	ErrIngressBackendField = errors.New("campo produzido pelo backend presente no ingresso")
)

var (
	commandIDPattern       = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)
	stableReferencePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]*$`)
)

// MaxEnvelopeBytes é o teto total do wire envelope, igual ao limite do
// commandjson. Documentos individuais também não podem excedê-lo.
const MaxEnvelopeBytes = 64 * 1024

const maxEnvelopeDocumentBytes = MaxEnvelopeBytes

// ValidateIngress valida apenas sintaxe, nulabilidade e coerência observável
// na borda. Fingerprints, authorization_decision_id e epochs de replay são
// derivados pelo backend e, portanto, não podem entrar no envelope de ingresso.
// Este método não autentica nenhuma identidade ou origem.
func (e *Envelope) ValidateIngress() error {
	if e == nil {
		return ErrInvalidEnvelope
	}
	if err := validateEnvelope(e, false, ""); err != nil {
		return err
	}
	if e.RequestFingerprintVersion != nil || e.RequestFingerprint != nil || e.AuthorizationDecisionID != nil {
		return fmt.Errorf("%w: %v", ErrInvalidEnvelope, ErrIngressBackendField)
	}
	if e.SourceReplayPolicyGeneration != nil || e.SourceReplayDeadline != nil {
		return fmt.Errorf("%w: epochs de replay são derivados pelo backend", ErrInvalidEnvelope)
	}
	return nil
}

// ValidateResolved valida o envelope interno após a resolução. O modo é
// obrigatório para impedir que uma chamada de suppress/denied seja tratada
// acidentalmente como execute. A validação continua estrutural; ela não prova
// autenticação, autorização, decisão, ownership ou disponibilidade.
func (e *Envelope) ValidateResolved(mode ResolutionMode) error {
	if mode != ResolutionExecute && mode != ResolutionSuppress && mode != ResolutionDenied {
		return fmt.Errorf("%w: modo de resolução inválido", ErrInvalidEnvelope)
	}
	return validateEnvelope(e, true, mode)
}

// ValidateIngress é a forma funcional equivalente para consumidores que
// preferem não usar métodos.
func ValidateIngress(e Envelope) error { return e.ValidateIngress() }

// ValidateResolved é a forma funcional equivalente para o executor I04.
func ValidateResolved(e Envelope, mode ResolutionMode) error { return e.ValidateResolved(mode) }

func validateEnvelope(e *Envelope, resolved bool, mode ResolutionMode) error {
	if e == nil || e.Version != EnvelopeVersion || !validUUIDv7(e.InvocationID) {
		return ErrInvalidEnvelope
	}
	for _, value := range []string{e.AuthContextID, e.AuthGeneration, e.SecurityGeneration, e.RegistryVersion, e.CorrelationID} {
		if !validText(value, 4096) {
			return ErrInvalidEnvelope
		}
	}
	if err := validateAuthAndActor(e); err != nil {
		return err
	}
	if err := validateOptionalUUID(e.UserID); err != nil {
		return err
	}
	if err := validateOptionalText(e.SessionID, 4096); err != nil {
		return err
	}
	if e.SessionID != nil && !validUUIDv7(*e.SessionID) {
		return ErrInvalidEnvelope
	}
	if err := validateOptionalText(e.WorkspaceID, 4096); err != nil {
		return err
	}
	if err := validateOptionalText(e.ConversationID, 4096); err != nil {
		return err
	}
	if e.ConversationID != nil && !validUUIDv7(*e.ConversationID) {
		return ErrInvalidEnvelope
	}
	if err := validateOptionalText(e.TurnID, 4096); err != nil {
		return err
	}
	if e.TurnID != nil && !validUUIDv7(*e.TurnID) {
		return ErrInvalidEnvelope
	}
	if (e.ConversationID == nil) != (e.TurnID == nil) {
		return fmt.Errorf("%w: conversation_id e turn_id devem aparecer juntos", ErrInvalidEnvelope)
	}
	if err := validateSurface(e); err != nil {
		return err
	}
	if err := validateGenerations(e); err != nil {
		return err
	}
	if err := validateDocuments(e); err != nil {
		return err
	}
	if mode == ResolutionSuppress && e.Arguments != nil {
		canonical, err := commandjson.Canonicalize([]byte(*e.Arguments))
		if err != nil || !bytes.Equal(canonical, []byte(`{}`)) {
			return fmt.Errorf("%w: suppress não aceita argumentos", ErrInvalidEnvelope)
		}
	}
	if err := validateProfilesAndDelegation(e); err != nil {
		return err
	}
	if err := validateJob(e); err != nil {
		return err
	}
	if err := validateBindings(e, resolved); err != nil {
		return err
	}
	if mode == ResolutionSuppress && len(e.BindingIDs) == 0 {
		return fmt.Errorf("%w: suppress exige ao menos um binding contribuinte", ErrInvalidEnvelope)
	}
	if err := validateTriggerAndCommand(e, resolved, mode); err != nil {
		return err
	}
	if err := validateSource(e, resolved, mode); err != nil {
		return err
	}
	if e.ReceivedAt.IsZero() {
		if resolved {
			return fmt.Errorf("%w: received_at obrigatório após resolução", ErrInvalidEnvelope)
		}
	} else if !validTime(e.ReceivedAt) {
		return ErrInvalidEnvelope
	}
	if e.ClientRequestedAt != nil && !validTime(*e.ClientRequestedAt) {
		return ErrInvalidEnvelope
	}
	if e.RequestFingerprintVersion != nil && !validText(*e.RequestFingerprintVersion, 64) {
		return ErrInvalidEnvelope
	}
	if e.RequestFingerprint != nil && !validText(*e.RequestFingerprint, 512) {
		return ErrInvalidEnvelope
	}
	if e.AuthorizationDecisionID != nil && !validUUIDv7(*e.AuthorizationDecisionID) {
		return ErrInvalidEnvelope
	}
	encoded, err := json.Marshal(e)
	if err != nil || len(encoded) > MaxEnvelopeBytes {
		return fmt.Errorf("%w: envelope excede 64 KiB", ErrInvalidEnvelope)
	}
	return nil
}

func validateAuthAndActor(e *Envelope) error {
	switch e.AuthContextType {
	case AuthLocalSession:
		if e.UserID == nil || e.SessionID == nil || !validUUIDv7(*e.UserID) || !validUUIDv7(*e.SessionID) || e.AuthContextID != *e.SessionID {
			return fmt.Errorf("%w: contexto local exige usuário e sessão UUIDv7", ErrInvalidEnvelope)
		}
	case AuthExternalToken:
		if e.UserID == nil || !validUUIDv7(*e.UserID) || e.SessionID != nil {
			return fmt.Errorf("%w: contexto externo exige usuário mapeado e não usa sessão", ErrInvalidEnvelope)
		}
	case AuthJobService:
		if e.UserID == nil || !validUUIDv7(*e.UserID) || e.SessionID != nil {
			return fmt.Errorf("%w: job_service exige proprietário UUIDv7 e não usa sessão", ErrInvalidEnvelope)
		}
	case AuthSystem:
		if e.UserID != nil || e.SessionID != nil {
			return fmt.Errorf("%w: contexto system não possui usuário ou sessão", ErrInvalidEnvelope)
		}
		if e.JobID != nil || e.JobSlug != nil || e.JobDefinitionFingerprint != nil || e.RunID != nil || e.DelegationFingerprint != nil || e.GrantGeneration != nil {
			return fmt.Errorf("%w: contexto system não possui job ou grant", ErrInvalidEnvelope)
		}
	default:
		return fmt.Errorf("%w: auth_context_type desconhecido", ErrInvalidEnvelope)
	}
	if e.ActorID == "" || (e.ActorID != strings.TrimSpace(e.ActorID)) || !utf8.ValidString(e.ActorID) {
		return ErrInvalidEnvelope
	}
	switch e.ActorType {
	case ActorUser:
		if e.UserID == nil || !validUUIDv7(e.ActorID) || e.ActorID != *e.UserID {
			return fmt.Errorf("%w: ator user exige UUIDv7", ErrInvalidEnvelope)
		}
	case ActorAgent, ActorAutomation:
		// Profile/job slugs são identidades canônicas, mas não são PKs UUIDv7.
	default:
		return fmt.Errorf("%w: actor_type desconhecido", ErrInvalidEnvelope)
	}
	if e.AuthContextType == AuthSystem && e.ActorType == ActorUser {
		return fmt.Errorf("%w: system não pode declarar ator user", ErrInvalidEnvelope)
	}
	return nil
}

func validateTriggerAndCommand(e *Envelope, resolved bool, mode ResolutionMode) error {
	if err := validateOptionalText(e.CommandID, 512); err != nil {
		return err
	}
	if e.CommandID != nil && !commandIDPattern.MatchString(*e.CommandID) {
		return fmt.Errorf("%w: command_id não é namespaced", ErrInvalidEnvelope)
	}
	if err := validateOptionalText(e.ObservedTriggerType, 256); err != nil {
		return err
	}
	if err := validateOptionalText(e.TriggerType, 256); err != nil {
		return err
	}
	if e.TriggerType != nil && !validTriggerType(*e.TriggerType) {
		return fmt.Errorf("%w: trigger_type desconhecido ou reservado", ErrInvalidEnvelope)
	}
	if e.ObservedTriggerType != nil && !validTriggerType(*e.ObservedTriggerType) {
		return fmt.Errorf("%w: observed_trigger_type desconhecido ou reservado", ErrInvalidEnvelope)
	}
	if !resolved && e.CommandID != nil && (e.TriggerType != nil || e.ObservedTriggerType != nil || e.TriggerSpec != nil) {
		return fmt.Errorf("%w: comando direto e trigger não podem coexistir", ErrInvalidEnvelope)
	}
	if e.TriggerType == nil {
		if e.ObservedTriggerType == nil && e.TriggerSpec != nil {
			return fmt.Errorf("%w: trigger_spec sem tipo de trigger", ErrInvalidEnvelope)
		}
		if e.ObservedTriggerType != nil && e.TriggerSpec == nil {
			return fmt.Errorf("%w: trigger físico exige trigger_spec", ErrInvalidEnvelope)
		}
		if mode == ResolutionSuppress || (resolved && mode == ResolutionExecute && e.CommandID == nil) {
			return fmt.Errorf("%w: resolução exige trigger_type", ErrInvalidEnvelope)
		}
		if e.CommandID == nil && e.ObservedTriggerType == nil {
			return fmt.Errorf("%w: command_id ou trigger_type obrigatório", ErrInvalidEnvelope)
		}
		return nil
	}
	if e.TriggerSpec == nil {
		return fmt.Errorf("%w: trigger_spec obrigatório com trigger_type", ErrInvalidEnvelope)
	}
	if e.CommandID != nil && mode == ResolutionSuppress {
		return fmt.Errorf("%w: suppress não possui command_id", ErrInvalidEnvelope)
	}
	if e.CommandID == nil && resolved && mode == ResolutionExecute {
		return fmt.Errorf("%w: execução resolvida exige command_id", ErrInvalidEnvelope)
	}
	return nil
}

func validateSource(e *Envelope, resolved bool, mode ResolutionMode) error {
	if e.SourceType == nil {
		if resolved && mode != ResolutionDenied {
			return fmt.Errorf("%w: source_type obrigatório após resolução", ErrInvalidEnvelope)
		}
		// Uma resolução negada pode não ter vencedor e, nesse caso, mantém
		// source_type nulo. A borda ainda pode ter identificado o adapter físico
		// antes da resolução; valide esse grupo pela taxonomia do trigger.
		triggerType := e.TriggerType
		if triggerType == nil {
			triggerType = e.ObservedTriggerType
		}
		physical := triggerType != nil && (*triggerType == string(SourceKeyboardLocal) || *triggerType == string(SourceKeyboardGlobal) || *triggerType == string(SourceStreamDeck))
		event := triggerType != nil && *triggerType == string(SourceEvent)
		if physical || event {
			if e.SourceInstanceID == nil || e.SourceEventID == nil || !validUUIDv7(*e.SourceInstanceID) || !validUUIDv7(*e.SourceEventID) || e.ObserverType == nil || !validText(*e.ObserverType, 256) {
				return fmt.Errorf("%w: trigger físico/evento exige identidade do adapter", ErrInvalidEnvelope)
			}
		} else if e.SourceInstanceID != nil || e.SourceEventID != nil || e.SourceOccurredAt != nil || e.SourceReplayPolicyGeneration != nil || e.SourceReplayDeadline != nil || e.ObserverType != nil {
			return fmt.Errorf("%w: origem sem tipo não aceita identidade física", ErrInvalidEnvelope)
		}
		if event {
			if e.SourceOccurredAt == nil || !validTime(*e.SourceOccurredAt) || e.Provenance == nil {
				return fmt.Errorf("%w: trigger event exige occurred_at e provenance", ErrInvalidEnvelope)
			}
			if resolved && (e.SourceReplayPolicyGeneration == nil || !validText(*e.SourceReplayPolicyGeneration, 256) || e.SourceReplayDeadline == nil || !validTime(*e.SourceReplayDeadline) || !e.SourceReplayDeadline.After(*e.SourceOccurredAt)) {
				return fmt.Errorf("%w: evento resolvido exige epoch e deadline válidos", ErrInvalidEnvelope)
			}
			if !resolved && (e.SourceReplayPolicyGeneration != nil || e.SourceReplayDeadline != nil) {
				return fmt.Errorf("%w: epochs de replay são derivados pelo backend", ErrInvalidEnvelope)
			}
		} else if e.SourceOccurredAt != nil || e.SourceReplayPolicyGeneration != nil || e.SourceReplayDeadline != nil {
			return fmt.Errorf("%w: timestamps de replay só existem para event", ErrInvalidEnvelope)
		}
		return nil
	}
	source := *e.SourceType
	if !validSource(source) {
		return fmt.Errorf("%w: source_type desconhecido", ErrInvalidEnvelope)
	}
	if e.AuthContextType == AuthSystem && source != SourceSystem {
		return fmt.Errorf("%w: auth system exige source system", ErrInvalidEnvelope)
	}
	if source == SourceSystem && e.AuthContextType != AuthSystem {
		return fmt.Errorf("%w: source system exige contexto system", ErrInvalidEnvelope)
	}
	if e.TriggerType != nil && source != SourceType(*e.TriggerType) {
		return fmt.Errorf("%w: source_type e trigger_type divergentes", ErrInvalidEnvelope)
	}
	physical := source == SourceKeyboardLocal || source == SourceKeyboardGlobal || source == SourceStreamDeck
	event := source == SourceEvent
	if physical || event {
		if e.SourceInstanceID == nil || e.SourceEventID == nil || !validUUIDv7(*e.SourceInstanceID) || !validUUIDv7(*e.SourceEventID) {
			return fmt.Errorf("%w: origem física/evento exige instance e event UUIDv7", ErrInvalidEnvelope)
		}
		if e.ObserverType == nil || !validText(*e.ObserverType, 256) {
			return fmt.Errorf("%w: origem física/evento exige observer_type", ErrInvalidEnvelope)
		}
	} else if e.SourceInstanceID != nil || e.SourceEventID != nil || e.ObserverType != nil {
		return fmt.Errorf("%w: origem direta não aceita identidade física", ErrInvalidEnvelope)
	}
	if event {
		if e.SourceOccurredAt == nil || !validTime(*e.SourceOccurredAt) {
			return fmt.Errorf("%w: evento exige occurred_at, epoch e deadline válidos", ErrInvalidEnvelope)
		}
		if resolved && (e.SourceReplayPolicyGeneration == nil || !validText(*e.SourceReplayPolicyGeneration, 256) || e.SourceReplayDeadline == nil || !validTime(*e.SourceReplayDeadline) || !e.SourceReplayDeadline.After(*e.SourceOccurredAt)) {
			return fmt.Errorf("%w: evento resolvido exige epoch e deadline válidos", ErrInvalidEnvelope)
		}
		if !resolved && (e.SourceReplayPolicyGeneration != nil || e.SourceReplayDeadline != nil) {
			return fmt.Errorf("%w: epochs de replay são derivados pelo backend", ErrInvalidEnvelope)
		}
		if e.Provenance == nil {
			return fmt.Errorf("%w: origem event exige provenance", ErrInvalidEnvelope)
		}
	} else if e.SourceOccurredAt != nil || e.SourceReplayPolicyGeneration != nil || e.SourceReplayDeadline != nil {
		return fmt.Errorf("%w: timestamps de replay só existem para event", ErrInvalidEnvelope)
	}
	if source == SourceSystem && (e.UserID != nil || e.WorkspaceID != nil || e.GlobalConfigGeneration != nil || e.WorkspaceConfigGeneration != nil || e.ActiveLayersGeneration != nil) {
		return fmt.Errorf("%w: system não acessa contexto de usuário", ErrInvalidEnvelope)
	}
	return nil
}

func validateGenerations(e *Envelope) error {
	if e.UserID == nil {
		if e.GlobalConfigGeneration != nil || e.WorkspaceConfigGeneration != nil || e.ActiveLayersGeneration != nil {
			return fmt.Errorf("%w: gerações de configuração sem usuário", ErrInvalidEnvelope)
		}
		return nil
	}
	if e.GlobalConfigGeneration == nil || e.ActiveLayersGeneration == nil || !validText(*e.GlobalConfigGeneration, 256) || !validText(*e.ActiveLayersGeneration, 256) {
		return fmt.Errorf("%w: usuário exige gerações global e de camadas", ErrInvalidEnvelope)
	}
	if e.WorkspaceID == nil {
		if e.WorkspaceConfigGeneration != nil {
			return fmt.Errorf("%w: geração de workspace sem workspace", ErrInvalidEnvelope)
		}
	} else if e.WorkspaceConfigGeneration == nil || !validText(*e.WorkspaceConfigGeneration, 256) {
		return fmt.Errorf("%w: workspace exige sua geração", ErrInvalidEnvelope)
	}
	return nil
}

func validateSurface(e *Envelope) error {
	values := []*string{e.SurfaceType, e.SurfaceID, e.SurfaceSnapshotVersion}
	present := 0
	for _, value := range values {
		if value != nil {
			present++
			if !validText(*value, 4096) {
				return fmt.Errorf("%w: campo de surface vazio", ErrInvalidEnvelope)
			}
		}
	}
	if present != 0 && present != len(values) {
		return fmt.Errorf("%w: surface exige tipo, id e versão", ErrInvalidEnvelope)
	}
	return nil
}

func validateDocuments(e *Envelope) error {
	for name, document := range map[string]*json.RawMessage{
		"arguments": e.Arguments, "trigger_spec": e.TriggerSpec, "foreground_snapshot": e.ForegroundSnapshot, "provenance": e.Provenance,
	} {
		if document == nil || len(*document) == 0 || len(*document) > maxEnvelopeDocumentBytes || !utf8.Valid(*document) {
			if document != nil {
				return fmt.Errorf("%w: documento %s inválido", ErrInvalidEnvelope, name)
			}
			continue
		}
		canonical, err := commandjson.Canonicalize([]byte(*document))
		if err != nil || len(canonical) == 0 || canonical[0] != '{' {
			return fmt.Errorf("%w: documento %s deve ser objeto JSON canônico", ErrInvalidEnvelope, name)
		}
		switch name {
		case "trigger_spec":
			triggerType := e.TriggerType
			if triggerType == nil {
				triggerType = e.ObservedTriggerType
			}
			if triggerType == nil || ValidateTriggerDocumentVersion(*triggerType, *document) != nil {
				return fmt.Errorf("%w: versão de trigger_spec não suportada", ErrInvalidEnvelope)
			}
		case "provenance":
			if err := validateVersionedDocument([]byte(*document), name); err != nil {
				return err
			}
		}
	}
	if e.ContextCapturedAtByProvider != nil {
		if len(*e.ContextCapturedAtByProvider) == 0 {
			return fmt.Errorf("%w: mapa de timestamps de provider vazio", ErrInvalidEnvelope)
		}
		for provider, captured := range *e.ContextCapturedAtByProvider {
			if !validText(provider, 256) || !validTime(captured) {
				return fmt.Errorf("%w: timestamp de provider inválido", ErrInvalidEnvelope)
			}
		}
	}
	if e.ContextVersion != nil && !validText(*e.ContextVersion, 4096) {
		return ErrInvalidEnvelope
	}
	return nil
}

// validateVersionedDocument applies the D2/D2.1 document envelope to the
// documents whose schema is owned by this contract. The version is deliberately
// checked as an integer so values such as 1.0, "1", null, and future versions
// cannot pass as version 1.
func validateVersionedDocument(raw []byte, name string) error {
	return validateDocumentVersion(raw, name, false)
}

// ValidateTriggerDocumentVersion valida somente o envelope estrutural do
// documento. A gramática específica continua sob responsabilidade da porta
// confiável do acionador; v2 existe exclusivamente para keyboard.local.
func ValidateTriggerDocumentVersion(triggerType string, raw []byte) error {
	return validateDocumentVersion(raw, "trigger_spec", triggerType == string(SourceKeyboardLocal))
}

func validateDocumentVersion(raw []byte, name string, localKeyboard bool) error {
	canonical, err := commandjson.Canonicalize(raw)
	if err != nil || len(canonical) == 0 || canonical[0] != '{' {
		return fmt.Errorf("%w: documento %s inválido", ErrInvalidEnvelope, name)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return fmt.Errorf("%w: documento %s inválido", ErrInvalidEnvelope, name)
	}
	rawVersion, ok := fields["version"]
	if !ok {
		return fmt.Errorf("%w: documento %s exige version", ErrInvalidEnvelope, name)
	}
	var version int
	if err := json.Unmarshal(rawVersion, &version); err != nil || (version != EnvelopeVersion && (!localKeyboard || version != 2)) {
		return fmt.Errorf("%w: versão do documento %s não suportada", ErrInvalidEnvelope, name)
	}
	return nil
}

func validateProfilesAndDelegation(e *Envelope) error {
	for _, value := range []*string{e.SourceProfileSlug, e.TargetProfileSlug, e.DelegationFingerprint, e.GrantGeneration} {
		if err := validateOptionalText(value, 4096); err != nil {
			return err
		}
	}
	if e.ActorType == ActorAgent && (e.SourceProfileSlug == nil || e.TargetProfileSlug == nil) {
		return fmt.Errorf("%w: ator agent exige profiles de origem e destino", ErrInvalidEnvelope)
	}
	if (e.DelegationFingerprint == nil) != (e.GrantGeneration == nil) {
		return fmt.Errorf("%w: delegation fingerprint e grant generation devem aparecer juntos", ErrInvalidEnvelope)
	}
	return nil
}

func validateJob(e *Envelope) error {
	values := []*string{e.JobID, e.JobSlug, e.JobDefinitionFingerprint, e.RunID}
	present := 0
	for _, value := range values {
		if value != nil {
			present++
			if !validText(*value, 4096) {
				return ErrInvalidEnvelope
			}
		}
	}
	if e.JobID != nil && !validUUIDv7(*e.JobID) {
		return fmt.Errorf("%w: job_id deve ser UUIDv7; job_slug não", ErrInvalidEnvelope)
	}
	if present != 0 && present != len(values) {
		return fmt.Errorf("%w: grupo job incompleto", ErrInvalidEnvelope)
	}
	if e.AuthContextType == AuthJobService && present != len(values) {
		return fmt.Errorf("%w: job_service exige grupo job completo", ErrInvalidEnvelope)
	}
	return nil
}

func validateBindings(e *Envelope, resolved bool) error {
	if e.BindingIDs == nil {
		if resolved {
			return fmt.Errorf("%w: binding_ids deve ser lista, inclusive vazia", ErrInvalidEnvelope)
		}
		return nil
	}
	seen := make(map[string]struct{}, len(e.BindingIDs))
	for _, id := range e.BindingIDs {
		if !validBindingReference(id) {
			return fmt.Errorf("%w: binding_id inválido", ErrInvalidEnvelope)
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("%w: binding_id repetido", ErrInvalidEnvelope)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func validBindingReference(value string) bool {
	return (validUUIDv7(value) || stableReferencePattern.MatchString(value)) && strings.TrimSpace(value) == value
}

func validateOptionalUUID(value *string) error {
	if value != nil && !validUUIDv7(*value) {
		return ErrInvalidEnvelope
	}
	return nil
}

func validateOptionalText(value *string, max int) error {
	if value != nil && !validText(*value, max) {
		return ErrInvalidEnvelope
	}
	return nil
}

func validText(value string, max int) bool {
	return value != "" && len(value) <= max && utf8.ValidString(value) && !strings.ContainsRune(value, '\x00') && strings.TrimSpace(value) == value
}

func validTime(value time.Time) bool {
	return !value.IsZero() && value.Location() != nil
}

func validUUIDv7(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.Version() == 7 && id.Variant() == uuid.RFC4122 && id.String() == value
}

func validSource(value SourceType) bool {
	switch value {
	case SourceKeyboardLocal, SourceKeyboardGlobal, SourceStreamDeck, SourcePalette, SourceUI, SourceChat, SourceCLI, SourceEvent, SourceSystem:
		return true
	default:
		return false
	}
}

func validTriggerType(value string) bool {
	// system é uma origem interna sem acionador configurável. Ele só pode
	// aparecer como source_type de uma solicitação direta do host.
	return validSource(SourceType(value)) && SourceType(value) != SourceSystem
}
