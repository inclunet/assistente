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

  it('abrir o histórico carregado mantém a falha sem saída acessível e silenciosa', () => {
    const sink = vi.fn();
    registerAnnouncerSink(sink);
    const unregister = registerVoiceAccessibilityActiveResolver(origin => origin?.tabId === 'visível');
    const initial = Array.from({ length: 200 }, (_, i) => new terminal.HistoryEntry({ id: String(i), command: 'anterior', output: '', exitCode: 0 }));
    const panes = (show: boolean) => <>{['oculta', 'visível'].map(tabId =>
      <TerminalHistory key={tabId} runningCommandId={null}
        entries={show ? [...initial.slice(1), new terminal.HistoryEntry({ id: tabId, command: tabId, output: '', exitCode: 1,
          endedAt: '2026-01-01T00:00:01Z', source: 'llm' })] : []} />)}</>;
    try {
      const { rerender } = render(panes(false));
      rerender(panes(true));
      expect(sink).not.toHaveBeenCalled();
      expect(screen.getAllByText('terminal.entry.exit 1')).toHaveLength(2);
    } finally { unregister(); unregisterAnnouncerSink(); }
  });

  it('renderiza estado vazio', () => {
    render(
      <TerminalHistory
        entries={[]}
        runningCommandId={null}
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
        runningCommandId={null}
      />
    );

    expect(screen.getAllByRole('listitem')).toHaveLength(2);
  });
});
