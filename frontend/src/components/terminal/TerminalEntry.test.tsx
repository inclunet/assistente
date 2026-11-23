import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { TerminalCommandNode, TerminalOutputNode } from './TerminalEntry';
import { terminal } from '../../../wailsjs/go/models';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  announce: vi.fn(),
}));

describe('TerminalEntry', () => {
  it('renderiza comando', () => {
    render(
      <TerminalCommandNode
        entry={new terminal.HistoryEntry({
          id: '1',
          command: 'ls',
          output: '',
          exitCode: 0,
          startedAt: new Date().toISOString(),
          endedAt: new Date().toISOString(),
          source: 'user',
        })}
      />
    );

    expect(screen.getByText('ls')).toBeInTheDocument();
    expect(screen.getByText('terminal.entry.command')).toBeInTheDocument();
  });

  it.each(['', 'x'])('preserva o emoji no limite do rótulo acessível (excedente: "%s")', tail => {
    const prefix = 'a'.repeat(499) + '🌍';
    const entry = new terminal.HistoryEntry({ id: 'id', command: prefix + tail, source: 'user' });
    render(<TerminalCommandNode entry={entry} />);
    const label = screen.getByRole('listitem').getAttribute('aria-label');
    expect(label?.endsWith(prefix + (tail ? '… terminal.entry.truncated' : ''))).toBe(true);
  });

  it.each([1, -999])('renderiza saída com código final %s', exitCode => {
    render(
      <TerminalOutputNode
        entry={new terminal.HistoryEntry({
          id: '1',
          command: 'ls',
          output: 'ok',
          exitCode,
          startedAt: new Date().toISOString(),
          endedAt: new Date().toISOString(),
          source: 'user',
        })}
      />
    );

    expect(screen.getByText('ok')).toBeInTheDocument();
    expect(screen.getByText(`terminal.entry.exit ${exitCode}`)).toBeInTheDocument();
  });

  it('permite leitura livre do conteúdo do comando', () => {
    const { container } = render(
      <TerminalCommandNode
        entry={new terminal.HistoryEntry({
          id: '1',
          command: 'git status --short',
          output: '',
          exitCode: 0,
          startedAt: new Date().toISOString(),
          endedAt: new Date().toISOString(),
          source: 'user',
        })}
      />
    );

    const node = screen.getByRole('listitem');
    node.focus();
    fireEvent.keyDown(node, { key: 'Enter' });

    const content = container.querySelector<HTMLElement>('.terminal-node__content');
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(content).toHaveAttribute('role', 'document');
    expect(content).toHaveFocus();

    fireEvent.keyDown(content!, { key: 'Escape' });
    expect(node).toHaveAttribute('role', 'listitem');
    expect(node).toHaveFocus();
  });

  it('entra em modo de leitura com Enter e restaura a navegação com Escape', () => {
    const onNavigateNext = vi.fn();
    const { container } = render(
      <TerminalOutputNode
        entry={new terminal.HistoryEntry({
          id: '1',
          command: 'dir',
          output: 'arquivo 1\narquivo 2',
          exitCode: 0,
          startedAt: new Date().toISOString(),
          endedAt: new Date().toISOString(),
          source: 'user',
        })}
        onNavigateNext={onNavigateNext}
      />
    );

    const node = screen.getByRole('listitem');
    node.focus();
    fireEvent.keyDown(node, { key: 'Enter' });

    const dialog = screen.getByRole('dialog', { name: 'terminal.entry.readingDialog' });
    const content = container.querySelector<HTMLElement>('.terminal-node__content');
    expect(content).toHaveAttribute('role', 'document');
    expect(content).toHaveFocus();

    fireEvent.keyDown(content!, { key: 'Escape' });

    expect(dialog).toHaveAttribute('role', 'listitem');
    expect(dialog).toHaveFocus();
    expect(content).not.toHaveAttribute('role');

    fireEvent.keyDown(dialog, { key: 'ArrowDown' });
    expect(onNavigateNext).toHaveBeenCalledOnce();
  });
});
