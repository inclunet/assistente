package app

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestCommandDeckCaptureInvalidatesPendingReservationAfterCancel(t *testing.T) {
	a := deckConfiguredFixture(t)
	p := a.commandProduct.Load()
	p.deckDriver = &appDeckDriver{}
	versions, err := p.host.Snapshot(context.Background(), p.principal)
	if err != nil {
		t.Fatal(err)
	}
	generation := p.deckInputGeneration()
	reservation, err := p.beginDeckCommand(context.Background(), "test-deck", uuid.Must(uuid.NewV7()).String(), 0, versions, commandWorkspaceTabCloseID, generation, "")
	if err != nil {
		t.Fatal(err)
	}
	request := uuid.NewString()
	if err := a.BeginCommandDeckCapture(request); err != nil {
		t.Fatal(err)
	}
	if err := a.CancelCommandDeckCapture(request); err != nil {
		t.Fatal(err)
	}
	if _, err := a.TakeUICommand(reservation.Ticket); err == nil {
		t.Fatal("pre-capture reservation survived capture cancellation")
	}
	if _, err := p.beginDeckCommand(context.Background(), "test-deck", uuid.Must(uuid.NewV7()).String(), 0, versions, commandWorkspaceTabCloseID, generation, ""); err == nil {
		t.Fatal("retired reader generation dispatched after capture")
	}
}

func TestCommandDeckCaptureRejectsInvalidRequestAndLockedSession(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	p := a.commandProduct.Load()
	p.deckDriver = &appDeckDriver{}
	if err := a.BeginCommandDeckCapture("bad-id"); err == nil {
		t.Fatal("invalid capture request accepted")
	}
	if err := p.host.SetOSSessionState(context.Background(), true, true); err != nil {
		t.Fatal(err)
	}
	if err := a.BeginCommandDeckCapture(uuid.NewString()); err == nil {
		t.Fatal("capture admitted while locked")
	}
	if p.currentDeckCapture() != nil {
		t.Fatal("denied request armed capture")
	}
}
