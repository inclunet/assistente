import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { TerminalHistory } from './TerminalHistory';
import { terminal } from '../../../wailsjs/go/models';

const announceSpy = vi.fn();

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  announce: (...args: unknown[]) => announceSpy(...args),
}));

describe('TerminalHistory', () => {
  beforeEach(() => {
    Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', {
      value: vi.fn(),
      writable: true,
    });
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
