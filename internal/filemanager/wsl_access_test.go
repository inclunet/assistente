package filemanager

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWSLDirectAccess(t *testing.T) {
	// Este teste só funciona se WSL estiver rodando
	wslPath := `\\wsl.localhost\Ubuntu-24.04\home`
	
	// Testa se o caminho existe
	info, err := os.Stat(wslPath)
	if err != nil {
		t.Skipf("WSL path not accessible (WSL may not be running): %v", err)
	}
	
	t.Logf("WSL path accessible: %s (IsDir: %v)", wslPath, info.IsDir())
	
	// Testa listagem de diretório
	entries, err := os.ReadDir(wslPath)
	if err != nil {
		t.Fatalf("Failed to read WSL directory: %v", err)
	}
	
	t.Logf("Found %d entries in WSL path", len(entries))
	for _, e := range entries {
		t.Logf("  - %s (IsDir: %v)", e.Name(), e.IsDir())
	}
}

func TestWSLWalkDir(t *testing.T) {
	// Este teste só funciona se WSL estiver rodando
	wslPath := `\\wsl.localhost\Ubuntu-24.04\home`
	
	_, err := os.Stat(wslPath)
	if err != nil {
		t.Skipf("WSL path not accessible: %v", err)
	}
	
	count := 0
	err = filepath.WalkDir(wslPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			t.Logf("Error accessing %s: %v", path, err)
			return nil // Continue walking
		}
		
		count++
		if count <= 10 {
			t.Logf("Found: %s", path)
		}
		
		if count > 50 {
			return filepath.SkipAll
		}
		return nil
	})
	
	if err != nil {
		t.Errorf("WalkDir failed: %v", err)
	}
	
	t.Logf("Total entries found via WalkDir: %d", count)
}

func TestWSLWithStorageProvider(t *testing.T) {
	wslPath := `\\wsl.localhost\Ubuntu-24.04\home`
	
	_, err := os.Stat(wslPath)
	if err != nil {
		t.Skipf("WSL path not accessible: %v", err)
	}
	
	p := NewLocalStorageProvider()
	ctx := context.Background()
	
	// Testa ListDirectory
	entries, err := p.ListDirectory(ctx, wslPath, ListOptions{ShowHidden: true})
	if err != nil {
		t.Fatalf("ListDirectory failed: %v", err)
	}
	
	t.Logf("ListDirectory found %d entries", len(entries))
	for _, e := range entries {
		t.Logf("  - %s (Provider: %s)", e.Name, e.Provider)
	}
	
	// Testa SearchByName
	results, err := p.SearchByName(ctx, wslPath, "*", SearchOptions{MaxResults: 10})
	if err != nil {
		t.Fatalf("SearchByName failed: %v", err)
	}
	
	t.Logf("SearchByName found %d results", len(results))
	for _, r := range results {
		t.Logf("  - %s", r.Path)
	}
}



