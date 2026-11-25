package pluginpackage

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const ManifestSchema = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"

var manifestName = regexp.MustCompile("^[a-z0-9]([a-z0-9.-]{0,62}[a-z0-9])?$")

type Author struct {
	Name  string
	Email string
	URL   string
}

type Manifest struct {
	DeclaresSkills bool `json:"-"`
	Format         string
	Unmapped       []string
	Name           string
	Version        string
	Description    string
	Author         Author
	Homepage       string
	Repository     string
	License        string
	Keywords       []string
	Extensions     map[string]json.RawMessage
}

// ReadManifest interprets only the local 1.0.0 schema. It never retrieves
// metadata URLs or accesses components while checking a package's identity.
func ReadManifest(root *os.Root) (Manifest, []string, error) {
	raw, err := ReadFile(root, "plugin.json", 1<<20)
	if err != nil {
		return Manifest{}, nil, err
	}
	return decodeManifest(raw)
}

func decodeManifest(raw []byte) (Manifest, []string, error) {
	if !utf8.Valid(raw) {
		return Manifest{}, nil, fmt.Errorf("plugin.json: invalid UTF-8")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return Manifest{}, nil, fmt.Errorf("plugin.json: expected JSON object")
	}
	var m Manifest
	var schema string
	for key, dst := range map[string]*string{"$schema": &schema, "name": &m.Name, "version": &m.Version, "description": &m.Description, "homepage": &m.Homepage, "repository": &m.Repository, "license": &m.License} {
		if value, ok := fields[key]; ok {
			if string(value) == "null" || json.Unmarshal(value, dst) != nil {
				return Manifest{}, nil, fmt.Errorf("plugin.json: %s must be a string", key)
			}
			delete(fields, key)
		}
	}
	if schema != ManifestSchema {
		return Manifest{}, nil, fmt.Errorf("plugin.json: unsupported $schema")
	}
	if !manifestName.MatchString(m.Name) || strings.Contains(m.Name, "--") || strings.Contains(m.Name, "..") {
		return Manifest{}, nil, fmt.Errorf("plugin.json: invalid name")
	}
	if raw, ok := fields["author"]; ok {
		var author map[string]any
		if json.Unmarshal(raw, &author) != nil || author == nil {
			return Manifest{}, nil, fmt.Errorf("plugin.json: invalid author")
		}
		for k, v := range author {
			str, ok := v.(string)
			if !ok {
				return Manifest{}, nil, fmt.Errorf("plugin.json: invalid author field")
			}
			switch k {
			case "name":
				m.Author.Name = str
			case "email":
				m.Author.Email = str
			case "url":
				m.Author.URL = str
			default:
				return Manifest{}, nil, fmt.Errorf("plugin.json: unknown author field")
			}
		}
		delete(fields, "author")
	}
	if raw, ok := fields["keywords"]; ok {
		var values []any
		if json.Unmarshal(raw, &values) != nil || values == nil {
			return Manifest{}, nil, fmt.Errorf("plugin.json: invalid keywords")
		}
		for _, v := range values {
			s, ok := v.(string)
			if !ok {
				return Manifest{}, nil, fmt.Errorf("plugin.json: invalid keyword")
			}
			m.Keywords = append(m.Keywords, s)
		}
		delete(fields, "keywords")
	}
	var diagnostics []string
	if raw, ok := fields["extensions"]; ok {
		if err := json.Unmarshal(raw, &m.Extensions); err != nil || m.Extensions == nil {
			diagnostics = append(diagnostics, "invalid extensions object")
			m.Extensions = nil
		}
		delete(fields, "extensions")
	}
	for k := range fields {
		diagnostics = append(diagnostics, "unknown manifest field: "+k)
	}
	sort.Strings(diagnostics)
	return m, diagnostics, nil
}
