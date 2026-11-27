package plugins

import (
	"encoding/json"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"assistente/internal/jobs"
	"assistente/internal/pluginpackage"
	"assistente/internal/skills"
	"gopkg.in/yaml.v3"
)

// Component identifies a contribution within a revision; runtime keys are
// allocated separately, so the same skill name can occur in several packages.
type Component struct {
	HookGeneration    string    `json:"hookGeneration,omitempty"`
	HookScript        string    `json:"hookScript,omitempty"`
	AgentProblemField string    `json:"agentProblemField,omitempty"`
	AgentInstructions string    `json:"agentInstructions,omitempty"`
	AgentTools        *[]string `json:"agentTools,omitempty"`
	CanCreateChatGPT  bool      `json:"canCreateChatGPT,omitempty"`
	MissingAllowlist  string    `json:"missingAllowlist,omitempty"`
	MissingProvider   string    `json:"missingProvider,omitempty"`
	ProfileDefinition string    `json:"profileDefinition,omitempty"`
	ProcessArgs       []string  `json:"processArgs,omitempty"`
	Tool              string    `json:"tool,omitempty"`

	JobMaxRunsPerHour int            `json:"jobMaxRunsPerHour,omitempty"`
	JobTriggers       []jobs.Trigger `json:"jobTriggers,omitempty"`
	RequestsShell     bool           `json:"requestsShell,omitempty"`
	EditedServer      string         `json:"editedServer,omitempty"`
	Endpoint          string         `json:"endpoint,omitempty"`
	Active            bool           `json:"active"`
	Slug              string         `json:"slug,omitempty"`
	Key               string         `json:"key"`
	Kind              string         `json:"kind"`
	Name              string         `json:"name"`
	Entry             string         `json:"entry"`
	Reason            string         `json:"reason,omitempty"`
	Transport         string         `json:"transport,omitempty"`
}

// DiscoverComponents identifies components using the files retained in the revision.
// Empty directories have no entries in a snapshot and cannot be distinguished from absent paths.
func DiscoverComponents(snapshot *pluginpackage.LocalSnapshot) []Component {
	components := nativeComponents(snapshot)
	claude := snapshot.Manifest.Format == pluginpackage.ClaudeCode
	var requiresManifestSupport, customSkills, customMCP bool
	for _, key := range snapshot.Manifest.Unmapped {
		kind := ""
		switch key {
		case "skills":
			customSkills = true
			kind = "skill"
		case "mcpServers":
			customMCP = true
			kind = "mcp"
		default:
			requiresManifestSupport = true
			continue
		}
		components = append(components, Component{Key: "manifest/" + key, Name: key, Kind: kind, Entry: key, Reason: "manifestConfiguration"})
	}
	var hasSkillFiles, hasExecutables bool
	supplemental := map[string]bool{}
	if _, ok := snapshot.Files["skills"]; ok {
		components = append(components, Component{Key: "skills", Kind: "invalidSkill", Entry: "skills", Name: "skills"})
	}
	for entry := range snapshot.Files {
		parts := strings.Split(entry, "/")
		if (claude || snapshot.Manifest.Format == pluginpackage.Cursor) && entry == "hooks/hooks.json" ||
			snapshot.Manifest.Format == pluginpackage.Cursor && entry == ".cursor/hooks.json" {
			components = append(components, hookFileComponents(entry, snapshot.Files[entry])...)
		}
		if claude && parts[0] == "agents" && len(parts) > 1 && path.Ext(entry) == ".md" {
			c := Component{Key: entry, Entry: entry, Name: strings.TrimPrefix(entry, "agents/"), Kind: "agent"}
			def, field, err := readClaudeAgent(snapshot.Files[entry])
			if err != nil {
				c.Reason = "agentConfiguration"
				c.AgentProblemField = field
				if err == errAgentTooLarge {
					c.Reason = "agentTooLarge"
				}
			} else {
				c.AgentInstructions = def.AgentInstructions
				if def.Tools != nil {
					c.AgentTools = &def.Tools
				}
			}
			components = append(components, c)
		}
		if parts[0] == "bin" && len(parts) > 1 {
			hasExecutables = true
		}
		if parts[0] == "skills" {
			hasSkillFiles = true
		}
		if snapshot.Manifest.Format != "" && snapshot.Manifest.Format != pluginpackage.AgentPlugins &&
			(parts[0] == "hooks" || (snapshot.Manifest.Format == pluginpackage.Cursor && (parts[0] == "rules" || strings.HasPrefix(entry, ".cursor/rules/"))) || entry == ".cursor/hooks.json" || entry == "hooks.json") {
			requiresManifestSupport = true
		}
		if len(parts) >= 3 && parts[0] == "skills" && (len(parts) != 3 || parts[2] != "SKILL.md") {
			supplemental[parts[1]] = true
		}
	}
	if _, exists := snapshot.Files["SKILL.md"]; exists && (claude || snapshot.Manifest.Format == pluginpackage.Cursor) && !hasSkillFiles && !snapshot.Manifest.DeclaresSkills {
		components = append(components, Component{Key: "SKILL.md", Entry: "SKILL.md", Name: snapshot.Manifest.Name, Kind: "skill", Reason: "rootSkill"})
	}
	for entry, raw := range snapshot.Files {
		parts := strings.Split(entry, "/")
		if (customSkills && !claude) || len(parts) != 3 || parts[0] != "skills" || parts[2] != "SKILL.md" {
			continue
		}
		name := parts[1]
		component := Component{Kind: "skill", Name: name, Entry: entry, Key: path.Join("skills", name)}
		meta, body, err := skills.Parse(string(raw))
		if err == nil && meta.Name == "" && snapshot.Manifest.Format != "" && snapshot.Manifest.Format != pluginpackage.AgentPlugins {
			meta.Name = name
		}
		requirement, valid := pluginSkillFrontmatter(string(raw))
		var requestsShell bool
		var runtimeRequirement string
		if claude {
			requestsShell, runtimeRequirement = skills.ClaudeCommandRequirements(body)
		}
		if !valid || err != nil || meta.Name != name || strings.TrimSpace(meta.Description) == "" || utf8.RuneCountInString(meta.Description) > 1024 {
			component.Kind = "invalidSkill"
		} else if !portableSkillName.MatchString(name) || strings.Contains(name, "--") || len(name) > 64 {
			component.Kind = "invalidSkill"
		} else if runtimeRequirement != "" {
			component.Reason = runtimeRequirement
		} else if snapshot.Manifest.Format != pluginpackage.ClaudeCode && skills.ContainsShellCommands(body) {
			component.Reason = "shellCommands"
		} else if supplemental[name] || (claude && hasExecutables && requestsShell) {
			component.Reason = "supportingFiles"
		} else {
			component.Reason = requirement
		}
		component.RequestsShell = component.Kind == "skill" && requestsShell
		components = append(components, component)
	}
	if filename, servers, exists := mcpEntries(snapshot); exists && (!customMCP || claude) {
		if servers == nil {
			components = append(components, Component{Key: filename, Kind: "invalidMcp", Entry: filename, Name: filename})
		} else {
			for name, value := range servers {
				transport, valid := mcpTransport(value, snapshot.Manifest.Format != "" && snapshot.Manifest.Format != pluginpackage.AgentPlugins)
				component := Component{Kind: "mcp", Name: name, Entry: filename, Key: "mcp/" + name}
				if !valid {
					component.Kind = "invalidMcp"
				} else {
					component.Transport = transport
					cfg, reason := portableMCP(value)
					component.Reason = reason
					component.Endpoint = cfg.URL
					// Claude merges additional declarations after the default file.
					if customMCP && reason == "" {
						component.Reason = "manifestConfiguration"
					}
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
	for n := range components {
		if components[n].Kind == "hook" {
			if script, ok := toolHookScript(snapshot, components[n].Key); ok {
				components[n].Reason = ""
				components[n].HookScript = script
			}
		}
	}
	sort.Slice(components, func(i, j int) bool { return components[i].Key < components[j].Key })
	return components
}

// Describe event declarations without interpreting or executing their handlers.
// Unmapped configurations keep the same package-wide activation guard.
func hookFileComponents(entry string, raw []byte) []Component {
	file := Component{Key: entry, Name: entry, Entry: entry, Kind: "hook", Reason: "hookConfiguration"}
	var declared struct {
		Hooks map[string]json.RawMessage
	}
	if len(raw) > 64<<10 {
		file.Reason = "hookInspectionLimit"
		return []Component{file}
	}
	if json.Unmarshal(raw, &declared) != nil || len(declared.Hooks) == 0 {
		return []Component{file}
	}
	if len(declared.Hooks) > 64 {
		file.Reason = "hookInspectionLimit"
		return []Component{file}
	}
	invalidName := false
	events := make([]Component, 0, len(declared.Hooks))
	for event, group := range declared.Hooks {
		if len(event) > 80 {
			file.Reason = "hookInspectionLimit"
			return []Component{file}
		}
		if len(event) == 0 || strings.IndexFunc(event, unicode.IsControl) >= 0 {
			invalidName = true
			continue
		}
		component := file
		component.Key = entry + "#" + event
		component.Name = event
		var handlers []json.RawMessage
		if json.Unmarshal(group, &handlers) == nil && len(handlers) > 0 {
			component.Reason = "hookExecutionPending"
		}
		events = append(events, component)
	}
	if invalidName {
		return []Component{file}
	}
	return events
}

var portableSkillName = regexp.MustCompile("^[a-z0-9]+(-[a-z0-9]+)*$")

// An unknown frontmatter key may declare a runtime restriction. Only expose
// a plain instruction skill once all its requirements have a local mapping.
func pluginSkillFrontmatter(raw string) (string, bool) {
	opening, rest, found := strings.Cut(strings.TrimLeft(raw, " \t\r\n"), "\n")
	if !found || strings.TrimRight(opening, " \t\r") != "---" {
		return "", false
	}
	// The runtime uses the first delimiter prefix. Reject ambiguous boundaries
	// so discovery and invocation always see the same metadata and body.
	frontmatter, suffix, found := strings.Cut(rest, "\n---")
	if !found {
		return "", false
	}
	trailing, _, _ := strings.Cut(suffix, "\n")
	if strings.Trim(trailing, " \t\r") != "" {
		return "", false
	}
	var fields map[string]any
	if yaml.Unmarshal([]byte(frontmatter), &fields) != nil {
		return "", false
	}
	requirement := ""
	for key, value := range fields {
		switch key {
		case "name", "description", "license", "version":
			if _, ok := value.(string); !ok {
				return "", false
			}
		case "metadata":
			metadata, ok := value.(map[string]any)
			if !ok {
				return "", false
			}
			for _, v := range metadata {
				if _, ok := v.(string); !ok {
					return "", false
				}
			}
		default:
			requirement = "frontmatter"
		}
	}
	return requirement, true
}
