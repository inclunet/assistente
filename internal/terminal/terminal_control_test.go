package terminal

import (
	"bytes"
	"strconv"
	"testing"
)

func TestViewportAtEveryCommandBoundary(t *testing.T) {
	for _, raw := range []string{
		"\x1b]0;title\x1b[8;20;7t\a",
		"\x1b]0;title\x1b[8;20;7t\x1b\\",
		"\x1bPdata\x1b[8;20;7t\x1b\\",
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
				s.startCommandOutput(m)
			}
		})
	}
}
