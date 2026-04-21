# AEP-0052 — Sistema de Contas de Usuário

**Status**: Proposta  
**Criado em**: 2026-04-21  
**Depende de**: AEP-0047 (Import/Export)  
**Precede**: AEP-0046 (UUIDv7 Migration)  
**Relacionado**: AEP-0014 (Credential Persistence), AEP-0022 (Welcome Wizard), AEP-0026 (Credential Fixes)

---

## Resumo

Introduzir um sistema de contas de usuário no aplicativo, atualmente 100% single-user. Cada usuário terá uma conta local com isolamento criptográfico completo (DEK própria, master key própria, recovery key própria). Todos os recursos do sistema — provedores LLM, conversas, credenciais, listas de tarefas — serão vinculados a um `user_id`.

A implementação mantém a experiência local transparente (auto-login via keyring) enquanto prepara a arquitetura para:
- Multi-user local (contas compartilhando a mesma máquina)
- Futuro deployment em nuvem (server remoto com login convencional)
- Social login com provedores externos (Google, GitHub, Microsoft)

Social login é **apenas previsto na modelagem** (campos e interfaces), sem implementação OAuth2 nesta AEP.

---

## Motivação

### 1. Preparação para nuvem

O app atualmente só roda localmente. Para oferecer modo cloud (servidor remoto), é indispensável ter identidade de usuário. Esta AEP cria a fundação sem forçar migração prematura.

### 2. Isolamento de recursos

Sem `user_id`, todos os recursos (conversas, providers, credenciais) são globais. Em cenário multi-user (local ou cloud), não há como separar dados entre usuários.

### 3. Isolamento criptográfico

O sistema de credenciais usa uma DEK (Data Encryption Key) global. Com contas, cada usuário tem sua própria DEK envolvida por sua própria senha. Um usuário não consegue descriptografar credenciais de outro — nem com acesso direto ao banco.

### 4. Pré-requisito para migrações DB

As AEPs 0046-0051 migram recursos para banco de dados. Ter `user_id` disponível antes dessas migrações evita retrabalho (adicionar coluna depois + backfill).

---

## Estado Atual

### Autenticação

- **Zero**: Não existe conceito de usuário, conta, login ou sessão
- O app abre direto; a única "autenticação" é a master key para descriptografar credenciais
- Master key armazenada no keyring do SO; se keyring falhar, pede senha novamente

### Recursos sem owner

| Recurso | Armazenamento | User ID | Tabela/Local |
|---------|---------------|---------|--------------|
| LLM Providers | DB | ❌ | `llm_providers` |
| Conversas | DB | ❌ | `conversations` |
| Mensagens | DB | herdado | `chat_messages` (via conversation) |
| Credenciais | DB (criptografado) | ❌ | `credential_entries` |
| Key Wraps | DB | ❌ | `credential_key_wraps` |
| Task Lists | DB | ❌ | `task_lists` |
| Tasks | DB | parcial | `tasks` (tem `creator_id`, `assignee_id`) |
| Task Notes | DB | parcial | `task_notes` (tem `author_id`) |
| Profiles | Filesystem JSON | ❌ | `~/.assistente/profiles/` |
| Skills | Filesystem MD | ❌ | `~/.assistente/skills/` |
| MCP Servers | Filesystem JSON | ❌ | `~/.assistente/mcp/` |

### Welcome Wizard atual

6 etapas: Senha Mestre → Recovery Key → Escolher Provider → URL → API Key → Modelo

Detecção: `NeedsWelcomeWizard()` retorna true se não há master key OU não há providers.

---

## Decisões

### D1. Senha da conta = Master Key (senha unificada)

Uma única senha serve para:
1. **Autenticação**: Hash Argon2id armazenado em `users.password_hash` — verifica identidade
2. **Criptografia**: Argon2id com salt diferente → chave que wraps a DEK — protege credenciais

O usuário não precisa memorizar duas senhas. A segurança não é reduzida porque os derivados usam salts independentes.

**Fluxo de login**:
```
senha → Argon2id(senha, salt_auth) → compara com password_hash   ✓ identidade
senha → Argon2id(senha, salt_wrap) → unwrap DEK                  ✓ criptografia
```

### D2. DEK por usuário (isolamento total)

Cada conta tem:
- Sua própria **DEK** (32 bytes aleatórios, AES-256)
- Seu próprio **master key wrap** (DEK envolvida pela senha do user)
- Sua própria **recovery key** (192 bits, exibida uma vez)

Credenciais de um usuário são inacessíveis a outro, mesmo com acesso ao banco.

Keyring do SO: `("assistente", "credential_dek_{user_id}")` — uma entrada por user.

### D3. Auto-login via keyring (modo local)

No modo local com um único usuário:
1. App abre → tenta carregar DEK do keyring
2. DEK encontrada → identifica user associado → login automático
3. DEK não encontrada → exibe tela de login (username + senha)

A experiência para quem já usa o app **não muda** — continua abrindo direto.

### D4. Social login: apenas modelagem

Campos na tabela `users`:
- `auth_provider TEXT DEFAULT 'local'` — valores futuros: `google`, `github`, `microsoft`
- `provider_user_id TEXT` — ID do usuário no provedor externo

Nenhuma implementação OAuth2 nesta AEP. Interface `SocialAuthProvider` criada com stubs.

### D5. Wizard: Etapa 0 vira "Criar Conta"

O welcome wizard atual começa com "Criar Senha Mestre". Com contas:

| Etapa | Antes | Depois |
|-------|-------|--------|
| 0 | Criar senha mestre | **Criar conta** (username + senha) |
| 1 | Exibir recovery key | Exibir recovery key (sem mudança) |
| 2-5 | Configurar provider | Configurar provider (sem mudança) |

`NeedsWelcomeWizard()` adiciona check: `users count == 0`.

A criação da conta na Etapa 0 também:
- Gera DEK + recovery key wraps
- Salva DEK no keyring
- Define o user como admin (`is_admin=true`)
- Configura credential manager com a DEK do user

### D6. Vinculação incremental de recursos

Recursos **no banco** recebem `user_id` nesta AEP:
- `llm_providers`, `conversations`, `credential_entries`, `credential_key_wraps`, `task_lists`

Recursos **em filesystem** (profiles, skills, MCP) receberão `user_id` quando migrarem para DB nas AEPs 0049-0051. Esta AEP **não** os modifica.

### D7. Migração de dados existentes

Para instalações já em uso (upgrade):
1. Criar um "default user" a partir dos dados existentes
2. Wizard pede ao user existente para "adotar" seus dados criando uma conta
3. Backfill `user_id` em todos os registros com o ID do default user
4. Se há key wraps existentes, vinculá-los ao default user

### D8. Herança de user_id por hierarquia

Nem toda tabela precisa de `user_id` direto:

| Tabela | Estratégia |
|--------|-----------|
| `conversations` | `user_id` direto |
| `chat_messages` | Herda via `conversation_id` FK |
| `task_lists` | `user_id` direto |
| `tasks` | Herda via `task_list_id` FK |
| `task_notes` | Herda via `task_id → task_list_id` |
| `task_list_workflows` | Herda via `task_list_id` FK |

### D9. Escopo de queries (session context)

Após login, o `user_id` fica disponível no contexto da sessão. Todas as queries de recursos devem filtrar por `user_id`:

```go
// Antes
db.Find(&providers)

// Depois
db.Where("user_id = ?", currentUserID).Find(&providers)
```

Implementado via middleware/helper no repository layer, não por filtro manual em cada query.

---

## Tabelas

### `users` (nova)

| Coluna | Tipo | Constraints | Notas |
|--------|------|-------------|-------|
| `id` | TEXT | PK | UUIDv7 |
| `username` | TEXT | UNIQUE NOT NULL | Login identifier, max 64 chars |
| `display_name` | TEXT | | Nome de exibição (fallback: username) |
| `email` | TEXT | | Futuro social login |
| `password_hash` | TEXT | NOT NULL | Argon2id hash |
| `auth_provider` | TEXT | NOT NULL DEFAULT 'local' | 'local', 'google', 'github', 'microsoft' |
| `provider_user_id` | TEXT | | ID externo (social login) |
| `is_admin` | BOOL | NOT NULL DEFAULT false | Primeiro user é admin |
| `is_active` | BOOL | NOT NULL DEFAULT true | Soft disable |
| `avatar_url` | TEXT | | Futuro |
| `last_login_at` | DATETIME | | Último login bem-sucedido |
| `created_at` | DATETIME | | |
| `updated_at` | DATETIME | | |

**Índices**:
- `idx_users_username` — UNIQUE em `username`
- `idx_users_auth_provider` — em `(auth_provider, provider_user_id)` para social login lookup

### Alterações em tabelas existentes

| Tabela | Coluna adicionada | Tipo | Constraints |
|--------|-------------------|------|-------------|
| `llm_providers` | `user_id` | TEXT | FK→users.id, INDEX |
| `conversations` | `user_id` | TEXT | FK→users.id, INDEX |
| `credential_entries` | `user_id` | TEXT | FK→users.id |
| `credential_key_wraps` | `user_id` | TEXT | FK→users.id |
| `task_lists` | `user_id` | TEXT | FK→users.id, INDEX |

**Mudanças de constraints**:
- `credential_entries`: unique muda de `(pattern)` para `(user_id, pattern)`
- `credential_key_wraps`: unique muda de `(kind)` para `(user_id, kind)`

---

## Fases

### Fase 1 — Tabela `users` + AuthService

1. Criar `internal/database/models_users.go`:
   - `UserModel` com todos os campos da tabela
   - GORM tags para índices e constraints
2. Adicionar `UserModel` ao `AutoMigrate` em `database.go`
3. Criar `internal/auth/service.go`:
   ```go
   type AuthService interface {
       CreateUser(ctx context.Context, req CreateUserRequest) (*User, error)
       Authenticate(ctx context.Context, username, password string) (*User, error)
       GetCurrentUser(ctx context.Context) (*User, error)
       GetUserByID(ctx context.Context, id string) (*User, error)
       GetUserByUsername(ctx context.Context, username string) (*User, error)
       SetCurrentUser(user *User)
       UserCount(ctx context.Context) (int64, error)
   }
   ```
4. Implementar `LocalAuthService` com `*gorm.DB`:
   - `CreateUser`: gera UUIDv7, hash Argon2id, insere no DB
   - `Authenticate`: busca por username, verifica hash, atualiza `last_login_at`
   - `GetCurrentUser`: retorna user em memória (set após login/auto-login)
5. Criar `internal/auth/types.go`:
   ```go
   type User struct {
       ID           string
       Username     string
       DisplayName  string
       Email        string
       AuthProvider string
       IsAdmin      bool
       IsActive     bool
   }

   type CreateUserRequest struct {
       Username    string
       Password    string
       DisplayName string
   }
   ```

### Fase 2 — Credential system per-user

6. `credential_key_wraps`: adicionar `user_id TEXT` + migração
7. `credential_entries`: adicionar `user_id TEXT` + migração
8. `internal/credentials/keyring.go`:
   - `SaveDEKToKeychain(userID string, dek []byte)` — chave: `"credential_dek_{userID}"`
   - `LoadDEKFromKeychain(userID string)` — busca por user
   - `LoadAnyDEKFromKeychain()` — fallback para auto-login (busca o único user)
9. `internal/credentials/store.go`:
   - Interface `Store` ganha `userID` nos métodos relevantes:
     - `SaveCredential(ctx, userID, cred)` / `ListCredentials(ctx, userID)` / `DeleteCredential(ctx, userID, pattern)`
     - `SaveKeyWrap(ctx, userID, wrap)` / `GetKeyWrap(ctx, userID, kind)` / `HasKeyWrap(ctx, userID, kind)`
10. `internal/credentials/manager.go`:
    - `NewManagerWithStore(userID, dek, store, persist)` — scoped ao user
    - Todas as operações internas passam `userID` para o store
11. `internal/credentials/master_key.go`:
    - `SetupMasterKey(store, userID, password)` — associa wraps ao user

### Fase 3 — Vincular recursos ao user

12. Adicionar `user_id TEXT` (nullable) a `llm_providers`, `conversations`, `task_lists`
13. Criar migração one-time (`internal/auth/migration.go`):
    - Detecta: tabela `users` vazia + registros existentes sem `user_id`
    - Fluxo:
      a. Se há key wraps existentes → apresentar wizard de "adoção" (criar conta para dados existentes)
      b. Criar default user com dados fornecidos
      c. `UPDATE llm_providers SET user_id = ? WHERE user_id IS NULL`
      d. `UPDATE conversations SET user_id = ? WHERE user_id IS NULL`
      e. `UPDATE credential_entries SET user_id = ? WHERE user_id IS NULL`
      f. `UPDATE credential_key_wraps SET user_id = ? WHERE user_id IS NULL`
      g. `UPDATE task_lists SET user_id = ? WHERE user_id IS NULL`
    - Após backfill: adicionar NOT NULL constraint via GORM
14. Atualizar repositories/queries existentes para filtrar por `user_id`:
    - `ProviderService`: `List(userID)`, `Get(userID, providerID)`
    - Conversation queries: `WHERE user_id = ?`
    - TaskList queries: `WHERE user_id = ?`

### Fase 4 — Welcome wizard adaptado

15. Modificar `controllers/welcome_controller.go`:
    - `NeedsWelcomeWizard()`: adicionar `authSvc.UserCount() == 0`
    - Etapa 0 ("Criar Conta"):
      - Questionnaire com campos: `username`, `password` (2x), `display_name` (opcional)
      - `authSvc.CreateUser()` → user criado
      - `credentials.SetupMasterKey(store, user.ID, password)` → DEK + wraps
      - `credentials.SaveDEKToKeychain(user.ID, dek)` → keyring
      - `configureCredentialManager(user.ID, dek, persist)` → manager ativo
    - Etapas subsequentes: provider e credenciais vinculados ao `user.ID`
16. Modificar `internal/app/app_credentials.go`:
    - `initCredentialManager()`:
      - Tentar `LoadAnyDEKFromKeychain()` → se encontrou, identificar user → auto-login
      - Se não: aguardar login manual ou wizard

### Fase 5 — Frontend: gate de autenticação

17. Criar `frontend/src/store/authStore.ts`:
    ```typescript
    interface AuthState {
        currentUser: User | null;
        isAuthenticated: boolean;
        isLoading: boolean;
        login: (username: string, password: string) => Promise<void>;
        logout: () => void;
        checkAutoLogin: () => Promise<void>;
    }
    ```
18. Modificar `frontend/src/App.tsx`:
    - No startup: `checkAutoLogin()` → se ok, renderiza app normal
    - Se não autenticado e não precisa de wizard: exibir tela de login mínima
    - Se precisa de wizard: wizard (como hoje, mas com Etapa 0 de conta)
19. Criar `frontend/src/components/auth/LoginScreen.tsx`:
    - Campos: username + senha
    - Chama backend `Authenticate(username, password)`
    - Mínimo viável — será expandido quando social login for implementado
20. Adicionar strings i18n nos 3 locales (`pt-BR.ts`, `en.ts`, `es.ts`):
    - `auth.login`, `auth.username`, `auth.password`, `auth.loginButton`
    - `wizard.createAccount`, `wizard.chooseUsername`, etc.

### Fase 6 — Social login (modelagem apenas)

21. Criar `internal/auth/social.go`:
    ```go
    type SocialAuthProvider interface {
        Name() string
        Authenticate(ctx context.Context, token string) (*SocialUser, error)
        GetUserInfo(ctx context.Context, accessToken string) (*SocialUser, error)
    }

    type SocialUser struct {
        ProviderUserID string
        Email          string
        DisplayName    string
        AvatarURL      string
    }
    ```
22. Documentar providers planejados (Google, GitHub, Microsoft) com comentários no código

### Fase 7 — Testes

23. `internal/auth/service_test.go`:
    - CreateUser: sucesso, username duplicado, senha fraca (se houver política)
    - Authenticate: sucesso, username errado, senha errada, user inativo
    - GetCurrentUser: após login, sem login
24. `internal/credentials/` tests adaptados:
    - `manager_test.go`: DEK per user, operações scoped por user_id
    - `keyring_test.go`: save/load per user, LoadAnyDEK
    - `store_test.go`: credentials isoladas por user
    - `master_key_test.go`: SetupMasterKey com userID
25. `internal/auth/migration_test.go`:
    - Backfill de user_id em todas as tabelas
    - Default user criado corretamente
    - Constraints NOT NULL após backfill
26. `controllers/welcome_controller_test.go`:
    - Wizard Etapa 0 cria conta + master key + recovery key
    - Provider vinculado ao user
27. Frontend: `authStore.test.ts` — auto-login, login manual, logout

---

## Mapeamento: Wizard Antes → Depois

### Antes (Wizard atual)

```
Etapa 0: Criar Senha Mestre
  → input: password (2x)
  → ação: SetupMasterKey(password) → DEK + recovery

Etapa 1: Recovery Key
  → exibição readonly + confirmação

Etapa 2: Escolher Provider
  → single_choice: 15 opções

Etapa 3: URL Custom (se necessário)
  → input: URL + validação

Etapa 4: API Key
  → input: key + probe de conexão

Etapa 5: Modelo
  → single_choice ou input manual
```

### Depois (Wizard com contas)

```
Etapa 0: Criar Conta            ← MUDOU
  → input: username, display_name (opcional), password (2x)
  → ação: CreateUser() + SetupMasterKey(userID, password) → DEK + recovery
  → ação: SaveDEKToKeychain(userID, dek)
  → ação: SetCurrentUser(user)

Etapa 1: Recovery Key            ← SEM MUDANÇA
  → exibição readonly + confirmação

Etapa 2: Escolher Provider       ← SEM MUDANÇA (mas vincula ao userID)
Etapa 3: URL Custom              ← SEM MUDANÇA
Etapa 4: API Key                 ← SEM MUDANÇA (credencial vinculada ao userID)
Etapa 5: Modelo                  ← SEM MUDANÇA
```

---

## Fluxo de Auto-Login

```
┌─────────────────────────────────────────────────────────┐
│                    App Startup                           │
└─────────────────────┬───────────────────────────────────┘
                      │
                      ▼
              ┌───────────────┐
              │ users.count() │
              └───────┬───────┘
                      │
            ┌─────────┴─────────┐
            │ == 0              │ > 0
            ▼                   ▼
    ┌───────────────┐  ┌────────────────────┐
    │ Welcome Wizard│  │ LoadAnyDEKFromKR() │
    │ (Etapa 0:     │  └────────┬───────────┘
    │  Criar Conta) │           │
    └───────────────┘  ┌────────┴────────┐
                       │ DEK found       │ DEK not found
                       ▼                 ▼
              ┌─────────────────┐ ┌────────────────┐
              │ Identify user   │ │ Login Screen   │
              │ from keyring    │ │ (username +    │
              │ → auto-login    │ │  senha)        │
              │ → load app      │ └────────────────┘
              └─────────────────┘
```

---

## Fluxo de Migração (Upgrade de Instalação Existente)

```
┌──────────────────────────────────────────────────────┐
│               App Startup (upgrade)                   │
└──────────────────────┬───────────────────────────────┘
                       │
                       ▼
              ┌─────────────────┐
              │ users.count()==0 │
              │ AND              │
              │ key_wraps exist  │
              └────────┬────────┘
                       │ true
                       ▼
          ┌────────────────────────────┐
          │ Wizard "Adotar Dados"       │
          │                             │
          │ "Detectamos dados de uma    │
          │  instalação anterior.        │
          │  Crie uma conta para         │
          │  vincular seus dados."       │
          │                             │
          │ → username + senha (mesma   │
          │   master key atual)         │
          └────────────┬───────────────┘
                       │
                       ▼
          ┌────────────────────────────┐
          │ 1. CreateUser(username,pw) │
          │ 2. Vincular key_wraps      │
          │ 3. Backfill user_id:       │
          │    - llm_providers         │
          │    - conversations         │
          │    - credential_entries    │
          │    - task_lists            │
          │ 4. Auto-login              │
          └────────────────────────────┘
```

---

## Arquivos afetados

### Novos

| Arquivo | Descrição |
|---------|-----------|
| `internal/database/models_users.go` | Model GORM `UserModel` |
| `internal/auth/service.go` | Interface `AuthService` + `LocalAuthService` |
| `internal/auth/types.go` | Structs `User`, `CreateUserRequest` |
| `internal/auth/social.go` | Interface `SocialAuthProvider` (stubs) |
| `internal/auth/migration.go` | Migração: backfill user_id + default user |
| `internal/auth/service_test.go` | Testes do AuthService |
| `internal/auth/migration_test.go` | Testes da migração |
| `frontend/src/store/authStore.ts` | Zustand store de autenticação |
| `frontend/src/components/auth/LoginScreen.tsx` | Tela de login mínima |
| `frontend/src/components/auth/LoginScreen.css` | Estilos (variáveis do tema) |

### Modificados

| Arquivo | Mudança |
|---------|---------|
| `internal/database/database.go` | AutoMigrate de `UserModel`, migração colunas |
| `internal/database/models.go` | `UserID` em LLMProvider, Conversation, CredentialEntry, CredentialKeyWrap, TaskList |
| `internal/credentials/manager.go` | Recebe `userID`, operações scoped |
| `internal/credentials/keyring.go` | `SaveDEK/LoadDEK` com userID suffix |
| `internal/credentials/store.go` | Interface com `userID` nos métodos |
| `internal/credentials/master_key.go` | `SetupMasterKey` com `userID` |
| `internal/app/app_credentials.go` | `initCredentialManager` com auto-login |
| `controllers/welcome_controller.go` | Etapa 0 → criar conta, `NeedsWelcomeWizard` |
| `frontend/src/App.tsx` | Gate de autenticação, checkAutoLogin |
| `frontend/src/locales/pt-BR.ts` | Strings de auth |
| `frontend/src/locales/en.ts` | Strings de auth |
| `frontend/src/locales/es.ts` | Strings de auth |

### Sem alteração

| Arquivo | Motivo |
|---------|--------|
| `chat_messages` | Herda user via conversation |
| `tasks`, `task_notes` | Herda user via task_list |
| `task_list_workflows` | Herda user via task_list |
| Profiles/Skills/MCP (filesystem) | Migram para DB em AEPs separadas |

---

## Segurança

### Modelo de ameaças

| Ameaça | Mitigação |
|--------|-----------|
| Brute force de senha | Argon2id (3 iterações, 64 MB, 4 threads) para hash E wrap |
| Acesso ao DB sem senha | Credenciais criptografadas com DEK; DEK wrapped com senha |
| User A acessa dados de User B | DEK isolada por user; queries filtradas por user_id |
| Keyring comprometido | DEK no keyring é proteção de conveniência, não de segurança; a senha ainda é necessária para unwrap |
| Social login token roubado | Não implementado nesta AEP; quando for, usar PKCE + state parameter |

### Derivação de chaves (mesma senha, usos diferentes)

```
Senha do usuário
    │
    ├──▶ Argon2id(senha, salt_auth)  → password_hash    [autenticação]
    │    - Armazenado: users.password_hash
    │    - Parâmetros: t=3, m=64MB, p=4
    │
    └──▶ Argon2id(senha, salt_wrap)  → wrap_key → AES-GCM(DEK)  [criptografia]
         - Armazenado: credential_key_wraps.wrapped_dek
         - Salt independente: credential_key_wraps.salt
         - Parâmetros: t=3, m=64MB, p=4
```

Salts diferentes garantem que comprometer um derivado não compromete o outro.

---

## Critérios de aceitação

1. **Conta local funcional**: Criar conta com username + senha, login, auto-login via keyring
2. **DEK isolada**: Cada user tem DEK própria; credenciais de um user inacessíveis a outro
3. **Migração transparente**: Instalação existente → criar conta → dados vinculados → app funciona
4. **Wizard adaptado**: Etapa 0 cria conta; demais etapas sem mudança funcional
5. **Resources scoped**: Providers, conversas, credenciais, task lists filtrados por user_id
6. **Auto-login**: DEK no keyring → login automático sem pedir senha
7. **Login manual**: Se keyring vazio → tela de login funcional
8. **Social login previsto**: Campos na tabela, interface com stubs, sem OAuth2
9. **Backward compatible**: App com dados existentes migra sem perda
10. **Testes**: AuthService, credentials per-user, migration, wizard, frontend store

---

## Riscos

| # | Risco | Probabilidade | Impacto | Mitigação |
|---|-------|---------------|---------|-----------|
| R1 | Migração perde dados existentes | Baixa | Alto | Backfill idempotente, testes com dados reais, não deleta dados |
| R2 | Keyring indisponível (CI, containers) | Média | Médio | Fallback para login manual; já tratado no código atual |
| R3 | Performance de Argon2id no login | Baixa | Baixo | Parâmetros conservadores (3 iterações); aceitável para login |
| R4 | Complexidade de queries com user_id | Média | Médio | Helper/middleware centralizado para scope; não filtrar manualmente |
| R5 | Migração de constraint unique | Média | Médio | SQLite não suporta ALTER CONSTRAINT; usar GORM AutoMigrate com cuidado |
| R6 | Dois Argon2id por login (hash + unwrap) | Baixa | Baixo | Total ~1s em hardware moderno; cache DEK em memória |

---

## Relação com outras AEPs

| AEP | Relação |
|-----|---------|
| **0047** (Import/Export) | Precede esta. Export/import de recursos precisará considerar user_id |
| **0046** (UUIDv7) | Sucede esta. `users.id` já usa UUIDv7; demais tabelas migram depois |
| **0048** (Jobs DB) | Sucede esta. Tabela `jobs` terá `user_id` desde o início |
| **0049** (MCP DB) | Sucede esta. Tabela `mcp_servers` terá `user_id` desde o início |
| **0050** (Profiles DB) | Sucede esta. Tabela `profiles` terá `user_id` desde o início |
| **0051** (Skills DB) | Sucede esta. Tabela `skills` terá `user_id` desde o início |
| **0014** (Credential Persistence) | Estende com isolamento per-user |
| **0022** (Welcome Wizard) | Modifica Etapa 0 para criar conta |

### Ordem de implementação atualizada

```
AEP-0047 (Import/Export)
    ↓
AEP-0052 (Multi-User — esta)
    ↓
AEP-0046 (UUIDv7)
    ↓
AEP-0048 (Jobs DB)      ─┐
AEP-0049 (MCP DB)        │ (paralelo)
AEP-0051 (Skills DB)     │
    ↓                     │
AEP-0050 (Profiles DB) ──┘ (depende de 0051)
```
