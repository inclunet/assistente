package tools

import (
	"context"
	"errors"
)

// A goroutine do hook nunca recebe a ferramenta. Uma resposta atrasada não pode despachá-la.
func awaitToolHook(ctx context.Context, run func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() {
		if err := ctx.Err(); err != nil {
			done <- err
			return
		}
		done <- callToolHook(run)
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

func callToolHook(run func() error) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("falha interna no hook")
		}
	}()
	return run()
}
