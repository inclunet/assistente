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

Use esta skill quando o usuário pedir para criar, revisar, reorganizar ou editar uma apresentação em **Reveal Markdown** dentro do editor.

## Objetivo

Produzir Markdown interoperável com Reveal.js, preservando o máximo possível da estrutura existente e entregando alterações aplicáveis pelo editor.

## Contexto de edição

Respeite o `surface_context` recebido do editor:

- Se houver seleção, edite a seleção.
- Se não houver seleção e existir `currentSlideMarkdown`, opere no slide atual indicado por `currentSlideIndex`.
- Se o usuário pedir o deck inteiro, produza ou reestruture a apresentação completa.
- Use o pedido do usuário para decidir se deve criar conteúdo novo, revisar clareza, condensar, traduzir, reorganizar ou corrigir Markdown.

Quando for editar conteúdo existente e a ferramenta `text_edit` estiver disponível, use `text_edit`. Preencha `original` com o trecho selecionado ou o slide atual e `replacement` com o Markdown final, sem explicações dentro do conteúdo substituto.

## Formato Reveal Markdown

- Separe slides horizontais com `---`.
- Preserve separadores `----` existentes para slides verticais. Não converta `----` para `---` sem pedido explícito.
- Cada slide deve ter um heading ou rótulo claro, preferencialmente `#`, `##` ou `###`.
- Preserve frontmatter YAML do deck quando existir.
- Preserve fences de código, diretivas Reveal e comentários relevantes já existentes.
- Evite HTML cru desnecessário. Use Markdown simples sempre que for suficiente.
- Mantenha sintaxe compatível com Reveal Markdown comum; não invente extensões específicas sem evidência no arquivo.

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
