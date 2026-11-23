# AEP-0089: Terminais como Recursos Efêmeros

## Status: In Progress

## Resumo

Transformar sessões PTY em recursos efêmeros de primeira classe, independentes
de abas e conversas. Uma aba de terminal é uma visualização conectada a uma
sessão viva; um chat pode listar, criar, escolher, usar, interromper e encerrar
várias sessões. Fechar uma aba apenas desconecta a visualização.

Uma sessão encerrada nunca é reaberta. Seu transcript pode permanecer
disponível para auditoria em modo somente leitura, mas uma nova execução sempre
recebe outro `terminalId`.

## Motivação

O modelo atual vincula implicitamente uma aba a uma sessão criada por ela,
encerra a sessão quando a aba fecha e substitui referências obsoletas por uma
nova sessão sem informar o usuário. A tool `run_command`, por sua vez, reutiliza
qualquer sessão idle do pool. Com isso:

- uma aba não consegue trocar de terminal;
- fechar uma aba mata o processo;
- um chat pode capturar silenciosamente um terminal manual;
- não há identidade confiável para abrir o terminal usado pelo chat;
- deep links podem apontar para uma sessão que será substituída;
- comandos em execução são difíceis de acompanhar ou interromper;
- estados de processo, sessão, comando e visualização ficam misturados.

## Decisões

### 1. Identidade e ciclo de vida

`terminalId` identifica uma única sessão PTY viva. A sessão possui estado
autoritativo no backend: `idle`, `running`, `closing` ou `exited`.

- `interrupt` envia Ctrl+C ao comando atual sem encerrar o shell;
- `terminate` encerra o PTY e invalida definitivamente o `terminalId`;
- `detach` é uma operação de UI e não altera o processo;
- EOF, falha do shell e shutdown transitam a sessão para `exited`;
- nenhuma operação cria outra sessão reutilizando um `terminalId` morto.

O aplicativo não mantém um daemon externo. Ao fechar o Assistente, todos os
PTYs são encerrados. Na próxima execução não existem sessões reconectáveis.

### 2. Sessões independem de abas e conversas

Abas e conversas referenciam sessões, mas não são suas proprietárias.

- uma aba exibe uma sessão por vez e pode trocar sua referência;
- fechar a aba não encerra a sessão;
- chat e aba podem observar e operar a mesma sessão;
- uma conversa pode usar várias sessões;
- no mesmo workspace, abrir uma sessão já exibida foca a aba existente;
- encerrar uma sessão é sempre uma ação explícita do usuário ou do chat.

### 3. Escolha explícita pelo chat

O chat pode listar, criar, selecionar, usar, interromper e encerrar terminais.
Essas ações aparecem como tool calls e produzem feedback acessível.

`run_command` aceita `terminal_id` opcional e `persistent`:

- quando `terminal_id` presente, usa exatamente a sessão indicada;
- quando ausente e `persistent=true`, cria uma nova sessão persistente;
- quando ausente e `persistent` omitido/false (padrão), executa de forma efêmera sem criar seção persistente nem ocupar o limite — não há `terminalId`/`deepLink` persistente;
- nunca adquire silenciosamente uma sessão idle global.

O resultado inclui `terminalId`, `commandId`, estado e, quando há sessão persistente, o deep link
`assistente://terminal/{terminalId}`. A allowlist e a confirmação do comando
continuam obrigatórias; ações de gestão de terminal não adicionam uma segunda
confirmação.

### 4. Protocolo de eventos

Eventos de terminal são contratos tipados e sempre carregam `terminalId`.
Eventos de comando também carregam `commandId`, origem e timestamps.

O backend emite, no mínimo:

- `terminal:session_created`;
- `terminal:command_start`;
- `terminal:command_output`;
- `terminal:command_end`;
- `terminal:raw_output`;
- `terminal:session_closed`;
- `terminal:exited`.

Listeners podem ser globais como infraestrutura, mas todo estado visual é
chaveado por `terminalId` e, quando necessário, por `commandId`/`tabId`.

### 5. Concorrência e integridade do PTY

Uma sessão executa no máximo um comando estruturado por vez. Input manual não é
aceito durante execução com markers, porque alteraria o protocolo e poderia
atribuir output ao comando errado. A UI oferece interrupção e informa por que o
input está indisponível.

### 6. Abas e deep links

A toolbar da aba contém um seletor de terminais vivos, semelhante ao seletor de
conversas, e ações para criar, interromper e encerrar.

`assistente://terminal/{terminalId}`:

- foca a aba existente que já mostra a sessão; ou
- abre uma aba conectada à sessão viva;
- informa claramente quando a sessão não existe ou já encerrou;
- nunca substitui silenciosamente o ID por uma nova sessão.

### 7. Transcript

O transcript é uma projeção observável de comandos e output, não a própria
sessão. Durante a vida do processo ele permite acompanhar um comando mesmo que
a aba seja aberta depois do início. Após o encerramento, pode ser preservado
como registro somente leitura, claramente marcado como encerrado.

A persistência durável de transcripts é opcional e pode ser entregue em fase
posterior. Ela não altera o ciclo de vida de PTYs.

## Estado implementado e pendências

Entregue:

- [x] Tool `terminal_session` com ações `list`, `create`, `interrupt` e `close`
  em `internal/tools/shell/terminal_session.go`; testes em
  `terminal_session_test.go` cobrem listagem, criação, deep link, diretório
  inválido, interrupção e fechamento.
- [x] `run_command` aceita `terminal_id`, usa exatamente a sessão viva indicada,
  rejeita ID morto e cria uma sessão nova quando o campo é omitido; regressões
  ficam em `internal/tools/shell/shell_test.go`.
- [x] API Wails autenticada oferece listar, criar, fechar, executar, enviar
  input, interromper e consultar histórico/estatísticas em
  `internal/wailsapi/terminal.go`, com falha fechada coberta por
  `terminal_test.go`.
- [x] Deep links abrem exatamente uma sessão viva e não substituem IDs mortos,
  coberto por `frontend/src/lib/deepLinks.test.ts`.
- [x] Cleanup de sessões efêmeras após timeout respeita a ordem interrupção,
  término e espera do processo, drenagem do leitor e fechamento do PTY; testes
  concorrentes em `internal/terminal/manager_lifecycle_test.go` cobrem a
  ausência da corrida entre leitor e ConPTY. Na troca de comando, a fila de
  saída é limitada e a escrita pode prosseguir sob fluxo contínuo. O início,
  os blocos já lidos e o encerramento chegam à interface nessa ordem. Cada
  evento de saída bruta identifica a última entrada manual pelo `commandId`,
  vazio se ainda não houve uma. Até o despacho da moldura, os bytes recebidos
  continuam no fluxo manual, inclusive durante o anúncio ou cancelamento.

Pendente ou parcial:

- [ ] Provar por teste que fechar/desconectar a última aba nunca encerra o PTY.
- [ ] Completar acompanhamento de uma execução já em andamento ao conectar nova
  aba, incluindo estado e output sem perda.
- [ ] Fechar isolamento de eventos/stores para múltiplas sessões simultâneas.
- [ ] Concluir feedback acessível, i18n e validação manual NVDA para todas as
  ações e estados.
- [ ] Persistência durável de transcripts continua opcional e fora do escopo
  necessário para concluir as Fases 1–4.

## Fases

### Fase 1 — Domínio e ciclo de vida 🚧

- separar estados de sessão, comando e visualização;
- detectar saída do shell e emitir eventos autoritativos;
- remover aquisição global implícita;
- distinguir interrupção, encerramento e desconexão;
- cobrir concorrência e encerramento com testes Go.

### Fase 2 — Tools e deep links ✅

- adicionar tools de listagem, criação, interrupção e encerramento;
- aceitar `terminal_id` em `run_command`;
- retornar metadados e deep link em toda execução;
- validar deep links contra sessões vivas.

### Fase 3 — Aba conectável 🚧

- adicionar seletor de terminal à toolbar;
- permitir troca sem encerrar a sessão anterior;
- remover recuperação e encerramento implícitos;
- representar sessão encerrada sem fabricar substituta.

### Fase 4 — Timeline e acessibilidade 🚧

- identificar comandos por IDs autoritativos;
- acompanhar streaming ao abrir uma aba durante execução;
- anunciar criação, início, conclusão, erro, interrupção e encerramento;
- validar teclado, foco, NVDA, axe-core e múltiplas superfícies.

### Fase 5 — Evoluções opcionais ⏳

- persistência durável e gestão de transcripts encerrados;
- renderer de terminal completo para programas full-screen e entrada tecla a
  tecla, preservando uma visualização de transcript acessível.

## Riscos

- múltiplas superfícies podem enviar input concorrente se o backend não arbitrar;
- eventos sem `commandId` podem atribuir output à execução errada;
- manter sessões sem abas pode consumir recursos até o limite do manager;
- encerramento pelo chat precisa permanecer visível para não surpreender;
- transcripts podem conter segredos e exigem política própria antes de persistir;
- um terminal visual completo pode regredir acessibilidade se substituir o
  transcript navegável.

### Resultado de comandos no PTY

- **Status.** A sessão entrega o texto entre os marcadores enquanto o processo escreve.
  O rodapé delimita o código numérico para que uma quebra visual entre dígitos preserve o resultado.
  Se o comando encerra o shell, a sessão só atribui o status do processo após observar o marcador
  inicial e drenar a leitura. No PowerShell, um retorno antecipado preserva o status da última
  operação identificável. Um pipeline ou retorno condicional de origem ambígua devolve 1.
  Em `$()` e `@()`, o status reflete a última operação do ramo de `if` executado,
  inclusive em `elseif` e `else` e quando a expressão é retornada por `return`.
  Um ramo ignorado não fornece código de saída.
  O CI exercita o terminal no Windows PowerShell 5.1.
- **Interrupção.** Cancelamento, falha de leitura e fechamento explícito preservam a causa e a saída
  parcial. Se o leitor falhar e o prazo expirar, o resultado conserva as duas causas. A ferramenta
  informa no texto e nos metadados a duração registrada pela sessão, inclusive no timeout sem saída,
  quando os dois horários da execução estão disponíveis.
  Antes da primeira escrita de um comando gerenciado, a sessão revalida o contexto após
  entregar eventos em espera. Se o contexto já terminou, conclui o histórico e o evento
  de término com código -1 e preserva a sessão para o próximo comando.
  Em comandos gerenciados, uma falha de escrita depois do início fica no histórico com código -1.
  Uma falha de escrita encerra a sessão, inclusive quando nenhum byte foi escrito.
  A sessão recusa novas entradas imediatamente. Depois que a escrita devolve erro, o comando
  aguarda a limpeza até o seu prazo ou cancelamento. Se o processo ou leitor continuar bloqueado,
  a limpeza prossegue em segundo plano. A saída já recebida acompanha o erro, inclusive se o prazo vencer.
  Se uma falha do leitor assume o encerramento, uma captura de fechamento anterior fica obsoleta
  e a sessão publica o evento de saída uma única vez.
  Um fechamento solicitado após a saída natural também conserva essa notificação única,
  mesmo quando sua entrega aguarda a conclusão do comando.
  O evento de término do comando precede o de saída da sessão, inclusive quando o próprio
  comando encerra o shell. O histórico já carregado continua recebendo eventos em segundo
  plano. Ao voltar à aba, ele inclui os comandos executados nesse intervalo.
  Na primeira abertura durante um comando, a aba recupera a saída em andamento
  e preserva o resultado que chegar durante a leitura do histórico, inclusive a saída
  manual já exibida quando a consulta devolve o mesmo comando sem saída.
  O evento de início dispara o anúncio do comando. Carregar o histórico é silencioso.
  O backend atribui o ID antes do início do comando.
  O histórico correlaciona eventos pelo ID do comando e preserva inclusive o
  código final -999, distinguindo-o de uma execução ainda sem término. A última
  prévia de saída manual é entregue antes do anúncio de encerramento. A prévia
  falada limita a saída a 300 pontos de código Unicode, inclusive no resultado final.
  Os dois percursos ignoram espaços iniciais antes da contagem e espaços ao fim da prévia.
  Indicam corte apenas quando o excedente contém texto além de espaços em branco.
  Quando a saída excede os 50 KiB de conteúdo por entrada no histórico, `terminal:command_end` inclui
  `announcementPreview` com a prévia do texto original e a indicação de corte.
  Sem saída final, o frontend usa a prévia acumulada antes de limitar o histórico.
  O produtor Go e o consumidor TypeScript validam a mesma tabela de limites e espaços
  Unicode em `internal/terminal/testdata/announcement-previews.json`.
  O editor preserva o rascunho após rejeição do envio. O aceite limpa o texto
  enviado se ele continua no campo, inclusive se a sessão já tiver terminado. Ao carregar a lista,
  mantém o pedido de foco se a origem desaparecer e respeita a navegação da pessoa.
  A área de trabalho escuta o encerramento mesmo com a aba fechada. A abertura da aba
  carrega a lista. O anúncio usa o nome conhecido da sessão ou uma mensagem genérica.
- **Controles do terminal.** O leitor reconhece mudanças de largura fora de sequências de controle.
  Uma DCS pode ocupar várias linhas até seu terminador ou cancelamento. A moldura cancela controles
  pendentes antes das marcas de início e fim. Ao trocar a entrada ativa, a sessão entrega primeiro
  o texto manual pendente. O leitor limita a 4 KiB a cópia literal guardada para recuperar controles
  incompletos. Ultrapassado o limite, consome o restante do controle até a retomada do texto.
- **Shells POSIX e zsh.** Comentários finais e erros de sintaxe ficam contidos no comando.
  O zsh usa suas operações internas para avaliar o comando e conferir o diretório.
  No Linux, sessões efêmeras e sessões persistentes criadas pelo modelo dispensam o completador
  global do Ubuntu. Seu diálogo de segurança pode consumir o primeiro byte do comando. O perfil
  do usuário continua ativo e as sessões criadas manualmente usam a configuração do sistema.

As regressões de encerramento incluem `TestBashExitBeforeEndMarker` e
`TestExplicitCloseAfterStartDoesNotCompleteCommand`. A saída e o status no PowerShell
são cobertos por `TestPowerShellReturnInPersistentSession`,
`TestConPTYRedrawAtStartMarkerPreservesBodyAcrossReads` e
`TestPowerShellFinalPipelineAndConditionalReturnThroughPTY`. Nos shells POSIX,
`TestPOSIXFooterSurvivesCommentsAndSyntaxErrors` verifica o rodapé. O CI Linux inclui
zsh nativo. `TestMarkerStreamConsumesSplitControlSequences` cobre leituras fragmentadas e
`BenchmarkMarkerStreamScale` mede o custo em tamanhos crescentes.

## Critérios de aceitação

- [x] Uma conversa pode criar e usar mais de um terminal pelas tools explícitas.
- [x] O chat pode listar e escolher explicitamente uma sessão.
- [x] Nenhuma tool reutiliza terminal manual sem receber seu `terminalId`;
  `run_command` sem ID cria sessão nova.
- [x] Uma aba pode conectar-se a outra sessão sem fabricar um novo ID.
- [ ] Fechar a última aba de uma sessão não encerra o PTY, com regressão focada.
- [x] Interrupção e encerramento são ações distintas.
- [x] Abrir o deep link de uma sessão viva mostra exatamente aquela sessão.
- [x] Deep link morto informa indisponibilidade e não cria substituta.
- [ ] Abrir uma aba durante comando em execução mostra estado e output atuais.
- [ ] Eventos e stores isolam duas sessões simultâneas.
- [x] Shutdown não promete reconexão a processos mortos.
- [ ] Todas as ações têm feedback acessível, strings nos três idiomas e
  validação correspondente.
