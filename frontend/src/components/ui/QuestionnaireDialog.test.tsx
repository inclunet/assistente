import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QuestionnaireDialog } from './QuestionnaireDialog';
import { Modal } from './Modal';

const announceMock = vi.hoisted(() => vi.fn());
const restoreDefaultFocusMock = vi.hoisted(() => vi.fn(() => {
  document.querySelector<HTMLElement>('[aria-label="Editor Markdown"]')?.focus();
  return true;
}));
const originalOffsetParentDescriptor = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'offsetParent',
);

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: announceMock,
  }),
}));

vi.mock('../../hooks/useDefaultFocus', () => ({
  restoreDefaultFocus: restoreDefaultFocusMock,
}));

describe('QuestionnaireDialog', () => {
  beforeEach(() => {
    announceMock.mockClear();
    restoreDefaultFocusMock.mockClear();
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

  it('move foco para o primeiro controle e anuncia a abertura', async () => {
    render(
      <QuestionnaireDialog
        isOpen
        data={{
          id: 'q-focus',
          title: 'Conflito de edição',
          description: 'Escolha como resolver a alteração externa.',
          questions: [
            {
              id: 'choice',
              type: 'single_choice',
              prompt: 'Ação',
              required: true,
              options: ['Usar disco', 'Usar minha versão'],
            },
          ],
        }}
        onSubmit={vi.fn()}
      />
    );

    await waitFor(() => {
      expect(screen.getByRole('radio', { name: 'Usar disco' })).toHaveFocus();
    });

    expect(announceMock).toHaveBeenCalledWith(
      'Conflito de edição. Escolha como resolver a alteração externa.',
      'assertive'
    );
  });

  it('toma foco imediatamente quando alteração externa abre com editor focado', () => {
    const externalChangeData = {
      id: 'ui-editor-external-change-test',
      title: 'Alteração externa detectada',
      description: 'O arquivo foi alterado fora do Assistente.',
      submitLabel: 'Aplicar',
      cancelLabel: 'Agora não',
      questions: [
        {
          id: 'path',
          type: 'readonly_code' as const,
          prompt: 'Arquivo',
          content: 'C:/tmp/documento.md',
        },
        {
          id: 'disk',
          type: 'readonly_code' as const,
          prompt: 'Versão no disco',
          content: 'conteúdo externo',
        },
        {
          id: 'local',
          type: 'readonly_code' as const,
          prompt: 'Minha versão',
          content: 'conteúdo local',
        },
        {
          id: 'choice',
          type: 'single_choice' as const,
          prompt: 'Ação',
          required: true,
          options: ['Usar versão do disco', 'Resolver merge', 'Usar minha versão'],
          default: 'Usar versão do disco',
        },
      ],
    };

    const { rerender } = render(
      <>
        <textarea aria-label="Editor Markdown" />
        <QuestionnaireDialog
          isOpen={false}
          data={externalChangeData}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
        />
      </>
    );

    const editor = screen.getByRole('textbox', { name: 'Editor Markdown' });
    editor.focus();
    expect(editor).toHaveFocus();

    rerender(
      <>
        <textarea aria-label="Editor Markdown" />
        <QuestionnaireDialog
          isOpen
          data={externalChangeData}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
        />
      </>
    );

    expect(editor).not.toHaveFocus();
    expect(screen.getByRole('radio', { name: 'Usar versão do disco' })).toHaveFocus();
    expect(announceMock).toHaveBeenCalledWith(
      'Alteração externa detectada. O arquivo foi alterado fora do Assistente.',
      'assertive'
    );
  });

  it('mantem foco no conflito externo quando chat modal fecha na mesma abertura', async () => {
    const externalChangeData = {
      id: 'ui-editor-external-change-chat-close',
      title: 'Alteração externa detectada',
      description: 'O arquivo foi alterado fora do Assistente.',
      submitLabel: 'Aplicar',
      cancelLabel: 'Agora não',
      questions: [
        {
          id: 'path',
          type: 'readonly_code' as const,
          prompt: 'Arquivo',
          content: 'C:/tmp/documento.md',
        },
        {
          id: 'choice',
          type: 'single_choice' as const,
          prompt: 'Ação',
          required: true,
          options: ['Usar versão do disco', 'Resolver merge', 'Usar minha versão'],
          default: 'Usar versão do disco',
        },
      ],
    };

    const { rerender } = render(
      <>
        <textarea aria-label="Editor Markdown" />
        <Modal isOpen onClose={vi.fn()} title="Chat do editor">
          <textarea aria-label="Mensagem" />
        </Modal>
        <QuestionnaireDialog
          isOpen={false}
          data={externalChangeData}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
        />
      </>
    );

    const chatInput = screen.getByRole('textbox', { name: 'Mensagem' });
    expect(chatInput).toHaveFocus();

    rerender(
      <>
        <textarea aria-label="Editor Markdown" />
        <Modal isOpen={false} onClose={vi.fn()} title="Chat do editor">
          <textarea aria-label="Mensagem" />
        </Modal>
        <QuestionnaireDialog
          isOpen
          data={externalChangeData}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
        />
      </>
    );

    const decisionControl = screen.getByRole('radio', { name: 'Usar versão do disco' });
    expect(decisionControl).toHaveFocus();

    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));

    expect(decisionControl).toHaveFocus();
    expect(restoreDefaultFocusMock).not.toHaveBeenCalled();
  });

  it('anuncia todos os erros de validação obrigatória', async () => {
    const user = userEvent.setup();
    render(
      <QuestionnaireDialog
        isOpen
        data={{
          id: 'q1',
          title: 'Perguntas',
          questions: [
            { id: 'nome', type: 'text', prompt: 'Nome', required: true },
            { id: 'email', type: 'text', prompt: 'Email', required: true },
          ],
        }}
        onSubmit={vi.fn()}
      />
    );

    await user.click(screen.getByRole('button', { name: 'Enviar' }));

    expect(announceMock).toHaveBeenCalledWith(
      'Nome: Resposta obrigatória. Email: Resposta obrigatória',
      'assertive'
    );
  });
});
