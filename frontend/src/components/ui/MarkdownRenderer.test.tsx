import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { MarkdownRenderer } from './MarkdownRenderer';

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

describe('MarkdownRenderer', () => {
  it('renderiza markdown e ajusta links para nova aba', () => {
    render(<MarkdownRenderer content="[Link](http://example.com)" />);

    const link = screen.getByRole('link', { name: 'Link' });
    expect(link).toHaveAttribute('target', '_blank');
    expect(link).toHaveAttribute('rel', 'noopener noreferrer');
    expect(link).toHaveAttribute('tabindex', '-1');
  });

  it('torna imagens interativas e foca acessível', () => {
    render(<MarkdownRenderer content={'![Gato](http://example.com/cat.png)'} />);

    const img = screen.getByAltText('Gato');
    expect(img).toHaveAttribute('role', 'button');
    expect(img).toHaveAttribute('tabindex', '0');
    expect(img).toHaveClass('markdown-image--interactive');
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
});
