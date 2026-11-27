package tools

import (
	"context"
	"errors"
)

// A goroutine do hook nunca recebe a ferramenta. Uma resposta atrasada não pode despachá-la.
func awaitBeforeTool(ctx context.Context, before func(context.Context, ToolCall) error, call ToolCall) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() {
		defer func() {
			if recover() != nil {
				done <- errors.New("falha interna no hook anterior")
			}
		}()
		if err := ctx.Err(); err != nil {
			done <- err
			return
		}
		done <- before(ctx, call)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		if err != nil {
			return err
		}
		return ctx.Err()
	}
}
