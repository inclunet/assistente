//go:build windows

package ossession

import (
	"context"
	"errors"
	"testing"
	"unsafe"
)

func TestWindowsABIStructSizes(t *testing.T) {
	wantClass, wantMessage := uintptr(80), uintptr(48)
	if unsafe.Sizeof(uintptr(0)) == 4 {
		wantClass, wantMessage = 48, 32
	}
	if unsafe.Sizeof(wndClassExW{}) != wantClass || unsafe.Sizeof(winMSG{}) != wantMessage || unsafe.Sizeof(rtlOSVersionInfoW{}) != 284 {
		t.Fatal("layout Win32 divergente", unsafe.Sizeof(wndClassExW{}), unsafe.Sizeof(winMSG{}), unsafe.Sizeof(rtlOSVersionInfoW{}))
	}
	if wtsInfoExDataOffset != 8 {
		t.Fatal("prefixo WTSINFOEXW desalinhado")
	}
}

func TestUnlockEventUsesCurrentQueryAndNeverGuesses(t *testing.T) {
	for _, failed := range []bool{false, true} {
		state := &nativeState{queue: newEventQueue(), currentSession: 42}
		calls := 0
		state.handleSessionChangeWithQuery(42, wtsSessionUnlock, func(session uint32) (State, error) {
			calls++
			if session != 42 {
				t.Fatal("consulta de outra sessão", session)
			}
			if failed {
				return State{Known: true}, errors.New("consulta falhou")
			}
			return State{Known: true, Locked: true}, nil // Lock mais recente.
		})
		event, ok, err := state.queue.pop(context.Background())
		if calls != 1 || !ok || err != nil || !event.state.Locked {
			t.Fatal("unlock inferido do evento", event, calls, ok, err)
		}
		if failed && (event.state.Known || state.fatalErr == nil) {
			t.Fatal("erro de consulta não fechou observador", event)
		}
	}
}
