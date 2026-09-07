package docextract

import (
	"bytes"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Detect classifica bytes pelo conteúdo (magic) com fallback de extensão.
// Prioridade: magic → texto UTF-8 → extensão → unsupported_binary.
func Detect(data []byte, filename string) Kind {
	if k := detectMagic(data); k != "" {
		return k
	}
	// Container ZIP nunca é texto, mesmo com extensão inocente: chamadas que só
	// veem o prefixo não conseguem ler a estrutura interna, e classificar como
	// texto abriria caminho para gravar/streamar um documento renomeado.
	if hasZipMagic(data) {
		if k := detectByExt(filename); isZipDocument(k) {
			return k
		}
		return KindUnsupportedBinary
	}
	if isLikelyText(data) {
		ext := strings.ToLower(filepath.Ext(filename))
		if ext == ".csv" {
			return KindCSV
		}
		if ext == ".rtf" || bytes.HasPrefix(bytes.TrimSpace(data), []byte(`{\rtf`)) {
			return KindRTF
		}
		return KindText
	}
	if k := detectByExt(filename); k != "" {
		return k
	}
	return KindUnsupportedBinary
}

func isZipDocument(kind Kind) bool {
	switch kind {
	case KindDOCX, KindXLSX, KindPPTX, KindODT, KindODS, KindODP, KindEPUB:
		return true
	default:
		return false
	}
}

func detectMagic(data []byte) Kind {
	if len(data) < 4 {
		return ""
	}
	// PDF
	if bytes.HasPrefix(data, []byte("%PDF")) {
		return KindPDF
	}
	// RTF
	trim := bytes.TrimLeft(data, " \t\r\n")
	if bytes.HasPrefix(trim, []byte(`{\rtf`)) {
		return KindRTF
	}
	// ZIP-based (OOXML / ODF / EPUB)
	if hasZipMagic(data) {
		if k := detectZipKind(data); k != "" && k != KindUnsupportedBinary {
			return k
		}
		// ZIP inválido ou não reconhecido: deixa Detect cair no fallback por extensão (D4).
		return ""
	}
	return ""
}

func hasZipMagic(data []byte) bool {
	return bytes.HasPrefix(data, []byte("PK\x03\x04")) || bytes.HasPrefix(data, []byte("PK\x05\x06"))
}

// HasZipMagic informa se o prefixo declara um container ZIP. É útil para
// chamadores que leem só o começo do arquivo: eles podem decidir carregar o
// container completo para distinguir OOXML/ODF/EPUB de um ZIP comum.
func HasZipMagic(data []byte) bool {
	return hasZipMagic(data)
}

func detectByExt(filename string) Kind {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".pdf":
		return KindPDF
	case ".docx":
		return KindDOCX
	case ".xlsx":
		return KindXLSX
	case ".pptx":
		return KindPPTX
	case ".odt":
		return KindODT
	case ".ods":
		return KindODS
	case ".odp":
		return KindODP
	case ".csv":
		return KindCSV
	case ".rtf":
		return KindRTF
	case ".epub":
		return KindEPUB
	default:
		return ""
	}
}

// isLikelyText aceita UTF-8 sem NUL e com baixa proporção de bytes de controle.
func isLikelyText(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return false
	}
	sample := data
	if len(sample) > 8192 {
		sample = sample[:8192]
	}
	if !utf8.Valid(sample) {
		return false
	}
	control := 0
	for _, b := range sample {
		if b < 0x09 || (b > 0x0d && b < 0x20) {
			control++
		}
	}
	return control*100/len(sample) < 5
}

// IsLikelyText expõe a mesma heurística usada pela classificação para
// consumidores que leem apenas um prefixo e precisam confirmar que um formato
// textual por extensão (CSV/RTF) não contém bytes binários.
func IsLikelyText(data []byte) bool {
	return isLikelyText(data)
}
