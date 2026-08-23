# AEP-0074 — Compactação e Retenção do Banco de Dados

**Status:** Done

## Dependências

- **AEP-0048** (Migração de Jobs para Banco de Dados): definiu a retenção por idade (30 dias) de `job_runs`, `job_events` e `job_run_events` via goroutine no Manager. Esta AEP **substitui** essa janela por uma retenção curta, configurável em horas (padrão 24h), por dados de jobs serem efêmeros.
- **AEP-0063** (Tool Invocations e Common Executor): definiu a retenção de `tool_invocations` (chat 30 dias; invocações de jobs alinhadas a `job_runs`). Esta AEP **redefine** a política: tool calls de **chat** seguem o ciclo de vida da conversa (sem expiração por tempo, por padrão), enquanto dry-runs operacionais são limpos por idade no loop periódico.

## Relacionadas

- **AEP-0052** (Multi-user accounts): toda retenção é escopada por `user_id`. A compactação física (`VACUUM`) é global ao arquivo `.db`.
- **Issue #195** (DB: crescimento excessivo das tabelas de jobs/execução): motivação direta.
- **Issue #292** (Contenção SQLite): restringe como/quando rodar `VACUUM`, pois é operação exclusiva.

## Resumo

O arquivo `conversations.db` cresce de forma monotônica mesmo com a retenção por idade funcionando. A causa-raiz **não** é falta de limpeza lógica — `DELETE`s acontecem — e sim a **ausência de compactação física**: no SQLite, `DELETE` libera páginas internamente (freelist) mas **não devolve espaço ao sistema operacional**. Sem `VACUUM` (ou `auto_vacuum`), o arquivo nunca encolhe.

Esta AEP define uma política de **compactação física** combinada a um **reforço da retenção lógica**:

1. `PRAGMA auto_vacuum=INCREMENTAL` para bancos novos + `PRAGMA incremental_vacuum` periódico (devolve páginas livres sem lock global pesado).
2. Para bancos legados (criados em `auto_vacuum=none`), um `VACUUM` completo único e oportunista, gated por limiar de páginas livres, que também converte o banco para o modo incremental dali em diante.
3. `PRAGMA wal_checkpoint(TRUNCATE)` após grandes deleções para limitar o crescimento do arquivo `-wal`.
4. Retenção de **dados de jobs por categoria**: janela por **idade curta e configurável** (padrão 24h) somada a um teto por **contagem** (últimos N runs por job), para conter jobs de alta frequência.
5. **Tool calls de chat seguem o ciclo de vida da conversa**: não expiram por tempo por padrão; saem em cascata quando a conversa/mensagem é removida, com uma varredura de órfãos como rede de segurança. Um cap de idade opcional (em dias) pode ser configurado. Dry-runs operacionais (job/tool_catalog) são limpos por idade no loop periódico.
6. **Toda a política vive no `config.json`** (seção `maintenance`), editável na tela de Configurações. **Não há variáveis de ambiente.**

## Motivação

1. **Arquivo só cresce**: usuários relatam `conversations.db` crescendo indefinidamente. A retenção de 30 dias remove linhas, mas o espaço liberado permanece reservado pelo arquivo. Sem compactação, o tamanho em disco só sobe.

2. **Janela de 30 dias é insuficiente para alta frequência**: um job disparando a cada minuto gera ~43k runs antes de qualquer purga por idade. Cada run carrega colunas TEXT volumosas (`inputs`, `output`, `trigger_data`, `events_emitted`) e ainda gera `job_run_events`/`job_events` e `tool_invocations`. Falta um teto por contagem.

3. **`tool_invocations` só limpa no login**: a limpeza por idade dessa tabela (AEP-0063) é disparada apenas em `reloadUserScopedRuntime` (login/refresh). Sessões longas (app aberto por dias) nunca disparam a purga.

4. **Contenção (issue #292)**: `VACUUM` completo adquire lock exclusivo e pode levar segundos em bancos grandes, agravando `SQLITE_BUSY`. A estratégia precisa ser oportunista (momento ocioso), throttled e proteger leituras interativas.

## Estado implementado

| Mecanismo | Onde | Comportamento |
|---|---|---|
| Retenção runs/eventos (24h, configurável) | `internal/jobs/manager.go` (`runRetention`, loop 24h + `Start`) | Remove `job_runs`/`job_events`/`job_run_events` por idade; cascata em `tool_invocations` (`origin_type=job_run`) |
| Tool calls de chat | ciclo de vida da conversa + `CleanOrphanChat` (login e loop) | Sem expiração por tempo por padrão; cap de idade opcional |
| Dry-runs operacionais | `CleanOldDryRuns` no `runRetention` | Idade curta de jobs |
| Guardas de volume na escrita | budget 10 MiB por resultado; truncamento de input/output | Limita tamanho por linha, não o total |
| Pragmas | `internal/database/database.go` (`Init`) | `journal_mode=WAL`, `synchronous=NORMAL`, `auto_vacuum=INCREMENTAL`, `busy_timeout` |
| `VACUUM` / `auto_vacuum` | `internal/database/maintenance.go` | Compactação incremental; `VACUUM` completo gated para bancos legados |

Testes de compactação, conversão de bancos legados e retenção ficam em
`internal/database/maintenance_test.go`, `sqlite_policy_test.go`,
`internal/toolinvocations/repository_test.go` e testes de retenção de jobs.

## Decisões

### D1 — `auto_vacuum=INCREMENTAL` para bancos novos

`Init` passa a executar `PRAGMA auto_vacuum=INCREMENTAL` **antes** do `AutoMigrate`. Para bancos criados do zero isso ativa o modo incremental imediatamente (o modo só pode ser definido antes de qualquer tabela existir, ou via `VACUUM`). No modo incremental, páginas livres vão para uma freelist e podem ser devolvidas ao SO sob demanda com `PRAGMA incremental_vacuum`, sem o custo de reescrever o arquivo inteiro.

### D2 — `VACUUM` completo oportunista para bancos legados

Bancos existentes nascem em `auto_vacuum=none` e o pragma sozinho não converte o modo. A função de manutenção detecta o modo atual; se for diferente de incremental **e** houver espaço livre relevante (ou compactação forçada), executa um `VACUUM` completo. Esse `VACUUM`:

- recupera o espaço acumulado de uma só vez;
- com o pragma já definido para incremental, **converte o banco** para o modo incremental dali em diante (o próximo `incremental_vacuum` passa a bastar).

O `VACUUM` completo é **gated** por um limiar de páginas livres (`PRAGMA freelist_count` × `page_size`), evitando rodar quando há pouco a recuperar. Padrão: 16 MiB.

### D3 — Compactação throttled e em momento ocioso

A compactação roda a partir do `runRetention` do Manager (em `Start`, logo após o login, e a cada 24h), mas com **throttle global** (no máximo 1× por intervalo de retenção) para não competir com a UI. `wal_checkpoint(TRUNCATE)` roda junto para limitar o `-wal`. `PRAGMA busy_timeout` é configurado para que a compactação aguarde locks transitórios em vez de falhar imediatamente — alinhado (sem antecipar) com a investigação da issue #292.

A compactação é **best-effort**: qualquer erro é logado e não interrompe o boot nem a retenção.

### D4 — Retenção de dados de jobs: idade curta + contagem

Dados de jobs são **operacionais e descartáveis**, sem valor histórico. A janela por idade passa de 30 dias para **horas configuráveis** (padrão **24h**), aplicada a `job_runs`/`job_events`/`job_run_events`. Soma-se um teto por **contagem**: no máximo **N runs por job** (padrão 200), escopado por usuário. A purga por contagem reaproveita a mesma cascata da purga por idade (remove `tool_invocations` de `origin_type=job_run`, `job_run_events` e `job_events` dos runs descartados antes de remover os `job_runs`). Isso protege contra jobs de alta frequência sem reter dados além do necessário.

### D5 — Tool calls de chat seguem o ciclo de vida da conversa

Diferentemente dos dados de jobs, tool calls de **chat** fazem parte do histórico da conversa. Por isso **não expiram por tempo por padrão**: só são removidas quando a conversa/mensagem de origem é deletada (cascata já existente). Como rede de segurança, `CleanOrphanChat` remove invocações de chat cujo `origin_id` não existe mais em `chat_messages` — roda no login e no loop periódico.

Um **cap de idade opcional** (`chat_tool_calls_retention_days`, padrão `0` = sem limite) permite ao usuário forçar a remoção de tool calls de chat mais antigas que X dias, via `CleanOldChat`.

Os **dry-runs operacionais** (`origin_type` em `tool_catalog`/`job_run`, `dry_run=true`) não têm valor histórico e são limpos por idade (janela de jobs) no `runRetention` via `CleanOldDryRuns`. A antiga `CleanOld` (que purgava chat por tempo) foi removida.

### D6 — Configurabilidade via `config.json` (sem env)

A política vive na seção `maintenance` do `config.json` (`internal/config`), única fonte de verdade. **Não há variáveis de ambiente.** Os acessores `config.GetMaintenance`/`SaveMaintenance` aplicam defaults e normalização; `database.Compact` recebe o limiar por parâmetro (`minFreeBytes`), sem ler configuração diretamente.

| Parâmetro (`config.json`) | Chave | Default |
|---|---|---|
| Idade de retenção de jobs (horas) | `job_retention_hours` | 24 |
| Runs mantidos por job | `runs_per_job_keep` | 200 |
| Cap de idade de tool calls de chat (dias; 0 = ilimitado) | `chat_tool_calls_retention_days` | 0 |
| Limiar de `VACUUM` completo (bytes) | `vacuum_min_free_bytes` | 16 MiB |

A tela de Configurações (aba **Dados**) expõe esses campos e um botão "Limpar agora" (`RunDatabaseMaintenance`), além do tamanho atual do banco (`GetDatabaseStats`).

### D7 — Remoção de fallbacks mortos de `DefaultModel`

Aproveitando a reforma do `config`, os fallbacks de modelo que liam `config.DefaultModel` (`send_message` e `App.GetEffectiveModel`) foram removidos: o modelo vem exclusivamente do perfil ativo. O teardown completo dos demais campos legados do `config.json` é rastreado em issue própria (#299).

## Fases

1. **Pragmas** — `auto_vacuum=INCREMENTAL` e `busy_timeout` no `Init`; guardar o caminho do `.db` no pacote `database`.
2. **Manutenção física** — `internal/database/maintenance.go` com `Compact(ctx, force, minFreeBytes)` (incremental/full gated + `wal_checkpoint`) e estatísticas de tamanho (`DatabaseStats`).
3. **Config** — seção `maintenance` em `internal/config` (`GetMaintenance`/`SaveMaintenance`); remoção das env vars e dos fallbacks de `DefaultModel`.
4. **Retenção por categoria** — `CleanRunsExceedingCount` (cap) nos jobs; `CleanOldDryRuns`/`CleanOldChat`/`CleanOrphanChat` em `toolinvocations`; `runRetention` lê o `config.json`.
5. **Wails + UI** — `GetMaintenanceSettings`/`SaveMaintenanceSettings`/`GetDatabaseStats`/`RunDatabaseMaintenance` e a seção na aba Dados (i18n/a11y/tokens).
6. **Testes** — cap por contagem com cascata; `Compact` gated (modo none → vacuum → incremental); `CleanOldDryRuns`/`CleanOldChat`/`CleanOrphanChat`.

## Riscos

| ID | Risco | Prob. | Impacto | Mitigação |
|---|---|---|---|---|
| R1 | `VACUUM` agrava contenção (issue #292) | Média | Médio | Throttle, momento ocioso (pós-login), `busy_timeout`, gating por limiar; incremental como caminho padrão após a primeira conversão |
| R2 | Cap por contagem remove runs ainda úteis | Baixa | Baixo | Default alto (200/job) + configurável no `config.json`; janela por idade separada |
| R5 | Retenção curta (24h) descarta dado de job ainda necessário | Média | Baixo | Configurável (horas) + cap por contagem independente; dados de jobs são operacionais por definição |
| R3 | `VACUUM` longo em banco muito grande | Baixa | Médio | Roda uma vez (converte para incremental); subsequentes usam `incremental_vacuum` barato |
| R4 | Erro de manutenção interrompe boot | Baixa | Alto | Best-effort: tudo logado, nada propaga erro fatal |

## Critérios de aceitação

- [x] `Init` define `auto_vacuum=INCREMENTAL` e `busy_timeout`; bancos novos nascem no modo incremental.
- [x] Existe `database.Compact(ctx, force, minFreeBytes)` que: roda `wal_checkpoint(TRUNCATE)`; usa `incremental_vacuum` no modo incremental; faz `VACUUM` completo gated em bancos legados, convertendo-os para incremental.
- [x] O arquivo `.db` encolhe após retenção + compactação (validado por teste com tamanho total antes/depois e `freelist_count`).
- [x] `CleanRunsExceedingCount` mantém os últimos N runs por job, removendo em cascata `tool_invocations`/`job_run_events`/`job_events`.
- [x] Tool calls de chat seguem o ciclo de vida da conversa; `CleanOrphanChat` remove órfãos; cap de idade opcional via `CleanOldChat`; dry-runs via `CleanOldDryRuns`.
- [x] `runRetention` lê o `config.json` e executa cap por contagem, limpeza de dry-runs, varredura de órfãos de chat e compactação throttled.
- [x] Parâmetros configuráveis no `config.json` (seção `maintenance`), sem variáveis de ambiente, com defaults seguros.
- [x] UI na aba Dados expõe a política e o botão "Limpar agora" (i18n nos 3 locales, a11y, tokens de tema), via Wails.
- [x] Toda a manutenção é best-effort e escopada por usuário onde aplicável.

## Follow-up (fora do escopo desta fatia)

- Teardown completo dos campos legados do `config.json` (welcome wizard, tokens controller, `App.tsx`, bindings) — issue #299.
- Reconciliar com a estratégia de concorrência da issue #292 (pooling, retries em `SQLITE_BUSY`).
