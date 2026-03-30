package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"assistente/internal/hotkey"
	"assistente/internal/messaging"
	"assistente/internal/tools"

	"gopkg.in/yaml.v3"
)

// SecretStore abstrai acesso a secrets para o job engine.
type SecretStore interface {
	GetSecret(key string) (string, error)
}

// ManagerConfig contem as dependencias externas do Manager.
type ManagerConfig struct {
	BaseDir        string // ~/.assistente/jobs/
	ToolRegistry   *tools.Registry
	HotkeyManager  *hotkey.Manager
	MsgGateway     *messaging.Gateway
	SecretStore    SecretStore
	EmitEvent      func(event string, data any) // Wails EventsEmit

	// Se true, nao inicia watcher (util para testes)
	DisableWatcher bool
}

// Manager orquestra todos os componentes do sistema de jobs.
type Manager struct {
	cfg            ManagerConfig
	registry       *Registry
	eventBus       *EventBus
	scheduler      *Scheduler
	executor       *JobExecutor
	logger         *Logger
	circuitBreaker *CircuitBreaker
	watcher        *Watcher
	hotkeyIDs      map[string][]int // jobID -> hotkey IDs registrados
	mu             sync.Mutex
	started        bool
}

// NewManager cria um Manager com todas as dependencias.
func NewManager(cfg ManagerConfig) *Manager {
	registry := NewRegistry()
	eventBus := NewEventBus()
	logger := NewLogger(cfg.BaseDir)
	circuitBreaker := NewCircuitBreaker()

	m := &Manager{
		cfg:            cfg,
		registry:       registry,
		eventBus:       eventBus,
		logger:         logger,
		circuitBreaker: circuitBreaker,
		hotkeyIDs:      make(map[string][]int),
	}

	// Cria o executor com as dependencias
	m.executor = NewJobExecutor(ExecutorConfig{
		ToolRegistry:   cfg.ToolRegistry,
		EventBus:       eventBus,
		Logger:         logger,
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

// Start carrega jobs do disco, registra triggers e inicia o scheduler e watcher.
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started {
		return nil
	}

	// Garante que o diretorio existe
	if err := os.MkdirAll(m.cfg.BaseDir, 0755); err != nil {
		return fmt.Errorf("create jobs dir: %w", err)
	}

	// Carrega todos os jobs do disco
	jobs, errs := LoadAllFromDir(m.cfg.BaseDir)
	for _, err := range errs {
		log.Printf("[Jobs] Load error: %v", err)
	}

	for _, job := range jobs {
		m.registerJob(job)
	}

	log.Printf("[Jobs] Loaded %d jobs (%d errors)", len(jobs), len(errs))

	// Inicia o scheduler
	m.scheduler.Start()

	// Gera catalogo inicial
	go func() {
		if err := GenerateCatalog(m.cfg.ToolRegistry, m.cfg.BaseDir); err != nil {
			log.Printf("[Jobs] Catalog generation error: %v", err)
		}
	}()

	// Inicia o watcher (em goroutine, pois Start() bloqueia)
	if !m.cfg.DisableWatcher {
		watcher, err := NewWatcher(m.cfg.BaseDir, WatcherCallback{
			OnUpdate: m.onFileChanged,
			OnRemove: m.onFileRemoved,
		})
		if err != nil {
			log.Printf("[Jobs] Watcher init error: %v", err)
		} else {
			m.watcher = watcher
			go func() {
				if err := m.watcher.Start(); err != nil {
					log.Printf("[Jobs] Watcher error: %v", err)
				}
			}()
		}
	}

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

	if m.watcher != nil {
		m.watcher.Stop()
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
			LastRun:     m.logger.GetLastRun(job.ID),
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
	job.LastRun = m.logger.GetLastRun(id)
	return job, nil
}

// ToggleJob ativa ou desativa um job e persiste no YAML.
func (m *Manager) ToggleJob(id string, enabled bool) error {
	job := m.registry.Get(id)
	if job == nil {
		return fmt.Errorf("job not found: %s", id)
	}

	job.Enabled = enabled

	// Persiste a mudanca no arquivo YAML
	if err := m.persistJob(job); err != nil {
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

	ctx := context.Background()
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

	ctx := context.Background()
	trigCtx := &TriggerContext{
		Type:         TriggerManual,
		EventPayload: make(map[string]any),
	}

	result := m.executor.ExecuteDryRun(ctx, job, trigCtx)
	return result, nil
}

// GetJobRun retorna um run log especifico pelo jobID e runID.
func (m *Manager) GetJobRun(jobID, runID string) (*RunLog, error) {
	return m.logger.GetRun(jobID, runID)
}

// GetJobRuns retorna o historico de execucoes de um job.
func (m *Manager) GetJobRuns(id string, limit int) ([]RunLog, error) {
	return m.logger.GetRuns(id, limit)
}

// GetJobEvents retorna a timeline de eventos de uma data (formato "2006-01-02").
func (m *Manager) GetJobEvents(date string) ([]EventEntry, error) {
	return m.logger.GetEvents(date)
}

// GetPipelines retorna os pipelines com seus jobs.
func (m *Manager) GetPipelines() []PipelineInfo {
	grouped := m.registry.GetByPipeline()
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
				LastRun:  m.logger.GetLastRun(job.ID),
			})
		}
		pipelines = append(pipelines, PipelineInfo{
			Name: name,
			Jobs: infos,
		})
	}

	return pipelines
}

// GetToolCatalog retorna o catalogo de tools.
func (m *Manager) GetToolCatalog() ([]CatalogEntry, error) {
	entries, err := GetCatalogEntries(m.cfg.BaseDir)
	if err != nil {
		// Catalogo nao existe ou esta corrompido -- regenera ao vivo
		if genErr := GenerateCatalog(m.cfg.ToolRegistry, m.cfg.BaseDir); genErr != nil {
			return nil, fmt.Errorf("generate catalog: %w", genErr)
		}
		return GetCatalogEntries(m.cfg.BaseDir)
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

		// 2. Disk-based last run (de sessões anteriores)
		if diskRun := m.logger.GetLastRun(job.ID); diskRun != nil && len(diskRun.Output) > 0 {
			log.Printf("[Jobs] InferEventSchema(%q): found disk output from job %s", eventName, job.ID)
			return diskRun.Output
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
// Valida, persiste no disco e registra no runtime.
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

	job.FilePath = filepath.Join(m.cfg.BaseDir, job.ID+".yaml")

	if err := m.persistJob(job); err != nil {
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

// DeleteJob remove um job do disco e do runtime.
func (m *Manager) DeleteJob(id string) error {
	job := m.registry.Get(id)
	if job == nil {
		return fmt.Errorf("job not found: %s", id)
	}

	// Remove do disco
	if job.FilePath != "" {
		if err := os.Remove(job.FilePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete job file: %w", err)
		}
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
			if eventData == nil { return nil }
			keys := make([]string, 0, len(eventData))
			for k := range eventData { keys = append(keys, k) }
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
	return GenerateCatalog(m.cfg.ToolRegistry, m.cfg.BaseDir)
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
		ctx := context.Background()
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
	// Busca a versao mais atual do registry (pode ter sido atualizada via hot reload)
	current := m.registry.Get(job.ID)
	if current == nil || !current.Enabled {
		return
	}

	m.executor.Execute(ctx, current, trigCtx)
}

func (m *Manager) persistJob(job *Job) error {
	if job.FilePath == "" {
		job.FilePath = filepath.Join(m.cfg.BaseDir, job.ID+".yaml")
	}

	data, err := marshalJobYAML(job)
	if err != nil {
		return err
	}

	return os.WriteFile(job.FilePath, data, 0644)
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
		"job_id":  jobID,
		"run_id":  runLog.RunID,
		"status":  runLog.Status,
		"error":   runLog.Error,
	})
}

func (m *Manager) onFileChanged(path string, job *Job) {
	existing := m.registry.Get(job.ID)
	if existing != nil {
		m.unregisterTriggers(existing)
	}

	m.registerJob(job)

	m.emitEvent("jobs:updated", map[string]any{
		"id":   job.ID,
		"name": job.Name,
	})
}

func (m *Manager) onFileRemoved(path string, jobID string) {
	m.unregisterJob(jobID)

	m.emitEvent("jobs:removed", map[string]any{
		"id": jobID,
	})
}

func (m *Manager) emitEvent(event string, data any) {
	if m.cfg.EmitEvent != nil {
		m.cfg.EmitEvent(event, data)
	}
}

// marshalJobYAML serializa um job para YAML, excluindo campos runtime.
func marshalJobYAML(job *Job) ([]byte, error) {
	persistable := struct {
		ID          string         `yaml:"id"`
		Name        string         `yaml:"name"`
		Description string         `yaml:"description,omitempty"`
		Enabled     bool           `yaml:"enabled"`
		Pipeline    string         `yaml:"pipeline,omitempty"`
		Tags        []string       `yaml:"tags,omitempty"`
		Triggers    []Trigger      `yaml:"triggers"`
		Tool        string         `yaml:"tool"`
		Inputs      map[string]any `yaml:"inputs,omitempty"`
		Output      OutputConfig   `yaml:"output,omitempty"`
		Events      EventsConfig   `yaml:"events,omitempty"`
		ErrorPolicy ErrorPolicy    `yaml:"error_policy,omitempty"`
		DryRun      DryRunConfig   `yaml:"dry_run,omitempty"`
		Metadata    Metadata       `yaml:"metadata,omitempty"`
	}{
		ID:          job.ID,
		Name:        job.Name,
		Description: job.Description,
		Enabled:     job.Enabled,
		Pipeline:    job.Pipeline,
		Tags:        job.Tags,
		Triggers:    job.Triggers,
		Tool:        job.Tool,
		Inputs:      job.Inputs,
		Output:      job.Output,
		Events:      job.Events,
		ErrorPolicy: job.ErrorPolicy,
		DryRun:      job.DryRun,
		Metadata:    job.Metadata,
	}

	return yaml.Marshal(persistable)
}
