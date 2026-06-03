package jobs

import "testing"

func TestJiraTimeNormalizesOffset(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		want  string
		isErr bool
	}{
		{"jira com ms", "2026-05-25T15:35:53.521-0300", "2026-05-25T15:35:53.521-03:00", false},
		{"jira sem ms", "2026-05-25T15:35:53-0300", "2026-05-25T15:35:53-03:00", false},
		{"jira offset positivo", "2026-05-25T15:35:53+0530", "2026-05-25T15:35:53+05:30", false},
		{"ja rfc3339 (passthrough)", "2026-05-25T15:35:53.521-03:00", "2026-05-25T15:35:53.521-03:00", false},
		{"utc Z", "2026-05-25T15:35:53Z", "2026-05-25T15:35:53Z", false},
		{"invalido", "not-a-time", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := tplJiraTime(c.in)
			if c.isErr {
				if err == nil {
					t.Fatalf("esperava erro para %q, got %q", c.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("jiraTime(%q) erro inesperado: %v", c.in, err)
			}
			if got != c.want {
				t.Fatalf("jiraTime(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestDateAcceptsJiraOffset garante que a função `date` (string) passou a aceitar
// o offset ±HHMM do Jira, não só RFC3339 — evitando o abort do render que zerava
// o payload inteiro.
func TestDateAcceptsJiraOffset(t *testing.T) {
	got, err := tplDate("2026-05-25T15:35:53.521-0300", "2006-01-02")
	if err != nil {
		t.Fatalf("date com offset Jira não deveria falhar: %v", err)
	}
	if got != "2026-05-25" {
		t.Fatalf("date = %q, want %q", got, "2026-05-25")
	}
}

func TestDateRendersFullPayloadWithJiraTime(t *testing.T) {
	ctx := &TemplateContext{Output: map[string]any{
		"updated": "2026-05-25T15:35:53.521-0300",
		"content": "oi",
	}}
	tmpl := `{"comment_updated": {{ json (jiraTime .output.updated) }}, "content": {{ json .output.content }}}`
	res, err := resolveTemplate(tmpl, ctx)
	if err != nil {
		t.Fatalf("render falhou: %v", err)
	}
	want := `{"comment_updated": "2026-05-25T15:35:53.521-03:00", "content": "oi"}`
	if res != want {
		t.Fatalf("render = %q, want %q", res, want)
	}
}
