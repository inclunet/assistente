package terminal

import (
	"bytes"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// parsedTerminalText acumula os bytes normalizados. Somente a confirmação de
// um redesenho do ConPTY pode retirar a quebra de linha mais recente.
type parsedTerminalText struct {
	conPTY           bool
	marker           *CommandMarker
	columns          int
	data             []byte
	lastText         int
	row, previousRow int
	cr, rawCR, crlf  bool
	escape           []byte
	escapeAfterCRLF  bool
	replay           string
	replayed         int
}

func (t *parsedTerminalText) write(s string) {
	if end := len(strings.TrimRight(s, "\r\n")); end != 0 {
		t.lastText = len(t.data) + end
	}
	t.data = append(t.data, s...)
}

func (t *parsedTerminalText) feed(chunk string) {
	for chunk != "" {
		if t.replay != "" {
			want := t.replay[t.replayed:]
			n := min(len(want), len(chunk))
			if want[:n] != chunk[:n] {
				t.write(t.replay[:t.replayed])
				t.replay, t.replayed = "", 0
				continue
			}
			chunk = chunk[n:]
			t.replayed += n
			if t.replayed == len(t.replay) {
				// O ConPTY pode redesenhar a última célula de um marcador.
				// Nesse caso descarte a célula repetida e mantenha o delimitador.
				beforeBreak := t.data[:len(t.data)-1]
				if !t.marker.endsProtocolLine(beforeBreak) {
					t.data = beforeBreak
				}
				t.row = len(t.data)
				t.replay, t.replayed = "", 0
			}
			continue
		}
		if len(t.escape) != 0 {
			if t.escapeByte(chunk[0]) {
				chunk = chunk[1:]
			}
			continue
		}
		if t.cr && chunk[0] != '\n' && chunk[0] != '\x1b' {
			t.write("\r")
			t.cr = false
		}
		switch chunk[0] {
		case '\r':
			t.cr, t.rawCR = true, true
			t.crlf = false
			chunk = chunk[1:]
		case '\n':
			t.previousRow = t.row
			t.data = append(t.data, '\n')
			t.row = len(t.data)
			t.crlf, t.cr = t.rawCR, false
			t.rawCR = false
			chunk = chunk[1:]
		case '\x1b':
			t.rawCR = false
			t.escape = append(t.escape, '\x1b')
			t.escapeAfterCRLF, t.crlf = t.crlf, false
			chunk = chunk[1:]
		default:
			t.crlf, t.rawCR = false, false
			n := strings.IndexAny(chunk, "\r\n\x1b")
			if n < 0 {
				n = len(chunk)
			}
			t.write(chunk[:n])
			chunk = chunk[n:]
		}
	}
}

// CSI e OSC podem atravessar leituras. Ao encontrar um escape inválido,
// preserve o texto e reexamine o byte que encerrou a tentativa.
func (t *parsedTerminalText) escapeByte(ch byte) bool {
	switch {
	case len(t.escape) == 1 && (ch == '[' || ch == ']' || ch == 'P' || ch == 'X' || ch == '^' || ch == '_'):
		t.escape = append(t.escape, ch)
		return true
	case len(t.escape) == 1 || (t.escape[1] >= 0x20 && t.escape[1] <= 0x2f):
		if ch >= 0x20 && ch <= 0x2f {
			t.escape = append(t.escape, ch)
			return true
		}
		if ch >= 0x30 && ch <= 0x7e {
			t.escape = t.escape[:0]
			return true
		}
	case t.escape[1] != '[':
		if (t.escape[1] == ']' && ch == '\a') || (ch == '\\' && t.escape[len(t.escape)-1] == '\x1b') {
			t.escape = t.escape[:0]
			return true
		}
		if ch != '\n' {
			t.escape = append(t.escape, ch)
			return true
		}
	// CSI aceita parâmetros (0x30–0x3f), depois intermediários (0x20–0x2f).
	case (ch >= 0x20 && ch <= 0x2f) || (ch >= 0x30 && ch <= 0x3f && t.escape[len(t.escape)-1] >= 0x30):
		t.escape = append(t.escape, ch)
		return true
	case ch >= 0x40 && ch <= 0x7e:
		if t.conPTY {
			switch ch {
			case 't':
				t.columns = viewportAfterOutput(append(t.escape, ch), t.columns)
			case 'H':
				if t.escapeAfterCRLF {
					t.beginReplay()
				}
			}
		}
		t.escape = t.escape[:0]
		return true
	}
	t.flushEscape()
	return false
}

func (t *parsedTerminalText) flushEscape() {
	if len(t.escape) != 0 {
		if t.cr {
			t.write("\r")
			t.cr = false
		}
		literal := StripANSI(string(t.escape))
		if strings.HasSuffix(literal, "\r") {
			literal = literal[:len(literal)-1]
			t.cr, t.rawCR = true, true
		}
		t.write(literal)
		t.escape = t.escape[:0]
	}
}

func (t *parsedTerminalText) beginReplay() {
	row, col, ok := bytes.Cut(t.escape[2:], []byte(";"))
	if !ok || len(row) == 0 || len(col) == 0 || col[0] == '0' {
		return
	}
	for _, ch := range row {
		if ch < '0' || ch > '9' {
			return
		}
	}
	column, err := strconv.Atoi(string(col))
	if err != nil || column != t.columns || column <= 0 {
		return
	}
	cells, lastWidth := 0, 0
	var last []byte
	rest := t.data[t.previousRow : len(t.data)-1]
	state := -1
	for len(rest) > 0 {
		last, rest, lastWidth, state = uniseg.FirstGraphemeCluster(rest, state)
		cells += lastWidth
		if cells > column {
			return
		}
	}
	if cells == column && lastWidth > 0 {
		t.replay = string(last)
	}
}

func (m *CommandMarker) endsProtocolLine(data []byte) bool {
	if bytes.HasSuffix(data, []byte(m.startTag)) {
		return true
	}
	// Um status escrito pelo shell ocupa até 20 bytes decimais com o sinal.
	// Limite a busca à moldura, inclusive em uma linha muito longa sem quebras.
	tail := data[max(0, len(data)-len(m.endTag)-20):]
	at := bytes.LastIndex(tail, []byte(m.endTag))
	if at < 0 {
		return false
	}
	_, err := strconv.Atoi(string(tail[at+len(m.endTag):]))
	return err == nil
}

func (t *parsedTerminalText) finish() {
	t.flushEscape()
	if t.cr {
		t.write("\r")
		t.cr = false
	}
	if t.replayed > 0 {
		t.write(t.replay[:t.replayed])
	}
	t.replay, t.replayed = "", 0
}

// Os índices avançam sobre os bytes novos. A busca recua no máximo o tamanho
// de uma tag para reconhecer um marcador partido entre duas leituras.
type markerStream struct {
	marker                        *CommandMarker
	text                          parsedTerminalText
	scanned, start, sent          int
	started                       bool
	candidate, cutoff, statusScan int
}

func newMarkerStream(marker *CommandMarker) *markerStream {
	return &markerStream{marker: marker, text: parsedTerminalText{conPTY: marker.conPTY, columns: marker.viewportColumns, marker: marker}}
}

func (s *markerStream) append(chunk string) ([]byte, ParseResult) {
	before := len(s.text.data)
	var last byte
	if before > 0 {
		last = s.text.data[before-1]
	}
	s.text.feed(chunk)
	// Um controle pendente pode consumir vários blocos sem alterar o texto.
	// O único recuo possível substitui ou remove a última quebra de linha.
	if before == len(s.text.data) && (before == 0 || last == s.text.data[before-1]) {
		return nil, ParseResult{}
	}
	return s.progress()
}

func (s *markerStream) progress() (delta []byte, result ParseResult) {
	data := s.text.data
	previous := s.scanned
	s.scanned = len(data)
	if !s.started {
		needle := []byte(s.marker.startTag + "\n")
		from := max(0, previous-len(needle))
		for from < len(data) {
			n := bytes.Index(data[from:], needle)
			if n < 0 {
				return
			}
			at := from + n
			if s.marker.conPTY || at == 0 || data[at-1] == '\n' {
				s.start = at + len(needle)
				s.sent = s.start
				s.started = true
				break
			}
			from = at + 1
		}
		if !s.started {
			return
		}
	}

	end := []byte(s.marker.endTag)
	from := max(s.start, previous-len(end))
	for from < len(data) {
		n := bytes.Index(data[from:], end)
		if n < 0 {
			break
		}
		at := from + n
		if s.marker.inlineEnd || at == s.start || data[at-1] == '\n' {
			after := at + len(end)
			if s.candidate != after {
				s.candidate = after
				s.statusScan = after
				s.cutoff = s.start + len(bytes.TrimRight(data[s.start:at], "\r\n"))
			}
		}
		from = at + len(end)
	}
	visible := s.text.lastText
	if s.candidate != 0 {
		if s.statusScan >= 0 {
			s.statusScan = min(s.statusScan, len(data))
			n := bytes.IndexByte(data[s.statusScan:], '\n')
			if n >= 0 {
				line := data[s.candidate : s.statusScan+n]
				s.statusScan = -1
				if code, err := strconv.Atoi(string(bytes.TrimSpace(line))); err == nil {
					result.Found = true
					result.ExitCode = code
					result.Output = string(bytes.TrimRight(data[s.start:s.candidate-len(end)], "\n"))
				} else {
					s.candidate = 0 // A linha inteira pertence à saída do comando.
				}
			} else {
				s.statusScan = len(data)
			}
		}
	}
	if s.candidate != 0 {
		visible = s.cutoff
	} else {
		// Retenha um prefixo possível do marcador e as quebras que o precedem.
		// Os demais bytes ficam disponíveis ao callback nesta leitura.
		for n := min(len(data)-s.start, len(end)-1); n > 0; n-- {
			if bytes.HasSuffix(data, end[:n]) {
				visible = max(s.sent, s.start+len(bytes.TrimRight(data[s.start:len(data)-n], "\r\n")))
				break
			}
		}
	}
	if result.Found {
		visible = s.start + len(result.Output)
	} else if visible > s.sent && data[visible-1] >= utf8.RuneSelf {
		first := visible - 1
		for first > max(s.sent, visible-utf8.UTFMax) && !utf8.RuneStart(data[first]) {
			first--
		}
		if !utf8.FullRune(data[first:visible]) {
			visible = first
		}
	}
	if visible > s.sent {
		delta = data[s.sent:visible]
		s.sent = visible
	}
	return
}
