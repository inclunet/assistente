package skills

import "testing"

func TestParseComposePreservesMarkdownWhitespace(t *testing.T) {
	for _, body := range []string{"    !example", "\t!example", "  \n# Título", "texto  \ncontinuação  "} {
		raw := "---\r\nname: example\r\n---\r\n" + body + "\r\n"
		for range 3 {
			meta, got, err := Parse(raw)
			if err != nil || got != body {
				t.Fatalf("corpo=%q esperado=%q err=%v", got, body, err)
			}
			raw, err = Compose(meta, got)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}
