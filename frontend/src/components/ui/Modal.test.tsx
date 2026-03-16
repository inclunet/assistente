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
});
