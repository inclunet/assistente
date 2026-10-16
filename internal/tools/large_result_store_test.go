package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"assistente/internal/tools/invocationctx"
	"assistente/internal/userctx"
)

func largeResultTestContext() context.Context {
	return userctx.WithUserID(context.Background(), "large-result-test")
}

func TestLargeResultStoreRejectsAnonymousAndInvalidUTF8Content(t *testing.T) {
	if _, ok := storeModelResult(context.Background(), "conteúdo"); ok {
		t.Fatal("store aceitou contexto sem usuário autenticado")
	}
	if _, ok := storeModelResult(largeResultTestContext(), string([]byte{0xff})); ok {
		t.Fatal("store anunciou conteúdo que não pode paginar como UTF-8")
	}
}

func TestProtectModelResultWithRedactorOnlyStoresRedactedCopy(t *testing.T) {
	ctx := largeResultTestContext()
	secret := strings.Repeat(`{"secret":"output-secret","visible":"ok"}`, 64)
	protected, ok := ProtectModelResultWithRedactor(ctx, ToolResult{
		Content:  secret,
		Metadata: map[string]any{"resource": "output-secret"},
		Annotations: &ResultAnnotations{HTTPResponse: &HTTPResponseAnnotation{
			URL: "https://example.invalid/output-secret",
		}},
	}, 512, func(result ToolResult) ToolResult {
		result.Content = strings.ReplaceAll(result.Content, "output-secret", "[redacted]")
		result.Metadata = nil
		result.Annotations = nil
		return result
	})
	if !ok || protected.Annotations == nil || protected.Annotations.OutputWindow == nil {
		t.Fatal("resultado não foi protegido")
	}
	stored, found := loadModelResult(ctx, protected.Annotations.OutputWindow.ResultID)
	if !found {
		t.Fatal("cópia protegida não foi armazenada")
	}
	if strings.Contains(stored, "output-secret") {
		t.Fatalf("store reteve segredo: %q", stored)
	}
	entry, found := loadModelResultEntry(ctx, protected.Annotations.OutputWindow.ResultID)
	if !found || entry.provenance != nil || entry.source != nil {
		t.Fatalf("proveniência lateral não foi omitida: %+v", entry)
	}
	if !strings.Contains(protected.Content, "output-secret") {
		t.Fatalf("resultado em memória foi redigido indevidamente: %q", protected.Content)
	}
}

func TestProtectorsRejectZeroBudget(t *testing.T) {
	for name, protect := range map[string]func(context.Context, ToolResult, int) (ToolResult, bool){
		"builtin": ProtectModelResult,
		"mcp":     ProtectExternalModelResult,
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := protect(largeResultTestContext(), ToolResult{Content: "x"}, 0); ok {
				t.Fatal("budget zero foi tratado como bypass")
			}
		})
	}
}

func TestReadToolResultRequiresExplicitOffset(t *testing.T) {
	id, ok := storeModelResult(largeResultTestContext(), "conteúdo")
	if !ok {
		t.Fatal("store falhou")
	}
	raw, _ := json.Marshal(map[string]any{"result_id": id})
	result, err := NewReadToolResult().Execute(largeResultTestContext(), raw)
	if err != nil || !result.IsError || result.Failure == nil ||
		result.Failure.Code != "invalid_args" || result.Failure.Kind != ErrorKindInvalidArgs {
		t.Fatalf("offset ausente reiniciou leitura: err=%v result=%+v", err, result)
	}
}

func TestReadToolResultClassifiesInvalidOffsets(t *testing.T) {
	id, ok := storeModelResult(largeResultTestContext(), "açúcar")
	if !ok {
		t.Fatal("store falhou")
	}
	for _, tc := range []struct {
		name   string
		offset int
		code   string
	}{
		{name: "fora do resultado", offset: 100, code: "result_offset_out_of_range"},
		{name: "meio de rune", offset: 2, code: "invalid_result_offset"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]any{"result_id": id, "offset": tc.offset})
			result, err := NewReadToolResult().Execute(largeResultTestContext(), raw)
			if err != nil || !result.IsError || result.Failure == nil ||
				result.Failure.Code != tc.code || result.Failure.Kind != ErrorKindInvalidArgs {
				t.Fatalf("offset inválido sem classificação estável: err=%v result=%+v", err, result)
			}
		})
	}
}

func TestProtectModelResultLayersByteResumeOverNaturalWindow(t *testing.T) {
	source := &OutputWindowAnnotation{
		HasMore: true, Unit: "lines", Offset: 0, Returned: 10, NextOffset: 11,
	}
	result := ToolResult{
		Content:     strings.Repeat("x", 1024),
		Annotations: &ResultAnnotations{OutputWindow: source},
	}
	modelContent := ContentForModelWithinLimit(largeResultTestContext(), result, 768, "read_file")
	if len(modelContent) > 768 || !strings.Contains(modelContent, `"result_id"`) ||
		!strings.Contains(modelContent, `"source_window"`) {
		t.Fatalf("pre-check não recompôs janela natural: %q", modelContent)
	}
	protected, ok := ProtectModelResult(largeResultTestContext(), result, 768)
	if !ok || protected.Annotations == nil || protected.Annotations.OutputWindow == nil {
		t.Fatalf("proteção da página natural falhou: ok=%v result=%+v", ok, protected)
	}
	window := protected.Annotations.OutputWindow
	if window.ResultID == "" || window.SourceWindow == nil ||
		window.SourceWindow.Unit != "lines" || window.SourceWindow.NextOffset != 11 {
		t.Fatalf("cursor natural não foi preservado como origem: %+v", window)
	}
	raw, _ := json.Marshal(map[string]any{
		"result_id": window.ResultID, "offset": window.NextOffset, "limit": 512,
	})
	next, err := NewReadToolResult().Execute(largeResultTestContext(), raw)
	if err != nil || next.IsError || next.Annotations == nil ||
		next.Annotations.OutputWindow == nil ||
		next.Annotations.OutputWindow.SourceWindow == nil ||
		next.Annotations.OutputWindow.SourceWindow.NextOffset != 11 {
		t.Fatalf("retomada perdeu cursor natural: err=%v result=%+v", err, next)
	}
}

func TestProtectModelResultUsesAnnotationsAndCanResume(t *testing.T) {
	original := strings.Repeat("abcç", 20_000)
	protected, ok := ProtectModelResult(largeResultTestContext(), ToolResult{Content: original}, 1024)
	if !ok || len(protected.Content) > 1024 {
		t.Fatalf("proteção falhou: ok=%v bytes=%d", ok, len(protected.Content))
	}
	window := protected.Annotations.OutputWindow
	if !window.HasMore || window.ResultID == "" || window.NextOffset != len(protected.Content) {
		t.Fatalf("anotação inválida: %+v", window)
	}
	if strings.Contains(strings.ToUpper(protected.Content), "TRUNCAD") {
		t.Fatal("aviso textual contaminou conteúdo")
	}
	raw, _ := json.Marshal(map[string]any{"result_id": window.ResultID, "offset": window.NextOffset, "limit": 777})
	next, err := NewReadToolResult().Execute(largeResultTestContext(), raw)
	if err != nil || next.IsError {
		t.Fatalf("retomada falhou: err=%v result=%+v", err, next)
	}
	if protected.Content+next.Content != original[:len(protected.Content)+len(next.Content)] {
		t.Fatal("retomada não preservou bytes exatos")
	}
}

func TestProtectExternalModelResultDelimitsPreviewWithoutChangingStoredJSON(t *testing.T) {
	original := `{"items":[` + strings.Repeat(`{"value":"abcdef"},`, 1000) + `null]}`
	protected, ok := ProtectExternalModelResult(largeResultTestContext(), ToolResult{Content: original}, 512)
	if !ok || !strings.HasPrefix(protected.Content, "--- INÍCIO DA PRÉVIA MCP") {
		t.Fatalf("prévia MCP não delimitada: %q", protected.Content)
	}
	window := protected.Annotations.OutputWindow
	stored, found := loadModelResult(largeResultTestContext(), window.ResultID)
	if !found || stored != original || !json.Valid([]byte(stored)) {
		t.Fatal("JSON MCP integral não foi preservado")
	}
}

func TestExecutorRejectsRawExactInsteadOfTruncating(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(&mockTool{
		name: "raw_test",
		exec: func(context.Context, json.RawMessage) (ToolResult, error) {
			return ToolResult{Content: strings.Repeat("x", 2048), RawExact: true}, nil
		},
	})
	cfg := DefaultExecutorConfig()
	cfg.MaxResultSize = 512
	got := NewExecutor(registry, cfg).ExecuteOne(largeResultTestContext(), ToolCall{
		ID: "call-raw", Function: FunctionCall{Name: "raw_test", Arguments: `{}`},
	})
	if got.ErrorCode != "raw_result_too_large" || !got.Result.IsError ||
		strings.Contains(got.Result.Content, strings.Repeat("x", 20)) {
		t.Fatalf("raw foi cortado silenciosamente: %+v", got)
	}
	if len(got.Result.Content) > cfg.MaxResultSize {
		t.Fatalf("mensagem de falha excedeu limite: %d", len(got.Result.Content))
	}
	if strings.Contains(got.Result.Content, "offset/limit") {
		t.Fatalf("orientação presumiu paginação inexistente na tool: %q", got.Result.Content)
	}
}

func TestExecutorProtectsLargeResultReturnedWithGoError(t *testing.T) {
	sentinel := errors.New("falha original")
	for _, tc := range []struct {
		name        string
		result      ToolResult
		recoverable bool
	}{
		{
			name:        "texto retomável",
			result:      ToolResult{Content: strings.Repeat("erro-textual-", 500)},
			recoverable: true,
		},
		{
			name:   "raw integral",
			result: ToolResult{Content: strings.Repeat("raw-segredo-", 500), RawExact: true},
		},
		{
			name: "JSON estruturado",
			result: ToolResult{
				Content:    `{"erro":"` + strings.Repeat("segredo", 500) + `"}`,
				Structured: true,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry := NewRegistry()
			registry.MustRegister(&mockTool{
				name: "error_result_test",
				exec: func(context.Context, json.RawMessage) (ToolResult, error) {
					return tc.result, sentinel
				},
			})
			cfg := DefaultExecutorConfig()
			cfg.MaxResultSize = 512
			got := NewExecutor(registry, cfg).ExecuteOne(largeResultTestContext(), ToolCall{
				ID: "call-error", Function: FunctionCall{Name: "error_result_test", Arguments: `{}`},
			})
			if !errors.Is(got.Error, sentinel) || got.ErrorKind != ErrorKindUnknown ||
				!got.Result.IsError || len(ContentForModel(got.Result)) > cfg.MaxResultSize {
				t.Fatalf("erro grande escapou ou perdeu classificação: %+v", got)
			}
			window := outputWindowOf(got.Result)
			if tc.recoverable {
				if window == nil || window.ResultID == "" || !window.HasMore {
					t.Fatalf("texto do erro não ficou retomável: %+v", got.Result)
				}
			} else if strings.Contains(got.Result.Content, "segredo") {
				t.Fatalf("resultado exato do erro foi devolvido parcialmente: %q", got.Result.Content)
			}
		})
	}
}

func TestExecutorRejectsIncompleteErrorResultForMachineConsumer(t *testing.T) {
	sentinel := errors.New("falha original")
	registry := NewRegistry()
	registry.MustRegister(&mockTool{
		name: "incomplete_error_test",
		exec: func(context.Context, json.RawMessage) (ToolResult, error) {
			return ToolResult{
				Content: "prévia",
				Annotations: &ResultAnnotations{OutputWindow: &OutputWindowAnnotation{
					HasMore: true, Unit: "results", Returned: 1,
				}},
			}, sentinel
		},
	})
	cfg := DefaultExecutorConfig()
	cfg.RequireCompleteResult = true
	got := NewExecutor(registry, cfg).ExecuteOne(largeResultTestContext(), ToolCall{
		ID: "call-incomplete", Function: FunctionCall{Name: "incomplete_error_test", Arguments: `{}`},
	})
	if !errors.Is(got.Error, sentinel) || !got.Result.IsError || got.Result.Failure == nil ||
		got.Result.Failure.Code != "result_too_large" ||
		strings.Contains(got.Result.Content, "prévia") {
		t.Fatalf("consumidor machine-facing recebeu janela incompleta: %+v", got)
	}
}

func TestExecutorAcceptsRawExactAtDeclaredLimit(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(&mockTool{
		name: "raw_at_limit",
		exec: func(context.Context, json.RawMessage) (ToolResult, error) {
			return ToolResult{Content: strings.Repeat("x", 128), RawExact: true}, nil
		},
	})
	cfg := DefaultExecutorConfig()
	cfg.MaxResultSize = 128
	got := NewExecutor(registry, cfg).ExecuteOne(context.Background(), ToolCall{
		ID: "call-raw-limit", Function: FunctionCall{Name: "raw_at_limit", Arguments: `{}`},
	})
	if got.Result.IsError || got.Result.Content != strings.Repeat("x", 128) {
		t.Fatalf("raw que cabe foi rejeitado: %+v", got)
	}
}

func TestExecutorMakesLargeMCPJSONRecoverable(t *testing.T) {
	original := `{"items":[` + strings.Repeat(`{"id":1},`, 2000) + `null]}`
	registry := NewRegistry()
	registry.MustRegister(&mockTool{
		name: "mcp_server__large",
		exec: func(context.Context, json.RawMessage) (ToolResult, error) {
			return ToolResult{Content: original}, nil
		},
	})
	cfg := DefaultExecutorConfig()
	cfg.MaxResultSize = 1024
	got := NewExecutor(registry, cfg).ExecuteOne(largeResultTestContext(), ToolCall{
		ID: "call-mcp", Function: FunctionCall{Name: "mcp_server__large", Arguments: `{}`},
	})
	if got.Result.IsError || got.Result.Annotations == nil || got.Result.Annotations.OutputWindow == nil {
		t.Fatalf("MCP grande não ficou recuperável: %+v", got)
	}
	if !strings.Contains(got.Result.Content, "PRÉVIA MCP") {
		t.Fatalf("prévia MCP não delimitada: %q", got.Result.Content)
	}
	stored, ok := loadModelResult(largeResultTestContext(), got.Result.Annotations.OutputWindow.ResultID)
	if !ok || stored != original || !json.Valid([]byte(stored)) {
		t.Fatal("executor corrompeu JSON MCP preservado")
	}
}

func TestExecutorDoesNotMistakeBuiltinMCPServerForBridge(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(&mockTool{
		name: "mcp_server",
		exec: func(context.Context, json.RawMessage) (ToolResult, error) {
			return ToolResult{Content: `{"value":"` + strings.Repeat("x", 2000) + `"}`, Structured: true}, nil
		},
	})
	cfg := DefaultExecutorConfig()
	cfg.MaxResultSize = 512
	got := NewExecutor(registry, cfg).ExecuteOne(context.Background(), ToolCall{
		ID: "call-builtin", Function: FunctionCall{Name: "mcp_server", Arguments: `{}`},
	})
	if got.ErrorCode != "result_too_large" || !got.Result.IsError {
		t.Fatalf("builtin mcp_server passou como bridge: %+v", got)
	}
}

func TestExecutorRejectsLargeLegacyJSONScalar(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(&mockTool{
		name: "legacy_json_scalar",
		exec: func(context.Context, json.RawMessage) (ToolResult, error) {
			return ToolResult{Content: `"` + strings.Repeat("x", 2000) + `"`}, nil
		},
	})
	cfg := DefaultExecutorConfig()
	cfg.MaxResultSize = 256
	got := NewExecutor(registry, cfg).ExecuteOne(context.Background(), ToolCall{
		ID: "call-json-scalar", Function: FunctionCall{Name: "legacy_json_scalar", Arguments: `{}`},
	})
	if got.ErrorCode != "result_too_large" || !got.Result.IsError {
		t.Fatalf("scalar JSON foi cortado como texto: %+v", got)
	}
}

func TestExecutorSizeFailurePreservesHTTPContextWithoutWindow(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(&mockTool{
		name: "large_http_json",
		exec: func(context.Context, json.RawMessage) (ToolResult, error) {
			return ToolResult{
				Content: strings.Repeat("x", 2048), Structured: true,
				Metadata: map[string]any{"status": 404, "result_id": "efemero"},
				Annotations: &ResultAnnotations{
					HTTPResponse: &HTTPResponseAnnotation{Method: "GET", URL: "https://example.test", Status: 404},
					OutputWindow: &OutputWindowAnnotation{HasMore: true, ResultID: "efemero"},
				},
			}, nil
		},
	})
	cfg := DefaultExecutorConfig()
	cfg.MaxResultSize = 256
	got := NewExecutor(registry, cfg).ExecuteOne(context.Background(), ToolCall{
		ID: "call-http-json", Function: FunctionCall{Name: "large_http_json", Arguments: `{}`},
	})
	if !got.Result.IsError || got.Result.Annotations == nil || got.Result.Annotations.HTTPResponse == nil {
		t.Fatalf("falha global perdeu contexto HTTP: %+v", got)
	}
	if got.Result.Annotations.OutputWindow != nil || got.Result.Metadata["result_id"] != nil {
		t.Fatalf("falha global preservou continuação efêmera: %+v", got.Result)
	}
}

func TestExecutorSizeFailureBoundsAnnotationEnvelope(t *testing.T) {
	for _, returnGoError := range []bool{false, true} {
		name := "normal_result"
		if returnGoError {
			name = "go_error"
		}
		t.Run(name, func(t *testing.T) {
			registry := NewRegistry()
			registry.MustRegister(&mockTool{
				name: "large_annotated_failure_" + name,
				exec: func(context.Context, json.RawMessage) (ToolResult, error) {
					result := ToolResult{
						Content:    strings.Repeat("x", 4096),
						Structured: true,
						Annotations: &ResultAnnotations{
							HTTPResponse: &HTTPResponseAnnotation{
								Method: "GET", URL: strings.Repeat("u", 4096), Status: 500,
								StatusText: strings.Repeat("s", 4096), ContentType: strings.Repeat("c", 4096),
							},
							DocumentProjection: &DocumentProjectionAnnotation{
								Source: strings.Repeat("p", 4096), Format: strings.Repeat("f", 4096),
								Warnings: []string{
									strings.Repeat("w", 4096), strings.Repeat("w", 4096),
									strings.Repeat("w", 4096), strings.Repeat("w", 4096),
									strings.Repeat("w", 4096),
								},
							},
							OutputWindow: &OutputWindowAnnotation{HasMore: true, ResultID: "efemero"},
						},
					}
					if returnGoError {
						return result, errors.New("falha esperada")
					}
					return result, nil
				},
			})
			cfg := DefaultExecutorConfig()
			cfg.MaxResultSize = 512
			got := NewExecutor(registry, cfg).ExecuteOne(context.Background(), ToolCall{
				ID: "call-" + name,
				Function: FunctionCall{
					Name: "large_annotated_failure_" + name, Arguments: `{}`,
				},
			})
			if !got.Result.IsError {
				t.Fatalf("resultado grande não virou falha: %+v", got)
			}
			if size := len(ContentForModel(got.Result)); size > cfg.MaxResultSize {
				t.Fatalf("envelope de falha excedeu teto: %d > %d", size, cfg.MaxResultSize)
			}
			if got.Result.Annotations != nil && got.Result.Annotations.OutputWindow != nil {
				t.Fatalf("falha preservou continuação efêmera: %+v", got.Result.Annotations)
			}
			if response := got.Result.Annotations; response != nil && response.HTTPResponse != nil {
				if len(response.HTTPResponse.URL) > failureAnnotationStringLimit ||
					len(response.HTTPResponse.StatusText) > failureAnnotationStringLimit ||
					len(response.HTTPResponse.ContentType) > failureAnnotationStringLimit {
					t.Fatalf("anotação HTTP não foi sanitizada: %+v", response.HTTPResponse)
				}
			}
		})
	}
}

func TestExecutorMachineFacingRejectsLargePlainText(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(&mockTool{
		name: "machine_text",
		exec: func(context.Context, json.RawMessage) (ToolResult, error) {
			return ToolResult{Content: strings.Repeat("x", 2048)}, nil
		},
	})
	cfg := DefaultExecutorConfig()
	cfg.MaxResultSize = 128
	cfg.RequireCompleteResult = true
	got := NewExecutor(registry, cfg).ExecuteOne(context.Background(), ToolCall{
		ID: "call-machine", Function: FunctionCall{Name: "machine_text", Arguments: `{}`},
	})
	if got.ErrorCode != "result_too_large" || !got.Result.IsError || got.Result.Annotations != nil {
		t.Fatalf("consumer machine-facing recebeu prévia: %+v", got)
	}
	if len(got.Result.Content) > cfg.MaxResultSize {
		t.Fatalf("falha machine-facing excedeu teto: %d", len(got.Result.Content))
	}
}

func TestExecutorMachineFacingRejectsIncompleteWindowEvenWhenPreviewFits(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(&mockTool{
		name: "machine_window",
		exec: func(context.Context, json.RawMessage) (ToolResult, error) {
			return ToolResult{
				Content: "prefixo",
				Annotations: &ResultAnnotations{OutputWindow: &OutputWindowAnnotation{
					HasMore: true, Unit: "bytes", Returned: 7, Total: 20, NextOffset: 7,
				}},
			}, nil
		},
	})
	cfg := DefaultExecutorConfig()
	cfg.MaxResultSize = 1024
	cfg.RequireCompleteResult = true
	got := NewExecutor(registry, cfg).ExecuteOne(context.Background(), ToolCall{
		ID: "call-machine-window", Function: FunctionCall{Name: "machine_window", Arguments: `{}`},
	})
	if got.ErrorCode != "result_too_large" || !got.Result.IsError || got.Result.Annotations != nil {
		t.Fatalf("consumer machine-facing recebeu janela incompleta: %+v", got)
	}
}

func TestLooksLikeCanonicalJSONAcceptsOneCompleteValue(t *testing.T) {
	for _, valid := range []string{`{}`, `[]`, `"texto"`, `123`, `true`, `false`, `null`} {
		if !IsCanonicalJSON(valid) {
			t.Errorf("JSON válido rejeitado: %s", valid)
		}
	}
	for _, invalid := range []string{"", "texto", "{} {}", "1 2"} {
		if IsCanonicalJSON(invalid) {
			t.Errorf("conteúdo inválido aceito como JSON: %q", invalid)
		}
	}
}

func TestProtectionKeepsOriginalResultIDWhenExecutorTightensBudget(t *testing.T) {
	original := strings.Repeat("conteúdo-", 5000)
	first, ok := ProtectToolResult(largeResultTestContext(), ToolResult{Content: original}, 4096)
	if !ok {
		t.Fatal("primeira proteção falhou")
	}
	firstID := first.Annotations.OutputWindow.ResultID
	second, ok := ProtectModelResult(largeResultTestContext(), first, 1024)
	if !ok {
		t.Fatal("segunda proteção falhou")
	}
	if second.Annotations.OutputWindow.ResultID != firstID {
		t.Fatalf("result_id mudou: %q -> %q", firstID, second.Annotations.OutputWindow.ResultID)
	}
	stored, found := loadModelResult(largeResultTestContext(), firstID)
	if !found || stored != original {
		t.Fatal("segunda proteção substituiu o conteúdo integral pela prévia")
	}
}

func TestProtectionFailsWhenExistingResultExpired(t *testing.T) {
	original := strings.Repeat("conteúdo-", 5000)
	first, ok := ProtectToolResult(largeResultTestContext(), ToolResult{Content: original}, 4096)
	if !ok {
		t.Fatal("primeira proteção falhou")
	}
	id := first.Annotations.OutputWindow.ResultID

	modelResultStore.mu.Lock()
	elem := modelResultStore.items[id]
	delete(modelResultStore.items, id)
	modelResultStore.bytes -= len(elem.Value.(storedLargeResult).content)
	modelResultStore.order.Remove(elem)
	modelResultStore.mu.Unlock()

	if _, ok := ProtectModelResult(largeResultTestContext(), first, 1024); ok {
		t.Fatal("prévia expirada foi republicada como se fosse resultado integral")
	}
}

func TestContentForModelWithinLimitRecalculatesRecoverableWindow(t *testing.T) {
	original := strings.Repeat("abcç", 5000)
	first, ok := ProtectToolResult(largeResultTestContext(), ToolResult{Content: original}, 4096)
	if !ok {
		t.Fatal("primeira proteção falhou")
	}
	got := ContentForModelWithinLimit(largeResultTestContext(), first, 1024, "")
	if len(got) > 1024 {
		t.Fatalf("resultado excedeu quota: %d", len(got))
	}
	parts := strings.SplitN(got, contentHeader, 2)
	if len(parts) != 2 {
		t.Fatalf("envelope ausente: %q", got)
	}
	var annotations ResultAnnotations
	if err := json.Unmarshal([]byte(strings.TrimPrefix(parts[0], annotationsHeader)), &annotations); err != nil {
		t.Fatalf("anotações inválidas: %v", err)
	}
	window := annotations.OutputWindow
	if window == nil || window.Returned != len(parts[1]) || window.NextOffset != len(parts[1]) {
		t.Fatalf("janela não corresponde ao corpo enviado: %+v, corpo=%d", window, len(parts[1]))
	}
	if stored, found := loadModelResult(largeResultTestContext(), window.ResultID); !found || stored != original {
		t.Fatal("resultado integral deixou de ser recuperável")
	}
}

func TestContentForModelWithinLimitNeverCutsRawOrStructured(t *testing.T) {
	for _, result := range []ToolResult{
		{Content: strings.Repeat("raw-", 100), RawExact: true},
		{Content: `{"value":"` + strings.Repeat("x", 500) + `"}`, Structured: true},
		{Content: `"` + strings.Repeat("x", 500) + `"`},
	} {
		got := ContentForModelWithinLimit(context.Background(), result, 256, "")
		if strings.Contains(got, result.Content[:100]) {
			t.Fatalf("conteúdo exato foi devolvido parcialmente: %q", got)
		}
		if !strings.Contains(got, "result_too_large") {
			t.Fatalf("falha explícita ausente: %q", got)
		}
		if strings.Contains(got, "offset/limit") {
			t.Fatalf("orientação presumiu paginação inexistente na tool: %q", got)
		}
	}
}

func TestContentForModelWithinLimitNeverRepaginatesRawResultPage(t *testing.T) {
	result := ToolResult{
		Content:  strings.Repeat("página-", 100),
		RawExact: true,
		Annotations: &ResultAnnotations{OutputWindow: &OutputWindowAnnotation{
			HasMore: true, Unit: "bytes", Offset: 500, Returned: 800,
			NextOffset: 1300, ResultID: "tool-result-existente",
		}},
	}
	got := ContentForModelWithinLimit(largeResultTestContext(), result, 64, "read_tool_result")
	if !strings.Contains(got, "raw_result_too_large") || strings.Contains(got, "página") {
		t.Fatalf("página raw foi repaginada em vez de rejeitada: %q", got)
	}
}

func TestContentForModelWithinLimitNeverExceedsTinyBudget(t *testing.T) {
	for _, budget := range []int{0, 1, 8} {
		got := ContentForModelWithinLimit(context.Background(), ToolResult{Content: "raw", RawExact: true}, budget, "")
		if len(got) > budget {
			t.Fatalf("falha raw excedeu budget %d: %q", budget, got)
		}
	}
}

func TestContentForModelWithinLimitKeepsMCPPreviewDelimited(t *testing.T) {
	original := strings.Repeat(`{"value":"abcdef"}`, 1000)
	first, ok := ProtectExternalModelResult(largeResultTestContext(), ToolResult{Content: original}, 4096)
	if !ok {
		t.Fatal("primeira proteção MCP falhou")
	}
	firstReturned := first.Annotations.OutputWindow.Returned
	got := ContentForModelWithinLimit(largeResultTestContext(), first, 1024, "mcp_server__large")
	if len(got) > 1024 || !strings.Contains(got, mcpPreviewPrefix) || !strings.Contains(got, mcpPreviewSuffix) {
		t.Fatalf("prévia MCP recomposta incorretamente: %q", got)
	}
	if first.Annotations.OutputWindow.Returned != firstReturned {
		t.Fatal("reproteção MCP alterou o resultado original por aliasing")
	}
}

func TestContentForModelWithinLimitDoesNotInferMCPFromContent(t *testing.T) {
	content := mcpPreviewPrefix + strings.Repeat("x", 2000)
	got := ContentForModelWithinLimit(largeResultTestContext(), ToolResult{Content: content}, 1024, "ordinary_tool")
	if strings.Count(got, mcpPreviewPrefix) != 1 {
		t.Fatalf("texto comum foi reclassificado como MCP: %q", got)
	}
}

func TestContentForModelWithinLimitDoesNotReclassifyExistingWindowAsJSON(t *testing.T) {
	original := "{}" + strings.Repeat(" ", 10_000) + "texto"
	first, ok := ProtectToolResult(largeResultTestContext(), ToolResult{Content: original}, 4096)
	if !ok {
		t.Fatal("proteção inicial falhou")
	}
	if !IsCanonicalJSON(first.Content) {
		t.Fatalf("fixture não produziu prévia acidentalmente JSON: %q", first.Content)
	}
	got := ContentForModelWithinLimit(largeResultTestContext(), first, 1024, "ordinary_tool")
	if !strings.Contains(got, `"result_id"`) || strings.Contains(got, "[result_too_large]") {
		t.Fatalf("janela retomável foi reclassificada como JSON: %q", got)
	}
}

func TestReadToolResultIsExactOrExecutorRejectsPage(t *testing.T) {
	id, ok := storeModelResult(largeResultTestContext(), strings.Repeat("página-", 1000))
	if !ok {
		t.Fatal("store falhou")
	}
	registry := NewRegistry()
	registry.MustRegister(NewReadToolResult())
	cfg := DefaultExecutorConfig()
	cfg.MaxResultSize = 128
	got := NewExecutor(registry, cfg).ExecuteOne(largeResultTestContext(), ToolCall{
		ID: "call-page",
		Function: FunctionCall{
			Name:      "read_tool_result",
			Arguments: `{"result_id":"` + id + `","offset":0,"limit":100}`,
		},
	})
	if got.ErrorCode != "raw_result_too_large" || !got.Result.IsError {
		t.Fatalf("página foi cortada ou recomeçada silenciosamente: %+v", got)
	}
}

func TestReadToolResultNeverExceedsLimitForMultibyteRune(t *testing.T) {
	id, ok := storeModelResult(largeResultTestContext(), "ç")
	if !ok {
		t.Fatal("store falhou")
	}
	raw, _ := json.Marshal(map[string]any{"result_id": id, "offset": 0, "limit": 1})
	got, _ := NewReadToolResult().Execute(largeResultTestContext(), raw)
	if !got.IsError || got.Failure == nil || got.Failure.Code != "result_page_limit_too_small" {
		t.Fatalf("página deveria falhar sem exceder limit: %+v", got)
	}
}

func TestReadToolResultEnforcesUserAndConversationOwnership(t *testing.T) {
	ownerCtx := userctx.WithUserID(context.Background(), "user-a")
	ownerCtx = invocationctx.With(ownerCtx, invocationctx.InvocationContext{ConversationID: "conversation-a"})
	id, ok := storeModelResult(ownerCtx, "conteúdo privado")
	if !ok {
		t.Fatal("store falhou")
	}
	args, _ := json.Marshal(map[string]any{"result_id": id, "offset": 0})

	otherConversation := userctx.WithUserID(context.Background(), "user-a")
	otherConversation = invocationctx.With(otherConversation, invocationctx.InvocationContext{ConversationID: "conversation-b"})
	denied, _ := NewReadToolResult().Execute(otherConversation, args)
	if !denied.IsError || denied.Failure == nil || denied.Failure.Code != "large_result_not_found" {
		t.Fatalf("outra conversa acessou resultado: %+v", denied)
	}

	allowed, _ := NewReadToolResult().Execute(ownerCtx, args)
	if allowed.IsError || allowed.Content != "conteúdo privado" {
		t.Fatalf("owner não conseguiu reler resultado: %+v", allowed)
	}
}
