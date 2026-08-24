# AEP-0076 — Versionamento de Schema do Banco (schema_migrations)

**Status:** Done

| Campo | Valor |
|-------|-------|
| Issue | [#247](https://github.com/inclunet/assistente/issues/247) |
| Relacionados | AEP-0046 (UUIDv7), AEP-0052 (Multi-user), AEP-0074 (Compaction) |

## Resumo

Introduz um mecanismo explícito de **versionamento de schema** para o banco SQLite (GORM). Antes, o histórico de schema era implícito no boot (`internal/database/database.go` → `Init()`): `AutoMigrate` de ~24 models seguido de um conjunto de migrações custom (`migrateToUUIDv7`, `dedupCredentialEntriesBeforeMigrate`, `ensure*Index`, `ensureUsernameCaseInsensitive`, normalização de booleanos, `migrateRefreshURLToEnc`) que **rodavam ou re-checavam a cada startup**.

A partir deste AEP, cada mudança estrutural é uma **migração numerada e idempotente**, registrada na tabela `schema_migrations` e espelhada em `PRAGMA user_version`. Cada migração roda **no máximo uma vez por banco**. `AutoMigrate` permanece responsável apenas pela **adição** de colunas/tabelas novas; mudanças estruturais (conversões de tipo, índices, normalizações de dados) passam a ser versionadas.

## Motivação

1. **Re-execução em todo boot**: a checagem da migração UUIDv7 (AEP-0046) e os vários `ensure*Index` rodavam a cada inicialização. Para a UUIDv7, isso significa abrir o banco e inspecionar o schema toda vez — desnecessário num banco já convertido.
2. **Histórico implícito e difícil de auditar**: não havia como saber, olhando o banco, em que "versão" de schema ele estava. Upgrades parciais e diagnósticos ficavam arriscados.
3. **Ordem frágil e acoplada**: a ordem das migrações custom existia apenas como sequência de chamadas em `Init()`, sem contrato explícito (algumas precisam rodar **antes** do `AutoMigrate`, outras **depois**).
4. **Evolução futura**: novas mudanças estruturais precisavam de um lugar canônico e seguro para serem adicionadas, sem reabrir a discussão de "rodar ou não a cada boot".

## Decisões

### D1 — Fonte de verdade: tabela `schema_migrations`

```sql
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    applied_at DATETIME NOT NULL
);
```

Cada migração aplicada insere uma linha. `version` é a chave primária (sequencial, única, estritamente crescente). A maior `version` **contígua** aplicada (o maior `N` tal que `1..N` estão todos registrados, sem buracos) é **espelhada** em `PRAGMA user_version` como atalho de inspeção (`PRAGMA user_version` sem ler a tabela), mas a **fonte de verdade é a tabela**. Usa-se o prefixo contíguo (e não `MAX(version)`) porque uma migração adiada (`errMigrationDeferred`) não é registrada enquanto versões posteriores podem ser; espelhar o contíguo evita que `user_version` salte à frente de uma migração anterior ainda pendente. Buracos eventuais permanecem visíveis e auditáveis na tabela.

### D2 — Migração como struct registrada

Em `internal/database/migrator.go`:

```go
type migration struct {
    Version       int
    Name          string
    Phase         migrationPhase // pré ou pós AutoMigrate
    Run           func(*gorm.DB) error
    DetectApplied func(*gorm.DB) (bool, error) // opcional
}

var schemaMigrations = []migration{ /* v1..vN ordenadas */ }
```

Regras:
- **Idempotência obrigatória** em `Run`: rodar de novo num banco já migrado não pode causar dano. A tabela evita reexecução no caminho feliz; a idempotência é a rede de segurança contra crash entre *aplicar* e *registrar*.
- **Nunca renumerar/reutilizar** versões liberadas — apenas adicionar novas ao final.

### D3 — Duas fases: pré e pós `AutoMigrate`

`AutoMigrate` continua no meio do boot. Migrações que **precisam preceder** o `AutoMigrate` rodam em `phasePreAutoMigrate`; as demais em `phasePostAutoMigrate`.

- **Pré** (`runMigrations(db, phasePreAutoMigrate)`):
  - `v1 uuidv7_id_migration` — conversão INTEGER → UUIDv7 (reescreve tabelas; precisa preceder a criação de colunas novas).
  - `v2 dedup_credential_entries_pre_unique` — dedup de `(user_id, pattern)` antes do `AutoMigrate` criar o índice unique (senão a criação falha).
- **`AutoMigrate`** — adição de colunas/tabelas.
- **Pós** (`runMigrations(db, phasePostAutoMigrate)`):
  - `v3 task_note_external_unique_index`
  - `v4 task_list_user_slug_unique_index`
  - `v5 chat_message_window_indexes`
  - `v6 credential_entry_legacy_index_cleanup`
  - `v7 username_case_insensitive`
  - `v8 normalize_summarizing_in_progress_bool`
  - `v9 refresh_url_to_enc`

A ordem v1..v9 é o **baseline histórico** entregue por esta AEP e preserva a
ordem anterior das chamadas custom em `Init()`. O mecanismo continuou recebendo
migrações depois dessa entrega. No registro vigente:

- **Pré-AutoMigrate:** `v10 acp_session_user_id_not_null`;
- **Pós-AutoMigrate:** `v11 drop_acp_session_prompt_prefix_hash`;
- **Pós-AutoMigrate:** `v12 acp_providers_single_type`.

`v12` é apenas a maior versão no snapshot atual, não um teto permanente. Novas
mudanças estruturais devem continuar sendo acrescentadas ao fim de
`schemaMigrations` com a próxima versão livre.

### D4 — Adoção de bancos existentes sem quebra (`DetectApplied`)

Bancos já em produção **já passaram** pelas migrações custom no fluxo antigo. Para não reexecutar trabalho pesado/destrutivo, cada migração pode declarar `DetectApplied`, que inspeciona o banco e retorna `true` quando o estado-alvo **já existe**. Nesse caso a migração é **marcada como aplicada sem rodar `Run`**.

Aplicado à `v1` (UUIDv7): se `conversations.id` já é `TEXT`, marca como aplicada; se ainda é `INTEGER`, executa a conversão real; se a tabela não existe (banco novo), deixa o `Run` rodar (no-op que apenas registra a versão).

As demais migrações (v2–v9) são **idempotentes e baratas** (índices `IF NOT EXISTS`, `UPDATE` com `WHERE` que não casa nada quando já normalizado, etc.), então não precisam de `DetectApplied`: rodam uma única vez sob o novo mecanismo num banco legado e ficam registradas.

### D5 — `AutoMigrate` permanece para adição de colunas

`AutoMigrate` não é removido nem versionado. Ele continua sendo a forma idiomática (GORM) de adicionar colunas e tabelas novas a partir dos models. Apenas **mudanças estruturais** (conversões de tipo, índices custom, normalizações de dados legados) passam pelo mecanismo versionado.

## Fases (de implementação)

1. **Mecanismo**: `migrator.go` com `migration`, `schemaMigrations`, `runMigrations`/`runMigrationList`, `ensureSchemaMigrationsTable`, `appliedMigrationVersions`, `recordMigration`, `syncUserVersion`.
2. **Migração do baseline custom histórico**: encapsular as funções que já
   existiam como entradas v1..v9, preservando ordem e fases.
3. **Refatorar `Init()`**: substituir as chamadas diretas por `runMigrations(db, phasePreAutoMigrate)` (antes do `AutoMigrate`) e `runMigrations(db, phasePostAutoMigrate)` (depois).
4. **Testes do framework e do registro evolutivo**: aplicação ordenada,
   idempotência, filtragem por fase, `DetectApplied` (carimba sem rodar), parada
   em erro, consistência do registro, detector UUIDv7 e integração do registro
   real (fresh DB + segundo boot no-op). Esses testes percorrem a lista vigente,
   hoje v1..v12, sem codificar v12 como versão final.

### Extensões posteriores ao baseline

As versões v10..v12 são uso continuado do framework entregue, não reabertura
das fases 1–4: `internal/database/migrator.go` as registra nas fases adequadas,
e `internal/database/migrator_test.go` valida ordenação estritamente crescente,
ausência de duplicatas, aplicação de todo o registro real e idempotência no
segundo boot.

## Riscos

- **Crash entre aplicar e registrar**: mitigado pela idempotência obrigatória — a migração reroda no próximo boot sem dano. Optou-se por não envolver `Run` + registro numa transação única porque algumas migrações (UUIDv7) já gerenciam transações próprias.
- **Banco legado em INTEGER aberto pela primeira vez**: `DetectApplied` da v1 retorna `false` e a conversão real roda — comportamento idêntico ao anterior, agora registrado ao final.
- **Edição incorreta da lista** (versão duplicada/fora de ordem): guardado por teste de consistência do registro.
- **Espelho `user_version` divergir da tabela**: `user_version` é apenas informativo; nenhuma lógica depende dele como fonte de verdade.

## Critérios de aceitação

- [x] Tabela `schema_migrations` criada no boot e `PRAGMA user_version` espelhando a maior versão contígua aplicada (prefixo 1..N sem buracos).
- [x] Migrações custom que formavam o baseline histórico foram migradas para o
  mecanismo (v1..v9), preservando ordem e fases.
- [x] Registro vigente inclui v10..v12 e permanece extensível; testes exercitam
  a lista completa sem tratar v12 como limite permanente.
- [x] Bancos já migrados **não** reexecutam a conversão UUIDv7 (marcada via `DetectApplied`).
- [x] `AutoMigrate` mantido para adição de colunas.
- [x] Testes: ordem, idempotência, fase, `DetectApplied`, parada em erro, registro real (fresh DB + segundo boot no-op).
- [x] `go build`, `go vet`, `go test`, `golangci-lint` verdes.
