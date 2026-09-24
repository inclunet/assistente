package commandconfig

import (
	"context"
	"errors"
	"testing"
)

func TestPaletteTriggerPortClosedGrammar(t *testing.T) {
	port := PaletteTriggerPort{}
	got, err := port.Normalize(context.Background(), []byte(`{"version":1,"selection":"workspace.list"}`))
	if err != nil || got != "palette:workspace.list" {
		t.Fatalf("normalize: %q %v", got, err)
	}
	if err := port.ValidateIdentity(context.Background(), got); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`{"version":1.0,"selection":"workspace.list"}`,
		`{"version":1,"selection":"workspace.list","selection":"other.command"}`,
		`{"version":1,"selection":"workspace.list","source":"keyboard.local"}`,
		`{"version":1,"selection":" workspace.list"}`,
		`{"version":1,"selection":"workspace"}`,
		`{"version":1,"selection":null}`,
		`{"selection":"workspace.list"}`,
		`{"version":1,"selection":"workspace.list"} {}`,
	} {
		if _, err := port.Normalize(context.Background(), []byte(raw)); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted %s: %v", raw, err)
		}
	}
	for _, identity := range []string{"keyboard.local:Control+KeyK", "palette:", "palette:workspace.list:forged"} {
		if err := port.ValidateIdentity(context.Background(), identity); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted identity %q: %v", identity, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := port.Normalize(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := port.Normalize(nil, nil); !errors.Is(err, ErrInvalid) { //nolint:staticcheck // Prova a recusa explícita de contexto nil.
		t.Fatal(err)
	}
}
