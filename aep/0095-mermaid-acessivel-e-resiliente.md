# AEP-0095: Mermaid acessível e resiliente

- **Status**: Done
- **Data**: 2026-08-21

## Resumo

Diagramas Mermaid no chat, no editor e em apresentações devem oferecer
navegação por teclado e leitor de telas quando o tipo puder ser representado
como grafo. Uma falha de sintaxe ou de renderização permanece confinada à
região do diagrama e nunca remove nem bloqueia o restante do conteúdo.

A integração usa `@inclunet/mermaid-a11y` pelas APIs headless para preservar os
menus e fluxos de edição próprios do Assistente. Desde a versão 0.2.0, os
anúncios são enviados diretamente ao announcer global conforme a AEP-0058.

## Motivação

O renderer atual produz SVG visual, mas não expõe nós e conexões como um widget
navegável. Além disso, erros do Mermaid incluem detalhes técnicos repetitivos,
como a versão da biblioteca, diretamente no fluxo de leitura.

`@inclunet/mermaid-a11y` já fornece extração do grafo, travessia e foco lógico.
A versão 0.1.0 exigia um elemento para receber anúncios, o que levou ao conector
temporário do Assistente. A versão 0.2.0 resolveu a
[issue mermaid-a11y#1](https://github.com/inclunet/mermaid-a11y/issues/1) com
um callback `onAnnounce` fornecido pelo host.

## Decisões

### 1. APIs headless e instância Mermaid do host

O Assistente usa `renderAccessibleDiagram` e `createNavigator`, passando a
instância Mermaid configurada com `securityLevel: strict`. O componente React
da biblioteca não é montado porque as superfícies já possuem menus, metadados e
fluxos de edição próprios.

### 2. Canal oficial para o announcer global

Cada navigator recebe o callback `onAnnounce` da versão 0.2.0, conectado
diretamente ao broker global. Não existe elemento intermediário, observer ou
live region por diagrama. O broker continua responsável pela repetição de
mensagens idênticas e pela arbitragem entre superfícies.

O som de limite usa o serviço de feedback sonoro do Assistente. O destaque
visual usa somente tokens de tema do aplicativo.

### 3. Política explícita de foco

O diagrama só recebe `tabindex="0"` e navegação interna quando o consumidor
habilita `tabNavigation`. Fora dos modos de leitura definidos pela AEP-0094, o
diagrama continua visual e não cria uma sequência paralela de Tab.

O adaptador respeita o campo `navigable` devolvido pela biblioteca. Tipos que
renderizam SVG, mas não permitem extração navegável, permanecem visíveis e não
recebem o navigator. A ausência de suporte navegável não é erro de renderização.

### 4. Falha isolada por diagrama

Cada bloco é renderizado e tratado independentemente. Se um bloco falhar:

- o conteúdo anterior e posterior continua visível e legível;
- outros diagramas continuam sendo processados;
- a região do diagrama mostra um resumo localizado;
- detalhes técnicos ficam recolhidos por padrão;
- copiar código, copiar erro, editar e tentar novamente continuam disponíveis.

A linha redundante de versão do Mermaid não aparece no resumo principal, mas o
erro integral pode permanecer nos detalhes e na ação de copiar para diagnóstico.

O Mermaid, por padrão, desenha o próprio cartaz de erro ("Syntax error in text"
mais a versão) antes de lançar a exceção. Como o adaptador da biblioteca chama
`mermaid.render` sem informar o host, esse cartaz nasce num nó temporário do
`body` que o Mermaid só remove quando o render termina bem — ou seja, ele
sobrevive à falha, aparece na tela fora do bloco e é lido pelo leitor de telas.
Por isso o Assistente liga `suppressErrorRendering` na inicialização, que é uma
chave protegida e só vale via `initialize`, e ainda remove os nós de erro que
escaparem para o `body`. A limpeza é restrita aos nós que contêm o desenho de
erro, para não derrubar um render simultâneo em andamento. O pedido para o
adaptador passar o host está em
[issue mermaid-a11y#4](https://github.com/inclunet/mermaid-a11y/issues/4).

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

- [x] acompanhar a issue upstream;
- [x] atualizar para a release que aceite announcer externo;
- [x] substituir o elemento neutro pelo contrato oficial;
- [x] manter a auditoria de live regions verde.

### Evidências

- Adaptador compartilhado, navigator e announcer:
  `frontend/src/lib/accessibleMermaid.ts` e
  `accessibleMermaid.test.ts`.
- Integração Markdown, fallback localizado e falha parcial:
  `frontend/src/components/ui/MarkdownRenderer.tsx` e
  `MarkdownRenderer.test.tsx`.
- Reuso no Reveal, cleanup, navegação e isolamento:
  `frontend/src/components/editor/RevealRenderer.tsx` e
  `RevealRenderer.test.tsx`.
- Integração real da biblioteca e regressões auxiliares:
  `accessibleMermaid.integration.test.ts`, `mermaidFence.test.ts` e testes do
  editor Mermaid.
- Entregas de implementação:
  [PR #570 — Mermaid acessível](https://github.com/inclunet/assistente/pull/570),
  [PR #571 — integração Reveal](https://github.com/inclunet/assistente/pull/571),
  [PR #572 — erro contido](https://github.com/inclunet/assistente/pull/572) e
  [PR #573 — canal oficial do announcer](https://github.com/inclunet/assistente/pull/573).

## Riscos

- APIs internas do Mermaid podem variar dentro da faixa suportada pela
  biblioteca; a versão efetiva deve ser coberta por testes.
- Um navigator montado fora do modo de leitura criaria tab stops indesejados.
- Cleanup incompleto pode deixar handlers ou overlays após rerender.
- Erros durante streaming podem gerar ruído; falhas não solicitadas não devem
  interromper a leitura global.
- O highlight padrão da biblioteca usa cores próprias; o Assistente deve
  fornecer implementação baseada em tokens.
- Alterações de DOM no Reveal exigem `sync()` posterior sem derrubar o deck se
  a sincronização falhar.

## Critérios de aceitação

- [x] Flowcharts suportados são navegáveis por teclado nos modos de leitura.
- [x] Fora do modo de leitura, diagramas não entram na ordem de Tab.
- [x] Não existe nova live region local em chat, editor ou Reveal.
- [x] Tipos não suportados continuam visualmente renderizados.
- [x] Um bloco inválido não impede a leitura do restante do documento nem o
  processamento de outros diagramas.
- [x] O erro exibido é localizado, conciso e possui detalhes recolhíveis.
- [x] Nenhum desenho de erro do Mermaid escapa do bloco para o `body`.
- [x] Menus e fluxos de copiar, editar, enviar e renderizar novamente permanecem.
- [x] Markdown e Reveal compartilham o mesmo contrato de renderização e cleanup.
- [x] Testes focados cobrem acessibilidade, integração, cleanup e falhas
  isoladas nas duas superfícies.
- [x] TypeScript está verde (`npx tsc --noEmit`).
- [x] ESLint, incluindo `jsx-a11y`, e Stylelint estão verdes.
- [x] Vitest, incluindo as regressões Mermaid e `axe-core`, está verde.

Os gates são reproduzíveis pelo contrato de
`.github/workflows/ci.yml`: etapas `TypeScript check`, `ESLint`, `Stylelint` e
`Tests (includes axe-core a11y)`, equivalentes a `npx tsc --noEmit`,
`npm run lint`, `npm run lint:css` e `npm run test`. A cobertura específica
permanece nos testes concretos listados em **Evidências**; os PRs de
implementação também estão vinculados ali. Checks de um PR documental posterior
não constituem evidência da implementação desta AEP.
