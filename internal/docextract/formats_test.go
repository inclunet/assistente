package docextract_test

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"assistente/internal/docextract"
)

func TestExtractXLSX(t *testing.T) {
	data := minimalXLSX(t)
	res, err := docextract.Extract(data, "s.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != docextract.KindXLSX {
		t.Fatalf("kind=%s", res.Kind)
	}
	if !strings.Contains(res.Markdown, "Hello") {
		t.Fatalf("markdown=%q", res.Markdown)
	}
}

func TestExtractXLSXCellSplitAcrossTokens(t *testing.T) {
	data := writeZip(t, map[string]string{
		"xl/workbook.xml": `<?xml version="1.0"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"
 xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets><sheet name="Folha1" sheetId="1" r:id="rId1"/></sheets>
</workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Target="worksheets/sheet1.xml"/>
</Relationships>`,
		// Entidade força o parser a emitir CharData em pedaços
		"xl/worksheets/sheet1.xml": `<?xml version="1.0"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <sheetData><row r="1"><c r="A1" t="inlineStr"><t>Ana &amp; Bia &amp; Caio</t></c></row></sheetData>
</worksheet>`,
	})
	res, err := docextract.Extract(data, "s.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Markdown, "Ana & Bia & Caio") {
		t.Fatalf("valor da célula fragmentado: %q", res.Markdown)
	}
}

func TestExtractODT(t *testing.T) {
	data := minimalODT(t, "Texto ODT")
	res, err := docextract.Extract(data, "a.odt")
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != docextract.KindODT {
		t.Fatalf("kind=%s", res.Kind)
	}
	if !strings.Contains(res.Markdown, "Texto ODT") {
		t.Fatalf("markdown=%q", res.Markdown)
	}
}

func TestExtractEPUB(t *testing.T) {
	data := minimalEPUB(t, "Capitulo EPUB")
	res, err := docextract.Extract(data, "a.epub")
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != docextract.KindEPUB {
		t.Fatalf("kind=%s", res.Kind)
	}
	if !strings.Contains(res.Markdown, "Capitulo EPUB") {
		t.Fatalf("markdown=%q", res.Markdown)
	}
}

// Span no meio do parágrafo não pode cortar o texto que vem depois dele.
func TestExtractODPKeepsTextAfterSpan(t *testing.T) {
	data := minimalODP(t, `foo <text:span>bar</text:span> baz`)
	res, err := docextract.Extract(data, "a.odp")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Markdown, "foo bar baz") {
		t.Fatalf("texto após o span foi perdido: %q", res.Markdown)
	}
}

func TestExtractEPUBKeepsParagraphs(t *testing.T) {
	data := minimalEPUB(t, "Primeiro paragrafo</p><p>Segundo paragrafo")
	res, err := docextract.Extract(data, "a.epub")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Markdown, "Primeiro paragrafo\n\nSegundo paragrafo") {
		t.Fatalf("paragrafos achatados: %q", res.Markdown)
	}
}

func TestExtractPPTX(t *testing.T) {
	data := minimalPPTX(t, "Slide Texto")
	res, err := docextract.Extract(data, "a.pptx")
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != docextract.KindPPTX {
		t.Fatalf("kind=%s", res.Kind)
	}
	if !strings.Contains(res.Markdown, "Slide Texto") {
		t.Fatalf("markdown=%q", res.Markdown)
	}
}

func writeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
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

func minimalXLSX(t *testing.T) []byte {
	t.Helper()
	return writeZip(t, map[string]string{
		"[Content_Types].xml": `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"></Types>`,
		"xl/workbook.xml": `<?xml version="1.0"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"
 xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets><sheet name="Folha1" sheetId="1" r:id="rId1"/></sheets>
</workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
</Relationships>`,
		"xl/sharedStrings.xml": `<?xml version="1.0"?>
<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="1" uniqueCount="1">
  <si><t>Hello</t></si>
</sst>`,
		"xl/worksheets/sheet1.xml": `<?xml version="1.0"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <sheetData><row r="1"><c r="A1" t="s"><v>0</v></c></row></sheetData>
</worksheet>`,
	})
}

func minimalODT(t *testing.T, text string) []byte {
	t.Helper()
	return writeZip(t, map[string]string{
		"mimetype": "application/vnd.oasis.opendocument.text",
		"content.xml": `<?xml version="1.0"?>
<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
 xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
  <office:body><office:text><text:p>` + text + `</text:p></office:text></office:body>
</office:document-content>`,
	})
}

func minimalODP(t *testing.T, paragraph string) []byte {
	t.Helper()
	return writeZip(t, map[string]string{
		"mimetype": "application/vnd.oasis.opendocument.presentation",
		"content.xml": `<?xml version="1.0"?>
<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
 xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0"
 xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
  <office:body><office:presentation>
    <draw:page draw:name="p1"><draw:frame><draw:text-box><text:p>` + paragraph + `</text:p></draw:text-box></draw:frame></draw:page>
  </office:presentation></office:body>
</office:document-content>`,
	})
}

func minimalEPUB(t *testing.T, text string) []byte {
	t.Helper()
	return writeZip(t, map[string]string{
		"mimetype": "application/epub+zip",
		"META-INF/container.xml": `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`,
		"OEBPS/content.opf": `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <manifest>
    <item id="c1" href="chap.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="c1"/></spine>
</package>`,
		"OEBPS/chap.xhtml": `<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml"><body><p>` + text + `</p></body></html>`,
	})
}

func minimalPPTX(t *testing.T, text string) []byte {
	t.Helper()
	return writeZip(t, map[string]string{
		"[Content_Types].xml": `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"></Types>`,
		"ppt/presentation.xml": `<?xml version="1.0"?><p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"></p:presentation>`,
		"ppt/slides/slide1.xml": `<?xml version="1.0"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
 xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
  <p:cSld><p:spTree><p:sp><p:txBody><a:p><a:r><a:t>` + text + `</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld>
</p:sld>`,
	})
}
