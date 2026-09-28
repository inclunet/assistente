package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/commandruntime"
	"assistente/internal/commandsecurity"
	"assistente/internal/logging"
	"gorm.io/gorm"
)

// Diagnóstico de leitura, nunca de pressionamento/execução. Não propaga
// atributos do contexto de usuário nem imprime Error(), SQL, argumentos ou IDs.
type commandLoadTraceKey struct{}

var commandLoadSequence atomic.Uint64

type commandLoadTrace struct {
	mu           sync.Mutex
	logger       *slog.Logger
	id           uint64
	operation    string
	currentStage string
	started      time.Time
	cause        error
	finished     bool
	timer        *time.Timer
}

func beginCommandLoad(ctx context.Context, operation string) (context.Context, *commandLoadTrace) {
	if ctx == nil {
		ctx = context.Background()
	}
	t := &commandLoadTrace{logger: logging.Logger(context.Background(), "app.command-load"), id: commandLoadSequence.Add(1), operation: operation, currentStage: "entry", started: time.Now()}
	t.timer = time.AfterFunc(3*time.Second, t.slow)
	return context.WithValue(ctx, commandLoadTraceKey{}, t), t
}

func commandLoadStage(ctx context.Context, stage string) {
	if ctx == nil {
		return
	}
	if t, ok := ctx.Value(commandLoadTraceKey{}).(*commandLoadTrace); ok {
		t.stage(stage)
	}
}

// Guarda a causa antes de a fronteira pública converter o erro em uma recusa
// genérica. Só códigos/tipos entram no log, nunca o texto arbitrário do erro.
func commandLoadCause(ctx context.Context, err error) {
	if ctx == nil || err == nil {
		return
	}
	if t, ok := ctx.Value(commandLoadTraceKey{}).(*commandLoadTrace); ok {
		t.mu.Lock()
		defer t.mu.Unlock()
		if t.cause == nil {
			t.cause = err
		}
	}
}

func (t *commandLoadTrace) stage(stage string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished {
		return
	}
	t.currentStage = stage
}

func (t *commandLoadTrace) slow() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.finished {
		t.write(slog.LevelWarn, "slow", nil)
	}
}

func (t *commandLoadTrace) finish(err error, counts ...slog.Attr) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished {
		return
	}
	t.finished = true
	if t.timer != nil {
		t.timer.Stop()
	}
	if err == nil {
		t.write(slog.LevelInfo, "succeeded", nil, counts...)
		return
	}
	if t.cause != nil {
		err = t.cause
	}
	t.write(slog.LevelWarn, "failed", err)
}

func (t *commandLoadTrace) write(level slog.Level, status string, err error, counts ...slog.Attr) {
	attrs := []slog.Attr{slog.Uint64("load_id", t.id), slog.String("operation", t.operation), slog.String("stage", t.currentStage), slog.String("status", status), slog.Int64("elapsed_ms", time.Since(t.started).Milliseconds())}
	if err != nil {
		attrs = append(attrs, slog.String("error_class", commandLoadErrorClass(err)), slog.Any("error_types", commandLoadErrorTypes(err)))
		var coded interface{ Code() int }
		if errors.As(err, &coded) {
			attrs = append(attrs, slog.Int("error_code", coded.Code()))
		}
	}
	attrs = append(attrs, counts...)
	t.logger.LogAttrs(context.Background(), level, "Command load", attrs...)
}

func commandLoadErrorClass(err error) string {
	for _, item := range []struct {
		err  error
		name string
	}{
		{context.DeadlineExceeded, "deadline"}, {context.Canceled, "cancelled"},
		{auth.ErrSessionExpired, "session_expired"}, {auth.ErrSessionRevoked, "session_revoked"},
		{auth.ErrUnauthenticatedLocalSession, "unauthenticated"}, {auth.ErrActiveUserNotFound, "active_user_missing"},
		{auth.ErrInactiveUser, "inactive_user"},
		{commandsecurity.ErrStaleEpoch, "stale_epoch"}, {commandsecurity.ErrDrainInProgress, "draining"},
		{commandsecurity.ErrDrainFailed, "drain_failed"},
		{commandruntime.ErrNotReady, "not_ready"}, {commandruntime.ErrStopped, "stopped"},
		{commandexecution.ErrDenied, "denied"}, {commandexecution.ErrStale, "stale"},
		{commandconfig.ErrStale, "configuration_stale"}, {gorm.ErrRecordNotFound, "not_found"},
		{commandexecution.ErrInvalidConfiguration, "invalid_configuration"},
		{commandexecution.ErrInvalidRequest, "invalid_request"}, {commandconfig.ErrInvalid, "invalid_configuration"},
	} {
		if errors.Is(err, item.err) {
			return item.name
		}
	}
	var coded interface{ Code() int }
	if errors.As(err, &coded) {
		if coded.Code()&255 == 5 || coded.Code()&255 == 6 {
			return "sqlite_busy"
		}
		return "database_error"
	}
	return "other"
}

func commandLoadErrorTypes(err error) []string {
	types := []string{}
	var visit func(error)
	visit = func(e error) {
		if e == nil || len(types) >= 8 {
			return
		}
		types = append(types, fmt.Sprintf("%T", e))
		if joined, ok := e.(interface{ Unwrap() []error }); ok {
			for _, inner := range joined.Unwrap() {
				visit(inner)
			}
		} else {
			visit(errors.Unwrap(e))
		}
	}
	visit(err)
	return types
}
