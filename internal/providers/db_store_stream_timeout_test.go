package providers

import (
	"testing"

	"assistente/internal/llm"
)

func TestDBStorePreservaStreamIdleTimeout(t *testing.T) {
	original := &llm.ProviderConfig{
		ID:                       "provider-1",
		Name:                     "Provider",
		StreamIdleTimeoutSeconds: 75,
	}

	restored, err := fromDBModel(toDBModel(original))
	if err != nil {
		t.Fatalf("round-trip: %v", err)
	}
	if restored.StreamIdleTimeoutSeconds != 75 {
		t.Fatalf("stream idle timeout=%d, esperado 75", restored.StreamIdleTimeoutSeconds)
	}
}
