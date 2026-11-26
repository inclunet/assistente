package invocationctx

import (
	"context"
	"strings"
)

// DelegatedTools is the optional upper bound on tools for a delegated run.
// Its zero value inherits the profile; an explicitly empty set allows no tool.
type DelegatedTools struct{ names map[string]struct{} }
type delegatedToolsKey struct{}

func (s DelegatedTools) Restricted() bool { return s.names != nil }

func (s DelegatedTools) Allows(name string) bool {
	if !s.Restricted() {
		return true
	}
	_, ok := s.names[name]
	return ok
}

func DelegatedToolsFrom(ctx context.Context) DelegatedTools {
	s, _ := ctx.Value(delegatedToolsKey{}).(DelegatedTools)
	return s
}

// WithDelegatedTools intersects the request with any enclosing delegation.
func WithDelegatedTools(ctx context.Context, names []string) context.Context {
	if names == nil {
		return ctx
	}
	parent := DelegatedToolsFrom(ctx)
	s := DelegatedTools{names: make(map[string]struct{}, len(names))}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name != "" && parent.Allows(name) {
			s.names[name] = struct{}{}
		}
	}
	return context.WithValue(ctx, delegatedToolsKey{}, s)
}
