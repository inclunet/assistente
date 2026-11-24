package invocationctx

import (
	"context"
	"crypto/sha256"
)

// InvocationContext carrega informações sobre o contexto de onde a tool foi invocada.
// É propagado via context.Context pelo agentic loop.
type InvocationContext struct {
	TabType        string // "editor", "chat", etc.
	ActiveFilePath string // caminho absoluto do arquivo ativo (apenas para editor tabs)
	SurfaceState   map[string]any
	SurfaceContext map[string]any

	// Identidade da conversa/turno corrente (AEP-0068). Permite que a tool
	// `subagent` saiba qual é a conversa-pai e o turno que a invocou, e qual
	// profile o pai está usando (para herança de profile por padrão).
	ConversationID string
	TurnID         string
	ProfileSlug    string
	Source         string
	SurfaceTabID   string
}

type ctxKey struct{}
type workspaceKey struct{}
type persistentShellKey struct{}
type shellCommandMetadataKey struct{}
type protectedDirectoryKey struct{}

// WithShellDirectoryMetadata transporta a representação pública de um caminho protegido.
func WithShellDirectoryMetadata(ctx context.Context, display string) context.Context {
	return context.WithValue(ctx, protectedDirectoryKey{}, display)
}

// ShellDirectoryMetadata devolve a projeção pública se o diretório for protegido.
func ShellDirectoryMetadata(ctx context.Context) string {
	display, _ := ctx.Value(protectedDirectoryKey{}).(string)
	return display
}

type protectedShellCommand struct {
	metadata     string
	prefix       string
	template     string
	resolvedHash [sha256.Size]byte
}

func ShellConsentPrefix(ctx context.Context) string {
	command, _ := ctx.Value(shellCommandMetadataKey{}).(protectedShellCommand)
	return command.prefix
}

// WithShellCommandMetadata marca o comando como protegido. O texto público vale para
// o histórico; uma confirmação oculta o comando resolvido e só admite aprovação pontual.
func WithShellCommandMetadata(ctx context.Context, command string) context.Context {
	return WithProtectedShellCommand(ctx, command, "")
}

// WithProtectedShellCommand agrupa a representação do histórico e o primeiro
// token literal do template, quando conhecido antes da resolução.
func WithProtectedShellCommand(ctx context.Context, metadata, prefix string) context.Context {
	return context.WithValue(ctx, shellCommandMetadataKey{}, protectedShellCommand{metadata: metadata, prefix: prefix})
}

// WithShellConsentTemplate vincula a fonte pública verificada ao despacho
// que a resolveu. O questionário nunca recebe o valor resolvido nem seu hash.
func WithShellConsentTemplate(ctx context.Context, source, resolved string) context.Context {
	command, ok := ctx.Value(shellCommandMetadataKey{}).(protectedShellCommand)
	if !ok {
		return ctx
	}
	command.template, command.resolvedHash = source, sha256.Sum256([]byte(resolved))
	return context.WithValue(ctx, shellCommandMetadataKey{}, command)
}

// ShellConsentTemplate retorna a fonte somente para o comando que a originou.
func ShellConsentTemplate(ctx context.Context, resolved string) string {
	command, _ := ctx.Value(shellCommandMetadataKey{}).(protectedShellCommand)
	if command.template != "" && command.resolvedHash == sha256.Sum256([]byte(resolved)) {
		return command.template
	}
	return ""
}

// LookupShellCommandMetadata informa se a chamada tem uma projeção pública explícita.
func LookupShellCommandMetadata(ctx context.Context) (string, bool) {
	command, ok := ctx.Value(shellCommandMetadataKey{}).(protectedShellCommand)
	return command.metadata, ok
}

// ShellCommandMetadata devolve o comando como deve aparecer nos metadados desta chamada.
func ShellCommandMetadata(ctx context.Context, command string) string {
	if safe, ok := LookupShellCommandMetadata(ctx); ok {
		return safe
	}
	return command
}

// WithPersistentShell marca terminais cujo diretório efetivo pode mudar após cd.
// Somente o executor define esta propriedade, nunca os dados da interface.
func WithPersistentShell(ctx context.Context, persistent bool) context.Context {
	return context.WithValue(ctx, persistentShellKey{}, persistent)
}

func PersistentShell(ctx context.Context) bool {
	persistent, _ := ctx.Value(persistentShellKey{}).(bool)
	return persistent
}

type maskedInvocationContext struct{}

// WithWorkspace captura no backend a raiz de origem antes de enfileirar o
// turno. Vazio significa origem sem workspace, nunca o workspace atual da UI.
func WithWorkspace(ctx context.Context, root string) context.Context {
	return context.WithValue(ctx, workspaceKey{}, root)
}

func Workspace(ctx context.Context) (string, bool) {
	root, ok := ctx.Value(workspaceKey{}).(string)
	return root, ok
}

// With retorna um novo context.Context com o InvocationContext embutido.
func With(ctx context.Context, inv InvocationContext) context.Context {
	return context.WithValue(ctx, ctxKey{}, inv)
}

// Get extrai o InvocationContext do context.Context.
// Retorna false se não houver contexto de invocação.
func Get(ctx context.Context) (InvocationContext, bool) {
	v, ok := ctx.Value(ctxKey{}).(InvocationContext)
	return v, ok
}

// Without cria uma fronteira de execução que não herda conversa, turno,
// superfície nem profile-pai do contexto que originou um evento.
func Without(ctx context.Context) context.Context {
	return WithWorkspace(context.WithValue(ctx, ctxKey{}, maskedInvocationContext{}), "")
}
