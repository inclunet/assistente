package tools

import (
	"container/list"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
)

const (
	largeResultPageBytes  = 50 * 1024
	largeResultStoreBytes = 32 * 1024 * 1024
	largeResultStoreItems = 64
)

type storedLargeResult struct {
	id      string
	content string
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

func storeModelResult(content string) (string, bool) {
	if len(content) > largeResultStoreBytes {
		return "", false
	}
	s := modelResultStore
	s.mu.Lock()
	defer s.mu.Unlock()
	id := "tool-result-" + uuid.NewString()
	elem := s.order.PushFront(storedLargeResult{id: id, content: content})
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

func loadModelResult(id string) (string, bool) {
	s := modelResultStore
	s.mu.Lock()
	defer s.mu.Unlock()
	elem, ok := s.items[id]
	if !ok {
		return "", false
	}
	s.order.MoveToFront(elem)
	return elem.Value.(storedLargeResult).content, true
}

// ProtectModelResult aplica a barreira host-level sem inserir avisos no corpo.
// O conteúdo completo fica em armazenamento limitado e pode ser retomado pela
// tool read_tool_result.
func ProtectModelResult(result ToolResult, maxBytes int) (ToolResult, bool) {
	if maxBytes <= 0 || len(ContentForModel(result)) <= maxBytes {
		return result, true
	}
	id, ok := storeModelResult(result.Content)
	if !ok {
		return ToolResult{}, false
	}
	originalBytes := len(result.Content)
	preview := truncateUTF8(result.Content, maxBytes)
	if result.Annotations == nil {
		result.Annotations = &ResultAnnotations{}
	}
	result.Annotations.OutputWindow = &OutputWindowAnnotation{
		HasMore:       true,
		Unit:          "bytes",
		Offset:        0,
		Returned:      len(preview),
		Total:         originalBytes,
		NextOffset:    len(preview),
		ResultID:      id,
		OriginalBytes: originalBytes,
	}
	result.Content = preview
	if result.Metadata == nil {
		result.Metadata = make(map[string]any)
	}
	result.Metadata["truncated"] = true
	result.Metadata["result_id"] = id
	if maxBytes >= 1024 {
		for len(ContentForModel(result)) > maxBytes && len(result.Content) > 0 {
			over := len(ContentForModel(result)) - maxBytes
			nextSize := len(result.Content) - over - 16
			if nextSize < 0 {
				nextSize = 0
			}
			result.Content = truncateUTF8(result.Content, nextSize)
			result.Annotations.OutputWindow.Returned = len(result.Content)
			result.Annotations.OutputWindow.NextOffset = len(result.Content)
		}
		if len(ContentForModel(result)) > maxBytes {
			return ToolResult{}, false
		}
	}
	return result, true
}

// ProtectExternalModelResult delimita explicitamente a prévia de um resultado
// externo (MCP). O payload do servidor permanece intacto no store; nenhum campo
// é injetado em JSON retornado pelo servidor.
func ProtectExternalModelResult(result ToolResult, maxBytes int) (ToolResult, bool) {
	if maxBytes <= 0 || len(ContentForModel(result)) <= maxBytes {
		return result, true
	}
	original := result.Content
	id, ok := storeModelResult(original)
	if !ok {
		return ToolResult{}, false
	}
	prefix := "--- INÍCIO DA PRÉVIA MCP; NÃO É O RESULTADO COMPLETO ---\n"
	suffix := "\n--- FIM DA PRÉVIA MCP ---"
	budget := maxBytes - len(prefix) - len(suffix)
	if budget < 1 {
		return ToolResult{}, false
	}
	preview := truncateUTF8(original, budget)
	result.Content = prefix + preview + suffix
	if result.Annotations == nil {
		result.Annotations = &ResultAnnotations{}
	}
	result.Annotations.OutputWindow = &OutputWindowAnnotation{
		HasMore: true, Unit: "bytes", Offset: 0, Returned: len(preview),
		Total: len(original), NextOffset: len(preview), ResultID: id,
		OriginalBytes: len(original),
	}
	if result.Metadata == nil {
		result.Metadata = make(map[string]any)
	}
	result.Metadata["truncated"] = true
	result.Metadata["result_id"] = id
	for len(ContentForModel(result)) > maxBytes && budget > 0 {
		over := len(ContentForModel(result)) - maxBytes
		budget -= over + 16
		if budget < 1 {
			return ToolResult{}, false
		}
		preview = truncateUTF8(original, budget)
		result.Content = prefix + preview + suffix
		result.Annotations.OutputWindow.Returned = len(preview)
		result.Annotations.OutputWindow.NextOffset = len(preview)
	}
	return result, true
}

// ReadToolResult relê, por bytes, um resultado grande preservado pelo host.
type ReadToolResult struct{}

func NewReadToolResult() *ReadToolResult { return &ReadToolResult{} }

func (t *ReadToolResult) Name() string { return "read_tool_result" }

func (t *ReadToolResult) Description() string {
	return "Reads a later byte range from a large tool result preserved by the host. Use it with result_id and next_offset from output_window annotations; pages are at most 50 KiB and never alter the stored content. Do not use it for ordinary files or to repeat the original operation. Results are ephemeral and may expire after restart or storage pressure. Risk: read-only access requires an opaque result identifier."
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

func (t *ReadToolResult) Execute(_ context.Context, raw json.RawMessage) (ToolResult, error) {
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
	content, ok := loadModelResult(args.ResultID)
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
		end++
		for end < len(content) && !isUTF8Start(content[end]) {
			end++
		}
	}
	page := content[args.Offset:end]
	hasMore := end < len(content)
	return ToolResult{
		Content: page,
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
