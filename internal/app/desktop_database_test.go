package app

import (
	"errors"
	"testing"
)

func TestDesktopDatabaseDoesNotReresolveReservedPath(t *testing.T) {
	previous := InitDatabase
	t.Cleanup(func() { InitDatabase = previous })
	InitDatabase = func() error { t.Fatal("desktop re-resolved reserved database"); return nil }
	a := &App{}
	// Um caminho inválido deve falhar, nunca cair na resolução padrão.
	SetDesktopDatabasePath(a, "relative.db")
	if err := a.initDesktopDatabase(); err == nil {
		t.Fatal("invalid pinned path accepted")
	}
}

func TestDesktopDatabaseKeepsDefaultForNonDesktopCallers(t *testing.T) {
	previous := InitDatabase
	t.Cleanup(func() { InitDatabase = previous })
	want := errors.New("default initializer")
	InitDatabase = func() error { return want }
	if err := (&App{}).initDesktopDatabase(); !errors.Is(err, want) {
		t.Fatalf("default initializer not preserved: %v", err)
	}
}
