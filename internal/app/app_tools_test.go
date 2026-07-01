package app

import "testing"

func TestNormalizeRuntimeToolCatalogLimit(t *testing.T) {
	tests := []struct {
		name  string
		limit int
		want  int
	}{
		{name: "default zero", limit: 0, want: 20},
		{name: "default negative", limit: -1, want: 20},
		{name: "preserve valid", limit: 12, want: 12},
		{name: "clamp maximum", limit: 51, want: 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeRuntimeToolCatalogLimit(tt.limit); got != tt.want {
				t.Fatalf("normalizeRuntimeToolCatalogLimit(%d) = %d, want %d", tt.limit, got, tt.want)
			}
		})
	}
}

func TestNormalizeRuntimeToolCatalogOffset(t *testing.T) {
	tests := []struct {
		name   string
		offset int
		want   int
	}{
		{name: "default negative", offset: -1, want: 0},
		{name: "preserve zero", offset: 0, want: 0},
		{name: "preserve positive", offset: 50, want: 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeRuntimeToolCatalogOffset(tt.offset); got != tt.want {
				t.Fatalf("normalizeRuntimeToolCatalogOffset(%d) = %d, want %d", tt.offset, got, tt.want)
			}
		})
	}
}

func TestRuntimeToolCatalogFilterToToolsNormalizesStrings(t *testing.T) {
	got := runtimeToolCatalogFilterToTools(RuntimeToolCatalogFilter{
		Origin:             " mcp_bridge ",
		MCPServerID:        " server-id ",
		Category:           " mcp:github ",
		Class:              " mcp_tool ",
		Package:            " coding ",
		Risk:               " network ",
		AvailabilityStatus: " available ",
		IncludeUnavailable: true,
		Limit:              99,
		Offset:             50,
	})

	if got.Origin != "mcp_bridge" ||
		got.MCPServerID != "server-id" ||
		got.Category != "mcp:github" ||
		got.Class != "mcp_tool" ||
		got.Package != "coding" ||
		got.Risk != "network" ||
		got.AvailabilityStatus != "available" {
		t.Fatalf("filter was not normalized: %#v", got)
	}
	if !got.IncludeUnavailable {
		t.Fatal("IncludeUnavailable should be preserved")
	}
	if got.Limit != 50 {
		t.Fatalf("Limit = %d, want 50", got.Limit)
	}
	if got.Offset != 50 {
		t.Fatalf("Offset = %d, want 50", got.Offset)
	}
}
