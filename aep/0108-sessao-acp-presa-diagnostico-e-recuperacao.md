# AEP-0108 — Sessão ACP presa: diagnóstico e recuperação

**Status:** In Progress — Fases 1–3 implementadas; pendentes estado na UI com
Cancelar (D1, parte visual) e sonda em sessão ociosa (D2, cobertura a).

## Resumo

Turno ACP que falha sem confirmar (processo morto, `SendRequest` sem resposta,
cancelamento não confirmado) prende o `turnSlot` da sessão (`internal/acp/session.go`,
serialização do AEP-0084 D10) e os turnos seguintes enfileiram em silêncio: no log
há `SendMessage` sem `session/prompt` depois, e no banco sobra `assistant` vazio.
Caso real: conversa `01a0c1ce` (review do PR 811), turnos `01a0c470` (11:48) e
`01a0c48f` (12:21) com 0 chars. Este AEP define observabilidade, fail-fast,
recuperação e proibição de `assistant` vazio em erro.

## Motivação

- AEP-0084 D10 manda recusar o turno seguinte com o motivo quando o cancelamento
  não é confirmado — mas o observado foi espera silenciosa + `assistant` vazio,
  ou seja, há caminho (prompt pendurado com conexão viva, sem confirmação nem
  morte do processo) que não chega a esse `ErrCancelNotConfirmed` visível.
- Sessão zumbi sobrevive no banco (`acp_sessions`) após a morte do processo
  (`exit 0x40010004` no caso real); reabrir o app reconecta, mas um turno preso
  volta a travar a conversa sem que o usuário (e o NVDA) saiba por quê.
- O gatilho original (relógio atrasado → `certificate is not yet valid`) está
  resolvido via NTP; o defeito estrutural permanece.

## Decisões

### D1. Fila visível

- `waitForTurn` loga quando começa a esperar (sessão, há quanto tempo o slot está
  tomado via `turnInFlight`) e quando sai da fila (admitido, `ErrCancelNotConfirmed`,
  `ErrSessionLost`, `ErrSessionClosed`, cancelamento do contexto).
- UI: estado "aguardando turno anterior" por conversa, com ação Cancelar
  (`session/cancel`), anunciado via `useAnnouncer` (nunca só-cor; announcer único,
  AEP-0058).

### D2. Heartbeat com fail-fast

- Processo morto com pipe fechado já é detectado hoje: `watch()` fecha `dead`
  quando `rpc.Done()` fecha (`client.go`), e `waitForTurn` escuta os dois. O
  buraco real é o agente **vivo mas sem resposta** (não responde o
  `session/prompt`, não morre, pipe aberto) — foi esse o sintoma às 11:48/12:21.
- Por isso a sonda periódica não fecha `cn.dead` ao estourar: `dead` significa
  "processo caiu" e o `conn` é compartilhado por sessão do mesmo processo —
  marcá-lo mentiria o diagnóstico e derrubaria conversas saudáveis. O estouro
  segue o caminho já previsto no D10 do AEP-0084: marca o turno como não
  confirmado (`unconfirmed`, `ErrCancelNotConfirmed`) para o próximo turno ser
  **recusado com o motivo**, nunca enfileirado em silêncio.
- Duas coberturas: (a) sessão **ociosa** — sonda leve; se o `SendRequest` da sonda
  falhar com pipe quebrado, aí sim o processo caiu de verdade e `markDead` é
  legítimo (`ErrSessionLost`); (b) turno **em voo sem atividade** (nenhum
  `session/update` por X) — watchdog de inatividade com o mesmo desfecho
  `unconfirmed`. O caso das 11:48 era o (b): slot ocupado por prompt pendurado,
  então só sonda ociosa não o pegaria.
- Implementado (b): `watchStall` com `stallTimeout` 10min / `stallPoll` 1min
  (campos, como `grace`), carimbo em `deliver`/`requestPermission`/`startTurn`,
  desfecho pelo caminho de abandono do `Prompt` (`abandonTurn`). A cobertura (a)
  ficou **adiada**: não há método leve universal no ACP para sondar (re-handshake
  é pesado e específico por agente), e um `SendRequest` de sonda pode ele mesmo
  pendurar no `Write` contra agente que parou de ler; morte de processo com pipe
  fechado segue detectada por `watch()`/`rpc.Done()`.
- `Prompt` mantém as checagens de `cn.isDead()` na entrada/saída; o heartbeat só
  adiciona o ponto de detecção que falta no meio: resposta que nunca chega.

### D3. Recuperação automática

- Em `ErrSessionLost`, o provider invalida a conversa (`Conversation.Invalidate`)
  para a tentativa seguinte retomar pelo identificador guardado ou abrir outra
  (`manager.go:883`, já testado em `TestDepoisDeInvalidarOProximoTurnoTentaRetomarAMesmaSessao`).
  A retentativa em si é a auto-recuperação existente do loop simples
  (`streamingRecoveryEnabled`, AEP-0064, default 3 tentativas), que reinvoca
  `StreamChat` quando o erro não foi marcado `NotRetryable` — e `ErrSessionLost`
  não aceito não é. Repetir é seguro aqui justamente porque o pedido nunca
  chegou ao agente (`Accepted=false`).
- O aviso trafega como `chat:notice` de kind novo (`agent_session_recovered`,
  com chave nos 3 locales) quando o pedido nem chegou ao agente; com aceite, só
  o invalida, e a mensagem de erro já orienta a conferir o estado.
- Sem empilhamento: vale o pipeline único `SendMessage`/`RetryMessage` (AEP-0040);
  nada de fluxo alternativo de envio.

### D4. Nunca persistir `assistant` vazio em erro

- Falha de turno ACP persiste texto de erro (sanitizado, D11 do AEP-0084) em vez de
  `content` vazio, com `announce()` do motivo (AEP-0058). O caminho de parcial
  (`persistAssistantPartialBestEffort`) já existe; a regra cobre o caso de
  zero conteúdo — os dois `assistant` de 0 chars do caso real apagaram o rastro
  na UI.

## Fases

1. **Fase 1 — Observabilidade (D1 + D4):** logs de fila (`acquireTurn`), erro
   persistido no placeholder vazio (`persistErrorWhenEmpty` + `OnDone`).
   ✅ Implementada (testes `TestOnDone*`, `TestPersistErrorWhenEmpty*`).
   Pendente a parte visual do D1 (estado na UI com Cancelar).
2. **Fase 2 — Fail-fast (D2, cobertura b):** watchdog de inatividade em turno em
   voo (`watchStall`, `stallTimeout` 10min/`stallPoll` 1min, desfecho
   `unconfirmed`). ✅ Implementada (testes `TestWatchStall*`,
   `TestDeliverCarimbaAtividade`). Cobertura (a), sonda ociosa, adiada (ver D2).
3. **Fase 3 — Recuperação (D3):** `Invalidate` em `ErrSessionLost` + aviso
   `agent_session_recovered` (3 locales + vitest). ✅ Implementada (teste
   `TestSessaoPerdidaInvalidaEAvisaSessaoNova`; retomada já coberta por
   `TestDepoisDeInvalidarOProximoTurnoTentaRetomarAMesmaSessao`).

## Riscos

- Watchdog lento demais ou agressivo demais (mitigação: conta inatividade de
  updates, não tempo total; 10min default; campos ajustáveis em teste).
- Retry duplicar efeito colateral no agente (mitigação: só repete o não aceito
  — `Accepted=false`; turno aceito não se repete porque pode ter editado
  arquivo/rodado comando, regra de `internal/agent/service.go`
  (`ErrorNotRetryable`, AEP-0084 D4)).
- Falso não-confirmado por lentidão do agente (mitigação: watchdog conta
  inatividade de `session/update`, não tempo total; timeout generoso).

## Critérios de aceitação

- [x] Turno ACP sem resposta gera log de espera (estado visível com Cancelar
      pendente — parte visual do D1).
- [x] Agente vivo sem resposta vira `ErrCancelNotConfirmed` via watchdog de
      inatividade (nunca `markDead`, que é por `conn`).
- [ ] Processo ACP morto sem fechar pipe com sessão ociosa (sonda da cobertura
      a — adiada, ver D2).
- [x] `ErrSessionLost` não aceito invalida a sessão e avisa (`agent_session_recovered`);
      a tentativa seguinte retoma/abre nova.
- [x] Nenhum caminho de erro do loop simples persiste `assistant` vazio
      (`persistErrorWhenEmpty` + `OnDone`).
- [x] Testes Go cobrindo fila, watchdog e recuperação; `go build/vet/test` verdes.
