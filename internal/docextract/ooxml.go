package docextract

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// xmlSpaceCollapse colapsa espaços horizontais sem tocar em `\n`, para não
// achatar os parágrafos que a conversão de HTML insere.
var xmlSpaceCollapse = regexp.MustCompile(`[^\S\n]+`)

// nextToken devolve (token, fim, erro). XML truncado/malformado vira erro real
// em vez de projeção parcial silenciosa.
func nextToken(dec *xml.Decoder) (xml.Token, bool, error) {
	tok, err := dec.Token()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, true, nil
		}
		return nil, true, fmt.Errorf("XML inválido: %w", err)
	}
	return tok, false, nil
}

func extractDOCX(data []byte, filename string) (*Result, error) {
	zr, err := openZip(data)
	if err != nil {
		return nil, err
	}
	f := findZipName(zr, "word/document.xml")
	if f == nil {
		return nil, fmt.Errorf("DOCX sem word/document.xml")
	}
	lim := &zipLimits{}
	body, err := readZipFile(f, lim)
	if err != nil {
		return nil, err
	}
	text, err := extractOOXMLDocumentText(body)
	if err != nil {
		return nil, err
	}
	return &Result{
		Kind:     KindDOCX,
		Source:   filename,
		Markdown: text,
	}, nil
}

func extractOOXMLDocumentText(xmlData []byte) (string, error) {
	dec := xml.NewDecoder(bytes.NewReader(xmlData))
	var b strings.Builder
	inText := false
	for {
		tok, done, err := nextToken(dec)
		if err != nil {
			return "", err
		}
		if done {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch local(t.Name) {
			case "t":
				inText = true
			case "tab":
				b.WriteByte('\t')
			case "br", "cr":
				b.WriteByte('\n')
			}
		case xml.EndElement:
			switch local(t.Name) {
			case "t":
				inText = false
			case "p":
				b.WriteString("\n\n")
			}
		case xml.CharData:
			if inText {
				b.Write(t)
			}
		}
	}
	return strings.TrimSpace(b.String()) + "\n", nil
}

func extractXLSX(data []byte, filename string) (*Result, error) {
	zr, err := openZip(data)
	if err != nil {
		return nil, err
	}
	lim := &zipLimits{}

	shared := []string{}
	if sf := findZipName(zr, "xl/sharedStrings.xml"); sf != nil {
		body, err := readZipFile(sf, lim)
		if err != nil {
			return nil, err
		}
		shared, err = parseSharedStrings(body)
		if err != nil {
			return nil, err
		}
	}

	wb := findZipName(zr, "xl/workbook.xml")
	if wb == nil {
		return nil, fmt.Errorf("XLSX sem xl/workbook.xml")
	}
	wbBody, err := readZipFile(wb, lim)
	if err != nil {
		return nil, err
	}
	sheets, err := parseWorkbookSheets(wbBody)
	if err != nil {
		return nil, err
	}

	// Relacionamentos sheet → path
	rels := map[string]string{}
	if rf := findZipName(zr, "xl/_rels/workbook.xml.rels"); rf != nil {
		rb, err := readZipFile(rf, lim)
		if err != nil {
			return nil, err
		}
		rels, err = parseOOXMLRels(rb)
		if err != nil {
			return nil, err
		}
	}

	var parts []string
	for i, sh := range sheets {
		target := rels[sh.RID]
		if target == "" {
			target = fmt.Sprintf("worksheets/sheet%d.xml", i+1)
		}
		sheetPath := path.Join("xl", strings.TrimPrefix(target, "/"))
		sheetPath = strings.ReplaceAll(sheetPath, "\\", "/")
		sf := findZipName(zr, sheetPath)
		if sf == nil {
			continue
		}
		sb, err := readZipFile(sf, lim)
		if err != nil {
			return nil, err
		}
		table, err := parseSheetToMarkdown(sb, shared)
		if err != nil {
			return nil, err
		}
		parts = append(parts, fmt.Sprintf("## Aba: %s\n\n%s", sh.Name, table))
	}

	return &Result{
		Kind:     KindXLSX,
		Source:   filename,
		Pages:    len(sheets),
		Markdown: strings.Join(parts, "\n\n") + "\n",
	}, nil
}

type wbSheet struct {
	Name string
	RID  string
}

func parseWorkbookSheets(data []byte) ([]wbSheet, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var sheets []wbSheet
	for {
		tok, done, err := nextToken(dec)
		if err != nil {
			return nil, err
		}
		if done {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok || local(se.Name) != "sheet" {
			continue
		}
		s := wbSheet{}
		for _, a := range se.Attr {
			switch local(a.Name) {
			case "name":
				s.Name = a.Value
			case "id":
				s.RID = a.Value
			}
		}
		sheets = append(sheets, s)
	}
	return sheets, nil
}

func parseOOXMLRels(data []byte) (map[string]string, error) {
	out := map[string]string{}
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, done, err := nextToken(dec)
		if err != nil {
			return nil, err
		}
		if done {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok || local(se.Name) != "Relationship" {
			continue
		}
		var id, target string
		for _, a := range se.Attr {
			switch local(a.Name) {
			case "Id":
				id = a.Value
			case "Target":
				target = a.Value
			}
		}
		if id != "" && target != "" {
			out[id] = target
		}
	}
	return out, nil
}

func parseSharedStrings(data []byte) ([]string, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var out []string
	var cur strings.Builder
	inSI, inT := false, false
	for {
		tok, done, err := nextToken(dec)
		if err != nil {
			return nil, err
		}
		if done {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch local(t.Name) {
			case "si":
				inSI = true
				cur.Reset()
			case "t":
				if inSI {
					inT = true
				}
			}
		case xml.EndElement:
			switch local(t.Name) {
			case "t":
				inT = false
			case "si":
				out = append(out, cur.String())
				inSI = false
			}
		case xml.CharData:
			if inT {
				cur.Write(t)
			}
		}
	}
	return out, nil
}

func parseSheetToMarkdown(data []byte, shared []string) (string, error) {
	type cell struct {
		ref, t, v string
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	var rows [][]cell
	var curRow []cell
	var cur cell
	inV := false
	for {
		tok, done, err := nextToken(dec)
		if err != nil {
			return "", err
		}
		if done {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch local(t.Name) {
			case "row":
				curRow = nil
			case "c":
				cur = cell{}
				for _, a := range t.Attr {
					switch local(a.Name) {
					case "r":
						cur.ref = a.Value
					case "t":
						cur.t = a.Value
					}
				}
			case "v", "t":
				inV = true
			}
		case xml.EndElement:
			switch local(t.Name) {
			case "v", "t":
				inV = false
			case "c":
				curRow = append(curRow, cur)
			case "row":
				if len(curRow) > 0 {
					rows = append(rows, curRow)
				}
			}
		case xml.CharData:
			if inV {
				cur.v += string(t)
			}
		}
	}

	if len(rows) == 0 {
		return "_(aba vazia)_", nil
	}

	maxCol := 0
	grid := make([]map[int]string, len(rows))
	for i, row := range rows {
		grid[i] = map[int]string{}
		for _, c := range row {
			col := colIndexFromRef(c.ref)
			val := c.v
			if c.t == "s" {
				idx, err := strconv.Atoi(strings.TrimSpace(c.v))
				if err == nil && idx >= 0 && idx < len(shared) {
					val = shared[idx]
				}
			}
			grid[i][col] = escapeMDCell(val)
			if col > maxCol {
				maxCol = col
			}
		}
	}

	var b strings.Builder
	writeMDRow := func(vals []string) {
		b.WriteString("| ")
		b.WriteString(strings.Join(vals, " | "))
		b.WriteString(" |\n")
	}
	header := make([]string, maxCol+1)
	for c := 0; c <= maxCol; c++ {
		header[c] = grid[0][c]
	}
	writeMDRow(header)
	sep := make([]string, maxCol+1)
	for i := range sep {
		sep[i] = "---"
	}
	writeMDRow(sep)
	for i := 1; i < len(grid); i++ {
		row := make([]string, maxCol+1)
		for c := 0; c <= maxCol; c++ {
			row[c] = grid[i][c]
		}
		writeMDRow(row)
	}
	return b.String(), nil
}

func colIndexFromRef(ref string) int {
	col := 0
	for _, r := range ref {
		if r < 'A' || r > 'Z' {
			break
		}
		col = col*26 + int(r-'A'+1)
	}
	if col == 0 {
		return 0
	}
	return col - 1
}

func escapeMDCell(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.TrimSpace(s)
}

func extractPPTX(data []byte, filename string) (*Result, error) {
	zr, err := openZip(data)
	if err != nil {
		return nil, err
	}
	lim := &zipLimits{}

	type slideRef struct {
		name string
		idx  int
	}
	var slideFiles []slideRef
	for _, f := range zr.File {
		n := strings.ReplaceAll(f.Name, "\\", "/")
		if strings.HasPrefix(n, "ppt/slides/slide") && strings.HasSuffix(n, ".xml") && !strings.Contains(n, "_rels") {
			base := path.Base(n)
			num, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(base, "slide"), ".xml"))
			if err != nil {
				continue
			}
			slideFiles = append(slideFiles, slideRef{n, num})
		}
	}
	sort.Slice(slideFiles, func(i, j int) bool { return slideFiles[i].idx < slideFiles[j].idx })

	var parts []string
	for _, sf := range slideFiles {
		f := findZipName(zr, sf.name)
		if f == nil {
			continue
		}
		body, err := readZipFile(f, lim)
		if err != nil {
			return nil, err
		}
		text, err := extractOOXMLDocumentText(body)
		if err != nil {
			return nil, err
		}
		parts = append(parts, fmt.Sprintf("## Slide %d\n\n%s", sf.idx, strings.TrimSpace(text)))
	}

	return &Result{
		Kind:     KindPPTX,
		Source:   filename,
		Pages:    len(parts),
		Markdown: strings.Join(parts, "\n\n") + "\n",
	}, nil
}

func local(n xml.Name) string {
	if n.Local != "" {
		return n.Local
	}
	return n.Space
}
