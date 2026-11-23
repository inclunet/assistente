import { describe, expect, it, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { TerminalHistory } from './TerminalHistory';
import { terminal } from '../../../wailsjs/go/models';

import { registerAnnouncerSink, unregisterAnnouncerSink } from '../../services/voiceAccessibility/announcerBroker';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

describe('TerminalHistory', () => {
  beforeEach(() => {
    Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', {
      value: vi.fn(),
      writable: true,
    });
  });

  it('abrir o histórico carregado mantém a falha sem saída acessível e silenciosa', () => {
    const sink = vi.fn();
    registerAnnouncerSink(sink);
    try {
      const { rerender } = render(<TerminalHistory entries={[]} />);
      const failed = new terminal.HistoryEntry({ id: 'falha', command: 'anterior', output: '', exitCode: 1,
        endedAt: '2026-01-01T00:00:01Z', source: 'llm' });
      const previous = Array.from({ length: 199 }, (_, i) => new terminal.HistoryEntry({
        id: String(i), command: 'pwd', output: '', exitCode: 0,
      }));
      rerender(<TerminalHistory entries={[...previous, failed]} />);
      expect(sink).not.toHaveBeenCalled();
      expect(screen.getByText('terminal.entry.exit 1')).toBeInTheDocument();
    } finally { unregisterAnnouncerSink(); }
  });

  it('oferece uma única saída navegável para texto da sessão sem comando', () => {
    render(<TerminalHistory entries={[
      new terminal.HistoryEntry({ source: 'llm', id: 'cmd', command: 'date', output: 'hoje', exitCode: 0 }),
      new terminal.HistoryEntry({ source: 'session-output', id: 'saída', command: '', output: 'pronto' }),
    ]} />);
    const nodes = screen.getAllByRole('listitem');
    expect(nodes).toHaveLength(3);
    nodes[1].focus();
    fireEvent.keyDown(nodes[1], { key: 'ArrowDown' });
    expect(nodes[2]).toHaveFocus();
    expect(nodes[2]).toHaveAccessibleName('terminal.entry.sessionOutput: pronto');
    expect(nodes[2]).not.toHaveTextContent('terminal.entry.exit');
  });

  it('renderiza estado vazio', () => {
    render(
      <TerminalHistory
        entries={[]}
      />
    );

    expect(screen.getByText(/terminal\.history\.emptyTitle/)).toBeInTheDocument();
  });

  it('renderiza lista de nos', () => {
    render(
      <TerminalHistory
        entries={[
          new terminal.HistoryEntry({
            id: '1',
            command: 'ls',
            output: 'ok',
            exitCode: 0,
            startedAt: new Date().toISOString(),
            endedAt: new Date().toISOString(),
            source: 'user',
          }),
        ]}
      />
    );

    expect(screen.getAllByRole('listitem')).toHaveLength(2);
  });
});
