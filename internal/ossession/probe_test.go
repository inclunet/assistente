package ossession

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestProbeWithFakeQueries(t *testing.T) {
	versionErr := errors.New("version query failed")
	sessionErr := errors.New("current session lookup failed")
	queryErr := errors.New("session state query failed")
	cases := []struct {
		name       string
		versionErr error
		sessionErr error
		query      func(uint32) (State, error)
		want       State
		wantErr    error
		wantCalls  []string
	}{
		{name: "unknown", query: func(uint32) (State, error) { return State{}, nil }, want: unknownState(), wantCalls: []string{"version", "session", "query"}},
		{name: "locked", query: func(uint32) (State, error) { return State{Known: true, Locked: true}, nil }, want: State{Known: true, Locked: true}, wantCalls: []string{"version", "session", "query"}},
		{name: "unlocked", query: func(uint32) (State, error) { return State{Known: true}, nil }, want: State{Known: true}, wantCalls: []string{"version", "session", "query"}},
		{name: "version error", versionErr: versionErr, want: unknownState(), wantErr: versionErr, wantCalls: []string{"version"}},
		{name: "session lookup error", sessionErr: sessionErr, want: unknownState(), wantErr: sessionErr, wantCalls: []string{"version", "session"}},
		{name: "query error", query: func(uint32) (State, error) { return State{Known: true}, queryErr }, want: unknownState(), wantErr: queryErr, wantCalls: []string{"version", "session", "query"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			state, err := probeWith(context.Background(),
				func() error { calls = append(calls, "version"); return tc.versionErr },
				func() (uint32, error) { calls = append(calls, "session"); return 41, tc.sessionErr },
				func(id uint32) (State, error) {
					calls = append(calls, "query")
					if id != 41 {
						t.Fatalf("query session ID = %d, want 41", id)
					}
					return tc.query(id)
				},
			)
			if state != tc.want || !errors.Is(err, tc.wantErr) {
				t.Fatalf("probeWith = (%+v, %v), want (%+v, %v)", state, err, tc.want, tc.wantErr)
			}
			if !reflect.DeepEqual(calls, tc.wantCalls) {
				t.Fatalf("query sequence = %v, want %v", calls, tc.wantCalls)
			}
		})
	}
}

func TestProbeWithFakeQueriesChecksContextBeforeAndAfterNativeCalls(t *testing.T) {
	t.Run("already cancelled skips native calls", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		called := false
		state, err := probeWith(ctx,
			func() error { called = true; return nil },
			func() (uint32, error) { called = true; return 1, nil },
			func(uint32) (State, error) { called = true; return State{Known: true}, nil },
		)
		if state != unknownState() || !errors.Is(err, context.Canceled) || called {
			t.Fatalf("cancelled probe = (%+v, %v), called=%v", state, err, called)
		}
	})

	for _, stage := range []string{"version", "session", "query"} {
		t.Run("cancelled during "+stage+" discards state", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var calls []string
			cancelAt := func(name string) {
				calls = append(calls, name)
				if name == stage {
					cancel()
				}
			}
			state, err := probeWith(ctx,
				func() error { cancelAt("version"); return nil },
				func() (uint32, error) { cancelAt("session"); return 1, nil },
				func(uint32) (State, error) { cancelAt("query"); return State{Known: true, Locked: false}, nil },
			)
			if state != unknownState() || !errors.Is(err, context.Canceled) {
				t.Fatalf("cancelled %s probe = (%+v, %v), want unknown/cancelled", stage, state, err)
			}
			wantCalls := map[string][]string{
				"version": {"version"},
				"session": {"version", "session"},
				"query":   {"version", "session", "query"},
			}[stage]
			if !reflect.DeepEqual(calls, wantCalls) {
				t.Fatalf("calls after cancellation = %v, want %v", calls, wantCalls)
			}
		})
	}
}

func TestProbeWithNilDependencyFailsClosed(t *testing.T) {
	state, err := probeWith(context.Background(), nil, func() (uint32, error) { return 1, nil }, func(uint32) (State, error) {
		return State{Known: true}, nil
	})
	if state != unknownState() || !errors.Is(err, errProbeUnavailable) {
		t.Fatalf("nil-dependency probe = (%+v, %v), want unknown/unavailable", state, err)
	}
}

func TestProbeWithNilContextIsUnknown(t *testing.T) {
	state, err := probeWith(nil, func() error { return nil }, func() (uint32, error) { return 1, nil }, func(uint32) (State, error) { //nolint:staticcheck // Contexto nil intencional para provar a recusa fail-closed.
		return State{Known: true}, nil
	})
	if state != unknownState() || !errors.Is(err, errNilContext) {
		t.Fatalf("nil-context probe = (%+v, %v), want unknown/context error", state, err)
	}
}
