package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"assistente/internal/hotkey"
	"assistente/internal/messaging"
	"assistente/internal/tools"
)

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

	ctx := m.context()
	jobs, err := m.cfg.Repository.ListJobs(ctx, JobFilter{})
	if err != nil {
		return fmt.Errorf("load jobs from database: %w", err)
	}
	m.registry = NewRegistry()
	for _, job := range jobs {
		jobCopy := job
		m.registerJob(&jobCopy)
	}

	log.Printf("[Jobs] Loaded %d jobs from database", len(jobs))

	// Inicia o scheduler
	m.scheduler.Start()

	m.started = true
	log.Printf("[Jobs] Manager started")
	return nil
}

// Stop para todos os componentes.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started {
		return
	}

	m.scheduler.Stop()
	m.eventBus.Close()
	m.unregisterAllHotkeys()

	m.started = false
	log.Printf("[Jobs] Manager stopped")
}

// --- Metodos publicos para UI/Wails ---

// GetJobs retorna info resumida de todos os jobs.
func (m *Manager) GetJobs() []JobInfo {
	jobs := m.registry.GetAll()
	infos := make([]JobInfo, 0, len(jobs))

	for _, job := range jobs {
		lastRun, _ := m.lastRun(job.ID)
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
			LastRun:     lastRun,
		}
		infos = append(infos, info)
	}

	return infos
}

// GetJob retorna detalhes completos de um job.
func (m *Manager) GetJob(id string) (*Job, error) {
	job := m.registry.Get(id)
	if job == nil {
		return nil, fmt.Errorf("job not found: %s", id)
	}
	job.LastRun, _ = m.lastRun(id)
	return job, nil
}

// ToggleJob ativa ou desativa um job e persiste no YAML.
func (m *Manager) ToggleJob(id string, enabled bool) error {
	job := m.registry.Get(id)
	if job == nil {
		return fmt.Errorf("job not found: %s", id)
	}

	job.Enabled = enabled

	if err := m.cfg.Repository.SaveJob(m.context(), job); err != nil {
		return fmt.Errorf("persist toggle: %w", err)
	}

	// Re-registra triggers
	if enabled {
		m.registerTriggers(job)
	} else {
		m.unregisterTriggers(job)
	}

	m.emitEvent("jobs:toggled", map[string]any{
		"id":      id,
		"enabled": enabled,
	})

	return nil
}

// RunJob executa um job manualmente.
func (m *Manager) RunJob(id string) (*RunLog, error) {
	job := m.registry.Get(id)
	if job == nil {
		return nil, fmt.Errorf("job not found: %s", id)
	}

	ctx := m.context()
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
		return nil, fmt.Errorf("job not found: %s", id)
	}

	ctx := m.context()
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

// GetJobRuns retorna o historico de execucoes de um job.
func (m *Manager) GetJobRuns(id string, limit int) ([]RunLog, error) {
	return m.cfg.Repository.GetRuns(m.context(), id, limit)
}

// GetJobEvents retorna a timeline de eventos de uma data (formato "2006-01-02").
func (m *Manager) GetJobEvents(date string) ([]EventEntry, error) {
	events, err := m.cfg.Repository.ListEvents(m.context(), EventFilter{Limit: 500})
	if err != nil || date == "" {
		return events, err
	}
	filtered := make([]EventEntry, 0, len(events))
	for _, event := range events {
		if event.Timestamp.Format("2006-01-02") == date {
			filtered = append(filtered, event)
		}
	}
	return filtered, nil
}

// GetPipelines retorna os pipelines com seus jobs.
func (m *Manager) GetPipelines() []PipelineInfo {
	grouped := m.registry.GetByPipeline()
	var pipelines []PipelineInfo

	for name, jobs := range grouped {
		infos := make([]JobInfo, 0, len(jobs))
		for _, job := range jobs {
			lastRun, _ := m.lastRun(job.ID)
			infos = append(infos, JobInfo{
				ID:       job.ID,
				Name:     job.Name,
				Enabled:  job.Enabled,
				Tool:     job.Tool,
				Status:   job.Status,
				Triggers: job.Triggers,
				LastRun:  lastRun,
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

func (m *Manager) SavePipeline(pipeline *Pipeline) error {
	return m.cfg.Repository.SavePipeline(m.context(), pipeline)
}

func (m *Manager) DeletePipeline(slug string) error {
	return m.cfg.Repository.DeletePipeline(m.context(), slug)
}

// GetToolCatalog retorna o catalogo de tools.
func (m *Manager) GetToolCatalog() ([]CatalogEntry, error) {
	if m.cfg.ToolRegistry == nil {
		return nil, fmt.Errorf("tool registry not configured")
	}
	names := m.cfg.ToolRegistry.Names()
	entries := make([]CatalogEntry, 0, len(names))
	for _, name := range names {
		tool, ok := m.cfg.ToolRegistry.Get(name)
		if !ok {
			continue
		}
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
// 2. LastRun no disco (sessões anteriores)
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

		// 2. Output.Schema persistido (salvo a partir de test output no builder)
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

	// Desregistra versao anterior se existia
	if existing := m.registry.Get(job.ID); existing != nil {
		m.unregisterTriggers(existing)
	}

	m.registerJob(job)

	m.emitEvent("jobs:updated", map[string]any{
		"id":   job.ID,
		"name": job.Name,
	})

	return nil
}

// DeleteJob remove um job do banco e do runtime.
func (m *Manager) DeleteJob(id string) error {
	job := m.registry.Get(id)
	if job == nil {
		return fmt.Errorf("job not found: %s", id)
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

// TestTool executa uma tool diretamente com inputs fornecidos, sem precisar de um job salvo.
// Util para testar no builder antes de salvar.
func (m *Manager) TestTool(toolName string, inputs map[string]any, eventData map[string]any) (*TestToolResult, error) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

// RegenerateCatalog forca regeneracao do catalogo.
func (m *Manager) RegenerateCatalog() error {
	// O catálogo de jobs agora é derivado do registry em tempo real.
	return nil
}

// --- Metodos internos ---

func (m *Manager) registerJob(job *Job) {
	m.registry.Set(job)
	m.registerTriggers(job)
	log.Printf("[Jobs] Registered: %s (enabled=%v)", job.ID, job.Enabled)
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
	if !job.Enabled {
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
					EventPayload: cleanPayload,
					ChainID:      chainID,
					ChainHistory: chainHistory,
				}

				m.executor.Execute(ctx, &jobCopy, trigCtx)
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
			EventPayload: make(map[string]any),
		}
		m.executor.Execute(ctx, &jobCopy, trigCtx)
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

func (m *Manager) executeJob(ctx context.Context, job *Job, trigCtx *TriggerContext) {
	_ = ctx // Scheduler/event callbacks podem fornecer ctx sem user_id; o Manager usa o provider autenticado.
	// Busca a versao mais atual do registry (pode ter sido atualizada via hot reload)
	current := m.registry.Get(job.ID)
	if current == nil || !current.Enabled {
		return
	}

	m.executor.Execute(m.context(), current, trigCtx)
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

func (m *Manager) context() context.Context {
	if m.cfg.ContextProvider != nil {
		if ctx := m.cfg.ContextProvider(); ctx != nil {
			return ctx
		}
	}
	return context.Background()
}

func (m *Manager) lastRun(jobID string) (*RunLog, error) {
	if m.cfg.Repository == nil {
		return nil, nil
	}
	runs, err := m.cfg.Repository.GetRuns(m.context(), jobID, 1)
	if err != nil || len(runs) == 0 {
		return nil, err
	}
	return &runs[0], nil
}
