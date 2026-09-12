package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestListDirectoryWindowCountsIgnoredEntriesInBody(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "node_modules"), 0o700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxEntries; i++ {
		name := filepath.Join(dir, fmt.Sprintf("z%04d.txt", i))
		if err := os.WriteFile(name, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := NewListDirectory(dir).Execute(context.Background(), mustJSON(t, map[string]any{
		"path": ".", "recursive": true,
	}))
	if err != nil || result.IsError {
		t.Fatalf("list_directory: err=%v result=%+v", err, result)
	}
	window := result.Annotations.OutputWindow
	if !window.HasMore || window.Returned != maxEntries {
		t.Fatalf("janela não conta linhas emitidas: %+v", window)
	}
}
