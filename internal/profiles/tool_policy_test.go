package profiles

import "testing"

func TestProfileValidateToolPolicyDefault(t *testing.T) {
	for _, state := range []string{"", "disabled", "on_demand"} {
		profile := DefaultProfile()
		profile.Chat.ToolPolicyDefault = state
		if err := profile.Validate(); err != nil {
			t.Fatalf("tool_policy_default %q deveria ser válido: %v", state, err)
		}
	}

	profile := DefaultProfile()
	profile.Chat.ToolPolicyDefault = "preloaded"
	if err := profile.Validate(); err == nil {
		t.Fatal("tool_policy_default preloaded deveria ser rejeitado")
	}
}
