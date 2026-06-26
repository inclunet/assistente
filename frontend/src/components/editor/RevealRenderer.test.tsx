import { render, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { RevealRenderer } from './RevealRenderer';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('reveal.js', () => ({
  default: class RevealMock {
    initialize = vi.fn();
    destroy = vi.fn();
    sync = vi.fn();
  },
}));

vi.mock('mermaid', () => ({
  default: {
    initialize: vi.fn(),
    render: vi.fn(async () => ({ svg: '<svg role="img"></svg>' })),
  },
}));

describe('RevealRenderer', () => {
  it('renderiza slides verticais como pilha aninhada do Reveal', () => {
    const { container } = render(
      <RevealRenderer
        markdown={`# Horizontal

----

## Vertical

---

# Próximo`}
      />
    );

    const slidesRoot = container.querySelector('.slides');
    const topLevelSlides = Array.from(slidesRoot?.children ?? []);
    const verticalStack = topLevelSlides[0];

    expect(topLevelSlides).toHaveLength(2);
    expect(verticalStack.children).toHaveLength(2);
    expect(verticalStack.children[0]).toHaveTextContent('Horizontal');
    expect(verticalStack.children[1]).toHaveTextContent('Vertical');
    expect(topLevelSlides[1]).toHaveTextContent('Próximo');
  });

  it('agrupa slides verticais órfãos em uma pilha Reveal válida', () => {
    const { container } = render(
      <RevealRenderer
        markdown={`----

## Vertical inicial

----

## Vertical seguinte`}
      />
    );

    const slidesRoot = container.querySelector('.slides');
    const topLevelSlides = Array.from(slidesRoot?.children ?? []);

    expect(topLevelSlides).toHaveLength(1);
    expect(topLevelSlides[0].children).toHaveLength(2);
    expect(topLevelSlides[0].children[0]).toHaveTextContent('Vertical inicial');
    expect(topLevelSlides[0].children[1]).toHaveTextContent('Vertical seguinte');
  });

  it('não trata Note dentro de bloco fenced como notas do Reveal', () => {
    const { container } = render(
      <RevealRenderer
        markdown={`# Slide

\`\`\`md
Note:
texto do exemplo
\`\`\``}
      />
    );

    expect(container.querySelector('aside.notes')).toBeNull();
    expect(container.querySelector('code')).toHaveTextContent('Note:');
  });

  it('remove atributos data de URL inseguros nos slides', () => {
    const { container } = render(
      <RevealRenderer
        markdown={`<!-- .slide: data-background-image="javascript:alert(1)" data-transition="fade" -->

# Slide`}
      />
    );

    const slide = container.querySelector('.slides section');

    expect(slide).toHaveAttribute('data-transition', 'fade');
    expect(slide).not.toHaveAttribute('data-background-image');
  });

  it('renderiza Mermaid dentro do preview Reveal preservando o alvo editável', async () => {
    const { container } = render(
      <RevealRenderer
        markdown={`# Slide

\`\`\`mermaid
flowchart TD
  A --> B
\`\`\``}
      />
    );

    await waitFor(() => {
      expect(container.querySelector('.mermaid-diagram')).not.toBeNull();
    });

    const diagram = container.querySelector('.mermaid-diagram');
    expect(diagram).toHaveAttribute('data-mermaid-index', '0');
    expect(diagram).toHaveAttribute('data-mermaid-code', 'flowchart TD\n  A --> B');
  });
});
