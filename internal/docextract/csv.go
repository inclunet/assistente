package docextract

import (
	"encoding/csv"
	"fmt"
	"strings"
	"unicode/utf8"
)

func extractCSV(data []byte, filename string) (*Result, error) {
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("CSV não é UTF-8 válido")
	}
	content := string(data)
	delim := ','
	if semi := strings.Count(content, ";"); semi > strings.Count(content, ",") {
		delim = ';'
	}
	r := csv.NewReader(strings.NewReader(content))
	r.Comma = delim
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		// Fallback: trata como texto bruto tabular
		return &Result{
			Kind:     KindCSV,
			Source:   filename,
			Markdown: "```csv\n" + content + "\n```\n",
			Warnings: []string{fmt.Sprintf("parser CSV falhou (%v); conteúdo em bloco de código", err)},
		}, nil
	}
	md := rowsToMarkdown(records)
	return &Result{
		Kind:     KindCSV,
		Source:   filename,
		Markdown: md + "\n",
	}, nil
}
