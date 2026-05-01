# AEP-0057: Sessoes de Superficie e Timeline de Chat

## Status: Draft

## Resumo

Separar o chat em tres conceitos distintos:

- `ConversationTimeline`: estado canonico e persistido da conversa, identificado por `conversationId`;
- `ChatSurfaceSession`: estado visual e interativo de uma superficie, identificado por `sessionKey`;
- `ConversationTurnQueue`: fila ordenada de execucao de turnos por conversa.

O objetivo nao e criar um "array de chat stores" indexado pela UI. O objetivo e fazer cada painel de chat enxergar uma sessao autocontida por meio de um provider/controller, enquanto mensagens e eventos persistidos continuam pertencendo a uma timeline compartilhada por `conversationId`.

Esta AEP detalha a fase de chat autocontido prevista na AEP-0056.

## Motivação

A migracao do chat para sessoes por `conversationId` eliminou o problema mais grave do singleton global, mas ainda deixa um cheiro arquitetural: a UI consulta um registry global por chave em varios componentes. Isso faz a arquitetura parecer um conjunto de stores indexadas manualmente, em vez de paineis autocontidos.

Ao mesmo tempo, duplicar a conversa inteira por aba tambem seria incorreto. Duas superficies podem apontar para a mesma conversa: por exemplo, uma aba de chat e um chat embutido no editor usando a mesma `conversationId`. Nessa situacao, as duas superficies devem ver a mesma timeline canonica de mensagens, mas nao devem compartilhar scroll, foco, input, selecao, expansoes ou erros locais.

Tambem ha uma diferenca entre paralelismo entre conversas e concorrencia dentro da mesma conversa. Conversas diferentes podem responder em paralelo. A mesma conversa deve preservar ordem de turnos. Quando duas superficies enviam interacoes para a mesma conversa, o comportamento correto e serializar esses turnos por uma fila da conversa, e nao deixar dois LLMs competirem pela mesma timeline sem ordem definida.

## Decisões

### 1. Separar timeline, sessao visual e fila de turnos

- `conversationId` identifica a `ConversationTimeline`.
- `sessionKey` identifica a `ChatSurfaceSession`.
- `ConversationTurnQueue` e identificada por `conversationId`.

Esses tres conceitos podem compartilhar infraestrutura, mas nao devem ser confundidos nos componentes de UI.

### 2. Timeline canonica por conversa

Mensagens, titulo, audio salvo, tool calls, reasoning persistido, estatisticas e eventos salvos pertencem ao `conversationId`.

Quando duas superficies exibem a mesma conversa, elas compartilham a timeline canonica. Uma mensagem criada em uma superficie deve aparecer nas outras superficies interessadas na mesma conversa, respeitando a politica de janela/renderizacao de cada uma.

### 3. Sessao visual por superficie

O estado visual e interativo deve ser escopado por `sessionKey`. A chave deve ser derivada de `tabId + conversationId` quando houver aba, ou de um identificador equivalente de superficie quando a conversa estiver em modal, painel embutido ou canal externo representado na UI.

Devem pertencer a `ChatSurfaceSession`:

- input local e midias anexadas ainda nao enviadas;
- scroll e ancora visual;
- janela de mensagens renderizada;
- cursor/paginacao de mensagens antigas;
- expansao de threads;
- expansao de reasoning;
- mensagem em edicao;
- modo de leitura/foco;
- erros locais e estado de retry;
- busy/queued state da superficie.

### 4. Provider/controller como fronteira da UI

Componentes de chat nao devem consultar diretamente mapas globais como `sessionsByConversationId[conversationId]` ou `sessionsBySessionKey[sessionKey]`.

Cada superficie deve montar um `ChatSessionProvider` ou controller equivalente que recebe `conversationId`, `tabId`/`surfaceId` e `surfaceType`, cria a sessao visual e expoe uma API local, como `useChatSession()`.

O registry global e permitido como infraestrutura interna do dominio de chat, mas nao deve ser o modelo mental nem a API primaria dos componentes de UI.

### 5. Fila serial por conversa

Conversas diferentes podem processar turnos em paralelo. A mesma conversa deve processar turnos em ordem por uma `ConversationTurnQueue`.

Se duas superficies enviarem mensagens para a mesma conversa:

- o primeiro turno entra em execucao;
- o segundo turno entra na fila da mesma conversa, ou fica bloqueado na UI em uma primeira implementacao simples;
- quando o primeiro turno termina, o proximo turno e processado com a timeline atualizada;
- ambas as superficies interessadas veem a timeline resultante, mantendo estado visual independente.

Processar dois turnos da mesma conversa simultaneamente nao e permitido nesta AEP, porque criaria ambiguidade de contexto e ordenacao.

### 6. Cancelamento e uma intencao explicita

Novo envio comum nao deve cancelar silenciosamente um turno em andamento da mesma conversa.

Cancelamento continua existindo para:

- botao/acao explicita de parar geracao;
- barge-in por voz/SIP;
- fluxo explicito de "enviar interrompendo a resposta atual".

O `StreamingManager` ou componente equivalente deve distinguir "registrar novo turno" de "cancelar turno atual".

### 7. Eventos carregam timeline e origem

Eventos de chat continuam globais e devem carregar `conversationId`, conforme AEP-0040.

Eventos iniciados por uma superficie Wails devem carregar tambem origem quando disponivel:

- `sessionKey`;
- `tabId` ou `surfaceId`;
- `surfaceType`;
- `profileSlug`;
- identificador de turno, quando existir.

O roteador frontend deve atualizar a timeline canonica por `conversationId` e notificar as sessoes visuais interessadas. Eventos sem origem conhecida, como canais externos, continuam roteados por `conversationId` e `source`.

## Fases

### Fase 1 — Reformular fronteira de sessao

- Criar tipos explicitos para `ConversationTimeline`, `ChatSurfaceSession`, `ConversationTurnQueue`, `ChatSessionKey` e `ChatSurfaceOrigin`.
- Introduzir helpers para derivar `sessionKey` a partir de `tabId`/`surfaceId` e `conversationId`.
- Criar `ChatSessionProvider`/`useChatSession()` como fronteira primaria da UI.
- Atualizar testes para duas superficies apontando para a mesma conversa.

### Fase 2 — Separar timeline de estado visual

- Separar cache/timeline por `conversationId` de estado visual por `sessionKey`.
- Remover acesso direto da UI a registries globais.
- Migrar `ChatSessionView`, `ChatToolbar`, `MessageList`, `MessageNode` e `ChatMessage` para `useChatSession()`.
- Garantir que fechar uma aba remove apenas sua sessao visual.

### Fase 3 — Fila de turnos por conversa

- Introduzir ou adaptar a fila de turnos por `conversationId`.
- Permitir execucao paralela entre conversas diferentes.
- Serializar envios da mesma conversa.
- Separar cancelamento explicito de novo envio comum.

### Fase 4 — Contrato de origem e eventos

- Ampliar `SendMessage`/`RetryMessage` para receber origem de superficie sem criar outro metodo de envio.
- Propagar origem ate os eventos `chat:*`.
- Adicionar identificador de turno quando necessario para correlacionar fila, streaming e retry.
- Manter compatibilidade conceitual com canais externos via `source`/`surfaceType`.

### Fase 5 — Loader, paginacao e janela

- Ajustar carregamento inicial para preencher timeline por `conversationId`.
- Manter janela, cursor e ancora visual por `sessionKey`.
- Evitar reload desnecessario quando outra superficie ja carregou a mesma timeline.
- Preparar a base para a AEP-0059.

### Fase 6 — Retry, erro e UI de fila

- Direcionar erro/retry para a sessao de origem quando houver origem.
- Refletir mensagens persistidas em todas as superficies interessadas.
- Expor estado de turno em fila ou bloqueado por conversa.
- Validar envio em duas conversas diferentes e envio serializado na mesma conversa.

## Riscos

- Separar timeline e sessao visual aumenta a complexidade inicial do dominio de chat.
- Uma fila por conversa exige UX clara para turnos aguardando execucao.
- Eventos sem origem podem atualizar mais sessoes do que o necessario.
- Se componentes continuarem consultando registries globais, a arquitetura continuara parecendo um array de stores.
- Cancelamento implicito pode reaparecer se `StreamingManager` continuar tratando novo registro como overwrite.
- Mostrar turnos em fila antes de persistir mensagens pode violar a regra backend-driven se nao houver evento backend adequado.

## Critérios de aceitação

- Duas abas com conversas diferentes podem enviar e receber respostas simultaneamente.
- Duas superficies com a mesma conversa compartilham a timeline canonica de mensagens.
- Duas superficies com a mesma conversa nao compartilham scroll, foco, input, edicao, expansao de threads ou erros locais.
- Envio comum para conversa ocupada nao cancela silenciosamente o turno atual.
- A mesma conversa processa turnos de forma serializada e ordenada.
- `SendMessage` continua sendo a única chamada de envio frontend-backend.
- Eventos de chat não dependem de `activeConversationId` global.
- Componentes de UI usam `ChatSessionProvider`/`useChatSession()` em vez de acessar registries globais diretamente.
- Fechar uma aba remove apenas sua sessao visual.
- Cache/timeline por `conversationId` nao força estado visual compartilhado.
- Testes cobrem isolamento por superficie, timeline compartilhada e fila por conversa.
