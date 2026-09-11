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
	"sync"
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
	ErrAuthorizationNotGranted   = errors.New("authorization_not_granted")
	ErrGrantGenerationChanged    = errors.New("grant_generation_changed")
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

type AuthorizationSnapshot struct {
	Config     DelegationConfig
	Generation uint64
}

type DisabledJob struct {
	UserID string
	JobID  string
	Slug   string
}

type Store struct {
	db                 *gorm.DB
	now                func() time.Time
	callbackMu         sync.RWMutex
	onJobsDisabledFunc func([]DisabledJob)
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db, now: time.Now}
}

func (s *Store) SetJobsDisabledCallback(callback func([]DisabledJob)) {
	s.callbackMu.Lock()
	s.onJobsDisabledFunc = callback
	s.callbackMu.Unlock()
}

func (s *Store) notifyJobsDisabled(jobs []DisabledJob) {
	if len(jobs) == 0 {
		return
	}
	s.callbackMu.RLock()
	callback := s.onJobsDisabledFunc
	s.callbackMu.RUnlock()
	if callback != nil {
		callback(jobs)
	}
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
	var config DelegationConfig
	err := database.WithSQLiteBusyRetry(ctx, "job_profile_grants.current", func() error {
		var readErr error
		config, readErr = currentDelegationDB(ctx, s.db.WithContext(ctx), jobID)
		return readErr
	})
	return config, err
}

func currentDelegationDB(ctx context.Context, db *gorm.DB, jobID string) (DelegationConfig, error) {
	var row database.Job
	err := database.ScopeByUser(ctx, db, "user_id").
		Where("slug = ?", strings.TrimSpace(jobID)).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = database.ScopeByUser(ctx, db, "user_id").
			Where("id = ?", strings.TrimSpace(jobID)).
			First(&row).Error
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DelegationConfig{}, ErrJobNotFound
	}
	if err != nil {
		return DelegationConfig{}, err
	}
	return delegationConfigFromRow(db, row)
}

func currentDelegationByDatabaseIDDB(ctx context.Context, db *gorm.DB, jobID string) (DelegationConfig, error) {
	var row database.Job
	err := database.ScopeByUser(ctx, db, "user_id").
		Where("id = ?", strings.TrimSpace(jobID)).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DelegationConfig{}, ErrJobNotFound
	}
	if err != nil {
		return DelegationConfig{}, err
	}
	return delegationConfigFromRow(db, row)
}

func delegationConfigFromRow(db *gorm.DB, row database.Job) (DelegationConfig, error) {
	toolName, err := effectiveToolNameDB(db, row)
	if err != nil {
		return DelegationConfig{}, err
	}
	if toolName != ToolSubagent {
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
		Tool:              toolName,
		ProfileExpression: expression,
		Fingerprint:       Fingerprint(toolName, expression),
	}, nil
}

func effectiveToolNameDB(db *gorm.DB, row database.Job) (string, error) {
	toolName := strings.TrimSpace(row.ToolName)
	if toolName != "" {
		return toolName, nil
	}
	var catalog database.ToolCatalog
	if err := db.Where("id = ?", row.ToolCatalogID).First(&catalog).Error; err != nil {
		return "", err
	}
	return strings.TrimSpace(catalog.Name), nil
}

func (s *Store) AuthorizationSnapshot(ctx context.Context, jobID, targetSlug string) (AuthorizationSnapshot, error) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return AuthorizationSnapshot{}, err
	}
	targetSlug = strings.TrimSpace(targetSlug)
	var snapshot AuthorizationSnapshot
	err = database.WithSQLiteBusyRetry(ctx, "job_profile_grants.snapshot", func() error {
		return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			config, currentErr := currentDelegationDB(ctx, tx, jobID)
			if currentErr != nil {
				return currentErr
			}
			epoch, epochErr := ensureEpochTx(tx, userID, config.JobID, targetSlug, config.Fingerprint)
			if epochErr != nil {
				return epochErr
			}
			snapshot = AuthorizationSnapshot{Config: config, Generation: epoch.Generation}
			return nil
		})
	})
	return snapshot, err
}

func ensureEpochTx(tx *gorm.DB, userID, jobID, targetSlug, fingerprint string) (database.JobProfileGrantEpoch, error) {
	var epoch database.JobProfileGrantEpoch
	naturalKey := func() *gorm.DB {
		return tx.Where(
			"user_id = ? AND job_id = ? AND target_profile_slug = ? AND delegation_fingerprint = ?",
			userID, jobID, targetSlug, fingerprint,
		)
	}
	err := naturalKey().First(&epoch).Error
	if err == nil {
		return epoch, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return epoch, err
	}
	epoch = database.JobProfileGrantEpoch{
		UserID: userID, JobID: jobID, TargetProfileSlug: targetSlug,
		DelegationFingerprint: fingerprint,
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&epoch).Error; err != nil {
		return epoch, err
	}
	epoch = database.JobProfileGrantEpoch{}
	if err := naturalKey().First(&epoch).Error; err != nil {
		return epoch, err
	}
	return epoch, nil
}

func (s *Store) HasValid(ctx context.Context, jobID, targetSlug, fingerprint string) (bool, error) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return false, err
	}
	valid := false
	err = database.WithSQLiteBusyRetry(ctx, "job_profile_grants.has_valid", func() error {
		return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			config, currentErr := currentDelegationByDatabaseIDDB(ctx, tx, jobID)
			if currentErr != nil {
				return currentErr
			}
			if config.Fingerprint != strings.TrimSpace(fingerprint) {
				return nil
			}
			if revokeErr := revokeStaleTx(tx, userID, config.JobID, config.Fingerprint, s.now().UTC(), "configuração alterada"); revokeErr != nil {
				return revokeErr
			}
			var count int64
			epochGeneration := tx.Model(&database.JobProfileGrantEpoch{}).
				Select("generation").
				Where("user_id = ? AND job_id = ? AND target_profile_slug = ? AND delegation_fingerprint = ?",
					userID, config.JobID, strings.TrimSpace(targetSlug), config.Fingerprint)
			if countErr := tx.Model(&database.JobProfileGrant{}).
				Where("user_id = ? AND job_id = ? AND target_profile_slug = ? AND delegation_fingerprint = ? AND revoked_at IS NULL",
					userID, config.JobID, strings.TrimSpace(targetSlug), config.Fingerprint).
				Where("NOT EXISTS (SELECT 1 FROM profile_grant_revocation_intents WHERE profile_grant_revocation_intents.target_profile_slug = job_profile_grants.target_profile_slug)").
				Where("generation = (?)", epochGeneration).
				Count(&count).Error; countErr != nil {
				return countErr
			}
			valid = count > 0
			return nil
		})
	})
	return valid, err
}

func (s *Store) ListValid(ctx context.Context, jobID string) ([]Grant, DelegationConfig, error) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, DelegationConfig{}, err
	}
	var config DelegationConfig
	var rows []database.JobProfileGrant
	err = database.WithSQLiteBusyRetry(ctx, "job_profile_grants.list", func() error {
		return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var currentErr error
			config, currentErr = currentDelegationDB(ctx, tx, jobID)
			if currentErr != nil {
				return currentErr
			}
			if revokeErr := revokeStaleTx(tx, userID, config.JobID, config.Fingerprint, s.now().UTC(), "configuração alterada"); revokeErr != nil {
				return revokeErr
			}
			return tx.Where("user_id = ? AND job_id = ? AND delegation_fingerprint = ? AND revoked_at IS NULL",
				userID, config.JobID, config.Fingerprint).
				Where("NOT EXISTS (SELECT 1 FROM profile_grant_revocation_intents WHERE profile_grant_revocation_intents.target_profile_slug = job_profile_grants.target_profile_slug)").
				Where("generation = (?)", tx.Model(&database.JobProfileGrantEpoch{}).
					Select("generation").
					Where("job_profile_grant_epochs.user_id = job_profile_grants.user_id AND job_profile_grant_epochs.job_id = job_profile_grants.job_id AND job_profile_grant_epochs.target_profile_slug = job_profile_grants.target_profile_slug AND job_profile_grant_epochs.delegation_fingerprint = job_profile_grants.delegation_fingerprint")).
				Order("target_profile_slug ASC").Find(&rows).Error
		})
	})
	if err != nil {
		return nil, DelegationConfig{}, err
	}
	grants := make([]Grant, 0, len(rows))
	for _, row := range rows {
		grants = append(grants, Grant{
			JobID:                 config.JobSlug,
			TargetProfileSlug:     row.TargetProfileSlug,
			DelegationFingerprint: row.DelegationFingerprint,
			GrantedAt:             row.GrantedAt,
		})
	}
	return grants, config, nil
}

func (s *Store) Grant(ctx context.Context, jobID, targetSlug, fingerprint, actor string, expectedGeneration uint64) error {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	targetSlug = strings.TrimSpace(targetSlug)
	if targetSlug == "" {
		return errors.New("configuração de delegação mudou durante a autorização")
	}
	return database.WithSQLiteBusyRetry(ctx, "job_profile_grants.grant", func() error {
		return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			config, currentErr := currentDelegationByDatabaseIDDB(ctx, tx, jobID)
			if currentErr != nil {
				return currentErr
			}
			if config.Fingerprint != strings.TrimSpace(fingerprint) {
				return errors.New("configuração de delegação mudou durante a autorização")
			}
			epoch, epochErr := ensureEpochTx(tx, userID, config.JobID, targetSlug, config.Fingerprint)
			if epochErr != nil {
				return epochErr
			}
			if epoch.Generation != expectedGeneration {
				return ErrGrantGenerationChanged
			}
			now := s.now().UTC()
			row := database.JobProfileGrant{
				UserID:                userID,
				JobID:                 config.JobID,
				TargetProfileSlug:     targetSlug,
				DelegationFingerprint: config.Fingerprint,
				Generation:            epoch.Generation,
				GrantedAt:             now,
				GrantedBy:             strings.TrimSpace(actor),
			}
			if row.GrantedBy == "" {
				row.GrantedBy = "desktop"
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
			var activeCount int64
			if err := tx.Model(&database.JobProfileGrant{}).
				Where("user_id = ? AND job_id = ? AND target_profile_slug = ? AND delegation_fingerprint = ? AND generation = ? AND revoked_at IS NULL",
					userID, config.JobID, targetSlug, config.Fingerprint, epoch.Generation).
				Count(&activeCount).Error; err != nil {
				return err
			}
			if activeCount != 1 {
				return ErrGrantGenerationChanged
			}
			return nil
		})
	})
}

func (s *Store) Revoke(ctx context.Context, jobID, targetSlug, actor string) error {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	var disabled []DisabledJob
	err = database.WithSQLiteBusyRetry(ctx, "job_profile_grants.revoke", func() error {
		return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			config, currentErr := currentDelegationDB(ctx, tx, jobID)
			if currentErr != nil {
				return currentErr
			}
			epoch, epochErr := ensureEpochTx(tx, userID, config.JobID, strings.TrimSpace(targetSlug), config.Fingerprint)
			if epochErr != nil {
				return epochErr
			}
			if updateErr := tx.Model(&database.JobProfileGrantEpoch{}).
				Where("id = ?", epoch.ID).
				UpdateColumn("generation", gorm.Expr("generation + 1")).Error; updateErr != nil {
				return updateErr
			}
			if updateErr := tx.Model(&database.JobProfileGrant{}).
				Where("user_id = ? AND job_id = ? AND target_profile_slug = ? AND revoked_at IS NULL",
					userID, config.JobID, strings.TrimSpace(targetSlug)).
				Updates(map[string]any{"revoked_at": s.now().UTC(), "revoked_by": strings.TrimSpace(actor)}).Error; updateErr != nil {
				return updateErr
			}
			disabledJob, disableErr := disableJobWithoutGrantTx(tx, userID, config.JobID)
			if disabledJob != nil {
				disabled = append(disabled, *disabledJob)
			}
			return disableErr
		})
	})
	if err == nil {
		s.notifyJobsDisabled(disabled)
	}
	return err
}

func (s *Store) RevokeStale(ctx context.Context, jobID, fingerprint, reason string) error {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	return database.WithSQLiteBusyRetry(ctx, "job_profile_grants.revoke_stale", func() error {
		return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			config, currentErr := currentDelegationDB(ctx, tx, jobID)
			if currentErr != nil {
				return currentErr
			}
			if config.Fingerprint != strings.TrimSpace(fingerprint) {
				return nil
			}
			return revokeStaleTx(tx, userID, config.JobID, config.Fingerprint, s.now().UTC(), reason)
		})
	})
}

func revokeStaleTx(tx *gorm.DB, userID, jobID, fingerprint string, now time.Time, reason string) error {
	if err := tx.Model(&database.JobProfileGrantEpoch{}).
		Where("user_id = ? AND job_id = ? AND delegation_fingerprint <> ?", userID, strings.TrimSpace(jobID), strings.TrimSpace(fingerprint)).
		UpdateColumn("generation", gorm.Expr("generation + 1")).Error; err != nil {
		return err
	}
	return tx.Model(&database.JobProfileGrant{}).
		Where("user_id = ? AND job_id = ? AND delegation_fingerprint <> ? AND revoked_at IS NULL",
			userID, strings.TrimSpace(jobID), strings.TrimSpace(fingerprint)).
		Updates(map[string]any{"revoked_at": now, "revoked_by": strings.TrimSpace(reason)}).Error
}

func (s *Store) RevokeJobTx(tx *gorm.DB, userID, jobID, actor string) error {
	if tx == nil {
		return errors.New("transação obrigatória")
	}
	now := s.now().UTC()
	if err := tx.Model(&database.JobProfileGrantEpoch{}).
		Where("user_id = ? AND job_id = ?", userID, jobID).
		UpdateColumn("generation", gorm.Expr("generation + 1")).Error; err != nil {
		return err
	}
	return tx.Model(&database.JobProfileGrant{}).
		Where("user_id = ? AND job_id = ? AND revoked_at IS NULL", userID, jobID).
		Updates(map[string]any{"revoked_at": now, "revoked_by": strings.TrimSpace(actor)}).Error
}

func (s *Store) BeginProfileRevocation(ctx context.Context, targetSlug, originalIdentity, actor string) error {
	row := database.ProfileGrantRevocationIntent{
		TargetProfileSlug: strings.TrimSpace(targetSlug),
		OriginalIdentity:  strings.TrimSpace(originalIdentity),
		RequestedBy:       strings.TrimSpace(actor),
	}
	return database.WithSQLiteBusyRetry(ctx, "job_profile_grants.begin_profile_revocation", func() error {
		return s.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "target_profile_slug"}},
			DoUpdates: clause.Assignments(map[string]any{
				"original_identity": row.OriginalIdentity,
				"requested_by":      row.RequestedBy,
			}),
		}).Create(&row).Error
	})
}

func (s *Store) CancelProfileRevocation(ctx context.Context, targetSlug string) error {
	return database.WithSQLiteBusyRetry(ctx, "job_profile_grants.cancel_profile_revocation", func() error {
		return s.db.WithContext(ctx).
			Where("target_profile_slug = ?", strings.TrimSpace(targetSlug)).
			Delete(&database.ProfileGrantRevocationIntent{}).Error
	})
}

func (s *Store) ReconcileProfileRevocations(ctx context.Context, currentIdentity func(string) string) error {
	var intents []database.ProfileGrantRevocationIntent
	if err := database.WithSQLiteBusyRetry(ctx, "job_profile_grants.list_profile_revocations", func() error {
		return s.db.WithContext(ctx).Order("created_at ASC").Find(&intents).Error
	}); err != nil {
		return err
	}
	for _, intent := range intents {
		if currentIdentity != nil && currentIdentity(intent.TargetProfileSlug) == intent.OriginalIdentity {
			if err := s.CancelProfileRevocation(ctx, intent.TargetProfileSlug); err != nil {
				return err
			}
			continue
		}
		if err := s.RevokeProfileGlobal(ctx, intent.TargetProfileSlug, intent.RequestedBy); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) RevokeProfileGlobal(ctx context.Context, targetSlug, actor string) error {
	now := s.now().UTC()
	var disabled []DisabledJob
	err := database.WithSQLiteBusyRetry(ctx, "job_profile_grants.revoke_profile", func() error {
		return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&database.JobProfileGrantEpoch{}).
				Where("target_profile_slug = ?", strings.TrimSpace(targetSlug)).
				UpdateColumn("generation", gorm.Expr("generation + 1")).Error; err != nil {
				return err
			}
			var affected []database.JobProfileGrant
			if err := tx.Where("target_profile_slug = ? AND revoked_at IS NULL", strings.TrimSpace(targetSlug)).
				Find(&affected).Error; err != nil {
				return err
			}
			if err := tx.Model(&database.JobProfileGrant{}).
				Where("target_profile_slug = ? AND revoked_at IS NULL", strings.TrimSpace(targetSlug)).
				Updates(map[string]any{"revoked_at": now, "revoked_by": strings.TrimSpace(actor)}).Error; err != nil {
				return err
			}
			seen := make(map[string]struct{}, len(affected))
			for _, grant := range affected {
				key := grant.UserID + "\x00" + grant.JobID
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				disabledJob, disableErr := disableJobWithoutGrantTx(tx, grant.UserID, grant.JobID)
				if disableErr != nil {
					return disableErr
				}
				if disabledJob != nil {
					disabled = append(disabled, *disabledJob)
				}
			}
			return tx.Where("target_profile_slug = ?", strings.TrimSpace(targetSlug)).
				Delete(&database.ProfileGrantRevocationIntent{}).Error
		})
	})
	if err == nil {
		s.notifyJobsDisabled(disabled)
	}
	return err
}

func disableJobWithoutGrantTx(tx *gorm.DB, userID, jobID string) (*DisabledJob, error) {
	var job database.Job
	if err := tx.Where("user_id = ? AND id = ?", userID, jobID).First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var inputs map[string]any
	if err := json.Unmarshal([]byte(job.Inputs), &inputs); err != nil {
		return nil, err
	}
	toolName, err := effectiveToolNameDB(tx, job)
	if err != nil {
		return nil, err
	}
	fingerprint, grantable := FingerprintForInputs(toolName, inputs)
	if !grantable {
		return nil, nil
	}
	expression, _ := inputs["profile"].(string)
	query := tx.Model(&database.JobProfileGrant{}).
		Where("user_id = ? AND job_id = ? AND delegation_fingerprint = ? AND revoked_at IS NULL",
			userID, jobID, fingerprint).
		Where("NOT EXISTS (SELECT 1 FROM profile_grant_revocation_intents WHERE profile_grant_revocation_intents.target_profile_slug = job_profile_grants.target_profile_slug)").
		Where("generation = (?)", tx.Model(&database.JobProfileGrantEpoch{}).
			Select("generation").
			Where("job_profile_grant_epochs.user_id = job_profile_grants.user_id AND job_profile_grant_epochs.job_id = job_profile_grants.job_id AND job_profile_grant_epochs.target_profile_slug = job_profile_grants.target_profile_slug AND job_profile_grant_epochs.delegation_fingerprint = job_profile_grants.delegation_fingerprint"))
	if !strings.Contains(expression, "{{") {
		query = query.Where("target_profile_slug = ?", strings.TrimSpace(expression))
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		if job.Enabled {
			if err := tx.Model(&database.Job{}).Where("user_id = ? AND id = ?", userID, jobID).Update("enabled", false).Error; err != nil {
				return nil, err
			}
		}
		return &DisabledJob{UserID: userID, JobID: jobID, Slug: job.Slug}, nil
	}
	return nil, nil
}
