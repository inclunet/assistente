package agentmanager

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"assistente/internal/llm"
	"assistente/internal/oauth"
)

// HTTPExecutor executa requisições HTTP com suporte a templates
type HTTPExecutor struct {
	client   *http.Client
	template *TemplateEngine
}

// HTTPExecutorConfig configuração do executor
type HTTPExecutorConfig struct {
	TimeoutSeconds int
	RetryCount     int
}

// HTTPRequest representa uma requisição HTTP a ser executada
type HTTPRequest struct {
	Method         string
	BaseURL        string
	PathTemplate   string
	QueryTemplate  string
	HeadersJSON    string
	BodyTemplate   string
	DefaultHeaders map[string]string
	AuthType       string
	AuthConfig     map[string]string
	EnvVars        map[string]string
}

// HTTPResponse representa a resposta de uma requisição HTTP
type HTTPResponse struct {
	StatusCode int               `json:"status_code"`
	Body       string            `json:"body"`
	Headers    map[string]string `json:"headers"`
	JSON       interface{}       `json:"json,omitempty"`
	Error      string            `json:"error,omitempty"`
}

// NewHTTPExecutor cria um novo executor HTTP
func NewHTTPExecutor(config HTTPExecutorConfig) *HTTPExecutor {
	timeout := config.TimeoutSeconds
	if timeout <= 0 {
		timeout = 30
	}

	return &HTTPExecutor{
		client:   llm.NewHTTPClientWithTimeout(time.Duration(timeout) * time.Second), // Usa pool compartilhado
		template: NewTemplateEngine(),
	}
}

// Execute executa uma requisição HTTP
func (e *HTTPExecutor) Execute(ctx context.Context, req HTTPRequest, params map[string]interface{}, agentName, displayName string) (*HTTPResponse, error) {
	// Cria o contexto do template
	tmplCtx := NewTemplateContext(params, req.EnvVars, agentName, displayName)

	fmt.Printf("  🌐 [HTTP] Params recebidos: %+v\n", params)

	// Processa a URL
	url, err := e.buildURL(req, tmplCtx)
	if err != nil {
		return nil, fmt.Errorf("erro ao construir URL: %w", err)
	}

	fmt.Printf("  🌐 [HTTP] URL construída: %s\n", url)

	// Processa o body (se houver)
	var body io.Reader
	if req.BodyTemplate != "" && (req.Method == "POST" || req.Method == "PUT" || req.Method == "PATCH") {
		bodyStr, err := e.template.Execute(req.BodyTemplate, tmplCtx)
		if err != nil {
			return nil, fmt.Errorf("erro ao processar body template: %w", err)
		}
		body = strings.NewReader(bodyStr)
	}

	// Cria a requisição
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, url, body)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar requisição: %w", err)
	}

	// Adiciona headers padrão
	for key, value := range req.DefaultHeaders {
		httpReq.Header.Set(key, value)
	}

	// Adiciona headers específicos do endpoint
	if req.HeadersJSON != "" {
		var headers map[string]string
		// Primeiro processa como template
		headersStr, err := e.template.Execute(req.HeadersJSON, tmplCtx)
		if err != nil {
			return nil, fmt.Errorf("erro ao processar headers template: %w", err)
		}
		if err := json.Unmarshal([]byte(headersStr), &headers); err != nil {
			return nil, fmt.Errorf("erro ao parsear headers JSON: %w", err)
		}
		for key, value := range headers {
			httpReq.Header.Set(key, value)
		}
	}

	// Adiciona autenticação
	if err := e.addAuth(httpReq, req.AuthType, req.AuthConfig, req.EnvVars); err != nil {
		return nil, fmt.Errorf("erro ao adicionar autenticação: %w", err)
	}

	// Executa a requisição
	resp, err := e.client.Do(httpReq)
	if err != nil {
		fmt.Printf("  ❌ [HTTP] Erro na requisição: %v\n", err)
		return &HTTPResponse{
			Error: fmt.Sprintf("erro na requisição: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	fmt.Printf("  ✅ [HTTP] Status: %d\n", resp.StatusCode)

	// Lê o body da resposta
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler resposta: %w", err)
	}

	fmt.Printf("  📄 [HTTP] Body: %s\n", string(respBody))

	// Constrói a resposta
	httpResp := &HTTPResponse{
		StatusCode: resp.StatusCode,
		Body:       string(respBody),
		Headers:    make(map[string]string),
	}

	// Copia headers da resposta
	for key := range resp.Header {
		httpResp.Headers[key] = resp.Header.Get(key)
	}

	// Tenta parsear como JSON
	var jsonBody interface{}
	if err := json.Unmarshal(respBody, &jsonBody); err == nil {
		httpResp.JSON = jsonBody
	}

	return httpResp, nil
}

// buildURL constrói a URL final a partir dos templates
func (e *HTTPExecutor) buildURL(req HTTPRequest, ctx *TemplateContext) (string, error) {
	// Processa base URL (pode ter template para multi-ambiente)
	baseURL, err := e.template.Execute(req.BaseURL, ctx)
	if err != nil {
		return "", fmt.Errorf("erro ao processar base URL: %w", err)
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	// Processa path template
	path, err := e.template.Execute(req.PathTemplate, ctx)
	if err != nil {
		return "", fmt.Errorf("erro ao processar path template: %w", err)
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	url := baseURL + path

	// Processa query template
	if req.QueryTemplate != "" {
		query, err := e.template.Execute(req.QueryTemplate, ctx)
		if err != nil {
			return "", fmt.Errorf("erro ao processar query template: %w", err)
		}
		if query != "" {
			url += "?" + query
		}
	}

	return url, nil
}

// addAuth adiciona autenticação à requisição
func (e *HTTPExecutor) addAuth(req *http.Request, authType string, authConfig, envVars map[string]string) error {
	switch authType {
	case "none", "":
		// Sem autenticação
		return nil

	case "bearer":
		token := e.resolveAuthValue(authConfig, "token", "token_env", envVars)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		return nil

	case "api_key":
		key := e.resolveAuthValue(authConfig, "value", "value_env", envVars)
		location := authConfig["location"]

		if location == "query" {
			paramName := authConfig["param_name"]
			if paramName == "" {
				paramName = "api_key"
			}
			// Adiciona à query string
			q := req.URL.Query()
			q.Set(paramName, key)
			req.URL.RawQuery = q.Encode()
		} else {
			// Default: header
			headerName := authConfig["header_name"]
			if headerName == "" {
				headerName = "X-API-Key"
			}
			req.Header.Set(headerName, key)
		}
		return nil

	case "basic":
		username := e.resolveAuthValue(authConfig, "username", "username_env", envVars)
		password := e.resolveAuthValue(authConfig, "password", "password_env", envVars)
		if username != "" {
			auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
			req.Header.Set("Authorization", "Basic "+auth)
		}
		return nil

	case "oauth2":
		// Configura cliente OAuth2
		oauth2Config := oauth.Config{
			GrantType:             authConfig["grant_type"],
			TokenURL:              authConfig["token_url"],
			AuthorizeURL:          authConfig["authorize_url"],
			ClientID:              authConfig["client_id"],
			ClientSecret:          authConfig["client_secret"],
			ClientIDEnv:           authConfig["client_id_env"],
			ClientSecretEnv:       authConfig["client_secret_env"],
			Audience:              authConfig["audience"],
			SendCredentialsInBody: authConfig["send_credentials_in_body"] == "true",
		}

		// Parse scopes
		if scopes := authConfig["scopes"]; scopes != "" {
			oauth2Config.Scopes = strings.Split(scopes, " ")
		}

		// Obtém token
		client := oauth.NewClient(oauth2Config, envVars)
		token, err := client.GetAccessToken(req.Context())
		if err != nil {
			return fmt.Errorf("erro ao obter token OAuth2: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+token)
		return nil

	default:
		return fmt.Errorf("tipo de autenticação não suportado: %s", authType)
	}
}

// resolveAuthValue resolve um valor de autenticação (direto ou via env var)
func (e *HTTPExecutor) resolveAuthValue(config map[string]string, directKey, envKey string, envVars map[string]string) string {
	// Primeiro tenta valor direto
	if value := config[directKey]; value != "" {
		return value
	}

	// Depois tenta via variável de ambiente
	if envVarName := config[envKey]; envVarName != "" {
		if value := envVars[envVarName]; value != "" {
			return value
		}
	}

	return ""
}

// FormatResponse formata a resposta usando um template
func (e *HTTPExecutor) FormatResponse(resp *HTTPResponse, responseTemplate string, params map[string]interface{}, agentName, displayName string) (string, error) {
	if responseTemplate == "" {
		// Sem template, retorna o body como está
		return resp.Body, nil
	}

	// Cria contexto combinando parâmetros originais com a resposta
	data := make(map[string]interface{})
	for k, v := range params {
		data[k] = v
	}

	// Adiciona campos da resposta
	if resp.JSON != nil {
		// Sempre adiciona a variável "response" com o JSON completo
		data["response"] = resp.JSON

		// Se a resposta é JSON map, também adiciona cada campo individualmente
		if jsonMap, ok := resp.JSON.(map[string]interface{}); ok {
			for k, v := range jsonMap {
				data[k] = v
			}
		}
	} else {
		data["response"] = resp.Body
	}
	data["status_code"] = resp.StatusCode

	ctx := NewTemplateContext(data, nil, agentName, displayName)
	return e.template.Execute(responseTemplate, ctx)
}
