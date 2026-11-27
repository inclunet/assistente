package filesystem

import (
	"context"
	"errors"
	"time"

	"assistente/internal/tools"
)

var errSearchBudget = errors.New("tempo de varredura esgotado")

// O tempo de decisão sobre o diretório não consome o orçamento da varredura.
func startSearchBudget(parent context.Context) (context.Context, func(*tools.ToolResult, *error)) {
	ctx, cancel := context.WithTimeoutCause(parent, 30*time.Second, errSearchBudget)
	return ctx, func(result *tools.ToolResult, err *error) {
		defer cancel()
		if errors.Is(context.Cause(ctx), errSearchBudget) {
			*err = nil
			*result = tools.ToolResult{IsError: true, Content: "A busca atingiu 30 segundos. Restrinja o diretório ou o padrão antes de tentar novamente.", Failure: &tools.ToolFailure{Kind: tools.ErrorKindTimeout, Code: "search_budget_exhausted"}}
		}
	}
}
