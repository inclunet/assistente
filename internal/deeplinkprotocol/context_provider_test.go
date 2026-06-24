package deeplinkprotocol

import (
	"context"
	"strings"
	"testing"

	"assistente/internal/contextprovider"
)

func TestContextProviderBuildsDeeplinkProtocol(t *testing.T) {
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1", len(blocks))
	}
	block := blocks[0]
	if block.Provider != "deeplink_protocol" || block.Name != "deeplink_protocol" {
		t.Fatalf("unexpected block identity: %+v", block)
	}
	if block.Volatility != contextprovider.VolatilityStable || block.Priority != 9 {
		t.Fatalf("unexpected block ordering metadata: %+v", block)
	}
	for _, needle := range []string{"<deeplink_protocol>", "link= values", "Markdown links", "does not grant content access", "assistente://", "open_deep_link"} {
		if !strings.Contains(block.Content, needle) {
			t.Fatalf("protocol block missing %q: %q", needle, block.Content)
		}
	}
}

func TestContextProviderTruncatesProtocolInsteadOfDroppingIt(t *testing.T) {
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		ProviderBudgets: map[string]int{"deeplink_protocol": 170},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want truncated protocol", len(blocks))
	}
	if runeLen(blocks[0].Content) > 170 {
		t.Fatalf("protocol length = %d, want <= 170: %q", runeLen(blocks[0].Content), blocks[0].Content)
	}
	if !strings.Contains(blocks[0].Content, "omitted due to context budget") {
		t.Fatalf("expected truncation notice: %q", blocks[0].Content)
	}
	if !strings.Contains(blocks[0].Content, "<deeplink_protocol>") || !strings.Contains(blocks[0].Content, "</deeplink_protocol>") {
		t.Fatalf("truncated protocol should keep tags: %q", blocks[0].Content)
	}
}

func TestContextProviderOmitsProtocolWhenTaggedEnvelopeCannotFit(t *testing.T) {
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		ProviderBudgets: map[string]int{"deeplink_protocol": 10},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 0 {
		t.Fatalf("len(blocks) = %d, want omitted protocol when tagged envelope cannot fit: %+v", len(blocks), blocks)
	}
}
