# AEP-0066 — Indicador de status de conexão com a API LLM

Status: Done — escopo principal entregue; follow-ups permanecem separados
Data: 2026-06-01
Autor: Inclunet + Cursor Agent

## Resumo

Este AEP define um **indicador de status de conexão** com o provider/API LLM ativo,
exibido na topbar, alimentado por um **health check periódico** no backend que emite
eventos Wails tipados. O indicador reflete os estados `online`, `offline` e
`checking` (mostrado como "reconectando"), expõe a **latência média** medida e dispara
**anúncios de queda/restauração** via o announcer global + toast existente.

Resolve o núcleo do GitHub issue #38. O item "fallback automático para modelo local"
fica documentado como **follow-up** (ver seção abaixo), por tocar a camada de roteamento
do LLM e poder conflitar com trabalho paralelo nessa área.

## Motivação

Antes desta proposta não havia detecção de status de rede ou da API do LLM: o usuário
só descobria que a API estava offline ou lenta ao tentar enviar uma mensagem. Já existia
infraestrutura de teste de conexão **sob demanda** (`ProbeConnection`/`TestConnection` em
`internal/providers`), mas nada periódico nem refletido na UI.

O AEP-0034 (Unified Workspace) já previa uma "Status Bar futura"; este indicador é um
primeiro passo concreto nessa direção, reaproveitando a infra de providers existente.

## Decisões

### 1) Reaproveitar a sondagem existente (sem duplicar)

- O health check usa `providers.Service.CheckHealth`, um novo método que **reaproveita
  `ProbeConnection`** (mesma rota `GET /models` usada pelo wizard e pela validação de
  providers) e mede a latência de ida e volta.
- `CheckHealth` resolve o perfil ativo (incluindo sentinelas `$default`), recupera a
  credencial persistida do provider e classifica o resultado:
  - `online`: URL acessível **e** autenticação OK.
  - `offline`: URL inacessível **ou** autenticação rejeitada (carrega `error`/`errorType`).

### 2) Monitor periódico desacoplado do Wails

- Novo pacote `internal/connstatus` com um `Monitor` que recebe uma `CheckFunc` e um
  `Emitter` abstrato — **não conhece Wails**, é testável isoladamente.
- Intervalo padrão: **30s** (`connstatus.DefaultInterval`). Intervalo configurável por
  usuário fica como follow-up.
- A primeira verificação roda imediatamente para a UI pintar rápido.
- Quando o último estado era `offline`, o monitor emite um evento `checking`
  (reconectando) antes da próxima sondagem.
- A **latência média** é calculada sobre uma janela das últimas 10 amostras `online`
  (amostras offline não poluem a média).

### 3) Contrato de evento

- Evento Wails: `llm:connection-status` (constante `connstatus.EventName`).
- Payload tipado (`connstatus.Event`): `state`, `providerId`, `providerName`, `model`,
  `latencyMs`, `avgLatencyMs`, `error`, `errorType`, `timestamp`.
- Espelhado no frontend em `frontend/src/types/connection.ts`.

### 4) Ciclo de vida user-scoped

- O monitor é iniciado em `reloadUserScopedRuntime` (pós-login) com um contexto próprio
  cancelável e encerrado em `stopUserScopedRuntime`, no logout (rollback) e no `Shutdown`.
- Não roda antes do login (depende de provider + credenciais user-scoped).

### 5) Acessibilidade (obrigatório)

- O estado é transmitido por **ícone + texto + `aria-label`**, nunca só por cor. A cor é
  reforço visual via tokens semânticos (`--color-success`/`--color-danger`/`--color-info`).
- Anúncios de **queda** (assertivo) e **restauração** (polite) usam o **announcer global**
  (`useAnnouncer`) + toast existente. **Não** é criada nova live region — o indicador não
  é `role="status"`/`aria-live`, respeitando a arbitragem de announcer único (AEP-0058).
- Strings internacionalizadas nos 3 locales (`pt-BR`, `en`, `es`) sob `connectionStatus`.
- Uma única assinatura do evento (`useConnectionStatusListener` montado em `App`) alimenta
  a `connectionStore`, de onde o indicador da topbar lê — evita múltiplos consumidores e
  anúncios duplicados.

## Fases

- **Fase 1 (este PR):** itens 1–4 do issue — indicador visual, health check periódico,
  notificação de queda/volta e latência média visível.
- **Fase 2 (follow-up):** fallback automático para modelo local (ver abaixo) e intervalo
  configurável por usuário/perfil.

## Follow-up: fallback automático para modelo local

O item 5 do issue ("fallback automático para modelo local quando disponível") **não** é
implementado aqui, intencionalmente:

- Ele toca o **caminho de roteamento do LLM** (escolha de provider/modelo em runtime) e
  tende a conflitar com trabalho paralelo nessa camada.
- Exige decisões de produto: critério de disparo (quantas falhas?), reversão automática ao
  provider primário quando voltar, sinalização clara na UI de que se está em fallback, e
  interação com perfis/`$default`.

Proposta de follow-up: um AEP dedicado que defina o gatilho (ex.: N falhas consecutivas no
health check), o provider local alvo e a política de retorno, reutilizando o mesmo evento
`llm:connection-status` (acrescentando um campo de "modo fallback") sem criar fluxo de envio
alternativo (respeitando AEP-0040).

## Riscos

- **Ruído de eventos:** mitigado pelo intervalo de 30s e por só emitir `checking` quando
  havia offline anterior.
- **Latência da sondagem:** `ProbeConnection` usa timeout de 15s; em endpoints lentos o
  estado pode demorar a refletir. Aceitável para feedback de status.
- **Falsos offline em 401:** auth rejeitada conta como offline — é o comportamento desejado
  (o provider não está utilizável), com `errorType` para diagnóstico.

## Critérios de aceitação

- [x] Indicador visual na topbar com estados online/offline/reconectando (ícone+texto+aria).
- [x] Health check periódico do provider ativo via evento Wails.
- [x] Notificação de queda/restauração via announcer global + toast (sem nova live region).
- [x] Latência média visível quando online.
- [x] i18n nos 3 locales; cores por token semântico.
- [x] Testes backend (mudança de estado, evento emitido, média) e frontend (indicador
      reflete estados; anúncio ao mudar).
- [ ] (Follow-up) Fallback automático para modelo local.
- [ ] (Follow-up) Intervalo configurável por usuário.
