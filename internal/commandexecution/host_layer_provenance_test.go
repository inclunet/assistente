package commandexecution

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/commandbindings"
)

func TestHostEquivalentLayerProvenancePreservesWatchAndVersions(t *testing.T) {
	state, principal, base := readyHostForRebuild(t)
	metadata := map[string][]commandbindings.LayerProvenance{
		"layer.guard": {{SourceID: "activation-1", Provenance: nil}},
	}
	first, err := base.WithLayerProvenance(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := publishGuardedHost(t, state, principal, first, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}

	before, err := state.Snapshot(context.Background(), principal)
	if err != nil {
		t.Fatal(err)
	}
	epoch, err := state.Epochs().Capture(context.Background(), principal.UserID, principal.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	runContext, release, err := state.Epochs().WatchEpoch(context.Background(), epoch)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	second, err := base.WithLayerProvenance(map[string][]commandbindings.LayerProvenance{
		"layer.guard": {{SourceID: "activation-1", Provenance: nil}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if first == second || !first.Equivalent(second) {
		t.Fatal("fixtures não representam snapshots equivalentes distintos")
	}
	if err := publishGuardedHost(t, state, principal, second, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	after, err := state.Snapshot(context.Background(), principal)
	if err != nil {
		t.Fatal(err)
	}
	if after.GlobalConfig != before.GlobalConfig || after.ActiveLayers != before.ActiveLayers {
		t.Fatalf("rebuild equivalente alterou versões: before=%+v after=%+v", before, after)
	}
	select {
	case <-runContext.Done():
		t.Fatal("rebuild equivalente cancelou watch")
	default:
	}
}

func TestHostLayerProvenanceChangeInvalidatesWatchAndVersions(t *testing.T) {
	tests := []struct {
		name   string
		second commandbindings.LayerProvenance
	}{
		{name: "source id", second: commandbindings.LayerProvenance{SourceID: "activation-2", Provenance: nil}},
		{name: "provenance", second: commandbindings.LayerProvenance{SourceID: "activation-1", Provenance: json.RawMessage(`{"revision":2}`)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, principal, base := readyHostForRebuild(t)
			first, err := base.WithLayerProvenance(map[string][]commandbindings.LayerProvenance{
				"layer.guard": {{SourceID: "activation-1", Provenance: nil}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := publishGuardedHost(t, state, principal, first, func(context.Context) error { return nil }); err != nil {
				t.Fatal(err)
			}
			before, err := state.Snapshot(context.Background(), principal)
			if err != nil {
				t.Fatal(err)
			}
			epoch, err := state.Epochs().Capture(context.Background(), principal.UserID, principal.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			runContext, release, err := state.Epochs().WatchEpoch(context.Background(), epoch)
			if err != nil {
				t.Fatal(err)
			}
			defer release()

			second, err := base.WithLayerProvenance(map[string][]commandbindings.LayerProvenance{
				"layer.guard": {tt.second},
			})
			if err != nil {
				t.Fatal(err)
			}
			if first.Equivalent(second) {
				t.Fatal("fixture alterado ainda é equivalente")
			}
			if err := publishGuardedHost(t, state, principal, second, func(context.Context) error { return nil }); err != nil {
				t.Fatal(err)
			}
			after, err := state.Snapshot(context.Background(), principal)
			if err != nil {
				t.Fatal(err)
			}
			if after.GlobalConfig == before.GlobalConfig || after.ActiveLayers == before.ActiveLayers {
				t.Fatalf("mudança de metadata não alterou versões: before=%+v after=%+v", before, after)
			}
			select {
			case <-runContext.Done():
			default:
				t.Fatal("mudança de metadata preservou watch")
			}
		})
	}
}
