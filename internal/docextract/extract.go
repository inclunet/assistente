package docextract

import (
	"fmt"
	"strings"
)

const maxExtractBytes = 32 << 20 // 32 MiB de entrada para extração de documento

// Extract detecta o formato e projeta para Markdown. Para KindText devolve
// o conteúdo bruto (sem cabeçalho de projeção — quem chama decide).
func Extract(data []byte, filename string) (*Result, error) {
	kind := Detect(data, filename)
	res := &Result{Kind: kind, Source: filename}

	// O limite vale só para extração; texto/código segue como antes (D8: sem
	// hard-deny artificial em arquivo grande que o chamador pagina por linhas).
	if kind != KindText && len(data) > maxExtractBytes {
		return nil, fmt.Errorf("arquivo muito grande para extração (%d bytes; máximo %d)", len(data), maxExtractBytes)
	}

	switch kind {
	case KindText:
		res.Markdown = string(data)
		return res, nil
	case KindUnsupportedBinary:
		return nil, &ErrUnsupported{
			Kind: kind,
			Msg:  "formato binário não suportado para leitura convertida — use um formato V1 (PDF textual, OOXML, ODF, CSV, RTF, EPUB) ou arquivo de texto",
		}
	case KindPDF:
		return extractPDF(data, filename)
	case KindDOCX:
		return extractDOCX(data, filename)
	case KindXLSX:
		return extractXLSX(data, filename)
	case KindPPTX:
		return extractPPTX(data, filename)
	case KindODT:
		return extractODT(data, filename)
	case KindODS:
		return extractODS(data, filename)
	case KindODP:
		return extractODP(data, filename)
	case KindCSV:
		return extractCSV(data, filename)
	case KindRTF:
		return extractRTF(data, filename)
	case KindEPUB:
		return extractEPUB(data, filename)
	default:
		return nil, &ErrUnsupported{Kind: kind}
	}
}

// FormatProjectionHeader monta o cabeçalho curto exigido pelo AEP-0093.
func FormatProjectionHeader(res *Result) string {
	var b strings.Builder
	b.WriteString("<!-- projeção Markdown (não é o arquivo original) -->\n")
	fmt.Fprintf(&b, "Origem: %s\n", res.Source)
	fmt.Fprintf(&b, "Formato: %s\n", res.Kind)
	if res.Pages > 0 {
		label := "Páginas"
		switch res.Kind {
		case KindPPTX, KindODP:
			label = "Slides"
		case KindXLSX, KindODS:
			label = "Abas"
		}
		fmt.Fprintf(&b, "%s: %d\n", label, res.Pages)
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(&b, "Aviso: %s\n", w)
	}
	b.WriteString("\n")
	return b.String()
}

// CheckWritable classifica data+filename e retorna ErrNotWritable se não for texto.
func CheckWritable(data []byte, filename string) error {
	kind := Detect(data, filename)
	if !IsWritableText(kind) {
		return &ErrNotWritable{Kind: kind}
	}
	return nil
}
