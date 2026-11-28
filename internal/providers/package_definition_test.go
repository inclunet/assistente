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

func TestComparePortableProviderDefaults(t *testing.T) {
	row := &database.LLMProvider{ID: "local", Slug: "team", Name: "Team", BaseURL: "https://team.example/v1", Type: "openai", AuthMode: "none"}
	raw := []byte(`{"slug":"team","name":"Team","base_url":"https://team.example/v1","type":"openai","model_aliases":{}}`)
	if !MatchesPackageDefinition(row, raw) {
		t.Fatal("defaults implícitos e coleção vazia impediram reconhecer o provedor")
	}
	row.APIFormat = string((&llm.ProviderConfig{Type: "openai", BaseURL: row.BaseURL}).GetAPIFormat())
	row.ReasoningContentMode = string(llm.ReasoningContentDisabled)
	if !MatchesPackageDefinition(row, raw) {
		t.Fatal("defaults de API e raciocínio deveriam coincidir")
	}
	row.APIFormat = string(llm.APIFormatAnthropic)
	if MatchesPackageDefinition(row, raw) {
		t.Fatal("formatos de API diferentes foram unificados")
	}
	row.APIFormat = ""
	row.AuthMode = "required"
	if MatchesPackageDefinition(row, raw) {
		t.Fatal("ignorou o modo de autenticação")
	}
}

func TestPackagedProviderStaysExplicitAfterReload(t *testing.T) {
	store := NewMemoryStore()
	p := &llm.ProviderConfig{ID: "instancia", Name: "Equipe", BaseURL: "https://equipe.example/v1", Type: "openai", RequiresExplicitDefault: true}
	if err := store.Save(t.Context(), []*llm.ProviderConfig{p}); err != nil {
		t.Fatal(err)
	}
	svc := NewService(ServiceConfig{Store: store, Registry: llm.NewProviderRegistry()})
	if err := svc.Load(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(t.Context(), p.ID, UpdateRequest{Name: "Equipe nova"}); err != nil {
		t.Fatal(err)
	}
	updated := svc.registry.Get(p.ID)
	if updated == nil || !updated.RequiresExplicitDefault {
		t.Fatal("edição tornou provedor elegível a padrão automático")
	}
	for _, skip := range []bool{false, true} {
		svc.skipBootRepairs = skip
		def, err := svc.profileDefaultProvider(t.Context())
		if err != nil || def != nil {
			t.Fatalf("padrão implícito: %+v %v", def, err)
		}
	}
	if err := store.SetDefault(t.Context(), p.ID); err != nil {
		t.Fatal(err)
	}
	def, err := svc.profileDefaultProvider(t.Context())
	if err != nil || def == nil || def.ID != p.ID {
		t.Fatalf("escolha explícita: %+v %v", def, err)
	}
}

func TestRemovePackagedProviderSerializesConcurrentEdit(t *testing.T) {
	ctx := t.Context()
	store := NewMemoryStore()
	p := &llm.ProviderConfig{ID: "local-id", Name: "Original", Type: "openai", BaseURL: "https://example.test/v1", RequiresExplicitDefault: true}
	if err := store.Save(ctx, []*llm.ProviderConfig{p}); err != nil {
		t.Fatal(err)
	}
	svc := NewService(ServiceConfig{Store: store, Registry: llm.NewProviderRegistry()})
	if err := svc.Load(ctx); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	edited := make(chan error, 1)
	err := svc.RemovePackagedProviders(func() ([]string, error) {
		if err := store.Delete(ctx, p.ID); err != nil {
			return nil, err
		}
		go func() {
			close(started)
			_, err := svc.Update(ctx, p.ID, UpdateRequest{Name: "Edição concorrente"})
			edited <- err
		}()
		<-started
		return []string{p.ID}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := <-edited; err == nil {
		t.Fatal("edição recriou o cadastro removido")
	}
	if svc.registry.Get(p.ID) != nil {
		t.Fatal("provedor removido ficou no registro")
	}
	if err := svc.Save(ctx); err != nil {
		t.Fatal(err)
	}
	if rows, err := store.Load(ctx); err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
}
