package providers

import (
	"testing"

	"assistente/internal/llm"
)

func TestDBStorePreservaStreamIdleTimeout(t *testing.T) {
	original := &llm.ProviderConfig{
		ID:                       "provider-1",
		Name:                     "Provider",
		CompatibilityRevision:    9,
		StreamIdleTimeoutSeconds: 75,
	}

	restored, err := fromDBModel(toDBModel(original))
	if err != nil {
		t.Fatalf("round-trip: %v", err)
	}
	if restored.StreamIdleTimeoutSeconds != 75 {
		t.Fatalf("stream idle timeout=%d, esperado 75", restored.StreamIdleTimeoutSeconds)
	}
	if restored.CompatibilityRevision != 9 {
		t.Fatalf("revisão de compatibilidade=%d, esperada 9", restored.CompatibilityRevision)
	}
}
