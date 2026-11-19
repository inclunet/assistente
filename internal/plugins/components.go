package plugins

import (
	"encoding/json"
	"gopkg.in/yaml.v3"
	"path"
	"regexp"
	"sort"
	"strings"

	"assistente/internal/pluginpackage"
	"assistente/internal/skills"
)

// Component identifies a contribution within a revision; runtime keys are
// allocated separately, so the same skill name can occur in several packages.
type Component struct {
	Active    bool   `json:"active"`
	Slug      string `json:"slug,omitempty"`
	Key       string `json:"key"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Entry     string `json:"entry"`
	Reason    string `json:"reason,omitempty"`
	Transport string `json:"transport,omitempty"`
}

func DiscoverComponents(snapshot *pluginpackage.LocalSnapshot) []Component {
	var components []Component
	supplemental := map[string]bool{}
	for entry := range snapshot.Files {
		parts := strings.Split(entry, "/")
		if len(parts) >= 3 && parts[0] == "skills" && (len(parts) != 3 || parts[2] != "SKILL.md") {
			supplemental[parts[1]] = true
		}
	}
	for entry, raw := range snapshot.Files {
		parts := strings.Split(entry, "/")
		if len(parts) != 3 || parts[0] != "skills" || parts[2] != "SKILL.md" {
			continue
		}
		name := parts[1]
		component := Component{Kind: "skill", Name: name, Entry: entry, Key: path.Join("skills", name)}
		meta, body, err := skills.Parse(string(raw))
		if err != nil || meta.Name != name || strings.TrimSpace(meta.Description) == "" {
			component.Kind = "invalidSkill"
		} else if !portableSkillName.MatchString(name) || strings.Contains(name, "--") || len(name) > 64 {
			component.Kind = "invalidSkill"
		} else if skills.ContainsShellCommands(body) {
			component.Reason = "shellCommands"
		} else if supplemental[name] {
			component.Reason = "supportingFiles"
		} else {
			component.Reason = skillRequirements(string(raw))
		}
		components = append(components, component)
	}
	if raw, ok := snapshot.Files["mcp.json"]; ok {
		var file struct {
			Schema  string                     `json:"$schema"`
			Servers map[string]json.RawMessage `json:"mcpServers"`
		}
		if json.Unmarshal(raw, &file) != nil || file.Schema != "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json" || file.Servers == nil {
			components = append(components, Component{Key: "mcp.json", Kind: "invalidMcp", Entry: "mcp.json", Name: "mcp.json"})
		} else {
			for name, value := range file.Servers {
				var server struct {
					Type string `json:"type"`
				}
				component := Component{Kind: "mcp", Name: name, Entry: "mcp.json", Key: "mcp/" + name}
				if json.Unmarshal(value, &server) != nil || server.Type == "" {
					component.Kind = "invalidMcp"
				} else {
					component.Transport = server.Type
				}
				components = append(components, component)
			}
		}
	}
	sort.Slice(components, func(i, j int) bool { return components[i].Key < components[j].Key })
	return components
}

var portableSkillName = regexp.MustCompile("^[a-z0-9]+(-[a-z0-9]+)*$")

// An unknown frontmatter key may declare a runtime restriction. Only expose
// a plain instruction skill once all its requirements have a local mapping.
func skillRequirements(raw string) string {
	rest := strings.TrimPrefix(strings.TrimSpace(raw), "---")
	frontmatter, _, found := strings.Cut(rest, "\n---")
	if !found {
		return "frontmatter"
	}
	var fields map[string]any
	if yaml.Unmarshal([]byte(frontmatter), &fields) != nil {
		return "frontmatter"
	}
	for key := range fields {
		switch key {
		case "name", "description", "license", "metadata":
		default:
			return "frontmatter"
		}
	}
	return ""
}
