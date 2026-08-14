import type { ReactNode } from 'react';
import { describe, expect, it, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { WorkspacePanelProvider } from '../components/workspace/WorkspacePanelContext';

const storeMocks = vi.hoisted(() => ({
  loadSessions: vi.fn(),
  createSession: vi.fn(),
  closeSession: vi.fn(),
  sendInput: vi.fn(),
  interrupt: vi.fn(),
  setupEventListeners: vi.fn(() => () => {}),
}));

const terminalPageMocks = vi.hoisted(() => ({
  registeredAdapter: null as unknown,
}));

const workspaceMocks = vi.hoisted(() => ({
  updateTab: vi.fn(),
}));

const storeState = vi.hoisted(() => ({
  sessions: [{ id: 'term-1', name: 'Terminal 1', cwd: '/tmp' }],
  historyBySession: { 'term-1': [] as Array<{ id: string; command: string; output: string; exitCode?: number }> },
  isLoadingSessions: false,
  loadingHistoryBySession: {},
  loadSessions: storeMocks.loadSessions,
  createSession: storeMocks.createSession,
  closeSession: storeMocks.closeSession,
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
        'terminal.buttons.terminate': 'Encerrar terminal',
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
      updateTab: workspaceMocks.updateTab,
    }),
    { getState: () => ({ workspace: { tabs: [] }, getActiveTab: () => undefined, updateTab: workspaceMocks.updateTab }), subscribe: () => () => {} }
  ),
  useActiveTab: () => undefined,
}));

vi.mock('../hooks/useRegisterWorkspaceChatAdapter', () => ({
  useRegisterWorkspaceChatAdapter: vi.fn((_tabId: string | undefined, adapter: unknown) => {
    terminalPageMocks.registeredAdapter = adapter;
  }),
}));

vi.mock('../components/ui/Toolbar', () => ({
  Toolbar: ({ left, right, actions = [] }: { left?: ReactNode; right?: ReactNode; actions?: Array<{ key: string; label: string; disabled?: boolean; onClick: () => void }> }) => (
    <div>
      {left}
      {actions.map((action) => (
        <button key={action.key} disabled={action.disabled} onClick={action.onClick}>{action.label}</button>
      ))}
      {right}
    </div>
  ),
  ToolbarButton: ({ label, onClick }: { label: string; onClick?: () => void }) => (
    <button onClick={onClick}>{label}</button>
  ),
  ToolbarSeparator: () => <span>|</span>,
}));

import TerminalPage from './TerminalPage';

const terminalTab = {
  id: 'terminal-tab',
  type: 'terminal' as const,
  title: 'Terminal',
  position: 0,
  state: { sessionId: 'term-1' },
};

function renderTerminalPage() {
  return render(
    <WorkspacePanelProvider value={{ tab: terminalTab, isActive: true }}>
      <TerminalPage />
    </WorkspacePanelProvider>,
  );
}

describe('TerminalPage', () => {
  beforeEach(() => {
    storeMocks.loadSessions.mockReset();
    storeMocks.createSession.mockReset();
    storeMocks.closeSession.mockReset();
    storeMocks.sendInput.mockReset();
    storeMocks.interrupt.mockReset();
    workspaceMocks.updateTab.mockReset();
    terminalPageMocks.registeredAdapter = null;
    storeState.historyBySession = { 'term-1': [] };
  });

  it('aciona acoes da toolbar', async () => {
    const user = userEvent.setup();
    renderTerminalPage();

    const stopButton = screen.getByRole('button', { name: 'Parar' });

    await user.click(stopButton);

    expect(storeMocks.interrupt).toHaveBeenCalledWith('term-1');
  });

  it('exibe o titulo da sessao ativa', () => {
    renderTerminalPage();
    expect(screen.getByRole('heading', { name: 'Terminal 1' })).toBeInTheDocument();
  });

  it('cria um terminal explicitamente e conecta a aba', async () => {
    storeMocks.createSession.mockResolvedValue('term-2');
    const user = userEvent.setup();
    renderTerminalPage();

    await user.click(screen.getByRole('button', { name: 'Novo' }));

    expect(storeMocks.createSession).toHaveBeenCalled();
    expect(storeMocks.loadSessions).toHaveBeenCalled();
    expect(workspaceMocks.updateTab).toHaveBeenCalledWith('terminal-tab', {
      state: { sessionId: 'term-2' },
    });
  });

  it('conecta terminal quando a aba ainda não tem estado', async () => {
    storeMocks.createSession.mockResolvedValue('term-2');
    const user = userEvent.setup();
    render(
      <WorkspacePanelProvider value={{ tab: { ...terminalTab, state: undefined }, isActive: true }}>
        <TerminalPage />
      </WorkspacePanelProvider>,
    );

    await user.click(screen.getByRole('button', { name: 'Novo' }));

    expect(workspaceMocks.updateTab).toHaveBeenCalledWith('terminal-tab', {
      state: { sessionId: 'term-2' },
    });
  });

  it('não intercepta Ctrl+C quando há texto selecionado no input', () => {
    renderTerminalPage();

    const input = screen.getByLabelText('chat-input') as HTMLInputElement;
    input.value = 'copiar';
    input.focus();
    input.setSelectionRange(0, input.value.length);

    fireEvent.keyDown(window, { key: 'c', ctrlKey: true });

    expect(storeMocks.interrupt).not.toHaveBeenCalled();
  });

  it('usa o mesmo histórico no preview e no envio do chat', async () => {
    storeState.historyBySession = {
      'term-1': Array.from({ length: 45 }, (_, index) => {
        const entryNumber = index + 1;
        return {
          id: `entry-${entryNumber}`,
          command: `cmd-${entryNumber}`,
          output: `out-${entryNumber}`,
          exitCode: 0,
        };
      }),
    };

    renderTerminalPage();

    const adapter = terminalPageMocks.registeredAdapter as {
      prepare: () => Promise<{ ok: true; contextDisplay: string }>;
      send: (
        instruction: string,
        media: undefined,
        meta: unknown,
        session: { tabId: string; conversationId: string },
      ) => Promise<{ paramsOverride?: { surfaceContextJson?: string } } | null>;
    };
    const prepared = await adapter.prepare();
    const plan = await adapter.send('Resuma o terminal', undefined, null, {
      tabId: 'terminal-tab',
      conversationId: 'conv-1',
    });
    const surfaceContext = JSON.parse(String(plan?.paramsOverride?.surfaceContextJson || '{}'));

    expect(surfaceContext.content.recentOutput).toBe(prepared.contextDisplay);
    expect(surfaceContext.content.recentOutput).toContain('cmd-6');
    expect(surfaceContext.content.recentOutput).toContain('cmd-45');
    expect(surfaceContext.content.recentOutput).not.toContain('cmd-5');
    expect(surfaceContext.content.truncated).toBe(true);
  });
});
