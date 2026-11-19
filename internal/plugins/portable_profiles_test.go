package plugins

import (
	"encoding/json"
	"testing"

	"assistente/internal/pluginpackage"
	"assistente/internal/profiles"
)

func TestDiscoverPortableProfile(t *testing.T) {
	const entry = nativeExtension + "/profiles/writer.json"
	snap := &pluginpackage.LocalSnapshot{
		Manifest: pluginpackage.Manifest{Extensions: map[string]json.RawMessage{
			nativeExtension: json.RawMessage(`{"version":1,"profiles":["writer"]}`),
		}},
		Files: map[string][]byte{entry: []byte(`{"name":"Writer","temperature":0,"maxTokens":250,"topP":0.8}`)},
	}
	components := DiscoverComponents(snap)
	if len(components) != 1 || components[0].Kind != "profile" || components[0].Name != "Writer" || components[0].Reason != "profileActivationPending" {
		t.Fatal(components)
	}
	p, err := profiles.ReadPortableDefinition(snap.Files[entry])
	if err != nil {
		t.Fatal(err)
	}
	if !p.Chat.DisableSkills || !p.Chat.DisableTools || p.Chat.LLMProvider != profiles.DefaultProviderSentinel || *p.Chat.Temperature != 0 || *p.Chat.MaxTokens != 250 || *p.Chat.TopP != 0.8 {
		t.Fatalf("unexpected projected profile: %+v", p.Chat)
	}
	for _, raw := range []string{"", `{"name":" "}`, `{"name":"x","maxTokens":-1}`, `{"name":"x","topP":2}`, `{"name":"x","command_allowlist":"local"}`, `{"name":"x"} {}`, "\xff"} {
		snap.Files[entry] = []byte(raw)
		c := DiscoverComponents(snap)
		if len(c) != 1 || c[0].Reason != "invalidProfile" {
			t.Errorf("accepted %q: %+v", raw, c)
		}
	}
}
