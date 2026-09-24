import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, useNavigate } from 'react-router-dom';
import type { ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ChatToolbar } from './ChatToolbar';
import { TokenStatsModal } from './TokenStatsModal';
import { registerOpenModal, unregisterOpenModal } from '../../lib/modalRegistry';
import { CHAT_PRESENTATION_COMMAND_EVENT, captureChatPickerTarget } from '../../lib/commandChatPickers';
import { CHAT_CLEAR_COMMAND, CHAT_CLEAR_EVENT, captureChatClearTarget, executeChatClear } from '../../lib/commandChatClear';
import { createLocalCommandKeyboard, type LocalCommandKeyboardController } from '../../lib/commandLocalKeyboard';
import type { CommandContextualBackendPort } from '../../lib/commandContextualBackendExecution';

const reservation = { ticket: 'clear-ticket', invocationId: 'clear-invocation', commandId: CHAT_CLEAR_COMMAND };
const clearPort = {
  beginUICommand: vi.fn<CommandContextualBackendPort['beginUICommand']>(),
  takeUICommand: vi.fn<CommandContextualBackendPort['takeUICommand']>(),
  commitBackendCommand: vi.fn<CommandContextualBackendPort['commitBackendCommand']>(),
  getUICommandResult: vi.fn<CommandContextualBackendPort['getUICommandResult']>(),
  completeUICommand: vi.fn<CommandContextualBackendPort['completeUICommand']>(),
  cancelUICommand: vi.fn<CommandContextualBackendPort['cancelUICommand']>(),
};
let keyboard: LocalCommandKeyboardController;
let restoreFocus: () => void;
let clearRequestListener: (event: Event) => void;
let presentationRequestListener: (event: Event) => void;
const pendingClears: Promise<unknown>[] = [];

beforeEach(async () => {
  const focus = vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  restoreFocus = () => focus.mockRestore();
  clearPort.beginUICommand.mockReset().mockResolvedValue(reservation);
  clearPort.takeUICommand.mockReset().mockResolvedValue({ ...reservation, handoffId: 'clear-handoff' });
  clearPort.commitBackendCommand.mockReset().mockResolvedValue(undefined);
  clearPort.getUICommandResult.mockReset().mockResolvedValue({ invocationId: reservation.invocationId, status: 'succeeded' });
  clearPort.completeUICommand.mockReset().mockResolvedValue(undefined);
  clearPort.cancelUICommand.mockReset().mockResolvedValue(undefined);
  const available = (keyboardTarget?: EventTarget | null) => {
    const target = captureChatClearTarget(() => '/', undefined, keyboardTarget);
    try { return target?.isCurrent() === true; } finally { target?.dispose(); }
  };
  keyboard = createLocalCommandKeyboard({
    target: window,
    loadMap: async () => ({ generation: 'chat-test', bindings: [{
      commandId: CHAT_CLEAR_COMMAND, handler: 'contextual',
      shortcut: { version: 1, code: 'KeyL', modifiers: ['Control'] },
    }] }),
    blocked: () => modalState.open && !available(),
    canHandle: (_, event) => available(event.target),
    canHandleEditable: (_, event) => event.target instanceof Element &&
      !!event.target.closest('[data-testid="chat-input"]') && available(),
    onDown: async () => {
      const target = captureChatClearTarget(() => '/');
      if (target) {
        const execution = executeChatClear(clearPort, target);
        pendingClears.push(execution);
        await execution;
      }
    },
    onUp: async () => {}, reset: async () => {},
  });
  clearRequestListener = event => {
    const instanceId = (event as CustomEvent<{ instanceId?: string }>).detail?.instanceId;
    if (!instanceId) return;
    const target = captureChatClearTarget(() => '/', instanceId);
    if (target) { event.preventDefault(); pendingClears.push(executeChatClear(clearPort, target)); }
  };
  window.addEventListener(CHAT_CLEAR_EVENT, clearRequestListener);
  presentationRequestListener = event => {
    const { commandID, instanceId } = (event as CustomEvent<{ commandID: string; instanceId: string }>).detail;
    const target = captureChatPickerTarget(() => '/', instanceId);
    try { if (target?.canOpen(commandID) && target.open(commandID)) event.preventDefault(); }
    finally { target?.dispose(); }
  };
  window.addEventListener(CHAT_PRESENTATION_COMMAND_EVENT, presentationRequestListener);
  await keyboard.refresh();
});
afterEach(async () => {
  keyboard.dispose();
  window.removeEventListener(CHAT_CLEAR_EVENT, clearRequestListener);
  window.removeEventListener(CHAT_PRESENTATION_COMMAND_EVENT, presentationRequestListener);
  await Promise.all(pendingClears.splice(0));
  expect(clearConversationMock).not.toHaveBeenCalled();
  restoreFocus();
});

const clearConversationMock = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));
const loadConversationSessionMock = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));
const clearConversationMessagesMock = vi.hoisted(() => vi.fn());
const updateTabMock = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));
const getProfileMock = vi.hoisted(() => vi.fn().mockResolvedValue({
  chat: { llm_provider: 'native-provider', model: 'modelo-perfil' },
}));
const getProvidersMock = vi.hoisted(() => vi.fn().mockResolvedValue([
  { id: 'native-provider', api_format: 'openai', is_default: true },
]));
const modelChangeRef = vi.hoisted(() => ({ current: null as null | ((model: string) => void) }));
const modelOpenMock = vi.hoisted(() => vi.fn());
const announceMock = vi.hoisted(() => vi.fn());
const profileChangeRef = vi.hoisted(() => ({ current: null as null | ((slug: string) => void) }));
const historyClickMock = vi.hoisted(() => vi.fn());
const getAgentSessionOptionsMock = vi.hoisted(() => vi.fn().mockResolvedValue({
  conversationId: 'conversation-1',
  available: false,
  options: [],
}));
const requestResourceEditMock = vi.hoisted(() => vi.fn());
const mockPanelTabRef = vi.hoisted(() => ({ current: { id: 'tab-chat', title: 'Chat', type: 'chat' } as unknown as Record<string, unknown> }));
const activeConversationRef = vi.hoisted(() => ({
  current: { id: 'conversation-1', title: 'Conversa' } as { id: string; title: string } | null,
}));
const isLoadingRef = vi.hoisted(() => ({ current: false }));
const openAtPointMock = vi.hoisted(() => vi.fn());
// A conversa deste teste não fala com agente de código: o diretório do agente
// não existe para ela, e o controle da barra some.
const getAgentWorkDirMock = vi.hoisted(() => vi.fn().mockRejectedValue(new Error('sem agente')));
const profileClickMock = vi.hoisted(() => vi.fn());
const modalState = vi.hoisted(() => ({
  id: null as string | null,
  open: false,
  inside: false,
  topmost: true,
}));
const tokenStatsModalLifecycle = vi.hoisted(() => ({
  transitions: [] as boolean[],
  requestClose: null as (() => void) | null,
}));
const tMock = vi.hoisted(() => (key: string, fallback?: unknown) => typeof fallback === 'string' ? fallback : key);
const sessionConversationRef = vi.hoisted(() => ({ current: 'conversation-1' as string | null }));
const contextRef = vi.hoisted(() => ({ owner: 'owner-1', session: 'session-1', workspace: 'workspace-1', tab: 'tab-chat', conversation: 'conversation-1' as string | null }));
const contextSubscribers = vi.hoisted(() => new Set<() => void>());
const subscribeContext = (changed: () => void) => { contextSubscribers.add(changed); return () => { contextSubscribers.delete(changed); }; };
const tokenStats = vi.hoisted(() => ({ conversationId: 'conversation-1', promptTokens: 10, completionTokens: 2, totalTokens: 12, contextTokens: 10, contextLimit: 100, contextUsage: 10, messageCount: 1, mostUsedModel: 'fixture-model', modelCallCount: 1, isNearLimit: false, isCritical: false, systemPromptEstimatedTokens: 0, summaryTokens: 0, messagesInContextTokens: 10, messagesOutOfContextTokens: 0, messagesInContextCount: 1, messagesOutOfContextCount: 0, toolsUsedCount: 0, toolBreakdown: [] }));
const getTokenStatsMock = vi.hoisted(() => vi.fn());
const shortcutHintsRef = vi.hoisted(() => ({
  current: {} as Record<string, string | undefined>,
}));
vi.mock('@wailsjs/go/wailsapi/Tokens', () => ({ GetConversationTokenStats: getTokenStatsMock }));

vi.mock('../../lib/commandShortcutHints', () => ({
  useCommandShortcutHints: () => (commandId: string) => shortcutHintsRef.current[commandId],
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: tMock, i18n: { language: 'en' },
  }),
}));

vi.mock('@wailsjs/go/wailsapi/Conversations', () => ({
  ClearConversation: clearConversationMock,
  GetPinnedMessages: vi.fn().mockResolvedValue([]),
  ToggleMessagePin: vi.fn(),
}));

vi.mock('@wailsjs/go/wailsapi/ACPWorkDir', () => ({
  GetAgentConversationWorkDir: getAgentWorkDirMock,
  SetAgentConversationWorkDir: vi.fn(),
}));

vi.mock('@wailsjs/go/wailsapi/ACPOptions', () => ({
  GetAgentSessionOptions: getAgentSessionOptionsMock,
  SetAgentSessionOption: vi.fn(),
}));

vi.mock('@wailsjs/go/wailsapi/Profiles', () => ({
  GetActiveProfileSlug: vi.fn().mockResolvedValue('padrao'),
  GetProfile: getProfileMock,
}));

vi.mock('@wailsjs/go/wailsapi/LLMProviders', () => ({
  GetLLMProvidersWithStatus: getProvidersMock,
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(() => vi.fn()),
}));

vi.mock('../ui/Modal', async () => {
  const React = await import('react');
  return {
    Modal: ({ children, isOpen, onClose }: { children: ReactNode; isOpen: boolean; onClose: () => void }) => {
      const previousOpen = React.useRef<boolean | null>(null);
      const onCloseRef = React.useRef(onClose);
      onCloseRef.current = onClose;
      if (isOpen) tokenStatsModalLifecycle.requestClose = () => onCloseRef.current();
      else tokenStatsModalLifecycle.requestClose = null;
      React.useEffect(() => {
        if (previousOpen.current !== null && previousOpen.current !== isOpen) {
          tokenStatsModalLifecycle.transitions.push(isOpen);
        }
        previousOpen.current = isOpen;
      }, [isOpen]);
      return isOpen ? <div>{children}</div> : null;
    },
    isModalOpen: () => modalState.open,
    useIsInsideModal: () => React.useState(modalState.inside)[0],
    useModalIsTopmost: () => {
      const topmost = modalState.topmost;
      return () => topmost;
    },
    useModalId: () => React.useState(modalState.id)[0],
  };
});

vi.mock('../pickers', async () => {
  const React = await import('react');
  return {
    HistoryPicker: React.forwardRef<HTMLButtonElement, { shortcut?: string }>(({ shortcut }) => (
      <button className="picker-button" type="button" title={shortcut} onClick={historyClickMock}>
        Historico
      </button>
    )),
  };
});

vi.mock('../pickers/ProfilePicker', async () => {
  const React = await import('react');
  return {
    ProfilePicker: React.forwardRef<HTMLButtonElement, { onChange: (slug: string) => void; shortcut?: string }>(({ onChange, shortcut }) => {
      profileChangeRef.current = onChange;
      return (
        <button className="picker-button" type="button" title={shortcut} onClick={profileClickMock}>
          Perfil
        </button>
      );
    }),
  };
});

vi.mock('../pickers/ModelPicker', () => ({
  ModelPicker: ({ value, label, onChange, shortcut }: {
    value: string;
    label: string;
    onChange: (model: string) => void;
    shortcut?: string;
  }) => {
    modelChangeRef.current = onChange;
    return (
      <button
        className="picker-button"
        type="button"
        aria-label={`${label}, ${value}`}
        title={shortcut}
        onClick={modelOpenMock}
      >
        {label}
      </button>
    );
  },
}));

vi.mock('./ChatSessionContext', () => ({
  useChatSession: () => ({
    conversationId: sessionConversationRef.current,
    session: { queuedTurnCount: 0 },
    conversation: activeConversationRef.current,
    isLoading: isLoadingRef.current,
    clearConversationMessages: clearConversationMessagesMock,
    loadConversationSession: loadConversationSessionMock,
  }),
}));

vi.mock('../workspace/WorkspacePanelContext', () => ({
  useWorkspacePanel: () => ({
    tab: mockPanelTabRef.current,
  }),
  useOptionalWorkspacePanel: () => ({
    tab: mockPanelTabRef.current,
  }),
}));

vi.mock('../../store/workspaceStore', () => ({
  useWorkspaceStore: Object.assign((selector?: (state: unknown) => unknown) => {
    const state = {
      workspace: { id: contextRef.workspace, profile: 'padrao', activeTabId: contextRef.tab, tabs: [{ id: 'tab-chat', title: 'Chat', type: 'chat', conversationId: contextRef.conversation }] },
      updateTab: updateTabMock,
    };
    return typeof selector === 'function' ? selector(state) : state;
  }, {
    getState: () => ({
      workspace: { id: contextRef.workspace, profile: 'padrao', activeTabId: contextRef.tab, tabs: [{ id: 'tab-chat', title: 'Chat', type: 'chat', conversationId: contextRef.conversation }] },
      updateTab: updateTabMock,
    }),
    subscribe: (changed: () => void) => subscribeContext(changed),
  }),
}));

vi.mock('../../store/authStore', () => ({
  useAuthStore: Object.assign(() => ({
    isAuthenticated: true,
    user: { userId: contextRef.owner, sessionId: contextRef.session },
  }), {
    getState: () => ({ isAuthenticated: true, user: { userId: contextRef.owner, sessionId: contextRef.session } }),
    subscribe: (changed: () => void) => subscribeContext(changed),
  }),
}));

vi.mock('../../store/workspaceChatModalStore', () => ({
  useWorkspaceChatModalStore: Object.assign(() => ({ boundConversationId: modalState.id ? 'conversation-1' : null }), {
    getState: () => ({ isOpen: Boolean(modalState.id), boundConversationId: modalState.id ? 'conversation-1' : null }),
    subscribe: (changed: () => void) => subscribeContext(changed),
  }),
}));

vi.mock('../../store/uiStore', () => ({
  useUIStore: (selector?: (state: unknown) => unknown) => {
    const state = { addToast: vi.fn() };
    return typeof selector === 'function' ? selector(state) : state;
  },
}));

vi.mock('../../store/navigationStore', () => ({
  useNavigationStore: {
    getState: () => ({ requestResourceEdit: requestResourceEditMock }),
  },
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: announceMock,
    announceRequest: vi.fn(),
  }),
}));

vi.mock('../../hooks/useDefaultFocus', () => ({
  restoreDefaultFocus: vi.fn(),
}));

vi.mock('../../hooks/useAnchoredContextMenu', () => ({
  useAnchoredContextMenu: () => ({
    menu: { visible: false, x: 0, y: 0, items: [], ariaLabel: '' },
    openAtPoint: openAtPointMock,
    closeMenu: vi.fn(),
    onSelectItem: vi.fn(),
  }),
}));

vi.mock('../menu', () => ({
  Menu: () => null,
}));


function renderToolbar() {
  return render(
    <MemoryRouter>
      <ChatToolbar />
    </MemoryRouter>,
  );
}

function dispatchCtrlKey(key: string, target: EventTarget = window) {
  const event = new KeyboardEvent('keydown', {
    key,
    code: `Key${key.toUpperCase()}`,
    ctrlKey: true,
    bubbles: true,
    cancelable: true,
  });
  target.dispatchEvent(event);
  target.dispatchEvent(new KeyboardEvent('keyup', { key, code: `Key${key.toUpperCase()}`, ctrlKey: true, bubbles: true }));
  const commandID = key.toLowerCase() === 'm'
    ? 'chat.model.open'
    : key.toLowerCase() === 'h' ? 'chat.history.open' : key.toLowerCase() === 'p' ? 'chat.profile.open' : null;
  if (commandID && !event.defaultPrevented && !event.repeat && !event.isComposing && event.keyCode !== 229 &&
      !event.shiftKey && !event.altKey && !event.metaKey) {
    const lease = captureChatPickerTarget(() => '/');
    if (lease?.canOpen(commandID, target)) {
      event.preventDefault();
      lease.open(commandID);
    }
    lease?.dispose();
  }
  return event;
}

function dispatchModelShortcut(
  target: EventTarget = window,
  init: Partial<KeyboardEventInit> = {},
) {
  const event = new KeyboardEvent('keydown', {
    key: 'm',
    ctrlKey: true,
    bubbles: true,
    cancelable: true,
    ...init,
  });
  target.dispatchEvent(event);
  if (!event.defaultPrevented && !event.repeat && !event.isComposing && event.keyCode !== 229 &&
      !event.shiftKey && !event.altKey && !event.metaKey) {
    const lease = captureChatPickerTarget(() => '/');
    if (lease?.canOpen('chat.model.open', target)) {
      event.preventDefault();
      lease.open('chat.model.open');
    }
    lease?.dispose();
  }
  return event;
}

beforeEach(() => {
  modalState.id = null;
  getTokenStatsMock.mockReset().mockImplementation(async (conversationId: string) => ({ ...tokenStats, conversationId }));
  sessionConversationRef.current = 'conversation-1';
  Object.assign(contextRef, { owner: 'owner-1', session: 'session-1', workspace: 'workspace-1', tab: 'tab-chat', conversation: 'conversation-1' });
  updateTabMock.mockReset().mockResolvedValue(undefined);
  getProfileMock.mockReset().mockResolvedValue({
    chat: { llm_provider: 'native-provider', model: 'modelo-perfil' },
  });
  getProvidersMock.mockReset().mockResolvedValue([
    { id: 'native-provider', api_format: 'openai', is_default: true },
  ]);
  modelChangeRef.current = null;
  modelOpenMock.mockClear();
  announceMock.mockClear();
  profileChangeRef.current = null;
  activeConversationRef.current = { id: 'conversation-1', title: 'Conversa' };
  isLoadingRef.current = false;
  mockPanelTabRef.current = { id: 'tab-chat', title: 'Chat', type: 'chat' } as unknown as Record<string, unknown>;
});

describe('ChatToolbar mensagens fixadas', () => {
  it('abre a lista enquanto os detalhes da conversa ainda carregam', async () => {
    activeConversationRef.current = null;
    renderToolbar();

    fireEvent.click(screen.getByRole('button', { name: 'chat.pins.button' }));

    expect(await screen.findByText('chat.pins.description')).toBeInTheDocument();
  });
});

describe('ChatToolbar apresentação pinned/tokens', () => {
  const commands = [
    { id: 'chat.pinned.open', button: 'chat.pins.button', content: 'chat.pins.description' },
    { id: 'chat.tokens.open', button: 'chat.tokenStatsButtonLabel', content: 'tokenStats.contextUsage' },
  ] as const;
  it.each(commands)('$id fecha na troca de rota sem desmontar Toolbar e não reabre no retorno', async ({ id, content }) => {
    function RouteHarness() {
      const navigate = useNavigate();
      return <>
        <button onClick={() => navigate('/settings')}>Outra rota</button>
        <button onClick={() => navigate('/')}>Voltar à rota</button>
        <ChatToolbar />
      </>;
    }
    render(<MemoryRouter><RouteHarness /></MemoryRouter>);
    const toolbar = screen.getByRole('toolbar');
    const target = captureChatPickerTarget(() => '/')!;
    act(() => { expect(target.open(id)).toBe(true); });
    target.dispose();
    expect(await screen.findByText(content)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Outra rota' }));
    expect(screen.getByRole('toolbar')).toBe(toolbar);
    expect(screen.queryByText(content)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Voltar à rota' }));
    expect(screen.getByRole('toolbar')).toBe(toolbar);
    expect(screen.queryByText(content)).not.toBeInTheDocument();
    expect(clearPort.beginUICommand).not.toHaveBeenCalled();
  });
  beforeEach(() => {
    modalState.open = false; modalState.inside = false; modalState.topmost = true;
    tokenStatsModalLifecycle.transitions = [];
    tokenStatsModalLifecycle.requestClose = null;
  });
  it.each(commands)('$id abre pelo registro sem ledger nem efeito de domínio', async ({ id, content }) => {
    renderToolbar();
    const target = captureChatPickerTarget(() => '/')!;
    expect(target.canOpen(id)).toBe(true);
    act(() => { expect(target.open(id)).toBe(true); });
    expect(target.open(id)).toBe(false);
    target.dispose();
    expect(await screen.findByText(content)).toBeInTheDocument();
    expect(clearPort.beginUICommand).not.toHaveBeenCalled();
    expect(clearPort.commitBackendCommand).not.toHaveBeenCalled();
    expect(updateTabMock).not.toHaveBeenCalled();
    expect(loadConversationSessionMock).not.toHaveBeenCalled();
  });
  it('mantém TokenStatsModal montado no fechamento para Modal restaurar foco padrão', async () => {
    renderToolbar();
    const target = captureChatPickerTarget(() => '/')!;
    act(() => { expect(target.open('chat.tokens.open')).toBe(true); });
    target.dispose();
    expect(await screen.findByText('tokenStats.contextUsage')).toBeInTheDocument();

    act(() => tokenStatsModalLifecycle.requestClose?.());

    await waitFor(() => expect(tokenStatsModalLifecycle.transitions).toContain(false));
    expect(screen.queryByText('tokenStats.contextUsage')).not.toBeInTheDocument();
  });
  it.each(commands)('$id botão usa evento com instância exata sem recursão', async ({ id, button, content }) => {
    renderToolbar();
    const target = captureChatPickerTarget(() => '/')!;
    const listener = vi.fn();
    window.addEventListener(CHAT_PRESENTATION_COMMAND_EVENT, listener);
    try {
      const trigger = await screen.findByRole('button', { name: button });
      fireEvent.click(trigger);
      expect(listener).toHaveBeenCalledOnce();
      expect((listener.mock.calls[0][0] as CustomEvent).detail).toEqual({ commandID: id, instanceId: target.instanceId });
      expect(await screen.findByText(content)).toBeInTheDocument();
      expect(clearPort.beginUICommand).not.toHaveBeenCalled();
    } finally { target.dispose(); window.removeEventListener(CHAT_PRESENTATION_COMMAND_EVENT, listener); }
  });
  it.each(commands)('$id indisponível sem conversa', ({ id }) => {
    sessionConversationRef.current = null;
    contextRef.conversation = null;
    activeConversationRef.current = null;
    renderToolbar();
    const target = captureChatPickerTarget(() => '/');
    expect(target?.canOpen(id)).toBe(false);
    expect(target?.open(id)).toBe(false);
    target?.dispose();
    expect(screen.getByRole('button', { name: 'chat.pins.button' })).toBeDisabled();
    expect(screen.queryByRole('button', { name: 'chat.tokenStatsButtonLabel' })).not.toBeInTheDocument();
  });
  it.each(commands)('$id rejeita modal bloqueante e alvo editável', ({ id }) => {
    renderToolbar();
    const target = captureChatPickerTarget(() => '/')!;
    const input = document.createElement('textarea');
    document.body.append(input);
    try {
      expect(target.canOpen(id, input)).toBe(false);
      modalState.open = true;
      expect(target.canOpen(id)).toBe(false);
      expect(target.open(id)).toBe(false);
    } finally { input.remove(); target.dispose(); }
  });
  for (const command of commands) {
    it.each(['owner', 'session', 'workspace', 'tab', 'conversation'] as const)(`${command.id} fecha e não retargeta após ABA de %s`, async field => {
      const view = renderToolbar();
      const target = captureChatPickerTarget(() => '/')!;
      act(() => { expect(target.open(command.id)).toBe(true); });
      target.dispose();
      expect(await screen.findByText(command.content)).toBeInTheDocument();
      const original = contextRef[field];
      act(() => {
        contextRef[field] = 'replacement';
        [...contextSubscribers].forEach(changed => changed());
        Object.assign(contextRef, { [field]: original });
        [...contextSubscribers].forEach(changed => changed());
      });
      view.rerender(<MemoryRouter><ChatToolbar /></MemoryRouter>);
      expect(screen.queryByText(command.content)).not.toBeInTheDocument();
      expect(clearPort.beginUICommand).not.toHaveBeenCalled();
    });
  }
  it('tokens usa ID da sessão mesmo quando activeConversation contém outro ID', async () => {
    activeConversationRef.current = { id: 'unrelated', title: 'Outra' };
    renderToolbar();
    fireEvent.click(await screen.findByRole('button', { name: 'chat.tokenStatsButtonLabel' }));
    expect(await screen.findByText('tokenStats.contextUsage')).toBeInTheDocument();
    expect(getTokenStatsMock.mock.calls.every(([id]) => id === 'conversation-1')).toBe(true);
  });
  for (const command of commands) {
    it.each(['tab', 'conversation'] as const)(`${command.id} fecha no chat modal quando muda %s mesmo com modal store atrasado`, async field => {
      modalState.id = 'chat-owning-modal';
      modalState.open = true; modalState.inside = true;
      const overlay = document.createElement('div');
      overlay.className = 'modal-overlay';
      overlay.setAttribute('data-modal-id', modalState.id);
      document.body.append(overlay);
      registerOpenModal(modalState.id);
      const view = render(<MemoryRouter><ChatToolbar /></MemoryRouter>, { container: overlay });
      try {
        const target = captureChatPickerTarget(() => '/')!;
        expect(target).toBeDefined();
        act(() => { expect(target.open(command.id)).toBe(true); });
        target.dispose();
        expect(await screen.findByText(command.content)).toBeInTheDocument();
        act(() => {
          contextRef[field] = 'different-context';
          [...contextSubscribers].forEach(changed => changed());
        });
        expect(screen.queryByText(command.content)).not.toBeInTheDocument();
        expect(clearPort.beginUICommand).not.toHaveBeenCalled();
      } finally {
        view.unmount(); unregisterOpenModal('chat-owning-modal'); overlay.remove();
        modalState.id = null;
      }
    });
  }
  it('TokenStatsModal descarta resposta atrasada da conversa anterior', async () => {
    let resolveOld!: (value: typeof tokenStats) => void;
    getTokenStatsMock.mockImplementationOnce(() => new Promise<typeof tokenStats>(resolve => { resolveOld = resolve; }));
    const view = render(<TokenStatsModal conversationId="old" isOpen onClose={() => {}} />);
    getTokenStatsMock.mockResolvedValue({ ...tokenStats, conversationId: 'new', contextUsage: 77 });
    view.rerender(<TokenStatsModal conversationId="new" isOpen onClose={() => {}} />);
    await waitFor(() => expect(screen.getByRole('progressbar', { name: 'tokenStats.contextUsage' })).toHaveAttribute('aria-valuenow', '77'));
    await act(async () => { resolveOld({ ...tokenStats, conversationId: 'old', contextUsage: 12 }); });
    expect(screen.getByRole('progressbar', { name: 'tokenStats.contextUsage' })).toHaveAttribute('aria-valuenow', '77');
  });
});

describe('ChatToolbar shortcuts', () => {
  it('preserva a lease durante rerender do diálogo de decisão e só commita após fechar', async () => {
    // Este harness usa modalId=null: cobre a toolbar da página sob a decisão.
    // A origem em modal próprio é coberta pelo modalRegistry no teste do módulo.
    const view = renderToolbar();
    const target = captureChatClearTarget(() => '/');
    expect(target).toBeDefined();
    if (!target) throw new Error('Toolbar não registrou seu alvo');
    let releaseDecision!: () => void;
    const decision = new Promise<void>(resolve => { releaseDecision = resolve; });
    clearPort.takeUICommand.mockImplementationOnce(async () => {
      await decision;
      return { ...reservation, handoffId: 'clear-handoff' };
    });
    const execution = executeChatClear(clearPort, target);
    pendingClears.push(execution);
    try {
      await waitFor(() => expect(clearPort.takeUICommand).toHaveBeenCalledOnce());
      modalState.open = true;
      modalState.topmost = false;
      view.rerender(<MemoryRouter><ChatToolbar /></MemoryRouter>);
      expect(target.isCurrent()).toBe(true);
      expect(target.canCommit()).toBe(false);
      expect(clearPort.commitBackendCommand).not.toHaveBeenCalled();

      modalState.open = false;
      modalState.topmost = true;
      view.rerender(<MemoryRouter><ChatToolbar /></MemoryRouter>);
      expect(target.isCurrent()).toBe(true);
      expect(target.canCommit()).toBe(true);
      releaseDecision();
      expect(await execution).toBe('succeeded');
      expect(clearPort.commitBackendCommand).toHaveBeenCalledExactlyOnceWith('clear-ticket', 'clear-handoff');
      expect(announceMock).toHaveBeenCalledWith('chat.conversationCleared');
      expect(clearConversationMock).not.toHaveBeenCalled();
    } finally {
      target.dispose();
      releaseDecision();
      await execution;
    }
  });

  it('botão publica a instância real e executa somente Commit pelo port', async () => {
    renderToolbar();
    fireEvent.click(screen.getByRole('button', { name: 'chat.clearBtn' }));
    await waitFor(() => expect(clearPort.commitBackendCommand).toHaveBeenCalledExactlyOnceWith('clear-ticket', 'clear-handoff'));
    expect(clearPort.beginUICommand).toHaveBeenCalledExactlyOnceWith(CHAT_CLEAR_COMMAND);
    expect(clearConversationMock).not.toHaveBeenCalled();
  });

  it('projeta os rótulos a partir dos bindings efetivos e omite comandos suprimidos', async () => {
    shortcutHintsRef.current = {
      [CHAT_CLEAR_COMMAND]: 'Alt+X',
      'chat.history.open': 'Alt+H',
      'chat.model.open': 'Alt+M',
      'chat.profile.open': undefined,
    };

    renderToolbar();

    expect(screen.getByRole('button', { name: 'chat.clearBtn, Alt+X' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Historico' })).toHaveAttribute('title', 'Alt+H');
    expect(await screen.findByRole('button', { name: 'chat.modelOverride.label, $default' })).toHaveAttribute('title', 'Alt+M');
    expect(screen.getByRole('button', { name: 'Perfil' })).not.toHaveAttribute('title', 'Ctrl+P');
  });

  it('abre o modelo pelo alvo estável mesmo sem data-shortcut', async () => {
    renderToolbar();
    const model = await screen.findByRole('button', { name: 'chat.modelOverride.label, $default' });
    expect(model).not.toHaveAttribute('data-shortcut');
    expect(model.closest('[data-chat-picker="model"]')).toBeInTheDocument();

    expect(dispatchCtrlKey('m').defaultPrevented).toBe(true);
    expect(modelOpenMock).toHaveBeenCalledOnce();
  });

  it.each(['hidden', 'inert', 'aria-hidden'] as const)('ancestral %s impede captura da toolbar', attribute => {
    const view = renderToolbar();
    view.container.setAttribute(attribute, attribute === 'aria-hidden' ? 'true' : '');
    expect(captureChatClearTarget(() => '/')).toBeUndefined();
    dispatchCtrlKey('l');
    expect(clearPort.beginUICommand).not.toHaveBeenCalled();
    expect(clearPort.commitBackendCommand).not.toHaveBeenCalled();
  });
  beforeEach(() => {
    clearConversationMock.mockClear();
    loadConversationSessionMock.mockClear();
    clearConversationMessagesMock.mockClear();
    historyClickMock.mockClear();
    profileClickMock.mockClear();
    updateTabMock.mockClear();
    getProfileMock.mockReset().mockResolvedValue({
      chat: { llm_provider: 'native-provider', model: 'modelo-perfil' },
    });
    getProvidersMock.mockReset().mockResolvedValue([
      { id: 'native-provider', api_format: 'openai', is_default: true },
    ]);
    modelChangeRef.current = null;
    profileChangeRef.current = null;
    shortcutHintsRef.current = {
      'chat.history.open': 'Ctrl+H',
      'chat.model.open': 'Ctrl+M',
      'chat.profile.open': 'Ctrl+P',
    };
    modalState.open = false;
    modalState.inside = false;
    modalState.topmost = true;
  });

  it('aciona atalhos do chat quando o toolbar esta dentro do modal topmost', async () => {
    modalState.open = true;
    modalState.inside = true;
    modalState.topmost = true;
    renderToolbar();
    await screen.findByRole('button', {
      name: 'chat.modelOverride.label, $default',
    });

    const modalOverlay = document.createElement('div');
    modalOverlay.className = 'modal-overlay';
    modalOverlay.setAttribute('role', 'dialog');
    const modalControl = document.createElement('button');
    modalOverlay.appendChild(modalControl);
    document.body.appendChild(modalOverlay);

    expect(dispatchCtrlKey('m', modalControl).defaultPrevented).toBe(true);
    expect(dispatchCtrlKey('h', modalControl).defaultPrevented).toBe(true);
    expect(dispatchCtrlKey('p', modalControl).defaultPrevented).toBe(true);
    expect(dispatchCtrlKey('l', modalControl).defaultPrevented).toBe(true);

    expect(modelOpenMock).toHaveBeenCalledOnce();
    expect(historyClickMock).toHaveBeenCalledTimes(1);
    expect(profileClickMock).toHaveBeenCalledTimes(1);
    await waitFor(() => {
      expect(clearPort.commitBackendCommand).toHaveBeenCalledExactlyOnceWith('clear-ticket', 'clear-handoff');
      expect(loadConversationSessionMock).not.toHaveBeenCalled();
    });
    modalOverlay.remove();
  });

  it('deixa a toolbar do modal tratar o evento prevenido pela superfície atrás dele', async () => {
    modalState.open = true;
    modalState.inside = false;
    renderToolbar();
    modalState.inside = true;
    renderToolbar();
    await waitFor(() => {
      expect(screen.getAllByRole('button', {
        name: 'chat.modelOverride.label, $default',
      })).toHaveLength(2);
    });

    expect(dispatchCtrlKey('m').defaultPrevented).toBe(false);
    expect(dispatchCtrlKey('h').defaultPrevented).toBe(false);
    expect(dispatchCtrlKey('p').defaultPrevented).toBe(false);
    expect(dispatchCtrlKey('l').defaultPrevented).toBe(true);

    expect(modelOpenMock).not.toHaveBeenCalled();
    expect(historyClickMock).not.toHaveBeenCalled();
    expect(profileClickMock).not.toHaveBeenCalled();
    await waitFor(() => expect(clearPort.commitBackendCommand).toHaveBeenCalledOnce());
  });

  it('bloqueia atalhos do chat quando outro modal esta no topo', () => {
    modalState.open = true;
    modalState.inside = true;
    modalState.topmost = false;
    renderToolbar();

    expect(dispatchCtrlKey('h').defaultPrevented).toBe(false);
    expect(dispatchCtrlKey('p').defaultPrevented).toBe(false);
    expect(dispatchCtrlKey('l').defaultPrevented).toBe(false);

    expect(historyClickMock).not.toHaveBeenCalled();
    expect(profileClickMock).not.toHaveBeenCalled();
    expect(clearConversationMock).not.toHaveBeenCalled();
    expect(clearPort.commitBackendCommand).not.toHaveBeenCalled();
  });

  it('nao deixa atalhos vazarem para toolbar atras de modal', () => {
    modalState.open = true;
    modalState.inside = false;
    modalState.topmost = true;
    renderToolbar();

    expect(dispatchCtrlKey('h').defaultPrevented).toBe(false);
    expect(dispatchCtrlKey('p').defaultPrevented).toBe(false);
    expect(dispatchCtrlKey('l').defaultPrevented).toBe(false);

    expect(historyClickMock).not.toHaveBeenCalled();
    expect(profileClickMock).not.toHaveBeenCalled();
    expect(clearConversationMock).not.toHaveBeenCalled();
    expect(clearPort.commitBackendCommand).not.toHaveBeenCalled();
  });

  it('continua acionando atalhos quando nenhum modal esta aberto', () => {
    renderToolbar();

    dispatchCtrlKey('H');
    dispatchCtrlKey('P');

    expect(screen.getByRole('heading', { name: 'Conversa' })).toBeInTheDocument();
    expect(historyClickMock).toHaveBeenCalledTimes(1);
    expect(profileClickMock).toHaveBeenCalledTimes(1);
  });

  it('não limpa a conversa enquanto a sessão está carregando', () => {
    isLoadingRef.current = true;
    renderToolbar();

    expect(dispatchCtrlKey('l').defaultPrevented).toBe(false);
    expect(clearConversationMock).not.toHaveBeenCalled();
    expect(clearPort.commitBackendCommand).not.toHaveBeenCalled();
    expect(clearConversationMessagesMock).not.toHaveBeenCalled();
  });

  it('Ctrl+M abre uma vez o seletor de modelos do chat ativo', async () => {
    renderToolbar();
    const trigger = await screen.findByRole('button', {
      name: 'chat.modelOverride.label, $default',
    });

    const event = dispatchModelShortcut();

    expect(event.defaultPrevented).toBe(true);
    expect(modelOpenMock).toHaveBeenCalledOnce();
    expect(trigger).toHaveAttribute('title', 'Ctrl+M');
  });

  it('em surfaces mantidas montadas, somente a ativa responde aos atalhos', async () => {
    render(
      <MemoryRouter>
        <ChatToolbar enableShortcuts={false} />
        <ChatToolbar enableShortcuts />
      </MemoryRouter>,
    );
    await waitFor(() => {
      expect(screen.getAllByRole('button', {
        name: 'chat.modelOverride.label, $default',
      })).toHaveLength(2);
    });

    dispatchModelShortcut();
    dispatchCtrlKey('h');
    dispatchCtrlKey('p');
    dispatchCtrlKey('l');

    expect(modelOpenMock).toHaveBeenCalledOnce();
    expect(historyClickMock).toHaveBeenCalledOnce();
    expect(profileClickMock).toHaveBeenCalledOnce();
    await waitFor(() => expect(clearPort.commitBackendCommand).toHaveBeenCalledOnce());
  });

  it('continua acionando os atalhos após Escape quando a superfície interrompe a propagação', async () => {
    renderToolbar();
    await screen.findByRole('button', {
      name: 'chat.modelOverride.label, $default',
    });

    const surface = document.createElement('div');
    surface.setAttribute('role', 'document');
    const trigger = document.createElement('button');
    const menu = document.createElement('div');
    const menuItem = document.createElement('button');
    menu.setAttribute('role', 'menu');
    menu.appendChild(menuItem);
    surface.append(trigger, menu);
    document.body.appendChild(surface);
    surface.addEventListener('keydown', (event) => {
      if (event.key === 'Escape') {
        menu.hidden = true;
        trigger.focus();
      }
      event.stopPropagation();
    });

    menuItem.focus();
    fireEvent.keyDown(menuItem, { key: 'Escape' });
    expect(trigger).toHaveFocus();

    dispatchCtrlKey('m', trigger);
    dispatchCtrlKey('h', trigger);
    dispatchCtrlKey('p', trigger);
    dispatchCtrlKey('l', trigger);

    expect(modelOpenMock).toHaveBeenCalledOnce();
    expect(historyClickMock).toHaveBeenCalledOnce();
    expect(profileClickMock).toHaveBeenCalledOnce();
    await waitFor(() => expect(clearPort.commitBackendCommand).toHaveBeenCalledOnce());

    surface.remove();
  });

  it('não captura atalhos durante a edição de uma mensagem', async () => {
    renderToolbar();
    await screen.findByRole('button', {
      name: 'chat.modelOverride.label, $default',
    });

    const editor = document.createElement('div');
    editor.className = 'chat-message__edit';
    const textarea = document.createElement('textarea');
    editor.appendChild(textarea);
    document.body.appendChild(editor);

    expect(dispatchCtrlKey('m', textarea).defaultPrevented).toBe(false);
    expect(dispatchCtrlKey('h', textarea).defaultPrevented).toBe(false);
    expect(dispatchCtrlKey('p', textarea).defaultPrevented).toBe(false);
    expect(dispatchCtrlKey('l', textarea).defaultPrevented).toBe(false);

    expect(modelOpenMock).not.toHaveBeenCalled();
    expect(historyClickMock).not.toHaveBeenCalled();
    expect(profileClickMock).not.toHaveBeenCalled();
    expect(clearConversationMock).not.toHaveBeenCalled();
    expect(clearPort.commitBackendCommand).not.toHaveBeenCalled();

    editor.remove();
  });

  it('não captura atalhos em descendentes de editor, terminal ou aba não-chat', async () => {
    renderToolbar();
    await screen.findByRole('button', {
      name: 'chat.modelOverride.label, $default',
    });

    const containers = [
      Object.assign(document.createElement('div'), { className: 'monaco-editor' }),
      Object.assign(document.createElement('div'), { className: 'xterm' }),
      document.createElement('div'),
      document.createElement('div'),
    ];
    containers[2].setAttribute('role', 'terminal');
    containers[3].setAttribute('data-tab-type', 'editor');

    containers.forEach((container) => {
      const descendant = document.createElement('button');
      container.appendChild(descendant);
      document.body.appendChild(container);
      expect(dispatchCtrlKey('m', descendant).defaultPrevented).toBe(false);
      expect(dispatchCtrlKey('h', descendant).defaultPrevented).toBe(false);
      expect(dispatchCtrlKey('p', descendant).defaultPrevented).toBe(false);
      expect(dispatchCtrlKey('l', descendant).defaultPrevented).toBe(false);
    });

    expect(modelOpenMock).not.toHaveBeenCalled();
    expect(historyClickMock).not.toHaveBeenCalled();
    expect(profileClickMock).not.toHaveBeenCalled();
    expect(clearConversationMock).not.toHaveBeenCalled();
    expect(clearPort.commitBackendCommand).not.toHaveBeenCalled();

    containers.forEach((container) => container.remove());
  });

  it('bloqueia atalhos dentro de diálogo virtual sem bloquear uma região de leitura comum', async () => {
    renderToolbar();
    await screen.findByRole('button', {
      name: 'chat.modelOverride.label, $default',
    });

    const virtualDialog = document.createElement('div');
    virtualDialog.setAttribute('role', 'dialog');
    virtualDialog.setAttribute('aria-modal', 'true');
    const documentRegion = document.createElement('div');
    documentRegion.setAttribute('role', 'document');
    const descendant = document.createElement('button');
    documentRegion.appendChild(descendant);
    virtualDialog.appendChild(documentRegion);
    document.body.appendChild(virtualDialog);

    expect(dispatchCtrlKey('m', descendant).defaultPrevented).toBe(false);
    expect(dispatchCtrlKey('h', descendant).defaultPrevented).toBe(false);
    expect(dispatchCtrlKey('p', descendant).defaultPrevented).toBe(false);
    expect(dispatchCtrlKey('l', descendant).defaultPrevented).toBe(false);

    expect(modelOpenMock).not.toHaveBeenCalled();
    expect(historyClickMock).not.toHaveBeenCalled();
    expect(profileClickMock).not.toHaveBeenCalled();
    expect(clearConversationMock).not.toHaveBeenCalled();
    expect(clearPort.commitBackendCommand).not.toHaveBeenCalled();

    virtualDialog.remove();
  });

  it('não intercepta atalhos em campos editáveis, editor ou terminal', async () => {
    renderToolbar();
    await screen.findByRole('button', {
      name: 'chat.modelOverride.label, $default',
    });

    const contentEditable = document.createElement('div');
    contentEditable.setAttribute('contenteditable', 'true');
    const targets: HTMLElement[] = [
      document.createElement('textarea'),
      document.createElement('input'),
      document.createElement('select'),
      contentEditable,
    ];
    const inheritedContentEditable = document.createElement('div');
    inheritedContentEditable.setAttribute('contenteditable', '');
    const inheritedEditableTarget = document.createElement('span');
    inheritedContentEditable.appendChild(inheritedEditableTarget);
    targets.push(inheritedEditableTarget);
    const plaintextContentEditable = document.createElement('div');
    plaintextContentEditable.setAttribute('contenteditable', 'plaintext-only');
    const plaintextEditableTarget = document.createElement('span');
    plaintextContentEditable.appendChild(plaintextEditableTarget);
    targets.push(plaintextEditableTarget);
    const monaco = document.createElement('div');
    monaco.className = 'monaco-editor';
    const monacoTarget = document.createElement('span');
    monaco.appendChild(monacoTarget);
    targets.push(monacoTarget);
    const terminal = document.createElement('div');
    terminal.className = 'xterm';
    const terminalTarget = document.createElement('span');
    terminal.appendChild(terminalTarget);
    targets.push(terminalTarget);
    targets.slice(0, 4).forEach((target) => document.body.appendChild(target));
    document.body.append(inheritedContentEditable, plaintextContentEditable, monaco, terminal);

    targets.forEach((target) => {
      expect(dispatchModelShortcut(target).defaultPrevented).toBe(false);
      expect(dispatchCtrlKey('h', target).defaultPrevented).toBe(false);
      expect(dispatchCtrlKey('p', target).defaultPrevented).toBe(false);
      expect(dispatchCtrlKey('l', target).defaultPrevented).toBe(false);
    });

    modalState.open = true;
    expect(dispatchModelShortcut().defaultPrevented).toBe(false);
    modalState.open = false;

    const menu = document.createElement('div');
    menu.setAttribute('role', 'menu');
    document.body.appendChild(menu);
    expect(dispatchModelShortcut().defaultPrevented).toBe(false);
    menu.remove();

    const picker = document.createElement('div');
    picker.className = 'picker-dropdown';
    const pickerInput = document.createElement('input');
    picker.appendChild(pickerInput);
    document.body.appendChild(picker);
    expect(dispatchModelShortcut(pickerInput).defaultPrevented).toBe(false);
    expect(dispatchCtrlKey('h', pickerInput).defaultPrevented).toBe(false);
    expect(dispatchCtrlKey('p', pickerInput).defaultPrevented).toBe(false);
    expect(dispatchCtrlKey('l', pickerInput).defaultPrevented).toBe(false);
    picker.remove();

    expect(modelOpenMock).not.toHaveBeenCalled();
    expect(historyClickMock).not.toHaveBeenCalled();
    expect(profileClickMock).not.toHaveBeenCalled();
    expect(clearConversationMock).not.toHaveBeenCalled();
    expect(clearPort.commitBackendCommand).not.toHaveBeenCalled();
    targets.forEach((target) => target.closest('body') && target.remove());
    inheritedContentEditable.remove();
    plaintextContentEditable.remove();
    monaco.remove();
    terminal.remove();
  });

  it('bloqueia listbox portalado visível, mas ignora o mesmo listbox oculto', async () => {
    renderToolbar();
    await screen.findByRole('button', {
      name: 'chat.modelOverride.label, $default',
    });

    const outsideFocus = document.createElement('button');
    const portalListbox = document.createElement('ul');
    portalListbox.setAttribute('role', 'listbox');
    document.body.append(outsideFocus, portalListbox);
    outsideFocus.focus();

    const blockedEvent = dispatchModelShortcut(outsideFocus);
    expect(blockedEvent.defaultPrevented).toBe(false);
    expect(dispatchCtrlKey('h', outsideFocus).defaultPrevented).toBe(false);
    expect(dispatchCtrlKey('p', outsideFocus).defaultPrevented).toBe(false);
    expect(dispatchCtrlKey('l', outsideFocus).defaultPrevented).toBe(false);
    expect(modelOpenMock).not.toHaveBeenCalled();
    expect(historyClickMock).not.toHaveBeenCalled();
    expect(profileClickMock).not.toHaveBeenCalled();
    expect(clearConversationMock).not.toHaveBeenCalled();
    expect(clearPort.commitBackendCommand).not.toHaveBeenCalled();

    portalListbox.hidden = true;
    const normalEvent = dispatchModelShortcut(outsideFocus);
    expect(normalEvent.defaultPrevented).toBe(true);
    dispatchCtrlKey('h', outsideFocus);
    dispatchCtrlKey('p', outsideFocus);
    dispatchCtrlKey('l', outsideFocus);
    expect(modelOpenMock).toHaveBeenCalledOnce();
    expect(historyClickMock).toHaveBeenCalledOnce();
    expect(profileClickMock).toHaveBeenCalledOnce();
    await waitFor(() => expect(clearPort.commitBackendCommand).toHaveBeenCalledOnce());

    outsideFocus.remove();
    portalListbox.remove();
  });

  it('respeita evento tratado, IME, modificadores extras e repetição', async () => {
    renderToolbar();
    await screen.findByRole('button', {
      name: 'chat.modelOverride.label, $default',
    });

    const prevented = new KeyboardEvent('keydown', {
      key: 'm',
      ctrlKey: true,
      bubbles: true,
      cancelable: true,
    });
    prevented.preventDefault();
    window.dispatchEvent(prevented);

    dispatchModelShortcut(window, { isComposing: true });
    dispatchModelShortcut(window, { shiftKey: true });
    dispatchModelShortcut(window, { altKey: true });
    dispatchModelShortcut(window, { metaKey: true });
    dispatchModelShortcut(window, { repeat: true });
    ['m', 'h', 'p', 'l'].forEach((key) => {
      const legacyIMEEvent = new KeyboardEvent('keydown', {
        key,
        code: `Key${key.toUpperCase()}`,
        ctrlKey: true,
        bubbles: true,
        cancelable: true,
      });
      Object.defineProperty(legacyIMEEvent, 'keyCode', { value: 229 });
      window.dispatchEvent(legacyIMEEvent);
    });

    expect(modelOpenMock).not.toHaveBeenCalled();
    expect(historyClickMock).not.toHaveBeenCalled();
    expect(profileClickMock).not.toHaveBeenCalled();
    expect(clearConversationMock).not.toHaveBeenCalled();
    expect(clearPort.commitBackendCommand).not.toHaveBeenCalled();
  });

  it('não mantém listener legado de M/H/P após desmontar', async () => {
    const view = renderToolbar();
    await screen.findByRole('button', {
      name: 'chat.modelOverride.label, $default',
    });
    view.unmount();

    dispatchModelShortcut();

    expect(modelOpenMock).not.toHaveBeenCalled();
  });

  it('keydown cru de M/H/P não executa sem o dispatcher', async () => {
    renderToolbar();
    await screen.findByRole('button', {
      name: 'chat.modelOverride.label, $default',
    });

    for (const key of ['m', 'h', 'p']) {
      const event = new KeyboardEvent('keydown', {
        key,
        ctrlKey: true,
        bubbles: true,
        cancelable: true,
      });
      window.dispatchEvent(event);
      expect(event.defaultPrevented).toBe(false);
    }
    expect(modelOpenMock).not.toHaveBeenCalled();
    expect(historyClickMock).not.toHaveBeenCalled();
    expect(profileClickMock).not.toHaveBeenCalled();
  });
});

// O seletor de modelo do agente precisa aparecer na barra da conversa aberta, e
// pela conversa dela: é o caminho que a pessoa usa para trocar de modelo, e uma
// ligação errada aqui trocaria o modelo de outra conversa (AEP-0084 D6).
describe('ChatToolbar e o modelo do agente', () => {
  beforeEach(() => {
    getAgentSessionOptionsMock.mockClear();
    getProfileMock.mockResolvedValue({
      chat: { llm_provider: 'agent-provider', model: 'modelo-a' },
    });
    getProvidersMock.mockResolvedValue([
      { id: 'agent-provider', api_format: 'acp', is_default: true },
    ]);
  });

  it('mostra o modelo do agente da conversa aberta', async () => {
    getAgentSessionOptionsMock.mockResolvedValueOnce({
      conversationId: 'conversation-1',
      available: true,
      options: [{
        id: 'model',
        name: 'Modelo',
        category: 'model',
        currentValue: 'modelo-a',
        values: [{ value: 'modelo-a', name: 'Modelo A' }, { value: 'modelo-b', name: 'Modelo B' }],
      }],
    });

    renderToolbar();

    await waitFor(() => expect(getAgentSessionOptionsMock).toHaveBeenCalledWith('conversation-1'));
    expect(await screen.findByRole('button', { name: 'Modelo, Modelo A' })).toBeInTheDocument();
  });

  it('abre o modelo ACP, e não o modo, pelo alvo de modelo separado', async () => {
    getAgentSessionOptionsMock.mockResolvedValue({
      conversationId: 'conversation-1',
      available: true,
      options: [
        {
          id: 'model',
          name: 'Modelo',
          category: 'model',
          currentValue: 'modelo-a',
          values: [{ value: 'modelo-a', name: 'Modelo A' }, { value: 'modelo-b', name: 'Modelo B' }],
        },
        {
          id: 'mode',
          name: 'Modo',
          category: 'mode',
          currentValue: 'agent',
          values: [{ value: 'agent' }, { value: 'plan' }],
        },
      ],
    });

    renderToolbar();

    const model = await screen.findByRole('button', { name: 'Modelo, Modelo A' });
    const mode = screen.getByRole('button', { name: 'Modo, chat.agentOptions.mode.agent' });
    expect(model.closest('[data-chat-picker="model"]')).toBeInTheDocument();
    expect(mode.closest('[data-chat-picker="model"]')).toBeNull();

    expect(dispatchCtrlKey('m').defaultPrevented).toBe(true);
    expect(await screen.findByRole('combobox', { name: 'Modelo - pickers.combobox.filterLabel' })).toBeInTheDocument();
    expect(screen.queryByRole('combobox', { name: 'Modo - pickers.combobox.filterLabel' })).not.toBeInTheDocument();
  });

  it('mostra o seletor nativo quando a conversa não fala com agente', async () => {
    getProfileMock.mockResolvedValue({
      chat: { llm_provider: 'native-provider', model: 'modelo-perfil' },
    });
    getProvidersMock.mockResolvedValue([
      { id: 'native-provider', api_format: 'openai', is_default: true },
    ]);
    getAgentSessionOptionsMock.mockResolvedValueOnce({
      conversationId: 'conversation-1',
      available: false,
      options: [],
    });

    renderToolbar();

    await waitFor(() => expect(getAgentSessionOptionsMock).toHaveBeenCalledWith('conversation-1'));
    expect(await screen.findByRole('button', { name: 'chat.modelOverride.label, $default' })).toBeInTheDocument();
  });
});

describe('ChatToolbar e o modelo nativo da aba', () => {
  beforeEach(() => {
    getAgentSessionOptionsMock.mockResolvedValue({
      conversationId: 'conversation-1',
      available: false,
      options: [],
    });
  });

  it('persiste a escolha no ProfileOverride da aba', async () => {
    renderToolbar();
    await screen.findByRole('button', { name: 'chat.modelOverride.label, $default' });

    modelChangeRef.current?.('modelo-b');

    await waitFor(() => expect(updateTabMock).toHaveBeenCalledWith('tab-chat', {
      profile_override: { model: 'modelo-b' },
    }));
  });

  it('remove o override com nil ao voltar ao modelo do perfil', async () => {
    mockPanelTabRef.current = {
      id: 'tab-chat',
      title: 'Chat',
      type: 'chat',
      profileOverride: { slug: 'padrao', model: 'modelo-b' },
    } as unknown as Record<string, unknown>;
    renderToolbar();
    await screen.findByRole('button', { name: 'chat.modelOverride.label, modelo-b' });

    modelChangeRef.current?.('$default');

    await waitFor(() => expect(updateTabMock).toHaveBeenCalledWith('tab-chat', {
      profile_override: { model: null },
    }));
  });

  it('normaliza o modelo e trata entrada vazia como reset', async () => {
    renderToolbar();
    await screen.findByRole('button', { name: 'chat.modelOverride.label, $default' });

    modelChangeRef.current?.('  modelo-b  ');
    await waitFor(() => expect(updateTabMock).toHaveBeenNthCalledWith(1, 'tab-chat', {
      profile_override: { model: 'modelo-b' },
    }));

    modelChangeRef.current?.('   ');
    await waitFor(() => expect(updateTabMock).toHaveBeenNthCalledWith(2, 'tab-chat', {
      profile_override: { model: null },
    }));
  });

  it('limpa modelo incompatível ao trocar para perfil de outro provider', async () => {
    mockPanelTabRef.current = {
      id: 'tab-chat',
      title: 'Chat',
      type: 'chat',
      profileOverride: { slug: 'perfil-a', model: 'modelo-a' },
    } as unknown as Record<string, unknown>;
    getProfileMock.mockImplementation(async (slug: string) => ({
      chat: {
        llm_provider: slug === 'perfil-b' ? 'provider-b' : 'provider-a',
        model: 'modelo-perfil',
      },
    }));
    getProvidersMock.mockResolvedValue([
      { id: 'provider-a', api_format: 'openai', is_default: true },
      { id: 'provider-b', api_format: 'anthropic' },
    ]);
    renderToolbar();
    await waitFor(() => expect(profileChangeRef.current).not.toBeNull());

    profileChangeRef.current?.('perfil-b');

    await waitFor(() => expect(updateTabMock).toHaveBeenCalledWith('tab-chat', {
      profile_override: { slug: 'perfil-b', model: null },
    }));
  });

  it('preserva modelo compatível ao trocar perfil no mesmo provider', async () => {
    mockPanelTabRef.current = {
      id: 'tab-chat',
      title: 'Chat',
      type: 'chat',
      profileOverride: { slug: 'perfil-a', model: 'modelo-a' },
    } as unknown as Record<string, unknown>;
    getProfileMock.mockResolvedValue({
      chat: { llm_provider: 'provider-a', model: 'modelo-perfil' },
    });
    getProvidersMock.mockResolvedValue([
      { id: 'provider-a', api_format: 'openai', is_default: true },
    ]);
    renderToolbar();
    await waitFor(() => expect(profileChangeRef.current).not.toBeNull());

    profileChangeRef.current?.('perfil-b');

    await waitFor(() => expect(updateTabMock).toHaveBeenCalledWith('tab-chat', {
      profile_override: { slug: 'perfil-b' },
    }));
  });

  it('serializa seleções rápidas e aplica a última por último', async () => {
    let resolveFirst!: () => void;
    updateTabMock
      .mockImplementationOnce(() => new Promise<void>((resolve) => {
        resolveFirst = resolve;
      }))
      .mockResolvedValueOnce(undefined);
    renderToolbar();
    await screen.findByRole('button', { name: 'chat.modelOverride.label, $default' });

    act(() => {
      modelChangeRef.current?.('modelo-a');
      modelChangeRef.current?.('modelo-b');
    });

    await waitFor(() => expect(updateTabMock).toHaveBeenCalledTimes(1));
    expect(updateTabMock).toHaveBeenNthCalledWith(1, 'tab-chat', {
      profile_override: { model: 'modelo-a' },
    });

    await act(async () => {
      resolveFirst();
      await Promise.resolve();
    });
    await waitFor(() => expect(updateTabMock).toHaveBeenCalledTimes(2));
    expect(updateTabMock).toHaveBeenNthCalledWith(2, 'tab-chat', {
      profile_override: { model: 'modelo-b' },
    });
  });
});

describe('ChatToolbar menu de perfil', () => {
  beforeEach(() => {
    requestResourceEditMock.mockClear();
    openAtPointMock.mockClear();
    mockPanelTabRef.current = { id: 'tab-chat', title: 'Chat', type: 'chat' } as unknown as Record<string, unknown>;
  });

  it('edita o perfil da aba quando há override', async () => {
    mockPanelTabRef.current = {
      id: 'tab-chat',
      title: 'Chat',
      type: 'chat',
      profileOverride: { slug: 'custom' },
    } as unknown as Record<string, unknown>;

    renderToolbar();
    const container = screen.getByTestId('profile-picker-container');
    fireEvent.contextMenu(container, { clientX: 10, clientY: 10 });

    await waitFor(() => expect(openAtPointMock).toHaveBeenCalled());
    const items = openAtPointMock.mock.calls[0][3] as Array<{ id: string; action: () => void }>;
    const editItem = items.find((i) => i.id === 'edit-active-profile');
    editItem?.action();
    expect(requestResourceEditMock).toHaveBeenCalledWith('profiles', 'custom', 'edit');
  });

  it('edita o perfil padrão quando não há override', async () => {
    mockPanelTabRef.current = { id: 'tab-chat', title: 'Chat', type: 'chat' } as unknown as Record<string, unknown>;

    renderToolbar();
    const container = screen.getByTestId('profile-picker-container');
    fireEvent.contextMenu(container, { clientX: 10, clientY: 10 });

    await waitFor(() => expect(openAtPointMock).toHaveBeenCalled());
    const items = openAtPointMock.mock.calls[0][3] as Array<{ id: string; action: () => void }>;
    const editItem = items.find((i) => i.id === 'edit-active-profile');
    editItem?.action();
    expect(requestResourceEditMock).toHaveBeenCalledWith('profiles', 'padrao', 'edit');
  });
});
