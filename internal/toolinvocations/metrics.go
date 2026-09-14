package toolinvocations

import "sync/atomic"

// Metrics agrega somente contagens e tamanhos técnicos. Payloads e valores de
// argumentos nunca entram nesta estrutura.
type Metrics struct {
	legacyRowsRemaining       atomic.Int64
	backfillAmbiguousTotal    atomic.Uint64
	backfillRows              atomic.Uint64
	backfillBytes             atomic.Uint64
	backfillDurationNanos     atomic.Uint64
	canonicalCutoverBlocked   atomic.Uint64
	timelineQueryCount        atomic.Uint64
	timelineDurationNanos     atomic.Uint64
	timelineBytes             atomic.Uint64
	detailsBatchItems         atomic.Uint64
	detailsDurationNanos      atomic.Uint64
	detailsCacheHits          atomic.Uint64
	detailsCacheMisses        atomic.Uint64
	detailsCacheEvictionBytes atomic.Uint64
	persistenceFailures       atomic.Uint64
	messageToolPayloadBytes   atomic.Uint64
}

type MetricsSnapshot struct {
	LegacyRowsRemaining       int64
	BackfillAmbiguousTotal    uint64
	BackfillRows              uint64
	BackfillBytes             uint64
	BackfillDurationNanos     uint64
	CanonicalCutoverBlocked   uint64
	TimelineQueryCount        uint64
	TimelineDurationNanos     uint64
	TimelineBytes             uint64
	DetailsBatchItems         uint64
	DetailsDurationNanos      uint64
	DetailsCacheHits          uint64
	DetailsCacheMisses        uint64
	DetailsCacheEvictionBytes uint64
	PersistenceFailures       uint64
	MessageToolPayloadBytes   uint64
}

func (m *Metrics) SetLegacyRowsRemaining(value int64) {
	if m != nil {
		m.legacyRowsRemaining.Store(value)
	}
}

func (m *Metrics) ObserveBackfill(rows, bytes, durationNanos uint64) {
	if m == nil {
		return
	}
	m.backfillRows.Add(rows)
	m.backfillBytes.Add(bytes)
	m.backfillDurationNanos.Add(durationNanos)
}

func (m *Metrics) IncBackfillAmbiguous() {
	if m != nil {
		m.backfillAmbiguousTotal.Add(1)
	}
}

func (m *Metrics) IncCanonicalCutoverBlocked() {
	if m != nil {
		m.canonicalCutoverBlocked.Add(1)
	}
}

func (m *Metrics) ObserveTimeline(queryCount, bytes, durationNanos uint64) {
	if m == nil {
		return
	}
	m.timelineQueryCount.Add(queryCount)
	m.timelineBytes.Add(bytes)
	m.timelineDurationNanos.Add(durationNanos)
}

func (m *Metrics) ObserveDetails(batchItems, durationNanos uint64) {
	if m == nil {
		return
	}
	m.detailsBatchItems.Add(batchItems)
	m.detailsDurationNanos.Add(durationNanos)
}

func (m *Metrics) ObserveDetailsCache(hit bool, evictionBytes uint64) {
	if m == nil {
		return
	}
	if hit {
		m.detailsCacheHits.Add(1)
	} else {
		m.detailsCacheMisses.Add(1)
	}
	m.detailsCacheEvictionBytes.Add(evictionBytes)
}

func (m *Metrics) IncPersistenceFailure() {
	if m != nil {
		m.persistenceFailures.Add(1)
	}
}

func (m *Metrics) AddMessageToolPayloadBytes(bytes uint64) {
	if m != nil {
		m.messageToolPayloadBytes.Add(bytes)
	}
}

func (m *Metrics) Snapshot() MetricsSnapshot {
	if m == nil {
		return MetricsSnapshot{}
	}
	return MetricsSnapshot{
		LegacyRowsRemaining:       m.legacyRowsRemaining.Load(),
		BackfillAmbiguousTotal:    m.backfillAmbiguousTotal.Load(),
		BackfillRows:              m.backfillRows.Load(),
		BackfillBytes:             m.backfillBytes.Load(),
		BackfillDurationNanos:     m.backfillDurationNanos.Load(),
		CanonicalCutoverBlocked:   m.canonicalCutoverBlocked.Load(),
		TimelineQueryCount:        m.timelineQueryCount.Load(),
		TimelineDurationNanos:     m.timelineDurationNanos.Load(),
		TimelineBytes:             m.timelineBytes.Load(),
		DetailsBatchItems:         m.detailsBatchItems.Load(),
		DetailsDurationNanos:      m.detailsDurationNanos.Load(),
		DetailsCacheHits:          m.detailsCacheHits.Load(),
		DetailsCacheMisses:        m.detailsCacheMisses.Load(),
		DetailsCacheEvictionBytes: m.detailsCacheEvictionBytes.Load(),
		PersistenceFailures:       m.persistenceFailures.Load(),
		MessageToolPayloadBytes:   m.messageToolPayloadBytes.Load(),
	}
}
