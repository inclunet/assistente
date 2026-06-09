# AEP-0051 — Migração de Skills para Banco de Dados

**Status**: Proposta (revisada 2026-06-08)  
**Criado em**: 2026-04-21  
**Depende de**: AEP-0046 (UUIDv7 Migration)  
**Fundação de**: AEP-0072 (Skill Catalog & Loading)  
**Relacionado**: AEP-0025 (Interaction Profiles), AEP-0047 (Import/Export), AEP-0048 (Jobs DB), AEP-0049 (MCP DB), AEP-0050 (Profiles DB)

> **Nota de revisão (2026-06-08).** Esta AEP foi reconciliada com a AEP-0072. Duas mudanças relevantes em relação à versão original de abril:
> 1. **Filesystem → DB deixa de ser uma "migração one-time destrutiva" e passa a ser uma importação legada não-destrutiva e idempotente** (mesmo padrão de MCP/Jobs — ver D9). Os arquivos `SKILL.md` originais NÃO são renomeados/apagados.
> 2. Esta AEP é **independente** da AEP-0050 (Profiles DB), cuja implementação foi adiada. A integração `profile_skills → skills.slug` fica para a issue de implementação da AEP-0050.

---

## Resumo

Migrar o sistema de skills de SKILL.md em filesystem (multi-diretório via `configdir.Resolver`) para SQLite via GORM. Skills são instruções Markdown com metadados YAML (frontmatter) que personalizam o comportamento do LLM — desde coding assistants até job managers. Atualmente vivem em `~/.assistente/skills/{slug}/SKILL.md` com resolução de 3 diretórios.

A migração cria 2 tabelas (`skills` e `skill_tools`), usa UUIDv7 como PK, mantém slug como identificador humano, seed de builtins (6 skills embeddados) com versionamento, e preserva compatibilidade com o formato SKILL.md para import/export. As skills de filesystem são trazidas para o banco via **importação legada não-destrutiva** (D9), no mesmo fluxo pós-login de MCP e Jobs. Esta AEP é a fundação de dados da AEP-0072 (catálogo, descoberta e carregamento sob demanda de skills).

---

## Motivação

### Problemas do modelo atual

1. **Filesystem disperso**: Skills em 3 diretórios (exe, home, workdir) com deduplicação por prioridade. Mesmo problema dos profiles.

2. **Parsing a cada load**: Cada `Get(slug)` lê arquivo do disco, parseia frontmatter YAML + Markdown, valida. Sem cache.

3. **Sem consultas eficientes**: Listar skills com filtro (autoload, user-invocable) requer ler e parsear todos os SKILL.md.

4. **Metadados ricos e estruturados**: SkillMetadata tem 40+ campos em 13 categorias. Armazenar como arquivo flat impede queries, validação por constraint, e versionamento.

5. **Habilita a AEP-0072**: catálogo compacto, gating e carregamento sob demanda de skills (AEP-0072) exigem skills queryáveis no banco — varrer/parsear filesystem a cada turno não escala.

6. **Alinhamento**: Jobs (AEP-0048), MCP (AEP-0049) e Profiles (AEP-0050) seguem o mesmo caminho. A importação filesystem → DB usa o mesmo fluxo legado de MCP/Jobs (D9).

---

## Estado Atual

### Armazenamento

- **Formato**: SKILL.md (YAML frontmatter + conteúdo Markdown) + arquivos complementares
- **Diretórios** (via `configdir.Resolver`):
  1. `<exe>/.assistente/skills/` — skills embeddados (read-only)
  2. `~/.assistente/skills/` — skills do usuário (read-write)
  3. `./.assistente/skills/` — skills do workspace (prioridade mais alta)
- **Builtins**: 6 skills em `internal/app/builtin/skills/` (coding, editor-texto, job-manager, memory, tasklist-manager, workspace) + fallback em `assets/builtin/skills/` (3 skills)

### Struct SkillMetadata (40+ campos)

> Recorte **ilustrativo** (pseudocódigo) apenas para dar a dimensão dos campos. Não reflete os nomes/tipos exatos (`Tools`, `Agent`, `Input`, `Output` etc. diferem). A fonte canônica é `internal/skills/types.go`.

```go
type SkillMetadata struct {
    // Identificação (obrigatório)
    Name        string // kebab-case, max 64 chars
    Version     string // semver X.Y.Z
    Description string // 10-160 chars

    // Apresentação
    DisplayName string
    Author      string
    License     string

    // Comportamento de invocação
    AutoLoad               bool   // Injeta no system prompt automaticamente
    DisableModelInvocation bool   // Bloqueia auto-invocação pelo modelo
    UserInvocable          *bool  // nil=default(true), false=esconde do menu /slash

    // Agent/Context
    SkillContext string      // Fork de contexto (subagent isolado)
    Agent       *AgentConfig // Config de agente avançado

    // Tools
    Tools       []ToolRef  // Lista de tools necessárias
    AllowedTools []string  // Tools permitidas (alternativo)

    // Permissões
    Filesystem  *FilesystemPermissions  // read/write paths, max size
    Network     *NetworkPermissions     // allowed domains, deny patterns
    Dependencies *DependenciesConfig    // required tools, packages

    // MCP Integration
    MCP *MCPConfig // Servidor MCP embeddado no skill

    // Input/Output
    Input  *InputSpec  // Argumentos esperados
    Output *OutputSpec // Formato de saída

    // Behavior
    Behavior *BehaviorConfig // timeout, retry, error handling
}
```

### Manager (14 operações)

- `List()`, `Get(slug)`, `Create(meta, content)`, `Update(slug, meta, content)`, `Delete(slug)`, `Duplicate(slug)`
- `GetAutoSkills()`, `GetAvailableSkills()`, `GetAllSkillsFull()`, `GetUserInvocableSkills()`
- `GetSkillFiles(slug)`, `GetSearchPaths()`, `EnsureDir()`

### Controller (8 métodos Wails)

- `GetSkills()`, `GetSkill(slug)`, `CreateSkill(req)`, `UpdateSkill(slug, req)`, `DeleteSkill(slug)`, `DuplicateSkill(slug)`
- `GetUserInvocableSkills()`, `GetSkillSearchPaths()`
- Eventos: `skill:created`, `skill:updated`, `skill:deleted`

---

## Decisões

### D1. Schema "colunas-completas": todos os escalares como colunas + JSON para structs aninhadas

> **Revisão de implementação (2026-06-08).** Durante a Fase 1 verificou-se que o `SkillMetadata` real (`internal/skills/types.go`) tem ~40 campos, vários ausentes da tabela ilustrativa original (Keywords, Category, Subcategory, Type, Difficulty, Audience, MinVersion/MaxVersion, Platforms, Languages, Frameworks, AuthorEmail/AuthorURL, Repository, Homepage, ArgumentHint, Model, Triggers, Hooks; e `agent` é `string`, não `*AgentConfig`). Para garantir o **roundtrip fiel** (critério 2), adotou-se a estratégia **colunas-completas**: todo campo escalar (incluindo slices simples de string serializados como JSON array) vira coluna; cada struct aninhada/opcional vira uma coluna JSON TEXT.

Skills têm campos com dois perfis distintos:

| Tipo | Estratégia | Exemplos |
|------|-----------|----------|
| Escalares + slices de string | Colunas diretas (slices como JSON array em TEXT) | name, version, description, auto_load, user_invocable, keywords, platforms, type, difficulty |
| Structs aninhadas opcionais | Coluna JSON TEXT | filesystem, network, tools, input, output, behavior, triggers, hooks, dependencies, mcp |
| Conteúdo Markdown | Coluna TEXT | content (corpo do SKILL.md) |

**Justificativa**: as structs aninhadas são opcionais (`*pointer`), raramente queryadas, e extremamente variáveis. Normalizar Filesystem/Network/MCP em tabelas separadas criaria 5+ tabelas pouco utilizadas. JSON é a escolha pragmática para elas; os escalares ficam em colunas para query e fidelidade.

**Exceção**: `skill_tools` como tabela separada (junction) para as tools allowed/denied, derivada de `tools_config`, pois beneficia de query eficiente.

### D2. UUIDv7 como PK + slug como identificador humano

- PK: `id TEXT` com UUIDv7 (AEP-0046)
- Slug: `slug TEXT UNIQUE NOT NULL` (nome kebab-case do skill)
- Referências: profiles usam slug na junction table `profile_skills`

### D3. Conteúdo Markdown na tabela principal

O conteúdo do SKILL.md (após frontmatter) é armazenado em `skills.content TEXT`. Não em tabela separada — skills não são enormes (tipicamente < 10KB) e o conteúdo é sempre necessário para invocação.

### D4. Skill files complementares

Alguns skills têm arquivos complementares (ex: templates, exemplos). Estes **permanecem em disco** por enquanto. O banco armazena o skill principal (SKILL.md). Arquivos complementares são acessados via `GetSkillFiles(slug)` que continua lendo do filesystem.

**Motivação**: Arquivos complementares são raros, podem ser binários, e não beneficiam de armazenamento em banco. Migração completa pode vir em AEP futura.

### D5. Builtin skills via seed no DB

No boot:
1. Lê skills embeddados do binário (6 skills)
2. Para cada: `SeedBuiltin(skill, version)`:
   - Não existe → insere com `is_builtin=true`
   - Existe + `is_customized=true` → skip
   - Existe + versão < embeddada → update
3. Skills do usuário não são afetados

### D6. Compatibilidade import/export com SKILL.md

O formato SKILL.md (YAML frontmatter + Markdown) continua sendo suportado para:
- **Import**: Ler SKILL.md → parsear → inserir no DB
- **Export**: Ler do DB → compor SKILL.md → salvar em disco
- Funções `Parse()` e `Compose()` permanecem

### D7. Permissões de tools em tabela separada

O parser atual (`ResolveToolsRaw` em `internal/skills/types.go`) resolve tudo para uma struct `ToolPermissions` (`Allowed`/`Denied`/`BashCommands`), a partir de três formatos de frontmatter:
- `allowed-tools: "Read, Grep, Glob"` — string comma-separated (formato Claude Code oficial) → `ToolPermissions.Allowed`.
- `tools: [read_file, write_file]` — lista simples legada → interpretada como `ToolPermissions.Allowed`.
- `tools: { allowed: [...], denied: [...], bashCommands: {...} }` — objeto (formato Agent Skills, usado inclusive nas builtins) → `ToolPermissions` completa.

Não existe `allowed_tools` (com underscore) nem "tools requeridas" / "tool definitions inline com description" no parser atual.

Persistência: para preservar `Parse()`/`Compose()`, a `ToolPermissions` resolvida é guardada como JSON em `tools_config`. A junction `skill_tools` é populada a partir dela para consultas eficientes: cada item de `Allowed` vira uma row com `relation="allowed"` e cada item de `Denied`, `relation="denied"`.

### D8. Runtime serve skills exclusivamente do DB

Após a importação legada (D9), o runtime passa a ler/servir skills **somente do banco** — o `configdir.Resolver` deixa de ser fonte de runtime. O código de descoberta multi-diretório, porém, **não é deletado**: ele é reaproveitado como `LegacyImportSource` (apenas lista/lê os `SKILL.md` para importar). Isso difere da versão original desta AEP (que eliminava o resolver por completo).

### D9. Filesystem → DB via importação legada não-destrutiva

As skills hoje em `configdir/skills/{slug}/SKILL.md` são tratadas como **fonte de importação legada**, exatamente como servidores MCP e definições de Jobs:

- Reusa o contrato genérico `ImportLegacyResourcesWithContext[T]` em `internal/portability/legacy_import.go` e pluga em `runPostLoginLegacyImports` (`internal/app/app_legacy_imports.go`), ao lado de `ImportLegacyMCPServersWithContext` e `jobMgr.ImportLegacyDefinitions`.
- **Não-destrutivo**: o contrato `LegacyImportSource` exige listar e ler os arquivos originais "without renaming, deleting, or rewriting them". Não há rename para `skills.migrated/`.
- **Idempotente**: re-executar a importação não duplica nem altera; skill já existente no DB = `skipped`.
- **Source** = código de descoberta de skills reaproveitado (lista `skills/{slug}/SKILL.md` nos 3 diretórios, com a mesma prioridade workdir > home > exe); **Parse** = `skills.Parse()`; **Import** = upsert idempotente no DB.

Esta decisão substitui a "migração one-time com backup/rename" descrita na versão original (ver Fases 5–6 revisadas).

---

## Tabelas

### `skills` (tabela principal)

Schema canônico: `internal/database/models_skills.go` (struct `Skill`).

| Coluna | Tipo | Constraints | Notas |
|--------|------|-------------|-------|
| `id` | TEXT | PK | UUIDv7 |
| `slug` | TEXT | UNIQUE NOT NULL INDEX | Nome kebab-case |
| `name` | TEXT | NOT NULL | Mesmo que slug (spec exige kebab-case) |
| `version` | TEXT | NOT NULL | Semver X.Y.Z |
| `description` | TEXT | NOT NULL | 10-160 chars |
| `display_name` | TEXT | | Nome legível (fallback: name) |
| `author` | TEXT | | |
| `author_email` | TEXT | | |
| `author_url` | TEXT | | |
| `license` | TEXT | | SPDX |
| `repository` | TEXT | | |
| `homepage` | TEXT | | |
| `keywords` | TEXT | | JSON array (max 10) |
| `category` | TEXT | INDEX | |
| `subcategory` | TEXT | | |
| `type` | TEXT | INDEX | command/agent/hook/mcp |
| `difficulty` | TEXT | | beginner/intermediate/advanced |
| `audience` | TEXT | | JSON array |
| `min_version` | TEXT | | Semver |
| `max_version` | TEXT | | Semver |
| `platforms` | TEXT | | JSON array |
| `languages` | TEXT | | JSON array |
| `frameworks` | TEXT | | JSON array |
| `auto_load` | BOOL | NOT NULL DEFAULT false INDEX | Injetar no system prompt |
| `disable_model_invocation` | BOOL | NOT NULL DEFAULT false | |
| `user_invocable` | BOOL | | NULL = default(true) |
| `argument_hint` | TEXT | | Dica de args |
| `skill_context` | TEXT | | "fork" (subagent isolado) |
| `agent` | TEXT | | subagent type quando context=fork (string, não JSON) |
| `model` | TEXT | | Modelo preferido |
| `allowed_tools` | TEXT | | String bruta "Read, Grep" (preserva fidelidade do frontmatter) |
| `tools_config` | TEXT | | JSON: `ToolPermissions` resolvida (allowed/denied/bashCommands) |
| `filesystem_config` | TEXT | | JSON: *FilesystemPermissions |
| `network_config` | TEXT | | JSON: *NetworkPermissions |
| `input_spec` | TEXT | | JSON: *InputConfig |
| `output_spec` | TEXT | | JSON: *OutputConfig |
| `behavior_config` | TEXT | | JSON: *BehaviorConfig |
| `triggers_config` | TEXT | | JSON: *TriggerConfig |
| `hooks_config` | TEXT | | JSON: hooks (any) |
| `dependencies_config` | TEXT | | JSON: *DependenciesConfig |
| `mcp_config` | TEXT | | JSON: *MCPConfig |
| `content` | TEXT | NOT NULL | Corpo Markdown (após frontmatter) |
| `is_builtin` | BOOL | NOT NULL DEFAULT false INDEX | |
| `builtin_version` | TEXT | | Versão para seed update |
| `is_customized` | BOOL | NOT NULL DEFAULT false | |
| `created_at` | DATETIME | | |
| `updated_at` | DATETIME | | |

### `skill_tools` (junction: permissão de tool por skill — allowed/denied)

| Coluna | Tipo | Constraints | Notas |
|--------|------|-------------|-------|
| `id` | TEXT | PK | UUIDv7 |
| `skill_id` | TEXT | FK→skills.id NOT NULL INDEX | Cascade delete |
| `tool_name` | TEXT | NOT NULL | Nome da tool |
| `relation` | TEXT | NOT NULL | "allowed" ou "denied" (de `ToolPermissions`) |

**Constraint**: UNIQUE(skill_id, tool_name, relation)

### Índices

- `idx_skills_slug` — UNIQUE em `skills.slug`
- `idx_skills_auto_load` — em `skills.auto_load` (filtro frequente)
- `idx_skill_tools_skill` — em `skill_tools.skill_id`

---

## Fases

### Fase 1 — Models GORM + AutoMigrate ✅ (PR desta fase)

1. Criar `internal/database/models_skills.go`:
   - `database.Skill` com `UUIDModel` + todas as colunas (convenção do projeto: sem sufixo `Model`, como `database.Job`/`database.MCPServer`)
   - `database.SkillTool` para a junction (allowed/denied), FK cascade
   - GORM tags para índices (`ux_skills_slug`, índice em `auto_load`/`is_builtin`/`category`/`type`; `ux_skill_tools_identity` em skill_id+tool_name+relation)
2. Funções de conversão em `internal/skills/dbmodel.go`:
   - `skillToModel(*Skill) → *database.Skill` (com junction `Tools` derivada de `tools_config`)
   - `skillFromModel(*database.Skill) → *Skill` (`tools_config` é a fonte de verdade; junction ignorada)
   - `skillInfoFromModel(*database.Skill) → SkillInfo`
   - Helpers `marshalJSONField`/`unmarshalJSONField` preservam `omitempty` (nil/empty → coluna vazia) para roundtrip fiel
3. Adicionar `&Skill{}` e `&SkillTool{}` ao `AutoMigrate`
4. Teste de roundtrip (`dbmodel_test.go`): Skill (cheio e mínimo) → model → Skill com `reflect.DeepEqual` no `SkillMetadata`

### Fase 2 — Repository layer ✅ (PR desta fase)

4. Criar `internal/skills/repository.go` com interface (todos os métodos recebem
   `ctx` por consistência com Jobs/MCP; skills **não** são user-scoped):

```go
type Repository interface {
    List(ctx context.Context) ([]SkillInfo, error)
    Get(ctx context.Context, slug string) (*Skill, error)
    GetByID(ctx context.Context, id string) (*Skill, error)
    Create(ctx context.Context, skill *Skill) (string, error)
    Update(ctx context.Context, slug string, skill *Skill) error
    Delete(ctx context.Context, slug string) error
    Duplicate(ctx context.Context, slug string) (string, error)
    ExistsBySlug(ctx context.Context, slug string) (bool, error)
    GetAutoSkills(ctx context.Context) ([]Skill, error)
    GetAvailableSkills(ctx context.Context) ([]Skill, error)
    GetAllSkillsFull(ctx context.Context) ([]Skill, error)
    GetUserInvocableSkills(ctx context.Context) ([]SkillInfo, error)
    SeedBuiltin(ctx context.Context, skill *Skill, version string) error
}
```

5. Implementar `DBRepository` com `*gorm.DB`:
   - `GetAutoSkills`: `WHERE auto_load = true AND disable_model_invocation = false` (= `IsAutoLoad()`)
   - `GetAvailableSkills`: `WHERE auto_load = false OR disable_model_invocation = true` (= `!IsAutoLoad()`, espelha o Manager filesystem para não deixar skills órfãos)
   - `GetUserInvocableSkills`: `WHERE user_invocable IS NULL OR user_invocable = true`
   - `Create`/`Update`/`Delete`: transação; `skill_tools` regravada via `replaceSkill` (delete-all + recreate); `Update` preserva colunas gerenciadas (is_builtin/builtin_version/is_customized/slug)
   - `SeedBuiltin`: versionamento semver (não existe → insere; customizado → skip; versão maior → atualiza)
   - `ExistsBySlug`: usado pela importação legada idempotente (Fase 5)

### Fase 3 — Migrar Manager para Repository ✅ (PR desta fase)

6. `Manager` ganha campo `repo Repository` + `SetRepository(repo)` (padrão de `mcp.Manager.SetRepository`) — **dual-mode**: sem repo = filesystem (comportamento atual preservado), com repo = DB. Isso permite mergear sem alterar o runtime; o corte acontece na Fase 6.
7. CRUD e listagens delegam ao Repository quando `repo != nil`
8. Manter `GetSkillFiles(slug)` usando filesystem (D4)
9. Manter `Parse()`, `Compose()`, `Invoke()`, `SubstituteArguments()` sem mudança
10. `NewManager()` permanece filesystem por default; App injeta o repo na Fase 6

### Fase 4 — Seed de builtins ✅ (PR desta fase)

11. Criar `internal/skills/seed.go`:
    - `SeedBuiltinSkills(ctx, repo Repository, fsys fs.FS, baseDir string) (SeedResult, error)`
    - Recebe `fs.FS` (testável via `testing/fstest`; produção = `embed.FS` do binário)
    - Layout `{slug}/SKILL.md`; parseia → `repo.SeedBuiltin()`; erros por skill não abortam o lote
12. Integração no `App.startup()` (injeção do embed) acontece na Fase 6

### Fase 5 — Importação legada filesystem → DB (não-destrutiva, D9) ✅ (PR desta fase)

13. `internal/skills/legacy_import.go`: `LegacySkillSource` (reaproveita `discoverAll()`, read-only) + `ImportLegacySkills(ctx)` via `ImportLegacyResourcesWithContext[*Skill]`:
    - `Source`: lista os `SKILL.md` descobertos (prioridade workdir > home > exe) **sem** renomear/apagar/reescrever; `ReadLegacyImportFile` é `os.ReadFile`.
    - `Parse`: `skills.Parse()` (slug = nome do diretório).
    - `Import`: idempotente — `ExistsBySlug` → skip; senão `Create` (com `skill_tools`).
14. Registrado em `runPostLoginLegacyImports`, guardado por `skillMgr.HasRepository()` (no-op até o corte da Fase 6).
15. Testes: criação, **não-destrutividade** (arquivo original intacto) e **idempotência** (re-import = skipped).

### Fase 6 — Corte do runtime para o DB ✅ (PR desta fase, backend-only)

15. `initSkills()` injeta `NewDBRepository(database.DB())` no Manager (`SetRepository`) quando há banco; `Manager` passa a servir skills do DB. Sem DB, fallback filesystem (`installBuiltinSkills`).
16. Seed de builtins no boot via `SeedBuiltinSkills(WithBootstrap(...))`; importação legada ativada pós-login (`HasRepository()` agora é true).
17. `discoverAll()` permanece apenas como `Source` da importação legada (D9) e para `GetSkillFiles` (D4).
18. **API Wails mantida estável neste PR**: `SkillInfo.Source` continua presente (mapeado de `is_builtin` por `skillInfoFromModel`: builtin→"exe", senão "home"); `GetSkillSearchPaths` continua funcionando (resolver permanece). Sem mudança no frontend.

### Fase 6b — Limpeza de API e frontend (fase dedicada, separada)

19. `SkillInfo.Source` → `SkillInfo.IsBuiltin` (Go + bindings Wails + frontend).
20. Remover `GetSkillSearchPaths()` do Controller/App/binding e ajustar `SkillsPage`/`ProfileSkillsSection` (coluna "Builtin/Custom", remover UI de paths) respeitando i18n/a11y/tokens de tema.

> **Motivo do split.** O corte de runtime (Fase 6) é backend-only e de baixo risco. O rename de campo + remoção de `GetSkillSearchPaths` tocam bindings gerados e o frontend (com regras estritas de i18n/a11y/tema), então ficam isolados na Fase 6b para revisão própria.

### Fase 7 — Testes

20. `repository_test.go`:
    - CRUD completo com skill_tools
    - Roundtrip: Skill → DB → Skill (deep equal)
    - Filtros: autoload, available, user-invocable
    - SeedBuiltin versionado
21. `legacy_import_test.go`:
    - SKILL.md → DB (importação)
    - Multi-dir dedup (prioridade workdir > home > exe)
    - Idempotência (re-import = skipped) e não-destrutividade (originais intactos)
22. `seed_test.go`:
    - 6 builtins inseridos
    - Update versionado, skip customizado
23. `manager_test.go`: Adaptar para DBRepository

---

## Mapeamento de dados: SKILL.md → DB

### Frontmatter YAML → colunas em `skills`

| Campo YAML | Coluna | Transformação |
|------------|--------|---------------|
| `name` | `slug`, `name` | Direto (kebab-case = slug) |
| `version` | `version` | Direto |
| `description` | `description` | Direto |
| `display-name` | `display_name` | Direto |
| `author` | `author` | Direto |
| `license` | `license` | Direto |
| `auto-load` | `auto_load` | Direto |
| `disable-model-invocation` | `disable_model_invocation` | Direto |
| `user-invocable` | `user_invocable` | Direto (nullable) |
| `context` | `skill_context` | Direto |
| `agent` | `agent` | Direto (string: subagent type) |
| `argument-hint` | `argument_hint` | Direto |
| `model` | `model` | Direto |
| `keywords`/`audience`/`platforms`/`languages`/`frameworks` | colunas homônimas | slice → JSON array |
| `category`/`subcategory`/`type`/`difficulty`/`minVersion`/`maxVersion` | colunas homônimas | Direto |
| `triggers`/`hooks` | `triggers_config`/`hooks_config` | → JSON |
| `allowed-tools` (string), `tools` (lista ou objeto) | `tools_config` + `skill_tools` | `ResolveToolsRaw` → `ToolPermissions` (JSON em `tools_config`); junction `skill_tools` populada de `allowed`/`denied` (relation="allowed"/"denied") |
| `filesystem` | `filesystem_config` | *FilesystemPermissions → JSON |
| `network` | `network_config` | *NetworkPermissions → JSON |
| `dependencies` | `dependencies_config` | *DependenciesConfig → JSON |
| `mcp` | `mcp_config` | *MCPConfig → JSON |
| `input` | `input_spec` | *InputSpec → JSON |
| `output` | `output_spec` | *OutputSpec → JSON |
| `behavior` | `behavior_config` | *BehaviorConfig → JSON |

### Corpo Markdown → `skills.content`

Tudo abaixo do frontmatter `---` vai para `content TEXT`.

---

## Arquivos afetados

### Novos

| Arquivo | Descrição |
|---------|-----------|
| `internal/database/models_skills.go` | 2 models GORM (`database.Skill`, `database.SkillTool`) |
| `internal/skills/dbmodel.go` | Conversões domínio↔model + helpers JSON |
| `internal/skills/repository.go` | Interface `Repository` + `DBRepository` |
| `internal/skills/seed.go` | Seed de builtins no DB |
| `internal/skills/legacy_import.go` | Importação legada não-destrutiva SKILL.md → DB (D9) |
| `internal/skills/repository_test.go` | Testes do repository |
| `internal/skills/legacy_import_test.go` | Testes da importação legada (idempotência, não-destrutividade) |
| `internal/skills/seed_test.go` | Testes do seed |

### Modificados

| Arquivo | Mudança |
|---------|---------|
| `internal/skills/manager.go` | Servir do Repository; `discoverAll()` reaproveitado como Source da importação legada |
| `internal/skills/types.go` | `SkillInfo.Source` → `SkillInfo.IsBuiltin` |
| `internal/database/database.go` | AutoMigrate de 2 tabelas |
| `internal/app/app_legacy_imports.go` | Registrar importador de skills ao lado de MCP/Jobs |
| `controllers/skills_controller.go` | Remover `GetSkillSearchPaths()` |
| `main.go` / `app.go` | Substituir `initSkills()` por seed + importação legada + Repository |

### Sem alteração

| Arquivo | Motivo |
|---------|--------|
| `internal/skills/types.go` (structs) | SkillMetadata permanece igual |
| `internal/skills/parser.go` | Parse/Compose para import/export |
| `internal/skills/invoke.go` | Invocação, template, preprocessing |
| `internal/prompt/builder.go` | BuildSkillsSection() — recebe skills como antes |
| Frontend | Mesma API Wails |

---

## Critérios de aceitação

1. **CRUD funcional**: Criar, ler, atualizar, deletar skills via DB
2. **Roundtrip fiel**: Skill → DB → Skill produz deep equal
3. **Filtros eficientes**: `GetAutoSkills()`, `GetAvailableSkills()`, `GetUserInvocableSkills()` via queries SQL (não scan + filter)
4. **Seed de builtins**: 6 skills no DB no primeiro boot
5. **Versionamento protegido**: Builtins atualizados; customizados preservados
6. **Importação não-destrutiva e idempotente**: SKILL.md existentes → DB sem renomear/apagar os originais; re-executar não duplica nem altera (já existente = `skipped`)
7. **Import/export preservado**: `Parse()` e `Compose()` continuam funcionando
8. **Skill files**: Arquivos complementares continuam acessíveis via filesystem
9. **Testes**: Cobertura para repository, migração, seed

---

## Riscos

| # | Risco | Probabilidade | Impacto | Mitigação |
|---|-------|---------------|---------|-----------|
| R1 | Perda de skills na importação | Baixa | Alto | Importação não-destrutiva (originais intactos no filesystem) e idempotente (D9) |
| R2 | Tools polymorphism (3 formatos) | Média | Médio | `tools_config` JSON para inline defs + `skill_tools` para nomes |
| R3 | Conteúdo Markdown muito grande | Baixa | Baixo | TEXT sem limite no SQLite; monitoring de tamanho |
| R4 | Skill files complementares órfãos | Média | Baixo | `GetSkillFiles()` continua usando filesystem; migração futura |
| R5 | Frontmatter YAML parsing edge cases | Baixa | Médio | Validação loose no import; testes com skills reais |

---

## Relação com AEP-0050 e AEP-0072

Esta AEP é **independente** e implementável sozinha. Ordem de dependência:

1. **AEP-0046** — UUIDv7 (infraestrutura base, já mergeada)
2. **AEP-0051** — Skills DB (esta AEP) — fundação de dados
3. **AEP-0072** — Skill Catalog & Loading (catálogo, descoberta, gating e carregamento sob demanda) — consome esta AEP

A **AEP-0050 (Profiles DB) foi adiada** (implementação rastreada por issue própria). Quando for implementada, poderá adicionar FK em `profile_skills.skill_slug → skills.slug` para integridade referencial. Nada nesta AEP depende da AEP-0050.
