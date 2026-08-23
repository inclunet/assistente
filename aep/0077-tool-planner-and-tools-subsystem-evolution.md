# AEP-0077 — ToolPlanner e Evolução do Subsistema de Tools

**Status:** Done

| Campo | Valor |
|-------|-------|
| Issues | [#122](https://github.com/inclunet/assistente/issues/122), [#120](https://github.com/inclunet/assistente/issues/120), [#119](https://github.com/inclunet/assistente/issues/119), [#121](https://github.com/inclunet/assistente/issues/121); rede de segurança: [#245](https://github.com/inclunet/assistente/issues/245) |
| Relacionados | AEP-0039 (Tool Calling Revamp), AEP-0063 (Tool Invocations & Executor Comum), AEP-0021 (MCP Native Mode), AEP-0040 (Backend-Driven Messaging), AEP-0071 (Structured Tool Output Size), AEP-0072 (Skill Loading Runtime), AEP-0075 (Context Providers), AEP-0081 (Tools por Perfil) |

## Resumo

O subsistema de tools funciona, mas sua **política de seleção está distribuída**, o **catálogo (`tool_catalog`) vive acoplado ao MCP**, os **metadados das builtins ficam num mapa central separado da definição da tool**, e o `tool_catalog` é hoje uma **listagem filtrável** — não um **planejador** que decide, com orçamento e ranking, *quais* tools entram no contexto de um turno por perfil/superfície.

Este AEP consolida quatro evoluções incrementais (#122 → #120 → #119 → #121) em um caminho único, precedido por uma fase de **rede de segurança** (#245: refatorar `RunAgenticLoop` e subir cobertura), de modo que cada mudança na fontanaria de tools seja feita sobre testes confiáveis. O capstone é o **ToolPlanner**: seleção determinística por **budget de schema bytes**, **ranking por perfil/superfície**, **pacotes preferenciais** e **resolução formal de conflitos bridge × native**.

Este AEP **estende** AEP-0039 e AEP-0063 (não os substitui). O registry runtime continua sendo a **fonte executável**; o catálogo permanece **índice/metadata** e `tool_invocations` continua sendo o storage canônico de resultados.

> **Evolução posterior (AEP-0081).** O ToolPlanner entregue aqui define ranking, budget de schema, pacotes preferenciais e resolução bridge×native. A política tri-state por perfil (`disabled` / `on_demand` / `preloaded`), o `tool_catalog` com `action=load|unload|list_loaded` e a persistência de tools carregadas por conversa/sessão são definidos separadamente na AEP-0081.

## Motivação

1. **Política de seleção distribuída** (#119): as decisões de "quais tools para este perfil/turno" estão espalhadas entre `internal/chat/tool_defs.go` (`BuildLLMToolDefs`, `ResolveInitialEnabledTools[WithRuntime]`, `BuildLLMToolDefsByNames`, `FilterToolNamesByEnabledTools`, `ResolveNativeMCPEnabled`, `FilterToolNamesForNativeMCP`, `ApplyNativeMCP`), o callback de expansão dinâmica no use case de envio, e os filtros do `tool_catalog`. Sem um ponto único, mudanças divergem e regressões passam despercebidas.
2. **Catálogo acoplado ao MCP** (#120): a persistência do `tool_catalog` mora em `internal/mcp/repository.go` + `internal/mcp/catalog_sync.go`. Builtins, MCP bridge e MCP native compartilham o catálogo, mas o dono do código é o pacote MCP — acoplamento que dificulta evolução e testes.
3. **Metadados de builtins num mapa central** (#122): `category/class/package/risk/tags` das builtins vivem separados da definição da tool (em `internal/tools/catalog.go`), criando uma fonte paralela de manutenção fácil de esquecer ao adicionar/alterar uma tool.
4. **`tool_catalog` é listagem, não planner** (#121): hoje (`internal/tools/catalog_tool.go`) ele filtra/lista capabilities; não há orçamento de contexto, ranking por relevância de perfil/superfície, pacotes preferenciais nem resolução determinística de conflito quando a mesma capability existe como bridge e native.
5. **`RunAgenticLoop` pouco testável** (#245, `prio:critical`): ~550 linhas em `internal/agent/service.go` concentram streaming, loop de tools, MCP nativo/fallback e recovery. Sem seams e cobertura, qualquer mudança na seleção de tools que esse loop consome é arriscada.

## Estado atual (mapa)

| Peça | Local | Papel |
|---|---|---|
| Registry runtime | `internal/tools/registry.go` | Fonte executável das tools |
| Catálogo (tipos + metadata builtin) | `internal/tools/catalog.go` | `ToolCatalogEntry` (`category/class/package/risk/schema_bytes`) + mapa central de builtins |
| Tool de listagem do catálogo | `internal/tools/catalog_tool.go` | Lista/filtra capabilities (exposta ao modelo) |
| Persistência do catálogo | `internal/toolcatalog/repository.go`, `internal/toolcatalog/service.go` | Sync/storage do catálogo (pacote dedicado; F2/#120 concluída — MCP consome via `Manager.SetCatalog`) |
| Política de seleção | `internal/chat/tool_selection_policy.go` (`ToolSelectionPolicy`); `tool_defs.go` são wrappers finos (F3/#119 concluída) | Resolve enabled tools, filtra por perfil, aplica MCP nativo e expansão dinâmica num ponto único |
| Loop agêntico | `internal/agent/service.go` (`RunAgenticLoop`) | Consome tool defs/política; streaming, loop, MCP nativo, recovery |
| Storage de resultados | `internal/toolinvocations/*` (AEP-0063) | `tool_invocations` canônico via `tool_catalog_id` |

## Decisões

- **D1 — Metadados na definição da tool (#122).** Cada builtin declara seus próprios metadados de catálogo (`category`, `class`, `package`, `risk`, `tags`, `schema`) junto da definição/registro, eliminando o mapa central paralelo. O catálogo passa a ser **completo e autoritativo** por construção. Sem mudança de schema do banco.
- **D2 — Serviço próprio de catálogo (#120).** Extrair a persistência/sync do `tool_catalog` para um pacote dedicado (ex.: `internal/toolcatalog`), com dono claro, consumido por MCP, builtins e (futuro) planner. **Mesma tabela e migrações**; muda a propriedade do código e os call sites.
- **D3 — Política de seleção única (#119).** Consolidar a seleção por perfil/superfície atrás de **um** serviço/contrato (ex.: `ToolSelectionPolicy`), reusado por chat, agent e CLI. As funções de `tool_defs.go` e o callback de expansão dinâmica passam a delegar a esse contrato. **Paridade comportamental obrigatória** (caracterizar antes de refatorar).
- **D4 — ToolPlanner (#121).** Sobre D1–D3, evoluir a listagem para um planejador determinístico com: **budget de schema bytes** (teto de bytes de schema injetados por turno), **ranking** por relevância de perfil/superfície, **pacotes preferenciais**, e **resolução de conflito bridge × native** alinhada à AEP-0021. A saída do planner é a lista final de `ToolDefinition` + telemetria (o que entrou, o que foi cortado e por quê).
- **D5 — Rede de segurança primeiro (#245).** Refatorar `RunAgenticLoop` extraindo seams (streaming, iteração de tools, MCP nativo/fallback, recovery) e elevar cobertura **antes** de tocar D3/D4. Esta fase não muda comportamento; só cria a malha de testes.

## Fases

| Fase | Issue | Conteúdo | Depende de | Risco |
|---|---|---|---|---|
| **0** | #245 | Refatorar `RunAgenticLoop` em seams testáveis + cobertura do loop (sem mudança de comportamento) | — | Médio (regressão) |
| **1** | #122 | Mover metadados de catálogo para junto das builtins; remover mapa central | — | Baixo |
| **2** | #120 | Extrair `internal/toolcatalog` (repo/serviço) do MCP; atualizar call sites | F1 | Médio |
| **3** | #119 | Política de seleção única (`ToolSelectionPolicy`); delegar `tool_defs.go` + expansão dinâmica; testes de caracterização | F0, F2 | Médio-Alto |
| **4** | #121 | ToolPlanner: budget, ranking, pacotes preferenciais, conflito bridge×native, telemetria | F1, F2, F3 | Alto |

**Paralelismo seguro:** F0 (#245) e F1 (#122) são independentes e podem rodar em paralelo (worktrees/PRs separados). F2→F3→F4 são sequenciais.

## Riscos

- **Regressão silenciosa de seleção** (F3/F4): a refatoração não pode mudar quais tools são oferecidas sem intenção. Mitigação: testes de caracterização (snapshots de tool defs por perfil) escritos na F0/F3 antes do refactor.
- **Conflito com AEP-0021** (MCP nativo): a resolução bridge×native do planner deve respeitar a política tri-state existente (`ResolveNativeMCPEnabled`) e a remoção de bridge tools em `ApplyNativeMCP`.
- **Acoplamento residual MCP↔catálogo** (F2): o sync do catálogo MCP não pode regredir; manter `catalog_sync` funcional sob o novo dono.
- **Budget mal calibrado** (F4): cortar tools demais quebra fluxos; cortar de menos estoura contexto. Mitigação: budget configurável + telemetria do que foi cortado.
- **F0 grande**: `RunAgenticLoop` é crítico; refatorar sem cobertura prévia é arriscado — daí F0 ser pré-requisito e ela própria começar pela malha de testes.

## Critérios de Aceitação

- [x] **F0/#245** (PR #318): `RunAgenticLoop` decomposto em unidades testáveis (`internal/agent/agentic_loop.go`); cobertura do loop agêntico elevada; comportamento idêntico (sem mudança de contrato de eventos AEP-0040).
- [x] **F1/#122** (PR #317): metadados de catálogo declarados junto de cada builtin (interface `CatalogMetadataProvider`); mapa central removido; catálogo de builtins idêntico ao anterior (teste de equivalência `golden`).
- [x] **F2/#120** (PR #320): `tool_catalog` num pacote próprio fora de `internal/mcp` (`internal/toolcatalog`); mesma tabela/migrações; MCP, builtins e demais call sites consumindo o novo serviço.
- [x] **F3/#119** (PR #321): um único contrato de política de seleção por perfil/superfície (`internal/chat.ToolSelectionPolicy`); `tool_defs.go` e a expansão dinâmica do use case de envio delegando a ele (callbacks duplicados eliminados); testes de caracterização (snapshots por perfil/cenário) garantindo paridade.
- [x] **F4/#121** (PR #322): ToolPlanner com budget de schema bytes, ranking por perfil/superfície, pacotes preferenciais e resolução bridge×native; telemetria de seleção (incluído/cortado + motivo); budget configurável. Implementado em `internal/toolcatalog/planner.go` (planner determinístico puro) e plugado em `internal/chat.ToolSelectionPolicy` (`PlanTurnToolDefs`/`ResolveExpandedToolDefs`); budget e pacotes preferenciais configuráveis por perfil (`ChatConfig.ToolSchemaBudgetBytes`/`PreferredToolPackages`); default seguro (budget `0` = ilimitado, sem regressão); o budget é orçado sobre o conjunto final/acumulado de cada caminho (nativo×adapter, expansão dinâmica) e a resolução bridge×native consome a decisão tri-state existente (AEP-0021) sem duplicá-la.
- [x] AEP marcado **Done** com as fases entregues e PRs referenciados (#317, #318, #320, #321, #322).

## Relações

- **AEP-0039 (Tool Calling Revamp)**: este AEP dá o passo de *seleção/planejamento* que a 0039 não detalhou; estende, não substitui.
- **AEP-0063 (Tool Invocations & Executor Comum)**: storage de resultados permanece em `tool_invocations` via `tool_catalog_id`; o planner decide *o que entra*, não *como persiste*.
- **AEP-0021 (MCP Native Mode)**: a resolução bridge×native do planner reusa a política tri-state existente.
- **AEP-0072 (Skill Loading Runtime)**: progressive disclosure de skills é o análogo já entregue; o ToolPlanner traz disciplina semelhante (budget/catálogo leve) para tools.
- **AEP-0071 (Structured Tool Output Size)**: o budget de *entrada* (schema bytes) do planner complementa o teto de *saída* já definido lá.
- **AEP-0081 (Tools por Perfil)**: estende o planner com estados tri-state por perfil, carregamento sob demanda via `tool_catalog` e persistência por conversa/sessão.
