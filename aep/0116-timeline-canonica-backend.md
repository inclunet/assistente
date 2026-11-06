# AEP-0116 — Timeline canônica de chat do backend à apresentação

## Status: In Progress — fase 1 concluída; fase 2 com lifecycle, ledger, mensagens, áudio, limpeza, exclusão e importação integrados; retenção por idade, órfãos, navegação, leitura leve e orçamento de fontes da janela com recuperação atômica e parcelada e participação histórica automática com publicação e replay atômicos integrados; followTail, retenção por foco/leitura, DTOs compartilhados, RPCs de inscrição, entrega pelo journal e transporte de eventos Wails integrados; núcleo transacional, coordenador e cliente Wails por sessão frontend e identidade de recuperação por execução, consulta autorizada de estado, leitura limitada, reserva explícita dos buffers finalizados, leitor de recuperação Wails, reserva do DTO canônico publicado e transferência de recuperação no Refresh integrados; DOM da lista unificado com retenção visual por foco; consumidor visual canônico e identidade dos segmentos integrados; reconstrução visual legada retirada do controlador operacional, incluindo invalidação na limpeza/reset e gates de persistência; gates remotos deste recorte e margem de desempenho pendentes

Data: 2026-10-30

### Recorte: retirada da reconstrução visual legada (2026-11-05)

Decisão confirmada pelo mantenedor em 05/11/2026: falha ao registrar uma
ferramenta encerra o turno com `persistence_error`, preserva o conteúdo recebido
e impede a execução da chamada sem admissão persistida. Não há diagnóstico de
ferramenta fabricado pelo frontend. Falha ao confirmar o resultado de uma ação
já executada impede retries e a próxima chamada ao modelo; não desfaz o efeito.
A mesma regra encerra o chat se o registro de MCP nativo falhar, embora sua ação
já tenha ocorrido externamente. Um lote paralelo pode conter ações já iniciadas;
nenhum novo grupo sequencial ou retry começa depois de detectar a falha.

`TestExecutionWriterLedgerFailureCannotSilentlyLoseDiagnostic` cobre, com SQLite
real, admissão, confirmação do resultado, falha adicional ao salvar parcial,
load_skill antes de ferramentas regulares, falha de confirmação da skill, MCP
nativo em terminal/iteração/erro e interrupção dos retries de um lote. Conta as
execuções e chamadas ao modelo e verifica o terminal canônico e o texto preservado.
Os quatro testes frontend correspondentes agora verificam o aviso de persistência
e a fonte canônica intacta, com/sem parcial e durabilidade persistida/volátil.

O controlador operacional mantém admissão, fila, cancelamento, anúncios, áudio e
correlação dos eventos. Não cria mensagens, não agrega deltas para apresentação
nem reconcilia patches terminais. A apresentação e a paginação pertencem ao
cliente canônico; o carregador de sessão consulta somente metadados da conversa.
O editor lê a conclusão pelas referências canônicas da execução concluída numa
capability própria, sem polling da árvore da UI. Fechar o modal ou trocar seu
vínculo invalida a leitura. A confirmação e a aplicação também verificam o
vínculo capturado; voltar à conversa anterior não reativa a operação descartada.
A segunda rodada remota do PR #1023 motivou os dois testes de troca durante
leitura e confirmação em `EditorPage.test.tsx` (57 testes do arquivo aprovados).

A migração dos testes acompanha a responsabilidade, sem restaurar um compositor
legado nas fixtures. Provas antigas de composição do controlador são substituídas
por provas do writer real, SQLite, projetor e apresentação, revisadas pelo revisor
independente `review_aep`:

- Retomada do escalar com delta e preservação dos cortes/ferramentas:
  `TestCompositionReplacesConfirmedPartialBySourceThenReturnsToStorage` e
  `TestCompositionKeepsConfirmedPrefixOnceAndScopesRepeatedIterations`.
- Retry com texto igual à resposta anterior e exclusão das ferramentas antigas:
  `TestTimelineExecutionSelectionMatchesWindowAndTerminal`, incluindo conteúdo
  idêntico entre execuções e preservação física da tentativa anterior.
- Retry sem atividade seguido de continuação, com falha ou limite de saída:
  `TestExecutionWriterRetryWithoutActivityThenContinueExcludesDiagnostic`.
- Continuação sobre conclusão editada, inclusive ferramenta antes do texto:
  `TestExecutionWriterContinuationPrefixOccursOnceAcrossToolIterations`.
- Textos idênticos de fontes distintas:
  `TestCompositionPreservesIdenticalTextFromDistinctSources`.
- Retry/continuação somente com ferramentas, conclusão, erro, cancelamento e
  limite: `TestExecutionWriterToolFirstTerminalUsesExplicitExecutionMode`.
  A prova cobre leitura entre iterações e reabertura persistida. A transferência
  de conteúdo/reasoning para a intermediária é atômica, sem duplicar o prefixo
  deixado no representante; `TestTurnIntermediateTransfersContentAtomically`
  verifica rollback na inserção e na limpeza, usuário e execução vigentes.

`chatTimelinePresentation.test.ts` mantém as provas de ordem, identidade,
referências da conclusão e paginação (página vazia, início/fim, loading isolado).
`ChatSessionLiveState.test.tsx` e `MessageNode.navigation.test.tsx` exercitam
atualização seletiva, terminal e interação com a fonte canônica. Testes de
anúncio, arbitragem, admissão e fila continuam na camada operacional.

Implementação e validação local deste recorte concluídas; CI e review remoto
completam o gate de entrega. Não representa conclusão da fase 3.

Evidências adicionais deste recorte:

- `TestExecutionWriterChronologySurvivesTerminalAndPersistenceFailure` verifica
  texto → ferramenta → ferramenta → texto em retry/continuação, terminais sem
  delta, erro, cancelamento, limite e falha de persistência.
- `TestExecutionWriterSecondContinuationKeepsExecutionChain` cobre duas
  continuações consecutivas preservando fontes, reasoning e ferramentas.
- `TestExecutionWriterCancellationPreservesRunningToolChronology` cancela uma
  ferramenta realmente em execução, com e sem falha de gravação.
- `TestTurnIntermediateTransfersContentAtomically` mantém também a atividade
  da conversa na mesma transação, sem avançá-la quando a transferência falha.
- `ChatSessionView.messaging.test.tsx` recusa continuar conteúdo volátil,
  perfil desabilitado ou seleção substituída antes do envio; o caso persistido
  envia a pergunta raiz correta. Ter representante persistido não basta para
  promover bytes voláteis a base de continuação.
- `chatTimelineWails.test.ts` exercita exclusão com o hook real ainda montado,
  isolamento de outra conversa e descarte de Open tardio. O coordenador da
  sessão invalida a apresentação; o store operacional não recebe essa tarefa.

Início/fim continuam sendo comandos de navegação mesmo quando a página já está
na borda: alteram explicitamente o modo histórico/followTail. Flags de presença
de página anterior/posterior não substituem essa decisão do backend.

O antigo cenário de edição de resposta assistant após continuação não possui
caminho de produto: a edição aceita mensagem user raiz. A prova de recusa de
edição assistant permanece; esta migração não cria uma nova modalidade de edição.

Validação local: 6.960 testes frontend passaram em 525 arquivos (18 pulados
pela seleção sem ACP); 144 testes selecionados de agent, 485 de chat e 365 de
banco passaram. A matriz de nove falhas do ledger e o teste E2E do envio pelo
modal passaram. Build, vet, TypeScript, ESLint e lint Go incremental contra
origin/main passaram; Stylelint conserva 2.123 avisos preexistentes, sem erros.
Testes ACP locais e NVDA não foram executados por orientação do mantenedor.

Revisão independente local: `review_aep` e `review_store_coverage`, sem pendências.
Atividade transacional, exclusão da apresentação e guardas do editor foram
corrigidas e revalidadas. O fechamento da política do ledger teve três rodadas
com `review_aep`: corrigiu propagação em retries/skills/MCP nativo e eventos de
início do grupo bloqueado, com aprovação na última rodada. A revisão de cobertura
confirmou a transferência das provas para o backend e a preservação da matriz
frontend; não houve eliminação de regressões de produto para obter teste verde.
CI e review remotos permanecem como gates da publicação.

Primeira revisão remota do PR #1023: três apontamentos do Copilot corrigidos.
A falha ao confirmar ferramenta preserva a intermediária e seu vínculo antes
do terminal; a regressão SQLite demonstrou ferramenta → texto antes da correção
e texto → ferramenta após reabertura com a correção. Comando com fonte explícita
também exige seleção vigente durante Begin/Take; erros técnicos da consulta do
editor ficam no log e a UI usa tradução. Revalidação: 144 testes selecionados
de agent e 112 frontend focados passaram; `review_aep` aprovou o ajuste local.
O inventário de logs foi atualizado de 711 para 708 formatos legados após
conferir a remoção de três formatos do loop, preservando a proibição de Printf.

Segunda rodada remota (#1023, Copilot e CodeRabbit): o vínculo do editor
invalida leitura, confirmação e callbacks tardios; continuação usa o perfil da
superfície e reenvio exige fonte ainda publicada. Diagnósticos operacionais
permanecem disponíveis sem criar mensagens nem autorizar novo envio. Cargas de
metadados são invalidadas por exclusão/reset; abas auxiliares podem tentar de
novo após falha, pelo mesmo caminho assíncrono das abas de chat.

Limpeza confirmada libera as reservas de recuperação pela RPC existente antes
de fechar threads, relê as raízes por Refresh e passa todas a followTail por
Navigate. Assim a janela histórica vazia recebe as próximas mensagens. Não
simula dados nem apaga reservas locais sem confirmação remota. Falha de liberação
mantém a reserva e impede o fechamento; outra conversa não participa da operação.

Falha de SaveIntermediate encerra com persistence_error e preserva o parcial
volátil, sem avançar o modelo. MCP simples também registra eventos nativos;
um erro no lote não impede tentar registrar as demais ações já concluídas.
A persistência desses fatos usa contexto de finalização limitado a três segundos,
preservando identidade e sobrevivendo ao cancelamento do stream. As regressões
SQLite cobrem terminal normal, erro e cancelamento; nenhuma suíte ACP local foi
executada. Revisores independentes `review_aep` e `review_store_coverage`
identificaram os casos de cancelamento e recuperação, corrigidos nesta rodada.
Reset confirmado invalida todas as janelas e leituras canônicas pendentes. A
compensação do vínculo de chat contextual também usa a exclusão coordenada,
fora dos locks de criação, e publica o evento de exclusão. Testes verificam
que a conversa preservada não é excluída e que Open tardio não ressuscita dados.
Validação local consolidada: 6.978 testes frontend (526 arquivos, 18 casos
preexistentes ignorados), 300 testes focados e 146 testes seguros de agent
passaram, além de build/vet/lint. Os nove testes de comandos contextuais passaram e verificam a notificação e a
passagem pelo coordenador na compensação. O E2E de envio pelo modal também passou.


Terceira rodada Copilot (#1023): a limpeza confirmada também autoriza descartar
as versões voláteis presentes no recebimento do evento, após obter snapshot
válido. Não tenta reservar fontes já removidas. Uma publicação nova durante a
operação continua protegida, e Refresh comum mantém o gate de transferência.
Falha da consulta conserva o visual anterior; regressões cobrem ambos os resultados.
A revisão independente também restringiu liberações às reservas capturadas na
confirmação, preservando transferências posteriores. Noventa testes do cliente
e transporte passaram. O E2E de limpar expôs uma fixture que exigia revisão
exata em Refresh; foi alinhada a `acceptsSnapshotRevision` do backend (intervalo
entre último snapshot e revisão atual), mantendo Navigate/SetRetention estritos.
Revalidação: nove E2Es de toolbar aprovados (um skip preexistente), TypeScript
e lint aprovados; `review_store_coverage` aprovou o cliente e a fixture.


### Decisão: identidade de apresentação após confirmação (2026-11-05)

Aprovada pelo mantenedor para o cutover. Esta decisão substitui a troca de IDs
na confirmação descrita nos recortes de composição e segmentos tipados abaixo.
Persistir os bytes troca sua fonte e durabilidade; não troca a identidade nem
os limites dos segmentos já publicados da mesma execução/geração. Seal com os
mesmos bytes preserva também a posição dos cortes de reasoning. Conteúdo terminal
diferente, retry e reset de transporte são substituições explícitas, com nova
identidade para os segmentos substituídos.

O backend conserva a associação comprovada pelo writer entre mensagem persistida
e faixas publicadas. A associação contém IDs, offsets e hashes de verificação,
nunca uma segunda cópia de texto ou detalhes de ferramenta. Continuações só herdam
associações das fontes explicitamente selecionadas pelo backend; retry não herda
a tentativa anterior. Edição persistida incompatível com os bytes confirmados
não pode ser sobrescrita por um buffer antigo.

A associação permanece disponível para Refresh depois da coleta do estado live
se o item continua materializado por alguma janela. Sua vida pertence à projeção
e à encarnação da conversa. Itens fora de uso podem liberar essas associações;
recriar a projeção não exige reproduzir identidades de uma interface já desmontada.
O orçamento adicional de metadados é 8 MiB por conversa: não expulsar associação
em uso para admitir outra; exceder o limite exige recuperação explícita.

### Recorte: consumidor visual canônico e identidade dos segmentos (2026-11-05)

A lista principal, threads paginadas, streaming, leitura, edição e comandos de
mensagem consomem a mesma fonte canônica por superfície. O backend fornece data,
origem, contagem canônica de filhos, alvo persistido das ações e referências dos
segmentos que compõem a conclusão. O cliente concatena somente essas referências;
não escolhe a última mensagem nem remove sobreposições por texto.

A identidade visual permanece distinta do representante persistido. Comandos,
áudio e envio ao editor preservam os guards de owner, sessão, seleção e revisão.
As interações retêm o item de thread e seus ancestrais nas respectivas janelas.
O ownership das inscrições filhas independe da página visível; fechar a mãe fecha
as descendentes, mesmo após paginar. Uma falha transitória de Open pode ser
recuperada sem desmontar a aba.

SetRetention retorna um recibo produzido sob o lock após o commit. O próximo
comando usa esse checkpoint da mesma época caso o lote de eventos ainda esteja
em trânsito; isso não altera o estado visual nem introduz retry automático.
Refresh normal preserva posição, modo e retenção. Escolher explicitamente as
mensagens mais recentes exige liberar as interações retidas e restabelece
followTail na raiz. Falhas conservam o conteúdo de recuperação; liberar uma
reserva exige decisão explícita. Avisos passam pelo announcer global.

Evidências: `presentation_segments_test.go` (confirmação, terminal, coleta,
edição, encarnação, rollback e conclusão completa),
`timeline_retention_receipt_test.go` (checkpoint e revogação durante commit),
`ChatMessage.test.tsx` (mesmo link e foco durante parcial → confirmação → terminal
→ novo snapshot), `chatTimelinePresentation.test.ts` (ordem, ownership, retenção,
reabertura e Refresh), `chatTimelineClient.test.ts` (recibo antes do evento).
As provas de comandos/edição mantêm recusa por ABA, owner, sessão e mudanças
concorrentes; suas fixtures injetam a apresentação e não restauram leitura de
buffers legados no produto.

Validação local deste recorte: 7.000 testes frontend passaram (522 arquivos;
18 testes pulados pela seleção sem ACP), mais 11 testes focados da apresentação.
Build, vet, TypeScript, ESLint e Stylelint passaram; Stylelint conserva avisos
preexistentes. O lint Go completo aponta 12 ocorrências em arquivos não alterados;
o lint incremental contra origin/main passou com zero ocorrências. A seleção
segura de chat, os 23 testes selecionados de banco e os 21 testes de timeline
Wails passaram. Bindings regenerados pelo Wails, sem edição manual.
Revisor independente `review_aep`: três rodadas de revisão com correções e
rechecagem final; gate aprovado sem pendências. O último apontamento sobre array
nullable foi retirado pelo revisor após conferir o produtor e as provas JSON.
CI e review remotos completam a validação da entrega.

Revisão do consumidor: a fase ativa de raciocínio vem explicitamente do projetor,
não da existência de texto acumulado. A razão terminal da tentativa corrente é
lida na mesma transação e aparece como diagnóstico categórico traduzido junto à
resposta parcial, inclusive após reabrir. Retenção/paginação não alteram o loading
da geração; conclusão não rouba foco da leitura. Paginação por teclado transfere
o foco à borda correspondente; scroll não faz essa transferência. A entrega ao
editor pelo modal libera a fonte visual somente após a transição autorizada,
conservando o ticket backend e as validações de sessão e destino. Edição de thread
é recusada antes de abrir o editor. O E2E usa o contrato canônico no backend
simulado; mantém os testes de comando, teclado, leitura e ferramentas.

Validação das correções de review: 7.013 testes frontend passaram em 523 arquivos
(18 pulados na seleção sem ACP); 479 testes seguros de chat e 12 de banco/startup
passaram. E2E: 239 cenários passaram na rodada completa; os dois casos de foco
restantes passaram na reexecução dos 11 cenários de landmarks/criação de aba,
após aguardar a conclusão assíncrona do F6. Seis casos permanecem pulados pela
suíte existente. TypeScript, ESLint, build/vet e lint Go incremental aprovados.
`review_aep` revisou as correções e confirmou o gate sem pendências, incluindo
compatibilidade de startup anterior à v44 e paginação concorrente com streaming.

Revisão adicional: copiar texto/Markdown de resposta terminal sem representante
persistido usa o mesmo ticket de comando, com capability da janela e identidade
exata da publicação (época, revisão, execução). Backend revalida no handoff;
cancelamento do comando interrompe a consulta. Não há reserva de recuperação,
texto fornecido como prova pelo cliente nem liberação de ações destrutivas.
Contagens legadas continuam refletindo filhos persistidos; contagens canônicas
incluem respostas admitidas e consultam turnos por identidade, a partir dos pais
selecionados, independentemente de ANALYZE. A configuração de voz captura a fonte
canônica mesmo quando o botão entrega uma cópia para renderização.

Validação adicional: 7.017 testes frontend passaram em 523 arquivos (18 pulados
na seleção sem ACP); 23 testes de timeline Wails e 17 testes de comandos App
passaram. A seleção ampla de 482 testes seguros de chat encontrou três
comparações de metadados; corrigido o produtor, as três provas e mais duas de
contagem/Refresh passaram sem alterar suas expectativas. A cópia também cobre
descarte entre Prepare e handoff e cancelamento durante autenticação/consulta.
Build/vet, TypeScript, ESLint e lint Go incremental passaram; bindings regenerados.
`review_aep` fez três rodadas adicionais e uma rechecagem dos ajustes de
compatibilidade, aprovando o gate sem pendências antes do push consolidado.

Na segunda revisão remota, o fim do raciocínio com bytes idênticos preserva
cortes, confirmação e versão de conteúdo, alterando apenas a fase ativa. A
composição protege todos os IDs do snapshot candidato até o fim da publicação;
o orçamento inclui esses itens e uma falha restaura o mapa anterior de
descritores. Regressões atravessam o handler real, duas respostas confirmadas,
coleta live, Refresh e recusa de orçamento sem perda de identidades anteriores.
Salto recusado não desloca foco nem deixa rejeição assíncrona sem tratamento;
nós filhos usam a mesma regra de chave da lista e paginação tem rótulo nos três
idiomas. Nenhum teste ACP é executado para validar esses percursos.

Permanece uma limitação anterior ao cutover: threads ligadas a uma fonte
intermediária não representante de resposta consolidada não têm acesso próprio
na UI. A conclusão desta parte exige metadados backend com alvos reais por fonte
e acesso distinguível a cada escopo; não somar contagens ao representante nem
reatribuir filhos. O teste de identidade agora também atravessa CommitContent,
terminal, coleta live e Refresh real da inscrição.

AEP permanece **In Progress**. O controlador operacional de envio e anúncios
continua contendo reducers visuais antigos, que deixaram de ser fonte da lista e
das ações. Sua retirada, os gates restantes de continuidade/retry/ferramentas e
a margem final de desempenho permanecem pendentes; este PR não declara a fase 3
nem o AEP integralmente concluídos. Testes locais ACP e NVDA não são executados
por orientação do mantenedor.

### Recorte: DOM estável e retenção visual da mensagem focada (2026-11-04)

MessageList usa uma única estrutura de linha nos modos curto e virtualizado.
Cruzar o limite de 40 itens altera posicionamento e faixa materializada, sem
trocar o wrapper do item. O range inclui a identidade focada, inclusive quando
o foco está em um editor, controle de ferramenta ou mensagem filha. Prepend
recalcula seu índice pela identidade; não mantém uma posição antiga como dono.
Retém apenas as linhas com foco ou interações abertas (edição, leitura, menu e diálogos
de ferramentas), sem copiar conteúdo nem acrescentar itens ausentes da janela
recebida. Cada interação adquire sua retenção explicitamente antes da abertura
e libera após restaurar o foco; sair do item não invalida o portal que ele abriu.
Fechamento, substituição e desmontagem liberam as respectivas retenções.

A atualização automática deixa de rolar para o fim enquanto houver foco em uma
mensagem ou interação pertencente a ela. Saída intencional libera a retenção
por foco. Foi removido o mecanismo que
interpretava focusout sem destino como desmontagem e restaurava pelo índice:
não há mais restauração por adivinhação nem frame tardio desse mecanismo capaz
de focar outra mensagem. A navegação explícita por teclado permanece disponível.

Evidências: MessageList.focusRetention.test.tsx cobre 40→41→40 com foco real no
item/editor/controle filho, conteúdo e seleção do editor, prepend com conclusão,
liberação ao sair para o compositor, limite de DOM e exclusão sem transferência
de foco. O teste antigo de focusout fabricado foi substituído por uma prova de
produto com elemento realmente focado e prepend. As provas de desempenho de
100/500/1.000 itens continuam exigindo materialização limitada e ausência de
renders adicionais em nós estáveis.

MessageList.interactions.test.tsx usa menu e diálogo de ferramenta reais para
provar abertura, atualização fora da faixa, retorno ao controle original e
liberação ao sair. MessageNode.test.tsx verifica inert dos novos irmãos na
transição 41→40 durante leitura. O isolamento acompanha materializações e
restaura apenas atributos adquiridos por sua instância. As linhas usam top,
sem transform ou contain:layout, preservando o backdrop fixo na viewport.
O salto explícito ao início/fim materializa e foca o índice de destino após
carregar a janela; não depende do último nó já montado nem da rolagem automática.
MessageList.test.tsx prova Ctrl+End com 120 itens e foco retido fora do destino.
ChatSessionView.test.tsx usa a lista real para provar carregamento, anúncio e
foco nos dois limites. O observer de leitura restaura e descarta referências
de irmãos desconectados a cada mutação, sem acumular subárvores virtualizadas.
A edição adquire retenção no commit antes do autofocus, cobrindo o timer do
menu que termina antes do frame do editor. Um snapshot DOM anterior à mutação
preserva o deslocamento na viewport da mesma identidade durante medição e
troca de modo. As regressões cobrem timer→frame e alturas alternadas em 40→41→40.
Na paginação, a restauração pela diferença de alturas só atua quando a âncora
não compensou aquele commit; retenção sem snapshot válido não basta para
suprimi-la. A prova de prepend dispara o carregamento por scroll com o item
visível e alturas totais diferentes, inclusive trocando o foco no mesmo commit.

Limites: a superfície ainda recebe o modelo legado e sua chave existente.
A prova it.fails de identidade de turno recebida tardiamente permanece: este
recorte não declara o cutover nem resolve a mudança de identidade na fonte.
O cutover deve migrar conjuntamente timestamps, threads, alvos de comandos,
referências leves de mídia, recuperação explícita e remoção dos overlays legados.
Nenhum teste ACP local nem teste NVDA é necessário para esta entrega.

### Recorte: Refresh com transferência explícita de recuperação (2026-11-04)

O cliente integra reserva, prova do DTO, promoção e leitura/liberação posterior
em um único ciclo de Refresh. Antes dos RPCs de snapshot, tenta reservar os itens
protegidos pela identidade exata publicada e confronta ReadRecoveryItem com todos
os campos do DTO anterior. Não reconstrói segmentos, não compara prefixos e não
mistura conteúdo de outra execução na timeline atual. A comparação ordena apenas
chaves de objetos JSON; conserva a ordem dos arrays e o limite de 512 KiB.

A projeção passa a publicar `executionId` explicitamente, vindo de
ChatTurn.CurrentExecutionID no persistido e da execução viva na composição. Ele
permanece após conclusão persistida, quando recoveryReference pode desaparecer.
Legado/importação sem identidade explícita não inventam uma execução. Troca de
execução gera upsert canônico, inclusive com o mesmo itemId e o mesmo conteúdo.

Um candidato preserva o item protegido somente na mesma época, com execução
explícita igual e revisão suficiente. Tombstone da mesma época e revisão
suficiente continua sendo remoção explícita. Outra época, outra execução ou
identidade ausente exigem reserva e prova; itemId coincidente não basta. Falha da
reserva não bloqueia um snapshot que comprova continuidade da mesma execução.
Execução ativa, fonte ausente e cotas excedidas continuam sem autorização de perda.
Aquisição percorre capabilities elegíveis do mesmo escopo, respeita reservas já
existentes por referência e tenta outra janela quando uma aquisição é recusada,
inclusive pela cota compartilhada com buffers. Não substitui uma reserva anterior
pela mesma referência em outra época/revisão. Tentativas ambíguas ficam registradas
até sua liberação; a promoção exige também o cleanup dessas tentativas.
Somente reservas verificadas entram no limite preventivo do cliente; recusas
não consomem a cota local nem impedem a tentativa de outro item. O backend segue
autoritativo sobre o limite real, inclusive buffers e aquisições ambíguas.

Depois dos snapshots, reservas não necessárias são liberadas. Antes da promoção,
o cliente revalida candidato e capabilities sem await e registra a transferência
junto da troca do coordenador. Close durante aquisição/leitura/snapshot invalida
a prova; falha parcial conserva a apresentação anterior. Reservas de tentativas
malsucedidas são liberadas; cleanup falho conserva a identidade e exige
cleanupRecoveryItems explícito antes de outro Refresh.
Se eventos mudarem quais reservas são necessárias durante uma liberação, o cliente
recalcula o cleanup após cada liberação até uma passagem não precisar aguardar RPC,
e só então promove. Reserva que passar a ser necessária durante outra liberação
é preservada; a regressão cobre retry recebido durante essa espera.

O cliente mantém somente descritores transferidos, separados da ordem e do índice
normal da timeline. getRecoveredItems lista esses descritores; readRecoveredItem
lê o DTO na capability proprietária; releaseRecoveredItem libera explicitamente
a cópia de recuperação, sem apagar execução persistida. Leituras tardias após
liberação/logout são recusadas. O descritor inclui windowId e não pode operar uma
reserva adquirida após reabrir a mesma superfície. Cleanup revalida exclusão de
Refresh a cada janela e nunca sobrescreve uma operação pendente. Close normal é recusado antes de alterar a
superfície enquanto houver reserva transferida; logout/dispose encerra a sessão
e suas capabilities. Falha de Release mantém a reserva acessível para retry.

Evidências: chatTimelineClient.test.ts cobre o ciclo completo, retry persistido,
mudança de época, recusa de aquisição com continuidade comprovada, divergência de
texto/ferramentas/metadados/identidade, cleanup, Close concorrente e logout,
distribuição entre janelas com cotas compartilhadas, conflito de referência,
descritores após reabertura e cleanup concorrente entre superfícies.
chatTimelineWails.test.ts percorre os DTOs dos RPCs até Refresh/leitura/liberação.
response_projection_test.go e timeline_recovery_reference_test.go provam a
identidade explícita no persistido, no live, no terminal e no retry. Bindings
regenerados; nenhuma edição manual dos modelos gerados.
timeline_snapshot_test.go também comprova que, logo após Begin(retry) e antes
de EnsureResponse, a leitura transacional contém a nova execução sem representante,
mensagens ou segmentos da anterior. A projeção consome fontes já filtradas pela
seleção do banco; continuação pode herdar bases explicitamente vinculadas.

Pendente: ativar o consumidor na UI, apresentar/confirmar liberação do conteúdo
recuperado por teclado/leitor de telas, remover o fluxo legado e medir desempenho.
Reservas continuam em memória e vinculadas à capability; não sobrevivem ao processo.
Execuções mutáveis e conteúdo não reservável conservam a apresentação congelada.
Este recorte fecha o ciclo do cliente, mas não declara os defeitos visuais de foco
resolvidos antes do cutover. Nenhum teste ACP local nem teste NVDA foi executado.

### Recorte: reserva do item canônico publicado (2026-11-04)

`ReserveRecoveryItem` captura o `CanonicalItem` exato já publicado, por
referência completa de execução, itemId, época e revisão do item. Exige execução
finalizada, publicação válida e ausência de estágio provisório. Não reconstrói
fontes no banco, não escolhe revisão mais nova e não interpreta IDs de segmentos.
Cache ausente, coletado ou divergente recusa a aquisição; ausência nunca prova
que o frontend pode liberar conteúdo protegido.

A identidade recuperada pode pertencer à encarnação anterior do armazenamento,
inclusive na primeira aquisição após importação, desde que aquele DTO exato ainda
esteja publicado no cache. A consulta de lifecycle autoriza a conversa atual;
não transforma a encarnação atual em identidade dos bytes antigos. Referência
da encarnação nova não seleciona o DTO antigo. Se a reconciliação já removeu o
DTO do cache ou mudou sua época, a aquisição é recusada. Essa reserva não afirma
que o item representa o armazenamento atual nem o reinsere na nova timeline.

A reserva guarda uma cópia JSON independente do DTO wire, incluindo segmentos,
resumos de ferramentas e metadados. “Item completo” significa todos os campos de
`CanonicalItem`; não inclui blobs de mídia nem argumentos/resultados completos
referenciados pelas ferramentas. A revisão identifica o DTO observado, não uma
prova de que ele contém todos os bytes finais da execução.

`ReadRecoveryItem` devolve esse DTO após revalidar capability, sessão e existência/
ownership da conversa, mesmo após evicção do cache, importação ou mudança de época.
Cada leitura cria contêineres independentes. Aquisição repetida da mesma identidade
é idempotente; identidade diferente não substitui a reserva. Uma revisão diferente
exige liberar explicitamente a anterior antes de adquirir outra. `ReleaseRecoveryItem`
libera somente a identidade exata e é idempotente. Close, exclusão da conversa e
descarte explícito da execução encerram as reservas. Uma resposta perdida exige
repetir aquisição/liberação ou fechar a capability; não implica rollback remoto.

Reservas de buffers e de itens compartilham o máximo de oito entradas e 8 MiB por
janela. A conta soma capacidade dos builders e tamanho JSON reservado; não promete
um limite exato do heap total. Cada DTO reservado tem no máximo 512 KiB; itens
maiores são recusados antes da cópia pelo tamanho já medido na publicação, com
verificação adicional do JSON efetivamente capturado. Nenhum scan, hash ou nova
cópia é feito por delta: o custo ocorre somente na recuperação explícita.

Os três RPCs Wails usam a mesma barreira cancelável e autorização das consultas
existentes. Os bindings são regenerados. ReconcileRecovery reconhece uma reserva
de item como retenção quando a fonte original já não existe. Nesse caso, retained
não promete buffers para ReadRecoveryPage: a leitura é ReadRecoveryItem.

Evidências: timeline_recovery_item_test.go cobre identidade, cotas combinadas,
cache ausente/provisório, imutabilidade, coleta/troca de época, sessão, duas
capabilities e descarte. chat_timeline_recovery_item_test.go percorre snapshot
real → reserva → Refresh → leitura; a matriz de cancelamento da reconciliação
inclui os novos RPCs, com Close/shutdown e espera por writer.

Pendente: comparação e transferência do DTO protegido no cliente, integração no
Refresh, execuções mutáveis, itens acima da cota, cutover visual e métricas finais.
Este recorte não libera cache frontend nem reinsere itens na janela. A reserva
pertence à capability e não sobrevive ao último Close ou ao processo.

### Recorte: leitor explícito de recuperação no cliente Wails (2026-11-04)

`ChatTimelineClient.openRecovery` captura a capability da abertura atual e a
referência emitida pelo backend. Lê uma primeira página, valida sua identidade
e reserva exatamente a versão recebida. Não consulta ReconcileRecovery antes,
não repete aquisição automaticamente e não reconstrói IDs de segmentos.
Execução ainda mutável ou versão alterada retorna o erro do backend.

O handle oferece `next()` serializado e `close()` idempotente. Conserva somente
a primeira página até sua entrega e metadados limitados; não acumula texto.
Fim da leitura não libera a reserva. `handle.close()` libera apenas a reserva;
Close da superfície invalida a capability inteira e todas as suas reservas.
Há no máximo um leitor por abertura, evitando que dois consumidores compartilhem
uma reserva backend e um deles a libere enquanto o outro ainda depende dela.

Cada página valida conversa, referência, versão, ordem estrutural, geração,
offsets em bytes UTF-8 e cursores sem repetição. As cotas são 128 chunks e 64 KiB
de texto por página, cursor de até 16 KiB, 8 MiB de texto total e no máximo 256
páginas. O conjunto de cursores tem, portanto, tamanho limitado; páginas vazias
com cursor novo são válidas e não indicam fim. Strings com surrogates isolados,
identidades divergentes, ausência ou lacunas não são convertidas em conteúdo.
Essas verificações não constituem prova de preservação do item visual inteiro.

Após tentativa de aquisição, até uma Promise rejeitada exige cleanup da mesma
referência/versão: a resposta pode ter sido perdida depois da reserva. Falha de
Release é exposta junto do erro original e conserva o leitor fechado na abertura,
bloqueando outro leitor até `closeRecovery` repetir a liberação ou Close encerrar
a capability. Fechar durante Reserve aguarda o RPC antes de enviar Release;
fechar a superfície cancela pelo Close remoto antes de aguardar consultas.
Resposta tardia não reativa o leitor nem alcança outra abertura ou sessão.
Rejeição tardia de Release só deixa de ser erro de cleanup após Close remoto
confirmado. Invalidação local ou Close malsucedido não bastam; o erro original
de leitura/aquisição continua sendo propagado.

O adaptador Wails usa os DTOs gerados dos três RPCs. O leitor é acessível durante
erro da timeline para recuperação explícita; abertura de leitor e consultas da
superfície compartilham a guarda de operação pendente. Refresh pode continuar
usando a mesma capability depois da aquisição, sem modificar a reserva.

Evidências: timelineRecoveryReader.test.ts cobre páginas, cotas, UTF-8, offsets,
ausência, aquisição ambígua, recibo adulterado, falha/retry de Release e Close
durante RPCs. chatTimelineClient.test.ts cobre isolamento entre janelas,
cleanup pendente, respostas tardias/reabertura e recusa de Refresh que perderia
item protegido mesmo após leitura reservada. chatTimelineWails.test.ts cobre
DTOs e invalidação na troca de sessão.

Pendente: prova e transferência do item protegido completo, integração dessa
decisão no Refresh, execuções mutáveis, cutover visual e métricas finais.
As guardas atuais de apresentação continuam intactas; este recorte não ativa UI.

### Recorte: reserva explícita de buffers finalizados (2026-11-04)

`ChatTimeline.ReserveRecovery` recebe capability, referência completa e versão
dos buffers obtida na leitura. Só admite execução finalizada com a versão exata.
Repete a autorização e leitura leve do lifecycle antes da aquisição; execução
ativa, versão obsoleta, ausência, erro ou excesso de cota não criam reserva.
Repetir a mesma aquisição é idempotente, inclusive após a coleta do turno original.
Resposta perdida não implica aquisição desfeita: repetir o pedido ou fechar a
capability resolve o resultado ambíguo, sem novas reservas.

A reserva pertence à janela e compartilha somente builders, com descritores limitados de geração, partes e
versão imutáveis. Não copia texto nem retém IDs de mensagens, publicationBase ou o restante do turno.
ReadRecoveryPage consulta essa referência exata mesmo após coleta ou importação,
revalidando sessão e existência/ownership da conversa em cada página. A reserva
atravessa Refresh, Navigate e mudança de época, enquanto a capability continuar
válida. ReconcileRecovery informa retained quando só a reserva mantém os bytes.

Por janela, no máximo oito referências e 8 MiB de capacidade dos builders,
contados conservadoramente por reserva mesmo se outra janela compartilha os mesmos
buffers. A aquisição visita no máximo 4.096 iterações/partes e soma Cap() sem
percorrer texto; cotas excedidas recusam a aquisição. Esses limites cobrem a
retenção adicional, não o heap total nem os buffers originais do projetor.
Contêineres e índice também têm percurso limitado. Não há contador novo por delta.

`ReleaseRecovery` é idempotente e exige a versão da reserva existente; libera
apenas sua referência, sem descartar a fonte original. Close/invalidação da
capability e exclusão da conversa encerram suas reservas. Descarte explícito por
DiscardRetained prevalece e invalida as reservas correspondentes de todas as
janelas, inclusive se a fonte original já foi coletada. Exclusão e recriação
com o mesmo ID não reativam a capability antiga. Falha de descarte/autorização
não libera reservas. As operações de reserva não se intercalam com os checkpoints
de publicação: compartilham a barreira da conversa e não participam de staging.

A resposta é uma garantia de retenção em memória da capability, não persistência
durável, sobrevivência ao processo ou transferência automática do cache visual.
Ainda é necessário confrontar os bytes protegidos com os reservados e manter
a capability durante essa proteção. Reservar não autoriza descartar um item
inteiro, que pode conter ferramentas, áudio ou outros dados fora destes buffers.
Execuções mutáveis continuam recusadas; última janela fechada encerra a garantia.
Este recorte não altera as guardas nem ativa a UI.

Evidências: timeline_recovery_reservation_test.go cobre coleta real do turno,
idempotência, cotas, mutação terminal recusada, versão, falhas, duas capabilities,
Release, Close e exclusão/recriação. A integração de importação percorre
export/import reais, Refresh e descarte explícito com e sem reserva.
Os testes Wails cobrem aquisição terminal, recusa ativa/sessão e cancelamento
de ReserveRecovery por Close/shutdown ou espera por writer. Bindings regenerados.

Pendente: integração da prova de bytes/ownership na recuperação frontend,
tratamento das execuções ainda mutáveis, cutover visual e métricas finais.

### Recorte: páginas limitadas dos buffers de recuperação (2026-11-04)

`ChatTimeline.ReadRecoveryPage` recebe a capability existente, a referência exata
e um cursor opaco opcional. Compartilha com ReconcileRecovery a autorização,
barreira cancelável e leitura transacional leve do lifecycle. Não materializa
mensagens persistidas ou ledger. Close/shutdown cancelam a consulta, inclusive
enquanto ela espera por um writer; cada página revalida a sessão.

Cada execução mantém uma versão dos buffers. StartStream e mutações aceitas de
texto/reasoning, Initial, Cut e Seal avançam essa versão. Confirmação no storage
e término sem alteração dos buffers não a avançam. O cursor inclui conversa,
referência completa, versão e posição estrutural; não é uma capability. Mudança
entre páginas retorna erro de cursor desatualizado e exige reiniciar. A leitura
não fixa snapshot nem mantém outra cópia integral: streaming contínuo pode exigir
aguardar estabilização. Retry conserva a versão da execução anterior retida.

Cada página copia no máximo 64 KiB de texto UTF-8 e visita no máximo 128 buffers,
inclusive vazios. Um índice ordenado de iterações, sem conteúdo duplicado, evita
ordenar todos os streams na consulta. Páginas vazias podem ter próximo cursor:
o consumidor deve seguir até cursor vazio. Fatias retornadas são copiadas para
não manter o backing buffer inteiro. Offsets são bytes UTF-8, não índices UTF-16;
nenhuma rune é dividida. Texto inválido ou ainda incompleto é recusado sem
substituição silenciosa; o consumidor pode reiniciar após novos bytes. Identidade
total, incluindo conversa, limitada a 1.024 bytes e cursor a 16 KiB, com envelope
JSON escapado abaixo de 512 KiB. O limite de percurso também vale para milhares
de partes vazias; nenhum hash, snapshot integral ou scan de conteúdo confirmado
é calculado na leitura.

`found=false` distingue referência ausente de execução existente sem texto.
Erros de autorização, storage, cursor ou cancelamento não viram ausência. A versão
identifica somente os bytes observados, não comprova retenção futura, persistência
ou transferência de responsabilidade. O frontend ainda não consome o RPC nem
libera cache protegido com esse resultado.

Evidências: `timeline_recovery_page_test.go` cobre reconstrução, cotas, UTF-8,
fatias independentes, vazio versus ausência, mutadores, retry, importação,
confirmação/terminal, cursor adulterado, autorização e cancelamento;
`chat_timeline_recovery_page_test.go` cobre snapshot → referência → páginas,
capability e sessão; os testes Wails de cancelamento da reconciliação também
exercitam o novo RPC em Close/shutdown e espera pela barreira de um writer.
Bindings regenerados pelo Wails.

Pendente: prova de preservação/transferência dos bytes, integração da recuperação
no frontend, cutover visual e critérios globais de desempenho.

### Recorte: consulta autorizada da referência de recuperação (2026-11-04)

`ChatTimeline.ReconcileRecovery` recebe uma capability de janela existente e a
referência completa emitida pelo backend. Reutiliza a barreira da conversa e
`ReadTurnLifecycles`: uma leitura transacional autorizada e limitada a um turno,
sem materializar mensagens ou ledger. Revalida a sessão após a leitura. Erros de
autorização, storage ou cancelamento não são convertidos em ausência.

O resultado contém conversa, referência e estado observado: `active` para execução
ativa compatível com o lifecycle atual; `retained` para execução não confirmada
conservada em memória, inclusive após retry/importação; `persisted` somente para
terminal compatível, sem volatilidade ou conteúdo não confirmado e com versão de
commit exatamente igual à versão lida; `unknown` quando não há prova suficiente.
Comparar sempre projeção, encarnação original, turno e execução. Recriação da
projeção ou mudança posterior de storage pode produzir `unknown` conservador.

A consulta não altera época, journal, inscrição ou retenção. Não exige que a época
da janela ainda seja vigente, pois serve à recuperação. Não é chamada por delta.
A referência identifica uma execução, não uma versão imutável dos seus bytes:
nenhum destes estados é recibo de transferência ou autorização para liberar o
cache protegido. O frontend ainda não consome esse RPC nem relaxa suas guardas.

Evidências: `timeline_reconciliation_test.go` cobre identidade, retry, importação,
pergunta removida inclusive após commit terminal, confirmação parcial, versão posterior e recusas de sessão,
storage, cancelamento e orçamento; `chat_timeline_reconciliation_test.go` cobre o
binding por capability, resposta ativa recebida a partir do snapshot, troca de sessão e cancelamento de consulta em espera por
Close/shutdown, sem cancelar o contexto pai. Consultas compartilham o cancelamento
da inscrição com Refresh/Navigate, sem adquirir o mutex de emissão.
Bindings regenerados pelo Wails.

Complemento da revisão remota: a barreira da conversa agora aceita contexto,
sem polling nem goroutine por waiter. Writers continuam usando a mesma exclusão;
capabilities compartilham a aquisição cancelável e reautorizam após adquiri-la.
Cancelar a espera devolve a reserva e agenda o cleanup pelo timer único existente,
sem readquirir a barreira no caminho de erro. `projection_mutex_test.go` prova
retorno antes do writer liberar, reservas equilibradas, token preservado e coleta
da última reserva abandonada. A integração Wails cobre Close durante essa espera;
o fechamento ainda drena writers em curso, mas a consulta cancelada já retorna.

Medição local de `BenchmarkLiveProjectionAppend` (Windows, três amostras de 200 ms,
1.000 deltas por operação): mediana de 79,8 µs/85.512 B/43 alocações antes para
112,8 µs/85.624 B/44 alocações depois. O custo adicional é por projeção e pela
exclusão dos deltas; não há consulta SQL por token. A medição não encerra os
critérios globais de desempenho do AEP.

Pendente: leitura limitada e prova de preservação/transferência dos bytes, sua
integração com a recuperação frontend e posterior cutover visual.

### Recorte: identidade de recuperação por execução (2026-11-03)

O item canônico ativo ou volátil carrega `recoveryReference`, emitida pelo backend:
`projectionId`, `incarnation`, `turnId` e `executionId`. A identidade da projeção
é criada por um único construtor, inclusive nas operações em lote; permanece
estável através de mudanças de época e muda quando a projeção é recriada.
A encarnação é a original da execução, mesmo após importação. Retry produz outra
referência e preserva a identidade da tentativa anterior retida no backend.

Snapshots RPC e eventos carregam o mesmo contrato. O frontend valida a referência
e a conserva sem interpretar IDs de segmentos ou reconstruir a execução.
A referência desaparece do item após conclusão confirmada e sem conteúdo volátil.
Ela não é autorização, comprovante de persistência nem confirmação de ownership;
sua ausência não permite descartar conteúdo protegido. As guardas atuais de
retenção e recuperação continuam vigentes.

Evidências: `timeline_recovery_reference_test.go` cobre estabilidade entre chunks,
retry, retenção, mudança de época, recriação, conclusão e criação em lote;
`timelineBatch.test.ts` cobre preservação, troca por retry e recusa atômica de
referências incompletas ou de outro turno. Bindings regenerados pelo Wails.

Pendente: reconciliação autorizada da referência exata, leitura limitada do
conteúdo de recuperação, integração visual e cutover. Este recorte não acrescenta
RPC de confirmação, descarte automático nem consumidor visual.

### Recorte: cliente Wails da timeline por sessão (2026-11-03)

`chatTimelineClient.ts` conecta o coordenador a um listener único, registrado
antes de Open, e às capabilities de cada superfície. O orçamento de cache é
compartilhado por sessão. Navigate e SetRetention usam checkpoints comprovados
pelo backend: a revisão aplicada pelo coordenador ou o recibo de uma retenção
confirmada na mesma época, caso seu evento ainda esteja em trânsito. O recibo
não modifica conteúdo, ordem ou metadados visuais; somente o lote atômico ou
um snapshot aceito os modifica. Consultas simultâneas na mesma superfície são
recusadas como busy.
Não há consulta por delta nem reconstrução semântica adicional no cliente.

Refresh recupera a conversa em um coordenador candidato, com tokens de todas as
superfícies registrados antes das consultas. Só promove o candidato após aceitar
todos os snapshots ainda necessários e seus buffers. Falha parcial conserva o
visual anterior congelado e guarda a base efetivamente retornada para cada
capability. A tentativa seguinte usa essas bases; nova época só entra por Refresh.
Uma posição explícita por superfície permite recuperar âncora histórica perdida,
sem fallback automático. Operações pendentes impedem iniciar a recuperação.

Close remove a superfície imediatamente e chama o backend antes de aguardar
consultas pendentes, permitindo cancelá-las. Um Open tardio é fechado mesmo após
dispose. Falhas de fechamento são reportadas sem reativar o conteúdo local.
`chatTimelineWails.ts` normaliza os DTOs RPC nullable e oferece um cliente lazy por
userId/sessionId; mudança de identidade ou estado desautenticado descarta o cliente.
Importar o módulo não inicia listeners nem inscrições de timeline.

Evidências: `chatTimelineClient.test.ts` cobre evento anterior à Promise, bases
após recuperação parcial, cancelamento, época mista, retenção após patch, lacuna
no buffer, âncora perdida e cota de capabilities aposentadas.
`chatTimelineWails.test.ts` cobre normalização e troca de sessão com Open tardio.

Complemento da revisão remota: capabilities fechadas são aposentadas inclusive
quando Open retorna depois do fechamento ou Refresh é cancelado antes do snapshot.
Durante Refresh, só entram eventos das capabilities ainda ativas. A decisão de
promover consulta os erros atuais do coordenador: fechar uma superfície elimina
seu erro local, mas nunca uma falha global de época/overflow.
O getter também verifica identidade diretamente, sem depender da ordem dos
assinantes do authStore. Liberação explícita de retenção vazia é permitida durante
falha usando época/revisão comprovadas por snapshot ou patch aplicado; não limpa
o fault. CAS stale continua sendo erro do backend, sem retry nem avanço local de
revisão. Se a revisão necessária não for conhecida e a posição não puder ser
recuperada, Close e nova abertura continuam sendo escolhas explícitas.

Close remoto falho mantém o slot fechado e a capability para retry explícito;
não restaura conteúdo. Sem resposta de Open, o fechamento visual é imediato,
mas sua Promise de cleanup aguarda descobrir a capability: o protocolo não tem
cancelamento de Open por request ID. Não introduzir timeout que abandone cleanup.
Coleta oportunista em Open/Close descarta coordenadores sem superfícies,
recuperação ou itens em cache; evicções podem deixar coordenadores vazios até a
próxima coleta. Conteúdo ativo/volatile e a LRU continuam preservados.
Correlação de abertura: o evento Wails agora ecoa `surfaceId` recebido em Open.
O cliente gera um UUID de inscrição novo por abertura, separado da identidade
lógica da superfície e do item/mensagem. Antes da Promise, conversa + inscrição
já permitem rotear; depois dela, windowId também precisa coincidir. Eventos sem
essa correlação não têm fallback. Fechar invalida a inscrição imediatamente,
inclusive após promoção de coordenador e nova abertura, sem copiar nem ampliar
índices de IDs encerrados. O índice limitado do coordenador puro permanece para
seus outros chamadores, mas não é a autoridade de roteamento do transporte.

Erros locais e globais do coordenador são consultados diretamente, sem espelho
no cliente; falha de recuperação por RPC é separada porque pode ter mudado bases
no backend. Ao fechar a última superfície, cache em erro pode ser descartado se
não houver itens ativos/volatile; cache saudável continua na LRU. Conteúdo
protegido não é descartado para liberar um erro: esse caso continua exigindo
ownership/transferência explícita no cutover. Refresh sem superfície ativa não
pode promover um candidato vazio para contornar essa proteção. Antes de toda
promoção, itens protegidos anteriores precisam estar representados no candidato
por revisão suficiente (ou remoção comprovada); caso contrário, a apresentação
anterior é preservada e a transferência de ownership é exigida. Isso também
cobre fechamento durante Refresh, com ou sem outra superfície aberta.
O candidato mantém referências transitórias apenas aos IDs protegidos da origem:
provas de remoção/persistência não são coletadas antes da decisão de ownership.
Essas referências são liberadas antes da promoção, com nova verificação do estado,
ou no dispose do candidato recusado; ausência de item nunca vale como exclusão.
Este recorte ainda não ativa consumidores de UI. Faltam conexão das superfícies
e retenções de foco/leitura/edição, cutover e remoção do fluxo legado, anexos e
medições finais. Replay RPC não é usado neste cliente: recuperação explícita usa
Refresh; a reaplicação local do buffer já pertence ao coordenador. A otimização
por Replay permanece não entregue. Não se afirma corrigida a perda visual de
foco antes do cutover.
### Recorte: coordenador frontend de snapshots, eventos e cache (2026-11-03)

`timelineConversationState.ts` mantém um índice de itens por conversa/época e
janelas por superfície, aplicando o núcleo transacional existente. Cada consulta
recebe um token por identidade de objeto: substituição, fechamento e dispose
invalidam retornos antigos antes de hidratar o índice. Snapshot substitui somente
a janela correspondente, sem rebaixar revisões já recebidas por outra janela.

Eventos recebidos enquanto Open/Refresh/Navigate estão pendentes ficam em buffer
limitado por conversa a 256 lotes/2 MiB. Após aceitar o snapshot, o coordenador
reaplica os lotes contíguos da mesma base e ignora envelopes da base substituída.
Lacuna ou revisão incompatível preserva o conteúdo e solicita recuperação.
Troca de época e overflow congelam a instância: o adaptador deverá construir uma
substituta com snapshots aceitos antes de liberar a apresentação anterior. Não
há RPC, retry automático ou consulta por delta dentro desse coordenador.

`TimelineCacheBudget` é compartilhado entre coordenadores da mesma sessão e
limita a LRU de itens dispensáveis a 240 itens/8 MiB. Janelas e IDs retidos pelo
backend são referências; itens preparing/streaming ou volatile não entram na
LRU. Fechar libera referências e dispose remove os registros da instância.
O adaptador deverá criar um orçamento por sessão e descartar os coordenadores no
logout; compartilhar o orçamento entre usuários não é permitido.

O piso de descarte considera as sequências aplicadas de todas as janelas e as
consultas pendentes. Snapshots anteriores ao piso são recusados mesmo com token
atual. Tombstones não viram conteúdo vivo; um corpo anterior pode permanecer
somente para apresentação da janela atrasada até seu patch, fechamento ou novo
snapshot. Se consultas atrasadas impedirem liberar mais de 240 registros ou
8 MiB sem referências, a instância congela e solicita recuperação, sem apagar o
texto visível nem continuar acumulando deltas. Não há TTL para exclusões.
Essa guarda de registros acima do piso é por conversa; o limite compartilhado
por sessão se aplica somente à LRU dispensável, não é um teto global de heap.
Lotes vazios também liberam registros quando o piso alcança a próxima exclusão;
não é preciso percorrer todo o índice a cada delta de texto.

Evidência: `timelineConversationState.test.ts` cobre eventos antes da Promise,
consultas substituídas, fechamento/reabertura, paginação concorrente, remoção com
janela atrasada, piso após GC, troca de época, overflow, LRU entre conversas,
conteúdo ativo/volatile e descarte no logout; os testes do núcleo continuam ativos.

Recorte preparatório ainda sem listener/RPC ou consumidor de UI ativo. Próximos:
adaptador Wails com listener antes de Open, normalização dos DTOs nullable,
Refresh como recuperação de nova época, fechamento das capabilities e conexão
das superfícies/retenções de foco, leitura e edição.
Referências externas de foco/edição ainda não têm API própria neste coordenador;
a proteção atual usa janelas, retainedItemIds e estados ativos/volatile.
Medidas de heap/renders e cutover com remoção do fluxo antigo continuam pendentes; não se afirma correção
visual de foco nesta entrega.

Complemento da revisão remota: fechar uma capability remove imediatamente seus
envelopes e registra seu windowId para rejeitar entregas tardias mesmo com outro
Open pendente. Retornos descartados de Open também aposentam a capability recém
descoberta. O mesmo descarte ocorre quando um Open atual é recusado por snapshot
inválido (por exemplo, item removido), sem afetar outra abertura pendente.
Refresh/Navigate recusados ou obsoletos não aposentam a inscrição ativa.
Esse índice de IDs encerrados não usa TTL e tem limite de 256 IDs/2 MiB por
instância; exceder a cota congela o coordenador sem apagar o visual. O adaptador
deverá fechar no backend a capability retornada por um Open descartado, inclusive
após dispose/congelamento; ignorar o snapshot local não substitui esse cleanup.
Falhar Refresh/Navigate também libera seus envelopes conhecidos, mesmo havendo
Open em outra superfície; a poda preserva apenas consultas ainda pendentes e
envelopes desconhecidos necessários às aberturas, sem ocupar cota com lotes órfãos.

### Recorte: aplicação transacional de lotes no frontend (2026-11-03)

`frontend/src/services/timelineBatch.ts` prepara um lote canônico sem alterar a
janela aceita nem o cache fornecido. O resultado contém somente os itens alterados
(incluindo tombstones) e a próxima janela; o futuro proprietário deve publicar
ambos na mesma transação síncrona antes de preparar outro lote. O cache consultado
pertence exclusivamente à mesma conversa/época. Não há ordenação ou reconstrução
semântica no frontend.

A sequência pertence a cada capability, pois duas janelas recebem subconjuntos
diferentes do mesmo lote. Revisões uint64 usam BigInt, hidratação antiga não rebaixa
itens e várias operações de segmentos na mesma revisão são aplicadas juntas.
Duplicatas não repetem texto; lacunas, troca de época e lote inválido devolvem
necessidade de recuperação sem publicar alterações parciais. Splices preservam
a ordem do backend; delta de texto conserva a referência da lista de IDs.

Evidência: `frontend/src/services/timelineBatch.test.ts` cobre duas janelas nas
duas ordens de entrega, múltiplos segmentos, atomicidade, sequência acima de 2^53,
patches, hidratação, tombstones e conclusão com o mesmo itemId.
Complementos da revisão remota provam rejeição atômica de upsert/hidratação
com escopo divergente mesmo sem splices, e preservação do estado concluído
quando um segmento é substituído em lote posterior.

Este é um recorte preparatório da fase 3, ainda sem consumidor ativo. Não afirma
corrigir foco/DOM na tela atual. Permanecem necessários: admissão de snapshots e
snapshotRevision, listener antes de Open, buffer limitado e replay, lifecycle de
sessão/capability, índice compartilhado com descarte seguro de tombstones e cotas,
e migração das superfícies com remoção do fluxo substituído. A ausência em uma
página não autoriza remoção global; o helper não decide quando descartar o cache.
Os limites de memória e a recuperação concorrente serão implementados no dono do
estado antes da ativação nas telas. Não serão mantidos dois reducers autoritativos
ativos em paralelo.

### Recorte: transporte de eventos Wails (2026-11-03)

Cada capability aberta mantém um leitor de `TimelineSubscription.Next` e emite
`chat:timeline` fora da barreira do core. O transporte usa o mesmo mapper público
de Replay; não mantém outra fila de conteúdo. O payload inclui conversationId,
windowId, epoch e snapshotRevision. Sua interface TypeScript é manual, pois um
payload exclusivo de eventos não pertence aos bindings gerados pelo Wails.

Refresh/Navigate e emissão compartilham uma exclusão por handle. Somente um
snapshot aceito substitui a base; entrega extraída antes dessa troca é descartada.
A Promise do RPC e o evento continuam sem ordem garantida no frontend: o futuro
consumidor deve registrar o listener antes de Open, bufferizar durante snapshots
e descartar capabilities encerradas. Close, logout e shutdown cancelam e drenam
os workers fora dos locks de autenticação e registry. Falha terminal do transporte
libera capability e cota; a recuperação exige nova abertura autenticada.

Revisão remota do CodeRabbit: Refresh/Navigate recebem contexto derivado do RPC
e ligado ao cancelamento do stream. Fechar a inscrição cancela a consulta em
andamento antes de aguardar a exclusão; shutdown não depende de cancelar depois
o contexto global do App. Prova: `TestTimelineEventStreamCloseCancelsSnapshotQuery`.
Os complementos de cobertura da revisão verificam remoção da capability/cota
quando o contexto da autorização termina e preservação de itens/metadados no wire.
Revisão posterior do Copilot: o gate de registro de Open também recusa contexto
de eventos já cancelado durante a leitura inicial, liberando reserva e inscrição.
`TestChatTimelinePendingOpenRejectsExpiredEventAuthorization` reproduz essa
corrida sem alterar a geração local; a abertura não devolve uma capability morta.

Decisão aprovada pelo mantenedor em 2026-11-03: revogação externa admite prazo
máximo de **1 segundo** para bloquear novas emissões. Uma lease compartilhada por
principal e geração revalida sessão/conta; sua validade é o menor limite entre
início da consulta + 1 segundo e expiração persistida. Consulta lenta não estende
esse prazo. A admissão consulta relógio/contexto/geração, sem SQL por delta, e
bloqueia enquanto uma autorização vencida aguarda renovação. Erro de validação
encerra a lease e seus streams. Cada RPC continua revalidando a persistência.
O último emissor libera o worker compartilhado; aberturas pendentes também
possuem uma referência e a liberam se forem recusadas.

Troca local invalida a geração imediatamente. O prazo refere-se à **admissão**
de emissões: eventos já admitidos e enfileirados no runtime não podem ser
recolhidos. O consumidor deve ignorar eventos de handles encerrados, inclusive
depois de logout. Não há consulta persistida nem callback Wails sob o lock do
projetor, e a espera por revalidação não bloqueia a publicação do core.

Provas focadas: `TestChatTimelineEventsMatchReplayAndReleaseFailedTransport`,
`TestTimelineEventStreamDiscardsReplacedSnapshot`,
`TestTimelineEventStreamCloseDrainsEmitterAndReleasesFeed`,
`TestChatTimelineEventAuthorizationSharedUntilLastClose`,
`TestChatTimelineExternalRevocationStopsIdleEventStreams` e testes
`TestTimelineAuthorization*`. Validação local: 210 testes seguros de Wails,
53 de autenticação e 1.180 do App passaram; os testes adicionados depois da
seleção ampla também passaram na rodada focada. Frontend: 5.887 testes em 484
arquivos, TypeScript, ESLint e Stylelint aprovados (avisos preexistentes de CSS,
sem erros). Build/vet e lint Go incremental passaram. ACP excluído das execuções
locais; sem NVDA. Revisor independente `review_aep`: sete rodadas técnicas,
incluindo lifecycle, autorização, integração, diff completo e correções remotas; gate documental
final aprovado, sem pendências.
CI/review remotos completam a entrega. Cutover frontend, anexos e gate global de desempenho
continuam pendentes; esta entrega ainda não altera a apresentação nem comprova
a correção visual do foco.

### Recorte: entrega incremental interna das inscrições (2026-11-03)

TimelineSubscription.Next oferece ao futuro adaptador de eventos os lotes
confirmados do mesmo journal usado por Replay, com o mesmo roteamento por janela.
Não cria fila de conteúdo, polling, goroutine por inscrição nem consulta SQL por
delta. Um sinal coalescido apenas acorda o leitor; o conteúdo é extraído sob a
barreira, copiado e devolvido para processamento fora dela. Existe um leitor por
capability; chamadas concorrentes são recusadas.

O cursor pertence ao estado da janela e acompanha o checkpoint de publicação.
Commit sinaliza inclusive overflow e remoção por revogação; rollback não entrega
operações provisórias. Mudança real de época, exclusão e Close acordam leitores.
Lacuna no journal (256 lotes/2 MiB), erro de publicação ou janela invalidada produz
SyncRequired uma vez e suspende a entrega até um novo snapshot. Open, Refresh e
Navigate posicionam o cursor na mesma barreira do snapshot; Navigate continua
exigindo época vigente, sem substituir Refresh na recuperação de encarnação.

A extração não é confirmação de recebimento: Replay permanece disponível para
falha do transporte. SnapshotRevision distingue uma entrega em trânsito de um
snapshot posterior, mesmo na mesma sequência. A futura borda deve reautorizar a
emissão e coordenar respostas de snapshot com entrega, descartando bases antigas.
Next valida o principal capturado e as invalidações coordenadas; não afirma
revalidar por delta a posse ou revogação persistida externa ainda não observada.
Replay/Refresh conservam suas verificações de storage. Nenhum callback externo
executa sob a barreira da conversa.

Provas em timeline_delivery_test.go: paridade com Replay, cópia independente,
lotes vazios, commit/rollback, overflow por quantidade/bytes/estágio, uma única
indicação de recuperação, cancelamento sem consumo, leitor exclusivo, despertar
por publicação/Close/exclusão/revogação, retomada por Refresh e duas superfícies
com splices e revisões sucessivas. A maioria usa fixture sem banco; os testes de
Refresh e participação usam SQLite e os escritores coordenados existentes.

Este recorte ainda não conecta Next ao Wails nem altera a UI. Eventos Wails,
consumidor frontend, anexos e gate global de desempenho continuam pendentes.
Validação: 438 testes seguros de chat, incluindo oito cenários top-level novos, e 5.887 testes de frontend em 484 arquivos passaram. O teste de espera/concorrência passou em 20 repetições. Build, vet, TypeScript, ESLint, Stylelint e lint Go incremental aprovados. ACP excluído da execução local; sem NVDA. Revisor independente review_aep: duas rodadas técnicas sem pendências; gate documental final aprovado, sem pendências. CI e review remotos completam a entrega deste recorte.

### Recorte: inscrições da timeline na borda Wails (2026-11-03)

ChatTimeline expõe Open, Refresh, Navigate, SetRetention, Replay e Close, usando
o mesmo LiveProjection e DBTimelineReader do runtime. A registry conserva apenas
capabilities e seu principal: não reconstrói itens, ordem ou estado de janelas.
DTOs de pedidos, replay e patches públicos vivem em apidto; índices internos do
journal não atravessam a borda. Lotes vazios continuam avançando a sequência.
Sequências de replay exigem representação decimal canônica de uint64.

A sessão é capturada sob authMu, com userID e SessionID coerentes, e revalidada
nas operações do core por geração atômica, sem readquirir authMu. Nenhum owner ou sessionId vem do payload. Troca de usuário
ou sessão invalida a geração da registry e libera inscrições; refresh da mesma
sessão as preserva. Aberturas pendentes contam no limite global de 256 handles e
reservas, além do limite por conversa do core. Uma abertura invalidada não pode
registrar nem devolver sua inscrição depois de logout/shutdown. Shutdown é
terminal; attach posterior não reabre a registry.

O detach troca a registry sob lock, mas fecha capabilities depois de liberar
authMu e o mutex da registry. Captura, autorização e chamadas ao projetor também
ocorrem fora do mutex da registry, preservando a ordem dos locks.

Cada RPC revalida sessão e conta persistidas com RevalidateLocalSession antes de
entrar no projetor. A consulta ocorre fora de authMu e da barreira, com prazo
limitado; um comprovante privado no contexto vincula a captura ao principal e à
geração efetivamente revalidados. Troca de sessão no intervalo recusa a captura.
Os callbacks do projetor consultam somente a geração atômica, pois a publicação
pode ocorrer sob um guard que já detém authMu.RLock. Isso evita reentrada quando
há um escritor aguardando. A prova de revogação persistida acontece na entrada
do RPC; não é uma transação conjunta entre autorização e snapshot.
Provas focadas cobrem os seis RPCs, replay/ordem, lotes vazios, sequências acima
de 2^53 e inválidas, sessão alheia, limite com reservas, abertura concorrente com
logout/shutdown, descarte de Refresh invalidado durante a leitura e ausência de ressurreição após retorno à sessão anterior. As reservas permanecem contabilizadas até a abertura terminar, mesmo após invalidação da geração.
Esta entrega não emite eventos nem troca o consumidor visual: transporte por
eventos, cutover e gate global de desempenho continuam pendentes.

Validação inicial: 202 testes seguros de wailsapi (incluindo sete focados nos RPCs), 1.170 testes seguros do App (incluindo dois de sessão), quatro
de apidto, 430 testes seguros de chat e 5.887 testes de frontend em 484 arquivos
passaram. Execução local exclui ACP. Build, vet, TypeScript, ESLint e Stylelint
aprovados; lint incremental sem achados. O lint completo aponta 12 achados em
arquivos fora deste diff, preservados para tratamento separado. Bindings gerados
com Wails 2.15.0 em perfil isolado. Revisor independente review_aep: seis rodadas de implementação;
a primeira apontou liberação precoce da cota de aberturas pendentes no detach,
corrigida e coberta por 256 aberturas concorrentes; segunda rodada sem pendências. A terceira confirmou documentação e integração da base remota sem alterar a árvore revisada.
A revisão remota apontou reentrada de authMu sob guard de comando e ausência de
revalidação de sessão persistida. Ambos corrigidos; 15 testes focados de timeline,
autenticação e sessão do editor passaram após a alteração. A regressão de deadlock
falhou com a reentrada reintroduzida e passou na versão final. A quarta revisão
independente aprovou o diff completo, incluindo comprovante privado e geração.
A quinta e a sexta revisões confirmaram os ajustes de lint no ramo de falha do teste, sem mudança de produção; os 15 testes passaram novamente.
CI e nova rodada remota completam a entrega deste recorte.
### Recorte: contrato compartilhado da timeline (2026-11-03)

Os DTOs leves de itens, segmentos, operações, metadados, splices, snapshot e
pedidos de navegação/retenção passam a ter definição única em internal/apidto,
conforme AEP-0088 D5. O domínio usa aliases; não há cópia de structs nem mapeamento
por campo para cruzar a borda. Lotes e patches do journal continuam internos,
pois contêm índices de roteamento que não pertencem ao contrato público.

Snapshots incluem conversationId e scopeParentId mesmo quando não têm itens.
O escopo retornado não compartilha ponteiro com o pedido nem com a inscrição.
Campos JSON usam lowerCamel; navegação mantém posição flat. IDs são strings
opacas e sequence/windowRevision/revision continuam strings decimais, inclusive
acima de 2^53. O resumo de ferramenta mantém seus campos internos fora do JSON.

Provas: internal/apidto/timeline_test.go verifica serialização, precisão e
proveniência; internal/chat/timeline_dto_test.go verifica raiz/thread vazias e
imutabilidade do escopo guardado diante de mutação do pedido e da resposta.
Bindings são regenerados pelo Wails, incluindo a referência ao resumo de ferramenta
em apidto. Este recorte ainda não expõe novos métodos Wails nem muda o consumidor
visual. RPCs, eventos, cutover e gate global de desempenho permanecem pendentes.

Validação local: 429 testes seguros de chat, quatro de apidto e 5.887 testes de
frontend em 484 arquivos passaram; 31 arquivos relacionados a ACP ficaram fora
da execução local. Build, vet, lint incremental, ESLint e Stylelint aprovados.
Bindings regenerados com Wails 2.15.0 em perfil isolado sob build/bin; TypeScript
aprovado sobre o resultado. Revisor independente review_aep: cinco rodadas, sem
pendências locais. CI e review remotos acompanham o PR empilhado.

### Recorte: execução isolada do pacote de chat no CI (2026-11-03)

O job geral com detector de corrida atingiu o limite acumulado de 20 minutos
no run 41600939964, com o teste corrente ativo havia dois segundos e o stack
na preparação SQLite da fixture. Isso comprova o limite da suíte acumulada;
não identifica uma asserção falhando nem demonstra deadlock no teste corrente.

O pacote internal/chat passa a rodar em dois grupos com runners próprios. O grupo geral
exclui exatamente internal/app e internal/chat, preservando subpacotes e novos
pacotes descobertos por go list. A descoberta Go atribui cada teste top-level,
exemplo e fuzz target a um grupo pelo checksum estável do nome; subtestes seguem
juntos. Nomes novos não exigem manutenção de listas nem redistribuem os existentes.
Nenhum teste ou cenário é descartado do chat;
-race, -short, -count=1 e -timeout=20m permanecem. A saída detalhada identifica
os testes em andamento. O agregador backend-race continua exigindo todos os grupos.

As anotações dos jobs 125845109686 e 125845662987 registraram timeout de 25 minutos,
sem disponibilizar o log nativo. A etapa recebe limite de 23 minutos; o job usa
45 minutos para acomodar também preparação e publicação do diagnóstico em caso
de falha. O upload tem limite próprio de dois minutos.
A saída é preservada em artefato por sete dias; pipefail conserva o erro do Go.
O timeout de 20 minutos dos testes não muda. Não se presume a causa do atraso
nem se reduz cobertura para contornar a ausência de diagnóstico.

O teste do script comprova destino único de App/Chat/subpacotes, flags, falhas
de execução, cobertura exata sem sobreposição dos grupos de chat e igualdade da
matriz com --groups. A fixture de snapshot usa logger
SQL parametrizado em nível Error: preserva erros de banco e evita os avisos de lentidão esperados nas cargas sintéticas.
Dados, asserções e timeouts da suíte permanecem iguais. A alteração não modifica
logger de produção nem declara concluído o gate de desempenho do produto.

A seleção local do grupo 0 revelou três linhas de 3 MiB no teste de âncora
excessiva: Scan do GORM 1.31.1 usa um recorder interno e repassa SQL já expandido,
contornando a parametrização do logger. A fixture passa a registrar erros, sem
avisos de SLOW SQL dessas cargas deliberadamente grandes. Dados, asserções e
limites continuam iguais. Essa redução de saída é comprovável localmente, mas
não estabelece sozinha a causa dos jobs remotos sem log; o CI precisa confirmar.
No mesmo teste de âncora, a saída caiu de 9.451.325 para 5.399 bytes. Os 211 testes seguros do grupo 0 desta base e os 429 testes seguros de chat passaram, além de vet e lint incremental.

O encerramento das consultas de preparação, também diagnosticado no run
41606700204, foi incorporado ao PR-base conforme a seção seguinte.

Validação local: bash -n, teste completo do script, 429 testes seguros de chat,
31 testes seguros de usecases,
build, vet e lint incremental aprovados. Sem ACP local. Revisor independente
review_aep: dez rodadas (incluindo integração da base, redução do log da fixture e coerência documental), sem pendências. YAML e propagação de falha pelo tee
validados localmente. No run 41652048015 (commit 8a54813be),
o grupo 0 concluiu os testes Go em 439,808 s e o log nativo ficou disponível. Essa rodada
confirma a execução desse grupo com a saída reduzida, não a conclusão de todo o CI.
O restante da matriz precisa concluir no run vinculado ao PR. Review remota sem achados de
código; a descrição histórica do logger foi alinhada ao nível Error.

### Correção de validação: encerramento da preparação (2026-11-03)

O run 41606500056 falhou ao limpar o banco de
TestSendMessageUseCase_ReturnsErrorWhenNoLLMProviders: PrepareContext cancelava,
mas não aguardava a consulta concorrente da conversa ao retornar por falta de
provedor. Agora cancela e aguarda as leituras antes do retorno, mantendo
paralelismo e precedência do erro. A regressão segura a consulta após o
cancelamento e exige seu encerramento antes de aceitar o retorno.

Essa correção acompanha o PR-base de retenção para não depender de um PR filho
para validar o próprio CI. A distribuição do race permanece no recorte separado.
Validação: 428 testes seguros de chat passaram nesta base; os mesmos arquivos da
correção já passaram por 31 testes de usecases, build, vet e lint na pilha.

### Recorte: retenção por foco/leitura na inscrição (2026-11-03)

SetRetention substitui os IDs retidos com época/revisão vigentes e somente IDs
visíveis. Não altera pinned persistido. Referências e retentionBlocked pertencem
à inscrição e chegam no mesmo lote de conteúdo e splices. Falha reverte janela,
referências, cache e journal. O flush provisório não consome timer, dirty nem
base de turno finalizado: o commit faz esse consumo; rollback conserva o
callback original e o delta pendente, sem mecanismo paralelo de restauração.

FollowTail preserva fronteiras históricas quando a cauda expulsaria um retido
sobrevivente. Exclusão/saída do escopo vence retenção; ausência na cauda não prova
exclusão. Refresh e atualização automática compartilham a seleção e recuperam a
cauda quando ela contém todos os sobreviventes na mesma versão. Fronteira perdida,
intervalo excedente ou retido fora dele exige sincronização local, sem corte.

Na janela historical, uma troca de retenção na mesma versão reutiliza IDs,
fronteiras e contagem já comprovados. Não reclassifica a conversa para confirmar
metadados. Autorização, lifecycle e seleção versionada do conteúdo continuam no
percurso comum; mudança de versão exige nova seleção histórica.

Navigate exige liberação prévia e época vigente: não recupera outra encarnação.
Refresh com posição explícita exige liberação, salvo nova encarnação comprovada
antes de efeitos. Refresh é o único caminho de recuperação da inscrição.
Falha auxiliar na mesma encarnação conserva retenção;
liberação em base invalidada só retira referências e mantém SyncRequired até
snapshot. Nova encarnação não herda referências; Close libera as da inscrição.
Snapshots copiam os IDs.

Refresh/Navigate guardam decisões de validade da janela anterior, na mesma versão,
antes dos efeitos e separadas dos registros completos do snapshot. A atualização
comum comprova os turnos de todas as janelas antes de qualquer corte, em storage
e lifecycle. Pergunta isolada também prova execução/ausência. Retry/exclusão
externa desvincula o stream antigo; execução ativa continua válida fora da página.
Não existe leitura específica duplicada em SetRetention.

A prova de lifecycle inclui também os IDs conservados por janelas syncRequired:
essas janelas não publicam conteúdo, mas ainda referenciam execuções em memória.
Uma transição coordenada de outro item deve desvincular a tentativa substituída
por retry externo sem reativar a janela nem descartar seus buffers. A regressão
timeline_sync_lifecycle_test.go reproduziu a aceitação indevida antes da correção.
O percurso parcelado e versionado é o mesmo; não há consulta por token.

ReadTurnLifecycles seleciona apenas user_message_id, active_execution_id e
current_execution_id, na mesma transação autorizada da versão. Não lê referências
de resposta, mensagens ou ledger. A cota SQL considera os campos lidos, antes da
materialização. O wrapper acumula somente decisões booleanas por turno. Conteúdo
e lifecycle usam visitVersionedSelections, com subdivisão por quantidade/bytes
e mesma versão em cada parcela; registro individual excessivo continua recusado.
Não há cota indevida somando janelas válidas nem SQL por token.

Visitantes respeitam cancelamento/prazo antes e depois da leitura. WithoutCancel
fica na fronteira posterior ao writer, que também precisa reconciliar commits
parciais. Refresh/Navigate cancelados antes da aplicação da prova não alteram
janela, journal ou lifecycle. A leitura completa também verifica cancelamento antes de
aceitar o snapshot e iniciar efeitos, inclusive sem turnos live. Cancelamento
posterior à aceitação pode concorrer com o commit. SetRetention conserva o contexto em todo o percurso
e descarta o estágio cancelado antes do commit. A escrita já confirmada continua
publicada mesmo após cancelamento do pedido.

Evidências em timeline_window_retention_test.go, timeline_window_lifecycle_test.go
e canonical_transaction_test.go: duas superfícies, liberação parcial/total,
replay, exclusão, fronteira perdida, item movido, rollback, pedidos obsoletos,
encarnação, pergunta isolada, retry/exclusão externos, execução fora da página,
corte por ambos os produtores, 80 turnos, três referências de execução de 3 MiB,
registro individual excessivo e falha/drift na segunda parcela. Cancelamento após
versão, durante seleção e prazo menor preservam retenção/janela/journal. Rollback
após flush conserva o callback e entrega o delta no replay, inclusive no terminal
volátil. Reuso histórico prova ausência de releitura só na mesma versão.

Validação: 429 testes seguros de chat, 39 focados e as regressões de snapshot cancelado e janela syncRequired, 33 de banco; 99 de controllers,
53 de handlers/turnos do agente e duas regressões de áudio nas rodadas pertinentes.
Build/vet e lint incremental aprovados. Revisor independente review_aep: vinte e cinco
rodadas, sem pendências locais. Sem ACP local ou NVDA. A fixture pesada compartilha
preparação entre os mesmos cenários e silencia SQL volumoso, mantendo dados e
assertivas. O logger da fixture de snapshot usa nível Error, sem avisos de SLOW SQL.
A parametrização não impede que Scan do GORM repasse SQL expandido em erros; não
há garantia de supressão de payloads nesse caso. A evolução do logger é descrita
no recorte de CI acima.
O perfil local de SetRetention caiu de 2,72 s para 0,32 s após seleção
pontual de âncoras e reuso histórico; a medição não substitui o gate final do produto.
Transporte, cutover e margem global de desempenho permanecem pendentes; este
recorte não declara resolução do foco na UI. CI/review remotos são acompanhados.

### Recorte: acompanhamento explícito da cauda (2026-11-03)

A inscrição passa a declarar modo historical ou followTail no snapshot. Subscribe
continua histórico por padrão. Navigate com Mode=followTail é o comando explícito
para acompanhar a cauda; aceita somente fim/before, sem cursor ou item-âncora,
com limite de 1 a 240. Época e revisão vigentes protegem a troca contra pedidos
atrasados. Navegação comum e Refresh com posição explícita voltam ao histórico;
Refresh sem posição conserva modo e consulta.

Em transições persistidas, followTail seleciona a cauda atual do mesmo escopo e
versão; publica crescimento/corte e cursores novos pelo estágio atômico existente.
A janela histórica mantém suas fronteiras. Exclusão de fronteira e cauda vazia
não interrompem o acompanhamento. Não há consulta por token nem outro pipeline.

Evidências em timeline_window_mode_test.go: duas superfícies com corte/entrada,
replay sem reload, fechamento independente, recuperação mantendo modo, paginação
interrompendo acompanhamento, fronteira excluída, vazio→mensagem, pedidos inválidos
e obsoletos, identidade e ordem da pergunta/resposta durante streaming.

Contrato ainda interno. Retenção por foco/leitura, transporte e consumidor
frontend permanecem pendentes; esta entrega não corrige sozinha o foco da UI.
As cotas por janela/journal e a pendência de benchmark final permanecem vigentes.
Validação: 399 testes seguros de chat aprovados; build/vet e lint incremental
sem erros. Revisor independente review_aep: quatro rodadas, UUID da fixture
corrigido e gate final sem pendências. Review remota do CodeRabbit incorporada:
os testes de vazio e nova pergunta/resposta aplicam os splices recebidos e
comprovam a ordem final; seis regressões focadas, vet e lint passaram. Sem ACP local ou NVDA. CI completo e
review remota são acompanhados no PR antes de iniciar o próximo recorte.

### Recorte: participação histórica e publicação atômica (2026-11-02)

Transições persistidas do lifecycle e escritores coordenados atualizam as janelas
históricas sob a barreira da conversa. A inscrição concentra consulta, IDs ordenados,
metadados, revisão e baseline do último snapshot; o capability não guarda outra
consulta mutável. A seleção preserva as fronteiras aceitas: inclusões externas
mudam contagem/índices e inclusões internas geram splices calculados no backend.
Fronteira removida/movida, thread excluída ou intervalo acima da capacidade emite
SyncRequired somente para aquela janela, preservando seus IDs até Refresh.
Sessão revogada deixa de participar; import substitutivo sempre invalida a época.

Cada escrita produz um único lote lógico com alterações de conteúdo, patches,
materialização de itens que entraram e metadados finais. A entrada de item já
cacheado não inventa revisão de conteúdo. O journal registra o roteamento de
cada lote; replay não filtra operações antigas pela participação atual. Navegação
e recuperação estabelecem um baseline: operações anteriores não são reaplicadas,
e a revisão anterior à última Navigate/Refresh permanece recusada. Refresh pode
usar uma revisão conhecida desde esse baseline, permitindo recuperar uma lacuna
que ocultou revisões automáticas sem sobrescrever navegação/recuperação posterior.

O estágio de publicação respeita 2 MiB. Overflow avança a sequência e deixa lacuna
explícita, nunca um lote parcial. Erro de leitura/composição/cota reverte cache,
journal e todas as janelas antes de liberar a barreira; não desfaz o commit do
writer nem os buffers recebidos. A cota de 8 MiB é validada sobre cada janela
resultante inteira, inclusive itens antigos, evictados e conteúdo live.

Para comprovar que conteúdo e participação pertencem à mesma versão, este recorte
reconcilia os IDs das janelas abertas em parcelas limitadas nas transições
persistidas. Não lê a conversa inteira nem consulta SQL por token/timer. O estágio
retém somente tamanhos por item para a validação agregada, sobrevivendo à evicção
do cache. Essa reconciliação adiciona custo por transição: o gate de desempenho
continua pendente; não se declara ganho ou aprovação do benchmark nesta entrega.

Evidências em timeline_window_membership_test.go: duas superfícies, metadados sem
seguir cauda, entrada/edição/saída antes do replay, baseline sem append duplicado,
entrada já cacheada, transação com inserção e edição em lote único, falha na última
janela, drift entre seleção e materialização, conteúdo externo anterior à escrita,
revogação, cota agregada com item evictado/crescimento sem splice, overflow e
recuperação com revisão automática perdida. Testes de navegação mantêm as recusas
de escopo/cursor e passam a consumir a revisão de SyncRequired quando necessária.

O contrato permanece interno. FollowTail, retenção de foco/leitura, transporte,
cutover frontend, migração de anexos e evidências finais continuam pendentes.
A primeira review remota apontou cotas indevidas entre janelas: entradas de
superfícies distintas eram lidas juntas e payload compartilhado era cobrado
novamente contra a cota de fontes. A materialização agora ocorre no percurso
parcelado único dos participantes; o estágio preenche as entradas e contabiliza
repetições apenas contra o journal. Duas janelas de 5 MiB ou três superfícies
com a mesma entrada de 3 MiB confirmam participação e oferecem snapshot após
lacuna, sem suspender a conversa. A cota final de 8 MiB permanece por janela.

A seleção de identidades respeita a cota por superfície, sem somar IDs repetidos
entre janelas. Regressões cobrem seleções independentes de 3 MiB, recusa de uma
seleção acima de 8 MiB e rollback do lifecycle após CommitResponse com falha na
última janela ou drift: cache, journal e participações são restaurados, enquanto
commit, buffers e conteúdo recuperado permanecem íntegros.

Nenhum teste ACP local ou teste NVDA foi executado neste recorte.

Validação local: 393 testes seguros de chat, 363 de banco, 99 de controllers
e duas regressões de áudio (857), build/vet e lint incremental sem erros. Revisão
independente review_aep: cinco rodadas; cota agregada e atomicidade corrigidas
após a primeira; rodadas seguintes sem pendências, incluindo as correções remotas.
O teste de áudio conserva uma única seleção e a omissão do payload, exigindo agora
todos os participantes em vez de somente o item alterado. Inscrição recusa reader
sem suporte à participação antes de consultar ou registrar a janela. CI completo/
race e review remota são verificados no PR antes da entrega.

### Recorte: fronteiras estáveis para participação histórica (2026-11-02)

ReadHistoricalMembership seleciona o intervalo inclusivo entre os dois cursores
aceitos pela janela, na mesma transação da versão. Reutiliza a chave total,
a autorização e a cota SQL de identidades da paginação canônica. Não consulta
payload de mensagens/ledger e não repete Anchor=end. Inserções e exclusões
anteriores ao intervalo mudam índices/contagem; inserções internas alteram a
participação. Mensagens novas posteriores não deslocam a página histórica.

Fronteira excluída ou movida exige sincronização explícita. Cursores de outro
escopo, limites invertidos ou incompletos são recusados. Crescimento além do
limite retorna erro de cota, sem truncar a janela. Uma janela sem fronteiras
permanece vazia e atualiza somente a contagem; seguir a cauda é contrato separado.
O método exportado do repositório aplica o contexto também ao SQL de fronteiras,
não apenas à autorização.

Evidências: timeline_historical_membership_test.go cobre raiz/thread, inserções
internas/externas, remoção interna e de fronteira, fronteira única/movida,
owner, escopo, cancelamento após autorização, cota de itens/identidades, conteúdo
pesado e commit por outra conexão entre versão e seleção. Este recorte é o
seletor necessário para participação automática: integração com writers,
splices/replay por janela, followTail, foco, transporte e frontend continuam
pendentes. Não altera comportamento visível nem executa testes ACP localmente.

### Recorte: seleção leve para participação de janelas (2026-11-02)

A seleção de identidades/metadados foi extraída da materialização de conteúdo.
SelectTimelineWindowWithContext reutiliza exatamente a paginação canônica,
autorização, validação de cursores e cota SQL de identidades do leitor existente.
ReadMessageWindowWithContext conserva o guard antes do fetch; o caminho legado
continua com sua seleção e materialização originais. Não há algoritmo paralelo
de ordenação/paginação nem consultas por token.

ReadWindowMembership captura versão e seleção na mesma transação SQLite,
sem consultar conteúdo de mensagens ou resumos do ledger. O resultado interno
traz identidades ordenadas, índices, contagem e flags da página; Messages fica
vazio. A validação de pai é compartilhada com a leitura de conteúdo e conserva
ownership, conversa e exigência de raiz.

Regressões em timeline_membership_test.go comparam começo/cauda/around/cursor,
thread e página sem itens com a leitura canônica completa. Conteúdo/áudio/ledger
acima da cota não impede a seleção leve; identidades excessivas continuam
recusadas antes de materializar. Um commit por outra conexão depois da leitura
de versão prova que índices e IDs pertencem ao mesmo snapshot. Revogação de
owner, cancelamento, pai não raiz e âncora excluída falham explicitamente.

Este recorte prepara a atualização de participação, ainda não publica splices,
não acompanha a cauda automaticamente e não muda o frontend. Integração com
writers, retenção por foco, protocolo de transporte e cutover continuam pendentes.
Não há mudança visível nem teste ACP local.

Validação local: 370 testes seguros de chat, 363 de banco e 99 de controllers
aprovados (832); build/vet/lint incremental sem erros. Revisão independente
review_aep: uma rodada, sem pendências.

### Recorte: orçamento agregado das fontes da janela (2026-11-02)

O leitor canônico aplica a cota de 8 MiB existente à soma das mensagens,
metadados de turnos e resumos de ferramentas selecionados, antes de materializar
seus conteúdos. A paginação continua única: o repositório oferece um ponto de
verificação após selecionar os itens e antes de buscar seus payloads, dentro da
mesma transação SQLite do leitor. Não repete seleção nem recarrega a conversa.
O leitor legado permanece sem essa cota durante a migração.

O estimador SQL compartilhado passa a incluir identificadores e vínculos
variáveis, além dos textos, metadados e previews já contados. Metadados de lifecycle acompanham também as perguntas visíveis para desvincular
streams de execuções substituídas/concluídas. Respostas e ledger continuam
restritos aos itens de resposta selecionados; o orçamento inclui os metadados
das perguntas, sem carregar o conteúdo da resposta fora da página. Áudio e mídia continuam fora da materialização.

Antes de recuperar bases de publicação ou compor itens com streaming, a janela
inteira passa por um acumulador conservador compartilhado com a publicação.
Conta o DTO persistido e os buffers live, incluindo overhead e identidades
derivadas, sem clonar texto para descobrir excesso. Mede o escape JSON real das
strings; a margem estrutural ainda pode recusar um DTO pouco abaixo de 8 MiB.
Não soma cópias internas ao DTO: janelas independentes com 4 MiB de texto cada
continuam válidas. Mantém uma consulta de orçamento para mensagens/metadados
e outra para ledger, sem consulta adicional por item. Não percorre tentativas
retidas ou turnos fora da página; não acrescenta trabalho por token.

Excesso retorna ErrTimelineSourceLimit, sem truncar conteúdo, escolher outra
página ou descartar buffers. Navigate e Refresh mantêm consulta, participação e
revisão anteriores. A recusa local não marca erro global de publicação.

Regressões em window_budget_test.go cobrem soma de mensagens/ledger antes do
fetch, perguntas sem resposta fora da página, preservação do legado, recusa de
navegação/refresh com buffers intactos, escape JSON e identificadores extensos.
A prova de snapshot com commit concorrente agora roda também no leitor canônico.
Após review remota, a regressão inclui pergunta persistida e buffer live de
4 MiB + 1 byte cada: o preflight persistido aceita, a composição agregada recusa,
e sequência/cache publicados e inscrição permanecem intactos.

A revisão do Copilot identificou três lacunas adicionais, corrigidas neste PR.
O orçamento do ledger conta o response_message_id por invocação, pois a
continuação pode repetir esse destino em cada resumo. A reconciliação de lifecycle
não depende mais de a resposta estar visível: mantém metadados leves para perguntas
selecionadas, sem puxar seu ledger.

A janela é validada antes de iniciar recuperação pendente de armazenamento. Essa
recuperação percorre todos os lotes para validar as fontes e os buffers antes de
publicar o primeiro. Reutiliza o mesmo percurso em uma segunda passagem, exigindo
versão idêntica; não retém snapshots de todas as janelas simultaneamente. Só o
caminho de recuperação pendente faz duas passagens. A barreira impede crescimento
de buffers entre elas; drift externo mantém a recuperação suspensa.

window_budget_review_test.go reproduz identificador de continuação de 512 KiB
repetido em vinte ferramentas, conclusão/retry fora do projetor com página só de
pergunta e recuperação pendente em mais de um lote, com buffer excessivo dentro
ou fora da página. A recusa preserva cache, sequência, inscrição e bytes recebidos.
Os três testes novos falham contra o código anterior às correções e passam com
elas. As expressões estáticas de tamanho SQL são construídas uma vez; o conjunto
de lifecycle reutiliza a deduplicação já feita durante a leitura.
A consulta de autorização carrega apenas o ID da conversa, mantendo ownership
sem transportar seu resumo. Os conjuntos de IDs do orçamento usam um parâmetro
JSON de hex ASCII, decodificado no lado direito de IN: evita expandir centenas
de placeholders sem alterar bytes opacos (inclusive NUL e UTF-8 inválido).
Regressões exercitam esses IDs e o EXPLAIN continua exigindo buscas indexadas.

A segunda rodada de review inclui a amplificação de identidades por cortes live:
cada ferramenta pode criar um grupo e um corte de texto, contabilizados antes da
clonagem. A regressão específica de offsets ACP fica reservada ao CI; a cota
conservadora também tem teste local sem ACP.

Na recuperação, a segunda passagem prepara contêineres provisórios de cache e
journal, compartilhando valores imutáveis. Falha ou drift posterior restaura toda
a publicação anterior; o teste compara cache, ordem, tamanhos, sequência e batches.
As cotas de cache e replay continuam vigentes, sem reter todos os snapshots.
A autorização por ID também cobre a seleção usada nas duas passagens.

A seleção SQL única agora soma as identidades da página/âncora e mascara os campos
antes do Scan quando excedem a cota. Parâmetros opacos recebem cota antes da
expansão hexadecimal e deduplicação. A regressão inspeciona o resultado SQL cru,
inclusive ID com NUL, e prova preservação da ordenação e contagem. Os testes de
plano continuam exigindo consultas em lote e buscas indexadas; a captura distingue
o preflight de payload da nova soma de identidades na seleção existente.

Validação da primeira rodada: 359 testes seguros de chat, 362 de banco e 99 de controllers
aprovados (820), build/vet/lint sem erros. A conta de escape é confrontada com
encoding/json, incluindo UTF-8 inválido; a codificação de substituição é medida
uma vez para acompanhar a versão ativa do encoder. Nenhum teste existente foi
removido ou relaxado. Revisor independente review_aep: quatorze rodadas, sem pendências.

A terceira rodada de review fecha o orçamento agregado da recuperação: as duas
passagens validam o lote inteiro e subdividem quando fontes ou buffers não cabem,
antes de compor/publicar. Uma recusa indivisível mantém a publicação anterior.
O preflight inclui os campos de identidade/escopo da pergunta por turno, sem seu
conteúdo; a validação do pai lê apenas conversa e presença de pai, sem turn_id.
Regressões cobrem nove buffers de 1 MiB, perguntas sem resposta com parent_id
extenso e página de thread vazia cujo pai tem turn_id legado excessivo.

Validação da segunda rodada: 364 testes seguros de chat, 363 de banco e 99 de
controllers aprovados (826), build/vet/lint sem erros; o cenário específico ACP
permanece para o CI. Revisor independente review_aep: rodada 10 sem achados
bloqueantes. As execuções interrompidas por falta de espaço não foram contadas;
foram repetidas depois de limpar apenas artefatos de build deste worktree.

Validação da terceira rodada: 367 testes seguros de chat, 363 de banco e 99 de
controllers aprovados (829), build/vet/lint sem erros. Revisor independente
review_aep: rodada 11 identificou teste de observação da consulta antiga,
corrigido; rodadas 12 e 13 aprovadas. O EXPLAIN detectou varredura na soma de
escopo, corrigida com busca pela PK, e o teste de deduplicação impediu contar
novamente a resposta selecionada pelo turno. Nenhum desses testes foi relaxado.

A validação remota da terceira rodada aprovou chat, backend, frontend, bindings
e E2E, mas revelou disputa de temporização na fixture de estados do ledger:
TestServicePersistsStatusesAndRetryable compartilhava 5 ms entre cancelamento e
timeout. O prazo curto agora pertence somente ao cenário de timeout; as asserções
de persistência, estado e retry permanecem intactas. Os 97 testes seguros do pacote
toolinvocations e 30 repetições do cenário passaram, assim como vet/lint; revisor
review_aep aprovou a rodada 14. A correção não altera contratos de produção.

Medição anterior à segunda rodada de review (não representa as novas proteções): Windows/amd64, três pares intercalados de 200
operações com GOMAXPROCS=2, fixture de 100 perguntas. Medianas de 5,625 ms e
925.935 B/op na base versus 6,771 ms e 1.047.161 B/op neste recorte: +20,4% de
tempo e +13,1% de alocações. O caso pequeno ainda não demonstra margem sob o
critério final de +20%; esse critério permanece pendente, sem declarar a fase
concluída. Séries locais anteriores tiveram grande variação de tempo inclusive
na base, motivando a comparação intercalada. Os números medem o leitor, não
composição live, recuperação pendente ou UI.

São cotas de fonte e estimativa de composição, não um teto do heap do processo:
SQLite, caches, cópias transitórias e buffers recebidos têm custos distintos.
O envelope wire final ainda não existe; medição do DTO completo e seu limite,
participação automática, transporte, cutover e migração de anexos continuam
pendentes. Nenhuma alteração visível de UI ou teste ACP local/NVDA neste recorte.

### Recorte: leitura leve da janela canônica (2026-11-02)

A materialização canônica de mensagens exclui media, audio e audio_mime_type no
SQL. Janela e seleção por IDs usam o mesmo fetch; foi removida a configuração
duplicada da seleção. A exclusão fica restrita à consulta de mensagens, sem
contaminar consultas de autorização/modelos diferentes no mesmo snapshot GORM.
O leitor canônico também deixa de resolver mídia herdada de continuações, que
não integra seu DTO atual. O caminho legado mantém sua mídia e resolução.

A identidade de mensagem ganha método de repositório injetado e parent_id entre
seus vínculos. As duas validações do pai da janela passam por essa consulta leve,
preservando ownership, conversa e recusa de pai que não é raiz. Nenhum blob ou
texto do pai é lido para conferir esses vínculos; nenhum dado persistido é alterado.

`window_light_read_test.go` cobre raiz/thread com blobs de 2 MiB, SQL sem as
colunas pesadas, metadados do pai, igualdade do DTO, isolamento de usuário,
recusa de pai não raiz e preservação do armazenamento/caminho legado. A regressão
de continuação prova que a consulta recursiva de mídia só ocorre no leitor legado.

Medição local Windows/amd64, fixture de duas mensagens com media e audio de
2 MiB cada, três amostras de cinco operações: medianas de 188,37 ms e 17.041.398
B/op no leitor legado versus 2,26 ms e 203.779 B/op no canônico. Ambos produziram
DTO de 2.183 bytes. Tempo e alocações incluem leitura, projeção canônica e
serialização JSON. É uma comparação controlada dos dois caminhos, não uma promessa
de desempenho da UI nem uma medição do heap retido.

Controle sem blobs, mesma máquina/benchmark de participação (100/500/1.000
perguntas, três amostras de cinco operações): medianas canônicas de
5,73/10,32/14,58 ms, ante 5,74/11,03/14,87 ms na base. Validação: 350 testes
seguros de chat, 362 de banco, 99 de controllers e 38 de speech aprovados; build/vet/lint sem
erros. Review_aep independente: três rodadas, zero pendências. As listas
exatas de colunas nos testes de controllers e speech incluem agora parent_id; continuam
recusando payloads. Nenhum teste ACP local ou NVDA; frontend inalterado.

Orçamento agregado de fonte/DTO, participação automática e transporte continuam
pendentes. A fase 4 não está concluída: a UI legada ainda usa sua leitura anterior,
e anexos referenciados/migração da duplicação de áudio exigem recorte próprio.

### Recorte: navegação da janela inscrita (2026-11-02)

`TimelineSubscription.Navigate` muda a fatia da inscrição na mesma barreira usada
pelos writers e pela recuperação. Preserva `windowId`, sessão, conversa e escopo;
o pedido só informa posição/direção/limite e a época/revisão esperadas. Consulta,
composição, publicação e troca dos IDs inscritos acontecem antes de liberar a
barreira. A leitura utiliza o leitor canônico existente e seu limite de 240 itens;
não existe algoritmo de paginação paralelo. Esse limite não é uma cota de bytes:
o orçamento agregado de payload da janela permanece pendente. As cotas de fonte,
cache e replay da publicação não limitam a materialização completa deste snapshot.

Snapshots internos passam a informar `windowRevision`, uma string decimal
independente da sequência de conteúdo. Cada snapshot aceito avança essa revisão,
inclusive Refresh. Entre pedidos concorrentes baseados na mesma revisão,
somente um navega; o outro recebe `ErrTimelineWindowStale`. Cursor inválido,
âncora removida, sessão revogada e leitura malsucedida preservam a consulta e os
IDs anteriores. Import/recriação detectado durante a leitura recusa época antiga
antes de trocar a inscrição. Recuperação explícita continua em Refresh.

Após a review remota do #997, Refresh aceita uma posição explícita de recuperação,
compartilhando `TimelineWindowPosition` e a montagem da consulta com Navigate.
Posição nula repete a última consulta; posição explícita permite abandonar um
cursor cuja âncora foi excluída, inclusive após mudança de época, sem recriar a
inscrição nem trocar seu escopo. Não há fallback silencioso. Replay também exige
a revisão da janela: pedido antigo que espera atrás de uma navegação não recebe
operações filtradas pela página nova. As assinaturas internas foram atualizadas
nos consumidores de teste, preservando todas as asserções anteriores.

A segunda rodada remota identificou mais duas intercalações: autorização anterior
à espera pela barreira e recuperação sem revisão concorrente. Os três métodos do
capability agora compartilham autorização após adquirir a barreira; Subscribe
também reconfirma a sessão após esperar. O autorizador confiável não reentra no
projetor. Refresh exige revisão da janela, independente da época: ainda recupera
import/recriação, mas não sobrescreve navegação ou recuperação mais recente.
Regressões sincronizam revogação após a reserva efetiva da chamada e recuperação
atrás de Navigate/Refresh. Nenhuma consulta de conteúdo ocorre na recusa.

`timeline_navigation_test.go` cobre paginação real por cursor, duas superfícies,
preservação da consulta em Refresh, revisão obsoleta, pedidos concorrentes,
falha sem mudança de participação, fechamento/revogação, escopo de thread,
âncora excluída e troca de época durante leitura. Duas regressões adicionais
cobrem cursor previamente aceito que se torna obsoleto (com/sem nova época) e
replay concorrente bloqueado atrás de navegação, sem sleeps de sincronização.

Validação local final: 348 testes seguros de chat, 99 de controllers e a regressão de áudio/timeline aprovados; build/vet e lint incremental
sem erros. Revisão independente review_aep: cinco rodadas, sem pendências finais após corrigir quatro apontamentos remotos e precisar o escopo de memória. Race local indisponível
por ausência de CGO/compilador C; validação com detector de corrida fica no CI.

Este contrato ainda é interno: não adiciona binding/evento nem altera a UI.
Orçamento agregado de payload da janela, participação automática com splices,
followTail, retenção por foco, transporte e
cutover permanecem pendentes. A fixture completa de crescimento simultâneo de
duas janelas continua aberta. Nenhum teste ACP local ou NVDA neste recorte.

### Recorte: limpeza coordenada de órfãos (2026-11-02)

As entradas legada e limitada de limpeza de órfãos passam pela mesma coordenação
de manutenção da retenção por idade. O predicado de orfandade é único para
seleção, descoberta e escrita; sua política não foi alterada. Cursores só avançam
após commit, e a primeira inscrição usa a barreira do projetor compartilhado.

No caminho limitado, a descoberta restringe pais à página e filhos ao mesmo
prefixo que pode ser desvinculado. A ordem parent_invocation_id + rowid mantém
esse prefixo estável usando o índice existente, sem ordenar todos os IDs de
filhos. A união das linhas selecionadas não varre invocações fora da página para
montar a projeção. Quando restam filhos, a próxima passagem retoma a página.

A transação comum mantém callback e COMMIT em tentativa única; adquirir
BEGIN IMMEDIATE preserva o retry cancelável canônico. A retenção por idade mantém
seu retry externo; órfãos não reexecutam callback/COMMIT automaticamente em busy. Mudança do conjunto afetado continua exigindo redescoberta limitada,
antes de qualquer escrita. A consulta de schema também observa cancelamento.

`orphan_publication_test.go` integra serviço, SQLite e projetor: entradas legada
e limitada, rollback de detach, cursor preservado, filhos em conversas distintas,
limite da descoberta por passagem e retirada do órfão da base publicada sem
apagar mensagens. Testes anteriores de cursores/limites continuam presentes.
Validação: 339 testes seguros de chat, 97 de toolinvocations, 382 de jobs e
25 de App aprovados; build/vet/lint sem erros. Review_aep aprovou o código e
solicitou precisão sobre retry de BEGIN IMMEDIATE, já corrigida neste texto.
Frontend inalterado; nenhum teste ACP executado localmente.

No CI do #996, duas execuções atingiram o timeout agregado padrão de 10 minutos
do pacote App (1.394 testes), com o teste corrente ativo por apenas 13–14 segundos.
O orçamento da suíte com cobertura passa a 15 minutos por pacote, com 30 minutos
para o job incluindo preparação e compilação. Nenhum teste ou asserção foi removido.
Uma execução intermediária falhou por SQLITE_BUSY na preparação de um teste de
comandos; esse teste passou em dez repetições locais consecutivas.

Participação de janelas, transporte e cutover frontend continuam pendentes;
este recorte não declara os sintomas de foco/ordem da UI resolvidos.

### Recorte: retenção de ferramentas por idade (2026-11-02)

O repositório de ledger do bootstrap recebe a coordenação de manutenção do mesmo
projetor. A retenção opcional por idade descobre somente identidades das conversas
afetadas, incluindo filhos que serão desvinculados e origens legadas autorizadas.
Sob manutenção → barreiras ordenadas do projetor → transação SQLite imediata,
revalida esse conjunto antes de executar o delete atômico existente. Mudança de
escopo aborta sem escrita e refaz a descoberta, com limite de oito tentativas.

`ApplyStorageBatchMutation` reutiliza a reserva de lotes e a reconciliação do
writer individual. A primeira inscrição espera o commit; buffers não são
separados como na importação. Erros da escrita permanecem erros; falhas posteriores
de leitura não incentivam repetir um delete confirmado. O writer de retenção
reutiliza o helper de desvincular filhos, eliminando a cópia do mesmo SQL.

Evidências: `maintenance_publication_test.go` integra serviço/repositório, SQLite
e projetor, cobrindo commit, rollback de delete+detach, filhos em outra conversa,
revalidação de escopo que mudou e primeira inscrição concorrente. Testes existentes
de retenção por idade preservados. A primeira revisão identificou dois P2:
vínculos legados por turn_id fora da descoberta e contexto ausente na consulta
inicial. A descoberta agora usa a união dos vínculos da v43, mais origem pré-v18,
com owner em todos os ramos e contexto nas consultas. Regressões reais cobrem
conversation_id nulo/divergente e cancelamento aguardando conexão.
Validação: 338 testes seguros de chat, 97 de toolinvocations, 382 de jobs e
25 de App aprovados; build/vet/lint sem erros. Revisão independente review_aep:
dois P2 corrigidos, rodada final sem pendências. Nenhum teste ACP local.
Frontend inalterado, mantendo validação da base.

Limpeza de órfãos, participação de janelas, transporte e cutover frontend seguem
pendentes. Dry-runs operacionais não pertencem à timeline de chat. Este recorte
mantém a política opcional e o escopo transacional por usuário já existentes;
não declara limitação por quantidade de registros nem problemas de foco resolvidos.

### Recorte: importação coordenada de conversas (2026-11-02)

A importação usa `ApplyConversationImport`, compartilhando a barreira e a
reconciliação de `ApplyStorageMutation`, para envolver cada transação de
conversa, inclusive a busca por conteúdo existente. O bootstrap injeta o mesmo
projetor dos demais escritores nas três entradas Wails. `ConversationImportHooks`
reúne a coordenação de escrita e o restore pós-commit, substituindo a assinatura
anterior sem manter outro pipeline público de importação.

O coordenador recebe o ID opaco e seleciona todos os itens materializados antes
da substituição. Reconciliação acontece sob manutenção antes de liberar tombstones;
falha de uma conversa não desfaz commits anteriores nem publica conteúdo revertido.
Parsing, credenciais e demais recursos continuam fora da barreira da timeline.
A ordem runtime → lifecycle → manutenção → projetor → SQLite permanece vigente.

Evidências: `import_publication_test.go` integra importador, SQLite, projetor e
inscrição reais; verifica substituição, retirada das fontes antigas da resposta, rollback
parcial, identidade de janela preservada e recusa antes do writer. A asserção
pós-commit inspeciona a base publicada antes de Refresh, evitando que releitura
mascare ausência de coordenação. Os três entrypoints verificam propagação dos
hooks em `export_import_test.go`.

A primeira revisão identificou sobreposição de buffers voláteis quando o backup
reutiliza o mesmo execution ID. Após commit, a importação separa os buffers
anteriores em retained e invalida suas capabilities antes de reconciliar; em
rollback preserva a projeção. A regressão exporta/reimporta v3 com execução
terminal volátil e comprova conteúdo restaurado e recuperação dos bytes antigos.
A regressão falhou com a coordenação genérica anterior e passou com a correção.
Validação: 333 testes seguros de chat, 195 de Wails, 166 de portabilidade e 42
de App aprovados; build/vet/lint sem erros. Frontend não alterado, mantendo a
validação da etapa anterior. Review_aep: duas rodadas, P2 corrigido e nenhuma
pendência. Nenhum teste ACP executado localmente.

A review remota identificou buffers retidos inacessíveis quando a importação
remove a pergunta original. `ReadRecoveryTurn` autoriza a conversa e lê o turno
opcional na mesma transação; recuperação/descarte não dependem mais da presença
da pergunta. Leitura estrita continua rejeitando pergunta ausente e publicação
mantém seu orçamento. A regressão comprova recuperação e descarte sem a pergunta,
recusa para outro usuário e recusa após excluir a conversa. Correção validada
com 334 testes seguros de chat e 137 de agent. A revisão local também identificou
um fake que precisava manter a leitura estrita; corrigido. Review_aep: quatro
rodadas ao total, última sem pendências.

Manutenção, participação automática das janelas, transporte e cutover frontend
permanecem pendentes. A descoberta de novos itens para janelas abertas pertence
ao recorte de participação; este PR não declara foco/ordem da UI resolvidos.

### Recorte: exclusão coordenada de conversas (2026-11-02)

Exclusão individual, lote e limpar todas compartilham a escrita sob manutenção
no controller. O bootstrap liga essa escrita ao projetor; não há outro pipeline
de delete nem outra transação. A ordem permanece runtime → lifecycle → manutenção
→ projetor → SQLite, com finalize/reset/evento somente após invalidar a projeção.
O seam redundante de delete sob lifecycle foi removido do controller.

O projetor reserva os IDs do lote e adquire as barreiras em ordem estável antes
do writer. A primeira inscrição também aguarda a operação. Após commit, somente
as conversas efetivamente excluídas perdem janelas, replay, bases e buffers;
capabilities antigos deixam de aceitar tokens ou devolver conteúdo, inclusive
após recriar o mesmo ID. Falha da transação não altera a projeção nem produz
efeitos pós-commit. Locks de conversa são liberados antes de readquirir o mutex
global no cleanup, evitando inversão de ordem em lotes concorrentes.

Evidências: `conversation_timeline_deletion_test.go` cobre SQLite/projetor/inscrição
reais, as três entradas, rollback, outro usuário, runtime ocupado, ordem dos
efeitos e integridade das conversas preservadas. `conversation_deletion_test.go`
cobre capabilities, liberação do conteúdo, recriação do ID, rollback, autenticação,
primeira inscrição concorrente e lotes em ordem inversa com IDs duplicados.
Validação: 99 testes seguros de controllers, 330 de chat e 362 de database,
mais build/vet/lint, aprovados. Frontend: tsc, ESLint, Stylelint (sem erros)
e 5.887 testes em 484 arquivos aprovados. Revisão independente review_aep:
duas rodadas sem pendências. Nenhum teste ACP foi executado localmente.

Importação, demais escritores de manutenção, participação automática das janelas,
transporte e migração frontend permanecem pendentes. Não declara foco/ordem da UI
resolvidos. Este recorte mantém o contrato de normalização do delete existente.

### Recorte: limpeza do conteúdo de conversa (2026-11-01)

As três entradas de limpeza do controller compartilham a coordenação da operação
inteira, selecionando todos os itens materializados para remover conteúdo antigo.
A conversa permanece existente. Reset de serviços e evento de limpeza ocorrem
após reconciliação, ainda sob os contratos de runtime/lifecycle e guard existentes.
A ordem de locks é runtime → lifecycle → guard → manutenção → projetor → SQLite.

A comparação de revisão e a exclusão continuam na mesma transação. A entrada
interna foi substituída pela variante `WithinMaintenance`, cujo caller já mantém
lifecycle e manutenção; não há aquisição recursiva nem wrapper produtivo sem
consumidores. Fixtures de storage assumem explicitamente a manutenção antes da
mesma operação, preservando todas as asserções anteriores.

`TestConversationClearCoordinatesBeforeEffects` usa banco/projetor/inscrição reais
para limpeza simples, com revisão e guardada, revisão obsoleta, guard recusado e
rollback. Verifica ordem manutenção/projeção, reconciliação antes dos efeitos,
conversa preservada, histórico vazio no refresh e integridade de mensagens,
ledger e resumo nas recusas. A revisão identificou normalização indevida em
dois helpers de limpeza; ambos preservam agora o ID opaco exato. Regressão
com conversas `conversation` e ` conversation ` cobre as três entradas, mensagens, ledger, resumo
e inscrições independentes; o cenário simples falhou antes da correção.
Validação: 98 testes seguros de controllers, 362 de database, build/vet/lint
aprovados. Review_aep: duas rodadas, P1 corrigido e nenhuma pendência.

Exclusão de conversas (inclusive lote), importação e demais escritores de manutenção
permanecem pendentes, assim como participação de janelas, transporte e cutover
frontend. Não há alteração da confirmação pública nem da UI. Nenhum teste ACP
é executado localmente.

### Recorte: persistência de áudio (2026-11-01)

`DBAudioStore` recebe obrigatoriamente o coordenador compartilhado do bootstrap.
Todas as gravações de áudio por speech e canais passam pela mesma instância;
geração TTS e transporte ficam fora da barreira. Apenas a gravação e a
reconciliação da mensagem/resposta são serializadas com a timeline.
A pré-leitura autoriza e seleciona somente ID, conversa e turno, sem materializar
o áudio anterior. IDs de turno vazios não viram identidades de resposta.
Falha do writer é preservada; ausência do coordenador impede a gravação.

`TestAudioPersistenceCoordinatesTimelineWithoutReadingPayload` usa SQLite,
projetor e inscrição reais para mensagens avulsas, resposta vinculada e vínculo
legado vazio. Cobre áudio anterior de 2 MiB, identidade, omissão de payload na
projeção, renovação do snapshot, recusa de ownership antes da coordenação e
falha de escrita sem alterar áudio confirmado. Não há mudança do contrato de
speech ou geração de bindings; armazenamento unificado de anexos continua na fase 4.

Limpeza/import/manutenção, participação de janelas, transporte e cutover
frontend continuam pendentes. Validação: 38 testes seguros de speech, build,
vet e lint aprovados; review_aep, primeira rodada sem pendências.
Nenhum teste ACP é executado localmente.

### Recorte: criação avulsa de mensagens (2026-11-01)

As seis entradas de criação de `ConversationsController` delegam a um núcleo
único que envolve a gravação no coordenador compartilhado e informa os IDs
persistidos após o commit. A criação usa seleção vazia inicialmente, sem
reler todos os itens materializados. Conteúdo, mídia, tokens, modelo e escopo
filho mantêm o contrato original; falha do writer não inventa identidade nem
vira sucesso. O pipeline de envio do agente mantém sua admissão própria,
sem adquirir recursivamente a barreira pelo repositório de storage.

`TestMessageCreationReconcilesAllControllerEntries` cobre as seis entradas com
SQLite, projetor e inscrição reais, reconciliação antes do retorno, metadados,
omissão de mídia na projeção, escopo filho e recusa de ownership.
`TestMessageCreationPreservesWriterFailureAndMissingRepository` cobre preservação
do erro original e configuração incompleta. Nenhum teste ACP é executado localmente.

A participação automática de itens novos em janelas ainda depende do recorte de
janelas; nesta etapa a inscrição descobre a criação ao renovar seu snapshot.
Validação: 96 testes seguros de controllers, build, vet e lint aprovados;
revisão independente review_aep, primeira rodada sem pendências.

Persistência de áudio, limpeza/import/manutenção, transporte e cutover frontend
continuam pendentes. Não há alteração de UI, bindings ou do contrato público.

### Recorte: edição, fixação e exclusão de mensagens (2026-11-01)

`ConversationsController` compartilha o coordenador do projetor para os comandos
guardados de editar, fixar e excluir, e para os métodos de mensagem ainda usados
por outros consumidores. Exclusão em lote é uma operação coordenada inteira:
mesmo quando um item posterior falha, a projeção reconcilia exclusões anteriores
já confirmadas sem converter o erro original em sucesso.

Os comandos mantêm a ordem lifecycle → guard de sessão/workspace → manutenção →
projetor → transação SQLite. `MutateMessageCommandWithinMaintenanceWithContext`
unifica a gravação transacional de editar/fixar/excluir. Os dois entrypoints antigos,
que ficaram sem consumidores produtivos, foram removidos; a aquisição de manutenção
nos testes de storage passou ao fixture, preservando suas asserções. O controller
usa o núcleo comum depois de adquirir os locks externos. Revisão
esperada, ownership, cancelamento/preparo e confirmação de ações permanecem nos
mesmos contratos. Eventos existentes são emitidos após sair da barreira do projetor.

Edições e fixações selecionam somente o item físico e a resposta de seu turno,
quando existente. Exclusões reconciliam os IDs materializados para cobrir cascatas.
Não se adiciona consulta por delta nem reconstrução de grupos no frontend.
A pré-leitura de edição, fixação e exclusão legadas (inclusive lote) autoriza e seleciona apenas ID, conversa e turno,
sem materializar texto, áudio ou anexos para entrar no coordenador.

Evidências em `controllers/conversation_timeline_mutation_test.go`: sete caminhos
reais com banco e projetor compartilhado, incluindo fixação do representante,
edição da pergunta/resposta, exclusão em cascata e lote parcialmente confirmado.
O evento só ocorre após a leitura de reconciliação; os comandos comprovam aquisição
de manutenção antes da projeção. Guard recusado não chega ao projetor.
Regressão com áudio de 2 MiB verifica as colunas da pré-leitura e a recusa de
ownership antes da coordenação, atendendo ao achado do Copilot.

Validação local: 362 testes seguros de database, 94 de controllers e 21 de app
(timeline/janelas/comandos de mensagem) aprovados; build, vet e golangci-lint
incremental sem novas ocorrências. O frontend não mudou desde a validação da
etapa anterior (TypeScript/ESLint/Stylelint e 5.887 testes seguros). Revisão local
independente review_aep: a revisão completa da oitava rodada identificou vínculo legado composto apenas por espaços; a geração de IDs agora ignora respostas sem identidade, preservando IDs opacos válidos sem normalização. Regressão com espaços ASCII e Unicode e ID opaco verifica edição, fixação e atualização da inscrição sem bloquear a publicação. A nona rodada confirmou a correção, sem pendências.
Nenhum teste ACP executado.

Limites deste recorte: criação de mensagens, limpeza de conversa/import/manutenção
e gravações de speech ainda precisam completar a integração. Participação de
janelas, transporte e cutover frontend permanecem pendentes; o contrato público
atual de eventos não foi substituído nesta entrega.

### Recorte: reconciliação de mutações e ledger (2026-11-01)

Falha auxiliar de publicação não bloqueia páginas legadas autorizadas; journal/replay e snapshot canônico continuam suspensos. Regressão com resposta acima de 8 MiB cobre página pequena, revogação e recuperação. O executor coordenado tem provas com ferramenta bloqueada (leitura e mutação concorrentes) e falha de MarkRunning (exclusão reconciliada antes do retorno), usando SQLite e projetor reais.

`ApplyStorageMutation` mantém a barreira da conversa durante a operação externa
inteira e sua reconciliação. Reserva a instância mesmo sem observadores prévios,
para a primeira inscrição não ultrapassar um commit pendente. Sem bases ou
observadores, libera a instância após a gravação sem consultar uma projeção inútil.
Callbacks de storage não reentram no projetor nem emitem eventos; a ordem é
barreira do projetor antes da transação SQLite. Uma falha após escrita parcial
também reconcilia o estado confirmado, preservando o resultado original do writer.

`ReadSelection` autoriza a conversa e lê IDs explícitos, seus escopos reais,
mensagens, turnos e ledger no mesmo snapshot. Ausência nesta seleção pode emitir
`item_remove`; ausência numa página continua sem significar exclusão. A seleção
omite mídia/áudio e aplica o orçamento de fonte antes da materialização. O
preflight usa até duas consultas por lote de 400 âncoras, não duas por item;
seleção apenas de mensagens dispensa a consulta de ledger. O orçamento distingue
mensagens e turnos: uma pergunta isolada não paga pelas respostas/ferramentas
fora da seleção. A fonte do SUM usa buscas indexadas e UNION sem dupla contagem.
O filtro de IDs antecede a classificação compartilhada; `EXPLAIN QUERY PLAN`
com 1.000 mensagens comprova busca por PK e pelo índice de turno/escopo, incluindo
o preflight de mensagens, a busca do ledger por usuário/origem/turno e a leitura
final dos payloads. Esta também seleciona por UNION antes de materializar,
mantendo escopo, filtro de execução e omissão de mídia.
Pergunta legada com `turn_id=id` só entra em `message:<id>`; orçamento e leitores
de resposta excluem `role=user` antes de carregar conteúdo, inclusive no lifecycle.

Falha de reconciliação suspende publicação/replay e troca a época. Recuperação
reconcilia todos os IDs materializados antes de liberar a publicação; conserva
buffers válidos e desvincula execuções comprovadamente substituídas/excluídas.
Cada inscrição precisa renovar seu próprio snapshot da nova época. Há um
suplemento limitado a 512 IDs de criações ainda não materializadas; além dessa
cota, o snapshot solicitado descobre os itens, sem fila ilimitada durante falha.
A recuperação parcela conjuntos maiores, reduzindo o lote quando atingir 8 MiB,
e exige versão idêntica entre parcelas antes de liberar replay. Uma execução
concluída ainda pendente de publicação também renova sua base; resultado tardio
do ledger não pode ser sobrescrito pelo flush terminal. Regressões cobrem duas
janelas independentes acima de 8 MiB somadas e drift depois da primeira parcela.
Recuperação explícita de turno permanece disponível se a publicação auxiliar
continuar acima da cota: `ReadTurn` autoriza e devolve persistência/buffers,
enquanto journal e replay ficam suspensos. A leitura não publica o item grande
nem ignora revogação; reduzir o conteúdo permite recuperar a publicação normal.

O bootstrap compartilha o mesmo projetor entre agente e ledger. Criação,
running, conclusão e exclusões de compensação passam pelo coordenador; a tool
executa fora da barreira. A atualização de vínculos de apresentação feita pelo
agente também participa. Origens antigas usam a mesma resolução autorizada de
`Create`, sem reconstrução de autoridade no frontend. Argumentos/resultados
permanecem exclusivamente no ledger (AEP-0104).

Evidências: `storage_publication_test.go` cobre escopo/ownership, exclusão com
erro parcial e preservação de buffers, item fora da seleção, recuperação e
época por janela, criação durante falha, primeira inscrição concorrente e ledger
real composto com texto vivo. A prova de lote verifica uma consulta para 80
mensagens e duas ao incluir turnos. Pergunta isolada com resposta e ledger acima
de 8 MiB fora da janela continua recuperável; seleção mensagem+turno não duplica
bytes. Não representa ainda medição de heap nem benchmark do cutover.

Validação local: 327 testes seguros de chat, 362 de database, 137 de agente, 96 de toolinvocations
e 10 de app; build/vet e lint Go incremental aprovados. TypeScript, ESLint e
Stylelint passaram (2.124 avisos CSS preexistentes, zero erros); Vitest seguro:
484 arquivos, 5.887 testes aprovados. Nenhum teste ACP foi executado localmente.
Revisor independente review_aep: quatorze rodadas; a primeira identificou a corrida
com a inscrição inicial e o custo por âncora. Corrigidos ambos, gate final aprovado
sem pendências. Copilot apontou o orçamento agregado, a base terminal antiga e
o filtro tardio da seleção; corrigidos com regressões e nova aprovação local.
Segunda rodada remota identificou custo/escopo excessivos no preflight; corrigidos
com seleção por tipo de item e EXPLAIN também das consultas de orçamento.
Terceira rodada remota identificou bloqueio da recuperação explícita por item
acima da cota; regressão cobre leitura autorizada, buffer, replay suspenso,
revogação e recuperação posterior da publicação. Nona revisão local aprovada.
Quarta rodada remota apontou payloads com filtro OR e a pergunta legada no
orçamento; corrigidos com EXPLAIN da materialização e regressão de pergunta
acima de 8 MiB com resposta curta, lifecycle e Refresh após mutação de ledger.
A revisão remota e o CI completam a entrega do PR.

Limites: o coordenador já trata exclusões, mas os comandos de edição/exclusão de
mensagens, import, manutenção e speech ainda precisam aderir. Patches de participação
das janelas, emissor autenticado, bindings Wails e consumidor frontend continuam
pendentes. Não há promessa de sincronização pública completa neste recorte.

### Recorte: publicação automática do lifecycle (2026-11-01)

O orçamento automático conta apenas mensagens de resposta; texto da pergunta não é consumido e não entra na cota. Isso inclui perguntas legadas com turn_id=id, excluídas também da materialização de resposta. A regressão de orçamento cobre pergunta acima de 8 MiB com resposta pequena, vínculo normal e legado.

Recuperar um turno removido também libera o flush de terminais pendentes de outros turnos, após leitura autorizada, mesmo sem item próprio para publicar. A matriz de recuperação cobre ausência do turno/resposta.

Admissão, início/alteração de stream, confirmação de conteúdo e terminal alimentam
o mesmo codificador de operações usado por snapshot e replay. Tokens apenas
alteram buffers e agendam um timer por conversa, com coalescing de 24 ms; não
consultam storage. Admissão e terminal descarregam sincronamente os itens sujos.
Geração do timer e identidade da instância impedem callbacks antigos após flush
ou evicção. O terminal volátil conserva conteúdo também na leitura de recuperação.
Flush e recuperação usam a mesma decisão de composição: terminal confirmado não
é substituído por um stream vazio, preservando o motivo persistido de falha do
provedor. Falha na gravação terminal permanece volátil mesmo sem stream ou com
todos os conteúdos já confirmados; durabilidade inclui o estado, não só o texto.

A base persistida é renovada sob a barreira nas transições do writer, por leitura
restrita ao turno. É uma cópia sem áudio/mídia e com orçamento de 8 MiB de JSON por
conversa (estimativa de retenção, não medição de heap). Conclusão publicada libera
a base; buffers não confirmados continuam disponíveis para recuperação explícita.
`ReadPublicationTurn` verifica bytes e custo por registro no SQL, dentro do snapshot
autorizado, antes de materializar mensagens/ledger. A pergunta carrega só metadados;
mídia/áudio não entram nessa leitura. Uma estimativa conservadora limita também a
cópia/serialização da projeção, incluindo expansão por escape JSON. Esse limite pode
recusar uma base menor que 8 MiB serializada; a recuperação explícita permanece disponível.
Leitura auxiliar tem prazo de três segundos, inclusive após cancelamento do caller.
Falha ao renovar invalida a base, suspende replay e muda a época uma única vez
enquanto houver erro. Não reclassifica um commit bem-sucedido como falha de escrita.
Leitura autorizada restaura a base e compõe os buffers atuais, sem perder deltas.
Ao remover o último erro, descarrega todos os turnos sujos e libera bases terminais,
tanto na recuperação individual quanto na leitura canônica de janela.
Desvinculação comprovada retira o turno dos bloqueadores de publicação, mantendo
os bytes retidos para recuperação explícita.
Admissão e leituras compartilham a reconciliação de encarnação: a troca comprovada
invalida bases/erros antigos também fora da página, sem apagar buffers recuperáveis.
O flush só admite a encarnação atual. Descarte autorizado e leitura de janela
legada descarregam os turnos pendentes depois de remover bloqueadores; leituras
recusadas não alteram publicação.
Turnos com erro ou publicação pendente não expiram pelo prazo de retenção de
60 segundos; permanecem até publicação ou reconciliação/descarte explícito.
Os quatro caminhos de recuperação também são exercitados após esse prazo.
Descarte da execução visual atual reconcilia o journal com a resposta persistida
ou `item_remove` quando ela não existe. Se outro bloqueador impedir publicação,
invalida época e journal, exigindo snapshot sem reproduzir operações descartadas.
Descartar tentativa antiga retida não altera a execução atual. O codificador
compartilhado de remoções é introduzido aqui e reutilizado pelas mutações externas.

Limite deste recorte: publicação automática no journal interno para os escritores
do lifecycle integrados. Alterações externas de mensagens/ledger, patches de
participação das janelas, emissor autenticado e bindings Wails seguem pendentes.
Não há ainda entrega pública completa nem migração do frontend. A fase 2 permanece
In Progress; o próximo recorte conecta invalidações e exclusões ao mesmo projetor.

Testes `AutomaticPublication` cobrem admissão sem representante, publicação
sem leitura por token, terminal antes do retorno, callback antigo, falha de leitura
após commit, recuperação com buffers novos, cancelamento/retry e limite/isolamento
da base, deadline após commit e desvinculação sem bloquear outro turno, inclusive
após terminal confirmado, replay recusado durante falha, recuperação com outro turno
pendente e limites antes da leitura (inclusive ledger legado). Passaram 309 testes seguros de chat, 137 de agente e
dez de app; build/vet e lint incremental sem ocorrências novas. Revisor independente
review_aep: três rodadas; corrigidos deadline ausente e bloqueio após desvinculação
nas variantes ativa e concluída. Terceira rodada aprovada sem pendências.
Revisão remota CodeRabbit: corrigidos flush incompleto na recuperação e limite tardio
da base; adicionada a asserção de replay sugerida. Duas rodadas locais adicionais de
review_aep corrigiram a leitura da pergunta e a cobertura de vínculos legados no
preflight; segunda rodada adicional aprovada sem pendências.
Copilot: corrigidas composição do erro sem chunks e durabilidade de terminal não
gravado; regressões com SQLite comparam journal e recuperação nas três variantes.
Sexta rodada local review_aep valida essas correções antes do push.
Segunda rodada Copilot apontou flush entre encarnações e desbloqueio sem flush
no descarte/janela legada. Corrigidos com regressões de publicação, retenção de
bytes, callback antigo e recusa de autorização; sétima revisão local aprovada.
Terceira rodada Copilot apontou expiração de terminais pendentes; retenção
corrigida e regressão temporal aprovada na oitava revisão local.
Quarta rodada Copilot levantou descarte sem correção do journal; a nona revisão
local cobre estado persistido, ausência comprovada, outro bloqueador, tentativa
antiga, idempotência e recusa de autorização em `discard_publication_test.go`.
Nenhum teste ACP executado localmente.

### Recorte: inscrições internas com recuperação explícita (2026-10-31)

O projetor oferece um capability interno por sessão/superfície/conversa/escopo.
A sessão vem de um adaptador confiável que a revalida, não de IDs no pedido da UI.
Abrir inscreve a janela e captura seu snapshot sob a mesma barreira; fechar libera
somente esse capability, inclusive depois de revogação, sem retornar conteúdo.
Há limite de 64 inscrições por conversa. Inscrições mantêm a instância viva.

Refresh é explícito. Replay é restrito aos IDs materializados daquela janela;
lotes sem alteração local continuam avançando a sequência. A posse é reautorizada
no storage e qualquer mudança de versão desde o snapshot exige recuperação,
impedindo ressuscitar itens excluídos por replay antigo. Snapshots não compartilham
memória mutável com publicação. Fechar uma janela não encerra a outra.

Este recorte permanece interno: sem binding Wails e sem emitter global de dados.
Não entrega ainda atualizações automáticas, coalescing de 24 ms, tombstones,
windowPatches, modo followTail/pins ou invalidações de ledger/storage conectadas a
todos os produtores. O adaptador de sessão do transporte e sua integração com
logout/revogação também devem ser conectados antes da API pública. Assim o front
não recebe um contrato parcialmente implementado. A fase 2 continua In Progress.

Testes TimelineSubscriptions/TimelineSubscription cobrem duas superfícies,
fechamento independente, troca/revogação de sessão, superfície duplicada,
recuperação após exclusão, isolamento das cópias e limite de 64 inscrições com
liberação após Close. Revisor independente review_aep: duas rodadas, sem pendências.
Build/vet e lint incremental passaram; 293 testes seguros de chat passaram.
Revalidação de sessão é por chamada; nenhuma entrega é enfileirada neste recorte.
Sem execução local de ACP.

### Recorte: operações canônicas e replay interno (2026-10-31)

Leituras coordenadas de turno/janela passam pela mesma projeção leve e pelo mesmo
codificador de operações. Identidade do item permanece estável; conteúdo crescente
no mesmo segmento gera append, substituição gera replace e mudança estrutural gera
upsert. Comparar prefixos aqui codifica um delta entre segmentos de identidade já
comprovada; não adivinha proveniência nem deduplica fontes.

O journal conserva as próprias operações canônicas, até 256 lotes/2 MiB, com
sequência decimal independente das revisões de storage e runtime. Snapshot/replay
não expõem referências mutáveis do journal. Lote maior perde continuidade de replay
e exige snapshot. O cache de base de diff tem limite separado de 512 itens/8 MiB de
JSON; evicção não remove buffers vivos nem conteúdo do usuário, e a próxima
publicação do item usa upsert. Medição de heap real continua pendente.

Limite explícito: publicação ainda é sob demanda nas leituras coordenadas. Alterações
sem leitura não são publicadas automaticamente. Não oferecer este replay como
sincronização integral nem expor API pública v1 antes da conexão dos produtores,
coalescing, inscrições e invalidações de storage/ledger. Ausência em uma página não
produz tombstone. Metadados/anexos completos e patches de janela seguem pendentes.

Testes `CanonicalPublication`, `CanonicalReplay` e `CanonicalItem` verificam delta,
reset, identidade nullable, limites, evicção, mudança de época, aplicação das operações
contra snapshot e isolamento de cópias. Build/vet e lint incremental passaram;
289 testes seguros de chat passaram. Revisor independente review_aep: desenho
revisado em duas propostas e gate final sem pendências. A revisão remota do
CodeRabbit sugeriu provar a fronteira do replay após evicção: after=0/43 recusados,
after=44 entrega sequências 45–300; após lote excessivo, after=300 recusa e
301 aceita vazio. Cinco testes focados passaram. Nenhum teste ACP local.

### Recorte: cursores canônicos validados no snapshot (2026-10-31)

A janela interna agora fornece e consome cursores opacos versionados com conversa,
escopo e chave total exata do banco. Owner é reautorizado antes da decodificação;
a chave da âncora é comparada na mesma transação da página. Âncora removida ou
movida exige sincronização, sem resposta vazia que pareça fim do histórico.
Cursor não é credencial; entrada malformada, escopo divergente ou âncoras
simultâneas são recusados. O tamanho aceito é limitado a 8 KiB.

`TestCanonicalCursorRoundTripAndInvalidation` e
`TestCanonicalCursorDoesNotGrantAccessOrChangeScope` cobrem ida/volta, mudança de
posição, exclusão, falta de autenticação, outro owner, escopo e entradas inválidas.
Revisor independente `review_aep`: uma rodada sem achados bloqueantes. Sem ACP local.

A API pública aguarda publicação coordenada e inscrições: este recorte não
expõe sequência fictícia nem um DTO provisório. Próximos recortes: mecanismo
único de lotes/snapshot e inscrições/recuperação, antes do cutover do frontend.

### Recorte: janela canônica desde a admissão (2026-10-31)

A consulta canônica inclui os turnos admitidos na mesma classificação SQL de
ordem, contagem e paginação, antes de existir mensagem representante. Pergunta
continua slot 0; resposta mantém slot 1 mesmo em preparing ou falha de criação.
Nenhum registro de mensagem é inventado na leitura. A exclusão da pergunta
remove também o item; âncoras podem identificar diretamente um item sem mensagem.

`ReadCanonicalWindow` usa a mesma barreira de admissão/streaming/commit e a mesma
composição da recuperação de turno. Autorização e escopo precedem qualquer
conteúdo em memória. Turnos fora da página não são confundidos com exclusões.

Compatibilidade temporária: `ReadWindow`/`MessageWindow` continuam contando somente
nós materializáveis pelo consumidor antigo. Ambos usam a mesma consulta e
projeção; `WithTimelineTurns` habilita exclusivamente a inclusão dos turnos sem
conteúdo para a nova fronteira. Remover essa opção e o adaptador antigo no
cutover da fase 3, quando todos os consumidores aceitarem representante nullable.
A etapa seguinte expõe a leitura canônica pelo contrato wire.

Evidências: `TestCanonicalWindowIncludesPreparingAndMissingRepresentativeWithoutMessages`
e `TestCanonicalWindowComposesActiveContentAndSharesTurnRecovery`, além das provas
existentes de barreira, ownership, thread, paginação e contagem. Não houve teste
ACP local nem alteração de mensagens otimistas ou identidade no frontend.
Também passaram as provas de isolamento de thread e `historical_unknown` sem
conteúdo. Build/vet e lint incremental passaram; 281 testes seguros de chat,
31 de banco e 10 de app passaram, além dos quatro casos canônicos finais.
Revisor independente `review_aep`: duas rodadas, sem pendências técnicas.
Revisão remota: paginação `around` com item sem representante e diagnóstico de
âncora ausente cobertos; o erro identifica `anchorItemId` ou `anchorMessageId`.

`BenchmarkCanonicalWindowTurnMembership` (20 operações, cinco amostras,
Windows/amd64, execução isolada) mediu medianas legado/canônico de
4,505/5,447 ms em 100 perguntas; 9,334/17,375 ms em 500; 21,180/26,752 ms
em 1.000. Em 100, retornam respectivamente 102/202 itens; nos demais, 240,
mas a composição difere: o canônico inclui respostas sem mensagem. B/op
canônico: aproximadamente 939 mil/1,062 milhão/1,063 milhão, contra
749 mil/1,539 milhão/1,541 milhão. Esses números revelam custo adicional,
não demonstram cumprimento do orçamento histórico nem uma comparação de
saídas equivalentes. O gate de desempenho permanece aberto: antes do cutover,
comparar saídas equivalentes e avaliar/otimizar a consulta canônica. Não há
SQL por delta neste recorte.

### Recorte: composição de fontes confirmadas e streaming (2026-10-31)

A recuperação coordenada fornece um item composto da execução ativa comprovada,
reutilizando a projeção de segmentos persistida. Cada stream substitui sua fonte
pelo ID confirmado pelo writer; não há comparação de prefixos ou deduplicação por
texto. Conteúdo confirmado usa o snapshot do banco, inclusive após edição.
Geração/execução/iteração identificam drafts. A regra original de substituir seus
IDs na confirmação foi superada pela decisão de identidade de apresentação de
2026-11-05: confirmar preserva IDs/cortes e troca somente a fonte dos bytes. Fonte confirmada ausente exige
recuperação explícita. Tentativas retidas/desvinculadas continuam separadas.

O compositor de mensagens aplica posições válidas de atividade por fonte, mesmo
quando o turno contém ferramentas locais e nativas. Chamadas sem posição válida
permanecem na ordem do ledger; nenhuma chamada é descartada ou duplicada. Esse
algoritmo é compartilhado por leitura persistida, terminal e composição live.

Evidências: `TestCompositionReplacesConfirmedPartialBySourceThenReturnsToStorage`,
`TestCompositionKeepsConfirmedPrefixOnceAndScopesRepeatedIterations`,
`TestCompositionPreservesLiveCutsAndTextIdentityDuringReasoning`,
`TestCompositionRejectsOtherAttemptAndPreservesGenerationIdentity` e
`TestMixedInvocationOriginsKeepKnownTextPositions`. Testes ACP são compilados,
mas não executados localmente; cobertura de origens mistas é executada no CI.

Este recorte não expõe novo RPC nem troca o consumidor visual. Os próximos quatro
recortes deste lote são janela com turnos sem representante, DTO/cursores,
inscrições por superfície e atualização/recuperação do protocolo. A fase 2 e o
cutover frontend permanecem incompletos; não há promessa de foco corrigido.

Validação: chat 278, runtime/recuperação 36 e janelas App 10 testes locais
selecionados passaram; build/vet/lint incremental sem ocorrências. Frontend
idêntico à árvore validada no lote anterior (1a80d4602a8e2fa432d8897ce1be6b46e8a04f3d).
Comparação sequencial 5×20 da janela, 100/500/1.000 mensagens: pai
4,977/12,622/32,177 ms; composição 4,933/12,942/31,882 ms. B/op:
812.847/1.776.597/1.777.231 → 822.723/1.775.402/1.831.903; payload inalterado.
O incremento contra o pai fica abaixo de 3,1%. Ambas as versões estão mais
lentas que a medição histórica neste ambiente, sobretudo no caso 1.000; isso
não comprova atendimento ao gate absoluto original, que permanece aberto para
o encerramento da fase. Não se redefine a baseline nem se declara ganho.

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
O CI identificou duas fixtures de atividade/cronologia ACP ainda sem esses
metadados; ambas passam a aplicar as mesmas migrações. A compilação do pacote
agent foi validada localmente, sem executar ACP; a execução fica no CI.

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
a decisão de 2026-11-05 preserva os IDs de apresentação já publicados. A fonte
persistida e a identidade visual são contratos distintos; confirmação não é reset. Este modelo é interno,
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

- [x] Migrar para itens e janelas por IDs, preservando isolamento das superfícies. Evidências: consumidor canônico e retirada do compositor descritos acima.
- [ ] Remover ordenador híbrido, chave alternativa, reconstrução semântica e espelhamentos substituídos.
- [ ] Unificar corte da janela e eliminar flags derivadas duplicadas.
- [x] Preservar identidade DOM/foco/leitura, inclusive no limiar de virtualização. Evidências: `ChatMessage.test.tsx`, `MessageList.focusRetention.test.tsx` e `ChatSessionLiveState.test.tsx`.
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
