package docextract

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

func extractODT(data []byte, filename string) (*Result, error) {
	text, err := extractODFText(data)
	if err != nil {
		return nil, err
	}
	return &Result{Kind: KindODT, Source: filename, Markdown: text}, nil
}

func extractODS(data []byte, filename string) (*Result, error) {
	tables, n, err := extractODFTables(data)
	if err != nil {
		return nil, err
	}
	return &Result{Kind: KindODS, Source: filename, Pages: n, Markdown: tables}, nil
}

func extractODP(data []byte, filename string) (*Result, error) {
	slides, n, err := extractODFSlides(data)
	if err != nil {
		return nil, err
	}
	return &Result{Kind: KindODP, Source: filename, Pages: n, Markdown: slides}, nil
}

func readODFContent(data []byte) ([]byte, error) {
	zr, err := openZip(data)
	if err != nil {
		return nil, err
	}
	f := findZipName(zr, "content.xml")
	if f == nil {
		return nil, fmt.Errorf("ODF sem content.xml")
	}
	return readZipFile(f, &zipLimits{})
}

func extractODFText(data []byte) (string, error) {
	body, err := readODFContent(data)
	if err != nil {
		return "", err
	}
	dec := xml.NewDecoder(bytes.NewReader(body))
	var b strings.Builder
	inP := false
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
			case "p", "h":
				inP = true
			case "line-break":
				b.WriteByte('\n')
			case "tab":
				b.WriteByte('\t')
			}
		case xml.EndElement:
			if local(t.Name) == "p" || local(t.Name) == "h" {
				b.WriteString("\n\n")
				inP = false
			}
		case xml.CharData:
			if inP {
				b.Write(t)
			}
		}
	}
	return strings.TrimSpace(b.String()) + "\n", nil
}

func extractODFTables(data []byte) (string, int, error) {
	body, err := readODFContent(data)
	if err != nil {
		return "", 0, err
	}
	dec := xml.NewDecoder(bytes.NewReader(body))
	var parts []string
	var curName string
	var rows [][]string
	var curRow []string
	var curCell strings.Builder
	inTable, inRow, inCell := false, false, false

	flushTable := func() {
		if !inTable {
			return
		}
		name := curName
		if name == "" {
			name = fmt.Sprintf("Planilha %d", len(parts)+1)
		}
		parts = append(parts, fmt.Sprintf("## Aba: %s\n\n%s", name, rowsToMarkdown(rows)))
		rows = nil
		curName = ""
		inTable = false
	}

	for {
		tok, done, err := nextToken(dec)
		if err != nil {
			return "", 0, err
		}
		if done {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch local(t.Name) {
			case "table":
				flushTable()
				inTable = true
				for _, a := range t.Attr {
					if local(a.Name) == "name" {
						curName = a.Value
					}
				}
			case "table-row":
				if inTable {
					inRow = true
					curRow = nil
				}
			case "table-cell", "covered-table-cell":
				if inRow {
					inCell = true
					curCell.Reset()
				}
			}
		case xml.EndElement:
			switch local(t.Name) {
			case "table-cell", "covered-table-cell":
				if inCell {
					curRow = append(curRow, escapeMDCell(curCell.String()))
					inCell = false
				}
			case "table-row":
				if inRow {
					rows = append(rows, curRow)
					inRow = false
				}
			case "table":
				flushTable()
			}
		case xml.CharData:
			if inCell {
				curCell.Write(t)
			}
		}
	}
	flushTable()
	return strings.Join(parts, "\n\n") + "\n", len(parts), nil
}

func rowsToMarkdown(rows [][]string) string {
	if len(rows) == 0 {
		return "_(aba vazia)_"
	}
	maxCol := 0
	for _, r := range rows {
		if len(r) > maxCol {
			maxCol = len(r)
		}
	}
	pad := func(r []string) []string {
		out := make([]string, maxCol)
		copy(out, r)
		return out
	}
	var b strings.Builder
	write := func(vals []string) {
		b.WriteString("| ")
		b.WriteString(strings.Join(vals, " | "))
		b.WriteString(" |\n")
	}
	write(pad(rows[0]))
	sep := make([]string, maxCol)
	for i := range sep {
		sep[i] = "---"
	}
	write(sep)
	for i := 1; i < len(rows); i++ {
		write(pad(rows[i]))
	}
	return b.String()
}

func extractODFSlides(data []byte) (string, int, error) {
	body, err := readODFContent(data)
	if err != nil {
		return "", 0, err
	}
	dec := xml.NewDecoder(bytes.NewReader(body))
	var parts []string
	var cur strings.Builder
	inPage, inText := false, false
	pageNum := 0

	flush := func() {
		if !inPage {
			return
		}
		parts = append(parts, fmt.Sprintf("## Slide %d\n\n%s\n", pageNum, strings.TrimSpace(cur.String())))
		cur.Reset()
		inPage = false
	}

	for {
		tok, done, err := nextToken(dec)
		if err != nil {
			return "", 0, err
		}
		if done {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch local(t.Name) {
			case "page":
				flush()
				pageNum++
				inPage = true
			case "p", "h", "span":
				if inPage {
					inText = true
				}
			}
		case xml.EndElement:
			switch local(t.Name) {
			case "p", "h":
				if inPage {
					cur.WriteString("\n\n")
					inText = false
				}
			case "span":
				inText = false
			case "page":
				flush()
			}
		case xml.CharData:
			if inPage && inText {
				cur.Write(t)
			}
		}
	}
	flush()
	return strings.Join(parts, "\n") + "\n", len(parts), nil
}
