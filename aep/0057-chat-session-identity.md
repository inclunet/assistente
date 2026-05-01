# AEP-0057: Identidade de Sessao de Chat por Aba

## Status: Draft

## Resumo

Definir a identidade canônica de uma sessão de chat no frontend como `tabId + conversationId`, e não apenas `conversationId`.

O `conversationId` continua sendo a identidade persistida da conversa no backend e no banco. O `tabId` passa a identificar a instância visual e interativa da conversa dentro do workspace. Essa separação permite que cada aba seja autocontida, mantenha seu próprio estado visual e participe de eventos globais sem depender de um singleton de chat ativo.

Esta AEP detalha a fase de chat autocontido prevista na AEP-0056.

## Motivação

A migração do chat para sessões por `conversationId` eliminou o problema mais grave do singleton global, mas ainda deixa uma ambiguidade arquitetural: duas abas que apontem para a mesma conversa compartilham a mesma sessão frontend.

Esse compartilhamento pode ser útil para dados persistidos, mas é inadequado para estado visual e ciclo de vida de UI. Scroll, seleção, foco, expansão de threads, mensagem em edição, leitura virtual, loading local e estado de streaming pertencem à instância da aba. Quando isso fica indexado só por `conversationId`, uma aba pode interferir em outra ou impedir que o workspace trate cada painel como autocontido.

A decisão `tabId + conversationId` também cria a base para TTS/STT/announcer saberem a origem real de uma ação e para permitir envio paralelo entre superfícies sem depender de uma conversa ativa global.

## Decisões

### 1. Separar identidade persistida e identidade de sessão

- `conversationId` identifica a conversa persistida no backend.
- `tabId` identifica a instância da aba no workspace.
- `sessionKey` identifica a sessão frontend e deve ser derivada de `tabId + conversationId`.

O formato da chave é detalhe interno do frontend. A regra arquitetural é que qualquer estado visual ou interativo da sessão use a identidade composta.

### 2. Dados persistidos continuam por conversa

Mensagens, título, áudio salvo, tool calls, reasoning persistido e estatísticas continuam pertencendo ao `conversationId`.

Quando duas abas exibirem a mesma conversa, elas podem compartilhar dados persistidos carregados do backend, mas não devem compartilhar estado visual ou ciclo de vida de UI.

### 3. Estado visual passa a ser por sessão de aba

Devem ser escopados por `sessionKey`:

- `isLoading` e busy state da superfície;
- `streamingMessageId` e segmentos em construção;
- janela de mensagens carregada;
- cursor/paginação de mensagens antigas;
- scroll e âncora visual;
- expansão de threads;
- expansão de reasoning;
- mensagem em edição;
- modo de leitura/foco;
- erros de envio e retry local.

### 4. Eventos continuam globais, mas são roteados por origem

O backend continua emitindo eventos globais com `conversationId`, conforme AEP-0040. O frontend deve rotear esses eventos para todas as sessões interessadas naquela conversa.

Eventos que representem uma ação iniciada por aba devem carregar, quando disponível, metadado de origem (`tabId`, `surfaceId` ou equivalente). Quando esse metadado não existir, o roteador deve atualizar as sessões interessadas pelo `conversationId` sem assumir aba ativa global.

### 5. Envio usa sessão de origem

Enviar mensagem sempre parte de uma sessão de aba. A chamada compartilhada ao backend continua sendo única (`SendMessage`), mas o frontend deve associar o envio ao `sessionKey` de origem para:

- liberar ou bloquear apenas a superfície correta;
- anexar `surfaceStateJson` e `surfaceContextJson` corretos;
- direcionar erros e retry para a aba que iniciou o envio;
- permitir envio simultâneo em outras sessões compatíveis.

### 6. Cache compartilhado é permitido, estado visual compartilhado não

É permitido manter cache de dados persistidos por `conversationId` para evitar chamadas duplicadas. Esse cache deve ser tratado como fonte de dados, não como sessão de UI.

## Fases

### Fase 1 — Tipos e registry

- Criar tipo explícito para `ChatSessionKey`.
- Introduzir helpers para derivar `sessionKey` a partir de `tabId` e `conversationId`.
- Separar no registry o que é cache por conversa e o que é sessão por aba.
- Atualizar testes unitários para cobrir duas abas apontando para a mesma conversa.

### Fase 2 — Store e selectors

- Migrar `sessionsByConversationId` para estrutura por `sessionKey`.
- Expor selectors por `tabId + conversationId`.
- Remover fallbacks que consultem uma conversa ativa global.
- Garantir que componentes de chat recebam a sessão a partir do contexto do painel.

### Fase 3 — Loader e paginação

- Ajustar carregamento inicial para preencher cache persistido por `conversationId` e sessão visual por `sessionKey`.
- Manter estado de janela/paginação por sessão.
- Evitar reload desnecessário quando outra aba já carregou a mesma conversa.

### Fase 4 — Event router

- Roteador global recebe eventos por `conversationId`.
- Eventos atualizam cache persistido quando aplicável.
- Sessões interessadas são notificadas sem depender da aba ativa.
- Eventos com origem conhecida atualizam primeiro a sessão de origem.

### Fase 5 — Envio paralelo e retry

- Associar `SendMessage` ao `sessionKey` de origem.
- Permitir que sessões independentes enviem em paralelo.
- Garantir que erro e retry fiquem na sessão que iniciou o envio.
- Validar envio simultâneo em duas abas diferentes.

## Riscos

- Duplicar estado persistido por sessão pode aumentar consumo de memória se a separação entre cache e UI não for clara.
- Eventos sem origem por aba podem atualizar mais sessões do que o necessário.
- Duas abas da mesma conversa podem exibir momentos diferentes da mesma timeline se a política de sincronização não for explícita.
- A migração pode reintroduzir dependência de aba ativa se selectors antigos permanecerem no código.

## Critérios de aceitação

- Duas abas com conversas diferentes podem enviar e receber respostas simultaneamente.
- Duas abas com a mesma conversa não compartilham scroll, foco, edição, expansão de threads ou erros locais.
- `SendMessage` continua sendo a única chamada de envio frontend-backend.
- Eventos de chat não dependem de `activeConversationId` global.
- Fechar uma aba remove apenas sua sessão visual.
- Cache persistido por `conversationId` não força estado visual compartilhado.
- Testes cobrem isolamento por `tabId + conversationId`.
