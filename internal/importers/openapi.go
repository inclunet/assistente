package importers

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// OpenAPISpec representa uma especificação OpenAPI/Swagger simplificada
type OpenAPISpec struct {
	OpenAPI      string                 `json:"openapi" yaml:"openapi"` // 3.x
	Swagger      string                 `json:"swagger" yaml:"swagger"` // 2.0
	Info         OpenAPIInfo            `json:"info" yaml:"info"`
	Servers      []OpenAPIServer        `json:"servers" yaml:"servers"`
	Host         string                 `json:"host" yaml:"host"`         // Swagger 2.0
	BasePath     string                 `json:"basePath" yaml:"basePath"` // Swagger 2.0
	Schemes      []string               `json:"schemes" yaml:"schemes"`   // Swagger 2.0
	Paths        map[string]PathItem    `json:"paths" yaml:"paths"`
	Components   OpenAPIComponents      `json:"components" yaml:"components"`                   // OpenAPI 3.x
	SecurityDefs map[string]SecurityDef `json:"securityDefinitions" yaml:"securityDefinitions"` // Swagger 2.0
}

// OpenAPIInfo contém metadados da API
type OpenAPIInfo struct {
	Title       string `json:"title" yaml:"title"`
	Description string `json:"description" yaml:"description"`
	Version     string `json:"version" yaml:"version"`
}

// OpenAPIServer representa um servidor
type OpenAPIServer struct {
	URL         string `json:"url" yaml:"url"`
	Description string `json:"description" yaml:"description"`
}

// PathItem representa os métodos de um path
type PathItem struct {
	Get     *Operation `json:"get" yaml:"get"`
	Post    *Operation `json:"post" yaml:"post"`
	Put     *Operation `json:"put" yaml:"put"`
	Delete  *Operation `json:"delete" yaml:"delete"`
	Patch   *Operation `json:"patch" yaml:"patch"`
	Options *Operation `json:"options" yaml:"options"`
	Head    *Operation `json:"head" yaml:"head"`
}

// Operation representa uma operação HTTP
type Operation struct {
	OperationID string              `json:"operationId" yaml:"operationId"`
	Summary     string              `json:"summary" yaml:"summary"`
	Description string              `json:"description" yaml:"description"`
	Tags        []string            `json:"tags" yaml:"tags"`
	Parameters  []Parameter         `json:"parameters" yaml:"parameters"`
	RequestBody *RequestBody        `json:"requestBody" yaml:"requestBody"` // OpenAPI 3.x
	Responses   map[string]Response `json:"responses" yaml:"responses"`
	Deprecated  bool                `json:"deprecated" yaml:"deprecated"`
}

// Parameter representa um parâmetro da operação
type Parameter struct {
	Name        string                 `json:"name" yaml:"name"`
	In          string                 `json:"in" yaml:"in"` // path, query, header, cookie
	Description string                 `json:"description" yaml:"description"`
	Required    bool                   `json:"required" yaml:"required"`
	Schema      map[string]interface{} `json:"schema" yaml:"schema"` // OpenAPI 3.x
	Type        string                 `json:"type" yaml:"type"`     // Swagger 2.0
	Format      string                 `json:"format" yaml:"format"` // Swagger 2.0
	Enum        []interface{}          `json:"enum" yaml:"enum"`
	Default     interface{}            `json:"default" yaml:"default"`
}

// RequestBody representa o corpo da requisição (OpenAPI 3.x)
type RequestBody struct {
	Description string               `json:"description" yaml:"description"`
	Required    bool                 `json:"required" yaml:"required"`
	Content     map[string]MediaType `json:"content" yaml:"content"`
}

// MediaType representa um tipo de mídia
type MediaType struct {
	Schema map[string]interface{} `json:"schema" yaml:"schema"`
}

// Response representa uma resposta
type Response struct {
	Description string               `json:"description" yaml:"description"`
	Content     map[string]MediaType `json:"content" yaml:"content"`
}

// OpenAPIComponents contém componentes reutilizáveis
type OpenAPIComponents struct {
	Schemas         map[string]map[string]interface{} `json:"schemas" yaml:"schemas"`
	SecuritySchemes map[string]SecurityDef            `json:"securitySchemes" yaml:"securitySchemes"`
}

// SecurityDef representa uma definição de segurança
type SecurityDef struct {
	Type   string `json:"type" yaml:"type"`     // apiKey, http, oauth2
	Name   string `json:"name" yaml:"name"`     // Nome do header/query param
	In     string `json:"in" yaml:"in"`         // header, query
	Scheme string `json:"scheme" yaml:"scheme"` // bearer, basic
}

// OpenAPIImportResult representa o resultado da importação
type OpenAPIImportResult struct {
	DisplayName    string                  `json:"display_name"`
	Description    string                  `json:"description"`
	BaseURL        string                  `json:"base_url"`
	AuthType       string                  `json:"auth_type"`
	AuthConfig     map[string]string       `json:"auth_config"`
	DefaultHeaders map[string]string       `json:"default_headers"`
	Endpoints      []OpenAPIEndpointResult `json:"endpoints"`
}

// OpenAPIEndpointResult representa um endpoint extraído
type OpenAPIEndpointResult struct {
	Name             string                 `json:"name"`
	Description      string                 `json:"description"`
	Method           string                 `json:"method"`
	PathTemplate     string                 `json:"path_template"`
	QueryTemplate    string                 `json:"query_template"`
	HeadersJSON      string                 `json:"headers_json"`
	BodyTemplate     string                 `json:"body_template"`
	Parameters       map[string]interface{} `json:"parameters"`
	ResponseTemplate string                 `json:"response_template"`
}

// OpenAPIParser parseia especificações OpenAPI/Swagger
type OpenAPIParser struct{}

// NewOpenAPIParser cria um novo parser
func NewOpenAPIParser() *OpenAPIParser {
	return &OpenAPIParser{}
}

// Parse analisa uma especificação OpenAPI/Swagger
func (p *OpenAPIParser) Parse(content string) (*OpenAPIImportResult, error) {
	var spec OpenAPISpec

	// Tenta parsear como JSON primeiro, depois YAML
	err := json.Unmarshal([]byte(content), &spec)
	if err != nil {
		err = yaml.Unmarshal([]byte(content), &spec)
		if err != nil {
			return nil, fmt.Errorf("erro ao parsear especificação: formato inválido (não é JSON nem YAML)")
		}
	}

	// Valida versão
	if spec.OpenAPI == "" && spec.Swagger == "" {
		return nil, fmt.Errorf("especificação inválida: campo 'openapi' ou 'swagger' não encontrado")
	}

	return p.convertToResult(&spec)
}

// convertToResult converte a especificação para o formato de resultado
func (p *OpenAPIParser) convertToResult(spec *OpenAPISpec) (*OpenAPIImportResult, error) {
	result := &OpenAPIImportResult{
		DisplayName:    spec.Info.Title,
		Description:    spec.Info.Description,
		DefaultHeaders: make(map[string]string),
		AuthConfig:     make(map[string]string),
		Endpoints:      make([]OpenAPIEndpointResult, 0),
	}

	// Determina base URL
	result.BaseURL = p.extractBaseURL(spec)

	// Extrai configuração de autenticação
	p.extractAuth(spec, result)

	// Header padrão
	result.DefaultHeaders["Content-Type"] = "application/json"

	// Processa cada path
	for path, pathItem := range spec.Paths {
		p.processPathItem(path, &pathItem, result)
	}

	return result, nil
}

// extractBaseURL extrai a URL base
func (p *OpenAPIParser) extractBaseURL(spec *OpenAPISpec) string {
	// OpenAPI 3.x
	if len(spec.Servers) > 0 {
		return strings.TrimSuffix(spec.Servers[0].URL, "/")
	}

	// Swagger 2.0
	if spec.Host != "" {
		scheme := "https"
		if len(spec.Schemes) > 0 {
			scheme = spec.Schemes[0]
		}
		baseURL := fmt.Sprintf("%s://%s", scheme, spec.Host)
		if spec.BasePath != "" {
			baseURL += strings.TrimSuffix(spec.BasePath, "/")
		}
		return baseURL
	}

	return "https://api.example.com"
}

// extractAuth extrai configuração de autenticação
func (p *OpenAPIParser) extractAuth(spec *OpenAPISpec, result *OpenAPIImportResult) {
	// OpenAPI 3.x
	if spec.Components.SecuritySchemes != nil {
		for name, scheme := range spec.Components.SecuritySchemes {
			p.processSecurityScheme(name, &scheme, result)
			break // Usa apenas o primeiro
		}
	}

	// Swagger 2.0
	if spec.SecurityDefs != nil {
		for name, scheme := range spec.SecurityDefs {
			p.processSecurityScheme(name, &scheme, result)
			break
		}
	}
}

// processSecurityScheme processa um esquema de segurança
func (p *OpenAPIParser) processSecurityScheme(name string, scheme *SecurityDef, result *OpenAPIImportResult) {
	switch scheme.Type {
	case "apiKey":
		result.AuthType = "api_key"
		if scheme.In == "header" {
			result.AuthConfig["header_name"] = scheme.Name
			result.AuthConfig["location"] = "header"
		} else {
			result.AuthConfig["param_name"] = scheme.Name
			result.AuthConfig["location"] = "query"
		}
		result.AuthConfig["value_env"] = strings.ToUpper(strings.ReplaceAll(name, "-", "_")) + "_KEY"

	case "http":
		if scheme.Scheme == "bearer" {
			result.AuthType = "bearer"
			result.AuthConfig["token_env"] = "API_TOKEN"
		} else if scheme.Scheme == "basic" {
			result.AuthType = "basic"
			result.AuthConfig["username_env"] = "API_USERNAME"
			result.AuthConfig["password_env"] = "API_PASSWORD"
		}

	case "oauth2":
		// OAuth2 será implementado separadamente
		result.AuthType = "bearer"
		result.AuthConfig["token_env"] = "OAUTH_TOKEN"
	}
}

// processPathItem processa todos os métodos de um path
func (p *OpenAPIParser) processPathItem(path string, pathItem *PathItem, result *OpenAPIImportResult) {
	if pathItem.Get != nil {
		endpoint := p.createEndpoint(path, "GET", pathItem.Get)
		result.Endpoints = append(result.Endpoints, endpoint)
	}
	if pathItem.Post != nil {
		endpoint := p.createEndpoint(path, "POST", pathItem.Post)
		result.Endpoints = append(result.Endpoints, endpoint)
	}
	if pathItem.Put != nil {
		endpoint := p.createEndpoint(path, "PUT", pathItem.Put)
		result.Endpoints = append(result.Endpoints, endpoint)
	}
	if pathItem.Delete != nil {
		endpoint := p.createEndpoint(path, "DELETE", pathItem.Delete)
		result.Endpoints = append(result.Endpoints, endpoint)
	}
	if pathItem.Patch != nil {
		endpoint := p.createEndpoint(path, "PATCH", pathItem.Patch)
		result.Endpoints = append(result.Endpoints, endpoint)
	}
}

// createEndpoint cria um endpoint a partir de uma operação
func (p *OpenAPIParser) createEndpoint(path, method string, op *Operation) OpenAPIEndpointResult {
	endpoint := OpenAPIEndpointResult{
		Method:      method,
		Description: p.getDescription(op),
		Parameters:  make(map[string]interface{}),
	}

	// Gera nome da operação
	endpoint.Name = p.generateOperationName(op, method, path)

	// Converte path do formato OpenAPI para Go template
	// /users/{user_id} -> /users/{{.user_id}}
	endpoint.PathTemplate = p.convertPathToTemplate(path)

	// Processa parâmetros
	properties := make(map[string]interface{})
	required := make([]string, 0)
	var queryParams []string

	for _, param := range op.Parameters {
		paramSchema := p.parameterToSchema(&param)
		properties[param.Name] = paramSchema

		if param.Required {
			required = append(required, param.Name)
		}

		// Coleta parâmetros de query
		if param.In == "query" {
			queryParams = append(queryParams, fmt.Sprintf("%s={{.%s}}", param.Name, param.Name))
		}
	}

	// Processa request body (OpenAPI 3.x)
	if op.RequestBody != nil && (method == "POST" || method == "PUT" || method == "PATCH") {
		bodySchema := p.extractBodySchema(op.RequestBody)
		if bodySchema != nil {
			// Mescla propriedades do body
			if props, ok := bodySchema["properties"].(map[string]interface{}); ok {
				for k, v := range props {
					properties[k] = v
				}
			}
			// Mescla required
			if reqd, ok := bodySchema["required"].([]interface{}); ok {
				for _, r := range reqd {
					if s, ok := r.(string); ok {
						required = append(required, s)
					}
				}
			}

			// Gera body template
			endpoint.BodyTemplate = p.generateBodyTemplate(bodySchema)
		}
	}

	// Monta query template
	if len(queryParams) > 0 {
		endpoint.QueryTemplate = strings.Join(queryParams, "&")
	}

	// Monta JSON Schema final
	endpoint.Parameters = map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		endpoint.Parameters["required"] = required
	}

	return endpoint
}

// getDescription obtém a descrição da operação
func (p *OpenAPIParser) getDescription(op *Operation) string {
	if op.Summary != "" {
		return op.Summary
	}
	if op.Description != "" {
		// Limita tamanho
		if len(op.Description) > 200 {
			return op.Description[:197] + "..."
		}
		return op.Description
	}
	return ""
}

// generateOperationName gera um nome para a operação
func (p *OpenAPIParser) generateOperationName(op *Operation, method, path string) string {
	// Usa operationId se disponível
	if op.OperationID != "" {
		return p.sanitizeName(op.OperationID)
	}

	// Gera a partir do método e path
	// GET /users/{id} -> get_user
	// POST /users -> create_user

	// Remove parâmetros do path
	cleanPath := path
	cleanPath = strings.ReplaceAll(cleanPath, "{", "")
	cleanPath = strings.ReplaceAll(cleanPath, "}", "")

	// Pega última parte significativa
	parts := strings.Split(strings.Trim(cleanPath, "/"), "/")
	resource := "resource"
	if len(parts) > 0 {
		for i := len(parts) - 1; i >= 0; i-- {
			if parts[i] != "" && !strings.HasPrefix(parts[i], "_") {
				resource = parts[i]
				break
			}
		}
	}

	// Gera prefixo baseado no método
	prefix := strings.ToLower(method)
	switch method {
	case "GET":
		if strings.Contains(path, "{") {
			prefix = "get"
		} else {
			prefix = "list"
		}
	case "POST":
		prefix = "create"
	case "PUT", "PATCH":
		prefix = "update"
	case "DELETE":
		prefix = "delete"
	}

	return p.sanitizeName(prefix + "_" + resource)
}

// sanitizeName limpa o nome para ser um identificador válido
func (p *OpenAPIParser) sanitizeName(name string) string {
	// Converte para snake_case
	name = strings.ToLower(name)

	// Substitui caracteres inválidos
	replacer := strings.NewReplacer(
		"-", "_",
		" ", "_",
		".", "_",
		"/", "_",
	)
	name = replacer.Replace(name)

	// Remove underscores duplicados
	for strings.Contains(name, "__") {
		name = strings.ReplaceAll(name, "__", "_")
	}

	// Remove underscores do início e fim
	name = strings.Trim(name, "_")

	return name
}

// convertPathToTemplate converte path OpenAPI para Go template
func (p *OpenAPIParser) convertPathToTemplate(path string) string {
	// /users/{user_id}/posts/{post_id} -> /users/{{.user_id}}/posts/{{.post_id}}
	result := path

	// Usa regex para substituir {param} por {{.param}}
	// Isso evita problemas com loop infinito
	re := regexp.MustCompile(`\{([^}]+)\}`)
	result = re.ReplaceAllString(result, "{{.$1}}")

	return result
}

// parameterToSchema converte um parâmetro para JSON Schema
func (p *OpenAPIParser) parameterToSchema(param *Parameter) map[string]interface{} {
	schema := make(map[string]interface{})

	// OpenAPI 3.x
	if param.Schema != nil {
		for k, v := range param.Schema {
			schema[k] = v
		}
	} else {
		// Swagger 2.0
		if param.Type != "" {
			schema["type"] = param.Type
		}
		if param.Format != "" {
			schema["format"] = param.Format
		}
	}

	if param.Description != "" {
		schema["description"] = param.Description
	}
	if len(param.Enum) > 0 {
		schema["enum"] = param.Enum
	}
	if param.Default != nil {
		schema["default"] = param.Default
	}

	// Garante que tem type
	if _, ok := schema["type"]; !ok {
		schema["type"] = "string"
	}

	return schema
}

// extractBodySchema extrai o schema do request body
func (p *OpenAPIParser) extractBodySchema(body *RequestBody) map[string]interface{} {
	if body.Content == nil {
		return nil
	}

	// Prefere application/json
	if mediaType, ok := body.Content["application/json"]; ok {
		return mediaType.Schema
	}

	// Usa o primeiro disponível
	for _, mediaType := range body.Content {
		return mediaType.Schema
	}

	return nil
}

// generateBodyTemplate gera um template de body a partir do schema
func (p *OpenAPIParser) generateBodyTemplate(schema map[string]interface{}) string {
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		return "{}"
	}

	bodyObj := make(map[string]string)
	for propName := range props {
		bodyObj[propName] = "{{." + propName + "}}"
	}

	bytes, err := json.MarshalIndent(bodyObj, "", "  ")
	if err != nil {
		return "{}"
	}

	// Remove as aspas extras dos placeholders
	result := string(bytes)
	for propName := range props {
		result = strings.ReplaceAll(result, `"{{.`+propName+`}}"`, `{{.`+propName+` | jsonEncode}}`)
	}

	return result
}






