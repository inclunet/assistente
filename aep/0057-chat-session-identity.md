# AEP-0057: Sessões de Superfície e Timeline de Chat

## Status: Draft

## Relação com a AEP-0056

Esta AEP é uma AEP filha da `AEP-0056: Workspace com Abas Autocontidas`.

A AEP-0056 define o alvo guarda-chuva: o workspace deve ser um shell fino, painéis visitados devem poder permanecer montados, e cada domínio deve controlar seu próprio conteúdo. Esta AEP detalha a parte de chat desse alvo: transformar cada painel/superfície de chat em uma unidade independente de UI, sem recriar um singleton global nem duplicar a conversa inteira por aba.

O objetivo final é que painéis de chat sejam totalmente independentes no que é visual e interativo: input, anexos, scroll, foco, seleção, expansão, paginação, erros locais, retry visual e estado de fila. Ao mesmo tempo, quando dois painéis apontam para a mesma conversa, eles compartilham a mesma timeline canônica por `conversationId`.

## Resumo

Separar o chat em três conceitos distintos:

- `ConversationTimeline`: estado canônico e persistido da conversa, identificado por `conversationId`;
- `ChatSurfaceSession`: estado visual e interativo de uma superfície, identificado por `sessionKey`;
- `ConversationTurnQueue`: fila ordenada de execução de turnos por conversa.

O objetivo não é criar um "array de chat stores" indexado pela UI. O objetivo é fazer cada painel de chat enxergar uma sessão autocontida por meio de um provider/controller, enquanto mensagens e eventos persistidos continuam pertencendo a uma timeline compartilhada por `conversationId`.

Esta AEP detalha a fase de chat autocontido prevista na AEP-0056.

## Motivação

A migração do chat para sessões por `conversationId` eliminou o problema mais grave do singleton global, mas ainda deixa um cheiro arquitetural: a UI consulta um registry global por chave em vários componentes. Isso faz a arquitetura parecer um conjunto de stores indexadas manualmente, em vez de painéis autocontidos.

Ao mesmo tempo, duplicar a conversa inteira por aba também seria incorreto. Duas superfícies podem apontar para a mesma conversa: por exemplo, uma aba de chat e um chat embutido no editor usando a mesma `conversationId`. Nessa situação, as duas superfícies devem ver a mesma timeline canônica de mensagens, mas não devem compartilhar scroll, foco, input, seleção, expansões ou erros locais.

Também há uma diferença entre paralelismo entre conversas e concorrência dentro da mesma conversa. Conversas diferentes podem responder em paralelo. A mesma conversa deve preservar ordem de turnos. Quando duas superfícies enviam interações para a mesma conversa, o comportamento correto é serializar esses turnos por uma fila da conversa, e não deixar dois LLMs competirem pela mesma timeline sem ordem definida.

## Decisões

### 1. Separar timeline, sessão visual e fila de turnos

- `conversationId` identifica a `ConversationTimeline`.
- `sessionKey` identifica a `ChatSurfaceSession`.
- `ConversationTurnQueue` é identificada por `conversationId`.

Esses três conceitos podem compartilhar infraestrutura, mas não devem ser confundidos nos componentes de UI.

### 2. Timeline canônica por conversa

Mensagens, título, áudio salvo, tool calls, reasoning persistido, estatísticas e eventos salvos pertencem ao `conversationId`.

Quando duas superfícies exibem a mesma conversa, elas compartilham a timeline canônica. Uma mensagem criada em uma superfície deve aparecer nas outras superfícies interessadas na mesma conversa, respeitando a política de janela/renderização de cada uma.

### 3. Sessão visual por superfície

O estado visual e interativo deve ser escopado por `sessionKey`. A chave deve ser derivada de `tabId + conversationId` quando houver aba, ou de um identificador equivalente de superfície quando a conversa estiver em modal, painel embutido ou canal externo representado na UI.

Devem pertencer a `ChatSurfaceSession`:

- input local e mídias anexadas ainda não enviadas;
- scroll e âncora visual;
- janela de mensagens renderizada;
- cursor/paginação de mensagens antigas;
- expansão de threads;
- expansão de reasoning;
- mensagem em edição;
- modo de leitura/foco;
- erros locais e estado de retry;
- busy/queued state da superfície.

### 4. Provider/controller como fronteira da UI

Componentes de chat não devem consultar diretamente mapas globais como `sessionsByConversationId[conversationId]` ou `sessionsBySessionKey[sessionKey]`.

Cada superfície deve montar um `ChatSessionProvider` ou controller equivalente que recebe `conversationId`, `tabId`/`surfaceId` e `surfaceType`, cria a sessão visual e expõe uma API local, como `useChatSession()`.

O registry global é permitido como infraestrutura interna do domínio de chat, mas não deve ser o modelo mental nem a API primária dos componentes de UI.

### 5. Fila serial por conversa

Conversas diferentes podem processar turnos em paralelo. A mesma conversa deve processar turnos em ordem por uma `ConversationTurnQueue`.

Se duas superfícies enviarem mensagens para a mesma conversa:

- o primeiro turno entra em execução;
- o segundo turno entra na fila da mesma conversa, ou fica bloqueado na UI em uma primeira implementação simples;
- quando o primeiro turno termina, o próximo turno é processado com a timeline atualizada;
- ambas as superfícies interessadas veem a timeline resultante, mantendo estado visual independente.

Processar dois turnos da mesma conversa simultaneamente não é permitido nesta AEP, porque criaria ambiguidade de contexto e ordenação.

### 6. Cancelamento é uma intenção explícita

Novo envio comum não deve cancelar silenciosamente um turno em andamento da mesma conversa.

Cancelamento continua existindo para:

- botão/ação explícita de parar geração;
- barge-in por voz/SIP;
- fluxo explícito de "enviar interrompendo a resposta atual".

O `StreamingManager` ou componente equivalente deve distinguir "registrar novo turno" de "cancelar turno atual".

### 7. Eventos carregam timeline e origem

Eventos de chat continuam globais e devem carregar `conversationId`, conforme AEP-0040.

Eventos iniciados por uma superfície Wails devem carregar também origem quando disponível:

- `sessionKey`;
- `tabId` ou `surfaceId`;
- `surfaceType`;
- `profileSlug`;
- identificador de turno, quando existir.

O roteador frontend deve atualizar a timeline canônica por `conversationId` e notificar as sessões visuais interessadas. Eventos sem origem conhecida, como canais externos, continuam roteados por `conversationId` e `source`.

## Fases

### Fase 1 — Reformular fronteira de sessão

- Criar tipos explícitos para `ConversationTimeline`, `ChatSurfaceSession`, `ConversationTurnQueue`, `ChatSessionKey` e `ChatSurfaceOrigin`.
- Introduzir helpers para derivar `sessionKey` a partir de `tabId`/`surfaceId` e `conversationId`.
- Criar `ChatSessionProvider`/`useChatSession()` como fronteira primária da UI.
- Atualizar testes para duas superfícies apontando para a mesma conversa.

### Fase 2 — Separar timeline de estado visual

- Separar cache/timeline por `conversationId` do estado visual por `sessionKey`.
- Remover acesso direto da UI a registries globais.
- Migrar `ChatSessionView`, `ChatToolbar`, `MessageList`, `MessageNode` e `ChatMessage` para `useChatSession()`.
- Garantir que fechar uma aba remove apenas sua sessão visual.

### Fase 3 — Fila de turnos por conversa

- Introduzir ou adaptar a fila de turnos por `conversationId`.
- Permitir execução paralela entre conversas diferentes.
- Serializar envios da mesma conversa.
- Separar cancelamento explícito de novo envio comum.

### Fase 4 — Contrato de origem e eventos

- Ampliar `SendMessage`/`RetryMessage` para receber origem de superfície sem criar outro método de envio.
- Propagar origem até os eventos `chat:*`.
- Adicionar identificador de turno quando necessário para correlacionar fila, streaming e retry.
- Manter compatibilidade conceitual com canais externos via `source`/`surfaceType`.

### Fase 5 — Loader, paginação e janela

- Ajustar carregamento inicial para preencher timeline por `conversationId`.
- Manter janela, cursor e âncora visual por `sessionKey`.
- Evitar reload desnecessário quando outra superfície já carregou a mesma timeline.
- Preparar a base para a AEP-0059.

### Fase 6 — Retry, erro e UI de fila

- Direcionar erro/retry para a sessão de origem quando houver origem.
- Refletir mensagens persistidas em todas as superfícies interessadas.
- Expor estado de turno em fila ou bloqueado por conversa.
- Validar envio em duas conversas diferentes e envio serializado na mesma conversa.

## Riscos

- Separar timeline e sessão visual aumenta a complexidade inicial do domínio de chat.
- Uma fila por conversa exige UX clara para turnos aguardando execução.
- Eventos sem origem podem atualizar mais sessões do que o necessário.
- Se componentes continuarem consultando registries globais, a arquitetura continuará parecendo um array de stores.
- Cancelamento implícito pode reaparecer se `StreamingManager` continuar tratando novo registro como overwrite.
- Mostrar turnos em fila antes de persistir mensagens pode violar a regra backend-driven se não houver evento backend adequado.

## Critérios de aceitação

- Duas abas com conversas diferentes podem enviar e receber respostas simultaneamente.
- Duas superfícies com a mesma conversa compartilham a timeline canônica de mensagens.
- Duas superfícies com a mesma conversa não compartilham scroll, foco, input, edição, expansão de threads ou erros locais.
- Envio comum para conversa ocupada não cancela silenciosamente o turno atual.
- A mesma conversa processa turnos de forma serializada e ordenada.
- `SendMessage` continua sendo a única chamada de envio frontend-backend.
- Eventos de chat não dependem de `activeConversationId` global.
- Componentes de UI usam `ChatSessionProvider`/`useChatSession()` em vez de acessar registries globais diretamente.
- Fechar uma aba remove apenas sua sessão visual.
- Cache/timeline por `conversationId` não força estado visual compartilhado.
- Testes cobrem isolamento por superfície, timeline compartilhada e fila por conversa.
