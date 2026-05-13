# AEP-0048 — Migração de Jobs para Banco de Dados

## Dependências

- **AEP-0046** (Migração de IDs sequenciais para UUIDv7): Deve ser implementada primeiro. Fornece o `UUIDModel` com hook `BeforeCreate` que gera UUIDv7 automaticamente. Todas as PKs das tabelas desta AEP usam esse modelo.
- **AEP-0052** (Multi-user accounts): Jobs, pipelines, runs e eventos nascem sempre com `user_id`.
- **AEP-0047** (Importação e Exportação): O mecanismo compartilhado de importações legadas é reaproveitado para migrar jobs do filesystem para o banco.
- **AEP-0063** (Tool Invocations): Depende desta AEP. Jobs precisam estar no banco antes de referenciar `tool_invocations` como execução técnica das tools.

## Resumo

Migrar o sistema de jobs de filesystem (YAML + JSON + JSONL) para SQLite via GORM, com tabelas normalizadas para pipelines, jobs, triggers, eventos, runs e timeline de runs. O runtime passa a ser DB-only: criação, edição, deleção, scheduler, subscriptions, runs e eventos deixam de depender de arquivos. A migração dos arquivos legados usa o serviço compartilhado de importações pós-login, idempotente e observável, reaproveitando o padrão introduzido em AEP-0047/AEP-0049.

Esta AEP também substitui o gerenciamento via `write_file`/`edit_file` por tools nativas opt-in de jobs e pipelines. A AEP-0063 usa esta base para unificar chamadas de tools em `tool_invocations`.

## Motivação

1. **Consistência**: Todos os demais recursos persistentes do app (conversas, mensagens, credenciais, tasklists) vivem no banco SQLite. Jobs são a única exceção — vivem em arquivos YAML no disco com logs em JSON/JSONL separados. Isso cria dois sistemas de persistência distintos para manter.

2. **Queries**: Com jobs no banco, é possível fazer queries SQL (listar jobs por status, filtrar runs por data/status, buscar eventos por tipo). Hoje isso requer leitura manual de diretórios e parse de dezenas de arquivos individuais.

3. **Atomicidade**: Operações de CRUD no filesystem não são atômicas. Um crash entre "atualizar YAML" e "registrar triggers" pode deixar o sistema em estado inconsistente. Com GORM + SQLite WAL, as operações são transacionais.

4. **LLM Tools**: Hoje o LLM gerencia jobs editando arquivos YAML via tools de filesystem (`write_file`, `edit_file`). Isso é frágil — erros de formatação YAML, paths incorretos, falta de validação. Com o banco, LLM tools dedicadas (`create_job`, `update_job`) fazem validação completa antes de persistir.

5. **Retenção**: Run logs acumulam indefinidamente no disco sem política de limpeza automática. No banco, uma goroutine periódica remove registros mais velhos que 30 dias.

6. **Preparação para AEP-0047**: O sistema de import/export (AEP-0047) precisa de acesso uniforme aos dados. Com jobs no banco, o export pode usar o mesmo pattern de Repository que os demais recursos.

## Estado atual

O sistema de jobs é 100% baseado em filesystem:

| Recurso | Formato | Local |
|---|---|---|
| Definição de job | 1 arquivo YAML por job | `~/.assistente/jobs/<id>.yaml` |
| Run logs | 1 JSON por execução | `~/.assistente/jobs/runs/<jobId>/<timestamp>.json` |
| Event logs | JSONL append-only, 1 por dia | `~/.assistente/jobs/events/<date>.jsonl` |
| Catálogo de tools | YAML auto-gerado (read-only) | `~/.assistente/jobs/catalog.yaml` |

- Hot reload via `fsnotify.Watcher` monitora alterações em YAML
- LLM não tem tools dedicadas para jobs — usa tools de filesystem
- 13 arquivos Go em `internal/jobs/`, 7 com testes
- Manager recebe `BaseDir` (path do diretório) como config principal

## Decisões

### D1 — Migração total para banco (sem dual YAML+banco)

O armazenamento YAML no disco é completamente substituído pelo banco SQLite. Não há modo dual (YAML + banco sincronizados) — a complexidade de manter dois sistemas em sincronia não justifica o benefício.

O file watcher (`internal/jobs/watcher.go`) é removido. O catálogo de tools (`catalog.yaml`) permanece em disco por ser dado derivado (gerado automaticamente, nunca editado pelo usuário).

### D2 — Pipelines e triggers normalizados

Pipelines e triggers passam a ter tabelas próprias:

- `job_pipelines`: agrupa jobs, permite listar pipelines, contar jobs por pipeline e filtrar execuções por pipeline sem depender de strings soltas.
- `job_triggers`: registra cada trigger de forma individual, com tipo, condição, payload e status. Isso evita carregar todos os jobs para saber quais inscrições precisam ser ativadas no scheduler/event bus.

Jobs continuam apontando para uma pipeline opcional via `pipeline_id`. Um job sem pipeline representa uma automação avulsa.

### D3 — Retenção de 30 dias para runs e eventos

Uma goroutine no Manager executa limpeza a cada 24 horas, removendo registros de `job_runs` e `job_events` com `started_at` / `timestamp` anterior a 30 dias. A limpeza também roda no `Start()` do Manager (ao iniciar o app).

### D4 — Event log migra para banco

O event log diário (JSONL) é substituído pela tabela `job_events`. Isso unifica toda a persistência em SQLite e permite queries SQL por tipo, data e job.

### D5 — Slug obrigatório e único

O antigo campo `id` dos jobs (slug humano legível como `fetch-jira-tickets`) vira a coluna `slug` com constraint `UNIQUE NOT NULL`. O PK `id` passa a ser UUIDv7 (via `UUIDModel` da AEP-0046).

O slug é o identificador usado em:
- API Wails (frontend referencia jobs por slug)
- LLM tools (`create_job`, `get_job`, etc. usam slug)
- Eventos inter-job (`on_success`/`on_failure` referenciam por slug)
- Logs de execução (legibilidade humana)

### D6 — Configs complexas em JSON, sem duplicar entidades consultáveis

Campos estruturados são serializados como JSON TEXT no SQLite:

| Coluna | Tipo Go serializado |
|---|---|
| `tags` | `[]string` |
| `inputs` | `map[string]any` |
| `output_config` | `OutputConfig` |
| `events_config` | `EventsConfig` |
| `error_policy` | `ErrorPolicy` |
| `dry_run_config` | `DryRunConfig` |

Leitura e escrita usam `json.Marshal` / `json.Unmarshal` nas funções de conversão Model ↔ Domain.

Triggers, pipelines, runs e eventos não ficam embutidos no JSON do job porque precisam ser listados, filtrados ou limpos independentemente.

### D7 — Tools nativas dedicadas (opt-in)

Tools nativas são criadas para o LLM gerenciar jobs e pipelines diretamente, sem editar arquivos:

| Tool | Descrição |
|---|---|
| `list_job_pipelines` | Lista pipelines com contagem de jobs |
| `get_job_pipeline` | Detalhes de uma pipeline por slug |
| `create_job_pipeline` | Cria pipeline |
| `update_job_pipeline` | Atualiza pipeline |
| `delete_job_pipeline` | Remove pipeline se não houver jobs vinculados |
| `list_jobs` | Lista jobs com filtros (pipeline, tag, enabled) |
| `get_job` | Detalhes completos por slug |
| `create_job` | Cria novo job (validação completa antes de persistir) |
| `update_job` | Atualiza por slug (merge parcial) |
| `delete_job` | Remove por slug |
| `toggle_job` | Ativa/desativa |
| `run_job` | Execução manual |
| `get_job_runs` | Histórico de execuções por slug |
| `get_job_run_events` | Timeline de uma execução |

As tools são registradas como **opt-in** via `RegisterOptIn()` — só aparecem quando o perfil de interação as habilita explicitamente. Isso evita poluir o contexto do LLM em perfis que não usam jobs.

### D8 — Importação legada pós-login, idempotente e observável

Na primeira sessão autenticada após a atualização, o serviço compartilhado de importações legadas detecta se existem arquivos em `~/.assistente/jobs/` e se o usuário ainda não recebeu a importação de jobs. Se sim:

1. Carrega pipelines implícitas a partir dos campos existentes e cria `job_pipelines` quando necessário.
2. Carrega todos os YAML e insere como `jobs` (slug = antigo id).
3. Extrai triggers do YAML e insere em `job_triggers`.
4. Carrega run logs JSON e insere em `job_runs`.
5. Carrega event logs JSONL e insere em `job_events` ou `job_run_events`, conforme houver `run_id`.
6. Registra resultado, erros e contadores em uma tabela/estrutura de importação observável compartilhada com MCP e outras migrações legadas.

A migração é idempotente por usuário e por recurso. Se parte da importação já existe, a execução seguinte não duplica dados. Não há fallback runtime para filesystem depois da migração: se arquivos antigos não forem importados, eles não continuam alimentando o sistema de jobs.

### D9 — Repository pattern

A persistência é abstraída por uma interface `Repository`:

```go
type Repository interface {
    ListPipelines() ([]Pipeline, error)
    GetPipeline(slug string) (*Pipeline, error)
    SavePipeline(pipeline *Pipeline) error
    DeletePipeline(slug string) error
    ListJobs(filter JobFilter) ([]Job, error)
    GetJob(slug string) (*Job, error)
    GetJobByID(id string) (*Job, error)
    SaveJob(job *Job) error
    DeleteJob(slug string) error
    ListTriggers(jobID string) ([]Trigger, error)
    SaveTriggers(jobID string, triggers []Trigger) error
    LogRun(rl *RunLog) error
    GetRuns(jobID string, limit int) ([]RunLog, error)
    GetRun(jobID, runID string) (*RunLog, error)
    LogEvent(entry *EventEntry) error
    LogRunEvent(entry *RunEvent) error
    GetRunEvents(runID string) ([]RunEvent, error)
    GetEvents(date string) ([]EventEntry, error)
    CleanOldRuns(maxAge time.Duration) (int, error)
    CleanOldEvents(maxAge time.Duration) (int, error)
}
```

Implementação concreta: `DBRepository` que recebe `*gorm.DB`. O Manager recebe a interface (testável com mocks).

### D10 — Frontend inalterado

A API Wails exposta ao frontend (`GetJobs`, `SaveJob`, `RunJob`, `GetJobRuns`, etc.) mantém as mesmas assinaturas e tipos de retorno. O frontend não percebe a mudança de backing store.

### D11 — Duration muda de string para inteiro

O campo `Duration` do `RunLog` muda de string (`"1.5s"`) para `DurationMs int64` (1500). Isso facilita queries de performance no banco e elimina parsing de duração no frontend.

Na migração, strings existentes são parseadas via `time.ParseDuration()` e convertidas para milissegundos.

### D12 — RunID muda para UUIDv7

O antigo `RunID` (formato `run_<unix_nano>`) é substituído pelo PK `id` da tabela `job_runs` (UUIDv7 auto-gerado). O campo `RunID` do struct `RunLog` é renomeado para `ID`.

## Tabelas

### job_pipelines

| Coluna | Tipo | Constraints | Notas |
|---|---|---|---|
| `id` | TEXT | PK | UUIDv7 via `UUIDModel.BeforeCreate` |
| `user_id` | TEXT | FK→users.id, NOT NULL, INDEX | Dono da pipeline |
| `slug` | TEXT | NOT NULL | Único por usuário |
| `name` | TEXT | NOT NULL | Nome legível |
| `description` | TEXT | | Opcional |
| `enabled` | BOOL | NOT NULL, DEFAULT true | Desativa os jobs da pipeline sem apagar configuração |
| `metadata` | TEXT | | JSON leve para dados de UI/futuro |
| `created_at` | DATETIME | | |
| `updated_at` | DATETIME | | |

Índice único: `(user_id, slug)`.

### jobs

| Coluna | Tipo | Constraints | Notas |
|---|---|---|---|
| `id` | TEXT | PK | UUIDv7 via `UUIDModel.BeforeCreate` |
| `user_id` | TEXT | FK→users.id, NOT NULL, INDEX | Dono do job |
| `pipeline_id` | TEXT | FK→job_pipelines.id, INDEX | Opcional |
| `slug` | TEXT | NOT NULL | Ex: `fetch-jira-tickets`. Único por usuário |
| `name` | TEXT | NOT NULL | Nome legível para exibição |
| `description` | TEXT | | Opcional |
| `enabled` | BOOL | NOT NULL, DEFAULT true | |
| `tags` | TEXT | | JSON array: `["tag1","tag2"]` |
| `tool_catalog_id` | TEXT | FK→tool_catalog.id, INDEX | Fonte canônica da tool a executar |
| `inputs` | TEXT | | JSON object: inputs fixos ou templates |
| `output_config` | TEXT | | JSON: `OutputConfig` (schema + map) |
| `events_config` | TEXT | | JSON: `EventsConfig` (on_success, on_failure, etc.) |
| `error_policy` | TEXT | | JSON: `ErrorPolicy` (retry, backoff, notify) |
| `max_runs_per_hour` | INT | DEFAULT 0 | 0 = sem limite |
| `dry_run_config` | TEXT | | JSON: `DryRunConfig` (enabled + mock_output) |
| `created_by` | TEXT | | `"user"` ou `"system"` |
| `created_at` | DATETIME | | |
| `updated_at` | DATETIME | | |

Índice único: `(user_id, slug)`.

### job_triggers

| Coluna | Tipo | Constraints | Notas |
|---|---|---|---|
| `id` | TEXT | PK | UUIDv7 |
| `user_id` | TEXT | FK→users.id, NOT NULL, INDEX | Dono do trigger |
| `job_id` | TEXT | FK→jobs.id, NOT NULL, INDEX | Job disparado |
| `type` | TEXT | NOT NULL, INDEX | `cron`/`interval`/`event`/`hotkey`/`webhook`/`manual` |
| `enabled` | BOOL | NOT NULL, DEFAULT true | Permite desativar um trigger sem desativar o job |
| `expression` | TEXT | | Cron, intervalo, nome de evento ou hotkey, conforme `type` |
| `config` | TEXT | | JSON com campos específicos do tipo |
| `last_triggered_at` | DATETIME | | Ajuda scheduler e UI |
| `created_at` | DATETIME | | |
| `updated_at` | DATETIME | | |

### job_runs

| Coluna | Tipo | Constraints | Notas |
|---|---|---|---|
| `id` | TEXT | PK | UUIDv7 (substitui `run_<unix_nano>`) |
| `user_id` | TEXT | FK→users.id, NOT NULL, INDEX | Dono da execução |
| `job_id` | TEXT | FK→jobs.id, NOT NULL, INDEX | |
| `trigger_id` | TEXT | FK→job_triggers.id, INDEX | Pode ser nulo em execução manual |
| `tool_catalog_id` | TEXT | FK→tool_catalog.id, INDEX | Fonte canônica da tool executada |
| `trigger_type` | TEXT | | `cron`/`interval`/`event`/`hotkey`/`manual`/`webhook` |
| `trigger_at` | DATETIME | | Quando o trigger disparou |
| `trigger_event` | TEXT | | Nome do evento (se trigger=event) |
| `trigger_data` | TEXT | | JSON: dados do evento trigger |
| `status` | TEXT | NOT NULL, INDEX | `completed`/`failed`/`retrying`/`skipped` |
| `started_at` | DATETIME | NOT NULL, INDEX | Para queries de retenção e ordenação |
| `completed_at` | DATETIME | | |
| `duration_ms` | INT | | Duração em milissegundos |
| `resolved_inputs` | TEXT | | JSON: inputs após resolução de templates |
| `output` | TEXT | | JSON: resultado da tool |
| `output_size` | INT | | Tamanho do output em bytes |
| `error` | TEXT | | Mensagem de erro (se falhou) |
| `retry_count` | INT | DEFAULT 0 | |
| `events_emitted` | TEXT | | JSON array: nomes de eventos emitidos |
| `is_dry_run` | BOOL | DEFAULT false | |
| `created_at` | DATETIME | | |

### job_events

| Coluna | Tipo | Constraints | Notas |
|---|---|---|---|
| `id` | TEXT | PK | UUIDv7 |
| `user_id` | TEXT | FK→users.id, NOT NULL, INDEX | Dono do evento |
| `job_id` | TEXT | FK→jobs.id, INDEX | |
| `timestamp` | DATETIME | NOT NULL, INDEX | Momento do evento |
| `type` | TEXT | NOT NULL, INDEX | `triggered`/`completed`/`failed`/`event_emitted`/`event_received` |
| `event` | TEXT | INDEX | Nome do evento (se aplicável) |
| `message` | TEXT | | Descrição legível |
| `data` | TEXT | | JSON: dados contextuais |
| `created_at` | DATETIME | | |

### job_run_events

| Coluna | Tipo | Constraints | Notas |
|---|---|---|---|
| `id` | TEXT | PK | UUIDv7 |
| `user_id` | TEXT | FK→users.id, NOT NULL, INDEX | Dono do evento |
| `job_run_id` | TEXT | FK→job_runs.id, NOT NULL, INDEX | Execução relacionada |
| `sequence` | INT | NOT NULL | Ordem estável dentro do run |
| `timestamp` | DATETIME | NOT NULL, INDEX | Momento do evento |
| `type` | TEXT | NOT NULL, INDEX | `queued`/`started`/`tool_started`/`tool_finished`/`completed`/`failed` |
| `message` | TEXT | | Descrição legível |
| `data` | TEXT | | JSON: dados técnicos do passo |
| `created_at` | DATETIME | | |

## Mapeamento de dados: filesystem → banco

### Job (YAML → tabela `jobs`)

| Campo YAML | Coluna DB | Transformação |
|---|---|---|
| `id` | `slug` | Renomeado. PK passa a ser UUIDv7 auto-gerado |
| `name` | `name` | Direto |
| `description` | `description` | Direto |
| `enabled` | `enabled` | Direto |
| `pipeline` | `pipeline_id` | Criar/buscar `job_pipelines` por slug e vincular |
| `tags` | `tags` | `[]string` → JSON array |
| `triggers` | `job_triggers` | Cada trigger vira uma linha em `job_triggers` |
| `tool` | `tool_catalog_id` | Resolver nome/slug no `tool_catalog` |
| `inputs` | `inputs` | `map[string]any` → JSON object |
| `output` | `output_config` | `OutputConfig` → JSON |
| `events` | `events_config` | `EventsConfig` → JSON |
| `error_policy` | `error_policy` | `ErrorPolicy` → JSON |
| `max_runs_per_hour` | `max_runs_per_hour` | Direto |
| `dry_run` | `dry_run_config` | `DryRunConfig` → JSON |
| `metadata.created_at` | `created_at` | String ISO → `time.Time` |
| `metadata.created_by` | `created_by` | Direto |
| `metadata.updated_at` | `updated_at` | String ISO → `time.Time` |

### RunLog (JSON → tabela `job_runs`)

| Campo JSON | Coluna DB | Transformação |
|---|---|---|
| `run_id` | `id` | Descartado — novo UUIDv7 gerado |
| `job_id` | `job_id` | Slug → resolvido para UUID do job correspondente |
| `tool_name` | `tool_catalog_id` | Resolver nome/slug no `tool_catalog` |
| `trigger.type` | `trigger_type` | Flatten de struct aninhada |
| `trigger.at` | `trigger_at` | Flatten |
| `trigger.event` | `trigger_event` | Flatten |
| `trigger.data` | `trigger_data` | Flatten → JSON |
| `status` | `status` | Direto |
| `started_at` | `started_at` | Direto |
| `completed_at` | `completed_at` | Direto |
| `duration` | `duration_ms` | Parsear string (`"1.5s"`) → int ms (1500) |
| `resolved_inputs` | `resolved_inputs` | `map` → JSON |
| `output` | `output` | `map` → JSON |
| `output_size` | `output_size` | Direto |
| `error` | `error` | Direto |
| `retry_count` | `retry_count` | Direto |
| `events_emitted` | `events_emitted` | `[]string` → JSON array |
| `is_dry_run` | `is_dry_run` | Direto |

### EventEntry (JSONL → tabela `job_events`)

| Campo JSONL | Coluna DB | Transformação |
|---|---|---|
| — | `id` | Novo UUIDv7 gerado |
| `job_id` | `job_id` | Slug → resolvido para UUID do job correspondente |
| `timestamp` | `timestamp` | Direto |
| `type` | `type` | Direto |
| `event` | `event` | Direto |
| `message` | `message` | Direto |
| `data` | `data` | `map` → JSON |

Eventos JSONL que tiverem associação inequívoca com um `run_id` também geram entradas em `job_run_events`, preservando a timeline da execução. Eventos globais ou sem run permanecem em `job_events`.

## Fases

### Fase 1 — Models GORM + AutoMigrate

1. Criar `internal/database/models_jobs.go` com models GORM para `job_pipelines`, `jobs`, `job_triggers`, `job_runs`, `job_events` e `job_run_events`.
2. Funções de conversão entre models e domínio (`Pipeline`, `Job`, `Trigger`, `RunLog`, `EventEntry`, `RunEvent`).
3. Adicionar todos os models ao `AutoMigrate` em `internal/database/database.go`.

### Fase 2 — Repository layer

4. Criar `internal/jobs/repository.go` com interface `Repository` (D9)
5. Implementar `DBRepository` que recebe `*gorm.DB`
6. Testes: CRUD de pipelines, jobs, triggers, runs, events e run events, limpeza por idade

### Fase 3 — Migrar Manager para usar Repository

7. Alterar `ManagerConfig`: adicionar campo `Repository` (manter `BaseDir` apenas para catálogo)
8. Reescrever `Start()`: carregar jobs do DB, remover inicialização do Watcher
9. Reescrever `SaveJob()`, `DeleteJob()`, `ToggleJob()`: usar Repository
10. Reescrever `GetJobRuns()`, `GetJobEvents()`, `ReplayRun()`: usar Repository

### Fase 4 — Migrar Executor para usar Repository

11. `Execute()` chama `Repository.LogRun()` e `Repository.LogEvent()` em vez do file logger

### Fase 5 — Retenção automática

12. Goroutine no Manager: a cada 24h, `Repository.CleanOldRuns(30 dias)` e `Repository.CleanOldEvents(30 dias)`
13. Executar limpeza também no `Start()` (ao iniciar o app)

### Fase 6 — Tools nativas

14. Criar tools nativas em `internal/tools/` para pipelines e jobs: `list_job_pipelines`, `get_job_pipeline`, `create_job_pipeline`, `update_job_pipeline`, `delete_job_pipeline`, `list_jobs`, `get_job`, `create_job`, `update_job`, `delete_job`, `toggle_job`, `run_job`, `get_job_runs`, `get_job_run_events`.
15. Registrar como opt-in em `initToolRegistry()`

### Fase 7 — Importação legada filesystem → banco

16. Criar `internal/jobs/migration.go` com lógica de importação legada idempotente (D8), integrada ao serviço compartilhado de importações.
17. Resolver slugs → UUIDs: ao importar runs/events, buscar job por slug para obter UUID
18. Registrar contadores/erros da importação para UI/logs e não depender de renomear diretórios como fonte de verdade.
19. Chamar importação pós-login, antes de iniciar scheduler/subscriptions do usuário.

### Fase 8 — Remoção de código filesystem

20. Remover `internal/jobs/watcher.go` (file watcher)
21. Remover `internal/jobs/logger.go` (file-based logging)
22. Remover `marshalJobYAML()`, `LoadAllFromDir()`, campo `FilePath` do struct `Job`
23. Simplificar `parser.go` — manter validação, remover I/O de disco
24. Manter `catalog.go` (catálogo continua em disco — dado derivado)

### Fase 9 — Testes

25. Testes Repository: CRUD pipelines, jobs, triggers, runs, events, run events, limpeza por idade, roundtrip JSON
26. Testes Manager: Start, Save, Delete, Toggle com DB
27. Testes migração: YAML→DB, triggers→DB, JSON runs→DB, JSONL events→DB, idempotência pós-login
28. Testes tools nativas: create/update/delete/list/run para pipelines e jobs
29. Atualizar testes existentes (`logger_test.go` → `repository_test.go`)

## Arquivos afetados

### Novos

| Arquivo | Descrição |
|---|---|
| `internal/database/models_jobs.go` | Models GORM de pipelines, jobs, triggers, runs e eventos |
| `internal/jobs/repository.go` | Interface `Repository` + `DBRepository` |
| `internal/jobs/repository_test.go` | Testes do repository |
| `internal/jobs/migration.go` | Importação legada filesystem → DB |
| `internal/jobs/migration_test.go` | Testes da migração |
| `internal/tools/job_tools.go` | Tools nativas para jobs e pipelines |

### Modificados

| Arquivo | Mudança |
|---|---|
| `internal/jobs/manager.go` | Refatorar para usar Repository em vez de filesystem |
| `internal/jobs/executor.go` | Logging via Repository em vez de file logger |
| `internal/jobs/types.go` | `RunID` → `ID`, `Duration string` → `DurationMs int64`, adicionar `Slug` |
| `internal/database/database.go` | Adicionar models de jobs ao `AutoMigrate` |
| `internal/app/app_tool_registry.go` | Registrar LLM tools de jobs como opt-in |
| `controllers/jobs_controller.go` | Ajustes mínimos de inicialização |

### Removidos

| Arquivo | Motivo |
|---|---|
| `internal/jobs/watcher.go` | File watcher obsoleto |
| `internal/jobs/logger.go` | File-based logging substituído por Repository |

### Sem alteração

- **Frontend**: mesma API Wails, mesmos tipos. Store, componentes e páginas inalterados (D10).

## Riscos

| # | Risco | Probabilidade | Impacto | Mitigação |
|---|---|---|---|---|
| R1 | Perda de dados na migração filesystem → DB | Baixa | Alto | Importação transacional, idempotente e observável; arquivos legados não são runtime fallback |
| R2 | Performance de queries com muitos runs | Baixa | Médio | Índices em `job_id` + `started_at` + `status`; retenção 30 dias limita volume (~86k rows máx com 10 jobs a cada 5 min) |
| R3 | Catálogo de tools fica órfão sem watcher | Baixa | Baixo | Catálogo é regenerado no `Start()` e sob demanda; sem mudança funcional |
| R4 | Tools nativas de jobs poluem contexto | Média | Baixo | Tools são opt-in; perfil escolhe quais habilitar |
| R5 | Serialização JSON de configs complexas perde tipo | Média | Médio | Testes de roundtrip (save → load → compare) para cada tipo |
| R6 | Migração falha parcialmente (parte importada, parte não) | Baixa | Médio | Migração em transação; rollback se qualquer etapa falhar |

## Critérios de aceitação

1. **CRUD completo**: criar, listar, buscar, atualizar, deletar e toggle de pipelines e jobs funciona via banco
2. **Run logs no banco**: execução de job persiste `RunLog` na tabela `job_runs` com todos os campos
3. **Event logs no banco**: eventos são persistidos em `job_events`/`job_run_events` em vez de JSONL
4. **Retenção**: runs e events mais velhos que 30 dias são removidos automaticamente
5. **Migração filesystem**: jobs YAML existentes são importados para o banco após login, sem duplicação em reexecuções
6. **Sem fallback filesystem**: runtime de jobs lê e escreve somente no banco após esta AEP
7. **Tools nativas**: tools opt-in disponíveis para o LLM gerenciar pipelines e jobs
8. **Frontend inalterado**: mesma API Wails, sem mudanças em stores/componentes
9. **Roundtrip JSON**: configs complexas (triggers, error_policy, etc.) sobrevivem save→load sem perda
10. **Testes**: repository, manager, migração e LLM tools cobertos por testes Go
11. **File watcher removido**: `internal/jobs/watcher.go` e `internal/jobs/logger.go` eliminados
12. **Catálogo preservado**: `catalog.yaml` continua sendo gerado em disco como dado derivado
