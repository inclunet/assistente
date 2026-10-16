package configdir

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecoverProfileTransactionRollsForwardEveryCrashPrefix(t *testing.T) {
	for applied := 0; applied <= 3; applied++ {
		t.Run("prefix-"+string(rune('0'+applied)), func(t *testing.T) {
			base := t.TempDir()
			changes := []ProfileFileChange{
				{Path: filepath.Join(base, "one.json"), Before: []byte("one-before"), BeforeExists: true, After: []byte("one-after"), AfterExists: true},
				{Path: filepath.Join(base, "two.json"), Before: []byte("two-before"), BeforeExists: true, After: []byte("two-after"), AfterExists: true},
				{Path: filepath.Join(base, "three.json"), Before: []byte("three-before"), BeforeExists: true, After: []byte("three-after"), AfterExists: true},
			}
			for i, change := range changes {
				data := change.Before
				if i < applied {
					data = change.After
				}
				if err := os.WriteFile(change.Path, data, 0644); err != nil {
					t.Fatal(err)
				}
			}
			journalPath := filepath.Join(base, ".profile-mutation.journal")
			writeTestProfileTransactionJournal(t, journalPath, changes)

			if err := RecoverProfileTransaction(journalPath, []string{base}); err != nil {
				t.Fatalf("recovery error = %v", err)
			}
			for _, change := range changes {
				got, err := os.ReadFile(change.Path)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, change.After) {
					t.Fatalf("%s = %s, want %s", change.Path, got, change.After)
				}
			}
			if _, err := os.Stat(journalPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("journal stat error = %v, want removed journal", err)
			}
		})
	}
}

func TestRecoverProfileTransactionFailsClosedOnUnknownState(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "profile.json")
	change := ProfileFileChange{Path: path, Before: []byte("before"), BeforeExists: true, After: []byte("after"), AfterExists: true}
	if err := os.WriteFile(path, []byte("external"), 0644); err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(base, ".profile-mutation.journal")
	writeTestProfileTransactionJournal(t, journalPath, []ProfileFileChange{change})

	if err := RecoverProfileTransaction(journalPath, []string{base}); !errors.Is(err, ErrProfileTransactionRecovery) {
		t.Fatalf("recovery error = %v, want fail-closed recovery error", err)
	}
	if _, err := os.Stat(journalPath); err != nil {
		t.Fatalf("journal stat error = %v, want journal retained", err)
	}
}

func TestWriteProfileTransactionAtomicPreservesDestinationWhenReplaceFails(t *testing.T) {
	base := t.TempDir()
	destination := filepath.Join(base, "blocked.json")
	if err := os.Mkdir(destination, 0755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(destination, "marker")
	if err := os.WriteFile(marker, []byte("preserve"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := writeProfileTransactionAtomic(destination, []byte("replacement"), 0644); err == nil {
		t.Fatal("replace unexpectedly succeeded over a directory")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("destination was not preserved: %v", err)
	}
}

func TestValidateProfileTransactionRejectsNonProfileAndSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.json")
	if err := os.WriteFile(secret, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := CommitProfileTransaction(filepath.Join(base, ".profile-mutation.journal"), []string{base}, []ProfileFileChange{{
		Path: filepath.Join(base, "secret.txt"), BeforeExists: false, After: []byte("not allowed"), AfterExists: true,
	}}); err == nil {
		t.Fatal("non-JSON transaction path unexpectedly accepted")
	}

	link := filepath.Join(base, "profile.json")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if err := CommitProfileTransaction(filepath.Join(base, ".profile-mutation.journal"), []string{base}, []ProfileFileChange{{
		Path: link, Before: []byte("secret"), BeforeExists: true, After: []byte("changed"), AfterExists: true,
	}}); err == nil {
		t.Fatal("symlink escape unexpectedly accepted")
	}
	got, err := os.ReadFile(secret)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte("secret")) {
		t.Fatalf("outside file changed to %q", got)
	}
}

func writeTestProfileTransactionJournal(t *testing.T, path string, changes []ProfileFileChange) {
	t.Helper()
	j := profileTransactionJournal{Version: 1, Created: time.Now().UTC().Format(time.RFC3339Nano), Changes: changes}
	data, err := json.Marshal(j)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
