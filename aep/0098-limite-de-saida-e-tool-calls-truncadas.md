# AEP-0098 — Limite de saída e tool calls truncadas

**Status:** Done — implementado

## Resumo

Propagar o motivo de término de geração dos providers até o loop agêntico e
tratar `max_tokens`/`max_output_tokens` como um desfecho distinto de sucesso,
erro de transporte e JSON originalmente inválido.

Quando o limite interromper uma resposta com tool calls locais, nenhuma chamada
daquela geração será executada. O loop poderá pedir uma única reformulação em
operações menores, antes de qualquer efeito colateral. Se o limite se repetir,
ou se a geração contiver somente texto parcial, o turno terminará como
`output_limit`, preservará o texto produzido e oferecerá continuação explícita.

Esta AEP implementa a fase 5 da AEP-0096 e complementa as AEPs 0039, 0064 e
0071.

## Motivação

O protocolo atual usa o tipo de callback para inferir o término:
`OnToolCalls` significa continuar o loop e `OnDone` significa sucesso. Os
providers, porém, já recebem motivos mais precisos, como `length`,
`max_tokens`, `MAX_TOKENS`, `max_output_tokens` e `tool_use`, mas descartam essa
informação antes de chegar ao handler.

Com isso, argumentos JSON cortados pelo limite de saída chegam ao executor como
se o modelo tivesse produzido um documento completo. O executor os classifica
corretamente como `invalid_args`, mas tarde demais: o harness não consegue
distinguir um erro de autoria do modelo de uma interrupção imposta pelo budget.
Além de gerar diagnóstico enganoso, um lote parcialmente gerado pode misturar
chamadas completas e incompletas; executar apenas a parte aparentemente válida
criaria efeitos parciais não pretendidos.

Elevar indiscriminadamente `max_tokens` não resolve tarefas de tamanho
arbitrário e apenas desloca o limite. Agentes de código precisam decompor
operações grandes em leituras, patches e escritas menores.

## Decisões

### D1 — Motivo de término normalizado e provider-agnostic

O pacote `internal/llm` define um motivo normalizado de término, no mínimo:

- `stop`: conclusão natural;
- `tool_calls`: o modelo encerrou para solicitar tools;
- `max_tokens`: o limite configurado de saída foi atingido;
- `content_filter`: bloqueio de segurança/conteúdo;
- `cancelled`: cancelamento reportado pelo provider;
- `other`: término conhecido, mas sem categoria operacional específica;
- vazio: provider não informou um motivo.

Cada adapter traduz o vocabulário nativo para esse contrato. O valor bruto pode
ser usado em log e teste do adapter, mas decisões do loop usam somente o motivo
normalizado.

A propagação ocorre por uma capability opcional de handler, invocada antes do
callback terminal existente. Isso preserva compatibilidade com handlers de
integrações e testes que implementam apenas `StreamHandler`, sem voltar a
inferir provider, modelo ou endpoint.

### D2 — Um lote interrompido é atômico antes de efeitos locais

Se o motivo for `max_tokens` e houver uma ou mais tool calls locais:

1. nenhuma chamada do lote é executada;
2. nenhuma `tool_invocation` daquele lote é persistida como execução;
3. o loop descarta os argumentos incompletos;
4. uma única mensagem de controle, não persistida como fala do usuário, pede ao
   modelo que refaça a operação em chamadas menores e JSON completo;
5. a tentativa consome uma iteração normal do loop.

A regra vale mesmo quando alguns argumentos do lote parecem JSON válido. O
motivo de término informa que a geração como um todo não foi concluída; executar
um prefixo criaria commit parcial de intenção.

Não há retry cego: a recuperação só ocorre quando o provider informou
explicitamente o limite, antes de qualquer efeito, e no máximo uma vez por
turno. Um segundo limite encerra o turno.

### D3 — JSON inválido continua sendo erro de argumentos quando não houve limite

Uma tool call terminada normalmente com JSON malformado continua passando pelo
executor e recebendo `ErrorKindInvalidArgs`. O harness não atribui truncamento
ao provider sem evidência no stop reason.

Assim, os dois casos ficam observavelmente distintos:

- término normal + JSON inválido: erro de argumentos produzido pelo modelo;
- `max_tokens` + tool call: geração truncada, bloqueada antes do executor.

### D4 — Texto parcial termina de forma explícita

Quando `max_tokens` ocorrer sem tool calls locais, o conteúdo parcial é
preservado e o evento terminal usa `reason: "output_limit"`. A interface anuncia
que a resposta atingiu o limite e mantém a ação de continuação explícita da
AEP-0064.

O harness não continua texto automaticamente. Prefill, fallback de continuação,
duplicação de conteúdo e cancelamento já têm contrato próprio na AEP-0064; o
limite de saída apenas torna esse estado detectável e acionável.

### D5 — Mapeamento mínimo por transporte

- OpenAI Chat Completions: `length` → `max_tokens`; `tool_calls` e `stop`
  preservam sua semântica.
- OpenAI Responses: evento `response.incomplete` com
  `max_output_tokens` → `max_tokens`; `content_filter` →
  `content_filter`.
- Anthropic: `max_tokens` → `max_tokens`; `tool_use` → `tool_calls`;
  `end_turn` e `stop_sequence` → `stop`.
- Google: `MAX_TOKENS` → `max_tokens`; `STOP` → `stop`; bloqueios →
  `content_filter`; `MALFORMED_FUNCTION_CALL` permanece término `other`, sem
  fingir que houve truncamento.
- ACP: `max_tokens` → `max_tokens`; `end_turn` → `stop`; demais motivos são
  normalizados sem alterar a regra de que pedidos já aceitos pelo agente não são
  repetidos automaticamente.

### D6 — Observabilidade sem persistir payload truncado

Logs do provider e do loop registram motivo normalizado, motivo bruto quando
disponível, iteração e nomes das tools, nunca o argumento potencialmente grande
ou incompleto.

`chat:done.reason` passa a admitir `output_limit`. Esse desfecho não é erro de
transporte e não dispara retry automático de streaming.

## Fases

1. **Contrato**
   - criar o motivo normalizado e a capability de handler;
   - cobrir normalização com testes unitários.
2. **Adapters**
   - propagar o motivo em OpenAI Chat Completions, OpenAI Responses,
     Anthropic, Google e ACP;
   - adicionar regressões para os valores nativos de limite.
3. **Loop agêntico**
   - bloquear atomicamente lotes truncados;
   - permitir uma reformulação orientada a fatiamento;
   - encerrar repetições como `output_limit`.
4. **Desfecho e interface**
   - propagar `output_limit` em `chat:done`;
   - anunciar o estado e manter disponível a continuação explícita.
5. **Validação**
   - executar suites backend e frontend;
   - validar que JSON inválido sem stop reason continua no caminho
     `invalid_args`.

## Riscos

- **Provider omite o motivo:** o comportamento legado permanece; não se inventa
  truncamento por heurística de tamanho ou JSON.
- **Tool call aparentemente completa em lote truncado:** será descartada para
  preservar atomicidade. O custo é repetir geração, sem repetir efeitos.
- **Modelo ignora a orientação de fatiamento:** apenas uma reformulação é
  permitida; depois o turno termina de forma explícita.
- **Texto parcial duplicado na continuação:** continua sob as regras e limites da
  AEP-0064, não por retry automático desta AEP.
- **Vocabulários futuros de provider:** mapeiam para `other` até receberem
  decisão explícita; não usam heurística por URL, marca ou modelo.

## Critérios de aceitação

- [x] O motivo de término chega dos cinco transports ao handler.
- [x] `max_tokens` é normalizado sem heurística por provider, URL ou modelo.
- [x] Nenhuma tool local de um lote interrompido é executada ou persistida.
- [x] Há no máximo uma reformulação automática, orientada a operações menores.
- [x] Um segundo limite encerra o turno como `output_limit`.
- [x] Texto parcial é preservado e a continuação explícita permanece disponível.
- [x] JSON malformado sem `max_tokens` continua classificado como `invalid_args`.
- [x] Frontend anuncia `output_limit` sem tratá-lo como erro de transporte.
- [x] Testes Go e Vitest cobrem os caminhos críticos.
