# MCP Modo Nativo

## Status: Revisado (v3)

> **Historico:** A versao original desta AEP descrevia um conceito aspiracional de modo nativo com `mcp_mode` (adapter/native/auto) no perfil. A infraestrutura foi parcialmente criada (`GetNativeServerInfo`, `TestMCPNativeSupport`, `ShouldUseMCPNative`) mas **nunca foi consumida no chat loop**. O sistema sempre operou em modo adapter.
>
> A v2 refletia a arquitetura [AEP-0037](0037-sdk-migration-chat-provider.md), mas descrevia `api_format: "openai"` como suportando MCP nativo, o que era incorreto.
>
> Esta v3 reflete a separacao real entre `openai` (Chat Completions only) e `openai_responses` (Responses API first), implementada no runtime.

---

## O que e MCP Nativo?

MCP nativo permite que o **provider LLM** se conecte diretamente a MCP servers remotos, sem que o Assistente precise fazer bridge local. O provider descobre as tools, chama-as, e retorna os resultados — tudo server-side.

### Modo Adapter (atual, funcional)

```
Usuario → LLM → tool_call → Assistente → MCPToolBridge → MCP Server
                                ↑ execucao local
```

### Modo Nativo (novo)

```
Usuario → LLM+Provider API → MCP Server (direto, server-side)
```

---

## Suporte por Provider (marco 2026)

| Provider | api_format | API | MCP Nativo | Formato |
|----------|------------|-----|------------|---------|
| OpenAI (real) | `openai_responses` | Responses API (`/v1/responses`) | **Sim** | `type: "mcp"` tool com `server_url` |
| Anthropic | `anthropic` | Messages API (`/v1/messages`) | **Sim** | `mcp_servers[]` + `mcp_toolset` + beta header |
| Google (Gemini) | `google` | Gemini API | **Nao** | Nao implementado |
| OpenAI-compatible | `openai` | Chat Completions (`/v1/chat/completions`) | **Nao** | N/A |

> **Nota:** `api_format: "openai"` (Chat Completions) **nao** suporta MCP nativo. Apenas `openai_responses` (Responses API) suporta. Essa separacao foi feita para nao fingir suporte em provedores OpenAI-compatible (OpenRouter, Ollama, Groq, etc) que apenas falam Chat Completions.

### Limitacoes comuns

- Apenas MCP servers **remotos HTTP** (SSE / Streamable HTTP) podem ser usados nativamente
- Servers **STDIO locais** devem continuar usando adapter (MCPToolBridge)
- Apenas **tool calls** sao suportados nativamente (nao resources/prompts/sampling)

---

## Nova Arquitetura

### Determinacao automatica via `api_format`

O campo `api_format` no `ProviderConfig` (ver [AEP-0037](0037-sdk-migration-chat-provider.md)) determina qual SDK/protocolo e usado. Cada `ChatProvider` implementa `SupportsNativeMCP() bool`:

- `api_format: "openai_responses"` → `OpenAIProvider(useResponses=true)` → Responses API → **suporta MCP nativo**
- `api_format: "openai"` → `OpenAIProvider(useResponses=false)` → Chat Completions → **nao suporta MCP nativo**
- `api_format: "anthropic"` → `AnthropicProvider` → Messages API → **suporta MCP nativo**
- `api_format: "google"` → `GoogleProvider` → Gemini API → **nao suporta MCP nativo**

Se o provider suporta MCP nativo (`SupportsNativeMCP() == true`), o Assistente envia os MCP servers HTTP diretamente na request ao provider via `WithMCPServers()`. MCP servers STDIO continuam como bridges no `tools[]` (adapter).

Providers sem suporte nativo (`SupportsNativeMCP() == false`) usam todos os MCP servers via adapter/bridge local, independente do tipo de transporte.

**Inferencia automatica:** Quando `api_format` nao esta definido, `GetAPIFormat()` infere `openai_responses` se o BaseURL contem `api.openai.com`. Qualquer outro URL cai no default conservador `openai` (Chat Completions).

### Decisao em runtime (sem toggle no perfil)

A decisao e puramente capability-driven, sem campo de perfil:

```
usarNativo = provider.SupportsNativeMCP() && server.Transport != "stdio"
```

O perfil nao participa da decisao. Se o provider suporta MCP nativo e o servidor e HTTP remoto, o caminho nativo e usado automaticamente.

### Codigo legado removido

Os seguintes artefatos da era `mcp_mode` foram removidos por nao terem efeito operacional:

- `mcp_mode` (campo de perfil "adapter"/"native"/"auto") — nunca consumido pelo chat loop
- `mcp_native_tested` (campo de perfil) — resultado de teste que nunca influenciou o runtime
- `ShouldUseMCPNative()` — decisao por profile, substituida por capability do provider
- `TestMCPNativeSupport()` — teste real a API que nunca foi integrado ao chat loop
- `ModelSupportsNativeMCP()` — heuristica hardcoded por nome de modelo
- `GetNativeServerInfo()` — retornava info para frontend que nunca a consumiu
- `GetMCPMode()`, `MCPNativeWasTested()`, `SetMCPNativeSupport()`, `ClearMCPTest()` — helpers dos campos acima

---

## Coexistencia: Tools Locais + MCP Nativo

Na mesma request, o Assistente envia:

1. **Tools internas** (task, task_list, etc.) como function calling normal
2. **MCP servers HTTP** na config MCP nativa do provider
3. **MCP servers STDIO** como bridges no function calling

O modelo pode usar todas simultaneamente. O agentic loop so executa localmente tools internas e STDIO bridges. MCP nativos sao resolvidos server-side.

### Exemplo: Anthropic

```json
{
  "model": "claude-opus-4-6",
  "tools": [
    {"type": "custom", "name": "task", "description": "...", "input_schema": {}},
    {"type": "custom", "name": "mcp_local_fs__read_file", "description": "...", "input_schema": {}},
    {"type": "mcp_toolset", "mcp_server_name": "jira-mcp"}
  ],
  "mcp_servers": [
    {"type": "url", "url": "https://jira-mcp.example.com/sse", "name": "jira-mcp"}
  ],
  "messages": [{"role": "user", "content": "Crie um ticket no Jira"}]
}
```

### Exemplo: OpenAI

```json
{
  "model": "gpt-5",
  "tools": [
    {"type": "function", "name": "task", "function": {"description": "...", "parameters": {}}},
    {"type": "function", "name": "mcp_local_fs__read_file", "function": {"description": "...", "parameters": {}}},
    {"type": "mcp", "server_label": "jira-mcp", "server_url": "https://jira-mcp.example.com/sse", "require_approval": "never"}
  ],
  "input": [{"role": "user", "content": "Crie um ticket no Jira"}]
}
```

---

## Impacto nos Jobs

**Nenhum.** Jobs chamam `toolRegistry.Get(name).Execute()` diretamente, sem LLM. MCP tools continuam registradas como `MCPToolBridge` no registry e funcionam identicamente.

---

## Servidor Elegivel para MCP Nativo

`GetNativeEligibleServers()` retorna apenas servers que:

1. Estao conectados
2. Tem transporte HTTP (SSE ou Streamable HTTP)
3. Tem URL elegivel (ver regra abaixo)
4. Tem pelo menos uma tool disponivel

Servers STDIO sao automaticamente excluidos e continuam via adapter.

**Regra de URL para MCP nativo:**

- `https://` → sempre elegivel
- `http://localhost`, `http://127.0.0.1`, `http://[::1]` → elegivel (dev local)
- `http://` com host remoto → **excluido** do caminho nativo (cai para adapter com log warning)

Racional: no MCP nativo, o provider LLM (OpenAI, Anthropic) faz a conexao server-side. Enviar auth tokens por HTTP sem encriptacao para hosts remotos e inseguro. URLs localhost sao permitidas para facilitar desenvolvimento e testes locais.

---

## Referencias

- [AEP-0020: MCP Implementation](0020-mcp-implementation.md)
- [AEP-0037: SDK Migration + ChatProvider](0037-sdk-migration-chat-provider.md)
- [Anthropic MCP Connector](https://docs.anthropic.com/en/docs/agents-and-tools/mcp-connector)
- [OpenAI MCP and Connectors](https://developers.openai.com/api/docs/guides/tools-remote-mcp)
