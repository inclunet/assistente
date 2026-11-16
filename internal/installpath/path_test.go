package installpath

import (
	"strings"
	"testing"
)

func TestEntryLifecyclePreservesOtherPaths(t *testing.T) {
	dir := `C:\Program Files\Inclunet\Assistente`
	for _, original := range []string{"", `C:\Tools;%USERPROFILE%\bin`, `C:\Tools;`, strings.Repeat(`C:\long;`, 2000)} {
		added, changed := addEntry(original, dir)
		if !changed {
			t.Fatal("did not add directory")
		}
		again, changed := addEntry(added, dir)
		if changed || again != added {
			t.Fatal("update duplicated PATH entry")
		}
		removed := removeEntry(added, dir)
		if removed != original {
			t.Fatalf("other entries changed: %q", removed)
		}
	}
}

func TestExistingEquivalentEntryIsNotOwned(t *testing.T) {
	for _, original := range []string{`C:\Tools;"c:\PROGRAM files\Inclunet\Assistente\"`, `C:/Program Files/Inclunet/Assistente`} {
		got, changed := addEntry(original, `C:\Program Files\Inclunet\Assistente`)
		if changed || got != original {
			t.Fatal("preexisting entry changed")
		}
	}
}
