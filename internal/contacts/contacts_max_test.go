package contacts

import (
	"testing"

	"assistente/internal/configdir"
)

func setupTempHome(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	configdir.ResetForTests()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Cleanup(configdir.ResetForTests)
}

func TestAuthorize_MaxContactsUnlimited(t *testing.T) {
	setupTempHome(t)

	if err := Authorize("telegram", "1", "A", "", -1); err != nil {
		t.Fatalf("authorize first: %v", err)
	}
	if err := Authorize("telegram", "2", "B", "", -1); err != nil {
		t.Fatalf("authorize second with unlimited max: %v", err)
	}

	has, allowed := IsAuthorized("telegram", -1, "2")
	if !has || !allowed {
		t.Fatalf("expected contact 2 authorized with unlimited max, has=%v allowed=%v", has, allowed)
	}
}

func TestAuthorize_MaxContactsZeroDefaultsToOne(t *testing.T) {
	setupTempHome(t)

	if err := Authorize("telegram", "1", "A", "", 0); err != nil {
		t.Fatalf("authorize first with max=0 (default 1): %v", err)
	}
	if err := Authorize("telegram", "2", "B", "", 0); err == nil {
		t.Fatal("expected error when max_contacts defaults to 1 already filled")
	}
}

func TestAuthorize_MaxContactsLimited(t *testing.T) {
	setupTempHome(t)

	if err := Authorize("telegram", "1", "A", "", 1); err != nil {
		t.Fatalf("authorize first: %v", err)
	}
	if err := Authorize("telegram", "2", "B", "", 1); err == nil {
		t.Fatal("expected error when max_contacts=1 already filled")
	}

	has, allowed := IsAuthorized("telegram", 1, "2")
	if !has || allowed {
		t.Fatalf("expected contact 2 rejected at limit, has=%v allowed=%v", has, allowed)
	}
}
