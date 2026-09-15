package commandconfig

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

var ErrMutationNotFound = errors.New("mutação não encontrada no escopo")

// BindingMutation é a auditoria da alteração de configuração global, não uma
// CommandInvocation. Registra somente IDs, fingerprint e a mudança de enabled;
// nunca documentos, texto do diálogo, tokens ou material de chave.
type BindingMutation struct {
	MutationID         string `gorm:"column:mutation_id;primaryKey"`
	SchemaVersion      int
	UserID             string
	SessionID          string
	Scope              string
	Operation          string
	BindingID          string
	DecisionID         string
	RequestFingerprint string
	AuthGeneration     string
	SecurityGeneration string
	GenerationID       string
	BeforeGeneration   int64
	AfterGeneration    int64
	BeforeEnabled      bool
	AfterEnabled       bool
	OccurredAt         time.Time
}

func (BindingMutation) TableName() string { return "command_config_mutations" }

func mutationAuditSchemaV1() string {
	return fmt.Sprintf(`CREATE TABLE command_config_mutations (
		mutation_id TEXT NOT NULL PRIMARY KEY CHECK %s,
		schema_version INTEGER NOT NULL CHECK (schema_version = 1),
		user_id TEXT NOT NULL CHECK %s,
		session_id TEXT NOT NULL CHECK %s,
		scope TEXT NOT NULL CHECK (scope = 'global'),
		operation TEXT NOT NULL CHECK (operation = 'binding_enabled'),
		binding_id TEXT NOT NULL CHECK %s,
		decision_id TEXT NOT NULL UNIQUE CHECK %s,
		request_fingerprint TEXT NOT NULL CHECK (length(request_fingerprint) BETWEEN 1 AND 256 AND trim(request_fingerprint) = request_fingerprint),
		auth_generation TEXT NOT NULL CHECK (length(auth_generation) BETWEEN 1 AND 256 AND trim(auth_generation) = auth_generation),
		security_generation TEXT NOT NULL CHECK (length(security_generation) BETWEEN 1 AND 256 AND trim(security_generation) = security_generation),
		generation_id TEXT NOT NULL CHECK %s,
		before_generation INTEGER NOT NULL CHECK (typeof(before_generation) = 'integer' AND before_generation >= 1 AND before_generation < 9223372036854775807),
		after_generation INTEGER NOT NULL CHECK (typeof(after_generation) = 'integer' AND after_generation = before_generation + 1),
		before_enabled BOOLEAN NOT NULL CHECK (before_enabled IN (0, 1)),
		after_enabled BOOLEAN NOT NULL CHECK (after_enabled IN (0, 1) AND after_enabled <> before_enabled),
		occurred_at DATETIME NOT NULL
	)`, uuid7Check("mutation_id"), uuid7Check("user_id"), uuid7Check("session_id"), uuid7Check("binding_id"), uuid7Check("decision_id"), uuid7Check("generation_id"))
}

// LegacyMutationAuditSchema expõe apenas o DDL v1 conhecido para o upgrade
// central comparar antes de reconstruir a tabela, nunca para aceitar drift.
func LegacyMutationAuditSchema() string { return mutationAuditSchemaV1() }

func mutationAuditSchema() string {
	s := mutationAuditSchemaV1()
	s = strings.Replace(s, "CHECK (schema_version = 1)", "CHECK (schema_version IN (1,2))", 1)
	s = strings.Replace(s, "CHECK (scope = 'global')", "CHECK (scope IN ('global','workspace'))", 1)
	s = strings.Replace(s, "CHECK (operation = 'binding_enabled')", "CHECK (operation IN ('binding_enabled','layer_create','layer_update','layer_delete','layer_enable','layer_disable','layer_restore','binding_create','binding_update','binding_delete','binding_enable','binding_disable','binding_restore','config_restore','rule_create','rule_update','rule_delete','rule_enable','rule_disable','rule_restore','default_upgrade','default_rebase'))", 1)
	s = strings.Replace(s, "binding_id TEXT NOT NULL", "binding_id TEXT", 1)
	s = strings.Replace(s, "before_enabled BOOLEAN NOT NULL", "before_enabled BOOLEAN", 1)
	s = strings.Replace(s, "after_enabled BOOLEAN NOT NULL", "after_enabled BOOLEAN", 1)
	s = strings.Replace(s, "occurred_at DATETIME NOT NULL", fmt.Sprintf(`workspace_id TEXT CHECK (workspace_id IS NULL OR %s),
		before_document TEXT CHECK (before_document IS NULL OR json_valid(before_document)),
		after_document TEXT CHECK (after_document IS NULL OR json_valid(after_document)),
		occurred_at DATETIME NOT NULL,
		CHECK ((scope = 'global' AND workspace_id IS NULL) OR (scope = 'workspace' AND workspace_id IS NOT NULL)),
		CHECK ((schema_version = 1 AND operation = 'binding_enabled' AND scope = 'global' AND binding_id IS NOT NULL AND before_enabled IS NOT NULL AND after_enabled IS NOT NULL AND before_document IS NULL AND after_document IS NULL)
		OR (schema_version = 2 AND operation <> 'binding_enabled' AND before_document IS NOT NULL AND after_document IS NOT NULL AND before_enabled IS NULL AND after_enabled IS NULL))`, workspaceCheck("workspace_id")), 1)
	return s
}

func migrateMutationAudit(tx *gorm.DB) error {
	var existing struct {
		Type string
		SQL  string
	}
	if err := tx.Raw("SELECT type, sql FROM sqlite_master WHERE name = ?", "command_config_mutations").Scan(&existing).Error; err != nil {
		return err
	}
	statement := mutationAuditSchema()
	if existing.Type != "" {
		// Não aceitar uma tabela pré-existente sem as constraints deste contrato.
		if existing.Type != "table" || normalizeSQL(existing.SQL) != normalizeSQL(statement) {
			return ErrInvalid
		}
		return nil
	}
	return tx.Exec(statement).Error
}

func recordBindingMutation(tx *gorm.DB, confirmed *ConfirmedBindingEnabledChange) error {
	change, request := confirmed.change, confirmed.request
	generation := change.baseline.generations[0]
	row := BindingMutation{MutationID: change.mutationID, SchemaVersion: 1,
		UserID: request.UserID, SessionID: request.SessionID, Scope: "global", Operation: "binding_enabled",
		BindingID: change.before.ID, DecisionID: request.DecisionID, RequestFingerprint: request.Fingerprint,
		AuthGeneration: request.AuthGeneration, SecurityGeneration: request.SecurityGeneration,
		GenerationID: generation.ID, BeforeGeneration: generation.Generation, AfterGeneration: generation.Generation + 1,
		BeforeEnabled: change.before.Enabled, AfterEnabled: change.after.Enabled, OccurredAt: time.Now().UTC()}
	return tx.Create(&row).Error
}

// GetBindingMutation exige owner/sessão derivados pelo host. O repository não
// autentica nem oferece listagem global; ID estrangeiro equivale a inexistente.
func (s *Store) GetBindingMutation(ctx context.Context, userID, sessionID, mutationID string) (BindingMutation, error) {
	if s == nil || s.db == nil || ctx == nil || !validID(userID) || !validID(sessionID) || !validID(mutationID) {
		return BindingMutation{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return BindingMutation{}, err
	}
	var row BindingMutation
	err := s.db.WithContext(ctx).Where("mutation_id = ? AND user_id = ? AND session_id = ?", mutationID, userID, sessionID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return BindingMutation{}, ErrMutationNotFound
	}
	if err != nil {
		return BindingMutation{}, err
	}
	return row, nil
}
