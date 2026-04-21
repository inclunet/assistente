# AEP-0050 — Migração de Profiles para Banco de Dados

**Status**: Proposta  
**Criado em**: 2026-04-21  
**Atualizado em**: 2026-04-21  
**Depende de**: AEP-0046 (UUIDv7 Migration), AEP-0051 (Skills DB Migration)  
**Relacionado**: AEP-0025 (Interaction Profiles), AEP-0027 (Profiles Refactor), AEP-0044 (Profile Settings Revamp), AEP-0048 (Jobs DB), AEP-0049 (MCP DB)

---

## Resumo

Migrar profiles de interação de JSON/filesystem (multi-diretório via `configdir.Resolver`) para SQLite via GORM com schema **totalmente normalizado**. Em vez de armazenar sub-configs como blobs JSON, cada estrutura determinística e consolidada tem sua própria tabela relacional: voice roles, triggers, tools habilitadas e skills habilitadas. Campos escalares de ChatConfig e InputConfig viram colunas diretas na tabela principal.

A migração usa UUIDv7 como PK (AEP-0046), slug como identificador humano, seed de builtins com versionamento protegido, e elimina completamente o sistema multi-diretório. Depende da AEP-0051 (Skills DB) que deve ser implementada antes, pois a junction table `profile_skills` referencia skills persistidos no banco.

---

## Motivação

### Problemas do modelo atual

1. **Resolução multi-diretório**: Profiles buscados em 3 diretórios (exe, home, workdir) com deduplicação por prioridade. Complexidade de debug, comportamento imprevisível com slugs duplicados.

2. **Sem consistência transacional**: "Desativar todos + ativar um" envolve múltiplas escritas em disco sem atomicidade. Crash pode deixar 0 ou 2 profiles ativos.

3. **Sem integridade referencial**: Channels e workspace referenciam por slug sem validação de existência.

4. **JSON blobs desperdiçam modelo relacional**: Voice roles são sempre 3, com campos fixos e bem definidos. Triggers têm tipos enumerados com campos determinísticos. Tools e skills são listas de referências — junction tables clássicas. Manter tudo como JSON impede queries, validação por constraint, e integridade referencial.

5. **Alinhamento com stack**: Conversas, mensagens, providers e credenciais já estão no SQLite. AEP-0048 (jobs) e AEP-0049 (MCP) seguem o mesmo caminho.

6. **Preparação para features futuras**: Import/export (AEP-0047), compartilhamento de profiles, versionamento.

---

## Estado Atual

### Armazenamento

- **Formato**: JSON, um arquivo por profile (`{slug}.json`)
- **Diretórios** (via `configdir.Resolver`):
  1. `<exe>/.assistente/profiles/` — builtins embeddados (read-only)
  2. `~/.assistente/profiles/` — profiles do usuário (read-write)
  3. `./.assistente/profiles/` — profiles do workspace (prioridade mais alta)
- **Builtins**: 5 profiles em `assets/builtin/profiles/` (padrao, canais-comunicacao, editor-texto, modelo-local, programacao)
- **Versionamento**: Campo `_builtin_version` no JSON; lock via `"999.0.0"`

### Structs principais

```go
type Profile struct {
    BuiltinVersion string
    Name           string         // Obrigatório
    Description    string
    Icon           string
    Active         bool           // Apenas 1 ativo
    Chat           ChatConfig     // ~15 campos escalares + EnabledTools + EnabledSkills
    Voice          VoiceConfig    // 3 roles fixos (assistant, user, system)
    Input          InputConfig    // 6 campos escalares + []TriggerConfig
    Channels       ChannelsConfig // 1 campo (ResponseMode)
    MediaSupport   *MediaSupport  // 4 campos nullable (auto-detectado)
}
```

### Referências no sistema

| Local | Campo | Tipo |
|-------|-------|------|
| Channels (`channels/*.json`) | `Profile` | slug string |
| Workspace (`*.yaml`) | `Profile` | slug string |
| Workspace Tab | `ProfileOverride` | `map[string]any` |
| Speech/TTS runtime | `profileSlug` | slug string |
| LLM routing | `ChatParams.ProfileSlug` | slug string |

---

## Decisões

### D1. Schema totalmente normalizado

Nenhum blob JSON. Cada estrutura determinística tem sua tabela:

| Dados | Estratégia | Justificativa |
|-------|-----------|---------------|
| Profile base (name, slug, icon, active) | Colunas na tabela `profiles` | Campos escalares queryáveis |
| ChatConfig (~15 campos) | Colunas na tabela `profiles` | Escalares determinísticos |
| InputConfig (6 campos STT) | Colunas na tabela `profiles` | Escalares determinísticos |
| ChannelsConfig (1 campo) | Coluna na tabela `profiles` | Escalar único |
| MediaSupport (4 campos) | Coluna JSON na tabela `profiles` | Auto-detectado, optional, mutável |
| Voice roles (3 fixos) | Tabela `profile_voice_configs` | 3 rows por profile, campos fixos |
| Triggers (N por profile) | Tabela `profile_triggers` | Lista variável, campos determinísticos |
| Enabled tools | Tabela `profile_tools` | Junction table (profile ↔ tool name) |
| Enabled skills | Tabela `profile_skills` | Junction table ordenada (profile ↔ skill slug) |

**Exceção**: `MediaSupport` permanece como JSON por ser auto-detectado, optional (`*MediaSupport`), e não ter semântica relacional.

### D2. UUIDv7 como PK + slug como identificador humano

- PK interna: `id TEXT` com UUIDv7 (AEP-0046)
- Identificador humano: `slug TEXT UNIQUE NOT NULL`
- Referências externas (channels, workspace): continuam por slug

### D3. Semântica nil vs vazio para tools e skills

A semântica atual de `EnabledTools` e `EnabledSkills` é:
- `nil` (ausência) → usar padrão (todas as tools / auto_load dos skills)
- `[]` (lista vazia) → desabilitar tudo
- `["a", "b"]` → lista explícita

No schema normalizado:
- **Ausência de registros na junction table + flag `use_default_tools=true`** → usar padrão
- **Ausência de registros + `use_default_tools=false`** → desabilitar tudo
- **Registros presentes na junction table** → lista explícita (flag ignorado)

Mesma lógica para skills com `use_default_skills`.

### D4. Builtin profiles via seed no DB

No boot:
1. Lê profiles embeddados de `assets/builtin/profiles/*.json`
2. Para cada: `SeedBuiltin(profile, version)`:
   - Não existe → insere com `is_builtin=true`
   - Existe + `is_customized=true` → skip
   - Existe + `builtin_version < embedded` → update
   - Existe + `builtin_version = "999.0.0"` → skip (locked)
3. `ensureActiveProfile()`: garante ao menos 1 ativo (fallback: "padrao")

### D5. Flag `is_customized` para proteger edições

`Update()` de profile builtin marca `is_customized=true`. Impede sobrescrita por futuras atualizações do app.

### D6. Channels e workspace continuam por slug

Sem migração nesta AEP.

### D7. Sem LLM tools para profiles

Profiles são configuração do usuário.

### D8. SetActive transacional

```sql
BEGIN;
  UPDATE profiles SET active = false WHERE active = true;
  UPDATE profiles SET active = true WHERE slug = ?;
COMMIT;
```

Atomicidade garantida.

### D9. ProfileInfo.Source → IsBuiltin

Campo `Source` (`"exe"|"home"|"workdir"`) substituído por `IsBuiltin bool`.

### D10. Migração one-time com backup

1. Detectar `~/.assistente/profiles/*.json` + tabela vazia
2. Carregar via Resolver (3 dirs, dedup)
3. Inserir no DB (incluindo tabelas filhas)
4. Renomear para `profiles.migrated/`

### D11. Depende de AEP-0051 (Skills DB)

A junction table `profile_skills` referencia skills por slug. Para integridade referencial futura, a AEP-0051 (Skills DB Migration) deve ser implementada antes ou em paralelo. Na fase inicial, `profile_skills.skill_slug` é apenas TEXT sem FK, mas o plano é adicionar FK quando skills estiverem no banco.

---

## Tabelas

### `profiles` (tabela principal)

| Coluna | Tipo | Constraints | Notas |
|--------|------|-------------|-------|
| `id` | TEXT | PK | UUIDv7 |
| `slug` | TEXT | UNIQUE NOT NULL INDEX | Derivado via `Slugify()` |
| `name` | TEXT | NOT NULL | Nome legível |
| `description` | TEXT | | |
| `icon` | TEXT | | Ex: "chatbox" |
| `active` | BOOL | NOT NULL DEFAULT false | 1 ativo por vez (D8) |
| `is_builtin` | BOOL | NOT NULL DEFAULT false | Veio de assets/builtin/ |
| `builtin_version` | TEXT | | Semver do builtin |
| `is_customized` | BOOL | NOT NULL DEFAULT false | Usuário editou builtin (D5) |
| — | — | **ChatConfig** | — |
| `llm_provider` | TEXT | | ID do provider ("openai-default", "$default") |
| `model` | TEXT | | Nome do modelo |
| `temperature` | REAL | | 0.0–2.0 |
| `max_tokens` | INT | | Limite de tokens |
| `max_tokens_mode` | TEXT | | "legacy" ou "completion_tokens" |
| `context_window` | INT | | 0 = auto |
| `max_context_messages` | INT | | Default 50 |
| `min_context_messages` | INT | | Default 10 |
| `top_p` | REAL | | 0.0–1.0 |
| `response_timeout` | INT | | Segundos (min 10) |
| `reasoning_effort` | TEXT | | off/low/medium/high/max/ollama |
| `disable_tools` | BOOL | NOT NULL DEFAULT false | Bloqueia all tool calling |
| `disable_skills` | BOOL | NOT NULL DEFAULT false | Desabilita injeção de skills |
| `disable_on_demand_skills` | BOOL | NOT NULL DEFAULT false | Só autoload |
| `use_default_tools` | BOOL | NOT NULL DEFAULT true | nil semantics (D3) |
| `use_default_skills` | BOOL | NOT NULL DEFAULT true | nil semantics (D3) |
| `command_allowlist` | TEXT | | Slug da allowlist |
| `max_agentic_iterations` | INT | | 0 = default 25 |
| — | — | **InputConfig** | — |
| `stt_enabled` | BOOL | NOT NULL DEFAULT false | Habilita voice input |
| `stt_provider` | TEXT | | "webspeech" / "whisper_api" |
| `stt_llm_provider_id` | TEXT | | Provider ID para Whisper |
| `stt_model` | TEXT | | "whisper-1" etc. |
| `stt_language` | TEXT | | "pt-BR", "en-US" |
| `stt_feedback_sounds` | BOOL | NOT NULL DEFAULT false | Beeps start/stop |
| — | — | **ChannelsConfig** | — |
| `channels_response_mode` | TEXT | | "mirror" / "always_text" / "always_audio" |
| — | — | **MediaSupport** | — |
| `media_support` | TEXT | | JSON: `*MediaSupport` (auto-detectado, nullable) |
| — | — | **Timestamps** | — |
| `created_at` | DATETIME | | |
| `updated_at` | DATETIME | | |

### `profile_voice_configs` (3 rows por profile)

| Coluna | Tipo | Constraints | Notas |
|--------|------|-------------|-------|
| `id` | TEXT | PK | UUIDv7 |
| `profile_id` | TEXT | FK→profiles.id NOT NULL INDEX | Cascade delete |
| `role` | TEXT | NOT NULL | "assistant" / "user" / "system" |
| `enabled` | BOOL | NOT NULL DEFAULT false | Habilita TTS para este role |
| `provider` | TEXT | | "disabled" / "webspeech" / "sapi5" / "openai" |
| `llm_provider_id` | TEXT | | Provider ID para credenciais |
| `voice_id` | TEXT | | Ex: "nova", "alloy" |
| `model` | TEXT | | Ex: "tts-1", "tts-1-hd" |
| `rate` | REAL | | 0.5–2.0 |
| `pitch` | REAL | | 0.5–2.0 |
| `volume` | REAL | | 0.0–1.0 |

**Constraint**: UNIQUE(profile_id, role)

### `profile_triggers` (N rows por profile)

| Coluna | Tipo | Constraints | Notas |
|--------|------|-------------|-------|
| `id` | TEXT | PK | UUIDv7 |
| `profile_id` | TEXT | FK→profiles.id NOT NULL INDEX | Cascade delete |
| `position` | INT | NOT NULL | Ordem no array original |
| `type` | TEXT | NOT NULL | hotkey/button_ptt/button_toggle/wakeword/vad |
| `enabled` | BOOL | NOT NULL DEFAULT false | |
| `auto_stop` | BOOL | NOT NULL DEFAULT false | VAD auto-stop |
| `hotkey` | TEXT | | "Ctrl+Shift+Space" |
| `hotkey_global` | BOOL | NOT NULL DEFAULT false | |
| `hotkey_bring_to_front` | BOOL | NOT NULL DEFAULT false | |
| `wakeword_keyword` | TEXT | | "assistente" |
| `wakeword_provider` | TEXT | | "webspeech" |
| `wakeword_sensitivity` | REAL | | 0.0–1.0 |
| `vad_silence_threshold` | REAL | | 0.0–1.0 |
| `vad_silence_duration` | INT | | ms |
| `vad_activity_threshold` | REAL | | 0.0–1.0 |
| `vad_activity_duration` | INT | | ms |

### `profile_tools` (junction: profile ↔ tool)

| Coluna | Tipo | Constraints | Notas |
|--------|------|-------------|-------|
| `id` | TEXT | PK | UUIDv7 |
| `profile_id` | TEXT | FK→profiles.id NOT NULL INDEX | Cascade delete |
| `tool_name` | TEXT | NOT NULL | Nome da tool no registry |

**Constraint**: UNIQUE(profile_id, tool_name)

### `profile_skills` (junction ordenada: profile ↔ skill)

| Coluna | Tipo | Constraints | Notas |
|--------|------|-------------|-------|
| `id` | TEXT | PK | UUIDv7 |
| `profile_id` | TEXT | FK→profiles.id NOT NULL INDEX | Cascade delete |
| `skill_slug` | TEXT | NOT NULL | Slug do skill |
| `position` | INT | NOT NULL | Ordem de autoload |

**Constraint**: UNIQUE(profile_id, skill_slug)

### Índices

- `idx_profiles_slug` — UNIQUE em `profiles.slug`
- `idx_profiles_active` — em `profiles.active`
- `idx_profile_voice_configs_profile` — em `profile_voice_configs.profile_id`
- `idx_profile_triggers_profile` — em `profile_triggers.profile_id`
- `idx_profile_tools_profile` — em `profile_tools.profile_id`
- `idx_profile_skills_profile` — em `profile_skills.profile_id`

---

## Fases

### Fase 1 — Models GORM + AutoMigrate

1. Criar `internal/database/models_profiles.go`:
   - `ProfileModel` — tabela `profiles` com todos os campos escalares + UUIDModel
   - `ProfileVoiceConfigModel` — tabela `profile_voice_configs`
   - `ProfileTriggerModel` — tabela `profile_triggers`
   - `ProfileToolModel` — tabela `profile_tools`
   - `ProfileSkillModel` — tabela `profile_skills`
   - GORM tags para FK, cascade, índices, constraints
2. Funções de conversão bidirecional:
   - `ProfileModel + filhas → profiles.Profile`
   - `profiles.Profile → ProfileModel + filhas`
   - `ProfileModel → profiles.ProfileInfo`
3. Adicionar 5 models ao `AutoMigrate` em `internal/database/database.go`

### Fase 2 — Repository layer

4. Criar `internal/profiles/repository.go` com interface:

```go
type Repository interface {
    List() ([]ProfileInfo, error)
    Get(slug string) (*Profile, error)
    GetByID(id string) (*Profile, error)
    GetActive() (*Profile, error)
    GetActiveSlug() (string, error)
    Create(profile *Profile) (string, error)
    Update(slug string, profile *Profile) error
    Delete(slug string) error
    SetActive(slug string) error
    SeedBuiltin(profile *Profile, version string) error
}
```

5. Implementar `DBRepository` com `*gorm.DB`:
   - `Get`/`GetActive`: Preload de todas as tabelas filhas
   - `Create`/`Update`: Transação com upsert de filhas (delete old + insert new)
   - `SetActive`: Transação atômica (D8)
   - `Update` de builtin: marca `is_customized=true` (D5)
   - `SeedBuiltin`: Lógica de versionamento (D4)
   - `Delete`: Cascade via GORM + proíbe deletar último ativo

### Fase 3 — Migrar Manager para Repository

6. `Manager` struct recebe `repo Repository`
7. Reescrever CRUD para delegar ao Repository
8. `Duplicate`: Get + Create com novo nome
9. Manter `Slugify()`, `Validate()` sem mudança
10. Atualizar `NewManager()` para receber `Repository`

### Fase 4 — Seed de builtins via DB

11. Criar `internal/profiles/seed.go`:
    - `SeedBuiltinProfiles(repo Repository, fs embed.FS) error`
    - Deserializa JSON → `Profile` → `SeedBuiltin()`
    - Inclui voice roles, triggers, tools, skills nas tabelas filhas
12. `EnsureActiveProfile(repo Repository) error`
13. Integrar no `App.startup()`

### Fase 5 — Migração one-time filesystem → DB

14. Criar `internal/profiles/migration.go`:
    - Detecta JSON no disco + tabela vazia
    - Carrega via Resolver (3 dirs, dedup)
    - Insere no DB incluindo todas as tabelas filhas
    - Mapeia arrays JSON → rows nas tabelas junction/filhas
    - Backup em `profiles.migrated/`
15. Executar antes do seed no startup

### Fase 6 — Remoção de código filesystem

16. Remover `configdir.Resolver` do Manager
17. Remover file I/O, `GetSearchPaths()`
18. Substituir `installBuiltinProfiles()` por seed
19. `ProfileInfo.Source` → `ProfileInfo.IsBuiltin` (D9)
20. Limpar imports

### Fase 7 — Testes

21. `repository_test.go`:
    - CRUD com todas as tabelas filhas
    - Roundtrip: Profile → DB → Profile (deep equal)
    - Voice roles: sempre 3 por profile
    - Triggers: CRUD de lista variável
    - Tools/skills: junction tables com semântica nil/vazio/lista (D3)
    - SetActive transacional
    - SeedBuiltin versionado
22. `migration_test.go`:
    - JSON→DB com normalização (voice/triggers/tools/skills)
    - Multi-dir dedup
    - Backup
23. `seed_test.go`:
    - Insert, update, skip customized, skip locked
    - Voice roles e triggers dos builtins presentes
24. `manager_test.go`: Adaptar para DBRepository

---

## Mapeamento de dados: JSON → DB normalizado

### Profile base

| Campo JSON | Tabela.Coluna | Transformação |
|------------|---------------|---------------|
| `_builtin_version` | `profiles.builtin_version` | Direto |
| `name` | `profiles.name` | Direto |
| (filename) | `profiles.slug` | `Slugify(name)` |
| `description` | `profiles.description` | Direto |
| `icon` | `profiles.icon` | Direto |
| `active` | `profiles.active` | Direto |
| — | `profiles.id` | Novo UUIDv7 |
| — | `profiles.is_builtin` | slug ∈ builtins conhecidos |

### ChatConfig → colunas em `profiles`

| Campo JSON | Coluna | Transformação |
|------------|--------|---------------|
| `chat.llm_provider` | `llm_provider` | Direto |
| `chat.model` | `model` | Direto |
| `chat.temperature` | `temperature` | Direto |
| `chat.max_tokens` | `max_tokens` | Direto |
| `chat.max_tokens_mode` | `max_tokens_mode` | Direto |
| `chat.context_window` | `context_window` | Direto |
| `chat.max_context_messages` | `max_context_messages` | Direto |
| `chat.min_context_messages` | `min_context_messages` | Direto |
| `chat.top_p` | `top_p` | Direto |
| `chat.response_timeout` | `response_timeout` | Direto |
| `chat.reasoning_effort` | `reasoning_effort` | Direto |
| `chat.disable_tools` | `disable_tools` | Direto |
| `chat.disable_skills` | `disable_skills` | Direto |
| `chat.disable_on_demand_skills` | `disable_on_demand_skills` | Direto |
| `chat.enabled_tools` | → tabela `profile_tools` | `nil` → `use_default_tools=true` + 0 rows; `[]` → `use_default_tools=false` + 0 rows; `["a"]` → 1 row |
| `chat.enabled_skills` | → tabela `profile_skills` | Idem com `use_default_skills` + `position` |
| `chat.command_allowlist` | `command_allowlist` | Direto |
| `chat.max_agentic_iterations` | `max_agentic_iterations` | Direto |

### VoiceConfig → tabela `profile_voice_configs`

| Campo JSON | Tabela.Coluna | Transformação |
|------------|---------------|---------------|
| `voice.assistant.*` | 1 row com `role="assistant"` | Flatten: cada campo → coluna |
| `voice.user.*` | 1 row com `role="user"` | Idem |
| `voice.system.*` | 1 row com `role="system"` | Idem |

### InputConfig → colunas em `profiles` + tabela `profile_triggers`

| Campo JSON | Destino | Transformação |
|------------|---------|---------------|
| `input.enabled` | `profiles.stt_enabled` | Direto |
| `input.stt_provider` | `profiles.stt_provider` | Direto |
| `input.llm_provider_id` | `profiles.stt_llm_provider_id` | Direto |
| `input.stt_model` | `profiles.stt_model` | Direto |
| `input.language` | `profiles.stt_language` | Direto |
| `input.feedback_sounds` | `profiles.stt_feedback_sounds` | Direto |
| `input.triggers[0..N]` | N rows em `profile_triggers` | Cada trigger → 1 row com `position` |

### ChannelsConfig → coluna em `profiles`

| Campo JSON | Coluna | Transformação |
|------------|--------|---------------|
| `channels.response_mode` | `channels_response_mode` | Direto |

### MediaSupport → JSON em `profiles`

| Campo JSON | Coluna | Transformação |
|------------|--------|---------------|
| `media_support` | `media_support` (JSON TEXT) | Serialização JSON (nullable) |

---

## Arquivos afetados

### Novos

| Arquivo | Descrição |
|---------|-----------|
| `internal/database/models_profiles.go` | 5 models GORM (Profile, VoiceConfig, Trigger, Tool, Skill) |
| `internal/profiles/repository.go` | Interface `Repository` + `DBRepository` |
| `internal/profiles/seed.go` | Seed de builtins no DB |
| `internal/profiles/migration.go` | Migração one-time filesystem → DB |
| `internal/profiles/repository_test.go` | Testes do repository |
| `internal/profiles/migration_test.go` | Testes da migração |
| `internal/profiles/seed_test.go` | Testes do seed |

### Modificados

| Arquivo | Mudança |
|---------|---------|
| `internal/profiles/manager.go` | Usar Repository em vez de Resolver |
| `internal/profiles/types.go` | `ProfileInfo.Source` → `ProfileInfo.IsBuiltin` |
| `internal/database/database.go` | AutoMigrate de 5 tabelas |
| `controllers/profiles_controller.go` | Remover `GetProfileSearchPaths()` |
| `main.go` / `app.go` | Substituir `installBuiltinProfiles()` por seed |
| `internal/profiles/manager_test.go` | Adaptar para DBRepository |

### Sem alteração

| Arquivo | Motivo |
|---------|--------|
| `internal/profiles/types.go` (structs) | Profile, ChatConfig, VoiceConfig, etc. permanecem iguais |
| `internal/profiles/slugify.go` | `Slugify()` sem mudança |
| `internal/profiles/validate.go` | Validação sem mudança |
| Frontend | Mesma API Wails |

---

## Critérios de aceitação

1. **CRUD funcional**: Criar, ler, atualizar, deletar profiles — com todas as 5 tabelas em sync
2. **Roundtrip fiel**: Profile → DB → Profile produz deep equal (sem perda de dados)
3. **Voice roles**: Sempre 3 rows por profile; impossível criar com mais ou menos
4. **Triggers**: Lista variável preservada com ordem (`position`)
5. **Tools/skills semântica**: `nil` vs `[]` vs lista explícita funciona como antes (D3)
6. **Seed de builtins**: 5 profiles com voice roles, triggers, tools e skills no DB no primeiro boot
7. **Versionamento protegido**: Builtins não-customizados atualizados; customizados preservados
8. **Migração transparente**: JSON existente → DB normalizado → backup criado
9. **SetActive atômico**: Impossível 0 ou 2+ ativos
10. **Testes**: Cobertura para repository, migração, seed, e Manager

---

## Riscos

| # | Risco | Probabilidade | Impacto | Mitigação |
|---|-------|---------------|---------|-----------|
| R1 | Perda de dados na migração | Baixa | Alto | Backup em `profiles.migrated/`, migração idempotente |
| R2 | Builtin update sobrescreve customização | Baixa | Médio | `is_customized` + lock `"999.0.0"` |
| R3 | Complexidade de CRUD com 5 tabelas | Média | Médio | Transações GORM com Preload/Association, testes extensivos de roundtrip |
| R4 | Performance de Preload em List() | Baixa | Baixo | List() retorna ProfileInfo (sem Preload); Preload só em Get() |
| R5 | Semântica nil/vazio de tools/skills | Média | Médio | Flags `use_default_tools`/`use_default_skills` + testes dedicados |
| R6 | Skills DB (AEP-0051) atrasada | Média | Baixo | `profile_skills.skill_slug` sem FK inicialmente; FK adicionada depois |
