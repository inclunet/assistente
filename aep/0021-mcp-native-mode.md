# MCP Modo Nativo

## Status: In Progress — revisão v7

> **Historico:** A versao original desta AEP descrevia um conceito aspiracional de modo nativo com `mcp_mode` (adapter/native/auto) no perfil. A infraestrutura foi parcialmente criada (`GetNativeServerInfo`, `TestMCPNativeSupport`, `ShouldUseMCPNative`) mas **nunca foi consumida no chat loop**. O sistema sempre operou em modo adapter.
>
> A v2 refletia a arquitetura [AEP-0037](0037-sdk-migration-chat-provider.md), mas descrevia `api_format: "openai"` como suportando MCP nativo, o que era incorreto.
>
> A v3 refletiu a separacao real entre `openai` (Chat Completions only) e `openai_responses` (Responses API first), e tratou o suporte como puramente capability-driven, sem participacao do perfil.
>
> A v4 tornou o suporte **configuravel POR PERFIL** (`Profile.Chat.NativeMCP`, tri-state), mas ainda usava uma **heuristica por endpoint** (`SupportsNativeMCP()` baseado em `api.openai.com`) como default do modo auto.
>
> A v5 eliminou qualquer heuristica por URL/endpoint (acoplamento a `api.openai.com` + fragilidade do `strings.Contains`) e removeu `SupportsNativeMCP()` do contrato. Porem fez o **default (auto) = adapter**, o que "nivela todo mundo por baixo": assume que nenhum endpoint suporta `type:"mcp"`, desligando MCP nativo ate para quem suporta.
>
> A v6 tornou o **default (auto) OTIMISTA com auto-degradacao e memoria persistida no perfil**, mas o fallback do mesmo turno dropava os MCP servers e re-tentava "pelado" — as tools MCP sumiam naquele turno e so voltavam (via adapter) no proximo.
>
> **Esta v7** elimina essa limitacao: no fallback, o turno e **re-tentado em modo ADAPTER com as bridge tools presentes** (MCP como function/bridge), preservando as tools ja naquele turno. Tudo o mais da v6 permanece: default auto otimista; auto-ajuste **persistido** do perfil (`Profile.Chat.NativeMCP` nil→false) como memoria (nao cache em runtime); sem heuristica por URL; unica dimensao estatica de provider = **capacidade fisica** (`NativeMCPCapable()` — Responses/Anthropic); override explicito (`true`/`false`) nunca sobrescrito.

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

A unica dimensao ESTATICA de provider que influencia MCP nativo e a **capacidade fisica de transporte** (`NativeMCPCapable()` — o transporte consegue emitir `type:"mcp"`?). **Nao ha heuristica por URL/endpoint.** O **default (auto)** e **otimista**: tenta nativo quando o provider e capaz; a dimensao DINAMICA "este modelo/endpoint nao suporta" e descoberta em runtime (erro) e memorizada como `native_mcp=false` no perfil.

| Provider | api_format | API | Capaz (fisico) | Default (auto) | Formato (quando nativo) |
|----------|------------|-----|----------------|----------------|---------|
| OpenAI (real) | `openai_responses` | Responses API (`/v1/responses`) | **Sim** | Nativo (otimista) | `type: "mcp"` tool com `server_url` |
| OpenAI-compatible via Responses | `openai_responses` | Responses API (proxy: LiteLLM/Azure) | **Sim** | Nativo (otimista) | `type: "mcp"` |
| Anthropic | `anthropic` | Messages API (`/v1/messages`) | **Sim** | Nativo (otimista) | `mcp_servers[]` + `mcp_toolset` + beta header |
| Google (Gemini) | `google` | Gemini API | **Nao** | Adapter | Nao implementado |
| OpenAI-compatible | `openai` | Chat Completions (`/v1/chat/completions`) | **Nao** | Adapter | N/A |

> **Nota:** `api_format: "openai"` (Chat Completions) e Google **nao sao fisicamente capazes** de MCP nativo — nenhum override de perfil os habilita (ver `NativeMCPCapable()`). `openai_responses` e `anthropic` sao capazes, entao o **default (auto) tenta nativo**. Se o modelo por tras do endpoint rejeitar `type:"mcp"` (comum em proxies tipo LiteLLM roteando para deepseek etc.), o turno degrada para adapter e o perfil e auto-ajustado para `false` — os proximos turnos ja usam adapter direto, sem repetir o 400. Nao ha distincao por URL (ex.: `api.openai.com` vs. proxy): a capacidade fisica e a mesma; a (in)compatibilidade do modelo e aprendida em runtime, nao chutada pela URL.

### Limitacoes comuns

- Apenas MCP servers **remotos HTTP** (SSE / Streamable HTTP) podem ser usados nativamente
- Servers **STDIO locais** devem continuar usando adapter (MCPToolBridge)
- Apenas **tool calls** sao suportados nativamente (nao resources/prompts/sampling)

---

## Nova Arquitetura

### Duas dimensoes de decisao

A decisao de usar MCP nativo combina **capacidade fisica** (do provider) com **politica** (do perfil):

1. **Capacidade fisica do provider** — `ChatProvider.NativeMCPCapable() bool`. O transporte consegue emitir `type:"mcp"`? (`openai_responses` e `anthropic` = sim; `openai`/Chat Completions e `google` = nao). Se `false`, **nenhum override habilita** — evita remover bridge tools sem ter como enviar `type:"mcp"`.
2. **Override do perfil** — `Profile.Chat.NativeMCP *bool` (tri-state). E a politica. O default (auto, `nil`) e **otimista**: tenta nativo se o provider for capaz.

A resolucao vive em `internal/chat.ResolveNativeMCPEnabled(streamer, override)`:

```
ResolveNativeMCPEnabled(streamer, override):
  se streamer == nil: false
  se override != nil && *override == false:
     return false                          // forca adapter
  return streamer.NativeMCPCapable()       // auto (nil) ou true → nativo se capaz
```

### Capability tri-state por perfil: `Profile.Chat.NativeMCP`

Campo `*bool` (ponteiro = compativel com perfis antigos), serializado como `native_mcp` no JSON do perfil:

- `nil` / ausente → **auto OTIMISTA**: tenta nativo se o provider for capaz. Se o modelo/endpoint rejeitar `type:"mcp"`, o turno degrada para adapter e o perfil e **auto-ajustado para `false` e persistido** (ver fluxo abaixo).
- `true` → **forca nativo** (`type:"mcp"`/`mcp_servers`), desde que o provider seja fisicamente capaz (`NativeMCPCapable()`). Escolha explicita: **nunca** e auto-sobrescrita. Se o modelo rejeitar, degrada so naquele turno e loga aviso.
- `false` → **forca adapter** (MCP como function/bridge tools). E o estado para o qual o auto-ajuste converge.

O override vale igualmente para **chat normal e sub-agentes**, pois ambos resolvem o mesmo `activeProfile` no pipeline unico de envio (`SendMessageUseCase`); o sub-agente apenas carrega um `ProfileSlug` diferente — o auto-ajuste recai sobre o profile efetivamente usado no run. A UI expoe um seletor tri-state (Automatico / Forcar nativo / Forcar adapter) na aba **Ferramentas** do editor de perfis.

### Auto-degradacao + memoria persistida no perfil

Quando o modo resolvido e nativo e a request falha com o erro caracteristico de nao-suporte a `type:"mcp"` (classificado por `looksLikeNativeMCPUnsupported` em `mcp_degradation.go` — ex.: 400 `unknown variant "mcp", expected "function"`), o pipeline reage assim:

1. **Sinalizacao (provider layer):** o loop de streaming (`streamChatResponses` / `doStreamBeta`) detecta o erro (antes de emitir qualquer chunk), dispara o hook de persistencia (item 3) e:
   - se houver `ChatParams.NativeMCPFallback` (caller capaz de re-tentar em adapter), chama `NativeMCPFallback.Trigger()` e **aborta sem emitir** done/erro — deixando o caller re-montar as tools em adapter;
   - senao (fallback de seguranca, ex.: caminho simples sem tools), **dropa os MCP servers** e re-tenta "pelado".
2. **Retry no MESMO turno COM bridge tools (agent layer):** o `SendMessageUseCase` captura, ANTES de `ApplyNativeMCP`, as alternativas em adapter — o provider BASE (sem `WithMCPServers`) e os tool defs COM as bridges — e as injeta em `ChatParams.NativeMCPFallback` (incluindo um `ResolveToolDefs` adapter para as iteracoes seguintes). O `RunAgenticLoop`, ao ver `NativeMCPFallback.Consume()` apos a chamada abortada, **troca streamer + toolDefs + resolveToolDefs para o modo adapter e re-tenta a iteracao** (sem consumir uma tentativa de recovery; `Consume()` garante troca unica). Resultado: o turno conclui SEM 400 **e** com as tools MCP disponiveis ao modelo via bridge/function — ja neste turno. A troca vale para as iteracoes seguintes do mesmo turno.
3. **Memoria = auto-ajuste persistido do perfil (use case layer):** o provider dispara o hook opcional `ChatParams.OnNativeMCPUnsupported` (sem conhecer a camada de perfis — separacao de camadas). O `SendMessageUseCase` liga esse hook a `chat.Interactor.HandleNativeMCPUnsupported(profileSlug, model, override)`, que:
   - **override == nil (auto):** rele o perfil do disco pelo slug e, **somente na transicao `nil`→`false`**, grava `NativeMCP=false` via `profiles.Manager.Update` e loga `[MCP] perfil X (modelo Y) ajustado para adapter automaticamente...`. Idempotente e **thread-safe** (mutex serializa o read-modify-write; runs concorrentes do mesmo perfil nao gravam em corrida — o segundo encontra o disco ja em `false` e nao regrava).
   - **override == true (forcar nativo):** NAO persiste; apenas loga `[MCP] modelo Y do perfil X nao suporta MCP nativo; usando adapter neste turno (perfil em 'forcar nativo')`.
   - **override == false:** nada a fazer.
4. **Proximos turnos:** com o perfil em `false`, `ResolveNativeMCPEnabled` resolve adapter naturalmente — os MCP servers ja entram como function/bridge tools desde o inicio e o 400 nem chega a ocorrer (era exatamente a poluicao de log que motivou esta AEP).

> **Persistencia vs cache:** a decisao deliberada e usar o **campo persistido do perfil** como memoria, e nao um cache em memoria por endpoint+modelo. O perfil ja amarra o modelo, sobrevive a restart e e auditavel/editavel pelo usuario. (Um cache em runtime foi considerado e descartado.)

### Aplicacao na montagem de tools

`internal/chat.ApplyNativeMCP(...)` e a `ToolSelectionPolicy` recebem o override
resolvido do perfil. A lista bruta do manager ainda não é a decisão final do
turno: a camada de chat cruza cada servidor candidato com as tools
**preloaded** pela política efetiva. Somente servidores com ao menos uma tool
nesse cruzamento são anexados ao provider via `WithMCPServers()`, com
`AllowedTools` restrito ao subconjunto preloaded, e as bridges correspondentes
são removidas de `tools[]`. Tools MCP carregadas sob demanda continuam como
function/bridge; servidores STDIO sempre continuam como bridges.

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

## Elegibilidade bruta do manager e decisão final por turno

Apesar do nome, `Manager.GetEligibleNativeMCPServers()` produz apenas
**candidatos de servidor**. No código vigente, essa etapa verifica:

1. servidor conectado;
2. transporte HTTP (SSE ou Streamable HTTP), excluindo STDIO;
3. URL presente e elegível pela regra abaixo;
4. ao menos uma tool disponível no servidor;
5. `prefer_bridge=false`.

Ela não conhece a capacidade do provider, o tri-state do perfil nem quais
tools estão preloaded naquele turno. A elegibilidade final é composta na camada
de chat:

1. `ChatProvider.NativeMCPCapable()` deve ser verdadeiro;
2. `Profile.Chat.NativeMCP` não pode resolver para adapter (`false`);
3. o candidato do manager deve ter `prefer_bridge=false`;
4. ao menos uma tool do servidor deve pertencer ao conjunto preloaded da
   política efetiva do perfil.

Se o servidor só tiver tools `on_demand`, ele não entra nativo inicialmente.
Quando essas tools forem carregadas durante o turno, permanecem
function/bridge; o carregamento não remonta retroativamente o provider como
MCP nativo. Esse cruzamento é coberto por
`internal/chat/tool_selection_policy_test.go`.

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
