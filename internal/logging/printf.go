package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"unicode"
)

// Logf records a formatted message through slog while preserving contextual attrs.
func Logf(ctx context.Context, level slog.Level, component string, format string, args ...any) {
	if ctx == nil {
		ctx = context.Background()
	}
	Logger(ctx, component).Log(ctx, level, normalizeLegacyMessage(fmt.Sprintf(format, args...)))
}

// Printf is the migration bridge for legacy formatted log call sites.
func Printf(ctx context.Context, component string, format string, args ...any) {
	Logf(ctx, slog.LevelInfo, component, format, args...)
}

// Debugf records a debug formatted message.
func Debugf(ctx context.Context, component string, format string, args ...any) {
	Logf(ctx, slog.LevelDebug, component, format, args...)
}

// Infof records an info formatted message.
func Infof(ctx context.Context, component string, format string, args ...any) {
	Logf(ctx, slog.LevelInfo, component, format, args...)
}

// Warnf records a warning formatted message.
func Warnf(ctx context.Context, component string, format string, args ...any) {
	Logf(ctx, slog.LevelWarn, component, format, args...)
}

// Errorf records an error formatted message.
func Errorf(ctx context.Context, component string, format string, args ...any) {
	Logf(ctx, slog.LevelError, component, format, args...)
}

// Print records arguments with fmt.Sprint semantics through slog.
func Print(ctx context.Context, component string, args ...any) {
	Logf(ctx, slog.LevelInfo, component, "%s", fmt.Sprint(args...))
}

// Println records arguments with fmt.Sprintln semantics through slog.
func Println(ctx context.Context, component string, args ...any) {
	Logf(ctx, slog.LevelInfo, component, "%s", fmt.Sprintln(args...))
}

// Fatalf records an error and exits with status 1, matching standard fatal semantics.
func Fatalf(ctx context.Context, component string, format string, args ...any) {
	Logf(ctx, slog.LevelError, component, format, args...)
	os.Exit(1)
}

func normalizeLegacyMessage(message string) string {
	message = strings.TrimSpace(message)
	for strings.HasPrefix(message, "[") {
		end := strings.Index(message, "]")
		if end <= 0 || end > 48 {
			break
		}
		message = strings.TrimSpace(message[end+1:])
	}
	message = strings.TrimLeftFunc(message, func(r rune) bool {
		return unicode.IsSymbol(r) || r == ':' || r == '-' || unicode.IsSpace(r)
	})
	return strings.TrimSpace(message)
}
