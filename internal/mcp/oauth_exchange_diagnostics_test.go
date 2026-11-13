package mcp

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func TestPKCEExchangeFailureDiagnosticDoesNotExposeResponse(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	secret := "sensitive-token-and-description"
	err := &oauth2.RetrieveError{
		Response:         &http.Response{StatusCode: http.StatusBadRequest},
		Body:             []byte(secret),
		ErrorCode:        "invalid_grant",
		ErrorDescription: secret,
	}
	logPKCEExchangeFailure(context.Background(), "server", err)
	got := output.String()
	for _, want := range []string{"oauth_code_exchange_failed", "cause=http", "http_status=400", "oauth_error=invalid_grant"} {
		if !strings.Contains(got, want) {
			t.Fatalf("diagnóstico sem %q: %s", want, got)
		}
	}
	if strings.Contains(got, secret) {
		t.Fatal("diagnóstico expôs conteúdo da resposta")
	}

	output.Reset()
	logPKCEExchangeFailure(context.Background(), "server", &oauth2.RetrieveError{ErrorCode: secret})
	if got := output.String(); strings.Contains(got, secret) || !strings.Contains(got, "oauth_error=other") {
		t.Fatalf("código OAuth não sanitizado: %s", got)
	}
	output.Reset()
	logPKCEExchangeFailure(context.Background(), "server", errors.Join(context.DeadlineExceeded, errors.New(secret)))
	if got := output.String(); strings.Contains(got, secret) || !strings.Contains(got, "cause=timeout") {
		t.Fatalf("diagnóstico de timeout inseguro ou incorreto: %s", got)
	}
}
