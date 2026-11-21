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
	Endpoint  string `json:"endpoint,omitempty"`
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
		var fields map[string]json.RawMessage
		var schema string
		var servers map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || len(fields) != 2 ||
			json.Unmarshal(fields["$schema"], &schema) != nil || schema != "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json" ||
			json.Unmarshal(fields["mcpServers"], &servers) != nil || servers == nil {
			components = append(components, Component{Key: "mcp.json", Kind: "invalidMcp", Entry: "mcp.json", Name: "mcp.json"})
		} else {
			for name, value := range servers {
				transport, valid := mcpTransport(value)
				component := Component{Kind: "mcp", Name: name, Entry: "mcp.json", Key: "mcp/" + name}
				if !valid {
					component.Kind = "invalidMcp"
				} else {
					component.Transport = transport
					cfg, reason := portableMCP(value)
					component.Reason = reason
					component.Endpoint = cfg.URL
					if reason == "invalid" {
						component.Kind = "invalidMcp"
					}
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
