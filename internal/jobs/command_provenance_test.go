package jobs

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCommandRunProvenanceDropsPayloadAndKeepsSeparateChains(t *testing.T) {
	e := &JobExecutor{}
	trigger := &TriggerContext{ChainID: "chain", ChainHistory: []string{"first-job"}, Provenance: map[string]any{"token": "secret-sentinel", "payload": map[string]any{"password": "secret-sentinel"}, "command_chain_history": []any{map[string]any{"command_id": "job.start", "invocation_id": uuid7ForTest(t), "layer_refs": []string{"user-layer"}}}}}
	p, err := e.commandRunProvenance(&Job{ID: "current-job"}, trigger, &RunLog{RunID: "run"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(p)
	if strings.Contains(string(raw), "secret-sentinel") || p["_source"] != "job" || p["_source_job_id"] != "current-job" {
		t.Fatalf("unsafe provenance: %s", raw)
	}
	history := p["_chain_history"].([]string)
	if len(history) != 2 || history[1] != "current-job" || len(trigger.ChainHistory) != 1 {
		t.Fatal("job chain changed incorrectly")
	}
	if p["command_chain_history"] == nil {
		t.Fatal("lost command chain")
	}
}

func TestCommandRunProvenanceRejectsMalformedCommandChain(t *testing.T) {
	for _, value := range []any{nil, "text", []any{map[string]any{"command_id": "x", "invocation_id": "y", "layer_refs": []string{}, "secret": "sentinel"}}} {
		if _, err := (&JobExecutor{}).commandRunProvenance(&Job{ID: "job"}, &TriggerContext{Provenance: map[string]any{"command_chain_history": value}}, &RunLog{RunID: "run"}); err == nil {
			t.Fatal("invalid chain accepted")
		}
	}
}
