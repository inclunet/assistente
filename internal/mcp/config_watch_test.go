package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

type recordingConfigWatcher struct {
	dirs []string
}

func (w *recordingConfigWatcher) Add(dir string) error {
	w.dirs = append(w.dirs, filepath.Clean(dir))
	return nil
}

func TestAddConfigWatchDirsWatchesLateCreatedConfigDir(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, ".assistente")
	target := filepath.Join(parent, "mcp")
	if err := os.MkdirAll(parent, 0755); err != nil {
		t.Fatalf("failed to create parent dir: %v", err)
	}

	watcher := &recordingConfigWatcher{}
	watched := make(map[string]struct{})
	m := newTestManager()

	if added := m.addConfigWatchDirs(watcher, []string{target}, watched); added != 1 {
		t.Fatalf("added initial watchers: got %d, want 1", added)
	}
	if got := watcher.dirs[0]; got != filepath.Clean(parent) {
		t.Fatalf("initial watcher: got %q, want %q", got, filepath.Clean(parent))
	}

	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatalf("failed to create target dir: %v", err)
	}
	if added := m.addConfigWatchDirs(watcher, []string{target}, watched); added != 1 {
		t.Fatalf("added target watcher: got %d, want 1", added)
	}
	if got := watcher.dirs[1]; got != filepath.Clean(target) {
		t.Fatalf("target watcher: got %q, want %q", got, filepath.Clean(target))
	}
}

func TestShouldHandleConfigWatchEventIncludesLateCreatedDirs(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, ".assistente", "mcp")
	dirs := []string{target}

	cases := []struct {
		name string
		path string
		want bool
	}{
		{"json config file", filepath.Join(target, "github.json"), true},
		{"mcp dir creation", target, true},
		{"parent dir creation", filepath.Dir(target), true},
		{"unrelated sibling", filepath.Join(root, ".assistente", "profiles"), false},
		{"unrelated file", filepath.Join(root, "notes.txt"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldHandleConfigWatchEvent(tc.path, dirs); got != tc.want {
				t.Fatalf("shouldHandleConfigWatchEvent(%q): got %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}
