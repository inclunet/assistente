package app

import (
	"context"
	"testing"
	"time"
)

func TestLocalKeyboardProjectionCurrentRequiresLiveExactSnapshot(t *testing.T) {
	a := readyCommandProduct(t)
	p := a.commandProduct.Load()
	if p == nil {
		t.Fatal("command product fixture is not mounted")
	}
	configuration, _, versions, err := p.host.ResolutionSnapshot(context.Background(), p.principal)
	if err != nil {
		t.Fatal(err)
	}
	distinctConfiguration := configuration.WithValidityDeadline(configuration.ValidUntil())
	if distinctConfiguration == configuration || !configuration.Equivalent(distinctConfiguration) {
		t.Fatal("distinct-snapshot case requires a separate but equivalent configuration pointer")
	}

	liveState := func(ctx context.Context, validUntil int64) *localCommandKeyboardState {
		return &localCommandKeyboardState{
			view:          LocalCommandKeyboardMap{ValidUntil: validUntil},
			configuration: configuration,
			versions:      versions,
			ctx:           ctx,
		}
	}
	tests := []struct {
		name  string
		state *localCommandKeyboardState
		want  bool
	}{
		{name: "exact live projection", state: liveState(context.Background(), 0), want: true},
		{name: "future deadline", state: liveState(context.Background(), time.Now().Add(time.Minute).UnixMilli()), want: true},
		{name: "absent", state: nil},
		{name: "canceled", state: liveState(canceledContext(t), 0)},
		{name: "expired", state: liveState(context.Background(), time.Now().Add(-time.Second).UnixMilli())},
		{name: "distinct equivalent configuration pointer", state: &localCommandKeyboardState{
			view: LocalCommandKeyboardMap{ValidUntil: 0}, configuration: distinctConfiguration,
			versions: versions, ctx: context.Background(),
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p.keyboardMu.Lock()
			previous := p.keyboardMap
			p.keyboardMap = tt.state
			p.keyboardMu.Unlock()
			t.Cleanup(func() {
				p.keyboardMu.Lock()
				p.keyboardMap = previous
				p.keyboardMu.Unlock()
			})

			if got := p.localKeyboardProjectionCurrent(context.Background()); got != tt.want {
				t.Fatalf("localKeyboardProjectionCurrent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func canceledContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}
