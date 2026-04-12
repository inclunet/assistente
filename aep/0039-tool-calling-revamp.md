# Tool Calling — Revamp & Enhancements

## Status: Proposto

---

## Motivacao

O sistema de tool calling funciona e atende o fluxo basico, mas acumulou inconsistencias e lacunas que limitam observabilidade, resiliencia e evolucao futura:

- **Eventos inconsistentes** entre bridge/built-in e MCP nativo
- **`chat:done` vazio** — frontend nao sabe o que aconteceu no loop
- **Sem pre-check de context window** antes de enviar tool results ao LLM
- **Falhas parciais silenciosas** no executor paralelo
- **Metadata perdida** na persistencia de MCP nativo

Esta AEP agrupa as melhorias em fases incrementais, cada uma entregavel isoladamente.

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

### Proposta

**Payload padrao para todos os eventos:**

```typescript
// chat:tool_start
{
  name: string
  callId: string
  args: string
  conversationId: number
  origin: "builtin" | "mcp_bridge" | "mcp_native"
  serverLabel?: string
}

// chat:tool_end
{
  name: string
  callId: string
  status: "ok" | "error"
  summary: string
  error?: string
  conversationId: number
  origin: "builtin" | "mcp_bridge" | "mcp_native"
  serverLabel?: string
}
```

**Helper centralizado:**

```go
// internal/agent/events.go
func emitToolStart(emitter Emitter, opts ToolStartEvent) { ... }
func emitToolEnd(emitter Emitter, opts ToolEndEvent) { ... }
```

`service.go` e `agentic_stream_handler.go` chamariam essas funcoes. Campo `native: bool` substituido por `origin` (mais expressivo).

### Arquivos
- Backend: `service.go`, `agentic_stream_handler.go` + novo `events.go`
- Frontend: tipos de evento no store + componente de exibicao

---

## Fase 2 — Enriquecimento de `chat:done` e `chat:segment_done`

### Problema

`chat:done` emite apenas `{conversationId}`. O frontend nao sabe:
- Quantas iteracoes rodaram
- Quais tools foram usadas
- Quantos tokens consumidos
- Se parou normalmente ou atingiu o limite de iteracoes

`chat:segment_done` emite `{content, iteration, hasMore, conversationId}` — sem mencao de quais tools rodaram naquela iteracao nem custo em tokens.

### Proposta

```typescript
// chat:done (enriquecido)
{
  conversationId: number
  reason: "completed" | "limit_reached" | "error"
  iterationCount: number
  toolCallCount: number
  toolsUsed: string[]
  tokens?: { prompt: number, completion: number }
  errorMessage?: string
}

// chat:segment_done (enriquecido)
{
  content: string
  iteration: number
  hasMore: boolean
  conversationId: number
  toolsInIteration: Array<{ name: string, status: string }>
  tokenDelta?: number
}
```

### Arquivos
- Backend: `service.go` (acumular stats durante loop, emitir no final)
- Frontend: store + UI de resumo (opcional)

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

---

## Ordem de execucao sugerida

| Fase | Risco | Impacto | Dependencias |
|------|-------|---------|-------------|
| 1 — Eventos unificados | Baixo | Medio | Nenhuma |
| 2 — chat:done/segment enriquecido | Baixo | Alto | Nenhuma |
| 3 — Resiliencia do executor | Medio | Alto | Nenhuma |
| 4 — Pre-check context window | Medio | Alto | Nenhuma |
| 5 — Persistencia enriquecida | Baixo | Medio | Fase 1 (campo `origin`) |

Fases 1-4 sao independentes entre si e podem ser feitas em qualquer ordem. Fase 5 aproveita o campo `origin` introduzido na fase 1.
