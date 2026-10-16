package commandexecution

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/auth"
)

type externalPanicVerifier struct{}

func (externalPanicVerifier) Validate(context.Context, string) (*auth.ExternalClaims, error) {
	panic("falha sintética no verificador")
}

func TestExternalWarmPanicDoesNotLeakLifecycleOperation(t *testing.T) {
	h := newExternalHarness(t, nil)
	broken := auth.NewExternalCommandAuthenticator(externalPanicVerifier{}, h.repository)
	broken.SetReadiness(h.repository.CheckReadiness)
	// Exercita a preparação de rede, antes do gate e de qualquer captura.
	h.service.authenticator = broken
	if _, err := h.service.ExecuteEnvelope(context.Background(), "token-a", externalCandidate()); !errors.Is(err, ErrDenied) {
		t.Fatalf("panic de autenticação não falhou fechado: %v", err)
	}
	if h.fixture.starts.Load() != 0 {
		t.Fatal("handler iniciado após panic de autenticação")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := h.service.Shutdown(ctx); err != nil {
		t.Fatalf("preparação deixou operação ativa: %v", err)
	}
}
