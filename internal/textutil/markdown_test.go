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
		{"```\nfn()\n```\nok", "bloco de código \nok"},
		{"- item\n- outro", "item\noutro"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := StripMarkdownForSpeech(tc.in); got != tc.want {
			t.Fatalf("StripMarkdownForSpeech(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}
