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

func TestPartialControlLiteralLimitAtEOF(t *testing.T) {
	for _, start := range []string{"\x1bP", "\x1b]"} {
		for _, size := range []int{maxControlLiteral - 1, maxControlLiteral, maxControlLiteral + 1} {
			sequence := start + strings.Repeat("x", size-len(start))
			m := newCommandMarkerForShell("bash")
			s := newMarkerStream(m)
			s.append(m.startTag + "\nantes")
			s.append(sequence[:size-1])
			s.finish(sequence[size-1:])
			want := "antes"
			if size <= maxControlLiteral {
				want += sequence
			}
			if got := s.partialOutput(); got != want {
				t.Fatalf("início=%q tamanho=%d: parcial tem %d bytes, esperados %d", start, size, len(got), len(want))
			}
		}
	}
}

func TestLargeControlsKeepBoundedStateInManualAndManagedOutput(t *testing.T) {
	const payloadSize = 3 << 20
	block := strings.Repeat("x", ptyReadSize)
	for _, tc := range []struct {
		name, start, end, want string
	}{
		{"DCS_ST", "\x1bP", "\x1b\\depois", "antesdepois"},
		{"DCS_CAN", "\x1bP", "\x18depois", "antesdepois"},
		{"DCS_EOF", "\x1bP", "", "antes"},
		{"OSC_ST", "\x1b]", "\x1b\\depois", "antesdepois"},
		{"OSC_BEL", "\x1b]", "\adepois", "antesdepois"},
		{"OSC_EOF", "\x1b]", "", "antes"},
		{"OSC_inválido", "\x1b]", "\ndepois", "antes\ndepois"},
	} {
		for _, mode := range []string{"manual", "gerenciado"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				var emitted strings.Builder
				session := &Session{onRawOutput: func(_, chunk string) { emitted.WriteString(chunk) }}
				m := newCommandMarkerForShell("powershell")
				parser := newMarkerStream(m)
				managed := mode == "gerenciado"
				if managed {
					session.startCommandOutput(m, &HistoryEntry{})
				}
				deliver := func(chunk string) {
					session.recordOutput(chunk)
					if managed {
						delta, result := parser.append(session.takeCommandOutput())
						if result.Found {
							t.Fatal("marca reconhecida dentro do controle")
						}
						emitted.Write(delta)
					}
				}
				if managed {
					deliver(m.startTag + "\n")
				}
				deliver("antes" + tc.start)
				for sent := 0; sent < payloadSize; sent += len(block) {
					deliver(block)
				}
				text := &session.rawOutput.text
				if managed {
					text = &parser.text
				}
				maxPending := 0
				if managed {
					maxPending = maxReusableOutputCapacity
				}
				if cap(text.escape) > maxControlLiteral || len(text.data) > 2*ptyReadSize || session.outputBuf.Len() != 0 || session.outputBuf.Cap() > maxPending || emitted.String() != "antes" {
					t.Fatalf("controle=%d texto=%d buffer bruto=%d bytes emitidos=%d", cap(text.escape), len(text.data), session.outputBuf.Cap(), emitted.Len())
				}
				deliver(m.endTag + "9!")
				for i := range len(tc.end) {
					deliver(tc.end[i : i+1])
				}
				if tc.end != "" {
					deliver("\x1b]pequeno\a") // novo controle volta a ser reconhecido
				}
				if managed {
					if tc.end == "" {
						parser.finish("")
						if got := parser.partialOutput(); got != tc.want {
							t.Fatalf("saída parcial=%q", got)
						}
					} else {
						result := parser.finish(m.endTag + "0!")
						if !result.Found || result.ExitCode != 0 || result.Output != tc.want {
							t.Fatalf("resultado após o controle=%+v", result)
						}
					}
				} else {
					session.finishRawOutput()
				}
				if emitted.String() != tc.want {
					t.Fatalf("saída transmitida=%q", emitted.String())
				}
			})
		}
	}
}
func TestTakeCommandOutputOwnsSnapshotAndReleasesPeak(t *testing.T) {
	session := &Session{suppressRawOutput: true}
	for _, size := range []int{ptyReadSize, maxReusableOutputCapacity, maxReusableOutputCapacity + 1} {
		first := strings.Repeat("a", size)
		next := strings.Repeat("b", size)
		session.recordOutput(first)
		snapshot := session.takeCommandOutput()
		if cap := session.outputBuf.Cap(); size > maxReusableOutputCapacity && cap != 0 || size <= maxReusableOutputCapacity && cap < size {
			t.Fatalf("lote=%d, capacidade retida=%d", size, cap)
		}
		session.recordOutput(next)
		if snapshot != first || session.takeCommandOutput() != next || session.takeCommandOutput() != "" {
			t.Fatalf("lote=%d: instantâneos de leituras sucessivas misturados", size)
		}
	}
}

func BenchmarkTakeCommandOutput(b *testing.B) {
	for _, size := range []int{ptyReadSize, 16 * ptyReadSize, 256 * ptyReadSize} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			session := &Session{suppressRawOutput: true}
			chunk := strings.Repeat("x", size)
			b.ReportAllocs()
			b.SetBytes(int64(size))
			for b.Loop() {
				session.recordOutput(chunk)
				if got := session.takeCommandOutput(); len(got) != size {
					b.Fatal(len(got))
				}
			}
		})
	}
}
