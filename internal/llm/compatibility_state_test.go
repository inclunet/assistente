package llm

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/llmcapabilities"
)

func TestCompatibilityStateRequiresPersistBeforeRetryAndSharesOneBudget(t *testing.T) {
	persistErr := errors.New("storage unavailable")
	persist := func(context.Context, llmcapabilities.Capability, llmcapabilities.FieldKey, string) error {
		return persistErr
	}
	state := NewCompatibilityState("model-1", 4, nil, persist)
	if state.LearnAndClaimRetry(context.Background(), llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTemperature, "openai.chat.unsupported_parameter") {
		t.Fatal("retry was claimed before restriction persistence succeeded")
	}
	if state.IsUnsupported(llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTemperature) {
		t.Fatal("failed persistence published an in-memory restriction")
	}

	state.persist = func(context.Context, llmcapabilities.Capability, llmcapabilities.FieldKey, string) error { return nil }
	if state.LearnAndClaimRetry(context.Background(), llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTemperature, "openai.chat.unsupported_parameter") {
		t.Fatal("failed persistence did not consume the shared retry budget")
	}
}

func TestCompatibilityStatePublishesOnlyPersistedKnownFields(t *testing.T) {
	calls := 0
	state := NewCompatibilityState("model-1", 4, nil, func(_ context.Context, capability llmcapabilities.Capability, field llmcapabilities.FieldKey, _ string) error {
		calls++
		if capability != llmcapabilities.CapabilityResponses || field != llmcapabilities.FieldMaxOutputTokens {
			t.Fatalf("persisted unexpected restriction: %s/%s", capability, field)
		}
		return nil
	})
	if !state.LearnAndClaimRetry(context.Background(), llmcapabilities.CapabilityResponses, llmcapabilities.FieldMaxOutputTokens, "openai.responses.unsupported_parameter") {
		t.Fatal("explicit known restriction did not permit compatibility retry")
	}
	if !state.IsUnsupported(llmcapabilities.CapabilityResponses, llmcapabilities.FieldMaxOutputTokens) {
		t.Fatal("persisted restriction was not published to this turn")
	}
	if state.LearnAndClaimRetry(context.Background(), llmcapabilities.CapabilityResponses, llmcapabilities.FieldTemperature, "openai.responses.unsupported_parameter") {
		t.Fatal("second compatibility retry was permitted")
	}
	if state.LearnAndClaimRetry(context.Background(), llmcapabilities.CapabilityResponses, "arbitrary_field", "openai.responses.unsupported_parameter") {
		t.Fatal("unknown field was permitted")
	}
	if calls != 1 {
		t.Fatalf("persistence called %d times, want 1", calls)
	}
}
