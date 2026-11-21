package plugins

import (
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
	RequestsShell bool   `json:"requestsShell,omitempty"`
	Endpoint      string `json:"endpoint,omitempty"`
	Active        bool   `json:"active"`
	Slug          string `json:"slug,omitempty"`
	Key           string `json:"key"`
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	Entry         string `json:"entry"`
	Reason        string `json:"reason,omitempty"`
	Transport     string `json:"transport,omitempty"`
}

func DiscoverComponents(snapshot *pluginpackage.LocalSnapshot) []Component {
	components := nativeComponents(snapshot)
	requiresManifestSupport := len(snapshot.Manifest.Unmapped) > 0
	supplemental := map[string]bool{}
	for entry := range snapshot.Files {
		parts := strings.Split(entry, "/")
		if snapshot.Manifest.Format != "" && snapshot.Manifest.Format != pluginpackage.AgentPlugins &&
			(parts[0] == "hooks" || (snapshot.Manifest.Format == pluginpackage.Cursor && (parts[0] == "rules" || strings.HasPrefix(entry, ".cursor/rules/"))) || entry == ".cursor/hooks.json" || entry == "hooks.json") {
			requiresManifestSupport = true
		}
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
		if err == nil && meta.Name == "" && snapshot.Manifest.Format != "" && snapshot.Manifest.Format != pluginpackage.AgentPlugins {
			meta.Name = name
		}
		if err != nil || meta.Name != name || strings.TrimSpace(meta.Description) == "" {
			component.Kind = "invalidSkill"
		} else if !portableSkillName.MatchString(name) || strings.Contains(name, "--") || len(name) > 64 {
			component.Kind = "invalidSkill"
		} else if snapshot.Manifest.Format == pluginpackage.ClaudeCode && (strings.Contains(body, "```!") || strings.Contains(body, "CLAUDE_PLUGIN_ROOT") || strings.Contains(body, "CLAUDE_PLUGIN_DATA")) {
			component.Reason = "shellCommands"
		} else if snapshot.Manifest.Format != pluginpackage.ClaudeCode && skills.ContainsShellCommands(body) {
			component.Reason = "shellCommands"
		} else if supplemental[name] {
			component.Reason = "supportingFiles"
		} else {
			component.Reason = skillRequirements(string(raw))
			component.RequestsShell = snapshot.Manifest.Format == pluginpackage.ClaudeCode && skills.ContainsClaudeCommand(body)
		}
		components = append(components, component)
	}
	if filename, servers, exists := mcpEntries(snapshot); exists {
		if servers == nil {
			components = append(components, Component{Key: filename, Kind: "invalidMcp", Entry: filename, Name: filename})
		} else {
			for name, value := range servers {
				transport, valid := mcpTransport(value)
				component := Component{Kind: "mcp", Name: name, Entry: filename, Key: "mcp/" + name}
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
	if requiresManifestSupport {
		for n := range components {
			if components[n].Reason == "" {
				components[n].Reason = "manifestConfiguration"
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
		case "name", "description", "version", "license", "metadata":
		default:
			return "frontmatter"
		}
	}
	return ""
}
