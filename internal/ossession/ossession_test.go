package ossession

import (
	"context"
	"encoding/binary"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestDecodeWTSInfoEx(t *testing.T) {
	tests := []struct {
		name    string
		flags   uint32
		want    State
		wantErr bool
	}{
		{name: "locked", flags: 0, want: State{Known: true, Locked: true}},
		{name: "unlocked", flags: 1, want: State{Known: true, Locked: false}},
		{name: "unknown", flags: ^uint32(0), wantErr: true},
		{name: "invalid", flags: 2, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buffer := make([]byte, 8+12)
			binary.LittleEndian.PutUint32(buffer[0:], 1)
			binary.LittleEndian.PutUint32(buffer[8:], 41)
			binary.LittleEndian.PutUint32(buffer[12:], uint32(wtsConnectStateActive))
			binary.LittleEndian.PutUint32(buffer[16:], test.flags)
			got, err := decodeWTSInfoEx(buffer, 8, 41)
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, test.wantErr)
			}
			if !test.wantErr && got != test.want {
				t.Fatalf("state = %+v, want %+v", got, test.want)
			}
			if test.wantErr && got != unknownState() {
				t.Fatalf("failed decode returned %+v instead of unknown", got)
			}
		})
	}
}

func TestDecodeWTSInfoExValidatesBufferLevelAndSession(t *testing.T) {
	short := make([]byte, 8+11)
	if _, err := decodeWTSInfoEx(short, 8, 1); err == nil {
		t.Fatal("short buffer was accepted")
	}

	buffer := make([]byte, 8+12)
	binary.LittleEndian.PutUint32(buffer[0:], 2)
	if _, err := decodeWTSInfoEx(buffer, 8, 1); err == nil {
		t.Fatal("unsupported level was accepted")
	}

	binary.LittleEndian.PutUint32(buffer[0:], 1)
	binary.LittleEndian.PutUint32(buffer[8:], 2)
	binary.LittleEndian.PutUint32(buffer[16:], 1)
	if _, err := decodeWTSInfoEx(buffer, 8, 1); err == nil {
		t.Fatal("session mismatch was accepted")
	}

	binary.LittleEndian.PutUint32(buffer[8:], 1)
	binary.LittleEndian.PutUint32(buffer[12:], 1)
	if got, err := decodeWTSInfoEx(buffer, 8, 1); err == nil || got != unknownState() {
		t.Fatalf("inactive session result = %+v, err %v; want unknown", got, err)
	}
}

func TestEventQueuePreservesAllEventsAndOrder(t *testing.T) {
	queue := newEventQueue()
	const count = 2000
	for i := 0; i < count; i++ {
		queue.push(pumpEvent{state: State{Known: true, Locked: i%2 == 0}})
	}
	queue.close(nil)

	for i := 0; i < count; i++ {
		event, ok, err := queue.pop(context.Background())
		if err != nil || !ok {
			t.Fatalf("pop %d = (%+v, %v, %v)", i, event, ok, err)
		}
		want := State{Known: true, Locked: i%2 == 0}
		if event.state != want {
			t.Fatalf("event %d = %+v, want %+v", i, event.state, want)
		}
	}
	if _, ok, err := queue.pop(context.Background()); ok || err != nil {
		t.Fatalf("closed queue result = ok %v, err %v", ok, err)
	}
}

func TestPumpSerializesFakeSourceAndStopsOnObserverError(t *testing.T) {
	states := []State{{Known: true}, {Known: true, Locked: true}, {Known: true, Locked: false}}
	index := 0
	var active atomic.Int32
	var maxActive atomic.Int32
	var stopped atomic.Bool
	var got []State

	next := func(context.Context) (pumpEvent, bool, error) {
		if index == len(states) {
			return pumpEvent{}, false, nil
		}
		state := states[index]
		index++
		return pumpEvent{state: state}, true, nil
	}
	observeErr := errors.New("stop")
	err := pump(context.Background(), next, func(state State) error {
		current := active.Add(1)
		if current > maxActive.Load() {
			maxActive.Store(current)
		}
		got = append(got, state)
		active.Add(-1)
		if len(got) == 2 {
			return observeErr
		}
		return nil
	}, func() { stopped.Store(true) })

	if err == nil || !errors.Is(err, observeErr) {
		t.Fatalf("pump error = %v, want observer error", err)
	}
	if !stopped.Load() {
		t.Fatal("pump did not request stop")
	}
	if maxActive.Load() != 1 {
		t.Fatalf("max concurrent callbacks = %d, want 1", maxActive.Load())
	}
	if !reflect.DeepEqual(got, states[:2]) {
		t.Fatalf("observed states = %+v, want %+v", got, states[:2])
	}
}

func TestPumpStopsWhenObserverPanics(t *testing.T) {
	var stopped atomic.Bool
	err := pump(context.Background(), func(context.Context) (pumpEvent, bool, error) {
		return pumpEvent{state: State{Known: true}}, true, nil
	}, func(State) error {
		panic("observer panic")
	}, func() { stopped.Store(true) })
	if err == nil {
		t.Fatal("panic was not converted to an error")
	}
	if !stopped.Load() {
		t.Fatal("panic did not trigger pump stop")
	}
}
