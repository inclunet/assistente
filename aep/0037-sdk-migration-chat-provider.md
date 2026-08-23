# SDK Migration + ChatProvider Interface

## Status: In Progress — providers e integração entregues; cleanup da Fase 6 permanece parcial

---

## Motivacao

A camada LLM atual (`internal/llm/client.go`, `sync_client.go`) reimplementa manualmente:

- Parsing de SSE streaming (fragil com edge cases)
- Acumulacao de tool calls em chunks de delta
- Tratamento de reasoning/thinking por provider
- Serializacao/desserializacao de tipos de request/response
- Gerenciamento de timeouts e erros HTTP

Tudo isso e feito por **SDKs oficiais** que sao mantidas, testadas e atualizadas pelos proprios vendors. Alem disso, o cliente manual usa exclusivamente o endpoint **Chat Completions** (`/v1/chat/completions`), que nao suporta MCP nativo em nenhum provider.

---

## Decisao: Adotar SDKs Oficiais

### SDKs

| Provider | SDK Go | Modulo |
|----------|--------|--------|
| OpenAI + OpenAI-compatible | `openai-go` | `github.com/openai/openai-go` |
| Anthropic | `anthropic-sdk-go` | `github.com/anthropics/anthropic-sdk-go` |
| Google Gemini | `genai` | `google.golang.org/genai` |

### Capacidades das SDKs

Todas suportam:

- **Custom base URL** (`option.WithBaseURL`) — permite OpenRouter, Ollama, etc. via `openai-go`
- **Custom HTTP client** (`option.WithHTTPClient`) — injeta `credMgr` como `http.RoundTripper`
- **API key dinamica** (`option.WithAPIKey`)
- **Streaming nativo**
- **Tool calling tipado**

A SDK `openai-go` com base URL customizada cobre todos os providers OpenAI-compatible: OpenAI, OpenRouter, Ollama, Together, Groq, Mistral, LiteLLM, DeepSeek, Fireworks, Perplexity, Azure OpenAI.

---

## Campo `api_format` no ProviderConfig

Novo campo que determina qual SDK/protocolo usar:

```go
type APIFormat string

const (
    APIFormatOpenAI          APIFormat = "openai"           // Chat Completions only (OpenAI-compatible)
    APIFormatOpenAICompatible          = APIFormatOpenAI    // alias semantico
    APIFormatOpenAIResponses APIFormat = "openai_responses" // Responses API first (OpenAI real)
    APIFormatAnthropic       APIFormat = "anthropic"        // anthropic-sdk-go SDK
    APIFormatGoogle          APIFormat = "google"           // google.golang.org/genai SDK
)

type ProviderConfig struct {
    // ... campos existentes ...
    APIFormat APIFormat `json:"api_format,omitempty"` // default inferido por GetAPIFormat()
}
```

`api_format` e independente de `ProviderType` (que continua como label de marca):

Extensões opcionais dentro de um formato também são capabilities explícitas,
nunca inferências por endpoint. O contrato de `reasoning_content_mode` e a
proibição de detecção por URL, marca ou modelo estão no
[AEP-0097](0097-capabilities-de-protocolo-por-provedor.md).

| Cenario | ProviderType | api_format | SDK | MCP Nativo |
|---------|-------------|------------|-----|------------|
| OpenAI direto | openai | openai_responses | openai-go (Responses API) | Sim |
| Claude direto | claude | anthropic | anthropic-sdk-go | Sim |
| Claude via OpenRouter | claude | openai | openai-go (Chat Completions) | Nao |
| Ollama local | ollama | openai | openai-go (Chat Completions) | Nao |
| Gemini direto | gemini | google | genai | Nao |
| DeepSeek | deepseek | openai | openai-go (Chat Completions) | Nao |
| Custom endpoint | custom | openai | openai-go (Chat Completions) | Nao |

> **Default conservador:** Quando `api_format` nao esta definido, `GetAPIFormat()` infere `openai_responses` para URLs com `api.openai.com`. Qualquer outro URL cai em `openai` (Chat Completions). Isso garante compatibilidade com configs legadas sem exigir migracao.

---

## Interface ChatProvider

```go
type ChatProvider interface {
    // StreamChat envia mensagens com streaming e executa handler para cada evento.
    // O handler recebe tokens de texto, tool calls, e sinais de conclusao.
    StreamChat(ctx context.Context, messages []Message, params StreamParams,
        handler StreamHandler, tools ...ToolDefinition) error

    // SupportsNativeMCP indica se este provider suporta MCP connector nativo.
    SupportsNativeMCP() bool

    // WithMCPServers retorna uma copia do provider configurada com MCP servers
    // para resolucao nativa server-side.
    WithMCPServers(servers []MCPServerConfig) ChatProvider
}
```

### Factory

```go
func NewChatProvider(provider *ProviderConfig, credMgr *credentials.Manager) ChatProvider {
    switch provider.GetAPIFormat() {
    case APIFormatAnthropic:
        return NewAnthropicProvider(provider, credMgr)
    case APIFormatGoogle:
        return NewGoogleProvider(provider, credMgr)
    case APIFormatOpenAIResponses:
        return NewOpenAIResponsesProvider(provider, credMgr)
    default:
        return NewOpenAIProvider(provider, credMgr)
    }
}
```

### Implementacoes

- **`OpenAIProvider(useResponses=false)`**: usa `openai-go` via Chat Completions API. Para provedores OpenAI-compatible (OpenRouter, Ollama, Groq, etc). `SupportsNativeMCP()` retorna `false`. Construtor: `NewOpenAIProvider()`.
- **`OpenAIProvider(useResponses=true)`**: usa `openai-go` via Responses API. Para OpenAI real. Suporta MCP nativo (`type: "mcp"`), reasoning summaries, tool_choice. `SupportsNativeMCP()` retorna `true`. Construtor: `NewOpenAIResponsesProvider()`.
- **`AnthropicProvider`**: usa `anthropic-sdk-go`. Messages API com MCP Connector (`mcp_servers[]` + `mcp_toolset`, beta header `mcp-client-2025-11-20`). `SupportsNativeMCP()` retorna `true`.
- **`GoogleProvider`**: usa `google.golang.org/genai`. Gemini API. `SupportsNativeMCP()` retorna `false` (nao implementado).

---

## Integracao com credMgr

Todas as SDKs aceitam `http.Client` customizado. O `credMgr` e integrado como um `http.RoundTripper`:

```go
type credentialTransport struct {
    base    http.RoundTripper
    credMgr *credentials.Manager
    host    string
}

func (t *credentialTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    if cred, ok := t.credMgr.Get(t.host); ok {
        req.Header.Set("Authorization", "Bearer "+cred)
    }
    return t.base.RoundTrip(req)
}
```

Uso:

```go
httpClient := &http.Client{
    Transport: &credentialTransport{
        base:    http.DefaultTransport,
        credMgr: credMgr,
        host:    provider.CredentialPattern,
    },
    Timeout: time.Duration(provider.Timeout) * time.Second,
}

// OpenAI
client := openai.NewClient(option.WithHTTPClient(httpClient), option.WithBaseURL(provider.BaseURL))

// Anthropic
client := anthropic.NewClient(option.WithHTTPClient(httpClient))

// Google
client, _ := genai.NewClient(ctx, &genai.ClientConfig{HTTPClient: httpClient})
```

---

## Integracao no Chat Loop

### `llm.go` (`sendMessageInternal`)

Antes:

```go
client := llm.NewClient(provider, cfg, credMgr)
// ... monta tools manualmente ...
client.StreamChat(ctx, messages, params, handler, tools...)
```

Depois:

```go
chatProvider := llm.NewChatProvider(provider, credMgr)

// MCP nativo: capability-driven, sem toggle de perfil
if chatProvider.SupportsNativeMCP() {
    httpServers := mcpMgr.GetEligibleNativeMCPServers()
    chatProvider = chatProvider.WithMCPServers(httpServers)
    // Remove bridge tools que agora vao por nativo (evita duplicata)
}

// Tools: internas + STDIO bridges (MCP HTTP servers nao vao como tools)
chatProvider.StreamChat(ctx, messages, params, handler, filteredTools...)
```

### `agent.go` (`runAgenticLoop`)

O loop continua identico na essencia:

1. Chama `chatProvider.StreamChat()` (em vez de `client.StreamChat()`)
2. Se `finish_reason == "tool_calls"` → executa localmente via `toolExecutor`
3. Se MCP nativo: tool calls de MCP servers HTTP sao resolvidas server-side e aparecem na resposta como blocos informativos (nao requerem execucao local)
4. Loop ate `finish_reason == "stop"`

---

## Roteamento Hibrido de Ferramentas

Quando MCP nativo esta ativo:

```
Tools no registry
├── Tools internas (task, task_list, ...) → function calling normal
├── MCP STDIO bridges (mcp_local_fs__*) → function calling normal (adapter)
└── MCP HTTP bridges (mcp_jira__*) → removidas do tools[], enviadas como MCP nativo
```

Quando MCP nativo esta **inativo** (ou provider nao suporta):

```
Tools no registry
├── Tools internas → function calling normal
├── MCP STDIO bridges → function calling normal (adapter)
└── MCP HTTP bridges → function calling normal (adapter) ← mesmo caminho
```

---

## Impacto nos Jobs

**Nenhum.** Jobs usam `toolRegistry.Get(name).Execute()` diretamente:

```
Job YAML → executor.go → toolRegistry.Get("mcp_jira__create_issue") → MCPToolBridge.Execute() → session.CallTool()
```

Este caminho nao envolve o cliente LLM nem o `ChatProvider`. MCP tools continuam registradas como `MCPToolBridge` no registry. Nenhuma alteracao necessaria.

---

## Migracao: Estrategia por Fases

### Fase 1: Foundation + OpenAI Provider

- Adicionar `APIFormat` ao `ProviderConfig` e ao model `LLMProvider` no banco
- Criar `credentialTransport`
- Implementar `OpenAIProvider` via `openai-go`
- Criar interface `ChatProvider` + factory
- Manter `client.go` original como fallback temporario

### Fase 2: Anthropic Provider

- Implementar `AnthropicProvider` via `anthropic-sdk-go`
- Suporte a streaming, tool calling, MCP Connector

### Fase 3: Google Provider

- Implementar `GoogleProvider` via `google.golang.org/genai`
- Suporte a streaming, tool calling

### Fase 4: Chat Integration

- `llm.go`: usar factory `NewChatProvider`, roteamento MCP hibrido
- `agent.go`: adaptar agentic loop para `ChatProvider`
- `mcp/manager.go`: `GetNativeEligibleServers()`

### Fase 5: Config e UI ✅

- ✅ Wizard define `api_format` automatico por provider
- ✅ Decisao MCP nativo e capability-driven (sem toggle no perfil)
- ✅ Frontend: dropdown `api_format` em ProviderForm com labels claros por formato
- ✅ OpenAI real usa `openai_responses` como default; providers compatible usam `openai`

### Fase 6: Cleanup 🚧

- ✅ Removidos: `mcp_mode`, `mcp_native_tested`, `ShouldUseMCPNative()`, `TestMCPNativeSupport()`, `ModelSupportsNativeMCP()`, `GetNativeServerInfo()`, `mcp_testing.go`
- ✅ `client.go` e `sync_client.go` já foram removidos.
- Pendente: revisar tipos manuais remanescentes de `types.go`.
- `internal/tools/http` permanece porque outros módulos ainda o utilizam; não
  pode ser removido como parte deste cleanup.

---

## Arquivos Afetados

### Novos

- `internal/llm/chat_provider.go` — interface + factory
- `internal/llm/openai_provider.go` — OpenAI SDK
- `internal/llm/anthropic_provider.go` — Anthropic SDK
- `internal/llm/google_provider.go` — Google GenAI SDK
- `internal/llm/credential_transport.go` — RoundTripper para credMgr

### Modificados

- `internal/llm/provider.go` — campo `APIFormat`
- `internal/database/models.go` — campo `APIFormat` + migration
- `llm.go` — factory de providers, roteamento MCP
- `agent.go` — agentic loop usando `ChatProvider`
- `app.go` — wizard, CRUD de providers
- `internal/mcp/manager.go` — `GetNativeEligibleServers()`
- `internal/profiles/types.go` — removidos campos legados `mcp_mode`, `mcp_native_tested` e metodos associados

### Removidos (Fase 6)

- `internal/llm/client.go`
- `internal/llm/sync_client.go`
- `internal/profiles/mcp_testing.go` ✅ removido

---

## Referencias

- [AEP-0021: MCP Modo Nativo (v2)](0021-mcp-native-mode.md)
- [AEP-0013: LLM Client Refactor](0013-llm-refactor.md)
- [AEP-0020: MCP Implementation](0020-mcp-implementation.md)
- [OpenAI Go SDK](https://github.com/openai/openai-go)
- [Anthropic Go SDK](https://github.com/anthropics/anthropic-sdk-go)
- [Google GenAI Go SDK](https://pkg.go.dev/google.golang.org/genai)
- [OpenAI Responses API + MCP](https://developers.openai.com/api/docs/guides/tools-remote-mcp)
- [Anthropic MCP Connector](https://docs.anthropic.com/en/docs/agents-and-tools/mcp-connector)
