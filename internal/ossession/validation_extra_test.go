package ossession

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
)

func TestDecoderNeverUnlocksInactiveSession(t *testing.T) {
	for _, connection := range []uint32{1, 2, 3, 4, 5, 6, 7, 8, 9, ^uint32(0)} {
		buffer := make([]byte, 20)
		binary.LittleEndian.PutUint32(buffer, 1)
		binary.LittleEndian.PutUint32(buffer[8:], 42)
		binary.LittleEndian.PutUint32(buffer[12:], connection)
		binary.LittleEndian.PutUint32(buffer[16:], 1) // UNLOCK não basta.
		state, err := decodeWTSInfoEx(buffer, 8, 42)
		if err == nil || state != unknownState() {
			t.Fatalf("sessão inativa %d liberada: %+v, %v", connection, state, err)
		}
	}
}

func TestWatchAlreadyCancelledDoesNotReachNativePlatform(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var observed []State
	err := Watch(ctx, func(state State) error { observed = append(observed, state); return nil })
	if !errors.Is(err, context.Canceled) || len(observed) != 2 {
		t.Fatal("cancelamento inicial", observed, err)
	}
	for _, state := range observed {
		if state != unknownState() {
			t.Fatal("observação não fechada", state)
		}
	}
}

func TestWatchRejectsCallbackBeforeCreatingNativeResources(t *testing.T) {
	want := errors.New("recusado pela fixture")
	if err := Watch(context.Background(), func(State) error { return want }); !errors.Is(err, want) {
		t.Fatal(err)
	}
	if err := Watch(context.Background(), func(State) error { panic("fixture") }); err == nil {
		t.Fatal("panic do observador não foi isolado")
	}
}

func TestFailedSourceDoesNotReplayQueuedUnlock(t *testing.T) {
	queue := newEventQueue()
	queue.push(pumpEvent{state: State{Known: true}})
	want := errors.New("observação perdida")
	queue.close(want)
	_, ok, err := queue.pop(context.Background())
	if ok || !errors.Is(err, want) {
		t.Fatal("reproduziu unlock depois de perder a fonte", ok, err)
	}
}

func TestDecoderRejectsInvalidOffsetWithoutOverflow(t *testing.T) {
	for _, offset := range []int{-1, 0, 4, 7, 9, int(^uint(0) >> 1)} {
		if state, err := decodeWTSInfoEx(make([]byte, 20), offset, 0); err == nil || state != unknownState() {
			t.Fatal("offset inválido aceito", offset, state, err)
		}
	}
}
