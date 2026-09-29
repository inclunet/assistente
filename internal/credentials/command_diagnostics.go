package credentials

import (
	"context"
	"errors"
	"time"

	"assistente/internal/logging"
)

type commandDiagnosticKey struct{}
type commandDiagnostic struct {
	credentialID string
	cacheRef     string
	generation   uint64
	reason       string
}

func withCommandDiagnostic(ctx context.Context, dc *DomainCredential, generation uint64) context.Context {
	reason := "initial"
	if generation > 0 {
		reason = "http_401"
	}
	return context.WithValue(ctx, commandDiagnosticKey{}, commandDiagnostic{credentialID: dc.ID, cacheRef: dc.command.diagnosticID, generation: generation, reason: reason})
}

// Uma linha por execução real, sem logs de cache hit nem material/configuração secreta.
func logCommandExecution(ctx context.Context, started time.Time, err error) {
	diagnostic, ok := ctx.Value(commandDiagnosticKey{}).(commandDiagnostic)
	if !ok {
		diagnostic.reason = "direct"
	}
	outcome := "success"
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		outcome = "timeout"
	case errors.Is(err, context.Canceled):
		outcome = "canceled"
	case err != nil:
		outcome = "failure"
	}
	logging.Logger(ctx, "credentials.command").InfoContext(ctx, "credential_command_execution",
		"reason", diagnostic.reason, "outcome", outcome,
		"credential_id", diagnostic.credentialID, "cache_ref", diagnostic.cacheRef,
		"cache_generation", diagnostic.generation, "duration_ms", time.Since(started).Milliseconds())
}

func withDirectCommandDiagnostic(ctx context.Context, credentialID string) context.Context {
	return context.WithValue(ctx, commandDiagnosticKey{}, commandDiagnostic{credentialID: credentialID, reason: "direct"})
}
