package main

import (
	"errors"
	"testing"
)

func TestCLIAppProfilesNilSafe(t *testing.T) {
	t.Parallel()
	cli := asCLI(nil)

	if _, err := cli.GetProfiles(); !errors.Is(err, errProfilesNotReady) {
		t.Fatalf("GetProfiles: want errProfilesNotReady, got %v", err)
	}
	if slug := cli.GetActiveProfileSlug(); slug != "" {
		t.Fatalf("GetActiveProfileSlug: want \"\", got %q", slug)
	}
	if _, err := cli.GetProfile("x"); !errors.Is(err, errProfilesNotReady) {
		t.Fatalf("GetProfile: want errProfilesNotReady, got %v", err)
	}
	if err := cli.SetActiveProfile("x"); !errors.Is(err, errProfilesNotReady) {
		t.Fatalf("SetActiveProfile: want errProfilesNotReady, got %v", err)
	}
}
