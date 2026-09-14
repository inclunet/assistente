package commandledger

import (
	"github.com/google/uuid"
	"testing"
)

func TestRequestValidationRejectsUnsupportedInputs(t *testing.T) {
	cases := map[string]func(*LocalReadRequest){
		"uuid4":       func(r *LocalReadRequest) { r.InvocationID = uuid.NewString() },
		"owner":       func(r *LocalReadRequest) { r.Owner.UserID = "" },
		"physical":    func(r *LocalReadRequest) { r.SourceType = "keyboard.local" },
		"event":       func(r *LocalReadRequest) { r.SourceType = "event" },
		"generation":  func(r *LocalReadRequest) { r.SecurityGeneration = " " },
		"fingerprint": func(r *LocalReadRequest) { r.RequestFingerprint = "" },
		"expiry":      func(r *LocalReadRequest) { r.ExpiresAt = r.ReceivedAt },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := validRequest()
			mutate(&r)
			if validateRequest(r) == nil {
				t.Fatal("solicitação inválida aceita")
			}
		})
	}
	for _, source := range []string{"palette", "ui.action", "cli"} {
		r := validRequest()
		r.SourceType = source
		if err := validateRequest(r); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStateMachineNeverRestartsTerminal(t *testing.T) {
	statuses := []Status{Evaluating, Queued, Running, Succeeded, Failed, Denied, Cancelled, CancelledStale, TimedOut, OutcomeUnknown, Status("invalid")}
	for _, from := range statuses {
		for _, to := range statuses {
			if (terminal(from) || from == to) && validTransition(from, to) {
				t.Fatalf("transição indevida %s -> %s", from, to)
			}
		}
	}
	for _, pair := range [][2]Status{{Evaluating, Queued}, {Queued, Running}, {Running, Succeeded}, {Running, OutcomeUnknown}} {
		if !validTransition(pair[0], pair[1]) {
			t.Fatal(pair)
		}
	}
	if validTransition(Evaluating, Succeeded) || validTransition(Running, TimedOut) {
		t.Fatal("conclusão sem handoff ou timeout após handoff")
	}
}
