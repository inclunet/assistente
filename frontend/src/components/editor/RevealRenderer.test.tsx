import { render, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { RevealRenderer } from './RevealRenderer';

vi.mock('react-i18next', () => ({
  initReactI18next: {
    type: '3rdParty',
    init: vi.fn(),
  },
  useTranslation: () => ({
    t: (key: string, values?: Record<string, string | number>) =>
      values?.title
        ? `${key}: ${values.title}`
        : values?.index
          ? `${key}: ${values.index}`
          : key,
    i18n: { language: 'pt-BR', resolvedLanguage: 'pt-BR' },
  }),
}));

vi.mock('react-router-dom', () => ({
  useNavigate: () => vi.fn(),
}));

const revealMocks = vi.hoisted(() => ({
  configure: vi.fn(),
  destroy: vi.fn(),
  initialize: vi.fn(),
  sync: vi.fn(),
}));

const accessibleMermaidMocks = vi.hoisted(() => ({
  cleanup: vi.fn(),
  loadMermaid: vi.fn(),
  renderAccessibleMermaid: vi.fn(),
}));

vi.mock('reveal.js', () => ({
  default: class RevealMock {
    initialize = revealMocks.initialize;
    destroy = revealMocks.destroy;
    sync = revealMocks.sync;
    configure = revealMocks.configure;
  },
}));

vi.mock('mermaid', () => ({
  default: {
    initialize: vi.fn(),
    render: vi.fn(async () => ({ svg: '<svg role="img"></svg>' })),
  },
}));

vi.mock('../../lib/accessibleMermaid', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../lib/accessibleMermaid')>();
  return {
    ...actual,
    loadMermaid: accessibleMermaidMocks.loadMermaid,
    renderAccessibleMermaid: accessibleMermaidMocks.renderAccessibleMermaid,
  };
});

describe('RevealRenderer', () => {
  beforeEach(() => {
    revealMocks.configure.mockReset();
    revealMocks.destroy.mockReset();
    revealMocks.initialize.mockReset();
    revealMocks.sync.mockReset();
    accessibleMermaidMocks.cleanup.mockReset();
    accessibleMermaidMocks.loadMermaid.mockReset();
    accessibleMermaidMocks.loadMermaid.mockResolvedValue({ render: vi.fn() });
    accessibleMermaidMocks.renderAccessibleMermaid.mockReset();
    accessibleMermaidMocks.renderAccessibleMermaid.mockImplementation(
      async ({ container, navigationEnabled, ariaLabel }: {
        container: HTMLElement;
        navigationEnabled: boolean;
        ariaLabel: string;
      }) => {
        container.setAttribute('role', 'group');
        container.setAttribute('aria-label', ariaLabel);
        container.tabIndex = navigationEnabled ? 0 : -1;
        const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
        svg.setAttribute('role', 'img');
        container.appendChild(svg);
        return {
          svg,
          diagramType: 'flowchart-v2',
          navigable: navigationEnabled,
          cleanup: accessibleMermaidMocks.cleanup,
        };
      },
    );
  });

  it('habilita links na ordem de Tab somente durante a leitura escopada', () => {
    const markdown = '[Documentação](https://example.com)';
    const { container, rerender } = render(
      <RevealRenderer markdown={markdown} tabNavigation="disabled" />,
    );
    expect(container.querySelector('a[href]')).toHaveAttribute('tabindex', '-1');

    rerender(<RevealRenderer markdown={markdown} tabNavigation="enabled" />);
    expect(container.querySelector('a[href]')).toHaveAttribute('tabindex', '0');
  });

  it('desativa o teclado do Reveal durante a leitura escopada', async () => {
    const { rerender } = render(
      <RevealRenderer markdown="# Slide" tabNavigation="enabled" />,
    );

    await waitFor(() => {
      expect(revealMocks.configure).toHaveBeenCalledWith({ keyboard: false });
    });

    rerender(<RevealRenderer markdown="# Slide" tabNavigation="disabled" />);

    await waitFor(() => {
      expect(revealMocks.configure).toHaveBeenLastCalledWith({ keyboard: true });
    });
  });

  it('expõe título do deck e rótulos acessíveis por slide', () => {
    const { container } = render(
      <RevealRenderer
        markdown={`---
title: Deck acessível
---

<!-- .slide: data-title="Abertura" -->

Boas-vindas

---

## Agenda`}
      />
    );

    expect(container.querySelector('.reveal-renderer')).toHaveAttribute(
      'aria-label',
      'editor.presentation.ariaWithTitle: Deck acessível'
    );

    const slides = container.querySelectorAll('.slides > section');
    expect(slides[0]).toHaveAttribute('aria-label', 'Abertura');
    expect(slides[1]).toHaveAttribute('aria-label', 'Agenda');
  });

  it('usa o título do documento como fallback acessível do deck', () => {
    const { container } = render(
      <RevealRenderer
        markdown={`Intro

---

Conteúdo`}
        documentTitle="Apresentação sem heading"
      />
    );

    expect(container.querySelector('.reveal-renderer')).toHaveAttribute(
      'aria-label',
      'editor.presentation.ariaWithTitle: Apresentação sem heading'
    );
  });

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

  it('preserva indentação significativa ao separar speaker notes', () => {
    const { container } = render(
      <RevealRenderer
        markdown={`    code block

Note:
    nota indentada`}
      />
    );

    expect(container.querySelector('pre code')).toHaveTextContent('code block');
    expect(container.querySelector('aside.notes pre code')).toHaveTextContent('nota indentada');
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

  it('aplica hardening explícito em links externos', () => {
    const { container } = render(
      <RevealRenderer markdown={'# Slide\n\n[Exemplo](https://example.com)'} />
    );

    const link = container.querySelector('a');

    expect(link).toHaveAttribute('target', '_blank');
    expect(link).toHaveAttribute('rel', 'noopener noreferrer');
    expect(link).toHaveAttribute('tabindex', '-1');
  });

  it('mantém metadados acessíveis de deep links no preview Reveal', () => {
    const { container } = render(
      <RevealRenderer
        markdown={'# Slide\n\n[Abrir](assistente://navigate/history)'}
        tabNavigation="enabled"
      />
    );

    const link = container.querySelector('a');

    expect(link).toHaveClass('deep-link', 'deep-link--navigate');
    expect(link).toHaveAttribute('data-deep-link', 'assistente://navigate/history');
    expect(link).toHaveAttribute('role', 'link');
    expect(link).toHaveAttribute('aria-label');
    expect(link).toHaveAttribute('tabindex', '0');
  });

  it('renderiza Mermaid dentro do preview Reveal preservando o alvo editável', async () => {
    const markdown = `# Slide

\`\`\`mermaid
flowchart TD
  A --> B
\`\`\``;
    const { container, rerender } = render(
      <RevealRenderer
        markdown={markdown}
        tabNavigation="disabled"
      />
    );

    await waitFor(() => {
      expect(container.querySelector('.mermaid-diagram')).not.toBeNull();
    });

    const diagram = container.querySelector('.mermaid-diagram');
    expect(diagram).toHaveAttribute('data-mermaid-index', '0');
    expect(diagram).toHaveAttribute('data-mermaid-code', 'flowchart TD\n  A --> B');
    expect(diagram).toHaveAttribute('tabindex', '-1');
    expect(accessibleMermaidMocks.renderAccessibleMermaid)
      .toHaveBeenLastCalledWith(expect.objectContaining({ navigationEnabled: false }));

    rerender(<RevealRenderer markdown={markdown} tabNavigation="enabled" />);

    await waitFor(() => {
      expect(container.querySelector('.mermaid-diagram')).toHaveAttribute('tabindex', '0');
    });
    expect(accessibleMermaidMocks.cleanup).toHaveBeenCalled();
    expect(accessibleMermaidMocks.renderAccessibleMermaid)
      .toHaveBeenLastCalledWith(expect.objectContaining({ navigationEnabled: true }));
  });

  it('isola um Mermaid inválido sem impedir os demais slides e diagramas', async () => {
    accessibleMermaidMocks.renderAccessibleMermaid
      .mockRejectedValueOnce(new Error('Syntax error in text\nmermaid version 11.14.0'))
      .mockImplementationOnce(async ({ container, ariaLabel }: {
        container: HTMLElement;
        ariaLabel: string;
      }) => {
        container.setAttribute('role', 'group');
        container.setAttribute('aria-label', ariaLabel);
        container.tabIndex = 0;
        const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
        container.appendChild(svg);
        return {
          svg,
          diagramType: 'flowchart-v2',
          navigable: true,
          cleanup: accessibleMermaidMocks.cleanup,
        };
      });

    const { container } = render(
      <RevealRenderer
        tabNavigation="enabled"
        markdown={`# Antes

\`\`\`mermaid
inválido
\`\`\`

---

# Depois

\`\`\`mermaid
flowchart LR
A --> B
\`\`\``}
      />,
    );

    await waitFor(() => {
      expect(accessibleMermaidMocks.renderAccessibleMermaid).toHaveBeenCalledTimes(2);
    });
    expect(container).toHaveTextContent('Antes');
    expect(container).toHaveTextContent('Depois');
    expect(container.querySelectorAll('.mermaid-diagram')).toHaveLength(2);
    expect(container.querySelector('.mermaid-diagram--error')).not.toBeNull();
    expect(container.querySelector('.mermaid-diagram--error details')).not.toHaveAttribute('open');
    expect(container.querySelector('.mermaid-diagram--error summary')).toHaveAttribute('tabindex', '0');
    expect(container.querySelectorAll('.mermaid-diagram svg')).toHaveLength(1);
  });
});
