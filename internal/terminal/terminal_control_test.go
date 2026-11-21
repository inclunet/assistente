package terminal

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
)

func TestViewportAtEveryCommandBoundary(t *testing.T) {
	for _, raw := range []string{
		"\x1b]0;title\x1b[8;20;7t\a",
		"\x1b]0;title\x1b[8;20;7t\x1b\\",
		"\x1bPdata\x1b[8;20;7t\x1b\\",
		"\x1bPdata\r\nnext\x1b[8;20;7t\x1b\\",
	} {
		input := raw + "\x1b[8;20;113t"
		for cut := 0; cut <= len(input); cut++ {
			columns := 80
			var control terminalControl
			control.trackViewport(input[:cut], &columns)
			if cut < len(input) && columns != 80 {
				t.Fatalf("controle=%q corte=%d aceitou largura %d", raw, cut, columns)
			}
			m := newCommandMarkerForShell("powershell")
			m.conPTY, m.viewportColumns, m.control = true, columns, control
			stream := newMarkerStream(m)
			for i := cut; i < len(input); i++ {
				stream.append(input[i : i+1])
				control.trackViewport(input[i:i+1], &columns)
			}
			if columns != 113 || stream.text.columns != columns || control.kind != 0 || stream.text.control.kind != 0 {
				t.Fatalf("controle=%q corte=%d sessão=%d comando=%d", raw, cut, columns, stream.text.columns)
			}
		}
	}
}

func TestMultilineDCSKeepsPayloadOutOfCommandOutput(t *testing.T) {
	m := newCommandMarkerForShell("powershell")
	m.conPTY, m.viewportColumns = true, 80
	body := "antes\x1bP1;2|dados\n" + m.endTag + "9!\r\n\x1b[8;40;3t\aoculto\x1b\\depois"
	for cut := 0; cut <= len(body); cut++ {
		s := newMarkerStream(m)
		s.append(m.startTag + "\n")
		var emitted string
		for _, part := range []string{body[:cut], body[cut:]} {
			delta, result := s.append(part)
			emitted += string(delta)
			if result.Found || s.text.columns != 80 {
				t.Fatalf("corte=%d aceitou protocolo dentro de DCS: resultado=%+v largura=%d", cut, result, s.text.columns)
			}
		}
		delta, result := s.append(m.endTag + "0!\n")
		if !result.Found || result.ExitCode != 0 || result.Output != "antesdepois" || emitted+string(delta) != result.Output {
			t.Fatalf("corte=%d saída=%q resultado=%+v", cut, emitted+string(delta), result)
		}
	}
}

func TestUnterminatedControlPreservesPartialOutput(t *testing.T) {
	for _, body := range []string{"antes\x1bPdados\npendentes", "antes\x1b]titulo\nvisível"} {
		m := newCommandMarkerForShell("bash")
		for cut := 0; cut <= len(body); cut++ {
			s := newMarkerStream(m)
			s.append(m.startTag + "\n")
			delta, _ := s.append(body[:cut])
			emitted := string(delta)
			s.finish(body[cut:])
			if got := s.partialOutput(); got != body || !strings.HasPrefix(got, emitted) {
				t.Fatalf("corte=%d entrada=%q emitido=%q drenado=%q", cut, body, emitted, got)
			}
		}
	}
}

func TestCANAndSUBResumeTextAndViewport(t *testing.T) {
	m := newCommandMarkerForShell("powershell")
	m.conPTY, m.viewportColumns = true, 80
	for _, cancel := range []string{"\x18", "\x1a"} {
		for _, prefix := range []string{"\x1b]0;titulo", "\x1bPdados\n", "\x1b[8;40;7"} {
			// BEL posterior ao cancelamento recebe o mesmo tratamento de um
			// BEL isolado. O segundo CSI, completo, define a largura real.
			body := "antes" + prefix + cancel + "ok\a\x1b[8;40;113tfim"
			const want = "antesok\afim"
			for cut := 0; cut <= len(body); cut++ {
				var c terminalControl
				columns := 80
				s := newMarkerStream(m)
				s.append(m.startTag + "\n")
				var emitted string
				for _, part := range []string{body[:cut], body[cut:]} {
					c.trackViewport(part, &columns)
					delta, result := s.append(part)
					emitted += string(delta)
					if result.Found || !strings.HasPrefix(want, emitted) {
						t.Fatalf("entrada=%q corte=%d delta acumulado=%q", body, cut, emitted)
					}
				}
				delta, result := s.append(m.endTag + "0!\n")
				if !result.Found || result.Output != want || emitted+string(delta) != want || columns != 113 || s.text.columns != 113 {
					t.Fatalf("entrada=%q corte=%d resultado=%+v emitido=%q larguras=%d/%d", body, cut, result, emitted+string(delta), columns, s.text.columns)
				}
			}
		}
	}
}

// Só o início de comando fica na região medida. O backing array representa
// um histórico já recebido pelo leitor, pronto para ser reutilizado.
func BenchmarkStartCommandOutput(b *testing.B) {
	for _, size := range []int{4 << 10, 64 << 10, 4 << 20} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			raw := make([]byte, size)
			s := &Session{viewportColumns: 80}
			m := newCommandMarkerForShell("powershell")
			b.ReportAllocs()
			for b.Loop() {
				s.outputBuf = *bytes.NewBuffer(raw)
				s.startCommandOutput(m, &HistoryEntry{})
			}
		})
	}
}
