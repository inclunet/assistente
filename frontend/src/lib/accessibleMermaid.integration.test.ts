import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { loadMermaid, renderAccessibleMermaid } from './accessibleMermaid';

function layoutPrototypes(): object[] {
  const scope = globalThis as unknown as Record<string, { prototype?: object } | undefined>;
  return ['SVGElement', 'SVGGraphicsElement', 'SVGTextContentElement']
    .map((name) => scope[name]?.prototype)
    .filter((prototype): prototype is object => Boolean(prototype));
}

describe('integração com a versão instalada do Mermaid', () => {
  beforeEach(() => {
    Object.defineProperty(SVGElement.prototype, 'getBBox', {
      configurable: true,
      value: vi.fn(() => ({
        x: 0,
        y: 0,
        width: 120,
        height: 40,
        top: 0,
        right: 120,
        bottom: 40,
        left: 0,
        toJSON: () => ({}),
      } as DOMRect)),
    });
  });

  afterEach(() => {
    delete (SVGElement.prototype as SVGElement & { getBBox?: () => DOMRect }).getBBox;
    document.body.replaceChildren();
  });

  it('extrai e torna navegável um flowchart real', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    const mermaid = await loadMermaid();

    const result = await renderAccessibleMermaid({
      chart: 'flowchart LR\nA[Início] --> B[Fim]',
      container,
      mermaid,
      locale: 'pt-BR',
      navigationEnabled: true,
      ariaLabel: 'Diagrama Mermaid',
    });

    expect(result.navigable).toBe(true);
    expect(result.diagramType).toMatch(/flowchart/);
    expect(container.querySelector('svg')).not.toBeNull();
    expect(container).toHaveAttribute('tabindex', '0');
    expect(container.querySelector('[aria-live]')).toBeNull();

    result.cleanup();
  });
});

describe('falha de layout com a versão instalada do Mermaid', () => {
  // O diagrama passa no parser e quebra na medição, que é justamente o caminho
  // em que o Mermaid monta o cartaz de erro. A quebra é forçada aqui para o
  // teste não depender de o ambiente deixar de implementar getBBox.
  beforeEach(() => {
    layoutPrototypes().forEach((prototype) => {
      Object.defineProperty(prototype, 'getBBox', {
        configurable: true,
        value: () => {
          throw new Error('medição indisponível');
        },
      });
    });
  });

  afterEach(() => {
    layoutPrototypes().forEach((prototype) => {
      delete (prototype as { getBBox?: unknown }).getBBox;
    });
    document.body.replaceChildren();
  });

  it('não deixa o cartaz de erro solto no documento', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    const mermaid = await loadMermaid();

    await expect(
      renderAccessibleMermaid({
        chart: 'flowchart LR\nA[Início] --> B[Fim]',
        container,
        mermaid,
        locale: 'pt-BR',
        navigationEnabled: true,
        ariaLabel: 'Diagrama Mermaid',
      }),
    ).rejects.toThrow();

    expect(document.body.textContent).not.toContain('Syntax error in text');
    expect(document.body.querySelector('[id^="dmermaidA11y"]')).toBeNull();
  });
});
