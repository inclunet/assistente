package logging

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"strings"
)

const toolLedgerLogLevelFlag = "--tool-ledger-log-level"

const LevelTrace = slog.LevelDebug - 4

var (
	ErrLogLevelRequired = errors.New("--tool-ledger-log-level requer info, debug ou trace")
	ErrLogLevelRepeated = errors.New("--tool-ledger-log-level foi informado mais de uma vez")
)

// ParseToolLedgerLogLevelArgs extrai o nível de diagnóstico do ledger sem
// expor a flag ao Wails. O default info evita ruído; debug e trace precisam de
// opt-in explícito e nunca elevam outros componentes.
func ParseToolLedgerLogLevelArgs(args []string) (string, []string, error) {
	remaining := make([]string, 0, len(args))
	level := ""
	for index := 0; index < len(args); index++ {
		arg := args[index]
		var value string
		switch {
		case arg == toolLedgerLogLevelFlag:
			if index+1 >= len(args) {
				return "", nil, ErrLogLevelRequired
			}
			index++
			value = args[index]
		case strings.HasPrefix(arg, toolLedgerLogLevelFlag+"="):
			value = strings.TrimPrefix(arg, toolLedgerLogLevelFlag+"=")
		default:
			remaining = append(remaining, arg)
			continue
		}
		if level != "" {
			return "", nil, ErrLogLevelRepeated
		}
		value = strings.ToLower(strings.TrimSpace(value))
		if _, err := ParseLevel(value); err != nil {
			return "", nil, err
		}
		level = value
	}
	if level == "" {
		level = "info"
	}
	return level, remaining, nil
}

func ParseLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "trace":
		return LevelTrace, nil
	default:
		return 0, fmt.Errorf("%w: %q", ErrLogLevelRequired, value)
	}
}

// ConfigureToolLedgerLevel instala um handler estruturado que eleva somente os
// componentes do ledger. Outros componentes permanecem em info, inclusive
// quando trace é solicitado, evitando ampliar logs sensíveis preexistentes.
func ConfigureToolLedgerLevel(value string) error {
	level, err := ParseLevel(value)
	if err != nil {
		return err
	}
	base := slog.NewTextHandler(log.Writer(), &slog.HandlerOptions{Level: LevelTrace})
	slog.SetDefault(slog.New(&componentLevelHandler{next: base, ledgerLevel: level}))
	return nil
}

type componentLevelHandler struct {
	next        slog.Handler
	ledgerLevel slog.Level
	component   string
}

func (h *componentLevelHandler) Enabled(ctx context.Context, level slog.Level) bool {
	if level >= slog.LevelInfo {
		return h.next.Enabled(ctx, level)
	}
	return isToolLedgerComponent(h.component) && level >= h.ledgerLevel
}

func (h *componentLevelHandler) Handle(ctx context.Context, record slog.Record) error {
	return h.next.Handle(ctx, record)
}

func (h *componentLevelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	component := h.component
	for _, attr := range attrs {
		if attr.Key == "component" {
			component = attr.Value.String()
		}
	}
	return &componentLevelHandler{
		next:        h.next.WithAttrs(attrs),
		ledgerLevel: h.ledgerLevel,
		component:   component,
	}
}

func (h *componentLevelHandler) WithGroup(name string) slog.Handler {
	return &componentLevelHandler{
		next:        h.next.WithGroup(name),
		ledgerLevel: h.ledgerLevel,
		component:   h.component,
	}
}

func isToolLedgerComponent(component string) bool {
	return component == "toolinvocations" ||
		strings.HasPrefix(component, "toolinvocations.") ||
		component == "database.tool-ledger" ||
		strings.HasPrefix(component, "database.tool-ledger.")
}

// Tracef registra diagnóstico de alto volume abaixo de debug.
func Tracef(ctx context.Context, component string, format string, args ...any) {
	Logf(ctx, LevelTrace, component, format, args...)
}
