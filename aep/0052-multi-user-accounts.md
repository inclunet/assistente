# AEP-0052 — Sistema de Contas de Usuário

**Status**: Em implementação  
**Criado em**: 2026-04-21  
**Depende de**: AEP-0046 (UUIDv7 Migration), AEP-0047 (Import/Export)  
**Precede**: AEP-0048 (Jobs DB), AEP-0049 (MCP DB), AEP-0050 (Profiles DB), AEP-0051 (Skills DB), AEP-0054/AEP-0055 (split/clientes)  
**Relacionado**: AEP-0014 (Credential Persistence), AEP-0022 (Welcome Wizard), AEP-0026 (Credential Fixes)

---

## Resumo

Introduzir um sistema de contas e autenticação no aplicativo, atualmente 100% single-user, com separação explícita entre:

- **Cofre (DEK global por instância)**: segredo de infraestrutura da instância (local ou remota) usado para criptografar credenciais e outros dados sensíveis do servidor. A DEK é persistida no keyring do SO quando possível, com recuperação via wraps `master`/`recovery` no banco.
- **Usuários (identidade)**: contas locais (modo 100% local) usadas para login, ownership lógico (`user_id`) e autorização por roles simples (`admin`/`user`).
- **Secret manager da instância**: reutiliza o `credentials.Manager` existente para segredos criptografados pela DEK, separando credenciais editáveis do usuário, segredos gerenciados por integrações e segredos internos da instância.

Esta AEP também introduz o primeiro passo real do “split”: no modo local, o backend passa a expor uma **API HTTP** para consumo por outros clientes/instâncias na rede.

Dois modos de autenticação/autorização são suportados:

- **`auth.mode=local`** (100% local): o Assistente emite e valida seus próprios tokens (JWT access + refresh token com rotação) e aplica roles `admin/user`.
- **`auth.mode=external`** (IdP existente): o Assistente atua como **resource server**, valida JWT do IdP via JWKS e aplica scopes/roles definidos externamente (sem sessão própria do Assistente nesta fase).

---

## Motivação

### 1. Preparação para nuvem

O app atualmente só roda localmente. Para oferecer modo cloud (servidor remoto), é indispensável ter identidade de usuário. Esta AEP cria a fundação sem forçar migração prematura.

### 2. Isolamento de recursos

Sem `user_id`, todos os recursos (conversas, providers, credenciais) são globais. Em cenário multi-user (local ou cloud), não há como separar dados entre usuários.

### 3. Isolamento criptográfico

O sistema de credenciais já usa uma DEK (Data Encryption Key) global. Nesta AEP, a DEK permanece **global por instância** (cofre de infraestrutura). O isolamento entre usuários é **lógico** (via `user_id` nas tabelas e enforcement em queries/handlers), não criptográfico por usuário.

### 4. Base para próximas migrações DB

AEP-0046 e AEP-0047 já estabelecem IDs UUIDv7 e o contrato portátil DB-only. A AEP-0052 adiciona ownership (`user_id`) aos recursos persistidos atuais e define o contrato que as próximas migrações DB (0048-0051) devem seguir desde o início, evitando adicionar isolamento de usuário depois que novos recursos forem movidos para o banco.

---

## Estado Atual

### Autenticação

- **Zero**: Não existe conceito de usuário, conta, login ou sessão
- O app abre direto; a única "autenticação" hoje é o cofre (DEK) para descriptografar credenciais
- A DEK é armazenada no keyring do SO quando disponível; se keyring falhar, há fluxo de recuperação via wraps `master`/`recovery`

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

Detecção: `NeedsWelcomeWizard()` retorna true se o cofre (DEK/wraps) não está inicializado OU não há providers.

---

## Decisões

### D1. Cofre (DEK global) é infraestrutura e é separado de usuários

- A DEK permanece **global por instância** (servidor local/daemon ou servidor remoto).
- O cofre é pré-requisito de infraestrutura para operar credenciais e segredos.
- Rotação de DEK: **fora de escopo** desta AEP.

### D2. Bootstrap obrigatório (ordem fixa)

1. **Inicializar o cofre** (antes de qualquer usuário): gerar DEK global, persistir wraps `master`/`recovery`, salvar DEK no keyring, exibir recovery key uma única vez.
2. **Criar o usuário admin local** (primeiro user).
3. Habilitar o restante (sessões, providers, recursos).

### D3. Estado `VaultLocked` (sem usuário)

Se a DEK não estiver disponível (keyring vazio + não houve unlock), o servidor entra em `VaultLocked` e expõe apenas endpoints mínimos (sem auth) para recuperar o cofre:

- `GET /vault/status`
- `POST /vault/unlock` (senha mestre do cofre ou recovery key)
- `POST /vault/setup` (apenas no primeiro uso, quando ainda não há wraps)

### D4. Dois modos de autenticação/autorização

- `auth.mode=local`: o Assistente é a autoridade de sessão e emite tokens próprios.
- `auth.mode=external`: o Assistente atua como **resource server**, valida JWT do IdP via JWKS e aplica scopes/roles do IdP.

Sem token exchange nesta fase.

### D5. Sessões locais com JWT access + refresh token

- Access token: JWT com expiração curta.
- Refresh token:
    - armazenado no keyring do SO no cliente
    - armazenado como **hash** no DB (`sessions.refresh_token_hash`)
    - **rotate always** no refresh
    - reuse detectado ⇒ revoga sessão inteira (`sid`) e exige novo login

### D6. Claims mínimas do JWT (access)

- Obrigatórias: `iss`, `aud`, `sub` (user_id), `sid` (session_id), `iat`, `exp`.
- Recomendada: `jti`.
- Defaults:
    - `exp`: 10–15 min
    - clock skew aceito: 60s

Não incluir PII no JWT.

### D7. Autorização local por roles simples

- `admin`: acesso total.
- `user`: acesso normal (sem operações administrativas).

No modo local, não listar usuários cadastrados: login é sempre por **username manual + senha**.

### D8. External mode: validação JWKS e enforcement por scopes/roles

- Validar JWT do IdP via JWKS.
- Enforce server-side por scopes/roles do token.
- Algoritmos aceitos devem ser controlados via allowlist (ex.: RS256/ES256/EdDSA conforme IdP).

### D9. API HTTP local (primeiro passo do split)

O backend local expõe uma API HTTP para consumo por clientes na rede.

Endpoints mínimos (local mode):

- `POST /auth/login` (username manual + senha; não expõe lista de usuários)
- `POST /auth/refresh`
- `POST /auth/logout`
- `GET /auth/me`
- `GET /.well-known/jwks.json`

Segurança:

- HTTPS é **obrigatório** quando o bind não for localhost.
- HTTP puro só permitido em `127.0.0.1` e/ou modo dev explícito.

### D10. Vinculação incremental de recursos por `user_id` (isolamento lógico)

Recursos no DB recebem `user_id` nesta AEP:

- `llm_providers`, `conversations`, `credential_entries`, `task_lists`

Recursos em filesystem não devem ser “prefixados” ad hoc por usuário nesta AEP. Enquanto não migram para DB, ficam classificados explicitamente:

| Recurso em disco | Classe | Decisão nesta AEP | Próximo passo correto |
|------------------|--------|-------------------|-----------------------|
| `conversations.db` | misto, com tabelas user-scoped e instance-scoped | Tabelas de dados de usuário exigem `user_id`; segredos internos usam `user_id=''` | manter guarda arquitetural para impedir chamadas DB sem contexto |
| `credential_key_wraps` / DEK / recovery | instância | Sem `user_id`; pré-requisito do servidor/local daemon | manter fora de listagens de credenciais do usuário |
| Refresh token no keyring do cliente | cliente/sessão local | Instância do cliente, não recurso compartilhado do servidor | no split, cada cliente guarda seu próprio refresh token |
| `profiles/*.json` | usuário | Não isolar por caminho nesta AEP para evitar árvore paralela frágil | migrar para DB em AEP-0050 com `user_id` obrigatório |
| `skills/` e skills customizadas | usuário | Não duplicar por pasta de usuário nesta AEP | migrar para DB em AEP-0051 com ownership e compartilhamento explícito |
| `mcp/*.json` e tokens OAuth MCP | usuário/integração | Config em arquivo ainda é legado; tokens ficam no secret manager com contexto do usuário quando acessados | migrar config para DB em AEP-0049 com `user_id` obrigatório |
| `channels/*.json`, contatos e mapeamento contato→conversa | usuário/integração | Legado em arquivo; ao tocar credenciais do canal, usar contexto autenticado | mover canais/contatos para DB ou tabela própria antes de modo multiusuário remoto |
| `jobs/` e logs de jobs | usuário/automação | Logs em disco ainda são legado local | migrar jobs para DB em AEP-0048 com `user_id` e retention |
| `workspace.yaml`, `workspaces/index.yaml` | cliente/UX local | Estado de UI do cliente, não autorização do servidor | no split, tratar como estado do cliente; IDs referenciados continuam validados no backend |
| `editor/state.json` e `editor/drafts/` | cliente/UX local | Estado local do cliente, pode conter conteúdo sensível | no split, manter local ao cliente ou migrar para recurso user-scoped se sincronizar |
| allowlists (`allowlists/*.json`) | usuário/perfil | Legado em arquivo vinculado ao perfil | migrar junto de profiles/skills ou referenciar por `user_id` no DB |
| config global (`config.json`, TLS, HTTP bind, updater) | instância | Sem `user_id` | manter como configuração operacional da instância |

Regra arquitetural: se o dado influencia autorização, histórico, credenciais, providers, tarefas, integrações ou conteúdo gerado pelo usuário, o destino correto é DB com `user_id`. Arquivos podem permanecer apenas para bootstrap da instância ou estado local do cliente, nunca como isolamento multiusuário definitivo.

### D11. Herança de `user_id` por hierarquia

| Tabela | Estratégia |
|--------|-----------|
| `conversations` | `user_id` direto |
| `chat_messages` | Herda via `conversation_id` FK |
| `task_lists` | `user_id` direto |
| `tasks` | Herda via `task_list_id` FK |
| `task_notes` | Herda via `task_id → task_list_id` |
| `task_list_workflows` | Herda via `task_list_id` FK |

### D12. Escopo de queries (token context)

Todas as queries de recursos devem filtrar por `user_id`, implementado via helper/middleware no repository layer.

### D13. `credentials.Manager` é o secret manager comum

O `credentials.Manager` permanece como mecanismo único de persistência criptografada por DEK, mas os patterns passam a ter classes explícitas:

| Classe | Exemplos | `user_id` | UI padrão |
|--------|----------|-----------|-----------|
| Credenciais de usuário | `api.openai.com`, `*.github.com` | obrigatório no runtime autenticado | visível/editável |
| Segredos gerenciados de integração | `mcp-client:{slug}`, `mcp-tokens:{slug}` | conforme integração; não editável manualmente | oculto |
| Segredos internos da instância | `internal-auth:jwt-signing-key`, `internal-tls:private-key`, `internal-tls:certificate` | vazio | oculto |

Segredos gerenciados e internos não aparecem na lista padrão de credenciais porque o usuário não deve editar/remover valores mantidos por fluxos automáticos. Telas específicas podem expor apenas metadados seguros (por exemplo, “MCP autenticado” ou validade do token), nunca o segredo.

### D14. Chave de assinatura JWT é segredo interno persistido

A chave Ed25519 usada para assinar access tokens deve ser criada uma vez por instância, criptografada pela DEK e armazenada no secret manager como `internal-auth:jwt-signing-key`.

Consequências:

- `/.well-known/jwks.json` permanece estável entre restarts.
- Access tokens emitidos antes de reiniciar continuam verificáveis até expirar.
- Rotação de chave JWT fica fora do escopo desta AEP, mas o modelo permite adicionar múltiplos `kid` no futuro.

---

## Tabelas

### `users` (nova)

| Coluna | Tipo | Constraints | Notas |
|--------|------|-------------|-------|
| `id` | TEXT | PK | UUIDv7 |
| `username` | TEXT | UNIQUE NOT NULL | Login identifier, max 64 chars |
| `display_name` | TEXT | | Nome de exibição (fallback: username) |
| `password_hash` | TEXT | NOT NULL | Argon2id hash (autenticação local) |
| `is_admin` | BOOL | NOT NULL DEFAULT false | Role simples (`admin`/`user`) |
| `is_active` | BOOL | NOT NULL DEFAULT true | Soft disable |
| `last_login_at` | DATETIME | | Último login bem-sucedido |
| `created_at` | DATETIME | | |
| `updated_at` | DATETIME | | |

**Índices**:
- `idx_users_username` — UNIQUE em `username`

### `sessions` (nova, modo local)

| Coluna | Tipo | Constraints | Notas |
|--------|------|-------------|-------|
| `id` | TEXT | PK | UUIDv7 |
| `user_id` | TEXT | FK→users.id, INDEX | Owner da sessão |
| `refresh_token_hash` | TEXT | UNIQUE | Hash do refresh token |
| `created_at` | DATETIME | | |
| `expires_at` | DATETIME | | Expiração do refresh token |
| `last_used_at` | DATETIME | | Último uso bem-sucedido |
| `revoked_at` | DATETIME | | Revogação (logout/reuse) |
| `client_label` | TEXT | | Identificador amigável (opcional) |

### Alterações em tabelas existentes

| Tabela | Coluna adicionada | Tipo | Constraints |
|--------|-------------------|------|-------------|
| `llm_providers` | `user_id` | TEXT | FK→users.id, INDEX |
| `conversations` | `user_id` | TEXT | FK→users.id, INDEX |
| `credential_entries` | `user_id` | TEXT | FK→users.id |
| `task_lists` | `user_id` | TEXT | FK→users.id, INDEX |

**Mudanças de constraints**:
- `credential_entries`: unique muda de `(pattern)` para `(user_id, pattern)`

---

## Fases

### Fase 0 — Cofre (DEK) + `VaultLocked`

1. Manter/estender o cofre global existente (DEK no keyring + wraps `master`/`recovery` no DB).
2. Implementar estado explícito `VaultLocked` quando a DEK não estiver disponível.
3. Expor endpoints mínimos (sem auth) para cofre:
   - `GET /vault/status`
   - `POST /vault/setup` (primeiro uso)
   - `POST /vault/unlock`

### Fase 1 — Usuários locais (admin/user)

4. Criar tabela `users` e `LocalIdentityService` (username/senha com Argon2id).
5. Criar o primeiro usuário local como admin **somente após o cofre**.
6. Definir roles simples `admin/user` e enforcement server-side.

### Fase 2 — Sessões locais + tokens

7. Criar tabela `sessions`.
8. Implementar `LocalSessionService`:
   - `IssueSession(userID) -> (access_jwt, refresh_token)`
   - `Refresh(refresh_token) -> (access_jwt, refresh_token_rotated)` (rotate always)
   - `Logout(refresh_token)` (revoga)
9. Implementar assinatura de JWT com Ed25519 persistida no secret manager e publicação de JWKS (`/.well-known/jwks.json`).

### Fase 3 — Scoping por `user_id`

10. Adicionar `user_id` em `llm_providers`, `conversations`, `credential_entries`, `task_lists`.
11. Atualizar repositories/queries para enforcement central de `user_id`.
12. Migração/backfill para instalações existentes.

### Fase 4 — HTTP API local + TLS

13. Rodar servidor `net/http` embutido no backend com endpoints `/vault/*`, `/auth/*` e `/.well-known/jwks.json`.
14. HTTPS obrigatório quando bind não for localhost; chave/certificado TLS, quando gerenciados pelo app, são segredos internos `internal-tls:*`.

### Fase 5 — Modo `external` (IdP)

15. Implementar validação JWKS (IdP) e enforcement por scopes/roles, sem token exchange.

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
Etapa 0: Inicializar Cofre (DEK global)   ← NOVO / OBRIGATÓRIO
  → input: senha mestre do cofre (2x)
  → ação: SetupMasterKey(...) → DEK global + wraps (master/recovery) + salva DEK no keyring

Etapa 1: Recovery Key                     ← SEM MUDANÇA
  → exibição readonly + confirmação

Etapa 2: Criar Admin Local                ← NOVO
  → input: username + password (2x)
  → ação: CreateUser(username, password) com is_admin=true

Etapa 3: Escolher Provider                ← SEM MUDANÇA
Etapa 4: URL Custom (se necessário)       ← SEM MUDANÇA
Etapa 5: API Key                          ← SEM MUDANÇA
Etapa 6: Modelo                           ← SEM MUDANÇA
```

---

## Fluxo de Auto-Login

```
┌─────────────────────────────────────────────────────────┐
│                    App Startup                           │
└─────────────────────┬───────────────────────────────────┘
                      │
                      ▼
        ┌────────────────────┐
        │ VaultLocked? (DEK) │
        └─────────┬──────────┘
      │
   ┌──────────────┴──────────────┐
   │ Sim                         │ Não
   ▼                             ▼
┌──────────────────────┐      ┌───────────────────────┐
│ Tela/fluxo de cofre   │      │ Refresh token existe? │
│ (/vault/setup/unlock) │      └─────────┬─────────────┘
└──────────┬───────────┘                │
     │                              │
     ▼                    ┌─────────┴─────────┐
   (Cofre destravado)           │ Sim               │ Não
        ▼                   ▼
           ┌───────────────────┐  ┌──────────────────┐
           │ /auth/refresh OK? │  │ Login Screen      │
           └─────────┬─────────┘  │ (username + senha)│
         │            └──────────────────┘
           ┌─────────┴─────────┐
           │ Sim               │ Não
           ▼                   ▼
    ┌───────────────┐   ┌──────────────────┐
    │ Auto-login     │   │ Login Screen      │
    │ (sem prompts)  │   │ (username + senha)│
    └───────────────┘   └──────────────────┘
```

---

## Fluxo de Migração (Upgrade de Instalação Existente)

```
┌──────────────────────────────────────────────────────┐
│               App Startup (upgrade)                   │
└──────────────────────┬───────────────────────────────┘
                       │
                       ▼
                ┌──────────────────────────┐
                │ Detecta dados legados     │
                │ (sem users + sem user_id) │
                └──────────┬───────────────┘
                     │
                     ▼
              ┌──────────────────────────────┐
              │ 1) Garantir cofre (DEK)      │
              │    - se keyring falhou:      │
              │      /vault/unlock (recovery)│
              └──────────┬───────────────────┘
                   │
                   ▼
              ┌──────────────────────────────┐
              │ 2) Criar admin local         │
              │    (username + senha)        │
              └──────────┬───────────────────┘
                   │
                   ▼
              ┌──────────────────────────────┐
              │ 3) Backfill user_id          │
              │    - llm_providers           │
              │    - conversations           │
              │    - credential_entries      │
              │    - task_lists              │
              └──────────────────────────────┘
```

---

## Arquivos afetados

### Novos

| Arquivo | Descrição |
|---------|-----------|
| `internal/database/models_users.go` | Model GORM `UserModel` |
| `internal/database/models_sessions.go` | Model GORM `SessionModel` (modo local) |
| `internal/auth/*` | Identidade local, sessões, JWT/JWKS, validadores externos, autorizadores |
| `internal/httpapi/*` | Servidor HTTP local + handlers `/vault/*`, `/auth/*`, `/.well-known/jwks.json` |
| `internal/auth/migration.go` | Migração/backfill de `user_id` + adoção de dados legados |
| `internal/config/auth.go` | Configuração `auth.mode`, HTTP API, TLS e IdP externo |
| `frontend/src/store/authStore.ts` | Store de autenticação (refresh/login/logout) |
| `frontend/src/components/auth/LoginScreen.tsx` | Tela de login (username manual + senha) |

### Modificados

| Arquivo | Mudança |
|---------|---------|
| `internal/database/database.go` | AutoMigrate de `UserModel`/`SessionModel` e migrações/backfill |
| `internal/database/models.go` | `user_id` em LLMProvider/Conversation/CredentialEntry/TaskList |
| `internal/credentials/*` | Mantém cofre global (DEK) + recovery + keyring (sem per-user) |
| `controllers/welcome_controller.go` | Bootstrap cofre → criar admin → resto do wizard |
| `frontend/src/App.tsx` | Gate de autenticação + fluxo VaultLocked/login |
| `frontend/src/locales/pt-BR.ts` | Strings de auth/vault |
| `frontend/src/locales/en.ts` | Strings de auth/vault |
| `frontend/src/locales/es.ts` | Strings de auth/vault |

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
| Brute force de senha (login local) | `users.password_hash` com Argon2id (parâmetros conservadores) |
| Enumeração de usuários | Sem listagem de usuários; login por username manual |
| Sniffing na LAN (credenciais/tokens) | HTTPS obrigatório fora de localhost |
| Roubo de refresh token | Hash no DB + rotação sempre + reuse revoga sessão |
| Acesso ao DB sem cofre | Credenciais permanecem criptografadas com DEK |
| Keyring indisponível | Fluxo `VaultLocked` + recovery via wraps `master`/`recovery` |
| Confusão de token (token para outro serviço) | `iss` + `aud` obrigatórios no JWT |
| Exposição acidental de segredos gerenciados | Patterns gerenciados ficam ocultos na UI padrão e bloqueados contra edição manual |
| Chave JWT efêmera | Chave Ed25519 persistida como segredo interno criptografado pela DEK |

---

## Critérios de aceitação

1. **Bootstrap do cofre**: DEK global + wraps `master/recovery` + keyring é inicializado antes de qualquer usuário.
2. **VaultLocked**: se a DEK não estiver disponível, `/vault/status` e `/vault/unlock` funcionam sem login.
3. **Admin local**: criação do primeiro usuário admin ocorre após o cofre.
4. **Login local**: login por username manual + senha (sem listagem de usuários) funciona.
5. **Sessões**: `sessions` persiste refresh token hash; logout revoga a sessão.
6. **Refresh rotation**: refresh rotaciona sempre; reuse revoga sessão inteira.
7. **JWT access**: JWT tem claims mínimas (`iss/aud/sub/sid/iat/exp`, `jti` recomendado) e expiração curta.
8. **JWT signing key**: chave Ed25519 persistida no secret manager; JWKS estável entre restarts.
9. **Scoping por user_id**: providers, conversas, credenciais e task lists filtrados por `user_id` derivado do token.
10. **Segredos gerenciados**: `mcp-*` e `internal-*` não aparecem na lista padrão de credenciais e não são editáveis manualmente.
11. **API HTTP local**: endpoints `/vault/*`, `/auth/*`, `/.well-known/jwks.json` disponíveis.
12. **TLS na LAN**: HTTPS obrigatório fora de localhost; HTTP puro só em localhost/dev explícito.
13. **External mode**: valida JWT do IdP via JWKS e aplica scopes/roles do IdP.
14. **Compatibilidade**: instalação existente migra sem perda (backfill de `user_id`).

---

## Riscos

| # | Risco | Probabilidade | Impacto | Mitigação |
|---|-------|---------------|---------|-----------|
| R1 | Migração perde dados existentes | Baixa | Alto | Backfill idempotente, testes com dados reais, não deleta dados |
| R2 | Keyring indisponível (CI, containers) | Média | Médio | `VaultLocked` + `/vault/unlock` via wraps `master`/`recovery` |
| R3 | Performance de Argon2id no login | Baixa | Baixo | Parâmetros conservadores (3 iterações); aceitável para login |
| R4 | Complexidade de queries com user_id | Média | Médio | Helper/middleware centralizado para scope; não filtrar manualmente |
| R5 | Migração de constraint unique | Média | Médio | SQLite não suporta ALTER CONSTRAINT; usar GORM AutoMigrate com cuidado |
| R6 | Complexidade de TLS na LAN | Média | Médio | HTTPS obrigatório fora de localhost; storage TLS separado do cofre |

---

## Relação com outras AEPs

| AEP | Relação |
|-----|---------|
| **0046** (UUIDv7) | Base já aplicada nesta branch. `users.id` e `sessions.id` usam o mesmo padrão UUIDv7 das tabelas core |
| **0047** (Import/Export) | Base já aplicada nesta branch. Export/import de recursos passa a operar no escopo do usuário atual |
| **0048** (Jobs DB) | Sucede esta. Tabela `jobs` terá `user_id` desde o início |
| **0049** (MCP DB) | Sucede esta. Tabela `mcp_servers` terá `user_id` desde o início |
| **0050** (Profiles DB) | Sucede esta. Tabela `profiles` terá `user_id` desde o início |
| **0051** (Skills DB) | Sucede esta. Tabela `skills` terá `user_id` desde o início |
| **0054/0055** (Split/Web Client) | Sucedem esta no contrato de clientes. Devem consumir auth, sessões e HTTP API definidos aqui |
| **0014** (Credential Persistence) | Base do cofre global (DEK + wraps + keyring) |
| **0022** (Welcome Wizard) | Wizard passa a: cofre → recovery → admin → provider |

### Ordem de implementação efetiva nesta branch

```
AEP-0046 (UUIDv7)
    ↓
AEP-0047 (Import/Export)
    ↓
AEP-0052 (Multi-User — esta)
    ↓
AEP-0048 (Jobs DB)      ─┐
AEP-0049 (MCP DB)        │ (paralelo)
AEP-0051 (Skills DB)     │
    ↓                     │
AEP-0050 (Profiles DB) ──┘ (depende de 0051)
```
