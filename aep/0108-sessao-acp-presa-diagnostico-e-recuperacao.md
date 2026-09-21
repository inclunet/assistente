# AEP-0108 — Sessão ACP presa: diagnóstico e recuperação

**Status:** Draft

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
- UI: estado "aguardando turno anterior" por conversa (AEP-0100) com ação Cancelar
  (`session/cancel`), anunciado via `announce()` (AEP-0091, sem só-cor).

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
- `Prompt` mantém as checagens de `cn.isDead()` na entrada/saída; o heartbeat só
  adiciona o ponto de detecção que falta no meio: resposta que nunca chega.

### D3. Recuperação automática (1 retry)

- Em `ErrSessionLost`/`ErrCancelNotConfirmed`, o `Manager` fecha a sessão morta,
  abre nova sessão ACP na mesma conversa (retomando por `loadSession`,
  `manager.go:883`) e retenta o turno **uma vez**. O aviso do que houve trafega
   como evento de chat (AEP-0040, nunca mensagem local no frontend) para o
   `announce()`/TTS arbitrados o apresentarem (AEP-0058).
- Sem empilhamento: vale o pipeline único `SendMessage`/`RetryMessage` (AEP-0040);
  nada de fluxo alternativo de envio.

### D4. Nunca persistir `assistant` vazio em erro

- Falha de turno ACP persiste texto de erro (sanitizado, D11 do AEP-0084) em vez de
  `content` vazio, com `announce()` do motivo. Dois `assistant` de 0 chars no caso
  real apagaram o rastro na UI.

## Fases

1. **Fase 1 — Observabilidade (D1 + D4):** logs de fila, estado na UI, erro
   persistido/anunciado. Sem mudança de comportamento de transporte.
2. **Fase 2 — Fail-fast (D2):** sonda em sessão ociosa + watchdog de inatividade
   em turno em voo, ambos com desfecho `unconfirmed`; teste de processo morto
   sem fechar o pipe e de agente vivo sem resposta.
3. **Fase 3 — Recuperação (D3):** fechar/reabrir + 1 retry com anúncio; teste de
   `ErrSessionLost` e `ErrCancelNotConfirmed`.

## Riscos

- Heartbeat agressivo demais derruba sessão saudável (mitigação: só em ociosidade,
  timeout generoso, sem `session/cancel` real).
- Retry duplicar efeito colateral no agente (mitigação: 1 retry só quando o turno
  **não** foi aceito — `Accepted=false`; turno aceito segue "sem repetição",
  AEP-0064).
- Falso não-confirmado por lentidão do agente (mitigação: watchdog conta
  inatividade de `session/update`, não tempo total; timeout generoso, nunca
  sondar sessão ociosa com `session/cancel` real).

## Critérios de aceitação

- [ ] Turno ACP sem resposta gera log de espera + estado visível com Cancelar.
- [ ] Processo ACP morto sem fechar pipe vira `ErrSessionLost` em tempo limitado
      (sonda ociosa com `SendRequest` falhando); agente vivo sem resposta vira
      `ErrCancelNotConfirmed` via watchdog de inatividade (nunca `markDead`,
      que é por `conn` e derrubaria as demais sessões do processo).
- [ ] `ErrSessionLost`/`ErrCancelNotConfirmed` recupera com nova sessão + 1 retry
      anunciado (quando não aceito).
- [ ] Nenhum caminho de erro ACP persiste `assistant` com conteúdo vazio.
- [ ] Testes Go cobrindo fila, heartbeat e recuperação; `go build/vet/test` verdes.
