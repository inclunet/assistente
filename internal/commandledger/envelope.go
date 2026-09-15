package commandledger

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"assistente/internal/commandcontract"
	"assistente/internal/commanddecision"
	"assistente/internal/commandjson"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Mode é o resultado do resolvedor confiável antes da reserva.
type Mode = commandcontract.ResolutionMode

const (
	ModeExecute  = commandcontract.ResolutionExecute
	ModeSuppress = commandcontract.ResolutionSuppress
	ModeDenied   = commandcontract.ResolutionDenied
)

// EnvelopeRequest reúne o envelope resolvido e os valores derivados pelo
// executor. ArgumentsFingerprint nunca contém o argumento bruto.
type EnvelopeRequest struct {
	Envelope             commandcontract.Envelope
	Mode                 Mode
	ArgumentsFingerprint string
	// InputFingerprint é o HMAC da entrada efetivamente recebida. É opcional
	// para consumidores legados; o engine novo deve sempre fornecê-lo.
	InputFingerprint string
	// RejectedStale é um sinal confiável do pipeline para stale contextual
	// detectado antes da reserva; tem precedência e nunca cria auditoria.
	RejectedStale bool
	ExpiresAt     time.Time
	Risk          string
}

// FullOwnership é a identidade completa usada em toda leitura e CAS. Actor
// faz parte do escopo para impedir que um principal diferente reutilize uma
// invocação do mesmo contexto autenticado.
type FullOwnership struct {
	UserID          *string
	AuthContextType commandcontract.AuthContextType
	AuthContextID   string
	ActorType       commandcontract.ActorType
	ActorID         string
}

// EnvelopeResult aceita somente metadados terminais allowlisted. O resumo JSON
// é gerado pelo ledger; texto arbitrário nunca é persistido.
type EnvelopeResult struct {
	ErrorCode string
	ResultRef string
}

// FullRecord é a projeção completa persistível/consultável. Envelope contém
// somente documentos redigidos; ArgumentsFingerprint e o fingerprint de
// request permanecem disponíveis para idempotência e diagnóstico.
type FullRecord struct {
	ID                        string
	Key                       string
	InvocationID              string
	Ownership                 FullOwnership
	Envelope                  commandcontract.Envelope
	Mode                      Mode
	ArgumentsSummary          string
	ArgumentsFingerprint      string
	RequestFingerprintVersion string
	RequestFingerprint        string
	InputFingerprint          string
	Risk                      string
	Status                    Status
	ResultSummary             *string
	ResultRef                 *string
	ErrorCode                 *string
	SourceEventID             *string
	ReceivedAt                time.Time
	ExpiresAt                 time.Time
}

type EnvelopeReservation struct {
	Record  FullRecord
	Created bool
}

type EnvelopeOutcomeVerifier func(context.Context, FullRecord) (Status, error)

var (
	envelopeVersionPattern = regexp.MustCompile(`^v[1-9][0-9]*$`)
	hex64Pattern           = regexp.MustCompile(`^[0-9a-f]{64}$`)
	resultCodePattern      = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
)

const redactedDocument = `{"version":1,"redacted":true}`

// ReserveEnvelope reserva a invocação e, quando aplicável, sua auditoria na
// mesma transação. Suppressed e rejected_stale sobrevivem somente no ledger.
func (s *Store) ReserveEnvelope(ctx context.Context, req EnvelopeRequest) (EnvelopeReservation, error) {
	if s == nil || s.db == nil || s.now == nil || ctx == nil {
		return EnvelopeReservation{}, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return EnvelopeReservation{}, err
	}
	now := s.now()
	if now.IsZero() {
		return EnvelopeReservation{}, ErrInvalidRequest
	}
	now = now.UTC()
	if err := validateEnvelopeRequest(req, now); err != nil {
		return EnvelopeReservation{}, err
	}

	envelope := req.Envelope
	envelope.ReceivedAt = envelope.ReceivedAt.UTC()
	expiresAt := req.ExpiresAt.UTC()
	status := Evaluating
	replayEvent := envelopeIsReplayEvent(envelope)
	if req.RejectedStale {
		status = RejectedStale
	} else if replayEvent && envelope.SourceReplayDeadline != nil && !envelope.SourceReplayDeadline.After(now) {
		status = RejectedStale
	} else if !expiresAt.After(now) && !replayEvent {
		return EnvelopeReservation{}, ErrExpired
	} else if req.Mode == ModeSuppress {
		status = Suppressed
	} else if req.Mode == ModeDenied {
		status = Denied
	}

	key := "invocation:" + envelope.InvocationID
	if envelope.SourceEventID != nil {
		key = "event:" + *envelope.SourceEventID
	}
	owner := ownershipFromEnvelope(envelope)
	requestVersion := *envelope.RequestFingerprintVersion
	requestFingerprint := *envelope.RequestFingerprint
	rowID, err := uuid.NewV7()
	if err != nil {
		return EnvelopeReservation{}, ErrInvalidRequest
	}
	userID := cloneString(envelope.UserID)
	actorType := string(envelope.ActorType)
	actorID := envelope.ActorID
	row := ledgerRow{
		ID: rowID.String(), Key: key, InvocationID: envelope.InvocationID, UserID: userID,
		AuthContextType: string(envelope.AuthContextType), AuthContextID: envelope.AuthContextID,
		ActorType: &actorType, ActorID: &actorID, SourceType: sourceString(envelope.SourceType),
		SourceInstanceID: cloneString(envelope.SourceInstanceID), SourceEventID: cloneString(envelope.SourceEventID),
		SourceOccurredAt: cloneTime(envelope.SourceOccurredAt), SourceReplayPolicyGeneration: cloneString(envelope.SourceReplayPolicyGeneration),
		SourceReplayDeadline: cloneTime(envelope.SourceReplayDeadline), RequestFingerprintVersion: requestVersion,
		RequestFingerprint: requestFingerprint, Status: status, ReceivedAt: envelope.ReceivedAt, ExpiresAt: expiresAt,
	}
	if req.InputFingerprint != "" {
		row.InputFingerprint = envelopeStringPtr(req.InputFingerprint)
	}
	if envelopeTerminal(status) {
		summary, code, resultRef, err := terminalResult(status, EnvelopeResult{})
		if err != nil {
			return EnvelopeReservation{}, err
		}
		row.ResultSummary, row.ResultRef = envelopeStringPtr(summary), resultRef
		_ = code
	}

	var result EnvelopeReservation
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected == 0 {
			existing, findErr := findExistingEnvelope(tx, envelope, key)
			if findErr != nil {
				if errors.Is(findErr, gorm.ErrRecordNotFound) {
					return ErrConflict
				}
				return findErr
			}
			if !sameEnvelopeIdentity(existing, envelope, key, owner, req.InputFingerprint) {
				return ErrConflict
			}
			inv, invErr := loadInvocation(tx, existing.InvocationID, owner)
			if invErr != nil && !errors.Is(invErr, gorm.ErrRecordNotFound) {
				return invErr
			}
			result = EnvelopeReservation{Record: fullRecord(existing, inv), Created: false}
			return nil
		}

		if status == Suppressed || status == RejectedStale {
			result = EnvelopeReservation{Record: fullRecord(row, nil), Created: true}
			return nil
		}
		invocation, err := invocationFromEnvelope(envelope, req, status, now)
		if err != nil {
			return err
		}
		if err := tx.Create(&invocation).Error; err != nil {
			return err
		}
		result = EnvelopeReservation{Record: fullRecord(row, &invocation), Created: true}
		return nil
	})
	if err != nil {
		return EnvelopeReservation{}, sanitizeEnvelopeError(err)
	}
	return result, nil
}

// GetEnvelopeByID consulta pelo ID de invocação e exige ownership completo.
func (s *Store) GetEnvelopeByID(ctx context.Context, owner FullOwnership, id string) (FullRecord, error) {
	if err := validateFullOwnership(owner); err != nil || !validUUID(id) {
		return FullRecord{}, ErrNotFound
	}
	if s == nil || s.db == nil || ctx == nil {
		return FullRecord{}, ErrNotFound
	}
	if err := ctx.Err(); err != nil {
		return FullRecord{}, err
	}
	var ledger ledgerRow
	query := applyOwnership(s.db.WithContext(ctx).Where("invocation_id = ?", id), owner)
	if err := query.First(&ledger).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return FullRecord{}, ErrNotFound
		}
		return FullRecord{}, err
	}
	inv, err := loadInvocation(s.db.WithContext(ctx), id, owner)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return FullRecord{}, err
	}
	if inv == nil && ledger.Status != Suppressed && ledger.Status != RejectedStale {
		return FullRecord{}, ErrInconsistent
	}
	return fullRecord(ledger, inv), nil
}

// GetEnvelopeByEvent consulta a chave de ocorrência física/event-driven no
// mesmo escopo de usuário, contexto autenticado e ator.
func (s *Store) GetEnvelopeByEvent(ctx context.Context, owner FullOwnership, eventID string) (FullRecord, error) {
	if err := validateFullOwnership(owner); err != nil || !validUUID(eventID) {
		return FullRecord{}, ErrNotFound
	}
	if s == nil || s.db == nil || ctx == nil {
		return FullRecord{}, ErrNotFound
	}
	var ledger ledgerRow
	query := applyOwnership(s.db.WithContext(ctx).Where("source_event_id = ?", eventID), owner)
	if err := query.First(&ledger).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return FullRecord{}, ErrNotFound
		}
		return FullRecord{}, err
	}
	inv, err := loadInvocation(s.db.WithContext(ctx), ledger.InvocationID, owner)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return FullRecord{}, err
	}
	if inv == nil && ledger.Status != Suppressed && ledger.Status != RejectedStale {
		return FullRecord{}, ErrInconsistent
	}
	return fullRecord(ledger, inv), nil
}

// CompareAndSwapEnvelope atualiza ledger e auditoria na mesma transação.
// Um resultado opcional só aceita ErrorCode derivado do status e ResultRef
// UUID; o resumo final é sempre produzido internamente.
func (s *Store) CompareAndSwapEnvelope(ctx context.Context, owner FullOwnership, id string, from, to Status, result ...EnvelopeResult) (bool, error) {
	if s == nil || s.db == nil || s.now == nil || ctx == nil || validateFullOwnership(owner) != nil || !validUUID(id) || !validEnvelopeTransition(from, to) || len(result) > 1 {
		return false, ErrInvalidTransition
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	var supplied EnvelopeResult
	if len(result) == 1 {
		supplied = result[0]
	}
	summary, code, ref, err := terminalResult(to, supplied)
	if err != nil {
		return false, err
	}
	now := s.now()
	if now.IsZero() {
		return false, ErrInvalidRequest
	}
	now = now.UTC()
	changed := false
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		changed, err = s.compareAndSwapEnvelopeTx(tx, owner, id, from, to, supplied, now, summary, code, ref, "")
		return err
	})
	if err != nil {
		return false, sanitizeEnvelopeError(err)
	}
	return changed, nil
}

// CompareAndSwapEnvelopeWithDecision consome um receipt local e faz a
// transição evaluating->queued na mesma transação SQL. O callback de
// ConsumeForDatabase recebe a única conexão transacional; não abre uma
// transação aninhada nem executa efeitos externos.
func (s *Store) CompareAndSwapEnvelopeWithDecision(ctx context.Context, owner FullOwnership, id string, request commanddecision.Request, decisions *commanddecision.Store) (bool, error) {
	if s == nil || s.db == nil || s.now == nil || ctx == nil || decisions == nil || validateFullOwnership(owner) != nil || !validUUID(id) ||
		owner.AuthContextType != commandcontract.AuthLocalSession || owner.UserID == nil || request.SubjectType != "invocation" || request.MutationID != id ||
		request.UserID != *owner.UserID || request.SessionID != owner.AuthContextID {
		return false, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	changed := false
	err := decisions.ConsumeForDatabase(ctx, s.db, request, func(tx *gorm.DB) error {
		var ledger ledgerRow
		if err := applyOwnership(tx.Model(&ledgerRow{}).Where("invocation_id = ?", id), owner).First(&ledger).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if request.Fingerprint != ledger.RequestFingerprint || request.AuthGeneration == "" || request.SecurityGeneration == "" {
			return ErrConflict
		}
		var audit invocationRow
		if err := applyOwnership(tx.Model(&invocationRow{}).Where("invocation_id = ?", id), owner).First(&audit).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInconsistent
			}
			return err
		}
		now := s.now().UTC()
		if request.AuthGeneration != audit.AuthGeneration || request.SecurityGeneration != audit.SecurityGeneration ||
			!request.ExpiresAt.UTC().After(now) || !ledger.ExpiresAt.UTC().After(now) || request.ExpiresAt.UTC().After(ledger.ExpiresAt.UTC()) ||
			audit.Status != Evaluating || ledger.Status != Evaluating {
			return ErrConflict
		}
		var err error
		changed, err = s.compareAndSwapEnvelopeTx(tx, owner, id, Evaluating, Queued, EnvelopeResult{}, now, "", "", nil, request.DecisionID)
		if err != nil {
			return err
		}
		if !changed {
			return ErrInvalidTransition
		}
		return nil
	})
	if err != nil {
		return false, sanitizeEnvelopeError(err)
	}
	return changed, nil
}

func (s *Store) compareAndSwapEnvelopeTx(tx *gorm.DB, owner FullOwnership, id string, from, to Status, supplied EnvelopeResult, now time.Time, summary, code string, ref *string, authorizationDecisionID string) (bool, error) {
	if summary == "" && envelopeTerminal(to) {
		var err error
		summary, code, ref, err = terminalResult(to, supplied)
		if err != nil {
			return false, err
		}
	}
	ledgerQuery := applyOwnership(tx.Model(&ledgerRow{}).Where("invocation_id = ? AND status = ?", id, from), owner)
	ledgerUpdates := map[string]any{"status": to}
	if envelopeTerminal(to) {
		ledgerUpdates["result_summary"] = summary
		ledgerUpdates["result_ref"] = ref
	}
	updated := ledgerQuery.Updates(ledgerUpdates)
	if updated.Error != nil {
		return false, updated.Error
	}
	if updated.RowsAffected == 0 {
		return false, nil
	}

	auditUpdates := map[string]any{"status": to}
	if from == Evaluating && to == Queued {
		auditUpdates["policy_decision"] = "allowed"
	}
	if from == Evaluating && to == Denied {
		auditUpdates["policy_decision"] = "denied"
	}
	if authorizationDecisionID != "" {
		auditUpdates["authorization_decision_id"] = authorizationDecisionID
	}
	if envelopeTerminal(to) {
		auditUpdates["completed_at"] = now
		auditUpdates["result_summary"] = summary
		auditUpdates["result_ref"] = ref
		if code == "" {
			auditUpdates["error_code"] = nil
		} else {
			auditUpdates["error_code"] = code
		}
	}
	auditQuery := applyOwnership(tx.Model(&invocationRow{}).Where("invocation_id = ? AND status = ?", id, from), owner)
	audit := auditQuery.Updates(auditUpdates)
	if audit.Error != nil {
		return false, audit.Error
	}
	if audit.RowsAffected != 1 {
		return false, ErrInconsistent
	}
	return true, nil
}

// ReconcileEnvelope consulta somente o verificador confiável para um
// outcome_unknown já auditado. O callback ocorre fora da transação e nunca é
// usado para iniciar novo efeito; panic e respostas inválidas falham fechado.
func (s *Store) ReconcileEnvelope(ctx context.Context, owner FullOwnership, id string, verify EnvelopeOutcomeVerifier) (bool, error) {
	if verify == nil {
		return false, ErrInvalidRequest
	}
	record, err := s.GetEnvelopeByID(ctx, owner, id)
	if err != nil {
		return false, err
	}
	if record.Status != OutcomeUnknown {
		return false, nil
	}
	outcome, err := callEnvelopeVerifier(ctx, verify, record)
	if err != nil {
		return false, err
	}
	if outcome != Succeeded && outcome != Failed {
		return false, ErrInvalidTransition
	}
	return s.CompareAndSwapEnvelope(ctx, owner, id, OutcomeUnknown, outcome)
}

func callEnvelopeVerifier(ctx context.Context, verify EnvelopeOutcomeVerifier, record FullRecord) (status Status, err error) {
	defer func() {
		if recover() != nil {
			status = ""
			err = ErrVerifierPanic
		}
	}()
	return verify(ctx, record)
}

var ErrVerifierPanic = errors.New("verificador de outcome falhou")

func validateEnvelopeRequest(req EnvelopeRequest, now time.Time) error {
	if req.Mode != ModeExecute && req.Mode != ModeSuppress && req.Mode != ModeDenied {
		return ErrInvalidRequest
	}
	if err := req.Envelope.ValidateResolved(req.Mode); err != nil {
		return err
	}
	if req.Envelope.RequestFingerprintVersion == nil || !envelopeVersionPattern.MatchString(*req.Envelope.RequestFingerprintVersion) ||
		req.Envelope.RequestFingerprint == nil || !hex64Pattern.MatchString(*req.Envelope.RequestFingerprint) ||
		!hex64Pattern.MatchString(req.ArgumentsFingerprint) {
		return ErrInvalidRequest
	}
	if req.InputFingerprint != "" && !hex64Pattern.MatchString(req.InputFingerprint) {
		return ErrInvalidRequest
	}
	if !validRisk(req.Risk) || req.ExpiresAt.IsZero() {
		return ErrInvalidRequest
	}
	if envelopeIsReplayEvent(req.Envelope) {
		deadline := req.Envelope.SourceReplayDeadline
		if deadline == nil || req.ExpiresAt.Before(*deadline) {
			return ErrInvalidRequest
		}
		// Replay vencido preserva o deadline original, mesmo anterior ao novo
		// received_at. Nunca alongar sua retenção para fabricar uma nova execução.
		if !req.ExpiresAt.After(req.Envelope.ReceivedAt) && deadline.After(now) {
			return ErrInvalidRequest
		}
	} else if !req.ExpiresAt.After(req.Envelope.ReceivedAt) {
		return ErrInvalidRequest
	}
	if !req.ExpiresAt.After(now) && !req.RejectedStale && !envelopeIsReplayEvent(req.Envelope) {
		return ErrExpired
	}
	return nil
}

func envelopeIsReplayEvent(envelope commandcontract.Envelope) bool {
	return envelope.SourceReplayDeadline != nil ||
		(envelope.SourceType != nil && *envelope.SourceType == commandcontract.SourceEvent)
}

func validRisk(value string) bool {
	switch value {
	case "low", "medium", "high", "critical":
		return true
	default:
		return false
	}
}

func validEnvelopeTransition(from, to Status) bool {
	switch from {
	case Evaluating:
		return to == Queued || to == Denied || to == Failed || to == Cancelled || to == CancelledStale || to == TimedOut
	case Queued:
		return to == Running || to == Failed || to == Cancelled || to == CancelledStale || to == TimedOut
	case Running:
		return to == Succeeded || to == Failed || to == Cancelled || to == OutcomeUnknown
	case OutcomeUnknown:
		return to == Succeeded || to == Failed
	default:
		return false
	}
}

func envelopeTerminal(status Status) bool {
	return terminal(status) || status == Suppressed || status == RejectedStale
}

func terminalResult(status Status, result EnvelopeResult) (string, string, *string, error) {
	if !envelopeTerminal(status) {
		if result != (EnvelopeResult{}) {
			return "", "", nil, ErrInvalidRequest
		}
		return "", "", nil, nil
	}
	code := ""
	if status != Succeeded {
		code = "command_" + string(status)
		if result.ErrorCode != "" && result.ErrorCode != code {
			return "", "", nil, ErrInvalidRequest
		}
	} else if result.ErrorCode != "" {
		return "", "", nil, ErrInvalidRequest
	}
	if result.ErrorCode != "" && !resultCodePattern.MatchString(result.ErrorCode) {
		return "", "", nil, ErrInvalidRequest
	}
	var ref *string
	if result.ResultRef != "" {
		if !validReferenceUUID(result.ResultRef) {
			return "", "", nil, ErrInvalidRequest
		}
		ref = envelopeStringPtr(result.ResultRef)
	}
	payload := map[string]any{"version": 1, "status": string(status)}
	if code != "" {
		payload["error_code"] = code
	}
	encoded, err := commandjson.Marshal(payload)
	if err != nil {
		return "", "", nil, ErrInvalidRequest
	}
	return string(encoded), code, ref, nil
}

func invocationFromEnvelope(envelope commandcontract.Envelope, req EnvelopeRequest, status Status, now time.Time) (invocationRow, error) {
	bindingIDs, err := commandjson.Marshal(envelope.BindingIDs)
	if err != nil {
		return invocationRow{}, ErrInvalidRequest
	}
	policy := "pending"
	if status == Denied {
		policy = "denied"
	}
	row := invocationRow{
		InvocationID: envelope.InvocationID, SchemaVersion: envelope.Version, UserID: cloneString(envelope.UserID),
		AuthContextType: string(envelope.AuthContextType), AuthContextID: envelope.AuthContextID, AuthGeneration: envelope.AuthGeneration,
		SessionID: cloneString(envelope.SessionID), SecurityGeneration: envelope.SecurityGeneration, WorkspaceID: cloneString(envelope.WorkspaceID),
		RegistryVersion: envelope.RegistryVersion, GlobalConfigGeneration: cloneString(envelope.GlobalConfigGeneration),
		WorkspaceConfigGeneration: cloneString(envelope.WorkspaceConfigGeneration), ActiveLayersGeneration: cloneString(envelope.ActiveLayersGeneration),
		CommandID: cloneString(envelope.CommandID), BindingIDs: string(bindingIDs), ObservedTriggerType: cloneString(envelope.ObservedTriggerType),
		TriggerType: cloneString(envelope.TriggerType), TriggerSpecSnapshot: redactedIfPresent(envelope.TriggerSpec),
		ActorType: string(envelope.ActorType), ActorID: envelope.ActorID, SourceType: sourceString(envelope.SourceType),
		ObserverType: cloneString(envelope.ObserverType), SourceInstanceID: cloneString(envelope.SourceInstanceID), SourceEventID: cloneString(envelope.SourceEventID),
		SourceOccurredAt: cloneTime(envelope.SourceOccurredAt), SourceReplayPolicyGeneration: cloneString(envelope.SourceReplayPolicyGeneration),
		SourceReplayDeadline: cloneTime(envelope.SourceReplayDeadline), ArgumentsSummary: redactedDocument,
		ArgumentsFingerprint: req.ArgumentsFingerprint, ConversationID: cloneString(envelope.ConversationID), TurnID: cloneString(envelope.TurnID),
		SurfaceType: cloneString(envelope.SurfaceType), SurfaceID: cloneString(envelope.SurfaceID), SurfaceSnapshotVersion: cloneString(envelope.SurfaceSnapshotVersion),
		ContextVersion: cloneString(envelope.ContextVersion), ContextSummary: redactedIfPresent(envelope.ContextVersion), ForegroundSummary: redactedIfPresent(envelope.ForegroundSnapshot),
		SourceProfileSlug: cloneString(envelope.SourceProfileSlug), TargetProfileSlug: cloneString(envelope.TargetProfileSlug),
		AuthorizationDecisionID: cloneString(envelope.AuthorizationDecisionID), DelegationFingerprint: cloneString(envelope.DelegationFingerprint),
		GrantGeneration: cloneString(envelope.GrantGeneration), JobID: cloneString(envelope.JobID), JobSlug: cloneString(envelope.JobSlug),
		JobDefinitionFingerprint: cloneString(envelope.JobDefinitionFingerprint), RunID: cloneString(envelope.RunID), Provenance: redactedIfPresent(envelope.Provenance),
		CorrelationID: envelope.CorrelationID, RequestFingerprintVersion: *envelope.RequestFingerprintVersion, RequestFingerprint: *envelope.RequestFingerprint,
		Risk: req.Risk, PolicyDecision: policy, Status: status, ClientRequestedAt: cloneTime(envelope.ClientRequestedAt), ReceivedAt: envelope.ReceivedAt,
	}
	if status == Denied {
		summary, code, ref, err := terminalResult(status, EnvelopeResult{})
		if err != nil {
			return invocationRow{}, err
		}
		row.ResultSummary, row.ResultRef, row.ErrorCode, row.CompletedAt = envelopeStringPtr(summary), ref, envelopeStringPtr(code), timePtr(now)
	}
	return row, nil
}

func findExistingEnvelope(tx *gorm.DB, envelope commandcontract.Envelope, key string) (ledgerRow, error) {
	query := tx.Where("key = ? OR invocation_id = ?", key, envelope.InvocationID)
	if envelope.SourceEventID != nil {
		query = query.Or("source_event_id = ?", *envelope.SourceEventID)
	}
	var row ledgerRow
	err := query.First(&row).Error
	return row, err
}

func sameEnvelopeIdentity(row ledgerRow, envelope commandcontract.Envelope, key string, owner FullOwnership, inputFingerprint string) bool {
	return row.Key == key && row.InvocationID == envelope.InvocationID && sameOwnership(row, owner) &&
		row.RequestFingerprintVersion == valueOrEmpty(envelope.RequestFingerprintVersion) && row.RequestFingerprint == valueOrEmpty(envelope.RequestFingerprint) &&
		valueOrEmpty(row.InputFingerprint) == inputFingerprint
}

func ownershipFromEnvelope(envelope commandcontract.Envelope) FullOwnership {
	return FullOwnership{UserID: cloneString(envelope.UserID), AuthContextType: envelope.AuthContextType, AuthContextID: envelope.AuthContextID, ActorType: envelope.ActorType, ActorID: envelope.ActorID}
}

func loadInvocation(db *gorm.DB, id string, owner FullOwnership) (*invocationRow, error) {
	var row invocationRow
	err := applyOwnership(db.Model(&invocationRow{}).Where("invocation_id = ?", id), owner).First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func applyOwnership(query *gorm.DB, owner FullOwnership) *gorm.DB {
	query = query.Where("auth_context_type = ? AND auth_context_id = ? AND actor_type = ? AND actor_id = ?", string(owner.AuthContextType), owner.AuthContextID, string(owner.ActorType), owner.ActorID)
	if owner.UserID == nil {
		return query.Where("user_id IS NULL")
	}
	return query.Where("user_id = ?", *owner.UserID)
}

func validateFullOwnership(owner FullOwnership) error {
	if owner.AuthContextID == "" || strings.TrimSpace(owner.AuthContextID) != owner.AuthContextID || owner.ActorID == "" || strings.TrimSpace(owner.ActorID) != owner.ActorID {
		return ErrInvalidRequest
	}
	if owner.UserID != nil && !validUUID(*owner.UserID) {
		return ErrInvalidRequest
	}
	switch owner.AuthContextType {
	case commandcontract.AuthLocalSession, commandcontract.AuthExternalToken, commandcontract.AuthJobService, commandcontract.AuthSystem:
	default:
		return ErrInvalidRequest
	}
	switch owner.ActorType {
	case commandcontract.ActorUser, commandcontract.ActorAgent, commandcontract.ActorAutomation:
	default:
		return ErrInvalidRequest
	}
	return nil
}

func sameOwnership(row ledgerRow, owner FullOwnership) bool {
	if (row.UserID == nil) != (owner.UserID == nil) || (owner.UserID != nil && *row.UserID != *owner.UserID) ||
		row.AuthContextType != string(owner.AuthContextType) || row.AuthContextID != owner.AuthContextID || row.ActorType == nil || row.ActorID == nil {
		return false
	}
	return *row.ActorType == string(owner.ActorType) && *row.ActorID == owner.ActorID
}

func fullRecord(ledger ledgerRow, invocation *invocationRow) FullRecord {
	record := FullRecord{ID: ledger.ID, Key: ledger.Key, InvocationID: ledger.InvocationID, Status: ledger.Status,
		RequestFingerprintVersion: ledger.RequestFingerprintVersion, RequestFingerprint: ledger.RequestFingerprint,
		InputFingerprint: valueOrEmpty(ledger.InputFingerprint),
		ResultSummary:    cloneString(ledger.ResultSummary), ResultRef: cloneString(ledger.ResultRef), SourceEventID: cloneString(ledger.SourceEventID),
		ReceivedAt: ledger.ReceivedAt, ExpiresAt: ledger.ExpiresAt, Ownership: FullOwnership{UserID: cloneString(ledger.UserID), AuthContextType: commandcontract.AuthContextType(ledger.AuthContextType), AuthContextID: ledger.AuthContextID}}
	if ledger.ActorType != nil {
		record.Ownership.ActorType = commandcontract.ActorType(*ledger.ActorType)
	}
	if ledger.ActorID != nil {
		record.Ownership.ActorID = *ledger.ActorID
	}
	record.Mode = modeForStatus(ledger.Status)
	if invocation == nil {
		return record
	}
	record.Ownership = FullOwnership{UserID: cloneString(invocation.UserID), AuthContextType: commandcontract.AuthContextType(invocation.AuthContextType), AuthContextID: invocation.AuthContextID, ActorType: commandcontract.ActorType(invocation.ActorType), ActorID: invocation.ActorID}
	record.Mode = modeForStatus(invocation.Status)
	record.ArgumentsSummary, record.ArgumentsFingerprint, record.Risk = invocation.ArgumentsSummary, invocation.ArgumentsFingerprint, invocation.Risk
	record.Status, record.ResultSummary, record.ResultRef, record.ErrorCode = invocation.Status, cloneString(invocation.ResultSummary), cloneString(invocation.ResultRef), cloneString(invocation.ErrorCode)
	record.Envelope = envelopeFromInvocation(*invocation, record)
	return record
}

func envelopeFromInvocation(row invocationRow, record FullRecord) commandcontract.Envelope {
	envelope := commandcontract.Envelope{Version: row.SchemaVersion, InvocationID: row.InvocationID, AuthContextType: commandcontract.AuthContextType(row.AuthContextType), AuthContextID: row.AuthContextID, AuthGeneration: row.AuthGeneration, SecurityGeneration: row.SecurityGeneration, ActorType: commandcontract.ActorType(row.ActorType), ActorID: row.ActorID, RegistryVersion: row.RegistryVersion, CorrelationID: row.CorrelationID, ReceivedAt: row.ReceivedAt, BindingIDs: decodeBindingIDs(row.BindingIDs), UserID: cloneString(row.UserID), SessionID: cloneString(row.SessionID), WorkspaceID: cloneString(row.WorkspaceID), CommandID: cloneString(row.CommandID), ObservedTriggerType: cloneString(row.ObservedTriggerType), TriggerType: cloneString(row.TriggerType), SourceType: sourceTypePtr(row.SourceType), SourceInstanceID: cloneString(row.SourceInstanceID), SourceEventID: cloneString(row.SourceEventID), SourceOccurredAt: cloneTime(row.SourceOccurredAt), SourceReplayPolicyGeneration: cloneString(row.SourceReplayPolicyGeneration), SourceReplayDeadline: cloneTime(row.SourceReplayDeadline), GlobalConfigGeneration: cloneString(row.GlobalConfigGeneration), WorkspaceConfigGeneration: cloneString(row.WorkspaceConfigGeneration), ActiveLayersGeneration: cloneString(row.ActiveLayersGeneration), ConversationID: cloneString(row.ConversationID), TurnID: cloneString(row.TurnID), SurfaceType: cloneString(row.SurfaceType), SurfaceID: cloneString(row.SurfaceID), SurfaceSnapshotVersion: cloneString(row.SurfaceSnapshotVersion), ContextVersion: cloneString(row.ContextVersion), SourceProfileSlug: cloneString(row.SourceProfileSlug), TargetProfileSlug: cloneString(row.TargetProfileSlug), AuthorizationDecisionID: cloneString(row.AuthorizationDecisionID), DelegationFingerprint: cloneString(row.DelegationFingerprint), GrantGeneration: cloneString(row.GrantGeneration), JobID: cloneString(row.JobID), JobSlug: cloneString(row.JobSlug), JobDefinitionFingerprint: cloneString(row.JobDefinitionFingerprint), RunID: cloneString(row.RunID), RequestFingerprintVersion: cloneString(&row.RequestFingerprintVersion), RequestFingerprint: cloneString(&row.RequestFingerprint), ClientRequestedAt: cloneTime(row.ClientRequestedAt)}
	arguments := json.RawMessage(record.ArgumentsSummary)
	envelope.Arguments = &arguments
	return envelope
}

func decodeBindingIDs(value string) []string {
	var IDs []string
	if json.Unmarshal([]byte(value), &IDs) != nil || IDs == nil {
		return []string{}
	}
	return IDs
}

func modeForStatus(status Status) Mode {
	if status == Suppressed {
		return ModeSuppress
	}
	if status == Denied {
		return ModeDenied
	}
	return ModeExecute
}

func redactedIfPresent[T any](value *T) *string {
	if value == nil {
		return nil
	}
	return envelopeStringPtr(redactedDocument)
}

func sourceString(value *commandcontract.SourceType) *string {
	if value == nil {
		return nil
	}
	return envelopeStringPtr(string(*value))
}

func sourceTypePtr(value *string) *commandcontract.SourceType {
	if value == nil {
		return nil
	}
	v := commandcontract.SourceType(*value)
	return &v
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	v := *value
	return &v
}

func envelopeStringPtr(value string) *string { return &value }

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	v := value.UTC()
	return &v
}

func timePtr(value time.Time) *time.Time { return &value }

func validReferenceUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.Variant() == uuid.RFC4122
}

func sanitizeEnvelopeError(err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrInvalidRequest) || errors.Is(err, ErrConflict) || errors.Is(err, ErrExpired) || errors.Is(err, ErrInconsistent) || errors.Is(err, ErrInvalidTransition) || errors.Is(err, ErrNotFound) || errors.Is(err, ErrVerifierPanic) || errors.Is(err, commandcontract.ErrInvalidEnvelope) {
		return err
	}
	return ErrInconsistent
}
