// Package docextract detecta formatos de documento e projeta conteúdo para Markdown
// (AEP-0093). A projeção não é o arquivo original; escrita continua só em texto.
package docextract

import "fmt"

// Kind classifica o conteúdo detectado.
type Kind string

const (
	KindText              Kind = "text"
	KindPDF               Kind = "pdf"
	KindDOCX              Kind = "docx"
	KindXLSX              Kind = "xlsx"
	KindPPTX              Kind = "pptx"
	KindODT               Kind = "odt"
	KindODS               Kind = "ods"
	KindODP               Kind = "odp"
	KindCSV               Kind = "csv"
	KindRTF               Kind = "rtf"
	KindEPUB              Kind = "epub"
	KindUnsupportedBinary Kind = "unsupported_binary"
)

// Result é o conteúdo devolvido pelo extrator: a projeção Markdown de um
// documento ou o próprio texto do arquivo, conforme o modo (D12).
type Result struct {
	Kind      Kind
	Markdown  string
	Pages     int      // páginas/slides/abas quando conhecido; 0 se N/A
	Warnings  []string // avisos de extração parcial / PDF sem texto
	Source    string   // path original (se conhecido)
	Projected bool     // true quando o conteúdo é derivado, não o do arquivo
}

// Mode escolhe quando o conteúdo é convertido para Markdown.
type Mode string

const (
	// ModeAuto converte só o que o modelo não consegue ler cru: PDF, OOXML,
	// ODF e EPUB. CSV, RTF, código e demais textos voltam como estão no disco.
	ModeAuto Mode = "auto"
	// ModeMarkdown pede a projeção também para os formatos que já são texto,
	// como a tabela Markdown de um CSV.
	ModeMarkdown Mode = "markdown"
)

// ErrUnsupported indica formato não suportado para leitura convertida.
type ErrUnsupported struct {
	Kind Kind
	Msg  string
}

func (e *ErrUnsupported) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return fmt.Sprintf("formato %s não suportado para leitura convertida", e.Kind)
}

// ErrNotWritable indica que o formato não admite escrita pelas tools de texto.
type ErrNotWritable struct {
	Kind Kind
}

func (e *ErrNotWritable) Error() string {
	if IsOpaqueDocument(e.Kind) {
		return fmt.Sprintf(
			"escrita não suportada no formato %s — use read_file para obter a projeção Markdown; write_file/edit_file/text_edit só aceitam arquivos de texto",
			e.Kind,
		)
	}
	return fmt.Sprintf(
		"escrita não suportada: conteúdo binário (%s) sem leitura convertida disponível; write_file/edit_file/text_edit só aceitam arquivos de texto",
		e.Kind,
	)
}

// ErrUnsupportedBinary é a recusa de leitura de binário sem projeção, com a
// mesma mensagem venha ela da extração ou da classificação por prefixo.
func ErrUnsupportedBinary() error {
	return &ErrUnsupported{
		Kind: KindUnsupportedBinary,
		Msg:  "formato binário não suportado para leitura convertida — use um formato V1 (PDF textual, OOXML, ODF, CSV, RTF, EPUB) ou arquivo de texto",
	}
}

// ErrTooLargeToExtract descreve a recusa por tamanho, com a mesma mensagem
// tanto para quem já leu o arquivo quanto para quem só olhou o tamanho no disco.
func ErrTooLargeToExtract(size int64) error {
	return fmt.Errorf("arquivo muito grande para extração (%d bytes; máximo %d)", size, MaxExtractBytes)
}

// IsOpaqueDocument retorna true para os formatos que o modelo não consegue ler
// no original: o conteúdo é binário (ou um container ZIP de XML), então a
// projeção Markdown é a única leitura útil e vale por padrão.
func IsOpaqueDocument(k Kind) bool {
	switch k {
	case KindPDF, KindDOCX, KindXLSX, KindPPTX, KindODT, KindODS, KindODP, KindEPUB:
		return true
	default:
		return false
	}
}

// IsDocument retorna true para todo formato com extrator, incluindo os que já
// são texto no disco (CSV, RTF) e só viram Markdown sob demanda.
func IsDocument(k Kind) bool {
	return IsOpaqueDocument(k) || k == KindCSV || k == KindRTF
}

// IsWritableText retorna true se o kind pode ser gravado pelas tools de texto.
// É o mesmo conjunto que a leitura devolve cru por padrão: o modelo lê e edita
// exatamente o mesmo conteúdo, sem projeção no meio (D12).
func IsWritableText(k Kind) bool {
	switch k {
	case KindText, KindCSV, KindRTF:
		return true
	default:
		return false
	}
}
