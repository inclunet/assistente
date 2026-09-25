package commandportability

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"assistente/internal/commandconfig"
)

func TestImportUsesSharedTriggerVersionCorpus(t *testing.T) {
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
			layer := portabilityLayer(t)
			layer.Scope = PortableScope{Kind: GlobalScope}
			layer.Bindings[0].TriggerType = tc.TriggerType
			layer.Bindings[0].TriggerSpec = tc.TriggerSpec
			refs := portabilityRefs(t)
			localPort := commandconfig.KeyboardLocalTriggerPort{}
			refs.Trigger = func(ctx context.Context, triggerType, spec string) (string, error) {
				if triggerType != "keyboard.local" {
					return "", commandconfig.ErrInvalid
				}
				return localPort.Normalize(ctx, []byte(spec))
			}
			_, planErr := PlanImport(context.Background(), []LayerExport{layer}, PlanOptions{Mode: KeepMode},
				func(context.Context, string, string) (Ownership, error) { return AbsentOwner, nil }, refs)
			if (planErr == nil) != tc.Accepted {
				t.Fatalf("planned=%v want=%v err=%v", planErr == nil, tc.Accepted, planErr)
			}
		})
	}
}
