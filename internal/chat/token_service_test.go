package chat

import (
	"math"
	"testing"
)

func TestApplyCacheDerivedStats(t *testing.T) {
	stats := &TokenStats{
		CacheReadTokens:  300,
		CacheWriteTokens: 100,
		CacheMissTokens:  600,
	}

	applyCacheDerivedStats(stats)

	if !stats.CacheTokensReported {
		t.Fatal("expected cache metrics to be marked as reported")
	}
	if math.Abs(stats.CacheHitRate-30) > 0.0001 {
		t.Fatalf("unexpected hit rate: got %.1f want 30.0", stats.CacheHitRate)
	}
}

func TestApplyCacheDerivedStatsWithoutReportedCache(t *testing.T) {
	stats := &TokenStats{}

	applyCacheDerivedStats(stats)

	if stats.CacheTokensReported {
		t.Fatal("expected empty cache metrics to be marked as not reported")
	}
	if stats.CacheHitRate != 0 {
		t.Fatalf("unexpected hit rate: got %.1f want 0.0", stats.CacheHitRate)
	}
}
