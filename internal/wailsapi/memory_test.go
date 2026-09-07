package wailsapi

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"assistente/internal/memory"
)

func TestMemoryNotWired(t *testing.T) {
	t.Parallel()
	api := NewMemory()
	if _, err := api.ListMemoryRecords(memory.Filter{}); !errors.Is(err, ErrMemoryNotWired) {
		t.Fatalf("ListMemoryRecords: got %v", err)
	}
	if _, err := api.SearchMemoryRecords("x", memory.Filter{}); !errors.Is(err, ErrMemoryNotWired) {
		t.Fatalf("SearchMemoryRecords: got %v", err)
	}
	if _, err := api.GetMemoryRecord("id"); !errors.Is(err, ErrMemoryNotWired) {
		t.Fatalf("GetMemoryRecord: got %v", err)
	}
	if _, err := api.CreateMemoryRecord(memory.RecordInput{}); !errors.Is(err, ErrMemoryNotWired) {
		t.Fatalf("CreateMemoryRecord: got %v", err)
	}
	if _, err := api.UpdateMemoryRecord("id", memory.RecordInput{}); !errors.Is(err, ErrMemoryNotWired) {
		t.Fatalf("UpdateMemoryRecord: got %v", err)
	}
	if _, err := api.ArchiveMemoryRecord("id"); !errors.Is(err, ErrMemoryNotWired) {
		t.Fatalf("ArchiveMemoryRecord: got %v", err)
	}
	if _, err := api.UnarchiveMemoryRecord("id", "retrievable"); !errors.Is(err, ErrMemoryNotWired) {
		t.Fatalf("UnarchiveMemoryRecord: got %v", err)
	}
	if err := api.DeleteMemoryRecord("id"); !errors.Is(err, ErrMemoryNotWired) {
		t.Fatalf("DeleteMemoryRecord: got %v", err)
	}
	if _, err := api.GetMemoryPolicySummary(); !errors.Is(err, ErrMemoryNotWired) {
		t.Fatalf("GetMemoryPolicySummary: got %v", err)
	}
}

func TestMemoryUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "memory.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("memory.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(session,") {
		t.Fatal("memory.go deve chamar WithUser(session,")
	}
}
