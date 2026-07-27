# AEP-0083 — Migração de Canais e Contatos para Banco de Dados

**Status:** ✅ Done (concluída via [PR #400](https://github.com/inclunet/assistente/pull/400))

## Dependências

- **AEP-0046** (UUIDv7 / `UUIDModel`)
- **AEP-0047** / contrato `ImportLegacyResourcesWithContext` (`internal/portability`)
- **AEP-0049** (padrão CredManager + import legado pós-login)
- **AEP-0052** D10 (canais/contatos classificados como legado em arquivo a migrar antes de multiusuário remoto)
- **AEP-0076** (AutoMigrate aditivo + versionamento de schema)

## Resumo

Migrar a configuração de canais de mensageria (`channels/*.json`), a lista de contatos autorizados (`contacts.json`) e o mapeamento contato→conversa para tabelas SQLite via GORM, sempre vinculadas ao usuário quando autenticado.

O DTO público `channels.ChannelConfig` permanece estável para Wails/gateway. A fachada `internal/channels` passa a usar o banco quando `channels.UseDatabase(db)` é chamado no boot (após `database.Init`). Contatos seguem o mesmo padrão via `contacts.UseDatabase(db)`.

Tokens e segredos **nunca** são gravados em plaintext nas tabelas: apenas refs de pattern do CredManager (`channel:{slug}:bot_token|app_token|api_token`). App id/secret futuros usam `channel:{slug}:app` com `ClientID`+`ClientSecret` (como `mcp-client:{slug}`) — documentados aqui; sem UI Teams nesta AEP.

Arquivos legados **não** são apagados. A importação pós-login é idempotente (skip se `(user_id, slug)` já existir).

**Entrega:** implementada e mergeada em `main` pelo PR #400 (`feat/channels-database-migration`).

## Motivação

1. **Multiusuário (AEP-0052 D10):** canais/contatos em arquivo não isolam por `user_id`. Antes do modo remoto, precisam ir para DB.
2. **Consistência:** jobs, MCP e demais recursos já migraram; canais eram o último núcleo de integração ainda em JSON.
3. **Atomicidade:** salvar config + mapeamento de conversas + contatos em arquivos separados não é transacional.
4. **UI honesta:** a ChannelsPage não deve mostrar placeholders Telegram/Signal/Slack “sempre presentes”; só canais que existem no DB.
5. **Segurança:** eliminar plaintext residual de tokens em `channels/*.json` migrando-os para o CredManager na importação.

## Decisões

### D1 — Três tabelas

1. **`channels`** (`database.Channel`):
   - `UUIDModel`, `UserID`, `Type` (`telegram|signal|slack`), `Slug` (v1: `slug == type`), `DisplayName`
   - `Enabled`, `Profile`, `MaxHistory`, `MaxContacts`
   - `Settings` (JSON text — Signal: `api_url`, `account`; extensível, ex.: `reply_chat_ids`)
   - `BotTokenRef`, `AppTokenRef`, `APITokenRef` (strings de pattern — **não** secrets)
   - `uniqueIndex (user_id, slug)`; `TableName: channels`

2. **`channel_contacts`** (`database.ChannelContact`):
   - `UUIDModel`, `UserID`, `ChannelID` (FK), `ExternalID`, `DisplayName`, `Username`, `AuthorizedAt`
   - unique `(channel_id, external_id)`

3. **`channel_contact_conversations`** (`database.ChannelContactConversation`):
   - `UUIDModel`, `ChannelID`, `ContactExternalID`, `ConversationID`
   - unique `(channel_id, contact_external_id)`

### D2 — DTO `ChannelConfig` permanece o contrato Wails/gateway

Campos de runtime legados (`BotToken`, `AppToken`, `APIToken`, `Account`, `APIURL`, `Conversations`, `ReplyChatIDs`, `OwnerUserID`) continuam no DTO. O mapeamento row↔DTO:

- `OwnerUserID` ↔ `UserID` da row
- `Account` / `APIURL` / `ReplyChatIDs` ↔ `Settings` JSON
- `Conversations` ↔ tabela `channel_contact_conversations`
- refs de token ↔ colunas `*TokenRef` (plaintext só em memória transitória / CredManager)

### D3 — CredManager: patterns de canal

| Pattern | Tipo AuthConfig | Uso |
|---------|-----------------|-----|
| `channel:{slug}:bot_token` | `secret` (`Token`) | Telegram / Slack bot |
| `channel:{slug}:app_token` | `secret` (`Token`) | Slack app (socket mode) |
| `channel:{slug}:api_token` | `secret` (`Token`) | Signal API opcional |
| `channel:{slug}:app` | `ClientID` + `ClientSecret` | Futuro (Teams/OAuth app) — **sem UI nesta AEP** |

Nunca gravar plaintext de token nas tabelas `channels` / `channel_contacts`.

### D4 — Fachada dual até o cutover de boot

- Sem `UseDatabase`: comportamento filesystem legado (testes/gateway sem DB).
- Com `UseDatabase(db)` no boot (após `database.Init`): leituras/escritas runtime vão ao DB.
- APIs Wails preferem `database.UserIDFromContext` / `RequireUserID`.
- `Load(slug)` sem user (gateway/adapters): se houver **exatamente um** canal `enabled` com aquele slug no DB, retornar; senão tentar owner conhecido (`OwnerUserID` / última atribuição); caso ambíguo, não inventar.
- `LoadEnabled`: lista todos enabled (startup de adapters — tipicamente um por slug no single-user local).

**Addendum (fail-closed no boot):** em produção, `initMessaging` (após `database.Init`) **exige** `database.DB() != nil` e chama `channels.UseDatabase` / `contacts.UseDatabase`. Se o DB estiver indisponível, o startup falha com erro explícito — **não** omitir `UseDatabase` e cair silenciosamente no filesystem para runtime de canais/contatos. O código FS permanece para testes unitários (sem `UseDatabase`) e para import legado read-only. Com DB ativo, `AdoptOrphans` adota apenas rows no SQLite (não escreve JSON legado).

### D5 — Contatos via `channel_id`

O pacote `contacts` mantém a API pública (`GetForChannel`, `Authorize`, `IsAuthorized`, `Remove`, …). Com DB, resolve o canal por slug → `channel_id` e opera em `channel_contacts`.

### D6 — Import legado pós-login (não destrutivo)

Registrar em `app_legacy_imports.go` como **"Channels"** usando `portability.ImportLegacyResourcesWithContext`:

- Fonte read-only: `channels/*.json` + `contacts.json`
- Idempotente: skip se `(user_id, slug)` já existe
- Migrar secrets plaintext do JSON → CredManager; gravar refs
- Importar contatos e mapeamentos contato→conversa
- **Não** apagar nem renomear arquivos legados

### D7 — `AdoptOrphans` no `CreateAdminUser`

Além do comportamento FS existente (canais sem `OwnerUserID`):

- Adotar rows DB com `user_id` vazio
- Manter idempotência (não sobrescrever dono existente)

### D8 — UI ChannelsPage

- Grid **só** com canais que existem (`GetAllChannelConfigs` / `ListAll`) — vazio se nenhum
- **Novo** → menu de tipos suportados → cria canal (`CreateChannelFromTemplate` / `Save` default) → abre editor
- Sem linhas placeholder Telegram/Signal/Slack sempre visíveis
- i18n nos 3 locales para strings novas

## Fases

Todas as fases abaixo foram entregues no PR #400:

1. ✅ **Docs** — esta AEP; menção de canais em AEP-0052 D10.
2. ✅ **Schema** — models GORM + AutoMigrate (`database.go` + `fullAutoMigrate`).
3. ✅ **Store/fachada** — map ChannelConfig ↔ row; `UseDatabase`; Save/Load/List/Delete/conversas/AdoptOrphans/CreateFromTemplate no DB.
4. ✅ **Contatos DB** — `contacts.UseDatabase` ligado a `channel_id`.
5. ✅ **Import legado + boot** — registrar importer; chamar `UseDatabase` após Init.
6. ✅ **UI** — ChannelsPage sem placeholders; testes Vitest.
7. ✅ **Testes Go** — map, import idempotente, Save/Load DB.

## Riscos

| Risco | Mitigação |
|-------|-----------|
| Colisão de slug entre usuários no `map[string]*ChannelConfig` | Single-user local é o caso tipico; Wails filtra por owner; `Load(slug)` exige unicidade de enabled |
| Tokens plaintext no JSON legado | Import migra para CredManager; arquivos não são apagados mas deixam de ser fonte runtime |
| Gateway chama `Load` sem ctx de user | Heurística “exatamente um enabled por slug” + owner conhecido |
| Contatos órfãos (canal ainda não importado) | Import de contacts após channels no mesmo importer; warnings se canal ausente |
| `ReplyChatIDs` fora do desenho de colunas | Persistidos em `Settings` JSON |

## Critérios de aceitação

- [x] Tabelas `channels`, `channel_contacts`, `channel_contact_conversations` no AutoMigrate
- [x] `channels.UseDatabase` + `contacts.UseDatabase` no boot após `database.Init`
- [x] Save/Load/ListAll/LoadEnabled/Delete/SaveConversationID/AdoptOrphans/CreateFromTemplate funcionam com DB
- [x] Nenhum plaintext de token nas tabelas; refs no CredManager
- [x] Import legado “Channels” idempotente; arquivos legados intactos
- [x] ChannelsPage: grid vazio sem canais; Novo cria e abre editor; sem placeholders fixos
- [x] Testes unitários (map, import, Save/Load) e Vitest (grid vazio / após create)
- [x] Pattern futuro `channel:{slug}:app` documentado (ClientID+ClientSecret)

## Smoke manual (pós-merge)

Checklist curto para validar o cutover em máquina com dados legados:

- [ ] Login com `channels/*.json` (e `contacts.json`, se existir) presentes no disco
- [ ] Import “Channels” pós-login idempotente (segundo login não duplica canais/contatos)
- [ ] Adapters sobem após o login (canais enabled no DB)
- [ ] Ida/volta de mensagem em pelo menos um canal configurado (Telegram e/ou Signal e/ou Slack)
- [ ] Contatos autorizados respeitados (não-autorizado bloqueado; autorizado conversa)
- [ ] Arquivos legados intactos no disco (não apagados nem renomeados pelo import)
