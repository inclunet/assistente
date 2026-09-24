package toolinvocations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"assistente/internal/tools"
)

type Service struct {
	repo     Repository
	executor *tools.Executor
	now      func() time.Time
	metrics  *Metrics

	// persistOpTimeout limita o tempo de cada operação síncrona de persistência.
	// A persistência deve sobreviver a cancelamento do usuário, mas não deve
	// travar indefinidamente em locks/IO de DB.
	persistOpTimeout time.Duration

	// persistMaxResultSize limita o output armazenado em tool_invocations,
	// independente do limite usado para a execução (que pode ser maior).
	persistMaxResultSize int

	// persistMaxErrorSize limita error_message, que pode vir de ToolResult.Content
	// (especialmente quando IsError=true sem error Go) e pode ser muito grande.
	persistMaxErrorSize int

	// persistMaxInputSize limita o input persistido (tool_invocations.input).
	// Sem isso, argumentos muito grandes podem crescer a tabela sem limite.
	persistMaxInputSize int
}

func NewService(repo Repository, executor *tools.Executor, configuredMetrics ...*Metrics) *Service {
	metrics := RuntimeMetrics()
	if len(configuredMetrics) > 0 && configuredMetrics[0] != nil {
		metrics = configuredMetrics[0]
	}
	return &Service{
		repo:                 repo,
		executor:             executor,
		now:                  time.Now,
		metrics:              metrics,
		persistOpTimeout:     3 * time.Second,
		persistMaxResultSize: tools.DefaultMaxResultSize,
		// Mantém o mesmo limite de persistência do Output para consistência.
		persistMaxErrorSize: tools.DefaultMaxResultSize,
		persistMaxInputSize: tools.DefaultMaxResultSize,
	}
}

func (s *Service) recordPersistenceFailure() {
	if s != nil {
		s.metrics.IncPersistenceFailure()
	}
}

func (s *Service) persistCtx(parent context.Context) context.Context {
	return context.WithoutCancel(parent)
}

func (s *Service) persistOpCtx(parent context.Context) (context.Context, context.CancelFunc) {
	if s == nil || s.persistOpTimeout <= 0 {
		return context.WithTimeout(parent, 3*time.Second)
	}
	return context.WithTimeout(parent, s.persistOpTimeout)
}

func (s *Service) CanPersist() bool {
	return s != nil && s.repo != nil
}

// CleanOldDryRuns remove invocações dry-run operacionais (job_run/tool_catalog)
// mais antigas que maxAge (AEP-0074).
func (s *Service) CleanOldDryRuns(ctx context.Context, maxAge time.Duration) (int, error) {
	if s == nil || s.repo == nil {
		return 0, nil
	}
	return s.repo.CleanOldDryRuns(ctx, maxAge)
}

// CleanOldChat remove invocações de chat mais antigas que maxAge (cap de idade
// OPCIONAL; só chamar quando o usuário configurar um limite explícito).
func (s *Service) CleanOldChat(ctx context.Context, maxAge time.Duration) (int, error) {
	if s == nil || s.repo == nil {
		return 0, nil
	}
	return s.repo.CleanOldChat(ctx, maxAge)
}

// CleanOrphanChat remove invocações de chat sem turno/mensagem de origem.
func (s *Service) CleanOrphanChat(ctx context.Context) (int, error) {
	if s == nil || s.repo == nil {
		return 0, nil
	}
	return s.repo.CleanOrphanChat(ctx)
}

func cancelledLedgerUnavailable(call tools.ToolCall) ExecuteResult {
	return ExecuteResult{
		Execution: executionCancelled(call, "Execução cancelada: não foi possível validar o item do chat ou persistir sua tool invocation"),
		Persisted: false,
	}
}

func (s *Service) Execute(ctx context.Context, req ExecuteRequest) ExecuteResult {
	if s == nil || s.executor == nil {
		return ExecuteResult{Execution: executionError(req.Call, "tool invocation service not configured")}
	}
	start := invocationStart{
		call: req.Call, persistedArguments: req.PersistedArguments, origin: req.Origin,
		parentID: req.ParentInvocationID, catalogID: req.ToolCatalogID,
		dryRun: req.DryRun, iteration: req.Iteration,
	}
	inv, err := s.beginInvocation(ctx, start)
	if err != nil {
		if errors.Is(err, errChatLedgerUnavailable) {
			return cancelledLedgerUnavailable(req.Call)
		}
		return ExecuteResult{Invocation: inv, Execution: executionError(req.Call, "tool invocation persistence unavailable")}
	}
	// Somente esta entrada executa tools. Record usa o mesmo ledger sem executor.
	exec := s.executorForRequest(req).ExecuteOne(WithCurrentInvocationID(ctx, inv.ID), req.Call)
	status, message := statusForExecution(exec)
	err = s.finishInvocation(ctx, &inv, start, invocationOutcome{
		status: status, message: message, result: exec.Result, errorKind: exec.ErrorKind,
		errorCode: exec.ErrorCode, retryable: exec.Retryable,
		retryabilityKnown: exec.RetryabilityKnown, durationMs: exec.DurationMs,
	})
	return ExecuteResult{Invocation: inv, Execution: exec, Persisted: err == nil}
}

func (s *Service) executorForRequest(req ExecuteRequest) *tools.Executor {
	if s == nil {
		return nil
	}
	if s.executor == nil {
		return nil
	}
	cfg := s.executor.Config()
	effectiveMax := req.ExecutionMaxResultSize
	if effectiveMax <= 0 {
		effectiveMax = cfg.MaxResultSize
	}
	if effectiveMax == cfg.MaxResultSize && req.RequireCompleteResult == cfg.RequireCompleteResult {
		return s.executor
	}
	// Config por request: pode aumentar OU reduzir o budget.
	cfg.MaxResultSize = effectiveMax
	cfg.RequireCompleteResult = req.RequireCompleteResult
	return tools.NewExecutor(s.executor.Registry(), cfg)
}

func (s *Service) truncateForPersistence(result tools.ToolResult) tools.ToolResult {
	max := s.persistMaxResultSize
	if result.Annotations != nil && result.Annotations.OutputWindow != nil &&
		result.Annotations.OutputWindow.ResultID != "" {
		window := result.Annotations.OutputWindow
		if window.HasMore {
			size := window.OriginalBytes
			if size <= 0 {
				size = len(result.Content)
			}
			return persistenceOmissionResult(size)
		}
		// A última página é conteúdo completo por si só. Preserva-a, mas remove o
		// identificador do LRU, que não é durável entre hidratações.
		annotations := *result.Annotations
		windowCopy := *window
		windowCopy.ResultID = ""
		annotations.OutputWindow = &windowCopy
		result.Annotations = &annotations
		if result.Metadata != nil {
			metadata := make(map[string]any, len(result.Metadata))
			for key, value := range result.Metadata {
				if key != "result_id" {
					metadata[key] = value
				}
			}
			result.Metadata = metadata
		}
	}
	if max <= 0 {
		return result
	}
	if len(result.Content) <= max {
		return result
	}
	// A cópia de auditoria nunca persiste prefixos: a hidratação reutiliza este
	// payload no contexto e não poderia distinguir um corte de um resultado
	// completo. Vale também para JSON legado ainda não marcado Structured.
	return persistenceOmissionResult(len(result.Content))
}

func persistenceOmissionResult(originalBytes int) tools.ToolResult {
	return tools.ToolResult{
		Content: "[result_omitted_for_persistence] Resultado exato omitido da cópia de auditoria.",
		IsError: true,
		Metadata: map[string]any{
			"omitted_for_persistence": true,
			"original_size_bytes":     originalBytes,
		},
	}
}

func (s *Service) truncateErrorForPersistence(message string) string {
	max := s.persistMaxErrorSize
	if max <= 0 {
		return message
	}
	if len(message) <= max {
		return message
	}
	suffix := fmt.Sprintf("\n\n[TRUNCADO: error_message tinha %d bytes, limite é %d bytes]", len(message), max)
	budget := max - len(suffix)
	if budget >= 1 {
		return truncateUTF8Safe(message, budget) + suffix
	}
	return truncateUTF8Safe(message, max)
}

func truncateUTF8Safe(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	for maxBytes > 0 && maxBytes < len(s) && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	if maxBytes <= 0 {
		return ""
	}
	return s[:maxBytes]
}

func statusForExecution(exec tools.ToolExecutionResult) (string, string) {
	if exec.ErrorKind == tools.ErrorKindCancelled {
		msg := strings.TrimSpace(exec.Result.Content)
		if msg == "" && exec.Error != nil {
			msg = exec.Error.Error()
		}
		if msg == "" {
			msg = "cancelled"
		}
		return StatusCancelled, msg
	}
	if exec.ErrorKind == tools.ErrorKindTimeout {
		msg := strings.TrimSpace(exec.Result.Content)
		if msg == "" && exec.Error != nil {
			msg = exec.Error.Error()
		}
		if msg == "" {
			msg = "timeout"
		}
		return StatusTimedOut, msg
	}
	if exec.Result.IsError || exec.Error != nil {
		if exec.Error != nil {
			// staticcheck: evita erro nil por segurança
			return StatusFailed, exec.Error.Error()
		}
		return StatusFailed, exec.Result.Content
	}
	return StatusSucceeded, ""
}

func buildInvocationInput(call tools.ToolCall) json.RawMessage {
	// Persiste tool_call com argumentos redigidos para evitar gravar secrets.
	// A execução usa o call original (sem redaction).
	redacted := call
	if strings.TrimSpace(call.Function.Arguments) != "" {
		redacted.Function.Arguments = redactArgumentsJSON(call.Function.Arguments)
	}
	data, _ := json.Marshal(map[string]any{"tool_call": redacted})
	return data
}

func (s *Service) buildInvocationInput(call tools.ToolCall) json.RawMessage {
	// Base: reaproveita a redaction existente.
	input := buildInvocationInput(call)
	max := s.persistMaxInputSize
	if max <= 0 || len(input) <= max {
		return input
	}

	// Tenta reduzir truncando apenas o campo tool_call.function.arguments.
	redacted := call
	if strings.TrimSpace(call.Function.Arguments) != "" {
		redacted.Function.Arguments = redactArgumentsJSON(call.Function.Arguments)
	}
	origSize := len(input)
	suffix := fmt.Sprintf("\n\n[TRUNCADO: argumentos originais tinham %d bytes, limite de input é %d bytes]", len(redacted.Function.Arguments), max)

	// Mede overhead sem argumentos.
	base := redacted
	base.Function.Arguments = ""
	basePayload := map[string]any{
		"tool_call":                  base,
		"_input_truncated":           true,
		"_input_original_size_bytes": origSize,
	}
	baseBytes, _ := json.Marshal(basePayload)
	if len(baseBytes) >= max {
		name := redacted.Function.Name
		id := redacted.ID
		for attempt := 0; attempt < 5; attempt++ {
			minimal := map[string]any{
				"_input_truncated":           true,
				"_input_original_size_bytes": origSize,
				"tool":                       map[string]any{"name": name, "id": id},
			}
			minBytes, _ := json.Marshal(minimal)
			if len(minBytes) <= max {
				return minBytes
			}
			// Reduz identificadores até caber.
			if len(name) > 0 {
				name = truncateUTF8Safe(name, len(name)/2)
			}
			if len(id) > 0 {
				id = truncateUTF8Safe(id, len(id)/2)
			}
			if name == "" && id == "" {
				break
			}
		}
		fallback := map[string]any{
			"_input_truncated":           true,
			"_input_original_size_bytes": origSize,
		}
		minBytes, _ := json.Marshal(fallback)
		return minBytes
	}

	budget := max - len(baseBytes)
	if budget < 1 {
		return baseBytes
	}

	args := redacted.Function.Arguments
	// Ajusta budget considerando o suffix e alguma folga para escaping.
	budget -= len(suffix) + 16
	if budget < 1 {
		budget = 1
	}

	for attempt := 0; attempt < 3; attempt++ {
		truncatedArgs := truncateUTF8Safe(args, budget)
		redacted.Function.Arguments = truncatedArgs + suffix
		payload := map[string]any{
			"tool_call":                  redacted,
			"_input_truncated":           true,
			"_input_original_size_bytes": origSize,
		}
		out, _ := json.Marshal(payload)
		if len(out) <= max {
			return out
		}
		over := len(out) - max
		budget -= over + 64
		if budget < 1 {
			break
		}
	}

	// Fallback: persiste sem argumentos, mas com metadados de truncamento.
	return baseBytes
}

func redactArgumentsJSON(args string) string {
	// Evita spikes de CPU/memória em payloads enormes: não tenta parsear/redigir JSON muito grande.
	const maxRedactionBytes = 256 * 1024
	if len(args) > maxRedactionBytes {
		return "{\"_redacted\":true,\"_too_large\":true}"
	}
	raw := []byte(args)
	if !json.Valid(raw) {
		return "{\"_redacted\":true}"
	}
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "{\"_redacted\":true}"
	}
	redacted := redactAny("", payload)
	out, err := json.Marshal(redacted)
	if err != nil {
		return "{\"_redacted\":true}"
	}
	return string(out)
}

func redactAny(key string, value any) any {
	if isSensitiveKey(key) {
		return "[redacted]"
	}
	switch v := value.(type) {
	case string:
		if looksSensitiveString(v) {
			return "[redacted]"
		}
		return v
	case map[string]any:
		out := make(map[string]any, len(v))
		for childKey, childValue := range v {
			out[childKey] = redactAny(childKey, childValue)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			out[i] = redactAny("", child)
		}
		return out
	default:
		return v
	}
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	compact := strings.NewReplacer("-", "", "_", "", " ", "", ".", "").Replace(normalized)
	for _, token := range []string{
		"api_key",
		"access_key",
		"authorization",
		"client_secret",
		"cookie",
		"credential",
		"jwt",
		"password",
		"private_key",
		"refresh_token",
		"secret",
		"session",
		"session_id",
		"token",
	} {
		tokenCompact := strings.ReplaceAll(token, "_", "")
		if strings.Contains(normalized, token) || strings.Contains(compact, tokenCompact) {
			return true
		}
	}
	return false
}

func looksSensitiveString(value string) bool {
	v := strings.TrimSpace(value)
	if v == "" {
		return false
	}
	if strings.HasPrefix(strings.ToLower(v), "bearer ") {
		return true
	}
	if strings.HasPrefix(v, "sk-") {
		return true
	}
	if strings.Contains(v, "-----BEGIN") {
		return true
	}
	// Heurística leve para JWT/opaque tokens longos.
	if len(v) >= 40 && strings.Count(v, ".") >= 2 && !strings.ContainsAny(v, " \n\t\r") {
		return true
	}
	// Base64-ish longo (sem espaços) tende a ser token.
	if len(v) >= 64 && !strings.ContainsAny(v, " \n\t\r") {
		// Evita marcar textos longos comuns.
		if strings.ContainsAny(v, "=_-") {
			return true
		}
	}
	return false
}

func (s *Service) ExecuteAll(ctx context.Context, calls []tools.ToolCall, origin Origin, iteration int) []ExecuteResult {
	results := make([]ExecuteResult, len(calls))
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func(idx int, tc tools.ToolCall) {
			defer wg.Done()
			results[idx] = s.Execute(ctx, ExecuteRequest{Call: tc, Origin: origin, Iteration: iteration})
		}(i, call)
	}
	wg.Wait()
	return results
}

// Record persiste uma invocação já executada fora do executor comum
// (por exemplo, MCP nativo executado pelo provedor LLM). Best-effort: não
// deve falhar o fluxo chamador.
func (s *Service) Record(ctx context.Context, req RecordRequest) (Invocation, error) {
	start := invocationStart{
		call: req.Call, persistedArguments: req.PersistedArguments, origin: req.Origin,
		parentID: req.ParentInvocationID, catalogID: req.ToolCatalogID, dryRun: req.DryRun,
		iteration: req.Iteration, external: true, observation: req.Observation,
	}
	inv, err := s.beginInvocation(ctx, start)
	if err != nil {
		return inv, err
	}
	status, message := statusForRecord(req)
	err = s.finishInvocation(ctx, &inv, start, invocationOutcome{
		status: status, message: message, result: req.Result, errorKind: req.ErrorKind,
		errorCode: req.ErrorCode, retryable: req.Retryable,
		retryabilityKnown: req.RetryabilityKnown, durationMs: req.DurationMs,
	})
	return inv, err
}

func (s *Service) buildInvocationDisplayMetadata(call tools.ToolCall, iteration int, durationMs int64, external bool) json.RawMessage {
	arguments := ""
	if strings.TrimSpace(call.Function.Arguments) != "" {
		arguments = redactArgumentsJSON(call.Function.Arguments)
	}
	return buildInvocationDisplayMetadata(call, arguments, iteration, durationMs, external, s.persistMaxInputSize)
}

// buildInvocationMetadata acrescenta somente o contrato de apresentação
// explicitamente emitido por buscas nativas. Nunca promove metadata genérica
// nem conteúdo de integrações MCP para a camada de interface.
func (s *Service) buildInvocationMetadata(call tools.ToolCall, iteration int, durationMs int64, external bool, result tools.ToolResult) json.RawMessage {
	base := s.buildInvocationDisplayMetadata(call, iteration, durationMs, external)
	if external {
		return base
	}
	var payload map[string]any
	if err := json.Unmarshal(base, &payload); err != nil || payload == nil {
		return base
	}
	if call.Function.Name == "search_files" || call.Function.Name == "grep_search" {
		if presentation, ok := result.Metadata[tools.SearchResultPresentationMetadataKey].(tools.SearchResultPresentation); ok && presentation.Version == 1 {
			payload[tools.SearchResultPresentationMetadataKey] = presentation
		}
	}
	if signals, ok := result.Metadata[tools.SecuritySignalsMetadataKey].([]tools.SecuritySignal); ok && len(signals) > 0 {
		payload[tools.SecuritySignalsMetadataKey] = signals
	}
	merged, err := json.Marshal(payload)
	if err != nil {
		return base
	}
	return merged
}

func buildInvocationDisplayMetadata(call tools.ToolCall, arguments string, iteration int, durationMs int64, external bool, maxBytes int) json.RawMessage {
	payload := buildInvocationDisplayPayload(call, arguments, iteration, durationMs, external, false, 0)
	metadata, _ := json.Marshal(payload)
	if maxBytes <= 0 || len(metadata) <= maxBytes {
		return metadata
	}

	origSize := len(arguments)
	basePayload := buildInvocationDisplayPayload(call, "", iteration, durationMs, external, true, origSize)
	baseBytes, _ := json.Marshal(basePayload)
	if len(baseBytes) >= maxBytes {
		return compactInvocationDisplayMetadata(origSize, maxBytes)
	}
	return baseBytes
}

func compactInvocationDisplayMetadata(originalSize int, maxBytes int) json.RawMessage {
	candidates := []map[string]any{
		{
			"display": map[string]any{
				"version":                        1,
				"_metadata_truncated":            true,
				"_arguments_truncated":           true,
				"_arguments_original_size_bytes": originalSize,
			},
		},
		{
			"display": map[string]any{
				"_metadata_truncated": true,
			},
		},
	}
	for _, candidate := range candidates {
		metadata, _ := json.Marshal(candidate)
		if maxBytes <= 0 || len(metadata) <= maxBytes {
			return metadata
		}
	}
	if maxBytes >= len([]byte(`{}`)) {
		return json.RawMessage(`{}`)
	}
	return nil
}

func buildInvocationDisplayPayload(call tools.ToolCall, arguments string, iteration int, durationMs int64, external bool, truncated bool, originalSize int) map[string]any {
	name := strings.TrimSpace(call.Function.Name)
	displayName := name
	origin := "builtin"
	serverLabel := ""
	if strings.HasPrefix(name, "mcp_") {
		idx := strings.Index(name, "__")
		if idx > len("mcp_") && idx+2 < len(name) {
			serverLabel = strings.TrimSpace(name[len("mcp_"):idx])
			displayName = strings.TrimSpace(name[idx+2:])
		}
	}
	if serverLabel != "" && displayName != "" {
		origin = "mcp_bridge"
	}
	if displayName == "" {
		displayName = name
	}
	display := map[string]any{
		"version":      1,
		"type":         firstNonEmpty(call.Type, "function"),
		"name":         displayName,
		"origin":       origin,
		"server_label": serverLabel,
		"iteration":    iteration,
		"duration_ms":  durationMs,
	}
	if !truncated || arguments != "" {
		display["arguments"] = arguments
	}
	payload := map[string]any{
		"display": display,
	}
	if truncated {
		display["_arguments_truncated"] = true
		display["_arguments_original_size_bytes"] = originalSize
	}
	if external {
		payload["external"] = true
		display["origin"] = "mcp_native"
	}
	return payload
}

func firstNonEmpty(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return fallback
}

func statusForRecord(req RecordRequest) (string, string) {
	if req.ErrorKind == tools.ErrorKindCancelled {
		return StatusCancelled, req.ErrorMessage
	}
	if req.ErrorKind == tools.ErrorKindTimeout {
		return StatusTimedOut, req.ErrorMessage
	}
	if req.ErrorKind != tools.ErrorKindNone && strings.TrimSpace(string(req.ErrorKind)) != "" {
		msg := strings.TrimSpace(req.ErrorMessage)
		if msg == "" {
			msg = strings.TrimSpace(req.Result.Content)
		}
		if msg == "" {
			msg = string(req.ErrorKind)
		}
		return StatusFailed, msg
	}
	if req.Result.IsError {
		if strings.TrimSpace(req.ErrorMessage) != "" {
			return StatusFailed, req.ErrorMessage
		}
		return StatusFailed, req.Result.Content
	}
	if strings.TrimSpace(req.ErrorMessage) != "" {
		return StatusFailed, req.ErrorMessage
	}
	return StatusSucceeded, ""
}

func executionError(call tools.ToolCall, message string) tools.ToolExecutionResult {
	return tools.ToolExecutionResult{
		CallID:   call.ID,
		ToolName: call.Function.Name,
		Result: tools.ToolResult{
			Content: message,
			IsError: true,
		},
		ErrorKind:         tools.ErrorKindUnknown,
		Retryable:         false,
		RetryabilityKnown: true,
		DurationMs:        0,
	}
}

func executionCancelled(call tools.ToolCall, message string) tools.ToolExecutionResult {
	if strings.TrimSpace(message) == "" {
		message = "Execução cancelada"
	}
	return tools.ToolExecutionResult{
		CallID:   call.ID,
		ToolName: call.Function.Name,
		Result: tools.ToolResult{
			Content: message,
			IsError: true,
		},
		ErrorKind:         tools.ErrorKindCancelled,
		Retryable:         false,
		RetryabilityKnown: true,
		DurationMs:        0,
	}
}

func (s *Service) outputForPersistence(result tools.ToolResult) json.RawMessage {
	max := s.persistMaxResultSize
	trimmed := s.truncateForPersistence(result)
	data := resultOutput(trimmed)
	if max <= 0 || len(data) <= max {
		return data
	}

	// Primeiro fallback: dropa metadata, que pode explodir o payload.
	trimmed.Metadata = map[string]any{"omitted_for_persistence": true}
	data = resultOutput(trimmed)
	if len(data) <= max {
		return data
	}

	// Se metadata/anotações fizeram o payload exceder o teto, não remova o
	// contrato e apresente o corpo como completo: persista omissão explícita.
	return minimalPersistenceOutput(max)
}

func minimalPersistenceOutput(max int) json.RawMessage {
	candidates := []map[string]any{{
		"content":  "Output omitido da cópia de auditoria por exceder o limite de persistência.",
		"is_error": true,
		"metadata": map[string]any{"omitted_for_persistence": true},
	}, {
		"content":  "[result_omitted_for_persistence]",
		"is_error": true,
	}}
	for _, candidate := range candidates {
		data, _ := json.Marshal(candidate)
		if max <= 0 || len(data) <= max {
			return data
		}
	}
	// Um valor JSON escalar de um byte cabe em qualquer teto positivo e não se
	// confunde com ToolResult vazio. O hidratador o reconhece como omissão.
	return json.RawMessage(persistenceOmissionSentinel)
}
