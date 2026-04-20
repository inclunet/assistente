# Tool Calling — Revamp & Enhancements

## Status: Proposto

---

## Motivacao

O sistema de tool calling funciona e atende o fluxo basico, mas acumulou inconsistencias e lacunas que limitam observabilidade, resiliencia e evolucao futura:

- **Eventos inconsistentes** entre bridge/built-in e MCP nativo
- **`chat:done` pouco informativo** — frontend nao sabe quantas iteracoes rodaram, quais tools foram usadas, nem se o loop parou normalmente
- **Sem pre-check de context window** antes de enviar tool results ao LLM
- **Falhas parciais silenciosas** no executor paralelo
- **Metadata perdida** na persistencia de MCP nativo

Esta AEP agrupa as melhorias em fases incrementais, cada uma entregavel isoladamente.

### Relacao com AEP-0040 (Backend-Driven Messaging)

A AEP-0040 ja foi implementada e estabeleceu a fundacao do protocolo de eventos:

- Todo evento carrega `conversationId` obrigatoriamente
- Structs tipados em `internal/core/ports/chat_events.go`: `DoneEvent`, `SegmentDoneEvent`, `ToolStartEvent`, `ToolEndEvent`
- Frontend reativo: listeners filtram por `conversationId`
- `chat:done` com `hadToolCalls=true` trigga reload de mensagens

**Esta AEP estende a 0040 — nao a substitui.** As mudancas sao aditivas (novos campos opcionais nos structs existentes) exceto a migracao `native→origin` na Fase 1, que requer deprecacao suave. Nenhuma estrutura da 0040 sera removida ou renomeada.

### Compatibilidade GUI + CLI

O projeto possui dois adapters de saida: Wails (GUI/frontend React) e CLI (`adapters/cli/`). Ambos implementam a mesma interface `ports.Emitter` e recebem os mesmos event structs de `internal/core/ports/chat_events.go`.

**Consequencia:** todas as mudancas nos structs Go de eventos propagam automaticamente para ambos os adapters. Nenhuma fase desta AEP e incompativel com a arquitetura multi-adapter.

**Ponto de atencao:** O CLI adapter (`adapters/cli/emitter.go`) atualmente ignora o conteudo dos eventos de tool — `handleTool()` so imprime o nome do evento. Cada fase que enriquece payloads deve incluir atualizacao do CLI adapter para exibir as informacoes relevantes no terminal (modo verbose).

### UX de tool calls no CLI

O CLI deve exibir progresso de tool calls de forma **compacta e acessivel**, sem poluir o historico do terminal. Design em dois niveis:

#### Modo padrao (sem `--verbose`)

Uma unica linha por iteracao no **stderr**, so quando ha tools:

```
[tools] 3 executadas em 2.1s (search_web, read_file, write_file)
```

Se multiplas iteracoes:

```
[tools] iteracao 1: 2 tools (search_web, read_file) — 1.3s
[tools] iteracao 2: 1 tool (write_file) — 0.8s
```

Ao final, quando houve tool calls, uma linha de resumo no `chat:done`:

```
[done] 2 iteracoes, 5 tool calls, completed
```

#### Modo verbose (`--verbose`)

Cada tool individual, com origin e duracao:

```
[tool:start] search_web (mcp_bridge/perplexity)
[tool:end]   search_web — ok (0.9s)
[tool:start] read_file (builtin)
[tool:end]   read_file — ok (0.2s)
```

Resumo final identico ao modo padrao.

#### Regras de acessibilidade

- **Todo output de tools vai no stderr** — stdout continua limpo so com a resposta do assistente (permite `asst "pergunta" > resposta.md`)
- **Linhas completas, sem `\r` ou overwrites** — leitores de tela leem cada linha na integra
- **Sem spinners ou animacoes** — incompativeis com leitores de tela
- **Formato consistente** — prefixo `[tools]`, `[tool:start]`, `[tool:end]`, `[done]` para facilitar parsing e grepping

---

## Fase 1 — Eventos unificados

### Problema

O agentic loop emite `chat:tool_start` / `chat:tool_end` para todos os tipos de tool, mas com payloads divergentes:

| Aspecto | Bridge / Built-in | MCP Nativo |
|---------|-------------------|------------|
| Quem emite | `service.go` apos `ExecuteAll()` | `agentic_stream_handler.go` via `OnMCPToolEvent()` |
| `native` no payload | Ausente | `true` |
| `serverLabel` no payload | Ausente | Presente |

Dois pontos de emissao com logica duplicada (truncateString, status mapping). O frontend ignora as diferencas — `native` nunca e consumido.

### Estado atual (pos AEP-0040)

Structs em `internal/core/ports/chat_events.go`:

```go
type ToolStartEvent struct {
    ConversationID uint   `json:"conversationId"`
    Name           string `json:"name"`
    CallID         string `json:"callId"`
    Args           string `json:"args,omitempty"`
    ServerLabel    string `json:"serverLabel,omitempty"`
    Native         bool   `json:"native,omitempty"`
}
```

### Proposta

Adicionar campo `Origin` ao struct existente e depreciar `Native`:

```go
type ToolStartEvent struct {
    ConversationID uint   `json:"conversationId"`
    Name           string `json:"name"`
    CallID         string `json:"callId"`
    Args           string `json:"args,omitempty"`
    ServerLabel    string `json:"serverLabel,omitempty"`
    Native         bool   `json:"native,omitempty"`         // DEPRECADO — manter por 1 release
    Origin         string `json:"origin,omitempty"`          // "builtin" | "mcp_bridge" | "mcp_native"
}
```

**Migracao suave (sem breaking change):**
1. Adicionar `Origin` e popular em todos os pontos de emissao
2. Manter `Native` populado em paralelo (compatibilidade)
3. Apos 1 release sem consumidores de `Native`: remover campo

**Helper centralizado:**

```go
// internal/agent/events.go
func emitToolStart(emitter Emitter, opts ToolStartEvent) { ... }
func emitToolEnd(emitter Emitter, opts ToolEndEvent) { ... }
```

`service.go` e `agentic_stream_handler.go` passam a chamar essas funcoes, eliminando duplicacao.

### Arquivos
- Backend: `internal/core/ports/chat_events.go` (estender structs), `service.go`, `agentic_stream_handler.go` + novo `events.go`
- Frontend: tipos de evento no store + componente de exibicao (consumir `origin`, ignorar `native`)
- CLI: `adapters/cli/emitter.go` — implementar dois niveis de exibicao conforme secao "UX de tool calls no CLI": modo padrao (uma linha por iteracao com contagem e nomes) e modo verbose (cada tool individual com origin/serverLabel e duracao). O adapter deve acumular tool events durante a iteracao e emitir a linha resumo ao receber `chat:segment_done` com `hasMore=true`

---

## Fase 2 — Enriquecimento de `chat:done` e `chat:segment_done`

### Problema

`chat:done` emite `{conversationId, assistantMessageId, hadToolCalls}` (AEP-0040). O frontend sabe que houve tool calls mas nao sabe:
- Quantas iteracoes rodaram
- Quais tools foram usadas
- Quantos tokens consumidos
- Se parou normalmente ou atingiu o limite de iteracoes

`chat:segment_done` emite `{content, iteration, hasMore, conversationId}` — sem mencao de quais tools rodaram naquela iteracao nem custo em tokens.

### Estado atual (pos AEP-0040)

```go
type DoneEvent struct {
    ConversationID     uint `json:"conversationId"`
    AssistantMessageID uint `json:"assistantMessageId,omitempty"`
    HadToolCalls       bool `json:"hadToolCalls,omitempty"`
}

type SegmentDoneEvent struct {
    ConversationID uint   `json:"conversationId"`
    Content        string `json:"content,omitempty"`
    Iteration      int    `json:"iteration,omitempty"`
    HasMore        bool   `json:"hasMore"`
}
```

### Proposta

Estender structs existentes com campos opcionais (backward compatible — zero values sao omitidos):

```go
type DoneEvent struct {
    ConversationID     uint     `json:"conversationId"`
    AssistantMessageID uint     `json:"assistantMessageId,omitempty"`
    HadToolCalls       bool     `json:"hadToolCalls,omitempty"`
    // Novos campos (AEP-0039)
    Reason         string   `json:"reason,omitempty"`         // "completed" | "limit_reached" | "error"
    IterationCount int      `json:"iterationCount,omitempty"`
    ToolCallCount  int      `json:"toolCallCount,omitempty"`
    ToolsUsed      []string `json:"toolsUsed,omitempty"`
    PromptTokens   int      `json:"promptTokens,omitempty"`
    CompletionTokens int    `json:"completionTokens,omitempty"`
    ErrorMessage   string   `json:"errorMessage,omitempty"`
}

type SegmentDoneEvent struct {
    ConversationID uint   `json:"conversationId"`
    Content        string `json:"content,omitempty"`
    Iteration      int    `json:"iteration,omitempty"`
    HasMore        bool   `json:"hasMore"`
    // Novos campos (AEP-0039)
    ToolsInIteration []ToolSummary `json:"toolsInIteration,omitempty"`
}

type ToolSummary struct {
    Name   string `json:"name"`
    Status string `json:"status"` // "ok" | "error"
}
```

O backend ja possui todos os dados necessarios no momento da emissao: `iteration` counter, `result.ToolCalls[]`, token stats callback.

### Arquivos
- Backend: `internal/core/ports/chat_events.go` (estender structs), `service.go` (acumular stats durante loop, popular novos campos na emissao)
- Frontend: store + UI de resumo (opcional) — campos novos sao opcionais, frontend existente continua funcionando
- CLI: `adapters/cli/emitter.go` — adicionar handler para `chat:done` que exiba a linha de resumo final no stderr (`[done] 2 iteracoes, 5 tool calls, completed`). Hoje o CLI so captura `chat:stream` com Done=true para sinalizar fim — precisa tambem processar `chat:done` enriquecido para exibir reason, contagem de iteracoes e tool calls

---

## Fase 3 — Resiliencia do executor

### Problema

`ExecuteAll()` executa tools em paralelo. Se 1 de 5 da timeout:
- Resultado de erro vai pro LLM como texto plano (`"ERROR: timeout"`)
- Sem distincao entre timeout (retryable) vs. JSON invalido (fatal)
- Sem circuit breaker: tool que falha repetidamente continua sendo chamada
- Truncamento de resultado > 100KB nao e UTF-8 safe

### Proposta

**Classificacao estruturada de erros:**

```go
type ToolExecutionResult struct {
    Content    string
    IsError    bool
    ErrorKind  string // "timeout" | "invalid_args" | "not_found" | "panic" | "unknown"
    Retryable  bool
    DurationMs int64
}
```

**Melhorias incrementais:**
1. Truncamento UTF-8 safe nos resultados
2. Campo `ErrorKind` + `Retryable` no resultado (backend consome para decidir retry)
3. Evento `chat:tool_failure` estruturado para o frontend (distinto de `chat:tool_end` com status error — este carrega contexto de retry)
4. Retry opcional: se `Retryable=true` e iteracoes restam, tentar 1x antes de propagar

### Arquivos
- Backend: `executor.go`, `service.go`
- CLI: sem impacto direto — resiliencia e interna ao executor. Se `chat:tool_failure` for criado como evento novo, adicionar handler no CLI para exibir aviso

---

## Fase 4 — Pre-check de context window

### Problema

Resultados de tools sao adicionados ao historico de mensagens sem verificar se cabem na janela de contexto do LLM. Se a soma de resultados exceder o limite, o erro so aparece na proxima chamada ao LLM — tarde demais.

Exemplo: `search_web` retorna 80KB, `read_file` retorna 50KB → 130KB adicionados ao historico que ja tem 90KB de conversa → estouro silencioso.

### Proposta

Apos `ExecuteAll()`, antes de adicionar ao historico:

```go
estimatedTokens := countTokens(existingMessages) + countTokens(toolResults)
if estimatedTokens > contextLimit * 0.9 {
    toolResults = truncateResults(toolResults, availableTokens)
    // ou: remover mensagens antigas do historico
}
```

Separar `MaxResultDisplaySize` (200 bytes para UI) de `MaxResultContextSize` (limite para historico LLM).

### Arquivos
- Backend: `service.go` + possivel novo `internal/agent/context.go`
- CLI: sem impacto — pre-check e interno ao loop, transparente para adapters

---

## Fase 5 — Persistencia enriquecida de metadata

### Problema

- MCP nativo: `serverLabel` emitido via evento mas perdido na persistencia (DB). No replay de conversa, nao da pra saber de qual server veio a tool.
- Argumentos da tool visiveis durante streaming (`activeToolCalls[].args`) mas ausentes no historico salvo.
- Sem registro de duracao de execucao nem iteracao de origem.

### Proposta

Enriquecer `AddToolResultMessage` / tool_calls JSON salvo no DB:

```json
{
  "name": "search_web",
  "call_id": "call_abc",
  "arguments": "{\"query\": \"...\"}",
  "origin": "mcp_native",
  "server_label": "perplexity",
  "iteration": 2,
  "duration_ms": 1500
}
```

Frontend pode usar essa metadata para renderizar badges de origem e mostrar args no historico.

### Arquivos
- Backend: `service.go` (persistencia), possivelmente `internal/core/ports/` (interface do repo)
- Frontend: `ToolCallsSection.tsx`, `MessageList.tsx`
- CLI: sem impacto direto — persistencia e interna ao backend. Eventual exibicao de historico enriquecido viria de comandos futuros (fora do escopo desta AEP)

---

## Compatibilidade multi-adapter por fase

| Fase | Propaga automatico | CLI precisa atualizar |
|------|-------------------|----------------------|
| 1 — Eventos unificados | Sim (structs Go) | Sim — dois niveis de detalhe (padrao/verbose) conforme UX definida |
| 2 — chat:done/segment | Sim (structs Go) | Sim — handler para `chat:done` com linha resumo final |
| 3 — Resiliencia executor | Sim (interno) | Opcional — handler para `chat:tool_failure` |
| 4 — Pre-check context | Sim (interno) | Nao |
| 5 — Persistencia | Sim (interno) | Nao |

Nenhuma fase e incompativel com a arquitetura multi-adapter. Fases 1 e 2 exigem atualizacao do CLI adapter para aproveitar os dados enriquecidos; fases 3-5 sao transparentes.

---

## Dependencias e conflitos com outras AEPs

### AEP-0040 (Backend-Driven Messaging) — IMPLEMENTADA (fases 1-2)

Base sobre a qual esta AEP constroi. Estabeleceu:
- `conversationId` obrigatorio em todos os eventos
- Structs tipados em `internal/core/ports/chat_events.go`
- Frontend reativo com listeners filtrados por conversa
- `chat:done` com `hadToolCalls` + reload de mensagens

**Conflito: nenhum.** Esta AEP estende os structs existentes com campos opcionais. Nenhuma remocao ou renomeacao de campos da 0040.

### AEP-0021 (MCP Native Mode) — IMPLEMENTADA (v3)

Introduziu `native: bool` nos eventos de tool. Esta AEP deprecia `native` em favor de `origin: string` (fase 1).

**Risco real: baixo.** O campo `native` e setado pelo backend para MCP nativo (`true`) mas nunca para bridge/built-in (default `false`). O frontend declara o campo no tipo TypeScript mas **nunca o consome** em nenhuma logica. A migracao pode ser feita com deprecacao suave (emitir ambos por 1 release) ou direta (substituir, ja que nenhum consumidor depende do campo).

### AEP-0037 (SDK Migration) — IMPLEMENTADA (v2)

Concluida. Sem risco de merge — os arquivos `service.go` e `runAgenticLoop()` ja estao na versao final pos-migracao, que esta AEP modifica.

### AEP-0045 (CLI Interface) — IMPLEMENTADA

Define a arquitetura do CLI adapter. Esta AEP estende o CLI com exibicao de tool calls e resumo `chat:done`.

**Conflito: nenhum.** 0045 e a fundacao; 0039 e consumidora.

### Demais AEPs

Nenhum conflito identificado. AEPs 0033 (MCP OAuth), 0034 (workspace), 0035 (split view), 0038 (voice), 0041 (TTS proativo), 0042-0044 nao tocam no agentic loop nem nos eventos de tool calling.

---

## Ordem de execucao sugerida

| Fase | Risco | Impacto | Dependencias |
|------|-------|---------|-------------|
| 1 — Eventos unificados | Baixo | Medio | AEP-0040 e 0037 implementadas (ja OK) |
| 2 — chat:done/segment enriquecido | Baixo | Alto | AEP-0040 implementada (ja OK) |
| 3 — Resiliencia do executor | Medio | Alto | Nenhuma |
| 4 — Pre-check context window | Medio | Alto | Nenhuma |
| 5 — Persistencia enriquecida | Baixo | Medio | Fase 1 (campo `origin`) |

Fases 1-4 sao independentes entre si e podem ser feitas em qualquer ordem (respeitadas as pre-condicoes). Fase 5 aproveita o campo `origin` introduzido na fase 1.

**Sequencia recomendada:** Fase 2 primeiro (maior impacto, menor risco, sem pre-condicoes alem de 0040), depois fase 1, depois 3-5.
