package llmcatalog

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

const minimalCatalog = `{"schema_version":1,"catalog_version":"v1","updated_at":"2026-10-17T12:00:00Z","servers":[{"id":"test","name":"Test","group":"custom","provider_family":"test","status":"active","api_format":"openai","adapter_id":"openai","auth":{"mode":"none","required":false},"fields":{"base_url":{"policy":"editable","required":true}}}],"models":[{"id":"vendor/model","vendor":"Vendor","name":"Model","status":"active","capabilities":{}}],"exposures":[{"server_profile_id":"test","api_format":"openai","exposed_model_id":"alias","canonical_model_id":"vendor/model"}]}`

func TestDecodePublishedCatalogAsset(t *testing.T) {
	data, err := os.ReadFile("../../docs/static/json/assistente-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	schemaData, err := os.ReadFile("../../docs/static/json/assistente-catalog.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(schemaData, &schema); err != nil {
		t.Fatalf("schema publicado inválido: %v", err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatalf("resolver schema publicado: %v", err)
	}
	var instance any
	if err := json.Unmarshal(data, &instance); err != nil {
		t.Fatal(err)
	}
	if err := resolved.Validate(instance); err != nil {
		t.Fatalf("asset não obedece ao JSON Schema publicado: %v", err)
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
		{name: "choice default fora das opções", json: strings.Replace(minimalCatalog, `"policy":"editable","required":true`, `"policy":"choice","default":"https://a.example.com","choices":["https://b.example.com"]`, 1), want: "deve corresponder a uma opção"},
		{name: "choices duplicadas", json: strings.Replace(minimalCatalog, `"policy":"editable","required":true`, `"policy":"choice","choices":["https://a.example.com","https://a.example.com"]`, 1), want: "opções duplicadas"},
		{name: "campo obrigatório ausente em auth", json: strings.Replace(minimalCatalog, `,"required":false`, "", 1), want: "auth.required ausente"},
		{name: "choices nulo", json: strings.Replace(minimalCatalog, `"required":true`, `"required":true,"choices":null`, 1), want: ".fields.base_url.choices não pode ser null"},
		{name: "limits nulo", json: strings.Replace(minimalCatalog, `"capabilities":{}`, `"capabilities":{},"limits":null`, 1), want: ".limits não pode ser null"},
		{name: "endpoints nulo", json: strings.Replace(minimalCatalog, `"fields":{"base_url"`, `"endpoints":null,"fields":{"base_url"`, 1), want: ".endpoints não pode ser null"},
		{name: "voices nulo", json: strings.Replace(minimalCatalog, `"canonical_model_id":"vendor/model"`, `"canonical_model_id":"vendor/model","voices":null`, 1), want: ".voices não pode ser null"},
		{name: "userinfo em URL de servidor", json: strings.Replace(minimalCatalog, `"group":"custom"`, `"group":"local","base_url":"https://usuario:senha@localhost:1234"`, 1), want: "URL inválida"},
		{name: "http em provedor hospedado", json: strings.Replace(minimalCatalog, `"group":"custom"`, `"group":"hosted","base_url":"http://api.example.com"`, 1), want: "esquema não permitido"},
		{name: "http em host remoto", json: strings.Replace(minimalCatalog, `"group":"custom"`, `"group":"proxy","base_url":"http://gateway.example.com/v1"`, 1), want: "URL inválida"},
		{name: "http em ipv4 loopback inválido", json: strings.Replace(minimalCatalog, `"group":"custom"`, `"group":"local","base_url":"http://127.999.0.1/v1"`, 1), want: "URL inválida"},
		{name: "http localhost não canônico", json: strings.Replace(minimalCatalog, `"group":"custom"`, `"group":"local","base_url":"http://LOCALHOST:11434/v1"`, 1), want: "URL inválida"},
		{name: "http ipv6 loopback não canônico", json: strings.Replace(minimalCatalog, `"group":"custom"`, `"group":"local","base_url":"http://[0:0:0:0:0:0:0:1]:11434/v1"`, 1), want: "URL inválida"},
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

func TestDecodeAllowsHTTPOnlyForLoopbackCatalogURLs(t *testing.T) {
	for _, baseURL := range []string{
		"http://localhost:11434/v1",
		"http://127.12.34.56:11434/v1",
		"http://[::1]:11434/v1",
	} {
		t.Run(baseURL, func(t *testing.T) {
			catalog := strings.Replace(minimalCatalog, `"group":"custom"`, `"group":"local","base_url":"`+baseURL+`"`, 1)
			if _, err := Decode([]byte(catalog)); err != nil {
				t.Fatalf("URL loopback deve ser aceita: %v", err)
			}
		})
	}
}

func TestDecodeAllowsAdditionalEndpointNamesOnlyForSupportedPaths(t *testing.T) {
	withCustomName := strings.Replace(minimalCatalog, `"auth":{`, `"endpoints":{"custom_metrics":{"path":"/models"}},"auth":{`, 1)
	if _, err := Decode([]byte(withCustomName)); err != nil {
		t.Fatalf("nome adicional permitido pelo schema deve aceitar rota allowlisted: %v", err)
	}

	withUnsupportedPath := strings.Replace(minimalCatalog, `"auth":{`, `"endpoints":{"custom_metrics":{"path":"/custom"}},"auth":{`, 1)
	if _, err := Decode([]byte(withUnsupportedPath)); err == nil || !strings.Contains(err.Error(), "rota não permitida") {
		t.Fatalf("rota fora da allowlist deveria ser rejeitada: %v", err)
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
	raw := strings.Replace(minimalCatalog,
		`"fields":{"base_url":{"policy":"editable","required":true}}`,
		`"fields":{"base_url":{"policy":"editable","required":true},"default_model_id":{"policy":"editable","default":null}}`, 1)
	doc, err := Decode([]byte(raw))
	if err != nil {
		t.Fatalf("default null explícito deve ser válido: %v", err)
	}
	encoded, err := json.Marshal(doc.Servers[0].Fields["default_model_id"])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"default":null`) {
		t.Fatalf("default null foi perdido na serialização: %s", encoded)
	}
}

func TestFixedFieldRequiresDefault(t *testing.T) {
	if err := validateFieldPolicy("fields.test", FieldPolicy{Policy: "fixed"}); err == nil || !strings.Contains(err.Error(), "obrigatório quando policy=fixed") {
		t.Fatalf("validateFieldPolicy() erro = %v; esperado default fixo obrigatório", err)
	}
}

func TestFixedBaseURLRequiresNonEmptyValue(t *testing.T) {
	raw := strings.Replace(minimalCatalog, `"policy":"editable","required":true`, `"policy":"fixed","default":null`, 1)
	if _, err := Decode([]byte(raw)); err == nil || !strings.Contains(err.Error(), "URL válida") {
		t.Fatalf("Decode() erro = %v; esperava rejeição de URL fixa sem valor", err)
	}
}

func TestFixedFieldDefaultsMatchFieldContracts(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "base_url remoto em HTTP",
			raw: strings.Replace(minimalCatalog,
				`"policy":"editable","required":true`,
				`"policy":"fixed","default":"http://gateway.example.com/v1"`, 1),
			want: "URL válida para o grupo",
		},
		{
			name: "api_format fora da allowlist",
			raw: strings.Replace(minimalCatalog,
				`"fields":{"base_url":{"policy":"editable","required":true}}`,
				`"fields":{"base_url":{"policy":"editable","required":true},"api_format":{"policy":"fixed","default":"unknown"}}`, 1),
			want: "formato de API suportado",
		},
		{
			name: "auth mode fora da allowlist",
			raw: strings.Replace(minimalCatalog,
				`"fields":{"base_url":{"policy":"editable","required":true}}`,
				`"fields":{"base_url":{"policy":"editable","required":true},"auth.mode":{"policy":"fixed","default":"unknown"}}`, 1),
			want: "modo de autenticação suportado",
		},
		{
			name: "default_model_id maior que o limite",
			raw: strings.Replace(minimalCatalog,
				`"fields":{"base_url":{"policy":"editable","required":true}}`,
				`"fields":{"base_url":{"policy":"editable","required":true},"default_model_id":{"policy":"fixed","default":"`+strings.Repeat("m", 201)+`"}}`, 1),
			want: "1 a 200 caracteres",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Decode([]byte(test.raw)); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Decode() erro = %v; esperado conter %q", err, test.want)
			}
		})
	}
}

func TestChoiceFieldValuesMatchFieldContracts(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "api_format fora da allowlist",
			raw: strings.Replace(minimalCatalog,
				`"fields":{"base_url":{"policy":"editable","required":true}}`,
				`"fields":{"base_url":{"policy":"editable","required":true},"api_format":{"policy":"choice","choices":["unknown"]}}`, 1),
			want: "formato de API suportado",
		},
		{
			name: "auth mode fora da allowlist",
			raw: strings.Replace(minimalCatalog,
				`"fields":{"base_url":{"policy":"editable","required":true}}`,
				`"fields":{"base_url":{"policy":"editable","required":true},"auth.mode":{"policy":"choice","choices":["unknown"]}}`, 1),
			want: "modo de autenticação suportado",
		},
		{
			name: "base_url remota HTTP",
			raw: strings.Replace(minimalCatalog,
				`"fields":{"base_url":{"policy":"editable","required":true}}`,
				`"fields":{"base_url":{"policy":"choice","choices":["http://gateway.example.com/v1"]}}`, 1),
			want: "URL válida para o grupo",
		},
		{
			name: "modelo choice vazio",
			raw: strings.Replace(minimalCatalog,
				`"fields":{"base_url":{"policy":"editable","required":true}}`,
				`"fields":{"base_url":{"policy":"editable","required":true},"default_model_id":{"policy":"choice","choices":["   "]}}`, 1),
			want: "cada opção deve ter de 1 a 200 caracteres",
		},
		{
			name: "modelo fixed com espaços externos",
			raw: strings.Replace(minimalCatalog,
				`"fields":{"base_url":{"policy":"editable","required":true}}`,
				`"fields":{"base_url":{"policy":"editable","required":true},"default_model_id":{"policy":"fixed","default":" model "}}`, 1),
			want: "deve ser um modelo de 1 a 200 caracteres",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Decode([]byte(test.raw)); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Decode() erro = %v; esperado conter %q", err, test.want)
			}
		})
	}
}

func TestFixedDefaultModelRequiresNonEmptyValue(t *testing.T) {
	raw := strings.Replace(minimalCatalog,
		`"fields":{"base_url":{"policy":"editable","required":true}}`,
		`"fields":{"base_url":{"policy":"editable","required":true},"default_model_id":{"policy":"fixed","default":null}}`, 1)
	if _, err := Decode([]byte(raw)); err == nil || !strings.Contains(err.Error(), "1 a 200 caracteres") {
		t.Fatalf("Decode() erro = %v; esperava rejeição de modelo fixo sem valor", err)
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

func TestClonePreservesEmptyVoiceItems(t *testing.T) {
	raw := strings.Replace(minimalCatalog, `"canonical_model_id":"vendor/model"`, `"canonical_model_id":"vendor/model","selection_mode":"model_only","voices":{"completeness":"complete","items":[]}`, 1)
	raw = strings.Replace(raw, `"policy":"editable","required":true`, `"policy":"fixed","default":"http://localhost:11434/v1"`, 1)
	doc, err := Decode([]byte(raw))
	if err != nil {
		t.Fatalf("catálogo com lista de vozes vazia deve ser válido: %v", err)
	}
	clone := doc.Clone()
	if doc.Exposures[0].Voices.Items == nil || clone.Exposures[0].Voices.Items == nil {
		t.Fatal("Clone converteu voices.items vazio em null")
	}
	encoded, err := json.Marshal(clone)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"items":[]`) {
		t.Fatalf("voices.items deveria permanecer array vazio: %s", encoded)
	}
	roundTrip, err := Decode(encoded)
	if err != nil {
		t.Fatalf("round-trip do documento serializado: %v", err)
	}
	reencoded, err := json.Marshal(roundTrip)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, reencoded) {
		t.Fatalf("round-trip não idempotente:\n primeiro: %s\nsegundo: %s", encoded, reencoded)
	}
}
