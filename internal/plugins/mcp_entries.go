package plugins

import (
	"assistente/internal/pluginpackage"
	"encoding/json"
)

// mcpEntries normalizes only documented, lossless transport aliases. Unknown
// fields stay on the entry so a constraint cannot disappear before validation.
func mcpEntries(snapshot *pluginpackage.LocalSnapshot) (string, map[string]json.RawMessage, bool) {
	name := "mcp.json"
	format := snapshot.Manifest.Format
	vendor := format != "" && format != pluginpackage.AgentPlugins
	if format == pluginpackage.ClaudeCode || format == pluginpackage.Codex {
		name = ".mcp.json"
	}
	raw, exists := snapshot.Files[name]
	if !exists {
		return "", nil, false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return name, nil, true
	}
	var servers map[string]json.RawMessage
	_, wrapped := fields["mcpServers"]
	if format == pluginpackage.ClaudeCode && !wrapped {
		servers = fields
	} else if json.Unmarshal(fields["mcpServers"], &servers) != nil || servers == nil {
		return name, nil, true
	}
	if !vendor {
		var schema string
		if len(fields) != 2 || json.Unmarshal(fields["$schema"], &schema) != nil || schema != "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json" {
			return name, nil, true
		}
		return name, servers, true
	}
	if wrapped && len(fields) != 1 {
		return name, nil, true
	}
	for id, entry := range servers {
		var cfg map[string]json.RawMessage
		if json.Unmarshal(entry, &cfg) != nil || cfg == nil {
			continue
		}
		var transport string
		if rawType, ok := cfg["type"]; ok {
			if json.Unmarshal(rawType, &transport) != nil {
				continue
			}
		} else if _, hasCommand := cfg["command"]; hasCommand {
			transport = "stdio"
		} else if _, hasURL := cfg["url"]; hasURL {
			transport = "streamable-http"
		}
		if transport == "http" {
			transport = "streamable-http"
		}
		cfg["type"], _ = json.Marshal(transport)
		servers[id], _ = json.Marshal(cfg)
	}
	return name, servers, true
}
