package main

import (
	"errors"
	"testing"

	"assistente/internal/profiles"
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
	profile := *profiles.DefaultProfile()
	cases := []struct {
		name string
		run  func() error
	}{
		{name: "create", run: func() error { _, err := cli.CreateProfile(profile); return err }},
		{name: "update", run: func() error { return cli.UpdateProfile("perfil", profile) }},
		{name: "duplicate", run: func() error { _, err := cli.DuplicateProfile("perfil"); return err }},
		{name: "delete", run: func() error { return cli.DeleteProfile("perfil") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); !errors.Is(err, errProfilesNotReady) {
				t.Fatalf("erro = %v, want %v", err, errProfilesNotReady)
			}
		})
	}
}
