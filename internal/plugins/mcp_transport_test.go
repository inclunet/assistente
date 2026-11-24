package plugins

import (
	"assistente/internal/pluginpackage"
	"testing"
)

func TestMCPTransportMatchesDeclaredShape(t *testing.T) {
	for _, raw := range []string{`{"type":"stdio","command":"/bin/example"}`, `{"type":"stdio","command":"cmd args"}`, `{"type":"stdio","command":"node","cwd":"${PLUGIN_DATA}/../outside"}`, `{"type":"sse","url":"relative"}`, `{"type":"streamable-http","url":"http://example.org/mcp"}`, `{"type":"sse","url":"https://user@example.org/#f"}`, `{"type":"sse","url":"https://example.org","headers":{"X-Test":"a","x-test":"b"}}`, `{"type":"sse","url":"https://example.org","headers":{"Bad Field":"x"}}`, `{"type":"stdio","command":42}`, `{"type":"stdio","command":"node","args":[null]}`, `{"type":"stdio","command":"node","env":{"PLUGIN_ROOT":"x"}}`, `{"type":"stdio","command":"node","cwd":"/"}`, `{"type":"sse","url":true}`, `{"type":"sse","url":"http://localhost","headers":{"x":null}}`, `{"type":"streamable-http","url":"http://localhost","args":[]}`, `{"type":"custom"}`} {
		if _, valid := mcpTransport([]byte(raw), false); valid {
			t.Fatal(raw)
		}
	}
	for _, raw := range []string{`{"type":"stdio","command":"node","args":["x"],"cwd":"./src","env":{"A":"b"}}`, `{"type":"sse","url":"https://example.org","headers":{"X":"example"}}`, `{"type":"streamable-http","url":"http://localhost"}`} {
		if transport, valid := mcpTransport([]byte(raw), false); !valid || transport == "" {
			t.Fatal(raw)
		}
	}
}

func TestDiscoverRejectsMCPEnvelopeAndKeepsIndependentSkill(t *testing.T) {
	for _, raw := range []string{
		`{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","McpServers":{}}`,
		`{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{},"extra":true}`,
	} {
		snap := &pluginpackage.LocalSnapshot{Files: map[string][]byte{"mcp.json": []byte(raw), "skills/note/SKILL.md": []byte("---\nname: note\ndescription: Note\n---\nHello")}}
		got := DiscoverComponents(snap)
		if len(got) != 2 || got[0].Kind != "invalidMcp" || got[1].Kind != "skill" || got[1].Reason != "" {
			t.Fatal(got)
		}
	}
}

func TestPreviewMCPWithPendingConfiguration(t *testing.T) {
	const endpoint = "https://example.org/path%2Fsegment"
	for _, declaration := range []string{
		`{"type":"sse","url":"` + endpoint + `?token=private"}`,
		`{"type":"sse","url":"` + endpoint + `?"}`,
		`{"type":"streamable-http","url":"` + endpoint + `","headers":{"Authorization":"private"}}`,
	} {
		snap := &pluginpackage.LocalSnapshot{Files: map[string][]byte{"mcp.json": []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"demo":` + declaration + `}}`)}}
		got := DiscoverComponents(snap)
		if len(got) != 1 || got[0].Endpoint != endpoint || got[0].Reason != "configuration" || got[0].Kind != "mcp" {
			t.Fatal(got)
		}
	}
}
