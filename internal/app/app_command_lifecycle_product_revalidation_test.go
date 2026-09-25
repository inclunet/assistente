package app

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/commandexecution"
)

func TestCommandPrincipalRevalidationFailurePreservesCancellation(t *testing.T) {
	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(want.Error(), func(t *testing.T) {
			if got := commandPrincipalRevalidationFailure(context.Background(), want); !errors.Is(got, want) {
				t.Fatalf("cancelamento traduzido para %v, want %v", got, want)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := commandPrincipalRevalidationFailure(ctx, errors.New("infraestrutura de sessão")); !errors.Is(got, context.Canceled) {
		t.Fatalf("ctx.Err perdido: got %v", got)
	}
	if got := commandPrincipalRevalidationFailure(context.Background(), errors.New("sessão não autenticada")); !errors.Is(got, commandexecution.ErrDenied) {
		t.Fatalf("erro de autenticação não foi fail-closed: got %v", got)
	}
}
