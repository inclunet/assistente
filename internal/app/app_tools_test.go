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
