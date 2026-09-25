package commandsecurity

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestAuthenticatedCaptureHoldsExclusiveGate(t *testing.T) {
	gate := &DispatchGate{}
	s, err := NewEpochService(gate)
	if err != nil {
		t.Fatal(err)
	}
	user, session := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()
	snapshot, err := s.CaptureAuthenticated(context.Background(), func(context.Context) (string, string, error) {
		if gate.mu.TryLock() {
			gate.mu.Unlock()
			t.Error("gate exclusivo ausente")
		}
		if gate.mu.TryRLock() {
			gate.mu.RUnlock()
			t.Error("admissão concorrente permitida")
		}
		return user, session, nil
	})
	if err != nil || snapshot.UserID != user || snapshot.SessionID != session || snapshot.AuthGeneration == "" || snapshot.SecurityGeneration == "" {
		t.Fatalf("snapshot incorreto: %+v, %v", snapshot, err)
	}
	if !gate.mu.TryLock() {
		t.Fatal("gate retido após captura")
	}
	gate.mu.Unlock()
	if err := s.InvalidatePrincipal(context.Background(), user, session); err != nil {
		t.Fatal(err)
	}
	err = s.Admit(context.Background(), snapshot, func(context.Context) error { t.Fatal("snapshot obsoleto revalidado"); return nil }, func() error { t.Fatal("handoff obsoleto"); return nil })
	if !errors.Is(err, ErrStaleEpoch) {
		t.Fatal(err)
	}
}

func TestAuthenticatedCaptureFailureDoesNotPublishIdentity(t *testing.T) {
	sentinel := errors.New("sessão recusada")
	for _, scenario := range []string{"authentication", "invalid_user", "invalid_session", "cancelled_during", "cancelled_before", "nil_callback", "nil_context", "transition"} {
		t.Run(scenario, func(t *testing.T) {
			s, err := NewEpochService(&DispatchGate{})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			user, session := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()
			calls := 0
			callback := func(context.Context) (string, string, error) {
				calls++
				switch scenario {
				case "authentication":
					return user, session, sentinel
				case "invalid_user":
					return "payload-user", session, nil
				case "invalid_session":
					return user, "payload-session", nil
				case "cancelled_during":
					cancel()
				}
				return user, session, nil
			}
			want := ErrInvalidEpochInput
			switch scenario {
			case "authentication":
				want = sentinel
			case "cancelled_before":
				cancel()
				want = context.Canceled
			case "cancelled_during":
				want = context.Canceled
			case "nil_callback":
				callback = nil
			case "nil_context":
				ctx = nil
				want = errNilContext
			case "transition":
				finish, err := s.BeginTransition(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer finish()
				want = ErrStaleEpoch
			}
			sequence := s.sequence
			snapshot, err := s.CaptureAuthenticated(ctx, callback)
			if !errors.Is(err, want) || snapshot != (EpochSnapshot{}) {
				t.Fatalf("%+v, %v; want %v", snapshot, err, want)
			}
			if len(s.sessions) != 0 || s.sequence != sequence {
				t.Fatal("falha publicou identidade/geração")
			}
			if (scenario == "cancelled_before" || scenario == "nil_context" || scenario == "transition") && calls != 0 {
				t.Fatal("callback invocado sem admissão")
			}
		})
	}
}

func TestAuthenticatedCapturePanicReleasesGate(t *testing.T) {
	gate := &DispatchGate{}
	s, err := NewEpochService(gate)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recover() != "fixture" {
				t.Error("panic não propagado")
			}
		}()
		_, _ = s.CaptureAuthenticated(context.Background(), func(context.Context) (string, string, error) { panic("fixture") })
	}()
	if !gate.mu.TryLock() {
		t.Fatal("gate retido após panic")
	}
	gate.mu.Unlock()
	if len(s.sessions) != 0 || s.sequence != 0 {
		t.Fatal("panic publicou identidade")
	}
}
