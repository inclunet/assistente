// Package command expõe as duas tools compostas de gerenciamento de comandos
// previstas no D10 da AEP-0103. A implementação aqui é apenas uma fronteira
// de transporte: autoridade, decisão e execução pertencem ao Backend.
package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"assistente/internal/commandjson"
	"assistente/internal/tools"
)

const (
	CatalogName = "command_catalog"
	ConfigName  = "command_config"
)

// Backend é a autoridade única das tools de comando. O wrapper não autentica,
// autoriza, classifica mutações nem chama serviços de domínio diretamente.
type Backend interface {
	Catalog(context.Context, Request) (any, error)
	Config(context.Context, Request) (any, error)
}

// Request é o envelope fechado compartilhado pelas duas tools.
type Request struct {
	Action    string          `json:"action"`
	CommandID string          `json:"command_id,omitempty"`
	ID        string          `json:"id,omitempty"`
	LayerID   string          `json:"layer_id,omitempty"`
	Scope     string          `json:"scope,omitempty"`
	Locale    string          `json:"locale,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// ConfigActionMutates retorna (muta, conhecida). A tabela é deliberadamente
// fechada e segue exatamente os verbos públicos do D10.
func ConfigActionMutates(action string) (bool, bool) {
	mutates, ok := configActions[action]
	return mutates, ok
}

var configActions = map[string]bool{
	"layer_list":             false,
	"layer_get":              false,
	"layer_create":           true,
	"layer_update":           true,
	"layer_delete":           true,
	"layer_enable":           true,
	"layer_disable":          true,
	"layer_restore":          true,
	"binding_list":           false,
	"binding_check_conflict": false,
	"binding_create":         true,
	"binding_update":         true,
	"binding_delete":         true,
	"binding_enable":         true,
	"binding_disable":        true,
	"binding_restore":        true,
	"config_import":          true,
	"config_export":          false,
}

var catalogActions = map[string]struct{}{
	"list":     {},
	"describe": {},
	"execute":  {},
}

// Catalog é a tool command_catalog.
type Catalog struct {
	backend Backend
}

// Config é a tool command_config.
type Config struct {
	backend Backend
}

func NewCatalog(backend Backend) *Catalog { return &Catalog{backend: backend} }

func NewConfig(backend Backend) *Config { return &Config{backend: backend} }

func (*Catalog) Name() string { return CatalogName }

func (*Config) Name() string { return ConfigName }

func (*Catalog) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{Category: "commands", Class: "command_management", Package: "commands", Risk: "write"}
}

func (*Config) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{Category: "commands", Class: "command_management", Package: "commands", Risk: "write"}
}

func (*Catalog) Description() string {
	return "Gerencia o catálogo de comandos por ações fechadas: list, describe e execute. A autoridade, as decisões e a execução permanecem no serviço de backend."
}

func (*Config) Description() string {
	return "Consulta e gerencia camadas e bindings por ações fechadas do D10. O backend é responsável por autenticação, autorização, decisões e persistência."
}

func (*Catalog) Parameters() json.RawMessage { return cloneSchema(catalogSchema) }

func (*Config) Parameters() json.RawMessage { return cloneSchema(configSchema) }

func (t *Catalog) Execute(ctx context.Context, raw json.RawMessage) (tools.ToolResult, error) {
	request, err := decodeCatalogRequest(raw)
	if err != nil {
		return invalidRequestResult(CatalogName), nil
	}
	return t.call(ctx, request)
}

func (t *Config) Execute(ctx context.Context, raw json.RawMessage) (tools.ToolResult, error) {
	request, err := decodeConfigRequest(raw)
	if err != nil {
		if errors.Is(err, errSensitiveExport) {
			return errorResult("sensitive_export_forbidden", "config_export não aceita includeCredentials=true"), nil
		}
		return invalidRequestResult(ConfigName), nil
	}
	if request.Action == "config_export" && containsCredentialsFlag(request.Payload) {
		return errorResult("sensitive_export_forbidden", "config_export não aceita includeCredentials=true"), nil
	}
	return t.call(ctx, request)
}

func (t *Catalog) call(ctx context.Context, request Request) (tools.ToolResult, error) {
	if err := contextError(ctx); err != nil {
		return tools.ToolResult{}, err
	}
	if t == nil || t.backend == nil {
		return errorResult("backend_unavailable", "backend de command_catalog indisponível"), nil
	}
	value, err := t.backend.Catalog(ctx, request)
	return backendResult(value, err, CatalogName)
}

func (t *Config) call(ctx context.Context, request Request) (tools.ToolResult, error) {
	if err := contextError(ctx); err != nil {
		return tools.ToolResult{}, err
	}
	if t == nil || t.backend == nil {
		return errorResult("backend_unavailable", "backend de command_config indisponível"), nil
	}
	value, err := t.backend.Config(ctx, request)
	return backendResult(value, err, ConfigName)
}

func backendResult(value any, backendErr error, toolName string) (tools.ToolResult, error) {
	if backendErr != nil {
		if errors.Is(backendErr, context.Canceled) || errors.Is(backendErr, context.DeadlineExceeded) {
			return tools.ToolResult{}, backendErr
		}
		switch {
		case errors.Is(backendErr, ErrStale):
			return errorResult("stale_configuration", "a configuração mudou; releia a visão atual antes de tentar novamente"), nil
		case errors.Is(backendErr, ErrDenied):
			return errorResult("access_denied", "a operação não está autorizada para esta sessão"), nil
		case errors.Is(backendErr, ErrInvalidRequest):
			return errorResult("invalid_arguments", "argumentos inválidos para "+toolName), nil
		}
		return errorResult("backend_error", "backend rejeitou a operação de "+toolName), nil
	}
	// Inputs use commandjson because command arguments are identity-bearing and
	// must be bounded/canonical. Outputs are ordinary tool results: the common
	// invocation executor owns their size/truncation/blob policy, so applying
	// commandjson's 64 KiB input limit here would reject valid catalogs.
	encoded, err := json.Marshal(value)
	if err != nil {
		return errorResult("serialization_failed", "backend retornou uma resposta inválida"), nil
	}
	return tools.ToolResult{
		Content:    string(encoded),
		Structured: true,
		Metadata:   map[string]any{"action": toolName},
	}, nil
}

func decodeCatalogRequest(raw []byte) (Request, error) {
	return decodeRequest(raw, catalogActions, requestKindCatalog)
}

func decodeConfigRequest(raw []byte) (Request, error) {
	return decodeRequest(raw, configActionsAsSet(), requestKindConfig)
}

type requestKind uint8

const (
	requestKindCatalog requestKind = iota
	requestKindConfig
)

func decodeRequest(raw []byte, actions map[string]struct{}, requestKindValue requestKind) (Request, error) {
	canonical, err := commandjson.Canonicalize(raw)
	if err != nil || len(canonical) == 0 || canonical[0] != '{' {
		return Request{}, errInvalidRequest
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(canonical, &fields); err != nil || fields == nil {
		return Request{}, errInvalidRequest
	}
	if requestKindValue == requestKindConfig && fields["action"] != nil {
		var action string
		if json.Unmarshal(fields["action"], &action) == nil && action == "config_export" {
			// Even misplaced credential requests are rejected with the stable
			// sensitive-export code before the closed envelope is applied.
			if value, present := fields["arguments"]; present && containsCredentialsFlag(value) {
				return Request{}, errSensitiveExport
			}
			if value, present := fields["payload"]; present && containsCredentialsFlag(value) {
				return Request{}, errSensitiveExport
			}
		}
	}
	for field := range fields {
		if !allowedRequestField(requestKindValue, field) {
			return Request{}, errInvalidRequest
		}
	}
	if _, ok := fields["action"]; !ok || !validStringField(fields["action"]) {
		return Request{}, errInvalidRequest
	}

	for _, field := range []string{"command_id", "id", "layer_id", "scope", "locale"} {
		if value, present := fields[field]; present && !validStringField(value) {
			return Request{}, errInvalidRequest
		}
	}
	if value, present := fields["arguments"]; present && !jsonObject(value) {
		return Request{}, errInvalidRequest
	}
	if value, present := fields["payload"]; present && !jsonObject(value) {
		return Request{}, errInvalidRequest
	}
	var request Request
	decoder := json.NewDecoder(bytes.NewReader(canonical))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return Request{}, errInvalidRequest
	}
	if err := ensureEOF(decoder); err != nil {
		return Request{}, errInvalidRequest
	}
	if request.Action == "" {
		return Request{}, errInvalidRequest
	}
	if _, ok := actions[request.Action]; !ok {
		return Request{}, errInvalidRequest
	}
	if requestKindValue == requestKindCatalog {
		switch request.Action {
		case "list":
			if _, present := fields["command_id"]; present {
				return Request{}, errInvalidRequest
			}
			if _, present := fields["arguments"]; present {
				return Request{}, errInvalidRequest
			}
		case "describe":
			if !presentNonEmptyString(fields, "command_id") {
				return Request{}, errInvalidRequest
			}
			if _, present := fields["arguments"]; present {
				return Request{}, errInvalidRequest
			}
		case "execute":
			if !presentNonEmptyString(fields, "command_id") {
				return Request{}, errInvalidRequest
			}
		}
	}
	if requestKindValue == requestKindConfig {
		if !presentNonEmptyString(fields, "scope") || (request.Scope != "global" && request.Scope != "workspace") {
			return Request{}, errInvalidRequest
		}
	}
	return request, nil
}

func allowedRequestField(kind requestKind, field string) bool {
	switch kind {
	case requestKindCatalog:
		switch field {
		case "action", "command_id", "locale", "arguments":
			return true
		default:
			return false
		}
	case requestKindConfig:
		switch field {
		case "action", "id", "layer_id", "scope", "locale", "payload":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func presentNonEmptyString(fields map[string]json.RawMessage, name string) bool {
	raw, ok := fields[name]
	if !ok || !validStringField(raw) {
		return false
	}
	var value string
	return json.Unmarshal(raw, &value) == nil && value != ""
}

func jsonObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) >= 2 && trimmed[0] == '{' && trimmed[len(trimmed)-1] == '}'
}

func validStringField(raw json.RawMessage) bool {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return false
	}
	var value string
	return json.Unmarshal(raw, &value) == nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errInvalidRequest
	}
	return nil
}

func containsCredentialsFlag(raw json.RawMessage) bool {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return false
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	return containsCredentialsFlagValue(value)
}

func containsCredentialsFlagValue(value any) bool {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			if key == "includeCredentials" {
				if enabled, ok := child.(bool); ok && enabled {
					return true
				}
			}
			if containsCredentialsFlagValue(child) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if containsCredentialsFlagValue(child) {
				return true
			}
		}
	}
	return false
}

func configActionsAsSet() map[string]struct{} {
	actions := make(map[string]struct{}, len(configActions))
	for action := range configActions {
		actions[action] = struct{}{}
	}
	return actions
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

var (
	// ErrInvalidRequest é retornado pelo backend quando o envelope ou payload
	// não representa uma solicitação válida para a operação escolhida.
	ErrInvalidRequest = errors.New("invalid command tool request")
	// ErrStale indica que a revisão/fingerprint usado pela mutação ficou velho.
	ErrStale = errors.New("stale command configuration")
	// ErrDenied indica que a sessão autenticada não pode realizar a operação.
	ErrDenied = errors.New("command access denied")

	errInvalidRequest  = ErrInvalidRequest
	errSensitiveExport = errors.New("sensitive command export")
)

func invalidRequestResult(toolName string) tools.ToolResult {
	return errorResult("invalid_arguments", "argumentos inválidos para "+toolName)
}

func errorResult(code, message string) tools.ToolResult {
	payload := map[string]any{"error": map[string]string{"code": code, "message": message}}
	encoded, _ := commandjson.Marshal(payload)
	kind := tools.ErrorKindUnknown
	if code == "invalid_arguments" || code == "sensitive_export_forbidden" {
		kind = tools.ErrorKindInvalidArgs
	}
	if code == "backend_unavailable" {
		kind = tools.ErrorKindUnavailable
	}
	if code == "access_denied" {
		kind = tools.ErrorKindAuthorization
	}
	if code == "stale_configuration" {
		kind = tools.ErrorKindConfiguration
	}
	return tools.ToolResult{
		Content:    string(encoded),
		IsError:    true,
		Structured: true,
		Failure:    &tools.ToolFailure{Code: code, Kind: kind, Retryable: false},
	}
}

var catalogSchema = json.RawMessage(`{
  "type":"object",
  "description":"Descobre comandos e, após consultar a descrição, executa um comando pelo seu command_id. A execução e a autorização continuam no backend.",
  "properties":{
    "action":{"type":"string","enum":["list","describe","execute"],"description":"Use list para descobrir comandos, describe para obter o schema de argumentos e execute para invocar um comando."},
    "command_id":{"type":"string","minLength":1,"description":"ID canônico do comando; obrigatório para describe e execute."},
    "locale":{"type":"string","description":"Locale da apresentação do comando, por exemplo pt-BR, en ou es."},
    "arguments":{"type":"object","description":"Argumentos do comando descrito. O backend valida o schema específico do comando.","additionalProperties":true}
  },
  "required":["action"],
  "additionalProperties":false,
  "allOf":[
    {"if":{"properties":{"action":{"enum":["describe","execute"]}}},"then":{"required":["command_id"]}},
    {"if":{"properties":{"action":{"const":"list"}}},"then":{"not":{"anyOf":[{"required":["command_id"]},{"required":["arguments"]}]}}},
    {"if":{"properties":{"action":{"const":"describe"}}},"then":{"not":{"required":["arguments"]}}}
  ]
}`)

var configSchema = json.RawMessage(`{
  "type":"object",
  "description":"Gerencia a configuração de camadas e bindings no escopo global ou no workspace atual. Toda mutação exige expectedRevision e expectedFingerprint; o backend continua autoridade para validação, decisão e persistência.",
  "properties":{
    "scope":{"type":"string","enum":["global","workspace"],"description":"Escopo da operação. workspace sempre significa o workspace autenticado atual."},
    "action":{"type":"string","enum":["layer_list","layer_get","layer_create","layer_update","layer_delete","layer_enable","layer_disable","layer_restore","binding_list","binding_check_conflict","binding_create","binding_update","binding_delete","binding_enable","binding_disable","binding_restore","config_import","config_export"],"description":"Ação fechada de leitura, mutação ou portabilidade."},
    "id":{"type":"string","description":"ID real da camada ou binding para ações que operam um recurso existente."},
    "layer_id":{"type":"string","description":"Filtra bindings por camada."},
    "locale":{"type":"string","description":"Locale usado ao renderizar o diff de uma mutação."},
    "payload":{
      "type":"object",
      "description":"Para CRUD, use expectedRevision/expectedFingerprint e o objeto layer ou binding apropriado. Para import/export, use jsonData/resolutions ou includeCredentials=false.",
      "additionalProperties":false,
      "properties":{
        "expectedRevision":{"type":"integer","minimum":1,"description":"Revisão observada do escopo antes da mutação."},
        "expectedFingerprint":{"type":"string","minLength":1,"description":"Fingerprint observado junto da revisão."},
        "layerRefKind":{"type":"string","enum":["user","builtin"],"description":"Proveniência da camada referenciada por um binding."},
        "layer":{
          "type":"object",
          "description":"Dados da camada a criar ou atualizar.",
          "additionalProperties":false,
          "properties":{
            "id":{"type":"string"},
            "name":{"type":"string"},
            "description":{"type":"string"},
            "enabled":{"type":"boolean"},
            "resolutionPriority":{"type":"integer"}
          },
          "required":["name","description","enabled","resolutionPriority"]
        },
        "binding":{
          "type":"object",
          "description":"Dados do binding a criar ou atualizar.",
          "additionalProperties":false,
          "properties":{
            "id":{"type":"string"},
            "layerId":{"type":"string"},
            "commandId":{"type":"string"},
            "triggerType":{"type":"string"},
            "triggerSpec":{"type":"string"},
            "arguments":{"type":"object","additionalProperties":true},
            "condition":{
              "type":"object",
              "additionalProperties":false,
              "properties":{
                "version":{"type":"integer"},
                "clauses":{"type":"array","items":{
                  "type":"object",
                  "additionalProperties":false,
                  "properties":{"field":{"type":"string"},"op":{"type":"string"},"value":{}},
                  "required":["field","op","value"]
                }}
              },
              "required":["version","clauses"]
            },
            "effect":{"type":"string"},
            "enabled":{"type":"boolean"},
            "resolutionPriority":{"type":"integer"},
            "replacesDefaultId":{"type":"string"},
            "replacesDefaultVersion":{"type":"string"},
            "replacesDefaultFingerprint":{"type":"string"},
            "presentation":{"type":"object","additionalProperties":true}
          },
          "required":["layerId","triggerType","triggerSpec","effect","enabled","resolutionPriority"]
        },
        "jsonData":{"type":"string","description":"Envelope JSON de command_layers para config_import."},
        "resolutions":{
          "type":"array",
          "description":"Inclua uma decisão commandLayers para o conjunto inteiro: identifier deve ser * e strategy deve ser skip, overwrite ou rename. Use commandLayerName somente para renomear uma camada cujo ID aparece no JSONData importado; use commandWorkspace somente para mapear um workspace de origem ao workspace atual autorizado. Não invente identificadores ou destinos: fora dessas referências o modelo não tem informação suficiente.",
          "minItems":1,
          "maxItems":64,
          "items":{
            "type":"object",
            "additionalProperties":false,
            "properties":{
              "resourceType":{"type":"string","enum":["commandLayers","commandLayerName","commandWorkspace"],"description":"commandLayers decide o conjunto; commandLayerName renomeia uma camada importada; commandWorkspace mapeia um workspace de origem para o workspace atual."},
              "identifier":{"type":"string","minLength":1,"description":"Use * para commandLayers; para os outros tipos, use somente um ID existente no JSONData importado."},
              "strategy":{"type":"string","enum":["skip","overwrite","rename"],"description":"commandLayers aceita skip, overwrite ou rename; commandLayerName e commandWorkspace aceitam somente rename."},
              "renameValue":{"type":"string","minLength":1,"description":"Obrigatório para renomear camada/workspace; é o destino autorizado pelo backend, não um workspace arbitrário."}
            },
            "required":["resourceType","identifier","strategy"],
            "allOf":[
              {"if":{"properties":{"resourceType":{"const":"commandLayers"}}},"then":{"properties":{"identifier":{"const":"*"}},"not":{"required":["renameValue"]}}},
              {"if":{"properties":{"resourceType":{"enum":["commandLayerName","commandWorkspace"]}}},"then":{"required":["renameValue"],"properties":{"strategy":{"const":"rename"}}}}
            ]
          }
        },
        "includeCredentials":{"type":"boolean","const":false,"description":"Deve ser false. Exportações sensíveis não são permitidas pela tool de chat."}
      }
    }
  },
  "required":["scope","action"],
  "additionalProperties":false,
  "allOf":[
    {"if":{"properties":{"action":{"enum":["layer_create","layer_update","layer_delete","layer_enable","layer_disable","layer_restore","binding_create","binding_update","binding_delete","binding_enable","binding_disable","binding_restore"]}}},"then":{"required":["payload"],"properties":{"payload":{"required":["expectedRevision","expectedFingerprint"]}}}},
    {"if":{"properties":{"action":{"enum":["layer_create","layer_update"]}}},"then":{"properties":{"payload":{"required":["layer"]}}}},
    {"if":{"properties":{"action":{"enum":["binding_create","binding_update"]}}},"then":{"properties":{"payload":{"required":["binding"]}}}},
    {"if":{"properties":{"action":{"const":"layer_restore"}}},"then":{"properties":{"payload":{"required":["layerRefKind"]}}}},
    {"if":{"properties":{"action":{"const":"config_import"}}},"then":{"required":["payload"],"properties":{"payload":{"required":["jsonData"]}}}},
    {"if":{"properties":{"action":{"enum":["layer_get","layer_update","layer_delete","layer_enable","layer_disable","layer_restore","binding_update","binding_delete","binding_enable","binding_disable","binding_restore"]}}},"then":{"required":["id"]}}
  ]
}`)

func cloneSchema(schema json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), schema...)
}

var _ tools.Tool = (*Catalog)(nil)
var _ tools.Tool = (*Config)(nil)
