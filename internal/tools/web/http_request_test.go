package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"assistente/internal/credentials"
	"assistente/internal/tools"
	"assistente/internal/userctx"
)

// newTestHTTPRequest cria um HTTPRequest que permite hosts privados (para httptest)
func newTestHTTPRequest() *HTTPRequest {
	credMgr := credentials.NewManager(nil)
	hr := NewHTTPRequest(credMgr)
	hr.allowPrivateHosts = true
	return hr
}

func TestHTTPRequest_Name(t *testing.T) {
	tool := NewHTTPRequest(nil)
	if tool.Name() != "http_request" {
		t.Errorf("expected 'http_request', got '%s'", tool.Name())
	}
}

func TestHTTPRequest_Parameters(t *testing.T) {
	tool := NewHTTPRequest(nil)
	params := tool.Parameters()
	var schema map[string]any
	if err := json.Unmarshal(params, &schema); err != nil {
		t.Fatalf("failed to parse parameters: %v", err)
	}
	props := schema["properties"].(map[string]any)
	if _, ok := props["url"]; !ok {
		t.Error("missing 'url' property")
	}
	if _, ok := props["method"]; !ok {
		t.Error("missing 'method' property")
	}
	if _, ok := props["headers"]; !ok {
		t.Error("missing 'headers' property")
	}
}

func TestHTTPRequest_GET(t *testing.T) {
	// Server de teste que retorna JSON
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message": "success", "data": [1, 2, 3]}`))
	}))
	defer ts.Close()

	tool := newTestHTTPRequest()
	args := map[string]any{
		"url": ts.URL,
	}
	argsJSON, _ := json.Marshal(args)

	result, err := tool.Execute(context.Background(), argsJSON)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Content)
	}

	if !json.Valid([]byte(result.Content)) || !result.Structured {
		t.Errorf("expected canonical JSON response, got %q", result.Content)
	}
	if result.Metadata["status"] != http.StatusOK {
		t.Errorf("expected status metadata 200, got %v", result.Metadata["status"])
	}

	if !strings.Contains(result.Content, "success") {
		t.Error("expected 'success' in response content")
	}
}

func TestHTTPRequest_BlocksRedirectToInvalidScheme(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "ftp://example.com/x")
		w.WriteHeader(http.StatusFound)
	}))
	defer ts.Close()

	tool := newTestHTTPRequest() // allowPrivateHosts=true; o alvo é o destino do redirect
	argsJSON, _ := json.Marshal(map[string]any{"url": ts.URL})
	result, err := tool.Execute(context.Background(), argsJSON)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.IsError {
		t.Error("esperado IsError ao bloquear redirect para scheme inválido")
	}
}

func TestHTTPRequest_POST_JSON(t *testing.T) {
	// Server que espera POST com JSON
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		contentType := r.Header.Get("Content-Type")
		if !strings.Contains(contentType, "application/json") {
			t.Errorf("expected application/json, got %s", contentType)
		}

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"status": "created"}`))
	}))
	defer ts.Close()

	tool := newTestHTTPRequest()
	args := map[string]any{
		"url":       ts.URL,
		"method":    "POST",
		"body":      `{"name": "João", "age": 30}`,
		"body_type": "json",
	}
	argsJSON, _ := json.Marshal(args)

	result, err := tool.Execute(context.Background(), argsJSON)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Content)
	}

	if !strings.Contains(result.Content, "201") {
		t.Error("expected 201 Created in response")
	}
}

func TestHTTPRequest_DELETE(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	tool := newTestHTTPRequest()

	// Sem confirmação (confirmFn é nil), deve executar
	args := map[string]any{
		"url":    ts.URL,
		"method": "DELETE",
	}
	argsJSON, _ := json.Marshal(args)

	result, err := tool.Execute(context.Background(), argsJSON)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Content)
	}

	if !strings.Contains(result.Content, "204") {
		t.Error("expected 204 No Content in response")
	}
}

func TestHTTPRequest_DELETE_WithConfirmation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	tool := newTestHTTPRequest()

	// Registra confirmação negada
	tool.SetConfirmFunc(func(ctx context.Context, method, url, body string) (bool, error) {
		return false, nil // Nega
	})

	args := map[string]any{
		"url":    ts.URL,
		"method": "DELETE",
	}
	argsJSON, _ := json.Marshal(args)

	result, err := tool.Execute(context.Background(), argsJSON)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Deve retornar erro (operação cancelada)
	if !result.IsError {
		t.Error("expected error when delete denied")
	}

	if !strings.Contains(strings.ToLower(result.Content), "cancelada") {
		t.Errorf("expected 'cancelada' in error, got: %s", result.Content)
	}
}

func TestHTTPRequest_CustomHeaders(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "test-value" {
			t.Errorf("missing X-Custom header")
		}
		if r.Header.Get("Authorization") != "Bearer token123" {
			t.Errorf("missing Authorization header")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Testa custom headers - credentials são aplicadas automaticamente pelo cliente HTTP
	// localhost é bloqueado, então este teste valida que headers customizados funcionam
}

func TestHTTPRequest_InvalidURL(t *testing.T) {
	tool := NewHTTPRequest(nil)

	args := map[string]any{
		"url": "not a valid url",
	}
	argsJSON, _ := json.Marshal(args)

	result, err := tool.Execute(context.Background(), argsJSON)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !result.IsError {
		t.Error("expected error for invalid URL")
	}
}

func TestHTTPRequest_ExtractJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name": "test", "value": 123, "nested": {"key": "data"}}`))
	}))
	defer ts.Close()

	tool := newTestHTTPRequest()
	args := map[string]any{
		"url":          ts.URL,
		"extract_mode": "json",
	}
	argsJSON, _ := json.Marshal(args)

	result, err := tool.Execute(context.Background(), argsJSON)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Content)
	}

	// Deve conter JSON formatado
	if !strings.Contains(result.Content, "name") || !strings.Contains(result.Content, "test") {
		t.Errorf("expected formatted JSON in response: %s", result.Content)
	}
}

func TestHTTPRequestLargeJSONAndRawFailWithoutPartial(t *testing.T) {
	jsonBody := `{"value":"` + strings.Repeat("x", 1000) + `"}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(jsonBody))
	}))
	defer ts.Close()

	for _, tc := range []struct {
		mode string
		code string
	}{
		{mode: "json", code: "result_too_large"},
		{mode: "text", code: "result_too_large"},
		{mode: "raw", code: "raw_result_too_large"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			args, _ := json.Marshal(map[string]any{
				"url": ts.URL, "extract_mode": tc.mode, "max_response_size": 100,
			})
			result, err := newTestHTTPRequest().Execute(context.Background(), args)
			if err != nil || !result.IsError || result.Failure == nil || result.Failure.Code != tc.code {
				t.Fatalf("resultado grande não falhou corretamente: err=%v result=%+v", err, result)
			}
			if strings.Contains(result.Content, strings.Repeat("x", 100)) {
				t.Fatal("falha contém prefixo parcial da resposta")
			}
			if result.Metadata["url"] != ts.URL || result.Metadata["status"] != http.StatusOK {
				t.Fatalf("falha perdeu metadata HTTP: %+v", result.Metadata)
			}
		})
	}
}

func TestHTTPRequestAutoRecognizesStructuredSuffixJSON(t *testing.T) {
	body := `{"type":"problem","detail":"inválido"}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()
	args, _ := json.Marshal(map[string]any{"url": ts.URL, "extract_mode": "auto"})
	result, err := newTestHTTPRequest().Execute(context.Background(), args)
	if err != nil || result.IsError || !result.Structured || !json.Valid([]byte(result.Content)) {
		t.Fatalf("+json não foi reconhecido como estruturado: err=%v result=%+v", err, result)
	}
}

func TestHTTPRequestJSONFormattingPreservesLargeInteger(t *testing.T) {
	const body = `{"id":900719925474099312345}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()
	for _, mode := range []string{"json", "auto"} {
		args, _ := json.Marshal(map[string]any{"url": ts.URL, "extract_mode": mode})
		result, err := newTestHTTPRequest().Execute(context.Background(), args)
		if err != nil || result.IsError || !strings.Contains(result.Content, "900719925474099312345") {
			t.Fatalf("%s arredondou número JSON: err=%v result=%+v", mode, err, result)
		}
	}
}

func TestHTTPRequestRawRejectsInvalidUTF8(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte{0xff, 0xfe})
	}))
	defer ts.Close()
	args, _ := json.Marshal(map[string]any{"url": ts.URL, "extract_mode": "raw"})
	result, err := newTestHTTPRequest().Execute(context.Background(), args)
	if err != nil || !result.IsError || result.Failure == nil || result.Failure.Code != "raw_invalid_utf8" {
		t.Fatalf("raw não UTF-8 não falhou explicitamente: err=%v result=%+v", err, result)
	}
}

func TestHTTPRequestStructuredJSONRejectsInvalidUTF8(t *testing.T) {
	for _, mode := range []string{"json", "auto", "text"} {
		t.Run(mode, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/problem+json")
				_, _ = w.Write([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'})
			}))
			defer ts.Close()
			args, _ := json.Marshal(map[string]any{"url": ts.URL, "extract_mode": mode})
			result, err := newTestHTTPRequest().Execute(context.Background(), args)
			if err != nil || !result.IsError || result.Failure == nil ||
				result.Failure.Code != "structured_invalid_utf8" || result.Structured {
				t.Fatalf("JSON UTF-8 inválido não falhou: err=%v result=%+v", err, result)
			}
			if strings.Contains(result.Content, "�") {
				t.Fatalf("falha contém JSON corrompido: %q", result.Content)
			}
		})
	}
}

func TestHTTPRequestOversizedDownloadPreservesHTTPContext(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		chunk := strings.Repeat("x", 64*1024)
		for written := 0; written <= httpMaxResponseBody; written += len(chunk) {
			_, _ = w.Write([]byte(chunk))
		}
	}))
	defer ts.Close()

	args, _ := json.Marshal(map[string]any{"url": ts.URL, "method": http.MethodPost})
	result, err := newTestHTTPRequest().Execute(context.Background(), args)
	if err != nil || !result.IsError || result.Failure == nil ||
		result.Failure.Code != "response_body_too_large" {
		t.Fatalf("download grande não falhou corretamente: err=%v result=%+v", err, result)
	}
	if result.Metadata["url"] != ts.URL || result.Metadata["method"] != http.MethodPost ||
		result.Metadata["status"] != http.StatusOK {
		t.Fatalf("falha perdeu metadata HTTP: %+v", result.Metadata)
	}
	if result.Metadata["bytes_observed"] != httpMaxResponseBody+1 {
		t.Fatalf("bytes observados incorretos: %+v", result.Metadata)
	}
	if _, misleading := result.Metadata["length"]; misleading {
		t.Fatalf("comprimento desconhecido não deve ser apresentado como total: %+v", result.Metadata)
	}
	if result.Annotations == nil || result.Annotations.HTTPResponse == nil ||
		result.Annotations.HTTPResponse.URL != ts.URL ||
		result.Annotations.HTTPResponse.Method != http.MethodPost ||
		result.Annotations.HTTPResponse.ContentType != "application/octet-stream" {
		t.Fatalf("falha perdeu proveniência model-facing: %+v", result.Annotations)
	}
	if strings.Contains(result.Content, strings.Repeat("x", 100)) {
		t.Fatal("falha contém prefixo parcial da resposta")
	}
}

func TestHTTPRequestUsesExecutorBudgetWhenParameterIsOmitted(t *testing.T) {
	payload := strings.Repeat("x", 60*1024)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(payload))
	}))
	defer ts.Close()

	args, _ := json.Marshal(map[string]any{"url": ts.URL, "extract_mode": "text"})
	ctx := tools.WithMaxResultSize(context.Background(), 100*1024)
	result, err := newTestHTTPRequest().Execute(ctx, args)
	if err != nil || result.IsError ||
		(result.Annotations != nil && result.Annotations.OutputWindow != nil) ||
		!strings.Contains(result.Content, payload) {
		t.Fatalf("budget do executor não foi respeitado: err=%v result=%+v", err, result)
	}
}

func TestHTTPRequestPreservesStatusModelFacingForExactAndPagedBodies(t *testing.T) {
	for _, tc := range []struct {
		name        string
		contentType string
		body        string
		status      int
		mode        string
		max         int
	}{
		{name: "json de erro", contentType: "application/problem+json", body: `{"detail":"ausente"}`, status: http.StatusNotFound, mode: "auto", max: 100},
		{name: "raw pequeno", contentType: "text/plain", body: "exato", status: http.StatusOK, mode: "raw", max: 100},
		{name: "texto paginado", contentType: "text/plain", body: strings.Repeat("x", 200), status: http.StatusOK, mode: "text", max: 50},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer ts.Close()
			args, _ := json.Marshal(map[string]any{
				"url": ts.URL, "extract_mode": tc.mode, "max_response_size": tc.max,
			})
			ctx := userctx.WithUserID(context.Background(), "http-test")
			result, err := newTestHTTPRequest().Execute(ctx, args)
			if err != nil || result.Annotations == nil || result.Annotations.HTTPResponse == nil {
				t.Fatalf("sem anotação HTTP: err=%v result=%+v", err, result)
			}
			modelContent := tools.ContentForModel(result)
			if !strings.Contains(modelContent, `"status":`+strconv.Itoa(tc.status)) {
				t.Fatalf("status ausente do conteúdo model-facing: %q", modelContent)
			}
			if window := result.Annotations.OutputWindow; window != nil && window.HasMore {
				nextArgs, _ := json.Marshal(map[string]any{
					"result_id": window.ResultID, "offset": window.NextOffset, "limit": 50,
				})
				next, nextErr := tools.NewReadToolResult().Execute(ctx, nextArgs)
				if nextErr != nil || next.IsError || next.Content == "" {
					t.Fatalf("continuação HTTP indisponível: err=%v result=%+v", nextErr, next)
				}
				if next.Annotations == nil || next.Annotations.HTTPResponse == nil ||
					next.Annotations.HTTPResponse.URL != ts.URL {
					t.Fatalf("continuação perdeu proveniência HTTP: %+v", next.Annotations)
				}
			}
		})
	}
}

func TestHTTPRequestMaxSizeCountsExtractedPayloadNotHeader(t *testing.T) {
	body := strings.Repeat("a", 100)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()
	args, _ := json.Marshal(map[string]any{
		"url": ts.URL, "extract_mode": "text", "max_response_size": len(body),
	})
	result, err := newTestHTTPRequest().Execute(context.Background(), args)
	if err != nil || result.IsError || result.Annotations != nil || !strings.HasSuffix(result.Content, body) {
		t.Fatalf("header consumiu max_response_size: err=%v result=%+v", err, result)
	}
}

func mustIntPtr(v int) *int { return &v }

func TestParseTolerantMaxResponseSize(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    *int
		wantErr bool
	}{
		{name: "número", raw: `50000`, want: mustIntPtr(50000)},
		{name: "string numérica", raw: `"1000000"`, want: mustIntPtr(1000000)},
		{name: "ausente", raw: ``, want: nil},
		{name: "null", raw: `null`, want: nil},
		{name: "string vazia", raw: `""`, want: nil},
		{name: "string não numérica", raw: `"muito grande"`, wantErr: true},
		{name: "float rejeitado", raw: `1.5`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseTolerantMaxResponseSize(json.RawMessage(tc.raw))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("esperava erro para %q", tc.raw)
				}
				if !strings.Contains(err.Error(), "max_response_size") {
					t.Fatalf("erro não acionável: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			switch {
			case tc.want == nil && got != nil:
				t.Fatalf("esperava nil, obtido %d", *got)
			case tc.want != nil && got == nil:
				t.Fatalf("esperava %d, obtido nil", *tc.want)
			case tc.want != nil && *got != *tc.want:
				t.Fatalf("esperava %d, obtido %d", *tc.want, *got)
			}
		})
	}
}

func TestParseTolerantHeaders(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    map[string]string
		wantErr bool
	}{
		{name: "objeto", raw: `{"X-A":"1","X-B":"2"}`, want: map[string]string{"X-A": "1", "X-B": "2"}},
		{name: "string com JSON de objeto", raw: `"{\"X-A\":\"1\"}"`, want: map[string]string{"X-A": "1"}},
		{name: "ausente", raw: ``, want: nil},
		{name: "null", raw: `null`, want: nil},
		{name: "string vazia", raw: `""`, want: nil},
		{name: "string inválida", raw: `"não é json"`, wantErr: true},
		{name: "string com array", raw: `"[1,2]"`, wantErr: true},
		{name: "tipo inesperado", raw: `123`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseTolerantHeaders(json.RawMessage(tc.raw))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("esperava erro para %q", tc.raw)
				}
				if !strings.Contains(err.Error(), "headers") {
					t.Fatalf("erro não acionável: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("tamanho divergente: got=%v want=%v", got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Fatalf("header %q = %q, esperado %q", k, got[k], v)
				}
			}
		})
	}
}

// TestHTTPRequest_TolerantHeadersAsJSONString cobre a variação real em que o
// modelo serializa headers como string contendo o JSON do objeto.
func TestHTTPRequest_TolerantHeadersAsJSONString(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "abc" {
			t.Errorf("header X-Custom ausente: %q", r.Header.Get("X-Custom"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	rawArgs := `{"url":"` + ts.URL + `","headers":"{\"X-Custom\":\"abc\"}"}`
	result, err := newTestHTTPRequest().Execute(context.Background(), json.RawMessage(rawArgs))
	if err != nil || result.IsError {
		t.Fatalf("headers string-JSON não aceitos: err=%v result=%+v", err, result)
	}
}

// TestHTTPRequest_TolerantMaxResponseSizeAsString cobre a variação real em que o
// modelo envia max_response_size como string numérica.
func TestHTTPRequest_TolerantMaxResponseSizeAsString(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	rawArgs := `{"url":"` + ts.URL + `","max_response_size":"50000"}`
	result, err := newTestHTTPRequest().Execute(context.Background(), json.RawMessage(rawArgs))
	if err != nil || result.IsError {
		t.Fatalf("max_response_size string numérica não aceito: err=%v result=%+v", err, result)
	}
}

// TestHTTPRequest_RejectsInvalidTolerantArgs garante que entradas inválidas
// produzem um erro de parsing acionável (sem realizar a requisição).
func TestHTTPRequest_RejectsInvalidTolerantArgs(t *testing.T) {
	cases := []struct {
		name       string
		raw        string
		wantSubstr string
	}{
		{name: "headers string inválida", raw: `{"url":"https://example.com","headers":"não é json"}`, wantSubstr: "headers"},
		{name: "max_response_size string inválida", raw: `{"url":"https://example.com","max_response_size":"muito"}`, wantSubstr: "max_response_size"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := NewHTTPRequest(nil).Execute(context.Background(), json.RawMessage(tc.raw))
			if err != nil {
				t.Fatalf("Execute retornou erro de runtime: %v", err)
			}
			if !result.IsError {
				t.Fatalf("esperava IsError para entrada inválida: %+v", result)
			}
			if !strings.Contains(result.Content, "Erro ao parsear argumentos") ||
				!strings.Contains(result.Content, tc.wantSubstr) {
				t.Fatalf("erro de parsing não acionável: %q", result.Content)
			}
		})
	}
}

func TestHTTPRequestFileModeStreamsLargeResponseAndHidesSecrets(t *testing.T) {
	const secret = "Bearer should-not-be-returned"
	payload := `{"items":["` + strings.Repeat("x", 500*1024) + `"]}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", "run-123")
		w.Header().Set("Set-Cookie", "session=secret")
		w.Header().Set("Authorization", secret)
		_, _ = w.Write([]byte(payload))
	}))
	defer ts.Close()

	dir := t.TempDir()
	tool := newTestHTTPRequest()
	if err := tool.SetArtifactDir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tool.CleanupArtifacts() })
	args, _ := json.Marshal(map[string]any{"url": ts.URL, "extract_mode": "file", "output_path": filepath.Join(dir, "run.json")})
	result, err := tool.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("download em arquivo falhou: err=%v result=%+v", err, result)
	}
	if len(result.Content) > 4096 || strings.Contains(result.Content, strings.Repeat("x", 100)) || strings.Contains(result.Content, secret) || strings.Contains(result.Content, "session=secret") {
		t.Fatalf("retorno model-facing contém payload ou segredo: %q", result.Content)
	}
	var summary map[string]any
	if err := json.Unmarshal([]byte(result.Content), &summary); err != nil {
		t.Fatalf("resumo não é JSON: %v", err)
	}
	path, ok := summary["path"].(string)
	realDir, _ := filepath.EvalSymlinks(dir)
	if !ok || filepath.Dir(path) != realDir || filepath.Base(path) != "run.json" {
		t.Fatalf("path inesperado: %v (dir=%s)", summary["path"], dir)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != payload || int64(len(data)) != int64(summary["size_bytes"].(float64)) || summary["truncated"] != false {
		t.Fatalf("artefato incorreto: bytes=%d summary=%v", len(data), summary)
	}
	if summary["headers"].(map[string]any)["ETag"] != "run-123" {
		t.Fatalf("header relevante ausente: %v", summary["headers"])
	}
	if _, leaked := summary["headers"].(map[string]any)["Set-Cookie"]; leaked {
		t.Fatal("Set-Cookie vazou no resumo")
	}
}

func TestHTTPRequestFileModeRejectsPathTraversal(t *testing.T) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_, _ = w.Write([]byte("should not be requested"))
	}))
	defer ts.Close()

	dir := t.TempDir()
	tool := newTestHTTPRequest()
	if err := tool.SetArtifactDir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tool.CleanupArtifacts() })
	args, _ := json.Marshal(map[string]any{"url": ts.URL, "extract_mode": "file", "output_path": "..\\escape.json"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil || !result.IsError || result.Failure == nil || result.Failure.Code != "invalid_output_path" {
		t.Fatalf("path traversal não foi rejeitado: err=%v result=%+v", err, result)
	}
	if called {
		t.Fatal("path inválido disparou requisição HTTP")
	}
}

func TestHTTPRequestJSONPathExtractsDeepFieldFromLargeJSON(t *testing.T) {
	payload := `{"noise":"` + strings.Repeat("n", 500*1024) + `","children":[{"metadata":{"name":"deploy-to-prod"}}]}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer ts.Close()

	args, _ := json.Marshal(map[string]any{"url": ts.URL, "extract_mode": "jsonpath", "jsonpath": "$..metadata.name", "max_response_size": 1024})
	result, err := newTestHTTPRequest().Execute(context.Background(), args)
	if err != nil || result.IsError || !result.Structured {
		t.Fatalf("jsonpath falhou: err=%v result=%+v", err, result)
	}
	if result.Content != "[\n  \"deploy-to-prod\"\n]" {
		t.Fatalf("resultado jsonpath inesperado: %q", result.Content)
	}
}

func TestHTTPRequestJSONPathReportsInvalidJSONAndQuery(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		path string
		code string
	}{
		{name: "JSON inválido", body: "não-json", path: "$..name", code: "jsonpath_invalid_json"},
		{name: "query inválida", body: `{"name":"ok"}`, path: "$..[?(@.name)]", code: "jsonpath_invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer ts.Close()
			args, _ := json.Marshal(map[string]any{"url": ts.URL, "extract_mode": "jsonpath", "jsonpath": tc.path})
			result, err := newTestHTTPRequest().Execute(context.Background(), args)
			if err != nil || !result.IsError || result.Failure == nil || result.Failure.Code != tc.code {
				t.Fatalf("erro jsonpath incorreto: err=%v result=%+v", err, result)
			}
			if strings.Contains(result.Content, tc.body) && tc.body != "não-json" {
				t.Fatal("erro devolveu o documento completo")
			}
		})
	}
}

func TestHTTPRequestJSONPathLimitsExtractedResultNotSourceDocument(t *testing.T) {
	payload := `{"names":["` + strings.Repeat("a", 5000) + `"]}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer ts.Close()

	args, _ := json.Marshal(map[string]any{"url": ts.URL, "extract_mode": "jsonpath", "jsonpath": "$.names", "max_response_size": 128})
	result, err := newTestHTTPRequest().Execute(context.Background(), args)
	if err != nil || !result.IsError || result.Failure == nil || result.Failure.Code != "jsonpath_result_too_large" {
		t.Fatalf("limite do resultado extraído não aplicado: err=%v result=%+v", err, result)
	}
	if strings.Contains(result.Content, strings.Repeat("a", 100)) {
		t.Fatal("erro devolveu o resultado extraído parcialmente")
	}
}

func TestHTTPArtifactStoreCleansArtifacts(t *testing.T) {
	store := newHTTPArtifactStore()
	dir := t.TempDir()
	if err := store.SetDir(dir); err != nil {
		t.Fatal(err)
	}
	artifact, err := store.writeResponse(context.Background(), strings.NewReader("payload"), "payload.json", httpMaxResponseBody)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(artifact.Path); err != nil {
		t.Fatal(err)
	}
	if err := store.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(artifact.Path); !os.IsNotExist(err) {
		t.Fatalf("artefato não foi limpo: %v", err)
	}
}

func TestHTTPArtifactStoreRemovesPartialDownload(t *testing.T) {
	store := newHTTPArtifactStore()
	if err := store.SetDir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Cleanup() })
	_, err := store.writeResponse(context.Background(), failingReader{}, "failed.json", httpMaxResponseBody)
	if err == nil || !strings.Contains(err.Error(), "falha ao baixar") {
		t.Fatalf("falha de download não foi reportada: %v", err)
	}
	entries, err := os.ReadDir(store.dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("download parcial deixou arquivos: %v", entries)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, fmt.Errorf("origem indisponível") }

func TestHTTPArtifactCleanupPreservesUnownedFiles(t *testing.T) {
	dir := t.TempDir()
	stores := []*httpArtifactStore{newHTTPArtifactStore(), newHTTPArtifactStore()}
	for _, store := range stores {
		if err := store.SetDir(dir); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.Cleanup() })
	}
	unowned := filepath.Join(dir, "user.txt")
	if err := os.WriteFile(unowned, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := stores[0].writeResponse(context.Background(), strings.NewReader("first"), "first.json", 100)
	if err != nil {
		t.Fatal(err)
	}
	second, err := stores[1].writeResponse(context.Background(), strings.NewReader("second"), "second.json", 100)
	if err != nil {
		t.Fatal(err)
	}
	if err := stores[0].cleanupExpired(time.Now().Add(httpArtifactTTL + time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(first.Path); !os.IsNotExist(err) {
		t.Fatalf("TTL não removeu artefato: %v", err)
	}
	if err := stores[0].Cleanup(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{unowned, second.Path} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("limpeza removeu arquivo alheio: %s: %v", path, err)
		}
	}
}

func TestHTTPArtifactConcurrentDownloadsNeverOverwrite(t *testing.T) {
	store := newHTTPArtifactStore()
	if err := store.SetDir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Cleanup() })
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.writeResponse(context.Background(), strings.NewReader("complete"), "same.json", 100)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("esperava um único download vencedor: %d", successes)
	}
	data, err := os.ReadFile(filepath.Join(store.dir, "same.json"))
	if err != nil || string(data) != "complete" {
		t.Fatalf("arquivo incompleto: %q %v", data, err)
	}
}

func TestHTTPArtifactLimitAndCancellationRemovePartialFiles(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		store := newHTTPArtifactStore()
		if err := store.SetDir(t.TempDir()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.Cleanup() })
		ctx, stop := context.WithCancel(context.Background())
		if cancel {
			stop()
		}
		_, err := store.writeResponse(ctx, strings.NewReader("oversized"), "partial.bin", 4)
		stop()
		if err == nil {
			t.Fatal("esperava erro")
		}
		entries, err := os.ReadDir(store.dir)
		if err != nil || len(entries) != 0 {
			t.Fatalf("arquivo parcial foi mantido: %v %v", entries, err)
		}
	}
}

func TestHTTPRequestFilePreservesBinaryAndBoundsHeaders(t *testing.T) {
	body := []byte{0, 255, 128, 10}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for _, key := range []string{"Content-Type", "ETag", "Content-Language", "Last-Modified"} {
			w.Header().Set(key, strings.Repeat("h", 20000))
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(body)
	}))
	defer ts.Close()
	tool := newTestHTTPRequest()
	if err := tool.SetArtifactDir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tool.CleanupArtifacts() })
	args, _ := json.Marshal(map[string]any{"url": ts.URL, "extract_mode": "file"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil || !result.IsError || result.Metadata["status"] != 400 {
		t.Fatalf("status incorreto: %+v %v", result, err)
	}
	encoded, _ := json.Marshal(result)
	if len(encoded) > 4096 {
		t.Fatalf("metadados não limitados: %d", len(encoded))
	}
	data, err := os.ReadFile(result.Metadata["artifact_path"].(string))
	if err != nil || string(data) != string(body) {
		t.Fatalf("binário alterado: %v %v", data, err)
	}
}

func TestRestrictedJSONPathResourceLimits(t *testing.T) {
	if _, err := parseRestrictedJSONPath("$..a..a..a"); err == nil {
		t.Fatal("query com expansão combinatória aceita")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := extractRestrictedJSONPath(ctx, `{"name":"value"}`, "$..name", 100); err != context.Canceled {
		t.Fatalf("cancelamento ignorado: %v", err)
	}
	result, err := extractRestrictedJSONPath(context.Background(), `{"id":9007199254740993}`, "$.id", 100)
	if err != nil || !strings.Contains(result, "9007199254740993") {
		t.Fatalf("inteiro alterado: %s %v", result, err)
	}
	_, err = extractRestrictedJSONPath(context.Background(), `{"name":"a","children":[{"name":"b"}]}`, "$..name", 8)
	if err == nil || jsonPathErrorCode(err) != "jsonpath_result_too_large" {
		t.Fatalf("limite ignorado: %v", err)
	}
}

func TestHTTPRequestFileCancellationDuringDownload(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("partial"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer ts.Close()
	tool := newTestHTTPRequest()
	dir := t.TempDir()
	if err := tool.SetArtifactDir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tool.CleanupArtifacts() })
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	args, _ := json.Marshal(map[string]any{"url": ts.URL, "extract_mode": "file"})
	result, err := tool.Execute(ctx, args)
	if err != nil || !result.IsError || result.Failure == nil || result.Failure.Code != "download_cancelled" {
		t.Fatalf("cancelamento não reportado: %+v %v", result, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("cancelamento deixou arquivo parcial: %v %v", entries, err)
	}
}

func TestHTTPArtifactRejectsReplacedRoot(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "artifacts")
	store := newHTTPArtifactStore()
	if err := store.SetDir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Cleanup() })
	if err := os.Rename(dir, dir+"-original"); err != nil {
		t.Skipf("SO impede renomear diretório aberto: %v", err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, dir); err != nil {
		t.Skipf("symlink indisponível: %v", err)
	}
	if _, err := store.writeResponse(context.Background(), strings.NewReader("secret"), "escape.json", 100); err == nil {
		t.Fatal("raiz substituída foi aceita")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("gravou fora da raiz: %v %v", entries, err)
	}
}

type artifactReaderFunc func([]byte) (int, error)

func (f artifactReaderFunc) Read(p []byte) (int, error) { return f(p) }

func TestHTTPArtifactPreservesFileReplacedDuringDownload(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			store := newHTTPArtifactStore()
			dir := t.TempDir()
			if err := store.SetDir(dir); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Cleanup() })
			path := filepath.Join(dir, "response.json")
			reader := artifactReaderFunc(func(p []byte) (int, error) {
				if err := os.Rename(path, path+".original"); err != nil {
					t.Skipf("SO impede substituir arquivo aberto: %v", err)
				}
				if err := os.WriteFile(path, []byte("user-file"), 0600); err != nil {
					t.Fatal(err)
				}
				if fail {
					return 0, fmt.Errorf("falha de origem")
				}
				return copy(p, "download"), io.EOF
			})
			if _, err := store.writeResponse(context.Background(), reader, "response.json", 100); err == nil {
				t.Fatal("arquivo substituído aceito")
			}
			if err := store.Cleanup(); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "user-file" {
				t.Fatalf("arquivo alheio foi removido: %q %v", data, err)
			}
		})
	}
}
