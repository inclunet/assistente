package filesystem

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"assistente/internal/docextract"

	"codeberg.org/go-pdf/fpdf"
)

func TestReadFileProjectsDOCX(t *testing.T) {
	dir := t.TempDir()
	docx := buildMinimalDOCX(t, "Texto no documento")
	path := filepath.Join(dir, "doc.docx")
	if err := os.WriteFile(path, docx, 0644); err != nil {
		t.Fatal(err)
	}

	tool := NewReadFile(dir)
	res, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{"path": "doc.docx"}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "projeção Markdown") {
		t.Fatalf("missing projection header: %s", res.Content)
	}
	if !strings.Contains(res.Content, "Texto no documento") {
		t.Fatalf("missing body: %s", res.Content)
	}
	if res.Metadata["projection"] != true {
		t.Fatalf("metadata=%v", res.Metadata)
	}
}

func TestReadFilePlainTextUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	if err := os.WriteFile(path, []byte("linha1\nlinha2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	tool := NewReadFile(dir)
	res, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{"path": "a.md"}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatal(res.Content)
	}
	if strings.Contains(res.Content, "projeção Markdown") {
		t.Fatalf("text should not be projected: %s", res.Content)
	}
	if !strings.Contains(res.Content, "linha1") {
		t.Fatal(res.Content)
	}
}

func TestWriteFileRejectsDisguisedPDF(t *testing.T) {
	dir := t.TempDir()
	pdf := buildTestPDF(t, "x")
	path := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(path, pdf, 0644); err != nil {
		t.Fatal(err)
	}
	tool := NewWriteFile(dir)
	res, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"path":    "notes.txt",
		"content": "novo",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected error")
	}
	if !strings.Contains(res.Content, "escrita não suportada") {
		t.Fatalf("got %s", res.Content)
	}
}

func TestEditFileRejectsPDF(t *testing.T) {
	dir := t.TempDir()
	pdf := buildTestPDF(t, "x")
	path := filepath.Join(dir, "a.pdf")
	if err := os.WriteFile(path, pdf, 0644); err != nil {
		t.Fatal(err)
	}
	tool := NewEditFile(dir, nil)
	res, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"path":       "a.pdf",
		"old_string": "x",
		"new_string": "y",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected error")
	}
}

func TestTextEditRejectsDisguisedPDF(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "ativo.txt")
	if err := os.WriteFile(filePath, buildTestPDF(t, "conteudo"), 0644); err != nil {
		t.Fatal(err)
	}
	quest := &fakeQuestionnaireRequester{}
	tool := NewTextEdit(dir, quest)
	res, err := tool.Execute(editorCtx(filePath), mustJSON(t, map[string]any{
		"original":    "conteudo",
		"replacement": "novo",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected error")
	}
	if !strings.Contains(res.Content, "escrita não suportada") {
		t.Fatalf("got %s", res.Content)
	}
	if quest.called {
		t.Error("confirmação não deveria ser exibida para documento")
	}
}

func TestWriteFileAllowsText(t *testing.T) {
	dir := t.TempDir()
	tool := NewWriteFile(dir)
	res, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"path":    "ok.md",
		"content": "# hi\n",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatal(res.Content)
	}
}

func TestWriteFileRejectsBinaryWithoutProjectionPromise(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "blob.bin")
	if err := os.WriteFile(path, []byte{0x00, 0x01, 0x02, 0xff, 0xfe}, 0644); err != nil {
		t.Fatal(err)
	}
	tool := NewWriteFile(dir)
	res, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"path":    "blob.bin",
		"content": "texto",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected error")
	}
	if strings.Contains(res.Content, "projeção Markdown") {
		t.Fatalf("binário não tem projeção: %s", res.Content)
	}
}

// Documento acima do teto é recusado pelo tamanho no disco, sem carregar o
// arquivo inteiro; texto do mesmo tamanho continua legível.
func TestReadFileRejectsOversizedDocumentWithoutLoading(t *testing.T) {
	dir := t.TempDir()
	docPath := filepath.Join(dir, "grande.pdf")
	f, err := os.Create(docPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("%PDF-1.4\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(docextract.MaxExtractBytes + 1); err != nil {
		_ = f.Close()
		t.Skipf("não foi possível criar arquivo grande: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	tool := NewReadFile(dir)
	res, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{"path": "grande.pdf"}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("documento acima do teto deveria falhar")
	}
	if !strings.Contains(res.Content, "muito grande para extração") {
		t.Fatalf("got %s", res.Content)
	}
}

func TestReadFileUnsupportedBinary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.bin")
	if err := os.WriteFile(path, []byte{0, 1, 2, 3, 255}, 0644); err != nil {
		t.Fatal(err)
	}
	tool := NewReadFile(dir)
	res, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{"path": "x.bin"}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected error")
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func buildTestPDF(t *testing.T, text string) []byte {
	t.Helper()
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "", 12)
	pdf.Cell(40, 10, text)
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func buildMinimalDOCX(t *testing.T, paragraph string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`,
		"word/document.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body><w:p><w:r><w:t>` + paragraph + `</w:t></w:r></w:p></w:body>
</w:document>`,
	}
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
