package providers

import (
	"encoding/json"
	"testing"

	"assistente/internal/database"
	"assistente/internal/llm"
)

func TestPackageProviderUsesNativeConfigurationValidation(t *testing.T) {
	for _, fields := range []map[string]string{
		{"auth_mode": "typo"},
		{"auth_mode": "none", "api_format": "google"},
		{"acp_agent_id": "agent"},
		{"credential_pattern": "privado"},
		{"insecure_http_authorized_url": "http://remote.invalid"},
		{"base_url": "https://example.org/v1?token=private"},
	} {
		fields["name"], fields["slug"], fields["type"] = "Equipe", "equipe", "openai"
		if fields["base_url"] == "" {
			fields["base_url"] = "https://example.org/v1"
		}
		raw, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ReadPackageDefinition(raw, "equipe"); err == nil {
			t.Fatalf("aceitou configuração com %v", fields)
		}
	}
	row := &database.LLMProvider{ID: "local", Slug: "equipe", Name: "Equipe", Type: "openai", APIFormat: "openai_responses", ErrorFamily: "openai", ServerProfileID: "openai", BaseURL: "https://api.openai.com/v1"}
	raw, err := MarshalPackageDefinition(row)
	if err != nil {
		t.Fatal(err)
	}
	p, err := ReadPackageDefinition(raw, "equipe")
	if err != nil || p.ErrorFamily != llm.ProviderErrorFamilyOpenAI {
		t.Fatalf("família do catálogo: %v %v", p, err)
	}
	for _, endpoint := range []string{"https://other.example/v1", "https://api.openai.com:9443/v1", "http://api.openai.com/v1"} {
		row.BaseURL = endpoint
		if _, err := MarshalPackageDefinition(row); err == nil {
			t.Fatalf("associação de catálogo fora da origem: %s", endpoint)
		}
	}

}
