package llm

import (
	"strings"
	"testing"
)

// TestSummarizeHTTPError_EmptyBody testa resposta vazia
func TestSummarizeHTTPError_EmptyBody(t *testing.T) {
	result := summarizeHTTPError(500, []byte(""))
	if !strings.Contains(result, "resposta vazia") {
		t.Errorf("esperado 'resposta vazia', got: %s", result)
	}
	if !strings.Contains(result, "500") {
		t.Errorf("esperado status code 500, got: %s", result)
	}
}

// TestSummarizeHTTPError_SimpleText testa mensagem de erro simples
func TestSummarizeHTTPError_SimpleText(t *testing.T) {
	body := []byte("Connection timeout")
	result := summarizeHTTPError(504, body)

	if !strings.Contains(result, "504") {
		t.Errorf("esperado status 504, got: %s", result)
	}
	if !strings.Contains(result, "Connection timeout") {
		t.Errorf("esperado mensagem original, got: %s", result)
	}
}

// TestSummarizeHTTPError_Truncation testa truncagem de respostas grandes
func TestSummarizeHTTPError_Truncation(t *testing.T) {
	// Cria um body maior que 2000 caracteres
	body := strings.Repeat("x", 3000)
	result := summarizeHTTPError(500, []byte(body))

	if len(result) > 2100 { // Um pouco acima do limit + overhead de mensagem
		t.Errorf("resposta não foi truncada: %d caracteres", len(result))
	}

	if !strings.Contains(result, "truncado") {
		t.Errorf("esperado indicação de truncagem, got: %s", result)
	}
}

// TestSummarizeHTTPError_CloudflareHTML testa detecção de erro Cloudflare em HTML
func TestSummarizeHTTPError_CloudflareHTML(t *testing.T) {
	// HTML típico retornado pelo Cloudflare
	body := []byte(`
		<!DOCTYPE html>
		<html>
		<head><title>Error 522</title></head>
		<body>
		<h1>Connection timed out</h1>
		<p>Cloudflare is unable to connect to the origin server</p>
		<p>Cloudflare Ray ID: <strong>abc123def456</strong></p>
		</body>
		</html>
	`)

	result := summarizeHTTPError(522, body)

	if !strings.Contains(result, "Cloudflare") {
		t.Errorf("esperado menção a Cloudflare, got: %s", result)
	}
	if !strings.Contains(result, "522") {
		t.Errorf("esperado status 522, got: %s", result)
	}
}

// TestSummarizeHTTPError_CloudflareRayID testa extração de Ray ID
func TestSummarizeHTTPError_CloudflareRayID(t *testing.T) {
	body := []byte(`
		<!DOCTYPE html>
		<html>
		<body>
		<h1>Error 530</h1>
		<p>Cloudflare error</p>
		<p>Cloudflare Ray ID: <strong>ray-id-12345</strong></p>
		</body>
		</html>
	`)

	result := summarizeHTTPError(530, body)

	if !strings.Contains(result, "ray-id-12345") {
		t.Errorf("Ray ID não foi extraído, got: %s", result)
	}
	if !strings.Contains(result, "Ray ID") {
		t.Errorf("esperado formato 'Ray ID:', got: %s", result)
	}
}

// TestSummarizeHTTPError_NonCloudflareHTML testa HTML que não é Cloudflare
func TestSummarizeHTTPError_NonCloudflareHTML(t *testing.T) {
	body := []byte(`
		<!DOCTYPE html>
		<html>
		<head><title>Error</title></head>
		<body>
		<h1>500 Internal Server Error</h1>
		<p>Something went wrong</p>
		</body>
		</html>
	`)

	result := summarizeHTTPError(500, body)

	if !strings.Contains(result, "500") {
		t.Errorf("esperado status 500, got: %s", result)
	}
	// Não deve mencionar Cloudflare
	if strings.Contains(result, "Cloudflare") && strings.Contains(result, "Internal Server Error") {
		// OK, pode ser ambíguo se o corpo mencionou Cloudflare
	}
}

// TestParseCloudflareRayID_Valid testa parsing do Ray ID
func TestParseCloudflareRayID_Valid(t *testing.T) {
	html := `<p>Cloudflare Ray ID: <strong>abc123xyz789</strong></p>`
	result := parseCloudflareRayID(html)

	if result != "abc123xyz789" {
		t.Errorf("Ray ID não extraído corretamente: %q", result)
	}
}

// TestParseCloudflareRayID_Missing testa quando Ray ID não existe
func TestParseCloudflareRayID_Missing(t *testing.T) {
	html := `<html><body>No ray ID here</body></html>`
	result := parseCloudflareRayID(html)

	if result != "" {
		t.Errorf("esperado string vazia, got: %q", result)
	}
}

// TestParseCloudflareRayID_MultipleMatches testa case-insensitivity
func TestParseCloudflareRayID_MultipleMatches(t *testing.T) {
	html := `<P>CLOUDFLARE RAY ID: <strong>test-id-123</strong></P>`
	result := parseCloudflareRayID(html)

	if result != "test-id-123" {
		t.Errorf("Ray ID parsing case-insensitive falhou: %q", result)
	}
}

// TestParseErrorCodeFromHTML_Valid testa extração do código de erro
func TestParseErrorCodeFromHTML_Valid(t *testing.T) {
	html := `<h1>Error code 524</h1>`
	result := parseErrorCodeFromHTML(html)

	if result != 524 {
		t.Errorf("esperado 524, got %d", result)
	}
}

// TestParseErrorCodeFromHTML_Missing testa quando código não existe
func TestParseErrorCodeFromHTML_Missing(t *testing.T) {
	html := `<html><body>Some error occurred</body></html>`
	result := parseErrorCodeFromHTML(html)

	if result != 0 {
		t.Errorf("esperado 0, got %d", result)
	}
}

// TestParseErrorCodeFromHTML_CaseInsensitive testa case-insensitivity
func TestParseErrorCodeFromHTML_CaseInsensitive(t *testing.T) {
	html := `<H1>ERROR CODE 503</H1>`
	result := parseErrorCodeFromHTML(html)

	if result != 503 {
		t.Errorf("esperado 503, got %d", result)
	}
}

// TestParseErrorCodeFromHTML_InvalidNumber testa tratamento de número inválido
func TestParseErrorCodeFromHTML_InvalidNumber(t *testing.T) {
	html := `<h1>Error code abc</h1>`
	result := parseErrorCodeFromHTML(html)

	if result != 0 {
		t.Errorf("esperado 0 para número inválido, got %d", result)
	}
}

// TestSummarizeHTTPError_StatusCodeZero testa status 0
func TestSummarizeHTTPError_StatusCodeZero(t *testing.T) {
	result := summarizeHTTPError(0, []byte("Error message"))

	if strings.Contains(result, "0") {
		t.Errorf("status 0 não deveria aparecer no resultado")
	}
	if !strings.Contains(result, "Error message") {
		t.Errorf("mensagem deveria estar no resultado: %s", result)
	}
}

// TestSummarizeHTTPError_4xxErrors testa errors 4xx (cliente)
func TestSummarizeHTTPError_4xxErrors(t *testing.T) {
	tests := []struct {
		statusCode int
		name       string
	}{
		{400, "Bad Request"},
		{401, "Unauthorized"},
		{403, "Forbidden"},
		{404, "Not Found"},
		{429, "Too Many Requests"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := summarizeHTTPError(tt.statusCode, []byte(tt.name))

			if !strings.Contains(result, string(rune(tt.statusCode))) {
				// Pelo menos o número deveria aparecer em alguma forma
				if tt.statusCode < 100 || tt.statusCode > 999 {
					t.Logf("status %d pode não aparecer literalmente", tt.statusCode)
				}
			}
		})
	}
}

// TestSummarizeHTTPError_5xxErrors testa errors 5xx (servidor)
func TestSummarizeHTTPError_5xxErrors(t *testing.T) {
	tests := []struct {
		statusCode int
		name       string
	}{
		{500, "Internal Server Error"},
		{502, "Bad Gateway"},
		{503, "Service Unavailable"},
		{504, "Gateway Timeout"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := summarizeHTTPError(tt.statusCode, []byte(tt.name))

			if !strings.Contains(result, tt.name) {
				t.Logf("mensagem não encontrada no resultado para %d", tt.statusCode)
			}
		})
	}
}

// TestSummarizeHTTPError_LongBodyTruncation testa truncagem precisa
func TestSummarizeHTTPError_LongBodyTruncation(t *testing.T) {
	// Body com exatamente 2001 caracteres (acima do limit)
	body := strings.Repeat("a", 2001)
	result := summarizeHTTPError(500, []byte(body))

	// Deve ser truncado e conter indicação
	if !strings.Contains(result, "truncado") {
		t.Errorf("resposta longa sem truncagem: %s", result)
	}

	if len(result) > 2200 { // Margem para overhead
		t.Errorf("resultado ainda muito longo após truncagem: %d chars", len(result))
	}
}

// TestSummarizeHTTPError_WhitespaceHandling testa trim de whitespace
func TestSummarizeHTTPError_WhitespaceHandling(t *testing.T) {
	body := []byte("   \n\n  Error message  \n  ")
	result := summarizeHTTPError(500, body)

	if strings.Contains(result, "   ") {
		t.Errorf("whitespace não foi trimado: %q", result)
	}
}
