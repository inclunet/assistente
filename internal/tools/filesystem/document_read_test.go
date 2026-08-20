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
	"assistente/internal/tools"

	"codeberg.org/go-pdf/fpdf"
)

func TestReadFileProjectsDOCX(t *testing.T) {
	dir := t.TempDir()
	docx := buildMinimalDOCX(t, "Texto no documento")
	path := filepath.Join(dir, "doc.docx")
	if err := os.WriteFile(path, docx, 0644); err != nil {
		t.Fatal(err)
	}

	// nil explícito também deve selecionar o cache padrão do construtor.
	tool := NewReadFile(dir, nil)
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

func TestReadFileCachesProjectionButKeepsRequestedSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.docx")
	if err := os.WriteFile(path, buildMinimalDOCX(t, "Texto em cache"), 0644); err != nil {
		t.Fatal(err)
	}
	cache := docextract.NewProjectionCache(docextract.DefaultCacheConfig())
	tool := NewReadFile(dir, cache)

	first, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{"path": "doc.docx"}))
	if err != nil || first.IsError {
		t.Fatalf("primeira leitura: err=%v result=%s", err, first.Content)
	}
	if first.Metadata["cache_hit"] != false {
		t.Fatalf("primeira leitura deveria extrair: %v", first.Metadata)
	}

	second, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{"path": "./doc.docx"}))
	if err != nil || second.IsError {
		t.Fatalf("segunda leitura: err=%v result=%s", err, second.Content)
	}
	if second.Metadata["cache_hit"] != true {
		t.Fatalf("segunda leitura deveria vir do cache: %v", second.Metadata)
	}
	if !strings.Contains(second.Content, "Origem: ./doc.docx") {
		t.Fatalf("cache vazou o path da primeira chamada: %s", second.Content)
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

// CSV, HTML e afins são texto: por padrão o modelo recebe o arquivo como ele é,
// e não uma tabela Markdown derivada (D12).
func TestReadFileKeepsTextFormatsVerbatim(t *testing.T) {
	dir := t.TempDir()
	casos := []struct {
		arquivo  string
		conteudo string
		trecho   string
	}{
		{"dados.csv", "nome,idade\nAna,30\n", "nome,idade"},
		{"pagina.html", "<h1>Titulo</h1>\n<p>corpo</p>\n", "<h1>Titulo</h1>"},
		{"nota.rtf", `{\rtf1\ansi Olamundo\par }`, `\rtf1`},
	}
	tool := NewReadFile(dir)
	for _, c := range casos {
		if err := os.WriteFile(filepath.Join(dir, c.arquivo), []byte(c.conteudo), 0644); err != nil {
			t.Fatal(err)
		}
		res, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{"path": c.arquivo}))
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("%s: %s", c.arquivo, res.Content)
		}
		if strings.Contains(res.Content, "projeção Markdown") {
			t.Fatalf("%s não deveria vir projetado: %s", c.arquivo, res.Content)
		}
		if res.Metadata["projection"] == true {
			t.Fatalf("%s: metadata=%v", c.arquivo, res.Metadata)
		}
		if !strings.Contains(res.Content, c.trecho) {
			t.Fatalf("%s: conteúdo original ausente: %s", c.arquivo, res.Content)
		}
	}
}

// A tabela Markdown do CSV continua a um parâmetro de distância.
func TestReadFileProjectsCSVOnDemand(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dados.csv"), []byte("nome,idade\nAna,30\n"), 0644); err != nil {
		t.Fatal(err)
	}
	tool := NewReadFile(dir)
	res, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"path":          "dados.csv",
		"document_mode": "markdown",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatal(res.Content)
	}
	if !strings.Contains(res.Content, "projeção Markdown") {
		t.Fatalf("faltou o cabeçalho de projeção: %s", res.Content)
	}
	if !strings.Contains(res.Content, "| Ana") {
		t.Fatalf("faltou a tabela: %s", res.Content)
	}
	if res.Metadata["projection"] != true {
		t.Fatalf("metadata=%v", res.Metadata)
	}
}

// Documento opaco não depende do modo: sem projeção não há leitura nenhuma.
func TestReadFileProjectsDOCXInEveryMode(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "doc.docx"), buildMinimalDOCX(t, "Corpo"), 0644); err != nil {
		t.Fatal(err)
	}
	tool := NewReadFile(dir)
	for _, mode := range []string{"", "auto", "markdown"} {
		args := map[string]any{"path": "doc.docx"}
		if mode != "" {
			args["document_mode"] = mode
		}
		res, err := tool.Execute(context.Background(), mustJSON(t, args))
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("modo %q: %s", mode, res.Content)
		}
		if !strings.Contains(res.Content, "Corpo") {
			t.Fatalf("modo %q sem corpo extraído: %s", mode, res.Content)
		}
	}
}

// Modo desconhecido falha em vez de virar auto silenciosamente.
func TestReadFileRejectsUnknownDocumentMode(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	tool := NewReadFile(dir)
	for _, mode := range []string{"ocr", "html"} {
		res, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
			"path":          "a.txt",
			"document_mode": mode,
		}))
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError {
			t.Fatalf("modo %q deveria falhar", mode)
		}
		if !strings.Contains(res.Content, "document_mode") {
			t.Fatalf("modo %q: mensagem sem o parâmetro: %s", mode, res.Content)
		}
	}
}

// Terminar o caminho em .csv não abre exceção para gravar bytes binários.
func TestWriteFileRejectsBinaryContentUnderTextExtension(t *testing.T) {
	dir := t.TempDir()
	tool := NewWriteFile(dir)
	res, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"path":    "planilha.csv",
		"content": "a\x00b\xff",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("conteúdo binário deveria ser recusado")
	}
	if !strings.Contains(res.Content, "binário") {
		t.Fatalf("mensagem inesperada: %s", res.Content)
	}
	if _, err := os.Stat(filepath.Join(dir, "planilha.csv")); !os.IsNotExist(err) {
		t.Fatal("o arquivo não deveria ter sido criado")
	}
}

// Substituir texto por bytes binários é escrita de binário, mesmo partindo de um
// arquivo de texto legítimo.
func TestEditToolsRejectBinaryReplacement(t *testing.T) {
	dir := t.TempDir()
	original := "alfa\nbeta\n"

	casos := []struct {
		nome string
		exec func(caminho string) (tools.ToolResult, error)
	}{
		{"edit_file", func(caminho string) (tools.ToolResult, error) {
			return NewEditFile(dir, nil).Execute(context.Background(), mustJSON(t, map[string]any{
				"path":       filepath.Base(caminho),
				"old_string": "beta",
				"new_string": "be\x00ta",
			}))
		}},
		{"text_edit", func(caminho string) (tools.ToolResult, error) {
			return NewTextEdit(dir, &fakeQuestionnaireRequester{}).Execute(editorCtx(caminho), mustJSON(t, map[string]any{
				"original":    "beta",
				"replacement": "be\x00ta",
			}))
		}},
	}

	for _, c := range casos {
		caminho := filepath.Join(dir, c.nome+".txt")
		if err := os.WriteFile(caminho, []byte(original), 0644); err != nil {
			t.Fatal(err)
		}
		res, err := c.exec(caminho)
		if err != nil {
			t.Fatalf("%s: %v", c.nome, err)
		}
		if !res.IsError {
			t.Fatalf("%s deveria recusar conteúdo binário", c.nome)
		}
		depois, err := os.ReadFile(caminho)
		if err != nil {
			t.Fatal(err)
		}
		if string(depois) != original {
			t.Fatalf("%s alterou o arquivo mesmo recusando: %q", c.nome, depois)
		}
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

// O teto de 32 MiB é da extração. CSV lido como texto passa por ele; só quando a
// projeção é pedida o arquivo precisa caber inteiro na memória (D12).
func TestOversizedCSVOnlyBlockedWhenProjectionIsRequested(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grande.csv")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("nome,idade\nAna,30\n"); err != nil {
		t.Fatal(err)
	}
	size := int64(docextract.MaxExtractBytes) + 1
	if err := f.Truncate(size); err != nil {
		_ = f.Close()
		t.Skipf("não foi possível criar arquivo grande: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	if msg, rejected := rejectOversizedDocument(path, "grande.csv", size, docextract.ModeAuto); rejected {
		t.Fatalf("leitura como texto não deveria bater no teto: %s", msg)
	}
	msg, rejected := rejectOversizedDocument(path, "grande.csv", size, docextract.ModeMarkdown)
	if !rejected {
		t.Fatal("projeção acima do teto deveria falhar")
	}
	if !strings.Contains(msg, "muito grande para extração") {
		t.Fatalf("mensagem inesperada: %s", msg)
	}
}

func TestReadFileSizeBytesIsInt64(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("conteudo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	tool := NewReadFile(dir)
	res, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{"path": "a.txt"}))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res.Metadata["size_bytes"].(int64); !ok {
		t.Fatalf("size_bytes=%T", res.Metadata["size_bytes"])
	}
}

// Binário sem leitura convertida é recusado pelo formato, não pelo tamanho.
func TestReadFileOversizedUnsupportedBinaryReportsFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grande.bin")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte{0x00, 0x01, 0x02, 0xff, 0xfe}); err != nil {
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
	res, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{"path": "grande.bin"}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected error")
	}
	if strings.Contains(res.Content, "muito grande") {
		t.Fatalf("motivo deveria ser o formato, não o tamanho: %s", res.Content)
	}
	if !strings.Contains(res.Content, "não suportado") {
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
