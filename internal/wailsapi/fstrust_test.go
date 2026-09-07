package wailsapi

import (
	"errors"
	"testing"
)

func TestFSTrustNotWired(t *testing.T) {
	t.Parallel()
	api := NewFSTrust()
	if _, err := api.GetPathAllowlist(); !errors.Is(err, ErrFSTrustNotWired) {
		t.Fatalf("GetPathAllowlist: want ErrFSTrustNotWired, got %v", err)
	}
	if err := api.RemovePathAllowlistEntry("global", "/tmp/a", "file", "read", "allow"); !errors.Is(err, ErrFSTrustNotWired) {
		t.Fatalf("RemovePathAllowlistEntry: want ErrFSTrustNotWired, got %v", err)
	}
	if err := api.AddPathAllowlistEntry("/tmp/a", "file", "read", "allow", "global", ""); !errors.Is(err, ErrFSTrustNotWired) {
		t.Fatalf("AddPathAllowlistEntry: want ErrFSTrustNotWired, got %v", err)
	}
}
