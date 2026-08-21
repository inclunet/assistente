import { StrictMode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, within, waitFor } from '@testing-library/react';
import { MarkdownRenderer } from './MarkdownRenderer';

const mermaidMocks = vi.hoisted(() => ({
  initialize: vi.fn(),
  render: vi.fn(),
}));

vi.mock('react-router-dom', () => ({
  useNavigate: () => vi.fn(),
}));

vi.mock('../../hooks/useAnchoredContextMenu', () => ({
  useAnchoredContextMenu: () => ({
    menu: { items: [], x: 0, y: 0, visible: false, ariaLabel: '' },
    openAtPoint: vi.fn(),
    closeMenu: vi.fn(),
    onSelectItem: vi.fn(),
  }),
}));

vi.mock('mermaid', () => ({
  default: mermaidMocks,
}));

describe('MarkdownRenderer', () => {
  beforeEach(() => {
    mermaidMocks.initialize.mockClear();
    mermaidMocks.render.mockReset();
    mermaidMocks.render.mockResolvedValue({ svg: '<svg aria-label="Diagrama"></svg>' });
  });

  it('renderiza markdown e ajusta links para nova aba', () => {
    render(<MarkdownRenderer content="[Link](http://example.com)" />);

    const link = screen.getByRole('link', { name: 'Link' });
    expect(link).toHaveAttribute('target', '_blank');
    expect(link).toHaveAttribute('rel', 'noopener noreferrer');
    expect(link).toHaveAttribute('tabindex', '-1');
  });

  it('habilita links na ordem de Tab somente quando a região está em leitura', () => {
    const { rerender } = render(
      <MarkdownRenderer content="[Link](http://example.com)" tabNavigation="disabled" />,
    );
    expect(screen.getByRole('link', { name: 'Link' })).toHaveAttribute('tabindex', '-1');

    rerender(
      <MarkdownRenderer content="[Link](http://example.com)" tabNavigation="enabled" />,
    );
    expect(screen.getByRole('link', { name: 'Link' })).toHaveAttribute('tabindex', '0');
  });

  it('torna imagens interativas sem criar tab stop fora da região de leitura', () => {
    render(<MarkdownRenderer content={'![Gato](http://example.com/cat.png)'} />);

    const img = screen.getByAltText('Gato');
    expect(img).toHaveAttribute('role', 'button');
    expect(img).toHaveAttribute('tabindex', '-1');
    expect(img).toHaveClass('markdown-image--interactive');
  });

  it('habilita imagens interativas na ordem de Tab durante a leitura', () => {
    render(
      <MarkdownRenderer
        content={'![Gato](http://example.com/cat.png)'}
        tabNavigation="enabled"
      />,
    );

    expect(screen.getByAltText('Gato')).toHaveAttribute('tabindex', '0');
  });

  it('abre o visualizador de imagem ao clicar', () => {
    render(<MarkdownRenderer content={'![Gato](http://example.com/cat.png)'} />);

    expect(screen.queryByRole('dialog')).toBeNull();

    fireEvent.click(screen.getByAltText('Gato'));

    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('abre o visualizador com Enter no teclado', () => {
    render(<MarkdownRenderer content={'![Gato](http://example.com/cat.png)'} />);

    fireEvent.keyDown(screen.getByAltText('Gato'), { key: 'Enter' });

    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('não abre o visualizador em clique modificado dentro de um link', () => {
    render(
      <MarkdownRenderer
        content={'[![Gato](http://example.com/cat.png)](http://example.com/page)'}
      />,
    );

    const img = screen.getByAltText('Gato');
    expect(img.closest('a[href]')).not.toBeNull();

    fireEvent.click(img, { ctrlKey: true });
    expect(screen.queryByRole('dialog')).toBeNull();

    fireEvent.click(img, { metaKey: true });
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('não abre o visualizador em middle-click dentro de um link', () => {
    render(
      <MarkdownRenderer
        content={'[![Gato](http://example.com/cat.png)](http://example.com/page)'}
      />,
    );

    fireEvent.click(screen.getByAltText('Gato'), { button: 1 });
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('abre o visualizador em clique simples mesmo dentro de um link', () => {
    render(
      <MarkdownRenderer
        content={'[![Gato](http://example.com/cat.png)](http://example.com/page)'}
      />,
    );

    fireEvent.click(screen.getByAltText('Gato'));
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('usa uma única parada de Tab para imagem interativa dentro de link', () => {
    const { container } = render(
      <MarkdownRenderer
        content={'[![Gato](http://example.com/cat.png)](http://example.com/page)'}
        tabNavigation="enabled"
      />,
    );

    const link = container.querySelector('a[href]');
    const image = screen.getByAltText('Gato');
    expect(link).not.toBeNull();
    expect(link).toHaveAttribute('role', 'button');
    expect(link).toHaveAttribute('tabindex', '0');
    expect(image).not.toHaveAttribute('role');
    expect(image).not.toHaveAttribute('tabindex');
    expect(container.querySelectorAll('[tabindex="0"]')).toHaveLength(1);

    fireEvent.keyDown(link!, { key: 'Enter' });
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('ignora imagens sem src e não as torna interativas', () => {
    render(<MarkdownRenderer content={'![vazia]() ![Gato](http://example.com/cat.png)'} />);

    const empty = screen.getByAltText('vazia');
    expect(empty).not.toHaveAttribute('role', 'button');
    expect(empty).not.toHaveClass('markdown-image--interactive');

    const valid = screen.getByAltText('Gato');
    expect(valid).toHaveAttribute('role', 'button');
  });

  it('clicar numa imagem válida abre a correta mesmo com imagem inválida antes', () => {
    render(<MarkdownRenderer content={'![vazia]() ![Gato](http://example.com/cat.png)'} />);

    fireEvent.click(screen.getByAltText('Gato'));

    const dialog = screen.getByRole('dialog');
    expect(within(dialog).getByAltText('Gato')).toBeInTheDocument();
    expect(within(dialog).queryByAltText('vazia')).toBeNull();
  });

  it('a navegação do viewer percorre apenas imagens válidas', () => {
    render(
      <MarkdownRenderer
        content={'![vazia]() ![Gato](http://example.com/cat.png) ![Cachorro](http://example.com/dog.png)'}
      />,
    );

    fireEvent.click(screen.getByAltText('Gato'));
    const dialog = screen.getByRole('dialog');
    expect(within(dialog).getByAltText('Gato')).toBeInTheDocument();

    // Só há 2 imagens válidas: voltar a partir da primeira leva à última (Cachorro),
    // nunca à imagem de src vazio.
    fireEvent.keyDown(document, { key: 'ArrowLeft' });
    expect(within(dialog).getByAltText('Cachorro')).toBeInTheDocument();
    expect(within(dialog).queryByAltText('vazia')).toBeNull();

    fireEvent.keyDown(document, { key: 'ArrowRight' });
    expect(within(dialog).getByAltText('Gato')).toBeInTheDocument();
  });

  it('não renderiza o mesmo bloco Mermaid duas vezes durante renderização assíncrona', async () => {
    render(
      <StrictMode>
        <MarkdownRenderer content={'```mermaid\ngraph TD\nA-->B\n```'} />
      </StrictMode>,
    );

    await waitFor(() => {
      expect(screen.getAllByRole('group', { name: 'Diagrama Mermaid' })).toHaveLength(1);
    });
  });

  it('mostra erro Mermaid compacto sem despejar stack no preview', async () => {
    mermaidMocks.render.mockRejectedValue(new Error('Syntax error in text\nmermaid version 11.14.0'));

    render(<MarkdownRenderer content={'```mermaid\ntexto inválido\n```'} />);

    expect(await screen.findByText('Erro ao renderizar Mermaid')).toBeInTheDocument();
    const error = screen.getByText(/Syntax error in text/);
    expect(error).toHaveTextContent('mermaid version 11.14.0');
    expect(error.textContent).not.toMatch(/\bat\s+/);
  });
});
