import { expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MenuButton } from './MenuButton';

it('fecha opções obsoletas pelo fluxo normal e devolve foco ao mesmo gatilho', async () => {
  const action = vi.fn();
  const { rerender } = render(<MenuButton buttonLabel="Ações" contextKey="ready" items={[{ id: 'edit', label: 'Editar', onClick: action }]} />);
  const trigger = screen.getByRole('button', { name: 'Ações' });
  await userEvent.click(trigger);
  screen.getByRole('menuitem', { name: 'Editar' }).focus();
  rerender(<MenuButton buttonLabel="Ações" contextKey="loading" items={[{ id: 'edit', label: 'Editar', disabled: true, onClick: action }]} />);
  expect(screen.queryByRole('menu')).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Ações' })).toBe(trigger);
  await waitFor(() => expect(trigger).toHaveFocus());
  expect(action).not.toHaveBeenCalled();
  await userEvent.click(trigger);
  expect(screen.getByRole('menuitem', { name: 'Editar' })).toBeDisabled();
});

it('não troca nem desfoca o gatilho fechado quando o contexto muda', () => {
  const { rerender } = render(<MenuButton buttonLabel="Ações" contextKey="one" items={[]} />);
  const trigger = screen.getByRole('button', { name: 'Ações' });
  trigger.focus();
  rerender(<MenuButton buttonLabel="Ações" contextKey="two" items={[]} />);
  expect(screen.getByRole('button', { name: 'Ações' })).toBe(trigger);
  expect(trigger).toHaveFocus();
});
