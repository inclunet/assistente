package toolinvocations

import (
	"assistente/internal/logging"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"assistente/internal/database"
	"assistente/internal/tools"

	"gorm.io/gorm"
)

type Service struct {
	repo     Repository
	executor *tools.Executor
	now      func() time.Time

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

func NewService(repo Repository, executor *tools.Executor) *Service {
	return &Service{
		repo:                 repo,
		executor:             executor,
		now:                  time.Now,
		persistOpTimeout:     3 * time.Second,
		persistMaxResultSize: tools.DefaultMaxResultSize,
		// Mantém o mesmo limite de persistência do Output para consistência.
		persistMaxErrorSize: tools.DefaultMaxResultSize,
		persistMaxInputSize: tools.DefaultMaxResultSize,
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

func (s *Service) Execute(ctx context.Context, req ExecuteRequest) ExecuteResult {
	if s == nil || s.executor == nil {
		return ExecuteResult{Execution: executionError(req.Call, "tool invocation service not configured"), Persisted: false}
	}
	if s.repo == nil {
		// Sem persistência configurada: ainda executa a tool.
		exec := s.executorForRequest(req).ExecuteOne(ctx, req.Call)
		return ExecuteResult{Execution: exec, Persisted: false}
	}

	// Persistência best-effort: deve funcionar mesmo se o ctx for cancelado.
	persistCtx := s.persistCtx(ctx)

	// Defesa best-effort: se a origem do chat já foi deletada, não criar
	// registros técnicos que ficarão órfãos. Alguns cenários de teste/migração
	// não têm a tabela de chat_messages disponível.
	if strings.TrimSpace(req.Origin.Type) == OriginChat && strings.TrimSpace(req.Origin.ID) != "" {
		if db := database.DB(); db != nil && db.Migrator().HasTable(&database.ChatMessage{}) {
			opCtx, cancel := s.persistOpCtx(persistCtx)
			_, err := database.GetMessageWithContext(opCtx, req.Origin.ID)
			cancel()
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					// Se o turno/mensagem foi removido antes da execução, não execute tools com efeitos colaterais.
					logging.Warnf(ctx, "toolinvocations.service", "[toolinvocations] chat origin %s deleted before execution; aborting tool execution", strings.TrimSpace(req.Origin.ID))
					return ExecuteResult{Execution: executionCancelled(req.Call, "Execução cancelada: o item do chat foi removido"), Persisted: false}
				}
				// Para falhas transitórias de DB, mantém best-effort e executa sem persistência.
				logging.Errorf(ctx, "toolinvocations.service", "[toolinvocations] failed to validate chat origin %s; executing without persistence (best-effort): %v", strings.TrimSpace(req.Origin.ID), err)
				exec := s.executorForRequest(req).ExecuteOne(ctx, req.Call)
				return ExecuteResult{Execution: exec, Persisted: false}
			}
		}
	}

	queuedAt := s.now()
	toolCatalogID := req.ToolCatalogID
	if strings.TrimSpace(toolCatalogID) != "" {
		opCtx, cancel := s.persistOpCtx(persistCtx)
		visible, err := s.repo.IsToolCatalogIDVisible(opCtx, toolCatalogID)
		cancel()
		if err != nil {
			logging.Errorf(ctx, "toolinvocations.service", "[toolinvocations] failed to validate tool_catalog_id (best-effort): %v", err)
			toolCatalogID = ""
		} else if !visible {
			logging.Infof(ctx, "toolinvocations.service", "[toolinvocations] tool_catalog_id not visible to user; falling back to resolve by name (best-effort) id=%s", strings.TrimSpace(toolCatalogID))
			toolCatalogID = ""
		} else {
			// Defesa: garante que o ID fornecido corresponde ao nome da tool.
			opCtx, cancel := s.persistOpCtx(persistCtx)
			resolved, err := s.repo.ResolveToolCatalogID(opCtx, req.Call.Function.Name)
			cancel()
			if err != nil {
				logging.Errorf(ctx, "toolinvocations.service", "[toolinvocations] failed to verify tool_catalog_id by name (best-effort): %v", err)
				toolCatalogID = ""
			} else if strings.TrimSpace(resolved) != "" && resolved != toolCatalogID {
				logging.Infof(ctx, "toolinvocations.service", "[toolinvocations] tool_catalog_id mismatch for %q; using resolved id", req.Call.Function.Name)
				toolCatalogID = resolved
			}
		}
	}
	if toolCatalogID == "" {
		opCtx, cancel := s.persistOpCtx(persistCtx)
		id, err := s.repo.ResolveToolCatalogID(opCtx, req.Call.Function.Name)
		cancel()
		if err != nil {
			// Best-effort: não bloqueia execução quando o catálogo está
			// desatualizado/indisponível. Executa a tool sem persistir.
			logging.Errorf(ctx, "toolinvocations.service", "[toolinvocations] failed to resolve tool_catalog_id (best-effort): %v", err)
			exec := s.executorForRequest(req).ExecuteOne(ctx, req.Call)
			return ExecuteResult{Execution: exec, Persisted: false}
		}
		toolCatalogID = id
	}

	input := s.buildInvocationInput(req.Call)
	// Encadeamento pai↔filho (AEP-0068): se o chamador não trouxe um
	// ParentInvocationID explícito, herda o carimbado no ctx (ex.: sub-conversa
	// de sub-agente herda a invocação da tool `subagent` que a originou).
	parentInvocationID := req.ParentInvocationID
	if parentInvocationID == "" {
		parentInvocationID = ParentInvocationIDFromContext(ctx)
	}
	inv := Invocation{
		ToolCatalogID:      toolCatalogID,
		OriginType:         req.Origin.Type,
		OriginID:           req.Origin.ID,
		ParentInvocationID: parentInvocationID,
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

	opCtx, cancel := s.persistOpCtx(persistCtx)
	if err := s.repo.Create(opCtx, &inv); err != nil {
		logging.Errorf(ctx, "toolinvocations.service", "[toolinvocations] failed to create invocation (best-effort): %v", err)
		inv.ID = ""
	}
	cancel()
	if inv.ID != "" {
		startedAt := s.now()
		opCtx, cancel := s.persistOpCtx(persistCtx)
		if err := s.repo.MarkRunning(opCtx, inv.ID, startedAt); err != nil {
			logging.Errorf(ctx, "toolinvocations.service", "[toolinvocations] failed to mark running (id=%s): %v", inv.ID, err)
		} else {
			inv.StartedAt = &startedAt
		}
		cancel()
	}

	// Carimba o ID da invocação corrente no ctx para que tools que delegam
	// (ex.: `subagent`) possam encadear suas sub-invocações a este turno.
	execCtx := WithCurrentInvocationID(ctx, inv.ID)
	exec := s.executorForRequest(req).ExecuteOne(execCtx, req.Call)
	persisted := false
	if s.repo != nil && inv.ID != "" {
		// Revalida a origem de chat antes de finalizar. Se o turno/mensagem foi
		// deletado enquanto a tool estava rodando, apaga a invocação recém-criada
		// para não deixar registros órfãos.
		if strings.TrimSpace(inv.OriginType) == OriginChat && strings.TrimSpace(inv.OriginID) != "" {
			if db := database.DB(); db != nil && db.Migrator().HasTable(&database.ChatMessage{}) {
				opCtx, cancel := s.persistOpCtx(persistCtx)
				_, err := database.GetMessageWithContext(opCtx, inv.OriginID)
				cancel()
				if err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						opCtx, cancel := s.persistOpCtx(persistCtx)
						delErr := s.repo.Delete(opCtx, inv.ID)
						cancel()
						if delErr != nil {
							logging.Errorf(ctx, "toolinvocations.service", "[toolinvocations] failed to delete orphan invocation (id=%s): %v", inv.ID, delErr)
						}
						return ExecuteResult{Invocation: inv, Execution: exec, Persisted: false}
					}
					// Se não conseguimos revalidar por erro transitório, tenta completar a invocação.
					logging.Warnf(ctx, "toolinvocations.service", "[toolinvocations] warning: failed to revalidate chat origin %s before complete; completing anyway (id=%s): %v", strings.TrimSpace(inv.OriginID), inv.ID, err)
				}
			}
		}

		status, errorMessage := statusForExecution(exec)
		completedAt := s.now()
		inv.Status = status
		inv.Output = s.outputForPersistence(exec.Result)
		inv.ErrorKind = string(exec.ErrorKind)
		inv.ErrorMessage = s.truncateErrorForPersistence(errorMessage)
		inv.Retryable = exec.Retryable
		inv.CompletedAt = &completedAt
		inv.DurationMs = exec.DurationMs
		inv.Metadata = nil
		opCtx, cancel := s.persistOpCtx(persistCtx)
		err := s.repo.Complete(opCtx, inv.ID, &inv)
		cancel()
		if err != nil {
			logging.Errorf(ctx, "toolinvocations.service", "[toolinvocations] failed to complete invocation (id=%s): %v", inv.ID, err)
			persisted = false
		} else {
			persisted = true
		}
	}

	return ExecuteResult{Invocation: inv, Execution: exec, Persisted: persisted}
}

func (s *Service) executorForRequest(req ExecuteRequest) *tools.Executor {
	if s == nil {
		return nil
	}
	if s.executor == nil {
		return nil
	}
	if req.ExecutionMaxResultSize <= 0 {
		return s.executor
	}
	cfg := s.executor.Config()
	if req.ExecutionMaxResultSize == cfg.MaxResultSize {
		return s.executor
	}
	// Config por request: pode aumentar OU reduzir o budget.
	cfg.MaxResultSize = req.ExecutionMaxResultSize
	return tools.NewExecutor(s.executor.Registry(), cfg)
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

	// Persistência de invocações externas também deve sobreviver a cancelamento.
	persistCtx := s.persistCtx(ctx)

	// Defesa best-effort: se a origem do chat já foi deletada, não criar
	// registros técnicos que ficarão órfãos.
	if strings.TrimSpace(req.Origin.Type) == OriginChat && strings.TrimSpace(req.Origin.ID) != "" {
		if db := database.DB(); db != nil && db.Migrator().HasTable(&database.ChatMessage{}) {
			opCtx, cancel := s.persistOpCtx(persistCtx)
			_, err := database.GetMessageWithContext(opCtx, req.Origin.ID)
			cancel()
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return Invocation{}, err
				}
				logging.Warnf(ctx, "toolinvocations.service", "[toolinvocations] warning: failed to validate chat origin %s for Record; proceeding (best-effort): %v", strings.TrimSpace(req.Origin.ID), err)
			}
		}
	}

	toolCatalogID := strings.TrimSpace(req.ToolCatalogID)
	if toolCatalogID != "" {
		opCtx, cancel := s.persistOpCtx(persistCtx)
		visible, err := s.repo.IsToolCatalogIDVisible(opCtx, toolCatalogID)
		cancel()
		if err != nil {
			return Invocation{}, err
		}
		if !visible {
			toolCatalogID = ""
		} else {
			opCtx, cancel := s.persistOpCtx(persistCtx)
			resolved, err := s.repo.ResolveToolCatalogID(opCtx, req.Call.Function.Name)
			cancel()
			if err != nil {
				// Melhor não persistir sob um ID possivelmente incorreto.
				toolCatalogID = ""
			} else if strings.TrimSpace(resolved) != "" && resolved != toolCatalogID {
				toolCatalogID = resolved
			}
		}
	}
	if toolCatalogID == "" {
		opCtx, cancel := s.persistOpCtx(persistCtx)
		id, err := s.repo.ResolveToolCatalogID(opCtx, req.Call.Function.Name)
		cancel()
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
		Input:              s.buildInvocationInput(req.Call),
		QueuedAt:           queuedAt,
	}
	if inv.OriginType == "" {
		inv.OriginType = OriginChat
	}

	opCtx, cancel := s.persistOpCtx(persistCtx)
	if err := s.repo.Create(opCtx, &inv); err != nil {
		cancel()
		return inv, err
	}
	cancel()
	startedAt := s.now()
	opCtx, cancel = s.persistOpCtx(persistCtx)
	if err := s.repo.MarkRunning(opCtx, inv.ID, startedAt); err != nil {
		logging.Errorf(ctx, "toolinvocations.service", "[toolinvocations] failed to mark running (id=%s): %v", inv.ID, err)
	} else {
		inv.StartedAt = &startedAt
	}
	cancel()
	status, errorMessage := statusForRecord(req)
	completedAt := s.now()
	inv.Status = status
	inv.Output = s.outputForPersistence(req.Result)
	inv.ErrorKind = string(req.ErrorKind)
	inv.ErrorMessage = s.truncateErrorForPersistence(errorMessage)
	inv.Retryable = req.Retryable
	inv.CompletedAt = &completedAt
	inv.DurationMs = req.DurationMs
	metadata, _ := json.Marshal(map[string]any{"external": true})
	inv.Metadata = metadata

	// Revalida a origem do chat antes de finalizar. Native MCP pode correr com
	// deleção de turno/mensagem após o pre-check do chamador.
	if strings.TrimSpace(inv.OriginType) == OriginChat && strings.TrimSpace(inv.OriginID) != "" {
		if db := database.DB(); db != nil && db.Migrator().HasTable(&database.ChatMessage{}) {
			opCtx, cancel := s.persistOpCtx(persistCtx)
			_, err := database.GetMessageWithContext(opCtx, inv.OriginID)
			cancel()
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					opCtx, cancel := s.persistOpCtx(persistCtx)
					delErr := s.repo.Delete(opCtx, inv.ID)
					cancel()
					if delErr != nil {
						logging.Errorf(ctx, "toolinvocations.service", "[toolinvocations] failed to delete orphan recorded invocation (id=%s): %v", inv.ID, delErr)
					}
					return inv, err
				}
				logging.Warnf(ctx, "toolinvocations.service", "[toolinvocations] warning: failed to revalidate chat origin %s before completing Record; completing anyway (id=%s): %v", strings.TrimSpace(inv.OriginID), inv.ID, err)
			}
		}
	}
	opCtx, cancel = s.persistOpCtx(persistCtx)
	err := s.repo.Complete(opCtx, inv.ID, &inv)
	cancel()
	if err != nil {
		logging.Errorf(ctx, "toolinvocations.service", "[toolinvocations] failed to complete recorded invocation (id=%s): %v", inv.ID, err)
		return inv, err
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
		ErrorKind:  tools.ErrorKindUnknown,
		Retryable:  false,
		DurationMs: 0,
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
		ErrorKind:  tools.ErrorKindCancelled,
		Retryable:  false,
		DurationMs: 0,
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
	trimmed.Metadata = nil
	data = resultOutput(trimmed)
	if len(data) <= max {
		return data
	}

	// Fallback final: reduz content até caber no JSON (UTF-8 safe).
	origSize := len(data)
	warning := fmt.Sprintf(
		"\n\n[TRUNCADO: payload serializado tinha %d bytes, limite é %d bytes]",
		origSize,
		max,
	)
	content := trimmed.Content
	// Começa com um budget razoável; ajusta iterativamente com base no marshal.
	budget := max
	for attempt := 0; attempt < 4; attempt++ {
		candidate := truncateUTF8Safe(content, budget)
		trimmed.Content = candidate + warning
		data = resultOutput(trimmed)
		if len(data) <= max {
			return data
		}
		over := len(data) - max
		budget -= over + 64
		if budget < 1 {
			break
		}
	}

	// Último recurso: JSON mínimo válido.
	isErr := result.IsError
	minimal, _ := json.Marshal(map[string]any{
		"content":  "[TRUNCADO: output excedeu limite de persistência]",
		"is_error": isErr,
	})
	if len(minimal) > 0 {
		return minimal
	}
	if isErr {
		return json.RawMessage(`{"content":"[TRUNCADO]","is_error":true}`)
	}
	return json.RawMessage(`{"content":"[TRUNCADO]","is_error":false}`)
}
