# AEP-0056: Workspace com Abas Autocontidas

## Status: Draft

## Resumo

Reorientar o workspace para ser um shell fino responsável por abas, foco, estado ativo/inativo e keep-alive, deixando cada aba controlar o próprio conteúdo: conversa, documento, terminal ou tasklist.

O objetivo é remover o modelo em que o workspace e stores globais assumem uma única superfície ativa para todos os domínios. Cada aba visitada pode permanecer montada durante a sessão, com controller próprio e estado visual próprio. Serviços verdadeiramente globais, como announcer, TTS e STT, continuam únicos e passam a ser arbitrados por política central.

Esta AEP é a fundação arquitetural do trabalho de abas autocontidas. Ela define o alvo geral e registra a consolidação feita nos PRs #110 e #111:

- AEP-0057: identidade de sessão de chat por `tabId + conversationId`;
- AEP-0058: arbitragem global de announcer, TTS e STT;
- AEP-0059: performance de conversas longas.

Esta AEP usa o número 0056 porque as AEPs 0053, 0054 e 0055 já estavam reservadas ou sendo tratadas em PRs próprios.

## Motivação

A tela de workspace apresenta lentidão ao trocar de abas e ao carregar conversas longas. Parte do problema é local, como renderização de mensagens e hidratação pesada do editor, mas a causa arquitetural maior é que o workspace desmonta a página anterior e vários domínios dependem de stores globais com uma única entidade ativa.

Isso também causa um problema funcional antigo: ao enviar uma mensagem em uma aba, tentar enviar outra em outra aba fica bloqueado como se houvesse um único assistente global ocupado. Conversas diferentes deveriam poder responder em paralelo, desde que cada uma respeite o contrato backend-driven por `conversationId`.

O workspace deve saber quais abas existem e qual está ativa, mas não deve orquestrar detalhes de chat, editor, terminal ou tasklist. Essa separação permite keep-alive, melhora performance percebida e torna possível otimizar cada tipo de conteúdo em etapas posteriores.

## Decisões

### 1. Workspace como shell fino

O workspace passa a ser responsável por:

- lista de abas abertas;
- ordem e fechamento de abas;
- foco e aba ativa;
- estado ativo/inativo dos painéis;
- keep-alive lazy de abas já visitadas;
- persistência mínima e opaca de `tab.state`.

O workspace não deve criar ou sincronizar diretamente conversas, documentos, sessões de terminal ou listas de tarefas. Essa lógica pertence aos controllers de domínio.

### 2. Abas autocontidas por domínio

Cada tipo de aba deve controlar seu conteúdo:

- chat controla sua conversa por `conversationId`;
- editor controla seu documento por `documentId`/`tabId`;
- terminal controla sua sessão por `sessionId`;
- tasklist controla sua lista por `tasklistId`.

Estados visuais como loading, scroll, streaming, expansão de threads, tool calls, seleção e edição devem ser escopados ao controller da aba ou ao conteúdo persistido, não a um singleton global que represente toda a aplicação.

### 3. Keep-alive lazy por aba visitada

Abas não visitadas permanecem inativas e não carregam conteúdo pesado. Ao visitar uma aba pela primeira vez, seu painel é montado. Depois disso, enquanto a aba continuar aberta e dentro da política de cache, o painel permanece vivo e apenas alterna entre ativo e inativo.

Painéis inativos devem ficar fora da navegação por teclado e da árvore de leitores de tela. Eles não podem capturar foco, atalhos locais, microfone ou ações de UI que pertençam à aba ativa.

### 4. Chat por controller de conversa/aba

O chat deixa de depender de uma única conversa ativa global para processar eventos e streaming. Cada aba ou superfície de chat pode instanciar um controller por `conversationId`, com estado próprio de:

- mensagens carregadas;
- janela/paginação de histórico;
- streaming;
- tool calls;
- reasoning;
- scroll;
- expansão de threads;
- edição/leitura da mensagem.

Todos os controllers continuam reutilizando o mesmo contrato compartilhado de envio e retry definido na AEP-0040. Não é permitido duplicar validação, serialização de mídia, montagem de parâmetros ou chamadas divergentes ao backend.

### 5. Eventos filtrados por `conversationId`

Todo evento de chat continua carregando `conversationId`. Um controller só processa eventos da própria conversa. Isso permite que duas abas enviem mensagens e recebam respostas em paralelo sem uma bloquear a outra.

### 6. Serviços globais arbitrados

Alguns recursos continuam globais:

- **Announcer:** existe uma live region global única. Controllers por aba solicitam anúncios a uma política central. A aba ativa pode anunciar progresso normal; abas inativas só anunciam eventos relevantes, como conclusão de resposta ou erro, sempre com contexto de aba/conversa.
- **TTS:** é globalmente exclusivo. Duas abas podem responder em paralelo, mas não podem falar ao mesmo tempo. A arbitragem respeita o perfil/configuração efetiva da aba que originou a fala, ou da aba ativa quando a ação for manual.
- **STT:** captura local de microfone só funciona na aba ativa. Abas inativas em keep-alive não podem ouvir, transcrever nem enviar mensagens por captura local. Canais externos, como Telegram, Slack ou Signal, seguem fluxo backend-driven próprio e independem da aba ativa.

### 7. Otimizações por conteúdo vêm depois da separação

Após separar os controllers por domínio, aplicar otimizações focadas:

- chat com paginação/janela de mensagens e renderização incremental;
- editor sem re-hidratação completa ao voltar para aba já visitada;
- terminal com listeners persistentes por sessão;
- tasklist com loading, erros e expansão escopados por lista.

### 8. Ownership explícito de estado por superfície

O PR de isolamento estrito dos painéis removeu os acoplamentos mais perigosos com entidades globais ativas. O PR seguinte endureceu a fronteira final: stores globais podem existir como cache ou infraestrutura, mas não podem ser a fonte de identidade de uma superfície de UI.

A regra passa a ser:

- `workspaceStore.activeTabId` só representa navegação, foco e visibilidade no shell;
- nenhum painel deve consultar "ativo global" para descobrir seu documento, conversa, terminal ou tasklist;
- estado visual/interativo de painel deve ser chaveado por `tabId`, `surfaceId`, `sessionKey` ou pelo identificador explícito do domínio;
- stores globais só podem expor dados compartilhados ou APIs parametrizadas por ID;
- ações iniciadas por painel devem carregar a identidade explícita da própria superfície.

Isso inclui scroll, seleção, input em edição, erro local, estado de carregamento visual, fila local, modal associado, sessão de voz local e qualquer outro estado que possa divergir entre dois painéis montados ao mesmo tempo.

### 9. Modais, adapters e serviços não definem identidade de painel

Infraestruturas globais como modal store, registry de adapters, announcer, TTS e STT podem continuar globais quando o recurso for naturalmente único. Porém, elas não podem inferir a origem por `activeTabId` no momento da ação.

Quando uma ação parte de um painel, a identidade deve ser capturada no ponto de origem e propagada explicitamente:

- modal de chat recebe `boundTabId`/`surfaceId`;
- adapters são registrados por `tabId` e não por "aba ativa";
- handlers globais validam `isActive` apenas para permissão de captura/foco, não para descobrir o alvo de dados;
- fechamento, retry, envio, interrupção e persistência sempre operam sobre IDs explícitos.

O uso de um singleton global para orquestrar um modal é aceitável como passo intermediário, desde que o singleton seja apenas transporte de estado já vinculado a uma superfície. A evolução preferida é migrar modais e painéis embutidos para controllers por superfície, preservando estado por `tabId`/`surfaceId` quando isso for necessário para UX.

## Fases

### Fase 1 — Contrato arquitetural

- Atualizar AEP-0040 para permitir controllers por aba/conversa, mantendo contrato compartilhado de envio.
- Atualizar instruções de agentes (`CLAUDE.md`, `.github/copilot-instructions.md`) para refletir a nova regra.
- Registrar esta AEP como plano guarda-chuva da mudança.
- Registrar AEPs filhas para os próximos PRs: AEP-0057, AEP-0058 e AEP-0059.

### Fase 2 — Workspace shell e keep-alive

- Introduzir um host de painéis com lazy mount por aba visitada.
- Expor contrato `active/inactive` para cada painel.
- Garantir que painéis inativos fiquem fora de foco, atalhos locais e árvore acessível.
- Reduzir acoplamentos do workspace com detalhes de DOM dos domínios.

### Fase 3 — Controllers por domínio

- Mover bridges de chat, editor, terminal e tasklist para controllers de domínio.
- Manter `tab.state` como metadado opaco persistido pelo workspace.
- Remover sincronizações específicas de domínio do shell do workspace quando houver controller equivalente.

### Fase 4 — Chat autocontido por conversa

Esta fase passa a ser detalhada pela AEP-0057.

- Definir identidade canônica de sessão como `tabId + conversationId`.
- Separar estado visual e streaming por instância de aba.
- Permitir envio simultâneo em conversas diferentes.
- Garantir que retry e nova mensagem continuem delegando ao contrato compartilhado.

### Fase 5 — Arbitragem global de acessibilidade e voz

Esta fase passa a ser detalhada pela AEP-0058.

- Centralizar política de anúncios para aba ativa/inativa.
- Garantir TTS exclusivo com fila/arbitragem e perfil efetivo da aba origem.
- Garantir STT local apenas na aba ativa.

### Fase 6 — Otimizações específicas

Esta fase passa a ser detalhada pela AEP-0059.

- Adicionar paginação de mensagens e carregamento incremental.
- Ajustar scroll e renderização de conversas longas.
- Evitar re-hidratação pesada do editor.
- Persistir eventos/histórico de terminal por sessão.
- Escopar estado visual de tasklist por lista.

### Fase 7 — Hardening de ownership por superfície

- Auditar stores globais para separar cache compartilhado de estado visual por superfície.
- Exigir APIs parametrizadas por ID para ações de domínio.
- Remover qualquer inferência de identidade baseada em `activeTabId` fora do shell de workspace.
- Transformar modal/adapters globais em infraestrutura vinculada a `tabId`/`surfaceId`.
- Adicionar testes com dois painéis reais do mesmo domínio montados simultaneamente.
- Documentar no código que `activeTabId` é contrato de navegação, não contrato de dados.

#### Consolidação no PR #110

O PR #110 conclui o hardening planejado na Fase 7 sem manter contratos de transição:

- APIs de abertura de modal e ações de painel recebem `tabId`/`surfaceId` explícitos; não há fallback por `getActiveTab()` para identidade de domínio.
- Hooks globais que ainda observam aba ativa usam essa informação apenas para navegação, visibilidade, foco ou título da janela.
- `activeDocumentId` e `activeSessionId` foram removidos das stores de editor e terminal; painéis operam por `documentId`/`sessionId` explícitos.
- Controles compartilhados, toolbars e atalhos encaminham a identidade do painel/superfície no ponto de origem da ação.
- Testes de regressão cobrem superfícies simultâneas para evitar vazamento de scroll, modal, retry e origem de envio entre instâncias.

#### Consolidação no PR #111

O PR #111 completa a remoção de dependências residuais de superfície global:

- Atalhos globais de editor e tasklist respeitam o estado ativo/inativo do painel e não disparam ações em painéis preservados por keep-alive.
- Estados visuais de carregamento e erro de terminal/tasklist são chaveados por sessão/lista, não por singleton global da aplicação.
- O chat não possui perfil global de contexto nem ativação implícita ao carregar conversa; perfil efetivo, sessão e origem são calculados na superfície que iniciou a ação.
- Deep links e canais externos entram no mesmo contrato de fila por conversa e carregam `ChatSurfaceOrigin` explícito.
- Eventos backend-driven de chat propagam origem de superfície até streaming, tool calls, conclusão e mensagens prontas.
- Announcer, som de recebimento e origem de voz usam a origem do evento ou da superfície, sem recalcular identidade a partir da aba ativa.

#### Validação no PR #112

O PR #112 não altera o alvo arquitetural nem entra na AEP-0059. Ele endurece a prova do contrato vigente:

- Eventos de chat com `surfaceOrigin` são validados até anúncios, som de recebimento e `chat:speak`.
- Requests assíncronos de terminal não recriam estado visual quando a sessão é fechada durante o carregamento.
- Efeitos globais de feedback ignoram origens vinculadas a abas do workspace que já foram removidas.
- Documentação antiga de contexto de superfície passa a apontar explicitamente para o contrato vigente das AEPs 0056, 0057 e 0058.

## Riscos

- Keep-alive pode aumentar uso de memória se muitas abas pesadas permanecerem montadas.
- Painéis inativos podem continuar executando efeitos indevidos se o contrato `active/inactive` não for respeitado.
- Múltiplos controllers de chat podem duplicar listeners se a limpeza de ciclo de vida não for rigorosa.
- Announcer, TTS e STT podem gerar ruído ou conflito se não houver arbitragem central.
- Mudanças futuras precisam preservar o contrato backend-driven de mensagens.
- Singletons globais de UI podem voltar a introduzir dependência acidental de `activeTabId` se não carregarem `tabId`/`surfaceId` explicitamente.
- Stores globais podem misturar cache compartilhado e estado visual por superfície se a fronteira de ownership não for clara.

## Critérios de aceitação

- Trocar entre abas já visitadas não remonta conteúdo pesado desnecessariamente.
- Workspace não orquestra detalhes internos de chat, editor, terminal ou tasklist.
- Duas abas com conversas diferentes podem enviar mensagens em paralelo.
- Eventos de chat são processados apenas pelo controller do `conversationId` correspondente.
- Aba inativa não captura foco, atalhos locais ou microfone.
- Existe apenas uma live region global para anúncios.
- TTS não fala duas respostas ao mesmo tempo e respeita o perfil efetivo da aba origem.
- STT local só funciona na aba ativa.
- Conversas longas carregam e renderizam de forma incremental conforme a AEP-0059.
- Cada commit do PR mantém build/lint/testes focados em estado revisável.
- Estado visual/interativo divergente entre painéis é sempre chaveado por `tabId`, `surfaceId`, `sessionKey` ou ID explícito de domínio.
- Ações de painel não dependem de `activeTabId` para descobrir o alvo de dados.
- Modais e adapters globais, quando existirem, são vinculados a uma superfície explícita antes de executar preparação, envio ou persistência.
