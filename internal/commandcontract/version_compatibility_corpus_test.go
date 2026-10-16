package commandcontract

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestIngressUsesSharedTriggerVersionCorpus(t *testing.T) {
	raw, err := os.ReadFile("../testdata/corpus/versioned-triggers.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name        string `json:"name"`
		TriggerType string `json:"trigger_type"`
		TriggerSpec string `json:"trigger_spec"`
		Accepted    bool   `json:"accepted"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("corpus de compatibilidade vazio")
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			e := resolvedDirectFixture()
			e.CommandID = nil
			e.SourceType = nil
			e.TriggerType = nil
			e.ObservedTriggerType = stringPointer(tc.TriggerType)
			e.TriggerSpec = rawPointer(tc.TriggerSpec)
			e.SourceInstanceID = stringPointer(testInstance)
			e.SourceEventID = stringPointer(testEvent)
			e.ObserverType = stringPointer("os.keyboard")
			e.BindingIDs = nil
			e.ReceivedAt = time.Time{}
			err := e.ValidateIngress()
			if (err == nil) != tc.Accepted {
				t.Fatalf("accepted=%v want=%v err=%v", err == nil, tc.Accepted, err)
			}
		})
	}
}
