package docextract

import (
	"fmt"
	"strings"
)

// MaxExtractBytes é o teto de entrada para extração de documento. Não vale para
// texto/código, que continua paginável por linhas.
const MaxExtractBytes = 32 << 20 // 32 MiB

// DetectPrefixBytes é quanto basta ler do início do arquivo para classificar sem
// carregar o conteúdo inteiro.
const DetectPrefixBytes = 8 << 10 // 8 KiB

// ExtractMode detecta o formato e decide, pelo modo, se converte para Markdown.
//
// Em ModeAuto só o formato opaco é convertido. Arquivo que já é texto no disco
// (CSV, RTF, código, marcação) volta como está: converter por padrão tiraria do
// modelo justamente o conteúdo que ele precisa ver para revisar ou editar, e
// ainda quebraria a simetria com as tools de escrita, que gravam esse mesmo
// texto. ModeMarkdown pede a projeção também para esses formatos (D12).
func ExtractMode(data []byte, filename string, mode Mode) (*Result, error) {
	kind := Detect(data, filename)
	if mode != ModeMarkdown && IsWritableText(kind) && isLikelyText(data) {
		return &Result{Kind: kind, Markdown: string(data), Source: filename}, nil
	}
	res, err := Extract(data, filename)
	if err != nil {
		return nil, err
	}
	res.Projected = IsDocument(res.Kind)
	return res, nil
}

// Extract projeta para Markdown todo formato com extrator. Para KindText devolve
// o conteúdo bruto (sem cabeçalho de projeção — quem chama decide).
func Extract(data []byte, filename string) (*Result, error) {
	kind := Detect(data, filename)
	res := &Result{Kind: kind, Source: filename}

	// O limite vale só para extração; texto/código segue como antes (D8: sem
	// hard-deny artificial em arquivo grande que o chamador pagina por linhas).
	if kind != KindText && len(data) > MaxExtractBytes {
		return nil, ErrTooLargeToExtract(int64(len(data)))
	}

	switch kind {
	case KindText:
		res.Markdown = string(data)
		return res, nil
	case KindUnsupportedBinary:
		return nil, ErrUnsupportedBinary()
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
//
// A extensão sozinha não autoriza a escrita: `.csv` e `.rtf` só chegam a KindCSV
// e KindRTF pela extensão, inclusive quando o conteúdo é binário, e aceitar isso
// deixaria gravar bytes arbitrários num caminho de aparência textual (D3/D10).
func CheckWritable(data []byte, filename string) error {
	kind := Detect(data, filename)
	if !IsWritableText(kind) {
		return &ErrNotWritable{Kind: kind}
	}
	if !isLikelyText(data) {
		return &ErrNotWritable{Kind: kind, BinaryContent: true}
	}
	return nil
}

// CheckWritableString evita copiar o conteúdo inteiro só para classificá-lo: a
// detecção olha o começo do arquivo, então o prefixo basta.
func CheckWritableString(content, filename string) error {
	if len(content) > DetectPrefixBytes {
		content = content[:DetectPrefixBytes]
	}
	return CheckWritable([]byte(content), filename)
}
