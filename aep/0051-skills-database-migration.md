# AEP-0051 — Migração de Skills para Banco de Dados

**Status**: Proposta  
**Criado em**: 2026-04-21  
**Depende de**: AEP-0046 (UUIDv7 Migration)  
**Precede**: AEP-0050 (Profiles DB Migration)  
**Relacionado**: AEP-0025 (Interaction Profiles), AEP-0048 (Jobs DB), AEP-0049 (MCP DB)

---

## Resumo

Migrar o sistema de skills de SKILL.md em filesystem (multi-diretório via `configdir.Resolver`) para SQLite via GORM. Skills são instruções Markdown com metadados YAML (frontmatter) que personalizam o comportamento do LLM — desde coding assistants até job managers. Atualmente vivem em `~/.assistente/skills/{slug}/SKILL.md` com resolução de 3 diretórios.

A migração cria 2 tabelas (`skills` e `skill_permissions`), usa UUIDv7 como PK, mantém slug como identificador humano, seed de builtins (6 skills embeddados) com versionamento, e preserva compatibilidade com o formato SKILL.md para import/export. Deve ser implementada **antes** da AEP-0050 (Profiles DB), pois a junction table `profile_skills` na AEP-0050 referencia skills no banco.

---

## Motivação

### Problemas do modelo atual

1. **Filesystem disperso**: Skills em 3 diretórios (exe, home, workdir) com deduplicação por prioridade. Mesmo problema dos profiles.

2. **Parsing a cada load**: Cada `Get(slug)` lê arquivo do disco, parseia frontmatter YAML + Markdown, valida. Sem cache.

3. **Sem consultas eficientes**: Listar skills com filtro (autoload, user-invocable) requer ler e parsear todos os SKILL.md.

4. **Metadados ricos e estruturados**: SkillMetadata tem 40+ campos em 13 categorias. Armazenar como arquivo flat impede queries, validação por constraint, e versionamento.

5. **Pré-requisito para AEP-0050**: A junction table `profile_skills` na migração de profiles referencia skills por slug. Com skills no banco, é possível adicionar FK e garantir integridade.

6. **Alinhamento**: Jobs (AEP-0048), MCP (AEP-0049) e Profiles (AEP-0050) seguem o mesmo caminho.

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

### D1. Schema híbrido: colunas escalares + JSON para configs complexas

Skills têm campos com dois perfis distintos:

| Tipo | Estratégia | Exemplos |
|------|-----------|----------|
| Escalares queryáveis | Colunas diretas | name, version, description, auto_load, user_invocable |
| Configs complexas opcionais | JSON | filesystem, network, dependencies, mcp, agent, behavior |
| Conteúdo Markdown | Coluna TEXT | content (corpo do SKILL.md) |

**Justificativa**: Diferente de Voice Roles (3 rows fixos), as sub-configs de skills são opcionais (`*pointer`), raramente queryadas, e extremamente variáveis. Normalizar Filesystem/Network/MCP em tabelas separadas criaria 5+ tabelas pouco utilizadas. JSON é a escolha pragmática aqui.

**Exceção**: `skill_permissions` como tabela separada para tools necessárias/permitidas, pois são referências que beneficiam de junction table.

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

Skills declaram tools necessárias/permitidas de 3 formas:
- `tools: ["read_file", "write_file"]` — tools requeridas
- `allowed_tools: ["*"]` — wildcard
- `tools: [{name: "custom", description: "..."}]` — tool definitions inline

As referências simples (nomes de tools) vão para `skill_permissions` como junction table. Tool definitions inline (com description) ficam como JSON na coluna `tools_config`.

### D8. Sem multi-diretório após migração

Resolver multi-diretório eliminado. Skills ficam exclusivamente no banco. Mesmo padrão da AEP-0050.

---

## Tabelas

### `skills` (tabela principal)

| Coluna | Tipo | Constraints | Notas |
|--------|------|-------------|-------|
| `id` | TEXT | PK | UUIDv7 |
| `slug` | TEXT | UNIQUE NOT NULL INDEX | Nome kebab-case |
| `name` | TEXT | NOT NULL | Mesmo que slug (spec exige kebab-case) |
| `version` | TEXT | NOT NULL | Semver X.Y.Z |
| `description` | TEXT | NOT NULL | 10-160 chars |
| `display_name` | TEXT | | Nome legível (fallback: name) |
| `author` | TEXT | | |
| `license` | TEXT | | |
| `auto_load` | BOOL | NOT NULL DEFAULT false | Injetar no system prompt |
| `disable_model_invocation` | BOOL | NOT NULL DEFAULT false | |
| `user_invocable` | BOOL | | NULL = default(true) |
| `is_builtin` | BOOL | NOT NULL DEFAULT false | |
| `builtin_version` | TEXT | | Versão para seed update |
| `is_customized` | BOOL | NOT NULL DEFAULT false | |
| `content` | TEXT | NOT NULL | Corpo Markdown (após frontmatter) |
| `skill_context` | TEXT | | Fork de contexto (subagent) |
| `agent_config` | TEXT | | JSON: *AgentConfig |
| `tools_config` | TEXT | | JSON: tool definitions inline |
| `filesystem_config` | TEXT | | JSON: *FilesystemPermissions |
| `network_config` | TEXT | | JSON: *NetworkPermissions |
| `dependencies_config` | TEXT | | JSON: *DependenciesConfig |
| `mcp_config` | TEXT | | JSON: *MCPConfig |
| `input_spec` | TEXT | | JSON: *InputSpec |
| `output_spec` | TEXT | | JSON: *OutputSpec |
| `behavior_config` | TEXT | | JSON: *BehaviorConfig |
| `created_at` | DATETIME | | |
| `updated_at` | DATETIME | | |

### `skill_tools` (junction: skill ↔ tool requerida/permitida)

| Coluna | Tipo | Constraints | Notas |
|--------|------|-------------|-------|
| `id` | TEXT | PK | UUIDv7 |
| `skill_id` | TEXT | FK→skills.id NOT NULL INDEX | Cascade delete |
| `tool_name` | TEXT | NOT NULL | Nome da tool |
| `relation` | TEXT | NOT NULL | "required" ou "allowed" |

**Constraint**: UNIQUE(skill_id, tool_name, relation)

### Índices

- `idx_skills_slug` — UNIQUE em `skills.slug`
- `idx_skills_auto_load` — em `skills.auto_load` (filtro frequente)
- `idx_skill_tools_skill` — em `skill_tools.skill_id`

---

## Fases

### Fase 1 — Models GORM + AutoMigrate

1. Criar `internal/database/models_skills.go`:
   - `SkillModel` com `UUIDModel` + todos os campos da tabela
   - `SkillToolModel` para junction table
   - GORM tags para FK, cascade, índices
2. Funções de conversão:
   - `SkillModel + SkillToolModels → skills.Skill`
   - `skills.Skill → SkillModel + SkillToolModels`
   - `SkillModel → skills.SkillInfo`
3. Adicionar 2 models ao `AutoMigrate`

### Fase 2 — Repository layer

4. Criar `internal/skills/repository.go` com interface:

```go
type Repository interface {
    List() ([]SkillInfo, error)
    Get(slug string) (*Skill, error)
    GetByID(id string) (*Skill, error)
    Create(skill *Skill) (string, error)
    Update(slug string, skill *Skill) error
    Delete(slug string) error
    Duplicate(slug string) (string, error)
    GetAutoSkills() ([]Skill, error)
    GetAvailableSkills() ([]Skill, error)
    GetUserInvocableSkills() ([]SkillInfo, error)
    SeedBuiltin(skill *Skill, version string) error
}
```

5. Implementar `DBRepository` com `*gorm.DB`:
   - `GetAutoSkills`: `WHERE auto_load = true AND disable_model_invocation = false`
   - `GetAvailableSkills`: `WHERE auto_load = false`
   - `GetUserInvocableSkills`: `WHERE user_invocable IS NULL OR user_invocable = true`
   - `Create`/`Update`: Transação com upsert de skill_tools
   - `SeedBuiltin`: Lógica de versionamento (como profiles)

### Fase 3 — Migrar Manager para Repository

6. `Manager` struct recebe `repo Repository`
7. Reescrever CRUD para delegar ao Repository
8. Manter `GetSkillFiles(slug)` usando filesystem (D4)
9. Manter `Parse()`, `Compose()`, `Invoke()`, `SubstituteArguments()` sem mudança
10. Atualizar `NewManager()` para receber `Repository`

### Fase 4 — Seed de builtins

11. Criar `internal/skills/seed.go`:
    - `SeedBuiltinSkills(repo Repository, fs embed.FS) error`
    - Lê de `internal/app/builtin/skills/` + `assets/builtin/skills/`
    - Parseia SKILL.md → `SeedBuiltin()`
12. Integrar no `App.startup()` após AutoMigrate

### Fase 5 — Migração one-time filesystem → DB

13. Criar `internal/skills/migration.go`:
    - Detecta `~/.assistente/skills/` + tabela vazia
    - Carrega via Resolver (3 dirs, dedup)
    - Insere no DB incluindo skill_tools
    - Renomeia para `skills.migrated/`
14. Executar antes do seed no startup

### Fase 6 — Remoção de código filesystem

15. Remover `configdir.Resolver` do Manager
16. Remover `discoverAll()`, `GetSearchPaths()`, `EnsureDir()` (dir handling)
17. Substituir `initSkills()` no App por seed
18. `SkillInfo.Source` → `SkillInfo.IsBuiltin`
19. Remover `GetSkillSearchPaths()` do Controller

### Fase 7 — Testes

20. `repository_test.go`:
    - CRUD completo com skill_tools
    - Roundtrip: Skill → DB → Skill (deep equal)
    - Filtros: autoload, available, user-invocable
    - SeedBuiltin versionado
21. `migration_test.go`:
    - SKILL.md → DB
    - Multi-dir dedup
    - Backup
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
| `skill-context` | `skill_context` | Direto |
| `agent` | `agent_config` | *AgentConfig → JSON |
| `tools` (inline defs) | `tools_config` | JSON array |
| `tools` (names only) | → `skill_tools` | 1 row per tool, relation="required" |
| `allowed-tools` | → `skill_tools` | 1 row per tool, relation="allowed" |
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
| `internal/database/models_skills.go` | 2 models GORM (SkillModel, SkillToolModel) |
| `internal/skills/repository.go` | Interface `Repository` + `DBRepository` |
| `internal/skills/seed.go` | Seed de builtins no DB |
| `internal/skills/migration.go` | Migração one-time SKILL.md → DB |
| `internal/skills/repository_test.go` | Testes do repository |
| `internal/skills/migration_test.go` | Testes da migração |
| `internal/skills/seed_test.go` | Testes do seed |

### Modificados

| Arquivo | Mudança |
|---------|---------|
| `internal/skills/manager.go` | Usar Repository em vez de Resolver |
| `internal/skills/types.go` | `SkillInfo.Source` → `SkillInfo.IsBuiltin` |
| `internal/database/database.go` | AutoMigrate de 2 tabelas |
| `controllers/skills_controller.go` | Remover `GetSkillSearchPaths()` |
| `main.go` / `app.go` | Substituir `initSkills()` por seed |

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
6. **Migração transparente**: SKILL.md existente → DB → backup criado
7. **Import/export preservado**: `Parse()` e `Compose()` continuam funcionando
8. **Skill files**: Arquivos complementares continuam acessíveis via filesystem
9. **Testes**: Cobertura para repository, migração, seed

---

## Riscos

| # | Risco | Probabilidade | Impacto | Mitigação |
|---|-------|---------------|---------|-----------|
| R1 | Perda de skills na migração | Baixa | Alto | Backup em `skills.migrated/`, migração idempotente |
| R2 | Tools polymorphism (3 formatos) | Média | Médio | `tools_config` JSON para inline defs + `skill_tools` para nomes |
| R3 | Conteúdo Markdown muito grande | Baixa | Baixo | TEXT sem limite no SQLite; monitoring de tamanho |
| R4 | Skill files complementares órfãos | Média | Baixo | `GetSkillFiles()` continua usando filesystem; migração futura |
| R5 | Frontmatter YAML parsing edge cases | Baixa | Médio | Validação loose no import; testes com skills reais |

---

## Relação com AEP-0050

Esta AEP **precede** a AEP-0050 (Profiles DB). A sequência de implementação é:

1. **AEP-0046** — UUIDv7 (infraestrutura base)
2. **AEP-0051** — Skills DB (esta AEP)
3. **AEP-0050** — Profiles DB (usa `profile_skills` junction table referenciando skills)

Com skills no banco, a AEP-0050 pode adicionar FK em `profile_skills.skill_slug → skills.slug` para garantir integridade referencial.
