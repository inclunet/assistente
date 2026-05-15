package toolinvocations

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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

	// persistMaxResultSize limita o output armazenado em tool_invocations,
	// independente do limite usado para a execução (que pode ser maior).
	persistMaxResultSize int

	// persistMaxErrorSize limita error_message, que pode vir de ToolResult.Content
	// (especialmente quando IsError=true sem error Go) e pode ser muito grande.
	persistMaxErrorSize int
}

func NewService(repo Repository, executor *tools.Executor) *Service {
	return &Service{
		repo:                 repo,
		executor:             executor,
		now:                  time.Now,
		persistMaxResultSize: tools.DefaultMaxResultSize,
		persistMaxErrorSize:  16 * 1024,
	}
}

func (s *Service) CleanOld(ctx context.Context, maxAge time.Duration) (int, error) {
	if s == nil || s.repo == nil {
		return 0, nil
	}
	return s.repo.CleanOld(ctx, maxAge)
}

func (s *Service) Execute(ctx context.Context, req ExecuteRequest) ExecuteResult {
	if s == nil || s.executor == nil {
		return ExecuteResult{Execution: executionError(req.Call, "tool invocation service not configured")}
	}
	if s.repo == nil {
		// Sem persistência configurada: ainda executa a tool.
		exec := s.executorForRequest(req).ExecuteOne(ctx, req.Call)
		return ExecuteResult{Execution: exec}
	}

	queuedAt := s.now()
	toolCatalogID := req.ToolCatalogID
	if toolCatalogID == "" {
		id, err := s.repo.ResolveToolCatalogID(ctx, req.Call.Function.Name)
		if err != nil {
			// Best-effort: não bloqueia execução quando o catálogo está
			// desatualizado/indisponível. Executa a tool sem persistir.
			log.Printf("[toolinvocations] failed to resolve tool_catalog_id (best-effort): %v", err)
			exec := s.executorForRequest(req).ExecuteOne(ctx, req.Call)
			return ExecuteResult{Execution: exec}
		}
		toolCatalogID = id
	}

	input := buildInvocationInput(req.Call)
	inv := Invocation{
		ToolCatalogID:      toolCatalogID,
		OriginType:         req.Origin.Type,
		OriginID:           req.Origin.ID,
		ParentInvocationID: req.ParentInvocationID,
		ToolCallID:         req.Call.ID,
		Status:             StatusQueued,
		DryRun:             req.DryRun,
		Input:              input,
		QueuedAt:           queuedAt,
	}
	if inv.OriginType == "" {
		inv.OriginType = OriginChat
	}
	if inv.ToolCatalogID == "" {
		return ExecuteResult{Execution: executionError(req.Call, "tool_catalog_id is required")}
	}

	// Persistência best-effort: falhas aqui não devem impedir execução
	// (chat/jobs usam este caminho no runtime).
	persistCtx := context.WithoutCancel(ctx)
	if err := s.repo.Create(persistCtx, &inv); err != nil {
		log.Printf("[toolinvocations] failed to create invocation (best-effort): %v", err)
		inv.ID = ""
	}
	if inv.ID != "" {
		if err := s.repo.MarkRunning(persistCtx, inv.ID, s.now()); err != nil {
			log.Printf("[toolinvocations] failed to mark running (id=%s): %v", inv.ID, err)
		}
	}

	exec := s.executorForRequest(req).ExecuteOne(ctx, req.Call)
	if s.repo != nil && inv.ID != "" {
		status, errorMessage := statusForExecution(exec)
		completedAt := s.now()
		inv.Status = status
		inv.Output = resultOutput(s.truncateForPersistence(exec.Result))
		inv.ErrorKind = string(exec.ErrorKind)
		inv.ErrorMessage = s.truncateErrorForPersistence(errorMessage)
		inv.Retryable = exec.Retryable
		inv.CompletedAt = &completedAt
		inv.DurationMs = exec.DurationMs
		inv.Metadata = nil
		if err := s.repo.Complete(persistCtx, inv.ID, &inv); err != nil {
			log.Printf("[toolinvocations] failed to complete invocation (id=%s): %v", inv.ID, err)
		}
	}

	return ExecuteResult{Invocation: inv, Execution: exec}
}

func (s *Service) executorForRequest(req ExecuteRequest) *tools.Executor {
	if s == nil || s.executor == nil {
		return s.executor
	}
	if req.ExecutionMaxResultSize <= 0 {
		return s.executor
	}
	cfg := s.executor.Config()
	// Garantia de segurança: não reduz o limite por acidente.
	if req.ExecutionMaxResultSize > cfg.MaxResultSize {
		cfg.MaxResultSize = req.ExecutionMaxResultSize
		return tools.NewExecutor(s.executor.Registry(), cfg)
	}
	return s.executor
}

func (s *Service) truncateForPersistence(result tools.ToolResult) tools.ToolResult {
	max := s.persistMaxResultSize
	if max <= 0 {
		return result
	}
	if len(result.Content) <= max {
		return result
	}

	// Evita mutar o mapa original (ToolResult.Metadata é map por referência).
	if result.Metadata != nil {
		result.Metadata = cloneAnyMap(result.Metadata)
	}

	// Truncamento UTF-8 safe: replica a semântica do executor.
	origSize := len(result.Content)
	warning := fmt.Sprintf(
		"\n\n[TRUNCADO: resultado original tinha %d bytes, limite é %d bytes]",
		origSize, max,
	)
	contentBudget := max - len(warning)
	if contentBudget >= 1 {
		result.Content = truncateUTF8Safe(result.Content, contentBudget) + warning
	} else {
		result.Content = truncateUTF8Safe(result.Content, max)
	}
	if result.Metadata == nil {
		result.Metadata = make(map[string]any)
	}
	result.Metadata["truncated_for_persistence"] = true
	result.Metadata["original_size_bytes"] = origSize
	return result
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

func cloneAnyMap(src map[string]any) map[string]any {
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
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
		return StatusCancelled, "cancelled"
	}
	if exec.ErrorKind == tools.ErrorKindTimeout {
		return StatusTimedOut, "timeout"
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

func redactArgumentsJSON(args string) string {
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
	if strings.HasPrefix(v, "Bearer ") {
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

func (s *Service) ExecuteAll(ctx context.Context, calls []tools.ToolCall, origin Origin) []ExecuteResult {
	results := make([]ExecuteResult, len(calls))
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func(idx int, tc tools.ToolCall) {
			defer wg.Done()
			results[idx] = s.Execute(ctx, ExecuteRequest{Call: tc, Origin: origin})
		}(i, call)
	}
	wg.Wait()
	return results
}

// Record persiste uma invocação já executada fora do executor comum
// (por exemplo, MCP nativo executado pelo provedor LLM). Best-effort: não
// deve falhar o fluxo chamador.
func (s *Service) Record(ctx context.Context, req RecordRequest) (Invocation, error) {
	if s == nil || s.repo == nil {
		return Invocation{}, fmt.Errorf("tool invocation repository not configured")
	}
	toolCatalogID := strings.TrimSpace(req.ToolCatalogID)
	if toolCatalogID == "" {
		id, err := s.repo.ResolveToolCatalogID(ctx, req.Call.Function.Name)
		if err != nil {
			return Invocation{}, err
		}
		toolCatalogID = id
	}
	queuedAt := s.now()
	inv := Invocation{
		ToolCatalogID:      toolCatalogID,
		OriginType:         req.Origin.Type,
		OriginID:           req.Origin.ID,
		ParentInvocationID: "",
		ToolCallID:         req.Call.ID,
		Status:             StatusQueued,
		DryRun:             req.DryRun,
		Input:              buildInvocationInput(req.Call),
		QueuedAt:           queuedAt,
	}
	if inv.OriginType == "" {
		inv.OriginType = OriginChat
	}

	persistCtx := context.WithoutCancel(ctx)
	if err := s.repo.Create(persistCtx, &inv); err != nil {
		return inv, err
	}
	startedAt := s.now()
	if err := s.repo.MarkRunning(persistCtx, inv.ID, startedAt); err != nil {
		log.Printf("[toolinvocations] failed to mark running (id=%s): %v", inv.ID, err)
	}
	status, errorMessage := statusForRecord(req)
	completedAt := s.now()
	inv.Status = status
	inv.Output = resultOutput(s.truncateForPersistence(req.Result))
	inv.ErrorKind = string(req.ErrorKind)
	inv.ErrorMessage = s.truncateErrorForPersistence(errorMessage)
	inv.Retryable = req.Retryable
	inv.CompletedAt = &completedAt
	inv.DurationMs = req.DurationMs
	metadata, _ := json.Marshal(map[string]any{"external": true})
	inv.Metadata = metadata
	if err := s.repo.Complete(persistCtx, inv.ID, &inv); err != nil {
		log.Printf("[toolinvocations] failed to complete recorded invocation (id=%s): %v", inv.ID, err)
	}
	return inv, nil
}

func statusForRecord(req RecordRequest) (string, string) {
	if req.ErrorKind == tools.ErrorKindCancelled {
		return StatusCancelled, req.ErrorMessage
	}
	if req.ErrorKind == tools.ErrorKindTimeout {
		return StatusTimedOut, req.ErrorMessage
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
		ErrorKind:  tools.ErrorKindUnknown,
		Retryable:  false,
		DurationMs: 0,
	}
}
