# AEP-0050 — Migração de Profiles para Banco de Dados

**Status**: Proposta  
**Criado em**: 2026-04-21  
**Depende de**: AEP-0046 (UUIDv7 Migration)  
**Relacionado**: AEP-0025 (Interaction Profiles), AEP-0027 (Profiles Refactor), AEP-0044 (Profile Settings Revamp), AEP-0048 (Jobs DB), AEP-0049 (MCP DB)

---

## Resumo

Migrar os profiles de interação do armazenamento atual em arquivos JSON no filesystem (com resolução multi-diretório via `configdir.Resolver`) para o banco de dados SQLite centralizado via GORM. A migração usa UUIDv7 como chave primária (dependência AEP-0046), mantém o slug como identificador humano estável e único, persiste configs complexas (Chat, Voice, Input, Channels, MediaSupport) em colunas JSON, implementa seed de profiles builtin no DB com versionamento protegido contra sobrescrita de customizações do usuário, e elimina completamente o sistema de resolução multi-diretório.

---

## Motivação

### Problemas do modelo atual

1. **Resolução multi-diretório**: Profiles são buscados em 3 diretórios (exe, home, workdir) com deduplicação por prioridade. Isso gera complexidade de debug, comportamento imprevisível quando o mesmo slug existe em mais de um diretório, e dificuldade para consultas programáticas.

2. **Sem consistência transacional**: Operações como "desativar todos + ativar um" envolvem múltiplas escritas em disco sem atomicidade. Crash no meio pode deixar 0 ou 2 profiles ativos.

3. **Sem integridade referencial**: Channels e workspace referenciam profiles por slug, mas não há validação de que o slug existe. Deletar um profile não limpa referências órfãs.

4. **Performance**: Listar profiles requer ler N arquivos JSON de 3 diretórios, deserializar cada um, e deduplicar. Com banco, é uma query indexada.

5. **Alinhamento com stack**: Conversas, mensagens, providers e credenciais já estão no SQLite. Profiles é o último subsistema significativo que ainda usa filesystem puro. AEP-0048 (jobs) e AEP-0049 (MCP) seguem o mesmo caminho.

6. **Preparação para features futuras**: Versionamento de profiles, compartilhamento entre dispositivos, e import/export (AEP-0047) ficam muito mais simples com banco.

---

## Estado Atual

### Armazenamento

- **Formato**: JSON, um arquivo por profile (`{slug}.json`)
- **Diretórios** (via `configdir.Resolver`):
  1. `<exe>/.assistente/profiles/` — profiles embeddados no binário (read-only)
  2. `~/.assistente/profiles/` — profiles do usuário (read-write, prioridade para escrita)
  3. `./.assistente/profiles/` — profiles do workspace (prioridade mais alta para leitura)
- **Builtins**: 5 profiles embeddados em `assets/builtin/profiles/` (padrao, canais-comunicacao, editor-texto, modelo-local, programacao)
- **Versionamento de builtins**: Campo `_builtin_version` no JSON. App atualiza builtins se versão embeddada > instalada. Usuário pode "travar" com `"999.0.0"`.

### Structs principais

```go
type Profile struct {
    BuiltinVersion string         // Versão do builtin
    Name           string         // Nome legível (obrigatório)
    Description    string         // Descrição opcional
    Icon           string         // Identificador de ícone
    Active         bool           // Apenas 1 ativo por vez
    Chat           ChatConfig     // Config LLM (provider, model, temperature, tools, skills...)
    Voice          VoiceConfig    // Config TTS (3 roles: assistant, user, system)
    Input          InputConfig    // Config STT + triggers (hotkey, wakeword, VAD...)
    Channels       ChannelsConfig // Comportamento em canais externos
    MediaSupport   *MediaSupport  // Capacidades de mídia do modelo (auto-detectado)
}

type ProfileInfo struct {
    Name        string // Nome do profile
    Slug        string // Identificador derivado do nome
    Description string
    Icon        string
    Source      string // "exe"|"home"|"workdir"
}
```

### Referências ao profile no sistema

| Local | Campo | Tipo | Descrição |
|-------|-------|------|-----------|
| Channels (`channels/*.json`) | `Profile` | `string` | Slug do profile para o canal (vazio = ativo global) |
| Workspace (`*.yaml`) | `Profile` | `string` | Slug do profile padrão do workspace |
| Workspace Tab | `ProfileOverride` | `map[string]any` | Override parcial de config por tab |
| Speech/TTS runtime | `profileSlug` | `string` | Passado em `OnSpeechRequest()` |
| LLM routing runtime | `ChatParams.ProfileSlug` | `string` | Usado em `resolveProfileDefaults()` |

### Manager (file I/O)

```go
type Manager struct {
    resolver *configdir.Resolver // Resolve profiles de 3 diretórios
}
```

Métodos: `List()`, `Get(slug)`, `Create(profile)`, `Update(slug, profile)`, `Delete(slug)`, `Duplicate(slug)`, `GetActive()`, `SetActive(slug)`, `GetActiveSlug()`, `GetSearchPaths()`.

### Eventos

- `profile:created` — novo profile adicionado
- `profile:updated` — profile existente modificado
- `profile:deleted` — profile removido
- `profile:changed` — profile ativo alterado (reinicializa speech/LLM/hotkeys)

---

## Decisões

### D1. Tudo no banco, eliminar multi-diretório

Profiles serão armazenados exclusivamente no SQLite. O sistema de resolução multi-diretório (`configdir.Resolver` para profiles) será eliminado. Isso simplifica drasticamente o modelo mental e remove classes inteiras de bugs de prioridade/deduplicação.

### D2. UUIDv7 como PK + slug como identificador humano

- **PK interna**: `id TEXT` com UUIDv7 (via `UUIDModel.BeforeCreate` do AEP-0046)
- **Identificador humano**: `slug TEXT UNIQUE NOT NULL` derivado do nome via `Slugify()`
- **Referências externas** (channels, workspace, runtime): continuam usando slug
- **Motivação**: UUID garante unicidade global para futuro import/export; slug garante legibilidade e estabilidade

### D3. Configs complexas em colunas JSON separadas

Cada sub-config do profile tem sua própria coluna TEXT com JSON serializado:
- `chat_config` — `ChatConfig` (provider, model, temperature, tools, skills...)
- `voice_config` — `VoiceConfig` (3 roles com provider, voice, rate, pitch, volume)
- `input_config` — `InputConfig` + `[]TriggerConfig` (STT + triggers)
- `channels_config` — `ChannelsConfig` (response mode)
- `media_support` — `*MediaSupport` (audio, image, document, video)

**Motivação**: Cada seção pode ser lida/escrita independentemente sem deserializar o profile inteiro. Colunas base (name, slug, active, icon) são queryáveis diretamente.

### D4. Builtin profiles via seed no DB

No primeiro boot (ou após update do app):
1. App lê profiles embeddados de `assets/builtin/profiles/*.json`
2. Para cada, chama `Repository.SeedBuiltin(profile, version)`:
   - Se profile não existe no DB → insere com `is_builtin=true`
   - Se existe + `is_customized=true` → **skip** (usuário editou, não sobrescrever)
   - Se existe + `builtin_version < embedded_version` → **update** (mantém `is_customized=false`)
   - Se existe + `builtin_version = "999.0.0"` → **skip** (locked pelo usuário)
3. Após seed, `ensureActiveProfile()` garante que ao menos 1 profile está ativo (fallback: "padrao")

### D5. Flag `is_customized` para proteger edições do usuário

Quando o usuário edita um profile builtin (via `Update()`), o campo `is_customized` é marcado como `true`. Isso impede que futuras atualizações do app sobrescrevam as customizações. O lock via `"999.0.0"` continua funcionando como mecanismo adicional.

### D6. Channels e workspace continuam por slug

Channels (`telegram.json`, `signal.json`) e workspace (`.yaml`) continuam referenciando profiles por slug string. Não há migração desses arquivos nesta AEP. Se/quando channels e workspace migrarem para DB (AEPs futuras), a referência pode evoluir para UUID.

### D7. Sem LLM tools para profiles

Profiles são configuração do usuário, não dados gerenciáveis por LLM. Diferente de jobs (AEP-0048), não faz sentido a LLM criar/editar profiles automaticamente. Acesso de leitura já existe via contexto do sistema.

### D8. MediaSupport persistido no banco

`MediaSupport` (capacidades de mídia auto-detectadas do modelo) será persistido como JSON no banco. Evita re-detecção a cada boot e permite que o frontend mostre ícones de capacidade sem chamar o backend.

### D9. Migração one-time com backup

Na primeira execução pós-update:
1. Detectar se `~/.assistente/profiles/*.json` existe **E** tabela `profiles` está vazia
2. Carregar todos os profiles JSON dos 3 diretórios (usando Resolver antigo, com dedup)
3. Inserir no DB via Repository
4. Identificar builtins (matching por slug com lista conhecida) e marcar `is_builtin=true`
5. Renomear diretório para `~/.assistente/profiles.migrated/` (backup)

### D10. SetActive transacional

`SetActive(slug)` executa em transação SQLite:
1. `UPDATE profiles SET active = false WHERE active = true`
2. `UPDATE profiles SET active = true WHERE slug = ?`
3. Se o slug não existe → rollback + erro

Isso garante atomicidade — impossível ter 0 ou 2 profiles ativos.

### D11. ProfileInfo.Source removido

O campo `Source` (`"exe"|"home"|"workdir"`) de `ProfileInfo` perde sentido com banco único. Será substituído por `IsBuiltin bool` no `ProfileInfo` para que o frontend possa distinguir builtins de profiles do usuário.

---

## Tabela: `profiles`

| Coluna | Tipo | Constraints | Notas |
|--------|------|-------------|-------|
| `id` | TEXT | PK | UUIDv7 via `UUIDModel.BeforeCreate` |
| `slug` | TEXT | UNIQUE NOT NULL INDEX | Derivado do nome via `Slugify()` |
| `name` | TEXT | NOT NULL | Nome legível do profile |
| `description` | TEXT | | Descrição opcional |
| `icon` | TEXT | | Identificador de ícone (ex: "chatbox") |
| `active` | BOOL | NOT NULL DEFAULT false | Apenas 1 ativo por vez (D10) |
| `is_builtin` | BOOL | NOT NULL DEFAULT false | Veio de `assets/builtin/` (D4) |
| `builtin_version` | TEXT | | Versão semver do builtin embeddado |
| `is_customized` | BOOL | NOT NULL DEFAULT false | Usuário editou um builtin (D5) |
| `chat_config` | TEXT | | JSON: `ChatConfig` completo |
| `voice_config` | TEXT | | JSON: `VoiceConfig` (3 roles) |
| `input_config` | TEXT | | JSON: `InputConfig` + `[]TriggerConfig` |
| `channels_config` | TEXT | | JSON: `ChannelsConfig` |
| `media_support` | TEXT | | JSON: `*MediaSupport` (D8) |
| `created_at` | DATETIME | | Via GORM |
| `updated_at` | DATETIME | | Via GORM |

### Índices

- `idx_profiles_slug` — UNIQUE em `slug` (lookup principal)
- `idx_profiles_active` — em `active` (busca do profile ativo)

---

## Fases

### Fase 1 — Model GORM + AutoMigrate

1. Criar `internal/database/models_profiles.go`:
   - `ProfileModel` com `UUIDModel` embeddado e todos os campos da tabela acima
   - GORM tags para nomes de coluna, constraints e índices
2. Funções de conversão:
   - `ProfileModelFromProfile(p *profiles.Profile, slug string) ProfileModel`
   - `(m *ProfileModel) ToProfile() *profiles.Profile`
   - `(m *ProfileModel) ToProfileInfo() profiles.ProfileInfo`
3. Adicionar `ProfileModel` ao `AutoMigrate` em `internal/database/database.go`

### Fase 2 — Repository layer

4. Criar `internal/profiles/repository.go` com interface:

```go
type Repository interface {
    List() ([]ProfileInfo, error)
    Get(slug string) (*Profile, error)
    GetByID(id string) (*Profile, error)
    GetActive() (*Profile, error)
    GetActiveSlug() (string, error)
    Create(profile *Profile) (string, error) // retorna slug
    Update(slug string, profile *Profile) error
    Delete(slug string) error
    SetActive(slug string) error
    SeedBuiltin(profile *Profile, version string) error
}
```

5. Implementar `DBRepository` com `*gorm.DB`:
   - `SetActive` usa transação (D10)
   - `Update` de builtin marca `is_customized=true` (D5)
   - `SeedBuiltin` implementa lógica de versionamento (D4)
   - `Delete` proíbe deletar o último profile ativo

### Fase 3 — Migrar Manager para Repository

6. Alterar `Manager` struct: adicionar campo `repo Repository`
7. Reescrever métodos CRUD para delegar ao `Repository`
8. `Duplicate(slug)`: `Get(slug)` + gerar novo nome + `Create(copy)`
9. Manter `Slugify()` e `Validate()` sem mudança
10. Atualizar `NewManager()` para receber `Repository`

### Fase 4 — Seed de builtins via DB

11. Criar `internal/profiles/seed.go`:
    - `SeedBuiltinProfiles(repo Repository, fs embed.FS) error`
    - Itera `assets/builtin/profiles/*.json`, deserializa, chama `SeedBuiltin()`
12. Criar `EnsureActiveProfile(repo Repository) error`:
    - Se nenhum profile ativo → `SetActive("padrao")`
    - Se "padrao" não existe → ativar o primeiro disponível
13. Integrar no `App.startup()` após `AutoMigrate`:
    - Chamar `SeedBuiltinProfiles()`
    - Chamar `EnsureActiveProfile()`

### Fase 5 — Migração one-time filesystem → DB

14. Criar `internal/profiles/migration.go`:
    - `MigrateFromFilesystem(repo Repository, resolver *configdir.Resolver) error`
    - Condição: diretório `~/.assistente/profiles/` existe **E** tabela vazia
    - Carregar todos profiles via Resolver (3 dirs, deduplicação)
    - Inserir no DB; identificar builtins por slug conhecido
    - Renomear diretório para `profiles.migrated/`
15. Chamar migração no startup **antes** do seed:
    1. `MigrateFromFilesystem()` — importa customizações existentes
    2. `SeedBuiltinProfiles()` — garante builtins presentes/atualizados
    3. `EnsureActiveProfile()` — garante um ativo

### Fase 6 — Remoção de código filesystem

16. Remover `resolver *configdir.Resolver` do `Manager` struct
17. Remover métodos de file I/O do Manager (read/write JSON)
18. Remover `GetSearchPaths()` do Manager e do `ProfilesController`
19. Substituir `installBuiltinProfiles()` do `App` pelo seed via DB (Fase 4)
20. Atualizar `ProfileInfo.Source` → `ProfileInfo.IsBuiltin` (D11)
21. Limpar imports não utilizados (os, path/filepath, configdir)

### Fase 7 — Testes

22. `internal/profiles/repository_test.go`:
    - CRUD completo (Create, Get, List, Update, Delete)
    - `SetActive` transacional (verifica atomicidade)
    - `SeedBuiltin` com versionamento (insert, update, skip customized, skip locked)
    - Roundtrip de configs JSON (save → load → compare deep-equal)
23. `internal/profiles/migration_test.go`:
    - Migração de JSON para DB (single dir, multi-dir)
    - Deduplicação multi-diretório
    - Backup de diretório (`profiles.migrated/`)
    - Migração idempotente (não re-importa se DB não está vazio)
24. `internal/profiles/seed_test.go`:
    - Seed de builtins (insert novo, update versão, skip customizado, skip locked)
    - `EnsureActiveProfile` (nenhum ativo → ativa padrao)
25. Atualizar `internal/profiles/manager_test.go`:
    - Usar `DBRepository` com SQLite in-memory
    - Manter cenários de teste existentes (validação, duplicate, etc.)

---

## Mapeamento de dados: JSON → DB

| Campo JSON | Coluna DB | Transformação |
|------------|-----------|---------------|
| `_builtin_version` | `builtin_version` | Direto |
| `name` | `name` | Direto |
| — | `slug` | Derivado via `Slugify(name)` (antes era filename) |
| `description` | `description` | Direto |
| `icon` | `icon` | Direto |
| `active` | `active` | Direto |
| — | `id` | Novo UUIDv7 |
| — | `is_builtin` | `true` se slug ∈ {padrao, canais-comunicacao, editor-texto, modelo-local, programacao} |
| — | `is_customized` | `false` inicialmente; marcado ao editar builtin |
| `chat` | `chat_config` | `ChatConfig` → JSON string |
| `voice` | `voice_config` | `VoiceConfig` → JSON string |
| `input` | `input_config` | `InputConfig` → JSON string |
| `channels` | `channels_config` | `ChannelsConfig` → JSON string |
| `media_support` | `media_support` | `*MediaSupport` → JSON string (pode ser null) |

---

## Arquivos afetados

### Novos

| Arquivo | Descrição |
|---------|-----------|
| `internal/database/models_profiles.go` | Model GORM (`ProfileModel`) com tags e conversões |
| `internal/profiles/repository.go` | Interface `Repository` + `DBRepository` |
| `internal/profiles/seed.go` | Seed de builtins no DB |
| `internal/profiles/migration.go` | Migração one-time filesystem → DB |
| `internal/profiles/repository_test.go` | Testes do repository |
| `internal/profiles/migration_test.go` | Testes da migração |
| `internal/profiles/seed_test.go` | Testes do seed |

### Modificados

| Arquivo | Mudança |
|---------|---------|
| `internal/profiles/manager.go` | Refatorar para usar `Repository` em vez de `Resolver` |
| `internal/profiles/types.go` | `ProfileInfo`: `Source string` → `IsBuiltin bool` |
| `internal/database/database.go` | Adicionar `ProfileModel` ao `AutoMigrate` |
| `controllers/profiles_controller.go` | Remover `GetProfileSearchPaths()`, adaptar init |
| `main.go` / `app.go` | Substituir `installBuiltinProfiles()` por seed via DB |
| `internal/profiles/manager_test.go` | Adaptar para usar `DBRepository` (SQLite in-memory) |

### Código removido (dentro de arquivos existentes)

| Arquivo | Código removido |
|---------|----------------|
| `internal/profiles/manager.go` | File I/O via `Resolver` (Read/Write JSON), `GetSearchPaths()` |
| `controllers/profiles_controller.go` | `GetProfileSearchPaths()` |
| `main.go` / `app.go` | `installBuiltinProfiles()` (substituído por seed.go) |

### Sem alteração

| Arquivo | Motivo |
|---------|--------|
| `internal/profiles/types.go` | Structs `Profile`, `ChatConfig`, `VoiceConfig`, etc. permanecem iguais |
| `internal/profiles/slugify.go` | Função `Slugify()` sem mudança |
| `internal/profiles/validate.go` | Validação sem mudança |
| Frontend (store, componentes, páginas) | Mesma API Wails, apenas backed por DB agora |

---

## Critérios de aceitação

1. **CRUD funcional**: Criar, ler, atualizar, deletar profiles via DB — mesma API para o frontend
2. **Seed de builtins**: App abre pela primeira vez → 5 profiles builtin presentes no DB
3. **Versionamento protegido**: Update do app atualiza builtins não-customizados; preserva customizados
4. **Migração transparente**: Usuário com profiles JSON existentes → dados migrados para DB no primeiro boot pós-update, backup criado
5. **SetActive atômico**: Impossível ter 0 ou 2+ profiles ativos
6. **Channels/workspace**: Continuam funcionando com slug sem alteração
7. **Testes**: Cobertura para repository (CRUD, SetActive, SeedBuiltin), migração (filesystem→DB), e seed
8. **Performance**: `List()` mais rápido que scan de 3 diretórios + N deserializações JSON

---

## Riscos

| # | Risco | Probabilidade | Impacto | Mitigação |
|---|-------|---------------|---------|-----------|
| R1 | Perda de profiles na migração filesystem→DB | Baixa | Alto | Backup automático em `profiles.migrated/`, migração idempotente |
| R2 | Builtin update sobrescreve customização do usuário | Baixa | Médio | Flag `is_customized` protege edições. Lock via `"999.0.0"` continua |
| R3 | Multi-dir resolution quebra para users com profiles em workdir | Média | Médio | Migração lê dos 3 diretórios e importa tudo para DB |
| R4 | Serialização JSON de configs perde precisão de tipos | Baixa | Médio | Testes de roundtrip (save→load→compare) para todos os tipos |
| R5 | Channels/workspace referem slug que mudou | Muito baixa | Alto | Slug permanece estável (derivado do nome), sem breaking change |
| R6 | Frontend quebra com mudança em ProfileInfo | Baixa | Baixo | `Source` → `IsBuiltin` é rename simples; adaptar store/componentes |
