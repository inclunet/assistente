import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { TerminalCommandNode, TerminalOutputNode } from './TerminalEntry';
import { terminal } from '../../../wailsjs/go/models';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
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

  it('renderiza saida com exit code', () => {
    render(
      <TerminalOutputNode
        entry={new terminal.HistoryEntry({
          id: '1',
          command: 'ls',
          output: 'ok',
          exitCode: 1,
          startedAt: new Date().toISOString(),
          endedAt: new Date().toISOString(),
          source: 'user',
        })}
      />
    );

    expect(screen.getByText('ok')).toBeInTheDocument();
    expect(screen.getByText('terminal.entry.exit 1')).toBeInTheDocument();
  });
});
