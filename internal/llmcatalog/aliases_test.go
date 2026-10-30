package llmcatalog

import "testing"

func TestPrivateAliasesResolveExactIDsWithoutExposureOverrides(t *testing.T) {
	doc := Document{Models: []Model{{ID: "vendor/model", Capabilities: map[string]string{"tools": "supported"}, Limits: map[string]int64{"context_tokens": 1000}}}, Exposures: []Exposure{{ServerProfileID: "proxy", APIFormat: "openai", ExposedModelID: "alias", CanonicalModelID: "vendor/model", CapabilityOverrides: map[string]string{"tools": "unsupported"}}}}
	aliases := map[string]string{"alias": "vendor/model", "not-offered": "vendor/model"}
	for _, profile := range []string{"", "proxy"} {
		resolved := doc.ResolveModels(profile, "openai", []string{"alias", "Alias", "vendor/model"}, aliases)
		if len(resolved) != 1 || resolved["alias"].Capabilities["tools"] != "supported" {
			t.Fatalf("unexpected metadata: %+v", resolved)
		}
		resolved["alias"].Limits["context_tokens"] = 1
		if doc.Models[0].Limits["context_tokens"] != 1000 {
			t.Fatal("snapshot mutated")
		}
	}
	aliases["alias"] = "deleted"
	if len(doc.ResolveModels("proxy", "openai", []string{"alias"}, aliases)) != 0 {
		t.Fatal("missing private target fell back to public mapping")
	}
	if len(doc.ResolveModels("", "openai", []string{"alias"}, nil)) != 0 {
		t.Fatal("binding leaked across connections")
	}
}

func TestPrivateAliasChangesPreserveSavedBindingsOffline(t *testing.T) {
	doc := Document{Models: []Model{{ID: "known"}}}
	for _, tc := range []struct {
		name           string
		next, previous map[string]string
		valid          bool
	}{
		{"new", map[string]string{"remote": "known"}, nil, true},
		{"unknown", map[string]string{"remote": "missing"}, nil, false},
		{"unchanged", map[string]string{"remote": "missing"}, map[string]string{"remote": "missing"}, true},
		{"changed", map[string]string{"remote": "missing2"}, map[string]string{"remote": "missing"}, false},
		{"removed", nil, map[string]string{"remote": "missing"}, true},
		{"blank", map[string]string{" ": "known"}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := doc.ValidateAliasChanges(tc.next, tc.previous); (err == nil) != tc.valid {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestPrivateAliasErrorPreservesTargetAsEscapedParameter(t *testing.T) {
	err := (Document{}).ValidateAliasChanges(map[string]string{"remote": "vendor/model with space"}, nil)
	if err == nil || err.Error() != "model_alias_error:v1:missing_target:vendor%2Fmodel%20with%20space" {
		t.Fatalf("diagnostic=%v", err)
	}
}
