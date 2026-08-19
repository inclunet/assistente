package docextract

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func extractRTF(data []byte, filename string) (*Result, error) {
	text := stripRTF(string(data))
	return &Result{
		Kind:     KindRTF,
		Source:   filename,
		Markdown: strings.TrimSpace(text) + "\n",
		Warnings: []string{"extração RTF é aproximada (control words removidos)"},
	}, nil
}

// stripRTF remove control words e grupos RTF de forma conservadora.
func stripRTF(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		c := s[i]
		switch c {
		case '\\':
			if i+1 >= len(s) {
				i++
				continue
			}
			next := s[i+1]
			if next == '\\' || next == '{' || next == '}' {
				b.WriteByte(next)
				i += 2
				continue
			}
			if next == '\'' && i+3 < len(s) {
				// hex escape \'hh
				h1, h2 := s[i+2], s[i+3]
				if isHex(h1) && isHex(h2) {
					v := unhex(h1)*16 + unhex(h2)
					if v != 0 {
						b.WriteByte(byte(v))
					}
					i += 4
					continue
				}
			}
			// control word
			i++
			wordStart := i
			for i < len(s) && ((s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z')) {
				i++
			}
			word := s[wordStart:i]
			if i < len(s) && (s[i] == '-' || (s[i] >= '0' && s[i] <= '9')) {
				if s[i] == '-' {
					i++
				}
				for i < len(s) && s[i] >= '0' && s[i] <= '9' {
					i++
				}
			}
			if i < len(s) && s[i] == ' ' {
				i++ // delimiter space
			}
			switch word {
			case "par", "line":
				b.WriteByte('\n')
			case "tab":
				b.WriteByte('\t')
			}
		case '{', '}':
			i++
		case '\r':
			i++
		case '\n':
			i++
		default:
			r, size := utf8.DecodeRuneInString(s[i:])
			if r == utf8.RuneError && size == 1 {
				i++
				continue
			}
			if unicode.IsPrint(r) || r == '\t' {
				b.WriteRune(r)
			}
			i += size
		}
	}
	return b.String()
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func unhex(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}
