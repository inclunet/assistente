package tools

import "context"

// ExecutionContext carrega metadados da origem do toolcalling (ex: skill invocado via /slash).
// Ele é propagado via context.Context até as Tool.Execute(...) para enforcement de permissões.
type ExecutionContext struct {
	InvokedSkillSlug string
	Filesystem       *FilesystemScope
}

// FilesystemScope define allowlist/denylist de caminhos (glob) para ferramentas de filesystem.
// Quando presente, as ferramentas devem respeitar este escopo além das validações padrão.
type FilesystemScope struct {
	Read  []string
	Write []string
	Deny  []string
}

type executionContextKey struct{}

// WithExecutionContext injeta o ExecutionContext no ctx.
func WithExecutionContext(ctx context.Context, ec ExecutionContext) context.Context {
	return context.WithValue(ctx, executionContextKey{}, ec)
}

// GetExecutionContext retorna o ExecutionContext do ctx, se existir.
func GetExecutionContext(ctx context.Context) (ExecutionContext, bool) {
	v := ctx.Value(executionContextKey{})
	ec, ok := v.(ExecutionContext)
	return ec, ok
}
