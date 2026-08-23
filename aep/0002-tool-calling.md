# Plano de Implementação — Tool Calling

**Status:** Done

## Visão Geral

Implementar um sistema de tool calling que permite ao assistente executar ferramentas (ler arquivos, buscar dados, acessar web) em loop, similar ao Cursor, Claude Code, Codex e Copilot.

### Requisitos

1. **Execução paralela** — LLM pode pedir N tools de uma vez, executamos todas em paralelo com goroutines
2. **Iterações sequenciais (agentic loop)** — LLM chama tools → recebe resultados → pode chamar mais tools → repete até se satisfazer
3. **Verbalização intermediária** — Cada segmento de texto do assistente é verbalizado imediatamente (TTS ou aria-live), sem esperar o turno terminar
4. **Agrupamento visual** — Todas as mensagens de um turno são renderizadas como um único bloco no frontend

---

## Modelo de Dados

### Campos novos no ChatMessage

| Campo | Tipo | Descrição |
|---|---|---|
| TurnID | *uint (nullable, indexed) | Agrupa mensagens de um mesmo turno. Aponta para o ID da mensagem `user` que iniciou o turno |
| ToolCalls | string (nullable) | JSON com as tools solicitadas pelo assistant: `[{"id":"call_a","name":"read_file","arguments":"{...}"}]` |
| ToolCallID | string (nullable) | Para role="tool": ID que vincula este resultado à chamada correspondente no ToolCalls do assistant |

### Regras de preenchimento

| Role | TurnID | ToolCalls | ToolCallID |
|---|---|---|---|
| user | null | null | null |
| system | null | null | null |
| assistant (pedindo tools) | ID do user | JSON [...] | null |
| tool (resultado) | ID do user | null | "call_xxx" |
| assistant (resposta final) | ID do user | null | null |

### Exemplo de interação completa

Usuário pergunta: "analisa o projeto"

#### Iteração 1 — Assistente fala e pede 2 tools

**Mensagem 1 — Usuário**

| Campo | Valor |
|---|---|
| ID | 1 |
| Role | user |
| Content | analisa o projeto |
| TurnID | null |
| ToolCalls | null |
| ToolCallID | null |

**Mensagem 2 — Assistente pede ferramentas**

| Campo | Valor |
|---|---|
| ID | 2 |
| Role | assistant |
| Content | Deixa eu verificar a estrutura do projeto. |
| TurnID | 1 |
| ToolCalls | [{"id":"call_a","name":"read_file","arguments":"{\"path\":\"main.go\"}"},{"id":"call_b","name":"read_file","arguments":"{\"path\":\"go.mod\"}"}] |
| ToolCallID | null |

> Verbalização: TTS/aria-live "Deixa eu verificar a estrutura do projeto."
> Verbalização: aria-live "Executando read_file..."

**Mensagem 3 — Resultado read_file(main.go)**

| Campo | Valor |
|---|---|
| ID | 3 |
| Role | tool |
| Content | package main\nimport "fmt"\nfunc main() {...} |
| TurnID | 1 |
| ToolCalls | null |
| ToolCallID | call_a |

**Mensagem 4 — Resultado read_file(go.mod)**

| Campo | Valor |
|---|---|
| ID | 4 |
| Role | tool |
| Content | module assistente\ngo 1.24.1\nrequire (...) |
| TurnID | 1 |
| ToolCalls | null |
| ToolCallID | call_b |

> Verbalização: aria-live "2 ferramentas concluídas"

#### Iteração 2 — Assistente quer mais detalhe

**Mensagem 5 — Assistente pede mais uma tool**

| Campo | Valor |
|---|---|
| ID | 5 |
| Role | assistant |
| Content | Achei alguns TODOs, deixa eu verificar melhor. |
| TurnID | 1 |
| ToolCalls | [{"id":"call_c","name":"grep_search","arguments":"{\"pattern\":\"TODO\"}"}] |
| ToolCallID | null |

> Verbalização: TTS/aria-live "Achei alguns TODOs, deixa eu verificar melhor."
> Verbalização: aria-live "Executando grep_search..."

**Mensagem 6 — Resultado grep_search**

| Campo | Valor |
|---|---|
| ID | 6 |
| Role | tool |
| Content | main.go:42: // TODO fix this\napp.go:15: // TODO refactor |
| TurnID | 1 |
| ToolCalls | null |
| ToolCallID | call_c |

> Verbalização: aria-live "grep_search concluído"

#### Iteração 3 — Resposta final

**Mensagem 7 — Resposta final (sem tools)**

| Campo | Valor |
|---|---|
| ID | 7 |
| Role | assistant |
| Content | O projeto tem 2 TODOs pendentes:\n1. main.go:42 — fix this\n2. app.go:15 — refactor |
| TurnID | 1 |
| ToolCalls | null |
| ToolCallID | null |

> Verbalização: TTS/aria-live "O projeto tem 2 TODOs pendentes..."

### O que o LLM recebe a cada iteração

**Iteração 1:**
```
[system]  "Você é um assistente..."
[user]    "analisa o projeto"
```

**Iteração 2 (com resultados da iteração 1):**
```
[system]     "Você é um assistente..."
[user]       "analisa o projeto"
[assistant]  "Deixa eu verificar..." + tool_calls: [call_a, call_b]
[tool]       resultado de main.go (tool_call_id: call_a)
[tool]       resultado de go.mod (tool_call_id: call_b)
```

**Iteração 3 (com tudo acumulado):**
```
[system]     "Você é um assistente..."
[user]       "analisa o projeto"
[assistant]  "Deixa eu verificar..." + tool_calls: [call_a, call_b]
[tool]       resultado de main.go (tool_call_id: call_a)
[tool]       resultado de go.mod (tool_call_id: call_b)
[assistant]  "Achei alguns TODOs..." + tool_calls: [call_c]
[tool]       resultado grep (tool_call_id: call_c)
```

O LLM vê esse histórico e decide: "Já tenho tudo" → responde com finish_reason: "stop".

---

## Ferramentas Planejadas

| # | Ferramenta | Descrição | Usa Resolver? | Fase |
|---|-----------|-----------|---------------|------|
| 1 | read_file | Lê arquivo com linhas numeradas; suporta offset/limit | Sim | 2 |
| 2 | list_directory | Lista diretórios (recursivo opcional) | Sim | 2 |
| 3 | search_files | Busca por padrão glob; usa **/ para recursivo | Sim | 2 |
| 4 | grep_search | Busca regex/literal com contexto (include opcional) | Não (workdir) | 6 |
| 5 | write_file | Cria/sobrescreve arquivo inteiro | Sim | 6 |
| 6 | edit_file | Substituição exata (old_string → new_string) | Sim | 6 |
| 7 | web_fetch | Baixa URL e extrai texto (ou raw/markdown) | Não | 8 |
| 8 | web_search | Pesquisa na web e retorna links/trechos | Não | 8 |
| 9 | run_command | Executa comandos no terminal (allowlist/timeout) | Não | 9 |
| 10 | send_message | Envia mensagem via Telegram/Signal | Não | 10 |

---

## Arquitetura

### Estrutura de diretórios

```
internal/tools/
├── types.go          # Interface Tool, ToolResult, ToolCall
├── registry.go       # Registro central de ferramentas
├── executor.go       # Orquestrador com execução paralela e timeout
├── filesystem/       # Tools de arquivo
│   ├── read_file.go
│   ├── list_dir.go
│   ├── search_files.go
│   ├── grep_search.go
│   ├── write_file.go
│   └── edit_file.go
├── web/              # Tools web
│   ├── web_fetch.go
│   └── web_search.go
├── messaging/         # Tools de mensageria
│   └── send_message.go
└── shell/            # Execução de comandos
    └── run_command.go
```

### Interface principal

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() json.RawMessage   // JSON Schema
    Execute(ctx context.Context, args json.RawMessage) (ToolResult, error)
}

type ToolResult struct {
    Content  string         `json:"content"`
    IsError  bool           `json:"is_error,omitempty"`
    Metadata map[string]any `json:"metadata,omitempty"`
}
```

### Agentic Loop (pseudocódigo)

```go
func (a *App) agenticLoop(ctx, messages, tools, turnID, handler) {
    for i := 0; i < maxIterations; i++ {
        // 1. Chama LLM com streaming
        result := streamChat(ctx, messages, tools, handler)

        // 2. Segmento de texto terminou → verbalizar
        emit("chat:segment_done", result.Content)

        // 3. Se não tem tool calls, acabou
        if result.FinishReason == "stop" {
            salvar(assistant, content, turnID)
            break
        }

        // 4. Salvar mensagem assistant com tool_calls
        salvar(assistant, content, turnID, toolCalls)

        // 5. Executar tools em paralelo
        for each toolCall {
            emit("chat:tool_start", toolCall.Name)
            resultado := registry.Execute(ctx, toolCall)
            emit("chat:tool_end", toolCall.Name, status)
            salvar(tool, resultado, turnID, toolCallID)
            messages = append(messages, toolMessage)
        }

        // 6. Volta ao passo 1
    }
    emit("chat:done")
}
```

### Proteções

| Proteção | Valor | Motivo |
|---|---|---|
| maxIterations | 25 | Evitar loop infinito |
| Timeout por tool | 30s | Tool travada não bloqueia tudo |
| Timeout global | 5min | Proteção total do turno |
| Max resultado | 100KB | Evitar explodir contexto |
| context.Context | cancelável | Usuário pode cancelar |

---

## Eventos Wails

> **Adendo (2026-05):** esta tabela é histórica. O contrato vigente de eventos de chat é **backend-driven** e está tipado em `internal/core/ports/chat_events.go` (AEP-0040), com enriquecimentos em `chat:done`/`chat:segment_done` (AEP-0039). Todo evento carrega `conversationId` e, quando aplicável, `turnId`/`surfaceOrigin`.

| Evento | Quando | Dados | Verbalização |
|---|---|---|---|
| chat:stream | Chunks de texto (como hoje) | {content, done} | — |
| chat:segment_done | Segmento de texto completo | {content, iteration, hasMore} | TTS se habilitado, senão aria-live |
| chat:tool_start | Tool começou a executar | {name, args} | aria-live (polite) |
| chat:tool_end | Tool terminou | {name, status, summary, error?} | aria-live (polite), assertive se erro |
| chat:done | Turno completo | {conversationId} | — |

---

## Frontend

### Renderização por TurnID

Mensagens com mesmo TurnID são agrupadas em um único bloco visual:

```
┌─ Bloco do Assistente (TurnID=1) ────────────────────────┐
│ "Deixa eu verificar a estrutura do projeto."             │
│ ┌─ 🔧 read_file("main.go") ✅ ────────────────────────┐│
│ └──────────────────────────────────────────────────────┘│
│ ┌─ 🔧 read_file("go.mod") ✅ ─────────────────────────┐│
│ └──────────────────────────────────────────────────────┘│
│ "Achei alguns TODOs, deixa eu verificar melhor."         │
│ ┌─ 🔍 grep_search("TODO") ✅ ─────────────────────────┐│
│ └──────────────────────────────────────────────────────┘│
│ "O projeto tem 2 TODOs pendentes:..."                    │
└──────────────────────────────────────────────────────────┘
```

Cards de ferramentas colapsados por padrão, expandíveis pelo usuário.

### Verbalização intermediária

| Evento | Canal | Quando |
|---|---|---|
| Texto do assistente | TTS se habilitado, senão aria-live | Imediatamente ao fim de cada segmento |
| Tool executando | Só aria-live (polite) | Ao iniciar execução |
| Tool concluída | Só aria-live (polite) | Ao terminar |
| Tool com erro | aria-live (assertive) | Ao ocorrer erro |

---

## Fases de Implementação

### Fase 1 — Tipos base, interfaces e registry
- `internal/tools/types.go`
- `internal/tools/registry.go`
- `internal/tools/executor.go`
- **Sem dependências**

### Fase 2 — Tools de filesystem básicas
- `read_file`, `list_directory`, `search_files`
- **Depende de:** Fase 1

### Fase 3 — Modelo de dados
- 3 campos novos no ChatMessage (TurnID, ToolCalls, ToolCallID)
- Funções helper no database package
- **Sem dependências** (paralelo com Fase 1)

### Fase 4 — LLM client: suporte a tools no protocolo
- Campos tools/tool_calls no ChatRequest/ChatResponse
- Parser SSE de tool_calls (delta acumulado)
- Novo callback OnToolCalls no StreamHandler
- **Sem dependências** (paralelo com Fase 1)

### Fase 5 — Agentic loop
- Orquestrador com execução paralela, iterações e eventos Wails
- **Depende de:** Fases 1, 3, 4

### Fase 6 — Tools de filesystem avançadas
- `grep_search`, `write_file`, `edit_file`
- **Depende de:** Fase 1

### Fase 7 — Frontend: renderização e verbalização
- Eventos de tool calling no chatStore
- Renderização de blocos por TurnID
- Verbalização intermediária (TTS + aria-live)
- **Depende de:** Fase 5

### Fase 8 — Tools web
- `web_fetch`, `web_search`
- **Depende de:** Fase 1

### Fase 9 — Tool shell
- `run_command` (com confirmação do usuário)
- **Depende de:** Fases 1, 7

### Fase 10 — Perfis: seleção de tools
- Checkboxes por perfil para habilitar/desabilitar tools
- **Depende de:** Fases 5, 7

### Diagrama de dependências

```
Fase 1 (tipos/registry) ──┬──→ Fase 2 (read/list/search)
                           ├──→ Fase 6 (grep/write/edit)
                           ├──→ Fase 8 (web)
                           └──→ Fase 9 (shell)

Fase 3 (banco) ────────────┐
Fase 4 (LLM client) ───────┤
Fase 1 ────────────────────┘──→ Fase 5 (agentic loop) ──→ Fase 7 (frontend)
                                                           ──→ Fase 10 (perfis)
```

Fases 1, 3 e 4 são paralelizáveis. Fases 2, 6, 8 e 9 também entre si.
