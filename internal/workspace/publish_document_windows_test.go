package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishWorkspaceDocumentInLongDirectory(t *testing.T) {
	m := newTestManager(t, t.TempDir())
	directory := t.TempDir()
	for len(directory) < 300 {
		directory = filepath.Join(directory, strings.Repeat("folder", 8))
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := m.createWorkspaceDocument(m.newWorkspace("Long path"), directory); err != nil {
		t.Fatal(err)
	}
	if err := m.createWorkspaceDocument(m.newWorkspace("Must not replace"), directory); !os.IsExist(err) {
		t.Fatalf("overwrite protection: %v", err)
	}
	ws, err := m.loadWorkspaceFile(filepath.Join(directory, assistenteDir, workspaceFile))
	if err != nil || ws.Name != "Long path" {
		t.Fatalf("workspace=%+v err=%v", ws, err)
	}
}
