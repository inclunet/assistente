import { describe, expect, it, vi } from 'vitest';
import type { ReactNode } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QuestionnaireDialog } from './QuestionnaireDialog';

const announceMock = vi.hoisted(() => vi.fn());

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: announceMock,
  }),
}));

vi.mock('./Modal', () => ({
  Modal: ({ isOpen, title, children }: { isOpen: boolean; title?: string; children: ReactNode }) =>
    (isOpen ? <div role="dialog" aria-label={title}>{children}</div> : null),
}));

describe('QuestionnaireDialog', () => {
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
