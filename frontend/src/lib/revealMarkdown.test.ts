import { describe, expect, it } from 'vitest';

import {
  detectRevealMarkdown,
  extractRevealSlideAttributes,
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
});

