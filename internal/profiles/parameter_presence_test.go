package profiles

import (
	"encoding/json"
	"testing"
)

func TestChatConfigPreservesExplicitZeroAndAbsentParameters(t *testing.T) {
	var absent, explicit ChatConfig
	if err := json.Unmarshal([]byte(`{"llm_provider":"provider"}`), &absent); err != nil {
		t.Fatal(err)
	}
	if absent.Temperature != nil || absent.TopP != nil || absent.MaxTokens != nil || absent.ReasoningEffort != nil {
		t.Fatal("absent parameters received fabricated defaults")
	}
	legacy := []byte(`{"llm_provider":"provider","temperature":0,"top_p":1,"max_tokens":4096,"reasoning_effort":""}`)
	if err := json.Unmarshal(legacy, &explicit); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(explicit)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip ChatConfig
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if roundTrip.Temperature == nil || *roundTrip.Temperature != 0 || roundTrip.TopP == nil || *roundTrip.TopP != 1 || roundTrip.MaxTokens == nil || *roundTrip.MaxTokens != 4096 || roundTrip.ReasoningEffort == nil || *roundTrip.ReasoningEffort != "" {
		t.Fatalf("legacy values changed: %s", encoded)
	}
}
