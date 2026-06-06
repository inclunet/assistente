# MCP Modo Nativo

## Status: Revisado (v5)

> **Historico:** A versao original desta AEP descrevia um conceito aspiracional de modo nativo com `mcp_mode` (adapter/native/auto) no perfil. A infraestrutura foi parcialmente criada (`GetNativeServerInfo`, `TestMCPNativeSupport`, `ShouldUseMCPNative`) mas **nunca foi consumida no chat loop**. O sistema sempre operou em modo adapter.
>
> A v2 refletia a arquitetura [AEP-0037](0037-sdk-migration-chat-provider.md), mas descrevia `api_format: "openai"` como suportando MCP nativo, o que era incorreto.
>
> A v3 refletiu a separacao real entre `openai` (Chat Completions only) e `openai_responses` (Responses API first), e tratou o suporte como puramente capability-driven, sem participacao do perfil.
>
> A v4 tornou o suporte **configuravel POR PERFIL** (`Profile.Chat.NativeMCP`, tri-state), mas ainda usava uma **heuristica por endpoint** (`SupportsNativeMCP()` baseado em `api.openai.com`) como default do modo auto.
>
> **Esta v5** elimina qualquer heuristica por URL/endpoint da decisao de MCP nativo. Motivos: (a) prender a funcionalidade a um provider especifico (`api.openai.com`) e acoplamento indevido; (b) `strings.Contains(BaseURL, "api.openai.com")` e fragil (falsos positivos em path/subdominio). A nova semantica e: **MCP nativo e opt-in POR PERFIL**; o **default (auto) e o modo adapter** (provider-agnostic, compativel com qualquer modelo, sem `type:"mcp"`); a **unica** dimensao de provider que influencia MCP nativo e a **capacidade fisica de transporte** (`NativeMCPCapable()` — Responses/Anthropic). Assim o `400 unknown variant "mcp", expected "function"` some por default em qualquer endpoint, sem acoplar a provedor; quem quer MCP nativo liga conscientemente no perfil (e so tem efeito em providers capazes). O metodo `SupportsNativeMCP()` foi removido do contrato `ChatProvider`.

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

A unica dimensao de provider que influencia MCP nativo e a **capacidade fisica de transporte** (`NativeMCPCapable()` — o transporte consegue emitir `type:"mcp"`?). **Nao ha heuristica por URL/endpoint.** O **default (auto)** e sempre o **modo adapter**, independentemente do provider; MCP nativo e opt-in por perfil e so tem efeito quando o provider e fisicamente capaz.

| Provider | api_format | API | Capaz (fisico) | Default (auto) | Formato (quando nativo, via opt-in) |
|----------|------------|-----|----------------|----------------|---------|
| OpenAI (real) | `openai_responses` | Responses API (`/v1/responses`) | **Sim** | Adapter | `type: "mcp"` tool com `server_url` |
| OpenAI-compatible via Responses | `openai_responses` | Responses API (proxy: LiteLLM/Azure) | **Sim** | Adapter | `type: "mcp"` |
| Anthropic | `anthropic` | Messages API (`/v1/messages`) | **Sim** | Adapter | `mcp_servers[]` + `mcp_toolset` + beta header |
| Google (Gemini) | `google` | Gemini API | **Nao** | Adapter | Nao implementado |
| OpenAI-compatible | `openai` | Chat Completions (`/v1/chat/completions`) | **Nao** | Adapter | N/A |

> **Nota:** `api_format: "openai"` (Chat Completions) e Google **nao sao fisicamente capazes** de MCP nativo — nenhum override de perfil os habilita (ver `NativeMCPCapable()`). `openai_responses` e `anthropic` sao capazes, mas o **default permanece adapter**: para usar `type:"mcp"`/`mcp_servers` e preciso ligar explicitamente `native_mcp: true` no perfil. Nao ha mais distincao por URL (ex.: `api.openai.com` vs. proxy): a capacidade fisica e a mesma e a decisao e do perfil.

### Limitacoes comuns

- Apenas MCP servers **remotos HTTP** (SSE / Streamable HTTP) podem ser usados nativamente
- Servers **STDIO locais** devem continuar usando adapter (MCPToolBridge)
- Apenas **tool calls** sao suportados nativamente (nao resources/prompts/sampling)

---

## Nova Arquitetura

### Duas dimensoes de decisao

A decisao final de usar MCP nativo combina **capacidade fisica** (do provider) com **politica** (do perfil). Nao ha mais uma terceira camada de "default por endpoint":

1. **Capacidade fisica do provider** — `ChatProvider.NativeMCPCapable() bool`. O transporte consegue emitir `type:"mcp"`? (`openai_responses` e `anthropic` = sim; `openai`/Chat Completions e `google` = nao). Se `false`, **nenhum override habilita** — evita remover bridge tools sem ter como enviar `type:"mcp"`.
2. **Override do perfil** — `Profile.Chat.NativeMCP *bool` (tri-state). E a politica. O default (auto, `nil`) e **adapter** — provider-agnostic, sem heuristica.

A resolucao vive em `internal/chat.ResolveNativeMCPEnabled(streamer, override)`:

```
ResolveNativeMCPEnabled(streamer, override):
  se streamer == nil: false
  se override != nil && *override == true:
     return streamer.NativeMCPCapable()   // forca nativo (so se capaz)
  return false                            // auto (nil) ou false → adapter
```

### Capability tri-state por perfil: `Profile.Chat.NativeMCP`

Campo `*bool` (ponteiro = compativel com perfis antigos), serializado como `native_mcp` no JSON do perfil:

- `nil` / ausente → **auto = adapter** (default seguro, provider-agnostic). NAO liga MCP nativo automaticamente nem usa URL; os MCP servers vao como function/bridge tools, compativel com qualquer modelo. Elimina o `400` por default em qualquer endpoint.
- `true` → **forca nativo** (`type:"mcp"`/`mcp_servers`), desde que o provider seja fisicamente capaz (`NativeMCPCapable()`). Opt-in consciente. Util para OpenAI real ou proxies (LiteLLM/Azure) cujo modelo selecionado aceita `type:"mcp"`.
- `false` → **forca adapter** (MCP como function/bridge tools). Identico ao auto na pratica, mas explicito.

O override vale igualmente para **chat normal e sub-agentes**, pois ambos resolvem o mesmo `activeProfile` no pipeline unico de envio (`SendMessageUseCase`); o sub-agente apenas carrega um `ProfileSlug` diferente. A UI expoe um seletor tri-state (Automatico / Forcar nativo / Forcar adapter) na aba **Ferramentas** do editor de perfis.

### Aplicacao na montagem de tools

`internal/chat.ApplyNativeMCP(...)` e `FilterToolNamesForNativeMCP(...)` recebem o override resolvido do perfil. Quando o resultado e nativo, os MCP servers HTTP elegiveis sao anexados ao provider via `WithMCPServers()` e suas bridge tools sao removidas do `tools[]`; quando e adapter, os servers permanecem como function/bridge tools. `WithMCPServers()` gate apenas pela **capacidade fisica** (a politica ja foi decidida na camada de chat). MCP servers STDIO sempre continuam como bridges.

**Inferencia automatica:** Quando `api_format` nao esta definido, `GetAPIFormat()` infere `openai_responses` se o BaseURL contem `api.openai.com`; qualquer outro URL cai no default conservador `openai` (Chat Completions). **Atencao:** essa heuristica decide apenas qual *API* falar (Responses vs Chat Completions) — NAO tem qualquer relacao com a decisao de MCP nativo, que e puramente capacidade fisica + opt-in de perfil.

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
