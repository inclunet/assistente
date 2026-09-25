# AEP-0063 — Tool Invocations e Executor Comum

**Status:** Done — executor comum e ledger exclusivo entregues

## Dependências

- **AEP-0039** (Tool Calling Revamp): base conceitual para tool calling, eventos e execução estruturada.
- **AEP-0046** (Migração de IDs sequenciais para UUIDv7): fornece o padrão de IDs UUIDv7 usado por `tool_invocations.id`.
- **AEP-0048** (Jobs Database Migration): jobs precisam estar no banco para referenciar execuções de tools sem depender de arquivos.
- **AEP-0049** (MCP Database Migration): `tool_catalog` é a fonte canônica de tools nativas e MCP.
- **AEP-0052** (Multi-user accounts): invocações são sempre associadas a `user_id`.

## Resumo

Criar uma camada única de execução e persistência para chamadas de tools: `ToolInvocationService` + tabela `tool_invocations`. O chat, jobs, testes/dry-run e integrações MCP passam a usar o mesmo executor, o mesmo modelo de status e a mesma trilha técnica de execução.

Resultados de tools não são armazenados como mensagens de chat ou logs em
arquivos. Mensagens representam a conversa; `tool_invocations` representa o
histórico técnico e segue a retenção definida por origem. A remoção integral
dos fallbacks e cópias foi concluída pela
[AEP-0104](0104-tool-invocations-como-ledger-canonico.md).

## Motivação

1. **Um executor para todos os fluxos**: chat, jobs e dry-run hoje tendem a duplicar parsing, execução, timeout, erro e serialização.
2. **Histórico técnico consultável**: chamadas de tools precisam ser auditáveis e depuráveis sem misturar output técnico com mensagens da conversa.
3. **Jobs e MCP alinhados**: jobs executam tools nativas e MCP; o mesmo registro deve funcionar para ambas.
4. **Dry-run reutilizável**: testes de tools pelo `tool_catalog` e dry-runs de jobs devem usar a mesma infraestrutura.
5. **Controle de volume**: logs de tools podem crescer rapidamente; precisam de retenção própria e não devem ser preservados como histórico permanente.

## Decisões

### D1 — `tool_catalog_id` é a referência canônica

`tool_invocations` referencia `tool_catalog.id`. Não armazenamos snapshot redundante de `tool_name`.

Se uma tool for removida acidentalmente do catálogo, o histórico técnico pode perder legibilidade parcial. Isso é aceitável porque `tool_invocations` é log efêmero, não registro permanente de negócio.

### D2 — Sem tool results em mensagens

O agentic loop pode usar resultados de tools para compor a próxima resposta, mas o armazenamento persistente do resultado vai para `tool_invocations`.

Mensagens podem conter referências leves, como `tool_invocation_id`, quando a UI precisar associar uma resposta a uma execução. O payload bruto fica fora da tabela de mensagens.

### D3 — `ToolInvocationService` como único caminho de execução

O serviço é dependência obrigatória no wiring/construtor de cada executor.
Configuração ausente falha na montagem; a defesa runtime falha antes de qualquer
efeito e nunca chama um executor direto como fallback. Caminhos de execução sem
ledger não fazem parte da compatibilidade histórica.

O serviço recebe um pedido normalizado:

- `user_id`
- `tool_catalog_id`
- `origin_type`
- `origin_id`
- `input`
- `dry_run`
- `timeout`
- metadados opcionais

Ele tenta resolver a tool no catálogo, aplica políticas de execução e chama o
adapter nativo ou MCP. Quando a resolução produz um registro persistível, grava
status/duração/input/output/erro. Desde a fase 3 da AEP-0104, catálogo ausente
gera resolução archival indisponível e falha de auditoria antes do `Create`
impede a execução.

### D4 — Origens explícitas

Cada invocação informa de onde veio:

| `origin_type` | `origin_id` |
|---|---|
| `chat` | `chat_messages.id` ou execução do agentic loop |
| `job_run` | `job_runs.id` |
| `tool_catalog` | teste manual do catálogo |
| `system` | automação interna |
| `command_invocation` | `command_invocations.invocation_id`, com owner autenticado |

`origin_id` é string para não acoplar a tabela a uma única FK. Quando houver origem conhecida e estável, o código valida a existência antes de executar.

Adendo AEP-0103 (15/09/2026, integração ainda não publicada no App): a ponte
`commandtoolbridge` exige rota tipada e ID canônico visível de catálogo,
preserva `command_invocation` como origem e delega ao MESMO
`toolinvocations.Service`. Não há execução direta nem fallback para usuário
system. Paths sensíveis do contrato acompanham input/output e os artefatos
retomáveis de resultados grandes; diagnóstico livre que possa repetir segredo
é omitido no armazenamento. Falha de adaptação do resultado depois de efeito
não equivale a prova de falha sem efeito: o comando usa `outcome_unknown`.
O registro das rotas reais e a retenção de origem de comando permanecem
dependências de montagem/manutenção da AEP-0103; esta adição não as habilita.

### D5 — Logs efêmeros e retenção própria

Adendo AEP-0103 (17/09/2026, R05.2 ainda parcial): `commandtoolbridge`
preserva conversa/turno, identidade estrutural da superfície e profile de
origem. Profile de destino nunca substitui o profile-pai usado para avaliar
delegação cross-profile. O caminho de jobs usa o mesmo ToolInvocationService,
com `origin_type=job_run`; o vínculo ao comando fica na cadeia estrutural do
run, sem reutilizar o ID da invocação técnica. Policies de paths da tool
delegada são fixadas pelo bootstrap e limitadas ao job alvo, não a seus
descendentes. Nada disso cria grants ou publica novas rotas no App.

`tool_invocations` tem retenção por origem conforme a
[AEP-0074-B — Compactação e Retenção do Banco de Dados](0074-database-compaction-and-retention.md).
Invocações de chat integram a timeline da conversa e, por padrão, acompanham
seu ciclo de vida sem expiração temporal. Invocações operacionais de
jobs/dry-run são efêmeras e acompanham a retenção curta dos jobs.

Contrato vigente:

- manter invocações de chat enquanto a conversa existir
  (`chat_tool_calls_retention_days=0` por padrão);
- manter invocações de jobs alinhadas a `job_retention_hours` e aos
  `job_runs` removidos;
- permitir limpeza manual futura pela UI/API.

### D6 — Dry-run é uma invocação real em modo teste

Dry-run usa o mesmo executor e grava `dry_run = true`. O booleano `dry_run` é modo de execução, não origem: a origem continua sendo `chat`, `job_run`, `tool_catalog` ou `system`.

A diferença está no adapter: ele pode usar mock output, validação de schema, modo seguro da tool ou execução real marcada como teste, conforme a capacidade declarada no catálogo.

Isso cobre tanto dry-run de jobs quanto teste manual de uma tool no `tool_catalog`.

## Tabela `tool_invocations`

| Coluna | Tipo | Constraints | Notas |
|---|---|---|---|
| `id` | TEXT | PK | UUIDv7 |
| `user_id` | TEXT | FK→users.id, NOT NULL, INDEX | Dono da invocação |
| `tool_catalog_id` | TEXT | FK→tool_catalog.id, NOT NULL, INDEX | Fonte canônica da tool |
| `origin_type` | TEXT | NOT NULL, INDEX | `chat`/`job_run`/`tool_catalog`/`system` |
| `origin_id` | TEXT | INDEX | ID da origem, quando houver |
| `status` | TEXT | NOT NULL, INDEX | `queued`/`running`/`succeeded`/`failed`/`cancelled`/`timed_out` |
| `dry_run` | BOOL | NOT NULL, DEFAULT false | Execução de teste |
| `input` | TEXT | | JSON normalizado enviado à tool |
| `output` | TEXT | | JSON/texto normalizado retornado |
| `error` | TEXT | | Erro legível |
| `error_code` | TEXT | | Código opcional para UI/retry |
| `error_kind` | TEXT | INDEX | Tipo estável da falha |
| `retryable` | BOOL | NOT NULL, DEFAULT false | Decisão de retry, quando conhecida |
| `retryability_known` | BOOL | NOT NULL, DEFAULT false | Distingue decisão explícita do default legado |
| `queued_at` | DATETIME | NOT NULL, INDEX | Momento em que a invocação entrou na fila |
| `started_at` | DATETIME | INDEX | Início real da execução; nulo enquanto `queued` |
| `completed_at` | DATETIME | | Fim |
| `duration_ms` | INT | | Duração entre `started_at` e `completed_at`, quando ambos existem |
| `metadata` | TEXT | | JSON pequeno para adapter, versão, policy |
| `created_at` | DATETIME | | |
| `updated_at` | DATETIME | | |

Índices:

- `(user_id, origin_type, origin_id)`
- `(user_id, origin_type, origin_id, queued_at, id)` para hidratação ordenada
  em lote dos turnos de chat (issue #739)
- `(user_id, tool_catalog_id, started_at)`
- `(user_id, status, queued_at)`
- `(user_id, dry_run, queued_at)`

O lote de hidratação continua em até 400 `origin_id`s, abaixo do limite de
variáveis do SQLite. Medição por `EXPLAIN QUERY PLAN` mostrou que reduzir
mecanicamente para 20 apenas multiplicaria round trips; o índice acima elimina
o scan amplo preservando uma consulta por lote e a ordem `queued_at, id`.

## Fluxo sem diagrama

1. Um fluxo pede uma execução: chat, job, dry-run ou sistema.
2. O chamador resolve ou informa o `tool_catalog_id`.
3. `ToolInvocationService` cria a invocação como `queued` e preenche `queued_at`.
4. O serviço muda para `running`, preenche `started_at`, aplica timeout/política e chama o adapter correto.
5. O adapter executa tool nativa ou MCP.
6. O serviço grava `succeeded`, `failed`, `timed_out` ou `cancelled`, com duração, output e erro.
7. O chamador recebe um resultado normalizado e decide como continuar o fluxo.

O resultado normalizado preserva ainda o código estável da falha, seu tipo e
uma decisão explícita de retryability. A ausência dessa decisão em tools
legadas não equivale a `Retryable=false`: chamadores conservam seu default
anterior. Jobs encerram na primeira tentativa apenas quando a execução declara
explicitamente uma falha permanente; falhas transitórias e não classificadas
continuam obedecendo `error_policy`, `max_retries` e backoff.

## Integração com jobs

Jobs passam a registrar execução operacional em `job_runs` e execução técnica de tools em `tool_invocations`.

- `job_runs`: status operacional do job, trigger usado, retry, duração e erro operacional.
- `job_run_events`: timeline do run.
- `tool_invocations`: chamada concreta da tool executada pelo run.

Um `job_run` pode ter uma ou mais invocações. O caso inicial é uma invocação por run, mas o modelo suporta jobs compostos no futuro.

`job_runs` não duplica `tool_name`, `trigger_type`, inputs resolvidos ou output bruto em texto. A tool vem do relacionamento com `tool_invocations.tool_catalog_id`; o trigger vem de `job_runs.trigger_id`; os dados técnicos da chamada ficam em `tool_invocations`. Eventos técnicos como início/fim de tool também ficam em `tool_invocations`, não em `job_run_events`.

## Integração com chat

O agentic loop deixa de persistir tool results como mensagens. Em vez disso:

- mensagens continuam guardando texto de usuário/assistente;
- o loop guarda `tool_invocations` para cada chamada;
- a reconstrução de contexto usa uma representação compacta quando necessário, não o output bruto permanente dentro de mensagens.

Isso reduz crescimento do histórico e separa conversa de execução técnica.

## Integração com MCP

O bridge MCP e as tools nativas usam o mesmo contrato:

- `tool_catalog` identifica a tool;
- adapter MCP traduz input/output para o protocolo MCP;
- adapter nativo chama handlers Go;
- ambos retornam `ToolInvocationResult`.

## Fases

### Fase 1 — Schema e repository ✅

1. Criar model GORM de `tool_invocations`.
2. Adicionar `AutoMigrate`.
3. Criar repository com CRUD mínimo, listagem por origem e limpeza por retenção.
4. Cobrir status, duration, erros e filtros por testes.

### Fase 2 — Executor comum ✅

5. Criar `internal/tools/invocation_service.go`.
6. Definir interfaces de adapter para native e MCP.
7. Implementar timeout, status transitions e persistência transacional.
8. Normalizar erros e resultado.

### Fase 3 — Jobs e dry-run ✅

9. Migrar execução de jobs para usar `ToolInvocationService`.
10. Conectar dry-run de jobs ao mesmo executor.
11. Criar teste manual de tools no `tool_catalog` usando `dry_run = true`, preferencialmente exposto pela tool composta `job_catalog` em vez de multiplicar tools de teste.

### Fase 4 — Chat ✅

12. Migrar agentic loop local/bridge para registrar invocações.
13. Parar de persistir resultados brutos de tools em mensagens novas.
14. Ajustar reconstrução de contexto para usar resumos/referências.

### Fase 5 — MCP e retenção ✅

15. Integrar MCP nativo/bridge ao mesmo executor.
16. Implementar limpeza periódica por idade e por runs removidos.
17. Expor listagem/diagnóstico para UI/API quando necessário.

### Fase 6 — Ledger exclusivo e teardown ✅

18. ✅ Backfill retomável de mensagens e runs publicados (v18).
19. ✅ Parar toda escrita `role=tool`/`tool_calls`/`tool_call_id`.
20. ✅ Migrar consumidores, projeções e detalhes lazy para o ledger.
21. ✅ Remover escrita e leitura direta das cópias técnicas de `job_runs`;
    detalhes são hidratados em lote pelo ledger.
22. ✅ Reconstruir o schema legado sem as colunas físicas.

Esta fase foi executada em sete PRs pela AEP-0104. A migração v19 conclui o
cutover físico e promove os checkpoints reconciliados para `canonical`.

## Arquivos previstos

| Arquivo | Mudança |
|---|---|
| `internal/database/models_tool_invocations.go` | Model GORM |
| `internal/tools/invocation_repository.go` | Repository |
| `internal/tools/invocation_service.go` | Executor comum |
| `internal/tools/invocation_service_test.go` | Testes do executor |
| `internal/jobs/executor.go` | Passa a chamar o executor comum |
| `internal/chat/interactor.go` | Registra tool calls como invocações |
| `internal/mcp/*` | Adapter MCP usa o executor comum |
| `controllers/*` | Endpoints de dry-run/diagnóstico, se necessário |

## Riscos

| # | Risco | Probabilidade | Impacto | Mitigação |
|---|---|---|---|---|
| R1 | Crescimento excessivo de `tool_invocations` | Alta | Médio | Retenção por idade/origem e limpeza associada a `job_runs` removidos |
| R2 | Reconstrução de contexto perde informação útil ao parar de gravar tool results como mensagens | Média | Alto | Representação compacta para contexto e links por `tool_invocation_id` quando necessário |
| R3 | Divergência entre adapters nativos e MCP no executor comum | Média | Médio | Contrato único de `ToolInvocationResult` e testes compartilhados de sucesso/falha/timeout |
| R4 | Dry-run executa efeitos colaterais por engano | Baixa | Alto | Capacidade de dry-run declarada no catálogo e adapters explícitos para mock/validação/modo seguro |
| R5 | `started_at` nulo em invocações enfileiradas quebra consultas antigas | Baixa | Baixo | Consultas de fila usam `queued_at`; duração só é calculada após transição para `running` |

## Critérios de aceitação

- [x] Toda chamada nova por chat, job ou dry-run que resolve uma entrada
  persistível de catálogo cria `tool_invocations`.
- [x] Chat sem entrada persistível resolve catálogo archival ou falha fechado,
  sem fallback em mensagens.
- [x] `tool_catalog_id` é canônico e não duplica `tool_name`.
- [x] Jobs mantêm estado em `job_runs` e execução técnica em invocações.
- [x] Chat não grava novos resultados brutos como mensagens no caminho feliz.
- [x] Dry-run e teste de catálogo usam o executor compartilhado.
- [x] Tools internas, MCP bridge e MCP nativo têm representação consistente.
- [x] Retenção/limpeza respeita a política por origem da
  [AEP-0074-B — Compactação e Retenção do Banco de Dados](0074-database-compaction-and-retention.md).
- [x] Testes cobrem sucesso, falha, timeout, dry-run, chat e `job_run`.
- [x] Nenhuma execução ou resultado técnico é persistido em `chat_messages`.
- [x] `job_runs` não recebe novas cópias de tool, input ou output da execução;
  consultas hidratam esses dados exclusivamente de `tool_invocations`.
- [x] Não existem dual-read, dual-write ou fallback legado após o cutover.

Evidências: `internal/toolinvocations/{repository,service}_test.go`,
`internal/toolinvocations/hydration_test.go`,
`internal/database/query_performance_indexes_test.go`,
`internal/agent/service_tool_calls_persistence_test.go`,
`internal/jobs/executor_toolinvocations_test.go`,
`manager_toolinvocations_test.go` e `internal/wailsapi/jobs_dryrun_test.go`.
O contrato de código/retryability e a interrupção seletiva de retries de jobs
são cobertos também por `internal/tools/executor_test.go` e
`internal/jobs/executor_toolinvocations_test.go`.

## Registro da transição concluída do Issue #127

Esta seção registra como o critério de aceite "há migração/compatibilidade para dados
existentes OU um plano explícito de transição" do issue #127 foi atendido naquele
escopo. O executor comum e a separação exclusiva do ledger foram concluídos;
L1/L3 e as cópias técnicas de runs foram removidos pela AEP-0104.

### O que já migrou para `tool_invocations`

Já usam o executor comum (`internal/toolinvocations.Service`) e persistem em `tool_invocations`:

- **Chat / agentic loop** (`internal/agent/service.go`): cada tool call passa por
  `Service.Execute`/`ExecuteAll` com `origin_type = chat` e
  `origin_id = turnID`, com `conversation_id`/`turn_id` explícitos. O resultado
  técnico fica em `tool_invocations`; catálogo ausente gera entrada archival e
  falha de persistência não aciona fallback em mensagens.
- **Jobs** (`internal/jobs/executor.go`): execuções reais de tools chamam `Service.Execute`
  com `origin_type = job_run` e `origin_id = run.RunID` (o ID do `job_run`). Isso é o vínculo
  origem→armazenamento comum: dado um `job_run`, é possível listar suas invocações por
  `(origin_type=job_run, origin_id=run_id)`. Coberto por
  `internal/jobs/executor_toolinvocations_test.go`.
- **Dry-run** (`internal/jobs/manager.go`, `internal/jobs/executor.go`): usa o mesmo executor
  com `dry_run = true` (a origem permanece `job_run`/`tool_catalog`, conforme D6).
- **MCP nativo** (`internal/agent/service.go` → `Service.Record`): invocações executadas fora
  do executor comum (pelo provedor LLM) são registradas via `Record` no mesmo formato,
  marcadas com `metadata.external = true`. MCP bridge e tools internas/nativas passam pelo
  mesmo caminho `Execute`, garantindo representação consistente (D-MCP / critério de
  consistência MCP↔builtin do issue #127).
- **Export/Import** (`internal/portability/service.go`): resultados são
  exportados e importados no bloco canônico `toolInvocations`; protocolo
  técnico embutido em mensagens é rejeitado.

### Estado final após o cutover

Compatibilidade antiga não constitui caminho alternativo de execução. L1/L3
foram migrados e removidos fisicamente; nenhum writer ou leitor os aceita. L2
não é compatibilidade:
`job_run_events` representa eventos operacionais, enquanto toda execução
técnica vive exclusivamente no ledger.

#### L1 — Fallback `role=tool` no chat

- **Onde existia**: `RunAgenticLoop` e `persistNativeMCPCalls`, via
  `msgRepo.AddToolResultMessage`.
- **Estado atual**: removido do runtime na fase 3 da AEP-0104. Catálogo ausente
  gera entrada archival; indisponibilidade do ledger impede o efeito local e
  MCP já executado pelo provider registra erro sem fabricar uma mensagem.
- **Status**: runtime, interfaces, leitores e schema físico removidos.

#### L2 — Timeline própria de jobs em `job_run_events`

- **Onde**: `internal/jobs/repository.go` (`LogRunEvent`/`GetRunEvents`, modelo
  `database.JobRunEvent`).
- **O que guarda**: eventos **operacionais** do run (triggered, event_emitted, completed,
  failed, skipped) — a timeline do job, não a chamada técnica da tool.
- **Por que permanece**: `job_run_events` e `tool_invocations` têm responsabilidades
  distintas e complementares (ver seção "Integração com jobs"): a timeline operacional do job
  vive em `job_run_events`; a execução técnica da tool vive em `tool_invocations`. Jobs
  **referenciam** o armazenamento comum pelo vínculo `origin_id = run_id`, satisfazendo o
  critério "jobs não dependem de um log isolado para representar chamadas de tools; usam ou
  referenciam o armazenamento comum". `job_run_events` não representa a chamada de tool — ela
  é referenciada via o `job_run`.
- **Status**: mantido por design. Não é storage redundante de tool calls.

#### L3 — `tool_calls` JSON em mensagens assistant

- **Onde existia**: `chat_messages.tool_calls` e leitores de compatibilidade.
- **Estado atual**: removido dos modelos, interfaces, leitores, testes e schema
  físico; timeline, exportação e sumarização usam apenas `tool_invocations`.
- **Status**: removido.

### Plano e critérios para deprecar cada legado

| Legado | Ação | Critério para deprecar |
|---|---|---|
| L1 `role=tool` | Remover do runtime. | Backfill comprovado e escrita ledger-only; falha de auditoria fecha a execução. |
| L2 `job_run_events` | **Não deprecar.** | Permanece como timeline operacional. Só seria reavaliado se a UI de jobs passar a derivar a timeline inteiramente de `tool_invocations` + `job_runs`, o que não é objetivo do issue #127. |
| L3 `tool_calls` JSON em mensagens | Migrar, cortar leitura e remover fisicamente. | Gate transacional e rebuild da Fase 7 da AEP-0104. |

### Migração dos dados existentes

- O backfill v18 é aditivo, user-scoped, retomável e mantém mensagens
  `role=tool`/`tool_calls` intactas durante a transição. Ausência, ambiguidade
  ou divergência de hash mantém o recurso `pending`.
- A migração v19 exige todos os recursos `backfilled`, cria backup verificável,
  remove as representações antigas e promove o estado para `canonical`.

### Critérios de aceite do issue #127 — mapeamento

| Critério do issue | Situação | Evidência |
|---|---|---|
| Executor comum em chat e jobs | Atendido | `internal/toolinvocations`, `internal/agent/service.go`, `internal/jobs/executor.go` |
| Invocações em tabela própria com vínculo à origem | Atendido | `database.ToolInvocation`, `origin_type`/`origin_id` |
| Jobs referenciam armazenamento comum (não dependem de log isolado) | Atendido | `executor.go` (`origin_id = run.RunID`) + `executor_toolinvocations_test.go`; `job_run_events` documentado como timeline operacional (L2) |
| Tool results de chat não exclusivamente como mensagens | Atendido | Persistência e hidratação exclusivas via `tool_invocations`; L1 removido |
| MCP e tools internas representadas de forma consistente | Atendido | `Execute`/`Record` unificados; `metadata.external` para MCP nativo |
| Migração/compatibilidade OU plano explícito de transição | **Atendido por esta seção** | Plano L1/L2/L3 + critérios de deprecação |
| Testes cobrindo chat, job e dry-run no mesmo executor | Atendido | `service_tool_calls_persistence_test.go`, `executor_toolinvocations_test.go`, `manager_toolinvocations_test.go`, `internal/wailsapi/jobs_dryrun_test.go` |

## Relação com issues

### Integração interna com comandos — AEP-0103, 17/09/2026

Seção 38: a preparação da origem de eventos acontece no worker da ponte,
antes de `Execute`, e sua revalidação final usa `BeforeExecute` após a
persistência. O runtime carimba a origem privada do comando/da camada reativa
no contexto da tool; efeitos sobre tasklists preservam essa raiz e a cadeia
no job descendente. Isso não cria run artificial nem amplia grants. Cleanup
da preparação preserva owner, cancelamento e deadline do pedido original.

A montagem local de alvo fixo do App usa `ToolInvocationService`, com a mesma
identidade de banco e registry do bootstrap. `BeforeExecute` revalida a
autorização após `Create`/`MarkRunning`, antes da tool. Recusa, panic e
cancelamento produzem resultado genérico, sem persistir mensagens do guard.
Falhas de persistência não autorizam a execução; na falha de finalização de
uma recusa, a remoção da linha é best-effort, não uma garantia contra perda
do banco/processo. A qualificação de recuperação continua pendente.

`ExpectedToolGeneration` fixa o registro autorizado: o executor captura tool
e geração atomicamente, recusando substituição com o mesmo nome. Chamadores
existentes sem pin continuam com sua política atual. Não há novo executor,
allowlist de shell ou permissão implícita para profiles. A confirmação do
comando não substitui políticas próprias da tool. Esse corte não publica
comandos de tools nem certifica as identidades agent/job ou ciclo reativo.

- Fecha a issue de unificação de execução e storage de tool calls.
- Inclui a base para a issue de dry-run/teste de tools pelo catálogo.
- Complementa a issue de redesign de skills, mas não redefine skills nesta AEP.
