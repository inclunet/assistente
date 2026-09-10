// Package jobprofilegrant persiste autorizações exatas de delegação de jobs.
package jobprofilegrant

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"assistente/internal/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ToolSubagent      = "subagent"
	FingerprintSchema = 1
)

var (
	ErrJobNotFound               = errors.New("job não encontrado")
	ErrNotSubagentJob            = errors.New("job não usa a tool subagent")
	ErrProfileExpressionRequired = errors.New("input profile do job é obrigatório")
)

// DelegationConfig é o recorte de segurança usado no fingerprint.
type DelegationConfig struct {
	JobID             string
	JobSlug           string
	JobName           string
	Tool              string
	ProfileExpression string
	Fingerprint       string
}

// Grant é o DTO seguro exposto para gestão; não contém configuração do job.
type Grant struct {
	JobID                 string    `json:"jobId"`
	TargetProfileSlug     string    `json:"targetProfileSlug"`
	DelegationFingerprint string    `json:"delegationFingerprint"`
	GrantedAt             time.Time `json:"grantedAt"`
}

type Store struct {
	db  *gorm.DB
	now func() time.Time
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db, now: time.Now}
}

// Fingerprint deriva somente da configuração que altera o alcance da
// delegação. A serialização de uma struct torna a ordem independente de maps.
func Fingerprint(tool, profileExpression string) string {
	payload := struct {
		Schema            int    `json:"schema"`
		Tool              string `json:"tool"`
		ProfileExpression string `json:"profile_expression"`
	}{
		Schema:            FingerprintSchema,
		Tool:              strings.TrimSpace(tool),
		ProfileExpression: strings.TrimSpace(profileExpression),
	}
	data, _ := json.Marshal(payload)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func FingerprintForInputs(tool string, inputs map[string]any) (string, bool) {
	if strings.TrimSpace(tool) != ToolSubagent {
		return "", false
	}
	expression, ok := inputs["profile"].(string)
	expression = strings.TrimSpace(expression)
	if !ok || expression == "" {
		return "", false
	}
	return Fingerprint(tool, expression), true
}

func (s *Store) CurrentDelegation(ctx context.Context, jobID string) (DelegationConfig, error) {
	if s == nil || s.db == nil {
		return DelegationConfig{}, errors.New("store de grants indisponível")
	}
	if _, err := database.RequireUserID(ctx); err != nil {
		return DelegationConfig{}, err
	}
	var row database.Job
	err := database.WithSQLiteBusyRetry(ctx, "job_profile_grants.current", func() error {
		return database.ScopeByUser(ctx, s.db.WithContext(ctx), "user_id").
			Where("id = ? OR slug = ?", strings.TrimSpace(jobID), strings.TrimSpace(jobID)).
			First(&row).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DelegationConfig{}, ErrJobNotFound
	}
	if err != nil {
		return DelegationConfig{}, err
	}
	if strings.TrimSpace(row.ToolName) != ToolSubagent {
		return DelegationConfig{}, ErrNotSubagentJob
	}
	var inputs map[string]any
	if err := json.Unmarshal([]byte(row.Inputs), &inputs); err != nil {
		return DelegationConfig{}, fmt.Errorf("inputs inválidos do job: %w", err)
	}
	expression, ok := inputs["profile"].(string)
	expression = strings.TrimSpace(expression)
	if !ok || expression == "" {
		return DelegationConfig{}, ErrProfileExpressionRequired
	}
	return DelegationConfig{
		JobID:             row.ID,
		JobSlug:           row.Slug,
		JobName:           row.Name,
		Tool:              row.ToolName,
		ProfileExpression: expression,
		Fingerprint:       Fingerprint(row.ToolName, expression),
	}, nil
}

func (s *Store) HasValid(ctx context.Context, jobID, targetSlug, fingerprint string) (bool, error) {
	config, err := s.CurrentDelegation(ctx, jobID)
	if err != nil {
		return false, err
	}
	if config.Fingerprint != strings.TrimSpace(fingerprint) {
		return false, nil
	}
	if err := s.RevokeStale(ctx, config.JobID, config.Fingerprint, "configuração alterada"); err != nil {
		return false, err
	}
	var count int64
	err = database.WithSQLiteBusyRetry(ctx, "job_profile_grants.has_valid", func() error {
		return database.ScopeByUser(ctx, s.db.WithContext(ctx), "user_id").
			Model(&database.JobProfileGrant{}).
			Where("job_id = ? AND target_profile_slug = ? AND delegation_fingerprint = ? AND revoked_at IS NULL",
				config.JobID, strings.TrimSpace(targetSlug), config.Fingerprint).
			Count(&count).Error
	})
	return count > 0, err
}

func (s *Store) ListValid(ctx context.Context, jobID string) ([]Grant, DelegationConfig, error) {
	config, err := s.CurrentDelegation(ctx, jobID)
	if err != nil {
		return nil, DelegationConfig{}, err
	}
	if err := s.RevokeStale(ctx, config.JobID, config.Fingerprint, "configuração alterada"); err != nil {
		return nil, DelegationConfig{}, err
	}
	var rows []database.JobProfileGrant
	err = database.WithSQLiteBusyRetry(ctx, "job_profile_grants.list", func() error {
		return database.ScopeByUser(ctx, s.db.WithContext(ctx), "user_id").
			Where("job_id = ? AND delegation_fingerprint = ? AND revoked_at IS NULL", config.JobID, config.Fingerprint).
			Order("target_profile_slug ASC").Find(&rows).Error
	})
	if err != nil {
		return nil, DelegationConfig{}, err
	}
	grants := make([]Grant, 0, len(rows))
	for _, row := range rows {
		grants = append(grants, Grant{
			JobID:                 row.JobID,
			TargetProfileSlug:     row.TargetProfileSlug,
			DelegationFingerprint: row.DelegationFingerprint,
			GrantedAt:             row.GrantedAt,
		})
	}
	return grants, config, nil
}

func (s *Store) Grant(ctx context.Context, jobID, targetSlug, fingerprint, actor string) error {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	config, err := s.CurrentDelegation(ctx, jobID)
	if err != nil {
		return err
	}
	targetSlug = strings.TrimSpace(targetSlug)
	if targetSlug == "" || config.Fingerprint != strings.TrimSpace(fingerprint) {
		return errors.New("configuração de delegação mudou durante a autorização")
	}
	now := s.now().UTC()
	row := database.JobProfileGrant{
		UserID:                userID,
		JobID:                 config.JobID,
		TargetProfileSlug:     targetSlug,
		DelegationFingerprint: config.Fingerprint,
		GrantedAt:             now,
		GrantedBy:             strings.TrimSpace(actor),
	}
	if row.GrantedBy == "" {
		row.GrantedBy = "desktop"
	}
	return database.WithSQLiteBusyRetry(ctx, "job_profile_grants.grant", func() error {
		return s.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "user_id"}, {Name: "job_id"}, {Name: "target_profile_slug"}, {Name: "delegation_fingerprint"},
			},
			DoUpdates: clause.Assignments(map[string]any{
				"granted_at": now, "granted_by": row.GrantedBy, "revoked_at": nil, "revoked_by": "",
			}),
		}).Create(&row).Error
	})
}

func (s *Store) Revoke(ctx context.Context, jobID, targetSlug, actor string) error {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	config, err := s.CurrentDelegation(ctx, jobID)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	return database.WithSQLiteBusyRetry(ctx, "job_profile_grants.revoke", func() error {
		return s.db.WithContext(ctx).Model(&database.JobProfileGrant{}).
			Where("user_id = ? AND job_id = ? AND target_profile_slug = ? AND revoked_at IS NULL",
				userID, config.JobID, strings.TrimSpace(targetSlug)).
			Updates(map[string]any{"revoked_at": now, "revoked_by": strings.TrimSpace(actor)}).Error
	})
}

func (s *Store) RevokeStale(ctx context.Context, jobID, fingerprint, reason string) error {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	return database.WithSQLiteBusyRetry(ctx, "job_profile_grants.revoke_stale", func() error {
		return s.db.WithContext(ctx).Model(&database.JobProfileGrant{}).
			Where("user_id = ? AND job_id = ? AND delegation_fingerprint <> ? AND revoked_at IS NULL",
				userID, strings.TrimSpace(jobID), strings.TrimSpace(fingerprint)).
			Updates(map[string]any{"revoked_at": now, "revoked_by": strings.TrimSpace(reason)}).Error
	})
}

func (s *Store) RevokeJobTx(tx *gorm.DB, userID, jobID, actor string) error {
	if tx == nil {
		return errors.New("transação obrigatória")
	}
	now := s.now().UTC()
	return tx.Model(&database.JobProfileGrant{}).
		Where("user_id = ? AND job_id = ? AND revoked_at IS NULL", userID, jobID).
		Updates(map[string]any{"revoked_at": now, "revoked_by": strings.TrimSpace(actor)}).Error
}

func (s *Store) RevokeProfile(ctx context.Context, targetSlug, actor string) error {
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	now := s.now().UTC()
	return database.WithSQLiteBusyRetry(ctx, "job_profile_grants.revoke_profile", func() error {
		return s.db.WithContext(ctx).Model(&database.JobProfileGrant{}).
			Where("target_profile_slug = ? AND revoked_at IS NULL", strings.TrimSpace(targetSlug)).
			Updates(map[string]any{"revoked_at": now, "revoked_by": strings.TrimSpace(actor)}).Error
	})
}
