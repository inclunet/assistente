package userctx

import (
	"context"
	"testing"
)

func TestWithUserIDAcceptsNilContextAndTrimsValue(t *testing.T) {
	var nilCtx context.Context

	ctx := WithUserID(nilCtx, "  user-1  ")

	got, ok := UserIDFromContext(ctx)
	if !ok {
		t.Fatal("UserIDFromContext() ok = false, want true")
	}
	if got != "user-1" {
		t.Fatalf("UserIDFromContext() = %q, want %q", got, "user-1")
	}
}

func TestUserIDFromContextRejectsEmptyTrimmedValue(t *testing.T) {
	ctx := WithUserID(context.Background(), "   ")

	got, ok := UserIDFromContext(ctx)
	if ok {
		t.Fatalf("UserIDFromContext() ok = true, want false with value %q", got)
	}
}
