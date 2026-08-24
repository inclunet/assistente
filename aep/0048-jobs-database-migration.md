# AEP-0048 — Migração de Jobs para Banco de Dados

**Status:** Done — runtime DB-only, importação legada, tools nativas e observabilidade do adendo entregues

## Dependências

- **AEP-0046** (Migração de IDs sequenciais para UUIDv7): Deve ser implementada primeiro. Fornece o `UUIDModel` com hook `BeforeCreate` que gera UUIDv7 automaticamente. Todas as PKs das tabelas desta AEP usam esse modelo.
- **AEP-0052** (Multi-user accounts): Jobs, pipelines, runs e eventos nascem sempre com `user_id`.
- **AEP-0047** (Importação e Exportação): O mecanismo compartilhado de importações legadas é reaproveitado para migrar jobs do filesystem para o banco.

## Relacionadas

- **AEP-0063** (Tool Invocations): é precedida por esta AEP. Jobs precisam estar no banco antes de referenciar `tool_invocations` como execução técnica das tools.

## Resumo

Migrar o sistema de jobs de filesystem (YAML + JSON + JSONL) para SQLite via GORM, com tabelas normalizadas para tags globais, pipelines, jobs, triggers, eventos, runs e timeline de runs. O runtime passa a ser DB-only: criação, edição, deleção, scheduler, subscriptions, runs e eventos deixam de depender de arquivos. A migração dos arquivos legados usa o serviço compartilhado de importações pós-login, idempotente e observável, reaproveitando o padrão introduzido em AEP-0047/AEP-0049.

Esta AEP também substitui o gerenciamento via `write_file`/`edit_file` por tools nativas opt-in de jobs e pipelines. A AEP-0063 usa esta base para unificar chamadas de tools em `tool_invocations`.

## Motivação

1. **Consistência**: Todos os demais recursos persistentes do app (conversas, mensagens, credenciais, tasklists) vivem no banco SQLite. Jobs são a única exceção — vivem em arquivos YAML no disco com logs em JSON/JSONL separados. Isso cria dois sistemas de persistência distintos para manter.

2. **Queries**: Com jobs no banco, é possível fazer queries SQL (listar jobs por status, filtrar runs por data/status, buscar eventos por tipo). Hoje isso requer leitura manual de diretórios e parse de dezenas de arquivos individuais.

3. **Atomicidade**: Operações de CRUD no filesystem não são atômicas. Um crash entre "atualizar YAML" e "registrar triggers" pode deixar o sistema em estado inconsistente. Com GORM + SQLite WAL, as operações são transacionais.

4. **LLM Tools**: Hoje o LLM gerencia jobs editando arquivos YAML via tools de filesystem (`write_file`, `edit_file`). Isso é frágil — erros de formatação YAML, paths incorretos, falta de validação. Com o banco, tools nativas compostas para jobs e pipelines fazem validação completa antes de persistir, sem multiplicar o catálogo com uma tool por verbo CRUD.

5. **Retenção**: Run logs acumulavam indefinidamente no disco. No contrato
   vigente, uma goroutine periódica aplica `job_retention_hours` (padrão 24h)
   e o teto `runs_per_job_keep` por job.

6. **Preparação para AEP-0047**: O sistema de import/export (AEP-0047) precisa de acesso uniforme aos dados. Com jobs no banco, o export pode usar o mesmo pattern de Repository que os demais recursos.

## Estado anterior à migração

Antes desta AEP, o sistema de jobs era 100% baseado em filesystem:

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

## Estado implementado

- Models e `DBRepository` vivem em `internal/database/models_jobs.go` e
  `internal/jobs/repository.go`.
- `Manager` exige `Repository`; `BaseDir` só identifica a fonte da importação
  legada. Não há watcher nem logger filesystem no runtime.
- CRUD, scheduler, executor, retenção e observabilidade usam SQLite.
- A importação YAML idempotente fica em `internal/jobs/migration.go`.
- As tools compostas `job`/`job_pipeline` ficam em `internal/tools/job/`.
- Repository, migração, manager, executor e tools têm testes focados nos
  respectivos arquivos `*_test.go`.

## Decisões

### D1 — Migração total para banco (sem dual YAML+banco)

O armazenamento YAML no disco é completamente substituído pelo banco SQLite. Não há modo dual (YAML + banco sincronizados) — a complexidade de manter dois sistemas em sincronia não justifica o benefício.

O file watcher (`internal/jobs/watcher.go`) é removido. O catálogo de tools deixa de ser materializado em `catalog.yaml`: a UI e os jobs consultam o registry/catálogo persistente em tempo real, evitando qualquer escrita nova no diretório legado.

### D2 — Pipelines e triggers normalizados

Pipelines e triggers passam a ter tabelas próprias:

- `job_pipelines`: agrupa jobs, permite listar pipelines, contar jobs por pipeline e filtrar execuções por pipeline sem depender de strings soltas.
- `job_triggers`: registra cada trigger de forma individual, com tipo, condição, payload e status. Isso evita carregar todos os jobs para saber quais inscrições precisam ser ativadas no scheduler/event bus.

Jobs continuam apontando para uma pipeline opcional via `pipeline_id`. Um job sem pipeline representa uma automação avulsa.

### D3 — Retenção configurável para runs e eventos

Uma goroutine no Manager executa limpeza a cada 24 horas, removendo registros
de `job_runs`, `job_events` e `job_run_events` com `started_at` /
`occurred_at` anterior à janela `maintenance.job_retention_hours`. O padrão é
24 horas, definido em `internal/config/config.go`; a política também limita a
quantidade por job com `runs_per_job_keep` (padrão 200). A limpeza roda ainda
no `Start()` do Manager.

A interface de persistência expõe limpeza separada para runs, eventos de domínio e timeline operacional (`CleanOldRuns`, `CleanOldEvents`, `CleanOldRunEvents`) para deixar explícito que as três tabelas participam da retenção.

### D4 — Separação entre eventos de domínio e timeline operacional

O event log diário (JSONL) é dividido conforme a responsabilidade:

- `job_events`: eventos de domínio do event bus, como eventos emitidos ou recebidos por jobs.
- `job_run_events`: timeline operacional de uma execução, como início, retry, conclusão, falha e vínculo com eventos de domínio.

Estados que já estão em `job_runs` não são duplicados como linhas de `job_events`. Eventos técnicos de tool (`tool_started`, `tool_finished`, payload bruto, erro técnico da chamada) pertencem à AEP-0063 em `tool_invocations`.

### D5 — Slug obrigatório e único

O antigo campo `id` dos jobs (slug humano legível como `fetch-jira-tickets`) vira a coluna `slug` com constraint `UNIQUE NOT NULL`. O PK `id` passa a ser UUIDv7 (via `UUIDModel` da AEP-0046).

O slug é o identificador usado em:
- API Wails (frontend referencia jobs por slug)
- LLM tools compostas (`job`, `job_run`, etc. usam slug)
- Eventos inter-job (`on_success`/`on_failure` referenciam por slug)
- Logs de execução (legibilidade humana)

### D6 — Tags globais normalizadas

Tags não ficam como JSON/TEXT em `jobs`. Elas viram recurso compartilhado do app para permitir marcar jobs, conversas e outros recursos futuros com a mesma taxonomia.

O desenho inicial cria:

- `tags`: catálogo de tags por usuário, com slug único, nome, cor/descrição opcionais.
- `tag_assignments`: associação de uma tag a um recurso, com `resource_type` e `resource_id`.

Jobs usam `tag_assignments(resource_type = 'job')`. Conversas poderão usar `resource_type = 'conversation'` sem recriar outro sistema de tags.

### D7 — Configs complexas em JSON, sem duplicar entidades consultáveis

Campos estruturados específicos do job são serializados como JSON TEXT no SQLite:

| Coluna | Tipo Go serializado |
|---|---|
| `inputs` | `map[string]any` |
| `output_config` | `OutputConfig` |
| `events_config` | `EventsConfig` |
| `error_policy` | `ErrorPolicy` |
| `dry_run_config` | `DryRunConfig` |

Leitura e escrita usam `json.Marshal` / `json.Unmarshal` nas funções de conversão Model ↔ Domain.

Tags, triggers, pipelines, runs e eventos não ficam embutidos no JSON do job porque precisam ser listados, filtrados, compartilhados ou limpos independentemente.

### D8 — Tools nativas compostas (opt-in)

As tools de jobs seguem o padrão das tools de tasklists (`task_list`, `task`, `task_note`): poucas tools compostas, uma por entidade/fluxo, em vez de uma tool para cada verbo CRUD. Isso evita inflar o catálogo com tools muito parecidas e concentra regras de resolução, validação e idempotência em um único contrato por recurso.

Tools previstas:

| Tool | Descrição |
|---|---|
| `job_pipeline` | Lista, lê, cria, atualiza, duplica, ativa/desativa ou remove pipelines conforme `pipeline_id`/`pipeline_slug`, `title`, `duplicate`, `delete`, `enabled` e filtros. |
| `job` | Lista, lê, cria, atualiza, duplica, remove, ativa/desativa, executa ou faz dry-run de jobs conforme `job_id`/`job_slug`, `pipeline_slug`, `delete`, `duplicate`, `enabled`, `run`, `dry_run` e payload de configuração. |
| `job_run` | Lista runs de um job, lê um run específico e retorna timeline/eventos com modos leves (`summary_only`) ou completos. |
| `job_catalog` | Consulta catálogo de tools disponíveis para jobs e schemas necessários para montar/validar `job.inputs`; também pode acionar teste/dry-run de tool quando a AEP-0063 estiver implementada. |

As tools são registradas como **opt-in** via `RegisterOptIn()` — só aparecem quando o perfil de interação as habilita explicitamente. Isso evita poluir o contexto do LLM em perfis que não usam jobs.

Regras de design:

- referências aceitam ID e slug quando aplicável; se ambos forem enviados, precisam apontar para o mesmo recurso;
- listar é o modo padrão quando não há identificador nem campos de escrita;
- ler detalhes é o modo padrão quando há identificador e não há campos de escrita;
- flags como `delete`, `duplicate`, `run` e `dry_run` são mutuamente validadas;
- criação/atualização usam validação completa antes de persistir;
- resultados retornam JSON compacto e metadata com IDs/ações, como nas tools de tasklists.

### D9 — Importação legada pós-login, idempotente e observável

Na primeira sessão autenticada após a atualização, o serviço compartilhado de importações legadas detecta se existem definições de jobs em `~/.assistente/jobs/` e se o usuário ainda não recebeu a importação de jobs. Se sim:

1. Carrega tags e pipelines implícitas a partir dos campos existentes e cria `tags`, `tag_assignments` e `job_pipelines` quando necessário.
2. Carrega todos os YAML e insere como `jobs` (slug = antigo id).
3. Extrai triggers do YAML e insere em `job_triggers`, garantindo um trigger `manual` por job para execuções manuais.
4. Ignora logs legados de runs (`runs/**/*.json`) e eventos (`events/*.jsonl`).
5. Registra resultado, erros e contadores em uma tabela/estrutura de importação observável compartilhada com MCP e outras migrações legadas.

A migração é idempotente por usuário e por recurso. Se parte da importação já existe, a execução seguinte não duplica dados. Não há fallback runtime para filesystem depois da migração: se arquivos antigos não forem importados, eles não continuam alimentando o sistema de jobs.

Logs legados de execução e eventos são dados descartáveis e não fazem parte da migração. `job_runs`, `job_events` e `job_run_events` começam a registrar apenas execuções/eventos novos depois que o runtime DB-only estiver ativo.

### D10 — Repository pattern

A persistência é abstraída por uma interface `Repository`:

```go
type Repository interface {
    ListTags(ctx context.Context) ([]Tag, error)
    UpsertTag(ctx context.Context, tag *Tag) error
    SetResourceTags(ctx context.Context, resourceType, resourceID string, tagSlugs []string) error
    GetResourceTags(ctx context.Context, resourceType, resourceID string) ([]Tag, error)

    ListPipelines(ctx context.Context) ([]Pipeline, error)
    GetPipeline(ctx context.Context, slug string) (*Pipeline, error)
    SavePipeline(ctx context.Context, pipeline *Pipeline) error
    DeletePipeline(ctx context.Context, slug string) error

    ListJobs(ctx context.Context, filter JobFilter) ([]Job, error)
    GetJob(ctx context.Context, slug string) (*Job, error)
    GetJobByID(ctx context.Context, id string) (*Job, error)
    SaveJob(ctx context.Context, job *Job) error
    DeleteJob(ctx context.Context, slug string) error

    ListTriggers(ctx context.Context, jobID string) ([]Trigger, error)
    SaveTriggers(ctx context.Context, jobID string, triggers []Trigger) error
    EnsureManualTrigger(ctx context.Context, jobID string) (*Trigger, error)

    LogRun(ctx context.Context, rl *RunLog) error
    GetRuns(ctx context.Context, jobID string, limit int) ([]RunLog, error)
    GetRun(ctx context.Context, jobID, runID string) (*RunLog, error)
    LogEvent(ctx context.Context, entry *EventEntry) error
    ListEvents(ctx context.Context, filter EventFilter) ([]EventEntry, error)
    LogRunEvent(ctx context.Context, entry *RunEvent) error
    GetRunEvents(ctx context.Context, runID string) ([]RunEvent, error)

    CleanOldRuns(ctx context.Context, maxAge time.Duration) (int, error)
    CleanOldEvents(ctx context.Context, maxAge time.Duration) (int, error)
    CleanOldRunEvents(ctx context.Context, maxAge time.Duration) (int, error)
}
```

Todos os métodos recebem `context.Context` e falham fechado sem usuário autenticado (`database.RequireUserID`), seguindo o padrão de MCP e tasklists. Implementação concreta: `DBRepository` que recebe `*gorm.DB`. O Manager recebe a interface (testável com mocks).

### D11 — Superfície Wails estável, DTOs atualizados

A API Wails exposta ao frontend (`GetJobs`, `SaveJob`, `RunJob`, `GetJobRuns`, etc.) mantém os mesmos métodos e fluxo de uso. Os DTOs de runs mudam para refletir o novo modelo (`ID`/`DurationMs`), e os bindings Wails/frontend precisam ser regenerados e ajustados junto com a implementação. A promessa aqui é não redesenhar a UX nem criar uma API paralela, não manter tipos antigos por compatibilidade.

### D12 — Duration muda de string para inteiro

O campo `Duration` do `RunLog` muda de string (`"1.5s"`) para `DurationMs int64` (1500). Isso facilita queries de performance no banco e elimina parsing de duração no frontend.

Na migração, strings existentes são parseadas via `time.ParseDuration()` e convertidas para milissegundos.

### D13 — RunID muda para UUIDv7

O antigo `RunID` (formato `run_<unix_nano>`) é substituído pelo PK `id` da tabela `job_runs` (UUIDv7 auto-gerado). O campo `RunID` do struct `RunLog` é renomeado para `ID`.

## Tabelas

### tags

| Coluna | Tipo | Constraints | Notas |
|---|---|---|---|
| `id` | TEXT | PK | UUIDv7 via `UUIDModel.BeforeCreate` |
| `user_id` | TEXT | FK→users.id, NOT NULL, INDEX | Dono da tag |
| `slug` | TEXT | NOT NULL | Identificador estável, único por usuário |
| `name` | TEXT | NOT NULL | Nome exibido |
| `description` | TEXT | | Opcional |
| `color` | TEXT | | Opcional para UI |
| `created_at` | DATETIME | | |
| `updated_at` | DATETIME | | |

Índice único: `(user_id, slug)`.

### tag_assignments

| Coluna | Tipo | Constraints | Notas |
|---|---|---|---|
| `id` | TEXT | PK | UUIDv7 |
| `user_id` | TEXT | FK→users.id, NOT NULL, INDEX | Dono da associação |
| `tag_id` | TEXT | FK→tags.id, NOT NULL, INDEX | Tag aplicada |
| `resource_type` | TEXT | NOT NULL, INDEX | Ex.: `job`, `conversation` |
| `resource_id` | TEXT | NOT NULL, INDEX | ID do recurso marcado |
| `created_at` | DATETIME | | |

Índice único: `(user_id, tag_id, resource_type, resource_id)`.
Índice de leitura por recurso: `(user_id, resource_type, resource_id)`.

Nesta AEP, apenas `resource_type = 'job'` precisa ser implementado para substituir `jobs.tags`. A tabela já nasce genérica para conversas e outros recursos futuros.

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
| `trigger_id` | TEXT | FK→job_triggers.id, NOT NULL, INDEX | Toda execução aponta para um trigger, incluindo `manual` |
| `status` | TEXT | NOT NULL, INDEX | `completed`/`failed`/`retrying`/`skipped` |
| `started_at` | DATETIME | NOT NULL, INDEX | Para queries de retenção e ordenação |
| `completed_at` | DATETIME | | |
| `duration_ms` | INT | | Duração em milissegundos |
| `error` | TEXT | | Mensagem de erro (se falhou) |
| `retry_count` | INT | DEFAULT 0 | |
| `is_dry_run` | BOOL | DEFAULT false | |
| `created_at` | DATETIME | | |

`job_runs` guarda somente o estado operacional da execução. Informações obtidas por relacionamento não são duplicadas em texto:

- tool executada vem de `jobs.tool_catalog_id` nesta AEP e de `tool_invocations.tool_catalog_id` na AEP-0063;
- tipo/expressão/configuração do trigger vêm de `job_triggers`;
- inputs resolvidos, output bruto, erro técnico da tool e duração da chamada ficam em `tool_invocations` quando a AEP-0063 for implementada;
- eventos emitidos são consultados em `job_events`/`job_run_events`, não como array textual em `job_runs`.

Execuções manuais usam um registro `job_triggers.type = 'manual'`. Isso evita um caso especial em `job_runs` e preserva a origem da execução por FK.

### job_events

| Coluna | Tipo | Constraints | Notas |
|---|---|---|---|
| `id` | TEXT | PK | UUIDv7 |
| `user_id` | TEXT | FK→users.id, NOT NULL, INDEX | Dono do evento |
| `job_id` | TEXT | FK→jobs.id, NOT NULL, INDEX | Job que emitiu ou recebeu o evento |
| `job_run_id` | TEXT | FK→job_runs.id, INDEX | Run associado, quando o evento veio de uma execução |
| `occurred_at` | DATETIME | NOT NULL, INDEX | Momento em que o evento ocorreu |
| `type` | TEXT | NOT NULL, INDEX | `event_emitted`/`event_received` |
| `event` | TEXT | NOT NULL, INDEX | Nome do evento de domínio |
| `message` | TEXT | | Descrição legível |
| `data` | TEXT | | JSON: dados contextuais |
| `created_at` | DATETIME | | |

`job_events` não registra `triggered`, `completed` ou `failed`; esses estados pertencem a `job_runs` e `job_run_events`.

### job_run_events

| Coluna | Tipo | Constraints | Notas |
|---|---|---|---|
| `id` | TEXT | PK | UUIDv7 |
| `user_id` | TEXT | FK→users.id, NOT NULL, INDEX | Dono do evento |
| `job_run_id` | TEXT | FK→job_runs.id, NOT NULL, INDEX | Execução relacionada |
| `sequence` | INT | NOT NULL | Ordem estável dentro do run |
| `occurred_at` | DATETIME | NOT NULL, INDEX | Momento em que o evento ocorreu dentro da execução |
| `type` | TEXT | NOT NULL, INDEX | `queued`/`started`/`retry_scheduled`/`completed`/`failed`/`skipped`/`event_emitted`/`event_received` |
| `message` | TEXT | | Descrição legível |
| `data` | TEXT | | JSON: dados técnicos do passo |
| `created_at` | DATETIME | | |

Eventos técnicos de execução de tool ficam fora desta tabela e serão registrados em `tool_invocations` pela AEP-0063.

## Mapeamento de dados: filesystem → banco

### Job (YAML → tabela `jobs`)

| Campo YAML | Coluna DB | Transformação |
|---|---|---|
| `id` | `slug` | Renomeado. PK passa a ser UUIDv7 auto-gerado |
| `name` | `name` | Direto |
| `description` | `description` | Direto |
| `enabled` | `enabled` | Direto |
| `pipeline` | `pipeline_id` | Criar/buscar `job_pipelines` por slug e vincular |
| `tags` | `tags` + `tag_assignments` | Criar/buscar tags por slug e associar ao job |
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

### Logs legados não importados

Arquivos antigos em `runs/**/*.json` e `events/*.jsonl` não são importados. Eles eram logs operacionais derivados, descartáveis e pouco úteis após a mudança de modelo. A migração lê apenas definições de jobs e suas configurações associadas.

Consequências:

- `job_runs` começa vazio e recebe apenas execuções novas.
- `job_events` começa vazio e recebe apenas eventos de domínio novos.
- `job_run_events` começa vazio e recebe apenas timeline operacional nova.
- Dados técnicos de chamadas de tools antigas não são preservados; chamadas novas serão tratadas pela AEP-0063 em `tool_invocations`.

## Fases

### Fase 1 — Models GORM + AutoMigrate ✅

1. Criar `internal/database/models_jobs.go` com models GORM para `tags`, `tag_assignments`, `job_pipelines`, `jobs`, `job_triggers`, `job_runs`, `job_events` e `job_run_events`.
2. Funções de conversão entre models e domínio (`Tag`, `Pipeline`, `Job`, `Trigger`, `RunLog`, `EventEntry`, `RunEvent`).
3. Adicionar todos os models ao `AutoMigrate` em `internal/database/database.go`.

### Fase 2 — Repository layer ✅

4. Criar `internal/jobs/repository.go` com interface `Repository` (D10)
5. Implementar `DBRepository` que recebe `*gorm.DB`
6. Testes: CRUD de tags, associações, pipelines, jobs, triggers, runs, events e run events, limpeza por idade

### Fase 3 — Migrar Manager para usar Repository ✅

7. Alterar `ManagerConfig`: adicionar campo `Repository` (`BaseDir` fica apenas como fonte da importação legada inicial)
8. Reescrever `Start()`: carregar jobs do DB, remover inicialização do Watcher
9. Reescrever `SaveJob()`, `DeleteJob()`, `ToggleJob()`: usar Repository
10. Reescrever `GetJobRuns()`, `GetJobEvents()`, `ReplayRun()`: usar Repository

### Fase 4 — Migrar Executor para usar Repository ✅

11. `Execute()` chama `Repository.LogRun()` e `Repository.LogEvent()` em vez do file logger

### Fase 5 — Retenção automática ✅

12. Goroutine no Manager: a cada 24h, chama `CleanOldRuns`,
    `CleanOldEvents` e `CleanOldRunEvents` com a janela configurável
    `job_retention_hours` (padrão 24h), além de aplicar o cap por contagem.
13. Executar limpeza também no `Start()` (ao iniciar o app)

### Fase 6 — Tools nativas ✅

14. Criar tools nativas compostas em `internal/tools/` para pipelines, jobs, runs e catálogo: `job_pipeline`, `job`, `job_run`, `job_catalog`.
15. Registrar como opt-in em `initToolRegistry()`

### Fase 7 — Importação legada filesystem → banco ✅

16. Criar `internal/jobs/migration.go` com lógica de importação legada idempotente (D9), integrada ao serviço compartilhado de importações.
17. Resolver slugs → UUIDs ao importar definições, tags, pipelines e triggers; não importar runs/events legados
18. Registrar contadores/erros da importação para UI/logs e não depender de renomear diretórios como fonte de verdade.
19. Chamar importação pós-login, antes de iniciar scheduler/subscriptions do usuário.

### Fase 8 — Remoção de código filesystem ✅

20. Remover `internal/jobs/watcher.go` (file watcher)
21. Remover `internal/jobs/logger.go` (file-based logging)
22. Remover `marshalJobYAML()`, `LoadAllFromDir()`, campo `FilePath` do struct `Job`
23. Simplificar `parser.go` — manter validação, remover I/O de disco
24. Remover geração/materialização de catálogo em disco; o catálogo passa a ser derivado em runtime do registry/catálogo persistente

### Fase 9 — Testes ✅

25. Testes Repository: CRUD tags, associações, pipelines, jobs, triggers, runs, events, run events, limpeza por idade, roundtrip JSON
26. Testes Manager: Start, Save, Delete, Toggle com DB
27. Testes migração: YAML→DB, tags→DB, triggers→DB, descarte explícito de runs/events legados, idempotência pós-login
28. Testes tools nativas compostas: list/read/create/update/delete/duplicate/toggle/run/dry-run para pipelines e jobs, cobrindo validações de flags incompatíveis
29. Atualizar testes existentes (`logger_test.go` → `repository_test.go`)

## Arquivos afetados

### Novos

| Arquivo | Descrição |
|---|---|
| `internal/database/models_jobs.go` | Models GORM de tags, pipelines, jobs, triggers, runs e eventos |
| `internal/jobs/repository.go` | Interface `Repository` + `DBRepository` |
| `internal/jobs/repository_test.go` | Testes do repository |
| `internal/jobs/migration.go` | Importação legada das definições filesystem → DB |
| `internal/jobs/migration_test.go` | Testes da migração |
| `internal/tools/job_tools.go` | Tools nativas compostas para jobs, pipelines, runs e catálogo |

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

### Ajustes de frontend

- **Frontend**: mantém a mesma UX e os mesmos métodos Wails principais, mas precisa regenerar bindings e ajustar os campos de run (`ID`, `DurationMs`) conforme D11-D13.

## Riscos

| # | Risco | Probabilidade | Impacto | Mitigação |
|---|---|---|---|---|
| R1 | Perda de definições na migração filesystem → DB | Baixa | Alto | Importação transacional, idempotente e observável; arquivos legados não são runtime fallback |
| R2 | Performance de queries com muitos runs | Baixa | Médio | Índices em `job_id` + `started_at` + `status`; retenção padrão de 24h e `runs_per_job_keep` limitam volume |
| R3 | Catálogo de tools fica órfão sem watcher | Baixa | Baixo | Catálogo é regenerado no `Start()` e sob demanda; sem mudança funcional |
| R4 | Tools nativas de jobs poluem contexto | Média | Baixo | Tools são opt-in; perfil escolhe quais habilitar |
| R5 | Serialização JSON de configs complexas perde tipo | Média | Médio | Testes de roundtrip (save → load → compare) para cada tipo |
| R6 | Migração falha parcialmente (parte importada, parte não) | Baixa | Médio | Migração em transação; rollback se qualquer etapa falhar |

## Critérios de aceitação

- [x] **CRUD completo** de pipelines e jobs funciona via banco.
- [x] **Run logs** são persistidos em `job_runs`.
- [x] **Event logs** usam `job_events`/`job_run_events`, não JSONL.
- [x] **Retenção** usa a política configurável vigente da AEP-0074.
- [x] **Migração filesystem** importa YAML sem duplicar em reexecuções.
- [x] **Sem fallback filesystem** no runtime de jobs.
- [x] **Tools nativas** opt-in gerenciam pipelines e jobs.
- [x] **Frontend alinhado** aos DTOs e bindings do backend.
- [x] **Roundtrip JSON** de configurações complexas é coberto no repository.
- [x] **Testes** cobrem repository, manager, migração, executor e tools.
- [x] **File watcher e logger filesystem** foram removidos.
- [x] **Catálogo derivado em runtime** substituiu `catalog.yaml`.
- [x] **Logs legados descartados** conforme contrato da importação.

## Adendo (2026-05-21) — Observabilidade de runs e eventos via tool `job`

Status do adendo: Done — observabilidade e filtros implementados
Autor: Leonardo Gleison (Inclunet)

As decisões abaixo preservam a forma do plano aprovado; a Fase 10, os critérios
marcados e os testes citados registram o estado entregue.

### Contexto

Após a implementação inicial, identificou-se que o objetivo **M2** da Motivação ("Queries: listar jobs por status, filtrar runs por data/status, buscar eventos por tipo") não está sendo entregue ao chat do assistente. Dados gravados em `job_run_events` e `job_events` chegam ao banco mas não chegam ao LLM, e a única ação de observabilidade exposta (`list_runs`) aceita apenas `limit`.

Este adendo cataloga os gaps identificados e atualiza o desenho original do D8 e da Fase 6.

### Gaps identificados no baseline anterior ao adendo

**G1 — Timeline `RunEvents` não é exposta no JSON.**
O struct `RunLog` declara `RunEvents []RunEvent` e `DomainEvents []EventEntry` com tag `json:"-"`, e `DBRepository.GetRuns` faz `Find(&rows)` sem `Preload` da relação `Events`. A tabela `job_run_events` é populada pelo executor, mas nunca chega ao chat. O `Manager` também não tem método público que invoque `Repository.GetRunEvents`.

**G2 — Sem ação para detalhar um run específico.**
`Repository.GetRun(jobID, runID)` existe e `Manager.GetJobRun(jobID, runID)` o expõe, mas a tool `job` não tem nenhuma flag que aceite `run_id`. Não há como inspecionar `output`, `error`, `resolved_inputs` ou timeline de uma execução específica via chat.

**G3 — Eventos de domínio (`JobEvent`) invisíveis ao LLM.**
`Manager.GetJobEventsContext(date)` e `GetJobEventsPageContext(date, limit, offset)` retornam `EventEntry`s do event bus, mas nenhuma tool nativa os expõe. O assistente não consegue investigar gatilhos disparados, eventos publicados por outros jobs, nem mensagens de scheduler.

**G4 — `list_runs` aceita apenas `limit`.**
Sem filtros por `status`, intervalo de tempo ou `is_dry_run`, dry-runs e execuções reais ficam misturados e jobs com muitos runs saturam o resultado antes de revelar as falhas relevantes.

### Decisões adicionais

#### D14 — Consolidar observabilidade na tool `job`; não criar `job_run` separada

Substitui a previsão original do D8 ("tools previstas: ... `job_run` — Lista runs de um job, lê um run específico e retorna timeline/eventos..."). A implementação consolidou observabilidade dentro da tool `job` (via `list_runs`), e mantemos esse desenho: simplifica catálogo, evita uma segunda tool com poucas ações e preserva o contrato já em uso. As ações faltantes (`get_run`, `list_events`) ganham flags próprias na mesma tool.

A Fase 6 original (item 14) referenciava `job_run` como tool independente; passa a referenciar `job` com as flags adicionais deste adendo.

#### D15 — `get_run` retorna `RunLog` completo com timeline

Nova ação na tool `job`: combinada com `job_id` + `run_id`, retorna o `RunLog` com `RunEvents` (timeline operacional ordenada por `sequence`, com `type`, `message`, `data`, `timestamp`) e `DomainEvents` (eventos de domínio correlacionados via `JobRunID`).

Mudanças entregues:

1. Manter `RunLog.RunEvents` e `RunLog.DomainEvents` como `json:"-"` (listagens leves) e introduzir DTO `RunDetail` (`RunLog` + `RunEvents` + `DomainEvents`) usado só pelo `get_run`.
2. Adicionar `Repository.GetRunDetail(ctx, jobID, runID)` que combina `GetRun` + `GetRunEvents` + `ListEvents(filter{RunID})` em uma única chamada. `EventFilter` ganha campo `RunID string` para filtrar `job_events` por `job_run_id`.
3. Expor via `Manager.GetJobRunDetailContext(ctx, jobID, runID)`.
4. Tool `job`: aceita `run_id` validado em conjunto com `job_id`. `run_id` é mutuamente exclusivo com `list_runs`, `run`, `dry_run`, `delete` e `list_events`.

#### D16 — `list_events` para eventos de domínio com filtros completos

Nova ação `list_events` na tool `job` que aceita:

| Parâmetro | Tipo | Default | Notas |
|---|---|---|---|
| `date` | string (`YYYY-MM-DD`) | hoje (local) | Atalho para `start_at`/`end_at` cobrindo 24h |
| `start_at` | string (RFC3339) | — | Sobrescreve `date` se informado |
| `end_at` | string (RFC3339) | — | Sobrescreve `date` se informado |
| `event_type` | string | — | Filtra por `JobEvent.Type` |
| `event_name` | string | — | Filtra por `JobEvent.Event` |
| `job_id` | string (slug) | — | Quando informado, restringe ao job; sem ele, lista global |
| `limit` | int | 50 | Máx 200 |
| `offset` | int | 0 | Paginação |

Implementação reaproveita `Manager.ListJobEventsContext` com `EventFilter` estendido. `list_events` é mutuamente exclusivo com `list_runs`, `run`, `dry_run`, `delete` e `run_id`.

#### D17 — `list_runs` aceita `RunFilter`

Adicionar `Repository.ListRuns(ctx, jobID, filter RunFilter)` ao lado de `GetRuns` (não breaking). `RunFilter`:

| Campo | Tipo | Notas |
|---|---|---|
| `Status` | `[]string` | Aceita `completed`, `failed`, `retrying`, `skipped`. Vazio = todos |
| `StartedAfter` | `time.Time` | RFC3339 na tool |
| `StartedBefore` | `time.Time` | RFC3339 na tool |
| `IncludeDryRun` | `bool` | Default `false` — dry-runs ficam fora a menos que explicitamente pedido |
| `Limit` | `int` | Default 20, máx 100 |

Filtros usam o índice composto `idx_job_runs_user_job_started_at` para `started_at`; `status` e `is_dry_run` já têm índice próprio (`models_jobs.go:97,103`).

### Fases adicionais

#### Fase 10 — Correções de observabilidade ✅

- [x] Adicionar struct `RunFilter` e DTO `RunDetail` em `types.go`; estender `EventFilter` com `RunID`.
- [x] Adicionar `Repository.ListRuns`, `Repository.GetRunDetail`; estender `Repository.ListEvents` para usar `filter.RunID`.
- [x] Adicionar `Manager.ListJobRunsContext`, `Manager.GetJobRunDetailContext`, `Manager.ListJobEventsContext`; métodos legados preservados.
- [x] Estender interface `Manager` da tool em `internal/tools/job/manager.go`.
- [x] Estender tool `job` em `internal/tools/job/job.go`:
    - Novos campos em `jobArgs`: `RunID`, `ListEvents`, `Status []string`, `StartedAfter`, `StartedBefore`, `IncludeDryRun`, `Date`, `StartAt`, `EndAt`, `EventType`, `EventName`, `Offset`.
    - Atualizar `boolCount` e a validação de exclusividade para incluir `list_events` e `run_id`.
    - Validar que filtros de runs só aparecem com `list_runs: true`; filtros de events só com `list_events: true`.
    - Roteamento: `run_id` presente → `getRunDetail`; `list_events` → `listEvents`; demais ramos mantidos.
- [x] Atualizar `Parameters()` JSON schema da tool com os novos campos e descrições.
- [x] Adicionar testes Go para hidratação, filtros, paginação, exclusividade e
  validação “filtro requer list”.

### Critérios de aceitação adicionais

- [x] Tool `job` com `job_id` + `run_id` retorna `RunDetail` com timelines correlacionadas.
- [x] `list_events` oferece filtros e paginação.
- [x] `list_runs` filtra status, intervalo e inclusão de dry-runs no banco.
- [x] Dry-runs são excluídos por padrão.
- [x] Não existe tool `job_run` independente.
- [x] `list_runs` permanece leve; somente `get_run` hidrata timelines.
- [x] `internal/jobs/repository_test.go` e
  `internal/tools/job/job_test.go` cobrem hidratação, filtros, paginação e
  exclusividade das flags.
