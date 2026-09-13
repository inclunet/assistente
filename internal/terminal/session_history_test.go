package terminal

import (
	"strings"
	"testing"
	"unicode/utf8"
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

func TestAddHistoryEntryCutsAtUTF8Boundary(t *testing.T) {
	session := &Session{}
	original := strings.Repeat("x", maxOutputSize-1) + "é" + strings.Repeat("z", 100)
	entry := &HistoryEntry{Output: original}

	session.addHistoryEntry(entry)

	if !utf8.ValidString(session.history[0].Output) {
		t.Fatalf("cópia do histórico contém UTF-8 inválido no corte")
	}
	if entry.Output != original {
		t.Fatal("histórico alterou o output bruto do chamador")
	}
}
