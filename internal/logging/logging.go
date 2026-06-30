package logging

import (
	"context"
	"log/slog"

	"assistente/internal/eventctx"
	"assistente/internal/toolctx"
	"assistente/internal/tools/invocationctx"
	"assistente/internal/userctx"
)

type contextAttrsKey struct{}

// WithAttrs carimba atributos estruturados no contexto para logs posteriores.
func WithAttrs(ctx context.Context, attrs ...slog.Attr) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(attrs) == 0 {
		return ctx
	}
	existing, _ := ctx.Value(contextAttrsKey{}).([]slog.Attr)
	merged := make([]slog.Attr, 0, len(existing)+len(attrs))
	merged = append(merged, existing...)
	merged = append(merged, attrs...)
	return context.WithValue(ctx, contextAttrsKey{}, merged)
}

// Logger retorna um slog.Logger com component e atributos de correlacao do ctx.
func Logger(ctx context.Context, component string) *slog.Logger {
	attrs := make([]slog.Attr, 0, 8)
	if component != "" {
		attrs = append(attrs, slog.String("component", component))
	}
	attrs = append(attrs, ContextAttrs(ctx)...)
	return slog.Default().With(attrsToArgs(attrs)...)
}

// ContextAttrs extrai atributos canonicos de correlacao do context.Context.
func ContextAttrs(ctx context.Context) []slog.Attr {
	if ctx == nil {
		return nil
	}
	attrs := make([]slog.Attr, 0, 8)
	if userID, ok := userctx.UserIDFromContext(ctx); ok {
		attrs = append(attrs, slog.String("user_id", userID))
	}
	if inv, ok := invocationctx.Get(ctx); ok {
		appendStringAttr(&attrs, "conversation_id", inv.ConversationID)
		appendStringAttr(&attrs, "turn_id", inv.TurnID)
		appendStringAttr(&attrs, "profile_slug", inv.ProfileSlug)
		appendStringAttr(&attrs, "surface_type", inv.TabType)
	}
	if prov, ok := eventctx.From(ctx); ok {
		appendStringAttr(&attrs, "source", prov.Source)
		appendStringAttr(&attrs, "source_job_id", prov.SourceJobID)
		appendStringAttr(&attrs, "chain_id", prov.ChainID)
		if len(prov.ChainHistory) > 0 {
			attrs = append(attrs, slog.Int("chain_depth", len(prov.ChainHistory)))
		}
	}
	appendStringAttr(&attrs, "tool_invocation_id", toolctx.CurrentInvocationID(ctx))
	appendStringAttr(&attrs, "parent_tool_invocation_id", toolctx.ParentInvocationIDFromContext(ctx))
	if extra, _ := ctx.Value(contextAttrsKey{}).([]slog.Attr); len(extra) > 0 {
		attrs = append(attrs, extra...)
	}
	return attrs
}

func appendStringAttr(attrs *[]slog.Attr, key, value string) {
	if value != "" {
		*attrs = append(*attrs, slog.String(key, value))
	}
}

func attrsToArgs(attrs []slog.Attr) []any {
	args := make([]any, 0, len(attrs))
	for _, attr := range attrs {
		args = append(args, attr)
	}
	return args
}
