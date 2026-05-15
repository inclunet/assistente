package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"assistente/internal/toolinvocations"
	"assistente/internal/tools"

	"github.com/google/uuid"
)

// JobExecutor executa jobs chamando tools do registry.
type JobExecutor struct {
	toolRegistry    *tools.Registry
	toolInvocations *toolinvocations.Service
	eventBus        *EventBus
	repository      Repository
	circuitBreaker  *CircuitBreaker
	secretResolver  SecretResolver
	notifyFunc      NotifyFunc

	// Callback emitido no inicio/fim de cada run (para atualizar UI)
	onRunStart func(jobID string, runID string)
	onRunEnd   func(jobID string, runLog *RunLog)
}

// NotifyFunc envia notificacao para canais (chat, telegram, etc.)
type NotifyFunc func(channels []string, message string)

// ExecutorConfig configura o JobExecutor.
type ExecutorConfig struct {
	ToolRegistry    *tools.Registry
	ToolInvocations *toolinvocations.Service
	EventBus        *EventBus
	Repository      Repository
	CircuitBreaker  *CircuitBreaker
	SecretResolver  SecretResolver
	NotifyFunc      NotifyFunc
	OnRunStart      func(jobID string, runID string)
	OnRunEnd        func(jobID string, runLog *RunLog)
}

// NewJobExecutor cria um executor com as dependencias fornecidas.
func NewJobExecutor(cfg ExecutorConfig) *JobExecutor {
	return &JobExecutor{
		toolRegistry:    cfg.ToolRegistry,
		toolInvocations: cfg.ToolInvocations,
		eventBus:        cfg.EventBus,
		repository:      cfg.Repository,
		circuitBreaker:  cfg.CircuitBreaker,
		secretResolver:  cfg.SecretResolver,
		notifyFunc:      cfg.NotifyFunc,
		onRunStart:      cfg.OnRunStart,
		onRunEnd:        cfg.OnRunEnd,
	}
}

// TriggerContext carrega informacoes do trigger que disparou a execucao.
type TriggerContext struct {
	Type         TriggerType
	EventName    string
	Expression   string
	Every        string
	Keys         string
	When         string
	EventPayload map[string]any
	ChainID      string   // ID da cadeia (para circuit breaker)
	ChainHistory []string // jobs ja executados nesta cadeia
}

// Execute executa um job: resolve inputs, chama a tool, processa output, emite eventos.
// Respeita error_policy com retry/backoff.
func (e *JobExecutor) Execute(ctx context.Context, job *Job, trigCtx *TriggerContext) *RunLog {
	runUUID, err := uuid.NewV7()
	if err != nil {
		runUUID = uuid.New()
	}
	runID := "run_" + runUUID.String()

	rl := &RunLog{
		RunID: runID,
		JobID: job.ID,
		Trigger: TriggerInfo{
			Type:       trigCtx.Type,
			At:         time.Now(),
			Event:      trigCtx.EventName,
			Expression: trigCtx.Expression,
			Every:      trigCtx.Every,
			Keys:       trigCtx.Keys,
			When:       trigCtx.When,
		},
		StartedAt: time.Now(),
	}

	if e.onRunStart != nil {
		e.onRunStart(job.ID, runID)
	}

	defer func() {
		rl.CompletedAt = time.Now()
		rl.Duration = rl.CompletedAt.Sub(rl.StartedAt).String()
		rl.addTerminalRunEvent(job.ID)
		rl.Replayable = rl.Status != "skipped" && rl.ToolName != "" && rl.ResolvedInputs != nil && !ContainsRedactedValue(rl.ResolvedInputs)

		if e.repository != nil {
			persistCtx := context.WithoutCancel(ctx)
			if err := e.repository.LogRun(persistCtx, rl); err != nil {
				log.Printf("[Jobs] Error logging run: %v", err)
			}
		} else {
			log.Printf("[Jobs] Error logging run: repository not configured")
		}

		if e.onRunEnd != nil {
			e.onRunEnd(job.ID, rl)
		}
	}()

	rl.addRunEvent("triggered", fmt.Sprintf("[%s] -> %s TRIGGERED", trigCtx.Type, job.ID), nil)

	// Circuit breaker: rate limit
	if err := e.circuitBreaker.CheckRateLimit(job.ID, job.MaxRunsPerHour); err != nil {
		rl.Status = "failed"
		rl.Error = err.Error()
		e.emitFailure(ctx, job, rl, trigCtx)
		return rl
	}

	// Circuit breaker: chain depth (baseado no tamanho do historico de jobs na cadeia)
	if trigCtx.ChainID != "" {
		chainDepth := len(trigCtx.ChainHistory)
		maxDepth := e.circuitBreaker.MaxChainDepth()
		if chainDepth >= maxDepth {
			err := fmt.Errorf("circuit breaker: chain %q exceeded max depth (%d)", trigCtx.ChainID, maxDepth)
			rl.Status = "failed"
			rl.Error = err.Error()
			e.emitFailure(ctx, job, rl, trigCtx)
			return rl
		}

		if err := e.circuitBreaker.DetectLoop(trigCtx.ChainHistory, job.ID); err != nil {
			rl.Status = "failed"
			rl.Error = err.Error()
			e.emitFailure(ctx, job, rl, trigCtx)
			return rl
		}
	}

	e.circuitBreaker.RecordRun(job.ID)

	// Dry run: retorna mock output sem executar a tool
	if job.DryRun.Enabled {
		rl.IsDryRun = true
		rl.Output = job.DryRun.MockOutput
		rl.Status = "completed"
		rl.OutputSize = estimateSize(job.DryRun.MockOutput)
		e.emitSuccess(ctx, job, rl, trigCtx)
		return rl
	}

	// Execucao com retry
	maxAttempts := 1
	if job.ErrorPolicy.Strategy == ErrorRetry && job.ErrorPolicy.MaxRetries > 0 {
		maxAttempts = job.ErrorPolicy.MaxRetries + 1
	}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			delay := e.calculateRetryDelay(job, attempt)
			log.Printf("[Jobs] %s: retry %d/%d after %s", job.ID, attempt, job.ErrorPolicy.MaxRetries, delay)
			rl.RetryCount = attempt

			select {
			case <-ctx.Done():
				rl.Status = "failed"
				rl.Error = "cancelled during retry"
				e.emitFailure(ctx, job, rl, trigCtx)
				return rl
			case <-time.After(delay):
			}
		}

		output, err := e.executeSingle(ctx, job, trigCtx, rl)
		if err == nil {
			rl.Output = output
			rl.OutputSize = estimateSize(output)
			rl.Status = "completed"
			e.emitSuccess(ctx, job, rl, trigCtx)
			return rl
		}

		lastErr = err
		log.Printf("[Jobs] %s: attempt %d failed: %v", job.ID, attempt+1, err)
	}

	// Todos os retries falharam
	rl.Status = "failed"
	rl.Error = lastErr.Error()

	if job.ErrorPolicy.Strategy == ErrorSkip {
		rl.Status = "skipped"
	}

	// Notificar se on_exhausted = notify
	if job.ErrorPolicy.OnExhausted == OnExhaustedNotify && e.notifyFunc != nil {
		channels := job.ErrorPolicy.NotifyChannels
		if len(channels) == 0 {
			channels = []string{"chat"}
		}
		e.notifyFunc(channels, fmt.Sprintf("Job %q falhou apos %d tentativas: %s", job.ID, maxAttempts, lastErr))
	}

	e.emitFailure(ctx, job, rl, trigCtx)
	return rl
}

// ExecuteDryRun executa um job em modo dry run, ignorando a flag do YAML.
func (e *JobExecutor) ExecuteDryRun(ctx context.Context, job *Job, trigCtx *TriggerContext) *DryRunResult {
	if job.DryRun.MockOutput != nil {
		return &DryRunResult{
			Success: true,
			Output:  job.DryRun.MockOutput,
		}
	}

	// Sem mock: executa a tool de verdade mas nao emite eventos
	output, err := e.executeSingle(ctx, job, trigCtx, nil)
	if err != nil {
		return &DryRunResult{
			Success: false,
			Error:   err.Error(),
		}
	}
	return &DryRunResult{
		Success: true,
		Output:  output,
	}
}

func (e *JobExecutor) executeSingle(ctx context.Context, job *Job, trigCtx *TriggerContext, rl *RunLog) (map[string]any, error) {
	// Resolve a tool no registry
	tool, ok := e.toolRegistry.Get(job.Tool)
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", job.Tool)
	}

	// Monta contexto de template
	tmplCtx := &TemplateContext{
		Event:   trigCtx.EventPayload,
		Secrets: e.secretResolver,
		Now:     time.Now(),
	}

	// Resolve templates nos inputs
	resolvedInputs, err := ResolveInputs(job.Inputs, tmplCtx)
	if err != nil {
		return nil, fmt.Errorf("resolve inputs: %w", err)
	}

	resolvedInputs = CoerceInputs(resolvedInputs, tool.Parameters())

	if rl != nil {
		rl.ToolName = job.Tool
		rl.ResolvedInputs = RedactResolvedInputs(job.Inputs, resolvedInputs)
	}

	// Serializa inputs para JSON (formato esperado por tool.Execute)
	argsJSON, err := json.Marshal(resolvedInputs)
	if err != nil {
		return nil, fmt.Errorf("marshal inputs: %w", err)
	}

	result, err := e.executeTool(ctx, job, rl, argsJSON)
	if err != nil {
		return nil, err
	}

	// Parse o resultado para map[string]any
	output := make(map[string]any)
	if result.Content != "" {
		if err := json.Unmarshal([]byte(result.Content), &output); err != nil {
			// Conteudo nao e JSON object — tenta como array
			var arr []any
			if arrErr := json.Unmarshal([]byte(result.Content), &arr); arrErr == nil {
				output["content"] = arr
				log.Printf("[Jobs] Output parsed as array with %d elements", len(arr))
			} else {
				output["content"] = result.Content
				log.Printf("[Jobs] Output stored as raw string (len=%d)", len(result.Content))
			}
		} else {
			log.Printf("[Jobs] Output parsed as object with keys: %v", func() []string {
				keys := make([]string, 0, len(output))
				for k := range output {
					keys = append(keys, k)
				}
				return keys
			}())
		}
	}
	if result.Metadata != nil {
		for k, v := range result.Metadata {
			output["_meta_"+k] = v
		}
	}

	// Aplica output map (transformacoes)
	if len(job.Output.Map) > 0 {
		tmplCtx.Output = output
		mapped, err := ResolveOutputMap(job.Output.Map, tmplCtx)
		if err != nil {
			return nil, fmt.Errorf("resolve output map: %w", err)
		}
		return mapped, nil
	}

	return output, nil
}

func (e *JobExecutor) executeTool(ctx context.Context, job *Job, rl *RunLog, argsJSON json.RawMessage) (tools.ToolResult, error) {
	if e.toolInvocations == nil {
		tool, ok := e.toolRegistry.Get(job.Tool)
		if !ok {
			return tools.ToolResult{}, fmt.Errorf("tool not found: %s", job.Tool)
		}
		result, err := tool.Execute(ctx, argsJSON)
		if err != nil {
			return tools.ToolResult{}, fmt.Errorf("tool execute: %w", err)
		}
		if result.IsError {
			return tools.ToolResult{}, fmt.Errorf("tool error: %s", result.Content)
		}
		return result, nil
	}

	callID := fmt.Sprintf("job_%s_%d", job.ID, time.Now().UnixNano())
	originID := job.ID
	if rl != nil {
		callID = fmt.Sprintf("%s_tool", rl.RunID)
		originID = rl.RunID
	}
	result := e.toolInvocations.Execute(ctx, toolinvocations.ExecuteRequest{
		Call: tools.ToolCall{
			ID:   callID,
			Type: "function",
			Function: tools.FunctionCall{
				Name:      job.Tool,
				Arguments: string(argsJSON),
			},
		},
		Origin: toolinvocations.Origin{
			Type: toolinvocations.OriginJobRun,
			ID:   originID,
		},
		DryRun: rl == nil,
	}).Execution
	if result.Error != nil {
		return tools.ToolResult{}, fmt.Errorf("tool execute: %w", result.Error)
	}
	if result.Result.IsError {
		return tools.ToolResult{}, fmt.Errorf("tool error: %s", result.Result.Content)
	}
	return result.Result, nil
}

func (e *JobExecutor) emitSuccess(ctx context.Context, job *Job, rl *RunLog, trigCtx *TriggerContext) {
	if job.Events.OnSuccess == "" {
		return
	}

	chainID := trigCtx.ChainID
	if chainID == "" {
		chainID = rl.RunID
	}
	chainHistory := append(trigCtx.ChainHistory, job.ID)

	// Fan-out: se for_each aponta para um campo que é array, emite um evento por item
	if job.Events.ForEach != "" {
		items := resolveForEachItems(rl.Output, job.Events.ForEach)
		if len(items) > 0 {
			emitted := 0
			for i, item := range items {
				itemPayload := make(map[string]any)
				if m, ok := item.(map[string]any); ok {
					for k, v := range m {
						itemPayload[k] = v
					}
				} else {
					itemPayload["content"] = item
				}
				itemPayload["_fan_out_index"] = i
				itemPayload["_fan_out_total"] = len(items)

				// Per-item emit_when filter
				if !e.checkEmitWhen(job, itemPayload, trigCtx) {
					continue
				}

				itemPayload = e.applyPayloadTemplate(job, itemPayload, trigCtx)
				filtered := e.buildEventPayload(job, itemPayload)
				enriched := make(map[string]any, len(filtered)+2)
				for k, v := range filtered {
					enriched[k] = v
				}
				enriched["_chain_id"] = chainID
				enriched["_chain_history"] = chainHistory

				e.eventBus.Publish(ctx, job.Events.OnSuccess, enriched)
				emitted++
			}

			if emitted > 0 {
				rl.addDomainEvent("event_emitted", job.ID, job.Events.OnSuccess,
					fmt.Sprintf("[%s] -> emitted %q x%d/%d (fan-out on %q)", job.ID, job.Events.OnSuccess, emitted, len(items), job.Events.ForEach), nil)
				rl.EventsEmitted = append(rl.EventsEmitted, fmt.Sprintf("%s x%d", job.Events.OnSuccess, emitted))
			} else {
				log.Printf("[Jobs] %s: all %d fan-out items filtered by emit_when", job.ID, len(items))
			}
			return
		}
		log.Printf("[Jobs] %s: for_each %q did not resolve to array, emitting single event", job.ID, job.Events.ForEach)
	}

	// Single event: emit_when against full output
	if !e.checkEmitWhen(job, rl.Output, trigCtx) {
		log.Printf("[Jobs] %s: emit_when condition not met, skipping event %q", job.ID, job.Events.OnSuccess)
		return
	}

	output := e.applyPayloadTemplate(job, rl.Output, trigCtx)
	payload := e.buildEventPayload(job, output)

	rl.addDomainEvent("event_emitted", job.ID, job.Events.OnSuccess,
		fmt.Sprintf("[%s] -> emitted %q", job.ID, job.Events.OnSuccess), nil)

	rl.EventsEmitted = append(rl.EventsEmitted, job.Events.OnSuccess)

	enriched := make(map[string]any, len(payload)+2)
	for k, v := range payload {
		enriched[k] = v
	}
	enriched["_chain_id"] = chainID
	enriched["_chain_history"] = chainHistory

	e.eventBus.Publish(ctx, job.Events.OnSuccess, enriched)
}

// resolveForEachItems navega o output usando um path separado por pontos e retorna o array.
// Ex: "results" -> output["results"], "data.items" -> output["data"]["items"]
func resolveForEachItems(output map[string]any, path string) []any {
	parts := splitDotPath(path)
	var current any = output

	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = m[part]
		if !ok {
			return nil
		}
	}

	arr, ok := current.([]any)
	if !ok {
		return nil
	}
	return arr
}

func splitDotPath(path string) []string {
	var parts []string
	current := ""
	for _, c := range path {
		if c == '.' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

func (e *JobExecutor) emitFailure(ctx context.Context, job *Job, rl *RunLog, trigCtx *TriggerContext) {
	eventType := rl.Status
	if eventType == "" {
		eventType = "failed"
	}
	rl.addRunEvent(eventType, fmt.Sprintf("[%s] %s: %s", job.ID, strings.ToUpper(eventType), rl.Error), nil)

	if job.Events.OnFailure == "" {
		return
	}

	payload := map[string]any{
		"error":  rl.Error,
		"job_id": job.ID,
		"run_id": rl.RunID,
	}

	rl.EventsEmitted = append(rl.EventsEmitted, job.Events.OnFailure)

	rl.addDomainEvent("event_emitted", job.ID, job.Events.OnFailure,
		fmt.Sprintf("[%s] -> emitted %q", job.ID, job.Events.OnFailure), nil)

	e.eventBus.Publish(ctx, job.Events.OnFailure, payload)
}

// checkEmitWhen evaluates the emit_when condition against the given data.
// Returns true if the event should be emitted (condition met or not set).
// Both .output (tool result or fan-out item) and .event (trigger payload) are available.
func (e *JobExecutor) checkEmitWhen(job *Job, data map[string]any, trigCtx *TriggerContext) bool {
	if job.Events.EmitWhen == "" {
		return true
	}
	ok, err := EvaluateCondition(job.Events.EmitWhen, &TemplateContext{
		Event:  trigCtx.EventPayload,
		Output: data,
		Now:    time.Now(),
	})
	if err != nil {
		log.Printf("[Jobs] %s: emit_when eval error: %v", job.ID, err)
		return false
	}
	return ok
}

// applyPayloadTemplate renderiza o PayloadTemplate contra o output, retornando
// o resultado parseado como map. Se o template não está definido ou falha,
// retorna o output original. Both .output and .event are available.
func (e *JobExecutor) applyPayloadTemplate(job *Job, output map[string]any, trigCtx *TriggerContext) map[string]any {
	if job.Events.PayloadTemplate == "" {
		return output
	}

	tmplCtx := &TemplateContext{
		Event:  trigCtx.EventPayload,
		Output: output,
		Now:    time.Now(),
	}

	rendered, err := resolveTemplate(job.Events.PayloadTemplate, tmplCtx)
	if err != nil {
		log.Printf("[Jobs] %s: payload_template render error: %v", job.ID, err)
		return output
	}

	renderedStr, ok := rendered.(string)
	if !ok {
		return output
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(renderedStr), &result); err != nil {
		log.Printf("[Jobs] %s: payload_template JSON parse error: %v", job.ID, err)
		return output
	}
	return result
}

func (e *JobExecutor) buildEventPayload(job *Job, output map[string]any) map[string]any {
	if job.Events.PayloadFilter == nil {
		return output
	}

	if len(job.Events.PayloadFilter.Include) > 0 {
		filtered := make(map[string]any, len(job.Events.PayloadFilter.Include))
		for _, key := range job.Events.PayloadFilter.Include {
			if val, ok := output[key]; ok {
				filtered[key] = val
			}
		}
		return filtered
	}

	if len(job.Events.PayloadFilter.Exclude) > 0 {
		filtered := make(map[string]any, len(output))
		for k, v := range output {
			filtered[k] = v
		}
		for _, key := range job.Events.PayloadFilter.Exclude {
			delete(filtered, key)
		}
		return filtered
	}

	return output
}

func (rl *RunLog) addDomainEvent(eventType, jobID, eventName, message string, data map[string]any) {
	if rl == nil {
		return
	}
	rl.DomainEvents = append(rl.DomainEvents, EventEntry{
		Timestamp: time.Now(),
		Type:      eventType,
		JobID:     jobID,
		RunID:     rl.RunID,
		Event:     eventName,
		Message:   message,
		Data:      data,
	})
}

func (rl *RunLog) addRunEvent(eventType, message string, data map[string]any) {
	if rl == nil {
		return
	}
	rl.RunEvents = append(rl.RunEvents, RunEvent{
		RunID:     rl.RunID,
		Sequence:  len(rl.RunEvents) + 1,
		Timestamp: time.Now(),
		Type:      eventType,
		Message:   message,
		Data:      data,
	})
}

func (rl *RunLog) addTerminalRunEvent(jobID string) {
	if rl == nil || rl.Status == "" {
		return
	}
	for _, event := range rl.RunEvents {
		if event.Type == "completed" || event.Type == "failed" || event.Type == "skipped" {
			return
		}
	}
	status := rl.Status
	message := fmt.Sprintf("[%s] %s", jobID, strings.ToUpper(status))
	if rl.Error != "" {
		message += ": " + rl.Error
	}
	rl.addRunEvent(status, message, nil)
}

func (e *JobExecutor) calculateRetryDelay(job *Job, attempt int) time.Duration {
	baseDelay := 30 * time.Second
	if job.ErrorPolicy.RetryDelay != "" {
		if d, err := parseInterval(job.ErrorPolicy.RetryDelay); err == nil {
			baseDelay = d
		}
	}

	switch job.ErrorPolicy.Backoff {
	case BackoffExponential:
		return time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt-1)))
	case BackoffLinear:
		return baseDelay * time.Duration(attempt)
	default: // fixed
		return baseDelay
	}
}

func estimateSize(v map[string]any) int {
	if v == nil {
		return 0
	}
	b, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return len(b)
}
