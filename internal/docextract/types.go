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

// Result é a projeção Markdown derivada de um documento.
type Result struct {
	Kind     Kind
	Markdown string
	Pages    int      // páginas/slides/abas quando conhecido; 0 se N/A
	Warnings []string // avisos de extração parcial / PDF sem texto
	Source   string   // path original (se conhecido)
}

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
	if IsDocument(e.Kind) {
		return fmt.Sprintf(
			"escrita não suportada no formato %s — use read_file para obter a projeção Markdown; write_file/edit_file só aceitam arquivos de texto",
			e.Kind,
		)
	}
	return fmt.Sprintf(
		"escrita não suportada: conteúdo binário (%s) sem leitura convertida disponível; write_file/edit_file só aceitam arquivos de texto",
		e.Kind,
	)
}

// IsDocument retorna true para formatos que a Fase 1 projeta (não texto nativo).
func IsDocument(k Kind) bool {
	switch k {
	case KindPDF, KindDOCX, KindXLSX, KindPPTX, KindODT, KindODS, KindODP, KindCSV, KindRTF, KindEPUB:
		return true
	default:
		return false
	}
}

// IsWritableText retorna true se o kind pode ser gravado pelas tools de texto.
// CSV e RTF são projetados na leitura, mas o conteúdo no disco permanece texto
// editável; binários de documento (PDF/OOXML/ODF/EPUB) não.
func IsWritableText(k Kind) bool {
	switch k {
	case KindText, KindCSV, KindRTF:
		return true
	default:
		return false
	}
}
