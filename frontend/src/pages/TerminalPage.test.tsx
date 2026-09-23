import type { ReactNode } from 'react';
import { describe, expect, it, vi, beforeEach } from 'vitest';
import { act, fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { WorkspacePanelProvider } from '../components/workspace/WorkspacePanelContext';
import {
  canFocusWorkspacePanelImmediately,
  getWorkspacePanelImmediateFocusHandler,
  requestWorkspacePanelFocus,
} from '../components/workspace/workspacePanelFocusRegistry';
import { capturePagePresentationTarget } from '../lib/commandPagePresentation';
import {
  TERMINAL_OPERATION_EVENT,
  TERMINAL_SESSION_CREATE_COMMAND,
  TERMINAL_SESSION_CLOSE_COMMAND,
} from '../lib/commandTerminalOperation';

const storeMocks = vi.hoisted(() => ({
  loadSessions: vi.fn(),
  createSession: vi.fn(),
  closeSession: vi.fn(),
  sendInput: vi.fn(),
  setupEventListeners: vi.fn(() => () => {}),
}));

const terminalPageMocks = vi.hoisted(() => ({
  registeredAdapter: null as unknown,
  slashMenuEnabled: undefined as boolean | undefined,
  surfaceGetter: null as (() => unknown) | null,
  modalOpen: false,
  onSend: null as ((message: string) => Promise<boolean | void>) | null,
}));

const workspaceMocks = vi.hoisted(() => ({
  updateTab: vi.fn(),
  workspace: {
    id: 'workspace-1',
    activeTabId: 'terminal-tab',
    tabs: [{ id: 'terminal-tab', type: 'terminal', state: { sessionId: 'term-1' } }],
  },
}));

const storeState = vi.hoisted(() => ({
  sessions: [{ id: 'term-1', name: 'Terminal 1', cwd: '/tmp', state: 'running', shell: 'sh' }],
  historyBySession: { 'term-1': [] as Array<{ id: string; command: string; output: string; exitCode?: number }> },
  activeEntryBySession: { 'term-1': null as string | null },
  isLoadingSessions: false,
  loadingHistoryBySession: {},
  loadSessions: storeMocks.loadSessions,
  createSession: storeMocks.createSession,
  closeSession: storeMocks.closeSession,
  sendInput: storeMocks.sendInput,
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
    subscribe: () => () => {},
  }),
}));

vi.mock('../components/terminal/TerminalTabs', () => ({
  TerminalTabs: () => <div>Tabs</div>,
}));

vi.mock('../components/terminal/TerminalHistory', async () => {
  const React = await import('react');
  return {
    TerminalHistory: React.forwardRef<HTMLDivElement, { entries?: Array<{ id: string }> }>((props, ref) => (
      <div ref={ref}>
        {(props.entries ?? []).map((entry) => (
          <div key={entry.id} className="terminal-node" tabIndex={0}>History</div>
        ))}
      </div>
    )),
  };
});

vi.mock('../components/chat/ChatInput', async () => {
  const React = await import('react');
  return {
    ChatInput: React.forwardRef<HTMLTextAreaElement, {
      placeholder: string;
      slashMenuEnabled?: boolean;
      onSend: (message: string) => Promise<boolean | void>;
    }>(({ placeholder, slashMenuEnabled, onSend }, ref) => {
      terminalPageMocks.slashMenuEnabled = slashMenuEnabled;
      terminalPageMocks.onSend = onSend;
      return (
        <textarea
          ref={ref}
          aria-label="chat-input"
          placeholder={placeholder}
          onKeyDown={(event) => {
            if (event.key === 'Enter') onSend(event.currentTarget.value);
          }}
        />
      );
    }),
  };
});

vi.mock('../components/pickers/ProfilePicker', () => ({
  ProfilePicker: ({ value }: { value?: string }) => <span data-testid="profile-picker">{value ?? 'default'}</span>,
}));

vi.mock('../components/ui/Modal', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../components/ui/Modal')>();
  return {
    ...actual,
    isModalOpen: () => terminalPageMocks.modalOpen,
  };
});

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(() => () => {}),
  EventsOff: vi.fn(),
}));

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: Object.assign(
    (selector: (state: Record<string, unknown>) => unknown) => selector({
      workspace: workspaceMocks.workspace,
      getActiveTab: () => undefined,
      updateTab: workspaceMocks.updateTab,
    }),
    { getState: () => ({ workspace: workspaceMocks.workspace, getActiveTab: () => undefined, updateTab: workspaceMocks.updateTab }), subscribe: () => () => {} }
  ),
  useActiveTab: () => undefined,
}));

vi.mock('../components/workspace/useWorkspaceCommandSurface', () => ({
  useWorkspaceCommandSurface: vi.fn((_surfaceType: string, getter: () => unknown) => {
    terminalPageMocks.surfaceGetter = getter;
  }),
}));

vi.mock('../lib/commandShortcutHints', () => ({
  useCommandShortcutHint: vi.fn(() => undefined),
}));

vi.mock('../store/authStore', () => ({
  useAuthStore: Object.assign(
    (selector: (state: Record<string, unknown>) => unknown) => selector({
      isAuthenticated: true,
      user: { userId: 'user-1', sessionId: 'session-1' },
    }),
    {
      getState: () => ({
        isAuthenticated: true,
        user: { userId: 'user-1', sessionId: 'session-1' },
      }),
      subscribe: () => () => {},
    },
  ),
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
    <MemoryRouter initialEntries={['/terminal']}>
      <WorkspacePanelProvider value={{ tab: terminalTab, isActive: true }}>
        <TerminalPage />
      </WorkspacePanelProvider>
    </MemoryRouter>,
  );
}

describe('TerminalPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    storeMocks.loadSessions.mockReset();
    storeMocks.createSession.mockReset();
    storeMocks.closeSession.mockReset();
    storeMocks.sendInput.mockReset();
    workspaceMocks.updateTab.mockReset();
    terminalPageMocks.registeredAdapter = null;
    terminalPageMocks.slashMenuEnabled = undefined;
    terminalPageMocks.surfaceGetter = null;
    terminalPageMocks.modalOpen = false;
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    terminalPageMocks.onSend = null;
    storeState.historyBySession = { 'term-1': [] };
    storeState.activeEntryBySession = { 'term-1': null };
    storeState.sessions = [{ id: 'term-1', name: 'Terminal 1', cwd: '/tmp', state: 'running', shell: 'sh' }];
    workspaceMocks.workspace.activeTabId = 'terminal-tab';
    workspaceMocks.workspace.tabs[0].state = { sessionId: 'term-1' };
  });

  it('aciona acoes da toolbar', async () => {
    const user = userEvent.setup();
    renderTerminalPage();

    const request = vi.fn((event: Event) => event.preventDefault());
    window.addEventListener(TERMINAL_OPERATION_EVENT, request);

    const stopButton = screen.getByRole('button', { name: 'Parar' });

    await user.click(stopButton);

    expect(request).toHaveBeenCalledTimes(1);
    expect((request.mock.calls[0][0] as CustomEvent).detail).toEqual({
      instanceId: 'terminal-operation:terminal-tab',
    });
    window.removeEventListener(TERMINAL_OPERATION_EVENT, request);
  });

  it('exibe o titulo da sessao ativa', () => {
    renderTerminalPage();
    expect(screen.getByRole('heading', { name: 'Terminal 1' })).toBeInTheDocument();
  });

  it('getter da surface rejeita retarget da mesma aba sem rerender ou notify', () => {
    renderTerminalPage();
    expect(terminalPageMocks.surfaceGetter).not.toBeNull();
    expect(terminalPageMocks.surfaceGetter?.()).toMatchObject({
      surfaceId: 'terminal-tab',
      surfaceType: 'terminal',
      metadata: { sessionId: 'term-1' },
    });

    workspaceMocks.workspace.tabs[0].state = { sessionId: 'term-2' };
    storeState.sessions = [
      ...storeState.sessions,
      { id: 'term-2', name: 'Terminal 2', cwd: '/tmp', state: 'running', shell: 'sh' },
    ];

    // A closure antiga não pode seguir o retarget da aba para a nova sessão.
    expect(terminalPageMocks.surfaceGetter?.()).toBeNull();
  });

  it('registra handler de foco de painel e foca o input ao ser solicitado', async () => {
    renderTerminalPage();
    const input = screen.getByLabelText('chat-input') as HTMLTextAreaElement;
    input.blur();
    expect(input).not.toHaveFocus();

    // O WorkspaceLayout roteia o foco via registry; o handler foca o input.
    act(() => {
      requestWorkspacePanelFocus('terminal-tab');
    });

    await vi.waitFor(() => expect(input).toHaveFocus());
  });

  it('abre o picker real pelo comando de apresentação', async () => {
    renderTerminalPage();
    const presentation = capturePagePresentationTarget(
      () => '/terminal',
      'terminal.sessions.open',
    );
    expect(presentation).toBeDefined();

    act(() => {
      expect(presentation?.open('terminal.sessions.open')).toBe(true);
    });

    expect(await screen.findByRole('combobox')).toBeInTheDocument();
  });

  it('invalida o alvo quando a sessão viva é substituída ou a aba muda o binding', () => {
    renderTerminalPage();
    const presentation = capturePagePresentationTarget(
      () => '/terminal',
      'terminal.focus.input',
    );
    expect(presentation?.isCurrent()).toBe(true);

    storeState.sessions = [
      { id: 'term-1', name: 'Terminal 1 atualizado', cwd: '/tmp', state: 'running', shell: 'sh' },
    ];
    expect(presentation?.isCurrent()).toBe(false);
    presentation?.dispose();

    storeState.sessions = [
      { id: 'term-1', name: 'Terminal 1', cwd: '/tmp', state: 'running', shell: 'sh' },
      { id: 'term-2', name: 'Terminal 2', cwd: '/tmp', state: 'running', shell: 'sh' },
    ];
    workspaceMocks.workspace.tabs[0].state = { sessionId: 'term-2' };
    const rebound = capturePagePresentationTarget(
      () => '/terminal',
      'terminal.focus.input',
    );
    expect(rebound).toBeUndefined();
  });

  it('foca o input e o histórico reais pelos comandos de apresentação', () => {
    storeState.historyBySession = {
      'term-1': [{ id: 'entry-1', command: 'echo ok', output: 'ok', exitCode: 0 }],
    };
    renderTerminalPage();
    const inputPresentation = capturePagePresentationTarget(
      () => '/terminal',
      'terminal.focus.input',
    );
    const historyPresentation = capturePagePresentationTarget(
      () => '/terminal',
      'terminal.focus.history',
    );
    const input = screen.getByLabelText('chat-input');

    act(() => {
      expect(inputPresentation?.open('terminal.focus.input')).toBe(true);
    });
    expect(input).toHaveFocus();

    const historyNode = document.querySelector('.terminal-node') as HTMLElement;
    expect(historyNode).toBeTruthy();
    act(() => {
      expect(historyPresentation?.open('terminal.focus.history')).toBe(true);
    });
    expect(historyNode).toHaveFocus();
  });

  it('expõe foco imediato do terminal e move o foco sincronicamente', () => {
    renderTerminalPage();
    const input = screen.getByLabelText('chat-input') as HTMLTextAreaElement;
    input.blur();
    const immediate = getWorkspacePanelImmediateFocusHandler('terminal-tab');

    expect(immediate).toBeTypeOf('function');
    expect(canFocusWorkspacePanelImmediately('terminal-tab')).toBe(true);
    act(() => {
      expect(immediate?.()).toBe(true);
    });
    expect(input).toHaveFocus();
  });

  it('recusa foco imediato do terminal quando inativo, modal aberto ou sessão não está pronta', () => {
    const inactive = render(
      <MemoryRouter initialEntries={['/terminal']}>
        <WorkspacePanelProvider value={{ tab: terminalTab, isActive: false }}>
          <TerminalPage />
        </WorkspacePanelProvider>
      </MemoryRouter>,
    );
    expect(canFocusWorkspacePanelImmediately('terminal-tab')).toBe(false);
    expect(getWorkspacePanelImmediateFocusHandler('terminal-tab')?.()).toBe(false);
    inactive.unmount();

    terminalPageMocks.modalOpen = true;
    const modal = renderTerminalPage();
    expect(canFocusWorkspacePanelImmediately('terminal-tab')).toBe(false);
    expect(getWorkspacePanelImmediateFocusHandler('terminal-tab')?.()).toBe(false);
    modal.unmount();

    terminalPageMocks.modalOpen = false;
    storeState.sessions = [];
    renderTerminalPage();
    expect(canFocusWorkspacePanelImmediately('terminal-tab')).toBe(false);
    expect(getWorkspacePanelImmediateFocusHandler('terminal-tab')?.()).toBe(false);
  });

  it('cria um terminal explicitamente e conecta a aba', async () => {
    const user = userEvent.setup();
    renderTerminalPage();
    const request = vi.fn((event: Event) => event.preventDefault());
    window.addEventListener(TERMINAL_OPERATION_EVENT, request);

    await user.click(screen.getByRole('button', { name: 'Novo' }));

    expect(request).toHaveBeenCalledTimes(1);
    expect((request.mock.calls[0][0] as CustomEvent).detail).toEqual({
      instanceId: 'terminal-operation:terminal-tab',
      commandId: TERMINAL_SESSION_CREATE_COMMAND,
    });
    expect(storeMocks.createSession).not.toHaveBeenCalled();
    window.removeEventListener(TERMINAL_OPERATION_EVENT, request);
  });

  it('conecta terminal quando a aba ainda não tem estado', async () => {
    const user = userEvent.setup();
    const request = vi.fn((event: Event) => event.preventDefault());
    window.addEventListener(TERMINAL_OPERATION_EVENT, request);
    render(
      <MemoryRouter initialEntries={['/terminal']}>
        <WorkspacePanelProvider value={{ tab: { ...terminalTab, state: undefined }, isActive: true }}>
          <TerminalPage />
        </WorkspacePanelProvider>
      </MemoryRouter>,
    );

    await user.click(screen.getByRole('button', { name: 'Novo' }));

    expect((request.mock.calls[0][0] as CustomEvent).detail.commandId).toBe(TERMINAL_SESSION_CREATE_COMMAND);
    window.removeEventListener(TERMINAL_OPERATION_EVENT, request);
  });

  it('fecha a sessão somente pelo pipeline de decisão do comando', async () => {
    const user = userEvent.setup();
    const request = vi.fn((event: Event) => event.preventDefault());
    window.addEventListener(TERMINAL_OPERATION_EVENT, request);

    renderTerminalPage();
    await user.click(screen.getByRole('button', { name: 'Encerrar terminal' }));

    expect((request.mock.calls[0][0] as CustomEvent).detail).toEqual({
      instanceId: 'terminal-operation:terminal-tab',
      commandId: TERMINAL_SESSION_CLOSE_COMMAND,
    });
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(storeMocks.closeSession).not.toHaveBeenCalled();
    window.removeEventListener(TERMINAL_OPERATION_EVENT, request);
  });

  it('não intercepta Ctrl+C quando há texto selecionado no input', () => {
    renderTerminalPage();

    const request = vi.fn();
    window.addEventListener(TERMINAL_OPERATION_EVENT, request);

    const input = screen.getByLabelText('chat-input') as HTMLInputElement;
    input.value = 'copiar';
    input.focus();
    input.setSelectionRange(0, input.value.length);

    fireEvent.keyDown(input, { key: 'c', ctrlKey: true });

    expect(request).not.toHaveBeenCalled();
    window.removeEventListener(TERMINAL_OPERATION_EVENT, request);
  });

  it('emite Ctrl+C contextual para o registry quando não há seleção', () => {
    renderTerminalPage();
    const request = vi.fn((event: Event) => event.preventDefault());
    window.addEventListener(TERMINAL_OPERATION_EVENT, request);
    const input = screen.getByLabelText('chat-input');

    fireEvent.keyDown(input, { key: 'c', ctrlKey: true });

    expect(request).toHaveBeenCalledTimes(1);
    expect((request.mock.calls[0][0] as CustomEvent).detail).toEqual({
      instanceId: 'terminal-operation:terminal-tab',
    });
    window.removeEventListener(TERMINAL_OPERATION_EVENT, request);
  });

  it('não intercepta Ctrl+C fora da raiz do terminal, como uma paleta ou picker global', () => {
    renderTerminalPage();
    const request = vi.fn();
    window.addEventListener(TERMINAL_OPERATION_EVENT, request);
    const outsidePicker = document.createElement('input');
    document.body.append(outsidePicker);
    outsidePicker.focus();

    fireEvent.keyDown(outsidePicker, { key: 'c', ctrlKey: true });

    expect(request).not.toHaveBeenCalled();
    outsidePicker.remove();
    window.removeEventListener(TERMINAL_OPERATION_EVENT, request);
  });

  it('desabilita o menu slash e envia "/" como texto normal ao shell', () => {
    renderTerminalPage();

    const input = screen.getByLabelText('chat-input');
    fireEvent.change(input, { target: { value: '/' } });
    fireEvent.keyDown(input, { key: 'Enter' });

    expect(terminalPageMocks.slashMenuEnabled).toBe(false);
    expect(storeMocks.sendInput).toHaveBeenCalledWith('term-1', '/');
  });

  it('retorna aceitação explícita para o ChatInput limpar o comando enviado', async () => {
    renderTerminalPage();

    await expect(terminalPageMocks.onSend?.('pwd')).resolves.toBe(true);
    expect(storeMocks.sendInput).toHaveBeenCalledWith('term-1', 'pwd');
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

    expect(surfaceContext.surfaceType).toBe('terminal');
    expect(surfaceContext.surfaceId).toBe('terminal-tab');
    expect(surfaceContext.snapshotVersion).toMatch(/^terminal:terminal-tab:/);
    expect(surfaceContext.content.recentOutput).toBe(prepared.contextDisplay);
    expect(surfaceContext.content.recentOutput).toContain('cmd-6');
    expect(surfaceContext.content.recentOutput).toContain('cmd-45');
    expect(surfaceContext.content.recentOutput).not.toContain('cmd-5');
    expect(surfaceContext.content.truncated).toBe(true);
  });
});
