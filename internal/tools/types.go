package tools

import (
	"context"
	"encoding/json"
)

// Tool define a interface que toda ferramenta deve implementar.
// Cada tool tem um nome, descrição, schema de parâmetros (JSON Schema)
// e um método Execute que recebe argumentos e retorna um resultado.
type Tool interface {
	// Name retorna o identificador único da ferramenta (ex: "read_file")
	Name() string

	// Description retorna uma descrição concisa para o LLM entender quando usar esta tool
	Description() string

	// Parameters retorna o JSON Schema dos parâmetros aceitos pela ferramenta.
	// Deve seguir o formato OpenAI: {"type":"object","properties":{...},"required":[...]}
	Parameters() json.RawMessage

	// Execute executa a ferramenta com os argumentos fornecidos.
	// O ctx permite cancelamento pelo usuário ou por timeout.
	// args é o JSON dos argumentos parseados pelo LLM.
	Execute(ctx context.Context, args json.RawMessage) (ToolResult, error)
}

// ToolResult representa o resultado da execução de uma ferramenta.
type ToolResult struct {
	// Content é o conteúdo textual do resultado (enviado de volta ao LLM)
	Content string `json:"content"`

	// IsError indica se o resultado representa um erro de execução.
	// Quando true, o LLM recebe o conteúdo como mensagem de erro.
	IsError bool `json:"is_error,omitempty"`

	// Metadata contém informações extras sobre a execução (não enviadas ao LLM).
	// Exemplos: bytes lidos, tempo de execução, número de resultados.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// ToolCall representa uma chamada de ferramenta solicitada pelo LLM.
// Corresponde ao formato tool_calls da API OpenAI-compatible.
type ToolCall struct {
	// ID é o identificador único da chamada (gerado pelo LLM, ex: "call_abc123")
	ID string `json:"id"`

	// Type é sempre "function" no protocolo atual
	Type string `json:"type"`

	// Function contém o nome e os argumentos da função chamada
	Function FunctionCall `json:"function"`
}

// FunctionCall contém os detalhes de uma chamada de função.
type FunctionCall struct {
	// Name é o nome da ferramenta (ex: "read_file")
	Name string `json:"name"`

	// Arguments é o JSON dos argumentos (ex: '{"path":"main.go"}')
	Arguments string `json:"arguments"`
}

// ToolDefinition representa a definição de uma ferramenta no formato OpenAI.
// É enviada no campo "tools" do ChatRequest para informar o LLM quais ferramentas estão disponíveis.
type ToolDefinition struct {
	// Type é sempre "function"
	Type string `json:"type"`

	// Function contém nome, descrição e schema de parâmetros
	Function FunctionDefinition `json:"function"`
}

// FunctionDefinition contém a especificação completa de uma função para o LLM.
type FunctionDefinition struct {
	// Name é o identificador da ferramenta
	Name string `json:"name"`

	// Description explica ao LLM quando e como usar a ferramenta
	Description string `json:"description"`

	// Parameters é o JSON Schema dos parâmetros aceitos
	Parameters json.RawMessage `json:"parameters"`
}

// ErrorKind classifica o tipo de erro de execução de uma tool (AEP-0039 Fase 3).
type ErrorKind string

const (
	ErrorKindNone       ErrorKind = ""              // Sem erro
	ErrorKindTimeout    ErrorKind = "timeout"       // Timeout de execução (retryable)
	ErrorKindInvalidArgs ErrorKind = "invalid_args" // JSON malformado nos argumentos (não retryable)
	ErrorKindNotFound   ErrorKind = "not_found"     // Ferramenta não encontrada no registry (não retryable)
	ErrorKindPanic      ErrorKind = "panic"         // Panic capturado durante execução (não retryable)
	ErrorKindCancelled  ErrorKind = "cancelled"     // Cancelamento pelo usuário (não retryable)
	ErrorKindUnknown    ErrorKind = "unknown"       // Erro genérico de execução (não retryable)
)

// ToolExecutionResult agrupa o resultado de uma execução com metadados do call original.
// Usado pelo executor para devolver resultados ao agentic loop.
type ToolExecutionResult struct {
	// CallID é o ID do ToolCall que originou esta execução
	CallID string `json:"call_id"`

	// ToolName é o nome da ferramenta executada
	ToolName string `json:"tool_name"`

	// Result é o resultado da execução
	Result ToolResult `json:"result"`

	// Error é o erro de execução (nil se sucesso)
	Error error `json:"-"`

	// ErrorKind classifica o tipo de erro (AEP-0039 Fase 3)
	ErrorKind ErrorKind `json:"error_kind,omitempty"`

	// Retryable indica se o erro permite retry automático (AEP-0039 Fase 3)
	Retryable bool `json:"retryable,omitempty"`

	// DurationMs é a duração da execução em milissegundos (AEP-0039 Fase 3)
	DurationMs int64 `json:"duration_ms,omitempty"`
}
