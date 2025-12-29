package filemanager

import (
	"bytes"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// Encodings conhecidos
const (
	EncodingUTF8        = "utf-8"
	EncodingUTF16LE     = "utf-16le"
	EncodingUTF16BE     = "utf-16be"
	EncodingLatin1      = "iso-8859-1"
	EncodingWindows1252 = "windows-1252"
	EncodingASCII       = "ascii"
	EncodingUnknown     = "unknown"
)

// BOMs conhecidos
var (
	bomUTF8    = []byte{0xEF, 0xBB, 0xBF}
	bomUTF16LE = []byte{0xFF, 0xFE}
	bomUTF16BE = []byte{0xFE, 0xFF}
)

// DetectEncoding detecta o encoding de um conteúdo binário
func DetectEncoding(data []byte) string {
	if len(data) == 0 {
		return EncodingUTF8
	}

	// Verifica BOMs
	if bytes.HasPrefix(data, bomUTF8) {
		return EncodingUTF8
	}
	if bytes.HasPrefix(data, bomUTF16BE) {
		return EncodingUTF16BE
	}
	if bytes.HasPrefix(data, bomUTF16LE) {
		return EncodingUTF16LE
	}

	// Verifica se é UTF-8 válido
	if utf8.Valid(data) {
		// Verifica se tem caracteres fora de ASCII
		hasNonASCII := false
		for _, b := range data {
			if b > 127 {
				hasNonASCII = true
				break
			}
		}
		if hasNonASCII {
			return EncodingUTF8
		}
		return EncodingASCII
	}

	// Tenta detectar Latin1/Windows-1252
	hasControlChars := false
	for _, b := range data {
		if b >= 0x80 && b <= 0x9F {
			hasControlChars = true
			break
		}
	}

	if hasControlChars {
		return EncodingWindows1252
	}

	return EncodingLatin1
}

// DecodeToUTF8 converte bytes do encoding detectado para UTF-8
func DecodeToUTF8(data []byte, enc string) string {
	if enc == "" {
		enc = DetectEncoding(data)
	}

	// Remove BOM se presente
	data = RemoveBOM(data)

	var decoder *encoding.Decoder

	switch enc {
	case EncodingUTF8, EncodingASCII:
		return string(data)

	case EncodingUTF16LE:
		decoder = unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewDecoder()

	case EncodingUTF16BE:
		decoder = unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM).NewDecoder()

	case EncodingLatin1:
		decoder = charmap.ISO8859_1.NewDecoder()

	case EncodingWindows1252:
		decoder = charmap.Windows1252.NewDecoder()

	default:
		return string(data)
	}

	result, _, err := transform.Bytes(decoder, data)
	if err != nil {
		return string(data)
	}
	return string(result)
}

// EncodeFromUTF8 converte uma string UTF-8 para o encoding especificado
func EncodeFromUTF8(text string, enc string) ([]byte, error) {
	if enc == "" || enc == EncodingUTF8 || enc == EncodingASCII {
		return []byte(text), nil
	}

	var encoder *encoding.Encoder

	switch enc {
	case EncodingUTF16LE:
		encoder = unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder()

	case EncodingUTF16BE:
		encoder = unicode.UTF16(unicode.BigEndian, unicode.UseBOM).NewEncoder()

	case EncodingLatin1:
		encoder = charmap.ISO8859_1.NewEncoder()

	case EncodingWindows1252:
		encoder = charmap.Windows1252.NewEncoder()

	default:
		return []byte(text), nil
	}

	result, _, err := transform.Bytes(encoder, []byte(text))
	return result, err
}

// RemoveBOM remove o BOM de um slice de bytes
func RemoveBOM(data []byte) []byte {
	if bytes.HasPrefix(data, bomUTF8) {
		return data[3:]
	}
	if bytes.HasPrefix(data, bomUTF16BE) || bytes.HasPrefix(data, bomUTF16LE) {
		return data[2:]
	}
	return data
}

// HasBOM verifica se os dados começam com um BOM
func HasBOM(data []byte) bool {
	return bytes.HasPrefix(data, bomUTF8) ||
		bytes.HasPrefix(data, bomUTF16BE) ||
		bytes.HasPrefix(data, bomUTF16LE)
}

// GetBOMEncoding retorna o encoding baseado no BOM, se presente
func GetBOMEncoding(data []byte) string {
	if bytes.HasPrefix(data, bomUTF8) {
		return EncodingUTF8
	}
	if bytes.HasPrefix(data, bomUTF16BE) {
		return EncodingUTF16BE
	}
	if bytes.HasPrefix(data, bomUTF16LE) {
		return EncodingUTF16LE
	}
	return ""
}

