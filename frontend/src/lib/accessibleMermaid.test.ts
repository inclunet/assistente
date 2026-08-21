import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, waitFor } from '@testing-library/dom';
import { loadMermaid, renderAccessibleMermaid } from './accessibleMermaid';

const mocks = vi.hoisted(() => ({
  announce: vi.fn(),
  initialize: vi.fn(),
  playBumpSound: vi.fn(),
  renderAccessibleDiagram: vi.fn(),
}));

vi.mock('mermaid', () => ({
  default: {
    initialize: mocks.initialize,
    render: vi.fn(),
  },
}));

vi.mock('../hooks/useAnnouncer', () => ({
  announce: mocks.announce,
}));

vi.mock('../services/audioFeedback', () => ({
  playBumpSound: mocks.playBumpSound,
}));

vi.mock('@inclunet/mermaid-a11y/mermaid', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@inclunet/mermaid-a11y/mermaid')>();
  return {
    ...actual,
    renderAccessibleDiagram: mocks.renderAccessibleDiagram,
  };
});

const graph = {
  diagramType: 'flowchart-v2',
  direction: 'LR',
  nodes: [
    { id: 'A', label: 'Início', svgId: 'node-a' },
    { id: 'B', label: 'Fim', svgId: 'node-b' },
  ],
  edges: [
    {
      id: 'A-B',
      source: 'A',
      target: 'B',
      directed: true,
      svgId: 'edge-a-b',
    },
  ],
};

function createRenderedGraph(nodes = graph.nodes) {
  const host = document.createElement('div');
  host.innerHTML = `
    <svg>
      <g id="node-a"><rect /></g>
      <g id="node-b"><rect /></g>
      <path id="edge-a-b" />
    </svg>
  `;
  return {
    svg: host.querySelector('svg') as unknown as SVGSVGElement,
    graph: { ...graph, nodes },
    diagramType: 'flowchart-v2',
  };
}

describe('renderAccessibleMermaid', () => {
  beforeEach(() => {
    mocks.announce.mockReset();
    mocks.initialize.mockReset();
    mocks.playBumpSound.mockReset();
    mocks.renderAccessibleDiagram.mockImplementation(async ({ host }: { host: HTMLElement }) => {
      const rendered = createRenderedGraph();
      host.replaceChildren(rendered.svg);
      return rendered;
    });
  });

  afterEach(() => {
    document.body.replaceChildren();
  });

  it('anexa navegação sem criar uma live region local', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);

    const result = await renderAccessibleMermaid({
      chart: 'flowchart LR\nA-->B',
      container,
      mermaid: { render: vi.fn() },
      locale: 'pt-BR',
      navigationEnabled: true,
      ariaLabel: 'Diagrama Mermaid',
    });

    expect(result.navigable).toBe(true);
    expect(container).toHaveAttribute('tabindex', '0');
    expect(container.querySelector('[aria-live]')).toBeNull();
    expect(container.querySelector('[role="status"], [role="alert"]')).toBeNull();

    container.focus();
    fireEvent.keyDown(container, { key: 'd' });

    await waitFor(() => {
      expect(mocks.announce).toHaveBeenCalledWith(expect.stringContaining('Início'));
    });
  });

  it('mantém o SVG sem tab stop quando o tipo não produz nós', async () => {
    mocks.renderAccessibleDiagram.mockImplementationOnce(async ({ host }: { host: HTMLElement }) => {
      const rendered = createRenderedGraph([]);
      host.replaceChildren(rendered.svg);
      return rendered;
    });
    const container = document.createElement('div');

    const result = await renderAccessibleMermaid({
      chart: 'sequenceDiagram',
      container,
      mermaid: { render: vi.fn() },
      navigationEnabled: true,
      ariaLabel: 'Diagrama Mermaid',
    });

    expect(result.navigable).toBe(false);
    expect(container).toHaveAttribute('tabindex', '-1');
    expect(container.querySelector('svg')).not.toBeNull();
  });

  it('não anexa o navigator fora do modo de leitura e limpa o SVG', async () => {
    const container = document.createElement('div');

    const result = await renderAccessibleMermaid({
      chart: 'flowchart LR\nA-->B',
      container,
      mermaid: { render: vi.fn() },
      navigationEnabled: false,
      ariaLabel: 'Diagrama Mermaid',
    });

    expect(result.navigable).toBe(false);
    expect(container).toHaveAttribute('tabindex', '-1');

    result.cleanup();
    expect(container).toBeEmptyDOMElement();
  });
});

describe('loadMermaid', () => {
  it('limpa a Promise rejeitada para permitir uma nova tentativa', async () => {
    mocks.initialize
      .mockImplementationOnce(() => {
        throw new Error('falha transitória');
      })
      .mockImplementationOnce(() => undefined);

    await expect(loadMermaid()).rejects.toThrow('falha transitória');
    await expect(loadMermaid()).resolves.toMatchObject({
      initialize: mocks.initialize,
    });
    expect(mocks.initialize).toHaveBeenCalledTimes(2);
  });
});
