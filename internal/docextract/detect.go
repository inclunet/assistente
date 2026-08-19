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
	if isLikelyText(data) {
		ext := strings.ToLower(filepath.Ext(filename))
		if ext == ".csv" || looksLikeCSV(data) {
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
	if bytes.HasPrefix(data, []byte("PK\x03\x04")) || bytes.HasPrefix(data, []byte("PK\x05\x06")) {
		return detectZipKind(data)
	}
	return ""
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

func looksLikeCSV(data []byte) bool {
	sample := data
	if len(sample) > 4096 {
		sample = sample[:4096]
	}
	lines := bytes.Split(sample, []byte("\n"))
	if len(lines) < 2 {
		return false
	}
	comma := bytes.Count(lines[0], []byte(","))
	semi := bytes.Count(lines[0], []byte(";"))
	return comma >= 1 || semi >= 1
}
