package docextract

import (
	"context"
	"fmt"
	"strings"
)

// MaxExtractBytes é o teto de entrada para extração de documento. Não vale para
// texto/código, que continua paginável por linhas.
const MaxExtractBytes = 32 << 20 // 32 MiB

// MaxExtractPages evita que um PDF pequeno, mas com milhares de páginas,
// monopolize uma busca. O limite é da projeção leve; OCR terá contrato próprio.
const MaxExtractPages = 1000

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
	return ExtractModeContext(context.Background(), data, filename, mode)
}

// ExtractModeContext é a variante cancelável usada pelas tools.
func ExtractModeContext(ctx context.Context, data []byte, filename string, mode Mode) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	kind := Detect(data, filename)
	if mode != ModeMarkdown && IsWritableText(kind) && isLikelyText(data) {
		return &Result{Kind: kind, Markdown: string(data), Source: filename}, nil
	}
	res, err := ExtractContext(ctx, data, filename)
	if err != nil {
		return nil, err
	}
	res.Projected = IsDocument(res.Kind)
	return res, nil
}

// Extract projeta para Markdown todo formato com extrator. Para KindText devolve
// o conteúdo bruto (sem cabeçalho de projeção — quem chama decide).
func Extract(data []byte, filename string) (*Result, error) {
	return ExtractContext(context.Background(), data, filename)
}

// ExtractContext projeta para Markdown e verifica cancelamento antes, durante
// as páginas de PDF e ao terminar os demais extratores limitados por tamanho.
func ExtractContext(ctx context.Context, data []byte, filename string) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	kind := Detect(data, filename)
	res := &Result{Kind: kind, Source: filename}

	// O limite vale só para extração; texto/código segue como antes (D8: sem
	// hard-deny artificial em arquivo grande que o chamador pagina por linhas).
	if kind != KindText && len(data) > MaxExtractBytes {
		return nil, ErrTooLargeToExtract(int64(len(data)))
	}

	var extracted *Result
	var err error
	switch kind {
	case KindText:
		res.Markdown = string(data)
		return res, nil
	case KindUnsupportedBinary:
		return nil, ErrUnsupportedBinary()
	case KindPDF:
		extracted, err = extractPDF(ctx, data, filename)
	case KindDOCX:
		extracted, err = extractDOCX(data, filename)
	case KindXLSX:
		extracted, err = extractXLSX(data, filename)
	case KindPPTX:
		extracted, err = extractPPTX(data, filename)
	case KindODT:
		extracted, err = extractODT(data, filename)
	case KindODS:
		extracted, err = extractODS(data, filename)
	case KindODP:
		extracted, err = extractODP(data, filename)
	case KindCSV:
		extracted, err = extractCSV(data, filename)
	case KindRTF:
		extracted, err = extractRTF(data, filename)
	case KindEPUB:
		extracted, err = extractEPUB(data, filename)
	default:
		return nil, &ErrUnsupported{Kind: kind}
	}
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return extracted, nil
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
// detecção de formato olha o começo do arquivo, então o prefixo basta.
//
// A regra do byte NUL, porém, é sobre o conteúdo todo — é ela que separa texto
// de binário em CheckWritable — e continua valendo aqui: strings.IndexByte
// percorre a string sem copiá-la, então a verificação completa não custa o pico
// de memória de converter tudo para []byte.
func CheckWritableString(content, filename string) error {
	prefix := content
	if len(prefix) > DetectPrefixBytes {
		prefix = prefix[:DetectPrefixBytes]
	}
	prefixBytes := []byte(prefix)
	if err := CheckWritable(prefixBytes, filename); err != nil {
		return err
	}
	if strings.IndexByte(content, 0) >= 0 {
		return &ErrNotWritable{Kind: Detect(prefixBytes, filename), BinaryContent: true}
	}
	return nil
}
