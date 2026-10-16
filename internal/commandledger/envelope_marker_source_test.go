package commandledger

import (
	"context"
	"testing"

	"assistente/internal/commandcontract"
)

func TestEnvelopeMarkerRetainsDetachedSourceWithoutInventingAudit(t *testing.T) {
	for _, stale := range []bool{false, true} {
		req, now := envelopeRequest(t)
		req.Mode, req.RejectedStale = ModeSuppress, stale
		req.Envelope.CommandID, req.Envelope.Arguments = nil, nil
		req.Envelope.TriggerType = stringPtr("palette")
		req.Envelope.TriggerSpec = rawPtr(`{"version":1,"selection":"workspace.list"}`)
		store, db := testStore(t, &now)
		reserved, err := store.ReserveEnvelope(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if reserved.Record.SourceType == nil || *reserved.Record.SourceType != commandcontract.SourcePalette || reserved.Record.Envelope.Version != 0 {
			t.Fatalf("marker lost source or fabricated envelope: %+v", reserved.Record)
		}
		*reserved.Record.SourceType = commandcontract.SourceSystem
		got, err := store.GetEnvelopeByID(context.Background(), reserved.Record.Ownership, req.Envelope.InvocationID)
		if err != nil || got.SourceType == nil || *got.SourceType != commandcontract.SourcePalette {
			t.Fatalf("source alias: %+v %v", got, err)
		}
		want := Suppressed
		if stale {
			want = RejectedStale
		}
		if got.Status != want {
			t.Fatalf("status %s, want %s", got.Status, want)
		}
		var count int64
		if err := db.Model(&invocationRow{}).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("unexpected audit: %d %v", count, err)
		}
	}
}
