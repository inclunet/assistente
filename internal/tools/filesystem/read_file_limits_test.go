package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"assistente/internal/tools"
)

func TestReadFileDefaultCapsLinesAndProvidesExactResume(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "8203.txt")
	var content strings.Builder
	for i := 1; i <= 8203; i++ {
		fmt.Fprintf(&content, "linha-%04d\n", i)
	}
	if err := os.WriteFile(path, []byte(content.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := NewReadFile(dir).Execute(context.Background(), mustJSON(t, map[string]any{"path": "8203.txt"}))
	if err != nil || res.IsError {
		t.Fatalf("read_file: err=%v result=%+v", err, res)
	}
	window := res.Annotations.OutputWindow
	if window.Returned != 2000 || !window.HasMore || window.NextOffset != 2001 || window.Total != 8204 {
		t.Fatalf("janela inesperada: %+v", window)
	}
	if len(res.Content) > readModelMaxBytes {
		t.Fatalf("resultado tem %d bytes", len(res.Content))
	}
	if got := len(tools.ContentForModel(res)); got > readModelMaxBytes {
		t.Fatalf("resultado model-facing tem %d bytes", got)
	}
	next, _ := NewReadFile(dir).Execute(context.Background(), mustJSON(t, map[string]any{
		"path": "8203.txt", "offset": window.NextOffset, "limit": 1,
	}))
	if !strings.Contains(next.Content, "linha-2001") {
		t.Fatalf("retomada incorreta: %q", next.Content)
	}
}

func TestReadFileByteCapPrecedesLineCap(t *testing.T) {
	dir := t.TempDir()
	var content strings.Builder
	for i := 0; i < 1000; i++ {
		content.WriteString(strings.Repeat("x", 200))
		content.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(dir, "wide.txt"), []byte(content.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	res, _ := NewReadFile(dir).Execute(context.Background(), mustJSON(t, map[string]any{"path": "wide.txt"}))
	if len(res.Content) > readModelMaxBytes {
		t.Fatalf("limite de bytes excedido: %d", len(res.Content))
	}
	if res.Annotations == nil || res.Annotations.OutputWindow == nil ||
		!res.Annotations.OutputWindow.HasMore || res.Annotations.OutputWindow.Returned >= readModelMaxLines {
		t.Fatalf("limite por bytes não observado: %+v", res.Annotations)
	}
	window := res.Annotations.OutputWindow
	next, _ := NewReadFile(dir).Execute(context.Background(), mustJSON(t, map[string]any{
		"path": "wide.txt", "offset": window.NextOffset, "limit": 1,
	}))
	want := fmt.Sprintf("%6d|", window.NextOffset)
	if !strings.Contains(next.Content, want) {
		t.Fatalf("retomada após limite de bytes incorreta: want %q em %q", want, next.Content)
	}
}

func TestReadFileRawSmallIsExactAndLargeFailsWithoutPartial(t *testing.T) {
	dir := t.TempDir()
	exact := "um\r\ndois\r\ntrês"
	if err := os.WriteFile(filepath.Join(dir, "small.txt"), []byte(exact), 0o600); err != nil {
		t.Fatal(err)
	}
	small, _ := NewReadFile(dir).Execute(context.Background(), mustJSON(t, map[string]any{"path": "small.txt", "raw": true}))
	if small.Content != exact || small.Annotations != nil || !small.RawExact {
		t.Fatalf("raw não exato: %#v", small)
	}

	large := strings.Repeat("linha suficientemente larga\n", 3000)
	if err := os.WriteFile(filepath.Join(dir, "large.txt"), []byte(large), 0o600); err != nil {
		t.Fatal(err)
	}
	got, _ := NewReadFile(dir).Execute(context.Background(), mustJSON(t, map[string]any{"path": "large.txt", "raw": true}))
	if !got.IsError || got.Failure == nil || got.Failure.Code != "raw_result_too_large" {
		t.Fatalf("raw grande deveria falhar: %+v", got)
	}
	if strings.Contains(got.Content, strings.Repeat("linha suficientemente larga\n", 10)) {
		t.Fatal("erro raw contém conteúdo parcial")
	}
}

func TestReadFileEnvelopeDoesNotPolluteContent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\nb\nc"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, _ := NewReadFile(dir).Execute(context.Background(), mustJSON(t, map[string]any{"path": "a.txt", "limit": 1}))
	if strings.Contains(strings.ToUpper(res.Content), "TRUNCAD") {
		t.Fatalf("aviso contaminou corpo: %q", res.Content)
	}
	model := tools.ContentForModel(res)
	if !strings.Contains(model, `"has_more":true`) || !strings.Contains(model, `"next_offset":2`) {
		t.Fatalf("envelope ausente: %q", model)
	}
}

func TestReadFileStreamingHonorsCancellation(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cancel.txt"), []byte(strings.Repeat("linha\n", 10_000)), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, _ := NewReadFile(dir).Execute(ctx, mustJSON(t, map[string]any{"path": "cancel.txt"}))
	if !res.IsError || !strings.Contains(strings.ToLower(res.Content), "cancel") {
		t.Fatalf("cancelamento ignorado: %+v", res)
	}
}
