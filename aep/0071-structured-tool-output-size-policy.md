# AEP-0071 — Política canônica de tamanho para saídas estruturadas de tools

Status: Done
Data: 2026-06-05
Autor: Inclunet + Cursor Agent

## Resumo

Esta AEP define a convenção canônica para lidar com o **limite de tamanho de
resultado** (`MaxResultSize`) quando uma tool retorna uma **saída estruturada**
(ex.: JSON canônico). O executor de tools trunca resultados acima do limite e
anexa um aviso textual; para saídas estruturadas isso **corromperia o JSON** e
quebraria consumidores que fazem `json.Unmarshal` (LLMs e jobs).

A solução: a tool declara `ToolResult.Structured = true` e o **executor comum**
passa a ser o dono único da política — em vez de truncar, ele falha de forma
**explícita** quando uma saída estruturada excede o limite. Tools deixam de
duplicar o guard de tamanho.

## Motivação

O problema apareceu primeiro em `feed_read` (AEP-0069) e depois em `web_search`
(AEP-0070). Em ambos, a tool serializava JSON canônico e, para não devolver JSON
truncado/inválido, repetia o mesmo bloco:

```go
maxResultSize := tools.MaxResultSizeFromContext(ctx)
if len(encoded) > maxResultSize {
    return tools.ToolResult{Content: "...reduza max_items/max_results...", IsError: true}, nil
}
```

Essa duplicação:

- **se repetiria** em toda tool de saída canônica futura;
- divergia em mensagem e em detalhes por tool;
- exigia que cada tool conhecesse `MaxResultSizeFromContext`, embora o
  truncamento, de fato, aconteça **no executor**.

A truncagem é uma responsabilidade do executor; logo, a política de "não corromper
saída estruturada" também deve viver lá.

## Design

### Contrato: `ToolResult.Structured`

```go
type ToolResult struct {
    Content    string
    IsError    bool
    Metadata   map[string]any
    // Structured sinaliza que Content é uma saída canônica/estruturada (ex.: JSON)
    // que NÃO pode ser truncada — truncar a corromperia.
    Structured bool
}
```

### Política no executor (fonte única da verdade)

No `internal/tools/executor.go`, ao aplicar o limite:

- **`Structured == true`** e `len(Content) > MaxResultSize` → substitui o
  resultado por um **erro explícito** padronizado, orientando a reduzir o escopo
  da chamada (ex.: `max_results`/`max_items`). Nunca trunca.
- **`Structured == false`** → comportamento atual: truncagem UTF-8 safe com aviso
  `[TRUNCADO: ...]` e `Metadata["truncated"] = true`.

A política cobre **os dois caminhos** de execução, pois ambos passam pelo
`tools.Executor`:

- **chat** (interativo) usa o executor com `MaxResultSize` default (100KB);
- **jobs** usam `toolinvocations.Service`, que cria/reusa um `tools.Executor` com
  `cfg.MaxResultSize = ExecutionMaxResultSize` (budget maior).

A truncagem de **persistência** (`truncateForPersistence`, que limita o que é
gravado em `tool_invocations`) é separada e independente: afeta apenas a cópia
armazenada no DB, não o resultado devolvido ao chamador.

### O que as tools fazem

```go
return tools.ToolResult{
    Content:    string(encoded), // JSON canônico
    Structured: true,
    Metadata:   map[string]any{ /* ... */ },
}, nil
```

Tools canônicas (`feed_read`, `web_search`, e futuras) **não** leem mais
`MaxResultSizeFromContext` nem repetem o guard.

## Limite "razoável"

Com a política centralizada, o limite passa a ser **um número** ajustável num só
lugar (`ExecutorConfig.MaxResultSize`, default `DefaultMaxResultSize` = 100KB;
jobs usam `JobExecutionMaxResultSizeBytes`). Se necessário, dá para elevar o
default para saídas estruturadas sem tocar nas tools.

## Alternativas consideradas

1. **Guard por tool (status quo)** — duplicação e divergência; rejeitado.
2. **Helper compartilhado** `tools.JSONResult(ctx, v)` que marshala e checa o
   limite — reduz duplicação, mas mantém a tool responsável por conhecer o
   limite e não cobre o caminho onde o executor reaplica truncagem; preterido em
   favor de centralizar no executor.
3. **Flag + política no executor (escolhido)** — fonte única da verdade, tools
   só declaram intenção, mensagem consistente, cobre chat e jobs.

## Impacto

- `internal/tools/types.go`: novo campo `ToolResult.Structured`.
- `internal/tools/executor.go`: truncagem ciente de `Structured`.
- `internal/tools/feed/feed_read.go` e `internal/tools/web/web_search.go`:
  removem o guard ad-hoc e passam `Structured: true`.
- Testes: política validada em `internal/tools/executor_test.go`
  (`TestStructuredResultNotTruncated`); as tools testam apenas que declaram
  `Structured`.

`MaxResultSizeFromContext`/`WithMaxResultSize` permanecem como utilitário público
(o executor ainda injeta o limite efetivo no ctx), mas não são mais necessários
para o tratamento de saída estruturada.
