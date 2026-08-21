# AEP-0095: Mermaid acessível e resiliente

- **Status**: In Progress
- **Data**: 2026-08-21

## Resumo

Diagramas Mermaid no chat, no editor e em apresentações devem oferecer
navegação por teclado e leitor de telas quando o tipo puder ser representado
como grafo. Uma falha de sintaxe ou de renderização permanece confinada à
região do diagrama e nunca remove nem bloqueia o restante do conteúdo.

A integração usa `@inclunet/mermaid-a11y` pelas APIs headless. A shell React da
biblioteca não será usada enquanto criar uma live region por instância, pois o
Assistente mantém um único announcer global conforme a AEP-0058.

## Motivação

O renderer atual produz SVG visual, mas não expõe nós e conexões como um widget
navegável. Além disso, erros do Mermaid incluem detalhes técnicos repetitivos,
como a versão da biblioteca, diretamente no fluxo de leitura.

`@inclunet/mermaid-a11y` já fornece extração do grafo, travessia e foco lógico,
mas a versão 0.1.0 exige um elemento para receber anúncios. A API precisa ser
adaptada sem criar regiões concorrentes até que a biblioteca aceite um
callback ou uma live region fornecida pelo host, solicitado na
[issue mermaid-a11y#1](https://github.com/inclunet/mermaid-a11y/issues/1).

## Decisões

### 1. APIs headless e instância Mermaid do host

O Assistente usa `renderAccessibleDiagram` e `createNavigator`, passando a
instância Mermaid configurada com `securityLevel: strict`. O componente React
da biblioteca não é montado, evitando sua live region interna e preservando os
menus, metadados e fluxos de edição já existentes.

### 2. Conector temporário para o announcer global

Enquanto a issue upstream não estiver disponível em uma release, cada
navigator recebe um elemento neutro, sem papel ARIA e fora da árvore de
acessibilidade. Um observador encaminha alterações desse elemento ao broker
global. Esse conector é infraestrutura temporária e deve ser removido quando a
biblioteca oferecer `onAnnounce` ou destino externo equivalente.

O som de limite usa o serviço de feedback sonoro do Assistente. O destaque
visual usa somente tokens de tema do aplicativo.

### 3. Política explícita de foco

O diagrama só recebe `tabindex="0"` e navegação interna quando o consumidor
habilita `tabNavigation`. Fora dos modos de leitura definidos pela AEP-0094, o
diagrama continua visual e não cria uma sequência paralela de Tab.

Tipos que renderizam SVG, mas não produzem nós extraíveis, permanecem visíveis
e não recebem o navigator. A ausência de suporte navegável não é erro de
renderização.

### 4. Falha isolada por diagrama

Cada bloco é renderizado e tratado independentemente. Se um bloco falhar:

- o conteúdo anterior e posterior continua visível e legível;
- outros diagramas continuam sendo processados;
- a região do diagrama mostra um resumo localizado;
- detalhes técnicos ficam recolhidos por padrão;
- copiar código, copiar erro, editar e tentar novamente continuam disponíveis.

A linha redundante de versão do Mermaid não aparece no resumo principal, mas o
erro integral pode permanecer nos detalhes e na ação de copiar para diagnóstico.

### 5. Um adaptador compartilhado

Markdown e Reveal usam o mesmo adaptador de renderização, navegação, anúncio,
fallback e cleanup. As superfícies preservam apenas suas responsabilidades
próprias, como menus de contexto no Markdown e `sync()` do deck no Reveal.

## Fases

### Fase 1 — Markdown, chat e editor

- [x] adicionar a dependência e o adaptador headless compartilhado;
- [x] conectar anúncios e feedback sonoro globais;
- [x] integrar ao `MarkdownRenderer`;
- [x] localizar e isolar erros por bloco;
- [x] cobrir foco, tipos sem suporte, cleanup e falhas parciais.

### Fase 2 — Reveal

- [x] remover o pipeline Mermaid duplicado do `RevealRenderer`;
- [x] reutilizar o adaptador compartilhado;
- [x] preservar edição, índices e sincronização dos slides;
- [x] cobrir falha isolada e navegação no deck.

### Fase 3 — Remoção do conector temporário

- [ ] acompanhar a issue upstream;
- [ ] atualizar para a release que aceite announcer externo;
- [ ] substituir o elemento neutro pelo contrato oficial;
- [ ] manter a auditoria de live regions verde.

## Riscos

- APIs internas do Mermaid podem variar dentro da faixa suportada pela
  biblioteca; a versão efetiva deve ser coberta por testes.
- Um navigator montado fora do modo de leitura criaria tab stops indesejados.
- Cleanup incompleto pode deixar observers, handlers ou overlays após rerender.
- Erros durante streaming podem gerar ruído; falhas não solicitadas não devem
  interromper a leitura global.
- O highlight padrão da biblioteca usa cores próprias; o Assistente deve
  fornecer implementação baseada em tokens.
- Alterações de DOM no Reveal exigem `sync()` posterior sem derrubar o deck se
  a sincronização falhar.

## Critérios de aceitação

- Flowcharts suportados são navegáveis por teclado nos modos de leitura.
- Fora do modo de leitura, diagramas não entram na ordem de Tab.
- Não existe nova live region local em chat, editor ou Reveal.
- Tipos não suportados continuam visualmente renderizados.
- Um bloco inválido não impede a leitura do restante do documento nem o
  processamento de outros diagramas.
- O erro exibido é localizado, conciso e possui detalhes recolhíveis.
- Menus e fluxos de copiar, editar, enviar e renderizar novamente permanecem.
- Markdown e Reveal compartilham o mesmo contrato de renderização e cleanup.
- Testes de acessibilidade, TypeScript, lint e Vitest permanecem verdes.
