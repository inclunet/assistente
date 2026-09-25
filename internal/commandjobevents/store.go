package commandjobevents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"assistente/internal/commandjson"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrSchemaUnavailable   = errors.New("command job events schema unavailable")
	ErrBootstrapIncomplete = errors.New("command job events bootstrap incomplete")
	ErrInvalidFact         = errors.New("invalid command job event fact")
	ErrFingerprintConflict = errors.New("command job event fingerprint conflict")
	ErrLeaseLost           = errors.New("command job event lease lost")
)

const EventFingerprintDomain = "assistente.command.job-run-state.v1"

const (
	DefaultReplayHorizon = 24 * time.Hour
	DefaultLeaseDuration = 3 * time.Minute
	DefaultMaxAttempts   = 8
	maxRequeueBatch      = 128
	maxPurgeBatch        = 128
)

// Store contém apenas operações de outbox e epochs. A decisão de habilitar o
// consumidor pertence às claims/manutenção da AEP-0103, ainda não integradas.
type Store struct {
	db            *gorm.DB
	now           func() time.Time
	leaseDuration time.Duration
	maxAttempts   int
	replayHorizon time.Duration
}

func NewStore(db *gorm.DB) *Store {
	return &Store{
		db: db, now: time.Now, leaseDuration: DefaultLeaseDuration,
		maxAttempts: DefaultMaxAttempts, replayHorizon: DefaultReplayHorizon,
	}
}

func (s *Store) DB() *gorm.DB {
	if s == nil {
		return nil
	}
	return s.db
}

func (s *Store) Available() bool {
	return s != nil && s.db != nil && s.db.Migrator().HasTable(&ActivationOutbox{}) && s.db.Migrator().HasTable(&ReplayPolicyEpoch{})
}

// Ready é a disponibilidade mínima do produtor. A migração das tabelas, por
// si só, não prova que o bootstrap registrou um epoch de replay para o
// produtor. Sem esse epoch, fatos elegíveis permanecem fora da outbox até a
// integração central concluir o bootstrap.
func (s *Store) Ready(ctx context.Context) bool {
	if ctx == nil || ctx.Err() != nil {
		return false
	}
	if !s.Available() {
		return false
	}
	var epoch ReplayPolicyEpoch
	err := s.db.WithContext(ctx).Where("producer_type = ? AND generation > 0", ProducerType).Order("generation DESC").First(&epoch).Error
	return err == nil && ctx.Err() == nil && epoch.ReplayHorizonSeconds > 0
}

func (s *Store) configure(now func() time.Time, lease, horizon time.Duration, maxAttempts int) {
	if now != nil {
		s.now = now
	}
	if lease > 0 {
		s.leaseDuration = lease
	}
	if horizon > 0 {
		s.replayHorizon = horizon
	}
	if maxAttempts > 0 {
		s.maxAttempts = maxAttempts
	}
}

// Fingerprint calcula a identidade semântica do fato sem incluir o próprio
// fingerprint. O JSON é redigido pelo chamador antes de chegar aqui.
func Fingerprint(fact Fact) (string, error) {
	copy := fact
	copy.EventFingerprint = ""
	b, err := commandjson.Marshal(copy)
	if err != nil {
		return "", fmt.Errorf("fingerprint fact: %w", err)
	}
	sum := sha256.Sum256(append([]byte(EventFingerprintDomain+"\x00"), b...))
	return hex.EncodeToString(sum[:]), nil
}

func validateFact(fact Fact) error {
	for name, value := range map[string]string{
		"schema version": fact.SchemaVersion, "event name": fact.EventName,
		"source event id": fact.SourceEventID, "user id": fact.UserID,
		"job database id": fact.JobDatabaseID, "job slug": fact.JobSlug,
		"run id": fact.RunID, "run event id": fact.RunEventID,
		"root origin type": fact.RootOriginType, "root origin id": fact.RootOriginID,
		"replay generation": fact.SourceReplayPolicyGeneration,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%w: %s vazio", ErrInvalidFact, name)
		}
	}
	if fact.SchemaVersion != SchemaVersion || fact.EventName != SchemaVersion || fact.RunEventID != fact.SourceEventID {
		return fmt.Errorf("%w: versão ou IDs inconsistentes", ErrInvalidFact)
	}
	for name, value := range map[string]string{
		"source event id": fact.SourceEventID, "user id": fact.UserID,
		"job database id": fact.JobDatabaseID, "run event id": fact.RunEventID,
	} {
		if !isCanonicalUUID7(value) {
			return fmt.Errorf("%w: %s não é UUIDv7 canônico", ErrInvalidFact, name)
		}
	}
	switch fact.State {
	case StateQueued, StateStarted, StateRetryScheduled, StateCompleted, StateFailed, StateSkipped:
	default:
		return fmt.Errorf("%w: estado %q não permitido", ErrInvalidFact, fact.State)
	}
	if fact.Sequence <= 0 || fact.OccurredAt.IsZero() || fact.SourceReplayDeadline.IsZero() || !fact.SourceReplayDeadline.After(fact.OccurredAt) {
		return fmt.Errorf("%w: sequência/timestamps inválidos", ErrInvalidFact)
	}
	if fact.RootOriginType != "manual" && fact.RootOriginType != "cron" && fact.RootOriginType != "interval" && fact.RootOriginType != "user_hotkey" && fact.RootOriginType != "internal_event" {
		return fmt.Errorf("%w: raiz não elegível %q", ErrInvalidFact, fact.RootOriginType)
	}
	return nil
}

func isCanonicalUUID7(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.Version() == 7 && id.Variant() == uuid.RFC4122 && id.String() == value
}

func (s *Store) ensureSchema(tx *gorm.DB) error {
	if s == nil || tx == nil || !tx.Migrator().HasTable(&ActivationOutbox{}) || !tx.Migrator().HasTable(&ReplayPolicyEpoch{}) {
		return ErrSchemaUnavailable
	}
	return nil
}

// EnsureReplayPolicyEpoch cria ou retorna o epoch vigente para a mesma
// ocorrência. Gerações são monotônicas por produtor; não há atualização in
// place de um epoch já usado.
func (s *Store) EnsureReplayPolicyEpoch(ctx context.Context, producer string, effectiveAt time.Time, horizon time.Duration) (ReplayPolicyEpoch, error) {
	effectiveAt = effectiveAt.UTC()
	if s == nil || s.db == nil || strings.TrimSpace(producer) == "" || effectiveAt.IsZero() || horizon <= 0 {
		return ReplayPolicyEpoch{}, ErrInvalidFact
	}
	var result ReplayPolicyEpoch
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.ensureSchema(tx); err != nil {
			return err
		}
		return s.ensureEpochTx(tx, producer, effectiveAt, horizon, &result)
	})
	return result, err
}

func (s *Store) ensureEpochTx(tx *gorm.DB, producer string, effectiveAt time.Time, horizon time.Duration, out *ReplayPolicyEpoch) error {
	effectiveAt = effectiveAt.UTC()
	if producer != ProducerType || effectiveAt.IsZero() || horizon <= 0 {
		return ErrInvalidFact
	}
	horizonSeconds := int64(horizon / time.Second)
	if horizonSeconds <= 0 {
		return ErrInvalidFact
	}
	var existing ReplayPolicyEpoch
	err := tx.Where("producer_type = ? AND effective_at = ?", producer, effectiveAt).First(&existing).Error
	if err == nil {
		if existing.ReplayHorizonSeconds != horizonSeconds {
			return ErrFingerprintConflict
		}
		*out = existing
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	var generation int64
	if err := tx.Model(&ReplayPolicyEpoch{}).Where("producer_type = ?", producer).Select("COALESCE(MAX(generation), 0)").Scan(&generation).Error; err != nil {
		return err
	}
	if generation < 0 || generation == int64(^uint64(0)>>1) {
		return ErrInvalidFact
	}
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	row := ReplayPolicyEpoch{ID: id.String(), ProducerType: producer, Generation: generation + 1, EffectiveAt: effectiveAt, ReplayHorizonSeconds: horizonSeconds, CreatedAt: s.now().UTC()}
	if row.ReplayHorizonSeconds <= 0 {
		return ErrInvalidFact
	}
	if err := tx.Create(&row).Error; err != nil {
		return err
	}
	*out = row
	return nil
}

func (s *Store) currentEpochTx(tx *gorm.DB, producer string, at time.Time) (ReplayPolicyEpoch, error) {
	at = at.UTC()
	var epoch ReplayPolicyEpoch
	err := tx.Where("producer_type = ? AND effective_at <= ?", producer, at).Order("effective_at DESC, generation DESC").First(&epoch).Error
	if err == nil {
		return epoch, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return ReplayPolicyEpoch{}, err
	}
	return ReplayPolicyEpoch{}, gorm.ErrRecordNotFound
}

// InsertFactTx insere a ocorrência na transação do job_run_event. Reentrega
// idêntica é no-op; o mesmo source_event_id com conteúdo diferente é conflito.
func (s *Store) InsertFactTx(tx *gorm.DB, fact Fact) error {
	if err := s.ensureSchema(tx); err != nil {
		return err
	}
	fact.OccurredAt = fact.OccurredAt.UTC()
	epoch, err := s.currentEpochTx(tx, ProducerType, fact.OccurredAt)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrBootstrapIncomplete
	}
	if err != nil {
		return err
	}
	// Geração e deadline pertencem ao epoch persistido, nunca ao payload do
	// produtor. Isso também torna o replay imutável quando a política muda.
	fact.SourceReplayPolicyGeneration = fmt.Sprintf("%s:%d", ProducerType, epoch.Generation)
	fact.SourceReplayDeadline = fact.OccurredAt.Add(time.Duration(epoch.ReplayHorizonSeconds) * time.Second)
	if err := validateFact(fact); err != nil {
		return err
	}
	fingerprint, err := Fingerprint(fact)
	if err != nil {
		return err
	}
	if fact.EventFingerprint != "" && fact.EventFingerprint != fingerprint {
		return ErrFingerprintConflict
	}
	fact.EventFingerprint = fingerprint
	provenance, err := marshalMap(fact.Provenance)
	if err != nil {
		return err
	}
	row := ActivationOutbox{
		SourceEventID: fact.SourceEventID, SchemaVersion: fact.SchemaVersion, EventName: fact.EventName,
		UserID: fact.UserID, JobDatabaseID: fact.JobDatabaseID, JobSlug: fact.JobSlug, RunID: fact.RunID,
		Sequence: fact.Sequence, State: fact.State, OccurredAt: fact.OccurredAt,
		RootOriginType: fact.RootOriginType, RootOriginID: fact.RootOriginID, Provenance: provenance,
		SourceReplayPolicyGeneration: fact.SourceReplayPolicyGeneration, SourceReplayDeadline: fact.SourceReplayDeadline,
		EventFingerprint: fingerprint, DeliveryState: DeliveryPending, CreatedAt: s.now().UTC(),
	}
	var existing ActivationOutbox
	err = tx.Where("source_event_id = ?", fact.SourceEventID).First(&existing).Error
	if err == nil {
		if existing.EventFingerprint != fingerprint {
			return ErrFingerprintConflict
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return tx.Create(&row).Error
}

func marshalMap(value map[string]any) (string, error) {
	if len(value) == 0 {
		return "", nil
	}
	b, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal provenance: %w", err)
	}
	return string(b), nil
}

func (s *Store) Get(ctx context.Context, sourceEventID string) (*ActivationOutbox, error) {
	if s == nil || s.db == nil || strings.TrimSpace(sourceEventID) == "" {
		return nil, ErrInvalidFact
	}
	if !s.Available() {
		return nil, ErrSchemaUnavailable
	}
	var row ActivationOutbox
	if err := s.db.WithContext(ctx).Where("source_event_id = ?", sourceEventID).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// Claim reserva pendências (ou leases vencidas) atomically. O owner é opaco e
// não é usado como identidade de usuário.
func (s *Store) Claim(ctx context.Context, owner string, limit int) ([]ActivationOutbox, error) {
	if limit <= 0 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}
	claimed, _, err := s.ClaimBatch(ctx, owner, limit)
	return claimed, err
}

// ClaimBatch reivindica no máximo limit ocorrências e informa se havia outra
// pendência no instante da seleção. A identidade do owner é somente a
// capacidade opaca de entrega; autenticação e autorização do fato acontecem
// depois, em Consumer.Consume.
func (s *Store) ClaimBatch(ctx context.Context, owner string, limit int) ([]ActivationOutbox, bool, error) {
	if s == nil || s.db == nil || ctx == nil || strings.TrimSpace(owner) == "" || limit <= 0 || limit > 100 {
		return nil, false, ErrInvalidFact
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if !s.Available() {
		return nil, false, ErrSchemaUnavailable
	}
	now := s.now().UTC()
	expiry := now.Add(s.leaseDuration)
	var claimed []ActivationOutbox
	var more bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []ActivationOutbox
		if err := tx.Where("delivery_state = ? OR (delivery_state = ? AND lease_expires_at <= ?)", DeliveryPending, DeliveryProcessing, now).
			Order("created_at ASC, source_event_id ASC").Limit(limit + 1).Find(&rows).Error; err != nil {
			return err
		}
		more = len(rows) > limit
		if more {
			rows = rows[:limit]
		}
		for _, row := range rows {
			if err := ctx.Err(); err != nil {
				return err
			}
			if row.DeliveryState == DeliveryProcessing && row.Attempts >= s.maxAttempts {
				res := tx.Model(&ActivationOutbox{}).Where("source_event_id = ? AND delivery_state = ? AND lease_expires_at <= ? AND attempts >= ?", row.SourceEventID, DeliveryProcessing, now, s.maxAttempts).
					Updates(map[string]any{"delivery_state": DeliveryDeadLetter, "lease_owner": nil, "lease_expires_at": nil, "last_error_code": "attempt_limit"})
				if res.Error != nil {
					return res.Error
				}
				continue
			}
			res := tx.Model(&ActivationOutbox{}).Where("source_event_id = ? AND (delivery_state = ? OR (delivery_state = ? AND lease_expires_at <= ?))", row.SourceEventID, DeliveryPending, DeliveryProcessing, now).
				Updates(map[string]any{"delivery_state": DeliveryProcessing, "lease_owner": owner, "lease_expires_at": expiry, "attempts": gorm.Expr("attempts + 1")})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 1 {
				row.DeliveryState = DeliveryProcessing
				row.LeaseOwner = &owner
				row.LeaseExpiresAt = &expiry
				row.Attempts++
				claimed = append(claimed, row)
			}
		}
		return ctx.Err()
	})
	if err != nil {
		return nil, false, err
	}
	return claimed, more, nil
}

func (s *Store) Ack(ctx context.Context, sourceEventID, owner string) error {
	return s.updateLease(ctx, sourceEventID, owner, map[string]any{"delivery_state": DeliveryDelivered, "lease_owner": nil, "lease_expires_at": nil, "delivered_at": s.now().UTC()})
}

func (s *Store) Retry(ctx context.Context, sourceEventID, owner, errorCode string) error {
	if s == nil || s.db == nil || !s.Available() {
		return ErrSchemaUnavailable
	}
	if strings.TrimSpace(errorCode) == "" {
		return ErrInvalidFact
	}
	var row ActivationOutbox
	if err := s.db.WithContext(ctx).Where("source_event_id = ?", sourceEventID).First(&row).Error; err != nil {
		return err
	}
	if row.Attempts >= s.maxAttempts {
		return s.DeadLetter(ctx, sourceEventID, owner, errorCode)
	}
	return s.updateLease(ctx, sourceEventID, owner, map[string]any{"delivery_state": DeliveryPending, "lease_owner": nil, "lease_expires_at": nil, "last_error_code": errorCode})
}

func (s *Store) DeadLetter(ctx context.Context, sourceEventID, owner, errorCode string) error {
	if strings.TrimSpace(errorCode) == "" {
		return ErrInvalidFact
	}
	return s.updateLease(ctx, sourceEventID, owner, map[string]any{"delivery_state": DeliveryDeadLetter, "lease_owner": nil, "lease_expires_at": nil, "last_error_code": errorCode})
}

func (s *Store) updateLease(ctx context.Context, sourceEventID, owner string, values map[string]any) error {
	if s == nil || s.db == nil || !s.Available() {
		return ErrSchemaUnavailable
	}
	if strings.TrimSpace(sourceEventID) == "" || strings.TrimSpace(owner) == "" {
		return ErrInvalidFact
	}
	res := s.db.WithContext(ctx).Model(&ActivationOutbox{}).Where("source_event_id = ? AND delivery_state = ? AND lease_owner = ? AND lease_expires_at > ?", sourceEventID, DeliveryProcessing, owner, s.now().UTC()).Updates(values)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrLeaseLost
	}
	return nil
}

// RequeueExpiredLeases é chamado pelo coordenador de manutenção. Cada chamada
// processa no máximo limit rows e informa se havia mais trabalho no momento da
// seleção; não cria loop nem altera leases que já foram renovadas em paralelo.
func (s *Store) RequeueExpiredLeases(ctx context.Context, limit int) (processed int, more bool, err error) {
	if ctx == nil || limit <= 0 || limit > maxRequeueBatch {
		return 0, false, ErrInvalidFact
	}
	if err := ctx.Err(); err != nil {
		return 0, false, err
	}
	if s == nil || s.db == nil || !s.Available() {
		return 0, false, ErrSchemaUnavailable
	}
	now := s.now().UTC()
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var rows []ActivationOutbox
		if err := tx.Model(&ActivationOutbox{}).
			Where("delivery_state = ? AND lease_expires_at IS NOT NULL AND lease_expires_at <= ?", DeliveryProcessing, now).
			Order("source_event_id ASC").Limit(limit + 1).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) > limit {
			more = true
			rows = rows[:limit]
		}
		if len(rows) == 0 {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		for _, row := range rows {
			state := DeliveryPending
			values := map[string]any{"delivery_state": state, "lease_owner": nil, "lease_expires_at": nil}
			if row.Attempts >= s.maxAttempts {
				values["delivery_state"] = DeliveryDeadLetter
				values["last_error_code"] = "attempt_limit"
			}
			result := tx.Model(&ActivationOutbox{}).
				Where("source_event_id = ? AND delivery_state = ? AND lease_expires_at IS NOT NULL AND lease_expires_at <= ?", row.SourceEventID, DeliveryProcessing, now).
				Updates(values)
			if result.Error != nil {
				return result.Error
			}
			processed += int(result.RowsAffected)
		}
		return nil
	})
	return processed, more, err
}

// PurgeExpired remove somente entregas terminais cujo horizonte de replay
// venceu. A ocorrência continua sendo a fonte autoritativa de um runtime que
// ainda possui claim ativa e lease viva; por isso essa relação é rechecada na
// seleção e no DELETE, dentro da mesma transação. Sem as tabelas de claim e
// lease, a operação falha fechado e não remove nada.
func (s *Store) PurgeExpired(ctx context.Context, limit int) (processed int, more bool, err error) {
	if ctx == nil || limit <= 0 || limit > maxPurgeBatch {
		return 0, false, ErrInvalidFact
	}
	if s == nil || s.db == nil || !s.Available() {
		return 0, false, ErrSchemaUnavailable
	}
	now := s.now().UTC()
	return s.PurgeExpiredAt(ctx, now, limit)
}

// PurgeExpiredAt é a forma usada pelo coordenador quando ele já possui o
// relógio da passagem. Mantém a decisão de retenção determinística e não
// permite que o caller forneça a ocorrência ou altere sua deadline.
func (s *Store) PurgeExpiredAt(ctx context.Context, now time.Time, limit int) (processed int, more bool, err error) {
	if ctx == nil || limit <= 0 || limit > maxPurgeBatch || now.IsZero() {
		return 0, false, ErrInvalidFact
	}
	if err := ctx.Err(); err != nil {
		return 0, false, err
	}
	if s == nil || s.db == nil || !s.Available() {
		return 0, false, ErrSchemaUnavailable
	}
	const liveClaim = `EXISTS (
		SELECT 1
		FROM command_layer_activation_state claim
		JOIN command_job_activation_leases lease
		  ON lease.activation_id = claim.activation_id
		 AND lease.user_id = claim.user_id
		 AND lease.run_id = command_job_activation_outbox.run_id
		WHERE claim.user_id = command_job_activation_outbox.user_id
		  AND claim.source_type = 'job'
		  AND claim.source_event_id = command_job_activation_outbox.source_event_id
		  AND claim.source_correlation_id = command_job_activation_outbox.run_id
		  AND claim.state = 'active'
		  AND (claim.expires_at IS NULL OR claim.expires_at > ?)
		  AND lease.expires_at > ?
	)`
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if !tx.Migrator().HasTable("command_layer_activation_state") || !tx.Migrator().HasTable("command_job_activation_leases") {
			return ErrSchemaUnavailable
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		var ids []string
		if err := tx.Model(&ActivationOutbox{}).
			Where("delivery_state IN ? AND source_replay_deadline <= ? AND NOT "+liveClaim, []string{DeliveryDelivered, DeliveryDeadLetter}, now, now, now).
			Order("source_event_id ASC").Limit(limit+1).Pluck("source_event_id", &ids).Error; err != nil {
			return err
		}
		if len(ids) > limit {
			more = true
			ids = ids[:limit]
		}
		if len(ids) == 0 {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		result := tx.Where("source_event_id IN ? AND delivery_state IN ? AND source_replay_deadline <= ? AND NOT "+liveClaim, ids, []string{DeliveryDelivered, DeliveryDeadLetter}, now, now, now).Delete(&ActivationOutbox{})
		processed = int(result.RowsAffected)
		return result.Error
	})
	if err != nil {
		return 0, false, err
	}
	return processed, more, err
}
