package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"assistente/internal/database"
	"assistente/internal/tools"

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
	CreatePipeline(ctx context.Context, pipeline *Pipeline) error
	SavePipeline(ctx context.Context, pipeline *Pipeline) error
	DeletePipeline(ctx context.Context, slug string) error

	ListJobs(ctx context.Context, filter JobFilter) ([]Job, error)
	GetJob(ctx context.Context, slug string) (*Job, error)
	GetJobByID(ctx context.Context, id string) (*Job, error)
	CreateJob(ctx context.Context, job *Job) error
	SaveJob(ctx context.Context, job *Job) error
	DeleteJob(ctx context.Context, slug string) error

	ListTriggers(ctx context.Context, jobID string) ([]Trigger, error)
	SaveTriggers(ctx context.Context, jobID string, triggers []Trigger) error
	EnsureManualTrigger(ctx context.Context, jobID string) (*Trigger, error)
	ListToolCatalog(ctx context.Context) ([]CatalogEntry, error)

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
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var rows []database.Tag
	err = r.db.WithContext(ctx).
		Joins("JOIN tag_assignments ON tag_assignments.tag_id = tags.id").
		Where("tag_assignments.user_id = ? AND tag_assignments.resource_type = ? AND tag_assignments.resource_id = ?", userID, resourceType, resourceID).
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

func (r *DBRepository) CreatePipeline(ctx context.Context, pipeline *Pipeline) error {
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
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		if isUniqueConstraintError(err) {
			return fmt.Errorf("%w: %s", ErrPipelineAlreadyExists, slug)
		}
		return err
	}
	pipeline.ID = row.ID
	pipeline.Slug = slug
	pipeline.Name = name
	pipeline.CreatedAt = row.CreatedAt
	pipeline.UpdatedAt = row.UpdatedAt
	return nil
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
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row database.JobPipeline
		if err := database.ScopeByUser(ctx, tx, "user_id").Where("slug = ?", normalizeSlug(slug)).First(&row).Error; err != nil {
			return err
		}
		if err := tx.Model(&database.Job{}).Where("user_id = ? AND pipeline_id = ?", userID, row.ID).
			Update("pipeline_id", gorm.Expr("NULL")).Error; err != nil {
			return err
		}
		return tx.Delete(&row).Error
	})
}

func (r *DBRepository) ListJobs(ctx context.Context, filter JobFilter) ([]Job, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	query := database.ScopeByUser(ctx, r.db.WithContext(ctx), "jobs.user_id").
		Model(&database.Job{}).
		Preload("Pipeline").
		Preload("Triggers", "enabled = ?", true).
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
			Where("tag_assignments.user_id = jobs.user_id AND tags.user_id = jobs.user_id").
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
		Preload("Pipeline").Preload("Triggers", "enabled = ?", true).Preload("ToolCatalog").
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
		Preload("Pipeline").Preload("Triggers", "enabled = ?", true).Preload("ToolCatalog").
		Where("jobs.id = ?", strings.TrimSpace(id)).First(&row).Error; err != nil {
		return nil, err
	}
	return r.jobModelToDomain(ctx, row)
}

func (r *DBRepository) CreateJob(ctx context.Context, job *Job) error {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if job == nil {
		return fmt.Errorf("job is required")
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
		if err := tx.Create(row).Error; err != nil {
			if isUniqueConstraintError(err) {
				return fmt.Errorf("%w: %s", ErrJobAlreadyExists, slug)
			}
			return err
		}
		if err := r.saveTriggersTx(ctx, tx, userID, row.ID, job.Triggers); err != nil {
			return err
		}
		if err := r.setResourceTagsTx(ctx, tx, userID, tagResourceJob, row.ID, job.Tags); err != nil {
			return err
		}
		pipelineEnabled := pipelineID == nil
		if pipelineID != nil {
			var err error
			pipelineEnabled, err = r.pipelineEnabledByIDTx(ctx, tx, userID, *pipelineID)
			if err != nil {
				return err
			}
		}
		job.ID = slug
		job.Pipeline = normalizeSlug(job.Pipeline)
		job.PipelineEnabled = pipelineEnabled
		job.Tags = uniqueSlugs(job.Tags)
		return nil
	})
}

func (r *DBRepository) SaveJob(ctx context.Context, job *Job) error {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if job == nil {
		return fmt.Errorf("job is required")
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
			row.CreatedBy = existing.CreatedBy
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
		pipelineEnabled := pipelineID == nil
		if pipelineID != nil {
			var err error
			pipelineEnabled, err = r.pipelineEnabledByIDTx(ctx, tx, userID, *pipelineID)
			if err != nil {
				return err
			}
		}
		job.ID = slug
		job.Pipeline = normalizeSlug(job.Pipeline)
		job.PipelineEnabled = pipelineEnabled
		job.Tags = uniqueSlugs(job.Tags)
		return nil
	})
}

func (r *DBRepository) DeleteJob(ctx context.Context, slug string) error {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row database.Job
		if err := database.ScopeByUser(ctx, tx, "user_id").Where("slug = ?", normalizeSlug(slug)).First(&row).Error; err != nil {
			return err
		}
		var runIDs []string
		if err := tx.Model(&database.JobRun{}).Where("user_id = ? AND job_id = ?", userID, row.ID).Pluck("id", &runIDs).Error; err != nil {
			return err
		}
		if len(runIDs) > 0 {
			if err := tx.Where("user_id = ? AND job_run_id IN ?", userID, runIDs).Delete(&database.JobRunEvent{}).Error; err != nil {
				return err
			}
			if err := tx.Where("user_id = ? AND job_run_id IN ?", userID, runIDs).Delete(&database.JobEvent{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("user_id = ? AND job_id = ?", userID, row.ID).Delete(&database.JobRun{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ? AND job_id = ?", userID, row.ID).Delete(&database.JobEvent{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ? AND job_id = ?", userID, row.ID).Delete(&database.JobTrigger{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ? AND resource_type = ? AND resource_id = ?", userID, tagResourceJob, row.ID).Delete(&database.TagAssignment{}).Error; err != nil {
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
	if err := database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").Where("job_id = ? AND enabled = ?", dbID, true).Order("created_at ASC").Find(&rows).Error; err != nil {
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

func (r *DBRepository) ListToolCatalog(ctx context.Context) ([]CatalogEntry, error) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var rows []database.ToolCatalog
	if err := r.db.WithContext(ctx).
		Joins("LEFT JOIN mcp_servers ON mcp_servers.id = tool_catalog.mcp_server_id").
		Where("tool_catalog.user_id IS NULL OR tool_catalog.user_id = ? OR mcp_servers.user_id = ?", userID, userID).
		Order("tool_catalog.name ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	entries := make([]CatalogEntry, 0, len(rows))
	for _, row := range rows {
		schema := json.RawMessage(row.Schema)
		if len(schema) == 0 {
			schema = json.RawMessage(`{}`)
		}
		source := "internal"
		if row.Origin == tools.ToolOriginMCPBridge || strings.HasPrefix(row.Name, "mcp_") {
			source = "mcp"
		}
		description := row.Description
		if description == "" && row.AvailabilityStatus != "" && row.AvailabilityStatus != tools.ToolAvailabilityAvailable {
			description = row.AvailabilityReason
		}
		entries = append(entries, CatalogEntry{
			Name:               row.Name,
			Description:        description,
			Schema:             schema,
			Source:             source,
			AvailabilityStatus: row.AvailabilityStatus,
			AvailabilityReason: row.AvailabilityReason,
		})
	}
	return entries, nil
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
	triggerData, err := marshalJSON(rl.Trigger)
	if err != nil {
		return err
	}
	inputs, err := marshalJSON(RedactResolvedInputs(nil, rl.ResolvedInputs))
	if err != nil {
		return err
	}
	output, err := marshalJSON(rl.Output)
	if err != nil {
		return err
	}
	events, err := marshalJSON(rl.EventsEmitted)
	if err != nil {
		return err
	}
	row := database.JobRun{
		UUIDModel:     database.UUIDModel{ID: strings.TrimSpace(rl.RunID)},
		UserID:        userID,
		JobID:         jobRow.ID,
		TriggerID:     triggerID,
		Status:        rl.Status,
		StartedAt:     rl.StartedAt,
		CompletedAt:   completedAt,
		DurationMs:    durationMillis(rl.StartedAt, rl.CompletedAt),
		Error:         rl.Error,
		RetryCount:    rl.RetryCount,
		IsDryRun:      rl.IsDryRun,
		ToolName:      rl.ToolName,
		TriggerData:   triggerData,
		Inputs:        inputs,
		Output:        output,
		EventsEmitted: events,
	}
	if row.StartedAt.IsZero() {
		row.StartedAt = r.now()
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		rl.RunID = row.ID
		for i, event := range rl.RunEvents {
			event.RunID = row.ID
			if event.Sequence <= 0 {
				event.Sequence = i + 1
			}
			if event.Timestamp.IsZero() {
				event.Timestamp = r.now()
			}
			data, err := marshalJSON(event.Data)
			if err != nil {
				return err
			}
			eventRow := database.JobRunEvent{
				UUIDModel:  database.UUIDModel{ID: strings.TrimSpace(event.ID)},
				UserID:     userID,
				JobRunID:   row.ID,
				Sequence:   event.Sequence,
				OccurredAt: event.Timestamp,
				Type:       event.Type,
				Message:    event.Message,
				Data:       data,
			}
			if err := tx.Create(&eventRow).Error; err != nil {
				return err
			}
		}
		for _, event := range rl.DomainEvents {
			event.RunID = row.ID
			if event.Timestamp.IsZero() {
				event.Timestamp = r.now()
			}
			data, err := marshalJSON(event.Data)
			if err != nil {
				return err
			}
			eventRow := database.JobEvent{
				UUIDModel:  database.UUIDModel{ID: strings.TrimSpace(event.ID)},
				UserID:     userID,
				JobID:      &jobRow.ID,
				JobRunID:   &row.ID,
				OccurredAt: event.Timestamp,
				Type:       event.Type,
				Event:      event.Event,
				Message:    event.Message,
				Data:       data,
			}
			if err := tx.Create(&eventRow).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *DBRepository) GetRuns(ctx context.Context, jobID string, limit int) ([]RunLog, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	jobRow, err := r.jobRowBySlug(ctx, jobID)
	if err != nil {
		return nil, err
	}
	query := database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").Where("job_id = ?", jobRow.ID).
		Order("started_at DESC, created_at DESC, id DESC")
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
	var runID *string
	if strings.TrimSpace(entry.RunID) != "" {
		var run database.JobRun
		if err := database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").
			Where("id = ?", strings.TrimSpace(entry.RunID)).First(&run).Error; err != nil {
			return err
		}
		runID = &run.ID
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
		JobRunID:   runID,
		OccurredAt: occurredAt,
		Type:       entry.Type,
		Event:      entry.Event,
		Message:    entry.Message,
		Data:       data,
	}
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *DBRepository) ListEvents(ctx context.Context, filter EventFilter) ([]EventEntry, error) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	type combinedEventRow struct {
		ID         string    `gorm:"column:id"`
		JobSlug    string    `gorm:"column:job_slug"`
		OccurredAt time.Time `gorm:"column:occurred_at"`
		Type       string    `gorm:"column:type"`
		Event      string    `gorm:"column:event"`
		Message    string    `gorm:"column:message"`
		Data       string    `gorm:"column:data"`
		JobRunID   string    `gorm:"column:job_run_id"`
	}
	whereJobEvents := []string{"job_events.user_id = ?"}
	jobArgs := []any{userID}
	whereRunEvents := []string{"job_run_events.user_id = ?"}
	runArgs := []any{userID}
	if filter.JobID != "" {
		whereJobEvents = append(whereJobEvents, "jobs.slug = ?")
		jobArgs = append(jobArgs, normalizeSlug(filter.JobID))
		whereRunEvents = append(whereRunEvents, "jobs.slug = ?")
		runArgs = append(runArgs, normalizeSlug(filter.JobID))
	}
	if filter.Type != "" {
		whereJobEvents = append(whereJobEvents, "job_events.type = ?")
		jobArgs = append(jobArgs, filter.Type)
		whereRunEvents = append(whereRunEvents, "job_run_events.type = ?")
		runArgs = append(runArgs, filter.Type)
	}
	if filter.Event != "" {
		whereJobEvents = append(whereJobEvents, "job_events.event = ?")
		jobArgs = append(jobArgs, filter.Event)
	}
	if !filter.StartAt.IsZero() {
		whereJobEvents = append(whereJobEvents, "job_events.occurred_at >= ?")
		jobArgs = append(jobArgs, filter.StartAt)
		whereRunEvents = append(whereRunEvents, "job_run_events.occurred_at >= ?")
		runArgs = append(runArgs, filter.StartAt)
	}
	if !filter.EndAt.IsZero() {
		whereJobEvents = append(whereJobEvents, "job_events.occurred_at < ?")
		jobArgs = append(jobArgs, filter.EndAt)
		whereRunEvents = append(whereRunEvents, "job_run_events.occurred_at < ?")
		runArgs = append(runArgs, filter.EndAt)
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	jobSQL := `SELECT job_events.id AS id, COALESCE(jobs.slug, '') AS job_slug, job_events.occurred_at AS occurred_at, job_events.type AS type, job_events.event AS event, job_events.message AS message, job_events.data AS data, COALESCE(job_events.job_run_id, '') AS job_run_id, job_events.created_at AS created_at FROM job_events LEFT JOIN jobs ON jobs.id = job_events.job_id WHERE ` + strings.Join(whereJobEvents, " AND ")
	parts := []string{jobSQL}
	args := append([]any{}, jobArgs...)
	if filter.Event == "" {
		runSQL := `SELECT job_run_events.id AS id, jobs.slug AS job_slug, job_run_events.occurred_at AS occurred_at, job_run_events.type AS type, '' AS event, job_run_events.message AS message, job_run_events.data AS data, job_run_events.job_run_id AS job_run_id, job_run_events.created_at AS created_at FROM job_run_events JOIN job_runs ON job_runs.id = job_run_events.job_run_id AND job_runs.user_id = job_run_events.user_id JOIN jobs ON jobs.id = job_runs.job_id WHERE ` + strings.Join(whereRunEvents, " AND ")
		parts = append(parts, runSQL)
		args = append(args, runArgs...)
	}
	sql := "SELECT * FROM (" + strings.Join(parts, " UNION ALL ") + ") AS combined ORDER BY occurred_at DESC, created_at DESC, id DESC"
	if filter.Limit > 0 {
		args = append(args, filter.Limit, offset)
		sql += " LIMIT ? OFFSET ?"
	}
	var rows []combinedEventRow
	if err := r.db.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]EventEntry, 0, len(rows))
	for _, row := range rows {
		var data map[string]any
		_ = unmarshalJSON(row.Data, &data)
		if row.JobRunID != "" {
			if data == nil {
				data = make(map[string]any)
			}
			data["run_id"] = row.JobRunID
		}
		out = append(out, EventEntry{
			ID:        row.ID,
			JobID:     row.JobSlug,
			RunID:     row.JobRunID,
			Timestamp: row.OccurredAt,
			Type:      row.Type,
			Event:     row.Event,
			Message:   row.Message,
			Data:      data,
		})
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
	var run database.JobRun
	if err := database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").
		Where("id = ?", strings.TrimSpace(entry.RunID)).First(&run).Error; err != nil {
		return err
	}
	data, err := marshalJSON(entry.Data)
	if err != nil {
		return err
	}
	occurredAt := entry.Timestamp
	if occurredAt.IsZero() {
		occurredAt = r.now()
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		sequence := entry.Sequence
		if sequence <= 0 {
			var maxSequence int
			if err := tx.Model(&database.JobRunEvent{}).
				Where("user_id = ? AND job_run_id = ?", userID, run.ID).
				Select("COALESCE(MAX(sequence), 0)").
				Scan(&maxSequence).Error; err != nil {
				return err
			}
			sequence = maxSequence + 1
		}
		row := database.JobRunEvent{
			UUIDModel:  database.UUIDModel{ID: strings.TrimSpace(entry.ID)},
			UserID:     userID,
			JobRunID:   run.ID,
			Sequence:   sequence,
			OccurredAt: occurredAt,
			Type:       entry.Type,
			Message:    entry.Message,
			Data:       data,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		entry.ID = row.ID
		entry.Sequence = sequence
		return nil
	})
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
	var runIDs []string
	if err := database.ScopeByUser(ctx, r.db.WithContext(ctx).Model(&database.JobRun{}), "user_id").
		Where("started_at < ?", cutoff).Pluck("id", &runIDs).Error; err != nil {
		return 0, err
	}
	if len(runIDs) > 0 {
		if err := database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").
			Where("job_run_id IN ?", runIDs).Delete(&database.JobRunEvent{}).Error; err != nil {
			return 0, err
		}
		if err := database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").
			Where("job_run_id IN ?", runIDs).Delete(&database.JobEvent{}).Error; err != nil {
			return 0, err
		}
	}
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
	userID, err := database.RequireUserID(ctx)
	if err != nil {
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
		Where("tags.user_id = ?", userID).
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

func (r *DBRepository) pipelineEnabledByIDTx(ctx context.Context, tx *gorm.DB, userID, id string) (bool, error) {
	var row database.JobPipeline
	if err := tx.WithContext(ctx).Where("user_id = ? AND id = ?", userID, id).First(&row).Error; err != nil {
		return false, err
	}
	return row.Enabled, nil
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
		Order("tool_catalog.mcp_server_id IS NULL ASC").
		Order("tool_catalog.availability_status = 'available' DESC").
		Order("tool_catalog.user_id IS NULL ASC").
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return r.createUnresolvedMCPToolTx(ctx, tx, userID, name)
		}
		return "", err
	}
	return row.ID, nil
}

func (r *DBRepository) createUnresolvedMCPToolTx(ctx context.Context, tx *gorm.DB, userID, name string) (string, error) {
	serverSlug, ok := mcpServerSlugFromToolName(name)
	if !ok {
		return "", fmt.Errorf("tool %q not found", name)
	}
	var server database.MCPServer
	if err := tx.WithContext(ctx).Where("user_id = ? AND slug = ?", userID, serverSlug).First(&server).Error; err != nil {
		return "", fmt.Errorf("tool %q references unknown MCP server %q: %w", name, serverSlug, err)
	}
	now := r.now()
	row := database.ToolCatalog{
		UserID:             &userID,
		MCPServerID:        &server.ID,
		Name:               name,
		DisplayName:        name,
		Origin:             tools.ToolOriginMCPBridge,
		Category:           "mcp:" + serverSlug,
		Class:              "mcp_tool",
		Package:            "mcp:" + serverSlug,
		Risk:               "network",
		AvailabilityStatus: tools.ToolAvailabilityUnavailable,
		AvailabilityReason: "not discovered yet",
		LastUnavailableAt:  &now,
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return "", err
	}
	return row.ID, nil
}

func mcpServerSlugFromToolName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if !strings.HasPrefix(name, "mcp_") {
		return "", false
	}
	rest := strings.TrimPrefix(name, "mcp_")
	serverSlug, _, ok := strings.Cut(rest, "__")
	serverSlug = normalizeSlug(serverSlug)
	return serverSlug, ok && serverSlug != ""
}

func (r *DBRepository) saveTriggersTx(ctx context.Context, tx *gorm.DB, userID, jobID string, triggers []Trigger) error {
	if err := tx.WithContext(ctx).Model(&database.JobTrigger{}).Where("user_id = ? AND job_id = ?", userID, jobID).Update("enabled", false).Error; err != nil {
		return err
	}
	hasManual := false
	for _, trigger := range triggers {
		if trigger.Type == TriggerManual {
			hasManual = true
		}
		if err := r.upsertTriggerTx(ctx, tx, userID, jobID, trigger); err != nil {
			return err
		}
	}
	if !hasManual {
		return r.upsertTriggerTx(ctx, tx, userID, jobID, Trigger{Type: TriggerManual})
	}
	return nil
}

func (r *DBRepository) upsertTriggerTx(ctx context.Context, tx *gorm.DB, userID, jobID string, trigger Trigger) error {
	row, err := triggerDomainToModel(userID, jobID, trigger)
	if err != nil {
		return err
	}
	var existing database.JobTrigger
	err = tx.WithContext(ctx).
		Where("user_id = ? AND job_id = ? AND type = ? AND expression = ? AND config = ?", userID, jobID, row.Type, row.Expression, row.Config).
		First(&existing).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return tx.WithContext(ctx).Create(row).Error
	case err != nil:
		return err
	default:
		return tx.WithContext(ctx).Model(&existing).Updates(map[string]any{
			"enabled":    true,
			"config":     row.Config,
			"updated_at": r.now(),
		}).Error
	}
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

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "unique constraint failed") ||
		strings.Contains(msg, "duplicate key value") ||
		strings.Contains(msg, "sqlstate 23505")
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
	switch {
	case info.Event != "":
		query = query.Where("expression = ?", info.Event)
		if info.When != "" {
			triggerRow, err := triggerDomainToModel(userID, jobID, Trigger{
				Type:   TriggerEvent,
				Listen: info.Event,
				When:   info.When,
			})
			if err != nil {
				return "", err
			}
			query = query.Where("config = ?", triggerRow.Config)
		}
	case info.Expression != "":
		query = query.Where("expression = ?", info.Expression)
		if info.When != "" {
			triggerRow, err := triggerDomainToModel(userID, jobID, Trigger{Type: TriggerCron, Expression: info.Expression, When: info.When})
			if err != nil {
				return "", err
			}
			query = query.Where("config = ?", triggerRow.Config)
		}
	case info.Every != "":
		query = query.Where("expression = ?", info.Every)
		if info.When != "" {
			triggerRow, err := triggerDomainToModel(userID, jobID, Trigger{Type: TriggerInterval, Every: info.Every, When: info.When})
			if err != nil {
				return "", err
			}
			query = query.Where("config = ?", triggerRow.Config)
		}
	case info.Keys != "":
		query = query.Where("expression = ?", info.Keys)
		if info.When != "" {
			triggerRow, err := triggerDomainToModel(userID, jobID, Trigger{Type: TriggerHotkey, Keys: info.Keys, When: info.When})
			if err != nil {
				return "", err
			}
			query = query.Where("config = ?", triggerRow.Config)
		}
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
	pipelineEnabled := true
	if row.Pipeline != nil {
		pipelineSlug = row.Pipeline.Slug
		pipelineEnabled = row.Pipeline.Enabled
	}
	toolName := row.ToolName
	if toolName == "" {
		toolName = row.ToolCatalogID
	}
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
		PipelineEnabled: pipelineEnabled,
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
	row := &database.Job{
		UserID:         userID,
		PipelineID:     pipelineID,
		Slug:           slug,
		Name:           strings.TrimSpace(job.Name),
		Description:    strings.TrimSpace(job.Description),
		Enabled:        job.Enabled,
		ToolCatalogID:  toolCatalogID,
		ToolName:       strings.TrimSpace(job.Tool),
		Inputs:         inputs,
		OutputConfig:   output,
		EventsConfig:   events,
		ErrorPolicy:    policy,
		MaxRunsPerHour: job.MaxRunsPerHour,
		DryRunConfig:   dryRun,
		CreatedBy:      job.Metadata.CreatedBy,
	}
	if ts, err := time.Parse(time.RFC3339, strings.TrimSpace(job.Metadata.CreatedAt)); err == nil && !ts.IsZero() {
		row.CreatedAt = ts
		if strings.TrimSpace(job.Metadata.UpdatedAt) == "" {
			row.UpdatedAt = ts
		}
	}
	if ts, err := time.Parse(time.RFC3339, strings.TrimSpace(job.Metadata.UpdatedAt)); err == nil && !ts.IsZero() {
		row.UpdatedAt = ts
	}
	return row, nil
}

func triggerDomainToModel(userID, jobID string, trigger Trigger) (*database.JobTrigger, error) {
	normalized := normalizeTriggerForStorage(trigger)
	config, err := marshalJSON(normalized)
	if err != nil {
		return nil, err
	}
	expression := triggerExpression(normalized)
	return &database.JobTrigger{
		UserID:     userID,
		JobID:      jobID,
		Type:       string(trigger.Type),
		Enabled:    true,
		Expression: expression,
		Config:     config,
	}, nil
}

func normalizeTriggerForStorage(trigger Trigger) Trigger {
	normalized := Trigger{Type: trigger.Type, When: strings.TrimSpace(trigger.When)}
	switch trigger.Type {
	case TriggerCron:
		normalized.Expression = strings.TrimSpace(trigger.Expression)
	case TriggerInterval:
		normalized.Every = strings.TrimSpace(trigger.Every)
	case TriggerEvent:
		normalized.Listen = strings.TrimSpace(trigger.Listen)
	case TriggerHotkey:
		normalized.Keys = strings.TrimSpace(trigger.Keys)
	case TriggerWebhook:
		normalized.Path = strings.TrimSpace(trigger.Path)
	case TriggerManual:
	default:
		normalized.Expression = strings.TrimSpace(trigger.Expression)
	}
	return normalized
}

func triggerExpression(trigger Trigger) string {
	switch trigger.Type {
	case TriggerCron:
		return strings.TrimSpace(trigger.Expression)
	case TriggerInterval:
		return strings.TrimSpace(trigger.Every)
	case TriggerEvent:
		return strings.TrimSpace(trigger.Listen)
	case TriggerHotkey:
		return strings.TrimSpace(trigger.Keys)
	case TriggerWebhook:
		return strings.TrimSpace(trigger.Path)
	default:
		return strings.TrimSpace(trigger.Expression)
	}
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
	var trigger TriggerInfo
	_ = unmarshalJSON(row.TriggerData, &trigger)
	var inputs map[string]any
	_ = unmarshalJSON(row.Inputs, &inputs)
	var output map[string]any
	_ = unmarshalJSON(row.Output, &output)
	var events []string
	_ = unmarshalJSON(row.EventsEmitted, &events)
	rl := RunLog{
		RunID:          row.ID,
		JobID:          jobSlug,
		ToolName:       row.ToolName,
		Trigger:        trigger,
		Status:         row.Status,
		StartedAt:      row.StartedAt,
		ResolvedInputs: inputs,
		Output:         output,
		OutputSize:     len(row.Output),
		Error:          row.Error,
		RetryCount:     row.RetryCount,
		EventsEmitted:  events,
		IsDryRun:       row.IsDryRun,
		Duration:       time.Duration(row.DurationMs * int64(time.Millisecond)).String(),
		CompletedAt:    time.Time{},
	}
	rl.Replayable = rl.Status != "skipped" && rl.ToolName != "" && rl.ResolvedInputs != nil && !ContainsRedactedValue(rl.ResolvedInputs)
	if row.CompletedAt != nil {
		rl.CompletedAt = *row.CompletedAt
	}
	return rl
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
	if string(b) == "null" {
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
