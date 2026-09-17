package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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
	artifacts         *httpArtifactStore
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
	t := &HTTPRequest{
		client:    client,
		artifacts: newHTTPArtifactStore(),
	}
	// net/http segue redirects automaticamente; sem isto uma URL pública poderia
	// redirecionar para um host privado (ex.: 127.0.0.1, 169.254.169.254) e burlar
	// o bloqueio anti-SSRF. Aplica o guard compartilhado no client desta tool.
	if bc := client.GetBaseClient(); bc != nil {
		bc.CheckRedirect = httpclient.RedirectGuard(httpclient.DefaultMaxRedirects, func() bool { return t.allowPrivateHosts })
		// Barreira anti-SSRF definitiva: valida o IP REAL pós-resolução de DNS no
		// DialContext, cobrindo DNS rebinding, formas numéricas não-padrão e os
		// redirects (que reusam este transport).
		httpclient.SetTransportGuard(bc, func() bool { return t.allowPrivateHosts })
	}
	return t
}

// SetArtifactDir configura a pasta exclusiva onde extract_mode=file grava
// respostas. O diretório deve ser controlado pelo host, não pelo modelo.
func (t *HTTPRequest) SetArtifactDir(dir string) error {
	return t.artifacts.SetDir(dir)
}

// CleanupArtifacts remove artefatos HTTP desta instância, normalmente chamado
// pelo host no encerramento do app.
func (t *HTTPRequest) CleanupArtifacts() error {
	return t.artifacts.Cleanup()
}

// SetConfirmFunc define callback para confirmar operações destrutivas (DELETE/PUT/PATCH).
func (t *HTTPRequest) SetConfirmFunc(fn func(ctx context.Context, method, url, body string) (bool, error)) {
	t.confirmFn = fn
}

// SetNetworkAuthorizer instala o authorizer anti-SSRF (consentimento + allowlist)
// no cliente HTTP desta tool. Sem authorizer, hosts privados/CGNAT continuam com
// hard-deny acionável.
func (t *HTTPRequest) SetNetworkAuthorizer(a httpclient.NetworkAuthorizer) {
	t.client.SetNetworkAuthorizer(a)
}

func (t *HTTPRequest) Name() string { return "http_request" }

// CatalogMetadata declara os metadados de catálogo da tool (AEP-0077, Fase 1).
func (t *HTTPRequest) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{Category: "http", Class: "http_api", Package: "web", Risk: "network"}
}

func (t *HTTPRequest) Description() string {
	return `Makes an HTTP(S) request with explicit method, headers, body, response mode, and size limit. Use for APIs or endpoints that require protocol-level control; for example {"url":"https://api.example.com/items","method":"GET","extract_mode":"json"}. Use extract_mode=file to stream a large response to a safe local artifact and receive only metadata, or extract_mode=jsonpath with a restricted field selector such as "$..metadata.name" to return matches from a large JSON response. Do not use to search for a URL (use web_search), read a normal page with readability extraction (use web_fetch), or parse feed entries (use feed_read). Credentials registered for the domain are applied automatically; do not place secrets in arguments. Risk: performs a network operation, and mutating methods can change remote state; PUT, PATCH, and DELETE may require user confirmation. Local/private destinations and redirects are guarded by the network policy. If unavailable, discover and load it with tool_catalog when the profile permits on-demand tools.`
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
				"description": "Tamanho máximo do payload extraído em bytes, sem header/envelope model-facing (padrão: 50000; em jobs, usa o budget do executor quando omitido)"
			},
			"extract_mode": {
				"type": "string",
				"enum": ["auto", "text", "json", "raw", "file", "jsonpath"],
				"description": "Modo de processamento: 'auto' detecta, 'text' extrai texto, 'json' formata JSON, 'raw' retorna sem processar, 'file' grava a resposta completa em artefato local, 'jsonpath' extrai campos com seletor restrito. Padrão: auto"
			},
			"output_path": {
				"type": "string",
				"description": "Nome do arquivo de saída para extract_mode=file. Deve ser um nome simples dentro da pasta de artefatos segura; se omitido, um nome único é gerado."
			},
			"jsonpath": {
				"type": "string",
				"description": "Seletor restrito para extract_mode=jsonpath, por exemplo '$..metadata.name'. Suporta apenas campos com ponto e descendência recursiva, sem filtros, scripts ou comandos."
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
	OutputPath      string            `json:"output_path,omitempty"`
	JSONPath        string            `json:"jsonpath,omitempty"`
}

// UnmarshalJSON torna o parsing de argumentos tolerante a variações comuns que
// os modelos produzem sem quebrar o contrato canônico (AEP-0016):
//   - max_response_size pode chegar como número (50000) OU string numérica
//     ("50000"); strings não numéricas são rejeitadas com erro acionável.
//   - headers pode chegar como objeto {"k":"v"} OU como string contendo o JSON
//     serializado desse objeto; strings inválidas são rejeitadas com erro claro.
//
// Os demais campos mantêm a semântica original de string.
func (a *httpRequestArgs) UnmarshalJSON(data []byte) error {
	// rawHTTPRequestArgs espelha httpRequestArgs, mas recebe headers e
	// max_response_size como RawMessage para permitir parsing tolerante sem
	// recursão no UnmarshalJSON.
	type rawHTTPRequestArgs struct {
		URL             string          `json:"url"`
		Method          string          `json:"method,omitempty"`
		Headers         json.RawMessage `json:"headers,omitempty"`
		Body            string          `json:"body,omitempty"`
		BodyType        string          `json:"body_type,omitempty"`
		MaxResponseSize json.RawMessage `json:"max_response_size,omitempty"`
		ExtractMode     string          `json:"extract_mode,omitempty"`
		OutputPath      string          `json:"output_path,omitempty"`
		JSONPath        string          `json:"jsonpath,omitempty"`
	}
	var raw rawHTTPRequestArgs
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	headers, err := parseTolerantHeaders(raw.Headers)
	if err != nil {
		return err
	}
	size, err := parseTolerantMaxResponseSize(raw.MaxResponseSize)
	if err != nil {
		return err
	}

	a.URL = raw.URL
	a.Method = raw.Method
	a.Headers = headers
	a.Body = raw.Body
	a.BodyType = raw.BodyType
	a.MaxResponseSize = size
	a.ExtractMode = raw.ExtractMode
	a.OutputPath = raw.OutputPath
	a.JSONPath = raw.JSONPath
	return nil
}

// parseTolerantHeaders aceita um objeto {"k":"v"} ou uma string contendo o JSON
// de um objeto. Ausência/null retornam headers nil. Entradas inválidas produzem
// um erro acionável.
func parseTolerantHeaders(raw json.RawMessage) (map[string]string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, nil
	}

	switch trimmed[0] {
	case '{':
		var m map[string]string
		if err := json.Unmarshal(trimmed, &m); err != nil {
			return nil, fmt.Errorf("headers inválidos: esperado um objeto {\"k\":\"v\"}: %v", err)
		}
		return m, nil
	case '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return nil, fmt.Errorf("headers inválidos: %v", err)
		}
		s = strings.TrimSpace(s)
		if s == "" || s == "null" {
			return nil, nil
		}
		var m map[string]string
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			return nil, fmt.Errorf("headers como string deve conter o JSON de um objeto {\"k\":\"v\"}; recebido %q", s)
		}
		return m, nil
	default:
		return nil, fmt.Errorf("headers deve ser um objeto {\"k\":\"v\"} ou uma string com esse JSON; recebido %s", string(trimmed))
	}
}

// parseTolerantMaxResponseSize aceita um inteiro (50000) ou uma string numérica
// ("50000"). Ausência/null retornam nil (usa o padrão). Strings não numéricas e
// valores não inteiros são rejeitados com erro acionável.
func parseTolerantMaxResponseSize(raw json.RawMessage) (*int, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, nil
	}

	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return nil, fmt.Errorf("max_response_size inválido: %v", err)
		}
		s = strings.TrimSpace(s)
		if s == "" || s == "null" {
			return nil, nil
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, fmt.Errorf("max_response_size deve ser um inteiro (ex.: 50000) ou uma string numérica; recebido %q", s)
		}
		return &n, nil
	}

	// Número JSON: usa json.Number para rejeitar floats/valores não inteiros.
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	var num json.Number
	if err := dec.Decode(&num); err != nil {
		return nil, fmt.Errorf("max_response_size deve ser um inteiro (ex.: 50000) ou uma string numérica; recebido %s", string(trimmed))
	}
	i64, err := num.Int64()
	if err != nil {
		return nil, fmt.Errorf("max_response_size deve ser um inteiro (ex.: 50000); recebido %s", string(trimmed))
	}
	n := int(i64)
	return &n, nil
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

	// Hosts locais/privados/CGNAT/etc. são barrados pela política anti-SSRF na
	// barreira pós-DNS do cliente centralizado (client.Do). Quando há um authorizer
	// configurado, esse bloqueio abre o fluxo de consentimento/allowlist e a
	// request é reexecutada; sem authorizer, o cliente devolve um erro acionável.
	// Por isso NÃO barramos aqui de forma seca.

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
	if extractMode == "jsonpath" && strings.TrimSpace(a.JSONPath) == "" {
		return tools.ToolResult{Content: "jsonpath é obrigatório quando extract_mode=jsonpath", IsError: true, Failure: &tools.ToolFailure{Code: "jsonpath_invalid", Kind: tools.ErrorKindInvalidArgs, Retryable: false}}, nil
	}
	if extractMode == "file" && t.artifacts == nil {
		t.artifacts = newHTTPArtifactStore()
	}
	if extractMode == "file" && strings.TrimSpace(a.OutputPath) != "" {
		if _, err := t.artifacts.resolveOutputPath(a.OutputPath); err != nil {
			return tools.ToolResult{Content: err.Error(), IsError: true, Failure: &tools.ToolFailure{Code: "invalid_output_path", Kind: tools.ErrorKindInvalidArgs, Retryable: false}}, nil
		}
	}

	maxLength := httpDefaultMaxLength
	if a.MaxResponseSize != nil && *a.MaxResponseSize > 0 {
		maxLength = *a.MaxResponseSize
	} else if effective, explicit := tools.ExplicitMaxResultSizeFromContext(ctx); explicit {
		maxLength = effective
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

	if extractMode == "file" {
		if err := t.artifacts.cleanupExpired(time.Now()); err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Erro ao preparar limpeza de artefatos HTTP: %v", err), IsError: true}, nil
		}
		artifact, writeErr := t.artifacts.writeResponse(ctx, resp.Body, a.OutputPath, httpMaxResponseBody)
		if writeErr != nil {
			metadata := map[string]any{
				"url": a.URL, "method": method, "status": resp.StatusCode,
				"content_type": resp.Header.Get("Content-Type"), "truncated": artifact.Truncated,
			}
			if artifact.Size > 0 {
				metadata["bytes_observed"] = artifact.Size
			}
			if errors.Is(writeErr, errHTTPArtifactTooLarge) {
				return tools.ToolResult{
					Content: fmt.Sprintf("Resposta excede o limite seguro de download de %d bytes; o artefato não foi mantido.", httpMaxResponseBody),
					IsError: true, Metadata: metadata,
					Annotations: &tools.ResultAnnotations{HTTPResponse: httpResponseAnnotation(a.URL, method, resp)},
					Failure:     &tools.ToolFailure{Code: "response_body_too_large", Kind: tools.ErrorKindUnknown, Retryable: false},
				}, nil
			}
			if ctx.Err() != nil {
				return tools.ToolResult{Content: "Download do artefato HTTP cancelado pelo usuário", IsError: true, Metadata: metadata, Failure: &tools.ToolFailure{Code: "download_cancelled", Kind: tools.ErrorKindCancelled, Retryable: false}}, nil
			}
			return tools.ToolResult{Content: fmt.Sprintf("Erro ao materializar resposta HTTP: %v", writeErr), IsError: true, Metadata: metadata, Failure: &tools.ToolFailure{Code: "artifact_download_failed", Kind: tools.ErrorKindUnknown, Retryable: false}}, nil
		}
		contentType := resp.Header.Get("Content-Type")
		metadata := map[string]any{
			"url": a.URL, "method": method, "status": resp.StatusCode,
			"content_type": contentType, "length": artifact.Size,
			"artifact_path": artifact.Path, "sha256": artifact.SHA256, "truncated": false,
		}
		return tools.ToolResult{
			Content: artifactSummaryContent(resp.StatusCode, resp.Status, contentType, relevantArtifactHeaders(resp.Header), artifact),
			IsError: resp.StatusCode >= 400, Structured: true, Metadata: metadata,
			Annotations: &tools.ResultAnnotations{HTTPResponse: httpResponseAnnotation(a.URL, method, resp)},
		}, nil
	}

	// Lê resposta com limite
	limitedReader := io.LimitReader(resp.Body, httpMaxResponseBody+1)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao ler resposta: %v", err), IsError: true}, nil
	}
	if len(body) > httpMaxResponseBody {
		contentType := resp.Header.Get("Content-Type")
		metadata := map[string]any{
			"url": a.URL, "method": method, "status": resp.StatusCode,
			"content_type": contentType, "bytes_observed": len(body),
		}
		if resp.ContentLength >= 0 {
			metadata["length"] = resp.ContentLength
		}
		return tools.ToolResult{
			Content:  fmt.Sprintf("Resposta excede o limite seguro de download de %d bytes; o conteúdo não foi devolvido parcialmente.", httpMaxResponseBody),
			IsError:  true,
			Metadata: metadata,
			Annotations: &tools.ResultAnnotations{HTTPResponse: &tools.HTTPResponseAnnotation{
				Method: method, URL: a.URL, Status: resp.StatusCode,
				StatusText: http.StatusText(resp.StatusCode), ContentType: contentType,
			}},
			Failure: &tools.ToolFailure{Code: "response_body_too_large", Kind: tools.ErrorKindUnknown, Retryable: false},
		}, nil
	}

	contentType := resp.Header.Get("Content-Type")
	if extractMode == "raw" && !utf8.Valid(body) {
		return tools.ToolResult{
			Content: "Resposta raw não é UTF-8 válida e não pode ser devolvida exatamente.",
			IsError: true,
			Metadata: map[string]any{
				"url": a.URL, "method": method, "status": resp.StatusCode,
				"content_type": contentType, "length": len(body),
			},
			Annotations: &tools.ResultAnnotations{HTTPResponse: &tools.HTTPResponseAnnotation{
				Method: method, URL: a.URL, Status: resp.StatusCode,
				StatusText: http.StatusText(resp.StatusCode), ContentType: contentType,
			}},
			Failure: &tools.ToolFailure{Code: "raw_invalid_utf8", Kind: tools.ErrorKindUnknown, Retryable: false},
		}, nil
	}
	expectsStructured := extractMode != "raw" &&
		(extractMode == "json" || isJSONMediaType(contentType))
	if expectsStructured && !utf8.Valid(body) {
		return tools.ToolResult{
			Content: "Resposta JSON não é UTF-8 válida e não pode ser preservada integralmente.",
			IsError: true,
			Metadata: map[string]any{
				"url": a.URL, "method": method, "status": resp.StatusCode,
				"content_type": contentType, "length": len(body),
			},
			Annotations: &tools.ResultAnnotations{HTTPResponse: &tools.HTTPResponseAnnotation{
				Method: method, URL: a.URL, Status: resp.StatusCode,
				StatusText: http.StatusText(resp.StatusCode), ContentType: contentType,
			}},
			Failure: &tools.ToolFailure{Code: "structured_invalid_utf8", Kind: tools.ErrorKindUnknown, Retryable: false},
		}, nil
	}
	responseContent := string(body)

	// Processa resposta baseado no extract_mode
	var extracted string
	structuredJSON := false
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
		if formatted, ok := formatJSONPreservingNumbers(responseContent); ok {
			extracted = formatted
			structuredJSON = true
		} else {
			extracted = responseContent
		}
	case "auto":
		// Detecta automaticamente
		if isJSONMediaType(contentType) {
			if formatted, ok := formatJSONPreservingNumbers(responseContent); ok {
				extracted = formatted
				structuredJSON = true
			} else {
				extracted = responseContent
			}
		} else if strings.Contains(contentType, "text/html") {
			extracted = htmlToText(responseContent)
		} else {
			extracted = responseContent
		}
	case "jsonpath":
		extracted, err = extractRestrictedJSONPath(responseContent, a.JSONPath)
		if err != nil {
			return tools.ToolResult{
				Content: err.Error(), IsError: true,
				Metadata:    map[string]any{"url": a.URL, "method": method, "status": resp.StatusCode, "content_type": contentType, "length": len(body)},
				Annotations: &tools.ResultAnnotations{HTTPResponse: httpResponseAnnotation(a.URL, method, resp)},
				Failure:     &tools.ToolFailure{Code: jsonPathErrorCode(err), Kind: tools.ErrorKindInvalidArgs, Retryable: false},
			}, nil
		}
		if len(extracted) > maxLength {
			return tools.ToolResult{
				Content:     fmt.Sprintf("resultado de jsonpath tem %d bytes, acima do limite de %d; reduza o seletor ou aumente max_response_size", len(extracted), maxLength),
				IsError:     true,
				Metadata:    map[string]any{"url": a.URL, "method": method, "status": resp.StatusCode, "content_type": contentType, "length": len(extracted)},
				Annotations: &tools.ResultAnnotations{HTTPResponse: httpResponseAnnotation(a.URL, method, resp)},
				Failure:     &tools.ToolFailure{Code: "jsonpath_result_too_large", Kind: tools.ErrorKindUnknown, Retryable: false},
			}, nil
		}
		structuredJSON = true
	default:
		extracted = responseContent
	}
	if !structuredJSON && extractMode != "raw" && isJSONMediaType(contentType) &&
		tools.IsCanonicalJSON(extracted) {
		structuredJSON = true
	}

	// Monta header informativo
	header := fmt.Sprintf("HTTP %s %s\n", method, a.URL)
	header += fmt.Sprintf("Status: %d %s\n", resp.StatusCode, resp.Status)
	header += fmt.Sprintf("Content-Type: %s\n", contentType)
	header += fmt.Sprintf("Content-Length: %d bytes | Extracted: %d bytes\n", len(body), len(extracted))
	header += "\n"

	// Determina se é erro baseado no status code
	isError := resp.StatusCode >= 400
	var annotations *tools.ResultAnnotations
	if isError || structuredJSON || extractMode == "raw" || len(extracted) > maxLength {
		annotations = &tools.ResultAnnotations{HTTPResponse: &tools.HTTPResponseAnnotation{
			Method: method, URL: a.URL, Status: resp.StatusCode,
			StatusText: http.StatusText(resp.StatusCode), ContentType: contentType,
		}}
	}

	content := header + extracted
	if structuredJSON || extractMode == "raw" {
		content = extracted
	}
	result := tools.ToolResult{
		Content:     content,
		IsError:     isError,
		Structured:  structuredJSON,
		RawExact:    extractMode == "raw",
		Annotations: annotations,
		Metadata: map[string]any{
			"url":          a.URL,
			"method":       method,
			"status":       resp.StatusCode,
			"content_type": contentType,
			"length":       len(extracted),
		},
	}
	if (result.Structured || result.RawExact) && len(result.Content) > maxLength {
		code := "result_too_large"
		kind := "JSON estruturado"
		if result.RawExact {
			code = "raw_result_too_large"
			kind = "resultado raw"
		}
		return tools.ToolResult{
			Content:     fmt.Sprintf("%s tem %d bytes, acima do limite de %d; reduza o escopo da requisição ou use um header Range aceito pelo servidor.", kind, len(result.Content), maxLength),
			IsError:     true,
			Metadata:    result.Metadata,
			Annotations: annotations,
			Failure:     &tools.ToolFailure{Code: code, Kind: tools.ErrorKindUnknown, Retryable: false},
		}, nil
	}
	if len(extracted) <= maxLength {
		return result, nil
	}
	// max_response_size mede o payload extraído. Quando há continuação, o corpo
	// retomável não inclui o header informativo, para que offsets sejam exatos.
	result.Content = extracted
	protected, ok := tools.ProtectToolResult(ctx, result, maxLength)
	if !ok {
		return tools.ToolResult{
			Content: "Resposta excede a capacidade segura de preservação; reduza o escopo.",
			IsError: true,
			Failure: &tools.ToolFailure{Code: "result_storage_limit", Kind: tools.ErrorKindUnknown, Retryable: false},
		}, nil
	}
	return protected, nil
}

func isJSONMediaType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])
	}
	mediaType = strings.ToLower(mediaType)
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

func formatJSONPreservingNumbers(content string) (string, bool) {
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return "", false
	}
	formatted, err := json.MarshalIndent(value, "", "  ")
	return string(formatted), err == nil
}

func httpResponseAnnotation(rawURL, method string, resp *http.Response) *tools.HTTPResponseAnnotation {
	return &tools.HTTPResponseAnnotation{
		Method: method, URL: rawURL, Status: resp.StatusCode,
		StatusText: http.StatusText(resp.StatusCode), ContentType: resp.Header.Get("Content-Type"),
	}
}
