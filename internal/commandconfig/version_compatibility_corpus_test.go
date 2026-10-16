package commandconfig

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

func TestPersistedProjectionUsesSharedTriggerVersionCorpus(t *testing.T) {
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
			fixture := newProjectionTestFixture(t)
			layer := projectionTestLayer(t, fixture, true)
			row := projectionTestBinding(t, fixture, layer)
			if err := fixture.db.Model(&Binding{}).Where("id = ?", row.ID).Updates(map[string]any{
				"trigger_type": tc.TriggerType,
				"trigger_spec": tc.TriggerSpec,
			}).Error; err != nil {
				t.Fatal(err)
			}
			fixture.options.ActiveUserLayerIDs = []string{layer.ID}
			snapshot := projectionTestReload(t, fixture)
			_, projectErr := ProjectLocalRead(context.Background(), snapshot, fixture.options)
			if (projectErr == nil) != tc.Accepted {
				t.Fatalf("projected=%v want=%v err=%v", projectErr == nil, tc.Accepted, projectErr)
			}
		})
	}
}
