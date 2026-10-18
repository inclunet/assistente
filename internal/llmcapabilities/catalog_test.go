package llmcapabilities

import "testing"

func TestCapabilityVocabularyIsClosed(t *testing.T) {
	for _, capability := range Capabilities() {
		if !HasCapability(capability) {
			t.Errorf("capability listada não é reconhecida: %q", capability)
		}
	}
	if HasCapability(Capability("unknown.operation")) {
		t.Fatal("aceitou capability fora do vocabulário")
	}
}

func TestFieldsAreScopedToOperation(t *testing.T) {
	tests := []struct {
		capability Capability
		field      FieldKey
		want       bool
	}{
		{CapabilityChatCompletions, FieldTemperature, true},
		{CapabilityResponses, FieldTemperature, true},
		{CapabilityTextToSpeech, FieldVoice, true},
		{CapabilitySpeechToText, FieldVoice, false},
		{Capability("unknown.operation"), FieldTemperature, false},
		{CapabilityChatCompletions, FieldKey("arbitrary"), false},
	}
	for _, test := range tests {
		if got := HasField(test.capability, test.field); got != test.want {
			t.Errorf("HasField(%q, %q) = %v, want %v", test.capability, test.field, got, test.want)
		}
	}
}