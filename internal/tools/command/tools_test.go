package command

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"assistente/internal/tools"
)

type fakeBackend struct {
	catalogCalls int
	configCalls  int
	lastCatalog  Request
	lastConfig   Request
	catalogValue any
	configValue  any
	catalogErr   error
	configErr    error
}

func (b *fakeBackend) Catalog(_ context.Context, request Request) (any, error) {
	b.catalogCalls++
	b.lastCatalog = request
	return b.catalogValue, b.catalogErr
}

func (b *fakeBackend) Config(_ context.Context, request Request) (any, error) {
	b.configCalls++
	b.lastConfig = request
	return b.configValue, b.configErr
}

func TestRequestParsingIsStrictAndCanonicalizesRawFields(t *testing.T) {
	backend := &fakeBackend{catalogValue: map[string]any{"ok": true}}
	tool := NewCatalog(backend)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"execute","command_id":"workspace.list","arguments":{"b":1,"a":2}}`))
	if err != nil || result.IsError {
		t.Fatalf("request válida falhou: result=%#v err=%v", result, err)
	}
	if got := string(backend.lastCatalog.Arguments); got != `{"a":2,"b":1}` {
		t.Fatalf("arguments não foi canonicalizado: %s", got)
	}
	if backend.lastCatalog.CommandID != "workspace.list" {
		t.Fatalf("command_id inesperado: %q", backend.lastCatalog.CommandID)
	}

	for _, raw := range []string{
		`{"action":"list","unknown":true}`,
		`{"action":"list","command_id":"workspace.list"}`,
		`{"action":"describe"}`,
		`{"action":"describe","command_id":"workspace.list","arguments":{}}`,
		`{"action":"execute","command_id":"workspace.list","arguments":[]}`,
		`{"action":"list"} trailing`,
		`[{"action":"list"}]`,
		`{"action":"unknown"}`,
		`{"action":null}`,
	} {
		before := backend.catalogCalls
		result, err := tool.Execute(context.Background(), json.RawMessage(raw))
		if err != nil || !result.IsError || result.Failure == nil || result.Failure.Code != "invalid_arguments" {
			t.Errorf("request %s não foi rejeitada como argumento inválido: result=%#v err=%v", raw, result, err)
		}
		if backend.catalogCalls != before {
			t.Errorf("backend chamado para request inválida %s", raw)
		}
	}
}

func TestActionSchemasAreExactD10Vocabulary(t *testing.T) {
	wantCatalog := []string{"list", "describe", "execute"}
	wantConfig := []string{
		"layer_list", "layer_get", "layer_create", "layer_update", "layer_delete", "layer_enable", "layer_disable", "layer_restore",
		"binding_list", "binding_check_conflict", "binding_create", "binding_update", "binding_delete", "binding_enable", "binding_disable", "binding_restore",
		"config_import", "config_export",
	}
	for _, test := range []struct {
		name   string
		schema json.RawMessage
		want   []string
	}{
		{CatalogName, NewCatalog(nil).Parameters(), wantCatalog},
		{ConfigName, NewConfig(nil).Parameters(), wantConfig},
	} {
		var schema struct {
			Properties map[string]struct {
				Enum []string `json:"enum"`
			} `json:"properties"`
			AdditionalProperties bool `json:"additionalProperties"`
		}
		if err := json.Unmarshal(test.schema, &schema); err != nil {
			t.Fatalf("schema %s inválido: %v", test.name, err)
		}
		if !reflect.DeepEqual(schema.Properties["action"].Enum, test.want) {
			t.Errorf("ações de %s = %#v, want %#v", test.name, schema.Properties["action"].Enum, test.want)
		}
		if schema.AdditionalProperties {
			t.Errorf("schema %s permite propriedades desconhecidas", test.name)
		}
	}
	catalogProperties := schemaProperties(t, NewCatalog(nil).Parameters())
	if got, want := sortedKeys(catalogProperties), []string{"action", "arguments", "command_id", "locale"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("campos de command_catalog = %#v, want %#v", got, want)
	}
	if got := catalogProperties["arguments"].(map[string]any)["type"]; got != "object" {
		t.Fatalf("arguments de command_catalog não é objeto: %#v", got)
	}
	configProperties := schemaProperties(t, NewConfig(nil).Parameters())
	if got, want := sortedKeys(configProperties), []string{"action", "id", "layer_id", "locale", "payload", "scope"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("campos de command_config = %#v, want %#v", got, want)
	}
	for _, forbidden := range []string{"command_id", "arguments"} {
		if _, ok := configProperties[forbidden]; ok {
			t.Fatalf("command_config não deve publicar %s", forbidden)
		}
	}
	payloadProperties := configProperties["payload"].(map[string]any)["properties"].(map[string]any)
	for _, field := range []string{"expectedRevision", "expectedFingerprint", "layer", "binding", "layerRefKind", "jsonData", "resolutions", "includeCredentials"} {
		if _, ok := payloadProperties[field]; !ok {
			t.Errorf("payload de command_config não descreve %s", field)
		}
	}
	for _, forbidden := range []string{"rule", "default"} {
		if _, ok := payloadProperties[forbidden]; ok {
			t.Errorf("payload CRUD não deve publicar %s", forbidden)
		}
	}
	if got := payloadProperties["includeCredentials"].(map[string]any)["const"]; got != false {
		t.Fatalf("includeCredentials não está fechado em false: %#v", got)
	}
	if _, ok := ConfigActionMutates("config_export_sensitive"); ok {
		t.Fatal("config_export_sensitive não pode existir na tool")
	}
}

func TestParametersReturnsIndependentSchemaCopy(t *testing.T) {
	first := NewCatalog(nil).Parameters()
	first[0] = 'X'
	if second := NewCatalog(nil).Parameters(); second[0] != '{' {
		t.Fatalf("Parameters compartilha o buffer do schema: %q", second[:1])
	}
}

func TestConfigSchemaDescribesImportResolutionsAndResourceIDs(t *testing.T) {
	var document map[string]any
	if err := json.Unmarshal(NewConfig(nil).Parameters(), &document); err != nil {
		t.Fatal(err)
	}
	properties := document["properties"].(map[string]any)
	payload := properties["payload"].(map[string]any)
	payloadProperties := payload["properties"].(map[string]any)
	resolutions := payloadProperties["resolutions"].(map[string]any)
	if got := resolutions["minItems"]; got != float64(1) {
		t.Fatalf("resolutions.minItems = %#v, want 1", got)
	}
	item := resolutions["items"].(map[string]any)
	itemProperties := item["properties"].(map[string]any)
	resourceType := itemProperties["resourceType"].(map[string]any)
	if got, want := resourceType["enum"], []any{"commandLayers", "commandLayerName", "commandWorkspace"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("resourceType enum = %#v, want %#v", got, want)
	}
	for _, phrase := range []string{"commandLayers", "commandLayerName", "commandWorkspace", "workspace atual", "JSONData importado"} {
		if !strings.Contains(resolutions["description"].(string), phrase) {
			t.Fatalf("descrição de resolutions não orienta sobre %s", phrase)
		}
	}

	foundIDCondition := false
	for _, raw := range document["allOf"].([]any) {
		condition := raw.(map[string]any)
		then := condition["then"].(map[string]any)
		required, ok := then["required"].([]any)
		if !ok {
			continue
		}
		for _, value := range required {
			if value == "id" {
				foundIDCondition = true
			}
		}
	}
	if !foundIDCondition {
		t.Fatal("schema não exige id condicionalmente nas ações de recurso")
	}
}

func TestConfigActionMutatesIsExhaustive(t *testing.T) {
	wantMutating := map[string]bool{
		"layer_create": true, "layer_update": true, "layer_delete": true, "layer_enable": true, "layer_disable": true, "layer_restore": true,
		"binding_create": true, "binding_update": true, "binding_delete": true, "binding_enable": true, "binding_disable": true, "binding_restore": true,
		"config_import": true,
	}
	wantReadOnly := map[string]bool{
		"layer_list": false, "layer_get": false, "binding_list": false, "binding_check_conflict": false, "config_export": false,
	}
	for action, want := range wantMutating {
		got, ok := ConfigActionMutates(action)
		if !ok || got != want {
			t.Errorf("classificação de %s = (%v,%v), want (%v,true)", action, got, ok, want)
		}
	}
	for action, want := range wantReadOnly {
		got, ok := ConfigActionMutates(action)
		if !ok || got != want {
			t.Errorf("classificação de %s = (%v,%v), want (%v,true)", action, got, ok, want)
		}
	}
	for _, action := range []string{"", "config_restore", "config_export_sensitive", "future_action"} {
		if got, ok := ConfigActionMutates(action); got || ok {
			t.Errorf("ação desconhecida %q classificada como (%v,%v)", action, got, ok)
		}
	}
}

func TestConfigForwardsAllD10ActionsWithoutAuthorization(t *testing.T) {
	backend := &fakeBackend{configValue: map[string]any{"accepted": true}}
	tool := NewConfig(backend)
	for action := range configActions {
		result, err := tool.Execute(context.Background(), json.RawMessage(validConfigRequest(action)))
		if err != nil || result.IsError {
			t.Fatalf("ação %s não foi encaminhada: result=%#v err=%v", action, result, err)
		}
	}
	if backend.configCalls != len(configActions) {
		t.Fatalf("backend recebeu %d chamadas, want %d", backend.configCalls, len(configActions))
	}
}

func TestConfigRejectsSensitiveExportIncludingPayload(t *testing.T) {
	backend := &fakeBackend{configValue: map[string]any{"accepted": true}}
	tool := NewConfig(backend)
	for _, raw := range []string{
		`{"action":"config_export","payload":{"includeCredentials":true}}`,
		`{"action":"config_export","arguments":{"nested":[{"includeCredentials":true}]}}`,
	} {
		result, err := tool.Execute(context.Background(), json.RawMessage(raw))
		if err != nil || !result.IsError || result.Failure == nil || result.Failure.Code != "sensitive_export_forbidden" {
			t.Errorf("exportação sensível não foi rejeitada: raw=%s result=%#v err=%v", raw, result, err)
		}
	}
	if backend.configCalls != 0 {
		t.Fatalf("backend não deveria ser chamado para exportação sensível: %d", backend.configCalls)
	}
}

func TestConfigRequestShapeIsStrict(t *testing.T) {
	backend := &fakeBackend{configValue: map[string]any{"accepted": true}}
	tool := NewConfig(backend)
	for _, raw := range []string{
		`{"action":"layer_list"}`,
		`{"action":"layer_list","scope":"tenant"}`,
		`{"action":"layer_list","scope":"global","command_id":"nope"}`,
		`{"action":"layer_list","scope":"global","arguments":{}}`,
		`{"action":"layer_list","scope":"global","payload":[]}`,
		`{"action":"layer_list","scope":"global","unknown":true}`,
		`{"action":"layer_list","scope":"global","action":"layer_get"}`,
	} {
		before := backend.configCalls
		result, err := tool.Execute(context.Background(), json.RawMessage(raw))
		if err != nil || !result.IsError || result.Failure == nil || result.Failure.Code != "invalid_arguments" {
			t.Errorf("request inválida aceita: %s result=%#v err=%v", raw, result, err)
		}
		if backend.configCalls != before {
			t.Errorf("backend chamado para request inválida: %s", raw)
		}
	}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"config_export","scope":"global","payload":{"includeCredentials":false}}`))
	if err != nil || result.IsError || backend.configCalls != 1 {
		t.Fatalf("exportação não sensível válida falhou: result=%#v err=%v calls=%d", result, err, backend.configCalls)
	}
}

func TestContextCancellationIsPropagatedBeforeBackend(t *testing.T) {
	backend := &fakeBackend{catalogValue: map[string]any{"ok": true}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := NewCatalog(backend).Execute(ctx, json.RawMessage(`{"action":"list"}`))
	if !errors.Is(err, context.Canceled) || result.IsError || backend.catalogCalls != 0 {
		t.Fatalf("cancelamento não propagado: result=%#v err=%v calls=%d", result, err, backend.catalogCalls)
	}
}

func TestNilBackendAndBackendErrorsAreSanitized(t *testing.T) {
	result, err := NewConfig(nil).Execute(context.Background(), json.RawMessage(`{"action":"layer_list","scope":"global"}`))
	if err != nil || !result.IsError || !result.Structured || result.Failure == nil || result.Failure.Code != "backend_unavailable" {
		t.Fatalf("nil backend não produziu erro estruturado: result=%#v err=%v", result, err)
	}
	if !json.Valid([]byte(result.Content)) {
		t.Fatalf("erro do nil backend não é JSON: %q", result.Content)
	}

	secret := "backend-secret-must-not-leak"
	backend := &fakeBackend{catalogErr: errors.New(secret)}
	result, err = NewCatalog(backend).Execute(context.Background(), json.RawMessage(`{"action":"list"}`))
	if err != nil || !result.IsError || !result.Structured || result.Failure == nil || result.Failure.Code != "backend_error" {
		t.Fatalf("erro do backend não foi sanitizado: result=%#v err=%v", result, err)
	}
	if strings.Contains(result.Content, secret) {
		t.Fatalf("erro bruto do backend vazou: %s", result.Content)
	}
}

func TestKnownBackendErrorsUseStableSanitizedCodes(t *testing.T) {
	tests := []struct {
		name    string
		backend error
		code    string
		message string
		kind    tools.ErrorKind
	}{
		{name: "stale", backend: ErrStale, code: "stale_configuration", message: "releia", kind: tools.ErrorKindConfiguration},
		{name: "denied", backend: ErrDenied, code: "access_denied", message: "autorizada", kind: tools.ErrorKindAuthorization},
		{name: "invalid", backend: ErrInvalidRequest, code: "invalid_arguments", message: "argumentos inválidos", kind: tools.ErrorKindInvalidArgs},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := &fakeBackend{catalogErr: test.backend}
			result, err := NewCatalog(backend).Execute(context.Background(), json.RawMessage(`{"action":"list"}`))
			if err != nil || !result.IsError || result.Failure == nil {
				t.Fatalf("erro conhecido não estruturado: result=%#v err=%v", result, err)
			}
			if result.Failure.Code != test.code || result.Failure.Kind != test.kind || result.Failure.Retryable {
				t.Fatalf("falha instável: %#v, want code=%q kind=%q non-retryable", result.Failure, test.code, test.kind)
			}
			if !strings.Contains(result.Content, test.message) || strings.Contains(result.Content, test.backend.Error()) {
				t.Fatalf("mensagem sanitizada inesperada: %s", result.Content)
			}
		})
	}
}

func TestBackendResultIsRealStructuredJSON(t *testing.T) {
	backend := &fakeBackend{catalogValue: map[string]any{"b": 1, "a": "ok"}}
	result, err := NewCatalog(backend).Execute(context.Background(), json.RawMessage(`{"action":"list"}`))
	if err != nil || result.IsError || !result.Structured {
		t.Fatalf("resposta não foi estruturada: result=%#v err=%v", result, err)
	}
	if got, want := result.Content, `{"a":"ok","b":1}`; got != want {
		t.Fatalf("JSON estruturado = %s, want %s", got, want)
	}
}

func TestBackendResultAllowsLargeStructuredJSON(t *testing.T) {
	backend := &fakeBackend{catalogValue: map[string]any{"catalog": strings.Repeat("x", 70*1024)}}
	result, err := NewCatalog(backend).Execute(context.Background(), json.RawMessage(`{"action":"list"}`))
	if err != nil || result.IsError || !result.Structured {
		t.Fatalf("resposta estruturada grande falhou: result=%#v err=%v", result, err)
	}
	if len(result.Content) <= 64*1024 || !json.Valid([]byte(result.Content)) {
		t.Fatalf("resposta estruturada grande inválida ou truncada: bytes=%d", len(result.Content))
	}
}

func TestBackendContextErrorIsPropagated(t *testing.T) {
	backend := &fakeBackend{catalogErr: context.DeadlineExceeded}
	result, err := NewCatalog(backend).Execute(context.Background(), json.RawMessage(`{"action":"list"}`))
	if !errors.Is(err, context.DeadlineExceeded) || result.IsError {
		t.Fatalf("erro de contexto do backend não foi propagado: result=%#v err=%v", result, err)
	}

	backend = &fakeBackend{catalogValue: func() {}}
	result, err = NewCatalog(backend).Execute(context.Background(), json.RawMessage(`{"action":"list"}`))
	if err != nil || !result.IsError || result.Failure == nil || result.Failure.Code != "serialization_failed" {
		t.Fatalf("resposta inválida do backend não foi sanitizada: result=%#v err=%v", result, err)
	}
}

var _ tools.Tool = (*Catalog)(nil)
var _ tools.Tool = (*Config)(nil)

func validConfigRequest(action string) string {
	payload := `{"expectedRevision":1,"expectedFingerprint":"fp"}`
	switch action {
	case "layer_create", "layer_update":
		payload = `{"expectedRevision":1,"expectedFingerprint":"fp","layer":{"name":"Camada","description":"Descrição","enabled":true,"resolutionPriority":1}}`
	case "binding_create", "binding_update":
		payload = `{"expectedRevision":1,"expectedFingerprint":"fp","binding":{"layerId":"layer","triggerType":"keyboard.local","triggerSpec":"{}","effect":"execute","enabled":true,"resolutionPriority":1}}`
	case "layer_restore":
		payload = `{"expectedRevision":1,"expectedFingerprint":"fp","layerRefKind":"user"}`
	case "config_import":
		payload = `{"jsonData":"{}","resolutions":[{"resourceType":"commandLayers","identifier":"*","strategy":"skip"}]}`
	case "config_export":
		payload = `{"includeCredentials":false}`
	}

	id := ""
	if strings.HasSuffix(action, "_get") || strings.Contains(action, "_update") || strings.Contains(action, "_delete") || strings.Contains(action, "_enable") || strings.Contains(action, "_disable") || strings.Contains(action, "_restore") {
		id = `,"id":"resource"`
	}
	layerID := ""
	if action == "binding_list" {
		layerID = `,"layer_id":"layer"`
	}
	if action == "layer_list" || action == "layer_get" || action == "binding_list" || action == "binding_check_conflict" {
		return `{"action":"` + action + `","scope":"global"` + id + layerID + `}`
	}
	return `{"action":"` + action + `","scope":"global"` + id + `,"payload":` + payload + `}`
}

func schemaProperties(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var schema struct {
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("schema inválido: %v", err)
	}
	return schema.Properties
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
