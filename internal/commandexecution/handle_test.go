package commandexecution

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandledger"
)

func TestAwaitOutcomeAcceptsExplicitStatuses(t *testing.T) {
	for _, status := range []commandledger.Status{
		commandledger.Succeeded,
		commandledger.Failed,
		commandledger.Cancelled,
	} {
		done := make(chan Outcome, 1)
		done <- Outcome{Status: status}
		var cancels atomic.Int32

		got := awaitOutcome(context.Background(), ExecutionHandle{
			ID:     "exec-1",
			Done:   done,
			Cancel: func() { cancels.Add(1) },
		})
		if got != status {
			t.Fatalf("status = %q, want %q", got, status)
		}
		if got := cancels.Load(); got != 0 {
			t.Fatalf("Cancel calls = %d, want 0", got)
		}
	}
}

func TestAwaitOutcomeInvalidHandleReturnsUnknownAndCancelsOnce(t *testing.T) {
	cases := []struct {
		name string
		make func(func()) ExecutionHandle
	}{
		{name: "id em branco", make: func(cancel func()) ExecutionHandle {
			return ExecutionHandle{ID: " \t", Done: make(chan Outcome), Cancel: cancel}
		}},
		{name: "done ausente", make: func(cancel func()) ExecutionHandle {
			return ExecutionHandle{ID: "exec-1", Cancel: cancel}
		}},
		{name: "cancel ausente", make: func(func()) ExecutionHandle {
			return ExecutionHandle{ID: "exec-1", Done: make(chan Outcome, 1)}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			got := awaitOutcome(context.Background(), tc.make(func() { calls.Add(1) }))
			if got != commandledger.OutcomeUnknown {
				t.Fatalf("status = %q, want %q", got, commandledger.OutcomeUnknown)
			}
			want := int32(0)
			if tc.name != "cancel ausente" {
				want = 1
			}
			if calls.Load() != want {
				t.Fatalf("Cancel calls = %d, want %d", calls.Load(), want)
			}
		})
	}
}

func TestAwaitOutcomeClosedOrUnknownOutcomeCancelsOnce(t *testing.T) {
	tests := []struct {
		name   string
		status commandledger.Status
		closed bool
	}{
		{name: "canal fechado", closed: true},
		{name: "status não reconhecido", status: commandledger.Running},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			done := make(chan Outcome, 1)
			if test.closed {
				close(done)
			} else {
				done <- Outcome{Status: test.status}
			}
			var calls atomic.Int32
			got := awaitOutcome(context.Background(), ExecutionHandle{
				ID: "exec-1", Done: done, Cancel: func() { calls.Add(1) },
			})
			if got != commandledger.OutcomeUnknown || calls.Load() != 1 {
				t.Fatalf("status = %q, Cancel calls = %d", got, calls.Load())
			}
		})
	}
}

func TestAwaitOutcomeContextCancellationCancelsSynchronouslyOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan Outcome)
	var calls atomic.Int32
	got := awaitOutcome(ctx, ExecutionHandle{
		ID: "exec-1", Done: done, Cancel: func() { calls.Add(1) },
	})
	if got != commandledger.OutcomeUnknown {
		t.Fatalf("status = %q, want %q", got, commandledger.OutcomeUnknown)
	}
	if calls.Load() != 1 {
		t.Fatalf("Cancel calls = %d, want 1", calls.Load())
	}
}

func TestAwaitOutcomeDoesNotObserveLateOutcome(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan Outcome, 1)
	var calls atomic.Int32
	cancel()
	got := awaitOutcome(ctx, ExecutionHandle{
		ID: "exec-1", Done: done, Cancel: func() { calls.Add(1) },
	})
	done <- Outcome{Status: commandledger.Succeeded}
	if got != commandledger.OutcomeUnknown || len(done) != 1 || calls.Load() != 1 {
		t.Fatalf("status = %q, queued outcomes = %d, Cancel calls = %d", got, len(done), calls.Load())
	}
}

func TestAwaitOutcomeRecoversCancelPanic(t *testing.T) {
	done := make(chan Outcome)
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)
	if got := awaitOutcome(ctx, ExecutionHandle{
		ID: "exec-1", Done: done, Cancel: func() { panic("adapter") },
	}); got != commandledger.OutcomeUnknown {
		t.Fatalf("status = %q, want %q", got, commandledger.OutcomeUnknown)
	}
}
