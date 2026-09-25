package workspace

import (
	"context"
	"errors"
	"testing"
)

func terminalBindingManager(t *testing.T) (*Manager, string, CommandSnapshot, *terminalCommandSessionValidator) {
	t.Helper()
	m := commandSnapshotManager(t)
	tabID := "terminal-binding"
	if err := m.AddTab(Tab{ID: tabID, Type: TabTypeTerminal, Title: "Terminal original", State: map[string]any{"cwd": "fixture"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetActiveTab(tabID); err != nil {
		t.Fatal(err)
	}
	expected, err := m.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	sessions := &terminalCommandSessionValidator{live: map[string]bool{"session-new": true}}
	return m, tabID, expected, sessions
}

func TestBindAndUnbindTerminalSessionPreserveActiveTabAndOtherState(t *testing.T) {
	m, tabID, expected, sessions := terminalBindingManager(t)
	before := m.Active()
	if _, err := m.BindTerminalSessionForCommand(context.Background(), expected, "session-new", sessions); err != nil {
		t.Fatal(err)
	}
	bound := m.Active().FindTab(tabID)
	if bound == nil || bound.State["sessionId"] != "session-new" || bound.State["cwd"] != "fixture" || m.Active().Tabs.Active != tabID || bound.Title != "Terminal original" {
		t.Fatalf("bind alterou dados indevidos: %#v", bound)
	}

	boundSnapshot, err := m.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.UnbindTerminalSessionForCommand(context.Background(), boundSnapshot, "session-new"); err != nil {
		t.Fatal(err)
	}
	unbound := m.Active().FindTab(tabID)
	if unbound == nil || unbound.State["sessionId"] != nil || unbound.State["cwd"] != "fixture" || m.Active().Tabs.Active != tabID || unbound.Title != "Terminal original" {
		t.Fatalf("unbind alterou dados indevidos: %#v", unbound)
	}
	if before.Tabs.Active != m.Active().Tabs.Active || before.FindTab(tabID).Title != m.Active().FindTab(tabID).Title {
		t.Fatal("bind/unbind alterou a aba ativa ou apresentação")
	}
}

func TestBindTerminalSessionReplacesBindingWithoutClosingPreviousSession(t *testing.T) {
	m, tabID, _, sessions := terminalBindingManager(t)
	sessions.live["session-old"] = true
	if err := m.UpdateTab(tabID, map[string]any{"state": map[string]any{
		"cwd": "fixture", "sessionId": "session-old",
	}}); err != nil {
		t.Fatal(err)
	}
	expected, err := m.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.BindTerminalSessionForCommand(context.Background(), expected, "session-new", sessions); err != nil {
		t.Fatal(err)
	}
	if got := m.Active().FindTab(tabID).State["sessionId"]; got != "session-new" {
		t.Fatalf("binding novo não substituiu somente sessionId: %v", got)
	}
	if !sessions.Has("session-old") {
		t.Fatal("bind novo encerrou silenciosamente a sessão anterior")
	}
}

func TestTerminalBindingRejectsStaleOrDifferentSessionWithoutMutation(t *testing.T) {
	m, tabID, expected, sessions := terminalBindingManager(t)
	stale := expected
	if err := m.UpdateTab(tabID, map[string]any{"state": map[string]any{"other": "changed"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.BindTerminalSessionForCommand(context.Background(), stale, "session-new", sessions); !errors.Is(err, ErrCommandTerminalBindingStale) {
		t.Fatalf("bind stale aceito: %v", err)
	}
	if got := m.Active().FindTab(tabID).State["sessionId"]; got != nil {
		t.Fatalf("stale criou vínculo: %v", got)
	}

	expected, err := m.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.BindTerminalSessionForCommand(context.Background(), expected, "session-new", sessions); err != nil {
		t.Fatal(err)
	}
	boundSnapshot, err := m.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.UnbindTerminalSessionForCommand(context.Background(), boundSnapshot, "other-session"); !errors.Is(err, ErrCommandTerminalBindingStale) {
		t.Fatalf("unbind de outra sessão aceito: %v", err)
	}
	if got := m.Active().FindTab(tabID).State["sessionId"]; got != "session-new" {
		t.Fatalf("unbind incorreto alterou vínculo: %v", got)
	}
}
