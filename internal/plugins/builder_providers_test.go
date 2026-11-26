package plugins

import (
	"assistente/internal/pluginpackage"
	"bytes"
	"encoding/json"
	"slices"
	"testing"

	"assistente/internal/database"
	"assistente/internal/providers"
)

func TestBuildNativeProviderOnly(t *testing.T) {
	db := managedJobTestDB(t)
	if err := db.AutoMigrate(&database.LLMProvider{}); err != nil {
		t.Fatal(err)
	}
	source := database.WithUserID(t.Context(), "source")
	other := database.WithUserID(t.Context(), "other")
	row := &database.LLMProvider{ID: "source-instance", Name: "Equipe", BaseURL: "https://equipe.example/v1", Type: "openai", Model: "local-model", CredentialPattern: "private-credential-source", IsDefault: true}
	if err := database.NewProviderRepository(db).SaveLLMProvider(source, row); err != nil {
		t.Fatal(err)
	}
	inventory := NewInventory(db)
	request := BuilderRequest{Name: "provider-only", Providers: []string{row.Slug}}
	if _, _, err := inventory.BuildPackage(other, request); err == nil {
		t.Fatal("provedor fora da conta")
	}
	_, snap, err := inventory.BuildPackage(source, request)
	if err != nil {
		t.Fatal(err)
	}
	components := DiscoverComponents(snap)
	if len(components) != 1 || components[0].Kind != "provider" || components[0].Endpoint != row.BaseURL || components[0].Reason != "" {
		t.Fatalf("prévia: %+v", components)
	}
	raw := snap.Files[components[0].Entry]
	for _, local := range []string{row.ID, row.CredentialPattern, "is_default"} {
		if bytes.Contains(raw, []byte(local)) {
			t.Fatalf("campo local no pacote: %s", local)
		}
	}
	p, err := providers.ReadPackageDefinition(raw, row.Slug)
	if err != nil || p.Model != row.Model || p.Slug != row.Slug {
		t.Fatalf("definição: %v %v", p, err)
	}
}

func TestDiscoverACPProviderShowsArgumentsAndRejectsLocalEnvironment(t *testing.T) {
	row := &database.LLMProvider{ID: "local", Slug: "assistente-local", Name: "Assistente local", Type: "acp", APIFormat: "acp", ACPCommand: "node", ACPArgs: `["/app/agent.js","um argumento"]`}
	raw, err := providers.MarshalPackageDefinition(row)
	if err != nil {
		t.Fatal(err)
	}
	entry := nativeExtension + "/providers/provider-assistente-local.json"
	snap := &pluginpackage.LocalSnapshot{Manifest: pluginpackage.Manifest{Extensions: map[string]json.RawMessage{nativeExtension: json.RawMessage(`{"version":1,"providers":["assistente-local"]}`)}}, Files: map[string][]byte{entry: raw}}
	items := DiscoverComponents(snap)
	if len(items) != 1 || items[0].Endpoint != "node" || !slices.Equal(items[0].ProcessArgs, []string{"/app/agent.js", "um argumento"}) {
		t.Fatalf("prévia do processo: %+v", items)
	}
	row.ACPEnv = `{"TENANT":"local"}`
	if _, err := providers.MarshalPackageDefinition(row); err == nil {
		t.Fatal("exportação descartou ambiente da conta")
	}
	row.ACPEnv = ""
	row.ACPCredentialEnv = `{"TOKEN":"local"}`
	if _, err := providers.MarshalPackageDefinition(row); err == nil {
		t.Fatal("exportação descartou referência local")
	}
}
