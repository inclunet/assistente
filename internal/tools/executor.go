package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
			ErrorKind:  ErrorKindNotFound,
			Retryable:  false,
			DurationMs: time.Since(start).Milliseconds(),
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
			ErrorKind:  ErrorKindInvalidArgs,
			Retryable:  false,
			DurationMs: time.Since(start).Milliseconds(),
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
					Error:      fmt.Errorf("panic: %v", r),
					ErrorKind:  ErrorKindPanic,
					Retryable:  false,
					DurationMs: time.Since(start).Milliseconds(),
				}
			}
		}()

		result, err := tool.Execute(toolCtx, args)
		if err != nil {
			// Detecta se o erro é um timeout (context deadline exceeded)
			errKind := ErrorKindUnknown
			retryable := false
			if errors.Is(err, context.DeadlineExceeded) && toolCtx.Err() != nil {
				errKind = ErrorKindTimeout
				retryable = true
			}
			resultCh <- ToolExecutionResult{
				CallID:   call.ID,
				ToolName: toolName,
				Result: ToolResult{
					Content: fmt.Sprintf("Erro ao executar '%s': %v", toolName, err),
					IsError: true,
				},
				Error:      err,
				ErrorKind:  errKind,
				Retryable:  retryable,
				DurationMs: time.Since(start).Milliseconds(),
			}
			return
		}

		// Trunca resultado se necessário (UTF-8 safe).
		// Reserva bytes para o aviso de truncamento, garantindo que
		// result.Content final ≤ MaxResultSize.
		if len(result.Content) > e.config.MaxResultSize {
			origSize := len(result.Content)
			warning := fmt.Sprintf(
				"\n\n[TRUNCADO: resultado original tinha %d bytes, limite é %d bytes]",
				origSize, e.config.MaxResultSize,
			)
			contentBudget := e.config.MaxResultSize - len(warning)
			if contentBudget >= 1 {
				result.Content = truncateUTF8(result.Content, contentBudget) + warning
			} else {
				// Warning não cabe — trunca sem aviso para respeitar o limite.
				result.Content = truncateUTF8(result.Content, e.config.MaxResultSize)
			}
			if result.Metadata == nil {
				result.Metadata = make(map[string]any)
			}
			result.Metadata["truncated"] = true
		}

		resultCh <- ToolExecutionResult{
			CallID:     call.ID,
			ToolName:   toolName,
			Result:     result,
			DurationMs: time.Since(start).Milliseconds(),
		}
	}()

	// Aguarda resultado ou timeout/cancelamento
	select {
	case result := <-resultCh:
		// Reclassifica: se a goroutine retornou um erro genérico mas o contexto
		// já foi cancelado/expirado, normaliza o ErrorKind e Result.Content para consistência.
		if result.Result.IsError && result.ErrorKind == ErrorKindUnknown {
			if ctx.Err() != nil {
				// Contexto pai cancelado — não é retryable
				result.ErrorKind = ""
				result.Retryable = false
				result.Result.Content = fmt.Sprintf("Execução de '%s' cancelada pelo usuário", toolName)
				result.Error = ctx.Err()
			} else if toolCtx.Err() != nil {
				// Timeout da tool
				result.ErrorKind = ErrorKindTimeout
				result.Retryable = true
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
				Error:      ctx.Err(),
				ErrorKind:  "",
				Retryable:  false,
				DurationMs: elapsed,
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
			Error:      context.DeadlineExceeded,
			ErrorKind:  ErrorKindTimeout,
			Retryable:  true,
			DurationMs: elapsed,
		}
	}
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
