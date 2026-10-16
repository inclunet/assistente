package app

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/hotkey"
)

type decisionRepeatFixture struct {
	app         *App
	callbacks   []hotkey.HotkeyCallback
	events      []decisionRepeatEvent
	releases    int
	allowed     bool
	registerErr error
	releaseErr  error
}

func newDecisionRepeatFixture(t *testing.T) *decisionRepeatFixture {
	t.Helper()
	f := &decisionRepeatFixture{app: &App{}, allowed: true}
	f.app.decisionRepeatHotkeys = newDecisionRepeatHotkeys(func(callback hotkey.HotkeyCallback) (func() error, error) {
		if f.registerErr != nil {
			return nil, f.registerErr
		}
		f.callbacks = append(f.callbacks, callback)
		return func() error { f.releases++; return f.releaseErr }, nil
	}, func(event decisionRepeatEvent) { f.events = append(f.events, event) }, func() bool { return f.allowed })
	t.Cleanup(f.app.decisionRepeatHotkeys.shutdown)
	return f
}

func TestDecisionRepeatHotkeyScopesEventsAndRestoresOnlyAfterClose(t *testing.T) {
	f := newDecisionRepeatFixture(t)
	session := f.app.OpenDecisionRepeatHotkeySession()
	if session == "" {
		t.Fatal("missing session")
	}
	if err := f.app.SetDecisionRepeatHotkey(session, 1, "decision-a"); err != nil {
		t.Fatal(err)
	}
	f.callbacks[0]()
	if len(f.events) != 1 || f.events[0] != (decisionRepeatEvent{session, 1, "decision-a"}) {
		t.Fatalf("events=%+v", f.events)
	}
	if err := f.app.SetDecisionRepeatHotkey(session, 1, "decision-a"); err != nil {
		t.Fatal(err)
	}
	if len(f.callbacks) != 1 || f.releases != 0 {
		t.Fatal("renewal recreated native registration")
	}
	if err := f.app.SetDecisionRepeatHotkey(session, 2, "decision-b"); err != nil {
		t.Fatal(err)
	}
	f.callbacks[0]()
	if len(f.events) != 1 {
		t.Fatal("old callback escaped after topmost changed")
	}
	f.callbacks[1]()
	if len(f.events) != 2 || f.events[1].DialogID != "decision-b" {
		t.Fatalf("events=%+v", f.events)
	}
	if err := f.app.SetDecisionRepeatHotkey(session, 3, ""); err != nil {
		t.Fatal(err)
	}
	f.callbacks[1]()
	if f.releases != 2 || len(f.events) != 2 {
		t.Fatalf("releases=%d events=%+v", f.releases, f.events)
	}
}

func TestDecisionRepeatHotkeyRejectsStaleRevisionsAndRenderer(t *testing.T) {
	f := newDecisionRepeatFixture(t)
	session := f.app.OpenDecisionRepeatHotkeySession()
	if err := f.app.SetDecisionRepeatHotkey(session, 2, "a"); err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct {
		session  string
		revision uint64
		dialog   string
	}{
		{session, 0, "a"}, {session, 1, ""}, {session, 2, "b"}, {"foreign", 3, "a"},
		{session, decisionRepeatMaxRevision + 1, "a"}, {session, 3, " whitespace "},
	} {
		if err := f.app.SetDecisionRepeatHotkey(input.session, input.revision, input.dialog); err == nil {
			t.Fatalf("accepted invalid update %+v", input)
		}
	}
	newSession := f.app.OpenDecisionRepeatHotkeySession()
	if newSession == "" || newSession == session {
		t.Fatal("renderer identity reused")
	}
	if err := f.app.SetDecisionRepeatHotkey(newSession, 1, "new-renderer"); err != nil {
		t.Fatal(err)
	}
	if err := f.app.SetDecisionRepeatHotkey(session, 3, "old-renderer"); err == nil {
		t.Fatal("old renderer replaced new")
	}
	if err := f.app.CloseDecisionRepeatHotkeySession(session); err != nil {
		t.Fatal(err)
	}
	f.callbacks[0]()
	f.callbacks[1]()
	if len(f.events) != 1 || f.events[0].SessionID != newSession {
		t.Fatalf("events=%+v", f.events)
	}
}

func TestDecisionRepeatHotkeyLeaseExpiryAndRenewal(t *testing.T) {
	f := newDecisionRepeatFixture(t)
	session := f.app.OpenDecisionRepeatHotkeySession()
	if err := f.app.SetDecisionRepeatHotkey(session, 1, "a"); err != nil {
		t.Fatal(err)
	}
	s := f.app.decisionRepeatHotkeys
	entry := s.active.Load()
	entry.expires.Store(time.Now().Add(-time.Second).UnixNano())
	f.callbacks[0]()
	if len(f.events) != 0 {
		t.Fatal("expired lease emitted before cleanup")
	}
	if err := f.app.SetDecisionRepeatHotkey(session, 1, "a"); err != nil {
		t.Fatal(err)
	}
	s.expire(entry) // Callback antigo do timer não remove uma lease renovada.
	if s.active.Load() != entry || f.releases != 0 {
		t.Fatal("old expiry removed renewed lease")
	}
	entry.expires.Store(time.Now().Add(-time.Second).UnixNano())
	s.expire(entry)
	if s.active.Load() != nil || f.releases != 1 {
		t.Fatal("expired renderer retained reservation")
	}
	if err := f.app.SetDecisionRepeatHotkey(session, 2, "a"); err == nil {
		t.Fatal("expired renderer renewed without opening a session")
	}
	if f.app.OpenDecisionRepeatHotkeySession() == "" {
		t.Fatal("could not reconnect after expiry")
	}
}

func TestDecisionRepeatHotkeySecurityFailureAndShutdown(t *testing.T) {
	f := newDecisionRepeatFixture(t)
	session := f.app.OpenDecisionRepeatHotkeySession()
	if err := f.app.SetDecisionRepeatHotkey(session, 1, "a"); err != nil {
		t.Fatal(err)
	}
	f.allowed = false
	f.callbacks[0]()
	if len(f.events) != 0 {
		t.Fatal("unsafe host announced decision")
	}
	f.allowed = true
	f.app.decisionRepeatHotkeys.shutdown()
	f.callbacks[0]()
	if len(f.events) != 0 || f.releases != 1 {
		t.Fatal("shutdown left callback enabled")
	}
	if f.app.OpenDecisionRepeatHotkeySession() != "" {
		t.Fatal("shutdown reopened")
	}
	if err := f.app.SetDecisionRepeatHotkey(session, 2, "a"); err == nil {
		t.Fatal("shutdown accepted update")
	}
}

func TestDecisionRepeatHotkeyNativeFailureCannotLeaveActiveDelivery(t *testing.T) {
	f := newDecisionRepeatFixture(t)
	session := f.app.OpenDecisionRepeatHotkeySession()
	f.registerErr = errors.New("native busy")
	if err := f.app.SetDecisionRepeatHotkey(session, 1, "a"); !errors.Is(err, f.registerErr) {
		t.Fatalf("register err=%v", err)
	}
	if f.app.decisionRepeatHotkeys.active.Load() != nil {
		t.Fatal("failed native registration remained active")
	}
	f.registerErr = nil
	if err := f.app.SetDecisionRepeatHotkey(session, 1, "a"); err != nil {
		t.Fatal(err)
	}
	f.releaseErr = errors.New("native unregister failed")
	if err := f.app.SetDecisionRepeatHotkey(session, 2, ""); !errors.Is(err, f.releaseErr) {
		t.Fatalf("release err=%v", err)
	}
	f.callbacks[0]()
	if len(f.events) != 0 || f.app.OpenDecisionRepeatHotkeySession() != "" {
		t.Fatal("native teardown failure reopened delivery")
	}
}

func TestDecisionRepeatHotkeyUnsupportedAppKeepsNoReservation(t *testing.T) {
	for _, a := range []*App{nil, {}} {
		if a.OpenDecisionRepeatHotkeySession() != "" {
			t.Fatal("unsupported opened native session")
		}
		if a.SetDecisionRepeatHotkey("x", 1, "a") == nil {
			t.Fatal("unsupported reserved native shortcut")
		}
		if err := a.CloseDecisionRepeatHotkeySession("x"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDecisionRepeatHotkeyInvalidatesBeforeNativeTeardownFinishes(t *testing.T) {
	entered, finish := make(chan struct{}), make(chan struct{})
	var callback hotkey.HotkeyCallback
	var emitted atomic.Int32
	s := newDecisionRepeatHotkeys(func(cb hotkey.HotkeyCallback) (func() error, error) {
		callback = cb
		return func() error { close(entered); <-finish; return nil }, nil
	}, func(decisionRepeatEvent) { emitted.Add(1) }, func() bool { return true })
	defer s.shutdown()
	a := &App{decisionRepeatHotkeys: s}
	session := a.OpenDecisionRepeatHotkeySession()
	if err := a.SetDecisionRepeatHotkey(session, 1, "a"); err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	go func() { closed <- a.CloseDecisionRepeatHotkeySession(session) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		close(finish)
		t.Fatal("teardown not entered")
	}
	callback()
	if emitted.Load() != 0 {
		t.Error("callback emitted while native teardown was pending")
	}
	close(finish)
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
}

func TestDecisionRepeatHotkeyRechecksIdentityAfterSecurityGuard(t *testing.T) {
	f := newDecisionRepeatFixture(t)
	session := f.app.OpenDecisionRepeatHotkeySession()
	if err := f.app.SetDecisionRepeatHotkey(session, 1, "a"); err != nil {
		t.Fatal(err)
	}
	f.app.decisionRepeatHotkeys.allowed = func() bool {
		if err := f.app.SetDecisionRepeatHotkey(session, 2, ""); err != nil {
			t.Fatal(err)
		}
		return true
	}
	f.callbacks[0]()
	if len(f.events) != 0 {
		t.Fatal("callback emitted after scope changed during security check")
	}
}
