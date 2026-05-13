# AEP-0052 — Sistema de Contas de Usuário

**Status**: Em implementação — **multi-user single-tenant alpha**
(P1-1 do re-review do PR #94). Não cobre cenários "enterprise
multi-tenant" (rotação multi-key com grace period, federação SSO,
isolamento criptográfico por usuário). Esses pontos estão listados
em [Limitações conhecidas](#limitacoes-conhecidas) e nas seções de
TODOs pós-merge.  
**Criado em**: 2026-04-21  
**Depende de**: AEP-0046 (UUIDv7 Migration), AEP-0047 (Import/Export)  
**Precede**: AEP-0048 (Jobs DB), AEP-0049 (MCP DB), AEP-0050 (Profiles DB), AEP-0051 (Skills DB), AEP-0054/AEP-0055 (split/clientes), AEP-0063 (Tool Invocations)

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

## Limitações conhecidas

Esta AEP entrega **multi-user single-tenant alpha**. Cenários abaixo
ficam fora deste escopo e exigem trabalho adicional antes de qualquer
claim "enterprise":

- **Rotação multi-key com grace period**: hoje há uma única chave
  Ed25519 ativa para JWT; rotação é manual e gera janela de até 15min
  de access tokens em voo inválidos. Procedimento operacional em
  [`docs/operations/key-rotation.md`](../docs/operations/key-rotation.md).
  TODO: aceitar tokens assinados com chave antiga durante uma janela
  configurável; JWKS expõe ambas. Pré-requisito para integração com
  consumidores HTTP que não tolerem 401 transitório.
- **Federação SSO**: `auth.mode=external` valida JWT de IdP via JWKS,
  mas não cobre OIDC discovery automático, mapeamento dinâmico de
  scopes para roles, nem provisionamento just-in-time. Fora de escopo
  inicial.
- **Isolamento criptográfico por usuário**: a DEK é global por
  instância. Isolamento entre usuários é lógico (`user_id` em queries),
  não criptográfico. Em ambiente "operador hostil ao tenant" — admin
  da instância acessando dados de outros — não há defesa. Ver §3 da
  Motivação para a justificativa.
- **Persistência de callbacks de canal externo (M14)**: respostas a
  mensagens de Telegram/Signal podem ser perdidas se o app crashar
  entre o registro do callback e a resposta do agente. Mitigado por
  TTL + recover, não por persistência. Ver
  [`internal/messaging/notifier.go`](../internal/messaging/notifier.go).
- **Detecção robusta de violação de unique constraint multi-dialect**:
  `isUniqueConstraintError` é heurística por string match, cobre
  SQLite/Postgres/MySQL via mensagem do driver. Quando o projeto
  adicionar driver Postgres/MySQL como dependência direta, trocar por
  `errors.As` contra os tipos concretos (`pgconn.PgError`,
  `mysql.MySQLError`).

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

Esse contrato também se aplica à AEP-0063: `tool_invocations` nasce com `user_id`, origem explícita e referências ao `tool_catalog` acessível ao usuário, incluindo tools builtin globais (`user_id` nulo) e tools MCP/user-scoped quando aplicável. As invocações em si nunca são logs globais compartilhados entre contas.

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

### Matriz de cobertura deste PR

| Área | Cobertura atual |
|------|-----------------|
| Cofre e `VaultLocked` | Implementado para bootstrap local; `/vault/status`, `/vault/setup` e `/vault/unlock` permanecem sem login, enquanto sessão/JWKS retornam indisponível quando a sessão ainda não existe. |
| Identidade local | Implementado com criação de admin, login por username manual, refresh rotation, logout e roles `admin`/`user`. |
| JWT/JWKS local | Implementado com Ed25519/EdDSA e JWKS publicado. `jti` e ES256 não fazem parte da implementação atual deste PR. |
| HTTP/TLS | HTTP API local implementada. Bind não-local sem TLS falha, exceto com `dev_insecure`; certificado/chave TLS são configurados por caminho, não gerados nem armazenados automaticamente como `internal-tls:*` neste PR. |
| External mode | Implementado como resource server via JWKS, issuer/audience, allowlist de algoritmos e scopes/roles configuráveis. A implementação atual suporta EdDSA e RS256. |
| Scoping por `user_id` | Implementado com fail-closed nos dados user-scoped, contexto autenticado no app, canais, import/export, mensagens, providers, conversas, credenciais, task lists, tasks por hierarquia e `task_notes`. |
| Credenciais | Credenciais de usuário exigem contexto autenticado; segredos de instância `internal-auth:*` e `internal-tls:*` ficam com `user_id=''`; MCP exige contexto de usuário para tokens/inline auth. |
| Constraints multiusuário | Implementado para providers/credenciais já existentes e ajustado para `task_lists.slug` e `task_notes` externos por usuário. |
| Frontend | `AuthGate` só renderiza a aplicação com sessão autenticada e usuário válido; refresh inválido não abre a UI; chamadas concorrentes de status são deduplicadas. |
| Legados em arquivo | Permanecem como escopo explícito de AEPs seguintes (`profiles`, `skills`, `mcp` config, `channels`, `jobs`, allowlists). Este PR impede que credenciais/tokens associados sejam usados sem contexto de usuário. |

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

---

## TODOs pós-merge (review da Fatia 4 — HTTP API + JWKS)

Estes itens foram triados durante o review do PR mas não cabem no escopo
da entrega inicial — ficam registrados aqui para que o próximo trabalho
sobre a API HTTP saiba o estado conhecido.

### B19 — Refresh token via JSON body

**Estado atual**: `POST /auth/refresh` recebe o refresh token em
`{"refreshToken": "..."}`. Não há cookie `httpOnly` neste canal porque
o cliente Wails persiste o refresh token no keyring do SO (sem acesso
JS), de modo que o vetor "JS comprometido lê localStorage" não se
aplica.

**Quando endereçar**: ao introduzir cliente web tradicional (browser
servindo a UI) — AEP-0055 ou similar. A migração esperada é:

1. Login emite cookie `Set-Cookie: refresh=...; HttpOnly; Secure; SameSite=Strict; Path=/auth`.
2. `/auth/refresh` lê o cookie; aceita body apenas para clientes não-browser explicitamente identificados (ex: API token CLI).
3. CSRF token (double-submit cookie) protege contra requests cross-origin.

### B22 — Multi-key signer (rotação)

**Estado atual**: `LoadOrCreateTokenSigner` carrega uma chave Ed25519 e
emite/verifica tokens com ela. JWKS publica `[chave_atual]`. Não há
rotação automática.

**Implicação**: trocar a chave (apagar o segredo `jwt-signing-key`)
invalida tokens em voo. Como access tokens duram 15min e refresh
emite novos a cada uso, a janela é curta — porém não documentada para
o operador.

**Quando endereçar**: quando houver caso operacional para rotacionar
(suspeita de comprometimento, política de compliance). A
implementação esperada:

1. `tokenSignerRecord` ganha campo `Version int` (Mi22 do review) para
   compatibilidade futura.
2. Storage passa a manter `Active key + N anteriores` (ex: 2 últimas).
3. `Sign` usa a `Active`; `Verify` aceita qualquer `kid` conhecido.
4. JWKS publica todas as chaves não revogadas.
5. Rotação automática opcional via background goroutine (configurável,
   default 30 dias com retenção de 2 anteriores).
6. `invalidateJWKSCache` é chamado após cada rotação.

### M21 — Rate-limit cluster-aware

**Estado atual**: `rateLimiter` é in-memory, per-instance. Adequado
para deploy single-process (que é o atual). Em deploys
multi-instância, atacante distribui requests entre réplicas.

**Quando endereçar**: ao introduzir deploy multi-instância. Substituir
backend in-memory por Redis (`token_bucket` em Lua) ou usar
`sliding_window` no proxy reverso (nginx/envoy).

### M22 — Cache LRU de tokens verificados

**Estado atual**: `requireAccess` chama `VerifyAccessToken` em todo
request autenticado (Ed25519 verify + lookup no signer). Em throughput
baixo é negligível; em throughput alto vira contention no signer mutex.

**Quando endereçar**: ao instrumentar a API com observabilidade e
detectar contention. Implementar LRU bounded com TTL = `exp - iat` do
token verificado (~15min); chave do cache = hash do token.

### Mi22 — `tokenSignerRecord` sem campo Version

Hoje a chave é persistida como `base64(privateKey)`. Adicionar wrapper
JSON com `{"version": 1, "keys": [...]}` na próxima migração de schema
do signer (junto com B22).

---

## TODOs pós-merge (review da Fatia 5 — Frontend + Token Persistence)

### B23 — Access token nunca persistido no frontend

**Estado atual**: o frontend Wails N\u00c3O recebe o JWT de access. As
bindings (`Login`, `RefreshAuth`) devolvem apenas `{userId, sessionId, role}`;
o backend retém o access token em mem\u00f3ria do `SessionService` e o
encaixa nas chamadas Wails como contexto interno via `currentAuthUser`.
Logo n\u00e3o existe `localStorage.setItem(ACCESS_TOKEN, ...)` nem
`setAccessToken()` no `authStore` real.

**Quando endere\u00e7ar**: ao introduzir o cliente web tradicional (AEP-0055).
Nesse cen\u00e1rio:

1. Backend devolve o access em response body do `/auth/login` HTTP.
2. Frontend mant\u00e9m o token apenas em mem\u00f3ria do store; nunca
   persiste em `localStorage`/`sessionStorage`.
3. Refresh token vira cookie `httpOnly; Secure; SameSite=Strict;
   Path=/auth` (B19 do Bloco 4).
4. Boot da SPA: `refreshSession()` valida a sess\u00e3o pelo cookie e
   recebe access novo a cada reload.

### B24 — Refresh token nunca em texto plano

**Estado atual**: em vez do `internal/auth/state/store.go` que o reviewer
descreveu, o backend persiste o refresh token via:

- `credMgr.RegisterInstanceSecret(InstanceSecretAuthRefreshToken, ...)`,
  que cifra com a DEK do vault em `users` (mesmo store das credenciais).
- Espelho no keychain do SO (`SaveAuthRefreshTokenToKeychain`) atrav\u00e9s
  do `go-keyring` — Windows Credential Manager / macOS Keychain /
  Secret Service no Linux.

Nenhum caminho deixa o token em texto plano em arquivo de configura\u00e7\u00e3o.
Backups do vault permanecem cifrados pela DEK.

**Quando reabrir**: caso futuramente algum binding caia em fallback que
grave em arquivo plano (ex: ambiente sem keychain como container CI),
documentar o trade-off e cifrar com DEK antes de gravar.

### B25 — Boot via backend (sem confiar em localStorage)

**Estado atual**: o `authStore.refresh()` n\u00e3o l\u00ea token de
`localStorage` — chama `RefreshAuth({})` e o backend procura o token
nas duas fontes seguras (vault + keychain). O `legacyRefreshTokenKey` em
`localStorage` \u00e9 usado apenas como **purge** (apaga qualquer res\u00edduo
de vers\u00e3o antiga) no boot do store e em logout. Ap\u00f3s +1 release
sem reportes de res\u00edduos, removemos a const por completo.

### B26 — Mutex de refresh

**Estado atual**: `refreshGuard` no `authStore` serializa chamadas
concorrentes; m\u00faltiplos disparos em paralelo (alt-tab, focus,
loadStatus) compartilham a mesma promise. N\u00e3o existe par\u00e2metro
`silent` para burlar a guarda — o caller que quiser silenciar UI de
loading deve fazer isso na pr\u00f3pria UI, nunca na pol\u00edtica de
sincroniza\u00e7\u00e3o do refresh.

### B27/B28 — Hooks de network/idle ainda n\u00e3o existem

`useAuthErrorRecovery`, `useIdleAuthRefresh` e `useAuthRefresh` s\u00e3o
hooks descritos pelo reviewer mas que **n\u00e3o existem no c\u00f3digo
atual**. O store hoje s\u00f3 expõe `loadStatus()` (chamado uma vez no
mount do `AuthGate`). Se algum dia introduzirmos refresh peri\u00f3dico
ou recovery em foreground:

1. Debounce de 1s em listeners `visibilitychange`/`focus`.
2. Diferenciar erros de rede ("conex\u00e3o perdida, reconectando...")
   de 401 (logout for\u00e7ado) e 5xx (toast gen\u00e9rico).
3. Reusar o `refreshGuard` para n\u00e3o duplicar requests.

### M30/M31 — A11y do `AuthGate`

**Aplicado neste PR**: `AuthGate.tsx` agora usa `useTranslation()` (i18n
em pt-BR/en/es), `useAnnouncer()` para feedback de erros e sucesso,
`aria-describedby` ligando inputs aos hints, `aria-labelledby` no card,
labels formais com `htmlFor`. CSS migrado para tokens `theme.css`
(zero cor hardcoded). Erros do servidor s\u00e3o anunciados via live
region global (assertive); erros de valida\u00e7\u00e3o local usam
`role="alert"` apenas dentro do card e n\u00e3o roubam contexto da
navega\u00e7\u00e3o por leitor de tela.

### Mi26 — Testes axe-core

**Aplicado neste PR**: dois cen\u00e1rios cobertos
(`AuthGate.test.tsx` — telas de signIn e setup) integrados ao
`vitest-axe`. Quando novas etapas forem adicionadas (criar admin,
desbloquear cofre, retry) os testes seguem o mesmo padr\u00e3o.

---

## TODOs pós-merge (review da Fatia 6 — Migrações + Schema)

O reviewer descreveu uma arquitetura de migrações versionada
(`internal/database/migrations/{migrate.go, 0001_users.go, 0002_sessions.go,
0003_credential_entries.go, 0004_credential_key_wraps.go,
0005_channel_refresh_token_enc.go}` com tabela `schema_migrations` e
sequenciador transacional `Apply + record_version`) que **não existe** no
projeto. O sistema atual é:

- `gorm.AutoMigrate(...)` declarativo a partir dos models em `models.go`.
- Uma migração one-shot legacy `migrateToUUIDv7()` em `migration_uuid.go`
  (já transacional via `tx.Begin/Commit`).
- Helpers `ensureXxxIndex()` / `ensureXxxCaseInsensitive()` para índices
  parciais e invariantes que GORM tag não expressa.
- Migrações de dados pontuais inline em `Init()` (refresh_url, normalização
  de booleans corrompidos, etc.).

Itens aplicáveis foram endereçados neste PR. Os itens abaixo ficam como
TODO porque dependem de mudanças arquiteturais ou de funcionalidades que
ainda não existem.

### B29 — Apply + record_version transacional

**N/A no código atual**: não há tabela `schema_migrations` nem sequenciador
de versões. `migrateToUUIDv7()` (a única migração custom) já é transacional;
`AutoMigrate` é gerenciado pelo GORM. Quando introduzirmos sistema
versionado (provavelmente junto de migrations cross-dialect para futuro
cloud), essa transação por step é parte do contrato.

### B32 — Re-cifragem de credentials legacy

**N/A**: o schema atual não tem coluna `storage` nem mistura `'plain'` com
`'enc'`. Credenciais já são cifradas via DEK no save (`credentials.Manager`)
e descifradas on-read; bases pré-cifragem são tratadas como entries
inválidas e descartadas. Se introduzirmos formato versionado de cifragem
(rotação de algoritmo), precisaremos do job background de re-encrypt
descrito no review.

### B33 — Cifragem retroativa de mensagens

**N/A**: não existe cifragem por mensagem hoje (`ChatMessage.Content` é
texto plano em SQLite local). A discussão de cifragem de mensagens com
flag `is_content_encrypted` está em escopo da AEP de cofre por usuário
(futura, ainda não escrita) e do plano cloud (AEP-0055). Quando isso for
introduzido, este TODO vira blocker da migração com job background +
gating do `vault_setup_completed`.

### M38 — Forward-only / rollback

**Política deliberada**: AutoMigrate + migrações idempotentes one-shot são
forward-only por design. Recovery = restore de backup pré-migração (já
implementado em `createBackup()` no `migrateToUUIDv7`). Para sistema
versionado futuro, o reviewer está certo: cada migration precisa de
`Down()` testado.

### M39 — PRAGMA cross-dialect

**N/A hoje, mitigado**: SQLite-only. Os helpers que precisam introspeção de
schema usam `pragma_table_info(...)` (B30) ou `db.Migrator().HasTable/HasColumn`
(quando o lookup no struct GORM é suficiente). Em portabilidade futura para
Postgres/MySQL, substituir os PRAGMAs por queries `information_schema` —
listadas como pré-requisito da AEP de cloud.

### M40 — FK `credential_key_wraps → users` com CASCADE

**N/A**: `CredentialKeyWrap` é per-instância (kind ∈ `{master, recovery}`),
não tem `UserID`. Cofre é global por instância. Quando introduzirmos cofre
por-usuário (AEP futura), aí sim precisará de FK + CASCADE.

### M41 — Index em `sessions.expires_at`

**Já existe**: `Session.ExpiresAt` declara `gorm:"index"` em `models.go`,
e GORM AutoMigrate cria automaticamente. `RevokedAt` também é indexado.
Validado pelos testes de `PurgeExpiredSessions` que usam ambos no WHERE.

### M44 — Timestamp em milissegundos

**N/A**: sem `applied_at` (não há tabela `schema_migrations`). O log de
`migrateToUUIDv7` agora registra elapsed via `time.Since(...).Truncate(ms)`.

### M45 — DROP COLUMN `refresh_url`

**Aplicado**: `migrateRefreshURLToEnc` (B30) executa `ALTER TABLE ... DROP
COLUMN refresh_url` após copiar dados (SQL puro porque GORM `DropColumn`
faz lookup na struct e vira noop quando o campo já foi removido do model).

### M46 — Gap em migrations

**N/A**: AutoMigrate não tem sequenciamento numérico. Cada model é
auto-migrado idempotentemente em todo boot.

### Mi32 — Filename convention enforcement

**N/A**: sem migrations numeradas. Quando vier o sistema versionado, o
linter pode validar `^\d{4}_[a-z_]+\.go$` em CI.

### Mi34 — `created_at`/`updated_at` em users

**Já existe**: `User` embute `UUIDModel` que declara `CreatedAt time.Time`
e `UpdatedAt time.Time`. GORM hooks tocam ambos automaticamente.

### Mi35/Mi36/Mi37 — CHECK constraints (`password_hash`, `wrap_alg`, `id` formato)

**N/A no contexto atual**: validação acontece em camada de serviço
(`password.go` exige Argon2id válido, signer valida `wrap_alg` antes de
persistir, `BeforeCreate` em `UUIDModel` gera UUIDv7 e rejeita IDs
malformados). CHECK constraint seria defense-in-depth útil, mas exige
cuidado em SQLite (sem `IF NOT EXISTS` para CHECK; recriar tabela).
Listado para inclusão na primeira migração com sistema versionado.

### Mi39 — Timezone em `applied_at`

**N/A**: sem `applied_at`. GORM persiste `time.Time` em UTC desde a
configuração do projeto. Em sistema versionado futuro, o timestamp será
ISO 8601 UTC explícito.

### Aplicados neste PR (Bloco 6)

- **B30**: `migrateRefreshURLToEnc` extraída para função dedicada com log
  de RowsAffected, drop via SQL direto, idempotência testada.
- **B31**: nova `dedupCredentialEntriesBeforeMigrate` roda **antes** do
  AutoMigrate, deduplicando entries com mesmo `(user_id, pattern)` em
  bases legadas (mantém a entry com maior `updated_at`, ties por `id`
  UUIDv7 desc). Sem isso o AutoMigrate falha ao criar o índice unique
  e o app não sobe.
- **M42**: trade-off documentado, não endereçado por restrição do SQLite.
  O reviewer pediu índice parcial `WHERE pattern <> ''`; o `clause.OnConflict`
  do `credentials/db_store.go` (UPSERT) só funciona contra índices unique
  full em SQLite, então o índice ficou non-partial. Em prática o app
  sempre grava patterns não-vazios (instance secrets têm nomes
  específicos), tornando o filtro irrelevante. Se um dia trocarmos
  o store para 2-step (try-update-then-insert), reabrimos o item.
- **B34**: nova função `ensureUsernameCaseInsensitive` normaliza usernames
  legacy para lowercase, desativa duplicatas case-variantes (rename para
  `<username>.legacy.<idprefix>` + `is_active=0`) e cria índice
  `users_username_lower_unique ON users(LOWER(username))` como defesa em
  DB contra INSERTs futuros com case diferente.
- **Mi33**: `migrateToUUIDv7` loga elapsed time em milissegundos.
- **Mi38**: `SessionService.PurgeExpiredSessions(ctx, retention)` apaga
  sessions cujo `expires_at` ou `revoked_at` ultrapassou a janela de
  retenção, com testes para retenção configurável e modo administrativo
  (retention=0).

## TODOs pós-merge (review da Fatia 7 — Controllers + CLI)

O reviewer descreveu uma camada de controllers/CLI com `Mount(ctx)`,
`c.appCtx`, `app.ListAllMessages`, `database.ListMessages()`,
`controllers/messages_controller.go`, comandos `db data backup`,
`db data restore`, `db wipe`, e uma versão de `app.Context()` que injeta
`userID` condicionalmente. **Nada disso existe no código.** A fatia
ficou predominantemente fora de escopo porque a triagem revelou que
quase todos os blockers e majors descrevem componentes alucinados.
Documentamos abaixo o que cabe ao código real e o que não.

### Aplicado neste PR

- **Achado real adicional (não estava no review)**: `App.ResetDatabase()`
  em `internal/app/app_database.go` era exposto via Wails Bind sem
  qualquer gate de auth. Em deployment multi-user, qualquer caller
  derrubava o DB inteiro de todos os usuários. Adicionado novo helper
  `requireAdminContext` (`internal/app/app_auth.go`) que combina
  `requireAuthenticatedContext` com checagem de `currentAuthUser.Role`
  sob lock — pré-login devolve `ErrUserScopeRequired`, logado mas
  não-admin devolve `ErrAdminRequired` (constante exportada). O método
  `ResetDatabase` agora chama esse gate antes de tocar em qualquer
  arquivo, e quando o reset real falha o caller recebe um erro genérico
  (`ErrDatabaseResetFailed`) — o detalhe (path, syscall, motivo) só vai
  para `log.Printf` local, evitando vazar estrutura de filesystem em
  multi-user. Cobertura em `internal/app/bloco7_admin_gate_test.go`
  com 5 cenários (pré-login, role=user, role=admin, ResetDatabase
  pré-auth, ResetDatabase role=user).

### N/A no contexto atual

#### B35 — `app.ListAllMessages` cross-user

**N/A — alucinação completa.** A função `ListAllMessages` não existe em
`internal/app/`, em `controllers/` nem em `internal/database/`. Não há
arquivo `internal/app/messages.go` nem `controllers/messages_controller.go`.
A função `database.ListMessages()` sem ctx não existe. Os endpoints
reais de mensagens (`GetMessages`, `GetRecentMessages`,
`GetMessagesBefore`, `GetConversationMessageWindow`,
`SearchConversationHistory`, etc.) em `internal/app/db.go` **todos**
chamam `requireAuthenticatedContext` no início (29 endpoints, validados
pelos testes de cross-user no Bloco 3) e propagam ctx autenticado para
repositórios fail-closed que executam `RequireUserID` antes de qualquer
JOIN.

#### B36 — Controllers sem `requireAuthenticatedContext`

**N/A na forma descrita.** O reviewer cita endpoints específicos
(`GetAllChannelConfigs`, `GetAuthorizedContacts`, `GetAvailableChannels`,
`GetLLMProviders`, `GetDefaultProvider`, `ListMessagesByConversation`)
como "expostos pre-login sem gate". Verificação direta:

- `GetAllChannelConfigs`, `GetAuthorizedContacts`, `GetAvailableChannels`
  em `internal/app/app_messaging.go` **JÁ chamam**
  `requireAuthenticatedContext` antes de tocar dados (B6 do Bloco 2,
  já endereçado).
- `ListMessagesByConversation` não existe; `GetMessages(convID, parentID)`
  é o método análogo e **JÁ chama** `requireAuthenticatedContext` em
  `db.go:73`.
- `GetLLMProviders()` e `GetLLMProvider(id)` lêem de
  `a.llmRegistry` em memória. O registry só é populado via
  `reloadUserScopedRuntime` (rodado pós-Login/RefreshAuth) e limpo no
  `setCurrentAuthUser(nil)` do Logout. Pré-login retorna lista vazia,
  não há vazamento cross-user. Trade-off: frontend não distingue
  "sem providers" de "não logado" — coberto pelo `AuthGate` que
  bloqueia toda a UI antes do Login (Bloco 5).

A camada de controllers desse projeto **não é** o que o reviewer
descreveu. Não há `Controllers` struct com `appCtx` field, não há
`Mount(ctx)`, controllers não têm bindings Wails diretas (só métodos
de `*App` viram bindings). Cada controller é um agregado de dependências
injetadas no construtor; ctx vem como argumento do método (Wails
injeta automaticamente quando o primeiro param é `context.Context`).

#### B37 — CLI bypassa auth

**N/A — alucinação.** Os subcomandos descritos (`db data backup`,
`db data restore`, `db wipe`) **não existem** em `cmd/asst/`. Os
comandos reais são `asst data export`, `asst data analyze`,
`asst data import`, todos delegando para `app.ExportData`/`ImportData`/
`AnalyzeImportData` que internamente passam por `importExportContext()`
(`internal/app/export_import.go:387`) — que só devolve um ctx
autenticado via `requireAuthenticatedContext`. Resultado: rodar
`asst data export --all` pré-login devolve `authenticated user
required`. Não há "dump completo cross-user" possível pelo CLI hoje.

#### B38 — `Context()` retorna ctx cru se `currentUserID==""`

**N/A na forma descrita.** A função `App.Context()` em
`internal/app/app.go:171` é literalmente `return a.ctx` — não injeta
userID condicionalmente. A função análoga ao código mostrado pelo
reviewer é `internalBootstrapCtx()` em `app_auth.go:521`, que JÁ:

1. Tem nome propositalmente assustador (Blocker C do re-review do
   AEP-0052) para evitar autocomplete enganoso.
2. Documenta os **2 únicos call sites permitidos**
   (`initCredentialManager` e MCP `SetAuthContextProvider`).
3. Encoraja `requireAuthenticatedContext` (existente desde a Fatia 1)
   para tudo o mais — que já é o padrão usado pelos 29 endpoints de
   `db.go`, pelos endpoints de messaging, providers, profiles, jobs,
   tokens etc.

A renomeação `Context() → AuthenticatedContext()` proposta pelo
reviewer é redundante: já temos `requireAuthenticatedContext` (fail-
closed) e `internalBootstrapCtx` (fail-open com nome bloqueador).

#### B39 — Race em `controllers.appCtx`

**N/A — alucinação.** Não existe `controllers/controllers.go`, não
existe `appCtx` field em nenhum controller, não existe método `Mount`.
Os controllers são montados em `app.go:339-444` via `NewXController`
com dependências passadas por struct config — sem field de ctx
mutável. A única struct compartilhada com lock é `App` (que tem
`authMu`, `authSessionMu`, `currentUserMu` etc., todos cobertos por
testes).

#### M47 — `CreateDefaultLLMProvider` persiste com `user_id=''`

**Trade-off documentado e auditado.** Em `app_llm_providers.go:194-200`
a função usa `internalBootstrapCtx`; quando não há userID, faz
`database.WithBootstrap(ctx)` explícito — único caminho permitido para
gravar provider sem userID, fail-closed em qualquer outra rota
(repositório de providers exige `RequireUserID` ou `IsBootstrap`).
Provider criado pelo wizard pré-login fica órfão até o primeiro
`AdoptLegacyData` (Login/Refresh do primeiro usuário). Em segundo
deployment multi-user, o operador roda `CreateDefaultLLMProvider`
**após** o login do admin, e o ctx já carrega userID — não é mais
órfão. Refatoração para "default como template em código + materializar
no Login" é desejável mas escopo de outra AEP (provider templates por
canal/perfil).

#### M48 — CLI sem confirmação interativa em operações destrutivas

**N/A.** Não há `wipe`, `restore` ou `backup` no CLI. O único caminho
destrutivo (`asst data import`) sobrescreve dados via arquivo
explícito; o usuário já cita o caminho. O `asst setup` JÁ tem prompt
"Deseja reconfigurar? (s/N)" quando detecta DB existente
(`cmd/asst/setup.go:71`).

#### M49 — Tests cross-user em controllers

**N/A na forma descrita.** Controllers não têm bindings Wails diretas
(só métodos de `*App` são expostos). Os testes cross-user existem
no nível de use-case e repositório (Blocos 1–3): DBStore.GetByID
rejeita acesso de outro user, `RequireUserID` é exigido em todos os
escritores fail-closed, e o pipeline de `SendMessage` valida
`OwnerUserID` no gateway (Bloco 2). Cobertura do gate admin do
Bloco 7 fica em `bloco7_admin_gate_test.go`.

#### M50 — `ListMessagesByConversation` ownership

**N/A — função não existe.** O método análogo é `App.GetMessages` em
`db.go:72`, que faz `requireAuthenticatedContext` e propaga ctx para
o repositório de chat — que executa `WHERE conversations.user_id = ?`
implicitamente via `database.ScopeByUser`. Cobertura nos testes de
cross-user da Fatia 3.

#### M51 — Setup detecta instância existente

**Já aplicado.** `cmd/asst/setup.go:69` chama `NeedsWelcomeWizard()`
e, quando há setup prévio, prompta "Deseja reconfigurar? (s/N)".
O reviewer alegou que setup "sobrescreve config crítica" — falso:
ele só re-roda os passos de senha mestre (sob `HasMasterKey()`) e de
provider; nada destrutivo de dados de usuário.

#### M52 — Audit log CLI

**Trade-off.** Sem audit log dedicado em `~/.assistente/audit.log`. O
CLI já registra cada operação destrutiva via `log.Printf` (capturado
pelo logger padrão). Audit log persistente append-only entra na
trilha de "deployment hardening" — escopo da AEP de cloud, não desta.

#### M53 — Sanitização de erros em controllers

**Parcialmente endereçado pelo único caminho real desta fatia**:
`ResetDatabase` agora retorna `ErrDatabaseResetFailed` genérico em
vez de `fmt.Errorf("erro ao remover banco de dados: %v", err)` que
vazava paths de filesystem. Os outros caminhos sensíveis (chat,
conversa, providers) já filtram no nível do app — o frontend recebe
mensagens curtas via i18n (Bloco 5).

### Minors — N/A

- **Mi40** (`mountController` dupla montagem): `mountController` não
  existe.
- **Mi41** (CLI `--help` rico): Cobra já gera `--help` por subcomando.
- **Mi42** (Documentação de bindings): bindings Wails são
  auto-geradas (`frontend/wailsjs/go/app/App.d.ts`); `tsc --noEmit`
  no CI valida tipos.
- **Mi43** (`closeController` timeout): `closeController` não existe.
- **Mi44** (Versionamento de schema em export): `portability` JÁ
  versiona via `Version` no envelope JSON (`internal/portability/`)
  e o `analyze` reporta a versão antes do import.
- **Mi45** (CLI `--dry-run`): pendente; baixa prioridade — `data
  analyze` cobre o caso de inspecionar antes de importar.
- **Mi46** (`--quiet/--verbose`): existe `--verbose` global em
  `cmd/asst/main.go:105`.
- **Mi47** (Path traversal em `--out`): `cobra` aceita o path; em
  contexto desktop o usuário escolhe onde escrever. Em deployment
  CLI-via-web (não suportado pelo projeto hoje) seria validação
  extra.

