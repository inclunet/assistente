package messaging

import "context"

type channelTraceCtxKey struct{}

// WithChannelTraceID anexa o TraceID do turno de canal ao ctx para que
// NotifyContext só dispare callbacks daquele turno (evita que Notify atrasado
// de um turno supersedido remova/dispare o callback do turno novo).
func WithChannelTraceID(ctx context.Context, traceID string) context.Context {
	if ctx == nil || traceID == "" {
		return ctx
	}
	return context.WithValue(ctx, channelTraceCtxKey{}, traceID)
}

// ChannelTraceIDFromContext devolve o TraceID injetado por WithChannelTraceID.
func ChannelTraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(channelTraceCtxKey{}).(string)
	return v
}
