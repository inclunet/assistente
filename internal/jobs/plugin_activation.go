package jobs

import (
	"context"
	"errors"

	"assistente/internal/database"
	"assistente/internal/tools"
	"gorm.io/gorm"
)

var ErrPluginJobInactive = errors.New("PLUGIN_JOB_INACTIVE")

func checkPluginJobRevision(ctx context.Context, tx *gorm.DB, owner string, row *database.Job) error {
	if row.PluginDigest == "" {
		return ErrPluginJobInactive
	}
	var count int64
	if err := tx.WithContext(ctx).Model(&database.PluginInstallation{}).Where("user_id=? AND id=? AND digest=? AND disabled=?", owner, row.PluginInstallationID, row.PluginDigest, false).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return ErrPluginJobInactive
	}
	return nil
}

// CheckPluginJobActivation runs before input expansion and again at dispatch.
// A registry entry captured before revocation cannot authorize another attempt.
func (r *DBRepository) CheckPluginJobActivation(ctx context.Context, job *Job) error {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	var count int64
	err = r.db.WithContext(ctx).Model(&database.Job{}).Joins("JOIN plugin_installations p ON jobs.plugin_installation_id=p.id AND jobs.plugin_digest=p.digest AND jobs.user_id=p.user_id").
		Where("jobs.user_id=? AND jobs.id=? AND jobs.enabled=? AND jobs.plugin_installation_id=? AND jobs.plugin_digest=? AND p.disabled=?", owner, job.DatabaseID, true, job.PluginInstallationID, job.PluginDigest, false).Count(&count).Error
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrPluginJobInactive
	}
	return nil
}

func (e *JobExecutor) checkPluginJob(ctx context.Context, job *Job) error {
	if job.PluginInstallationID == "" {
		return nil
	}
	store, ok := e.repository.(interface {
		CheckPluginJobActivation(context.Context, *Job) error
	})
	if !ok {
		return ErrPluginJobInactive
	}
	return store.CheckPluginJobActivation(ctx, job)
}

func (e *JobExecutor) pluginJobContext(ctx context.Context, job *Job) (context.Context, error) {
	if job.PluginInstallationID == "" {
		return ctx, nil
	}
	if err := e.checkPluginJob(ctx, job); err != nil {
		return ctx, err
	}
	ec, _ := tools.GetExecutionContext(ctx)
	parent := ec.CheckCommandActivation
	ec.CheckCommandActivation = func(current context.Context) error {
		if parent != nil {
			if err := parent(current); err != nil {
				return err
			}
		}
		return e.checkPluginJob(current, job)
	}
	return tools.WithExecutionContext(ctx, ec), nil
}

// RefreshPluginJob reloads committed state through the native registry and scheduler.
func (m *Manager) RefreshPluginJob(ctx context.Context, slug string) error {
	m.runtimeMu.Lock()
	defer m.runtimeMu.Unlock()
	ctx, err := m.scopedContext(ctx)
	if err != nil {
		return err
	}
	job, err := m.cfg.Repository.GetJob(ctx, slug)
	if err != nil {
		return err
	}
	if previous := m.registry.Get(slug); previous != nil {
		m.unregisterTriggers(previous)
	}
	m.registerJob(job)
	m.emitEvent("jobs:updated", map[string]any{"id": job.ID, "name": job.Name})
	return nil
}
