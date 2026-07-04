import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QuestionnaireDialog } from './QuestionnaireDialog';

const announceMock = vi.hoisted(() => vi.fn());

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: announceMock,
  }),
}));

describe('QuestionnaireDialog', () => {
  beforeEach(() => {
    announceMock.mockClear();
    Object.defineProperty(HTMLElement.prototype, 'offsetParent', {
      configurable: true,
      get() {
        return document.body;
      },
    });
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
