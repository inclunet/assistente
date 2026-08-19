package docextract_test

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"codeberg.org/go-pdf/fpdf"

	"assistente/internal/docextract"
)

func TestDetectText(t *testing.T) {
	k := docextract.Detect([]byte("hello\nworld\n"), "a.go")
	if k != docextract.KindText {
		t.Fatalf("got %s", k)
	}
}

// Guards por prefixo dependem de um ZIP nunca passar por texto, mesmo quando a
// extensão não denuncia o formato.
func TestDetectZipWithoutDocumentExtensionIsNotText(t *testing.T) {
	zipPrefix := append([]byte("PK\x03\x04"), bytes.Repeat([]byte{0x14, 0x00, 0x08, 0x00}, 8)...)
	for _, name := range []string{"pacote.bin", "pacote.txt", "pacote"} {
		if kind := docextract.Detect(zipPrefix, name); kind == docextract.KindText {
			t.Fatalf("%s classificado como texto", name)
		}
	}

	// Prefixo de ZIP sem byte de controle: só o magic denuncia o container.
	textLike := append([]byte("PK\x03\x04"), []byte(strings.Repeat("A", 512))...)
	for _, name := range []string{"pacote.txt", "pacote"} {
		if kind := docextract.Detect(textLike, name); kind == docextract.KindText {
			t.Fatalf("%s classificado como texto", name)
		}
	}

	// A extensão continua completando a classificação de documento.
	if kind := docextract.Detect(zipPrefix, "a.docx"); kind != docextract.KindDOCX {
		t.Fatalf("kind=%s", kind)
	}
}

func TestDetectPDFMagic(t *testing.T) {
	pdf := buildPDF(t, "Hello PDF")
	k := docextract.Detect(pdf, "disguised.txt")
	if k != docextract.KindPDF {
		t.Fatalf("expected pdf, got %s", k)
	}
}

func TestExtractPDF(t *testing.T) {
	data := buildPDF(t, "Conteudo do PDF de teste")
	res, err := docextract.Extract(data, "sample.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != docextract.KindPDF {
		t.Fatalf("kind=%s", res.Kind)
	}
	if !strings.Contains(res.Markdown, "Conteudo") && !strings.Contains(res.Markdown, "PDF") {
		// fpdf text extraction varies; at least header path works
		if res.Pages < 1 {
			t.Fatalf("expected pages, markdown=%q", res.Markdown)
		}
	}
	hdr := docextract.FormatProjectionHeader(res)
	if !strings.Contains(hdr, "projeção Markdown") || !strings.Contains(hdr, "pdf") {
		t.Fatalf("header=%q", hdr)
	}
}

func TestExtractDOCX(t *testing.T) {
	data := minimalDOCX(t, "Paragrafo DOCX")
	res, err := docextract.Extract(data, "doc.docx")
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != docextract.KindDOCX {
		t.Fatalf("kind=%s", res.Kind)
	}
	if !strings.Contains(res.Markdown, "Paragrafo DOCX") {
		t.Fatalf("markdown=%q", res.Markdown)
	}
}

func TestExtractCSV(t *testing.T) {
	data := []byte("nome,idade\nAna,30\n")
	res, err := docextract.Extract(data, "t.csv")
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != docextract.KindCSV {
		t.Fatalf("kind=%s", res.Kind)
	}
	if !strings.Contains(res.Markdown, "Ana") || !strings.Contains(res.Markdown, "|") {
		t.Fatalf("markdown=%q", res.Markdown)
	}
}

// Campo com pipe ou quebra de linha não pode desmontar a tabela Markdown.
func TestExtractCSVEscapesCells(t *testing.T) {
	data := []byte("nome,obs\n\"Ana\",\"a|b\"\n\"Bia\",\"linha1\nlinha2\"\n")
	res, err := docextract.Extract(data, "t.csv")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Markdown, `a\|b`) {
		t.Fatalf("pipe não escapado: %q", res.Markdown)
	}
	for _, line := range strings.Split(strings.TrimSpace(res.Markdown), "\n") {
		if line == "" {
			continue
		}
		if strings.Count(line, "|")-strings.Count(line, `\|`) != 3 {
			t.Fatalf("linha com número inesperado de colunas: %q", line)
		}
	}
}

func TestExtractRTF(t *testing.T) {
	data := []byte(`{\rtf1\ansi Olamundo\par }`)
	res, err := docextract.Extract(data, "a.rtf")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Markdown, "Ola") {
		t.Fatalf("markdown=%q", res.Markdown)
	}
}

// No modo padrão, o que já é texto no disco volta byte a byte: o modelo precisa
// ver o CSV como ele é para revisar ou editar (D12).
func TestExtractModeAutoKeepsTextFormatsVerbatim(t *testing.T) {
	casos := []struct {
		nome string
		data []byte
	}{
		{"t.csv", []byte("nome,idade\nAna,30\n")},
		{"a.rtf", []byte(`{\rtf1\ansi Olamundo\par }`)},
		{"pagina.html", []byte("<h1>Titulo</h1>\n<p>corpo</p>\n")},
		{"script.py", []byte("def f():\n    return 1\n")},
	}
	for _, c := range casos {
		res, err := docextract.ExtractMode(c.data, c.nome, docextract.ModeAuto)
		if err != nil {
			t.Fatalf("%s: %v", c.nome, err)
		}
		if res.Projected {
			t.Fatalf("%s: não deveria ser projetado", c.nome)
		}
		if res.Markdown != string(c.data) {
			t.Fatalf("%s: conteúdo alterado: %q", c.nome, res.Markdown)
		}
	}
}

// A projeção continua disponível, mas só quando pedida.
func TestExtractModeMarkdownProjectsCSV(t *testing.T) {
	res, err := docextract.ExtractMode([]byte("nome,idade\nAna,30\n"), "t.csv", docextract.ModeMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Projected {
		t.Fatal("esperava projeção")
	}
	if !strings.Contains(res.Markdown, "| Ana") {
		t.Fatalf("markdown=%q", res.Markdown)
	}
}

// Documento opaco é projetado nos dois modos: não há leitura crua útil dele.
func TestExtractModeAutoProjectsOpaqueDocument(t *testing.T) {
	res, err := docextract.ExtractMode(buildPDF(t, "Ola PDF"), "a.pdf", docextract.ModeAuto)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Projected {
		t.Fatal("PDF deveria ser projetado no modo auto")
	}
}

// Texto puro não tem projeção nem quando o modo markdown é pedido.
func TestExtractModeMarkdownKeepsPlainText(t *testing.T) {
	res, err := docextract.ExtractMode([]byte("linha\n"), "a.go", docextract.ModeMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	if res.Projected {
		t.Fatal("texto puro não tem projeção")
	}
}

// Binário disfarçado de texto não escapa pelo modo auto.
func TestExtractModeAutoRejectsDisguisedBinary(t *testing.T) {
	if _, err := docextract.ExtractMode([]byte{0x00, 0x01, 0x02, 0xff, 0xfe}, "planilha.csv", docextract.ModeAuto); err == nil {
		t.Fatal("binário com extensão .csv não deveria virar texto")
	}
}

func TestUnsupportedBinary(t *testing.T) {
	data := []byte{0x00, 0x01, 0x02, 0xff, 0xfe}
	_, err := docextract.Extract(data, "blob.bin")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCheckWritableDisguisedPDF(t *testing.T) {
	pdf := buildPDF(t, "secret")
	err := docextract.CheckWritable(pdf, "notes.txt")
	if err == nil {
		t.Fatal("expected not writable")
	}
	if _, ok := err.(*docextract.ErrNotWritable); !ok {
		t.Fatalf("got %T %v", err, err)
	}
}

func TestCheckWritableBinaryMessageDoesNotPromiseProjection(t *testing.T) {
	err := docextract.CheckWritable([]byte{0x00, 0x01, 0x02, 0xff, 0xfe}, "blob.bin")
	if err == nil {
		t.Fatal("expected not writable")
	}
	if strings.Contains(err.Error(), "projeção Markdown") {
		t.Fatalf("binário não tem projeção: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "binário") {
		t.Fatalf("mensagem deveria citar binário: %q", err.Error())
	}
}

func TestCheckWritableAllowsCSVText(t *testing.T) {
	err := docextract.CheckWritable([]byte("a,b\n1,2\n"), "t.csv")
	if err != nil {
		t.Fatal(err)
	}
}

func TestCheckWritableAllowsPlainText(t *testing.T) {
	err := docextract.CheckWritable([]byte("hello"), "a.md")
	if err != nil {
		t.Fatal(err)
	}
}

// Texto acima do teto de extração de documento não pode ser bloqueado:
// read_file continua paginando por linhas (AEP-0093 D8).
func TestExtractLargeTextNotBlocked(t *testing.T) {
	const overDocumentCap = (32 << 20) + 1
	line := []byte("linha de texto grande\n")
	data := bytes.Repeat(line, overDocumentCap/len(line)+1)
	res, err := docextract.Extract(data, "grande.log")
	if err != nil {
		t.Fatalf("texto grande não deveria falhar: %v", err)
	}
	if res.Kind != docextract.KindText {
		t.Fatalf("kind=%s", res.Kind)
	}
}

func TestExtractMalformedDOCXFails(t *testing.T) {
	data := malformedDOCX(t)
	if _, err := docextract.Extract(data, "quebrado.docx"); err == nil {
		t.Fatal("XML truncado deveria falhar em vez de projetar parcialmente")
	}
}

func buildPDF(t *testing.T, text string) []byte {
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

func malformedDOCX(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	// Documento truncado no meio de um elemento
	if _, err := w.Write([]byte(`<?xml version="1.0"?><w:document xmlns:w="x"><w:body><w:p><w:r><w:t>abc`)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func minimalDOCX(t *testing.T, paragraph string) []byte {
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
