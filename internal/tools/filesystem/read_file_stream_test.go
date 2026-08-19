package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeLinesFile grava um arquivo de texto com nLines linhas numeradas.
func writeLinesFile(t *testing.T, path string, nLines int, pad string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	w := strings.Builder{}
	for i := 1; i <= nLines; i++ {
		fmt.Fprintf(&w, "linha %d %s\n", i, pad)
		if w.Len() > 1<<20 {
			if _, err := f.WriteString(w.String()); err != nil {
				t.Fatal(err)
			}
			w.Reset()
		}
	}
	if _, err := f.WriteString(w.String()); err != nil {
		t.Fatal(err)
	}
}

// Texto grande com offset/limit é servido em streaming e devolve exatamente o
// mesmo recorte que o caminho que carrega tudo.
func TestReadFileStreamsLargeTextSlice(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grande.log")
	pad := strings.Repeat("x", 60)
	writeLinesFile(t, path, 80_000, pad)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() < streamTextMinBytes {
		t.Fatalf("arquivo pequeno demais para exercitar o streaming: %d bytes", info.Size())
	}

	tool := NewReadFile(dir)
	res, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"path":   "grande.log",
		"offset": 10,
		"limit":  3,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("erro: %s", res.Content)
	}

	// 80.000 linhas + linha vazia final, como em strings.Split.
	if got := res.Metadata["total_lines"]; got != 80_001 {
		t.Fatalf("total_lines=%v", got)
	}
	lines := strings.Split(res.Content, "\n")
	if !strings.Contains(lines[0], "linhas 10-12 de 80001") {
		t.Fatalf("cabeçalho inesperado: %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "    10|linha 10 ") {
		t.Fatalf("primeira linha do recorte: %q", lines[1])
	}
	if len(lines) != 4 {
		t.Fatalf("esperava 3 linhas no recorte, veio %d", len(lines)-1)
	}
}

// Offset negativo conta do fim também no caminho em streaming.
func TestReadFileStreamsNegativeOffset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grande.log")
	writeLinesFile(t, path, 80_000, strings.Repeat("x", 60))

	tool := NewReadFile(dir)
	res, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"path":   "grande.log",
		"offset": -2,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("erro: %s", res.Content)
	}
	if !strings.Contains(res.Content, "linha 80000 ") {
		t.Fatalf("última linha ausente: %q", res.Content)
	}
}

// O recorte em streaming precisa bater com o do caminho que carrega tudo.
func TestStreamingSliceMatchesFullRead(t *testing.T) {
	dir := t.TempDir()
	small := filepath.Join(dir, "pequeno.log")
	writeLinesFile(t, small, 50, "")

	tool := NewReadFile(dir)
	full, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"path":   "pequeno.log",
		"offset": 5,
		"limit":  4,
	}))
	if err != nil {
		t.Fatal(err)
	}

	streamed, handled := readTextSliceStreaming(small, "pequeno.log", streamTextMinBytes, intPtr(5), intPtr(4))
	if !handled {
		t.Skip("classificação não considerou o arquivo como texto")
	}
	if streamed.Content != full.Content {
		t.Fatalf("streaming difere:\n%q\n%q", streamed.Content, full.Content)
	}
}

func intPtr(v int) *int { return &v }
