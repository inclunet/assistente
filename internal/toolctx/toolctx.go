package toolctx

import "context"

type currentInvocationIDKey struct{}

type parentInvocationIDKey struct{}

// WithCurrentInvocationID returns a context carrying the current tool invocation.
func WithCurrentInvocationID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, currentInvocationIDKey{}, id)
}

// CurrentInvocationID extracts the current tool invocation from ctx.
func CurrentInvocationID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(currentInvocationIDKey{}).(string)
	return id
}

// WithParentInvocationID returns a context carrying the parent tool invocation.
func WithParentInvocationID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, parentInvocationIDKey{}, id)
}

// ParentInvocationIDFromContext extracts the parent tool invocation from ctx.
func ParentInvocationIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(parentInvocationIDKey{}).(string)
	return id
}
