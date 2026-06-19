package contextprovider

import (
	"context"
	"errors"
	"testing"
)

type testProvider struct {
	name   string
	blocks []Block
	err    error
}

func (p testProvider) Name() string { return p.name }

func (p testProvider) Build(context.Context, BuildRequest) ([]Block, error) {
	if p.err != nil {
		return p.blocks, p.err
	}
	return p.blocks, nil
}

func TestRegistryBuildSortsByVolatilityPriorityAndProvider(t *testing.T) {
	registry := NewRegistry(
		testProvider{name: "workspace", blocks: []Block{{Name: "workspace", Volatility: VolatilityFastDynamic, Priority: 100, Content: "workspace"}}},
		testProvider{name: "memory", blocks: []Block{{Name: "memory", Volatility: VolatilitySlowDynamic, Priority: 100, Content: "memory"}}},
		testProvider{name: "stable", blocks: []Block{{Name: "instructions", Volatility: VolatilityStable, Priority: 100, Content: "stable"}}},
	)

	blocks, err := registry.Build(context.Background(), BuildRequest{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	rendered := RenderBlocks(blocks)
	want := []string{"stable", "memory", "workspace"}
	if len(rendered) != len(want) {
		t.Fatalf("rendered len = %d, want %d: %#v", len(rendered), len(want), rendered)
	}
	for i := range want {
		if rendered[i] != want[i] {
			t.Fatalf("rendered[%d] = %q, want %q (all=%#v)", i, rendered[i], want[i], rendered)
		}
	}
}

func TestRegistryBuildSkipsFailedProvider(t *testing.T) {
	registry := NewRegistry(
		testProvider{name: "memory", err: errors.New("boom")},
		testProvider{name: "workspace", blocks: []Block{{Name: "workspace", Volatility: VolatilityFastDynamic, Priority: 100, Content: "workspace"}}},
	)

	blocks, err := registry.Build(context.Background(), BuildRequest{})
	if err != nil {
		t.Fatalf("Build returned provider error: %v", err)
	}
	rendered := RenderBlocks(blocks)
	if len(rendered) != 1 || rendered[0] != "workspace" {
		t.Fatalf("rendered = %#v, want workspace only", rendered)
	}
}

func TestRegistryBuildKeepsPartialBlocksFromFailedProvider(t *testing.T) {
	registry := NewRegistry(
		testProvider{name: "memory", blocks: []Block{{Name: "instructions", Volatility: VolatilityStable, Priority: 10, Content: "stable memory instructions"}}, err: errors.New("boom")},
		testProvider{name: "workspace", blocks: []Block{{Name: "workspace", Volatility: VolatilityFastDynamic, Priority: 100, Content: "workspace"}}},
	)

	blocks, err := registry.Build(context.Background(), BuildRequest{})
	if err != nil {
		t.Fatalf("Build returned provider error: %v", err)
	}
	rendered := RenderBlocks(blocks)
	want := []string{"stable memory instructions", "workspace"}
	if len(rendered) != len(want) {
		t.Fatalf("rendered len = %d, want %d: %#v", len(rendered), len(want), rendered)
	}
	for i := range want {
		if rendered[i] != want[i] {
			t.Fatalf("rendered[%d] = %q, want %q (all=%#v)", i, rendered[i], want[i], rendered)
		}
	}
}

func TestRegistryBuildSkipsDisabledProvider(t *testing.T) {
	registry := NewRegistry(
		testProvider{name: "memory", blocks: []Block{{Name: "memory", Volatility: VolatilitySlowDynamic, Content: "memory"}}},
		testProvider{name: "workspace", blocks: []Block{{Name: "workspace", Volatility: VolatilityFastDynamic, Content: "workspace"}}},
	)

	blocks, err := registry.Build(context.Background(), BuildRequest{
		ProviderEnabled: map[string]bool{
			"memory":    false,
			"workspace": true,
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	rendered := RenderBlocks(blocks)
	if len(rendered) != 1 || rendered[0] != "workspace" {
		t.Fatalf("rendered = %#v, want workspace only", rendered)
	}
}

func TestRegistryMetadataReturnsEmptySliceWhenNoProviders(t *testing.T) {
	registry := NewRegistry()

	metadata := registry.Metadata()
	if metadata == nil {
		t.Fatal("metadata = nil, want empty slice")
	}
	if len(metadata) != 0 {
		t.Fatalf("len(metadata) = %d, want 0", len(metadata))
	}

	var nilRegistry *Registry
	nilMetadata := nilRegistry.Metadata()
	if nilMetadata == nil {
		t.Fatal("nil registry metadata = nil, want empty slice")
	}
	if len(nilMetadata) != 0 {
		t.Fatalf("len(nil registry metadata) = %d, want 0", len(nilMetadata))
	}
}

func TestRegistryMetadataUsesProviderFallback(t *testing.T) {
	registry := NewRegistry(testProvider{name: "memory"})

	metadata := registry.Metadata()
	if len(metadata) != 1 {
		t.Fatalf("len(metadata) = %d, want 1", len(metadata))
	}
	if metadata[0].Name != "memory" || !metadata[0].DefaultEnabled {
		t.Fatalf("metadata = %+v, want fallback metadata for memory", metadata[0])
	}
}
