import type { ReactNode } from 'react';
import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const storeMocks = vi.hoisted(() => ({
  loadSessions: vi.fn(),
  createSession: vi.fn(),
  closeSession: vi.fn(),
  setActiveSession: vi.fn(),
  sendInput: vi.fn(),
  interrupt: vi.fn(),
  setupEventListeners: vi.fn(() => () => {}),
}));

const storeState = vi.hoisted(() => ({
  sessions: [{ id: 'term-1', name: 'Terminal 1', cwd: '/tmp' }],
  activeSessionId: 'term-1',
  historyBySession: { 'term-1': [] },
  isLoading: false,
  loadSessions: storeMocks.loadSessions,
  createSession: storeMocks.createSession,
  closeSession: storeMocks.closeSession,
  setActiveSession: storeMocks.setActiveSession,
  sendInput: storeMocks.sendInput,
  interrupt: storeMocks.interrupt,
  setupEventListeners: storeMocks.setupEventListeners,
}));

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string) =>
      ({
        'terminal.pageTitle': 'Terminal',
        'terminal.buttons.stop': 'Parar',
        'terminal.buttons.new': 'Novo',
        'terminal.placeholders.creating': 'Criando terminal...',
        'terminal.placeholders.command': 'Digite um comando',
        'terminal.aria.toolbar': 'Barra de ferramentas do terminal',
      } as Record<string, string>)[key] ?? key,
  }),
}));

vi.mock('../store/terminalStore', () => ({
  useTerminalStore: Object.assign(() => storeState, {
    getState: () => storeState,
  }),
}));

vi.mock('../components/terminal/TerminalTabs', () => ({
  TerminalTabs: () => <div>Tabs</div>,
}));

vi.mock('../components/terminal/TerminalHistory', async () => {
  const React = await import('react');
  return {
    TerminalHistory: React.forwardRef<HTMLDivElement, Record<string, unknown>>(() => <div>History</div>),
  };
});

vi.mock('../components/chat/ChatInput', async () => {
  const React = await import('react');
  return {
    ChatInput: React.forwardRef<HTMLTextAreaElement, { placeholder: string }>(
      ({ placeholder }, ref) => <input ref={ref as React.RefObject<HTMLInputElement>} aria-label="chat-input" placeholder={placeholder} />
    ),
  };
});

vi.mock('../components/pickers/ProfilePicker', () => ({
  ProfilePicker: ({ value }: { value?: string }) => <span data-testid="profile-picker">{value ?? 'default'}</span>,
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(() => () => {}),
  EventsOff: vi.fn(),
}));

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: Object.assign(
    (selector: (state: Record<string, unknown>) => unknown) => selector({
      workspace: { tabs: [], profile: undefined },
      getActiveTab: () => undefined,
      updateTab: vi.fn(),
    }),
    { getState: () => ({ workspace: { tabs: [] }, getActiveTab: () => undefined }), subscribe: () => () => {} }
  ),
}));

vi.mock('../components/ui/Toolbar', () => ({
  Toolbar: ({ left, right }: { left?: ReactNode; right?: ReactNode }) => (
    <div>
      {left}
      {right}
    </div>
  ),
  ToolbarButton: ({ label, onClick }: { label: string; onClick?: () => void }) => (
    <button onClick={onClick}>{label}</button>
  ),
  ToolbarSeparator: () => <span>|</span>,
}));

import TerminalPage from './TerminalPage';

describe('TerminalPage', () => {
  beforeEach(() => {
    storeMocks.loadSessions.mockReset();
    storeMocks.createSession.mockReset();
    storeMocks.closeSession.mockReset();
    storeMocks.setActiveSession.mockReset();
    storeMocks.sendInput.mockReset();
    storeMocks.interrupt.mockReset();
  });

  it('aciona acoes da toolbar', async () => {
    const user = userEvent.setup();
    render(<TerminalPage />);

    const stopButton = screen.getByRole('button', { name: 'Parar' });

    await user.click(stopButton);

    expect(storeMocks.interrupt).toHaveBeenCalled();
  });

  it('exibe o titulo da sessao ativa', () => {
    render(<TerminalPage />);
    expect(screen.getByText('Terminal 1')).toBeInTheDocument();
  });
});
