package oauthflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRefreshDiagnosticsAreSafeAndPreservePolicy(t *testing.T) {
	const secret = "SECRET-MUST-NOT-APPEAR"
	success := `{"access_token":"` + secret + `","refresh_token":"rotated-secret","token_type":"Bearer","expires_in":3600}`
	for _, tc := range []struct {
		name                     string
		status                   int
		body, stage, cause, code string
		failRevision             uint64
		transportError           error
		bodyError                error
		want                     error
		calls                    int
	}{
		{name: "success", status: 200, body: success, stage: "complete", cause: "none", code: "none", calls: 1},
		{name: "invalid grant", status: 400, body: `{"error":"invalid_grant","error_description":"` + secret + `"}`, stage: "exchange", cause: "http_rejected", code: "invalid_grant", want: ErrReauthorize, calls: 1},
		{name: "unknown remote error", status: 400, body: `{"error":"` + secret + `","error_description":"` + secret + `"}`, stage: "exchange", cause: "http_rejected", code: "other", want: ErrReauthorize, calls: 1},
		{name: "rate limited", status: 429, body: `{"error":"temporarily_unavailable"}`, stage: "exchange", cause: "http_rejected", code: "temporarily_unavailable", want: ErrTransient, calls: 1},
		{name: "server failure", status: 503, body: secret, stage: "exchange", cause: "http_rejected", code: "none", want: ErrTransient, calls: 1},
		{name: "malformed response", status: 200, body: secret, stage: "exchange", cause: "invalid_response", code: "none", want: ErrReauthorize, calls: 1},
		{name: "invalid token response", status: 200, body: `{"access_token":"` + secret + `","token_type":"invalid"}`, stage: "exchange", cause: "invalid_response", code: "none", want: ErrReauthorize, calls: 1},
		{name: "transport", transportError: errors.New(secret), stage: "exchange", cause: "transport", code: "none", want: ErrTransient, calls: 0},
		{name: "timeout", transportError: fmt.Errorf("%s: %w", secret, context.DeadlineExceeded), stage: "exchange", cause: "timeout", code: "none", want: ErrTransient, calls: 0},
		{name: "canceled", transportError: fmt.Errorf("%s: %w", secret, context.Canceled), stage: "exchange", cause: "canceled", code: "none", want: ErrTransient, calls: 0},
		{name: "body timeout", status: 200, bodyError: fmt.Errorf("%s: %w", secret, context.DeadlineExceeded), stage: "exchange", cause: "timeout", code: "none", want: ErrReauthorize},
		{name: "body canceled", status: 200, bodyError: fmt.Errorf("%s: %w", secret, context.Canceled), stage: "exchange", cause: "canceled", code: "none", want: ErrReauthorize},
		{name: "socket timeout", transportError: fmt.Errorf("%s: %w", secret, os.ErrDeadlineExceeded), stage: "exchange", cause: "timeout", code: "none", want: ErrTransient},
		{name: "body socket timeout", status: 200, bodyError: fmt.Errorf("%s: %w", secret, os.ErrDeadlineExceeded), stage: "exchange", cause: "timeout", code: "none", want: ErrReauthorize},
		{name: "rejection body timeout", status: 400, bodyError: context.DeadlineExceeded, stage: "exchange", cause: "timeout", code: "none", want: ErrReauthorize},
		{name: "rejection body canceled", status: 400, bodyError: context.Canceled, stage: "exchange", cause: "canceled", code: "none", want: ErrReauthorize},
		{name: "server error body socket timeout", status: 503, bodyError: os.ErrDeadlineExceeded, stage: "exchange", cause: "timeout", code: "none", want: ErrTransient},
		{name: "pending write", status: 200, body: success, failRevision: 2, stage: "persist_pending", cause: "storage", code: "none", calls: 0},
		{name: "token write", status: 200, body: success, failRevision: 3, stage: "persist_tokens", cause: "storage", code: "none", want: ErrReauthorize, calls: 1},
		{name: "rejection write", status: 400, body: `{"error":"invalid_grant"}`, failRevision: 3, stage: "persist_rejection", cause: "storage", code: "none", want: ErrReauthorize, calls: 1},
		{name: "token validation", status: 200, body: `{"access_token":"` + secret + `","token_type":"Bearer","earliest_refresh_at":"invalid"}`, stage: "validate_tokens", cause: "token_validation", code: "none", want: ErrReauthorize, calls: 1},
		{name: "permission missing", status: 200, body: `{"access_token":"` + secret + `","token_type":"Bearer","scope":"other"}`, stage: "validate_permissions", cause: "permission", code: "none", want: ErrPermission, calls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
			defer slog.SetDefault(previous)
			var calls atomic.Int32
			service, store, server := fixture(t, func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			})
			store.failRevision = tc.failRevision
			if tc.transportError != nil {
				service.HTTP.Transport = resourceTestRT(func(*http.Request) (*http.Response, error) { return nil, tc.transportError })
			}
			if tc.bodyError != nil {
				service.HTTP.Transport = resourceTestRT(func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: tc.status, Body: io.NopCloser(diagnosticErrorReader{tc.bodyError}), Header: make(http.Header)}, nil
				})
			}
			_, err := service.Resolve(context.Background(), store, store.r.ID, store.r.Resource, "")
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("sentinela perdida: %v", err)
			}
			if (err == nil) != (tc.name == "success") {
				t.Fatalf("resultado inesperado: %v", err)
			}
			if calls.Load() != int32(tc.calls) {
				t.Fatalf("chamadas=%d, esperado %d", calls.Load(), tc.calls)
			}
			var entry map[string]any
			if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &entry); err != nil {
				t.Fatalf("log: %v: %s", err, output.String())
			}
			for key, want := range map[string]any{"msg": "oauth_refresh", "stage": tc.stage, "cause": tc.cause, "oauth_error": tc.code, "reason": "expiry"} {
				if entry[key] != want {
					t.Errorf("%s=%v esperado %v", key, entry[key], want)
				}
			}
			outcome := "failure"
			if tc.name == "success" {
				outcome = "success"
			}
			if entry["outcome"] != outcome {
				t.Errorf("outcome=%v esperado %s", entry["outcome"], outcome)
			}
			status := 0
			if tc.stage == "exchange" {
				status = tc.status
			}
			if entry["http_status"] != float64(status) {
				t.Errorf("http_status=%v esperado %d", entry["http_status"], status)
			}
			for _, sensitive := range []string{secret, "old-access", "old-refresh", "rotated-secret", server.URL} {
				if strings.Contains(output.String(), sensitive) {
					t.Fatalf("segredo no log: %s", sensitive)
				}
			}
			// An ambiguous or rejected rotation still cannot cause an implicit retry.
			if tc.stage == "exchange" && err != nil || tc.stage == "persist_tokens" || tc.stage == "validate_tokens" {
				_, again := service.Resolve(context.Background(), store, store.r.ID, store.r.Resource, "")
				wantAgain := ErrReauthorize
				if tc.stage == "persist_tokens" {
					wantAgain = ErrTransient
				}
				if !errors.Is(again, wantAgain) || calls.Load() != int32(tc.calls) {
					t.Fatalf("retry indevido: %v, calls=%d", again, calls.Load())
				}
			}
		})
	}
}

type diagnosticErrorReader struct{ err error }

func (r diagnosticErrorReader) Read([]byte) (int, error) { return 0, r.err }
