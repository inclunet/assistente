# AEP-0079: Modo Apresentação Reveal.js no Editor

## Status: Done — renderer, edição por slide, contexto acessível e testes entregues

## Resumo

Adicionar suporte a apresentações Reveal.js como uma evolução do Editor existente, sem criar uma nova superfície de Workspace.

O arquivo continua sendo Markdown/HTML compatível com Reveal.js. O modo código exibe o documento completo, o modo rico pode editar um slide por vez quando o documento for detectado como apresentação, e o modo renderizado usa Reveal.js em vez do renderer Markdown padrão.

Esta AEP complementa `aep/0032-editor-rico.md` e preserva as decisões centrais do Editor: Markdown como fonte de verdade, aplicação de alterações por patch revisável e uso do pipeline de chat existente.

## Motivação

O usuário deve conseguir criar, editar e apresentar slides com ajuda do assistente, mantendo interoperabilidade com ferramentas externas. Reveal.js atende melhor ao escopo inicial do que PPTX porque usa Markdown/HTML, é versionável e combina com o fluxo atual do Editor.

Criar uma nova superfície `slides` duplicaria capacidades já existentes no Editor: abertura/salvamento de Markdown, modo rico, modo código, preview renderizado, chat contextual e aplicação de patches. A abordagem correta é evoluir o Editor com uma variação de renderização e edição para documentos que sejam claramente apresentações.

## Decisões

1. Reveal.js é um modo especializado do Editor, não um novo `TabType`.

2. O `tabType` enviado ao chat continua sendo `editor`. O contexto de apresentação viaja em `surfaceContextJson`, por exemplo: modo `reveal`, quantidade de slides, índice do slide atual e Markdown do slide atual quando aplicável.

3. Markdown comum mantém o comportamento atual. A detecção automática deve ser conservadora:
   - `<!-- .slide: ... -->` fora de bloco fenced é o sinal explícito de apresentação.
   - separadores `---`/`----` dividem slides somente depois que o documento já
     foi reconhecido como apresentação ou quando o modo Reveal foi solicitado
     explicitamente pelo consumidor.
   - separadores, ainda que múltiplos e acompanhados de headings, `Note:`,
     frontmatter YAML, imagens ou listas não bastam para ativar apresentação,
     pois todos também são Markdown comum válido.

4. Deve haver caminho para override manual em evolução futura: “tratar como apresentação” e “tratar como Markdown comum”. Essa preferência pertence ao estado da aba/editor, não ao conteúdo do arquivo.

5. O modo código sempre mostra o Markdown completo do documento.

6. O modo rico, quando o documento for apresentação, pode mostrar um slide por vez para reduzir complexidade visual e melhorar acessibilidade. A navegação deve ser por controle simples, como dropdown “Slide N de M”, sem sidebar de miniaturas no MVP.

7. O modo renderizado, quando o documento for apresentação, usa Reveal.js em modo embutido. Caso contrário, mantém o renderer Markdown atual.

8. Layouts devem usar padrões Reveal-native, como `<!-- .slide: class="two-columns" -->`, `data-background-image` e HTML/CSS compatível. O Assistente não deve inventar metadados proprietários de layout no arquivo.

9. Títulos e rótulos acessíveis devem ser derivados do Markdown compatível com Reveal, sem formato proprietário: título do deck por `title` no frontmatter, primeiro H1 ou título do documento como fallback; rótulo do slide por primeiro heading, `title`/`data-title` em diretiva `.slide` ou fallback traduzido “Slide N”.

10. Imagens continuam em Markdown/HTML padrão. A UI deve incentivar texto alternativo e não depender apenas de cor para transmitir informação.

11. Alterações por LLM continuam no contrato do Editor: `SendMessage`/`RetryMessage`, contexto estruturado de superfície e patch revisável antes de aplicar. Não deve existir `SendSlidesMessage`.

## Fases

1. Criar detector/parser conservador de Markdown Reveal-compatible com testes
   contra falsos positivos de Markdown comum. A detecção automática exige
   diretiva `.slide`; decks sem diretiva dependem do override explícito.

2. Integrar Reveal.js ao modo renderizado do Editor, mantendo fallback para `MarkdownRenderer`.

3. Evoluir o modo rico para editar um slide por vez quando o documento for apresentação, com navegação por dropdown e ações mínimas para criar slide.

4. Enviar contexto de apresentação ao chat do Editor sem alterar o fluxo de mensagens.

5. Adicionar templates acessíveis de layout Reveal-native e melhorias de imagens/alt text.

6. Avaliar exportação estática de pacote Reveal (`index.html`, `deck.md`, `assets/`) após estabilizar a edição/renderização.

## Riscos

- Falso positivo na detecção pode mudar a experiência de arquivos Markdown comuns. Mitigação: detector conservador, testes e fallback manual.

- HTML em slides é necessário para layouts Reveal, mas aumenta risco de XSS. Mitigação: renderizar com sanitização e permitir apenas tags/atributos necessários.

- Sincronizar edição rica de um slide com o Markdown completo pode corromper separadores. Mitigação: parser com offsets, substituição localizada e testes de preservação.

- Reveal.js traz CSS e comportamento próprios. Mitigação: isolar o renderer em componente dedicado e preservar o restante do Editor.

- O modo rico por slide pode divergir do modo código se houver debounce pendente. Mitigação: flush antes de trocar de slide ou modo.

## Critérios de aceitação

- Arquivos Markdown comuns continuam abrindo, editando e renderizando como antes.

- Um documento com diretiva `.slide` é detectado como apresentação; múltiplas
  réguas horizontais sem diretiva permanecem Markdown comum.

- No modo código, o documento completo permanece visível e editável.

- No modo rico, apresentações exibem um slide por vez com navegação acessível.

- No modo renderizado, apresentações são exibidas com Reveal.js.

- A apresentação e cada slide expõem nomes acessíveis derivados do conteúdo quando possível, e a navegação de slides usa esses rótulos.

- O chat do Editor envia contexto de apresentação sem criar fluxo paralelo de mensagens.

- Testes cobrem detecção, falsos positivos, divisão/substituição de slides e renderização básica.

