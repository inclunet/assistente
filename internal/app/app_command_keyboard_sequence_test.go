package app

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
)

func TestCommandKeyboardSequenceShortcutV2RoundtripKeepsCanonicalSteps(t *testing.T) {
	shortcut := LocalCommandShortcut{Version: 2, Steps: []LocalCommandShortcutStep{
		{Code: "KeyN", Modifiers: []string{"Control"}},
		{Code: "KeyC", Modifiers: []string{}},
	}}
	raw, err := json.Marshal(shortcut)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(raw), `{"version":2,"steps":[{"code":"KeyN","modifiers":["Control"]},{"code":"KeyC","modifiers":[]}]}`; got != want {
		t.Fatalf("JSON v2 = %s, want %s", got, want)
	}
	var decoded LocalCommandShortcut
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, shortcut) {
		t.Fatalf("roundtrip = %+v, want %+v", decoded, shortcut)
	}
	if !localCommandShortcutAllowed(decoded) {
		t.Fatal("sequência canônica rejeitada")
	}
}

func TestCommandKeyboardShortcutAllowsShiftWithPrimaryForV1AndV2(t *testing.T) {
	for name, shortcut := range map[string]LocalCommandShortcut{
		"v1": {Version: 1, Code: "KeyL", Modifiers: []string{"Control", "Shift"}},
		"v2": {Version: 2, Steps: []LocalCommandShortcutStep{
			{Code: "KeyN", Modifiers: []string{"Control", "Shift"}},
			{Code: "KeyL", Modifiers: []string{}},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			if !localCommandShortcutAllowed(shortcut) {
				t.Fatalf("atalho Control+Shift rejeitado: %+v", shortcut)
			}
		})
	}
	if localCommandShortcutAllowed(LocalCommandShortcut{Version: 1, Code: "KeyL", Modifiers: []string{"Shift"}}) ||
		localCommandShortcutAllowed(LocalCommandShortcut{Version: 2, Steps: []LocalCommandShortcutStep{
			{Code: "KeyN", Modifiers: []string{"Shift"}}, {Code: "KeyL", Modifiers: []string{}},
		}}) {
		t.Fatal("Shift isolado foi aceito")
	}
}

func TestCommandKeyboardSequenceShortcutV2RejectsInvalidFinalStep(t *testing.T) {
	for name, shortcut := range map[string]LocalCommandShortcut{
		"escape":                 {Version: 2, Steps: []LocalCommandShortcutStep{{Code: "KeyN", Modifiers: []string{"Control"}}, {Code: "Escape", Modifiers: []string{}}}},
		"modificador final":      {Version: 2, Steps: []LocalCommandShortcutStep{{Code: "KeyN", Modifiers: []string{"Control"}}, {Code: "KeyC", Modifiers: []string{"Shift"}}}},
		"prefix sem modificador": {Version: 2, Steps: []LocalCommandShortcutStep{{Code: "KeyN", Modifiers: []string{}}, {Code: "KeyC", Modifiers: []string{}}}},
		"passo extra":            {Version: 2, Steps: []LocalCommandShortcutStep{{Code: "KeyN", Modifiers: []string{"Control"}}, {Code: "KeyC", Modifiers: []string{}}, {Code: "KeyD", Modifiers: []string{}}}},
	} {
		t.Run(name, func(t *testing.T) {
			if localCommandShortcutAllowed(shortcut) {
				t.Fatal("sequência inválida aceita")
			}
		})
	}
}

func TestCommandKeyboardShortcutMarshalRejectsMixedVersions(t *testing.T) {
	cases := []LocalCommandShortcut{
		{Version: 1, Code: "KeyN", Modifiers: []string{"Control"}, Steps: []LocalCommandShortcutStep{{Code: "KeyC", Modifiers: []string{}}}},
		{Version: 2, Code: "KeyN", Steps: []LocalCommandShortcutStep{{Code: "KeyN", Modifiers: []string{"Control"}}, {Code: "KeyC", Modifiers: []string{}}}},
		{Version: 2, Modifiers: []string{}, Steps: []LocalCommandShortcutStep{{Code: "KeyN", Modifiers: []string{"Control"}}, {Code: "KeyC", Modifiers: []string{}}}},
		{Version: 3, Code: "KeyN", Modifiers: []string{"Control"}},
	}
	for _, shortcut := range cases {
		if _, err := json.Marshal(shortcut); err == nil {
			t.Fatalf("payload misto/incompatível foi serializado: %+v", shortcut)
		}
	}
}

func TestCommandKeyboardSequenceMapDeepCloneDoesNotShareSteps(t *testing.T) {
	in := LocalCommandKeyboardMap{Bindings: []LocalCommandKeyboardBinding{{Shortcut: LocalCommandShortcut{Version: 2, Steps: []LocalCommandShortcutStep{{Code: "KeyN", Modifiers: []string{"Control"}}, {Code: "KeyC", Modifiers: []string{}}}}}}}
	out := cloneLocalCommandKeyboardMap(in)
	out.Bindings[0].Shortcut.Steps[0].Modifiers[0] = "Alt"
	if in.Bindings[0].Shortcut.Steps[0].Modifiers[0] != "Control" {
		t.Fatal("clone compartilhou modificadores do prefixo")
	}
}

func TestCommandKeyboardSequenceCandidateKeepsKeyboardLocalProvenance(t *testing.T) {
	shortcut := LocalCommandShortcut{Version: 2, Steps: []LocalCommandShortcutStep{
		{Code: "KeyN", Modifiers: []string{"Control"}},
		{Code: "KeyC", Modifiers: []string{}},
	}}
	raw, err := json.Marshal(shortcut)
	if err != nil {
		t.Fatal(err)
	}
	candidate := localKeyboardCandidate("inv-sequence", raw)
	if candidate.TriggerType != string(commandcatalog.KeyboardLocal) || string(candidate.Arguments) != "{}" {
		t.Fatalf("candidato de sequência perdeu origem/args: %+v", candidate)
	}
	identity, err := (commandconfig.KeyboardLocalTriggerPort{}).Normalize(context.Background(), candidate.TriggerSpec)
	if err != nil || identity != "keyboard.local:Control+KeyN KeyC" {
		t.Fatalf("identidade/proveniência da sequência: %q %v", identity, err)
	}
}

func TestCommandSettingsTriggerSpecRoundtripSequenceIdentity(t *testing.T) {
	raw, err := commandSettingsTriggerSpec(nil, "keyboard.local:Control+KeyN KeyT")
	if err != nil {
		t.Fatal(err)
	}
	var got LocalCommandShortcut
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatal(err)
	}
	want := LocalCommandShortcut{Version: 2, Steps: []LocalCommandShortcutStep{{Code: "KeyN", Modifiers: []string{"Control"}}, {Code: "KeyT", Modifiers: []string{}}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("spec = %+v, want %+v", got, want)
	}
}

func TestCommandKeyboardSequenceWailsJSONRejectsUnknownNullAndMixedFields(t *testing.T) {
	for name, raw := range map[string]string{
		"root desconhecido":  `{"version":2,"steps":[{"code":"KeyN","modifiers":["Control"]},{"code":"KeyC","modifiers":[]}],"extra":true}`,
		"passo desconhecido": `{"version":2,"steps":[{"code":"KeyN","modifiers":["Control"],"extra":true},{"code":"KeyC","modifiers":[]}]}`,
		"code nulo":          `{"version":2,"code":null,"steps":[{"code":"KeyN","modifiers":["Control"]},{"code":"KeyC","modifiers":[]}]}`,
		"modifiers nulo":     `{"version":2,"steps":[{"code":"KeyN","modifiers":null},{"code":"KeyC","modifiers":[]}]}`,
		"v1 com steps":       `{"version":1,"code":"KeyN","modifiers":["Control"],"steps":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			var shortcut LocalCommandShortcut
			if err := json.Unmarshal([]byte(raw), &shortcut); err == nil {
				t.Fatalf("payload inválido aceito: %s", raw)
			}
		})
	}
}

func TestCommandKeyboardSequenceDefaultSuppressAndRestoreThroughRealAPI(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	p := a.commandProduct.Load()
	projection, err := commandProductProjection(p.registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	var defaultID string
	for _, item := range projection.BuiltinLayers[1].Defaults {
		if item.Candidate.ID == "builtin.keyboard.ctrl-n.workspace.tab.tasklist.create" {
			defaultID = item.Candidate.ID
		}
	}
	if defaultID == "" {
		t.Fatal("default Ctrl+N T ausente")
	}
	initial, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	if len(initial.Bindings) != 62 {
		t.Fatalf("mapa inicial = %d bindings", len(initial.Bindings))
	}
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SetDefaultCommandSuppressed(defaultID, true)
	})
	suppressed, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	if len(suppressed.Bindings) != 61 {
		t.Fatalf("mapa após supressão = %d bindings", len(suppressed.Bindings))
	}
	for _, candidate := range suppressed.Bindings {
		if candidate.CommandID == commandWorkspaceTabTasklistCreateID && candidate.Shortcut.Version == 2 {
			t.Fatal("Ctrl+N T continuou publicado após supressão")
		}
	}
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SetDefaultCommandSuppressed(defaultID, false)
	})
	restored, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.Bindings) != 62 {
		t.Fatalf("mapa restaurado = %d bindings", len(restored.Bindings))
	}
}
