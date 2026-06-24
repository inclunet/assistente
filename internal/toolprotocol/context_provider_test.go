package toolprotocol

import (
	"context"
	"strings"
	"testing"

	"assistente/internal/contextprovider"
	"assistente/internal/tools"
)

func TestContextProviderBuildsCatalogFirstProtocol(t *testing.T) {
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		ToolCallingEnabled: true,
		EnabledTools:       []string{tools.ToolCatalogName, tools.LoadSkillName},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1", len(blocks))
	}
	block := blocks[0]
	if block.Provider != "tool_protocol" || block.Name != "tool_selection_protocol" {
		t.Fatalf("unexpected block identity: %+v", block)
	}
	if block.Volatility != contextprovider.VolatilityStable || block.Priority != 8 {
		t.Fatalf("unexpected block ordering metadata: %+v", block)
	}
	if !strings.Contains(block.Content, "<tool_selection_protocol>") || !strings.Contains(block.Content, tools.ToolCatalogName) {
		t.Fatalf("unexpected protocol content: %q", block.Content)
	}
}

func TestContextProviderOmitsProtocolWhenCatalogFirstInactive(t *testing.T) {
	cases := []contextprovider.BuildRequest{
		{ToolCallingEnabled: false, EnabledTools: []string{tools.ToolCatalogName}},
		{ToolCallingEnabled: true, EnabledTools: []string{"read_file"}},
		{ToolCallingEnabled: true, EnabledTools: []string{tools.ToolCatalogName, "read_file"}},
	}
	for _, req := range cases {
		blocks, err := NewContextProvider().Build(context.Background(), req)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		if len(blocks) != 0 {
			t.Fatalf("expected no protocol for req %+v, got %+v", req, blocks)
		}
	}
}

func TestContextProviderTruncatesProtocolInsteadOfDroppingIt(t *testing.T) {
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		ToolCallingEnabled: true,
		EnabledTools:       []string{tools.ToolCatalogName},
		ProviderBudgets:    map[string]int{"tool_protocol": 180},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want truncated protocol", len(blocks))
	}
	if blocks[0].Name != "tool_selection_protocol" {
		t.Fatalf("unexpected block: %+v", blocks[0])
	}
	if runeLen(blocks[0].Content) > 180 {
		t.Fatalf("protocol length = %d, want <= 180: %q", runeLen(blocks[0].Content), blocks[0].Content)
	}
	if !strings.Contains(blocks[0].Content, "omitted due to context budget") {
		t.Fatalf("expected truncation notice: %q", blocks[0].Content)
	}
}

func TestContextProviderOmitsProtocolWhenTaggedEnvelopeCannotFit(t *testing.T) {
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		ToolCallingEnabled: true,
		EnabledTools:       []string{tools.ToolCatalogName},
		ProviderBudgets:    map[string]int{"tool_protocol": 10},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 0 {
		t.Fatalf("len(blocks) = %d, want omitted protocol when tagged envelope cannot fit: %+v", len(blocks), blocks)
	}
}
