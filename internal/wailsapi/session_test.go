package wailsapi

import (
	"context"
	"errors"
	"testing"
)

type stubSession struct {
	ctx context.Context
	err error
}

func (s stubSession) AuthenticatedContext() (context.Context, error) {
	return s.ctx, s.err
}

func TestWithUserFailClosed(t *testing.T) {
	t.Parallel()
	want := errors.New("no auth")
	_, err := WithUser(stubSession{err: want}, func(ctx context.Context) (string, error) {
		t.Fatal("fn não deve rodar sem auth")
		return "", nil
	})
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}

type ctxKey string

const ctxUID ctxKey = "uid"

func TestWithUserOK(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), ctxUID, "u1")
	got, err := WithUser(stubSession{ctx: ctx}, func(c context.Context) (string, error) {
		if c.Value(ctxUID) != "u1" {
			t.Fatalf("ctx perdido")
		}
		return "ok", nil
	})
	if err != nil || got != "ok" {
		t.Fatalf("got %q %v", got, err)
	}
}

func TestWithUserNilSession(t *testing.T) {
	t.Parallel()
	_, err := WithUser[string](nil, func(context.Context) (string, error) {
		t.Fatal("não deve chamar")
		return "", nil
	})
	if !errors.Is(err, errNilSession) {
		t.Fatalf("got %v", err)
	}
}

func TestWithUser2(t *testing.T) {
	t.Parallel()
	a, b, err := WithUser2(stubSession{ctx: context.Background()}, func(context.Context) (bool, float64, error) {
		return true, 42.5, nil
	})
	if err != nil || !a || b != 42.5 {
		t.Fatalf("got %v %v %v", a, b, err)
	}
}
