package llm

import "context"

// toolChoiceCtxKey é usado para passar um override opcional de tool_choice
// via context, sem alterar a assinatura de StreamChat.
type toolChoiceCtxKey struct{}

// WithToolChoice define um override de tool_choice para uma chamada específica.
// Ex.: "required", "auto", "none" ou um objeto conforme a spec.
func WithToolChoice(ctx context.Context, choice interface{}) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, toolChoiceCtxKey{}, choice)
}

func toolChoiceFromContext(ctx context.Context) (interface{}, bool) {
	if ctx == nil {
		return nil, false
	}
	v := ctx.Value(toolChoiceCtxKey{})
	if v == nil {
		return nil, false
	}
	return v, true
}
