package terminal

import (
	"bytes"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// Um controle incompleto curto pode ser devolvido literalmente no EOF. Depois
// desse limite ainda reconhecemos ST, BEL e cancelamento pelo estado do parser.
const maxControlLiteral = 4096

// parsedTerminalText acumula os bytes normalizados. Somente a confirmação de
// um redesenho do ConPTY pode retirar a quebra de linha mais recente.
type parsedTerminalText struct {
	conPTY                  bool
	marker                  *CommandMarker
	columns                 int
	data                    []byte
	lastText                int
	row, previousRow        int
	cr, rawCR, crlf         bool
	control                 terminalControl
	escape                  []byte
	escapeAfterCRLF         bool
	escapeFromPreviousOwner bool
	replay                  string
	replayed                int
	// Depois do rodapé, somente o CRLF e o cursor da margem podem repetir o terminador.
	footerColumn   int
	skipFooterCell bool
}

func (t *parsedTerminalText) write(s string) {
	if end := len(strings.TrimRight(s, "\r\n")); end != 0 {
		t.lastText = len(t.data) + end
	}
	t.data = append(t.data, s...)
}

func (t *parsedTerminalText) feed(chunk string) {
	for chunk != "" {
		if t.skipFooterCell {
			t.skipFooterCell = false
			if chunk[0] == markerStatusTerminator[0] {
				chunk = chunk[1:]
				continue
			}
		}
		if t.footerColumn > 0 && t.control.kind == 0 {
			expected := byte('\r')
			if t.rawCR {
				expected = '\n'
			} else if t.crlf {
				expected = '\x1b'
			}
			if chunk[0] != expected {
				t.footerColumn = 0
			}
		}
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
				// O ConPTY pode redesenhar a última célula do marcador de início.
				// Nesse caso descarte a célula repetida e mantenha o delimitador.
				beforeBreak := t.data[:len(t.data)-1]
				if !bytes.HasSuffix(beforeBreak, []byte(t.marker.startTag)) {
					t.data = beforeBreak
				}
				t.row = len(t.data)
				t.replay, t.replayed = "", 0
			}
			continue
		}
		if t.control.kind != 0 {
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
		case '\x18':
			// CAN fora de um escape não ocupa uma célula no terminal.
			t.crlf, t.rawCR = false, false
			chunk = chunk[1:]
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
			t.control.start()
			t.escapeFromPreviousOwner = false
			t.escape = append(t.escape, '\x1b')
			t.escapeAfterCRLF, t.crlf = t.crlf, false
			chunk = chunk[1:]
		default:
			t.crlf, t.rawCR = false, false
			n := strings.IndexAny(chunk, "\r\n\x1b\x18")
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
	kind := t.control.kind
	switch t.control.consume(ch, &t.columns) {
	case escapeMore:
		// Um escape herdado começou antes do comando e só precisa ser consumido.
		if len(t.escape) == maxControlLiteral {
			t.escape = nil
		} else if len(t.escape) != 0 {
			t.escape = append(t.escape, ch)
		}
	case escapeEnd:
		if kind == '[' && ch == 'H' && t.escapeAfterCRLF && len(t.escape) >= 2 {
			column := cursorColumn(t.escape[2:])
			if t.footerColumn > 0 {
				t.skipFooterCell = column == t.footerColumn
			} else if t.conPTY && column > 0 && column == t.columns {
				t.replay = string(lastCellAt(t.data[t.previousRow:len(t.data)-1], column))
			}
		}
		t.footerColumn = 0
		t.escape = t.escape[:0]
	case escapeInvalid:
		t.footerColumn = 0
		t.flushEscape()
		return false
	}
	return true
}

func (t *parsedTerminalText) flushEscape() {
	if t.escapeFromPreviousOwner {
		t.escape = t.escape[:0]
		return
	}
	if len(t.escape) != 0 {
		if t.cr {
			t.write("\r")
			t.cr = false
		}
		// Um DCS incompleto pode conter várias linhas. Normalize suas quebras
		// como as do texto que chegou fora do controle.
		literal := strings.ReplaceAll(stripANSI(string(t.escape)), "\r\n", "\n")
		if strings.HasSuffix(literal, "\r") {
			literal = literal[:len(literal)-1]
			t.cr, t.rawCR = true, true
		}
		t.write(literal)
		t.escape = t.escape[:0]
	}
}

func cursorColumn(position []byte) int {
	row, col, ok := bytes.Cut(position, []byte(";"))
	if !ok || len(row) == 0 || len(col) == 0 || col[0] == '0' {
		return 0
	}
	for _, ch := range row {
		if ch < '0' || ch > '9' {
			return 0
		}
	}
	column, err := strconv.Atoi(string(col))
	if err != nil {
		return 0
	}
	return column
}

func lastCellAt(rest []byte, column int) []byte {
	cells, lastWidth := 0, 0
	var last []byte
	state := -1
	for len(rest) > 0 {
		last, rest, lastWidth, state = uniseg.FirstGraphemeCluster(rest, state)
		cells += lastWidth
		if cells > column {
			return nil
		}
	}
	if cells == column && lastWidth > 0 {
		return last
	}
	return nil
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
	return &markerStream{marker: marker, text: parsedTerminalText{conPTY: marker.conPTY, columns: marker.viewportColumns, marker: marker, control: marker.control}}
}

func (s *markerStream) append(chunk string) ([]byte, parseResult) {
	// Pare no byte físico que confirma o rodapé. O restante ainda tem os
	// controles e bytes UTF-8 originais e pertence ao fluxo manual da sessão.
	first := -1
	var result parseResult
	for chunk != "" {
		part := chunk
		if at := strings.IndexByte(chunk, markerStatusTerminator[0]); at >= 0 {
			part = chunk[:at+1]
		}
		chunk = chunk[len(part):]
		var delta []byte
		delta, result = s.appendPart(part, chunk != "")
		if first < 0 && len(delta) > 0 {
			first = s.sent - len(delta)
		}
		if result.Found {
			result.afterFooter = chunk
			if s.text.conPTY && s.text.columns > 0 && string(lastCellAt(s.text.data[s.text.row:], s.text.columns)) == markerStatusTerminator {
				result.footerColumn = s.text.columns
			}
			break
		}
	}
	if first >= 0 {
		return s.text.data[first:s.sent], result
	}
	return nil, result
}

func (s *markerStream) appendPart(chunk string, more bool) ([]byte, parseResult) {
	before := len(s.text.data)
	var last byte
	if before > 0 {
		last = s.text.data[before-1]
	}
	s.text.feed(chunk)
	// Exclamações ordinárias no corpo não requerem buscas de status. O último
	// trecho publica o delta inteiro mesmo quando nenhum terminador foi encontrado.
	if more && s.candidate == 0 && !bytes.Contains(s.text.data[max(0, before-len(s.marker.endTag)):], []byte(s.marker.endTag)) {
		return nil, parseResult{}
	}
	// Um controle pendente pode consumir vários blocos sem alterar o texto.
	// O único recuo possível substitui ou remove a última quebra de linha.
	if s.scanned == before && before == len(s.text.data) && (before == 0 || last == s.text.data[before-1]) {
		return nil, parseResult{}
	}
	return s.progress()
}

func (s *markerStream) progress() (delta []byte, result parseResult) {
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
			n := bytes.IndexAny(data[s.statusScan:], markerStatusTerminator+"\n")
			if n < 0 {
				s.statusScan = len(data)
			} else {
				at := s.statusScan + n
				status := string(bytes.TrimSpace(data[s.candidate:at]))
				code, err := strconv.Atoi(status)
				switch {
				case data[at] == markerStatusTerminator[0] && err == nil:
					s.statusScan = -1
					result.Found = true
					result.ExitCode = code
					result.Output = string(bytes.TrimRight(data[s.start:s.candidate-len(end)], "\n"))
				case s.marker.conPTY && data[at] == '\n' && at == len(data)-1 && (err == nil || status == "" || status == "-" || status == "+"):
					// Na margem, a próxima leitura pode desfazer esse CRLF.
					// Só o terminador confirma que todos os dígitos chegaram.
					s.statusScan = at
				default:
					s.candidate = 0 // A linha inteira pertence à saída do comando.
				}
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
	} else if visible > s.sent {
		visible = s.sent + completeUTF8Prefix(data[s.sent:visible])
	}
	if visible > s.sent {
		delta = data[s.sent:visible]
		s.sent = visible
	}
	return
}

// Preserva a última runa para a próxima entrega se seus bytes ainda estão chegando.
func completeUTF8Prefix(data []byte) int {
	n := len(data)
	if n == 0 || data[n-1] < utf8.RuneSelf {
		return n
	}
	first := n - 1
	for first > max(0, n-utf8.UTFMax) && !utf8.RuneStart(data[first]) {
		first--
	}
	if !utf8.FullRune(data[first:]) {
		return first
	}
	return n
}
