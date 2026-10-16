package commandbindings

import (
	"encoding/json"
	"testing"
)

func TestCandidateJSONPreservesExistingIdentityAndExplicitEmptyGates(t *testing.T) {
	for _, conditions := range [][]Facts{nil, {}, {{SurfaceType: "chat"}}} {
		candidate := Candidate{ID: "binding", LayerRef: "layer", LayerConditions: conditions}
		raw, err := json.Marshal(candidate)
		if err != nil {
			t.Fatal(err)
		}
		var document map[string]json.RawMessage
		if err := json.Unmarshal(raw, &document); err != nil {
			t.Fatal(err)
		}
		if string(document["LayerRef"]) != `"layer"` {
			t.Fatalf("existing identity field lost: %s", raw)
		}
		if _, present := document["LayerConditions"]; present != (conditions != nil) {
			t.Fatalf("nil and empty gates conflated: %s", raw)
		}
		var decoded Candidate
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		if (decoded.LayerConditions == nil) != (conditions == nil) || len(decoded.LayerConditions) != len(conditions) {
			t.Fatalf("gate serialization broadened activation: %+v", decoded)
		}
	}
}
