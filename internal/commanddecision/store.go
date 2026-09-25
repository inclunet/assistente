package commanddecision

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type receiptRow struct {
	ID                 string `gorm:"column:decision_id;primaryKey;not null"`
	MutationID         string `gorm:"column:subject_id;not null"`
	UserID             string `gorm:"not null"`
	SessionID          string `gorm:"column:auth_context_id;not null"`
	Fingerprint        string `gorm:"column:request_fingerprint;not null"`
	AuthGeneration     string `gorm:"not null"`
	SecurityGeneration string `gorm:"not null"`
	ExpiresMS          int64  `gorm:"column:expires_at;not null"`
	State              string `gorm:"column:status;not null;check:status IN ('pending','accepted','denied','cancelled','expired','consumed')"`
	AuthContextType    string `gorm:"not null;check:auth_context_subject,auth_context_type IN ('local_session','external_token') AND (auth_context_type <> 'external_token' OR subject_type = 'invocation')"`
	SubjectType        string `gorm:"not null;check:subject_type IN ('config_mutation','invocation')"`
	AllowedActionIDs   string `gorm:"not null"`
	AcceptedActionID   *string
	RespondedAt        *int64
	ConsumedAt         *int64
}

func (receiptRow) TableName() string { return "command_decision_receipts" }

type auditRow struct {
	ID         string `gorm:"primaryKey;not null"`
	DecisionID string `gorm:"not null;index"`
	State      string `gorm:"not null;check:state IN ('pending','accepted','denied','cancelled','expired','consumed')"`
	OccurredMS int64  `gorm:"not null"`
}

func (auditRow) TableName() string { return "command_decision_receipt_events" }

type Store struct {
	db        *gorm.DB
	presenter Presenter
	now       func() time.Time
}

// Migrate é explícita e transacional. Não faz parte das migrações do App.
func Migrate(ctx context.Context, db *gorm.DB) error {
	if ctx == nil || db == nil {
		return ErrInvalid
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.AutoMigrate(&receiptRow{}, &auditRow{}); err != nil {
			return err
		}
		const statement = "CREATE INDEX ix_command_decision_recovery_session ON command_decision_receipts (user_id, auth_context_id, status, decision_id)"
		var existing struct {
			Type string
			SQL  string
		}
		if err := tx.Raw("SELECT type, sql FROM sqlite_master WHERE name = ?", "ix_command_decision_recovery_session").Scan(&existing).Error; err != nil {
			return err
		}
		if existing.Type != "" {
			if existing.Type != "index" || strings.Join(strings.Fields(strings.ToLower(existing.SQL)), " ") != strings.ToLower(statement) {
				return ErrInvalid
			}
			return nil
		}
		return tx.Exec(statement).Error
	})
}

func New(db *gorm.DB, presenter Presenter, now func() time.Time) (*Store, error) {
	if db == nil || presenter == nil || now == nil {
		return nil, ErrInvalid
	}
	return &Store{db: db, presenter: presenter, now: now}, nil
}

func validID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.Version() == 7 && id.Variant() == uuid.RFC4122 && id.String() == value
}
func validRequest(r Request) bool {
	authContextType := effectiveAuthContextType(r.AuthContextType)
	if authContextType != "local_session" && authContextType != "external_token" {
		return false
	}
	subjectType := effectiveSubjectType(r.SubjectType)
	if subjectType != "config_mutation" && subjectType != "invocation" {
		return false
	}
	if authContextType == "external_token" && subjectType != "invocation" {
		return false
	}
	if !validID(r.DecisionID) || !validID(r.MutationID) || !validID(r.UserID) || !validAuthContextID(authContextType, r.SessionID) || r.ExpiresAt.UnixMilli() <= 0 {
		return false
	}
	for _, value := range []string{r.Fingerprint, r.AuthGeneration, r.SecurityGeneration} {
		if strings.TrimSpace(value) != value || value == "" || len(value) > 256 || !utf8.ValidString(value) {
			return false
		}
	}
	return true
}

func effectiveAuthContextType(value string) string {
	if value == "" {
		return "local_session"
	}
	return value
}

func effectiveSubjectType(value string) string {
	if value == "" {
		return "config_mutation"
	}
	return value
}

func validAuthContextID(authContextType, value string) bool {
	if authContextType == "local_session" {
		return validID(value)
	}
	if authContextType != "external_token" || value == "" || !utf8.ValidString(value) {
		return false
	}
	var parts []string
	if err := json.Unmarshal([]byte(value), &parts); err != nil || len(parts) != 3 {
		return false
	}
	for _, part := range parts[:2] {
		if part == "" || strings.TrimSpace(part) != part || strings.IndexByte(part, 0) >= 0 {
			return false
		}
		for _, r := range part {
			if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
				return false
			}
		}
	}
	fingerprint, err := hex.DecodeString(parts[2])
	if err != nil || len(fingerprint) != 32 || hex.EncodeToString(fingerprint) != parts[2] {
		return false
	}
	canonical, err := json.Marshal(parts)
	return err == nil && string(canonical) == value
}

func rowOf(r Request) receiptRow {
	subject := effectiveSubjectType(r.SubjectType)
	return receiptRow{ID: r.DecisionID, MutationID: r.MutationID, UserID: r.UserID, SessionID: r.SessionID,
		Fingerprint: r.Fingerprint, AuthGeneration: r.AuthGeneration, SecurityGeneration: r.SecurityGeneration, ExpiresMS: r.ExpiresAt.UnixMilli(), State: Pending,
		AuthContextType: effectiveAuthContextType(r.AuthContextType), SubjectType: subject, AllowedActionIDs: `["apply","deny"]`}
}
func appendEvent(tx *gorm.DB, id, state string, now time.Time) error {
	eventID, err := uuid.NewV7()
	if err != nil {
		return err
	}
	return tx.Create(&auditRow{ID: eventID.String(), DecisionID: id, State: state, OccurredMS: now.UnixMilli()}).Error
}

// Decide persiste pending antes de apresentar. Só uma resposta do presenter
// confiável pode fazer o CAS terminal. Deadline inclui espera na fila da UI.
// Erro/cancelamento nunca produz receipt afirmativa. Não repete apresentações.
func (s *Store) Decide(ctx context.Context, request Request) (string, error) {
	if s == nil || s.db == nil || s.presenter == nil || s.now == nil || ctx == nil || !validRequest(request) ||
		strings.TrimSpace(request.Body) == "" || len(request.Body) > 64*1024 || !utf8.ValidString(request.Body) {
		return "", ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	request.ExpiresAt = time.UnixMilli(request.ExpiresAt.UnixMilli()).UTC()
	if !request.ExpiresAt.After(s.now()) {
		return "", ErrStale
	}
	row := rowOf(request)
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return appendEvent(tx, row.ID, Pending, s.now())
	}); err != nil {
		return "", err
	}
	decisionCtx, cancel := context.WithDeadline(ctx, request.ExpiresAt)
	defer cancel()
	response, presentationErr := present(s.presenter, decisionCtx, request)
	state := Cancelled
	if !request.ExpiresAt.After(s.now()) || decisionCtx.Err() == context.DeadlineExceeded {
		state = Expired
	} else if presentationErr == nil && decisionCtx.Err() == nil {
		if response.DecisionID != request.DecisionID || (response.Cancelled && response.ActionID != "") {
			presentationErr = ErrInvalid
		} else if response.Cancelled {
			state = Cancelled
		} else if response.ActionID == ApplyAction {
			state = Accepted
		} else if response.ActionID == DenyAction {
			state = Denied
		} else {
			presentationErr = ErrInvalid
		}
	}
	// Cancelamento do solicitante não pode impedir registrar o encerramento.
	// Usa contexto de limpeza limitado, sem reapresentar nem executar efeitos.
	cleanup, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cleanupCancel()
	if err := s.db.WithContext(cleanup).Transaction(func(tx *gorm.DB) error {
		var accepted *string
		if state == Accepted {
			action := ApplyAction
			accepted = &action
		}
		result := tx.Model(&receiptRow{}).Where("decision_id = ? AND status = ?", request.DecisionID, Pending).
			Updates(map[string]any{"status": state, "accepted_action_id": accepted, "responded_at": s.now().UnixMilli()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrStale
		}
		// A espera por um writer SQLite pode ter ultrapassado o prazo depois
		// de receber a ação. Corrige o terminal antes do evento e do commit.
		if state == Accepted && (!request.ExpiresAt.After(s.now()) || decisionCtx.Err() != nil) {
			state = Cancelled
			if !request.ExpiresAt.After(s.now()) || decisionCtx.Err() == context.DeadlineExceeded {
				state = Expired
			}
			if err := tx.Model(&receiptRow{}).Where("decision_id = ?", request.DecisionID).
				Updates(map[string]any{"status": state, "accepted_action_id": nil}).Error; err != nil {
				return err
			}
		}
		return appendEvent(tx, request.DecisionID, state, s.now())
	}); err != nil {
		return "", err
	}
	if presentationErr != nil {
		return state, presentationErr
	}
	if err := decisionCtx.Err(); err != nil {
		return state, err
	}
	return state, nil
}

func present(presenter Presenter, ctx context.Context, request Request) (response Response, err error) {
	defer func() {
		if recover() != nil {
			response = Response{}
			err = ErrInvalid
		}
	}()
	return presenter.Present(ctx, request)
}

// Consume revalida todos os vínculos e consome accepted em transação com o
// efeito local. O chamador deve deter DispatchGate, reautenticar e passar as
// gerações/fingerprint autoritativos atuais. expected não vem do cliente.
// apply deve usar SOMENTE o tx recebido, sem UI, rede ou reentrada no gate.
// Falha reverte consumo, evento e efeito; não há retry automático. Esta API
// é composta pela configuração via ConsumeForDatabase com configStore.db, de
// modo que receipt e binding compartilhem o mesmo banco; o ledger de comandos
// write ainda não está integrado.
func (s *Store) Consume(ctx context.Context, expected Request, apply func(*gorm.DB) error) error {
	var db *gorm.DB
	if s != nil {
		db = s.db
	}
	if isTransactionalDB(db) {
		return ErrInvalid
	}
	return s.consumeBatch(ctx, db, []Request{expected}, apply)
}

// ConsumeForDatabase valida que o banco da composição é a raiz não
// transacional do Store e delega o consumo à única transação aberta pelo core.
// O callback continua recebendo somente a transação criada pelo core.
func (s *Store) ConsumeForDatabase(ctx context.Context, db *gorm.DB, expected Request, apply func(*gorm.DB) error) error {
	return s.ConsumeBatchForDatabase(ctx, db, []Request{expected}, apply)
}

// ConsumeBatchForDatabase consome, atomicamente, um conjunto de receipts
// accepted e aplica um único efeito na mesma base raiz do Store. Todas as
// referências são verificadas antes de qualquer mudança persistente; o
// callback deve usar exclusivamente a transação recebida.
func (s *Store) ConsumeBatchForDatabase(ctx context.Context, db *gorm.DB, expected []Request, apply func(*gorm.DB) error) error {
	if s == nil || s.db == nil || s.db.Config == nil || db == nil || db.Config == nil || isTransactionalDB(s.db) || isTransactionalDB(db) {
		return ErrInvalid
	}
	storeSQLDB, err := s.db.DB()
	if err != nil || storeSQLDB == nil {
		return ErrInvalid
	}
	databaseSQLDB, err := db.DB()
	if err != nil || databaseSQLDB == nil || storeSQLDB != databaseSQLDB {
		return ErrInvalid
	}
	return s.consumeBatch(ctx, db, expected, apply)
}

func (s *Store) consumeBatch(ctx context.Context, db *gorm.DB, expected []Request, apply func(*gorm.DB) error) error {
	if s == nil || s.db == nil || s.now == nil || ctx == nil || db == nil || apply == nil || len(expected) == 0 {
		return ErrInvalid
	}
	expected = append([]Request(nil), expected...)
	seen := make(map[string]struct{}, len(expected))
	for i, request := range expected {
		if !validRequest(request) {
			return ErrInvalid
		}
		if _, exists := seen[request.DecisionID]; exists {
			return ErrInvalid
		}
		seen[request.DecisionID] = struct{}{}
		if i > 0 && (request.UserID != expected[0].UserID || request.SessionID != expected[0].SessionID ||
			effectiveAuthContextType(request.AuthContextType) != effectiveAuthContextType(expected[0].AuthContextType) ||
			request.AuthGeneration != expected[0].AuthGeneration || request.SecurityGeneration != expected[0].SecurityGeneration) {
			return ErrInvalid
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, request := range expected {
			now := s.now()
			if !time.UnixMilli(request.ExpiresAt.UnixMilli()).After(now) {
				return ErrStale
			}
			// Acquire the SQLite writer before creating a read snapshot. A SELECT
			// followed by UPDATE can fail with BUSY_SNAPSHOT when another receipt
			// commits meanwhile, even with busy_timeout. The complete authorization
			// predicate belongs in this CAS; no callback is retried or moved outside
			// the transaction, and a later invalid receipt rolls back the batch.
			want := rowOf(request)
			result := tx.Model(&receiptRow{}).
				Where("decision_id = ? AND subject_id = ? AND user_id = ? AND auth_context_id = ?", want.ID, want.MutationID, want.UserID, want.SessionID).
				Where("request_fingerprint = ? AND auth_generation = ? AND security_generation = ?", want.Fingerprint, want.AuthGeneration, want.SecurityGeneration).
				Where("expires_at = ? AND expires_at > ?", want.ExpiresMS, now.UnixMilli()).
				Where("status = ? AND auth_context_type = ? AND subject_type = ?", Accepted, want.AuthContextType, want.SubjectType).
				Where("allowed_action_ids = ? AND accepted_action_id = ? AND responded_at IS NOT NULL AND consumed_at IS NULL", `["apply","deny"]`, ApplyAction).
				Updates(map[string]any{"status": Consumed, "consumed_at": now.UnixMilli()})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 || !time.UnixMilli(request.ExpiresAt.UnixMilli()).After(s.now()) {
				return ErrStale
			}
			if err := appendEvent(tx, request.DecisionID, Consumed, s.now()); err != nil {
				return err
			}
		}
		if err := apply(tx); err != nil {
			return err
		}
		for _, request := range expected {
			if !time.UnixMilli(request.ExpiresAt.UnixMilli()).After(s.now()) {
				return ErrStale
			}
		}
		return ctx.Err()
	})
}

func isTransactionalDB(db *gorm.DB) bool {
	if db == nil {
		return false
	}
	connPool := db.ConnPool
	if db.Statement != nil && db.Statement.ConnPool != nil {
		connPool = db.Statement.ConnPool
	}
	_, ok := connPool.(gorm.TxCommitter)
	return ok
}
