package commandcontract

import (
	"encoding/json"
	"testing"
)

func TestTriggerDocumentVersionsRemainSourceSpecific(t *testing.T) {
	for _, source := range []string{"keyboard.local", "keyboard.global", "streamdeck.key", "palette", "ui"} {
		for _, version := range []string{"1", "2", "3", "1.0", "2.0", `"2"`, "null"} {
			t.Run(source+"/"+version, func(t *testing.T) {
				raw := json.RawMessage(`{"version":` + version + `,"steps":[{"code":"KeyK","modifiers":["Control"]},{"code":"KeyL","modifiers":[]}]}`)
				want := version == "1" || (source == "keyboard.local" && version == "2")
				if got := ValidateTriggerDocumentVersion(source, raw) == nil; got != want {
					t.Fatalf("accepted=%v want=%v", got, want)
				}
				// The envelope must apply the same source-specific version check,
				// including the observed type before physical resolution.
				for _, observed := range []bool{false, true} {
					e := Envelope{TriggerSpec: &raw}
					if observed {
						e.ObservedTriggerType = &source
					} else {
						e.TriggerType = &source
					}
					if got := validateDocuments(&e) == nil; got != want {
						t.Fatalf("observed=%v accepted=%v want=%v", observed, got, want)
					}
				}
			})
		}
	}
	if err := validateVersionedDocument([]byte(`{"version":2}`), "provenance"); err == nil {
		t.Fatal("provenance v2 must remain unsupported")
	}
}
