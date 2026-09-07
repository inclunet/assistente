package textutil

import "testing"

func TestStripMarkdownForSpeech(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"**negrito** e *italico*", "negrito e italico"},
		{"# Titulo\n\ntexto", "Titulo\n\ntexto"},
		{"veja [docs](https://ex.com)", "veja docs"},
		{"use `code` aqui", "use code aqui"},
		{"```\nfn()\n```\nok", "code block \nok"},
		{"- item\n- outro", "item\noutro"},
		{"veja ![diagrama](https://ex.com/a.png) aqui", "veja diagrama aqui"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := StripMarkdownForSpeech(tc.in); got != tc.want {
			t.Fatalf("StripMarkdownForSpeech(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestCodeBlockSpeechLabel(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"pt-BR":     "bloco de código",
		"pt":        "bloco de código",
		"PT_br":     "bloco de código",
		"es-ES":     "bloque de código",
		"en-US":     "code block",
		"":          "code block",
		"ja":        "code block",
		"  pt-BR  ": "bloco de código",
	}
	for language, want := range cases {
		if got := CodeBlockSpeechLabel(language); got != want {
			t.Fatalf("CodeBlockSpeechLabel(%q)=%q want %q", language, got, want)
		}
	}
}

func TestStripMarkdownForSpeechLabeled(t *testing.T) {
	t.Parallel()
	const in = "```\nfn()\n```\nok"
	cases := []struct {
		label, want string
	}{
		{"bloco de código", "bloco de código \nok"},
		{"bloque de código", "bloque de código \nok"},
		{"code block", "code block \nok"},
		{"", "code block \nok"},
		{"   ", "code block \nok"},
	}
	for _, tc := range cases {
		if got := StripMarkdownForSpeechLabeled(in, tc.label); got != tc.want {
			t.Fatalf("StripMarkdownForSpeechLabeled(%q, %q)=%q want %q", in, tc.label, got, tc.want)
		}
	}
}

// O rótulo é inserido literalmente: `$1` num rótulo não pode virar expansão de grupo.
func TestStripMarkdownForSpeechLabeledEscapesExpansion(t *testing.T) {
	t.Parallel()
	got := StripMarkdownForSpeechLabeled("```\nfn()\n```", "$1 bloco")
	if got != "$1 bloco" {
		t.Fatalf("rótulo com $1 expandido indevidamente: %q", got)
	}
}
