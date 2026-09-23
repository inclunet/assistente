package commandforeground

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewSnapshotNormalizesAndKeepsIdentityPrivate(t *testing.T) {
	before := time.Now()
	first, err := NewSnapshot(11, 22, 33, `C:\Users\private\Editor.EXE`, " EditorWindow ")
	if err != nil || ValidateSnapshot(first, time.Minute) != nil {
		t.Fatalf("snapshot inválido: %+v, %v", first, err)
	}
	if first.CapturedAt.Before(before) || first.Summary.Executable != "editor.exe" || first.Summary.WindowClass != "EditorWindow" {
		t.Fatalf("observação não normalizada: %+v", first)
	}
	again, err := NewSnapshot(11, 22, 33, `D:\different\editor.exe`, "EditorWindow")
	if err != nil || again.Version != first.Version || !again.Identity.Equal(first.Identity) {
		t.Fatalf("versão incluiu caminho ou instante: %+v, %v", again, err)
	}
	reused, err := NewSnapshot(11, 22, 34, `C:\Editor.EXE`, "EditorWindow")
	if err != nil || reused.Version == first.Version || reused.Identity.Equal(first.Identity) {
		t.Fatalf("reutilização de PID perdeu lifetime: %+v, %v", reused, err)
	}
	encoded, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"private", "Users", "Identity", "creationTime", "process", "window\""} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("snapshot vazou %q: %s", forbidden, encoded)
		}
	}
}

func TestNewSnapshotRejectsIncompleteObservation(t *testing.T) {
	for _, tc := range []struct {
		name        string
		window      uintptr
		process     uint32
		created     uint64
		path, class string
	}{
		{"window", 0, 2, 3, "editor.exe", "Editor"},
		{"process", 1, 0, 3, "editor.exe", "Editor"},
		{"lifetime", 1, 2, 0, "editor.exe", "Editor"},
		{"path", 1, 2, 3, " ", "Editor"},
		{"class", 1, 2, 3, "editor.exe", " "},
		{"class-control", 1, 2, 3, "editor.exe", "Editor\nWindow"},
		{"class-path", 1, 2, 3, "editor.exe", `C:\private\window`},
		{"class-drive-relative", 1, 2, 3, "editor.exe", "C:private.txt"},
		{"class-format", 1, 2, 3, "editor.exe", "Editor\u202eWindow"},
		{"executable-format", 1, 2, 3, "editor\u202e.exe", "Editor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, err := NewSnapshot(tc.window, tc.process, tc.created, tc.path, tc.class)
			if !errors.Is(err, ErrUnknown) || snapshot != (Snapshot{}) {
				t.Fatalf("observação incompleta aceita: %+v, %v", snapshot, err)
			}
		})
	}
}

func TestValidateSnapshotRejectsModifiedObservation(t *testing.T) {
	base, err := NewSnapshot(1, 2, 3, "editor.exe", "Editor")
	if err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Snapshot){
		"version":    func(s *Snapshot) { s.Version = "invented" },
		"executable": func(s *Snapshot) { s.Summary.Executable = "other.exe" },
		"path":       func(s *Snapshot) { s.Summary.Executable = `C:\private\editor.exe` },
		"class":      func(s *Snapshot) { s.Summary.WindowClass = "OtherClass" },
		"provider":   func(s *Snapshot) { s.Summary.ProviderVersion = "unknown" },
		"lifetime":   func(s *Snapshot) { s.Identity.creationTime = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			snapshot := base
			change(&snapshot)
			if !errors.Is(ValidateSnapshot(snapshot, time.Minute), ErrInvalidSnapshot) {
				t.Fatal("observação alterada aceita")
			}
			_, err := CaptureBeforeShow(context.Background(), &fakeReader{snapshot: snapshot}, func() error {
				t.Fatal("show chamado com observação alterada")
				return nil
			})
			if !errors.Is(err, ErrInvalidSnapshot) {
				t.Fatalf("captura alterada aceita antes de show: %v", err)
			}
		})
	}
}

func TestNewSnapshotAcceptsClassColonWithoutDrivePath(t *testing.T) {
	if _, err := NewSnapshot(1, 2, 3, "editor.exe", "ATL:00012345"); err != nil {
		t.Fatalf("classe com dois-pontos sem caminho recusada: %v", err)
	}
}
