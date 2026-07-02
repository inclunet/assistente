package tools

import "context"

// ExecutionContext carrega metadados da origem do toolcalling (ex: skill invocado via /slash).
// Ele é propagado via context.Context até as Tool.Execute(...) para enforcement de permissões.
type ExecutionContext struct {
	InvokedSkillSlug   string
	Filesystem         *FilesystemScope
	AllowedTools       []string
	DeniedTools        []string
	AllowedBash        []string
	DeniedBash         []string
	NetworkAllowedHost []string
	NetworkDeniedHost  []string
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

type toolCatalogVisibilityKey struct{}

// WithToolCatalogVisibleNames restringe a descoberta do tool_catalog aos nomes
// visíveis pela política efetiva do perfil. nil significa sem restrição; slice
// vazio significa nenhuma tool visível.
func WithToolCatalogVisibleNames(ctx context.Context, names []string) context.Context {
	return context.WithValue(ctx, toolCatalogVisibilityKey{}, names)
}

func ToolCatalogVisibleNamesFromContext(ctx context.Context) ([]string, bool) {
	v := ctx.Value(toolCatalogVisibilityKey{})
	names, ok := v.([]string)
	return names, ok
}

type toolCatalogRuntimeKey struct{}

type ToolCatalogRuntime struct {
	Store          *LoadedToolStore
	ConversationID string
	ProfileSlug    string
	VisibleNames   []string
	PreloadedNames []string
	ControlPlane   []string
}

func WithToolCatalogRuntime(ctx context.Context, runtime ToolCatalogRuntime) context.Context {
	return context.WithValue(ctx, toolCatalogRuntimeKey{}, runtime)
}

func ToolCatalogRuntimeFromContext(ctx context.Context) (ToolCatalogRuntime, bool) {
	v := ctx.Value(toolCatalogRuntimeKey{})
	runtime, ok := v.(ToolCatalogRuntime)
	return runtime, ok
}

type maxResultSizeKey struct{}

// WithMaxResultSize injeta no ctx o limite efetivo de tamanho de resultado que o
// executor aplicará a esta execução (ele varia por chamador: jobs usam um budget
// maior que o default). É chamado pelo executor antes de Tool.Execute.
func WithMaxResultSize(ctx context.Context, size int) context.Context {
	return context.WithValue(ctx, maxResultSizeKey{}, size)
}

// MaxResultSizeFromContext retorna o limite efetivo de resultado injetado pelo
// executor. Tools que produzem saída estruturada (ex.: JSON canônico) podem usá-lo
// para falhar de forma controlada em vez de serem truncadas para um payload
// inválido. Quando ausente (ex.: tool chamada fora do executor) retorna
// DefaultMaxResultSize.
func MaxResultSizeFromContext(ctx context.Context) int {
	if v, ok := ctx.Value(maxResultSizeKey{}).(int); ok && v > 0 {
		return v
	}
	return DefaultMaxResultSize
}
