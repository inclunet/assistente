import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { TerminalHistory } from './TerminalHistory';
import { terminal } from '../../../wailsjs/go/models';

import { registerAnnouncerSink, registerVoiceAccessibilityActiveResolver, unregisterAnnouncerSink } from '../../services/voiceAccessibility/announcerBroker';

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

  it('anuncia o novo comando apenas na aba visível e mantém o código final sem saída no histórico', () => {
    const sink = vi.fn();
    registerAnnouncerSink(sink);
    const unregister = registerVoiceAccessibilityActiveResolver(origin => origin?.tabId === 'visível');
    const initial = Array.from({ length: 200 }, (_, i) => new terminal.HistoryEntry({ id: String(i), command: 'anterior', output: '', exitCode: 0 }));
    const panes = (show: boolean) => <>{['oculta', 'visível'].map(tabId =>
      <TerminalHistory key={tabId} tabId={tabId} runningCommandId={null}
        entries={show ? [...initial.slice(1), new terminal.HistoryEntry({ id: tabId, command: tabId, output: '', exitCode: 1,
          endedAt: '2026-01-01T00:00:01Z', source: 'llm' })] : initial} />)}</>;
    try {
      const { rerender } = render(panes(false));
      rerender(panes(true));
      expect(sink.mock.calls).toEqual([['terminal.history.commandPrefix visível', 'polite']]);
      expect(screen.getAllByText('terminal.entry.exit 1')).toHaveLength(2);
    } finally { unregister(); unregisterAnnouncerSink(); }
  });

  it('renderiza estado vazio', () => {
    render(
      <TerminalHistory
        tabId="tab"
        entries={[]}
        runningCommandId={null}
      />
    );

    expect(screen.getByText(/terminal\.history\.emptyTitle/)).toBeInTheDocument();
  });

  it('renderiza lista de nos', () => {
    render(
      <TerminalHistory
        tabId="tab"
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
        runningCommandId={null}
      />
    );

    expect(screen.getAllByRole('listitem')).toHaveLength(2);
  });
});
