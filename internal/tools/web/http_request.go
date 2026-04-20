package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"assistente/internal/credentials"
	"assistente/internal/tools"
	httpclient "assistente/internal/tools/http"
)

// HTTPRequest é uma ferramenta completa para requisições HTTP.
// Suporta todos os métodos HTTP, headers customizados, body, autenticação.
// Credenciais são resolvidas automaticamente pelo cliente HTTP centralizado.
// O modelo nunca vê ou passa credenciais.
type HTTPRequest struct {
	client            *httpclient.Client                                                // Cliente HTTP centralizado com auth/retry
	allowPrivateHosts bool                                                              // Para testes (padrão: false)
	confirmFn         func(ctx context.Context, method, url, body string) (bool, error) // Callback para confirmar operações destrutivas
}

// NewHTTPRequest cria uma nova instância de HTTPRequest.
func NewHTTPRequest(credMgr *credentials.Manager) *HTTPRequest {
	if credMgr == nil {
		credMgr = credentials.NewManager(nil) // Cria manager vazio se não fornecido
	}
	// Usar cliente HTTP centralizado com retry policy
	client := httpclient.New(&httpclient.Config{
		CredentialManager: credMgr,
	}, map[string]string{})
	return &HTTPRequest{
		client: client,
	}
}

// SetConfirmFunc define callback para confirmar operações destrutivas (DELETE/PUT/PATCH).
func (t *HTTPRequest) SetConfirmFunc(fn func(ctx context.Context, method, url, body string) (bool, error)) {
	t.confirmFn = fn
}

func (t *HTTPRequest) Name() string { return "http_request" }

func (t *HTTPRequest) Description() string {
	return "Makes complete HTTP requests supporting all methods (GET/POST/PUT/DELETE/PATCH), custom headers, request body, and authentication. Blocks local/private hosts by default. Use for API calls, REST endpoints, and data submission."
}

func (t *HTTPRequest) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {
				"type": "string",
				"description": "URL completa para requisição (deve começar com http:// ou https://)"
			},
			"method": {
				"type": "string",
				"enum": ["GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"],
				"description": "Método HTTP (padrão: GET)"
			},
			"headers": {
				"type": "object",
				"additionalProperties": {"type": "string"},
				"description": "Headers HTTP customizados (ex: {\"Content-Type\": \"application/json\"}). Autenticação é aplicada automaticamente."
			},
			"body": {
				"type": "string",
				"description": "Body da requisição (para POST/PUT/PATCH). Pode ser JSON string, form data ou texto."
			},
			"body_type": {
				"type": "string",
				"enum": ["json", "form", "text", "raw"],
				"description": "Tipo do body: 'json' (application/json), 'form' (application/x-www-form-urlencoded), 'text' (text/plain), 'raw' (sem Content-Type). Padrão: json"
			},
			"max_response_size": {
				"type": "integer",
				"description": "Tamanho máximo da resposta em caracteres (padrão: 50000)"
			},
			"extract_mode": {
				"type": "string",
				"enum": ["auto", "text", "json", "raw"],
				"description": "Modo de processamento da resposta: 'auto' detecta automaticamente, 'text' extrai texto, 'json' formata JSON, 'raw' retorna sem processar. Padrão: auto"
			}
		},
		"required": ["url"],
		"additionalProperties": false
	}`)
}

type httpRequestArgs struct {
	URL             string            `json:"url"`
	Method          string            `json:"method,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	Body            string            `json:"body,omitempty"`
	BodyType        string            `json:"body_type,omitempty"`
	MaxResponseSize *int              `json:"max_response_size,omitempty"`
	ExtractMode     string            `json:"extract_mode,omitempty"`
}

// Limites de segurança
const (
	httpDefaultMaxLength = 50000
	httpMaxResponseBody  = 10 * 1024 * 1024 // 10MB max download
)

func (t *HTTPRequest) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var a httpRequestArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return tools.ToolResult{Content: "Erro ao parsear argumentos: " + err.Error(), IsError: true}, nil
	}

	// Valida URL
	if a.URL == "" {
		return tools.ToolResult{Content: "Parâmetro 'url' é obrigatório", IsError: true}, nil
	}

	parsedURL, err := url.Parse(a.URL)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("URL inválida: %v", err), IsError: true}, nil
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return tools.ToolResult{Content: "URL deve usar http:// ou https://", IsError: true}, nil
	}

	// Bloqueia hosts locais/privados (exceto em modo teste)
	if !t.allowPrivateHosts && isPrivateHost(parsedURL.Hostname()) {
		return tools.ToolResult{Content: "Acesso a hosts locais/privados não é permitido", IsError: true}, nil
	}

	// Define valores padrão
	method := "GET"
	if a.Method != "" {
		method = strings.ToUpper(a.Method)
	}

	// GUARDRAIL 1: Confirmação para operações destrutivas
	if method == "DELETE" || method == "PUT" || method == "PATCH" {
		if t.confirmFn != nil {
			bodyPreview := a.Body
			if len(bodyPreview) > 200 {
				bodyPreview = bodyPreview[:200] + "..."
			}
			confirmed, err := t.confirmFn(ctx, method, a.URL, bodyPreview)
			if err != nil {
				return tools.ToolResult{
					Content: fmt.Sprintf("Erro ao solicitar confirmação: %v", err),
					IsError: true,
				}, nil
			}
			if !confirmed {
				return tools.ToolResult{
					Content: fmt.Sprintf("Operação %s cancelada pelo usuário", method),
					IsError: true,
				}, nil
			}
		}
	}

	bodyType := "json"
	if a.BodyType != "" {
		bodyType = a.BodyType
	}

	extractMode := "auto"
	if a.ExtractMode != "" {
		extractMode = a.ExtractMode
	}

	maxLength := httpDefaultMaxLength
	if a.MaxResponseSize != nil && *a.MaxResponseSize > 0 {
		maxLength = *a.MaxResponseSize
	}

	// Prepara body
	var bodyReader io.Reader
	if a.Body != "" {
		bodyReader = strings.NewReader(a.Body)
	}

	// Cria requisição
	req, err := http.NewRequestWithContext(ctx, method, a.URL, bodyReader)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao criar requisição: %v", err), IsError: true}, nil
	}

	// Define User-Agent padrão
	req.Header.Set("User-Agent", "Assistente/1.0 (Tool HTTPRequest; +https://github.com)")

	// Adiciona headers customizados
	for k, v := range a.Headers {
		req.Header.Set(k, v)
	}

	// Define Content-Type baseado em body_type (se não foi especificado)
	if a.Body != "" && req.Header.Get("Content-Type") == "" {
		switch bodyType {
		case "json":
			req.Header.Set("Content-Type", "application/json")
		case "form":
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		case "text":
			req.Header.Set("Content-Type", "text/plain")
			// "raw" não define Content-Type
		}
	}

	// Executar requisição usando cliente HTTP centralizado
	// Autenticação é aplicada automaticamente pelo interceptor
	resp, err := t.client.Do(ctx, req)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao executar requisição: %v", err), IsError: true}, nil
	}
	defer func() { _ = resp.Body.Close() }()

	// Lê resposta com limite
	limitedReader := io.LimitReader(resp.Body, httpMaxResponseBody)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao ler resposta: %v", err), IsError: true}, nil
	}

	contentType := resp.Header.Get("Content-Type")
	responseContent := string(body)

	// Processa resposta baseado no extract_mode
	var extracted string
	switch extractMode {
	case "raw":
		extracted = responseContent
	case "text":
		// Se for HTML, extrai texto
		if strings.Contains(contentType, "text/html") {
			extracted = htmlToText(responseContent)
		} else {
			extracted = responseContent
		}
	case "json":
		// Tenta formatar JSON
		var jsonData interface{}
		if err := json.Unmarshal([]byte(responseContent), &jsonData); err == nil {
			formatted, _ := json.MarshalIndent(jsonData, "", "  ")
			extracted = string(formatted)
		} else {
			extracted = responseContent
		}
	case "auto":
		// Detecta automaticamente
		if strings.Contains(contentType, "application/json") {
			var jsonData interface{}
			if err := json.Unmarshal([]byte(responseContent), &jsonData); err == nil {
				formatted, _ := json.MarshalIndent(jsonData, "", "  ")
				extracted = string(formatted)
			} else {
				extracted = responseContent
			}
		} else if strings.Contains(contentType, "text/html") {
			extracted = htmlToText(responseContent)
		} else {
			extracted = responseContent
		}
	default:
		extracted = responseContent
	}

	// Trunca se necessário
	truncated := false
	if len(extracted) > maxLength {
		extracted = extracted[:maxLength]
		truncated = true
	}

	// Monta header informativo
	header := fmt.Sprintf("HTTP %s %s\n", method, a.URL)
	header += fmt.Sprintf("Status: %d %s\n", resp.StatusCode, resp.Status)
	header += fmt.Sprintf("Content-Type: %s\n", contentType)
	header += fmt.Sprintf("Content-Length: %d bytes | Extracted: %d chars\n", len(body), len(extracted))
	if truncated {
		header += fmt.Sprintf("(TRUNCADO: limite de %d caracteres)\n", maxLength)
	}
	header += "\n"

	// Determina se é erro baseado no status code
	isError := resp.StatusCode >= 400

	return tools.ToolResult{
		Content: header + extracted,
		IsError: isError,
		Metadata: map[string]any{
			"url":          a.URL,
			"method":       method,
			"status":       resp.StatusCode,
			"content_type": contentType,
			"length":       len(extracted),
			"truncated":    truncated,
		},
	}, nil
}
