package pluginpackage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	AgentPlugins = "agent-plugins"
	ClaudeCode   = "claude-code"
	Cursor       = "cursor"
	Codex        = "codex"
)

var manifestFiles = []struct{ name, format string }{
	{"plugin.json", AgentPlugins}, {".claude-plugin/plugin.json", ClaudeCode}, {".cursor-plugin/plugin.json", Cursor}, {".codex-plugin/plugin.json", Codex},
}

// LocateManifest requires an unambiguous interpreter. Multiple package
// manifests require choosing an interpreter, since their policies can differ.
func LocateManifest(read func(string) ([]byte, error)) (string, []byte, Manifest, []string, error) {
	var selected, format string
	var contents []byte
	for _, file := range manifestFiles {
		raw, err := read(file.name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", nil, Manifest{}, nil, err
		}
		if selected != "" {
			return "", nil, Manifest{}, nil, errors.New("PLUGIN_FORMAT_AMBIGUOUS")
		}
		selected, format, contents = file.name, file.format, raw
		// Standard packages historically allowed additional ecosystem overlays.
		if format == AgentPlugins {
			break
		}
	}
	if selected == "" {
		return "", nil, Manifest{}, nil, fmt.Errorf("PLUGIN_MANIFEST_MISSING: %w", fs.ErrNotExist)
	}
	if format == AgentPlugins {
		m, w, err := decodeManifest(contents)
		m.Format = format
		return selected, contents, m, w, err
	}
	m, w, err := decodeVendorManifest(format, contents)
	return selected, contents, m, w, err
}
func decodeVendorManifest(format string, raw []byte) (Manifest, []string, error) {
	var m Manifest
	m.Format = format
	var fields map[string]json.RawMessage
	if !utf8.Valid(raw) || json.Unmarshal(raw, &fields) != nil || fields == nil {
		return m, nil, errors.New("invalid plugin manifest")
	}
	get := func(key string, dst *string) error {
		if v, ok := fields[key]; ok && (string(v) == "null" || json.Unmarshal(v, dst) != nil) {
			return fmt.Errorf("invalid manifest field %s", key)
		}
		return nil
	}
	for key, dst := range map[string]*string{"name": &m.Name, "description": &m.Description, "version": &m.Version, "license": &m.License} {
		if err := get(key, dst); err != nil {
			return m, nil, err
		}
	}
	if !manifestName.MatchString(m.Name) || strings.Contains(m.Name, "..") || strings.Contains(m.Name, "--") {
		return m, nil, errors.New("invalid plugin name")
	}
	if v, ok := fields["author"]; ok {
		if json.Unmarshal(v, &m.Author) != nil && json.Unmarshal(v, &m.Author.Name) != nil {
			return m, nil, errors.New("invalid author")
		}
	}
	var warnings []string
	for key := range fields {
		switch key {
		case "metadata":
			if format == ClaudeCode {
				var object map[string]json.RawMessage
				if json.Unmarshal(fields[key], &object) == nil && object != nil {
					continue
				}
			}
		case "skills":
			m.DeclaresSkills = true
			var dir string
			if json.Unmarshal(fields[key], &dir) == nil && strings.TrimSuffix(strings.TrimPrefix(dir, "./"), "/") == "skills" {
				continue
			}
		case "mcpServers":
			var file string
			if json.Unmarshal(fields[key], &file) == nil && ((format == Cursor && strings.TrimPrefix(file, "./") == "mcp.json") || (format != Cursor && strings.TrimPrefix(file, "./") == ".mcp.json")) {
				continue
			}
		case "name", "version", "description", "author", "license", "homepage", "repository", "keywords", "interface", "logo", "icon", "documentationUrl", "supportUrl", "privacyPolicyUrl", "termsOfServiceUrl", "category", "displayName":
			continue
		}
		m.Unmapped = append(m.Unmapped, key)
		warnings = append(warnings, "unknown manifest field: "+key)
	}
	sort.Strings(m.Unmapped)
	sort.Strings(warnings)
	return m, warnings, nil
}

// CapturedManifest selects the interpreter using only the retained revision.
func CapturedManifest(files map[string][]byte) (string, Manifest, []string, error) {
	name, _, manifest, warnings, err := LocateManifest(func(name string) ([]byte, error) {
		if raw, ok := files[name]; ok {
			return raw, nil
		}
		return nil, fs.ErrNotExist
	})
	return name, manifest, warnings, err
}
