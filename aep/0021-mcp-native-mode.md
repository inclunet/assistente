# MCP Modo Nativo

## Status: Revisado (v4)

> **Historico:** A versao original desta AEP descrevia um conceito aspiracional de modo nativo com `mcp_mode` (adapter/native/auto) no perfil. A infraestrutura foi parcialmente criada (`GetNativeServerInfo`, `TestMCPNativeSupport`, `ShouldUseMCPNative`) mas **nunca foi consumida no chat loop**. O sistema sempre operou em modo adapter.
>
> A v2 refletia a arquitetura [AEP-0037](0037-sdk-migration-chat-provider.md), mas descrevia `api_format: "openai"` como suportando MCP nativo, o que era incorreto.
>
> A v3 refletiu a separacao real entre `openai` (Chat Completions only) e `openai_responses` (Responses API first), e tratou o suporte como puramente capability-driven, sem participacao do perfil.
>
> **Esta v4** corrige a decisao: o suporte a MCP nativo passa a ser **configuravel POR PERFIL** (`Profile.Chat.NativeMCP`, tri-state), com a heuristica por endpoint servindo apenas como **default (auto)**. Motivo: um mesmo endpoint OpenAI-compatible que fala a Responses API (ex.: proxy LiteLLM) pode rotear para varios modelos — alguns que aceitam `type:"mcp"` e outros que NAO (ex.: `deepseek-v4-flash`, que devolvia `400 unknown variant "mcp", expected "function"` a cada turno). Gatear apenas por provider/URL e grosseiro demais; o perfil ja amarra modelo + comportamento, entao e o lugar correto para essa capability.

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

A tabela abaixo lista a **capacidade fisica** (`NativeMCPCapable()` — o transporte consegue emitir `type:"mcp"`?) e o **default (auto)** quando o perfil nao forca nada (`SupportsNativeMCP()`).

| Provider | api_format | API | Capaz (fisico) | Default (auto) | Formato |
|----------|------------|-----|----------------|----------------|---------|
| OpenAI (real) | `openai_responses` | Responses API (`/v1/responses`) | **Sim** | **Nativo** (URL `api.openai.com`) | `type: "mcp"` tool com `server_url` |
| OpenAI-compatible via Responses | `openai_responses` | Responses API (proxy: LiteLLM/Azure) | **Sim** | **Adapter** (URL nao-real) | `type: "mcp"` (se forcado por perfil) |
| Anthropic | `anthropic` | Messages API (`/v1/messages`) | **Sim** | **Nativo** | `mcp_servers[]` + `mcp_toolset` + beta header |
| Google (Gemini) | `google` | Gemini API | **Nao** | Adapter | Nao implementado |
| OpenAI-compatible | `openai` | Chat Completions (`/v1/chat/completions`) | **Nao** | Adapter | N/A |

> **Nota:** `api_format: "openai"` (Chat Completions) e Google **nao sao fisicamente capazes** de MCP nativo — nenhum override de perfil os habilita (ver `NativeMCPCapable()`). Ja `openai_responses` e capaz, mas o **default** depende da URL: apenas a OpenAI real (`api.openai.com`) usa `type:"mcp"` por padrao; proxies caem em adapter por seguranca (podem rotear para modelos que rejeitam `type:"mcp"`). O perfil pode sobrescrever esse default (ver abaixo).

### Limitacoes comuns

- Apenas MCP servers **remotos HTTP** (SSE / Streamable HTTP) podem ser usados nativamente
- Servers **STDIO locais** devem continuar usando adapter (MCPToolBridge)
- Apenas **tool calls** sao suportados nativamente (nao resources/prompts/sampling)

---

## Nova Arquitetura

### Tres camadas de decisao

A decisao final de usar MCP nativo combina tres camadas, da mais forte para a mais fraca:

1. **Capacidade fisica do provider** — `ChatProvider.NativeMCPCapable() bool`. O transporte consegue emitir `type:"mcp"`? (`openai_responses` e `anthropic` = sim; `openai`/Chat Completions e `google` = nao). Se `false`, **nenhum override habilita** — evita remover bridge tools sem ter como enviar `type:"mcp"`.
2. **Override do perfil** — `Profile.Chat.NativeMCP *bool` (tri-state). Quando setado, manda (dentro da capacidade fisica).
3. **Default (auto) do provider** — `ChatProvider.SupportsNativeMCP() bool`, a heuristica segura por endpoint (OpenAI real por `api.openai.com`). Usado quando o perfil nao diz nada (`nil`).

A resolucao vive em `internal/chat.ResolveNativeMCPEnabled(streamer, override)`:

```
ResolveNativeMCPEnabled(streamer, override):
  se streamer == nil: false
  se override != nil:
     se *override == true:  return streamer.NativeMCPCapable()   // forca nativo (se capaz)
     senao:                 return false                          // forca adapter
  return streamer.SupportsNativeMCP()                             // auto: default por endpoint
```

### Capability tri-state por perfil: `Profile.Chat.NativeMCP`

Campo `*bool` (ponteiro = compativel com perfis antigos), serializado como `native_mcp` no JSON do perfil:

- `nil` / ausente → **auto**: usa o default do endpoint (heuristica por URL).
- `true` → **forca nativo** (`type:"mcp"`), desde que o provider seja fisicamente capaz. Util para proxies (LiteLLM/Azure) cujo modelo selecionado aceita `type:"mcp"`.
- `false` → **forca adapter** (MCP como function/bridge tools). Util quando o endpoint serve um modelo que NAO aceita `type:"mcp"` (ex.: `deepseek-v4-flash` via LiteLLM), eliminando o `400` recorrente.

O override vale igualmente para **chat normal e sub-agentes**, pois ambos resolvem o mesmo `activeProfile` no pipeline unico de envio (`SendMessageUseCase`); o sub-agente apenas carrega um `ProfileSlug` diferente. A UI expoe um seletor tri-state (Automatico / Forcar nativo / Forcar adapter) na aba **Ferramentas** do editor de perfis.

### Aplicacao na montagem de tools

`internal/chat.ApplyNativeMCP(...)` e `FilterToolNamesForNativeMCP(...)` recebem o override resolvido do perfil. Quando o resultado e nativo, os MCP servers HTTP elegiveis sao anexados ao provider via `WithMCPServers()` e suas bridge tools sao removidas do `tools[]`; quando e adapter, os servers permanecem como function/bridge tools. `WithMCPServers()` gate apenas pela **capacidade fisica** (a politica ja foi decidida na camada de chat). MCP servers STDIO sempre continuam como bridges.

**Inferencia automatica:** Quando `api_format` nao esta definido, `GetAPIFormat()` infere `openai_responses` se o BaseURL contem `api.openai.com`. Qualquer outro URL cai no default conservador `openai` (Chat Completions).

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
