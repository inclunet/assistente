package pluginpackage

import (
	"os"
	"strings"
	"testing"
)

func TestReadManifest(t *testing.T) {
	for _, tc := range []struct {
		name, fields string
		invalid      bool
		warnings     int
	}{
		{"mínimo", "", false, 0},
		{"metadados", `,"version":"nightly","author":{"name":"Exemplo","url":"site interno"},"keywords":["relatórios"],"extensions":{"com.example.other":null}`, false, 0},
		{"campo desconhecido", `,"skills":"./elsewhere"`, false, 1},
		{"extensões inválidas", `,"extensions":[]`, false, 1},
		{"autor inválido", `,"author":{"token":"x"}`, true, 0},
		{"palavra-chave nula", `,"keywords":[null]`, true, 0},
		{"versão nula", `,"version":null`, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, err := os.OpenRoot(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = root.Close() })
			raw := `{"$schema":"` + ManifestSchema + `","name":"one.example"` + tc.fields + "}"
			if err := root.WriteFile("plugin.json", []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			m, warnings, err := ReadManifest(root)
			if tc.invalid != (err != nil) || len(warnings) != tc.warnings {
				t.Fatalf("%+v warnings=%v err=%v", m, warnings, err)
			}
			if !tc.invalid && m.Name != "one.example" {
				t.Fatal(m)
			}
		})
	}
}

func TestReadManifestRejectsUnknownSchemaAndBadIdentity(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	for _, raw := range []string{`{}`, `{"name":"one","$schema":"https://example.org/schema.json"}`, `{"name":"two..dots","$schema":"` + ManifestSchema + `"}`, `{"name":"` + strings.Repeat("a", 65) + `","$schema":"` + ManifestSchema + `"}`} {
		if err := root.WriteFile("plugin.json", []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := ReadManifest(root); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}
