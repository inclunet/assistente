package wailsapi

import (
	"errors"
	"testing"
)

func TestFSTrustNotWired(t *testing.T) {
	t.Parallel()
	api := NewFSTrust()
	if err := api.AddPathAllowlistEntry("/tmp/a", "file", "read", "allow", "global", ""); !errors.Is(err, ErrFSTrustNotWired) {
		t.Fatalf("AddPathAllowlistEntry: want ErrFSTrustNotWired, got %v", err)
	}
}
