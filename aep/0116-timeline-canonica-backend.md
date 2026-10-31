# AEP-0116 — Timeline canônica de chat do backend à apresentação

## Status: In Progress — fase 1 concluída; fase 2 com escritores, seleção e portabilidade por execução; composição e cutover pendentes

Data: 2026-10-30

### Decisão: retry substitui a execução apresentada (2026-10-31)

Decisão explícita do mantenedor: repetir uma resposta substitui no visual toda a
execução anterior, inclusive intermediárias e ferramentas, preservando-a no
backend. Não acumular automaticamente as execuções no mesmo item. A identidade
`turn:<questionId>` e a posição permanecem estáveis. Tentativa da ferramenta é
um conceito diferente de execução da resposta.

A inspeção reproduziu uma mistura real: depois de concluir B, a projeção continha
uma intermediária de A e o final de B. Além disso, o final de A era sobrescrito.
Somente adicionar um ID não preservaria esse conteúdo. A implementação deve
criar um representante por execução e selecionar explicitamente a execução
exibida, sem apagar a anterior nem inferir sua identidade.

A v44 prepara seis vínculos nullable, sem atribuição retrospectiva:

- `chat_messages.chat_execution_id` e `tool_invocations.chat_execution_id`:
  proveniência única; legado/import continua NULL quando não há evidência.
- `chat_turns.current_execution_id`: seleção persistente após o terminal,
  distinta de `active_execution_id`, que representa trabalho em curso.
- `chat_turn_attempts.response_message_id`: representante daquela execução.
  O ponteiro do turno é mantido com ele na mesma transação, não outra seleção.
- `base_execution_id` e `base_response_message_id` na tentativa: continuação
  declara de onde herda intermediárias/tools e qual texto originou seu prefixo.
  Base legada pode ter mensagem conhecida e execução desconhecida.

A base é capturada antes de começar a continuação. Novos vínculos são validados
por turno/conversa/escopo e não podem ser reatribuídos. O prefixo é preservado
como conteúdo da nova resposta; edição posterior da base não o reescreve.
`base_response_message_id` só pode ser limpo pela exclusão real da mensagem-base;
NULL passa a significar indisponibilidade, sem apagar a execução-base, o modo
de continuação ou o prefixo já copiado. Essa forma também é restaurável no
import. Uma base indisponível não autoriza escolher outra por heurística.
Novas continuações exigem base disponível; restauração histórica pode declarar
a indisponibilidade sem inventar vínculos.
A referência à execução impede ciclos: aponta a execução anterior concluída e
não pode ser modificada. Anexos continuam referenciados pela base; não copiar
blobs para preservar uma continuação textual.

Ativação em recortes após #977 (portabilidade separada para revisão antes dos writers):

1. Schema aditivo e invariantes (este recorte). Não ativa novos writers.
2. Seleção compartilhada antes de limites/paginação: janela, terminal, contexto
   do LLM e hidratação. Auditoria, exportação e métricas preservam todas as execuções.
3. Portabilidade das execuções e seleção, com validação e preservação dos IDs estáveis (AEP-0047).
   Importação não pode tornar conteúdo substituído novamente atual.
4. Ativação atômica no lifecycle e em todos os produtores, com representante novo
   e vínculos explícitos para mensagens e ledger. Legado continua reconhecível.
5. Composição live/persistida na recuperação e janela por esses vínculos; retenção
   volátil anterior continua separada. Lotes, inscrições e cutover visual vêm depois.

Evidências de schema: `TestExecutionContentMigrationKeepsLegacyUnattributed`,
`TestExecutionContentSchemaRejectsCrossScopeAndReassignment`,
`TestExecutionContentSchemaMarksDeletedBaseUnavailable` e
`TestExecutionContentMigrationRollsBackOnFailure`. A migração não altera texto,
usage, IDs nem seleção de dados existentes e não inventa execução no upgrade.

### Recorte: ativação dos escritores por execução (2026-10-31)

A admissão atribui current_execution_id e active_execution_id à mesma tentativa,
limpa o representante do turno e captura a base explícita de continuação. A
criação da resposta gera outro registro persistido, vincula-o à tentativa e ao
turno na mesma transação; repetir a chamada de criação é idempotente. Retry não
sobrescreve a resposta anterior. O item lógico turn:<questionId> permanece igual;
a troca dos consumidores frontend para essa identidade continua pendente.

Continuação copia o prefixo textual/raciocínio uma única vez. Mídia é resolvida
pela cadeia de referências na projeção e no contexto do modelo, sem duplicar
blobs em chat_messages. Áudio gerado para o texto anterior não é atribuído ao
novo texto. Uma base excluída não é substituída por outra inferida.

Mensagens intermediárias e todos os produtores de ledger do agente (local,
nativo e atividade externa) recebem a proveniência da execução admitida. Os
writers de inserção são compartilhados com importação; modelos de leitura não
reatribuem proveniência em Save/Update. Dados legados continuam sem atribuição.
Tentativas de ferramenta e tagging de assistant_message_id são isolados por
execução; concluir uma ferramenta já registrada pode atualizar o resultado
anterior, mas não admite outra chamada daquela execução depois do terminal.

Retry/continue invalidam o resumo da conversa na admissão, inclusive quando seu
boundary está em outro turno. O commit assíncrono compara o resumo anterior e
confirma que suas mensagens-fonte continuam selecionadas, sob o writer lock.
Resumo obsoleto é descartado sem anúncio falso de conclusão; não há contador
paralelo nem coluna nova para resolver essa corrida.

Evidências: `TestExecutionWritersPreserveRetryHistoryAndRejectStaleSummary`,
`TestInvocationWriterScopesAttemptsAndAllowsLateResult`,
`TestExecutionWriterNativeCallsRetryAndReferencedMedia`,
`TestExecutionWriterContinuationPreservesReasoningThroughRuntime`,
`TestExecutionWriterContinuationPrefixOccursOnceAcrossToolIterations`,
`TestTurnExecutionAtomicResponseAndLateWrites` e
`TestTurnRuntimeRecoveryKeepsOneAttemptAndResponse`. Testes de identidade foram
ajustados ao contrato aprovado: exigem registro novo e preservação do anterior,
sem retirar rollback, terminais atrasados, usage e identidade do item lógico.

O prefixo de continuação deixa de ser reaplicado depois que uma intermediária
foi confirmada. Recuperação sem confirmação mantém o prefixo; callbacks de
thinking compõem o raciocínio novo com a base, sem apagá-la no terminal.

Validação local: database 361, chat 274, agent 136, ledger 96, portabilidade 166,
sumarização 16 e App 29 testes selecionados passaram, sem execução ACP/NVDA.
Build/vet e lint incremental passaram. A árvore frontend permanece idêntica à
validada nesta pilha (484 arquivos/5.887 testes, TypeScript, ESLint e Stylelint).
Benchmark de janela: medianas 4,109/10,366/14,051 ms e
805.380/1.752.802/1.775.588 B/op para 100/500/1.000 mensagens, cinco amostras de
20 operações; payload 37.262/88.881/88.882 bytes. A última medição foi isolada
por tamanho: duas execuções agrupadas exibiram dispersão de 24–29 ms no caso
1.000, enquanto suas duas primeiras amostras ficaram em 13–14 ms. Esse ruído
fica registrado; não é evidência de ganho. A medição isolada atende ao limite
histórico de +20%, e a fixture continua sendo de histórico legado.

A composição entre segmentos voláteis e persistidos, a prova de ordem final de
origens mistas, o protocolo de lotes/inscrições e o frontend ainda são pendentes.
Esta etapa não declara resolvida a perda de foco nem encerra a fase 2.

### Recorte: portabilidade da seleção e do histórico de execuções (2026-10-31)

O envelope v3 preserva turnos, seleção atual, execuções, representantes e bases
explícitas, junto de todas as mensagens e ferramentas. IDs estáveis seguem o
AEP-0047; colisões não são remapeadas silenciosamente. Leitura do conteúdo e dos
metadados ocorre no mesmo snapshot transacional. Arquivos v1/v2 continuam aceitos
sem proveniência inventada; campos de execução exigem v3.

A importação valida o grafo e restaura bases em ordem topológica, dentro da mesma
transação da conversa. Usa as invariantes produtivas de admissão para inserir a
proveniência imutável; nenhuma tarefa, evento ou lease é criado. Tentativas que
estavam ativas são restauradas como interrupted/restart; seleção continua
persistida, active_execution_id fica NULL. Falhas revertem inclusive overwrite.
O export rico mantém o histórico completo, associando ferramentas à sua execução;
o JSON canônico conserva os metadados necessários para a seleção na reimportação.

Evidências: `TestExecutionHistoryRoundTripPreservesSelectionAndAudit`,
`TestExecutionHistoryContinuationAndActiveImport`,
`TestExecutionHistoryRejectsInvalidGraphAndRollsBack`,
`TestExecutionHistoryLegacyContinuationPreservesUnknownProvenance`,
`TestExecutionHistoryCollisionDoesNotOverwriteOtherConversation`,
`TestExecutionHistoryRichExportKeepsToolsInTheirExecution` e
`TestExecutionHistoryRequiresVersionThree`, `TestExecutionHistoryPreservesOpaqueIDs`
e `TestImportConversationsPreservesOpaqueRestoredIDsBeforeHook`.
Ainda não ativa produtores; lifecycle e invalidação de sumários são o próximo
recorte. A composição live/persistida e o cutover visual permanecem pendentes.

### Recorte: seleção por execução antes dos limites (2026-10-31)

A seleção compartilhada usa `current_execution_id` e a cadeia-base explícita.
Retry exclui da apresentação intermediárias e ferramentas anteriores; continuação
herda somente bases declaradas e não repete seus finais já copiados no prefixo.
Sem seleção explícita, o legado continua sem atribuição inventada. A filtragem
ocorre antes de limites, contagens, paginação e janela de contexto do modelo.
Janela e terminal usam a mesma leitura; exportação/auditoria não filtram execuções.

O ledger distingue `(executionID, callID)` e agrupa `(executionID, iteration)`;
a profundidade da cadeia ordena bases antes da execução selecionada. Identidades
opacas não são concatenadas para deduplicação. Hidratação preserva `LedgerID`
até a sumarização; registros novos sem vínculo de mensagem não recebem associação
por heurística de iteração. `get_messages` por ID de resposta preservada consulta
aquela execução, enquanto o ID da pergunta expande a seleção atual. Ambos usam
a mesma seleção de cadeia, com autorização por usuário.

Evidências: `TestExecutionSelectionRetryAndContinuationBeforeLimits`,
`TestExecutionSelectionLegacyBaseAndUnavailableBase`,
`TestExecutionSelectionKeepsCallsDistinctAcrossContinuations`,
`TestGetMessagesHistoricalExecutionDoesNotHydrateCurrentTools`,
`TestSummarizationExecutionIdentitySurvivesRepeatedCallIDs` e
`TestTimelineExecutionSelectionMatchesWindowAndTerminal`.
As fixtures de leitores aplicam as migrações produtivas 41/44; não mantêm DDL
paralelo nem adicionam fallback de schema ausente ao código de produção.

Desempenho: a seleção do ledger é relacionada uma vez por `JOIN`; a janela
sem âncora calcula total e página na mesma consulta (`COUNT(*) OVER ()`). Não há
cache de conteúdo nem modo alternativo por heurística. Medianas finais de cinco
amostras de 20 operações: 100 mensagens 4,134 ms / 805.094 B; 500 mensagens
10,150 ms / 1.755.961 B; 1.000 mensagens 13,443 ms / 1.796.902 B. A confirmação
isolada de 1.000 variou de 13,208 a 13,636 ms. Tempo e B/op ficam no orçamento
original de +20%; payload permanece 37.262 / 88.881 / 88.882 bytes, sem aumento
neste recorte. A execução agregada teve dispersão em 1.000 (13,385–30,596 ms);
a comparação do pai no mesmo ambiente também apresentou dispersão tardia.
Registrar ambas, sem tratar a melhor amostra como resultado. A implementação
inicial excedia o orçamento em 100; os números acima são após reduzir as consultas.

Este recorte ainda **não ativa escritores com proveniência**. Antes da ativação,
a invalidação de resumos deve abranger qualquer resposta substituída, mesmo quando
o boundary do resumo pertence a um turno posterior. Lifecycle, export/import,
associação explícita de todos os produtores e composição live/persistida ainda
são pendências. A ordem final de segmentos em continuações com origens mistas
precisa de prova integrada no recorte de composição; a projeção SQL de âncora
por si só não comprova essa ordem. Lotes, inscrições e cutover visual continuam
posteriores. Não reportar a fase 2 como concluída.

### Recorte: itens compartilhados e proveniência do streaming (2026-10-31)

`TimelineItem` amplia a projeção de resposta existente para mensagens independentes.
A janela produtiva materializa itens na ordem SQL e seu adaptador legado passa a
consumi-los diretamente: não reagrupar mensagens, reordenar nem selecionar outro
representante nesse adaptador. Leitura de turno e janela compartilham segmentos,
IDs de mensagens, escopo e chave de posição com timestamp SQLite exato.
A chave interna não aloca codificação opaca que o adaptador antigo não usa;
a codificação pertence à futura borda wire. O
representante nullable continua explícito; a compatibilidade visual fica na borda.
`CommitContent` exige o ID retornado pelo writer. O snapshot de cada stream informa
esse vínculo e se seus bytes continuam confirmados; deltas posteriores retiram a
confirmação, e uma nova geração de transporte não herda o vínculo por texto igual.

Evidências: `TestTimelineItemsShareWindowAndTerminalProjection`,
`TestTimelineItemsPreserveNullRepresentativeAtLegacyBoundary`,
`TestTimelineItemPositionPreservesSQLitePrecision`,
`TestTimelineItemsRejectMissingMaterialization` e
`TestTimelineContentAcknowledgementKeepsPersistedIdentity`.
Este recorte ainda não publica DTO wire, não inscreve janelas e não troca o
consumidor frontend. A composição live/persistida, resposta sem mensagem na
paginação, lotes, recuperação incremental e migração visual continuam pendentes.
Não renomeia a revisão interna ou o contador do storage para sequence wire.

Medição deste recorte: a primeira implementação excedeu o orçamento de
alocações; foi corrigida antes do push, retirando a codificação antecipada e a
cópia de ordenação para turno com uma mensagem. A rodada final 100/500/1.000
teve medianas 3,751/9,932/17,408 ms; a última fixture variou de 13,549 a
23,045 ms. Repetição isolada de cinco amostras de 20 operações confirmou
13,508 ms para 1.000 (13,164–14,061). Registrar a dispersão, sem escolher a
melhor amostra. As medianas de alocação foram 770.479/1.693.573/1.693.625 B/op
(último valor da repetição isolada), abaixo do limite histórico de +20%; o
payload legado permanece 37.262/88.881/88.882 bytes. A medição isolada final
fica dentro do orçamento de tempo, sem alegação de melhoria de UI.


### Recorte: barreira da janela produtiva (2026-10-31)

O controller de conversas recebe `Service.ReadTimelineWindow` no wiring do App.
Sem leitor explícito, a operação retorna erro; não existe fallback de consulta
direta fora da barreira. As fixtures de janela também usam o coordenador.
A abertura/paginação passa pela mesma barreira por usuário/conversa usada por
admissão, deltas, commits e recuperação do turno. A consulta continua transacional
e autorizada pelo leitor compartilhado. Falha não retorna memória nem altera a
execução; fatos de turnos presentes na página desvinculam uma execução que o banco
já não reconhece. Ausência em outra página não equivale a exclusão. Os bytes
recebidos permanecem recuperáveis, sem aceitar novos callbacks do vínculo antigo.

`TestWindowRecoverySerializesDeltasAndPreservesOffPageExecution`,
`TestWindowRecoveryFailureDoesNotChangeExecution` e
`TestWindowRecoveryUsesAuthorizedTransactionalReader` cobrem esses contratos.
`TurnIDs` é metadado interno da leitura, não campo novo no payload da janela.
A leitura de turno também deixa de montar `Nodes`: após o adaptador terminal
consumir `Response`, essa árvore era calculada e descartada em produção. As
provas de snapshot, cronologia, escopo e paridade agora verificam o patch ou a
projeção consumidos em produção, sem manter trabalho runtime só para fixtures.
Não há SQL por delta, nova tabela nem reconstrução de mensagens no frontend.

Medição local sequencial, cinco amostras de 20 operações na fixture
`BenchmarkConversationMessageWindowBaseline`, resultado de tool 1 KiB:
100/500/1.000 mensagens. Pai: 3,834/9,919/13,531 ms e
837.098/1.850.400/1.929.640 B/op. Coordenado: 3,804/10,015/13,539 ms e
830.000/1.868.543/1.892.318 B/op. Payload igual nos dois:
37.262/88.881/88.882 bytes (exceção já documentada da fase 2).
O incremento de tempo contra o pai fica abaixo de 1%; tempo e B/op coordenados
ficam dentro de +20% da baseline histórica. Uma amostra preliminar sob condições
diferentes deu 4,840 ms para 100 mensagens; a comparação sequencial posterior
substitui essa amostra, não é evidência de ganho por remoção de trabalho.

Validação: build, vet, lint incremental e testes direcionados de chat, agent,
controller e janelas App passaram, sem executar ACP. Frontend idêntico ao #974:
reutilizada validação da mesma árvore (484 arquivos/5.887 testes, TypeScript,
ESLint e Stylelint; 31 arquivos ACP excluídos). Não houve mudança de bindings.

Este recorte coordena a leitura persistida existente; não entrega ainda janela
wire com overlay live, inscrição, cursor opaco ou replay. Esses itens, a emissão
por lotes e o cutover frontend seguem pendentes; fase 2 permanece In Progress.

### Recorte: segmentos de recuperação (2026-10-31)

`LiveStreamSnapshot` usa `ResponseSegment`, removendo `LiveFragment` e o formato
paralelo de pares texto/raciocínio. Snapshots omitem segmentos vazios e conservam
bytes, cortes e ordem das iterações. IDs incluem execução, iteração, geração de
transporte, corte e tipo: append preserva identidade, reinício explícito troca
segmentos sem trocar `itemId`. O teste
`TestLiveProjectionTypedSegmentsDoNotReuseTransportIdentity` comprova essas regras.

A confirmação persistida continua substituindo a origem por referências do banco;
não se presume igualdade entre IDs draft e persistidos. O futuro protocolo deve
emitir substituição explícita de segmentos no mesmo item. Este modelo é interno,
não anuncia `revision` como sequência wire nem acrescenta um novo evento paralelo.
Lotes, inscrições e cutover permanecem pendentes; a fase 2 continua parcial.

### Recorte: adaptador terminal da resposta (2026-10-31)

O patch terminal passa a consumir `PersistedResponse.TerminalPatch`, eliminando
seleção de nós e conversão de segmentos no serviço de execução. Janela e terminal
consomem os segmentos da mesma projeção. Um representante explicitamente nulo não
é substituído por uma mensagem intermediária no terminal: o adaptador legado de
janela ainda pode apresentá-la, mas não comprova conclusão nem persistência final.
`TestPersistedResponseTerminalRefusesNullRepresentative` cobre essa separação.

Este recorte não conclui a fase 2: faltam composição live unificada, janelas sob
barreira, lotes/inscrições wire e migração dos consumidores. Não altera o payload
de janela nem acrescenta SQL por delta. Testes ACP permanecem excluídos localmente.

## Resumo

Unificar a representação da conversa entregue pelo backend para abertura,
paginação, streaming e conclusão. O frontend aplica atualizações e apresenta
itens; não reconstrói a identidade, a ordem ou a composição conversacional.
A migração deve reduzir estados inconsistentes possíveis e remover mecanismos
substituídos, sem manter duas arquiteturas de chat em paralelo.

Este AEP propõe a direção e os critérios de entrega. Não altera o contrato em
produção nem autoriza a remoção imediata de fallbacks. A fase 1 fecha o schema
wire e as decisões de persistência antes da implementação dependente.

Relaciona-se às AEPs [0040](0040-backend-driven-messaging.md),
[0057](0057-chat-session-identity.md), [0059](0059-long-conversation-performance.md),
[0064](0064-streaming-recovery-explicito.md) e
[0104](0104-tool-invocations-como-ledger-canonico.md).
Os contratos vigentes continuam valendo até a entrega de cada recorte. Seus
status globais não são alterados por esta proposta.

## Motivação

A investigação identificou responsabilidades repetidas:

- A janela recebe nós consolidados no backend, enquanto o streaming monta
  segmentos, ferramentas e continuação em `chatEventController.ts`.
- `chat:messages_ready` entrega ID e texto, mas a UI atribui horário local.
- `sortTimelineNodes` compara índices quando ambos existem e horários nos
  demais pares. Esse comparador admite relações não transitivas.
- A chave visual pode passar de `message:<id>` para `turn:<turnId>` quando o
  turno chega depois; a recuperação de foco usa posição no array.
- Timeline, sessão padrão e superfícies mantêm representações de mensagens
  que exigem combinação e espelhamento. Flags de paginação também se repetem.
- O terminal pode chegar sem `turnPatch` se sua montagem falhar. A preservação
  local atende a uma falha real e não pode ser removida sem substituição.

Referências: `frontend/src/services/chatSessionRegistry.ts`,
`frontend/src/services/chatEventController.ts`, `frontend/src/store/chatStore.ts`,
`frontend/src/components/chat/MessageList.tsx`, `internal/core/ports/chat_events.go`
e `internal/agent/service.go`.

### Evidências e limites

Experimentos exploratórios em 30/10/2026, sobre o checkout de investigação
baseado em `2da2160a7` (com alterações locais preexistentes em bindings e Go):

- O ordenador produtivo coloca resposta de timestamp 150 antes da pergunta
  com timestamp local 200.
- A(index=0,time=300), B(index=1,time=100), C(sem índice,time=200) produzem
  A<B, B<C e C<A nas comparações par a par.
- No jsdom, `MessageList` real com conteúdo do nó substituído por uma div
  focável perde foco ao trocar a chave, sem `focusout` artificial. Com turno
  definido desde a criação, o mesmo elemento e foco são preservados.

São reproduções de mecanismos, não diagnóstico confirmado de uma sessão real.
Os scripts exploratórios ficaram fora do versionamento; a fase 1 deve converter
as reproduções em fixtures versionadas, executadas sobre uma base limpa.
325 testes frontend focados existentes passaram; testes Go direcionados de
consolidação e janela também. Isso não equivale a cobertura integrada completa.

O benchmark existente `BenchmarkConversationMessageWindowBaseline`, executado
com `-benchtime=1x`, resultado de tool de 1 KiB e 100/500/1000 mensagens, mediu
respectivamente 4,46/8,94/11,41 ms e 33.212/79.161/79.162 bytes de janela.
São amostras exploratórias de SQLite, projeção e serialização, sem orçamento
estatístico, renderização ou extrapolação para mídia pesada.

Correção da investigação: `BuildMessageNodes` preserva a ordem recebida;
`BuildTimelineMessageNodes` não reordena a janela pela mensagem representante.
Não há evidência para reescrever o armazenamento apenas por custo da consulta.

## Decisões

### 1. Item conversacional explícito

O backend fornece identidade estável do item, vínculo com pergunta/turno,
ordenação, estado e segmentos identificados. O ID da mensagem persistida é
uma referência para operações e não determina a identidade visual do turno.
A resposta não pode anteceder sua pergunta. A ordenação deve ser total e
consistente entre páginas, eventos e reload; relógio local não participa dela.

A identidade existe antes do primeiro evento materializável. Eventos de
preparação sem item não criam mensagens. Retry/continuação mantêm as identidades
conversacionais aplicáveis, mas distinguem tentativa/execução para impedir que
terminais antigos afetem a execução atual (AEP-0064).

### 2. Uma projeção para snapshot, deltas e terminal

Backend é responsável por composição de texto, reasoning, referências leves
às invocações e continuação. A UI não detecta texto já representado para montar
cronologia. Janela e terminal usam o mesmo contrato de item; deltas identificam
o item e segmento que atualizam.

Preservar coalescing e aplicação por frame. Não reenviar conteúdo acumulado por
token, reconsultar SQLite a cada delta nem recarregar a conversa inteira após
tools. A projeção incremental deve ter custo proporcional à mudança; o reload
reconstrói a mesma estrutura a partir das fontes persistidas.

Tools permanecem exclusivamente em `tool_invocations`, com detalhes lazy/batch
(AEP-0104). A projeção não cria outro ledger nem copia resultados integrais.

### 3. Precedência e recuperação fazem parte do protocolo

Definir na fase 1 época/revisão, sequência e regras para repetição, lacuna,
snapshot antigo, exclusão e eventos atrasados. Uma consulta iniciada antes do
terminal não pode desfazer a conclusão ao retornar depois. Recriar uma conversa
ou trocar usuário/sessão não reaproveita estado de outra identidade.

`chat_message_revisions` já protege comandos com revisão opaca. Avaliar seu
reuso sem tratá-la como contador ordenável ou expor tokens de autorização como
versões públicas. Revisão de item, revisão da janela e identidade da execução
têm finalidades distintas; o schema deve explicitar a relação entre elas.

Se persistência ou projeção terminal falhar, o backend informa estado e
recuperação explícitos. Preservar conteúdo já entregue, distinguir memória de
durabilidade e não declarar sucesso falso. Retomada após reinício precisa ter
resultado definido; conteúdo não persistido não pode ser prometido como
recuperável. Só remover `preserveReceivedSegments` e caminhos equivalentes
após comprovar o substituto em erro/cancelamento/falha do patch.

### 4. Frontend apresenta e mantém estado de interação

Um índice de itens por conversa, com cada superfície recebendo do backend uma
lista de IDs na ordem canônica, sem reordená-la localmente.
Foco, modo de leitura, expansão, edição, scroll e rascunho permanecem locais à
superfície; não migram ao backend. Dados conversacionais não são espelhados em
árvores independentes por sessão. Política de cache deve limitar retenção sem
expulsar itens em uso; fechar superfícies deve ter efeito verificável na memória.

Conclusão atualiza o mesmo item e preserva seu componente. Revisar também a
mudança de estrutura DOM ao ativar virtualização, elementos internos focados e
conteúdo em leitura. Usar identidade lógica para recuperação inevitável;
conservar saídas intencionais para input/modal/outra superfície. Um item em
leitura/foco deve permanecer materializado enquanto necessário.

### 5. Persistência acompanha o produto, sem remodelagem especulativa

`chat_messages` tem consumidores reais para conteúdo, reasoning, tokens,
modelo, origem, fixação e relações. Não remover campos por aparência de legado.
Separar conceitualmente mensagem, turno/tentativa, anexo e projeção; decidir as
tabelas e constraints na fase 1 com os escritores e importadores inventariados.
Não há decisão antecipada de materializar uma tabela de timeline.

O caminho de áudio recebido pode persistir o mesmo base64 em `media` e `audio`;
a janela busca registros completos antes de omitir áudio do DTO. Evoluir para
conteúdo de anexo único, com papéis explícitos (recebido/gerado), metadados leves
e leitura sob demanda. Preservar TTS, STT, contexto multimodal, exportação e
importação; migrar com verificação de integridade e recuperação documentada.

`parent_id` tem leitores, importação, exclusão de descendentes e API pública;
a ausência de chamada UI não autoriza removê-lo. Inventariar o uso efetivo e
propor eventual depreciação separada. Tokens acumulados e ocupação de contexto
não são intercambiáveis. Ownership continua ancorado na conversa e validado em
toda consulta, evento e cache.

### 6. Migração finita e compatibilidade

Não criar outro SendMessage/RetryMessage. Inventariar Wails, eventos, canais,
ACP, CLI/HTTP quando aplicável, tools de histórico, speech, exportação/importação
e consumidores de contexto. Atualizar schemas e regenerar bindings; payloads
de eventos seguem interfaces próprias, conforme AGENTS.md.

Adaptadores temporários só nas fronteiras, com consumidores e condição de
remoção listados. Nunca executar duas reduções autoritativas concorrentes.
Cada PR declara o que substitui e remove. Atualizar AEPs relacionados quando o
contrato mudar e docs/content no mesmo PR de comportamento visível.

## Fases

### Fase 1 — Contrato e provas versionadas

- [x] Inventariar produtores, consumidores e escritores de cada campo relevante (adendo abaixo).
- [x] Versionar reproduções de ordem/foco e fixtures de paridade entre eventos e reload (seção Provas).
- [x] Fechar DTOs, ordenação/cursor, precedência, tentativas e recuperação de falhas (adendo wire v1).
- [x] Decidir persistência, constraints, upgrade/import e política de cache (adendo de armazenamento).
- [x] Fixar ambiente e medir baseline repetido; definir limites de regressão antes de alterar código (seção Medição).

Saída: adendo deste AEP com decisões fechadas e testes de contrato. Dependências
não resolvidas bloqueiam os recortes correspondentes, não são fallbacks implícitos.

### Fase 2 — Projeção canônica no backend

- [ ] Produzir item estável antes de streaming; projetar segmentos incrementalmente.
- [ ] Unificar janela e terminal e implementar precedência/recuperação acordadas.
- [ ] Cobrir sucesso, tools locais/nativas/ACP, erro, retry, continuação e cancelamento.

Saída: contrato backend testado e adaptadores temporários identificados.

Primeiro recorte da fase 2: identidade aditiva na fronteira atual. `itemId`
passa a acompanhar os nós consolidados de histórico, `chat:messages_ready`
(pergunta), `chat:stream`, `chat:done` e o envelope `turnPatch` (resposta).
Uma única regra backend distingue `message:<id>` de `turn:<userId>`. Trocar a
mensagem representante, repetir ou terminar uma execução não troca o item.
Terminais operacionais anteriores à existência de turno omitem itemId; não
inventam mensagem local nem usam o ID de uma mensagem assistant como fallback.

Adaptadores temporários: `MessageNode`, `TurnPatchEvent` e os eventos atuais
transportam a identidade que será usada por `TimelineItem`. As interfaces de
eventos e bindings refletem o campo, mas a UI ainda não o usa como chave. Retirar
esses adaptadores no cutover da fase 3, após consumidores Wails/CLI/canais
migrarem. Não há nova store, reducer ou fluxo de envio neste recorte.

Evidências: testes de timeline preservam itemId quando muda o representante e
mantêm mensagens legadas independentes; handlers verificam primeiro delta e
retry; terminal compara identidade com janela e preserva-a mesmo se persistência
ou patch falhar. O evento da pergunta só publica sua identidade após commit.
Testes ACP locais permanecem suspensos por solicitação do mantenedor.

Revisão remota deste recorte: os segmentos preservam IDs de invocação e de chamada do provedor; teste com SQLite verifica patch → consulta dos detalhes sem colisão entre IDs distintos. O cache frontend preserva as mesmas referências na consulta e na invalidação, com regressão de entradas independentes. Além disso, o patch seleciona o nó por itemId de resposta,
nunca por posição na lista. Uma pergunta legada com turn_id igual ao próprio ID
continua independente e, sozinha, não produz patch de resposta. IDs não vazios
mantêm seus bytes originais na identidade, agrupamento e hidratação de tools;
`u` e ` u ` não podem colidir. O helper de placeholder também preserva o turno ao criar/reutilizar a resposta; regressão SQLite compara persistência, retry e identidade da projeção. Regressões cobrem ambos os casos.

Limite deste recorte: a preservação de IDs acima vale para identidade aditiva,
placeholder, projeção e consulta/cache de detalhes, não para toda a aplicação.
A escrita/validação do ledger (`toolinvocations/lifecycle.go`, `repository.go`)
ainda normaliza IDs; os testes SQLite provam leitura fiel de registros existentes,
não preservação global ao criar invocações. Associação de tools, lifecycle/cleanup
(`agent/service.go`, `chat/timeline.go`, `database/message_repository.go`),
prefill/finalização de messageId e alvo de contexto ainda possuem normalizações
legadas. `chatEventHub.ts` e `chatSessionRegistry.ts` continuam normalizando turnId
até o cutover. Auditar e tratar essas fronteiras em recorte dedicado das fases 2–3
antes de prometer preservação global de IDs opacos; não é uma migração global neste PR.
Validação local do recorte: build/vet, ports/chat/events e seleção explícita
dos handlers/terminais de agent e das projeções de toolinvocations passaram sem executar ACP. TypeScript, ESLint,
Stylelint (avisos preexistentes) e 6.848 testes Vitest em 515 arquivos passaram.
Bindings regenerados com Wails 2.15.0, a versão do go.mod. Lint Go local 2.15.0
mantém 12 ocorrências fora do diff; análise contra a base 977a950cc tem zero novas.

Benchmark isolado no mesmo ambiente da fase 1, cinco amostras de 20 operações:
medianas de 3,568 / 9,299 / 12,324 ms para 100/500/1.000 mensagens; memória de
710.935 / 1.637.854 / 1.636.750 B/op. Não interpretar variações de tempo como
ganho causado por identidade: não houve otimização da consulta. Payload de
37.262 / 88.881 / 88.882 bytes, acréscimo de 4.050 / 9.720 / 9.720 bytes
(aproximadamente 12,2%) sobre a baseline. Tempo e B/op ficam dentro de +20%.

Exceção de payload limitada a este adaptador de migração: o itemId precisa
conviver com message.id, turnId e originalIndex enquanto a UI antiga consome
esses campos. O aumento é linear nos no máximo 240 itens e não inclui conteúdo
pesado, consultas ou eventos novos. Evitar o campo obrigaria o frontend a
continuar reconstruindo a identidade; remover campos antigos agora quebraria
consumidores. No cutover da fase 3, retirar o envelope legado e medir de novo;
esta exceção não relaxa o orçamento geral de +10% para o DTO final.

Segundo recorte da fase 2 ([issue #962](https://github.com/inclunet/assistente/issues/962)):
leitura de resultados para `get_messages` e sumarização preserva IDs de mensagem,
turno, chamada e invocação, inclusive ao deduplicar tentativas e associar resultados
à mensagem assistant. Espaços só detectam entrada vazia; não alteram identidades.
A hidratação reutiliza a deduplicação de IDs existente antes de separar lotes,
sem novo normalizador, ledger ou fallback. Conteúdo e permissões continuam nos
mesmos caminhos. Não há migração de banco, novo evento ou consulta por token.

Regressões SQLite: `TestHydrationPreservesOpaqueIDsAndRetryIsolation` cobre turnos,
chamadas, retry com timestamps invertidos, repetição entre lotes e isolamento de
usuário; `TestGetMessagesPreservesOpaqueMessageTurnCallAndLedgerIDs` percorre a
ferramenta pública; `TestSummarizationPreservesOpaqueIDsFromLedgerToPrompt` percorre
consulta, vínculo de mensagem, prompt e contagem sem duplicação. Este recorte
fecha a fronteira de leitura da issue #962; normalizações na escrita do ledger,
lifecycle/cleanup e consumidores visuais antigos continuam pendentes conforme
os limites acima. Não conclui a projeção incremental nem a fase 2.
Pendências da fase 2: projeção incremental de segmentos, sequência/época,
cursores, inscrições de janela e recuperação unificada da projeção. As caixas acima permanecem abertas; este recorte não corrige sozinho
ordenação ou foco da UI e não conclui a fase 2.

Terceiro recorte da fase 2: migração v41 (aguarda a conclusão da v19, inclusive
adoção de usuário no upgrade 0.1.9) e repositório transacional de
`chat_turns`/`chat_turn_attempts`, sem duplicar texto, usage ou ferramentas.
O backfill em lotes reutiliza a seleção legada de representante, preserva IDs
opacos e registra `historical_unknown`/`legacy`; não inventa tentativas nem
converte presença de texto ou tokens em evidência de sucesso. Vínculos de
resposta incompatíveis ficam sem representante; mensagens originais são
preservadas. Perguntas órfãs de conversa ficam fora dos novos metadados, com
contagem registrada, sem bloquear a abertura nem apagar o histórico. Backup consistente com manifesto SHA-256 precede a transação de
DDL, constraints e backfill; falha reverte toda a nova estrutura.

O repositório gera executionId no backend, admite uma tentativa ativa por
turno e rejeita terminais de execuções anteriores. Retry/continue preservam
o turno. Finalização pode compartilhar a transação do chamador que grava
resposta/usage. Recuperação marca tentativas interrompidas por reinício,
limitada ao usuário; sua chamada deve anteceder a admissão dos workers.

O terceiro recorte preparou o armazenamento sem ativá-lo. A integração abaixo
reconcilia o intervalo entre a migração e sua ativação antes de usar os vínculos.

Evidências: `chat_turn_repository_test.go` cobre concorrência, terminal
atrasado, escopo por usuário, constraints, rollback junto da resposta,
recuperação, cascata, lotes, idempotência, backup restaurável e rollback de
schema. `TestPublishedRelease*` verifica upgrades das fixtures publicadas e
reabertura. Testes ACP locais permanecem excluídos por orientação do mantenedor.

Quarto recorte da fase 2: lifecycle integrado ao único pipeline de envio.

- [x] Admitir tentativa após confirmar a pergunta e antes de preparar a resposta;
  retry/continue ganham executionId, recovery de transporte preserva a tentativa.
- [x] Criar/reutilizar response_message_id explícito em transação vinculada à tentativa ativa;
  continuação lê esse ID exato, sem selecionar o primeiro assistant nem normalizá-lo.
- [x] Confirmar resposta, reasoning, usage e terminal atomicamente; rejeitar gravações
  parciais e terminais antigos por executionId antes de tocar no conteúdo.
- [x] Encerrar erros de preparação/provider, cancelamento, limites e panic; garantir
  chat:done mesmo se TTS/notificação falhar após um commit confirmado.
- [x] Recuperar tentativas sem lease vivo antes dos workers e reconciliar
  legados sem tentativas; retry/continue também reconciliam esse subconjunto.
- [x] Importar mensagens e metadados juntos, com origem imported e estado
  historical_unknown, sem restaurar execuções ou inferir sucesso pelo texto.

Falha de persistência publica erro sem patch obsoleto. Se o terminal também não
puder ser salvo, o Service retém apenas a chave usuário/conversa/turno e executionId
que ele encerrou localmente. Antes de novo BeginTurn, confirma failed/persistence_error
para essa execução por CAS. Erro mantém a pendência e impede nova admissão; stale
remove a pendência sem alterar execução posterior. Não recupera execução viva por
relógio ou pelo conteúdo. Reinício usa a recuperação persistente já definida.

Evidências: `TestTurnExecution*` verifica atomicidade, writes atrasados, reconciliação
e recuperação no boot; `TestTurnRuntime*` usa SQLite real para recovery de transporte,
retry, cancelamento, falhas de placeholder/gravação/terminal, ID opaco, provider sem
terminal e panic pós-commit; `TestImportConversationCreatesHistoricalTurnAtomically`
prova importação com rollback. Fixtures isoladas de handlers recebem TurnStore
explícito; produção exige o repositório real. Nenhum teste ACP local foi executado.

Revisão remota deste recorte: a reserva desktop não exclui CLI (AEP-0111).
Cada worker adquire lease nativo de executionId antes da admissão no banco e o
mantém até terminar seus writes e liberar a execução. Startup só recupera uma
tentativa cujo lease consiga adquirir, confirmando o terminal por CAS; posse
ocupada preserva a tentativa viva, independentemente da superfície. Não inferir
morte por PID, tempo ou disponibilidade do lock desktop. A migração v42 adiciona somente lease_path à tentativa. Sidecars privados e
estáveis ficam junto do banco; o endereço absoluto é gravado antes de publicar
active_execution_id. Recuperação abre exatamente esse endereço, inclusive com
cache/ambiente divergentes ou banco acessado por hardlink; não recria arquivo
ausente como prova de morte. Controle ilegível preserva a tentativa ativa, registra
diagnóstico e não bloqueia a recuperação das demais nem o startup. A ação de
operação é encerrar todas as interfaces e restaurar cópia consistente da pasta
de dados (banco e controles); não oferecer encerramento forçado sem prova de posse.
Recusas comprovadas antes do INSERT removem o sidecar ainda não admitido; falhas
de armazenamento/COMMIT preservam-no porque o resultado pode ser ambíguo.
Não são conteúdo nem executáveis. Cópia/movimentação
de banco ativo entre máquinas/usuários e remoção externa dos sidecars não são
prova suportada de encerramento. V41/v42 pendentes bloqueiam o
envio antes de gravar a pergunta, mantendo login/adoção disponíveis para concluir
o upgrade no próximo boot. `TestTurnRuntimeStartupPreservesLiveCLIAndRecoversReleasedExecution`,
`TestExecutionLeaseExcludesAnotherOwnerUntilReleased` e
`TestSendMessageRejectsPendingTurnMigrationBeforeWriting` verificam essas fronteiras.
Reconciliação não atualiza representante inalterado; a leitura legada em lotes
continua necessária para detectar escritas de versões anteriores após v41. Não
adicionar um segundo checkpoint derivado sem contrato de invalidação.


Limite: a janela e o patch visual ainda usam o contrato legado. Projeção incremental,
época/sequência, recuperação de conteúdo volatile e cutover do frontend permanecem
pendentes; esta entrega não declara corrigidos foco/ordem da UI nem conclui a fase 2.
Não há consulta por delta nem duplicação de conteúdo em chat_turns.

Quinto recorte da fase 2: a migração v43 (dependente da v42) adiciona
`chat_timeline_versions`, sem copiar mensagens ou ferramentas. Cada conversa
possui uma incarnation aleatória e um contador inteiro de alterações persistidas.
O par muda ao excluir/recriar a conversa mesmo com o mesmo ID. Triggers de
mensagens, ledger de chat, catálogo de ferramentas, turnos, tentativas e conversa participam da mesma
transação dos escritores, incluindo SQL direto, importação e speech. Mudanças
de vínculo invalidam origem e destino; updates sem alteração não incrementam.
Cascatas apenas atualizam registros existentes, sem recriar conversas excluídas.

`ReadChatTimelineVersion` autoriza por usuário e exige que o consumidor passe a
mesma transação de leitura dos dados projetados. Ausência de metadado é erro,
não versão zero. O contador não é sequence de eventos, epoch do projetor nem
revisão de comando; deltas voláteis continuam fora dele. Nenhum writer executa
invalidação adicional, nenhuma consulta por token foi adicionada. Migrações que
acrescentem colunas às fontes devem reinstalar os triggers na mesma migração.

Evidências: `TestChatTimelineVersion*` verifica escrita direta, no-op, rollback,
lotes, autorização, lifecycle, mudança de conversa/vínculo nulo, cascatas,
recriação, reinstalação, snapshot WAL e dependência/atomicidade da migração.
`TestTimelineVersionFollowsLedgerProjection` verifica resumos reais após mudar
catálogo ou invocações com conversation_id nulo/divergente; a invalidação segue
também turn_id, com owner, conforme a leitura existente. Um índice parcial por
catálogo limita a busca das referências de chat. Importação real e rollback são
verificados em `TestImportConversationCreatesHistoricalTurnAtomically`. O consumidor da
barreira, DTO com representante nullable e projeção incremental continuam
pendentes. Não ativar seleção por ponteiro no adapter antigo: uma resposta
excluída pode deixar intermediários com representante NULL; o DTO novo deve
preservar esse estado sem eleger outra resposta. Janela e terminal também devem
ler referências/conteúdo consistentemente e não confundir corte de segmentos
com referência inválida.

Medição do quinto recorte: `BenchmarkChatTimelineInvalidation`, Windows/amd64,
mesmo processador/Go da baseline, SQLite em memória, `-benchtime=100x -count=5`.
Medianas sem/com triggers: mensagem de 64 KiB 0,120/0,228 ms e 1.297/1.386 B/op;
update de invocação 0,011/0,182 ms e 1.063/1.303 B/op; catálogo referenciado por
100 conversas/1.000 invocações 0,009/5,404 ms e 1.047/1.241 B/op. São custos
adicionados de escrita, não ganho de desempenho nem medição ponta a ponta. O
catálogo só invalida quando id/nome/display_name/origem mudam; renovar seu
updated_at não propaga invalidação. `TestChatTimelineVersionCatalogUsesReferenceIndexes`
verifica o plano das buscas. O orçamento de janela/DTO final permanece pendente
para o cutover; este microbenchmark não o substitui.

Validação local do quinto recorte: build/vet, lint sem ocorrências novas,
testes selecionados de database e integração real de importação/ledger;
TypeScript/ESLint/Stylelint sem erros e 5.887 testes Vitest em 484 arquivos.
Por restrição do mantenedor, não executar ACP localmente: 31 arquivos frontend
que mencionam ACP também foram excluídos. Review independente local: três
rodadas; dois achados de cobertura na primeira, corrigidos e verificados nas
seguintes, sem pendências na terceira.

Sexto recorte da fase 2: `DBTimelineReader` reúne a leitura produtiva da janela
e do patch terminal. A primeira consulta autoriza a conversa e lê a versão v43;
mensagens, paginação, metadados nullable de turnos, resumos do ledger e contagem
de filhos da janela vêm da mesma transação SQLite. O leitor do ledger recebe o
handle transacional explicitamente, sem voltar ao banco global nem transformar
schema ausente em histórico vazio. A consolidação visual ocorre após encerrar
a transação, sobre os dados capturados; uma única conversão de Summary substitui
as cópias no controller e no serviço do agente.

`ReadTurn` autoriza a pergunta, conversa e escopo dentro do snapshot. Lê somente
esse turno e remove o corte silencioso de 1.000 mensagens, com a mesma completude
do item carregado pela janela. O terminal mantém WithoutCancel + timeout de 3 s
para concluir após cancelar a geração; a janela continua cancelável. O timeout
não é limite de memória: turnos grandes ainda são materializados integralmente;
a projeção incremental permanece necessária. O serviço do agente recebe o leitor
como dependência obrigatória; fakes existem apenas nos testes isolados de handlers.

O snapshot preserva ausência de registro de turno e response_message_id NULL,
sem reeleger um representante persistido. `MessageWindow`/`TurnPatchEvent` seguem
adaptadores antigos; ainda usam a seleção visual legada até o DTO nullable. Os
outros leitores legados de lista/árvore não são novos consumidores deste snapshot.
A versão fica no backend e não amplia o payload Wails.

Limite explícito: esta é consistência de leitura persistida, não a barreira do
projetor com eventos. Uma consulta posterior a um retry captura o estado atual;
não identifica qual execução motivou a consulta. Epoch, sequence, precedência de
terminais atrasados, recuperação volatile e cutover continuam pendentes. Não
prometer correção de foco/ordenação nem fechamento da fase 2 neste recorte.

Evidências: `TestTimelineSnapshotReadsOneCommitAcrossWindowAndTurn` intercala um
commit externo WAL entre versão e consultas e prova ausência de snapshots
híbridos (texto, ferramenta, estado, filhos). Regressões cobrem owner, thread,
metadado ausente, NULL/legado, cancelamento, retry e turno acima de 1.000 mensagens.
`TestTurnRuntimeRecoveryKeepsOneAttemptAndResponse` compara o patch realmente
emitido com a janela. A fixture de benchmark da janela mantém SQLite em memória;
as novas integrações usam InitPath com as migrações reais.

Validação local deste recorte: build, vet, golangci-lint (zero issues), testes
focados de snapshot, janela, runtime, envio e ledger; TypeScript, ESLint,
Stylelint e 5.887 testes frontend em 484 arquivos aprovados. Testes ACP não foram
executados localmente. A primeira execução frontend teve duas falhas sensíveis
a timing no Topbar; o arquivo isolado (122 testes) e a repetição completa com
quatro workers passaram sem alteração de expectativas.

Benchmark comparativo em cinco amostras de 20 iterações, mediana, mesma máquina:
para históricos de 100/500/1.000 mensagens, o tempo foi respectivamente de
3,197/9,013/11,768 ms para 3,553/9,362/12,242 ms (+11,1%/+3,9%/+4,0%).
Alocações passaram de 711.323/1.637.081/1.636.437 B/op para
777.663/1.772.782/1.792.104 B/op (+9,3%/+8,3%/+9,5%). Payloads permaneceram
37.262/88.881/88.882 bytes. Todos dentro do orçamento de 20%. A baseline usa o
commit f21161683 do recorte anterior; a candidata inclui metadados de turno
persistidos no preparo da fixture, fora da medição. O snapshot de turno com mais
de 1.000 mensagens teve mediana de 18,565 ms e 5.133.805 B/op em cinco amostras
de dez iterações, evidenciando a materialização ainda proporcional ao turno.
Revisão local independente: agente review_aep, três rodadas; zero pendências
na rodada final, incluindo conferência das métricas e diff --check.

Sétimo recorte da fase 2: `PersistedResponse` é o modelo interno compartilhado
pelos adaptadores da janela e do terminal. Quando existe `chat_turns`, seu
representante, estado e origem são a autoridade; referência não nula ausente ou
fora de conversa/turno/escopo falha explicitamente. Não há eleição por usage ou
data para substituir o representante persistido. NULL continua NULL mesmo com
mensagens intermediárias restantes. Ausência do registro usa a política legada
e `historical_unknown`, sem inferir sucesso a partir de texto ou tokens.

O motor existente agora produz `ResponseSegment` com identidade derivada de
mensagem/tipo/offset inicial ou da primeira invocação do grupo, usando codificação
inequívoca dos IDs opacos. Texto igual em mensagens diferentes não colide;
crescimento do texto não troca o segmento. Raciocínio é um segmento próprio;
resumos continuam leves e referências do ledger não são IDs do provedor.
A projeção mantém segmentos até para uma resposta simples. O adaptador antigo
suprime o caso simples e mantém o reasoning no campo antigo da mensagem.

Compatibilidade explícita: `legacyMessage` ainda escolhe uma mensagem para
apresentar quando o representante persistido é NULL, porque `MessageNode` não
admite mensagem nula. Essa escolha não altera o representante, o estado ou os
segmentos da projeção. Leitores antigos de árvore/lista continuam usando a
política histórica. Não há uma segunda montagem de segmentos: ambos os caminhos
usam o mesmo motor, com políticas distintas de seleção e adaptação.

Este modelo não é o DTO wire v1 completo: ordem total de itens/paginação,
representação visual de turno sem mensagens, epoch/sequence/revision, barreira
com eventos, projeção incremental e frontend permanecem pendentes. Janelas
projetam somente respostas materializadas; a leitura de turno preserva metadados
mesmo sem conteúdo. Não inserir respostas fora da janela nem inventar contagens.

Evidências: testes `TestPersistedResponse*` cobrem representante contrário à
heurística, estado explícito, NULL/legado, referências inválidas, IDs estáveis,
reasoning e vínculo opaco com espaços. `TestTimelineSnapshotProjectsSameAuthoritativeResponse`
compara projeção e adaptadores após edição tardia e exclusão do representante,
incluindo janela que contém somente a pergunta. Teste ACP teve somente adaptação
de nomes de campos ao tipo de segmento; não foi executado localmente.

Validação local deste recorte: build/vet, golangci-lint (zero issues), regressões
chat/snapshot/janela/runtime/envio, TypeScript, ESLint e Stylelint aprovados;
5.887 testes frontend em 484 arquivos aprovados, excluindo ACP. Não foi rodada
a suíte Go integral local nem qualquer teste ACP.

Comparação isolada contra o snapshot do PR #968, mesma fixture e binário Go,
cinco amostras de 20 iterações: medianas de tempo para 100/500/1.000 mensagens
3,640/9,470/12,696 ms → 3,674/9,414/12,568 ms (+0,9%/-0,6%/-1,0%).
B/op: 774.978/1.806.732/1.771.229 → 833.128/1.916.088/1.914.372
(+7,5%/+6,1%/+8,1%). Payload idêntico: 37.262/88.881/88.882 bytes.
Contra as alocações da baseline fixa da fase 1, o acumulado fica em
+19,9%/+17,6%/+17,6%: ainda no orçamento, mas o caso de 100 mensagens está
próximo do limite e deve ser acompanhado nos próximos recortes. A exceção de
payload já documentada no primeiro recorte não muda nesta entrega.
Revisão local independente: agente review_aep, quatro rodadas. Um achado de
normalização de IDs foi corrigido com regressão; gate final sem pendências.

Oitavo recorte da fase 2: a janela persistida usa a chave total do adendo
`(anchor.created_at, anchor.id, slot, itemId)`. Respostas usam a pergunta como
âncora, slot 1; mensagens independentes usam a própria identidade, slot 0.
Ordenação ascendente/descendente, contagem antes da âncora e paginação
before/after/around usam a mesma chave, com comparação binária dos IDs. A chave
temporal conserva a representação exata do SQLite, sem conversão ou perda de
nanossegundos ao montar os parâmetros da próxima consulta.

A classificação fica no CTE compartilhado: somente pergunta `user` na mesma
conversa e escopo pode ancorar uma resposta. A própria pergunta nunca participa
do grupo de resposta, mesmo com `turn_id` legado igual ao seu ID. Vínculos
inválidos são itens independentes com diagnóstico estruturado por fatia; a cópia
de mensagem retornada omite esse vínculo para os adaptadores não a reagruparem.
Nenhum reparo é gravado no banco. O teste de vínculo vazio usa o conjunto Unicode
White_Space no SQL e no Go; os bytes de IDs não vazios continuam intactos.

O CTE agrega os membros por item, removendo o ROW_NUMBER que elegia a primeira
mensagem para ordenar a resposta. Mensagens internas continuam buscadas em lote;
a pergunta só vem quando seu item está na janela. Uma âncora inválida/excluída
produz erro explícito mesmo em conversa vazia ou consulta de limite zero. Não
há novo cursor wire ou evento de sincronização neste recorte: a API atual mantém
seu contrato de erro e os limites da janela.

Evidências: `TestTimelineOrder*` verifica todos os limites before/after/around,
âncora intermediária, empates binários, nanossegundos, timestamps invertidos,
pergunta com vínculo próprio, vínculos inválidos/cross-conversa/cross-thread e
âncora excluída. Integração com migrações reais comprova classificação até os nós,
contagem/OriginalIndex, janelas só de pergunta/resposta e ausência de escrita na
leitura. Regressão percorre Unicode White_Space com perguntas realmente existentes
para impedir divergência entre a classificação SQL e a montagem Go.

Limites: este recorte ordena carregamento e paginação persistidos. Streaming ainda
usa eventos/adaptadores anteriores; não declarar corrigida a ordenação em tempo
real ou o foco. Projetor incremental, barreira snapshot/eventos, epoch/sequence,
inscrições/cursors wire e migração do consumidor frontend permanecem pendentes.

Medição isolada deste recorte contra #970, mesma fixture, cinco amostras de
20 iterações: medianas de tempo 100/500/1.000 mensagens
3,862/9,938/13,039 ms → 3,913/10,265/13,620 ms (+1,3%/+3,3%/+4,5%).
B/op: 827.238/1.914.348/1.935.189 → 827.231/1.867.098/1.885.280
(aproximadamente 0%/-2,5%/-2,6%). Payload inalterado: 37.262/88.881/88.882 bytes.
As alocações acumuladas contra a baseline fixa ficam +19,1%/+14,6%/+15,8%,
dentro do orçamento de 20%; o caso de 100 mensagens ainda exige acompanhamento.
Não houve nova consulta por mensagem nem ampliação do payload Wails.

Validação local: build/vet, golangci-lint (zero issues), testes focados de banco,
ordenação, snapshot, janela e runtime aprovados. TypeScript, ESLint, Stylelint
e 5.887 testes frontend em 484 arquivos aprovados. Nenhum teste ACP executado
localmente; suíte Go integral local não executada. Revisor independente
review_aep: três rodadas, achado de divergência Unicode corrigido com regressão;
zero pendências no gate final, incluindo conferência das métricas.

Nono recorte da fase 2: `LiveProjection` recebe comandos diretamente dos
handlers, antes dos timers de IPC, e conserva texto/raciocínio por execução,
iteração e fragmento. O lifecycle instala um binding imutável após admissão no
TurnStore; a factory de handlers agênticos deve implementá-lo. Callbacks de
execuções substituídas, tentativas de transporte anteriores e streams selados
não alteram a projeção. Retry de transporte reinicia explicitamente sua iteração;
retry explícito troca executionId e conserva itemId da pergunta.

A serialização é por usuário/conversa. Não há SQL por delta nem novo timer de
coalescing. O ledger permanece autoridade de ferramentas; o projetor não copia
argumentos/resultados nem cria uma segunda máquina de estados dessas atividades.
IDs `draft:<execution>:<iteration>:<fragment>` identificam fragmentos internos
voláteis; não são IDs persistidos de segmentos nem um novo contrato wire. Seal
aceita a substituição explícita do conteúdo final pelo provider. A revisão interna
não é a sequence do protocolo. A futura hidratação precisa declarar a substituição
dos fragmentos pelo modelo persistido, preservando itemId.

Conclusão distingue confirmação do conteúdo de confirmação apenas do estado.
Panic/retorno sem terminal após chunks conserva conteúdo volatile, mesmo quando
o metadado failed foi salvo. Falha de gravação mantém failed/persistence_error.
Conteúdo concluído e confirmado pode ser descartado após 60 s; ativo/volatile
não expira. Uma tentativa volatile substituída permanece retida por executionId.
O segundo PR deste par entrega leitura/descarte autorizado e barreira com storage;
este primeiro PR não expõe ainda esse conteúdo por RPC nem promete recuperação
visual. Os acumuladores legados permanecem temporariamente até o cutover, sem
terceira cópia consolidada; medir o custo incremental é parte deste recorte.

Evidências: `TestLiveProjection*` cobre callback antigo, retry de transporte e
explícito, segmentos, substituição final, snapshot concorrente, isolamento,
retenção e terminal sem conteúdo confirmado. O runtime real usa as migrações e
prova preservação antes de flush e falha de escrita parcial. Eventos legados
continuam sujeitos ao contrato anterior: a guarda do projetor não torna a emissão
IPC atômica com a troca de execução. Foco, protocolo wire, inscrições e migração
do frontend permanecem pendentes; fase 2 continua In Progress.

Confirmação de parcial/intermediário ocorre explicitamente após a escrita
correspondente, por iteração e bytes exatos; novos deltas invalidam essa confirmação.
Isso permite expirar cancelamentos já salvos sem perder trechos posteriores nem
intermediários cuja gravação falhou. A substituição/descarte de reasoning abrange
o stream inteiro, inclusive após cortes.

Validação local: build/vet e golangci-lint (zero issues); regressões de projeção,
lifecycle real, patches e finalização aprovadas. TypeScript, ESLint e Stylelint
aprovados (2.124 avisos CSS preexistentes), Vitest com 484 arquivos e 5.887 testes
aprovados. Os 31 arquivos frontend relacionados a ACP foram excluídos; nenhum
teste ACP nem a suíte Go integral foram executados localmente. O detector de corrida
local não executou porque o toolchain disponível está sem CGO; CI mantém esse gate.

`BenchmarkLiveProjectionAppend` isolado, cinco amostras de 100 operações, cada
operação criando uma execução e recebendo 1.000 deltas de 18 bytes: mediana
57,868 microssegundos, 84.825 B/op, 35 allocs/op. Isso mede custo incremental do
projetor, não heap retido ou ganho contra o handler. O snapshot, o IPC e o buffer
legado não participam dessa fixture; a duplicação transitória dos acumuladores
continua explícita até a fase 3. Não houve alteração de SQL/payload da janela.
Revisão independente `review_aep`: problemas de confirmação de conteúdo e
substituição de reasoning corrigidos com regressões; gate técnico sem pendências.

Revisão remota adicional: o runner agêntico verifica a propriedade do stream
antes de interpretar o resultado. Rejeição por binding substituído encerra a
iteração, sem converter resultado zero em rodada de ferramentas vazia ou erro
repetível do provider. Regressão cobre OnDone e OnToolCalls atrasados, sem retry
nem segment_done; o resultado selado de um binding válido permanece aceito.

Décimo recorte da fase 2 (PR empilhado sobre o nono): `ReadTurnRecovery`
coordena leitura persistida autorizada e buffers em memória pela mesma barreira
por usuário/conversa. Até a primeira leitura reserva a instância, impedindo que
uma admissão simultânea crie outro projetor. Referências de operações impedem
evicção sem manter o mutex global durante I/O de uma conversa. Admissão,
atribuição da resposta, gravações de conteúdo e commits terminais usam essa
barreira. Setters/finalizadores diretos substituídos foram removidos.

O resultado separa `Persisted`, `Active` e `Retained`. Active exige executionId
e incarnation compatíveis; ausência de active_execution_id no banco não prova
compatibilidade com uma execução terminal. Tentativas retidas não são mescladas
por texto, prefixo ou horário, e representante persistido nulo continua válido.
Leitura recusada por propriedade/exclusão/erro de storage não expõe conteúdo da
memória. A cópia permanece protegida para nova consulta autorizada.

`FinishResponseWithVersion` captura incarnation/revision na própria transação
que grava resposta e estado. O adaptador de patch terminal só usa o persistido
quando há prova positiva da execução e versão exata. Assim, o terminal A não
publica o patch de B, mesmo se B já terminou. Mudança posterior de storage sem
prova de compatibilidade retorna recoveryRequired e inicia nova época interna,
sem repetir a troca para a mesma versão. Isso é conservador: até uma mudança
legítima de outro turno pode exigir ressincronização. A leitura sempre reidrata
o estado persistido atual; não há cache persistido nem SQL por delta. O lifecycle
de startup reutiliza a mesma gravação sem exigir v43 antes da migração; a admissão
de novos envios exige v43 antes de gravar a pergunta.

`DiscardRetainedTurn` endereça usuário/conversa/turno/executionId, autoriza pelo
banco e é idempotente. Recusa execução ativa; uma execução incompatível com o
storage precisa primeiro ser marcada como desvinculada na recuperação para
permitir descarte. Retry conserva conteúdo não confirmado da tentativa anterior,
mas essa retenção não bloqueia o patch de uma nova tentativa confirmada.

Evidências: `TestTurnRecovery*` cobre snapshot/delta, commit/leitura, primeira
leitura/admissão, independência de conversas sob I/O, terminal A/retry B ativo e
concluído, versão externa, mudança de incarnation, falha de autorização, descarte
idempotente e recusa de execução ativa. Integração com SQLite real cobre representante
nulo, exclusão, retenção após término sem gravação, retry, descarte e versão do
commit terminal. Testes de lifecycle/startup e v43 permanecem íntegros.

Limites: essas APIs são internas do backend. O adaptador terminal já usa a leitura
coordenada, mas recuperação visual/descarte ainda não estão expostos por Wails.
O par não entrega protocolo wire, inscrições, cursores ou consumidor frontend.
Eventos legados continuam sem garantia atômica entre verificação e emissão;
correção integral de foco/ordenação permanece dependente do cutover. Fase 2 segue
In Progress; não declarar esses defeitos de UI resolvidos por esta entrega.

Validação do décimo recorte: build/vet, golangci-lint (zero issues), regressões
selecionadas de projeção/recuperação/runtime/finalização/lifecycle/v43 aprovadas.
TypeScript, ESLint e Stylelint aprovados (avisos CSS preexistentes); Vitest com
484 arquivos e 5.887 testes aprovados, excluindo os 31 arquivos relacionados a
ACP. Nenhum teste ACP nem a suíte Go integral foram executados localmente;
detector de corrida fica no CI por indisponibilidade local de CGO.

Benchmark incremental isolado, comparando #972 (7708c31f4) com este recorte,
mesma fixture de 1.000 deltas/18.000 bytes e cinco amostras de 1 s:
medianas 78,225 → 87,899 microssegundos (+12,4%), 84.824 → 84.984 B/op (+0,19%),
35 → 36 allocs/op. A alocação adicional corresponde à liberação da referência
que impede evicção durante I/O. As amostras curtas de 100 operações apresentaram
variação alta e não foram usadas nesta comparação. O benchmark mede admissão
em memória e append, não custo SQL, heap retido ou UI; SQL/payload da janela
persistida não foram alterados. Revisor independente `review_aep`: prova positiva
do terminal e barreira da primeira leitura corrigidas com regressões; gate
final sem pendências, incluindo captura dinâmica de conteúdo não confirmado.


### Fase 3 — Consumidor frontend e remoções

- [ ] Migrar para itens e janelas por IDs, preservando isolamento das superfícies.
- [ ] Remover ordenador híbrido, chave alternativa, reconstrução semântica e espelhamentos substituídos.
- [ ] Unificar corte da janela e eliminar flags derivadas duplicadas.
- [ ] Preservar identidade DOM/foco/leitura, inclusive no limiar de virtualização.
- [ ] Validar descarte de cache e consultas concorrentes com eventos.

Saída: uma única aplicação do contrato, sem caminho antigo ativo em paralelo.

### Fase 4 — Anexos e recortes de armazenamento

- [ ] Remover leitura desnecessária de áudio da janela.
- [ ] Unificar anexos e migrar duplicação de áudio com integridade e portabilidade.
- [ ] Implementar apenas mudanças de esquema aprovadas na fase 1; legacy de threads em recorte separado se necessário.

Pode ser PR independente após fechar o contrato de anexos; não bloqueia a
correção inicial de identidade e foco das fases 2–3.

### Fase 5 — Encerramento e evidências

- [ ] Remover adaptadores e APIs comprovadamente sem consumidores.
- [ ] Preservar regressões de produto ao substituir testes de mecanismos removidos.
- [ ] Medir consultas, payload, retenção de memória e renders antes/depois.
- [ ] Atualizar documentação de usuário, AEPs afetados e índice com evidências.

Não apagar ou enfraquecer testes para obter aprovação. Reescrever cobertura de
mecanismos extintos somente mantendo os comportamentos e falhas relevantes.

## Riscos

- Troca de chave, wrapper ou conteúdo interno pode perder foco mesmo com ID estável.
- Projeção incremental e reload podem divergir se usarem regras independentes.
- Revisão mal definida pode rejeitar atualização válida ou ressuscitar item excluído.
- Retirar fallback sem recuperação perde texto em falhas de persistência.
- Cache normalizado sem descarte mantém crescimento de memória.
- Migração de anexos pode quebrar mídia histórica, exportação ou referências.
- Transferir trabalho ao backend sem limites pode aumentar IPC, SQL e latência.

## Critérios de aceitação

- [ ] Resposta nunca precede sua pergunta; ordem total é igual em janela, eventos e reload.
- [ ] Conclusão não remonta o item em leitura nem desloca foco intencional.
- [ ] Snapshot antigo, duplicação, lacunas e terminais de outra tentativa têm comportamento testado.
- [ ] Exclusão, troca de usuário/sessão e duas superfícies preservam isolamento.
- [ ] Histórico após reload equivale à projeção persistida dos eventos, inclusive tools e continuação.
- [ ] Erro de persistência/projeção é explícito e não apaga silenciosamente conteúdo recebido.
- [ ] Frontend não infere identidade, ordem, representante ou composição conversacional.
- [ ] Anexos não duplicam áudio recebido nem entram integralmente na janela sem necessidade.
- [ ] Metas de desempenho da fase 1 passam em fixtures reprodutíveis e comparáveis.
- [ ] Não restam adaptadores sem prazo de remoção nem duas fontes autoritativas.

NVDA não é requisito desta investigação/PR documental, por orientação do
mantenedor. Os critérios de teclado, foco, semântica acessível e leitura
continuam obrigatórios; testes DOM não serão apresentados como validação NVDA.

## Adendo da fase 1 — contrato e inventário (30/10/2026)

Este adendo especifica o alvo das fases 2–4. Os DTOs abaixo ainda não são
bindings produtivos. As provas adicionadas nesta fase não implementam o protocolo.

### Inventário de responsabilidades e consumidores

- Entrada e retry: `internal/core/usecases/send_message.go`,
  `internal/chat/interactor.go`, `internal/chat/handler.go`. Persistem a pergunta,
  resolvem mídia, asseguram placeholder e finalizam a resposta. Manter um único
  pipeline e a fila por conversa; preparar identidade antes de emitir item.
- Streaming direto/agentic: `internal/agent/simple_stream_handler.go`,
  `stream_handler_base.go`, `agentic_loop.go`, `service.go`; ACP:
  `agent_activity.go`. Produzem texto, reasoning, segmentos, tools e terminal.
  Devem alimentar um projetor comum em vez de diferentes montagens da timeline.
- Eventos auxiliares: `internal/events/base_handler.go` e `emitter.go` também
  emitem stream/thinking/erro. Inventário da migração inclui esses caminhos;
  preparação sem mensagem permanece aviso, sem criar item local.
- Persistência e projeção de janela: `internal/database/message_repository.go`,
  `controllers/conversations_controller.go`, `internal/chat/timeline.go`.
  Retornam ordenação/contagem da mesma unidade; retirar inferência de representante
  do consumidor sem perder âncoras internas e autorização por conversa.
- Frontend: `chatEventHub.ts` filtra execução; `chatEventController.ts` monta
  segmentos e continuação; `chatSessionRegistry.ts` e `chatStore.ts` espelham
  árvores/janelas. Substituir a montagem, não duplicar esses caminhos.
- Consumidores laterais: `TokenStatsButton.tsx`, `TokenStatsModal.tsx`,
  `useEditorInlineChatPatch.ts`, `useEditorInlineChat.ts` e os serviços de voz
  dependem de eventos de término/tools. Preservar seus efeitos uma única vez;
  evento de sincronização/reload nunca repete fala, aplicação no editor ou aviso.
- Canais: `internal/messaging/gateway.go`, `notifier.go` e o controller de
  messaging encaminham respostas e limpam callbacks. Não presumir que um novo
  evento visual substitui o notifier; terminal mantém a conclusão operacional
  e a liberação em `OnFinished` do pipeline existente.
- Exposição: `internal/wailsapi/conversations.go` e `speech.go`; controllers
  são fronteira reutilizada pelos transportes. `cmd/asst/chat.go` chama
  SendMessage/GetConversation/CancelStreamingForConversation no App;
  `adapters/cli/emitter.go` consome stream, done, error, segment_done, notice e
  tool events, incluindo espera de término. Migrar esse adaptador junto do
  contrato, preservando stdout incremental e término idempotente. As rotas em
  `internal/httpapi/server.go` expõem autenticação e execução/consulta de comandos
  externos, não um endpoint de snapshot/stream de timeline. A fonte de comando
  `chat` não equivale a transporte de histórico. Não criar envio paralelo nem
  exigir superfície Wails para projetar uma conversa.
- Contexto e sumarização: `internal/chat/history.go`, `media.go`,
  `internal/summarization/service.go`, tools em `internal/tools/history/` e
  estatísticas em `internal/database/token_repository.go` leem mensagens/ledger.
  Não substituir contexto do LLM pela projeção visual nem contar itens como chamadas.
- Importação/exportação: `internal/portability/service.go` escreve diretamente
  mensagens e remapeia relações; deve invalidar a projeção e reconstruir metadados.
  Speech em `internal/speech/service.go` grava áudio posteriormente; pin, edição,
  exclusão e limpeza no repositório também são mutações, não apenas SendMessage.

### Campos persistidos: decisão de retenção

- `id`, `conversation_id`, `role`, `content`: manter. Escrita pelo repositório,
  pipeline e importador; leitura por histórico, contexto, ferramentas e exportação.
- `created_at`: manter imutável durante edição/continuação; usar como dado legado
  de ordenação no backend. `updated_at`: manter para auditoria, nunca como identidade
  ou precedência pública. Imports substitutivos invalidam a época da projeção.
- `turn_id`: manter como vínculo à pergunta; `parent_id`: manter para threads,
  exclusão e importação. Validar mesmo owner/conversa e ciclos; não remover por
  ausência de chamada UI a `AddChildMessage`.
- `reasoning`: manter; handler/repositório gravam, projeção e leitura consomem.
- `prompt_tokens`, `completion_tokens`, `total_tokens`, `cache_read_tokens`,
  `cache_write_tokens`, `cache_miss_tokens`, `model`: manter valores reportados.
  Estatísticas agregadas e ocupação atual são consumidores distintos. Não usar
  `total_tokens > 0` para escolher a identidade da resposta.
- `source`, `pinned`: manter origem e ação persistente de fixação; projeção entrega
  metadados sem inferência visual. Speech não deve invalidar foco ao atualizar áudio.
- `media`, `audio`, `audio_mime_type`: manter compatibilidade até fase 4; migrar
  para anexos com referência única e papéis received/generated. Primeiro áudio
  recebido não será copiado em dois lugares. Contexto e playback resolvem a mesma
  referência; arquivos externos só se houver gestão de ciclo de vida e backup.
- `chat_message_revisions`: manter exclusivamente para comandos/concorrência
  existentes. Seu nonce não vira número de versão nem credencial da timeline.
- `tool_invocations`: continua fonte única técnica. Migração não reintroduz
  `tool_calls`, `tool_call_id` ou mensagens `role=tool` persistidas.

### DTO wire v1 e ordenação

IDs e cursores são strings opacas; inteiros incrementais são strings decimais
para não perder precisão no JavaScript. Todos os retornos são autorizados pelo
usuário/sessão do transporte, nunca pelo owner fornecido no payload.

- `TimelineItem`: `itemId`, `conversationId`, `scopeParentId` (null na raiz),
  `kind` (message/response), `turnId` (null em item sem turno), `replyToItemId`,
  `messageIds`, `representativeMessageId`, `position`, `revision`, `state`,
  `durability`, `segments`, `metadata` (origem, pin, uso, modelo e anexos leves).
- `TimelineSegment`: `segmentId`, `kind` (text/reasoning/tool_invocations),
  `text` ou `invocations` (resumos do ledger); ordem é a posição no array.
  Proibido transportar argumentos/resultados integrais de tools nesta projeção.
- `TimelineWindow`: `conversationId`, `scopeParentId`, `epoch`, `sequence`,
  `items`, `orderedItemIds`, `totalCount`, `startIndex`, `endIndex`, `hasBefore`,
  `hasAfter`, `beforeCursor`, `afterCursor`. Índices são de itens navegáveis.
- `TimelineUpdate`: `conversationId`, `epoch`, `sequence`, `executionId` quando
  houver execução, `operations`. Operações: item upsert/remove, segment insert/
  append/replace/remove e metadados de contagem. A ordem materializada é entregue
  em patches de janela descritos abaixo; frontend não compara `position` localmente.
- `TimelineSyncRequired`: `conversationId`, nova `epoch`, razão controlada.
  Não remove dados visíveis enquanto sincronização não completar; bloqueia
  aplicação de deltas incompatíveis e sinaliza carregamento/erro de forma acessível.

Janelas parciais têm `windowId` atribuído pelo backend, vinculado à sessão,
superfície, conversa e escopo; fechar superfície cancela essa inscrição. O
backend conhece os IDs materializados e entrega, no mesmo lote, `windowPatches`
endereçados por windowId com splices (índice local, quantidade removida, IDs
inseridos), itens necessários e metadados finais de paginação/contagem. Os
índices são calculados pelo backend e aplicados na ordem do lote. Cada patch
informa a revisão anterior da janela; divergência exige resync dessa janela.
Atualização de conteúdo de item existente não substitui a lista nem seu DOM.

Evento fora da janela atualiza somente itens já presentes no cache compartilhado
e os metadados de contagem/cursores enviados pelo backend. Não insere item sem
patch endereçado, não procura predecessor ausente e não dispara reload. Janela
histórica preserva suas âncoras; janela inscrita em modo followTail recebe os
splices de crescimento/corte. Mudar de modo é comando ao backend. Foco/leitura
fixam IDs via inscrição para impedir corte; fechar libera essa retenção. A
mesma conversa pode ter duas janelas diferentes e uma única sequência: lotes
sem alteração local também avançam essa sequência. Snapshot/paginação atualizam
a inscrição na mesma barreira do projetor. Snapshot não é disparado por cada
token; somente lacuna real, época ou revisão incompatível exige recuperação.

Fixture obrigatória na fase 2: duas superfícies, uma no início do histórico e
outra na cauda, recebem nova pergunta/resposta. A primeira mantém IDs/foco sem
reload e recebe contagem; a segunda recebe inserções ordenadas. Incluir remoção
da âncora, paginação concorrente e fechamento de uma das inscrições.

Identidade: mensagem simples usa `message:<id>`; resposta usa `turn:<userId>`
desde o primeiro evento. `representativeMessageId` pode mudar sem trocar itemId.
Uma mensagem histórica sem turno permanece item message até migração explícita,
que inicia nova época; não se promove silenciosamente durante streaming.

Ordem backend: chave total `(anchor.created_at, anchor.id, slot, itemId)`,
onde resposta usa a pergunta como anchor e slot=1; mensagem simples slot=0.
Escopo de thread é separado. A comparação é feita no backend com a precisão do
banco e comparação binária de IDs, não `localeCompare`. Import com vínculo
inexistente não inventa pergunta: rejeitar vínculo inválido no novo formato;
legado é classificado e preservado como item independente com diagnóstico.

`position` e cursores codificam essa chave e escopo; não são autorização. Paginar
por cursor validado no mesmo owner/conversa; âncora excluída retorna sincronização
necessária, nunca outra conversa nem janela vazia falsamente definitiva.

### Sequência, consulta concorrente e tentativas

Projetor por conversa serializa mutações visuais. `epoch` é UUID aleatório por
instância da projeção (reinício/evicção/import substitutivo); `sequence` cresce
por lote emitido. `revision` do item é a sequence de sua última mutação nessa
época. Não comparar sequências de épocas diferentes.

Snapshot captura estado do projetor em uma barreira; consulta persistida e
alterações concorrentes são reconciliadas pelo backend antes da resposta.
Cliente registra eventos antes da consulta e mantém buffer limitado. Ao receber
snapshot S, aplica-o e reaplica apenas eventos contíguos posteriores a S na
mesma época. Se já processou estado mais novo, não sobrescreve diretamente com
S. Duplicatas são ignoradas; lacuna, overflow ou mudança de época exige resync,
sem interpretar timestamps. Paginação em andamento usa a mesma regra, com
identidade da solicitação/superfície para descartar retorno de sessão encerrada.

Limites iniciais: buffer de até 256 lotes ou 2 MiB por conversa; atingir qualquer
limite descarta o buffer e pede snapshot. Backend mantém até 256 lotes/2 MiB
para replay na época; fora disso entrega snapshot. Esses limites não descartam
texto exibido, apenas o histórico de transporte. Deltas seguem coalescing de
24 ms; terminal descarrega o lote pendente antes de concluir.

Cada tentativa tem `executionId` próprio gerado/validado pelo backend, vinculado
a usuário, conversa e turno. Retry/continuação não mudam itemId nem a posição;
mudam execução e estado, com reset explícito de segmentos definidos pelo backend.
Evento de execução encerrada não altera a atual. Remoção gera tombstone na
sequência; após resync o snapshot substitui a janela e remove itens ausentes,
impedindo ressurreição por respostas antigas.

### Estado, falhas e armazenamento aprovado para fase 2

Estados da resposta: preparing, streaming, completed, interrupted, failed e
historical_unknown (somente legado sem evidência terminal).
`durability`: persisted ou volatile. Falha antes de criar pergunta retorna erro
operacional sem item. Uma pergunta confirmada e uma falha em criar resposta não
são confundidas com resposta vazia concluída.

Adicionar `chat_turns` com `user_message_id` (PK/FK), conversation_id,
response_message_id (FK nullable), state e active_execution_id; e
`chat_turn_attempts` com execution_id (PK), turn_id (FK), modo new/retry/continue,
state, início/fim e motivo terminal controlado. Não duplicar texto/usage/tools
nessas tabelas. Constraints de estados/modos; vínculos pertencem à mesma conversa
na transação. Exclusão da pergunta remove metadados do turno e suas tentativas.
Não criar tabela materializada de timeline ou cópia persistida do JSON visual.

`response_message_id` torna explícita a mensagem final, eliminando escolha por
uso/timestamp nos turnos novos. Mensagens intermediárias continuam conteúdo;
ledger fornece a relação e os offsets para tools/ACP. IDs de segmentos derivam
de referências persistidas e posição de trecho, não do texto. Edição/retry pode
substituir segmentos por operação explícita; identidade do item permanece.

Upgrade: backfill determinístico usa o consolidador atual uma única vez para
selecionar representante; não inventa execução histórica. Turnos legados recebem
origem imported/legacy em metadado de migração separado do estado operacional.
Somente registro terminal explícito comprova completed/failed/interrupted;
texto ou tokens não provam término. Sem essa evidência, expor historical_unknown,
sem indicador de erro/interrupção nem oferta automática de continuação. Preservar
leitura normal do conteúdo histórico; ação explícita de nova execução passa pelo
mesmo comando e validações do backend. Não disparar retry automático no upgrade. Backup,
transação e verificação de vínculos precedem ativação; qualquer falha preserva
schema anterior. O importador reconstrói vínculos/identidades, não restaura
executionId ativo de outra instalação. Rollback após novas escritas exige
exportação compatível ou restauração de backup, nunca downgrade silencioso.

Mutação de mensagem/ledger/turno invalida projeção pelo caminho transacional
comum. Escritores diretos (import, manutenção, speech) precisam participar;
triggers de invalidação persistente por conversa cobrem SQL fora do repositório.
Esse contador detecta alterações de storage, não substitui a sequência de deltas
voláteis. Antes de snapshot o projetor verifica o contador; alteração externa
não reconciliável inicia nova época. Nenhuma consulta SQL por token.

Ao falhar gravação terminal, manter projeção incremental volatile e emitir
failed com causa persistence_error, sem sucesso falso. Snapshot de recuperação
na mesma instância entrega essa projeção; após reinício só conteúdo confirmado
é recuperável e tentativa antes ativa vira interrupted. Falha em consultar o
patch não perde texto: projetor já possui estado recebido, emite estado com
recoveryRequired e repete sincronização limitada. Liberação da fila/OnFinished
é garantida mesmo se gravação ou emissão falhar. Novo envio não apaga item
volatile; ele permanece até descarte explícito ou persistência bem-sucedida.

### Cache e anexos

Frontend mantém referências às janelas (limite atual de 240 itens), foco/leitura,
edição e execuções ativas; itens sem referências entram em LRU limitada a 240
itens ou 8 MiB por usuário. Fechar última superfície remove sua janela e libera
referências; logout limpa todo o cache. Itens ativos/volatile não são expulsos
silenciosamente: limite excedido bloqueia novas retenções opcionais e pede
resolução explícita quando não houver conteúdo dispensável.

Backend conserva projetor enquanto houver execução ou assinante, com descarte
após 60 s sem ambos; conteúdo volatile exige persistência ou descarte explícito,
não timeout. Reinício/evicção produz nova época e snapshot. Não armazenar JSON
integral de tools no cache visual. Medir retained heap e contagem de itens em
abertura/fechamento repetidos na fase 3.

Fase 4 adiciona anexos referenciados por mensagem, com ID, papel, MIME, tamanho
e conteúdo binário único; deduplicação limitada ao usuário. Tabela de vínculos
preserva ordem dos anexos e distingue recebido de TTS gerado. Migração valida
bytes/hash e export/import antes de retirar media/audio; payload visual contém
somente metadados e IDs. Não adotar arquivos soltos sem lifecycle/backup.

### Provas executáveis e transição dos testes

`frontend/src/services/chatTimelineContract.test.ts` registra os contratos de
ordem hoje violados. `MessageList.identity.test.tsx` registra perda de foco sem
focusout artificial e controles de identidade estável/saída intencional.
Três casos usam `it.fails`: a expectativa é o comportamento correto, a falha é
conhecida e obrigatória nesta fase. Não são testes pulados; se o defeito deixar
de ocorrer, Vitest exige retirar `.fails`. A fase 3 deve convertê-los em testes
normais, sem inverter expectativas. Não alterar testes existentes para mascarar
falhas. O renderer do nó é simplificado; não é validação de Markdown/NVDA.

`TestACPCronologiaNoTerminalEReabertura` compara agora o patch completo do evento
terminal com reconstrução em outra instância, em sucesso, erro, cancelamento e
cauda vazia. Isso comprova paridade do contrato atual no caso ACP; não comprova
o protocolo futuro. Fase 2 acrescenta as mesmas provas para DTO v1, ferramentas
locais/nativas, concorrência snapshot/delta, exclusão e mudança de época.

### Medição reproduzível e limites

Baseline: Windows/amd64, Intel Core Ultra 7 155H, Go 1.27.1 (toolchain local),
checkout limpo de produção em c7a75bf72; alterações desta fase são testes/docs.
Temporários e GOCACHE dentro de build/bin do worktree. Dependências frontend
reutilizadas localmente; CI valida instalação pelo lockfile.

Comando: `go test ./internal/app -o build/bin/phase1/window-benchmark.test.exe -run '^$' -bench
'BenchmarkConversationMessageWindowBaseline/messages_(100|500|1000)_result_1024$'
-benchmem -benchtime=20x -count=5`. Comparar mediana de cinco amostras, nunca o
melhor resultado. Fixtures usam SQLite em memória, ledger e serialização; medir
mídia pesada e cold disk separadamente na fase 4.

Medição isolada em 30/10/2026, após encerrar as suítes concorrentes, com
GOMAXPROCS padrão (22). Cada amostra executou 20 operações:

- 100 mensagens: amostras 3,703 / 3,721 / 3,801 / 3,641 / 3,614 ms;
  mediana 3,703 ms, 694.841 B/op, 10.630 allocs/op; janela 33.212 bytes.
- 500 mensagens: 10,181 / 11,447 / 11,641 / 11,666 / 13,682 ms;
  mediana 11,641 ms, 1.629.859 B/op, 24.140 allocs/op; janela 79.161 bytes.
- 1.000 mensagens: 13,779 / 13,267 / 13,255 / 13,498 / 12,813 ms;
  mediana 13,267 ms, 1.628.394 B/op, 24.136 allocs/op; janela 79.162 bytes.

Bytes e alocações são medianas independentes das cinco amostras. As quantidades
100/500/1.000 são mensagens persistidas; a consulta limita a janela a 240 linhas,
não transfere todo o histórico. Esta execução substitui a amostra exploratória
única citada no diagnóstico. A rodada sob carga concorrente foi descartada como
baseline; não comparar seus tempos com os isolados como ganho de implementação.

Orçamentos antes da implementação: mediana de tempo e B/op da mesma fixture no
mesmo ambiente não cresce mais de 20% contra baseline; payload da janela não
cresce mais de 10% sem justificativa revisada. Zero queries por delta; terminal
não recarrega a conversa inteira. Na UI, testes existentes de MessageList
performance exigem zero renders adicionais de nós estáveis por mudança de
loading e um único nó atualizado quando só seu conteúdo muda, em 100/500/1000
nós. Medida de heap de WebView permanece gate da fase 3, não resultado desta fase.

### Validação e restrições desta entrega

TypeScript, ESLint e Stylelint passaram (Stylelint mantém avisos preexistentes).
Vitest completo inicial: 515 arquivos, 6.846 testes; após revisão remota,
12 testes focados passaram, incluindo controle separado de índices e a prova
de transitividade para todas as triplas distintas. TypeScript e ESLint dos
arquivos alterados passaram novamente. Go build e vet passaram. Antes da
restrição abaixo, a fixture ACP de paridade passou, inclusive em execução com
binário de saída explícito. Esses resultados não significam correção de runtime.

golangci-lint local 2.15.0 apontou 12 ocorrências em arquivos idênticos à base;
`golangci-lint run --new-from-rev=c7a75bf72` passou com zero ocorrências novas.
Não declarar o lint global limpo; CI usa a versão configurada no workflow.
Revisor independente `review_aep`: quatro rodadas; primeira com três achados,
segunda aprovou as correções, terceira aprovou o diff completo e quarta aprovou
os ajustes da revisão remota sem pendências. CodeRabbit: achado de transitividade
e sugestão de identidade DOM atendidos no mesmo push consolidado.

A suíte Go completa foi interrompida: houve timeout do teste de scheduler do
atualizador sob carga (três repetições isoladas passaram) e encerramentos
0xffffffff em pacotes ACP. A pedido do mantenedor, não repetir testes ACP
localmente: podem disparar antivírus e alertas de Infosec. Não executar
`go test ./...` localmente nesse contexto. Preservar os testes versionados e
registrar a restrição; a cobertura completa fica a cargo do CI remoto.
