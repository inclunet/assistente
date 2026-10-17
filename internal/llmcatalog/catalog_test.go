package llmcatalog

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

const minimalCatalog = `{"schema_version":1,"catalog_version":"v1","updated_at":"2026-10-17T12:00:00Z","servers":[{"id":"test","name":"Test","group":"custom","provider_family":"test","status":"active","api_format":"openai","adapter_id":"openai","auth":{"mode":"none","required":false},"fields":{"base_url":{"policy":"editable","required":true}}}],"models":[{"id":"vendor/model","vendor":"Vendor","name":"Model","status":"active","capabilities":{}}],"exposures":[{"server_profile_id":"test","api_format":"openai","exposed_model_id":"alias","canonical_model_id":"vendor/model"}]}`

func TestDecodePublishedCatalogAsset(t *testing.T) {
	data, err := os.ReadFile("../../docs/static/json/assistente-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Decode(data)
	if err != nil {
		t.Fatalf("asset publicado deve obedecer o contrato: %v", err)
	}
	if doc.SchemaVersion != 1 || len(doc.Servers) == 0 || len(doc.Models) == 0 || len(doc.Exposures) == 0 {
		t.Fatalf("catálogo carregado incompleto: versão=%d servers=%d models=%d exposures=%d", doc.SchemaVersion, len(doc.Servers), len(doc.Models), len(doc.Exposures))
	}
}

func TestDecodeRejectsInvalidDocuments(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{name: "referência inexistente", json: strings.Replace(minimalCatalog, `"server_profile_id":"test"`, `"server_profile_id":"missing"`, 1), want: "referência de servidor inexistente"},
		{name: "choice default fora das opções", json: strings.Replace(minimalCatalog, `"policy":"editable","required":true`, `"policy":"choice","default":"a","choices":["b"]`, 1), want: "deve corresponder a uma opção"},
		{name: "choices duplicadas", json: strings.Replace(minimalCatalog, `"policy":"editable","required":true`, `"policy":"choice","choices":["a","a"]`, 1), want: "opções duplicadas"},
		{name: "campo obrigatório ausente em auth", json: strings.Replace(minimalCatalog, `,"required":false`, "", 1), want: "auth.required ausente"},
		{name: "choices nulo", json: strings.Replace(minimalCatalog, `"required":true`, `"required":true,"choices":null`, 1), want: ".fields.base_url.choices não pode ser null"},
		{name: "limits nulo", json: strings.Replace(minimalCatalog, `"capabilities":{}`, `"capabilities":{},"limits":null`, 1), want: ".limits não pode ser null"},
		{name: "endpoints nulo", json: strings.Replace(minimalCatalog, `"fields":{"base_url"`, `"endpoints":null,"fields":{"base_url"`, 1), want: ".endpoints não pode ser null"},
		{name: "voices nulo", json: strings.Replace(minimalCatalog, `"canonical_model_id":"vendor/model"`, `"canonical_model_id":"vendor/model","voices":null`, 1), want: ".voices não pode ser null"},
		{name: "userinfo em URL de servidor", json: strings.Replace(minimalCatalog, `"group":"custom"`, `"group":"local","base_url":"https://usuario:senha@localhost:1234"`, 1), want: "URL inválida"},
		{name: "http em provedor hospedado", json: strings.Replace(minimalCatalog, `"group":"custom"`, `"group":"hosted","base_url":"http://api.example.com"`, 1), want: "esquema não permitido"},
		{name: "json final excedente", json: minimalCatalog + ` {}`, want: "há conteúdo após o JSON"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Decode([]byte(test.json))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Decode() erro = %v; esperado conter %q", err, test.want)
			}
		})
	}
}

func TestValidateRejectsDuplicateExposureTuple(t *testing.T) {
	doc, err := Decode([]byte(minimalCatalog))
	if err != nil {
		t.Fatal(err)
	}
	doc.Exposures = append(doc.Exposures, doc.Exposures[0])
	if err := Validate(doc); err == nil || !strings.Contains(err.Error(), "tupla") {
		t.Fatalf("Validate() erro = %v; esperava tupla duplicada", err)
	}
}

func TestFieldPolicyPreservesExplicitNullDefault(t *testing.T) {
	raw := strings.Replace(minimalCatalog, `"policy":"editable","required":true`, `"policy":"fixed","default":null`, 1)
	doc, err := Decode([]byte(raw))
	if err != nil {
		t.Fatalf("default null explícito deve ser válido: %v", err)
	}
	encoded, err := json.Marshal(doc.Servers[0].Fields["base_url"])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"default":null`) {
		t.Fatalf("default null foi perdido na serialização: %s", encoded)
	}
}

func TestFixedFieldRequiresDefault(t *testing.T) {
	raw := strings.Replace(minimalCatalog, `"policy":"editable","required":true`, `"policy":"fixed"`, 1)
	if _, err := Decode([]byte(raw)); err == nil || !strings.Contains(err.Error(), "obrigatório quando policy=fixed") {
		t.Fatalf("Decode() erro = %v; esperado default fixo obrigatório", err)
	}
}

func TestCloneIsIndependent(t *testing.T) {
	doc, err := Decode([]byte(strings.Replace(minimalCatalog, `"capabilities":{}`, `"capabilities":{"input.text":"supported"}`, 1)))
	if err != nil {
		t.Fatal(err)
	}
	clone := doc.Clone()
	clone.Servers[0].Fields["base_url"] = FieldPolicy{Policy: "hidden"}
	clone.Models[0].Capabilities["input.text"] = "unsupported"
	if doc.Servers[0].Fields["base_url"].Policy != "editable" || doc.Models[0].Capabilities["input.text"] != "supported" {
		t.Fatal("mutar clone alterou o documento original")
	}
}
