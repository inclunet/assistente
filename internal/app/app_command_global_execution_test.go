package app

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/commandui"
	"assistente/internal/database"
	"github.com/google/uuid"
)

// Controller tests establish native-only minting. This test starts at the
// private adapter boundary and exercises the real resolver, ledger and UI broker.
func globalVoiceExecutionFixture(t *testing.T) (*App, commandGlobalBinding) {
	t.Helper()
	a := readyCommandProduct(t)
	p := a.commandProduct.Load()
	b, err := newCommandGlobalBinding(commandGlobalVoiceID, "Ctrl+Shift+A", "profile-input-fingerprint", map[string]any{"profile_slug": "voice-profile", "trigger_type": "hotkey", "bring_to_front": true})
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := commandbindings.NewConfiguration([]commandbindings.Default{{Version: "1", Fingerprint: b.Fingerprint, Candidate: commandbindings.Candidate{ID: b.ID, Trigger: b.Identity, CommandID: b.CommandID, ArgumentsKey: string(b.Arguments), ExecutionScopeKey: "global", Scope: commandbindings.Application, Enabled: true, LayerActive: true}}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = p.host.RebuildUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) { return p.principal, nil }, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return configuration, nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return a, b
}

func reserveGlobalVoiceForTest(t *testing.T, a *App, b commandGlobalBinding, current func() bool) commandui.Reservation {
	t.Helper()
	p := a.commandProduct.Load()
	o, err := p.prepareGlobalOccurrence(context.Background(), b, current)
	if err != nil {
		t.Fatal(err)
	}
	o.validate = func(context.Context) error { return nil }
	r, err := a.beginCommandUI(p, b.CommandID, func(r commandui.Reservation) (commandexecution.EnvelopeCandidate, error) {
		if err := p.addGlobalOccurrence(r.InvocationID, o); err != nil {
			return commandexecution.EnvelopeCandidate{}, err
		}
		return commandexecution.EnvelopeCandidate{InvocationID: r.InvocationID, CorrelationID: r.InvocationID, TriggerType: string(commandcatalog.KeyboardGlobal), TriggerSpec: b.Trigger, Arguments: json.RawMessage(`{}`)}, nil
	}, func(ctx context.Context, c commandexecution.EnvelopeCandidate) (commandledger.FullRecord, error) {
		defer p.removeGlobalOccurrence(c.InvocationID)
		return p.globalExecution.service.ExecuteEnvelope(ctx, "", c)
	}, current)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCommandGlobalVoiceUsesExecutorAndAuthoritativeHandoff(t *testing.T) {
	a, b := globalVoiceExecutionFixture(t)
	r := reserveGlobalVoiceForTest(t, a, b, func() bool { return true })
	h, err := a.TakeGlobalVoiceCommand(r.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	if h.CommandID != b.CommandID || h.InvocationID != r.InvocationID || h.ProfileSlug != "voice-profile" || h.TriggerType != "hotkey" || !h.BringToFront {
		t.Fatalf("handoff=%+v", h)
	}
	if _, err := a.TakeGlobalVoiceCommand(r.Ticket); err == nil {
		t.Fatal("second take accepted")
	}
	if err := a.CompleteUICommand(r.Ticket, h.HandoffID, string(commandledger.Succeeded)); err != nil {
		t.Fatal(err)
	}
	result := getUIResultEventually(t, a, r.Ticket)
	if result.Status != string(commandledger.Succeeded) {
		t.Fatalf("result=%+v", result)
	}
	var row struct {
		SourceType string
		Status     string
	}
	if err := database.DB().Table("command_invocations").Select("source_type,status").Where("invocation_id = ?", r.InvocationID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.SourceType != "keyboard.global" || row.Status != "succeeded" {
		t.Fatalf("ledger=%+v", row)
	}
	var count int64
	if err := database.DB().Table("command_invocations").Where("invocation_id = ?", r.InvocationID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestCommandGlobalVoiceRejectsRetiredAndForgedOccurrences(t *testing.T) {
	a, b := globalVoiceExecutionFixture(t)
	var live atomic.Bool
	live.Store(true)
	r := reserveGlobalVoiceForTest(t, a, b, live.Load)
	live.Store(false)
	if _, err := a.TakeGlobalVoiceCommand(r.Ticket); err == nil {
		t.Fatal("retired target was delivered")
	}
	_ = a.CancelUICommand(r.Ticket)
	if a.AdmitGlobalCommandOccurrence(uuid.NewString(), true) {
		t.Fatal("forged admission accepted")
	}
	if a.AdmitGlobalCommandOccurrence(r.InvocationID, true) {
		t.Fatal("voice occurrence admitted as job")
	}
	p := a.commandProduct.Load()
	id := uuid.Must(uuid.NewV7()).String()
	record, err := p.globalExecution.service.ExecuteEnvelope(context.Background(), "", commandexecution.EnvelopeCandidate{InvocationID: id, CorrelationID: id, TriggerType: string(commandcatalog.KeyboardGlobal), TriggerSpec: b.Trigger, Arguments: json.RawMessage(`{}`)})
	if err == nil && record.Status == commandledger.Succeeded {
		t.Fatal("unregistered occurrence executed")
	}
}

func TestCommandGlobalVoiceRevalidatesProfileBeforeTake(t *testing.T) {
	a, b := globalVoiceExecutionFixture(t)
	r := reserveGlobalVoiceForTest(t, a, b, func() bool { return true })
	p := a.commandProduct.Load()
	o, ok := p.globalOccurrence(r.InvocationID)
	if !ok {
		t.Fatal("private occurrence missing")
	}
	o.validate = func(context.Context) error { return commandexecution.ErrStale }
	if _, err := a.TakeGlobalVoiceCommand(r.Ticket); err == nil {
		t.Fatal("changed profile delivered")
	}
	result, err := a.GetUICommandResult(r.Ticket)
	if err == nil && result.Status == string(commandledger.Succeeded) {
		t.Fatal("changed profile succeeded")
	}
}

func TestCommandGlobalJobAdmissionIsNativeBoundAndOneShot(t *testing.T) {
	a, _ := globalVoiceExecutionFixture(t)
	p := a.commandProduct.Load()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	id := uuid.Must(uuid.NewV7()).String()
	o := &commandGlobalOccurrence{ctx: ctx, current: func() bool { return true }, binding: commandGlobalBinding{CommandID: commandGlobalJobID}, admission: make(chan bool, 1)}
	if err := p.addGlobalOccurrence(id, o); err != nil {
		t.Fatal(err)
	}
	defer p.removeGlobalOccurrence(id)
	if !a.AdmitGlobalCommandOccurrence(id, false) {
		t.Fatal("native occurrence veto not accepted")
	}
	if allowed := <-o.admission; allowed {
		t.Fatal("veto admitted a job")
	}
	if a.AdmitGlobalCommandOccurrence(id, true) {
		t.Fatal("second admission overrode veto")
	}
	cancel()
	if a.AdmitGlobalCommandOccurrence(id, true) {
		t.Fatal("cancelled occurrence admitted")
	}
}

func TestCommandGlobalCatalogDoesNotOpenPublicOrAudioIngress(t *testing.T) {
	a := readyCommandProduct(t)
	registry, handlers, err := a.commandProductCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{commandGlobalVoiceID, commandGlobalJobID} {
		definition, ok := registry.Lookup(id)
		if !ok || len(definition.AllowedSources) != 1 || !definition.AllowsSource(commandcatalog.KeyboardGlobal) {
			t.Fatalf("global source contract: %+v", definition)
		}
		if _, err := a.BeginUICommand(id); err == nil {
			t.Fatalf("public palette ingress accepted %s", id)
		}
	}
	if handlers[commandGlobalJobID].ExecutionTimeout != 5*time.Minute || !handlers[commandGlobalJobID].RuntimeOwnsDeadline {
		t.Fatal("job must bound admission independently of the existing job runtime")
	}
	voice, _ := registry.Lookup(commandGlobalVoiceID)
	if _, err := voice.ValidateArguments(json.RawMessage(`{"profile_slug":"voice-profile","trigger_type":"hotkey","bring_to_front":false,"audio":"must-not-enter-the-command-envelope"}`)); err == nil {
		t.Fatal("audio accepted into voice command envelope")
	}
}

func TestCommandGlobalShutdownRetiresNativeAdmission(t *testing.T) {
	a := readyCommandProduct(t)
	p := a.commandProduct.Load()
	id := uuid.Must(uuid.NewV7()).String()
	o := &commandGlobalOccurrence{ctx: context.Background(), current: func() bool { return true }, binding: commandGlobalBinding{CommandID: commandGlobalJobID}, admission: make(chan bool, 1)}
	if err := p.addGlobalOccurrence(id, o); err != nil {
		t.Fatal(err)
	}
	defer p.removeGlobalOccurrence(id)
	// Even an exhausted drain budget must retire native admission immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = p.Shutdown(ctx)
	if p.globalExecution.ctx.Err() == nil {
		t.Fatal("global lifetime survived shutdown")
	}
	if _, ok := p.globalOccurrence(id); ok {
		t.Fatal("native occurrence survived shutdown")
	}
	if a.AdmitGlobalCommandOccurrence(id, true) {
		t.Fatal("retired native admission accepted")
	}
}
