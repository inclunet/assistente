# AEP-0089: Terminais como Recursos Efêmeros

## Status: Draft

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

`run_command` aceita `terminal_id` opcional:

- quando presente, usa exatamente a sessão indicada;
- quando ausente, cria uma nova sessão;
- nunca adquire silenciosamente uma sessão idle global.

O resultado inclui `terminalId`, `commandId`, estado e o deep link
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

## Fases

### Fase 1 — Domínio e ciclo de vida

- separar estados de sessão, comando e visualização;
- detectar saída do shell e emitir eventos autoritativos;
- remover aquisição global implícita;
- distinguir interrupção, encerramento e desconexão;
- cobrir concorrência e encerramento com testes Go.

### Fase 2 — Tools e deep links

- adicionar tools de listagem, criação, interrupção e encerramento;
- aceitar `terminal_id` em `run_command`;
- retornar metadados e deep link em toda execução;
- validar deep links contra sessões vivas.

### Fase 3 — Aba conectável

- adicionar seletor de terminal à toolbar;
- permitir troca sem encerrar a sessão anterior;
- remover recuperação e encerramento implícitos;
- representar sessão encerrada sem fabricar substituta.

### Fase 4 — Timeline e acessibilidade

- identificar comandos por IDs autoritativos;
- acompanhar streaming ao abrir uma aba durante execução;
- anunciar criação, início, conclusão, erro, interrupção e encerramento;
- validar teclado, foco, NVDA, axe-core e múltiplas superfícies.

### Fase 5 — Evoluções opcionais

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

## Critérios de aceitação

- uma conversa pode criar e usar mais de um terminal;
- o chat pode listar e escolher explicitamente uma sessão;
- nenhuma tool reutiliza terminal manual sem receber seu `terminalId`;
- uma aba pode trocar de sessão sem matar a anterior;
- fechar a última aba de uma sessão não encerra o PTY;
- interrupção e encerramento são ações distintas;
- abrir o deep link de uma sessão viva mostra exatamente aquela sessão;
- deep link morto informa indisponibilidade e não cria substituta;
- abrir uma aba durante comando em execução mostra estado e output atuais;
- eventos e stores isolam duas sessões simultâneas;
- shutdown não promete reconexão a processos mortos;
- todas as ações têm feedback acessível e strings nos três idiomas.
