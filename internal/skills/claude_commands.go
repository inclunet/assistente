package skills

import (
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Claude skill placeholders begin at a word boundary. Offsets refer to the
// original definition; neither user text nor command output is scanned again.
func nextClaudeCommand(text string, from int) (start, end int, ok bool) {
	for from < len(text) {
		offset := strings.Index(text[from:], "!`")
		if offset < 0 {
			return 0, 0, false
		}
		start = from + offset
		prior, _ := utf8.DecodeLastRuneInString(text[:start])
		if start != 0 && !unicode.IsSpace(prior) {
			from = start + 2
			continue
		}
		stop := strings.IndexAny(text[start+2:], "`\r\n")
		if stop < 0 || text[start+2+stop] != '`' || stop == 0 {
			from = start + 2
			continue
		}
		return start, start + 3 + stop, true
	}
	return 0, 0, false
}
func ContainsClaudeCommand(text string) bool { _, _, ok := nextClaudeCommand(text, 0); return ok }

func renderClaudeCommands(ctx context.Context, skill *Skill, content string, run SkillCommandRunner, expand func(string) string) (string, error) {
	var out strings.Builder
	writeText := func(s string) {
		if expand != nil {
			s = expand(s)
		}
		out.WriteString(s)
	}
	pos := 0
	for {
		start, end, ok := nextClaudeCommand(content, pos)
		if !ok {
			writeText(content[pos:])
			break
		}
		writeText(content[pos:start])
		command := content[start+2 : end-1]
		if expand != nil {
			command = expand(command)
		}
		if strings.ContainsAny(command, "\r\n") || run == nil {
			return "", fmt.Errorf("comando de skill indisponível")
		}
		result, err := run(ctx, skill, command)
		if err != nil {
			return "", err
		}
		if len(result) > MaxCommandOutputSize {
			return "", fmt.Errorf("saída excedeu o limite da skill %q", skill.Slug)
		}
		out.WriteString(strings.TrimRight(result, "\r\n"))
		pos = end
	}
	return out.String(), nil
}
