import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Modal } from './Modal';

const originalOffsetParentDescriptor = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'offsetParent',
);

describe('Modal', () => {
  beforeEach(() => {
    Object.defineProperty(HTMLElement.prototype, 'offsetParent', {
      configurable: true,
      get() {
        return document.body;
      },
    });
  });

  afterEach(() => {
    if (originalOffsetParentDescriptor) {
      Object.defineProperty(HTMLElement.prototype, 'offsetParent', originalOffsetParentDescriptor);
    } else {
      delete (HTMLElement.prototype as { offsetParent?: unknown }).offsetParent;
    }
  });

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

  it('nao fecha no Escape quando o evento ja foi tratado', () => {
    const onClose = vi.fn();

    render(
      <Modal isOpen={true} onClose={onClose} title="Titulo">
        <button
          onKeyDown={(event) => {
            if (event.key === 'Escape') {
              event.preventDefault();
            }
          }}
        >
          Acao
        </button>
      </Modal>
    );

    const button = screen.getByRole('button', { name: 'Acao' });
    button.focus();
    fireEvent.keyDown(button, { key: 'Escape' });

    expect(onClose).not.toHaveBeenCalled();
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

  it('nao move foco inicial para controle oculto da arvore acessivel', () => {
    render(
      <Modal isOpen={true} onClose={vi.fn()} title="Titulo" allowClose={false}>
        <div aria-hidden="true">
          <button>Oculto</button>
        </div>
        <button>Visivel</button>
      </Modal>
    );

    expect(screen.getByRole('button', { name: 'Visivel' })).toHaveFocus();
  });

  it('foca o elemento do initialFocusSelector ao abrir', () => {
    render(
      <Modal isOpen={true} onClose={vi.fn()} title="Titulo" initialFocusSelector="#alvo">
        <input aria-label="Campo" />
        <pre id="alvo" tabIndex={0}>
          conteudo
        </pre>
      </Modal>
    );

    expect(document.getElementById('alvo')).toHaveFocus();
  });

  it('usa a heuristica padrao quando initialFocusSelector e invalido, sem lancar', () => {
    render(
      <Modal isOpen={true} onClose={vi.fn()} title="Titulo" initialFocusSelector=":::seletor-invalido(">
        <input aria-label="Campo" />
      </Modal>
    );

    expect(screen.getByRole('textbox', { name: 'Campo' })).toHaveFocus();
  });

  it('usa a heuristica padrao quando initialFocusSelector nao encontra elemento', () => {
    render(
      <Modal isOpen={true} onClose={vi.fn()} title="Titulo" initialFocusSelector="#inexistente">
        <input aria-label="Campo" />
      </Modal>
    );

    expect(screen.getByRole('textbox', { name: 'Campo' })).toHaveFocus();
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
