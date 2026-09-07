import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
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

  it('nao move foco inicial para controle oculto da arvore acessivel', async () => {
    render(
      <Modal isOpen={true} onClose={vi.fn()} title="Titulo" allowClose={false}>
        <div aria-hidden="true">
          <button>Oculto</button>
        </div>
        <button>Visivel</button>
      </Modal>
    );

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Visivel' })).toHaveFocus();
    });
  });

  it('foca o elemento do initialFocusSelector ao abrir', async () => {
    render(
      <Modal isOpen={true} onClose={vi.fn()} title="Titulo" initialFocusSelector="#alvo">
        <input aria-label="Campo" />
        <pre id="alvo" tabIndex={0}>
          conteudo
        </pre>
      </Modal>
    );

    await waitFor(() => {
      expect(document.getElementById('alvo')).toHaveFocus();
    });
  });

  it('usa a heuristica padrao quando initialFocusSelector aponta para elemento nao focavel', async () => {
    render(
      <Modal isOpen={true} onClose={vi.fn()} title="Titulo" initialFocusSelector="#nao-focavel">
        <div id="nao-focavel">texto sem tabindex</div>
        <input aria-label="Campo" />
      </Modal>
    );

    await waitFor(() => {
      expect(screen.getByRole('textbox', { name: 'Campo' })).toHaveFocus();
    });
  });

  it('usa a heuristica padrao quando initialFocusSelector e invalido, sem lancar', async () => {
    render(
      <Modal isOpen={true} onClose={vi.fn()} title="Titulo" initialFocusSelector=":::seletor-invalido(">
        <input aria-label="Campo" />
      </Modal>
    );

    await waitFor(() => {
      expect(screen.getByRole('textbox', { name: 'Campo' })).toHaveFocus();
    });
  });

  it('usa a heuristica padrao quando initialFocusSelector nao encontra elemento', async () => {
    render(
      <Modal isOpen={true} onClose={vi.fn()} title="Titulo" initialFocusSelector="#inexistente">
        <input aria-label="Campo" />
      </Modal>
    );

    await waitFor(() => {
      expect(screen.getByRole('textbox', { name: 'Campo' })).toHaveFocus();
    });
  });

  it('aplica o foco inicial apos o paint (rAF), nao sincronamente na insercao', async () => {
    render(
      <Modal isOpen={true} onClose={vi.fn()} title="Titulo">
        <input aria-label="Campo" />
      </Modal>
    );

    // No mesmo tick da inserção do portal o foco ainda NÃO foi aplicado:
    // aplicar foco no frame de inserção faz o NVDA perder o evento
    // (árvore de acessibilidade do subtree ainda não existe).
    const input = screen.getByRole('textbox', { name: 'Campo' });
    expect(input).not.toHaveFocus();

    await waitFor(() => {
      expect(input).toHaveFocus();
    });
  });

  it('foca o radio marcado do grupo, nao o primeiro radio', async () => {
    render(
      <Modal isOpen={true} onClose={vi.fn()} title="Titulo">
        <label>
          <input type="radio" name="acao" value="disco" />
          Usar versao do disco
        </label>
        <label>
          <input type="radio" name="acao" value="minha" defaultChecked />
          Manter minha versao
        </label>
      </Modal>
    );

    await waitFor(() => {
      expect(screen.getByRole('radio', { name: 'Manter minha versao' })).toHaveFocus();
    });
    expect(screen.getByRole('radio', { name: 'Usar versao do disco' })).not.toHaveFocus();
  });

  it('foca o primeiro radio quando nenhum membro do grupo esta marcado', async () => {
    render(
      <Modal isOpen={true} onClose={vi.fn()} title="Titulo">
        <label>
          <input type="radio" name="acao" value="a" />
          Opcao A
        </label>
        <label>
          <input type="radio" name="acao" value="b" />
          Opcao B
        </label>
      </Modal>
    );

    await waitFor(() => {
      expect(screen.getByRole('radio', { name: 'Opcao A' })).toHaveFocus();
    });
  });

  it('reaplica o foco na verificacao (~150ms) se algo roubou o foco apos abrir', async () => {
    render(
      <>
        <button>Fora do modal</button>
        <Modal isOpen={true} onClose={vi.fn()} title="Titulo">
          <input aria-label="Campo" />
        </Modal>
      </>
    );

    const input = screen.getByRole('textbox', { name: 'Campo' });
    await waitFor(() => {
      expect(input).toHaveFocus();
    });

    // Simula roubo de foco logo após a abertura (ex.: efeito tardio de outro
    // componente devolvendo foco à página).
    screen.getByRole('button', { name: 'Fora do modal' }).focus();
    expect(input).not.toHaveFocus();

    // A verificação única de ~150ms deve trazer o foco de volta ao modal.
    await waitFor(() => {
      expect(input).toHaveFocus();
    });
  });

  it('cancela rAF/timeout pendentes quando o modal fecha rapido', async () => {
    const cancelRafSpy = vi.spyOn(window, 'cancelAnimationFrame');

    const { rerender } = render(
      <>
        <input aria-label="Editor" />
        <Modal isOpen={true} onClose={vi.fn()} title="Titulo" returnFocusOnClose={false}>
          <input aria-label="Campo" />
        </Modal>
      </>
    );

    const editor = screen.getByRole('textbox', { name: 'Editor' });
    editor.focus();

    // Fecha antes de o rAF de foco disparar: o cleanup deve cancelar o
    // agendamento e o foco não pode ser puxado para um modal já fechado.
    rerender(
      <>
        <input aria-label="Editor" />
        <Modal isOpen={false} onClose={vi.fn()} title="Titulo" returnFocusOnClose={false}>
          <input aria-label="Campo" />
        </Modal>
      </>
    );

    expect(cancelRafSpy).toHaveBeenCalled();

    // Espera mais que o double-rAF + verificação de 150ms: nada deve roubar o foco.
    await new Promise((resolve) => setTimeout(resolve, 250));
    expect(editor).toHaveFocus();

    cancelRafSpy.mockRestore();
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
