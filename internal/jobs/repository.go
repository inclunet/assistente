package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"assistente/internal/database"

	"gorm.io/gorm"
)

const (
	tagResourceJob = "job"
)

// Repository abstrai a persistência DB-only do runtime de jobs.
type Repository interface {
	ListTags(ctx context.Context) ([]Tag, error)
	UpsertTag(ctx context.Context, tag *Tag) error
	SetResourceTags(ctx context.Context, resourceType, resourceID string, tagSlugs []string) error
	GetResourceTags(ctx context.Context, resourceType, resourceID string) ([]Tag, error)

	ListPipelines(ctx context.Context) ([]Pipeline, error)
	GetPipeline(ctx context.Context, slug string) (*Pipeline, error)
	SavePipeline(ctx context.Context, pipeline *Pipeline) error
	DeletePipeline(ctx context.Context, slug string) error

	ListJobs(ctx context.Context, filter JobFilter) ([]Job, error)
	GetJob(ctx context.Context, slug string) (*Job, error)
	GetJobByID(ctx context.Context, id string) (*Job, error)
	SaveJob(ctx context.Context, job *Job) error
	DeleteJob(ctx context.Context, slug string) error

	ListTriggers(ctx context.Context, jobID string) ([]Trigger, error)
	SaveTriggers(ctx context.Context, jobID string, triggers []Trigger) error
	EnsureManualTrigger(ctx context.Context, jobID string) (*Trigger, error)

	LogRun(ctx context.Context, rl *RunLog) error
	GetRuns(ctx context.Context, jobID string, limit int) ([]RunLog, error)
	GetLastRuns(ctx context.Context, jobIDs []string) (map[string]*RunLog, error)
	GetRun(ctx context.Context, jobID, runID string) (*RunLog, error)
	LogEvent(ctx context.Context, entry *EventEntry) error
	ListEvents(ctx context.Context, filter EventFilter) ([]EventEntry, error)
	LogRunEvent(ctx context.Context, entry *RunEvent) error
	GetRunEvents(ctx context.Context, runID string) ([]RunEvent, error)

	CleanOldRuns(ctx context.Context, maxAge time.Duration) (int, error)
	CleanOldEvents(ctx context.Context, maxAge time.Duration) (int, error)
	CleanOldRunEvents(ctx context.Context, maxAge time.Duration) (int, error)
}

// DBRepository implementa Repository usando GORM.
type DBRepository struct {
	db  *gorm.DB
	now func() time.Time
}

func NewDBRepository(db *gorm.DB) *DBRepository {
	return &DBRepository{db: db, now: time.Now}
}

func (r *DBRepository) ListTags(ctx context.Context) ([]Tag, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	var rows []database.Tag
	if err := database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").Order("slug ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Tag, 0, len(rows))
	for _, row := range rows {
		out = append(out, tagModelToDomain(row))
	}
	return out, nil
}

func (r *DBRepository) UpsertTag(ctx context.Context, tag *Tag) error {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if tag == nil {
		return fmt.Errorf("tag nil")
	}
	slug := normalizeSlug(tag.Slug)
	if slug == "" {
		slug = normalizeSlug(tag.Name)
	}
	if slug == "" {
		return fmt.Errorf("slug da tag é obrigatório")
	}
	name := strings.TrimSpace(tag.Name)
	if name == "" {
		name = slug
	}
	row := database.Tag{
		UserID:      userID,
		Slug:        slug,
		Name:        name,
		Description: strings.TrimSpace(tag.Description),
		Color:       strings.TrimSpace(tag.Color),
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing database.Tag
		err := tx.Where("user_id = ? AND slug = ?", userID, slug).First(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			tag.ID = row.ID
		case err != nil:
			return err
		default:
			row.ID = existing.ID
			row.CreatedAt = existing.CreatedAt
			if err := tx.Model(&existing).Select("*").Omit("id", "created_at").Updates(&row).Error; err != nil {
				return err
			}
			tag.ID = existing.ID
		}
		tag.Slug = slug
		tag.Name = name
		return nil
	})
}

func (r *DBRepository) SetResourceTags(ctx context.Context, resourceType, resourceID string, tagSlugs []string) error {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	resourceType = strings.TrimSpace(resourceType)
	resourceID = strings.TrimSpace(resourceID)
	if resourceType == "" || resourceID == "" {
		return fmt.Errorf("resourceType e resourceID são obrigatórios")
	}
	slugs := uniqueSlugs(tagSlugs)
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ? AND resource_type = ? AND resource_id = ?", userID, resourceType, resourceID).
			Delete(&database.TagAssignment{}).Error; err != nil {
			return err
		}
		for _, slug := range slugs {
			tag, err := r.ensureTagTx(ctx, tx, userID, slug)
			if err != nil {
				return err
			}
			assign := database.TagAssignment{
				UserID:       userID,
				TagID:        tag.ID,
				ResourceType: resourceType,
				ResourceID:   resourceID,
			}
			if err := tx.Create(&assign).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *DBRepository) GetResourceTags(ctx context.Context, resourceType, resourceID string) ([]Tag, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	var rows []database.Tag
	err := r.db.WithContext(ctx).
		Joins("JOIN tag_assignments ON tag_assignments.tag_id = tags.id").
		Where("tag_assignments.resource_type = ? AND tag_assignments.resource_id = ?", resourceType, resourceID).
		Scopes(func(tx *gorm.DB) *gorm.DB { return database.ScopeByUser(ctx, tx, "tags.user_id") }).
		Order("tags.slug ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]Tag, 0, len(rows))
	for _, row := range rows {
		out = append(out, tagModelToDomain(row))
	}
	return out, nil
}

func (r *DBRepository) ListPipelines(ctx context.Context) ([]Pipeline, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	var rows []database.JobPipeline
	if err := database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").Order("slug ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Pipeline, 0, len(rows))
	for _, row := range rows {
		out = append(out, pipelineModelToDomain(row))
	}
	return out, nil
}

func (r *DBRepository) GetPipeline(ctx context.Context, slug string) (*Pipeline, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	var row database.JobPipeline
	if err := database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").
		Where("slug = ?", normalizeSlug(slug)).First(&row).Error; err != nil {
		return nil, err
	}
	p := pipelineModelToDomain(row)
	return &p, nil
}

func (r *DBRepository) SavePipeline(ctx context.Context, pipeline *Pipeline) error {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if pipeline == nil {
		return fmt.Errorf("pipeline nil")
	}
	slug := normalizeSlug(pipeline.Slug)
	if slug == "" {
		slug = normalizeSlug(pipeline.Name)
	}
	if slug == "" {
		return fmt.Errorf("slug da pipeline é obrigatório")
	}
	name := strings.TrimSpace(pipeline.Name)
	if name == "" {
		name = slug
	}
	meta, err := marshalJSON(pipeline.Metadata)
	if err != nil {
		return err
	}
	row := database.JobPipeline{
		UserID:      userID,
		Slug:        slug,
		Name:        name,
		Description: strings.TrimSpace(pipeline.Description),
		Enabled:     pipeline.Enabled,
		Metadata:    meta,
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing database.JobPipeline
		err := tx.Where("user_id = ? AND slug = ?", userID, slug).First(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			pipeline.ID = row.ID
		case err != nil:
			return err
		default:
			row.ID = existing.ID
			row.CreatedAt = existing.CreatedAt
			if err := tx.Model(&existing).Select("*").Omit("id", "created_at").Updates(&row).Error; err != nil {
				return err
			}
			pipeline.ID = existing.ID
		}
		pipeline.Slug = slug
		pipeline.Name = name
		return nil
	})
}

func (r *DBRepository) DeletePipeline(ctx context.Context, slug string) error {
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	return database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").
		Where("slug = ?", normalizeSlug(slug)).Delete(&database.JobPipeline{}).Error
}

func (r *DBRepository) ListJobs(ctx context.Context, filter JobFilter) ([]Job, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	query := database.ScopeByUser(ctx, r.db.WithContext(ctx), "jobs.user_id").
		Model(&database.Job{}).
		Preload("Pipeline").
		Preload("Triggers").
		Preload("ToolCatalog").
		Joins("LEFT JOIN job_pipelines ON job_pipelines.id = jobs.pipeline_id")
	if filter.Pipeline != "" {
		query = query.Where("job_pipelines.slug = ?", normalizeSlug(filter.Pipeline))
	}
	if filter.Enabled != nil {
		query = query.Where("jobs.enabled = ?", *filter.Enabled)
	}
	if filter.Tag != "" {
		query = query.Joins("JOIN tag_assignments ON tag_assignments.resource_id = jobs.id AND tag_assignments.resource_type = ?", tagResourceJob).
			Joins("JOIN tags ON tags.id = tag_assignments.tag_id").
			Where("tags.slug = ?", normalizeSlug(filter.Tag))
	}
	var rows []database.Job
	if err := query.Order("jobs.slug ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	jobIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		jobIDs = append(jobIDs, row.ID)
	}
	tagsByJobID, err := r.tagSlugsByResourceIDs(ctx, tagResourceJob, jobIDs)
	if err != nil {
		return nil, err
	}
	out := make([]Job, 0, len(rows))
	for _, row := range rows {
		job, err := jobModelToDomainWithTags(row, tagsByJobID[row.ID])
		if err != nil {
			return nil, err
		}
		out = append(out, *job)
	}
	return out, nil
}

func (r *DBRepository) GetJob(ctx context.Context, slug string) (*Job, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	var row database.Job
	if err := database.ScopeByUser(ctx, r.db.WithContext(ctx), "jobs.user_id").
		Preload("Pipeline").Preload("Triggers").Preload("ToolCatalog").
		Where("jobs.slug = ?", normalizeSlug(slug)).First(&row).Error; err != nil {
		return nil, err
	}
	return r.jobModelToDomain(ctx, row)
}

func (r *DBRepository) GetJobByID(ctx context.Context, id string) (*Job, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	var row database.Job
	if err := database.ScopeByUser(ctx, r.db.WithContext(ctx), "jobs.user_id").
		Preload("Pipeline").Preload("Triggers").Preload("ToolCatalog").
		Where("jobs.id = ?", strings.TrimSpace(id)).First(&row).Error; err != nil {
		return nil, err
	}
	return r.jobModelToDomain(ctx, row)
}

func (r *DBRepository) SaveJob(ctx context.Context, job *Job) error {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if err := Validate(job); err != nil {
		return err
	}
	slug := normalizeSlug(job.ID)
	if slug == "" {
		return fmt.Errorf("slug do job é obrigatório")
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		pipelineID, err := r.pipelineIDForSlugTx(ctx, tx, userID, job.Pipeline)
		if err != nil {
			return err
		}
		toolCatalogID, err := r.toolCatalogIDForNameTx(ctx, tx, userID, job.Tool)
		if err != nil {
			return err
		}
		row, err := jobDomainToModel(userID, slug, pipelineID, toolCatalogID, job)
		if err != nil {
			return err
		}
		var existing database.Job
		err = tx.Where("user_id = ? AND slug = ?", userID, slug).First(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			if err := tx.Create(row).Error; err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			row.ID = existing.ID
			row.CreatedAt = existing.CreatedAt
			if strings.TrimSpace(row.CreatedBy) == "" {
				row.CreatedBy = existing.CreatedBy
			}
			if err := tx.Model(&existing).Select("*").Omit("id", "created_at").Updates(row).Error; err != nil {
				return err
			}
		}
		if err := r.saveTriggersTx(ctx, tx, userID, row.ID, job.Triggers); err != nil {
			return err
		}
		if err := r.setResourceTagsTx(ctx, tx, userID, tagResourceJob, row.ID, job.Tags); err != nil {
			return err
		}
		job.ID = slug
		return nil
	})
}

func (r *DBRepository) DeleteJob(ctx context.Context, slug string) error {
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row database.Job
		if err := database.ScopeByUser(ctx, tx, "user_id").Where("slug = ?", normalizeSlug(slug)).First(&row).Error; err != nil {
			return err
		}
		if err := tx.Where("job_id = ?", row.ID).Delete(&database.JobTrigger{}).Error; err != nil {
			return err
		}
		if err := tx.Where("resource_type = ? AND resource_id = ?", tagResourceJob, row.ID).Delete(&database.TagAssignment{}).Error; err != nil {
			return err
		}
		return tx.Delete(&row).Error
	})
}

func (r *DBRepository) ListTriggers(ctx context.Context, jobID string) ([]Trigger, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	dbID, err := r.resolveJobDBID(ctx, jobID)
	if err != nil {
		return nil, err
	}
	var rows []database.JobTrigger
	if err := database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").Where("job_id = ?", dbID).Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Trigger, 0, len(rows))
	for _, row := range rows {
		trig, err := triggerModelToDomain(row)
		if err != nil {
			return nil, err
		}
		out = append(out, trig)
	}
	return out, nil
}

func (r *DBRepository) SaveTriggers(ctx context.Context, jobID string, triggers []Trigger) error {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	dbID, err := r.resolveJobDBID(ctx, jobID)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return r.saveTriggersTx(ctx, tx, userID, dbID, triggers)
	})
}

func (r *DBRepository) EnsureManualTrigger(ctx context.Context, jobID string) (*Trigger, error) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	dbID, err := r.resolveJobDBID(ctx, jobID)
	if err != nil {
		return nil, err
	}
	var row database.JobTrigger
	err = r.db.WithContext(ctx).Where("user_id = ? AND job_id = ? AND type = ?", userID, dbID, string(TriggerManual)).First(&row).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		row = database.JobTrigger{UserID: userID, JobID: dbID, Type: string(TriggerManual), Enabled: true}
		if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
			return nil, err
		}
	case err != nil:
		return nil, err
	}
	trig, err := triggerModelToDomain(row)
	return &trig, err
}

func (r *DBRepository) LogRun(ctx context.Context, rl *RunLog) error {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if rl == nil {
		return fmt.Errorf("run log nil")
	}
	jobRow, err := r.jobRowBySlug(ctx, rl.JobID)
	if err != nil {
		return err
	}
	triggerID, err := r.triggerIDForRun(ctx, userID, jobRow.ID, rl.Trigger)
	if err != nil {
		return err
	}
	completedAt := nullableTime(rl.CompletedAt)
	row := database.JobRun{
		UUIDModel:   database.UUIDModel{ID: strings.TrimSpace(rl.RunID)},
		UserID:      userID,
		JobID:       jobRow.ID,
		TriggerID:   triggerID,
		Status:      rl.Status,
		StartedAt:   rl.StartedAt,
		CompletedAt: completedAt,
		DurationMs:  durationMillis(rl.StartedAt, rl.CompletedAt),
		Error:       rl.Error,
		RetryCount:  rl.RetryCount,
		IsDryRun:    rl.IsDryRun,
	}
	if row.StartedAt.IsZero() {
		row.StartedAt = r.now()
	}
	return r.db.WithContext(ctx).Save(&row).Error
}

func (r *DBRepository) GetRuns(ctx context.Context, jobID string, limit int) ([]RunLog, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	jobRow, err := r.jobRowBySlug(ctx, jobID)
	if err != nil {
		return nil, err
	}
	query := database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").Where("job_id = ?", jobRow.ID).Order("started_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var rows []database.JobRun
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]RunLog, 0, len(rows))
	for _, row := range rows {
		out = append(out, runModelToDomain(row, jobRow.Slug))
	}
	return out, nil
}

func (r *DBRepository) GetLastRuns(ctx context.Context, jobIDs []string) (map[string]*RunLog, error) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	slugs := uniqueSlugs(jobIDs)
	out := make(map[string]*RunLog, len(slugs))
	if len(slugs) == 0 {
		return out, nil
	}
	type lastRunRow struct {
		database.JobRun
		JobSlug string `gorm:"column:job_slug"`
	}
	var rows []lastRunRow
	if err := r.db.WithContext(ctx).
		Raw(`
			SELECT *
			FROM (
				SELECT
					job_runs.*,
					jobs.slug AS job_slug,
					ROW_NUMBER() OVER (
						PARTITION BY job_runs.job_id
						ORDER BY job_runs.started_at DESC, job_runs.created_at DESC, job_runs.id DESC
					) AS rn
				FROM job_runs
				JOIN jobs ON jobs.id = job_runs.job_id
				WHERE job_runs.user_id = ? AND jobs.user_id = ? AND jobs.slug IN ?
			)
			WHERE rn = 1
			ORDER BY started_at DESC, created_at DESC
		`, userID, userID, slugs).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.JobSlug == "" {
			continue
		}
		rl := runModelToDomain(row.JobRun, row.JobSlug)
		out[row.JobSlug] = &rl
	}
	return out, nil
}

func (r *DBRepository) GetRun(ctx context.Context, jobID, runID string) (*RunLog, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	jobRow, err := r.jobRowBySlug(ctx, jobID)
	if err != nil {
		return nil, err
	}
	var row database.JobRun
	if err := database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").
		Where("job_id = ? AND id = ?", jobRow.ID, strings.TrimSpace(runID)).First(&row).Error; err != nil {
		return nil, err
	}
	rl := runModelToDomain(row, jobRow.Slug)
	return &rl, nil
}

func (r *DBRepository) LogEvent(ctx context.Context, entry *EventEntry) error {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if entry == nil {
		return fmt.Errorf("event entry nil")
	}
	var jobID *string
	if strings.TrimSpace(entry.JobID) != "" {
		jobRow, err := r.jobRowBySlug(ctx, entry.JobID)
		if err != nil {
			return err
		}
		jobID = &jobRow.ID
	}
	data, err := marshalJSON(entry.Data)
	if err != nil {
		return err
	}
	occurredAt := entry.Timestamp
	if occurredAt.IsZero() {
		occurredAt = r.now()
	}
	row := database.JobEvent{
		UUIDModel:  database.UUIDModel{ID: strings.TrimSpace(entry.ID)},
		UserID:     userID,
		JobID:      jobID,
		OccurredAt: occurredAt,
		Type:       entry.Type,
		Event:      entry.Event,
		Message:    entry.Message,
		Data:       data,
	}
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *DBRepository) ListEvents(ctx context.Context, filter EventFilter) ([]EventEntry, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	type eventRow struct {
		database.JobEvent
		JobSlug string `gorm:"column:job_slug"`
	}
	query := database.ScopeByUser(ctx, r.db.WithContext(ctx), "job_events.user_id").Model(&database.JobEvent{}).
		Select("job_events.*, jobs.slug AS job_slug").
		Joins("LEFT JOIN jobs ON jobs.id = job_events.job_id")
	if filter.JobID != "" {
		query = query.Where("jobs.slug = ?", normalizeSlug(filter.JobID))
	}
	if filter.Type != "" {
		query = query.Where("job_events.type = ?", filter.Type)
	}
	if filter.Event != "" {
		query = query.Where("job_events.event = ?", filter.Event)
	}
	if !filter.StartAt.IsZero() {
		query = query.Where("job_events.occurred_at >= ?", filter.StartAt)
	}
	if !filter.EndAt.IsZero() {
		query = query.Where("job_events.occurred_at < ?", filter.EndAt)
	}
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	var rows []eventRow
	if err := query.Order("job_events.occurred_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]EventEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, eventModelToDomain(row.JobEvent, row.JobSlug))
	}
	return out, nil
}

func (r *DBRepository) LogRunEvent(ctx context.Context, entry *RunEvent) error {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if entry == nil {
		return fmt.Errorf("run event nil")
	}
	data, err := marshalJSON(entry.Data)
	if err != nil {
		return err
	}
	occurredAt := entry.Timestamp
	if occurredAt.IsZero() {
		occurredAt = r.now()
	}
	row := database.JobRunEvent{
		UUIDModel:  database.UUIDModel{ID: strings.TrimSpace(entry.ID)},
		UserID:     userID,
		JobRunID:   strings.TrimSpace(entry.RunID),
		Sequence:   entry.Sequence,
		OccurredAt: occurredAt,
		Type:       entry.Type,
		Message:    entry.Message,
		Data:       data,
	}
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *DBRepository) GetRunEvents(ctx context.Context, runID string) ([]RunEvent, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	var rows []database.JobRunEvent
	if err := database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").
		Where("job_run_id = ?", strings.TrimSpace(runID)).Order("sequence ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]RunEvent, 0, len(rows))
	for _, row := range rows {
		out = append(out, runEventModelToDomain(row))
	}
	return out, nil
}

func (r *DBRepository) CleanOldRuns(ctx context.Context, maxAge time.Duration) (int, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return 0, err
	}
	cutoff := r.now().Add(-maxAge)
	res := database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").
		Where("started_at < ?", cutoff).Delete(&database.JobRun{})
	return int(res.RowsAffected), res.Error
}

func (r *DBRepository) CleanOldEvents(ctx context.Context, maxAge time.Duration) (int, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return 0, err
	}
	cutoff := r.now().Add(-maxAge)
	res := database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").
		Where("occurred_at < ?", cutoff).Delete(&database.JobEvent{})
	return int(res.RowsAffected), res.Error
}

func (r *DBRepository) CleanOldRunEvents(ctx context.Context, maxAge time.Duration) (int, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return 0, err
	}
	cutoff := r.now().Add(-maxAge)
	res := database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").
		Where("occurred_at < ?", cutoff).Delete(&database.JobRunEvent{})
	return int(res.RowsAffected), res.Error
}

func (r *DBRepository) ensureTagTx(ctx context.Context, tx *gorm.DB, userID, slug string) (*database.Tag, error) {
	slug = normalizeSlug(slug)
	if slug == "" {
		return nil, fmt.Errorf("tag vazia")
	}
	var row database.Tag
	err := tx.WithContext(ctx).Where("user_id = ? AND slug = ?", userID, slug).First(&row).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		row = database.Tag{UserID: userID, Slug: slug, Name: slug}
		if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
			return nil, err
		}
	case err != nil:
		return nil, err
	}
	return &row, nil
}

func (r *DBRepository) setResourceTagsTx(ctx context.Context, tx *gorm.DB, userID, resourceType, resourceID string, tagSlugs []string) error {
	if err := tx.WithContext(ctx).Where("user_id = ? AND resource_type = ? AND resource_id = ?", userID, resourceType, resourceID).
		Delete(&database.TagAssignment{}).Error; err != nil {
		return err
	}
	for _, slug := range uniqueSlugs(tagSlugs) {
		tag, err := r.ensureTagTx(ctx, tx, userID, slug)
		if err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Create(&database.TagAssignment{
			UserID:       userID,
			TagID:        tag.ID,
			ResourceType: resourceType,
			ResourceID:   resourceID,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *DBRepository) tagSlugsByResourceIDs(ctx context.Context, resourceType string, resourceIDs []string) (map[string][]string, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	out := make(map[string][]string, len(resourceIDs))
	if len(resourceIDs) == 0 {
		return out, nil
	}
	type row struct {
		ResourceID string `gorm:"column:resource_id"`
		Slug       string `gorm:"column:slug"`
	}
	var rows []row
	if err := database.ScopeByUser(ctx, r.db.WithContext(ctx).Model(&database.TagAssignment{}), "tag_assignments.user_id").
		Select("tag_assignments.resource_id, tags.slug").
		Joins("JOIN tags ON tags.id = tag_assignments.tag_id").
		Where("tag_assignments.resource_type = ? AND tag_assignments.resource_id IN ?", resourceType, resourceIDs).
		Order("tags.slug ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ResourceID] = append(out[row.ResourceID], row.Slug)
	}
	return out, nil
}

func (r *DBRepository) pipelineIDForSlugTx(ctx context.Context, tx *gorm.DB, userID, slug string) (*string, error) {
	slug = normalizeSlug(slug)
	if slug == "" {
		return nil, nil
	}
	var row database.JobPipeline
	err := tx.WithContext(ctx).Where("user_id = ? AND slug = ?", userID, slug).First(&row).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		row = database.JobPipeline{UserID: userID, Slug: slug, Name: slug, Enabled: true}
		if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
			return nil, err
		}
	case err != nil:
		return nil, err
	}
	return &row.ID, nil
}

func (r *DBRepository) toolCatalogIDForNameTx(ctx context.Context, tx *gorm.DB, userID, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("tool é obrigatória")
	}
	var row database.ToolCatalog
	err := tx.WithContext(ctx).
		Joins("LEFT JOIN mcp_servers ON mcp_servers.id = tool_catalog.mcp_server_id").
		Where("tool_catalog.name = ? AND (tool_catalog.user_id IS NULL OR tool_catalog.user_id = ? OR mcp_servers.user_id = ?)", name, userID, userID).
		Order("tool_catalog.user_id IS NULL ASC").
		First(&row).Error
	if err != nil {
		return "", err
	}
	return row.ID, nil
}

func (r *DBRepository) saveTriggersTx(ctx context.Context, tx *gorm.DB, userID, jobID string, triggers []Trigger) error {
	if err := tx.WithContext(ctx).Where("job_id = ?", jobID).Delete(&database.JobTrigger{}).Error; err != nil {
		return err
	}
	hasManual := false
	for _, trigger := range triggers {
		if trigger.Type == TriggerManual {
			hasManual = true
		}
		row, err := triggerDomainToModel(userID, jobID, trigger)
		if err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Create(row).Error; err != nil {
			return err
		}
	}
	if !hasManual {
		return tx.WithContext(ctx).Create(&database.JobTrigger{UserID: userID, JobID: jobID, Type: string(TriggerManual), Enabled: true}).Error
	}
	return nil
}

func (r *DBRepository) resolveJobDBID(ctx context.Context, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	var row database.Job
	if err := database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").
		Where("id = ? OR slug = ?", ref, normalizeSlug(ref)).First(&row).Error; err != nil {
		return "", err
	}
	return row.ID, nil
}

func (r *DBRepository) jobRowBySlug(ctx context.Context, slug string) (*database.Job, error) {
	var row database.Job
	if err := database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").
		Where("slug = ?", normalizeSlug(slug)).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *DBRepository) triggerIDForRun(ctx context.Context, userID, jobID string, info TriggerInfo) (string, error) {
	triggerType := string(info.Type)
	if triggerType == "" {
		triggerType = string(TriggerManual)
	}
	var row database.JobTrigger
	query := r.db.WithContext(ctx).Where("user_id = ? AND job_id = ? AND type = ?", userID, jobID, triggerType)
	if info.Event != "" {
		query = query.Where("expression = ?", info.Event)
	}
	err := query.First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) && triggerType == string(TriggerManual) {
		row = database.JobTrigger{UserID: userID, JobID: jobID, Type: string(TriggerManual), Enabled: true}
		err = r.db.WithContext(ctx).Create(&row).Error
	}
	if err != nil {
		return "", err
	}
	return row.ID, nil
}

func (r *DBRepository) jobModelToDomain(ctx context.Context, row database.Job) (*Job, error) {
	tags, err := r.GetResourceTags(ctx, tagResourceJob, row.ID)
	if err != nil {
		return nil, err
	}
	tagSlugs := make([]string, 0, len(tags))
	for _, tag := range tags {
		tagSlugs = append(tagSlugs, tag.Slug)
	}
	return jobModelToDomainWithTags(row, tagSlugs)
}

func jobModelToDomainWithTags(row database.Job, tagSlugs []string) (*Job, error) {
	var inputs map[string]any
	if err := unmarshalJSON(row.Inputs, &inputs); err != nil {
		return nil, err
	}
	var output OutputConfig
	if err := unmarshalJSON(row.OutputConfig, &output); err != nil {
		return nil, err
	}
	var events EventsConfig
	if err := unmarshalJSON(row.EventsConfig, &events); err != nil {
		return nil, err
	}
	var policy ErrorPolicy
	if err := unmarshalJSON(row.ErrorPolicy, &policy); err != nil {
		return nil, err
	}
	var dryRun DryRunConfig
	if err := unmarshalJSON(row.DryRunConfig, &dryRun); err != nil {
		return nil, err
	}
	triggers := make([]Trigger, 0, len(row.Triggers))
	for _, trigRow := range row.Triggers {
		trig, err := triggerModelToDomain(trigRow)
		if err != nil {
			return nil, err
		}
		triggers = append(triggers, trig)
	}
	pipelineSlug := ""
	if row.Pipeline != nil {
		pipelineSlug = row.Pipeline.Slug
	}
	toolName := row.ToolCatalogID
	if row.ToolCatalog != nil && row.ToolCatalog.Name != "" {
		toolName = row.ToolCatalog.Name
	}
	return &Job{
		ID:             row.Slug,
		Name:           row.Name,
		Description:    row.Description,
		Enabled:        row.Enabled,
		Pipeline:       pipelineSlug,
		Tags:           tagSlugs,
		Triggers:       triggers,
		Tool:           toolName,
		Inputs:         inputs,
		Output:         output,
		Events:         events,
		ErrorPolicy:    policy,
		MaxRunsPerHour: row.MaxRunsPerHour,
		DryRun:         dryRun,
		Metadata: Metadata{
			CreatedAt: row.CreatedAt.Format(time.RFC3339),
			CreatedBy: row.CreatedBy,
			UpdatedAt: row.UpdatedAt.Format(time.RFC3339),
		},
	}, nil
}

func jobDomainToModel(userID, slug string, pipelineID *string, toolCatalogID string, job *Job) (*database.Job, error) {
	inputs, err := marshalJSON(job.Inputs)
	if err != nil {
		return nil, err
	}
	output, err := marshalJSON(job.Output)
	if err != nil {
		return nil, err
	}
	events, err := marshalJSON(job.Events)
	if err != nil {
		return nil, err
	}
	policy, err := marshalJSON(job.ErrorPolicy)
	if err != nil {
		return nil, err
	}
	dryRun, err := marshalJSON(job.DryRun)
	if err != nil {
		return nil, err
	}
	return &database.Job{
		UserID:         userID,
		PipelineID:     pipelineID,
		Slug:           slug,
		Name:           strings.TrimSpace(job.Name),
		Description:    strings.TrimSpace(job.Description),
		Enabled:        job.Enabled,
		ToolCatalogID:  toolCatalogID,
		Inputs:         inputs,
		OutputConfig:   output,
		EventsConfig:   events,
		ErrorPolicy:    policy,
		MaxRunsPerHour: job.MaxRunsPerHour,
		DryRunConfig:   dryRun,
		CreatedBy:      job.Metadata.CreatedBy,
	}, nil
}

func triggerDomainToModel(userID, jobID string, trigger Trigger) (*database.JobTrigger, error) {
	config, err := marshalJSON(trigger)
	if err != nil {
		return nil, err
	}
	expression := trigger.Expression
	if expression == "" {
		expression = trigger.Every
	}
	if expression == "" {
		expression = trigger.Listen
	}
	if expression == "" {
		expression = trigger.Keys
	}
	return &database.JobTrigger{
		UserID:     userID,
		JobID:      jobID,
		Type:       string(trigger.Type),
		Enabled:    true,
		Expression: expression,
		Config:     config,
	}, nil
}

func triggerModelToDomain(row database.JobTrigger) (Trigger, error) {
	var trigger Trigger
	if strings.TrimSpace(row.Config) != "" {
		if err := json.Unmarshal([]byte(row.Config), &trigger); err != nil {
			return trigger, err
		}
	}
	if trigger.Type == "" {
		trigger.Type = TriggerType(row.Type)
	}
	return trigger, nil
}

func pipelineModelToDomain(row database.JobPipeline) Pipeline {
	var meta map[string]any
	_ = unmarshalJSON(row.Metadata, &meta)
	return Pipeline{
		ID:          row.ID,
		Slug:        row.Slug,
		Name:        row.Name,
		Description: row.Description,
		Enabled:     row.Enabled,
		Metadata:    meta,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func tagModelToDomain(row database.Tag) Tag {
	return Tag{
		ID:          row.ID,
		Slug:        row.Slug,
		Name:        row.Name,
		Description: row.Description,
		Color:       row.Color,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func runModelToDomain(row database.JobRun, jobSlug string) RunLog {
	rl := RunLog{
		RunID:       row.ID,
		JobID:       jobSlug,
		Status:      row.Status,
		StartedAt:   row.StartedAt,
		Error:       row.Error,
		RetryCount:  row.RetryCount,
		IsDryRun:    row.IsDryRun,
		Duration:    time.Duration(row.DurationMs * int64(time.Millisecond)).String(),
		CompletedAt: time.Time{},
	}
	if row.CompletedAt != nil {
		rl.CompletedAt = *row.CompletedAt
	}
	return rl
}

func eventModelToDomain(row database.JobEvent, jobSlug string) EventEntry {
	var data map[string]any
	_ = unmarshalJSON(row.Data, &data)
	return EventEntry{
		ID:        row.ID,
		JobID:     jobSlug,
		Timestamp: row.OccurredAt,
		Type:      row.Type,
		Event:     row.Event,
		Message:   row.Message,
		Data:      data,
	}
}

func runEventModelToDomain(row database.JobRunEvent) RunEvent {
	var data map[string]any
	_ = unmarshalJSON(row.Data, &data)
	return RunEvent{
		ID:        row.ID,
		RunID:     row.JobRunID,
		Sequence:  row.Sequence,
		Timestamp: row.OccurredAt,
		Type:      row.Type,
		Message:   row.Message,
		Data:      data,
	}
}

func marshalJSON(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	if string(b) == "null" || string(b) == "{}" || string(b) == "[]" {
		return "", nil
	}
	return string(b), nil
}

func unmarshalJSON(raw string, dst any) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), dst)
}

func normalizeSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

func uniqueSlugs(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		slug := normalizeSlug(value)
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		out = append(out, slug)
	}
	return out
}

func nullableTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func durationMillis(start, end time.Time) int64 {
	if start.IsZero() || end.IsZero() {
		return 0
	}
	return end.Sub(start).Milliseconds()
}
