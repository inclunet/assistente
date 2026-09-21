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
  ou seja, há caminho (prompt pendurado, conexão morta sem `cn.dead` fechado) que
  não chega a esse `ErrCancelNotConfirmed` visível.
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

- Sonda periódica em sessão ACP ociosa (capacidades/handshake leve) com timeout
  curto; ao estourar, fecha `cn.dead` para `waitForTurn` acordar com
  `ErrSessionLost` em vez de esperar para sempre.
- `Prompt` mantém as checagens de `cn.isDead()` na entrada/saída; o heartbeat só
  adiciona o ponto de detecção que falta no meio.

### D3. Recuperação automática (1 retry)

- Em `ErrSessionLost`/`ErrCancelNotConfirmed`, o `Manager` fecha a sessão morta,
  abre nova sessão ACP na mesma conversa (retomando por `loadSession`,
  `manager.go`) e retenta o turno **uma vez**, anunciando o que houve.
- Sem empilhamento: vale o pipeline único `SendMessage`/`RetryMessage` (AEP-0040);
  nada de fluxo alternativo de envio.

### D4. Nunca persistir `assistant` vazio em erro

- Falha de turno ACP persiste texto de erro (sanitizado, D11 do AEP-0084) em vez de
  `content` vazio, com `announce()` do motivo. Dois `assistant` de 0 chars no caso
  real apagaram o rastro na UI.

## Fases

1. **Fase 1 — Observabilidade (D1 + D4):** logs de fila, estado na UI, erro
   persistido/anunciado. Sem mudança de comportamento de transporte.
2. **Fase 2 — Fail-fast (D2):** heartbeat + fechamento de `cn.dead`, com teste de
   processo morto sem fechar o pipe.
3. **Fase 3 — Recuperação (D3):** fechar/reabrir + 1 retry com anúncio; teste de
   `ErrSessionLost` e `ErrCancelNotConfirmed`.

## Riscos

- Heartbeat agressivo demais derruba sessão saudável (mitigação: só em ociosidade,
  timeout generoso, sem `session/cancel` real).
- Retry duplicar efeito colateral no agente (mitigação: 1 retry só quando o turno
  **não** foi aceito — `Accepted=false`; turno aceito segue "sem repetição",
  AEP-0064).
- Falso `ErrSessionLost` por lentidão do agente (mitigação: distinguir timeout de
  sonda de turno em voo; nunca sondar com turno ativo).

## Critérios de aceitação

- [ ] Turno ACP sem resposta gera log de espera + estado visível com Cancelar.
- [ ] Processo ACP morto sem fechar pipe vira `ErrSessionLost` em tempo limitado
      (teste com fakeagent/processo).
- [ ] `ErrSessionLost`/`ErrCancelNotConfirmed` recupera com nova sessão + 1 retry
      anunciado (quando não aceito).
- [ ] Nenhum caminho de erro ACP persiste `assistant` com conteúdo vazio.
- [ ] Testes Go cobrindo fila, heartbeat e recuperação; `go build/vet/test` verdes.
