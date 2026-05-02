# AEP-0059: Performance de Conversas Longas

## Status: Draft

## Relação com a AEP-0056

Esta AEP é uma AEP filha da `AEP-0056: Workspace com Abas Autocontidas`.

A AEP-0056 define que abas visitadas devem poder permanecer montadas e que conversas longas não devem degradar a troca de abas. A AEP-0057 define a separação entre timeline compartilhada e sessão visual por superfície. Esta AEP detalha a consequência de performance: cada painel de chat precisa poder manter sua própria janela, scroll e âncora sem obrigar outros painéis da mesma conversa a renderizarem ou navegarem no mesmo ponto.

O objetivo é que painéis de chat sejam independentes também sob carga: uma conversa com centenas de mensagens não deve forçar remount, rerender pesado, scroll compartilhado ou paginação compartilhada entre superfícies.

## Resumo

Definir a estratégia para carregar, renderizar e navegar conversas longas sem degradar a troca de abas ou a interação do chat.

O objetivo é combinar carregamento incremental, janela de mensagens, virtualização progressiva e memoização por mensagem. A separação por sessão definida na AEP-0057 deve garantir que cada aba mantenha sua própria janela visual, enquanto os dados persistidos continuam associados ao `conversationId`.

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

## Critérios de aceitação

- Abrir conversa longa não bloqueia a UI de forma perceptível.
- Trocar entre abas visitadas não força rerender pesado da conversa inteira.
- Carregar mensagens antigas preserva posição de leitura.
- Streaming em conversa longa atualiza apenas a mensagem relevante.
- Navegação por teclado continua passando nos e2e existentes.
- Leitores de tela recebem anúncios de carregamento e posição de forma consistente.
- Duas abas da mesma conversa podem ter janelas visuais diferentes.
- Testes cobrem conversas com pelo menos 500 mensagens sintéticas.
