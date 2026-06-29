---
name: slides-reveal-markdown
version: 1.0.0
description: "Orienta criação e edição de apresentações Reveal Markdown dentro do editor de texto."
displayName: Slides Reveal Markdown
author: Assistente
type: agent
category: editor
difficulty: beginner
platforms:
  - windows
  - macos
  - linux
tools:
  allowed:
    - text_edit
output:
  format: markdown
---

# Slides Reveal Markdown

Use esta skill quando o usuário pedir para criar, revisar, reorganizar, resumir, expandir ou editar uma apresentação em **Reveal Markdown** dentro do editor.

## Objetivo

Produzir Markdown interoperável com Reveal.js para criar decks completos ou aplicar edições localizadas, preservando a estrutura existente quando houver conteúdo aberto no editor.

## Contexto de edição

Respeite o `surface_context` recebido do editor:

- O contexto pode vir como bloco XML-like com `<surface_context ... mode="reveal">`.
- Se houver `<selection ...>...</selection>`, trate esse conteúdo como alvo principal da edição.
- Se não houver seleção e existir `<content kind="reveal_slide">...</content>`, use esse conteúdo como o slide atual. O foco pode aparecer em `<focus kind="slide" ... slide_index="..." />` e o índice também pode aparecer em `<metadata key="current_slide_index">...</metadata>`.
- Se houver apenas contexto do arquivo/deck, use-o para preservar estilo, idioma, separadores, frontmatter e padrões existentes.
- Se não houver `surface_context` útil, ainda assim crie ou proponha o deck a partir do pedido do usuário.
- Use o pedido do usuário para decidir se deve criar conteúdo novo, revisar clareza, condensar, expandir, traduzir, reorganizar, transformar texto em slides ou corrigir Markdown.

Quando for editar conteúdo existente e a ferramenta `text_edit` estiver disponível, use `text_edit`. Preencha `original` com o trecho de `<selection>` ou, se não houver seleção, com o conteúdo de `<content kind="reveal_slide">`; preencha `replacement` com o Markdown final, sem explicações dentro do conteúdo substituto.

## Criação de deck completo

Quando o usuário pedir para criar slides do zero ou transformar um material em apresentação:

- Identifique objetivo, audiência, duração, idioma, tom e formato esperado. Se um desses dados for essencial e estiver ausente, faça uma pergunta curta; caso contrário, assuma um padrão razoável e siga.
- Planeje o número aproximado de slides pela duração e densidade do tema. Prefira menos slides densos e mais slides claros.
- Organize o deck com: título, agenda ou contexto, seções temáticas, desenvolvimento com exemplos/dados essenciais, conclusão e próximos passos.
- Escreva cada slide com uma ideia principal, heading claro e pouco texto. Use bullets curtos quando ajudarem a leitura.
- Inclua notas do apresentador quando o usuário pedir roteiro, fala, treinamento ou detalhes que não devem aparecer no slide.
- Se o usuário fornecer texto longo, transforme em narrativa de slides: agrupe ideias, remova repetição, crie transições e destaque mensagens-chave.
- Se o usuário pedir uma versão executiva, reduza detalhes e preserve decisões, impactos e próximos passos.

## Formato Reveal Markdown

- Separe slides horizontais com `---`.
- Preserve separadores `----` existentes para slides verticais. Não converta `----` para `---` sem pedido explícito.
- Cada slide deve ter um heading ou rótulo claro, preferencialmente `#`, `##` ou `###`.
- Use frontmatter YAML apenas quando fizer sentido para metadados do deck ou quando já existir no arquivo.
- Preserve frontmatter YAML do deck quando existir.
- Preserve fences de código, diretivas Reveal e comentários relevantes já existentes.
- Use `Note:` para notas do apresentador quando o deck ou o pedido indicar suporte a speaker notes.
- Para imagens, use Markdown com texto alternativo: `![descrição objetiva](caminho-ou-url)`.
- Para layouts Reveal, prefira padrões compatíveis, como comentários `.slide`, classes ou atributos já usados no deck.
- Evite HTML cru desnecessário. Use Markdown simples sempre que for suficiente.
- Mantenha sintaxe compatível com Reveal Markdown comum; não invente extensões específicas sem evidência no arquivo.

## Escopo da resposta

- Gere um deck completo quando o usuário pedir criar uma apresentação, montar slides do zero, converter um texto em deck ou reestruturar a apresentação inteira.
- Edite apenas a seleção quando houver seleção e o pedido for localizado.
- Edite apenas o slide atual quando não houver seleção, houver `<content kind="reveal_slide">` e o pedido falar do slide atual.
- Reorganize, expanda ou resuma o deck inteiro somente quando o usuário pedir essa abrangência ou quando o contexto deixar claro que a apresentação completa é o alvo.
- Ao inserir novos slides em um deck existente, mantenha separadores, idioma, estilo de headings e padrões de nota/layout já presentes.

## Acessibilidade e clareza

- Use headings claros para orientar navegação e leitores de tela.
- Inclua texto alternativo significativo em imagens: `![descrição objetiva](caminho-ou-url)`.
- Prefira pouco texto por slide, listas curtas e frases diretas.
- Não dependa apenas de cor para transmitir significado; use texto, ícones com rótulo ou estrutura.
- Quando sugerir layout, contraste ou hierarquia visual, priorize legibilidade e contraste suficiente.
- Evite tabelas grandes, blocos densos e excesso de bullets em um único slide.

## Preservação

- Não remova frontmatter, notas do apresentador, atributos de slide, diretivas Reveal, IDs, classes ou fences de código sem necessidade.
- Ao editar uma parte, mantenha o restante do deck intacto.
- Se a seleção cortar uma fence, uma diretiva ou um slide pela metade, peça mais contexto antes de aplicar uma alteração arriscada.
- Se o pedido exigir uma mudança estrutural ampla, preserve a intenção e a ordem do material original, a menos que o usuário peça reorganização.

## Resposta

- Para edições, use `text_edit` e mantenha a resposta normal curta.
- Para criação de deck completo, retorne somente o Markdown final da apresentação, salvo se o usuário pedir explicação.
- Se faltar contexto para uma edição segura, peça exatamente o contexto necessário.
