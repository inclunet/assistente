import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Modal } from './Modal';

describe('Modal', () => {
  it('renderiza conteudo quando aberto e fecha no botao', () => {
    const onClose = vi.fn();

    render(
      <Modal isOpen={true} onClose={onClose} title="Titulo">
        <button>Acao</button>
      </Modal>
    );

    expect(screen.getByRole('dialog')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'ui.modal.close' }));
    expect(onClose).toHaveBeenCalled();
  });

  it('expoe aria-modal="true" no dialogo', () => {
    render(
      <Modal isOpen={true} onClose={vi.fn()} title="Titulo">
        <button>Acao</button>
      </Modal>
    );

    expect(screen.getByRole('dialog')).toHaveAttribute('aria-modal', 'true');
  });

  it('fecha ao pressionar Escape', () => {
    const onClose = vi.fn();

    render(
      <Modal isOpen={true} onClose={onClose} title="Titulo">
        <button>Acao</button>
      </Modal>
    );

    fireEvent.keyDown(document, { key: 'Escape' });
    expect(onClose).toHaveBeenCalled();
  });

  it('nao renderiza quando fechado', () => {
    const onClose = vi.fn();

    render(
      <Modal isOpen={false} onClose={onClose} title="Titulo">
        <button>Acao</button>
      </Modal>
    );

    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('aplica role="application" no corpo por padrao (forca focus mode no NVDA)', () => {
    render(
      <Modal isOpen={true} onClose={vi.fn()} title="Configuracoes">
        <button>Salvar</button>
      </Modal>
    );

    const application = screen.getByRole('application');
    expect(application).toHaveClass('modal-body');
    expect(screen.queryByRole('document')).toBeNull();
  });

  it('aplica role="document" no corpo quando readingMode esta ligado', () => {
    render(
      <Modal isOpen={true} onClose={vi.fn()} title="Detalhes" readingMode>
        <p>Texto para leitura</p>
      </Modal>
    );

    const document = screen.getByRole('document');
    expect(document).toHaveClass('modal-body');
    expect(screen.queryByRole('application')).toBeNull();
  });
});
