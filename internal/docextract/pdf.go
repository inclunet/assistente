package docextract

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)

func extractPDF(data []byte, filename string) (*Result, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		msg := err.Error()
		if strings.Contains(strings.ToLower(msg), "encrypt") || strings.Contains(strings.ToLower(msg), "password") {
			return nil, fmt.Errorf("PDF protegido por senha ou criptografado — senha não é suportada neste AEP")
		}
		return nil, fmt.Errorf("falha ao abrir PDF: %w", err)
	}

	n := r.NumPage()
	var pages []string
	emptyPages := 0
	for i := 1; i <= n; i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			emptyPages++
			continue
		}
		text, err := p.GetPlainText(nil)
		if err != nil {
			emptyPages++
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			emptyPages++
			continue
		}
		var b strings.Builder
		fmt.Fprintf(&b, "## Página %d\n\n%s\n", i, text)
		pages = append(pages, b.String())
	}

	res := &Result{
		Kind:   KindPDF,
		Source: filename,
		Pages:  n,
	}

	if len(pages) == 0 {
		res.Markdown = ""
		res.Warnings = append(res.Warnings,
			"PDF sem camada de texto extraível. OCR ainda não está disponível (previsto para a Fase 3 do AEP-0093).",
		)
		return res, nil
	}

	res.Markdown = strings.Join(pages, "\n")
	if emptyPages > 0 {
		res.Warnings = append(res.Warnings,
			fmt.Sprintf("%d página(s) sem texto extraível; OCR ainda não está disponível (previsto para a Fase 3 do AEP-0093).", emptyPages),
		)
	}
	return res, nil
}
