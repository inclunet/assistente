package tools

import (
	"container/list"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"assistente/internal/tools/invocationctx"
	"assistente/internal/userctx"

	"github.com/google/uuid"
)

const (
	largeResultPageBytes  = 50 * 1024
	largeResultStoreBytes = 32 * 1024 * 1024
	largeResultStoreItems = 64
	mcpPreviewPrefix      = "--- INÍCIO DA PRÉVIA MCP; NÃO É O RESULTADO COMPLETO ---\n"
	mcpPreviewSuffix      = "\n--- FIM DA PRÉVIA MCP ---"
	envelopeFitSlackBytes = 16 // absorve mudanças de dígitos nos offsets do JSON
)

type storedLargeResult struct {
	id      string
	content string
	owner   largeResultOwner
}

type largeResultOwner struct {
	userID         string
	conversationID string
}

type largeResultStore struct {
	mu    sync.Mutex
	items map[string]*list.Element
	order *list.List
	bytes int
}

var modelResultStore = &largeResultStore{
	items: make(map[string]*list.Element),
	order: list.New(),
}

func storeModelResult(ctx context.Context, content string) (string, bool) {
	if len(content) > largeResultStoreBytes {
		return "", false
	}
	s := modelResultStore
	s.mu.Lock()
	defer s.mu.Unlock()
	id := "tool-result-" + uuid.NewString()
	elem := s.order.PushFront(storedLargeResult{id: id, content: content, owner: largeResultOwnerFromContext(ctx)})
	s.items[id] = elem
	s.bytes += len(content)
	for s.bytes > largeResultStoreBytes || s.order.Len() > largeResultStoreItems {
		last := s.order.Back()
		entry := last.Value.(storedLargeResult)
		delete(s.items, entry.id)
		s.bytes -= len(entry.content)
		s.order.Remove(last)
	}
	return id, true
}

func loadModelResult(ctx context.Context, id string) (string, bool) {
	s := modelResultStore
	s.mu.Lock()
	defer s.mu.Unlock()
	elem, ok := s.items[id]
	if !ok {
		return "", false
	}
	entry := elem.Value.(storedLargeResult)
	if entry.owner != largeResultOwnerFromContext(ctx) {
		return "", false
	}
	s.order.MoveToFront(elem)
	return entry.content, true
}

func largeResultOwnerFromContext(ctx context.Context) largeResultOwner {
	userID, _ := userctx.UserIDFromContext(ctx)
	invocation, _ := invocationctx.Get(ctx)
	return largeResultOwner{userID: userID, conversationID: invocation.ConversationID}
}

// ProtectModelResult aplica a barreira host-level sem inserir avisos no corpo.
// O conteúdo completo fica em armazenamento limitado e pode ser retomado pela
// tool read_tool_result.
func ProtectModelResult(ctx context.Context, result ToolResult, maxBytes int) (ToolResult, bool) {
	return protectModelResult(ctx, result, maxBytes, true)
}

// ProtectToolResult aplica um limite próprio da tool ao corpo. O executor ainda
// aplicará depois o limite global à mensagem completa, incluindo o envelope.
func ProtectToolResult(ctx context.Context, result ToolResult, maxContentBytes int) (ToolResult, bool) {
	return protectModelResult(ctx, result, maxContentBytes, false)
}

// ContentForModelWithinLimit recompõe um resultado para um budget de contexto
// menor que o budget do executor. Resultados exatos nunca são cortados; textos
// retomáveis recebem uma nova janela coerente com os bytes realmente enviados.
func ContentForModelWithinLimit(ctx context.Context, result ToolResult, maxBytes int, toolName string) string {
	content := ContentForModel(result)
	if len(content) <= maxBytes {
		return content
	}
	mcpBridge := isMCPBridgeToolName(toolName)
	hasWindow := result.Annotations != nil && result.Annotations.OutputWindow != nil
	if !mcpBridge && (result.RawExact || result.Structured || (!hasWindow && IsCanonicalJSON(result.Content))) {
		code := "result_too_large"
		kind := "estruturado"
		if result.RawExact {
			code = "raw_result_too_large"
			kind = "raw"
		}
		failure := fmt.Sprintf(
			"[%s] Resultado %s integral não cabe no contexto disponível; reduza offset/limit ou o escopo da chamada.",
			code, kind,
		)
		if len(failure) <= maxBytes {
			return failure
		}
		return truncateUTF8("["+code+"]", maxBytes)
	}
	if maxBytes <= 0 {
		return ""
	}

	var (
		protected ToolResult
		ok        bool
	)
	if mcpBridge {
		protected, ok = ProtectExternalModelResult(ctx, result, maxBytes)
	} else {
		protected, ok = ProtectModelResult(ctx, result, maxBytes)
	}
	if ok {
		return ContentForModel(protected)
	}
	failure := "[result_storage_limit] Resultado retomável indisponível; execute novamente a tool de origem."
	if len(failure) <= maxBytes {
		return failure
	}
	return ""
}

func protectModelResult(ctx context.Context, result ToolResult, maxBytes int, includeEnvelope bool) (ToolResult, bool) {
	currentBytes := len(result.Content)
	if includeEnvelope {
		currentBytes = len(ContentForModel(result))
	}
	if maxBytes <= 0 || currentBytes <= maxBytes {
		return result, true
	}
	original := result.Content
	id := ""
	var sourceWindow *OutputWindowAnnotation
	if result.Annotations != nil && result.Annotations.OutputWindow != nil {
		existing := result.Annotations.OutputWindow
		sourceWindow = existing
		if existing.ResultID != "" {
			if stored, found := loadModelResult(ctx, existing.ResultID); found {
				original = stored
				id = existing.ResultID
				sourceWindow = existing.SourceWindow
			} else {
				// A prévia não contém bytes suficientes para reconstruir o
				// resultado. Publicar outro ID aqui criaria uma continuação
				// aparentemente válida, porém incompleta.
				return ToolResult{}, false
			}
		}
	}
	if id == "" {
		var ok bool
		id, ok = storeModelResult(ctx, original)
		if !ok {
			return ToolResult{}, false
		}
	}
	originalBytes := len(original)
	preview := truncateUTF8(original, maxBytes)
	result = cloneMutableResultFields(result)
	result.Annotations.OutputWindow = &OutputWindowAnnotation{
		HasMore:       true,
		Unit:          "bytes",
		Offset:        0,
		Returned:      len(preview),
		Total:         originalBytes,
		NextOffset:    len(preview),
		ResultID:      id,
		OriginalBytes: originalBytes,
		SourceWindow:  sourceWindow,
	}
	result.Content = preview
	result.Metadata["truncated"] = true
	result.Metadata["result_id"] = id
	if includeEnvelope {
		modelContent := ContentForModel(result)
		for len(modelContent) > maxBytes && len(result.Content) > 0 {
			over := len(modelContent) - maxBytes
			nextSize := len(result.Content) - over - envelopeFitSlackBytes
			if nextSize < 0 {
				nextSize = 0
			}
			result.Content = truncateUTF8(result.Content, nextSize)
			result.Annotations.OutputWindow.Returned = len(result.Content)
			result.Annotations.OutputWindow.NextOffset = len(result.Content)
			modelContent = ContentForModel(result)
		}
		if len(modelContent) > maxBytes {
			return ToolResult{}, false
		}
	}
	return result, true
}

// ProtectExternalModelResult delimita explicitamente a prévia de um resultado
// externo (MCP). O payload do servidor permanece intacto no store; nenhum campo
// é injetado em JSON retornado pelo servidor.
func ProtectExternalModelResult(ctx context.Context, result ToolResult, maxBytes int) (ToolResult, bool) {
	if maxBytes <= 0 || len(ContentForModel(result)) <= maxBytes {
		return result, true
	}
	original := result.Content
	id := ""
	var sourceWindow *OutputWindowAnnotation
	if result.Annotations != nil && result.Annotations.OutputWindow != nil {
		existing := result.Annotations.OutputWindow
		sourceWindow = existing
		if existing.ResultID != "" {
			if stored, found := loadModelResult(ctx, existing.ResultID); found {
				original = stored
				id = existing.ResultID
				sourceWindow = existing.SourceWindow
			} else {
				return ToolResult{}, false
			}
		}
	}
	if id == "" {
		var ok bool
		id, ok = storeModelResult(ctx, original)
		if !ok {
			return ToolResult{}, false
		}
	}
	budget := maxBytes - len(mcpPreviewPrefix) - len(mcpPreviewSuffix)
	if budget < 1 {
		return ToolResult{}, false
	}
	preview := truncateUTF8(original, budget)
	result.Content = mcpPreviewPrefix + preview + mcpPreviewSuffix
	result = cloneMutableResultFields(result)
	result.Annotations.OutputWindow = &OutputWindowAnnotation{
		HasMore: true, Unit: "bytes", Offset: 0, Returned: len(preview),
		Total: len(original), NextOffset: len(preview), ResultID: id,
		OriginalBytes: len(original), SourceWindow: sourceWindow,
	}
	result.Metadata["truncated"] = true
	result.Metadata["result_id"] = id
	modelContent := ContentForModel(result)
	for len(modelContent) > maxBytes && budget > 0 {
		over := len(modelContent) - maxBytes
		budget -= over + envelopeFitSlackBytes
		if budget < 1 {
			return ToolResult{}, false
		}
		preview = truncateUTF8(original, budget)
		result.Content = mcpPreviewPrefix + preview + mcpPreviewSuffix
		result.Annotations.OutputWindow.Returned = len(preview)
		result.Annotations.OutputWindow.NextOffset = len(preview)
		modelContent = ContentForModel(result)
	}
	if len(modelContent) > maxBytes {
		return ToolResult{}, false
	}
	return result, true
}

func cloneMutableResultFields(result ToolResult) ToolResult {
	if result.Annotations == nil {
		result.Annotations = &ResultAnnotations{}
	} else {
		annotations := *result.Annotations
		result.Annotations = &annotations
	}
	metadata := make(map[string]any, len(result.Metadata)+2)
	for key, value := range result.Metadata {
		metadata[key] = value
	}
	result.Metadata = metadata
	return result
}

// ReadToolResult relê, por bytes, um resultado grande preservado pelo host.
type ReadToolResult struct{}

func NewReadToolResult() *ReadToolResult { return &ReadToolResult{} }

func (t *ReadToolResult) Name() string { return "read_tool_result" }

func (t *ReadToolResult) Description() string {
	return "Reads a later byte range from a large tool result preserved by the host. Use it with result_id and next_offset from output_window annotations; pages are at most 50 KiB and never alter the stored content. Do not use it for ordinary files or to repeat the original operation. Results are scoped to their user and conversation, are ephemeral, and may expire after restart or storage pressure. Risk: read-only access requires an opaque result identifier."
}

func (t *ReadToolResult) CatalogMetadata() CatalogMetadata {
	return CatalogMetadata{Category: "system", Class: "read_context", Package: "coding_readonly", Risk: "read"}
}

func (t *ReadToolResult) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"result_id":{"type":"string","description":"Opaque result_id from output_window annotations."},
			"offset":{"type":"integer","minimum":0,"description":"Byte offset to resume from."},
			"limit":{"type":"integer","minimum":1,"maximum":51200,"description":"Maximum bytes to return; default 51200."}
		},
		"required":["result_id","offset"],
		"additionalProperties":false
	}`)
}

func (t *ReadToolResult) Execute(ctx context.Context, raw json.RawMessage) (ToolResult, error) {
	var args struct {
		ResultID string `json:"result_id"`
		Offset   int    `json:"offset"`
		Limit    int    `json:"limit"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return ToolResult{Content: "Parâmetros inválidos: " + err.Error(), IsError: true}, nil
	}
	args.ResultID = strings.TrimSpace(args.ResultID)
	if args.ResultID == "" || args.Offset < 0 {
		return ToolResult{Content: "result_id e offset não negativo são obrigatórios", IsError: true}, nil
	}
	content, ok := loadModelResult(ctx, args.ResultID)
	if !ok {
		return ToolResult{
			Content: "Resultado grande não encontrado ou expirado; execute novamente a tool de origem.",
			IsError: true,
			Failure: &ToolFailure{Code: "large_result_not_found", Kind: ErrorKindNotFound, Retryable: false},
		}, nil
	}
	if args.Offset > len(content) {
		return ToolResult{Content: fmt.Sprintf("offset %d excede o resultado de %d bytes", args.Offset, len(content)), IsError: true}, nil
	}
	if args.Offset < len(content) && !isUTF8Start(content[args.Offset]) {
		return ToolResult{Content: "offset aponta para o meio de um caractere UTF-8; use exatamente next_offset da página anterior", IsError: true}, nil
	}
	limit := args.Limit
	if limit <= 0 || limit > largeResultPageBytes {
		limit = largeResultPageBytes
	}
	end := args.Offset + limit
	if end > len(content) {
		end = len(content)
	}
	end = utf8BoundaryBefore(content, end)
	if end == args.Offset && end < len(content) {
		return ToolResult{
			Content: "limit é pequeno demais para conter o próximo caractere UTF-8 completo; aumente o limit",
			IsError: true,
			Failure: &ToolFailure{Code: "result_page_limit_too_small", Kind: ErrorKindInvalidArgs, Retryable: false},
		}, nil
	}
	page := content[args.Offset:end]
	hasMore := end < len(content)
	return ToolResult{
		Content:  page,
		RawExact: true,
		Annotations: &ResultAnnotations{OutputWindow: &OutputWindowAnnotation{
			HasMore:  hasMore,
			Unit:     "bytes",
			Offset:   args.Offset,
			Returned: len(page),
			Total:    len(content),
			NextOffset: func() int {
				if hasMore {
					return end
				}
				return 0
			}(),
			ResultID:      args.ResultID,
			OriginalBytes: len(content),
		}},
		Metadata: map[string]any{"result_id": args.ResultID, "offset": args.Offset, "returned": len(page)},
	}, nil
}

func utf8BoundaryBefore(s string, end int) int {
	if end >= len(s) {
		return len(s)
	}
	for end > 0 && !isUTF8Start(s[end]) {
		end--
	}
	return end
}

func isUTF8Start(b byte) bool { return b&0xC0 != 0x80 }

var _ Tool = (*ReadToolResult)(nil)
