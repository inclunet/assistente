# AEP-0059: Performance de Conversas Longas

## Status: Draft

## Relação com a AEP-0056

Esta AEP é uma AEP filha da `AEP-0056: Workspace com Abas Autocontidas`.

A AEP-0056 define que abas visitadas devem poder permanecer montadas e que conversas longas não devem degradar a troca de abas. A AEP-0057 define a separação entre timeline compartilhada e sessão visual por superfície. Esta AEP detalha a consequência de performance: cada painel de chat precisa poder manter sua própria janela, scroll e âncora sem obrigar outros painéis da mesma conversa a renderizarem ou navegarem no mesmo ponto.

O objetivo é que painéis de chat sejam independentes também sob carga: uma conversa com centenas de mensagens não deve forçar remount, rerender pesado, scroll compartilhado ou paginação compartilhada entre superfícies.

## Resumo

Definir a estratégia para carregar, renderizar e navegar conversas longas sem degradar a troca de abas ou a interação do chat.

O objetivo é combinar carregamento incremental, janela visual por sessão, virtualização progressiva e memoização por item renderizado. A separação por sessão definida na AEP-0057 deve garantir que cada aba mantenha sua própria janela visual, enquanto os dados persistidos continuam associados ao `conversationId`.

Importante: a unidade semântica da lista não é uma linha bruta de `chat_messages`. A unidade acessível e navegável é um **item de timeline**. Mensagens normais podem ocupar um item; um turno com chamadas de ferramenta pode ser consolidado em um único item, mesmo que internamente tenha múltiplas mensagens persistidas.

Esta AEP detalha a fase de otimizações específicas prevista na AEP-0056.

## Motivação

Conversas com 100 a 500+ mensagens pressionam a interface em várias camadas:

- payload inicial grande;
- transformação de árvore de mensagens;
- renderização de Markdown, tool calls e reasoning;
- efeitos de foco e navegação por teclado;
- cálculo de scroll;
- atualização de streaming em listas grandes;
- remount ao alternar abas sem keep-alive.

O PR de abas autocontidas reduz remount e introduz carregamento recente, mas ainda não resolve completamente o custo de renderização quando muitas mensagens estão presentes no DOM. A próxima etapa precisa transformar a lista de mensagens em uma superfície incremental e previsível.

## Decisões

### 1. Carregamento inicial limitado

Ao abrir uma conversa, a UI deve carregar apenas uma janela recente de mensagens.

O tamanho inicial deve ser configurável por constante interna, com valor conservador suficiente para contexto visual imediato. Mensagens antigas são carregadas sob demanda.

### 2. Paginação por direção

A conversa deve suportar carregamento incremental para trás, a partir da mensagem mais antiga presente na janela.

O estado de paginação pertence à sessão visual (`sessionKey`) porque duas abas podem estar em pontos diferentes da mesma conversa.

### 2.1. Janela canônica por item de timeline

Esta fase é uma evolução dedicada e posterior à primeira entrega de janela incremental. A entrega inicial pode paginar mensagens raiz persistidas para reduzir payload, separar janelas por sessão e habilitar navegação incremental. Ela não deve ser tratada como a implementação final da semântica de timeline items.

Na fase 2.1, `GetConversationMessageWindow` deve passar a ser entendido como uma API de janela de timeline, não como uma API de linhas cruas do banco.

Cada item de timeline representa exatamente uma entrada navegável na lista:

- uma mensagem raiz normal sem consolidação;
- um turno consolidado identificado por `turnId`;
- no futuro, outro tipo explícito de item, se houver uma decisão arquitetural para isso.

Quando a fase 2.1 for implementada, `totalCount`, `startIndex`, `endIndex`, `hasBefore` e `hasAfter` devem ser calculados sobre itens de timeline. Eles não devem contar mensagens internas de tool calling separadamente quando a UI renderiza esse conjunto como um único item.

O backend será responsável por montar essa unidade canônica. O frontend deve consumir a janela pronta e usar os índices retornados para acessibilidade, sem recalcular posições absolutas a partir de mensagens brutas.

### 2.2. Agrupamento por turno

O identificador de turno (`turnId`) é a chave lógica para consolidar mensagens de um ciclo de resposta.

A convenção atual deve ser preservada: `turnId` aponta para o ID da mensagem do usuário que iniciou o turno. A mensagem do usuário em si pode existir como item próprio, e as mensagens subsequentes do assistente/tool que carregam esse `turnId` formam o item consolidado de resposta.

Durante streaming, o backend deve tornar essa relação explícita nos eventos do turno. Eventos como `chat:messages_ready`, `chat:stream`, `chat:tool_start`, `chat:tool_end` e `chat:done` devem carregar ou permitir derivar de forma inequívoca o `turnId`. O frontend pode manter um item transitório local durante streaming, mas esse item precisa ser reconciliável pelo mesmo `turnId` quando a janela persistida for recarregada.

### 2.3. Consulta em lote, sem N+1

Montar itens de timeline no backend não deve significar carregar a conversa inteira nem executar uma consulta por item.

A estratégia esperada é:

- paginar primeiro os identificadores dos itens de timeline da janela;
- contar o total de itens pela mesma unidade lógica;
- buscar em lote as mensagens internas dos turnos presentes na janela (`id IN (...)` e/ou `turn_id IN (...)`);
- montar os segmentos em memória preservando ordenação por `created_at, id`.

Para uma janela de N itens, o número de consultas deve ser pequeno e previsível. O contrato não deve depender de pós-processamento frágil no frontend para corrigir contagem, posição ou grouping.

### 3. Cache persistido separado da janela visual

Dados carregados do backend podem ser cacheados por `conversationId`, mas a janela renderizada pertence à sessão de aba.

Isso evita recarregar dados já buscados sem forçar duas abas a compartilharem posição de scroll, mensagens expandidas ou âncora visual.

### 4. Virtualização progressiva

Quando a quantidade de mensagens renderizadas ultrapassar um limite, `MessageList` deve renderizar apenas os itens visíveis e overscan.

A virtualização deve preservar:

- navegação por teclado entre mensagens;
- `aria-setsize`, `aria-posinset` ou alternativa acessível equivalente;
- foco restaurável ao carregar mensagens antigas;
- ancoragem de scroll ao prepender itens;
- leitura de mensagens por leitores de tela sem expor conteúdo invisível como ativo.

### 5. Streaming fora do caminho pesado

Atualizações de streaming devem afetar apenas a mensagem em construção.

Transformações de árvore, consolidação de turnos e renderização de Markdown não devem recalcular toda a lista a cada token.

Após a fase 2.1, o item de streaming deve seguir a mesma unidade semântica da janela persistida: um item transitório por `turnId`. Tool calls, resultados e texto parcial entram como segmentos desse item, não como múltiplos itens navegáveis independentes.

### 6. Mensagens pesadas sob demanda

Partes caras de uma mensagem devem carregar ou expandir sob demanda quando possível:

- tool calls detalhadas;
- reasoning;
- anexos grandes;
- áudio salvo;
- filhos de thread;
- blocos de Markdown muito grandes.

### 7. Acessibilidade não pode depender de DOM completo

A lista virtualizada deve manter experiência consistente para teclado e leitor de tela. A ausência de todos os nós no DOM não pode quebrar:

- ArrowUp/ArrowDown;
- Home/End;
- PageUp/PageDown;
- Enter para leitura virtual;
- Escape para restaurar foco;
- menus de contexto;
- anúncios de carregamento.

## Fases

### Fase 1 — Medição e limites

- Medir tempo de carregamento, transformação e renderização em conversas sintéticas de 100, 500 e 1000 mensagens.
- Definir limites internos para janela inicial, paginação e ativação de virtualização.
- Criar fixture/testes de performance funcional para conversa longa.

### Fase 2 — Janela por sessão

- Mover estado de janela para a sessão `tabId + conversationId`.
- Separar cache por conversa da lista renderizada.
- Garantir que carregar mensagens antigas em uma aba não altere a janela de outra aba.
- Preservar âncora de scroll ao prepender mensagens.
- Nesta fase, a janela ainda pode usar mensagens raiz persistidas como unidade de paginação. Se houver consolidação local de turnos, a UI deve preferir uma contagem visual honesta na janela renderizada em vez de expor índices absolutos crus incorretos.

### Fase 2.1 — Timeline items canônicos

Esta é a fase alvo do PR dedicado posterior ao PR #113.

- Evoluir `GetConversationMessageWindow` para paginar itens de timeline, não linhas brutas de `chat_messages`.
- Agrupar turnos por `turnId` no backend e retornar nós/segmentos já coerentes com a lista navegável.
- Calcular `totalCount`, `startIndex`, `endIndex`, `hasBefore` e `hasAfter` pela quantidade de itens renderizáveis.
- Garantir que streaming crie um item transitório reconciliável pelo mesmo `turnId`.
- Remover a dependência de consolidação local para definir posições acessíveis.

#### Consolidação no PR #113

O PR #113 concluiu a primeira entrega de janela incremental por sessão:

- `GetConversationMessageWindow` passou a ser a API única de carregamento incremental de conversa e thread.
- Janelas visuais passaram a pertencer à `ChatSurfaceSession`, preservando independência entre superfícies.
- O carregamento inicial e a paginação deixaram de depender de carregar a conversa inteira.
- A UI passou a usar contagem local honesta quando a consolidação visual de turnos ainda era feita no frontend, evitando anunciar posições absolutas cruas incorretas.
- A expansão de fronteiras de turno foi mantida apenas como mitigação temporária, não como contrato final de timeline item.

#### Contrato do PR de Fase 2.1

O PR de Fase 2.1 conclui a semântica canônica de itens de timeline:

- A unidade de paginação, contagem e acessibilidade é o item de timeline.
- Um item normal representa uma mensagem navegável sem consolidação.
- Um item de turno representa as mensagens persistidas com o mesmo `turnId`, normalmente mensagens de assistant/tool produzidas pela resposta a uma mensagem de usuário.
- `turnId` continua apontando para o ID da mensagem de usuário que iniciou o turno.
- `anchorMessageId` pode apontar para a mensagem representante ou para uma mensagem interna de um turno; o backend normaliza isso para o item de timeline correspondente.
- `originalIndex`, `totalCount`, `startIndex`, `endIndex`, `hasBefore` e `hasAfter` são calculados pelo backend sobre itens de timeline.
- O frontend consome esses índices como canônicos e não corrige posições absolutas com agrupamento local.
- O backend monta os itens em lote, com número pequeno e previsível de consultas por janela (contagem, normalização opcional de âncora e busca em lote das mensagens internas), sem uma consulta por item.

#### Decisão pós-Fase 2.1 sobre virtualização

Após a Fase 2.1, a próxima decisão de performance deve ser baseada em medição com a nova unidade canônica. Virtualização acessível continua pertencendo às Fases 4 e 5, mas não deve entrar no mesmo PR da Fase 2.1 salvo se os testes de conversa longa ainda mostrarem renderização perceptivelmente bloqueante com a janela já limitada.

Critério prático:

- Se a janela canônica limitada mantiver a UI responsiva em conversas sintéticas de 500+ mensagens, virtualização fica em PR separado.
- Se a renderização da própria janela continuar pesada, o próximo PR deve implementar virtualização acessível antes de expandir features que aumentem conteúdo renderizado.
- A decisão deve preservar `aria-posinset`/`aria-setsize` canônicos e navegação por teclado independente do DOM completo.

### Fase 3 — Memoização e atualização granular

- Garantir que `MessageNode` renderize novamente apenas quando sua mensagem ou estado visual local mudar.
- Separar estado de streaming da lista consolidada sempre que possível.
- Evitar recriar arrays e callbacks globais em cada token.
- Cobrir regressões com testes de render ou contadores em ambiente de teste.

### Fase 4 — Virtualização acessível

- Introduzir virtualização em `MessageList` atrás de um limite.
- Implementar navegação por teclado independente de todos os elementos estarem montados.
- Garantir foco e leitura virtual para itens materializados sob demanda.
- Validar com e2e de teclado e acessibilidade.

### Fase 5 — Conteúdo pesado sob demanda

- Carregar filhos de thread apenas quando expandidos.
- Manter tool calls e reasoning colapsados sem render caro inicial.
- Adiar áudio e anexos grandes até interação explícita.
- Evitar parse/render completo de Markdown fora da janela visível.

## Riscos

- Virtualização pode quebrar navegação por teclado se assumir que todos os nós existem no DOM.
- Leitores de tela podem perder contexto se a lista virtualizada não expuser tamanho e posição adequados.
- Prepender mensagens antigas pode deslocar scroll se a âncora não for preservada.
- Memoização incorreta pode deixar streaming ou edição visualmente stale.
- Cache por conversa pode crescer demais em sessões longas se não houver política de descarte.
- Contar linhas brutas do banco enquanto a UI navega itens consolidados causa anúncios incorretos como saltos de posição (`1 de 100`, `5 de 100`, `18 de 100`).
- Agrupar turnos apenas no frontend é frágil em paginação parcial, streaming, retries e janelas que começam ou terminam no meio de um turno.

## Critérios de aceitação

- Abrir conversa longa não bloqueia a UI de forma perceptível.
- Trocar entre abas visitadas não força rerender pesado da conversa inteira.
- Carregar mensagens antigas preserva posição de leitura.
- Streaming em conversa longa atualiza apenas a mensagem relevante.
- Navegação por teclado continua passando nos e2e existentes.
- Leitores de tela recebem anúncios de carregamento e posição de forma consistente.
- Duas abas da mesma conversa podem ter janelas visuais diferentes.
- Testes cobrem conversas com pelo menos 500 mensagens sintéticas.
- Na fase 2.1, `aria-posinset` e `aria-setsize` refletem a posição e o total de itens navegáveis, não a quantidade de mensagens internas persistidas.
- Na fase 2.1, turnos com tool calls são anunciados como um único item quando renderizados como um único item.
