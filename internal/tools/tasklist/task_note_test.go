package tasklist

import "testing"

// TestParseExternalUpdatedAt_JiraOffset garante que external_updated_at aceita o
// formato ISO-8601 do Jira com offset ±HHMM (sem dois-pontos), além de RFC3339.
func TestParseExternalUpdatedAt_JiraOffset(t *testing.T) {
	valid := []string{
		"2026-05-25T15:35:53.521-0300", // Jira com ms
		"2026-05-25T15:35:53-0300",     // Jira sem ms
		"2026-05-25T15:35:53.521-03:00", // RFC3339 (com :)
		"2026-05-25T15:35:53Z",          // UTC
		"2026-05-25",                    // só data
	}
	for _, in := range valid {
		tm, err := parseExternalUpdatedAt(in)
		if err != nil {
			t.Fatalf("parseExternalUpdatedAt(%q) erro inesperado: %v", in, err)
		}
		if tm == nil {
			t.Fatalf("parseExternalUpdatedAt(%q) retornou nil sem erro", in)
		}
	}

	// String vazia continua sendo "sem valor" (nil, nil), não erro.
	if tm, err := parseExternalUpdatedAt("   "); err != nil || tm != nil {
		t.Fatalf("vazio deveria dar (nil,nil), got tm=%v err=%v", tm, err)
	}

	// Lixo continua sendo rejeitado.
	if _, err := parseExternalUpdatedAt("not-a-time"); err == nil {
		t.Fatalf("esperava erro para timestamp inválido")
	}
}
