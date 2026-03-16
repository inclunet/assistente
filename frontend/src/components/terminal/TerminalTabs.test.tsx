import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { TerminalTabs } from './TerminalTabs';

const createSessionSpy = vi.fn();
const closeSessionSpy = vi.fn();

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: vi.fn() }),
}));

vi.mock('../../store/terminalStore', () => ({
  useTerminalStore: () => ({
    sessions: [
      { id: 'a', name: 'Sessao A', state: 'idle' },
      { id: 'b', name: 'Sessao B', state: 'busy' },
    ],
    activeSessionId: 'a',
    setActiveSession: vi.fn(),
    closeSession: closeSessionSpy,
    createSession: createSessionSpy,
  }),
}));

vi.mock('../ui/tabs', () => ({
  Tabs: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  TabList: ({ children, listRef, ariaLabel, className }: { children: React.ReactNode; listRef?: React.Ref<HTMLDivElement>; ariaLabel?: string; className?: string }) => (
    <div role="tablist" aria-label={ariaLabel} ref={listRef} className={className}>
      {children}
    </div>
  ),
  Tab: ({ children, value, className, controlsId }: { children: React.ReactNode; value: string; className?: string; controlsId?: string | null }) => (
    <button role="tab" data-tab-value={value} className={className} aria-controls={controlsId ?? undefined}>
      {children}
    </button>
  ),
}));

describe('TerminalTabs', () => {
  it('renderiza tabs e cria nova sessao', () => {
    render(<TerminalTabs />);

    expect(screen.getByRole('tab', { name: /Sessao A/i })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: /Sessao B/i })).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'terminal.tabs.newShortcut' }));
    expect(createSessionSpy).toHaveBeenCalled();
  });

  it('fecha sessao com botao', () => {
    render(<TerminalTabs />);

    fireEvent.click(screen.getByRole('button', { name: /terminal.tabs.close Sessao B/i }));
    expect(closeSessionSpy).toHaveBeenCalledWith('b');
  });
});
