package speech

import "testing"

func TestMapRateToSAPI5(t *testing.T) {
	tests := []struct {
		name     string
		rate     float64
		expected int
	}{
		{"default 1.0", 1.0, 0},
		{"zero", 0, 0},
		{"negative", -1, 0},
		{"slow 0.5", 0.5, -6},
		{"very slow 0.25", 0.25, -10},
		{"fast 2.0", 2.0, 3},
		{"very fast 4.0", 4.0, 10},
		{"slightly slow 0.85", 0.85, -2},
		{"slightly fast 1.3", 1.3, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapRateToSAPI5(tt.rate)
			if got != tt.expected {
				t.Errorf("mapRateToSAPI5(%v) = %d, want %d", tt.rate, got, tt.expected)
			}
		})
	}
}

func TestMapRateToSAPI5_Clamp(t *testing.T) {
	// Rate extremamente alto — deve clampar em 10
	got := mapRateToSAPI5(100.0)
	if got > 10 {
		t.Errorf("mapRateToSAPI5(100.0) = %d, want <= 10", got)
	}

	// Rate extremamente baixo — deve clampar em -10
	got = mapRateToSAPI5(0.01)
	if got < -10 {
		t.Errorf("mapRateToSAPI5(0.01) = %d, want >= -10", got)
	}
}
