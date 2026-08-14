# AEP-0088 — Concluir a migração Strangler Fig da borda Wails (`App`)

**Status:** 📝 Draft  
**Issue:** [#248](https://github.com/inclunet/assistente/issues/248)

## Resumo

A borda pública do desktop ainda é um único `*App` (`internal/app`): dezenas de
campos, centenas de métodos em dezenas de `app_*.go`, quase todos pass-through
para `controllers` → serviços. A “Clean Arch Fase 2” já existe por dentro
(hexagonal, ports, controllers), mas o Wails só enxerga `App` — pagamos o custo
das duas arquiteturas ao mesmo tempo.

Este AEP define o **alvo** da migração Strangler Fig, as decisões que destravam
a implementação e um **plano em fases** com PRs pequenos por domínio, até a
borda Wails deixar de ser uma fachada monolítica.

## Motivação

Estado atual (ordem de grandeza na `main` ao redigir este AEP):

- ~80 arquivos `app_*.go` / helpers no pacote `internal/app`
- Dezenas de arquivos com `func (a *App)` (centenas de métodos no total)
- `requireAuthenticatedContext` repetido da ordem de ~200 vezes
- `StartupWithAdapters` com ~380 linhas de wiring manual
- Aliases de DTO `type X = controllers.X` em `app.go` e outros (acoplamento
  invertido: a borda Wails importa tipos do controller)
- `main.go` faz `Bind: []interface{}{a}` — só o `App` é bindado

Consequências:

1. **Toda feature nova toca três camadas** mesmo quando a lógica já está no
   controller/serviço.
2. **Auth é copy-paste**: fácil esquecer ou divergir o fail-closed.
3. **Bindings TypeScript** herdam a superfície do `App`; `map[string]any` e
   aliases obscurecem o contrato.
4. **Startup e testes** ficam frágeis: subir “só um domínio” exige o monólito.
5. **Onboarding e review** pagam o mapa mental do `App` inteiro.

O ganho desejado não é performance perceptível: é **velocidade e segurança para
mudar o backend** sem o `App` ser gargalo e fonte de regressão.

## Decisões

### D1. Alvo: multi-bind por domínio; `App` vira só ciclo de vida

**Rejeitado como alvo final:** gerar automaticamente uma fachada `App` que
continue concentrando todos os métodos. Isso preserva o monólito na API pública
e só esconde o copy-paste.

**Alvo:** o Wails faz `Bind` de **vários structs** — um por domínio de borda
(ex.: `SkillsAPI`, `ProfilesAPI`, `MCPAPI`, …) — além de um `App` enxuto que
só cuida de `Startup` / `Shutdown` / adapters de janela e o que for
intrinsecamente global.

Cada struct de domínio:

- expõe métodos Wails tipados;
- delega a **um** controller (ou use-case) sem pass-through 1:1 inútil;
- não importa DTOs “de dentro” do controller: usa tipos do pacote neutro (D5).

A migração é Strangler Fig: domínios saem do `App` um a um; enquanto um domínio
não migrou, seus métodos permanecem em `App` e o frontend continua importando
de `@wailsjs/go/app/App`. Quando o domínio migra, o frontend passa a importar
do módulo gerado daquele bind (e o CI de bindings cobre a regeneração).

### D2. Autenticação em um único ponto na borda — `WithUser` (fechado na Fase 1)

`requireAuthenticatedContext` deixa de ser chamado manualmente em cada método
público dos binds de domínio.

**Escolhido:** helper fail-closed `wailsapi.WithUser(session, fn)` no pacote
`internal/wailsapi`. O `App` implementa a interface `Session` reusando
`requireAuthenticatedContext`. Cada método autenticado do bind de domínio chama
só `WithUser` — sem copy-paste do require.

**Rejeitado:** decorator por reflection na construção do bind — frágil com a
geração de métodos do Wails e mais difícil de testar/ler no call site.

Fail-closed permanece: sem usuário no contexto, a chamada retorna erro; não há
caminho “anônimo” implícito para APIs autenticadas (AEP-0052). Métodos sem auth
(login, status de sessão, etc.) ficam no `App` com allowlist explícita.

### D3. API pública tipada — sem `map[string]any` na borda Wails

Novos métodos bindados e, na migração de cada domínio, os existentes:

- usam **structs** (request/response) com campos exportados estáveis;
- não devolvem `map[string]any` nem `interface{}` opacos na assinatura
  pública, salvo allowlist documentada (ex.: payload genérico de evento que
  já tem contrato tipado no frontend à mão — ver regras de `wailsjs/`).

Isso melhora `frontend/wailsjs` e o `tsc` do job de bindings.

### D4. `StartupWithAdapters` vira composição por domínio

O método monolítico é quebrado em **construtores/registradores por domínio**
(`wireSkills`, `wireMCP`, `wireChat`, …) orquestrados por um `Startup` curto.

Regras:

- ordem de dependência explícita (ex.: auth/DB antes de canais);
- cada `wireX` é testável sem subir o app inteiro;
- falha de um domínio não-essencial pode degradar (quando o produto já tiver
  política de degradação); falha de auth/DB continua fatal.

### D5. DTOs da borda em pacote neutro — `internal/apidto` (fechado na Fase 1)

Os tipos que atravessam Wails **não** vivem em `controllers` nem são
reexportados por `app`.

**Pacote alvo:** `internal/apidto` (evita colidir semanticamente com a HTTP
API / `internal/httpapi`). Controllers e binds importam de lá. Aliases
`type X = controllers.X` em `internal/app` são removidos ao migrar o domínio
(ou numa fase dedicada de extração quando o tipo for compartilhado).

### D6. Um domínio por PR (ou fatia menor)

Não há “big bang”. Cada PR:

- migra **um** domínio (ou um subconjunto vertical claro);
- regenera bindings;
- atualiza imports do frontend daquele domínio;
- mantém CI verde (inclui job `bindings`);
- não reescreve domínios vizinhos “já que estamos aqui”.

Ordem fechada na Fase 1 (piloto = **tokens**):

1. **tokens** (piloto) → allowlists → skills → profiles — pass-through puro.
2. Domínios médios: MCP, credentials, settings, updater, hotkeys, tools.
3. Domínios quentes por último: chat/messaging, workspace, ACP, auth
   (auth permanece no `App` por mais tempo por ser transversal).

### D7. Escopo explícito do que *não* é este AEP

Fora de escopo (podem ter issues/AEPs próprias):

- reescrever a arquitetura hexagonal interna dos serviços;
- mudar o protocolo de eventos frontend↔backend (AEP-0040 permanece);
- unificar auth HTTP API e Wails além do necessário para a borda desktop;
- performance de startup como meta principal.

## Fases

### Fase 0 — Este AEP e vínculo com #248 (feita)

- Publicar AEP-0088 e indexar em `aep/README.md`.
- Marcar a issue #248 como épico que implementa este AEP.

**Aceite:** AEP revisável; decisão D1–D7 legíveis sem ler o código.

### Fase 1 — Inventário + spike de multi-bind (feita)

- Inventário por arquivo/domínio no [Anexo A](#anexo-a--inventário-da-borda-app).
- Spike: `internal/wailsapi.Probe` com `StranglerFigProbe()`, bindado em
  `main.go` ao lado do `App`; `wailsjs` regenerado (`wailsapi` / `Probe`).
- D2 fechado: `WithUser` (implementação na Fase 2).
- D5 fechado: `internal/apidto`.
- Piloto da Fase 2+: **tokens**.

**Aceite:** spike no `Bind`; inventário anexo; D2/D5 fechados.

### Fase 2 — Auth único na borda + allowlist (feita)

- `wailsapi.Session` + `WithUser` / `WithUser2`; `App.AuthenticatedContext`.
- Domínio piloto **tokens**: bind `wailsapi.Tokens` (4 métodos); removidos de
  `App`; FE importa `@wailsjs/go/wailsapi/Tokens`.
- Allowlist `UnauthenticatedAppMethods` + testes; Tokens só via `WithUser`.

**Aceite:** domínio piloto sem `requireAuthenticatedContext` manual;
allowlist coberta por teste; nenhum método autenticado fora do mecanismo.

### Fase 3 — Pacote neutro de DTOs (feita)

- Criado `internal/apidto` com `TokenStats` / `ToolUsageBreakdown`.
- `chat` reexporta via alias; controller e bind `Tokens` tipam em `apidto`.
- Zero aliases `type X = controllers.X` no caminho tokens.

**Aceite:** zero aliases `controllers.*` no domínio piloto; job `bindings`
verde; frontend tipado contra os structs gerados.

### Fase 4 — Quebrar `StartupWithAdapters` (feita)

- Extraídos `wireTokens` e `wireAllowlist` de `StartupWithAdapters`.
- Testes de wiring sem GUI (`app_wire_test.go`).

**Aceite:** `StartupWithAdapters` orquestra; wiring de pelo menos dois
domínios em funções separadas.

### Fase 5+ — Migração Strangler por domínio

Para cada domínio da ordem D6:

1. Criar struct de bind do domínio.
2. Mover métodos públicos de `App` → bind (assinaturas tipadas, D3).
3. Registrar no `Bind` de `main.go`.
4. Regenerar `wailsjs`; atualizar imports frontend.
5. Remover métodos/aliases mortos de `App`.
6. PR pequeno; CI + review zerados.

**Aceite por domínio:** frontend não importa mais esses métodos de `App`;
pass-throughs 1:1 daquele domínio sumiram ou são só o thin bind.

**Progresso:**
- [x] **tokens** (piloto Fases 2–4)
- [x] **allowlists** (CRUD + questionnaire → `wailsapi.Allowlists`)
- [x] **skills** (CRUD + invocable/search paths → `wailsapi.Skills`; DTO em `apidto`)
- [x] **tools** (available tools + runtime catalog → `wailsapi.Tools`; DTO em `apidto`)
- [x] **updater** (version/check/apply/start → `wailsapi.Updater`)
- [x] **profiles** (CRUD + active + context providers → `wailsapi.Profiles`)
- [x] **hotkeys** (IsGlobalHotkeySupported → `wailsapi.Hotkeys`; WithUser fail-closed)
- [x] **nettrust** (network allowlist → `wailsapi.NetTrust`; DTO em `apidto`)
- [x] **credentials** (CRUD List/Upsert/Delete/ListExternalSources → `wailsapi.Credentials`; DTOs em `apidto`; vault pré-sessão permanece no `App`)
- [x] **settings** (clear*/test connection/native TTS/reset config → `wailsapi.Settings`)
- [x] **MCP** (servers/tools/resources/OAuth/logs → `wailsapi.MCP`; `MCPServerAuthInfo` em `apidto`)
- [x] **signal** (register/verify/link/unregister/checkAPI/listAccounts → `wailsapi.Signal`; `SignalAPIStatus` em `apidto`)
- [x] **terminal** (sessões PTY: list/create/close/history/run/input/interrupt/stats → `wailsapi.Terminal`; managers e eventos `terminal:*` permanecem no `*App`)
- [x] **memory** (CRUD + search + policy summary → `wailsapi.Memory`; tipos de `memory`/`database`, sem DTO extra)
- [x] **welcome** (NeedsWelcomeWizard dual-mode + RunWelcomeWizard → `wailsapi.Welcome`; sem `WithUser`; não entra em `UnauthenticatedAppMethods` — allowlist só de `*App`)
- [x] **channels_legacy_cleanup** / legacy cleanup (`CleanupLegacyChannelJSON` → `wailsapi.LegacyCleanup`; DTOs em `apidto`)
- [x] **database** (reset/clear + maintenance/stats → `wailsapi.Database`; tipos de `config`/`database`, sem DTO extra)
- [x] **subagent** (ListSubAgentRuns + CancelSubAgentRun → `wailsapi.Subagent`; Manager e delivery parent permanecem no `*App`)
- [x] **tasklist_actions** (custom actions → `wailsapi.TasklistActions`; `CustomActionView` em `apidto`; `customActionEventNames` permanece no `App` para jobs)
- [x] **jobs** (CRUD/runs/catalog/dry-run → `wailsapi.Jobs`; `customActionEventNames` injetado no Attach; helpers MCP dry-run em `jobs_dryrun.go`)
- [x] **media** (UpdateProfileMediaSupport → `wailsapi.Profiles`; estendido em Profiles, sem domínio Media novo; `extractAudioFromMedia` permanece interno no `App`)
- [x] **llm_providers** (CRUD/test/models/default/reload → `wailsapi.LLMProviders`; DTOs em `apidto`; `CreateDefaultLLMProvider` sem WithUser para bootstrap wizard/CLI; helpers `applyInstalledBinaryEnv`/`initLLMClient`/… permanecem no `App`)
- [x] **acp_commands** (`GetAgentSessionCommands` → `wailsapi.ACPCommands`; DTOs em `apidto`; `agentSessionCommandsChanged` permanece no `App`)
- [x] **acp_providers** (DetectACPAgent + TestACPAgent → `wailsapi.ACPProviders`; DTOs em `apidto`)
- [x] **acp_install** (plan/install/update/cancel/remove/list → `wailsapi.ACPInstall`; DTOs em `apidto`; handshake/progresso/repontar permanecem no `*App` via hooks)
- [x] **acp_trust** (GetAgentPermissions + RevokeAgentPermission → `wailsapi.ACPTrust`; DTO em `apidto`; handlers de permissão em tempo de turno e `profileNames` permanecem no `*App`)
- [ ] …

### Fase N — `App` enxuto

Quando os domínios migrados cobrirem a superfície útil:

- `App` só ciclo de vida + o que for comprovadamente global;
- critério métrico: número de `func (a *App)` públicos Wails reduzido ao
  essencial (meta qualitativa: <~20 métodos de ciclo de vida/utilitários, o
  restante nos binds de domínio);
- issue #248 fechada.

**Aceite:** métrica atingida; README/AEP marcados ✅ Done; #248 fechada.

## Riscos

| Risco | Mitigação |
|-------|-----------|
| Regeneração `wailsjs` quebra imports em massa | Um domínio por PR; CI `bindings` obrigatório |
| Multi-bind do Wails com nomes colidentes | Prefixo/nome do struct estável; spike na Fase 1 |
| Auth decorator esquece método novo | Teste/allowlist + convenção no AGENTS.md após Fase 2 |
| Domínio “quente” (chat) migrado cedo demais | Ordem D6: deixar por último |
| Big bang disfarçado em PR “só refatoração” | D6 rígido; review rejeita varredura multi-domínio |

## Critérios de aceitação (épico)

- [x] AEP-0088 publicado; Fase 1 fechou D2 (`WithUser`) e D5 (`internal/apidto`).
- [x] Spike de multi-bind feito (`wailsapi.Probe` no `Bind`).
- [x] Mecanismo único de auth na borda em produção para domínios migrados (`WithUser` + Tokens).
- [x] DTOs da borda fora de `controllers` para domínios migrados (`internal/apidto` tokens).
- [x] `Startup` composto por `wireX` por domínio (`wireTokens`, `wireAllowlist`).
- [ ] Maioria dos métodos Wails fora de `*App`; #248 fechada.
- [ ] Frontend + job `bindings` verdes a cada fase.

## Referências

- Issue [#248](https://github.com/inclunet/assistente/issues/248)
- `main.go` (`Bind: []interface{}{a, wailsapi.NewProbe(), …}`)
- `internal/app` (`App`, `StartupWithAdapters`, `requireAuthenticatedContext`)
- `internal/wailsapi` (binds de domínio + Probe)
- `controllers/` (camada atual de orquestração)
- AEP-0052 (contas / escopo de usuário)
- Regras de `frontend/wailsjs/` em `AGENTS.md`

## Anexo A — Inventário da borda `App`

Contagem de métodos exportados `func (a *App) NomeMaiúsculo` por arquivo
`app_*.go` na `main` da Fase 1 (~326 métodos). O domínio **tokens** (4 métodos)
é o piloto da Fase 2.

| Arquivo / domínio | N | Métodos (resumo) |
|-------------------|---|------------------|
| tasklist | 37 | CRUD tarefas/listas/workflow/notas |
| mcp | 20 | servers, tools, resources, OAuth, logs |
| jobs | 17 | jobs, runs, catalog, dry-run |
| speech | 17 | TTS/STT, providers, synthesize |
| workspace | 16 | tabs, workspaces, import/export |
| messaging | 15 | canais, contatos, assign |
| llm_providers | 12 | CRUD providers, test, models |
| profiles | 12 | CRUD profiles, active, context providers |
| auth | 9 | login, vault, session |
| credentials | 8 | vault credentials, external sources |
| memory | 9 | CRUD memory + policy |
| settings | 9 | test connection, clear*, native TTS |
| skills | 8 | CRUD skills |
| terminal | 8 | sessions, run, interrupt |
| allowlists | 7 | CRUD allowlists + questionnaire |
| acp_install | 7 | install/update/remove agents |
| signal | 7 | Signal link/register |
| database | 6 | reset, maintenance, stats |
| tasklist_actions | 5 | custom actions |
| **tokens (piloto)** | **4** | **stats conversa/turno, threshold** |
| updater | 4 | version, check, apply |
| app (ciclo de vida) | 4 | Context, Startup, ShowWindow, Shutdown |
| tools | 2 | available tools, runtime catalog |
| chat | 2 | SendMessage, RetryMessage |
| nettrust | 2 | network allowlist |
| welcome | 2 | wizard |
| + ACP options/providers/registry/trust/workdir, hotkeys, media, speech_events, channels_legacy_cleanup, acp_commands | 1–2 cada | ver arquivos `app_*.go` |

Spike Fase 1 (fora do `App`): `wailsapi.Probe.StranglerFigProbe`.
