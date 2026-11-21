package terminal

import (
	"strings"
)

const ptyReadSize = 4096

// O terminal manual compartilha o interpretador de controles, sem a
// reconstrução de redesenhos do histórico de comandos. Cada evento entrega
// texto UTF-8 e converte CR isolado e CRLF em uma única quebra.
type rawTerminalOutput struct {
	text       parsedTerminalText
	reportedCR bool
}

func (r *rawTerminalOutput) append(chunk string, suppressed bool) string {
	if suppressed {
		r.text.control.trackViewport(chunk, &r.text.columns)
		return ""
	}
	r.text.feed(chunk)
	return r.take(false)
}

// Uma runa interrompida pertence à entrada anterior. Um controle aberto
// continua a ser interpretado sem transferir seu prefixo literal ao novo dono.
func (r *rawTerminalOutput) changeOwner() string {
	previous := r.take(true)
	control := r.text.control
	*r = rawTerminalOutput{text: parsedTerminalText{control: control}}
	return previous
}

func (r *rawTerminalOutput) finish() string {
	r.text.finish()
	return r.take(true)
}

func (r *rawTerminalOutput) take(final bool) string {
	data := r.text.data
	if r.reportedCR && len(data) > 0 {
		// feed acaba de materializar o CR anterior ou o LF que o completa.
		data = data[1:]
		r.reportedCR = false
	}
	n := len(data)
	if !final && !r.text.cr {
		n = completeUTF8Prefix(data)
	}
	out := strings.ReplaceAll(string(data[:n]), "\r", "\n")
	out = strings.ToValidUTF8(out, "\uFFFD")
	r.text.data = append(r.text.data[:0], data[n:]...)
	if r.text.cr && !r.reportedCR {
		out += "\n"
		r.reportedCR = true
	}
	// Um controle incompleto pode precisar ser devolvido literalmente no
	// EOF. Libere seu armazenamento assim que o controle estiver resolvido.
	if len(r.text.escape) == 0 && cap(r.text.escape) > ptyReadSize {
		r.text.escape = nil
	}
	if cap(r.text.data) > 2*ptyReadSize {
		r.text.data = append([]byte(nil), r.text.data...)
	}
	return out
}
