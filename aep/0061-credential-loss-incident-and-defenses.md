# AEP-0061 — Incident report: credenciais não resolvidas após login, routing cruzado entre profiles e MCPs reabrindo OAuth a cada restart

**Status:** Accepted
**Data:** 2026-05-11
**Autor:** Engenharia Assistente
**Relacionado:** AEP-0046 (UUIDv7), AEP-0052 (multi-user accounts)

## Resumo

Em 10–11/05/2026, após a release AEP-0052 (multi-user accounts), três
sintomas começaram a aparecer em runtime:

1. `Post "https://…/responses": credencial gerenciada não resolvida para
   pattern "ist-prod-litellm.nullmplatform.com" e usuário "019e14c3-…"`
   — mesmo com a credencial gravada no DB e visível em
   `ListVisibleCredentials`. Reaparecia em sessões diferentes, sem
   passar por nenhum wizard.
2. Ao selecionar "perfil Y" na UI de chat, o app usava o provider
   configurado em "perfil X" (o ativo global), e o erro acima saía com
   o nome do provider X — não do que o user tinha escolhido.
3. Toda vez que o app reabria, TODOS os servidores MCP com OAuth
   pediam reauth, abrindo N janelas do navegador simultaneamente.
   "Já tinha autenticado" não bastava: a cada restart, browser de novo.

Investigação encontrou TRÊS causas raiz independentes, todas
acoplamentos implícitos no código (não bugs em queries SQL nem perda
de dados no DB).

A operação foi recuperada restaurando 13 rows de `credential_entries`
do backup `conversations.db.pre-uuid.bak` antes da investigação
começar — confirmação adicional de que o problema NÃO era perda no
banco: as credenciais estavam lá, apenas não chegavam à memória.

## Causa raiz #1 — `LoadFromStore` mágico (credenciais não chegam à memória)

`credentials.Manager.LoadFromStore(ctx)` tinha comportamento
diferente conforme o ctx:

- ctx com `userID` injetado → carregava todas as credenciais do user.
- ctx sem `userID` → caía silenciosamente em `ListInstanceCredentials`
  e carregava APENAS instance secrets (`internal-auth:*`,
  `internal-tls:*`).

Os callers em `internal/app/app_auth.go` (`adoptLegacyDataForUser`)
e `internal/app/app_credentials.go` (`initCredentialManager`,
`configureCredentialManager`) passavam `context.Background()` —
acreditando que estavam "carregando tudo" no boot e no Login. Como o
ctx vinha sem `userID`, o Manager carregava só os 3 instance secrets
e abandonava as 11 user-scoped no DB.

Resultado em runtime: `ResolveForURLWithContext`, que filtra o cache
em memória por `userID` do request, não achava nada e retornava
`unresolvedCredentialError` — exatamente o sintoma reportado, mesmo
com a credencial intacta no banco.

## Causa raiz #3 — autoconnect MCP no boot pré-login + `loadUserTokens` sem ctx user-scoped

O sintoma "todos os MCPs OAuth abrem browser a cada restart" era a
soma de três bugs encadeados em `internal/mcp`:

1. **`Manager.LoadConfigs` disparava `go m.Connect(slug)` para cada
   server enabled+autoconnect.** E `LoadConfigs` é chamado de
   `app.initMCP()` durante `StartupWithAdapters` — antes de qualquer
   Login. Logo, o autoconnect roda quando ainda não há sessão.

2. **`loadUserTokens(credMgr, slug)` e `loadClientCreds(credMgr, slug)`
   usavam `credMgr.GetByPattern(...)` sem ctx.** Internamente vira
   `GetByPatternWithContext(context.Background(), pattern)`, e
   `Manager.GetByPatternWithContext` tem um filtro anti-leak que
   ignora credenciais user-scoped quando o ctx não tem `userID`. O
   resultado: mesmo POST-Login, com o token salvo no banco e em
   memória, esses callsites NUNCA achavam o token user-scoped.

3. **Pós-login, nada redisparava conexão MCP.**
   `reloadUserScopedRuntime` (chamado pelo Login/RefreshAuth)
   reinicializava providers, perfis, etc. — mas não tocava no
   `mcpMgr`. O autoconnect já tinha falhado no boot e ficava parado.

Resultado em runtime: a cada restart do app, todos os servidores
MCP com `auto_connect=true` tentavam conectar antes do login, não
achavam o token (causa #1+#2), caíam no flow OAuth interativo, e
abriam N janelas simultâneas (porque `LoadConfigs` disparava com
`go`, sem coordenação).

## Causa raiz #4 — DEK do keychain divergiu da DEK que cifrou as credenciais

Após as recuperações de #1, #2 e #3, em 11/05 voltou a aparecer um
erro DIFERENTE: `Post "https://…/responses": credencial
"ist-prod-litellm.nullmplatform.com" ilegível para usuário "…":
cipher: message authentication failed`.

O erro é o que AES-GCM retorna quando a chave usada pra decifrar não é
a que cifrou (tag mismatch). A credencial estava no banco e estava no
cache em memória, mas `decryptAuth` falhava no `gcm.Open` da DEK
atual.

Diagnóstico forense (12 credenciais inspecionadas no banco vivo):

| Cred | updated_at | DEK do keychain decifra? |
|---|---|---|
| `internal-auth:*` (3 entries) | 2026-05-10/11 | sim |
| `mcp-tokens:atlassian/glean/nu-mcp` (refresh OAuth recente) | 2026-05-11 | sim |
| `ist-prod-litellm.…` | 2026-04-09 | **não** |
| `*.openai.com` | 2026-03-10 | **não** |
| `api.github.com` | 2026-03-09 | **não** |
| `api.nu.workflows.dev` | 2026-04-10 | **não** |
| `mcp.slack.com` | 2026-03-16 | **não** |
| `mcp-tokens:slack` | 2026-04-25 | **não** |

Padrão claríssimo: tudo escrito a partir de 2026-05-10 decifra, tudo
anterior **falha**. Os 6 `credential_key_wraps` (master/recovery)
continuam com timestamps de 2026-03-09 (nunca foram regerados). Logo:
a DEK do keychain foi trocada em algum momento sem regerar wraps e
sem recifrar as credenciais antigas.

Em código atual nenhum caminho faz isso de propósito, mas o histórico
do repo tem 4 commits (entre 6–7 de maio) corrigindo variantes do
mesmo bug — `bc4bdab2 fix(auth): adopt existing keyring DEK during
vault setup`, `5356cd18 fix(credentials): preserve existing DEK
during setup`, `b73fde6b fix(credentials): harden DEK and user scope
migration`, `d4011692 fix(credentials): avoid keyring writes when
adopting DEK`. Cada um corrigiu UM caminho específico onde o setup
gerava nova DEK mesmo havendo DEK preexistente no keychain. Nenhum
deles, porém, introduziu uma **invariante explícita** verificável a
posteriori — a divergência continuou possível de ser introduzida por
qualquer caminho histórico (incluindo execuções que aconteceram em
versões antigas e cuja DEK divergente sobreviveu para versões novas)
e impossível de detectar no boot.

Nesta instalação, sem senha mestre nem recovery key, as 6
credenciais antigas estão criptograficamente perdidas (a DEK_X que as
cifrou só está embrulhada no `master_wrap` do banco e o
desembrulho exige a senha que o usuário não tem mais). O usuário
optou por reemitir as creds afetadas e implementar a defesa
arquitetural para que isso NÃO possa repetir.

## Causa raiz #2 — `inheritProfileRoutingFields` silencioso

`internal/chat/interactor.go` definia `inheritProfileRoutingFields(base,
fallback)`: se `base.Chat.LLMProvider` estava vazio, a função
substituía silenciosamente pelo `fallback.Chat.LLMProvider`. O caller
em `PrepareContext` chamava com `fallback = profileMgr.GetActive()`
(o profile global ativo).

Profiles legacy (criados antes da introdução do sentinela `$default`)
tinham `LLMProvider=""` em vez de `"$default"`. O efeito: ao escolher
profile Y na UI, qualquer campo de routing vazio em Y era preenchido
com o do profile X global ativo — sem nenhum sinal pra UI nem pro
usuário. Daí o "selecionei Y, mas usa o provider de X".

A herança era uma "conveniência" que escondia configuração ambígua e
produzia profiles híbridos em memória que não correspondiam a nada no
disco.

## Decisões e fixes

### D1. `LoadFromStore` mágico vira API explícita

Removido `Manager.LoadFromStore(ctx)`. Em vez dele:

- `Manager.LoadInstanceSecrets(ctx)` — carrega APENAS instance
  secrets. É o que o boot pré-login chama.
- `Manager.LoadUserCredentials(ctx, userID)` — carrega todas as
  credenciais do `userID`. Exige `userID` não vazio. É o que o pós-Login
  chama.

Cada caller diz no nome o que precisa. O bug "passei o ctx errado e
ninguém me avisou" deixa de ser representável: existe uma função para
cada intenção e elas não se confundem.

Callers atualizados:

- `app_auth.adoptLegacyDataForUser` → `LoadUserCredentials(userID)`.
- `app_credentials.initCredentialManager` → `LoadInstanceSecrets`.
- `app_credentials.configureCredentialManager` → `LoadInstanceSecrets`.

### D2. Profiles legacy normalizados na carga

`profiles.Manager.Get` agora chama `normalizeRoutingFields(profile)`
imediatamente após decodar o JSON. Profiles com
`Chat.LLMProvider`/`Chat.Model`/`Voice.Assistant.LLMProviderID`/
`Input.LLMProviderID` vazios passam a expor `$default` para o resto
do app. `providers.Service.ResolveProfileDefaults` já sabe resolver o
sentinela para o provider/modelo default do user.

`inheritProfileRoutingFields` foi removido em
`internal/chat/interactor.go`. O `PrepareContext` agora carrega o
profile escolhido tal qual o disco (já normalizado) e segue. Sem
fallback cruzado entre profiles.

### D3. Contratos limpos em `DeleteCredential` / `SaveCredential`

`DBStore.DeleteCredential` e `DBStore.SaveCredential` rejeitam pattern
vazio (`ErrEmptyPatternDelete` / erro simétrico). Isso não é defesa
contra causa raiz nenhuma — é só fechar uma ambiguidade real do
contrato: pattern faz parte da identidade da credencial e "limpar
tudo" precisa ser expressado como iteração sobre a lista visível, não
como uma chamada sem nome.

### D4. `ClearAllCredentials` itera por construção

`controllers/SettingsController.ClearAllCredentials` e
`internal/config/SettingsService.ClearAllCredentials` listam
credenciais visíveis ao usuário (`ListVisibleCredentialsWithContext`,
que já filtra instance secrets e cross-user) e iteram chamando
`DeletePattern(ctx, p)` por pattern. Sem chamada "limpar tudo" sem
nome. `SettingsController.ClearAllCredentials` exige `RequireUserID`.

### D6. MCP autoconnect só pós-login

`mcp.Manager.LoadConfigs` agora APENAS popula o estado em memória
(parsea os JSONs e registra `m.servers`). Não dispara mais nenhum
`go Connect(slug)`.

Adicionado `mcp.Manager.AutoConnectAll(ctx)` que conecta
sequencialmente (ordem determinística por slug) todos os servidores
`Enabled && AutoConnect`. É chamado de `reloadUserScopedRuntime`,
depois de `LoadUserCredentials` ter populado o cache user-scoped do
Manager. O ctx passado herda apenas o `userID` do contexto
autenticado — não usa o ctx-com-timeout do reload (esse é curto
demais para N handshakes seriais).

### D7. `loadUserTokens` / `loadClientCreds` exigem ctx user-scoped

Assinaturas mudaram para `loadUserTokens(ctx, credMgr, slug)` e
`loadClientCreds(ctx, credMgr, slug)`. Internamente usam
`Manager.GetByPatternWithContext(ctx, ...)` em vez de
`GetByPattern(...)`. Todos os callsites passam o ctx vindo de
`m.credentialContext()` (que já existia e devolve o ctx user-scoped
vigente).

`pkceRoundTripper` ganhou `authCtxProvider func() context.Context`
para os refreshes assíncronos da `oauth2.TokenSource` (chamados pelo
oauth2 lib em background, sem ctx do caller): persistTokens e
persistClientCreds usam `rt.authCtx()` para gravar com o `user_id`
correto.

### D8. `DisconnectAll` no logout/troca de user

`mcp.Manager.DisconnectAll` (novo método) fecha todas as conexões
abertas SEM cancelar o ctx base do Manager. É o caminho do logout: as
credenciais user-scoped do user que sai vão ficar inacessíveis (vault
Lock), então as conexões MCP precisam soltar — mas o Manager continua
vivo para reconectar quando o próximo user logar. `App.Logout` chama
`DisconnectAll` antes de `vaultSvc.Lock()`.

`CloseAll` continua sendo o shutdown definitivo (cancela `m.cancel()`
e desconecta tudo). Usado só no Stop do app.

### D10. Identidade pública da DEK (`dek_id`) e contrato `PersistDEKConsistent`

A defesa contra a causa raiz #4 é uma **invariante observável** no
banco: `credential_key_wraps.dek_id`, calculado a partir de
`credentials.DEKIdentity(dek)` (sha256("assistente-dek-id-v1\x00" ||
dek) truncado em 16 bytes hex). Determinístico, irreversível,
domain-separated.

Mudanças concretas:

- **Coluna `dek_id`** em `credential_key_wraps`. `WrapDEK` popula a
  partir de `DEKIdentity(dek)` em toda escrita; wraps pré-AEP-0061
  vêm com string vazia e são adotados pelo boot (assumindo a DEK do
  keychain como referência).
- **`credentials.PersistDEKConsistent(ctx, store, dek, …)`** é a ÚNICA
  via permitida em código de produção pra escrever DEK no keychain. Ela:
  1. Lê a DEK que está no keychain.
  2. Se já tem DEK divergente (DEKIdentity diferente da nova),
     retorna `ErrDEKWouldOverwrite` SEM gravar.
  3. Se já tem a mesma DEK ou keychain está vazio, grava
     normalmente.
  4. Adota `dek_id` em wraps existentes que ainda estejam vazios.
- **`saveDEKToKeychain` foi privatizada** (era `SaveDEKToKeychain`
  exportada). Caller que tente escrever direto não compila — o code
  review e o `go vet` do package pegam.
- **`OverwriteKeychainDEK`** existe como rota explícita de recovery
  quando o usuário deliberadamente confirma "sim, troque a DEK do
  keychain pela do `master_wrap`, aceitando perder X credenciais
  cifradas com a anterior". Documentado para uso APENAS após
  confirmação interativa.
- **`Manager.verifyDEKConsistency(ctx)`** roda no boot (chamado de
  `LoadInstanceSecrets`). Compara `DEKIdentity(m.encKey)` com
  `wraps.master.dek_id`:
  - bate → OK.
  - wrap sem `dek_id` (legado) → adota.
  - DEKs divergentes → `m.persist = false` (bloqueia escritas novas
    para não perpetuar dois universos), log de CRITICAL e
    `IntegrityStatus.OK = false` com `Reason` legível.
- **Detecção de credenciais ilegíveis**: o boot varre todas as
  credenciais persistidas, tenta decifrar com a DEK atual e marca os
  IDs que falharam em `IntegrityStatus.UnreadableCredentialIDs`.
- **Auto-purge no boot** (decisão do usuário pela política
  `auto_purge` em vez de `keep_marked` ou `interactive_now`):
  `App.handleVaultIntegrityOnBoot` chama
  `Manager.PurgeUnreadableCredentials` removendo as creds órfãs após
  log explícito. UI consulta `App.GetVaultIntegrityStatus` para
  mostrar histórico.
- **`vault.Unlock` fail-closed**: se desembrulhar a DEK do
  `master_wrap` produz uma DEK diferente da que está no keychain,
  Unlock NÃO sobrescreve mais o keychain; mantém a sessão unlocked em
  runtime e devolve sucesso (a DEK em runtime serve para decifrar
  creds dessa DEK). UI deve oferecer `UnlockOverwriteKeychain` se o
  usuário quiser deliberadamente "esquecer" a DEK_keychain atual.

Por que não tentar "auto-heal" recifrando creds antigas com a DEK
atual: o app não tem a DEK antiga (essa é o problema), não consegue
decifrar pra recifrar. Recovery requer senha mestre / recovery key
(rota `OverwriteKeychainDEK` ou — futura Fase 3 — uma ferramenta
interativa que desembrulha o wrap, recifra todas as creds pra DEK
atual e regera os wraps).

### D9. Arbiter global de OAuth flows interativos

`oauthFlowArbiter sync.Mutex` (var de package em
`internal/mcp/oauth.go`) serializa flows OAuth interativos entre
servidores DIFERENTES. `pkceRoundTripper.authorize` faz Lock/Unlock
no início do método. Sem isso, dois servidores que precisam reauth
simultaneamente disparavam `browser.OpenURL` ao mesmo tempo —
sintoma do "N janelas no startup".

Sem timeout próprio: cada `authorize()` já tem seu (5min PKCE, poll
budget Device Flow). Um flow congelado bloqueia o próximo, e isso é
o desejado — não faz sentido empilhar fluxos abertos.

### D5. Testes que provam os contratos

- `internal/credentials/manager_load_scope_test.go`:
  - `LoadInstanceSecrets` não carrega user-scoped.
  - `LoadUserCredentials(userID)` carrega tudo do user e nada de outros.
  - `LoadUserCredentials("")` falha com `ErrUserScopeRequired`.
- `internal/credentials/db_store_safety_test.go`:
  - Pattern vazio rejeitado em delete/save.
  - Cross-user isolation no delete.
  - Instance secret deletado pelo escopo `user_id=''`.
  - Caller anônimo não consegue apagar credenciais user-scoped.
- `internal/config/settings_service_test.go`:
  - `ClearAllCredentials` itera N patterns visíveis e nunca passa
    `pattern=""` adiante.
- `internal/chat/interactor_test.go`:
  - `TestPrepareContext_ProfileSlugDoesNotInheritFromGlobalActiveProfile`
    bloqueia regressão da causa raiz #2.
- `internal/database/credential_loss_repro_test.go`:
  - O pipeline canônico do boot AEP-0052 não perde nem corrompe rows
    nem zera `token_enc`/`client_secret_enc` (defesa contra o bug
    histórico de `migrateToUUIDv7` em 27/04, fixado em commit
    `227f0333`). Confirma que o sintoma original NÃO era perda no DB.
  - `AdoptLegacyData` é idempotente e nunca dropa rows claimed.
- `internal/mcp/manager_autoconnect_test.go`:
  - `LoadConfigs` não inicia conexão (zero `m.connections` após).
  - `AutoConnectAll` respeita ctx cancelado.
  - `loadUserTokens(userCtx, ...)` acha token user-scoped;
    `loadUserTokens(bgCtx, ...)` retorna nil pelo filtro anti-leak;
    `loadUserTokens(otherUserCtx, ...)` não vê tokens de outro user.
  - `oauthFlowArbiter` serializa N callers concorrentes (max=1).
  - `DisconnectAll` mantém o Manager utilizável (não cancela ctx).
- `internal/credentials/dek_consistency_test.go` (defesa #4):
  - `DEKIdentity` é determinística e distinta para DEKs distintas.
  - `PersistDEKConsistent` recusa sobrescrever DEK divergente
    (`ErrDEKWouldOverwrite`) e é idempotente quando a DEK bate.
  - `PersistDEKConsistent` grava normalmente quando keychain está
    vazio e popula `dek_id` em wraps existentes.
  - `OverwriteKeychainDEK` sobrescreve sem validar (rota de recovery).
  - **`TestVerifyDEKConsistency_DetectaDivergenciaERevogaPersistencia`**
    é o teste de regressão direto do incidente: keychain=DEK_Y,
    `master_wrap` embrulha DEK_X, cred cifrada com DEK_X — boot
    detecta, marca `OK=false`, revoga `m.persist`, lista a cred em
    `UnreadableCredentialIDs`.
  - `TestVerifyDEKConsistency_AdotaDekIDLegado` cobre a migração de
    instalações pré-AEP-0061.
  - `TestPurgeUnreadableCredentials_RemoveOrfãsEDeixaResto` valida
    a política `auto_purge`.
  - `TestSetupMasterKey_RecusaSobrescreverDEKExistente` cobre o
    cenário histórico do incidente: chamar Setup quando já há DEK
    no keychain agora retorna `ErrDEKWouldOverwrite` em vez de
    sobrescrever silenciosamente.

## Invariantes congelados

1. `Manager` não tem método "carregar credenciais" cuja semântica
   dependa de o ctx ter ou não userID. Cada caminho de carga é uma
   função nominal: `LoadInstanceSecrets` ou `LoadUserCredentials(userID)`.
2. Profile lido de disco com routing field vazio é normalizado para
   `$default` em `Manager.Get`. Profile NUNCA herda silenciosamente
   campos de outro profile.
3. Pattern vazio é sempre erro em `DBStore.DeleteCredential` e
   `DBStore.SaveCredential`.
4. "Limpar tudo" no UI é sempre iterativo: lista visível + delete por
   pattern. Cada delete é nominal e escopado.
5. `AdoptLegacyData` faz `UPDATE` do `user_id` em órfãs e DELETE
   apenas de duplicatas órfãs cujo pattern já está claimed pelo user
   (resolução determinística), nunca apaga rows com tokens
   legítimos.
6. `AutoMigrate` não dropa colunas de `credential_entries` (defendido
   por `TestMigration_CredentialColumnsMatchGORMModel` e pelo teste
   de reprodução acima).
7. `mcp.Manager.LoadConfigs` NUNCA conecta. Auto-connect é exclusivo
   de `AutoConnectAll(ctx)`, chamado pós-login pelo
   `reloadUserScopedRuntime`.
8. Toda leitura/escrita de credencial dentro de `internal/mcp` recebe
   ctx user-scoped explícito. `loadUserTokens` / `loadClientCreds`
   exigem ctx; `pkceRoundTripper` carrega `authCtxProvider` para
   refreshes assíncronos.
9. Logout chama `mcp.Manager.DisconnectAll`, que fecha conexões mas
   mantém o Manager vivo para o próximo login.
10. Flows OAuth interativos do MCP são serializados pelo
    `oauthFlowArbiter`. Dois servidores com reauth pendente ao mesmo
    tempo abrem browser em série, não em paralelo.
11. **`DEKIdentity(DEK_keychain) == credential_key_wraps.master.dek_id`**
    é uma invariante observável que o boot DEVE validar antes de
    aceitar persistência. Divergência → `m.persist=false` e
    `IntegrityStatus.OK=false` exposto pra UI.
12. Toda escrita de DEK no keychain em código de produção passa por
    `credentials.PersistDEKConsistent` (rejeita sobrescrita silenciosa)
    ou `credentials.OverwriteKeychainDEK` (rota explícita de recovery
    deliberado). `saveDEKToKeychain` é não-exportada por construção.
13. Wraps gravados via `WrapDEK`/`SaveKeyWrap` SEMPRE carregam
    `dek_id`. Wraps pré-AEP-0061 com `dek_id == ""` são adotados pelo
    boot a partir da DEK do keychain do momento (pressuposto: o
    keychain é a referência válida no upgrade).
14. Credenciais que não decifram com a DEK do keychain ficam listadas
    em `Manager.IntegrityStatus().UnreadableCredentialIDs`. Política
    do produto: `App.handleVaultIntegrityOnBoot` faz purge automático
    (decisão `auto_purge` em AEP-0061).

## Riscos residuais

- Profiles legacy continuam no disco com campos vazios; a
  normalização acontece em memória. Não migramos os JSONs porque a
  semântica "vazio == `$default`" é estável e migrar arquivos do
  usuário sem evento explícito é gambiarra. `Manager.Update` (chamada
  pelo editor de profile) sobrescreve com `$default` quando o user
  edita.
- Telemetria de cobertura: não temos métrica em produção que conte
  "quantas credenciais entraram em memória após Login". Se o bug do
  D1 ressurgir por alguma rota nova de bootstrap, só o teste de
  regressão pega — em runtime o sintoma volta a ser
  `unresolvedCredentialError`.

## Critérios de aceitação

- [x] `internal/credentials/manager_load_scope_test.go` passa.
- [x] `internal/credentials/db_store_safety_test.go` passa.
- [x] `internal/database/credential_loss_repro_test.go` passa.
- [x] `internal/chat/interactor_test.go::TestPrepareContext_ProfileSlugDoesNotInheritFromGlobalActiveProfile` passa.
- [x] `internal/mcp/manager_autoconnect_test.go` passa (5 testes).
- [x] `go test ./...` permanece verde.
- [x] `inheritProfileRoutingFields` removido do código.
- [x] `Manager.LoadFromStore` removido; substituído por
      `LoadInstanceSecrets` / `LoadUserCredentials`.
- [x] `mcp.Manager.LoadConfigs` não dispara mais `go Connect`;
      `AutoConnectAll(ctx)` chamado de `reloadUserScopedRuntime`.
- [x] `loadUserTokens` / `loadClientCreds` exigem ctx user-scoped
      em todos os callsites.
- [x] `App.Logout` chama `mcp.Manager.DisconnectAll`.
- [x] `oauthFlowArbiter` serializa flows OAuth interativos do MCP.
- [x] Recuperação operacional do banco do usuário concluída antes
      desta investigação (13 credenciais restauradas a partir de
      `conversations.db.pre-uuid.bak`).
- [x] `internal/credentials/dek_consistency_test.go` passa
      (9 testes, incluindo o de regressão direto do incidente).
- [x] `credential_key_wraps.dek_id` adicionado via AutoMigrate.
- [x] `WrapDEK` popula `DekID` em toda escrita.
- [x] `saveDEKToKeychain` privatizada; `PersistDEKConsistent` é a
      única via de produção pra escrever DEK no keychain.
- [x] `Manager.LoadInstanceSecrets` chama `verifyDEKConsistency` no
      boot; divergência revoga `persist`.
- [x] `App.handleVaultIntegrityOnBoot` purga creds ilegíveis após
      log explícito (política `auto_purge`).
- [x] `App.GetVaultIntegrityStatus` expõe estado pra UI/Wails.
- [x] `vault.Unlock` fail-closed contra sobrescrita silenciosa;
      `vault.UnlockOverwriteKeychain` é a rota explícita de recovery.
