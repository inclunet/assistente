import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { TerminalPicker } from './TerminalPicker';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, params?: Record<string, string>) => {
      if (key === 'terminal.picker.label') return 'Terminal conectado';
      if (key === 'terminal.picker.description') return 'Escolha o terminal';
      if (key === 'terminal.picker.placeholder') return 'Buscar terminal';
      if (key === 'terminal.picker.empty') return 'Nenhum terminal vivo';
      if (key === 'pickers.base.empty') return 'Nenhuma opção disponível';
      if (key === 'terminal.picker.itemDescription') return `${params?.state} — ${params?.cwd}`;
      if (key === 'terminal.states.idle') return 'ocioso';
      if (key === 'terminal.states.running') return 'executando';
      return key;
    },
  }),
}));

describe('TerminalPicker', () => {
  it('lista sessões vivas e seleciona pelo ID explícito', async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    render(
      <TerminalPicker
        sessions={[
          { id: 'term-1', name: 'Build', cwd: '/repo', state: 'idle', shell: 'bash', createdAt: '', lastUsed: '' },
          { id: 'term-2', name: 'Testes', cwd: '/repo', state: 'running', shell: 'bash', createdAt: '', lastUsed: '' },
        ]}
        value="term-1"
        onChange={onChange}
      />,
    );

    await user.click(screen.getByRole('button', { name: /Terminal conectado, Build/i }));
    await user.click(await screen.findByRole('option', { name: /Testes/i }));

    expect(onChange).toHaveBeenCalledWith('term-2');
  });

  it('informa quando não existem terminais vivos', () => {
    render(<TerminalPicker sessions={[]} onChange={vi.fn()} />);

    expect(screen.getByText('Nenhum terminal vivo')).toBeInTheDocument();
  });

  it('usa o ID como nome acessível quando a sessão não tem nome', async () => {
    const user = userEvent.setup();
    render(
      <TerminalPicker
        sessions={[
          { id: 'term-sem-nome', name: '', cwd: '/repo', state: 'idle', shell: 'bash', createdAt: '', lastUsed: '' },
        ]}
        onChange={vi.fn()}
      />,
    );

    await user.click(screen.getByRole('button', { name: /Terminal conectado/i }));

    expect(await screen.findByRole('option', { name: /term-sem-nome/i })).toBeInTheDocument();
  });
});
