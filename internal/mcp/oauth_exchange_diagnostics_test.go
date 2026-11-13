package mcp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"assistente/internal/oauthflow"
	"golang.org/x/oauth2"
)

func TestPKCECallbackDoesNotClaimSuccessBeforeTokenExchange(t *testing.T) {
	for _, tc := range []struct{ name, key, value string }{
		{"código", "code", "private-code"},
		{"recusa", "error", "access_denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			callback, err := oauthflow.ReserveCallback(oauthflow.CallbackConfig{Host: "localhost", Path: "/callback", PortPolicy: "ephemeral"})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(callback.Close)
			if err := callback.Start("state", oauthflow.CallbackPage{Success: authCallbackHTML, Failure: authCallbackHTML, ContentType: "text/html; charset=utf-8", HTMLNonce: true}); err != nil {
				t.Fatal(err)
			}
			query := url.Values{"state": {"state"}, tc.key: {tc.value}}
			client := &http.Client{Timeout: 2 * time.Second}
			response, err := client.Get(callback.RedirectURI() + "?" + query.Encode())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = response.Body.Close() }()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			text := string(body)
			if response.StatusCode != http.StatusOK || !strings.Contains(text, `lang="en"`) || !strings.Contains(text, "document.documentElement.lang=language") ||
				!strings.Contains(text, "Retorno recebido") || !strings.Contains(text, "Return received") || !strings.Contains(text, "Retorno recibido") || !strings.Contains(text, "window.close()") {
				t.Fatal("callback não apresentou retorno transitório")
			}
			if strings.Contains(text, "Autorização concluída") || strings.Contains(text, "private-code") {
				t.Fatal("callback afirmou sucesso prematuro ou expôs o código")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			result, err := callback.Wait(ctx)
			if err != nil || result.Get(tc.key) != tc.value {
				t.Fatal("callback não entregou o resultado ao aplicativo")
			}
		})
	}
}

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
