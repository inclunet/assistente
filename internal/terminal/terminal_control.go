package terminal

import (
	"math"
	"strings"
)

type escapeStep byte

const (
	escapeMore escapeStep = iota
	escapeEnd
	escapeInvalid
)

// terminalControl acompanha um único escape. Não retém texto de OSC/DCS,
// listas de parâmetros ou dados de comandos anteriores.
type terminalControl struct {
	kind, last byte
	resize     resizeStage
	columns    int
}

type resizeStage byte

const (
	resizeStart resizeStage = iota
	resizeAfterEight
	resizeFirstRowDigit
	resizeRow
	resizeFirstColumnDigit
	resizeColumn
	resizeInvalid
)

func (c *terminalControl) start() { *c = terminalControl{kind: '\x1b'} }

func (c *terminalControl) consume(ch byte, columns *int) escapeStep {
	// CAN e SUB descartam o controle em curso. O próximo byte já é texto
	// comum ou o início de outro escape.
	if ch == '\x18' || ch == '\x1a' {
		c.kind = 0
		return escapeEnd
	}
	switch {
	case c.kind == '\x1b':
		if ch == '[' || ch == ']' || ch == 'P' || ch == 'X' || ch == '^' || ch == '_' || ch >= 0x20 && ch <= 0x2f {
			c.kind, c.last = ch, ch
			return escapeMore
		}
	case c.kind >= 0x20 && c.kind <= 0x2f:
		if ch >= 0x20 && ch <= 0x2f {
			return escapeMore
		}
	case c.kind == '[':
		if ch >= 0x20 && ch <= 0x2f || ch >= 0x30 && ch <= 0x3f && c.last >= 0x30 {
			c.resizeParameter(ch)
			c.last = ch
			return escapeMore
		}
		if ch >= 0x40 && ch <= 0x7e {
			if ch == 't' && c.resize == resizeColumn {
				*columns = c.columns
			}
			c.kind = 0
			return escapeEnd
		}
		c.kind = 0
		return escapeInvalid
	default:
		if c.kind == ']' && ch == '\a' || c.last == '\x1b' && ch == '\\' {
			c.kind = 0
			return escapeEnd
		}
		// DCS pode transportar várias linhas antes de ST ou de cancelamento.
		if ch != '\n' || c.kind == 'P' {
			c.last = ch
			return escapeMore
		}
		c.kind = 0
		return escapeInvalid
	}
	c.kind = 0
	if ch >= 0x30 && ch <= 0x7e {
		return escapeEnd
	}
	return escapeInvalid
}

func (c *terminalControl) resizeParameter(ch byte) {
	digit := ch >= '0' && ch <= '9'
	switch {
	case c.resize == resizeStart && ch == '8', c.resize == resizeAfterEight && ch == ';',
		c.resize == resizeFirstRowDigit && digit, c.resize == resizeRow && ch == ';':
		c.resize++
	case c.resize == resizeRow && digit:
		return
	case (c.resize == resizeFirstColumnDigit || c.resize == resizeColumn) && digit:
		d := int(ch - '0')
		if c.columns > (math.MaxInt-d)/10 {
			c.resize = resizeInvalid
			return
		}
		c.columns = c.columns*10 + d
		c.resize = resizeColumn
	default:
		c.resize = resizeInvalid
	}
}

func (c *terminalControl) trackViewport(chunk string, columns *int) {
	for len(chunk) != 0 {
		if c.kind == 0 {
			at := strings.IndexByte(chunk, '\x1b')
			if at < 0 {
				return
			}
			c.start()
			chunk = chunk[at+1:]
			continue
		}
		if c.consume(chunk[0], columns) != escapeInvalid {
			chunk = chunk[1:]
		}
	}
}
