package app

import (
	"strings"
	"testing"
)

func TestCommandKeyboardBareFunctionKeyAdmission(t *testing.T) {
	for _, tc := range []struct {
		code      string
		modifiers []string
		want      bool
	}{
		{"F1", []string{}, true},
		{"F5", []string{}, true},
		{"KeyA", []string{}, false},
		{"Digit1", []string{}, false},
		{"F2", []string{}, false},
		{"Tab", []string{}, false},
		{"Enter", []string{}, false},
		{"F1", []string{"Shift"}, false},
		{"F1", []string{"Alt"}, true},
		{"F1", []string{"Control", "Shift"}, true},
		{"F5", []string{"Shift"}, false},
	} {
		t.Run(tc.code+"/"+strings.Join(tc.modifiers, "+"), func(t *testing.T) {
			shortcut := LocalCommandShortcut{Version: 1, Code: tc.code, Modifiers: tc.modifiers}
			if got := localCommandShortcutAllowed(shortcut); got != tc.want {
				t.Fatalf("admissão %+v = %v; esperava %v", shortcut, got, tc.want)
			}
		})
	}
}

func TestCommandKeyboardHelpDefaultPublishedAsLocalUI(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, binding := range view.Bindings {
		if binding.Shortcut.Code == "F1" && len(binding.Shortcut.Modifiers) == 0 {
			if binding.CommandID != "navigation.help.open" || binding.Handler != "local_ui" {
				t.Fatalf("F1 não publicado como ajuda local: %+v", binding)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("F1 ausente do mapa publicado")
	}
	for _, suppressed := range []bool{true, false} {
		settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
			return a.SetDefaultCommandSuppressed("builtin.keyboard.f1.navigation.help.open", suppressed)
		})
		updated, err := a.GetLocalCommandKeyboardMap()
		if err != nil {
			t.Fatal(err)
		}
		present := false
		for _, binding := range updated.Bindings {
			if binding.Shortcut.Code == "F1" && len(binding.Shortcut.Modifiers) == 0 {
				present = true
			}
		}
		if present == suppressed {
			t.Fatalf("supressão F1=%v; presente no mapa=%v", suppressed, present)
		}
	}
}
