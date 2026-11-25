package plugins

import (
	"context"
	"errors"
	"strings"
	"time"

	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/jobs"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrJobSyncPending = errors.New("PLUGIN_JOB_SYNC_PENDING")

func (i *Inventory) WithJobRefresh(refresh func(context.Context, string, commandsecurity.Admission) error) *Inventory {
	copy := *i
	copy.refreshJob = refresh
	return &copy
}

func readPackageJob(key, entry string, data []byte) (*jobs.Job, error) {
	if strings.HasSuffix(entry, ".yaml") {
		return jobs.ParsePackageDefinition(data)
	}
	return jobs.ReadPortableJob(key, data)
}

func (i *Inventory) PrepareJob(ctx context.Context, id, digest, key string, admit commandsecurity.Admission) (string, error) {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return "", err
	}
	if admit == nil {
		return "", database.ErrUserScopeRequired
	}
	if i.refreshJob == nil {
		return "", ErrComponentUnavailable
	}
	snap, err := i.installedSnapshot(ctx, id, digest)
	if err != nil {
		return "", err
	}
	var definition *jobs.Job
	for _, c := range DiscoverComponents(snap) {
		if c.Key == key && c.Kind == "job" && c.Reason == "" {
			definition, err = readPackageJob(strings.TrimPrefix(key, "jobs/"), c.Entry, snap.Files[c.Entry])
			if err != nil {
				return "", err
			}
			break
		}
	}
	if definition == nil {
		return "", ErrComponentUnavailable
	}
	slug := jobs.NormalizeSlug("plugin-" + strings.TrimPrefix(key, "jobs/") + "-" + uuid.NewString())
	publish, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err = database.WithSQLiteAdmittedCommit(publish, i.db, "plugins.job.prepare", func(tx *gorm.DB) error {
		var installed database.PluginInstallation
		if err := tx.Where("id=? AND user_id=? AND digest=? AND disabled=?", id, owner, digest, false).Take(&installed).Error; err != nil {
			return err
		}
		var current database.Job
		err := tx.Select("slug").Where("user_id=? AND plugin_installation_id=? AND plugin_digest=? AND plugin_component_key=?", owner, id, digest, key).Take(&current).Error
		if err == nil {
			slug = current.Slug
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		job := *definition
		job.ID = slug
		job.Enabled = false
		job.Metadata = jobs.Metadata{CreatedBy: "plugin"}
		return jobs.CreateDisabledPluginJobTx(ctx, tx, &job, id, digest, key)
	}, admit)
	if err != nil {
		return "", err
	}
	if err := i.syncPendingJob(ctx, slug, admit); err != nil {
		return slug, errors.Join(ErrJobSyncPending, err)
	}
	return slug, nil
}

// CopyJob preserves the owner's saved changes even after the managed revision is revoked.
func (i *Inventory) CopyJob(ctx context.Context, installationID, slug, request, name string, admit commandsecurity.Admission) (string, error) {
	if admit == nil || i.refreshJob == nil {
		return "", ErrComponentUnavailable
	}
	var copied string
	publish, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := database.WithSQLiteAdmittedCommit(publish, i.db, "plugins.job.copy", func(tx *gorm.DB) error {
		var err error
		copied, err = jobs.CopyManagedJobTx(ctx, tx, installationID, slug, request, name)
		return err
	}, admit)
	if err != nil {
		return "", err
	}
	if err := i.syncPendingJob(ctx, copied, admit); err != nil {
		return copied, errors.Join(ErrJobSyncPending, err)
	}
	return copied, nil
}

// Revoke the exact installed generation while preserving the native job and its history.
// Re-enabling an installation never revives the previous grant.
func revokeJobs(tx *gorm.DB, owner, id string) ([]string, error) {
	var slugs []string
	query := tx.Model(&database.Job{}).Where("user_id=? AND plugin_installation_id=? AND plugin_digest<>''", owner, id)
	if err := query.Session(&gorm.Session{}).Pluck("slug", &slugs).Error; err != nil {
		return nil, err
	}
	if len(slugs) > 0 {
		if err := query.Updates(map[string]any{"enabled": false, "plugin_digest": "", "plugin_refresh_pending": true}).Error; err != nil {
			return nil, err
		}
	}
	return slugs, nil
}

// SyncPendingJobs retries committed changes, including jobs whose installation was removed.
func (i *Inventory) SyncPendingJobs(ctx context.Context, admit commandsecurity.Admission) error {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	var slugs []string
	if err := i.db.WithContext(ctx).Model(&database.Job{}).Where("user_id=? AND plugin_refresh_pending=?", owner, true).Pluck("slug", &slugs).Error; err != nil {
		return err
	}
	var failures []error
	for _, slug := range slugs {
		if err := i.syncPendingJob(ctx, slug, admit); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (i *Inventory) syncPendingJob(ctx context.Context, slug string, admit commandsecurity.Admission) error {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	var job database.Job
	err = i.db.WithContext(ctx).Select("id", "plugin_digest").Where("user_id=? AND slug=? AND plugin_refresh_pending=?", owner, slug, true).Take(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if i.refreshJob == nil {
		return ErrJobSyncPending
	}
	if err := i.refreshJob(ctx, slug, admit); err != nil {
		return errors.Join(ErrJobSyncPending, err)
	}
	return i.db.WithContext(ctx).Model(&database.Job{}).Where("id=? AND user_id=? AND plugin_digest=?", job.ID, owner, job.PluginDigest).UpdateColumn("plugin_refresh_pending", false).Error
}

func (i *Inventory) refreshRuntime(ctx context.Context, slugs []string, syncMCP bool, admit commandsecurity.Admission) error {
	var failures []error
	for _, slug := range slugs {
		if i.refreshJob == nil {
			failures = append(failures, ErrJobSyncPending)
			break
		}
		if err := i.syncPendingJob(ctx, slug, admit); err != nil {
			failures = append(failures, errors.Join(ErrJobSyncPending, err))
		}
	}
	if syncMCP {
		if err := i.SyncMCP(ctx, admit); err != nil {
			failures = append(failures, errors.Join(ErrMCPSyncPending, err))
		}
	}
	return errors.Join(failures...)
}
