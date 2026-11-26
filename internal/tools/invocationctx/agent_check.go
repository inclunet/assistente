package invocationctx

import "context"

type agentCheckKey struct{}

// WithAgentCheck preserves the grants of all enclosing plugin delegations.
func WithAgentCheck(ctx context.Context, check func(context.Context) error) context.Context {
	parent, _ := ctx.Value(agentCheckKey{}).(func(context.Context) error)
	if parent != nil {
		child := check
		check = func(current context.Context) error {
			if err := parent(current); err != nil {
				return err
			}
			return child(current)
		}
	}
	return context.WithValue(ctx, agentCheckKey{}, check)
}

// CheckAgent revalidates every enclosing activation before dispatch.
func CheckAgent(ctx context.Context) error {
	if check, ok := ctx.Value(agentCheckKey{}).(func(context.Context) error); ok {
		return check(ctx)
	}
	return nil
}

// HasAgentCheck indicates that dispatch must remain under application control.
func HasAgentCheck(ctx context.Context) bool { return ctx.Value(agentCheckKey{}) != nil }

// WithTurnModel carries the resolved provider and model to delegated calls.
func WithTurnModel(ctx context.Context, provider, model string) context.Context {
	return context.WithValue(ctx, turnModelKey{}, [2]string{provider, model})
}

type turnModelKey struct{}

// TurnModel returns the provider and model actually used by the enclosing turn.
func TurnModel(ctx context.Context) (provider, model string) {
	pair, _ := ctx.Value(turnModelKey{}).([2]string)
	return pair[0], pair[1]
}
