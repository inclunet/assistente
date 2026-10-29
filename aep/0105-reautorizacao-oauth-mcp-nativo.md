# AEP-0105 — Reautorização OAuth interativa para MCP nativo

**Status:** 🚧 In Progress — backend (ação `ReauthorizeServer`, guarda de token
expirado no caminho nativo e sinalização `NeedsReauth`), binding Wails, UI de
reautorização, testes, documentação de usuário e CI verde entregues no PR #771.
Integração com o ciclo compartilhado do AEP-0112 em andamento; recuperação interativa implementada; confirmação com servidor real e NVDA pendente.

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
  - Autorizações compostas resolvem validade e renovação no `oauthflow.Service`.
    Cadastros OAuth antigos são recusados com `oauth_migration_required`, antes
    de resolver tokens, até a migração explícita prevista no AEP-0112.
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
- Autorizações compostas usam `oauthflow.Service.AuthorizeUsing`, com lease e
  persistência no registro composto; o protocolo interativo mantém a arbitragem
  global (AEP-0061). Cadastros antigos exigem **Reconectar e migrar**.
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
  validação de validade pelo serviço compartilhado, `signalNeedsReauth`/`clearNeedsReauth`, campos
  `NeedsReauth`/`AuthType` e evento `MCPServerReauthEvent`.
- **F2 — Reautorização interativa (backend + binding).** ✅ Concluída.
  `ReauthorizeServer`, `authorizeManagedOAuthInAttempt`,
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
- [x] Documentação de usuário atualizada (`MCP_CONFIG_EXAMPLES.md`) e CI verde
  (backend, backend-race, bindings, e2e, frontend e scripts no PR #771).

## Referências

- AEP-0021 — MCP Modo Nativo (elegibilidade e passthrough do Bearer).
- AEP-0033 — MCP OAuth Auto-Discovery (`effectiveScopes`, `scopes_supported`).
- AEP-0061 — Incidente de perda de credenciais e defesas (`oauthFlowArbiter`,
  persistência de tokens).


## Integração incremental com AEP-0112

O status desta proposta permanece **In Progress**. Para os novos cadastros MCP
com `oauth_authorization_id`, a reautorização usa o lifecycle compartilhado:
`oauthProtocol` executa somente o protocolo; `oauthflow.Service.AuthorizeUsing`
possui o lease e persiste o resultado no registro composto. Resolução nativa,
bridge e renovação sob demanda usam o mesmo serviço. O temporizador de renovação
por conexão foi removido. Cadastros sem referência composta permanecem disponíveis
para snapshot e migração explícita, mas não conectam nem renovam tokens pelo
runtime antigo. Autoconexão e requisições incidentais desses cadastros não
abrem navegador: Conectar e Reautorizar são as entradas interativas explícitas.

Evidências: `TestManagedOAuthPKCEDCRPersistsCallbackAndRefreshAfterRestart`,
`TestManagedOAuthDeviceAndStartupNeverOpenBrowserImplicitly` e
`TestManagedOAuthDisconnectCancelsRefreshPreflight` em
`internal/mcp/oauth_authorization_test.go`.

A retirada do fallback nativo antigo (AEP-0112) elimina a leitura/renovação PKCE
pelo transporte legado. A garantia de validade é coberta pelo lifecycle composto;
`TestManagedNativeRefreshUsesFreshClientWithoutBootstrapOverwrite` prova segredo
atual entre instâncias, rotação persistida e recusa de grant incerto. Credenciais
históricas permanecem recuperáveis, mas não autorizam fallback por hostname.

O AEP-0112 também remove fisicamente o transporte PKCE, token sources e APIs de
escrita/lease históricos. Reautorização usa somente o adaptador de protocolo e
o record compartilhado. DCR persiste cliente/callback por checkpoint do serviço;
falhas de gravação impedem abertura do browser e não publicam registro parcial.
Os testes de callback, rollback, concorrência e recuperação foram migrados para
esse caminho, preservando as garantias anteriores. O status continua In Progress
enquanto os aceites funcionais da integração compartilhada estiverem pendentes.

## Recuperação interativa de autorização — 28/10/2026

Status: **In Progress**. Contrato refinado com o mantenedor: o chat desktop pode
abrir o fluxo OAuth existente quando uma ação em curso precisa do servidor e o
refresh não resolve. O contexto desse turno é explícito e compartilhado pelas
ferramentas. Cada autorização recebe no máximo uma tentativa interativa por turno;
recusa/cancelamento é preservado nas próximas chamadas desse turno. Uma nova ação
explícita pode tentar novamente. O árbitro OAuth existente continua serializando
consentimentos e mantendo a mesma persistência e proteção de sessão.

Autoconexão, manutenção, listagem e jobs apenas registram `NeedsReauth`. Subagentes
em background não herdam o escopo interativo do turno que os iniciou. A tool
`mcp_server` propaga o contexto da ação: não herda a permissão incondicional do
botão desktop Conectar. `connect` reutiliza uma conexão já existente sem exigir
nova resolução OAuth; uma chamada real ao recurso continua validando o token. Client Credentials continua exigindo configuração, sem
abrir navegador. No modo nativo só servidores selecionados pelo plano de tools
podem iniciar recuperação; o contexto da ação chega à resolução da credencial.

Antes do envio ao recurso, a chamada aguarda autorização e segue uma vez. Se o
recurso já recebeu a requisição e recusou o token, a reautorização pode recuperar
as próximas chamadas, mas a requisição original não é repetida por esse fluxo.
Não se cria outra pipeline de mensagens nem se reenvia uma conversa.

A sinalização central cobre transporte, modo nativo e autoconexão. Marcar e limpar
pendência exige a mesma identidade do consumidor; respostas obsoletas não mudam
o estado de uma autorização substituta. A UI tem um único proprietário de eventos
MCP por sessão, anuncia a abertura do navegador pelo serviço de acessibilidade e
mantém Reautorizar na página MCP. Não há aviso persistente obrigando clique extra.
Trocar idioma não reinicia o estado; logout remove assinaturas e resultados tardios.

- [x] Sinalização, recuperação e proteção contra consumidor substituído:
  `reauth_presentation_test.go`.
- [x] Consentimento sob demanda, concorrência e recusa sem repetição no turno:
  `interactive_oauth_test.go`.
- [x] Assinatura global, anúncio, deduplicação e isolamento de sessão:
  `frontend/src/hooks/useMCPRuntime.test.tsx`.
- [ ] Validar a experiência real e anúncio com NVDA após instalar.


## Uso após iniciar desconectado — 28/10/2026

Status: **In Progress**. A recuperação interativa agora alcança ferramentas
previamente descobertas cujo servidor iniciou desconectado. O carregamento
sob demanda e a seleção preloaded usam `ConnectWithContext`, aguardam o OAuth
existente e confirmam a definição atual. Busca e startup continuam não interativos.
Contrato de catálogo e política: AEP-0081. Evidência automatizada:
`TestCatalogRecoveryAfterStartupRequiresUseAndFreshDiscovery`; experiência real
com servidor e NVDA permanece pendente.
