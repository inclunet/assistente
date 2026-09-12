package terminal

import (
	"strings"
	"testing"
)

func TestAddHistoryEntryDoesNotMutateCallerOutput(t *testing.T) {
	session := &Session{}
	original := strings.Repeat("x", maxOutputSize+100)
	entry := &HistoryEntry{Output: original}

	session.addHistoryEntry(entry)

	if entry.Output != original {
		t.Fatal("histórico mutilou o output devolvido ao chamador")
	}
	if len(session.history) != 1 || len(session.history[0].Output) <= maxOutputSize {
		t.Fatal("cópia limitada não foi registrada no histórico")
	}
}
