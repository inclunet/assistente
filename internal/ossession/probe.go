package ossession

import (
	"context"
	"errors"
)

var errProbeUnavailable = errors.New("ossession: probe dependencies unavailable")

// probeWith performs a one-shot observation with injected platform queries.
// Context cancellation always wins over a concurrent native-query error, and
// any failure returns a closed/unknown state.
func probeWith(
	ctx context.Context,
	ensureSupported func() error,
	currentSession func() (uint32, error),
	query func(uint32) (State, error),
) (State, error) {
	if ctx == nil {
		return unknownState(), errNilContext
	}
	if err := ctx.Err(); err != nil {
		return unknownState(), err
	}
	if ensureSupported == nil || currentSession == nil || query == nil {
		return unknownState(), errProbeUnavailable
	}
	if err := ensureSupported(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return unknownState(), ctxErr
		}
		return unknownState(), err
	}
	if err := ctx.Err(); err != nil {
		return unknownState(), err
	}
	sessionID, err := currentSession()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return unknownState(), ctxErr
	}
	if err != nil {
		return unknownState(), err
	}
	state, err := query(sessionID)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return unknownState(), ctxErr
	}
	if err != nil {
		return unknownState(), err
	}
	if !state.Known {
		return unknownState(), nil
	}
	return state, nil
}
