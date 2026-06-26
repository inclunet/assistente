import { describe, expect, it } from 'vitest';

import {
  detectRevealMarkdown,
  extractRevealSlideAttributes,
  getRevealSlideEditableMarkdown,
  mergeRevealSlideEditableMarkdown,
  parseRevealMarkdown,
  replaceRevealSlide,
  splitRevealSlides,
  stripRevealDirectives,
} from './revealMarkdown';

describe('detectRevealMarkdown', () => {
  it('detecta atributos Reveal como sinal forte', () => {
    const markdown = `# Título

<!-- .slide: class="title-slide" -->

Conteúdo`;

    expect(detectRevealMarkdown(markdown)).toEqual({
      kind: 'reveal',
      confidence: 'strong',
      reason: 'slideAttribute',
    });
  });

  it('detecta decks com múltiplos separadores e conteúdo', () => {
    const markdown = `# Slide 1

---

## Slide 2

---

## Slide 3`;

    expect(detectRevealMarkdown(markdown)).toEqual({
      kind: 'reveal',
      confidence: 'strong',
      reason: 'multipleSeparators',
    });
  });

  it('não trata frontmatter como apresentação', () => {
    const markdown = `---
title: Artigo
---

# Documento

Texto normal.`;

    expect(detectRevealMarkdown(markdown).kind).toBe('markdown');
  });

  it('não soma frontmatter com uma régua horizontal do texto', () => {
    const markdown = `---
title: Artigo
---

# Documento

Parte 1.

---

Parte 2.`;

    expect(detectRevealMarkdown(markdown).kind).toBe('markdown');
  });

  it('não trata uma régua horizontal isolada como apresentação', () => {
    const markdown = `# Documento

Introdução.

---

Conclusão.`;

    expect(detectRevealMarkdown(markdown).kind).toBe('markdown');
  });

  it('não trata Note comum com régua horizontal como apresentação', () => {
    const markdown = `# Documento

Note:

Este é um bloco de observação do artigo.

---

Continuação do texto.`;

    expect(detectRevealMarkdown(markdown).kind).toBe('markdown');
  });

  it('ignora atributos Reveal dentro de blocos fenced', () => {
    const markdown = `# Exemplo

\`\`\`md
<!-- .slide: class="title-slide" -->
\`\`\``;

    expect(detectRevealMarkdown(markdown).kind).toBe('markdown');
  });

  it('ignora Note dentro de blocos fenced na detecção', () => {
    const markdown = `# Exemplo

\`\`\`md
Note:
\`\`\`

---

Texto comum.`;

    expect(detectRevealMarkdown(markdown).kind).toBe('markdown');
  });

  it('respeita override manual', () => {
    expect(detectRevealMarkdown('# Texto', 'reveal')).toEqual({
      kind: 'reveal',
      confidence: 'manual',
      reason: 'manual',
    });
    expect(detectRevealMarkdown('<!-- .slide: class="x" -->', 'markdown')).toEqual({
      kind: 'markdown',
      confidence: 'manual',
      reason: 'manual',
    });
  });
});

describe('splitRevealSlides', () => {
  it('preserva offsets para substituição de um slide', () => {
    const markdown = `# Slide 1

---

## Slide 2

----

### Slide 2.1`;

    const slides = splitRevealSlides(markdown);

    expect(slides).toHaveLength(3);
    expect(slides[0]).toMatchObject({ index: 0, level: 'horizontal', markdown: '# Slide 1' });
    expect(slides[1]).toMatchObject({ index: 1, level: 'horizontal', markdown: '## Slide 2' });
    expect(slides[2]).toMatchObject({ index: 2, level: 'vertical', markdown: '### Slide 2.1' });

    const next = replaceRevealSlide(markdown, slides[1], '## Slide atualizado');

    expect(next).toContain('## Slide atualizado');
    expect(next).toContain('### Slide 2.1');
    expect(next).not.toContain('## Slide 2\n\n----');
  });

  it('não divide slides em separadores dentro de blocos fenced', () => {
    const markdown = `# Slide 1

\`\`\`yaml
---
key: value
---
\`\`\`

---

## Slide 2`;

    const slides = splitRevealSlides(markdown);

    expect(slides).toHaveLength(2);
    expect(slides[0].markdown).toContain('```yaml\n---\nkey: value\n---\n```');
    expect(slides[1].markdown).toBe('## Slide 2');
  });

  it('ignora delimitadores de frontmatter YAML no split', () => {
    const markdown = `---
title: Deck
author: Assistente
---

<!-- .slide: class="title-slide" -->

# Slide 1

---

## Slide 2`;

    const slides = splitRevealSlides(markdown);

    expect(slides).toHaveLength(2);
    expect(slides[0].markdown).toContain('# Slide 1');
    expect(slides[0].markdown).not.toContain('title: Deck');
    expect(slides[1].markdown).toBe('## Slide 2');

    const next = replaceRevealSlide(markdown, slides[0], '# Slide atualizado');
    expect(next).toContain('title: Deck');
    expect(next).toContain('# Slide atualizado');
    expect(next).not.toContain('# Slide 1');
  });

  it('não cria slide vazio quando o deck começa com separador', () => {
    const markdown = `---

<!-- .slide: class="title-slide" -->

# Slide inicial

---

## Slide 2`;

    const slides = splitRevealSlides(markdown);

    expect(slides).toHaveLength(2);
    expect(slides[0]).toMatchObject({
      index: 0,
      level: 'horizontal',
      markdown: '<!-- .slide: class="title-slide" -->\n\n# Slide inicial',
      separatorBefore: '---',
    });
    expect(slides[1]).toMatchObject({ index: 1, markdown: '## Slide 2' });
  });
});

describe('parseRevealMarkdown', () => {
  it('só retorna slides quando detecta Reveal', () => {
    expect(parseRevealMarkdown('# Documento').slides).toHaveLength(0);
    expect(parseRevealMarkdown('# A\n\n---\n\n# B\n\n---\n\n# C').slides).toHaveLength(3);
  });
});

describe('extractRevealSlideAttributes', () => {
  it('extrai classes e atributos data do comentário .slide', () => {
    const markdown = `<!-- .slide: class="two-columns bad$script" data-background-image="assets/bg.png" -->

# Slide`;

    expect(extractRevealSlideAttributes(markdown)).toEqual({
      className: 'two-columns badscript',
      data: {
        'data-background-image': 'assets/bg.png',
      },
    });
    expect(stripRevealDirectives(markdown)).toBe('# Slide');
  });

  it('ignora diretivas .slide que não estejam no início do slide', () => {
    const markdown = `# Slide

\`\`\`md
<!-- .slide: class="fake-slide" -->
\`\`\``;

    expect(extractRevealSlideAttributes(markdown)).toEqual({ data: {} });
    expect(stripRevealDirectives(markdown)).toContain('<!-- .slide: class="fake-slide" -->');
  });

  it('remove todas as diretivas .slide iniciais ao renderizar o corpo', () => {
    const markdown = `<!-- .slide: class="section-slide" -->
<!-- .slide: data-transition="fade" -->

# Slide`;

    expect(stripRevealDirectives(markdown)).toBe('# Slide');
  });
});

describe('editable slide markdown', () => {
  it('remove diretivas .slide do corpo editável e restaura ao mesclar', () => {
    const slide = `<!-- .slide: class="title-slide" -->

# Título

Subtítulo`;

    expect(getRevealSlideEditableMarkdown(slide)).toBe('# Título\n\nSubtítulo');
    expect(mergeRevealSlideEditableMarkdown(slide, '# Novo')).toBe(`<!-- .slide: class="title-slide" -->

# Novo`);
  });
});

