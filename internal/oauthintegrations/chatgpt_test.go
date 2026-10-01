package oauthintegrations

import (
	"assistente/internal/oauthflow"
	"testing"
)

func TestChatGPTReconnectIncludesRetainedIdentity(t *testing.T) {
	integration := ChatGPT()
	for _, state := range []string{"reauthorization_required", "disconnected"} {
		record := oauthflow.Record{State: state, Client: oauthflow.ClientRegistration{ID: "registered"}, Tokens: oauthflow.Tokens{ID: "validated-identity"}}
		values := integration.AuthorizationParameters(record, "host")
		if values.Get("id_token_hint") != "validated-identity" {
			t.Fatalf("identity hint lost for %s", state)
		}
	}
}
