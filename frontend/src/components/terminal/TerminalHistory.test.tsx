import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
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
