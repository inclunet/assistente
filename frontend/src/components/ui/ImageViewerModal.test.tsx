import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, createEvent } from '@testing-library/react';
import { ImageViewerModal, type ImageViewerImage } from './ImageViewerModal';
import { Modal } from './Modal';

const single: ImageViewerImage[] = [
  { src: 'http://example.com/a.png', alt: 'Foto A' },
];

const multiple: ImageViewerImage[] = [
  { src: 'http://example.com/a.png', alt: 'Foto A' },
  { src: 'http://example.com/b.png', alt: 'Foto B' },
];

describe('ImageViewerModal', () => {
  it('não renderiza nada quando fechado', () => {
    const { container } = render(
      <ImageViewerModal isOpen={false} images={single} onClose={vi.fn()} />,
    );
    expect(container).toBeEmptyDOMElement();
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('renderiza a imagem ampliada e o diálogo quando aberto', () => {
    render(<ImageViewerModal isOpen images={single} onClose={vi.fn()} />);

    expect(screen.getByRole('dialog')).toBeInTheDocument();
    const img = screen.getByAltText('Foto A');
    expect(img).toHaveAttribute('src', 'http://example.com/a.png');
  });

  it('fecha ao clicar no botão de fechar', () => {
    const onClose = vi.fn();
    render(<ImageViewerModal isOpen images={single} onClose={onClose} />);

    fireEvent.click(screen.getByRole('button', { name: 'ui.modal.close' }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('fecha ao pressionar Escape', () => {
    const onClose = vi.fn();
    render(<ImageViewerModal isOpen images={single} onClose={onClose} />);

    fireEvent.keyDown(document, { key: 'Escape' });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('aplica zoom in e zoom out via botões', () => {
    render(<ImageViewerModal isOpen images={single} onClose={vi.fn()} />);

    expect(screen.getByText('100%')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'ui.imageViewer.zoomIn' }));
    expect(screen.getByText('150%')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'ui.imageViewer.zoomOut' }));
    expect(screen.getByText('100%')).toBeInTheDocument();
  });

  it('aplica zoom com o scroll do mouse e previne o scroll do container', () => {
    render(<ImageViewerModal isOpen images={single} onClose={vi.fn()} />);

    const stage = document.querySelector('.image-viewer__stage') as HTMLElement;
    expect(stage).not.toBeNull();
    expect(screen.getByText('100%')).toBeInTheDocument();

    // Scroll para cima amplia e cancela o scroll do container.
    const wheelUp = createEvent.wheel(stage, { deltaY: -100 });
    fireEvent(stage, wheelUp);
    expect(wheelUp.defaultPrevented).toBe(true);
    expect(screen.getByText('150%')).toBeInTheDocument();

    // Scroll para baixo reduz e também cancela o scroll.
    const wheelDown = createEvent.wheel(stage, { deltaY: 100 });
    fireEvent(stage, wheelDown);
    expect(wheelDown.defaultPrevented).toBe(true);
    expect(screen.getByText('100%')).toBeInTheDocument();
  });

  it('desabilita zoom out no zoom mínimo', () => {
    render(<ImageViewerModal isOpen images={single} onClose={vi.fn()} />);

    expect(screen.getByRole('button', { name: 'ui.imageViewer.zoomOut' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'ui.imageViewer.resetZoom' })).toBeDisabled();
  });

  it('não mostra controles de navegação para uma única imagem', () => {
    render(<ImageViewerModal isOpen images={single} onClose={vi.fn()} />);

    expect(screen.queryByRole('button', { name: 'ui.imageViewer.next' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'ui.imageViewer.previous' })).toBeNull();
  });

  it('navega entre múltiplas imagens com os botões', () => {
    render(<ImageViewerModal isOpen images={multiple} initialIndex={0} onClose={vi.fn()} />);

    expect(screen.getByAltText('Foto A')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'ui.imageViewer.next' }));
    expect(screen.getByAltText('Foto B')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'ui.imageViewer.previous' }));
    expect(screen.getByAltText('Foto A')).toBeInTheDocument();
  });

  it('navega com as setas do teclado', () => {
    render(<ImageViewerModal isOpen images={multiple} initialIndex={0} onClose={vi.fn()} />);

    fireEvent.keyDown(document, { key: 'ArrowRight' });
    expect(screen.getByAltText('Foto B')).toBeInTheDocument();

    fireEvent.keyDown(document, { key: 'ArrowLeft' });
    expect(screen.getByAltText('Foto A')).toBeInTheDocument();
  });

  it('não responde a setas/zoom quando outro modal está por cima', () => {
    render(
      <>
        <ImageViewerModal isOpen images={multiple} initialIndex={0} onClose={vi.fn()} />
        <Modal isOpen onClose={vi.fn()} title="Modal do topo">
          <p>conteúdo do topo</p>
        </Modal>
      </>,
    );

    // O viewer está atrás do segundo modal (topo da stack).
    expect(screen.getByAltText('Foto A')).toBeInTheDocument();
    expect(screen.getByText('100%')).toBeInTheDocument();

    fireEvent.keyDown(document, { key: 'ArrowRight' });
    expect(screen.getByAltText('Foto A')).toBeInTheDocument();

    fireEvent.keyDown(document, { key: '+' });
    expect(screen.getByText('100%')).toBeInTheDocument();
  });

  it('volta a responder às teclas quando o modal do topo é fechado', () => {
    const { rerender } = render(
      <>
        <ImageViewerModal isOpen images={multiple} initialIndex={0} onClose={vi.fn()} />
        <Modal isOpen onClose={vi.fn()} title="Modal do topo">
          <p>conteúdo do topo</p>
        </Modal>
      </>,
    );

    fireEvent.keyDown(document, { key: 'ArrowRight' });
    expect(screen.getByAltText('Foto A')).toBeInTheDocument();

    // Fecha o modal do topo: o viewer volta a ser o modal ativo.
    rerender(
      <>
        <ImageViewerModal isOpen images={multiple} initialIndex={0} onClose={vi.fn()} />
        <Modal isOpen={false} onClose={vi.fn()} title="Modal do topo">
          <p>conteúdo do topo</p>
        </Modal>
      </>,
    );

    fireEvent.keyDown(document, { key: 'ArrowRight' });
    expect(screen.getByAltText('Foto B')).toBeInTheDocument();
  });
});
