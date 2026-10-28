package oauthflow

import (
	"context"
	"errors"
	"time"

	"assistente/internal/logging"
)

// exchangeFailure retains only bounded diagnostic metadata, never response
// bodies, URLs, transport messages or provider-supplied descriptions.
type exchangeFailure struct {
	err      error
	category string
	status   int
	code     string
}

func (e *exchangeFailure) Error() string { return e.err.Error() }
func (e *exchangeFailure) Unwrap() error { return e.err }

func exchangeErrorCategory(err error, fallback string) string {
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return fallback
}

func safeOAuthErrorCode(code string) string {
	switch code {
	case "invalid_client", "invalid_scope", "invalid_grant", "invalid_refresh_token",
		"token_expired", "refresh_token_expired", "refresh_token_invalidated", "refresh_token_reused",
		"invalid_request", "unsupported_grant_type", "unauthorized_client", "access_denied",
		"server_error", "temporarily_unavailable":
		return code
	case "":
		return "none"
	default:
		return "other"
	}
}

func logRefreshResult(ctx context.Context, r Record, reason, stage string, started time.Time, err error) {
	outcome, cause, status, code := "success", "none", 0, "none"
	if err != nil {
		outcome, cause = "failure", "other"
		var failure *exchangeFailure
		switch {
		case errors.As(err, &failure):
			cause, status = failure.category, failure.status
			if failure.code != "" {
				code = failure.code
			}
		case errors.Is(err, context.Canceled):
			cause = "canceled"
		case errors.Is(err, context.DeadlineExceeded):
			cause = "timeout"
		case errors.Is(err, ErrConflict):
			cause = "conflict"
		case stage == "persist_pending" || stage == "persist_tokens" || stage == "persist_rejection":
			cause = "storage"
		case errors.Is(err, ErrPermission):
			cause = "permission"
		case stage == "validate_tokens":
			cause = "token_validation"
		case errors.Is(err, ErrResource):
			cause = "resource"
		case errors.Is(err, ErrTransient):
			cause = "transient"
		}
	}
	logging.Logger(ctx, "oauthflow").Info("oauth_refresh",
		"credential_id", r.ID, "integration", r.Integration, "reason", reason,
		"duration_ms", time.Since(started).Milliseconds(), "outcome", outcome,
		"stage", stage, "cause", cause, "http_status", status, "oauth_error", code)
}
