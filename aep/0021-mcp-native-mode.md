# MCP Modo Nativo

## Status: Revisado (v2)

> **Historico:** A versao original desta AEP descrevia um conceito aspiracional de modo nativo com `mcp_mode` (adapter/native/auto) no perfil. A infraestrutura foi parcialmente criada (`GetNativeServerInfo`, `TestMCPNativeSupport`, `ShouldUseMCPNative`) mas **nunca foi consumida no chat loop**. O sistema sempre operou em modo adapter.
>
> Esta revisao reflete a nova arquitetura definida na [AEP-0037](0037-sdk-migration-chat-provider.md).

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

| Provider | API | MCP Nativo | Formato |
|----------|-----|------------|---------|
| OpenAI | Responses API (`/v1/responses`) | Sim, via `type: "mcp"` tool | `server_url` + `server_label` |
| Anthropic | Messages API (`/v1/messages`) | Sim, via `mcp_servers[]` + `mcp_toolset` | `mcp_servers` array + beta header |
| Google | Gemini API | Experimental | Em evolucao |
| OpenAI-compat | Chat Completions (`/v1/chat/completions`) | Nao | N/A |

### Limitacoes comuns

- Apenas MCP servers **remotos HTTP** (SSE / Streamable HTTP) podem ser usados nativamente
- Servers **STDIO locais** devem continuar usando adapter (MCPToolBridge)
- Apenas **tool calls** sao suportados nativamente (nao resources/prompts/sampling)

---

## Nova Arquitetura

### Determinacao automatica via `api_format`

O campo `api_format` no `ProviderConfig` (ver [AEP-0037](0037-sdk-migration-chat-provider.md)) determina qual SDK/protocolo e usado. Cada `ChatProvider` implementa `SupportsNativeMCP() bool`:

- `api_format: "openai"` → `OpenAIProvider` → Responses API → **suporta MCP nativo**
- `api_format: "anthropic"` → `AnthropicProvider` → Messages API → **suporta MCP nativo**
- `api_format: "google"` → `GoogleProvider` → Gemini API → **suporte experimental**

Se o provider suporta MCP nativo, o Assistente envia os MCP servers HTTP diretamente na request ao provider. MCP servers STDIO continuam como bridges no `tools[]` (adapter).

### Toggle no perfil: `native_mcp`

Substituicao do antigo `mcp_mode` (adapter/native/auto):

```json
{
  "chat": {
    "native_mcp": true
  }
}
```

- `true` (default): usa MCP nativo quando o provider suporta
- `false`: forca modo adapter para todos os MCP servers (util para debug/auditoria)

Logica em runtime:

```
usarNativo = provider.SupportsNativeMCP() && profile.NativeMCP && server.Transport != "stdio"
```

### Campos deprecados (remover)

- `mcp_mode` (string "adapter"/"native"/"auto") → substituido por `native_mcp` bool
- `mcp_native_tested` → desnecessario (capacidade vem do `api_format`)
- `ShouldUseMCPNative()` → substituido pela logica acima
- `TestMCPNativeSupport()` → desnecessario

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
3. Tem URL valida (HTTPS para providers remotos)
4. Tem pelo menos uma tool disponivel

Servers STDIO sao automaticamente excluidos e continuam via adapter.

---

## Referencias

- [AEP-0020: MCP Implementation](0020-mcp-implementation.md)
- [AEP-0037: SDK Migration + ChatProvider](0037-sdk-migration-chat-provider.md)
- [Anthropic MCP Connector](https://docs.anthropic.com/en/docs/agents-and-tools/mcp-connector)
- [OpenAI MCP and Connectors](https://developers.openai.com/api/docs/guides/tools-remote-mcp)
