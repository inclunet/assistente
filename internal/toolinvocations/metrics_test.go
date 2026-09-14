package toolinvocations

import "testing"

func TestMetricsSnapshotAgregaSomenteMedidas(t *testing.T) {
	var metrics Metrics
	metrics.SetLegacyRowsRemaining(7)
	metrics.ObserveBackfill(2, 128, 30)
	metrics.IncBackfillAmbiguous()
	metrics.IncCanonicalCutoverBlocked()
	metrics.ObserveTimeline(3, 2048, 40)
	metrics.ObserveDetails(5, 50)
	metrics.ObserveDetailsCache(true, 64)
	metrics.ObserveDetailsCache(false, 0)
	metrics.IncPersistenceFailure()
	metrics.AddMessageToolPayloadBytes(12)

	got := metrics.Snapshot()
	if got.LegacyRowsRemaining != 7 ||
		got.BackfillRows != 2 ||
		got.BackfillBytes != 128 ||
		got.BackfillDurationNanos != 30 ||
		got.BackfillAmbiguousTotal != 1 ||
		got.CanonicalCutoverBlocked != 1 ||
		got.TimelineQueryCount != 3 ||
		got.TimelineBytes != 2048 ||
		got.TimelineDurationNanos != 40 ||
		got.DetailsBatchItems != 5 ||
		got.DetailsDurationNanos != 50 ||
		got.DetailsCacheHits != 1 ||
		got.DetailsCacheMisses != 1 ||
		got.DetailsCacheEvictionBytes != 64 ||
		got.PersistenceFailures != 1 ||
		got.MessageToolPayloadBytes != 12 {
		t.Fatalf("snapshot inesperado: %+v", got)
	}
}

func TestNilMetricsSaoNoOp(t *testing.T) {
	var metrics *Metrics
	metrics.ObserveBackfill(1, 2, 3)
	metrics.ObserveTimeline(1, 2, 3)
	metrics.ObserveDetails(1, 2)
	metrics.ObserveDetailsCache(false, 3)
	metrics.IncPersistenceFailure()
	if got := metrics.Snapshot(); got != (MetricsSnapshot{}) {
		t.Fatalf("snapshot nil inesperado: %+v", got)
	}
}
