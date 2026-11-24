package plugincatalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCatalogAdaptersReadLocalIndices(t *testing.T) {
	dir := t.TempDir()
	for _, index := range indices {
		source := any("./plugins/sample")
		if index.format == Cursor {
			source = "plugins/sample"
		}
		if index.format == Codex {
			source = map[string]string{"source": "local", "path": "./plugins/sample"}
		}
		raw, err := json.Marshal(map[string]any{"name": "example", "owner": map[string]string{"name": "Publisher"}, "plugins": []any{map[string]any{"name": "sample", "source": source, "policy": map[string]string{"installation": "AVAILABLE", "authentication": "ON_INSTALL"}, "category": "Utility"}}})
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Join(dir, index.file)
		if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ReadLocal(dir)
	if err != nil || len(got) != 3 {
		t.Fatal(got, err)
	}
	for n, index := range got {
		if index.Format != Codex && (index.Entries[0].Authentication != "" || index.Entries[0].Installation != "") {
			t.Fatal(index)
		}
		if index.Format != indices[n].format || len(index.Entries) != 1 || index.Entries[0].Path != "plugins/sample" || index.Entries[0].Diagnostic != "" {
			t.Fatal(index)
		}
	}
}
func TestCatalogRejectsEscapeAndRetainsUnavailableEntries(t *testing.T) {
	for _, source := range []string{"./../other", "https://example.com", "C:/outside", "dir\\other"} {
		data, _ := json.Marshal(map[string]any{"name": "store", "owner": map[string]string{"name": "a"}, "plugins": []any{map[string]string{"name": "p", "source": source}}})
		if _, err := Decode(Claude, data); err == nil {
			t.Fatal(source)
		}
	}
	index, err := Decode(Codex, []byte(`{"name":"store","plugins":[{"name":"restricted","category":"Utility","source":{"source":"local","path":"./a"},"policy":{"installation":"NOT_AVAILABLE","authentication":"ON_INSTALL"}},{"name":"external","category":"Utility","source":{"source":"npm","package":"pkg"},"policy":{"installation":"AVAILABLE","authentication":"ON_INSTALL"}}]}`))
	if err != nil || index.Entries[0].Diagnostic != "policy" || index.Entries[1].Diagnostic != "source" {
		t.Fatal(index, err)
	}
}

func TestUnknownAuthenticationRequiresConfiguration(t *testing.T) {
	for _, auth := range []string{"", "UNKNOWN"} {
		raw, _ := json.Marshal(map[string]any{"name": "store", "plugins": []any{map[string]any{"name": "one", "source": "./one", "category": "Utility", "policy": map[string]string{"installation": "AVAILABLE", "authentication": auth}}}})
		index, err := Decode(Codex, raw)
		if err != nil || len(index.Entries) != 1 || index.Entries[0].Diagnostic != "policy" || index.Entries[0].Authentication != auth || index.Entries[0].Category != "Utility" {
			t.Fatal(index, err)
		}
	}
}

func TestCatalogDoesNotDropInlineHooks(t *testing.T) {
	idx, err := Decode(Claude, []byte(`{"name":"store","owner":{"name":"publisher"},"plugins":[{"name":"sample","source":"./sample","hooks":{"PostToolUse":[]}}]}`))
	if err != nil || idx.Entries[0].Diagnostic != "configuration" {
		t.Fatal(idx, err)
	}
}

func TestClaudeRelativeAndBareSources(t *testing.T) {
	index, err := Decode(Claude, []byte(`{"name":"market","owner":{"name":"alice"},"metadata":{"pluginRoot":"./plugins"},"plugins":[{"name":"root","source":"."},{"name":"nested","source":"./tools/nested","strict":true},{"name":"bare","source":"bare"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	root, nested, bare := index.Entries[0], index.Entries[1], index.Entries[2]
	if root.Diagnostic != "source" || nested.Path != "tools/nested" || nested.Diagnostic != "" || bare.Path != "plugins/bare" {
		t.Fatal(index)
	}
}

func TestCodexProductRestrictionAndDeferredAuthentication(t *testing.T) {
	index, err := Decode(Codex, []byte(`{"name":"catalog","plugins":[{"name":"allowed","source":"./allowed","category":"Utility","policy":{"installation":"AVAILABLE","authentication":"ON_USE"}},{"name":"limited","source":"./limited","category":"Utility","policy":{"installation":"AVAILABLE","authentication":"ON_USE","products":["CODEX"]}}]}`))
	if err != nil || len(index.Entries) != 2 {
		t.Fatal(index, err)
	}
	if index.Entries[0].Diagnostic != "" || index.Entries[1].Diagnostic != "policy" || len(index.Entries[1].Products) != 1 {
		t.Fatal(index)
	}
}
