package docextract

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strings"
)

const (
	zipMaxEntries       = 4096
	zipMaxTotalUncomp   = 64 << 20 // 64 MiB expandido
	zipMaxSingleUncomp  = 32 << 20 // 32 MiB por entrada
	zipMaxCompressionRatio = 100   // uncomp/comp
)

type zipLimits struct {
	entries int
	total   int64
}

func openZip(data []byte) (*zip.Reader, error) {
	// O diretório central é lido antes de qualquer entrada, e nele um arquivo
	// pequeno pode declarar centenas de milhares de entradas. Conferir a contagem
	// no rodapé custa alguns bytes e evita que zip.NewReader materialize essa
	// lista inteira só para a recusa vir depois (D11).
	if n, ok := zipDeclaredEntries(data); ok && n > zipMaxEntries {
		return nil, fmt.Errorf("ZIP com demasiadas entradas (%d)", n)
	}
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("arquivo ZIP inválido: %w", err)
	}
	if len(r.File) > zipMaxEntries {
		return nil, fmt.Errorf("ZIP com demasiadas entradas (%d)", len(r.File))
	}
	return r, nil
}

// Tamanho máximo do rodapé de um ZIP: o registro de fim de diretório central tem
// 22 bytes fixos mais um comentário de até 64 KiB.
const zipFooterScanBytes = 22 + 0xFFFF

// zipDeclaredEntries lê a contagem de entradas declarada no rodapé do ZIP, sem
// abrir o arquivo. ok=false quando o rodapé não é encontrado ou é ambíguo — aí a
// conferência fica por conta de zip.NewReader, como antes.
func zipDeclaredEntries(data []byte) (count int64, ok bool) {
	eocd := lastIndexFrom(data, []byte("PK\x05\x06"), zipFooterScanBytes)
	if eocd < 0 || len(data) < eocd+22 {
		return 0, false
	}
	total := int64(binary.LittleEndian.Uint16(data[eocd+10:]))
	if total != 0xFFFF {
		return total, true
	}
	// 0xFFFF é o marcador de ZIP64: a contagem real está no rodapé estendido.
	z64 := lastIndexFrom(data, []byte("PK\x06\x06"), zipFooterScanBytes+56)
	if z64 < 0 || len(data) < z64+40 {
		return 0, false
	}
	total64 := binary.LittleEndian.Uint64(data[z64+32:])
	if total64 > math.MaxInt64 {
		return 0, false
	}
	return int64(total64), true
}

// lastIndexFrom procura sig apenas na cauda de data, de tamanho window.
func lastIndexFrom(data, sig []byte, window int) int {
	start := 0
	if len(data) > window {
		start = len(data) - window
	}
	idx := bytes.LastIndex(data[start:], sig)
	if idx < 0 {
		return -1
	}
	return start + idx
}

func readZipFile(f *zip.File, lim *zipLimits) ([]byte, error) {
	if f.UncompressedSize64 > zipMaxSingleUncomp {
		return nil, fmt.Errorf("entrada ZIP %q excede tamanho máximo", f.Name)
	}
	if f.CompressedSize64 > 0 {
		ratio := float64(f.UncompressedSize64) / float64(f.CompressedSize64)
		if ratio > zipMaxCompressionRatio && f.UncompressedSize64 > 1<<20 {
			return nil, fmt.Errorf("entrada ZIP %q com razão de compressão suspeita", f.Name)
		}
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()

	lim.entries++
	if lim.entries > zipMaxEntries {
		return nil, fmt.Errorf("limite de entradas ZIP excedido")
	}
	limited := io.LimitReader(rc, zipMaxSingleUncomp+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > zipMaxSingleUncomp {
		return nil, fmt.Errorf("entrada ZIP %q excede tamanho máximo", f.Name)
	}
	lim.total += int64(len(data))
	if lim.total > zipMaxTotalUncomp {
		return nil, fmt.Errorf("ZIP expandido excede limite de %d bytes", zipMaxTotalUncomp)
	}
	return data, nil
}

func findZipName(r *zip.Reader, name string) *zip.File {
	name = strings.ReplaceAll(name, "\\", "/")
	for _, f := range r.File {
		if strings.ReplaceAll(f.Name, "\\", "/") == name {
			return f
		}
	}
	return nil
}

func hasZipPrefix(r *zip.Reader, prefix string) bool {
	prefix = strings.ReplaceAll(prefix, "\\", "/")
	for _, f := range r.File {
		n := strings.ReplaceAll(f.Name, "\\", "/")
		if strings.HasPrefix(n, prefix) {
			return true
		}
	}
	return false
}

func detectZipKind(data []byte) Kind {
	r, err := openZip(data)
	if err != nil {
		return KindUnsupportedBinary
	}
	// EPUB: mimetype no início (sem compressão) ou META-INF/container.xml
	if f := findZipName(r, "mimetype"); f != nil {
		lim := &zipLimits{}
		body, err := readZipFile(f, lim)
		if err == nil && strings.Contains(string(body), "application/epub+zip") {
			return KindEPUB
		}
	}
	if findZipName(r, "META-INF/container.xml") != nil && hasZipPrefix(r, "OEBPS/") {
		return KindEPUB
	}
	if findZipName(r, "word/document.xml") != nil {
		return KindDOCX
	}
	if findZipName(r, "xl/workbook.xml") != nil {
		return KindXLSX
	}
	if findZipName(r, "ppt/presentation.xml") != nil {
		return KindPPTX
	}
	if f := findZipName(r, "mimetype"); f != nil {
		lim := &zipLimits{}
		body, err := readZipFile(f, lim)
		if err == nil {
			mt := string(body)
			switch {
			case strings.Contains(mt, "opendocument.text"):
				return KindODT
			case strings.Contains(mt, "opendocument.spreadsheet"):
				return KindODS
			case strings.Contains(mt, "opendocument.presentation"):
				return KindODP
			}
		}
	}
	if findZipName(r, "content.xml") != nil {
		// ODF genérico sem mimetype: tenta heurística fraca
		if hasZipPrefix(r, "Thumbnails/") || hasZipPrefix(r, "Configurations2/") {
			return KindODT
		}
	}
	return KindUnsupportedBinary
}
