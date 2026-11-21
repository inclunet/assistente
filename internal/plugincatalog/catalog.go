// Package plugincatalog interprets publisher indices without fetching or executing plugins.
package plugincatalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"assistente/internal/pluginpackage"
)

type Format string

const (
	Claude Format = "claude-code"
	Cursor Format = "cursor"
	Codex  Format = "codex"
)

var indices = []struct {
	format Format
	file   string
}{
	{Claude, ".claude-plugin/marketplace.json"}, {Cursor, ".cursor-plugin/marketplace.json"}, {Codex, ".agents/plugins/marketplace.json"},
}
var identifier = regexp.MustCompile("^[a-z0-9][a-z0-9._-]{0,127}$")

type Entry struct {
	Category       string `json:"category,omitempty"`
	Authentication string `json:"authentication,omitempty"`
	Installation   string `json:"installation,omitempty"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	Version        string `json:"version,omitempty"`
	Author         string `json:"author,omitempty"`
	License        string `json:"license,omitempty"`
	Path           string `json:"path,omitempty"`
	SourceKind     string `json:"sourceKind"`
	Diagnostic     string `json:"diagnostic,omitempty"`
}
type Index struct {
	Format  Format  `json:"format"`
	Name    string  `json:"name"`
	Entries []Entry `json:"entries"`
}

// ReadLocal reports every format in a multi-client repository separately.
func ReadLocal(directory string) ([]Index, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	return ReadRoot(root)
}

func ReadRoot(root *os.Root) ([]Index, error) {
	return ReadIndices(func(name string) ([]byte, error) { return pluginpackage.ReadFile(root, name, 10<<20) })
}

func ReadIndices(read func(string) ([]byte, error)) ([]Index, error) {
	var result []Index
	for _, index := range indices {
		raw, err := read(index.file)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		parsed, err := Decode(index.format, raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", index.file, err)
		}
		result = append(result, parsed)
	}
	if len(result) == 0 {
		return nil, errors.New("PLUGIN_CATALOG_NOT_FOUND")
	}
	return result, nil
}

func portablePath(value string) (string, bool) {
	value = strings.TrimPrefix(value, "./")
	if value == "." || !fs.ValidPath(value) || strings.ContainsAny(value, "\\:\x00") {
		return "", false
	}
	_, err := filepath.Localize(value)
	return value, err == nil
}

func Decode(format Format, raw []byte) (Index, error) {
	var doc struct {
		Name     string
		Owner    struct{ Name string }
		Metadata struct{ PluginRoot string }
		Plugins  []struct {
			Name        string
			Source      json.RawMessage
			Version     string
			Description string
			Author      struct{ Name string }
			License     string
			Category    string
			Policy      struct {
				Installation   string
				Authentication string
			}
		}
	}
	if format != Claude && format != Cursor && format != Codex {
		return Index{}, errors.New("unsupported catalog format")
	}
	if len(raw) > 10<<20 || !utf8.Valid(raw) || json.Unmarshal(raw, &doc) != nil || !identifier.MatchString(doc.Name) || doc.Plugins == nil || len(doc.Plugins) > 10000 || format != Codex && strings.TrimSpace(doc.Owner.Name) == "" {
		return Index{}, errors.New("invalid marketplace")
	}
	var declared struct{ Plugins []map[string]json.RawMessage }
	if err := json.Unmarshal(raw, &declared); err != nil {
		return Index{}, err
	}
	prefix := ""
	if doc.Metadata.PluginRoot != "" {
		var ok bool
		prefix, ok = portablePath(doc.Metadata.PluginRoot)
		if !ok {
			return Index{}, errors.New("invalid pluginRoot")
		}
	}
	result := Index{Format: format, Name: doc.Name, Entries: make([]Entry, 0, len(doc.Plugins))}
	seen := map[string]bool{}
	for pos, src := range doc.Plugins {
		if !identifier.MatchString(src.Name) || seen[src.Name] {
			return Index{}, errors.New("invalid or duplicate plugin name")
		}
		seen[src.Name] = true
		entry := Entry{Name: src.Name, Version: src.Version, Description: src.Description, Author: src.Author.Name, License: src.License, SourceKind: "local", Category: src.Category}
		if format == Codex {
			entry.Authentication = src.Policy.Authentication
			entry.Installation = src.Policy.Installation
		}
		var dir string
		if err := json.Unmarshal(src.Source, &dir); err != nil {
			var spec struct {
				Source string
				Path   string
			}
			if json.Unmarshal(src.Source, &spec) != nil {
				return Index{}, errors.New("invalid plugin source")
			}
			entry.SourceKind = spec.Source
			if format == Cursor && spec.Source == "" {
				entry.SourceKind = "local"
			}
			if entry.SourceKind == "local" {
				dir = spec.Path
			} else {
				entry.Diagnostic = "source"
			}
		}
		if entry.Diagnostic == "" {
			cleaned, ok := portablePath(dir)
			if !ok || format == Codex && !strings.HasPrefix(dir, "./") {
				return Index{}, errors.New("invalid plugin path")
			}
			entry.Path = path.Join(prefix, cleaned)
		}
		if format == Codex && ((src.Policy.Installation != "AVAILABLE" && src.Policy.Installation != "INSTALLED_BY_DEFAULT") || src.Policy.Authentication != "ON_INSTALL" || strings.TrimSpace(src.Category) == "") {
			entry.Diagnostic = "policy"
		}
		for _, field := range []string{"skills", "commands", "agents", "rules", "hooks", "mcpServers", "variables", "userConfig", "dependencies", "strict", "lspServers", "outputStyles", "channels", "monitors", "themes", "workflows"} {
			if _, ok := declared.Plugins[pos][field]; ok && entry.Diagnostic == "" {
				entry.Diagnostic = "configuration"
				break
			}
		}
		result.Entries = append(result.Entries, entry)
	}
	return result, nil
}
