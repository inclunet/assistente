package toolctx

import "testing"

func TestWithInvocationIDsAcceptNilContext(t *testing.T) {
	ctx := WithCurrentInvocationID(nil, "current-1")
	if got := CurrentInvocationID(ctx); got != "current-1" {
		t.Fatalf("CurrentInvocationID() = %q, want %q", got, "current-1")
	}

	ctx = WithParentInvocationID(nil, "parent-1")
	if got := ParentInvocationIDFromContext(ctx); got != "parent-1" {
		t.Fatalf("ParentInvocationIDFromContext() = %q, want %q", got, "parent-1")
	}
}
