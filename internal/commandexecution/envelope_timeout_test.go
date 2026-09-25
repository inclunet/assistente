package commandexecution

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandledger"
)

func TestEnvelopeHandlerTimeoutUsesResolvedHostHandlerAndCallerDeadline(t *testing.T) {
	for _, tc := range []struct {
		name                             string
		custom, caller, minimum, maximum time.Duration
	}{
		{"default", 0, 0, 0, 2 * time.Second},
		{"interactive", time.Minute, 0, 50 * time.Second, time.Minute},
		{"caller-shorter", time.Minute, 10 * time.Second, 0, 10 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newEnvelopePipelineFixture(t)
			config := f.service.config
			handler := config.Handlers["pipe.write"]
			handler.ExecutionTimeout = tc.custom
			config.Handlers["pipe.write"] = handler
			service, err := NewComplete(config)
			if err != nil {
				t.Fatal(err)
			}
			f.start = func(ctx context.Context, _ Invocation) (ExecutionHandle, error) {
				if err := ctx.Err(); err != nil {
					t.Fatalf("preparation canceled execution: %v", err)
				}
				deadline, ok := ctx.Deadline()
				remaining := time.Until(deadline)
				if !ok || remaining <= tc.minimum || remaining > tc.maximum {
					t.Fatalf("deadline remaining=%v", remaining)
				}
				return pipelineCompletedHandle(commandledger.Succeeded), nil
			}
			ctx := context.Background()
			if tc.caller > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tc.caller)
				defer cancel()
			}
			result, err := service.ExecuteEnvelope(ctx, f.token, f.candidate(newTestUUID(), "pipe.write", `{}`))
			if err != nil || result.Status != commandledger.Succeeded || f.startCalls.Load() != 1 {
				t.Fatalf("result=%s err=%v", result.Status, err)
			}
		})
	}
}

func TestEnvelopeRejectsInvalidHandlerTimeout(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	for _, timeout := range []time.Duration{-time.Second, 5*time.Minute + time.Nanosecond} {
		config := f.service.config
		h := config.Handlers["pipe.write"]
		h.ExecutionTimeout = timeout
		config.Handlers["pipe.write"] = h
		if _, err := NewComplete(config); !errors.Is(err, ErrInvalidConfiguration) {
			t.Fatalf("timeout=%v err=%v", timeout, err)
		}
	}
}
