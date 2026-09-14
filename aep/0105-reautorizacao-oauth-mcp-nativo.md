# AEP-0105 — Reautorização OAuth interativa para MCP nativo

**Status:** 🚧 In Progress — backend (ação `ReauthorizeServer`, guarda de token
expirado no caminho nativo e sinalização `NeedsReauth`), binding Wails, UI de
reautorização e testes entregues. Documentação de usuário e verificação de CI
pendentes de finalização no PR.

## Resumo

Servidores MCP autenticados por OAuth2 PKCE (ex.: Atlassian) podem ter o
`access_token` expirado sem que o app perceba, especialmente no **modo nativo**,
em que o provedor LLM (ex.: Responses API da OpenAI) conecta direto ao servidor
MCP remoto usando um `Authorization: Bearer` fornecido pelo app. Nesse modo, o
401 acontece **server-side**, longe do cliente OAuth local, então o único ponto
que abria o browser para reautorizar (`pkceRoundTripper.authorize`, disparado por
um 401 incidental do caminho *bridge*) nunca é acionado.

Esta AEP define duas garantias complementares:

1. **Não entregar token morto em silêncio.** Antes de montar a lista de
   servidores MCP nativos, o app verifica a expiração do token OAuth e tenta um
   refresh forçado. Se o refresh falhar (sem `refresh_token`, `invalid_grant`,
   etc.), o servidor é **excluído** do caminho nativo e **sinalizado** como
   precisando de reautorização, em vez de enviar um Bearer expirado ao provedor.
2. **Reautorização interativa sob demanda.** Uma ação explícita
   (`Manager.ReauthorizeServer`) força o fluxo OAuth interativo (abre o browser)
   reutilizando a infraestrutura existente, independentemente de o servidor estar
   em modo nativo ou bridge e sem depender de um 401 incidental.

## Motivação

- O `access_token` do Atlassian expira (~1h) e o app continuava entregando o
  token vencido ao provedor nativo. Sintoma: `Failed to fetch accessible
  resources: 401` e o servidor remoto fechando a conexão com erro `4001`.
- `GetEligibleNativeMCPServers` lia o token cru do cofre
  (`GetByPatternWithContext(userTokensPattern(slug))`) **sem checar expiração nem
  renovar**.
- `Manager.Reconnect` só faz `Disconnect`+`connect`, relendo o mesmo token
  vencido — não reautoriza.
- O refresh proativo (`tokenRefreshLoop` → `checkAndRefreshToken` →
  `refreshOAuthTokenBestEffort`) é silencioso via `refresh_token`; quando não há
  `refresh_token` utilizável, ele apenas loga e desiste. Não havia nenhum caminho
  de reautorização interativa fora do 401 incidental do bridge.

## Decisões

### D1 — Guarda de token no caminho nativo (sem deadlock)

`GetEligibleNativeMCPServers` foi reestruturada para não segurar o `RLock` do
`Manager` durante operações que adquirem locks (refresh/persistência):

- `collectNativeMCPCandidates()` coleta, **sob `RLock`**, um snapshot imutável
  (`nativeMCPCandidate`: slug, name, url, authType, toolNames) dos servidores
  elegíveis (conectados, HTTP, com tools, URL segura, `prefer_bridge=false`).
- Fora do lock, `resolveNativeAuthToken(ctx, candidate)` resolve o Bearer:
  - Para `oauth2_pkce` com token **expirado ou perto de expirar**
    (`nativeTokenExpiredOrNear`, mesmo `tokenRefreshThreshold` do loop proativo),
    chama `refreshOAuthTokenBestEffort(ctx, slug, force=true)`.
  - Se o refresh falhar, chama `signalNeedsReauth(...)` e retorna `ok=false`; o
    servidor **não** entra na lista nativa.
  - Se o refresh der certo, relê o token novo do cofre, limpa o sinal e entrega.
  - Tokens não-OAuth (Bearer por hostname) e `AuthNone` preservam o comportamento
    anterior (entregar como está / sem Bearer).

### D2 — Sinalização `NeedsReauth` tipada, sem spam

- `ServerStatus.NeedsReauth` (runtime) e `ServerInfo.NeedsReauth` (frontend-safe)
  expõem o estado. `ServerInfo.AuthType` também passa a ser exposto para a UI
  habilitar a ação apenas em servidores `oauth2_pkce`.
- `signalNeedsReauth` emite `mcp:server_needs_reauth`
  (`MCPServerReauthEvent{Slug,Name,Reason}`) **apenas na transição `false→true`**,
  para não repetir o alerta a cada turno de chat (a resolução roda por turno).
- `clearNeedsReauth` emite `mcp:server_reauthorized` apenas na transição
  `true→false` (após refresh bem-sucedido ou reautorização).

### D3 — `ReauthorizeServer(ctx, slug)`

- Só aplicável a `AuthOAuth2PKCE`; caso contrário retorna erro descritivo.
- Monta o `pkceRoundTripper` via `buildPKCERoundTripperForServer` (extraído de
  `buildAuthHTTPClient`, que agora o reutiliza) e chama `rt.authorize(ctx)`, que
  respeita o `oauthFlowArbiter` global (serializa flows interativos entre
  servidores — AEP-0061) e persiste os tokens ao concluir.
- Ao final, limpa `NeedsReauth` e reconecta o servidor para adotar o token novo e
  atualizar tools/resources/prompts.
- Exposto como binding Wails autenticado `ReauthorizeMCPServer(slug)` (via
  `controllers.MCPController`), fora da allowlist não autenticada.

### D4 — `offline_access` (verificação)

Confirmado que `effectiveScopes()` já acrescenta `offline_access` quando o auth
server o anuncia em `scopes_supported` (RFC 8414/9728), evitando `invalid_scope`
em servidores que não o suportam. Isso é o que garante o `refresh_token` no
Atlassian (issue #193). Comportamento coberto por
`TestEffectiveScopes_*` em `internal/mcp/oauth_test.go`. **Nenhuma mudança de
código necessária aqui** — apenas registro da decisão e dos testes existentes.

## Fases

- **F1 — Guarda nativa + sinalização (backend).** ✅ Concluída.
  `collectNativeMCPCandidates`, `resolveNativeAuthToken`,
  `nativeTokenExpiredOrNear`, `signalNeedsReauth`/`clearNeedsReauth`, campos
  `NeedsReauth`/`AuthType` e evento `MCPServerReauthEvent`.
- **F2 — Reautorização interativa (backend + binding).** ✅ Concluída.
  `ReauthorizeServer`, `buildPKCERoundTripperForServer`,
  `ReauthorizeMCPServer` no controller e no Wails API.
- **F3 — UI.** ✅ Concluída. Ação "Reautorizar" (menu de linha e toolbar) apenas
  para `oauth2_pkce`, badge "Reautorização necessária" na coluna de status,
  i18n pt-BR/en/es, acessibilidade (aria/announce) e tokens de tema.
- **F4 — Testes.** ✅ Concluída. Backend (`internal/mcp/reauth_test.go`) e
  frontend (`frontend/src/store/mcpStore.test.ts`).
- **F5 — Documentação de usuário + CI verde.** 🚧 Em andamento no PR.

## Riscos

- **Deadlock de mutex.** Mitigado pela coleta sob `RLock` + resolução fora do
  lock (D1). Refresh/persistência têm locks próprios.
- **Múltiplos browsers simultâneos.** Mitigado pelo `oauthFlowArbiter` (AEP-0061),
  reutilizado tanto pelo caminho incidental quanto por `ReauthorizeServer`.
- **Spam de eventos por turno.** Mitigado pela emissão apenas nas transições de
  estado de `NeedsReauth`.
- **Fluxo interativo longo.** `ReauthorizeServer` usa contexto sem timeout
  (autenticado) para acomodar a interação no browser.

## Critérios de aceitação

- [x] Token OAuth expirado sem refresh possível **não** é entregue ao caminho
  nativo; o servidor é sinalizado como `NeedsReauth`.
  (`TestGetEligibleNativeMCPServers_ExpiredWithoutRefreshSignalsReauthAndSkips`)
- [x] Token expirado com `refresh_token` válido é renovado e entregue.
  (`TestGetEligibleNativeMCPServers_ExpiredTokenRefreshedAndDelivered`)
- [x] Token válido é entregue sem refresh desnecessário.
  (`TestGetEligibleNativeMCPServers_ValidTokenDeliveredWithoutRefresh`)
- [x] `ReauthorizeServer` roda o fluxo interativo (browser stub), persiste o
  token e reconecta.
  (`TestReauthorizeServer_RunsInteractiveFlowPersistsTokenAndReconnects`)
- [x] `ReauthorizeServer` rejeita servidores não-OAuth e desconhecidos.
- [x] `offline_access` é solicitado quando anunciado (`TestEffectiveScopes_*`).
- [x] UI expõe "Reautorizar" distinta de "Reconectar", só para `oauth2_pkce`,
  com i18n nos 3 locales e acessibilidade.
- [ ] Documentação de usuário atualizada e CI verde.

## Referências

- AEP-0021 — MCP Modo Nativo (elegibilidade e passthrough do Bearer).
- AEP-0033 — MCP OAuth Auto-Discovery (`effectiveScopes`, `scopes_supported`).
- AEP-0061 — Incidente de perda de credenciais e defesas (`oauthFlowArbiter`,
  persistência de tokens).
