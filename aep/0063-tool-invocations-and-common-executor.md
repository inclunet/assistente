# AEP-0063 — Tool Invocations e Executor Comum

**Status:** Done

## Dependências

- **AEP-0039** (Tool Calling Revamp): base conceitual para tool calling, eventos e execução estruturada.
- **AEP-0046** (Migração de IDs sequenciais para UUIDv7): fornece o padrão de IDs UUIDv7 usado por `tool_invocations.id`.
- **AEP-0048** (Jobs Database Migration): jobs precisam estar no banco para referenciar execuções de tools sem depender de arquivos.
- **AEP-0049** (MCP Database Migration): `tool_catalog` é a fonte canônica de tools nativas e MCP.
- **AEP-0052** (Multi-user accounts): invocações são sempre associadas a `user_id`.

## Resumo

Criar uma camada única de execução e persistência para chamadas de tools: `ToolInvocationService` + tabela `tool_invocations`. O chat, jobs, testes/dry-run e integrações MCP passam a usar o mesmo executor, o mesmo modelo de status e a mesma trilha técnica de execução.

Resultados de tools deixam de ser armazenados como mensagens de chat ou logs em arquivos. Mensagens continuam representando a conversa; `tool_invocations` representa o histórico técnico, efêmero e sujeito a retenção.

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
status/duração/input/output/erro; se o catálogo não puder resolver a tool, a
execução continua em modo best-effort e o chamador usa o fallback documentado.

### D4 — Origens explícitas

Cada invocação informa de onde veio:

| `origin_type` | `origin_id` |
|---|---|
| `chat` | `chat_messages.id` ou execução do agentic loop |
| `job_run` | `job_runs.id` |
| `tool_catalog` | teste manual do catálogo |
| `system` | automação interna |

`origin_id` é string para não acoplar a tabela a uma única FK. Quando houver origem conhecida e estável, o código valida a existência antes de executar.

### D5 — Logs efêmeros e retenção própria

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
| `queued_at` | DATETIME | NOT NULL, INDEX | Momento em que a invocação entrou na fila |
| `started_at` | DATETIME | INDEX | Início real da execução; nulo enquanto `queued` |
| `completed_at` | DATETIME | | Fim |
| `duration_ms` | INT | | Duração entre `started_at` e `completed_at`, quando ambos existem |
| `metadata` | TEXT | | JSON pequeno para adapter, versão, policy |
| `created_at` | DATETIME | | |
| `updated_at` | DATETIME | | |

Índices:

- `(user_id, origin_type, origin_id)`
- `(user_id, tool_catalog_id, started_at)`
- `(user_id, status, queued_at)`
- `(user_id, dry_run, queued_at)`

## Fluxo sem diagrama

1. Um fluxo pede uma execução: chat, job, dry-run ou sistema.
2. O chamador resolve ou informa o `tool_catalog_id`.
3. `ToolInvocationService` cria a invocação como `queued` e preenche `queued_at`.
4. O serviço muda para `running`, preenche `started_at`, aplica timeout/política e chama o adapter correto.
5. O adapter executa tool nativa ou MCP.
6. O serviço grava `succeeded`, `failed`, `timed_out` ou `cancelled`, com duração, output e erro.
7. O chamador recebe um resultado normalizado e decide como continuar o fluxo.

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
  persistível de catálogo cria `tool_invocations`; catálogo ausente/não
  resolvido mantém execução best-effort e fallback sem afirmar persistência.
- [x] `tool_catalog_id` é canônico e não duplica `tool_name`.
- [x] Jobs mantêm estado em `job_runs` e execução técnica em invocações.
- [x] Chat não grava novos resultados brutos como mensagens no caminho feliz.
- [x] Dry-run e teste de catálogo usam o executor compartilhado.
- [x] Tools internas, MCP bridge e MCP nativo têm representação consistente.
- [x] Retenção/limpeza respeita a política por origem da
  [AEP-0074-B — Compactação e Retenção do Banco de Dados](0074-database-compaction-and-retention.md).
- [x] Testes cobrem sucesso, falha, timeout, dry-run, chat e `job_run`.

Evidências: `internal/toolinvocations/{repository,service}_test.go`,
`internal/agent/service_tool_calls_persistence_test.go`,
`internal/jobs/executor_toolinvocations_test.go`,
`manager_toolinvocations_test.go` e `internal/wailsapi/jobs_dryrun_test.go`.

## Plano de Transição e Compatibilidade (Issue #127)

Esta seção registra como o critério de aceite "há migração/compatibilidade para dados
existentes OU um plano explícito de transição" do issue #127 foi atendido. O núcleo do
AEP-0063 está implementado. A transição posterior do L3 foi concluída pela AEP-0078:
novas mensagens não gravam `tool_calls` no caminho feliz, enquanto dados históricos
continuam legíveis como fallback.

### O que já migrou para `tool_invocations`

Já usam o executor comum (`internal/toolinvocations.Service`) e persistem em `tool_invocations`:

- **Chat / agentic loop** (`internal/agent/service.go`): cada tool call passa por
  `Service.Execute`/`ExecuteAll` com `origin_type = chat` e
  `origin_id = turnID`. Quando o catálogo resolve uma entrada persistível, o
  resultado técnico fica em `tool_invocations`; caso contrário, `Persisted=false`
  aciona o fallback best-effort descrito em L1.
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
- **Export/Import** (`internal/portability/service.go`): hidratação reconstrói os resultados de
  tools a partir de `tool_invocations`, sem depender exclusivamente de mensagens.

### O que permanece em armazenamento legado e por quê

Três mecanismos legados continuam ativos de forma **intencional**. Nenhum é removido neste
ciclo (issue #127) por serem de alto risco; cada um tem função de compatibilidade ou de
domínio distinta da trilha técnica de `tool_invocations`.

#### L1 — Fallback `role=tool` no chat

- **Onde**: `internal/agent/service.go` (`RunAgenticLoop` e `persistNativeMCPCalls`),
  via `msgRepo.AddToolResultMessage`.
- **Quando dispara**: somente quando a persistência técnica não pôde ser usada como fonte
  para hidratação — isto é, quando `tool_invocations` não persistiu (`Persisted = false`,
  ex.: catálogo indisponível) **ou** quando a mensagem assistant `tool_calls` falhou ao salvar
  (`assistantToolCallsSaved = false`). No caminho feliz, **não** são criadas mensagens
  `role=tool` (ver `TestRunAgenticLoop_ToolCalls_SuppressesRoleToolOnSuccessfulPersistence`).
- **Por que permanece**: é a rede de segurança que evita órfãos no histórico/exportação quando
  a hidratação a partir de `tool_invocations` + assistant `tool_calls` não é possível. Remover
  agora poderia perder rastreabilidade em falhas transitórias de DB.
- **Status**: compatibilidade ativa, acionada apenas em caminho de exceção.

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

- **Onde permanece**: dados históricos em `chat_messages.tool_calls` e caminhos de
  compatibilidade de leitura.
- **Estado atual**: o agentic loop não grava L3 no caminho feliz
  (`internal/agent/agentic_loop.go`); timeline, exportação e sumarização hidratam
  chamadas por `tool_invocations`.
- **Compatibilidade**: mensagens antigas continuam legíveis e exportáveis. O fallback
  não autoriza novos consumidores nem novas escritas de L3.
- **Status**: deprecação funcional concluída pela AEP-0078; coluna legada preservada
  para leitura.

### Plano e critérios para deprecar cada legado

| Legado | Ação | Critério para deprecar |
|---|---|---|
| L1 `role=tool` | Reduzir gradualmente o acionamento. | Quando a hidratação por `tool_invocations` + assistant `tool_calls` cobrir 100% dos caminhos de leitura (UI, export, sumarização) **e** métricas mostrarem 0 acionamentos do fallback em produção por um período de observação. Só então remover `AddToolResultMessage` do caminho de chat. |
| L2 `job_run_events` | **Não deprecar.** | Permanece como timeline operacional. Só seria reavaliado se a UI de jobs passar a derivar a timeline inteiramente de `tool_invocations` + `job_runs`, o que não é objetivo do issue #127. |
| L3 `tool_calls` JSON em mensagens | Não gravar em mensagens novas; manter leitura legada. | Concluído pela AEP-0078; remoção física da coluna exigiria migração separada. |

### Compatibilidade com dados existentes

- Não há backfill destrutivo. Mensagens `role=tool` e `tool_calls` históricas continuam
  legíveis; a hidratação prioriza `tool_invocations` quando presente e cai para o conteúdo de
  mensagens quando não há registro técnico (dados anteriores à introdução da tabela).
- `tool_invocations` é log efêmero (D5): a ausência de registros antigos é esperada e tratada
  pela leitura como "sem trilha técnica", sem quebrar a exibição da conversa/job.

### Critérios de aceite do issue #127 — mapeamento

| Critério do issue | Situação | Evidência |
|---|---|---|
| Executor comum em chat e jobs | Atendido | `internal/toolinvocations`, `internal/agent/service.go`, `internal/jobs/executor.go` |
| Invocações em tabela própria com vínculo à origem | Atendido | `database.ToolInvocation`, `origin_type`/`origin_id` |
| Jobs referenciam armazenamento comum (não dependem de log isolado) | Atendido | `executor.go` (`origin_id = run.RunID`) + `executor_toolinvocations_test.go`; `job_run_events` documentado como timeline operacional (L2) |
| Tool results de chat não exclusivamente como mensagens | Atendido | Hidratação via `tool_invocations`; `role=tool` só como fallback (L1) |
| MCP e tools internas representadas de forma consistente | Atendido | `Execute`/`Record` unificados; `metadata.external` para MCP nativo |
| Migração/compatibilidade OU plano explícito de transição | **Atendido por esta seção** | Plano L1/L2/L3 + critérios de deprecação |
| Testes cobrindo chat, job e dry-run no mesmo executor | Atendido | `service_tool_calls_persistence_test.go`, `executor_toolinvocations_test.go`, `manager_toolinvocations_test.go`, `internal/wailsapi/jobs_dryrun_test.go` |

## Relação com issues

- Fecha a issue de unificação de execução e storage de tool calls.
- Inclui a base para a issue de dry-run/teste de tools pelo catálogo.
- Complementa a issue de redesign de skills, mas não redefine skills nesta AEP.
