package commandexecution

import (
	"encoding/json"
	"testing"
)

func TestCanonicalCandidateLocalSequenceVersion(t *testing.T) {
	for _, source := range []string{"keyboard.local", "keyboard.global", "streamdeck.key", "palette"} {
		for _, version := range []string{"1", "2", "3", "1.0", "2.0", `"2"`, "null"} {
			t.Run(source+"/"+version, func(t *testing.T) {
				candidate := EnvelopeCandidate{
					InvocationID:  "01890f00-0000-7000-8000-000000000001",
					CorrelationID: "01890f00-0000-7000-8000-000000000002",
					TriggerType:   source,
					TriggerSpec:   json.RawMessage(`{"version":` + version + `,"steps":[{"code":"KeyK","modifiers":["Control"]},{"code":"KeyL","modifiers":[]}]}`),
				}
				_, err := canonicalCandidate(candidate)
				want := version == "1" || (source == "keyboard.local" && version == "2")
				if (err == nil) != want {
					t.Fatalf("accepted=%v want=%v err=%v", err == nil, want, err)
				}
			})
		}
	}
}
