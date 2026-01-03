package importers

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// PostmanCollection representa uma coleção do Postman
type PostmanCollection struct {
	Info     PostmanInfo       `json:"info"`
	Item     []PostmanItem     `json:"item"`
	Auth     *PostmanAuth      `json:"auth"`
	Variable []PostmanVariable `json:"variable"`
}

// PostmanInfo contém metadados da coleção
type PostmanInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Schema      string `json:"schema"`
}

// PostmanItem representa um item (request ou folder)
type PostmanItem struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Item        []PostmanItem   `json:"item,omitempty"`    // Se for folder
	Request     *PostmanRequest `json:"request,omitempty"` // Se for request
}

// PostmanRequest representa uma requisição
type PostmanRequest struct {
	Method      string          `json:"method"`
	Header      []PostmanHeader `json:"header"`
	Body        *PostmanBody    `json:"body"`
	URL         PostmanURL      `json:"url"`
	Description string          `json:"description"`
	Auth        *PostmanAuth    `json:"auth"`
}

// PostmanURL representa a URL da requisição
type PostmanURL struct {
	Raw      string            `json:"raw"`
	Protocol string            `json:"protocol"`
	Host     []string          `json:"host"`
	Port     string            `json:"port"`
	Path     []string          `json:"path"`
	Query    []PostmanQuery    `json:"query"`
	Variable []PostmanVariable `json:"variable"`
}

// PostmanQuery representa um parâmetro de query
type PostmanQuery struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description"`
	Disabled    bool   `json:"disabled"`
}

// PostmanVariable representa uma variável
type PostmanVariable struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description"`
	Type        string `json:"type"`
}

// PostmanHeader representa um header
type PostmanHeader struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description"`
	Disabled    bool   `json:"disabled"`
}

// PostmanBody representa o corpo da requisição
type PostmanBody struct {
	Mode       string              `json:"mode"` // raw, formdata, urlencoded
	Raw        string              `json:"raw"`
	Options    *PostmanBodyOptions `json:"options"`
	FormData   []PostmanFormData   `json:"formdata"`
	URLEncoded []PostmanFormData   `json:"urlencoded"`
}

// PostmanBodyOptions opções do body
type PostmanBodyOptions struct {
	Raw PostmanRawOptions `json:"raw"`
}

// PostmanRawOptions opções para body raw
type PostmanRawOptions struct {
	Language string `json:"language"`
}

// PostmanFormData representa um campo de form
type PostmanFormData struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Disabled    bool   `json:"disabled"`
}

// PostmanAuth representa autenticação
type PostmanAuth struct {
	Type   string      `json:"type"` // apikey, bearer, basic, oauth2
	APIKey []PostmanKV `json:"apikey"`
	Bearer []PostmanKV `json:"bearer"`
	Basic  []PostmanKV `json:"basic"`
}

// PostmanKV representa um par chave-valor
type PostmanKV struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Type  string `json:"type"`
}

// PostmanImportResult representa o resultado da importação
type PostmanImportResult struct {
	DisplayName    string                  `json:"display_name"`
	Description    string                  `json:"description"`
	BaseURL        string                  `json:"base_url"`
	AuthType       string                  `json:"auth_type"`
	AuthConfig     map[string]string       `json:"auth_config"`
	DefaultHeaders map[string]string       `json:"default_headers"`
	Endpoints      []PostmanEndpointResult `json:"endpoints"`
	Variables      map[string]string       `json:"variables"`
}

// PostmanEndpointResult representa um endpoint extraído
type PostmanEndpointResult struct {
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

// PostmanParser parseia coleções do Postman
type PostmanParser struct{}

// NewPostmanParser cria um novo parser
func NewPostmanParser() *PostmanParser {
	return &PostmanParser{}
}

// Parse analisa uma coleção Postman
func (p *PostmanParser) Parse(content string) (*PostmanImportResult, error) {
	var collection PostmanCollection

	err := json.Unmarshal([]byte(content), &collection)
	if err != nil {
		return nil, fmt.Errorf("erro ao parsear coleção Postman: %w", err)
	}

	// Valida
	if collection.Info.Name == "" {
		return nil, fmt.Errorf("coleção inválida: campo 'info.name' não encontrado")
	}

	return p.convertToResult(&collection)
}

// convertToResult converte a coleção para o formato de resultado
func (p *PostmanParser) convertToResult(collection *PostmanCollection) (*PostmanImportResult, error) {
	result := &PostmanImportResult{
		DisplayName:    collection.Info.Name,
		Description:    collection.Info.Description,
		DefaultHeaders: make(map[string]string),
		AuthConfig:     make(map[string]string),
		Variables:      make(map[string]string),
		Endpoints:      make([]PostmanEndpointResult, 0),
	}

	// Extrai variáveis da coleção
	for _, v := range collection.Variable {
		result.Variables[v.Key] = v.Value
	}

	// Extrai autenticação da coleção
	if collection.Auth != nil {
		p.extractAuth(collection.Auth, result)
	}

	// Header padrão
	result.DefaultHeaders["Content-Type"] = "application/json"

	// Processa todos os itens recursivamente
	baseURL := ""
	p.processItems(collection.Item, result, &baseURL, "")

	// Define base URL
	if baseURL != "" {
		result.BaseURL = baseURL
	} else {
		result.BaseURL = "{{.env.API_BASE_URL}}"
	}

	return result, nil
}

// extractAuth extrai configuração de autenticação
func (p *PostmanParser) extractAuth(auth *PostmanAuth, result *PostmanImportResult) {
	switch auth.Type {
	case "apikey":
		result.AuthType = "api_key"
		for _, kv := range auth.APIKey {
			switch kv.Key {
			case "key":
				result.AuthConfig["header_name"] = kv.Value
			case "value":
				// Se parece com variável {{xxx}}, usa como env
				if strings.HasPrefix(kv.Value, "{{") {
					varName := strings.Trim(kv.Value, "{}")
					result.AuthConfig["value_env"] = strings.ToUpper(varName)
				} else {
					result.AuthConfig["value"] = kv.Value
				}
			case "in":
				result.AuthConfig["location"] = kv.Value
			}
		}

	case "bearer":
		result.AuthType = "bearer"
		for _, kv := range auth.Bearer {
			if kv.Key == "token" {
				if strings.HasPrefix(kv.Value, "{{") {
					varName := strings.Trim(kv.Value, "{}")
					result.AuthConfig["token_env"] = strings.ToUpper(varName)
				} else {
					result.AuthConfig["token"] = kv.Value
				}
			}
		}

	case "basic":
		result.AuthType = "basic"
		for _, kv := range auth.Basic {
			switch kv.Key {
			case "username":
				if strings.HasPrefix(kv.Value, "{{") {
					varName := strings.Trim(kv.Value, "{}")
					result.AuthConfig["username_env"] = strings.ToUpper(varName)
				} else {
					result.AuthConfig["username"] = kv.Value
				}
			case "password":
				if strings.HasPrefix(kv.Value, "{{") {
					varName := strings.Trim(kv.Value, "{}")
					result.AuthConfig["password_env"] = strings.ToUpper(varName)
				} else {
					result.AuthConfig["password"] = kv.Value
				}
			}
		}
	}
}

// processItems processa itens recursivamente
func (p *PostmanParser) processItems(items []PostmanItem, result *PostmanImportResult, baseURL *string, prefix string) {
	for _, item := range items {
		if item.Request != nil {
			// É uma requisição
			endpoint := p.createEndpoint(&item, prefix)
			result.Endpoints = append(result.Endpoints, endpoint)

			// Extrai base URL da primeira requisição
			if *baseURL == "" {
				*baseURL = p.extractBaseURL(item.Request)
			}
		} else if len(item.Item) > 0 {
			// É uma pasta, processa recursivamente
			newPrefix := item.Name
			if prefix != "" {
				newPrefix = prefix + "_" + item.Name
			}
			p.processItems(item.Item, result, baseURL, newPrefix)
		}
	}
}

// extractBaseURL extrai a base URL de uma requisição
func (p *PostmanParser) extractBaseURL(req *PostmanRequest) string {
	if req.URL.Protocol != "" && len(req.URL.Host) > 0 {
		baseURL := req.URL.Protocol + "://" + strings.Join(req.URL.Host, ".")
		if req.URL.Port != "" {
			baseURL += ":" + req.URL.Port
		}
		return p.convertPostmanVariables(baseURL)
	}

	// Tenta extrair da URL raw
	if req.URL.Raw != "" {
		parsed, err := url.Parse(req.URL.Raw)
		if err == nil && parsed.Host != "" {
			return parsed.Scheme + "://" + parsed.Host
		}
	}

	return ""
}

// createEndpoint cria um endpoint a partir de um item Postman
func (p *PostmanParser) createEndpoint(item *PostmanItem, prefix string) PostmanEndpointResult {
	req := item.Request
	endpoint := PostmanEndpointResult{
		Method:     req.Method,
		Parameters: make(map[string]interface{}),
	}

	// Nome do endpoint
	endpoint.Name = p.generateName(item.Name, prefix)

	// Descrição
	endpoint.Description = item.Description
	if endpoint.Description == "" && req.Description != "" {
		endpoint.Description = req.Description
	}

	// Path template
	endpoint.PathTemplate = p.buildPathTemplate(req)

	// Parâmetros e query template
	properties := make(map[string]interface{})
	required := make([]string, 0)
	var queryParams []string

	// Extrai parâmetros do path (variáveis :xxx ou {{xxx}})
	pathParams := p.extractPathParameters(endpoint.PathTemplate)
	for _, param := range pathParams {
		properties[param] = map[string]interface{}{
			"type":        "string",
			"description": fmt.Sprintf("Parâmetro de path: %s", param),
		}
		required = append(required, param)
	}

	// Processa query parameters
	for _, q := range req.URL.Query {
		if q.Disabled {
			continue
		}

		properties[q.Key] = map[string]interface{}{
			"type":        "string",
			"description": q.Description,
		}

		// Cria query template
		queryParams = append(queryParams, fmt.Sprintf("%s={{.%s}}", q.Key, q.Key))
	}

	if len(queryParams) > 0 {
		endpoint.QueryTemplate = strings.Join(queryParams, "&")
	}

	// Headers específicos (não padrão)
	headers := make(map[string]string)
	for _, h := range req.Header {
		if h.Disabled {
			continue
		}
		// Ignora headers comuns
		if strings.EqualFold(h.Key, "Content-Type") || strings.EqualFold(h.Key, "Authorization") {
			continue
		}
		headers[h.Key] = p.convertPostmanVariables(h.Value)
	}
	if len(headers) > 0 {
		headersJSON, _ := json.Marshal(headers)
		endpoint.HeadersJSON = string(headersJSON)
	}

	// Body template
	if req.Body != nil && (req.Method == "POST" || req.Method == "PUT" || req.Method == "PATCH") {
		bodyTemplate, bodyProps := p.processBody(req.Body)
		endpoint.BodyTemplate = bodyTemplate

		// Adiciona propriedades do body
		for k, v := range bodyProps {
			properties[k] = v
		}
	}

	// Monta JSON Schema
	endpoint.Parameters = map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		endpoint.Parameters["required"] = required
	}

	return endpoint
}

// generateName gera um nome para o endpoint
func (p *PostmanParser) generateName(name, prefix string) string {
	result := name
	if prefix != "" {
		result = prefix + "_" + name
	}

	// Converte para snake_case
	result = strings.ToLower(result)

	// Substitui caracteres inválidos
	replacer := strings.NewReplacer(
		"-", "_",
		" ", "_",
		".", "_",
		"/", "_",
		"(", "",
		")", "",
	)
	result = replacer.Replace(result)

	// Remove underscores duplicados
	for strings.Contains(result, "__") {
		result = strings.ReplaceAll(result, "__", "_")
	}

	// Remove underscores do início e fim
	result = strings.Trim(result, "_")

	return result
}

// buildPathTemplate constrói o path template
func (p *PostmanParser) buildPathTemplate(req *PostmanRequest) string {
	var path string

	if len(req.URL.Path) > 0 {
		path = "/" + strings.Join(req.URL.Path, "/")
	} else if req.URL.Raw != "" {
		parsed, err := url.Parse(req.URL.Raw)
		if err == nil {
			path = parsed.Path
		}
	}

	if path == "" {
		path = "/"
	}

	// Converte variáveis Postman para Go template
	path = p.convertPostmanVariables(path)

	// Converte :param para {{.param}}
	re := regexp.MustCompile(`:([a-zA-Z_][a-zA-Z0-9_]*)`)
	path = re.ReplaceAllString(path, "{{.$1}}")

	return path
}

// convertPostmanVariables converte variáveis {{xxx}} do Postman para Go template
func (p *PostmanParser) convertPostmanVariables(s string) string {
	// {{variable}} -> {{.variable}} ou {{.env.VARIABLE}}
	re := regexp.MustCompile(`\{\{([^}]+)\}\}`)
	return re.ReplaceAllStringFunc(s, func(match string) string {
		varName := strings.Trim(match, "{}")
		varName = strings.TrimSpace(varName)

		// Se já tem ponto, mantém
		if strings.Contains(varName, ".") {
			return "{{." + varName + "}}"
		}

		// Verifica se parece ser variável de ambiente
		if strings.ToUpper(varName) == varName {
			return "{{.env." + varName + "}}"
		}

		return "{{." + varName + "}}"
	})
}

// extractPathParameters extrai nomes de parâmetros do path
func (p *PostmanParser) extractPathParameters(path string) []string {
	var params []string

	// Encontra {{.xxx}}
	re := regexp.MustCompile(`\{\{\.([a-zA-Z_][a-zA-Z0-9_]*)\}\}`)
	matches := re.FindAllStringSubmatch(path, -1)

	for _, match := range matches {
		if len(match) > 1 {
			// Ignora env.XXX
			if !strings.HasPrefix(match[1], "env.") {
				params = append(params, match[1])
			}
		}
	}

	return params
}

// processBody processa o body da requisição
func (p *PostmanParser) processBody(body *PostmanBody) (string, map[string]interface{}) {
	properties := make(map[string]interface{})

	switch body.Mode {
	case "raw":
		template := p.convertPostmanVariables(body.Raw)

		// Tenta extrair propriedades do JSON
		var jsonObj map[string]interface{}
		if err := json.Unmarshal([]byte(body.Raw), &jsonObj); err == nil {
			p.extractPropertiesFromJSON(jsonObj, "", properties)
		}

		return template, properties

	case "formdata", "urlencoded":
		var fields []PostmanFormData
		if body.Mode == "formdata" {
			fields = body.FormData
		} else {
			fields = body.URLEncoded
		}

		parts := make([]string, 0)
		for _, f := range fields {
			if f.Disabled {
				continue
			}
			properties[f.Key] = map[string]interface{}{
				"type":        "string",
				"description": f.Description,
			}
			parts = append(parts, fmt.Sprintf("%s={{.%s | urlEncode}}", f.Key, f.Key))
		}

		return strings.Join(parts, "&"), properties
	}

	return "", properties
}

// extractPropertiesFromJSON extrai propriedades de um objeto JSON
func (p *PostmanParser) extractPropertiesFromJSON(obj map[string]interface{}, prefix string, properties map[string]interface{}) {
	for key, value := range obj {
		propName := key
		if prefix != "" {
			propName = prefix + "_" + key
		}

		schema := map[string]interface{}{
			"description": fmt.Sprintf("Campo: %s", key),
		}

		switch v := value.(type) {
		case string:
			schema["type"] = "string"
			// Se é uma variável Postman, extrai o nome
			if strings.HasPrefix(v, "{{") && strings.HasSuffix(v, "}}") {
				varName := strings.Trim(v, "{}")
				propName = varName
			}
		case float64:
			if v == float64(int64(v)) {
				schema["type"] = "integer"
			} else {
				schema["type"] = "number"
			}
		case bool:
			schema["type"] = "boolean"
		case []interface{}:
			schema["type"] = "array"
		case map[string]interface{}:
			// Objeto aninhado, processa recursivamente
			p.extractPropertiesFromJSON(v, propName, properties)
			continue
		default:
			schema["type"] = "string"
		}

		properties[propName] = schema
	}
}






