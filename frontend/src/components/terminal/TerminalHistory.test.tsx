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

  it.each(['hoje', ''])('agrupa a saída da sessão no nó anterior e navega para o próximo comando (saída %s)', output => {
    render(<TerminalHistory entries={[
      new terminal.HistoryEntry({ source: 'llm', id: 'cmd', command: 'date', output, exitCode: 0 }),
      new terminal.HistoryEntry({ source: 'session-output', id: 'saída', command: '', output: 'PS C:\\> ' }),
      new terminal.HistoryEntry({ source: 'llm', id: 'próximo', command: 'pwd', output: '', exitCode: 0 }),
    ]} />);
    const nodes = screen.getAllByRole('listitem');
    expect(nodes).toHaveLength(output ? 3 : 2);
    const previous = nodes[nodes.length - 2];
    expect(previous).toHaveAccessibleName(/terminal\.entry\.sessionOutput: PS C:/);
    expect(previous.textContent).toContain('PS C:\\> ');
    previous.focus();
    fireEvent.keyDown(previous, { key: 'ArrowDown' });
    expect(nodes[nodes.length - 1]).toHaveFocus();
    expect(nodes[nodes.length - 1]).toHaveAccessibleName(/pwd/);
  });

  it('mantém a saída anterior ao primeiro comando como item navegável e legível', () => {
    render(<TerminalHistory entries={[
      new terminal.HistoryEntry({ source: 'session-output', id: 'inicial', command: '', output: '  pronto' }),
      new terminal.HistoryEntry({ source: 'session-output', id: 'continuação', command: '', output: ' para uso' }),
    ]} />);
    const node = screen.getByRole('listitem', { name: 'terminal.entry.sessionOutput: pronto para uso' });
    node.focus();
    expect(node).toHaveFocus();
    expect(node).not.toHaveTextContent('terminal.entry.exit');
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
