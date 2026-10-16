import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const sendMessageMock = vi.fn();
const updateMessageMock = vi.fn();
const showMenuMock = vi.fn();
const hideMenuMock = vi.fn();
const copyMessageMock = vi.fn();
const speakMessageMock = vi.fn();
let capturedPanelProps: import('../components/chat/ChatPanel').ChatPanelProps | undefined;
vi.mock('../components/chat/ChatPanel', async importOriginal => {
  const actual = await importOriginal<typeof import('../components/chat/ChatPanel')>();
  return { ...actual, ChatPanel: (props: import('../components/chat/ChatPanel').ChatPanelProps) => {
    capturedPanelProps = props;
    return <actual.ChatPanel {...props} />;
  } };
});
let capturedCommandSurfaceGetter: (() => unknown) | undefined;
let capturedCommandSurfaceSubscribe: ((invalidate: () => void) => () => void) | undefined;
const ensureWorkspaceTabHasConversationMock = vi.fn().mockResolvedValue('01926b90-7a5a-7c4e-8d3f-000000000001');
const conversationId = '01926b90-7a5a-7c4e-8d3f-000000000001';
const activeConversation = { id: conversationId, title: 'Conversa', threadedMessages: [] };
type ChatSessionMock = {
  conversation: typeof activeConversation;
  sessionKey: string;
  isLoading: boolean;
  isThinking: boolean;
  streamingMessageId: string | null;
  queuedTurnCount: number;
  hasOlderMessages: boolean;
  isLoadingOlderMessages: boolean;
};
type ChatStoreListenerState = {
  sessionsByConversationId: Record<string, ChatSessionMock>;
};
const chatStoreListeners: Array<(state: ChatStoreListenerState) => void> = [];
const workspaceStoreState = {
  workspace: {
    id: 'workspace-a',
    profile: 'workspace-profile',
    activeTabId: 'chat-tab',
    tabs: [{ id: 'chat-tab', type: 'chat' as const, conversationId, title: 'Conversa', position: 0 }],
  },
  getActiveTab: () => ({ id: 'chat-tab', type: 'chat' as const, conversationId, title: 'Conversa', position: 0 }),
};

vi.mock('react-router-dom', () => ({
  useLocation: () => ({ pathname: '/' }),
  useNavigate: () => vi.fn(),
}));

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

vi.mock('../services/tts', () => ({
  ttsService: {
    isEnabled: () => false,
    hasVoiceConfig: () => false,
    on: vi.fn(),
    off: vi.fn(),
  },
}));

const chatStoreState = {
  getDraftRevision: () => 0,
  getMessagingPipelineRevision: () => 0,
  waitForMessagingAdmission: async () => {},
  clearConversationDraft: vi.fn(),
  sendMessageToConversation: sendMessageMock,
  ensureConversationSurfaceSession: vi.fn(),
  removeConversationSurfaceSession: vi.fn(),
  clearConversationSendFailure: vi.fn(),
  sessionsByConversationId: {
    [conversationId]: {
      conversation: activeConversation,
      sessionKey: `page:tab:chat-tab:${conversationId}`,
      isLoading: false,
      isThinking: false,
      streamingMessageId: null,
      queuedTurnCount: 0,
      hasOlderMessages: false,
      isLoadingOlderMessages: false,
    },
  } as Record<string, ChatSessionMock>,
  timelinesByConversationId: {},
  surfaceSessionsByKey: {} as Record<string, ReturnType<typeof createEmptyChatSurfaceSession>>,
  loadMessageChildren: vi.fn(),
  loadConversationSession: vi.fn(),
  updateConversationMessage: updateMessageMock,
  toggleConversationReasoningExpanded: vi.fn(),
  isConversationReasoningExpanded: () => false,
  startConversationEditing: vi.fn(),
};

vi.mock('../store/chatStore', () => ({
  useChatStore: Object.assign(
    (selector?: (s: typeof chatStoreState) => unknown) => {
      if (typeof selector === 'function') {
        return selector(chatStoreState);
      }
      return chatStoreState;
    },
    {
      getState: () => chatStoreState,
      subscribe: (listener: (state: ChatStoreListenerState) => void) => {
        chatStoreListeners.push(listener);
        return () => {
          const index = chatStoreListeners.indexOf(listener);
          if (index >= 0) chatStoreListeners.splice(index, 1);
        };
      },
    },
  ),
}));

vi.mock('../components/workspace/useWorkspaceCommandSurface', () => ({
  useWorkspaceCommandSurface: (
    _surfaceType: string,
    getter: () => unknown,
    subscribe?: (invalidate: () => void) => () => void,
  ) => {
    capturedCommandSurfaceGetter = getter;
    capturedCommandSurfaceSubscribe = subscribe;
  },
}));

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: Object.assign(
    (selector?: (s: typeof workspaceStoreState) => unknown) => {
      if (typeof selector === 'function') return selector(workspaceStoreState);
      return workspaceStoreState;
    },
    { getState: () => workspaceStoreState, subscribe: () => () => {} },
  ),
  useActiveTab: () => workspaceStoreState.getActiveTab(),
}));

vi.mock('../lib/workspaceConversation', () => ({
  ensureWorkspaceTabHasConversation: (...args: unknown[]) => ensureWorkspaceTabHasConversationMock(...args),
}));

vi.mock('../store/editorStore', () => ({
  useEditorStore: {
    getState: () => ({
      activeTabId: 'chat-tab',
      tabs: [{ id: 'chat-tab', conversationId: '01926b90-7a5a-7c4e-8d3f-00000000000a' }],
    }),
  },
}));

vi.mock('../hooks/useChatKeyboardNav', () => ({
  useChatKeyboardNav: () => {},
}));

vi.mock('../hooks/useTabsKeyboardShortcuts', () => ({
  useTabsKeyboardShortcuts: () => {},
}));

vi.mock('../hooks/useContextMenu', () => ({
  useContextMenu: () => ({
    menuVisible: true,
    menuPosition: { x: 1, y: 2 },
    menuItems: [{ id: 'copy', label: 'Copiar' }],
    showMenu: showMenuMock,
    hideMenu: hideMenuMock,
  }),
  useMessageActions: () => ({
    copyMessage: copyMessageMock,
    speakMessage: speakMessageMock,
  }),
}));

vi.mock('@wailsjs/go/wailsapi/Conversations', () => ({
  DeleteMessage: vi.fn(),
}));

vi.mock('@wailsjs/go/wailsapi/ACPWorkDir', () => ({
  // A conversa destes testes não fala com agente de código: sem
  // diretório de agente a mostrar.
  GetAgentConversationWorkDir: vi.fn().mockRejectedValue(new Error('sem agente')),
  SetAgentConversationWorkDir: vi.fn(),
}));

vi.mock('@wailsjs/go/wailsapi/ACPCommands', () => ({
  // Sem comandos do agente no menu da barra.
  GetAgentSessionCommands: vi.fn().mockResolvedValue({ conversationId: '', commands: [] }),
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: () => () => {},
}));

vi.mock('../components/chat/ChatToolbar', () => ({
  ChatToolbar: ({ inputRef }: { inputRef?: React.RefObject<HTMLTextAreaElement> }) => (
    <div>Toolbar {inputRef ? 'ok' : 'no-ref'}</div>
  ),
}));

vi.mock('../components/chat/MessageList', () => ({
  MessageList: ({ onContextMenu }: { onContextMenu?: (event: MouseEvent, message: { id: string; role: string }) => void }) => (
    <button
      type="button"
      onClick={() => onContextMenu?.(new MouseEvent('contextmenu'), { id: 'm1', role: 'user' })}
    >
      open-menu
    </button>
  ),
}));

vi.mock('../components/chat/ChatInput', async () => {
  const React = await import('react');
  return {
    ChatInput: React.forwardRef<HTMLTextAreaElement, { onSend: (value: string) => void }>(
      ({ onSend }, ref) => (
        <button ref={ref as React.RefObject<HTMLButtonElement>} type="button" onClick={() => onSend('oi')}>
          send
        </button>
      )
    ),
  };
});

vi.mock('../components/menu', () => ({
  ContextMenu: ({ visible, items }: { visible: boolean; items: Array<{ id: string; label: string }> }) => (
    <div>{visible ? items.map((item) => item.label).join(',') : 'closed'}</div>
  ),
}));

vi.mock('../components/ui/KeyboardShortcutsHelp', () => ({
  KeyboardShortcutsHelp: ({ isOpen }: { isOpen: boolean }) => <div>{isOpen ? 'help-open' : 'help-closed'}</div>,
}));

vi.mock('../hooks/useAnnouncer', () => ({
  announce: vi.fn(),
  useAnnouncer: () => ({ announce: vi.fn(), announceRequest: vi.fn(() => true) }),
}));

vi.mock('../utils/errorHandler', () => ({
  ErrorSeverity: { RECOVERABLE: 'recoverable' },
  ErrorMessages: { CHAT: { SEND_FAILED: 'Falha ao enviar', DELETE_FAILED: 'Falha ao deletar' } },
  handleError: vi.fn(),
}));

import ChatPage from './ChatPage';
import { useAuthStore } from '../store/authStore';
import { createEmptyChatSurfaceSession } from '../services/chatSessionRegistry';
import { CHAT_MESSAGING_COMMAND_EVENT, executeChatMessaging, type ChatMessagingRequest } from '../lib/commandChatMessaging';
const commandStatuses: string[] = [];
const commandPort = {
  beginUICommand: vi.fn(async (commandId: string) => ({ ticket: 'ticket', invocationId: 'invocation', commandId })),
  takeUICommand: vi.fn(async () => ({ ticket: 'ticket', invocationId: 'invocation', commandId: 'chat.message.send', handoffId: 'handoff' })),
  getUICommandResult: vi.fn(async () => ({ invocationId: 'invocation', status: 'succeeded' as const })),
  cancelUICommand: vi.fn(async () => {}),
  completeUICommand: vi.fn(async () => {}),
  commitBackendCommand: vi.fn(async () => {}),
};
// Topbar is outside this fixture: keep its event ingress and the real audited executor.
function handleMessagingRequest(event: Event) {
  const { target, commandId } = (event as CustomEvent<ChatMessagingRequest>).detail;
  if (!target) return;
  event.preventDefault();
  void executeChatMessaging(commandPort, target, commandId).then(status => commandStatuses.push(status));
}
import { WorkspacePanelProvider } from '../components/workspace/WorkspacePanelContext';

function getPanelTab() {
  return workspaceStoreState.getActiveTab();
}

function renderChatPage() {
  return render(
    <WorkspacePanelProvider value={{ tab: getPanelTab(), isActive: true }}>
      <ChatPage />
    </WorkspacePanelProvider>,
  );
}

describe('ChatPage', () => {
  beforeEach(() => {
    capturedPanelProps = undefined;
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    useAuthStore.setState({ isAuthenticated: true, user: { userId: 'owner', sessionId: 'session', role: 'admin' } });
    commandStatuses.length = 0;
    commandPort.beginUICommand.mockClear();
    commandPort.getUICommandResult.mockReset().mockResolvedValue({ invocationId: 'invocation', status: 'succeeded' });
    window.addEventListener(CHAT_MESSAGING_COMMAND_EVENT, handleMessagingRequest);
    chatStoreState.surfaceSessionsByKey = {};
    chatStoreState.ensureConversationSurfaceSession.mockImplementation((id: string, key: string) => {
      chatStoreState.surfaceSessionsByKey[key] ??= { ...createEmptyChatSurfaceSession(id, key), draftMessage: 'oi' };
    });
    capturedCommandSurfaceGetter = undefined;
    capturedCommandSurfaceSubscribe = undefined;
    chatStoreListeners.splice(0);
    workspaceStoreState.workspace = {
      id: 'workspace-a',
      profile: 'workspace-profile',
      activeTabId: 'chat-tab',
      tabs: [{ id: 'chat-tab', type: 'chat' as const, conversationId, title: 'Conversa', position: 0 }],
    };
    chatStoreState.sessionsByConversationId[conversationId] = {
      conversation: activeConversation,
      sessionKey: `page:tab:chat-tab:${conversationId}`,
      isLoading: false,
      isThinking: false,
      streamingMessageId: null,
      queuedTurnCount: 0,
      hasOlderMessages: false,
      isLoadingOlderMessages: false,
    };
    workspaceStoreState.getActiveTab = () => ({
      id: 'chat-tab',
      type: 'chat' as const,
      conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001',
      title: 'Conversa',
      position: 0,
    });
    sendMessageMock.mockReset();
    showMenuMock.mockReset();
    hideMenuMock.mockReset();
    ensureWorkspaceTabHasConversationMock.mockClear();
    sendMessageMock.mockResolvedValue(undefined);
  });

  afterEach(() => {
    window.removeEventListener(CHAT_MESSAGING_COMMAND_EVENT, handleMessagingRequest);
    vi.mocked(document.hasFocus).mockRestore();
  });

  it('abre menu de contexto ao acionar MessageList', async () => {
    renderChatPage();
    await userEvent.click(screen.getByRole('button', { name: 'open-menu' }));

    expect(showMenuMock).toHaveBeenCalled();
    expect(screen.getByText('Copiar')).toBeInTheDocument();
  });

  it('expõe sessão carregada coerente e recusa retarget silencioso da aba', () => {
    renderChatPage();

    expect(capturedCommandSurfaceGetter?.()).toMatchObject({
      surfaceId: 'chat-tab',
      metadata: { conversationId, sessionLoaded: true },
    });

    act(() => {
      workspaceStoreState.workspace.tabs[0].conversationId = 'conversation-other';
    });
    expect(capturedCommandSurfaceGetter?.()).toBeNull();
  });

  it('invalida fatos ABA da sessão, mas ignora notificações de outra conversa', () => {
    renderChatPage();
    expect(capturedCommandSurfaceSubscribe).toBeDefined();

    const invalidate = vi.fn();
    const unsubscribe = capturedCommandSurfaceSubscribe?.(invalidate);
    const original = chatStoreState.sessionsByConversationId[conversationId];
    const replacement = { ...original, sessionKey: `${original.sessionKey}:replacement` };
    chatStoreState.sessionsByConversationId[conversationId] = replacement;
    chatStoreListeners.forEach((listener) => listener(chatStoreState));
    chatStoreState.sessionsByConversationId[conversationId] = original;
    chatStoreListeners.forEach((listener) => listener(chatStoreState));
    expect(invalidate).toHaveBeenCalledTimes(2);

    chatStoreState.sessionsByConversationId['conversation-other'] = {
      ...original,
      conversation: { ...activeConversation, id: 'conversation-other' },
    };
    chatStoreListeners.forEach((listener) => listener(chatStoreState));
    expect(invalidate).toHaveBeenCalledTimes(2);
    unsubscribe?.();
  });

  it('adapter legado garante a conversa da aba antes de enviar quando ainda não existe conversationId', async () => {
    workspaceStoreState.getActiveTab = () => ({
      id: 'chat-tab',
      type: 'chat' as const,
      conversationId: '',
      title: 'Conversa',
      position: 0,
    });
    ensureWorkspaceTabHasConversationMock.mockResolvedValue('01926b90-7a5a-7c4e-8d3f-00000000002a');
    sendMessageMock.mockResolvedValueOnce(undefined);

    renderChatPage();

    expect(capturedPanelProps).toBeDefined();
    const panel = capturedPanelProps!;
    await act(async () => panel.onSend('oi', undefined, { conversationId: null, origin: panel.surface }));

    await waitFor(() => {
      expect(ensureWorkspaceTabHasConversationMock).toHaveBeenCalled();
      expect(sendMessageMock).toHaveBeenCalledWith(
        '01926b90-7a5a-7c4e-8d3f-00000000002a',
        'oi',
        undefined,
        expect.objectContaining({
          profileSlug: 'workspace-profile',
          tabType: 'chat',
        }),
        {
          command: undefined,
          origin: expect.objectContaining({
            conversationId: '01926b90-7a5a-7c4e-8d3f-00000000002a',
            sessionKey: 'page:tab:chat-tab:01926b90-7a5a-7c4e-8d3f-00000000002a',
            surfaceId: 'page:tab:chat-tab',
            surfaceType: 'page',
            tabId: 'chat-tab',
          }),
        },
      );
    });
  });

  it('mostra falha comprovada da sessão e permite retry auditado', async () => {
    const user = userEvent.setup();
    const key = `page:tab:chat-tab:${conversationId}`;
    chatStoreState.surfaceSessionsByKey[key] = {
      ...createEmptyChatSurfaceSession(conversationId, key),
      sendFailureMessage: 'Falha ao enviar', sendFailureRetryable: true,
      sendFailureRetryContent: 'oi',
    };
    renderChatPage();

    expect(await screen.findByText('Falha ao enviar')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();

    sendMessageMock.mockResolvedValueOnce(undefined);
    // Mock i18n returns translation keys as literal strings
    await user.click(screen.getByRole('button', { name: 'chat.retryAriaLabel' }));

    await waitFor(() => {
      expect(commandStatuses).toEqual(['succeeded']);
    });
    expect(sendMessageMock).toHaveBeenCalledTimes(1);
    expect(commandPort.beginUICommand).toHaveBeenCalledWith('chat.message.send');
    expect(sendMessageMock).toHaveBeenCalledWith(conversationId, 'oi', undefined,
      expect.objectContaining({ profileSlug: 'workspace-profile', tabType: 'chat' }),
      expect.objectContaining({ command: expect.objectContaining({ handoff: { ticket: 'ticket', handoffId: 'handoff' } }), origin: expect.objectContaining({ sessionKey: key }) }));
  });

  it('envio normal usa a conversa capturada e handoff, sem criar outra conversa', async () => {
    renderChatPage();
    await userEvent.click(screen.getByRole('button', { name: 'send' }));
    await waitFor(() => expect(commandStatuses).toEqual(['succeeded']));
    expect(ensureWorkspaceTabHasConversationMock).not.toHaveBeenCalled();
    expect(sendMessageMock).toHaveBeenCalledWith(conversationId, 'oi', undefined,
      expect.objectContaining({ profileSlug: 'workspace-profile', tabType: 'chat' }),
      { command: expect.objectContaining({ handoff: { ticket: 'ticket', handoffId: 'handoff' }, isCurrent: expect.any(Function) }),
        origin: expect.objectContaining({ conversationId, sessionKey: `page:tab:chat-tab:${conversationId}`, surfaceId: 'page:tab:chat-tab', surfaceType: 'page', tabId: 'chat-tab' }) });
  });

  it('sem conversa pronta, o botão não inicia admission nem criação implícita', async () => {
    workspaceStoreState.workspace.tabs[0].conversationId = '';
    workspaceStoreState.getActiveTab = () => workspaceStoreState.workspace.tabs[0];
    renderChatPage();
    await userEvent.click(screen.getByRole('button', { name: 'send' }));
    expect(commandPort.beginUICommand).not.toHaveBeenCalled();
    expect(ensureWorkspaceTabHasConversationMock).not.toHaveBeenCalled();
    expect(sendMessageMock).not.toHaveBeenCalled();
  });
});
