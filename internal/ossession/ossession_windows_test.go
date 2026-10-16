//go:build windows

package ossession

import (
	"context"
	"errors"
	"testing"
)

func TestSessionEventMappingUsesFailClosedLockAndFakeQuery(t *testing.T) {
	queue := newEventQueue()
	state := &nativeState{queue: queue, currentSession: 7}
	queryCalled := false
	state.handleSessionChangeWithQuery(7, wtsSessionLock, func(uint32) (State, error) {
		queryCalled = true
		return State{Known: true, Locked: false}, nil
	})
	if queryCalled {
		t.Fatal("lock event queried a transitional state")
	}
	event, ok, err := queue.pop(context.Background())
	if err != nil || !ok || event.state != (State{Known: true, Locked: true}) {
		t.Fatalf("lock event = %+v, ok %v, err %v", event.state, ok, err)
	}
}

func TestSessionEventMappingDisconnectIsUnknown(t *testing.T) {
	queue := newEventQueue()
	state := &nativeState{queue: queue, currentSession: 7}
	state.handleSessionChangeWithQuery(7, wtsRemoteDisconnect, func(uint32) (State, error) {
		t.Fatal("disconnect event queried session state")
		return State{}, nil
	})
	event, ok, err := queue.pop(context.Background())
	if err != nil || !ok || event.state != unknownState() {
		t.Fatalf("disconnect event = %+v, ok %v, err %v", event.state, ok, err)
	}
}

func TestSessionEventMappingRejectsMismatchedSession(t *testing.T) {
	queue := newEventQueue()
	state := &nativeState{queue: queue, currentSession: 7}
	state.handleSessionChangeWithQuery(8, wtsSessionUnlock, func(uint32) (State, error) {
		t.Fatal("mismatched event queried session state")
		return State{}, nil
	})
	event, ok, err := queue.pop(context.Background())
	if err != nil || !ok || event.state != unknownState() {
		t.Fatalf("mismatched event = %+v, ok %v, err %v", event.state, ok, err)
	}
	if state.fatalErr == nil || errors.Is(state.fatalErr, errPumpStopped) {
		t.Fatalf("mismatched event fatal error = %v", state.fatalErr)
	}
}
