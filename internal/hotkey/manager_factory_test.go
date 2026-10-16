package hotkey

import (
	"reflect"
	"testing"

	nativehotkey "golang.design/x/hotkey"
)

func TestManagerFactoryCannotMutateModifiersUsedForRestoration(t *testing.T) {
	var inputs [][]nativehotkey.Modifier
	manager := NewManager(func(modifiers []nativehotkey.Modifier, _ nativehotkey.Key) NativeHotkey {
		inputs = append(inputs, append([]nativehotkey.Modifier(nil), modifiers...))
		// Um adapter pode normalizar seu argumento sem alterar o registro lógico.
		modifiers[0] = 0
		return newTemporaryTestNative()
	})
	t.Cleanup(manager.Stop)
	modifiers := []nativehotkey.Modifier{ModCtrl}
	if _, err := manager.Register(modifiers, nativehotkey.KeyR, func() {}); err != nil {
		t.Fatal(err)
	}
	release, err := manager.ReserveTemporary(modifiers, nativehotkey.KeyR, func() {})
	if err != nil {
		t.Fatal(err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	want := [][]nativehotkey.Modifier{{ModCtrl}, {ModCtrl}, {ModCtrl}}
	if !reflect.DeepEqual(inputs, want) {
		t.Fatalf("factory inputs = %v, want %v", inputs, want)
	}
	if !reflect.DeepEqual(modifiers, []nativehotkey.Modifier{ModCtrl}) {
		t.Fatalf("caller modifiers mutated: %v", modifiers)
	}
}
