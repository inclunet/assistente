package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"assistente/internal/database"
	"assistente/internal/hotkey"
	"assistente/internal/messaging"
	"assistente/internal/tools"
)

var ErrJobNotFound = errors.New("job not found")

// SecretStore abstrai acesso a secrets para o job engine.
type SecretStore interface {
	GetSecret(key string) (string, error)
}

// ManagerConfig contem as dependencias externas do Manager.
type ManagerConfig struct {
	BaseDir         string // Diretório legado usado apenas como fonte da importação inicial.
	Repository      Repository
	ContextProvider func() context.Context
	ToolRegistry    *tools.Registry
	HotkeyManager   *hotkey.Manager
	MsgGateway      *messaging.Gateway
	SecretStore     SecretStore
	EmitEvent       func(event string, data any) // Wails EventsEmit
}

// Manager orquestra todos os componentes do sistema de jobs.
type Manager struct {
	cfg            ManagerConfig
	registry       *Registry
	eventBus       *EventBus
	scheduler      *Scheduler
	executor       *JobExecutor
	circuitBreaker *CircuitBreaker
	hotkeyIDs      map[string][]int // jobID -> hotkey IDs registrados
	retentionStop  chan struct{}
	mu             sync.Mutex
	started        bool
}

// NewManager cria um Manager com todas as dependencias.
func NewManager(cfg ManagerConfig) *Manager {
	registry := NewRegistry()
	eventBus := NewEventBus()
	circuitBreaker := NewCircuitBreaker()

	m := &Manager{
		cfg:            cfg,
		registry:       registry,
		eventBus:       eventBus,
		circuitBreaker: circuitBreaker,
		hotkeyIDs:      make(map[string][]int),
	}

	// Cria o executor com as dependencias
	m.executor = NewJobExecutor(ExecutorConfig{
		ToolRegistry:   cfg.ToolRegistry,
		EventBus:       eventBus,
		Repository:     cfg.Repository,
		CircuitBreaker: circuitBreaker,
		SecretResolver: m.resolveSecret,
		NotifyFunc:     m.notifyChannels,
		OnRunStart:     m.onRunStart,
		OnRunEnd:       m.onRunEnd,
	})

	// Cria o scheduler com a funcao de execucao
	m.scheduler = NewScheduler(m.executeJob)

	return m
}

// Start carrega jobs do banco, registra triggers e inicia o scheduler.
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started {
		return nil
	}

	if m.cfg.Repository == nil {
		return fmt.Errorf("jobs repository not configured")
	}
	if m.eventBus == nil || m.eventBus.closed {
		m.eventBus = NewEventBus()
		m.executor.eventBus = m.eventBus
	}

	ctx := m.context()
	jobs, err := m.cfg.Repository.ListJobs(ctx, JobFilter{})
	if err != nil {
		return fmt.Errorf("load jobs from database: %w", err)
	}
	loaded := make([]*Job, 0, len(jobs))
	for _, job := range jobs {
		jobCopy := job
		loaded = append(loaded, &jobCopy)
	}
	m.registry.Replace(loaded)
	for _, job := range loaded {
		m.registerTriggers(job)
		log.Printf("[Jobs] Registered: %s (enabled=%v pipeline_enabled=%v)", job.ID, job.Enabled, job.PipelineEnabled)
	}

	log.Printf("[Jobs] Loaded %d jobs from database", len(jobs))

	// Inicia o scheduler
	m.scheduler.Start()
	m.runRetention(ctx)
	m.startRetentionLoop(ctx)

	m.started = true
	log.Printf("[Jobs] Manager started")
	return nil
}

// Stop para todos os componentes.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started {
		m.registry.Clear()
		m.circuitBreaker.Reset()
		return
	}

	if m.retentionStop != nil {
		close(m.retentionStop)
		m.retentionStop = nil
	}
	m.scheduler.Stop()
	m.eventBus.Close()
	m.unregisterAllHotkeys()
	m.registry.Clear()
	m.circuitBreaker.Reset()

	m.started = false
	log.Printf("[Jobs] Manager stopped")
}

// --- Metodos publicos para UI/Wails ---

// GetJobs retorna info resumida de todos os jobs.
func (m *Manager) GetJobs() []JobInfo {
	jobs := m.registry.GetAll()
	infos := make([]JobInfo, 0, len(jobs))
	lastRuns := m.lastRuns(jobs)

	for _, job := range jobs {
		info := JobInfo{
			ID:          job.ID,
			Name:        job.Name,
			Description: job.Description,
			Enabled:     job.Enabled,
			Pipeline:    job.Pipeline,
			Tags:        job.Tags,
			Tool:        job.Tool,
			Status:      job.Status,
			Triggers:    job.Triggers,
			LastRun:     lastRuns[job.ID],
		}
		infos = append(infos, info)
	}

	return infos
}

func (m *Manager) GetJobsContext(ctx context.Context) ([]JobInfo, error) {
	ctx, err := m.scopedContext(ctx)
	if err != nil {
		return nil, err
	}
	jobs := m.registry.GetAll()
	infos := make([]JobInfo, 0, len(jobs))
	lastRuns := m.lastRunsWithContext(ctx, jobs)

	for _, job := range jobs {
		info := JobInfo{
			ID:          job.ID,
			Name:        job.Name,
			Description: job.Description,
			Enabled:     job.Enabled,
			Pipeline:    job.Pipeline,
			Tags:        job.Tags,
			Tool:        job.Tool,
			Status:      job.Status,
			Triggers:    job.Triggers,
			LastRun:     lastRuns[job.ID],
		}
		infos = append(infos, info)
	}

	return infos, nil
}

// GetJob retorna detalhes completos de um job.
func (m *Manager) GetJob(id string) (*Job, error) {
	job := m.registry.Get(id)
	if job == nil {
		return nil, fmt.Errorf("%w: %s", ErrJobNotFound, id)
	}
	copy, err := cloneJob(job)
	if err != nil {
		return nil, err
	}
	copy.LastRun, _ = m.lastRun(id)
	return copy, nil
}

func (m *Manager) GetJobContext(ctx context.Context, id string) (*Job, error) {
	ctx, err := m.scopedContext(ctx)
	if err != nil {
		return nil, err
	}
	job := m.registry.Get(id)
	if job == nil {
		return nil, fmt.Errorf("%w: %s", ErrJobNotFound, id)
	}
	copy, err := cloneJob(job)
	if err != nil {
		return nil, err
	}
	copy.LastRun, _ = m.lastRunWithContext(ctx, id)
	return copy, nil
}

// ToggleJob ativa ou desativa um job e persiste no YAML.
func (m *Manager) ToggleJob(id string, enabled bool) error {
	job := m.registry.Get(id)
	if job == nil {
		return fmt.Errorf("%w: %s", ErrJobNotFound, id)
	}

	updated := *job
	updated.Enabled = enabled
	if err := m.cfg.Repository.SaveJob(m.context(), &updated); err != nil {
		return fmt.Errorf("persist toggle: %w", err)
	}

	m.unregisterTriggers(job)
	m.registry.Set(&updated)
	if m.effectiveJobEnabled(&updated) {
		m.registerTriggers(&updated)
	} else {
		m.unregisterTriggers(&updated)
	}

	m.emitEvent("jobs:toggled", map[string]any{
		"id":      id,
		"enabled": enabled,
	})

	return nil
}

func (m *Manager) ToggleJobContext(ctx context.Context, id string, enabled bool) error {
	ctx, err := m.scopedContext(ctx)
	if err != nil {
		return err
	}
	job := m.registry.Get(id)
	if job == nil {
		return fmt.Errorf("%w: %s", ErrJobNotFound, id)
	}
	updated := *job
	updated.Enabled = enabled
	if err := m.cfg.Repository.SaveJob(ctx, &updated); err != nil {
		return fmt.Errorf("persist toggle: %w", err)
	}
	m.unregisterTriggers(job)
	m.registry.Set(&updated)
	if m.effectiveJobEnabled(&updated) {
		m.registerTriggers(&updated)
	} else {
		m.unregisterTriggers(&updated)
	}
	m.emitEvent("jobs:toggled", map[string]any{"id": id, "enabled": enabled})
	return nil
}

// RunJob executa um job manualmente.
func (m *Manager) RunJob(id string) (*RunLog, error) {
	job := m.registry.Get(id)
	if job == nil {
		return nil, fmt.Errorf("%w: %s", ErrJobNotFound, id)
	}

	ctx := m.context()
	trigCtx := &TriggerContext{
		Type:         TriggerManual,
		EventPayload: make(map[string]any),
	}

	rl := m.executor.Execute(ctx, job, trigCtx)
	return rl, nil
}

func (m *Manager) RunJobContext(ctx context.Context, id string) (*RunLog, error) {
	ctx, err := m.scopedContext(ctx)
	if err != nil {
		return nil, err
	}
	job := m.registry.Get(id)
	if job == nil {
		return nil, fmt.Errorf("%w: %s", ErrJobNotFound, id)
	}
	trigCtx := &TriggerContext{
		Type:         TriggerManual,
		EventPayload: make(map[string]any),
	}
	rl := m.executor.Execute(ctx, job, trigCtx)
	return rl, nil
}

// DryRunJob executa um dry run de um job.
func (m *Manager) DryRunJob(id string) (*DryRunResult, error) {
	job := m.registry.Get(id)
	if job == nil {
		return nil, fmt.Errorf("%w: %s", ErrJobNotFound, id)
	}

	ctx := m.context()
	trigCtx := &TriggerContext{
		Type:         TriggerManual,
		EventPayload: make(map[string]any),
	}

	result := m.executor.ExecuteDryRun(ctx, job, trigCtx)
	return result, nil
}

func (m *Manager) DryRunJobContext(ctx context.Context, id string) (*DryRunResult, error) {
	ctx, err := m.scopedContext(ctx)
	if err != nil {
		return nil, err
	}
	job := m.registry.Get(id)
	if job == nil {
		return nil, fmt.Errorf("%w: %s", ErrJobNotFound, id)
	}
	trigCtx := &TriggerContext{
		Type:         TriggerManual,
		EventPayload: make(map[string]any),
	}
	result := m.executor.ExecuteDryRun(ctx, job, trigCtx)
	return result, nil
}

// GetJobRun retorna um run log especifico pelo jobID e runID.
func (m *Manager) GetJobRun(jobID, runID string) (*RunLog, error) {
	return m.cfg.Repository.GetRun(m.context(), jobID, runID)
}

func (m *Manager) GetJobRunContext(ctx context.Context, jobID, runID string) (*RunLog, error) {
	ctx, err := m.scopedContext(ctx)
	if err != nil {
		return nil, err
	}
	return m.cfg.Repository.GetRun(ctx, jobID, runID)
}

// GetJobRuns retorna o historico de execucoes de um job.
func (m *Manager) GetJobRuns(id string, limit int) ([]RunLog, error) {
	return m.cfg.Repository.GetRuns(m.context(), id, limit)
}

func (m *Manager) GetJobRunsContext(ctx context.Context, id string, limit int) ([]RunLog, error) {
	ctx, err := m.scopedContext(ctx)
	if err != nil {
		return nil, err
	}
	return m.cfg.Repository.GetRuns(ctx, id, limit)
}

// GetJobEvents retorna a timeline de eventos de uma data (formato "2006-01-02").
func (m *Manager) GetJobEvents(date string) ([]EventEntry, error) {
	return m.GetJobEventsContext(m.context(), date)
}

func (m *Manager) GetJobEventsContext(ctx context.Context, date string) ([]EventEntry, error) {
	ctx, err := m.scopedContext(ctx)
	if err != nil {
		return nil, err
	}
	filter := EventFilter{}
	if date != "" {
		start, err := time.ParseInLocation("2006-01-02", date, time.Local)
		if err != nil {
			return nil, fmt.Errorf("invalid date %q: %w", date, err)
		}
		filter.StartAt = start
		filter.EndAt = start.Add(24 * time.Hour)
	}
	return m.cfg.Repository.ListEvents(ctx, filter)
}

func (m *Manager) GetJobEventsPage(date string, limit, offset int) ([]EventEntry, error) {
	return m.GetJobEventsPageContext(m.context(), date, limit, offset)
}

func (m *Manager) GetJobEventsPageContext(ctx context.Context, date string, limit, offset int) ([]EventEntry, error) {
	ctx, err := m.scopedContext(ctx)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 500
	}
	if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}
	filter := EventFilter{Limit: limit, Offset: offset}
	if date != "" {
		start, err := time.ParseInLocation("2006-01-02", date, time.Local)
		if err != nil {
			return nil, fmt.Errorf("invalid date %q: %w", date, err)
		}
		filter.StartAt = start
		filter.EndAt = start.Add(24 * time.Hour)
	}
	return m.cfg.Repository.ListEvents(ctx, filter)
}

// GetPipelines retorna os pipelines com seus jobs.
func (m *Manager) GetPipelines() []PipelineInfo {
	grouped := m.registry.GetByPipeline()
	allJobs := m.registry.GetAll()
	lastRuns := m.lastRuns(allJobs)
	var pipelines []PipelineInfo

	for name, jobs := range grouped {
		infos := make([]JobInfo, 0, len(jobs))
		for _, job := range jobs {
			infos = append(infos, JobInfo{
				ID:       job.ID,
				Name:     job.Name,
				Enabled:  job.Enabled,
				Tool:     job.Tool,
				Status:   job.Status,
				Triggers: job.Triggers,
				LastRun:  lastRuns[job.ID],
			})
		}
		pipelines = append(pipelines, PipelineInfo{
			Name: name,
			Jobs: infos,
		})
	}

	return pipelines
}

func (m *Manager) ListPipelines() ([]Pipeline, error) {
	return m.cfg.Repository.ListPipelines(m.context())
}

func (m *Manager) ListPipelinesContext(ctx context.Context) ([]Pipeline, error) {
	ctx, err := m.scopedContext(ctx)
	if err != nil {
		return nil, err
	}
	return m.cfg.Repository.ListPipelines(ctx)
}

func (m *Manager) SavePipeline(pipeline *Pipeline) error {
	if err := m.cfg.Repository.SavePipeline(m.context(), pipeline); err != nil {
		return err
	}
	m.applyPipelineState(pipeline.Slug, pipeline.Enabled)
	return nil
}

func (m *Manager) SavePipelineContext(ctx context.Context, pipeline *Pipeline) error {
	ctx, err := m.scopedContext(ctx)
	if err != nil {
		return err
	}
	if err := m.cfg.Repository.SavePipeline(ctx, pipeline); err != nil {
		return err
	}
	m.applyPipelineState(pipeline.Slug, pipeline.Enabled)
	return nil
}

func (m *Manager) DeletePipeline(slug string) error {
	slug = normalizeSlug(slug)
	if err := m.cfg.Repository.DeletePipeline(m.context(), slug); err != nil {
		return err
	}
	m.clearPipelineFromRegistry(slug)
	return nil
}

func (m *Manager) DeletePipelineContext(ctx context.Context, slug string) error {
	ctx, err := m.scopedContext(ctx)
	if err != nil {
		return err
	}
	slug = normalizeSlug(slug)
	if err := m.cfg.Repository.DeletePipeline(ctx, slug); err != nil {
		return err
	}
	m.clearPipelineFromRegistry(slug)
	return nil
}

// GetToolCatalog retorna o catalogo de tools.
func (m *Manager) GetToolCatalog() ([]CatalogEntry, error) {
	if m.cfg.ToolRegistry == nil {
		return nil, fmt.Errorf("tool registry not configured")
	}
	registryTools := m.cfg.ToolRegistry.All()
	entries := make([]CatalogEntry, 0, len(registryTools))
	for _, tool := range registryTools {
		source := "internal"
		if strings.HasPrefix(tool.Name(), "mcp_") {
			source = "mcp"
		}
		entries = append(entries, CatalogEntry{
			Name:        tool.Name(),
			Description: tool.Description(),
			Schema:      tool.Parameters(),
			Source:      source,
		})
	}
	return entries, nil
}

// InferEventSchema tenta inferir o schema de um evento a partir dos jobs existentes.
// Procura jobs que emitem o evento e retorna dados na ordem:
// 1. LastRun em memória (sessão atual)
// 2. Último run persistido no banco
// 3. Output.Schema persistido (salvo via builder)
func (m *Manager) InferEventSchema(eventName string) map[string]any {
	if eventName == "" {
		return nil
	}

	for _, job := range m.registry.GetAll() {
		if job.Events.OnSuccess != eventName && job.Events.OnFailure != eventName {
			continue
		}

		// 1. In-memory last run (atualizado durante a sessão pelo onRunEnd)
		if job.LastRun != nil && len(job.LastRun.Output) > 0 {
			log.Printf("[Jobs] InferEventSchema(%q): found in-memory output from job %s", eventName, job.ID)
			return job.LastRun.Output
		}

		if lastRun, err := m.lastRun(job.ID); err == nil && lastRun != nil && len(lastRun.Output) > 0 {
			log.Printf("[Jobs] InferEventSchema(%q): found persisted output from job %s", eventName, job.ID)
			return lastRun.Output
		}

		// 3. Output.Schema persistido (salvo a partir de test output no builder)
		if len(job.Output.Schema) > 0 {
			var schema map[string]any
			if err := json.Unmarshal(job.Output.Schema, &schema); err == nil {
				log.Printf("[Jobs] InferEventSchema(%q): found output schema from job %s", eventName, job.ID)
				return schema
			}
		}

		log.Printf("[Jobs] InferEventSchema(%q): job %s emits this event but has no output data", eventName, job.ID)
	}

	log.Printf("[Jobs] InferEventSchema(%q): no emitting job found", eventName)
	return nil
}

// ListKnownEvents retorna todos os nomes de eventos unicos referenciados pelos jobs.
func (m *Manager) ListKnownEvents() []string {
	seen := make(map[string]bool)
	for _, job := range m.registry.GetAll() {
		if job.Events.OnSuccess != "" {
			seen[job.Events.OnSuccess] = true
		}
		if job.Events.OnFailure != "" {
			seen[job.Events.OnFailure] = true
		}
		for _, t := range job.Triggers {
			if t.Type == TriggerEvent && t.Listen != "" {
				seen[t.Listen] = true
			}
		}
	}
	result := make([]string, 0, len(seen))
	for k := range seen {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}

// SaveJob cria ou atualiza um job a partir de dados do frontend.
// Valida, persiste no banco e registra no runtime.
func (m *Manager) SaveJob(job *Job) error {
	if err := Validate(job); err != nil {
		return err
	}

	// Define metadata se novo
	if job.Metadata.CreatedAt == "" {
		job.Metadata.CreatedAt = time.Now().Format(time.RFC3339)
		job.Metadata.CreatedBy = "ui"
	} else {
		job.Metadata.UpdatedAt = time.Now().Format(time.RFC3339)
	}

	if err := m.cfg.Repository.SaveJob(m.context(), job); err != nil {
		return fmt.Errorf("save job: %w", err)
	}
	saved, err := m.cfg.Repository.GetJob(m.context(), job.ID)
	if err != nil {
		return fmt.Errorf("reload saved job: %w", err)
	}

	// Desregistra versao anterior se existia
	if existing := m.registry.Get(job.ID); existing != nil {
		m.unregisterTriggers(existing)
	}

	m.registerJob(saved)

	m.emitEvent("jobs:updated", map[string]any{
		"id":   job.ID,
		"name": job.Name,
	})

	return nil
}

func (m *Manager) SaveJobContext(ctx context.Context, job *Job) error {
	ctx, err := m.scopedContext(ctx)
	if err != nil {
		return err
	}
	if err := Validate(job); err != nil {
		return err
	}

	if job.Metadata.CreatedAt == "" {
		job.Metadata.CreatedAt = time.Now().Format(time.RFC3339)
		job.Metadata.CreatedBy = "ui"
	} else {
		job.Metadata.UpdatedAt = time.Now().Format(time.RFC3339)
	}

	if err := m.cfg.Repository.SaveJob(ctx, job); err != nil {
		return fmt.Errorf("save job: %w", err)
	}
	saved, err := m.cfg.Repository.GetJob(ctx, job.ID)
	if err != nil {
		return fmt.Errorf("reload saved job: %w", err)
	}

	if existing := m.registry.Get(job.ID); existing != nil {
		m.unregisterTriggers(existing)
	}

	m.registerJob(saved)
	m.emitEvent("jobs:updated", map[string]any{"id": job.ID, "name": job.Name})
	return nil
}

// DeleteJob remove um job do banco e do runtime.
func (m *Manager) DeleteJob(id string) error {
	job := m.registry.Get(id)
	if job == nil {
		return fmt.Errorf("%w: %s", ErrJobNotFound, id)
	}

	if err := m.cfg.Repository.DeleteJob(m.context(), id); err != nil {
		return fmt.Errorf("delete job: %w", err)
	}

	m.unregisterJob(id)

	m.emitEvent("jobs:removed", map[string]any{
		"id": id,
	})

	return nil
}

func (m *Manager) DeleteJobContext(ctx context.Context, id string) error {
	ctx, err := m.scopedContext(ctx)
	if err != nil {
		return err
	}
	job := m.registry.Get(id)
	if job == nil {
		return fmt.Errorf("%w: %s", ErrJobNotFound, id)
	}

	if err := m.cfg.Repository.DeleteJob(ctx, id); err != nil {
		return fmt.Errorf("delete job: %w", err)
	}

	m.unregisterJob(id)
	m.emitEvent("jobs:removed", map[string]any{"id": id})
	return nil
}

// TestTool executa uma tool diretamente com inputs fornecidos, sem precisar de um job salvo.
// Util para testar no builder antes de salvar.
func (m *Manager) TestTool(toolName string, inputs map[string]any, eventData map[string]any) (*TestToolResult, error) {
	return m.TestToolContext(m.context(), toolName, inputs, eventData)
}

func (m *Manager) TestToolContext(parent context.Context, toolName string, inputs map[string]any, eventData map[string]any) (*TestToolResult, error) {
	execCtx := m.contextFrom(parent)
	tool, ok := m.cfg.ToolRegistry.Get(toolName)
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", toolName)
	}

	log.Printf("[Jobs] TestTool(%q): inputs=%v, eventData keys=%v, eventData nil=%v",
		toolName, inputs, func() []string {
			if eventData == nil {
				return nil
			}
			keys := make([]string, 0, len(eventData))
			for k := range eventData {
				keys = append(keys, k)
			}
			return keys
		}(), eventData == nil)

	if eventData != nil {
		if c, ok := eventData["content"]; ok {
			log.Printf("[Jobs] TestTool: eventData.content type=%T", c)
		}
		ctx := &TemplateContext{
			Event: eventData,
			Now:   time.Now(),
		}
		resolved, err := ResolveInputs(inputs, ctx)
		if err != nil {
			return nil, fmt.Errorf("resolve templates: %w", err)
		}
		log.Printf("[Jobs] TestTool: resolved inputs=%v", resolved)
		inputs = resolved
	}

	inputs = CoerceInputs(inputs, tool.Parameters())

	argsJSON, err := json.Marshal(inputs)
	if err != nil {
		return nil, fmt.Errorf("marshal inputs: %w", err)
	}

	ctx, cancel := context.WithTimeout(execCtx, 30*time.Second)
	defer cancel()

	start := time.Now()
	result, err := tool.Execute(ctx, argsJSON)
	duration := time.Since(start)

	if err != nil {
		return &TestToolResult{
			Success:  false,
			Error:    err.Error(),
			Duration: duration.String(),
		}, nil
	}

	if result.IsError {
		return &TestToolResult{
			Success:  false,
			Error:    result.Content,
			Duration: duration.String(),
		}, nil
	}

	output := make(map[string]any)
	if result.Content != "" {
		if jsonErr := json.Unmarshal([]byte(result.Content), &output); jsonErr != nil {
			var arr []any
			if arrErr := json.Unmarshal([]byte(result.Content), &arr); arrErr == nil {
				output["content"] = arr
			} else {
				output["content"] = result.Content
			}
		}
	}
	if result.Metadata != nil {
		for k, v := range result.Metadata {
			output["_meta_"+k] = v
		}
	}

	return &TestToolResult{
		Success:  true,
		Output:   output,
		Duration: duration.String(),
	}, nil
}

// RegenerateCatalog é mantido para compatibilidade com a UI antiga.
// O catálogo de jobs é derivado ao vivo do registry de tools, então não há
// artefato persistente a regenerar.
func (m *Manager) RegenerateCatalog() error {
	return nil
}

// --- Metodos internos ---

func (m *Manager) registerJob(job *Job) {
	m.registry.Set(job)
	m.registerTriggers(job)
	log.Printf("[Jobs] Registered: %s (enabled=%v pipeline_enabled=%v)", job.ID, job.Enabled, job.PipelineEnabled)
}

func (m *Manager) unregisterJob(jobID string) {
	job := m.registry.Get(jobID)
	if job != nil {
		m.unregisterTriggers(job)
	}
	m.registry.Remove(jobID)
	log.Printf("[Jobs] Unregistered: %s", jobID)
}

func (m *Manager) registerTriggers(job *Job) {
	if !m.effectiveJobEnabled(job) {
		return
	}

	// Schedule cron/interval
	if err := m.scheduler.Schedule(job); err != nil {
		log.Printf("[Jobs] Schedule error for %s: %v", job.ID, err)
	}

	// Register event listeners
	for _, t := range job.Triggers {
		if t.Type == TriggerEvent && t.Listen != "" {
			jobCopy := *job
			triggerWhen := t.When
			m.eventBus.Subscribe(t.Listen, job.ID, func(ctx context.Context, eventName string, payload map[string]any) {
				// Extrai chain context do payload
				chainID, _ := payload["_chain_id"].(string)
				chainHistory, _ := payload["_chain_history"].([]string)

				// Remove metadados de chain do payload visivel
				cleanPayload := make(map[string]any, len(payload))
				for k, v := range payload {
					if k != "_chain_id" && k != "_chain_history" {
						cleanPayload[k] = v
					}
				}

				// Evaluate trigger condition before executing
				if triggerWhen != "" {
					ok, err := EvaluateCondition(triggerWhen, &TemplateContext{
						Event: cleanPayload,
						Now:   time.Now(),
					})
					if err != nil {
						log.Printf("[Jobs] %s: trigger when eval error: %v", jobCopy.ID, err)
						return
					}
					if !ok {
						log.Printf("[Jobs] %s: trigger when condition not met, skipping", jobCopy.ID)
						return
					}
				}

				trigCtx := &TriggerContext{
					Type:         TriggerEvent,
					EventName:    eventName,
					Expression:   eventName,
					When:         triggerWhen,
					EventPayload: cleanPayload,
					ChainID:      chainID,
					ChainHistory: chainHistory,
				}

				m.executeJob(ctx, &jobCopy, trigCtx)
			})
		}

		// Register hotkeys
		if t.Type == TriggerHotkey && t.Keys != "" {
			m.registerJobHotkey(job, t.Keys)
		}
	}
}

func (m *Manager) unregisterTriggers(job *Job) {
	m.scheduler.Unschedule(job.ID)
	m.eventBus.UnsubscribeAll(job.ID)
	m.unregisterJobHotkeys(job.ID)
}

func (m *Manager) registerJobHotkey(job *Job, keys string) {
	if m.cfg.HotkeyManager == nil {
		return
	}

	modifiers, key, err := hotkey.ParseCombination(keys)
	if err != nil {
		log.Printf("[Jobs] Hotkey parse error for %s (%s): %v", job.ID, keys, err)
		return
	}

	jobCopy := *job
	id, err := m.cfg.HotkeyManager.Register(modifiers, key, func() {
		ctx := m.context()
		trigCtx := &TriggerContext{
			Type:         TriggerHotkey,
			Keys:         keys,
			EventPayload: make(map[string]any),
		}
		m.executeJob(ctx, &jobCopy, trigCtx)
	})
	if err != nil {
		log.Printf("[Jobs] Hotkey register error for %s (%s): %v", job.ID, keys, err)
		return
	}

	m.hotkeyIDs[job.ID] = append(m.hotkeyIDs[job.ID], id)
	log.Printf("[Jobs] Hotkey registered for %s: %s", job.ID, keys)
}

func (m *Manager) unregisterJobHotkeys(jobID string) {
	if m.cfg.HotkeyManager == nil {
		return
	}

	for _, id := range m.hotkeyIDs[jobID] {
		if err := m.cfg.HotkeyManager.Unregister(id); err != nil {
			log.Printf("[Jobs] Hotkey unregister error for %s (id=%d): %v", jobID, id, err)
		}
	}
	delete(m.hotkeyIDs, jobID)
}

func (m *Manager) unregisterAllHotkeys() {
	if m.cfg.HotkeyManager == nil {
		return
	}

	for jobID := range m.hotkeyIDs {
		m.unregisterJobHotkeys(jobID)
	}
}

func (m *Manager) effectiveJobEnabled(job *Job) bool {
	if job == nil || !job.Enabled {
		return false
	}
	return normalizeSlug(job.Pipeline) == "" || job.PipelineEnabled
}

func (m *Manager) clearPipelineFromRegistry(slug string) {
	slug = normalizeSlug(slug)
	if slug == "" {
		return
	}
	for _, job := range m.registry.GetAll() {
		if normalizeSlug(job.Pipeline) != slug {
			continue
		}
		updated := *job
		updated.Pipeline = ""
		updated.PipelineEnabled = true
		m.unregisterTriggers(job)
		m.registry.Set(&updated)
		if m.effectiveJobEnabled(&updated) {
			m.registerTriggers(&updated)
		}
	}
}

func (m *Manager) applyPipelineState(slug string, enabled bool) {
	slug = normalizeSlug(slug)
	if slug == "" {
		return
	}
	for _, job := range m.registry.GetAll() {
		if normalizeSlug(job.Pipeline) != slug {
			continue
		}
		m.unregisterTriggers(job)
		updated := *job
		updated.PipelineEnabled = enabled
		m.registry.Set(&updated)
		if m.effectiveJobEnabled(&updated) {
			m.registerTriggers(&updated)
		}
	}
}

func (m *Manager) executeJob(ctx context.Context, job *Job, trigCtx *TriggerContext) {
	// Busca a versao mais atual do registry (pode ter sido atualizada via hot reload)
	current := m.registry.Get(job.ID)
	if current == nil || !m.effectiveJobEnabled(current) {
		return
	}

	ctx, err := m.scopedContext(ctx)
	if err != nil {
		log.Printf("[Jobs] %s: authenticated context required: %v", current.ID, err)
		return
	}
	m.executor.Execute(ctx, current, trigCtx)
}

const (
	jobRetentionAge      = 30 * 24 * time.Hour
	jobRetentionInterval = 24 * time.Hour
)

func (m *Manager) runRetention(ctx context.Context) {
	if m.cfg.Repository == nil {
		return
	}
	if deleted, err := m.cfg.Repository.CleanOldRunEvents(ctx, jobRetentionAge); err != nil {
		log.Printf("[Jobs] retention run events failed: %v", err)
	} else if deleted > 0 {
		log.Printf("[Jobs] retention removed %d run event(s)", deleted)
	}
	if deleted, err := m.cfg.Repository.CleanOldEvents(ctx, jobRetentionAge); err != nil {
		log.Printf("[Jobs] retention events failed: %v", err)
	} else if deleted > 0 {
		log.Printf("[Jobs] retention removed %d event(s)", deleted)
	}
	if deleted, err := m.cfg.Repository.CleanOldRuns(ctx, jobRetentionAge); err != nil {
		log.Printf("[Jobs] retention runs failed: %v", err)
	} else if deleted > 0 {
		log.Printf("[Jobs] retention removed %d run(s)", deleted)
	}
}

func (m *Manager) startRetentionLoop(ctx context.Context) {
	if m.retentionStop != nil {
		return
	}
	stop := make(chan struct{})
	m.retentionStop = stop
	go func() {
		ticker := time.NewTicker(jobRetentionInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m.runRetention(ctx)
			case <-stop:
				return
			}
		}
	}()
}

func (m *Manager) resolveSecret(key string) (string, error) {
	if m.cfg.SecretStore == nil {
		return "", fmt.Errorf("no secret store configured")
	}
	return m.cfg.SecretStore.GetSecret(key)
}

func (m *Manager) notifyChannels(channels []string, message string) {
	if m.cfg.MsgGateway == nil {
		log.Printf("[Jobs] Notify (no gateway): %s", message)
		return
	}

	for _, ch := range channels {
		if ch == "chat" {
			m.emitEvent("jobs:notification", map[string]any{"message": message})
			continue
		}

		messenger, ok := m.cfg.MsgGateway.GetMessenger(ch)
		if !ok {
			log.Printf("[Jobs] Notify: channel %q not available", ch)
			continue
		}

		if err := messenger.Send(context.Background(), messaging.OutgoingMessage{
			Text: message,
		}); err != nil {
			log.Printf("[Jobs] Notify error on %s: %v", ch, err)
		}
	}
}

func (m *Manager) onRunStart(jobID string, runID string) {
	if job := m.registry.Get(jobID); job != nil {
		job.Status = JobStatusRunning
	}

	m.emitEvent("jobs:run_start", map[string]any{
		"job_id": jobID,
		"run_id": runID,
	})
}

func (m *Manager) onRunEnd(jobID string, runLog *RunLog) {
	if job := m.registry.Get(jobID); job != nil {
		if runLog.Status == "failed" {
			job.Status = JobStatusError
		} else {
			job.Status = JobStatusIdle
		}
		job.LastRun = runLog
	}

	m.emitEvent("jobs:run_end", map[string]any{
		"job_id": jobID,
		"run_id": runLog.RunID,
		"status": runLog.Status,
		"error":  runLog.Error,
	})
}

func (m *Manager) emitEvent(event string, data any) {
	if m.cfg.EmitEvent != nil {
		m.cfg.EmitEvent(event, data)
	}
}

func cloneJob(job *Job) (*Job, error) {
	if job == nil {
		return nil, nil
	}
	raw, err := json.Marshal(job)
	if err != nil {
		return nil, fmt.Errorf("clone job %s: %w", job.ID, err)
	}
	var copy Job
	if err := json.Unmarshal(raw, &copy); err != nil {
		return nil, fmt.Errorf("clone job %s: %w", job.ID, err)
	}
	copy.PipelineEnabled = job.PipelineEnabled
	return &copy, nil
}

func (m *Manager) context() context.Context {
	if m.cfg.ContextProvider != nil {
		if ctx := m.cfg.ContextProvider(); ctx != nil {
			return ctx
		}
	}
	return context.Background()
}

func (m *Manager) contextFrom(parent context.Context) context.Context {
	if parent == nil {
		parent = context.Background()
	}
	if _, ok := database.UserIDFromContext(parent); ok {
		return parent
	}
	base := m.context()
	userID, ok := database.UserIDFromContext(base)
	if !ok {
		return parent
	}
	return database.WithUserID(parent, userID)
}

func (m *Manager) scopedContext(parent context.Context) (context.Context, error) {
	ctx := m.contextFrom(parent)
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	return ctx, nil
}

func (m *Manager) lastRun(jobID string) (*RunLog, error) {
	if m.cfg.Repository == nil {
		return nil, nil
	}
	return m.lastRunWithContext(m.context(), jobID)
}

func (m *Manager) lastRunWithContext(ctx context.Context, jobID string) (*RunLog, error) {
	if m.cfg.Repository == nil {
		return nil, nil
	}
	runs, err := m.cfg.Repository.GetRuns(ctx, jobID, 1)
	if err != nil || len(runs) == 0 {
		return nil, err
	}
	return &runs[0], nil
}

func (m *Manager) lastRuns(jobs []*Job) map[string]*RunLog {
	return m.lastRunsWithContext(m.context(), jobs)
}

func (m *Manager) lastRunsWithContext(ctx context.Context, jobs []*Job) map[string]*RunLog {
	out := make(map[string]*RunLog, len(jobs))
	if m.cfg.Repository == nil || len(jobs) == 0 {
		return out
	}
	ids := make([]string, 0, len(jobs))
	for _, job := range jobs {
		if job != nil {
			ids = append(ids, job.ID)
		}
	}
	runs, err := m.cfg.Repository.GetLastRuns(ctx, ids)
	if err != nil {
		log.Printf("[Jobs] Error loading last runs: %v", err)
		return out
	}
	return runs
}
