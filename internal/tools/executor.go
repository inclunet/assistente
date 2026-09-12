package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Limites padrão do executor
const (
	// DefaultToolTimeout é o timeout padrão para execução de uma única tool.
	// Ferramentas como run_command podem ter seus próprios timeouts internos
	// maiores — por isso este valor deve acomodar o timeout máximo permitido
	// (maxTimeout de run_command = 5 min) + margem.
	DefaultToolTimeout = 6 * time.Minute

	// DefaultMaxResultSize é o tamanho máximo do resultado de uma tool (100KB)
	DefaultMaxResultSize = 100 * 1024

	// DefaultMaxIterations é o número máximo de iterações do agentic loop
	DefaultMaxIterations = 25
)

// ExecutorConfig contém configurações do executor de ferramentas.
type ExecutorConfig struct {
	// ToolTimeout é o timeout para execução de cada ferramenta individual
	ToolTimeout time.Duration

	// MaxResultSize é o tamanho máximo em bytes do resultado de uma tool.
	// Resultados maiores são truncados com aviso.
	MaxResultSize int

	// MaxIterations é o número máximo de iterações do agentic loop
	MaxIterations int
}

// DefaultExecutorConfig retorna a configuração padrão do executor.
func DefaultExecutorConfig() ExecutorConfig {
	return ExecutorConfig{
		ToolTimeout:   DefaultToolTimeout,
		MaxResultSize: DefaultMaxResultSize,
		MaxIterations: DefaultMaxIterations,
	}
}

// Executor orquestra a execução de ferramentas.
// Suporta execução paralela de múltiplas tools com timeout individual.
type Executor struct {
	registry *Registry
	config   ExecutorConfig
}

// NewExecutor cria um novo executor com o registry e configuração fornecidos.
func NewExecutor(registry *Registry, config ExecutorConfig) *Executor {
	return &Executor{
		registry: registry,
		config:   config,
	}
}

// ExecuteAll executa uma lista de tool calls em paralelo.
// Cada tool é executada com seu próprio timeout, respeitando o ctx pai.
// Os resultados são retornados na mesma ordem dos calls recebidos.
func (e *Executor) ExecuteAll(ctx context.Context, calls []ToolCall) []ToolExecutionResult {
	results := make([]ToolExecutionResult, len(calls))
	var wg sync.WaitGroup

	for i, call := range calls {
		wg.Add(1)
		go func(idx int, tc ToolCall) {
			defer wg.Done()
			results[idx] = e.executeSingle(ctx, tc)
		}(i, call)
	}

	wg.Wait()
	return results
}

// ExecuteOne executa uma única tool call.
func (e *Executor) ExecuteOne(ctx context.Context, call ToolCall) ToolExecutionResult {
	return e.executeSingle(ctx, call)
}

// executeSingle executa uma única tool com timeout e tratamento de erro.
func (e *Executor) executeSingle(ctx context.Context, call ToolCall) ToolExecutionResult {
	toolName := call.Function.Name
	start := time.Now()

	if err := validateExecutionContextToolAccess(ctx, toolName); err != nil {
		return ToolExecutionResult{
			CallID:   call.ID,
			ToolName: toolName,
			Result: ToolResult{
				Content: err.Error(),
				IsError: true,
			},
			Error:             err,
			ErrorKind:         ErrorKindInvalidArgs,
			Retryable:         false,
			RetryabilityKnown: true,
			DurationMs:        time.Since(start).Milliseconds(),
		}
	}

	// Busca a ferramenta no registry
	tool, ok := e.registry.Get(toolName)
	if !ok {
		return ToolExecutionResult{
			CallID:   call.ID,
			ToolName: toolName,
			Result: ToolResult{
				Content: fmt.Sprintf("Ferramenta '%s' não encontrada", toolName),
				IsError: true,
			},
			Error:             fmt.Errorf("ferramenta '%s' não encontrada", toolName),
			ErrorKind:         ErrorKindNotFound,
			Retryable:         false,
			RetryabilityKnown: true,
			DurationMs:        time.Since(start).Milliseconds(),
		}
	}

	// Valida que os argumentos são JSON válido
	args := json.RawMessage(call.Function.Arguments)
	if !json.Valid(args) {
		return ToolExecutionResult{
			CallID:   call.ID,
			ToolName: toolName,
			Result: ToolResult{
				Content: fmt.Sprintf("Argumentos inválidos para '%s': JSON malformado", toolName),
				IsError: true,
			},
			Error:             fmt.Errorf("argumentos inválidos para '%s': JSON malformado", toolName),
			ErrorKind:         ErrorKindInvalidArgs,
			Retryable:         false,
			RetryabilityKnown: true,
			DurationMs:        time.Since(start).Milliseconds(),
		}
	}

	// Cria contexto com timeout para esta execução
	toolCtx, cancel := context.WithTimeout(ctx, e.config.ToolTimeout)
	defer cancel()

	// Executa com recover para capturar panics
	resultCh := make(chan ToolExecutionResult, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				resultCh <- ToolExecutionResult{
					CallID:   call.ID,
					ToolName: toolName,
					Result: ToolResult{
						Content: fmt.Sprintf("Erro interno em '%s': %v", toolName, r),
						IsError: true,
					},
					Error:             fmt.Errorf("panic: %v", r),
					ErrorKind:         ErrorKindPanic,
					Retryable:         false,
					RetryabilityKnown: true,
					DurationMs:        time.Since(start).Milliseconds(),
				}
			}
		}()

		// Expõe à tool o limite efetivo de resultado deste executor, para que
		// tools com saída estruturada possam falhar de forma controlada em vez de
		// serem truncadas (o que invalidaria, p.ex., um JSON canônico).
		execCtx := WithMaxResultSize(toolCtx, e.config.MaxResultSize)
		result, err := tool.Execute(execCtx, args)
		if err != nil {
			errKind := ErrorKindUnknown
			retryable := false
			retryabilityKnown := false
			switch {
			case errors.Is(err, context.Canceled):
				errKind = ErrorKindCancelled
				retryabilityKnown = true
			case errors.Is(err, context.DeadlineExceeded) && toolCtx.Err() != nil:
				errKind = ErrorKindTimeout
				retryable = true
				retryabilityKnown = true
			case result.Failure != nil:
				errKind = result.Failure.Kind
				retryable = result.Failure.Retryable
				retryabilityKnown = true
			}
			if result.Content == "" {
				result.Content = fmt.Sprintf("Erro ao executar '%s': %v", toolName, err)
			}
			result.IsError = true
			resultCh <- ToolExecutionResult{
				CallID:            call.ID,
				ToolName:          toolName,
				Result:            result,
				Error:             err,
				ErrorKind:         errKind,
				ErrorCode:         failureCode(result),
				Retryable:         retryable,
				RetryabilityKnown: retryabilityKnown,
				DurationMs:        time.Since(start).Milliseconds(),
			}
			return
		}
		if result.Failure != nil {
			result.IsError = true
		}

		// Última barreira de tamanho. Structured e RawExact são integrais ou
		// falham; texto comum é preservado no store controlado e recebe apenas uma
		// prévia, com continuação estruturada nas anotações.
		var execErr error
		execKind := ErrorKindNone
		modelBytes := len(ContentForModel(result))
		if modelBytes > e.config.MaxResultSize {
			// json.Valid varre o payload inteiro; só precisamos inferir JSON
			// canônico quando a barreira realmente precisaria cortá-lo.
			structured := result.Structured
			if !structured && !result.RawExact {
				structured = looksLikeCanonicalJSON(result.Content)
			}
			mcpBridge := isMCPBridgeToolName(toolName)
			if (structured || result.RawExact) && !mcpBridge {
				// Falha classificada do executor (AEP-0039): preenche Error/ErrorKind
				// para que agent/service.go emita tool_failure e persista o error_kind.
				code := "result_too_large"
				label := "estruturado"
				guidance := "Reduza o escopo da chamada (ex.: max_results/max_items) para obter um payload menor."
				if result.RawExact {
					code = "raw_result_too_large"
					label = "raw"
					guidance = "Use offset/limit menores; conteúdo raw é exato e nunca é devolvido parcialmente."
				}
				message := fmt.Sprintf(
					"Resultado %s tem %d bytes, acima do limite de %d. %s",
					label, modelBytes, e.config.MaxResultSize, guidance,
				)
				result = ToolResult{
					Content: boundedFailureContent(message, code, e.config.MaxResultSize),
					IsError: true,
					Failure: &ToolFailure{
						Code:      code,
						Kind:      ErrorKindUnknown,
						Retryable: false,
					},
				}
				execErr = fmt.Errorf("saída %s de '%s' tem %d bytes model-facing, acima do limite de %d", label, toolName, modelBytes, e.config.MaxResultSize)
				execKind = ErrorKindUnknown
			} else {
				var protected ToolResult
				var stored bool
				if mcpBridge {
					protected, stored = ProtectExternalModelResult(result, e.config.MaxResultSize)
				} else {
					protected, stored = ProtectModelResult(result, e.config.MaxResultSize)
				}
				if !stored {
					result = ToolResult{
						Content: fmt.Sprintf(
							"Resultado tem %d bytes model-facing e excede a capacidade segura de preservação. Reduza o escopo da chamada.",
							modelBytes,
						),
						IsError: true,
						Failure: &ToolFailure{Code: "result_storage_limit", Kind: ErrorKindUnknown, Retryable: false},
					}
					execErr = fmt.Errorf("saída de '%s' excede armazenamento seguro: %d bytes model-facing", toolName, modelBytes)
					execKind = ErrorKindUnknown
				} else {
					result = protected
				}
			}
		}

		resultCh <- ToolExecutionResult{
			CallID:            call.ID,
			ToolName:          toolName,
			Result:            result,
			Error:             execErr,
			ErrorKind:         firstFailureKind(execKind, result),
			ErrorCode:         failureCode(result),
			Retryable:         failureRetryable(result),
			RetryabilityKnown: result.Failure != nil,
			DurationMs:        time.Since(start).Milliseconds(),
		}
	}()

	// Aguarda resultado ou timeout/cancelamento
	select {
	case result := <-resultCh:
		// Reclassifica: se a goroutine retornou um erro genérico mas o contexto
		// já foi cancelado/expirado, normaliza o ErrorKind e Result.Content para consistência.
		if result.Result.IsError && result.ErrorKind == ErrorKindUnknown && result.Result.Failure == nil {
			if ctx.Err() != nil {
				// Contexto pai cancelado — não é retryable
				result.ErrorKind = ErrorKindCancelled
				result.Retryable = false
				result.RetryabilityKnown = true
				result.Result.Content = fmt.Sprintf("Execução de '%s' cancelada pelo usuário", toolName)
				result.Error = ctx.Err()
			} else if toolCtx.Err() != nil {
				// Timeout da tool
				result.ErrorKind = ErrorKindTimeout
				result.Retryable = true
				result.RetryabilityKnown = true
				result.Result.Content = fmt.Sprintf("Timeout ao executar '%s' (limite: %s)", toolName, e.config.ToolTimeout)
				result.Error = context.DeadlineExceeded
			}
		}
		return result
	case <-toolCtx.Done():
		elapsed := time.Since(start).Milliseconds()
		if ctx.Err() != nil {
			// Contexto pai cancelado (usuário cancelou) — não é timeout
			return ToolExecutionResult{
				CallID:   call.ID,
				ToolName: toolName,
				Result: ToolResult{
					Content: fmt.Sprintf("Execução de '%s' cancelada pelo usuário", toolName),
					IsError: true,
				},
				Error:             ctx.Err(),
				ErrorKind:         ErrorKindCancelled,
				Retryable:         false,
				RetryabilityKnown: true,
				DurationMs:        elapsed,
			}
		}
		// Timeout da tool
		return ToolExecutionResult{
			CallID:   call.ID,
			ToolName: toolName,
			Result: ToolResult{
				Content: fmt.Sprintf("Timeout ao executar '%s' (limite: %s)", toolName, e.config.ToolTimeout),
				IsError: true,
			},
			Error:             context.DeadlineExceeded,
			ErrorKind:         ErrorKindTimeout,
			Retryable:         true,
			RetryabilityKnown: true,
			DurationMs:        elapsed,
		}
	}
}

func looksLikeCanonicalJSON(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	switch trimmed[0] {
	case '{', '[', '"', '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', 't', 'f', 'n':
	default:
		return false
	}
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	var value json.RawMessage
	if err := decoder.Decode(&value); err != nil {
		return false
	}
	var extra json.RawMessage
	return errors.Is(decoder.Decode(&extra), io.EOF)
}

func boundedFailureContent(message, code string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(message) <= maxBytes {
		return message
	}
	compact := "[" + code + "]"
	if len(compact) <= maxBytes {
		return compact
	}
	return truncateUTF8(compact, maxBytes)
}

func isMCPBridgeToolName(name string) bool {
	if !strings.HasPrefix(name, "mcp_") {
		return false
	}
	rest := strings.TrimPrefix(name, "mcp_")
	parts := strings.SplitN(rest, "__", 2)
	return len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != ""
}

func failureCode(result ToolResult) string {
	if result.Failure != nil {
		return result.Failure.Code
	}
	if code, ok := result.Metadata["error_code"].(string); ok {
		return code
	}
	return ""
}

func firstFailureKind(executorKind ErrorKind, result ToolResult) ErrorKind {
	if executorKind != ErrorKindNone {
		return executorKind
	}
	if result.Failure != nil {
		return result.Failure.Kind
	}
	if result.IsError {
		return ErrorKindUnknown
	}
	return ErrorKindNone
}

func failureRetryable(result ToolResult) bool {
	return result.Failure != nil && result.Failure.Retryable
}

func validateExecutionContextToolAccess(ctx context.Context, toolName string) error {
	ec, ok := GetExecutionContext(ctx)
	if !ok {
		return nil
	}
	if containsString(ec.DeniedTools, toolName) {
		return fmt.Errorf("tool '%s' bloqueada pela denylist do skill '%s'", toolName, ec.InvokedSkillSlug)
	}
	if len(ec.AllowedTools) > 0 && !containsString(ec.AllowedTools, toolName) {
		return fmt.Errorf("skill '%s' não permite uso da tool '%s'", ec.InvokedSkillSlug, toolName)
	}
	return nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// truncateUTF8 trunca uma string até maxBytes sem cortar runes no meio.
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	// Recua até achar um limite de rune válido
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}

// Config retorna a configuração atual do executor.
func (e *Executor) Config() ExecutorConfig {
	return e.config
}

// Registry expõe o registry associado ao executor.
// Útil para criar um executor derivado com configuração diferente, mantendo
// o mesmo conjunto de tools registradas.
func (e *Executor) Registry() *Registry {
	return e.registry
}
