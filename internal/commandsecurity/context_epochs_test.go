package commandsecurity

import (
	"context"
	"github.com/google/uuid"
	"testing"
)

func TestGenericContextsShareSecurityGateAndCannotCollide(t *testing.T) {
	s, e := NewEpochService(&DispatchGate{})
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	user := uuid.Must(uuid.NewV7()).String()
	a := ContextPrincipal{UserID: user, Type: "external_token", ID: "issuer/token"}
	b := ContextPrincipal{UserID: user, Type: "job_service", ID: "issuer/token"}
	capture := func(p ContextPrincipal) EpochSnapshot {
		v, e := s.CaptureContextAuthenticated(ctx, func(context.Context) (ContextPrincipal, error) { return p, nil })
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	ea, eb := capture(a), capture(b)
	if ea.SessionID == eb.SessionID {
		t.Fatal("colisão entre tipos")
	}
	watch, release, e := s.WatchEpoch(ctx, ea)
	if e != nil {
		t.Fatal(e)
	}
	defer release()
	if e := s.MutateContext(ctx, a, func() error { return nil }); e != nil {
		t.Fatal(e)
	}
	if watch.Err() == nil {
		t.Fatal("revogação não cancelou watch")
	}
	if e := s.Admit(ctx, eb, func(context.Context) error { return nil }, func() error { return nil }); e != nil {
		t.Fatal("revogação atravessou outro contexto")
	}
	if e := s.MutateSecurity(ctx, func() error { return nil }); e != nil {
		t.Fatal(e)
	}
	if e := s.Admit(ctx, eb, func(context.Context) error { return nil }, func() error { return nil }); e == nil {
		t.Fatal("lock não invalidou job")
	}
	if _, e := s.CaptureContextAuthenticated(ctx, func(context.Context) (ContextPrincipal, error) {
		return ContextPrincipal{UserID: user, Type: "system", ID: "process"}, nil
	}); e == nil {
		t.Fatal("system aceitou owner")
	}
}
