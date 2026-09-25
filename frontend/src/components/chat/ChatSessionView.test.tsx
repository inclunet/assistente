import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import {
  canFocusWorkspacePanelImmediately,
  getWorkspacePanelImmediateFocusHandler,
  hasWorkspacePanelFocusHandler,
  requestWorkspacePanelFocus,
} from '../workspace/workspacePanelFocusRegistry';
import userEvent from '@testing-library/user-event';
import { MediaCategory, type MediaFile } from '../../services/mediaService';
import { chat } from '../../../wailsjs/go/models';

const updateMessageMock = vi.fn();
const updateMessagePinnedMock = vi.fn();
const deleteMessageMock = vi.hoisted(() => vi.fn());
const prepareChatMessageCommandMock = vi.hoisted(() => vi.fn());
const commitChatMessageCommandMock = vi.hoisted(() => vi.fn());
const showMenuMock = vi.fn();
const hideMenuMock = vi.fn();
const copyMessageMock = vi.fn();
const speakMessageMock = vi.fn();
const conversationId = '01926b90-7a5a-7c4e-8d3f-000000000001';
const voiceMessage = new chat.EnrichedMessage({
  id: '01926b90-7a5a-7c4e-8d3f-00000000000b', conversationId, role: 'assistant', content: 'Olá',
});
type MockThreadedMessage = {
  id?: string;
  message?: { id: string; role?: string; isStreaming?: boolean; turnId?: string; content?: string };
  children?: MockThreadedMessage[];
  level?: number;
  childCount?: number;
  originalIndex?: number;
};

const activeConversation: { id: string; title: string; threadedMessages: MockThreadedMessage[] } = {
  id: conversationId,
  title: 'Conversa',
  threadedMessages: [],
};

const modalState = vi.hoisted(() => ({ open: false }));
const contextMenuState = vi.hoisted(() => ({ visible: true, realHook: false }));
const runtimeEventHandlers = vi.hoisted(() => new Map<string, (data: unknown) => void>());
const messageListRenderMock = vi.hoisted(() => vi.fn());
const translateMock = vi.hoisted(() => (key: string, options?: { start?: number; end?: number; total?: number }) => (
  options?.total !== undefined ? `${key}:${options.start}-${options.end}-${options.total}` : key
));
const handleErrorMock = vi.hoisted(() => vi.fn());
const requestConfirmMock = vi.hoisted(() => vi.fn());
const executeDeepLinkMock = vi.hoisted(() => vi.fn());
const navigateMock = vi.hoisted(() => vi.fn());

vi.mock('react-router-dom', () => ({
  useLocation: () => ({ pathname: '/' }),
  useNavigate: () => navigateMock,
}));

vi.mock('../../store/confirmStore', () => ({
  requestConfirm: (...args: unknown[]) => requestConfirmMock(...args),
}));

vi.mock('../../lib/deepLinks', () => ({
  executeDeepLink: (...args: unknown[]) => executeDeepLinkMock(...args),
}));

vi.mock('../ui/Modal', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../ui/Modal')>();
  return { ...actual, isModalOpen: () => modalState.open };
});

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: translateMock,
  }),
}));

vi.mock('../../services/tts', () => ({
  ttsService: {
    isEnabled: () => false,
    hasVoiceConfig: () => false,
    on: vi.fn(),
    off: vi.fn(),
  },
}));

const chatStoreState = {
  getConversationMessages: () => [voiceMessage],
  getDraftRevision: () => 0,
  getMessagingPipelineRevision: () => 0,
  waitForMessagingAdmission: async () => {},
  cancelStreaming: vi.fn(),
  retryMessageToConversation: vi.fn(),
  ensureConversationSurfaceSession: vi.fn(),
  removeConversationSurfaceSession: vi.fn(),
  sessionsByConversationId: {
    [conversationId]: {
      conversation: activeConversation,
      isLoading: false,
      hasOlderMessages: false,
      isLoadingOlderMessages: false,
    },
  },
  timelinesByConversationId: {},
  surfaceSessionsByKey: {} as Record<string, ReturnType<typeof createEmptyChatSurfaceSession>>,
  loadMessageChildren: vi.fn(),
  loadConversationSession: vi.fn(),
  updateConversationMessage: updateMessageMock,
  updateConversationMessagePinned: updateMessagePinnedMock,
  toggleConversationReasoningExpanded: vi.fn(),
  isConversationReasoningExpanded: () => false,
  startConversationEditing: vi.fn(),
  setConversationScrollState: vi.fn(),
  loadOlderMessagesForConversation: vi.fn(),
  loadNewerMessagesForConversation: vi.fn(),
  loadBoundaryMessagesForConversation: vi.fn(),
  setConversationDraftMessage: vi.fn(),
  setConversationDraftMediaFiles: vi.fn(),
  clearConversationDraft: vi.fn(),
  clearConversationSendFailure: vi.fn(),
  setConversationEditingMessageId: vi.fn(),
  toggleConversationThreadExpanded: vi.fn(),
};

vi.mock('../../store/chatStore', () => ({
  useChatStore: Object.assign((selector?: (s: typeof chatStoreState) => unknown) => {
    if (typeof selector === 'function') {
      return selector(chatStoreState);
    }
    return chatStoreState;
  }, { getState: () => chatStoreState, subscribe: () => () => {} }),
}));

vi.mock('../../store/editorStore', () => ({
  useEditorStore: {
    getState: () => ({
      activeTabId: 'chat-tab',
      tabs: [{ id: 'chat-tab', conversationId: '01926b90-7a5a-7c4e-8d3f-00000000000a' }],
    }),
  },
}));

vi.mock('../../hooks/useChatKeyboardNav', () => ({
  useChatKeyboardNav: () => {},
}));

vi.mock('../../hooks/useContextMenu', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../hooks/useContextMenu')>();
  return {
  useContextMenu: (options: Parameters<typeof actual.useContextMenu>[0]) => {
    const realMenu = actual.useContextMenu(options);
    return {
      menuVisible: contextMenuState.visible,
      menuPosition: { x: 1, y: 2 },
      menuItems: [{ id: 'copy', label: 'Copiar' }],
      showMenu: contextMenuState.realHook ? realMenu.showMenu : showMenuMock,
      hideMenu: hideMenuMock,
    };
  },
  useMessageActions: () => ({
    copyMessage: copyMessageMock,
    speakMessage: speakMessageMock,
  }),
};
});

vi.mock('@wailsjs/go/wailsapi/Editor', () => ({
  EditorGetDraftPath: vi.fn().mockResolvedValue(''),
}));

vi.mock('@wailsjs/go/wailsapi/Conversations', () => ({
  DeleteMessage: (...args: unknown[]) => deleteMessageMock(...args),
  ToggleMessagePin: vi.fn(),
}));

vi.mock('../../lib/commandChatMessageWails', () => ({
  prepareChatMessageCommand: (...args: unknown[]) => prepareChatMessageCommandMock(...args),
  commitChatMessageCommand: (...args: unknown[]) => commitChatMessageCommandMock(...args),
}));

vi.mock('@wailsjs/go/wailsapi/ACPWorkDir', () => ({
  // A conversa destes testes não fala com agente de código: não há
  // diretório de agente a mostrar.
  GetAgentConversationWorkDir: vi.fn().mockRejectedValue(new Error('sem agente')),
  SetAgentConversationWorkDir: vi.fn(),
}));

vi.mock('@wailsjs/go/wailsapi/ACPCommands', () => ({
  // O menu da barra só tem as skills do app.
  GetAgentSessionCommands: vi.fn().mockResolvedValue({ conversationId: '', commands: [] }),
}));

vi.mock('@wailsjs/go/wailsapi/Profiles', () => ({
  GetActiveProfile: vi.fn().mockResolvedValue({
    chat: { streaming_recovery_show_continue: true },
  }),
  GetActiveProfileSlug: vi.fn().mockResolvedValue('padrao'),
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (event: string, handler: (data: unknown) => void) => {
    runtimeEventHandlers.set(event, handler);
    return () => runtimeEventHandlers.delete(event);
  },
}));

vi.mock('./ChatToolbar', () => ({
  ChatToolbar: ({ inputRef }: { inputRef?: React.RefObject<HTMLTextAreaElement> }) => (
    <div>Toolbar {inputRef ? 'ok' : 'no-ref'}</div>
  ),
}));

vi.mock('./MessageList', async () => {
  const React = await import('react');
  return {
    MessageList: React.forwardRef<HTMLDivElement, {
      onContextMenu?: (event: MouseEvent, message: { id: string; role: string }) => void;
      onReachEnd?: () => void;
      onDelete?: (message: { id: string }) => Promise<void>;
      threadedMessages?: Array<{ id?: string; message?: { id: string; role?: string; isStreaming?: boolean; turnId?: string; content?: string } }>;
      shouldShowContinue?: (message: { id: string; role?: string; isStreaming?: boolean; turnId?: string; content?: string }) => boolean;
      onSpeak?: (message: { id: string; role: string; content: string }) => void;
      onJumpToStart?: () => Promise<void> | void;
      onJumpToEnd?: () => Promise<void> | void;
      onLoadNewer?: (trigger: 'scroll' | 'navigation') => Promise<void> | void;
      isLoading?: boolean;
      origin?: unknown;
    }>((
    {
      onContextMenu,
      onReachEnd,
      onDelete,
      threadedMessages = [],
      shouldShowContinue,
      onJumpToStart,
      onJumpToEnd,
      onLoadNewer,
      onSpeak,
      isLoading,
      origin,
    },
    ref: React.Ref<HTMLDivElement>,
  ) => {
    messageListRenderMock({ onContextMenu, onReachEnd, onDelete, threadedMessages, isLoading, origin });
    return (
    <div ref={ref} data-testid="message-list">
      <div
        role="list"
        tabIndex={0}
        onKeyDown={(event) => {
          if (event.ctrlKey && event.key === 'Home') {
            event.preventDefault();
            void Promise.resolve(onJumpToStart?.());
          }
          if (event.ctrlKey && event.key === 'End') {
            event.preventDefault();
            void Promise.resolve(onJumpToEnd?.());
          }
        }}
      >
      <button
        type="button"
        onClick={() => onContextMenu?.(new MouseEvent('contextmenu'), { id: 'm1', role: 'user' })}
      >
        open-menu
      </button>
      <button type="button" onClick={() => void Promise.resolve(onLoadNewer?.('scroll'))}>
        load-newer-scroll
      </button>
      <button
        type="button"
        onClick={() => onSpeak?.(voiceMessage)}
      >
        speak-message
      </button>
      {threadedMessages.map((message) => (
        <div
          key={message.message?.id ?? message.id}
          className="message-node"
          tabIndex={-1}
          data-message-node
          data-level="0"
          data-message-id={message.message?.id ?? message.id}
          data-show-continue={String(shouldShowContinue?.(message.message ?? { id: String(message.id || '') }) ?? false)}
        >
          {message.message?.id ?? message.id}
        </div>
      ))}
      </div>
    </div>
    );
  }),
  };
});

vi.mock('./ChatInput', async () => {
  const React = await import('react');
  return {
    ChatInput: React.forwardRef<HTMLTextAreaElement, { onSend: (value: string) => void; disabled?: boolean; onArrowUp?: () => boolean }>(
      ({ onSend, disabled, onArrowUp }, ref) => (
        <button
          ref={ref as React.RefObject<HTMLButtonElement>}
          type="button"
          disabled={disabled}
          onKeyDown={(event) => {
            if (event.key === 'ArrowUp' && onArrowUp?.()) event.preventDefault();
          }}
          onClick={() => onSend('oi')}
        >
          send
        </button>
      )
    ),
  };
});

vi.mock('../menu', () => ({
  ContextMenu: ({ visible, items }: { visible: boolean; items: Array<{ id: string; label: string }> }) => (
    <div>{visible ? items.map((item) => item.label).join(',') : 'closed'}</div>
  ),
}));

vi.mock('../ui/KeyboardShortcutsHelp', () => ({
  KeyboardShortcutsHelp: ({ isOpen }: { isOpen: boolean }) => <div>{isOpen ? 'help-open' : 'help-closed'}</div>,
}));

const announceRequestMock = vi.hoisted(() => vi.fn(() => true));

vi.mock('../../hooks/useAnnouncer', () => ({
  announce: vi.fn(),
  useAnnouncer: () => ({ announce: vi.fn(), announceRequest: announceRequestMock }),
}));

vi.mock('../../utils/errorHandler', () => ({
  ErrorSeverity: { RECOVERABLE: 'recoverable' },
  ErrorMessages: { CHAT: { SEND_FAILED: 'Falha ao enviar', DELETE_FAILED: 'Falha ao deletar' } },
  handleError: handleErrorMock,
}));

import { ChatSessionView } from './ChatSessionView';
import { createEmptyChatSurfaceSession, type ChatSurfaceIdentity } from '../../services/chatSessionRegistry';
import { WorkspacePanelProvider } from '../workspace/WorkspacePanelContext';
import { announce } from '../../hooks/useAnnouncer';
import { useShortcutsHelpStore } from '../../store/shortcutsHelpStore';
import { useAuthStore } from '../../store/authStore';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { CHAT_MESSAGING_COMMAND_EVENT, executeChatMessaging, type ChatMessagingRequest } from '../../lib/commandChatMessaging';
import { CHAT_NAVIGATION_COMMAND_EVENT, captureChatNavigationTarget, type ChatNavigationRequest } from '../../lib/commandChatNavigation';
function handleNavigationRequest(event: Event) {
  const { commandID, instanceId } = (event as CustomEvent<ChatNavigationRequest>).detail;
  const target = captureChatNavigationTarget(() => '/', commandID, instanceId);
  if (!target) return;
  try { if (target.open(commandID)) event.preventDefault(); } finally { target.dispose(); }
}
const commandStatuses: string[] = [];
const commandPort = {
  beginUICommand: vi.fn(async (commandId: string) => ({ ticket: 'ticket', invocationId: 'invocation', commandId })),
  takeUICommand: vi.fn(async () => ({ ticket: 'ticket', invocationId: 'invocation', commandId: commandPort.beginUICommand.mock.lastCall?.[0] ?? 'chat.message.send', handoffId: 'handoff' })),
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

const panelTab = {
  id: 'chat-tab',
  type: 'chat' as const,
  title: 'Chat',
  position: 0,
  conversationId,
};

const surface = (overrides: Partial<ChatSurfaceIdentity> = {}): ChatSurfaceIdentity => {
  const surfaceId = overrides.surfaceId ?? `page:tab:${panelTab.id}`;
  const targetConversationId = overrides.conversationId !== undefined
    ? overrides.conversationId
    : conversationId;
  return {
    conversationId: targetConversationId,
    sessionKey: overrides.sessionKey ?? `${surfaceId}:${targetConversationId ?? 'none'}`,
    surfaceId,
    surfaceType: overrides.surfaceType ?? 'page',
    tabId: overrides.tabId ?? panelTab.id,
  };
};

function renderWithPanel(ui: React.ReactElement) {
  return render(
    <WorkspacePanelProvider value={{ tab: panelTab, isActive: true }}>
      {ui}
    </WorkspacePanelProvider>,
  );
}

describe('ChatSessionView', () => {
  beforeEach(() => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    useAuthStore.setState({ isAuthenticated: true, user: { userId: 'owner', sessionId: 'session', role: 'admin' } });
    useWorkspaceStore.setState({ workspace: { id: 'workspace', name: 'Workspace', activeTabId: panelTab.id, tabs: [panelTab] } });
    commandStatuses.length = 0;
    commandPort.beginUICommand.mockClear();
    commandPort.getUICommandResult.mockReset().mockResolvedValue({ invocationId: 'invocation', status: 'succeeded' });
    window.addEventListener(CHAT_MESSAGING_COMMAND_EVENT, handleMessagingRequest);
    window.addEventListener(CHAT_NAVIGATION_COMMAND_EVENT, handleNavigationRequest);
    chatStoreState.ensureConversationSurfaceSession.mockImplementation((id: string, key: string) => {
      chatStoreState.surfaceSessionsByKey[key] ??= { ...createEmptyChatSurfaceSession(id, key), draftMessage: 'oi' };
    });
    showMenuMock.mockReset();
    hideMenuMock.mockReset();
    chatStoreState.setConversationScrollState.mockReset();
    chatStoreState.loadBoundaryMessagesForConversation.mockReset();
    deleteMessageMock.mockReset();
    prepareChatMessageCommandMock.mockReset().mockResolvedValue(undefined);
    commitChatMessageCommandMock.mockReset().mockResolvedValue(undefined);
    chatStoreState.sessionsByConversationId[conversationId].isLoading = false;
    chatStoreState.sessionsByConversationId[conversationId].conversation = activeConversation;
    chatStoreState.sessionsByConversationId[conversationId].hasOlderMessages = false;
    chatStoreState.sessionsByConversationId[conversationId].isLoadingOlderMessages = false;
    (chatStoreState.sessionsByConversationId[conversationId] as typeof chatStoreState.sessionsByConversationId[typeof conversationId] & { sendFailureMessage?: string | null; sendFailureRetryable?: boolean }).sendFailureMessage = null;
    (chatStoreState.sessionsByConversationId[conversationId] as typeof chatStoreState.sessionsByConversationId[typeof conversationId] & { sendFailureAnnounced?: boolean }).sendFailureAnnounced = false;
    (chatStoreState.sessionsByConversationId[conversationId] as typeof chatStoreState.sessionsByConversationId[typeof conversationId] & { sendFailureRetryable?: boolean; sendFailureRetryContent?: string | null; sendFailureRetryMediaFiles?: unknown[] }).sendFailureRetryable = false;
    (chatStoreState.sessionsByConversationId[conversationId] as typeof chatStoreState.sessionsByConversationId[typeof conversationId] & { sendFailureRetryContent?: string | null }).sendFailureRetryContent = null;
    (chatStoreState.sessionsByConversationId[conversationId] as typeof chatStoreState.sessionsByConversationId[typeof conversationId] & { sendFailureRetryMediaFiles?: unknown[] }).sendFailureRetryMediaFiles = [];
    (activeConversation.threadedMessages as unknown[]) = [];
    messageListRenderMock.mockClear();
    (announce as ReturnType<typeof vi.fn>).mockReset();
    announceRequestMock.mockClear();
    chatStoreState.cancelStreaming.mockReset();
    chatStoreState.clearConversationSendFailure.mockReset();
    chatStoreState.clearConversationDraft.mockClear();
    chatStoreState.surfaceSessionsByKey = {};
    handleErrorMock.mockReset();
    modalState.open = false;
    contextMenuState.visible = true;
    contextMenuState.realHook = false;
    runtimeEventHandlers.clear();
    requestConfirmMock.mockReset();
    requestConfirmMock.mockResolvedValue(false);
    executeDeepLinkMock.mockReset();
    executeDeepLinkMock.mockResolvedValue(undefined);
    navigateMock.mockReset();
    useShortcutsHelpStore.setState({ isOpen: false });
  });

  afterEach(() => {
    window.removeEventListener(CHAT_MESSAGING_COMMAND_EVENT, handleMessagingRequest);
    window.removeEventListener(CHAT_NAVIGATION_COMMAND_EVENT, handleNavigationRequest);
    vi.mocked(document.hasFocus).mockRestore();
  });

  it('registra handler de foco de painel (variant page) e foca o input ao ser solicitado', async () => {
    render(
      <WorkspacePanelProvider value={{ tab: panelTab, isActive: true }}>
        <ChatSessionView variant="page" surface={surface()} onSend={vi.fn()} showShortcutsHelp={false} />
      </WorkspacePanelProvider>,
    );

    const input = await screen.findByRole('button', { name: 'send' });
    input.blur();
    expect(input).not.toHaveFocus();

    // O WorkspaceLayout roteia o foco via registry; o handler foca o input.
    act(() => {
      requestWorkspacePanelFocus('chat-tab');
    });

    await waitFor(() => expect(input).toHaveFocus());
  });

  it('expõe capability imediata real e recusa foco com modal ou painel inativo', async () => {
    const { rerender } = render(
      <WorkspacePanelProvider value={{ tab: panelTab, isActive: true }}>
        <ChatSessionView variant="page" surface={surface()} onSend={vi.fn()} showShortcutsHelp={false} />
      </WorkspacePanelProvider>,
    );
    const input = await screen.findByRole('button', { name: 'send' });
    const immediate = getWorkspacePanelImmediateFocusHandler('chat-tab');
    expect(immediate).toBeDefined();
    expect(canFocusWorkspacePanelImmediately('chat-tab')).toBe(true);

    input.blur();
    expect(immediate?.()).toBe(true);
    expect(document.activeElement).toBe(input);

    modalState.open = true;
    expect(canFocusWorkspacePanelImmediately('chat-tab')).toBe(false);
    expect(immediate?.()).toBe(false);

    modalState.open = false;
    rerender(
      <WorkspacePanelProvider value={{ tab: panelTab, isActive: false }}>
        <ChatSessionView variant="page" surface={surface()} onSend={vi.fn()} showShortcutsHelp={false} />
      </WorkspacePanelProvider>,
    );
    expect(canFocusWorkspacePanelImmediately('chat-tab')).toBe(false);
    expect(immediate?.()).toBe(false);
  });

  it('não rouba o foco de uma mensagem ao rotear foco do painel (retorno de menu)', async () => {
    const { container } = render(
      <WorkspacePanelProvider value={{ tab: panelTab, isActive: true }}>
        <ChatSessionView variant="page" surface={surface()} onSend={vi.fn()} showShortcutsHelp={false} />
      </WorkspacePanelProvider>,
    );

    const input = await screen.findByRole('button', { name: 'send' });
    // Simula uma mensagem focada (ex.: o `.message-node` ao qual o menu de
    // contexto restaura o foco depois do Escape).
    const root = container.querySelector('.chat-session-view') as HTMLElement;
    const messageNode = document.createElement('button');
    messageNode.className = 'message-node';
    messageNode.setAttribute('data-level', '0');
    root.appendChild(messageNode);
    messageNode.focus();
    expect(messageNode).toHaveFocus();

    act(() => {
      requestWorkspacePanelFocus('chat-tab');
    });
    await new Promise((resolve) => requestAnimationFrame(() => resolve(null)));

    // O roteamento de painel não sobrepõe a restauração intencional de foco.
    expect(messageNode).toHaveFocus();
    expect(input).not.toHaveFocus();
  });

  it('ArrowUp mantém o foco no input de conversa vazia', async () => {
    activeConversation.threadedMessages = [];
    renderWithPanel(
      <ChatSessionView variant="page" surface={surface()} onSend={vi.fn()} showShortcutsHelp={false} />,
    );

    const input = await screen.findByRole('button', { name: 'send' });
    input.focus();
    const event = new KeyboardEvent('keydown', { key: 'ArrowUp', bubbles: true, cancelable: true });
    input.dispatchEvent(event);

    expect(event.defaultPrevented).toBe(false);
    expect(input).toHaveFocus();
  });

  it('ArrowUp entra na lista com mensagens e, após limpar, permanece no mesmo input', async () => {
    activeConversation.threadedMessages = [{
      message: { id: 'message-before-clear', role: 'assistant', content: 'Conteúdo' },
      children: [],
      level: 0,
    }];
    const view = renderWithPanel(
      <ChatSessionView variant="page" surface={surface()} onSend={vi.fn()} showShortcutsHelp={false} />,
    );

    const input = await screen.findByRole('button', { name: 'send' });
    input.focus();
    const handledEvent = new KeyboardEvent('keydown', { key: 'ArrowUp', bubbles: true, cancelable: true });
    input.dispatchEvent(handledEvent);
    expect(handledEvent.defaultPrevented).toBe(true);
    expect(view.container.querySelector('[data-message-id="message-before-clear"]')).toHaveFocus();

    activeConversation.threadedMessages = [];
    view.rerender(
      <WorkspacePanelProvider value={{ tab: panelTab, isActive: true }}>
        <ChatSessionView variant="page" surface={surface()} onSend={vi.fn()} showShortcutsHelp={false} />
      </WorkspacePanelProvider>,
    );
    input.focus();
    const emptyEvent = new KeyboardEvent('keydown', { key: 'ArrowUp', bubbles: true, cancelable: true });
    input.dispatchEvent(emptyEvent);

    expect(emptyEvent.defaultPrevented).toBe(false);
    expect(input).toHaveFocus();
    expect(view.container.querySelector('[data-message-id="message-before-clear"]')).toBeNull();
  });

  it('não registra handler de foco de painel na variante embedded', () => {
    render(
      <WorkspacePanelProvider value={{ tab: panelTab, isActive: true }}>
        <ChatSessionView variant="embedded" surface={surface({ surfaceType: 'embedded' })} onSend={vi.fn()} showShortcutsHelp={false} />
      </WorkspacePanelProvider>,
    );

    // A aba hospedeira (ex.: tasklist/editor) é quem registra o foco do painel.
    // O chat embutido não pode sequestrar o tabId da aba hospedeira.
    expect(hasWorkspacePanelFocusHandler('chat-tab')).toBe(false);
  });

  it('? abre o painel de atalhos quando nenhum modal está aberto', () => {
    renderWithPanel(
      <ChatSessionView variant="embedded" surface={surface({ surfaceType: 'embedded' })} onSend={vi.fn()} showShortcutsHelp />,
    );

    const event = new KeyboardEvent('keypress', { key: '?', bubbles: true, cancelable: true });
    document.dispatchEvent(event);

    expect(event.defaultPrevented).toBe(true);
    expect(useShortcutsHelpStore.getState().isOpen).toBe(true);
  });

  it('anuncia skill carregada para a conversa atual', async () => {
    renderWithPanel(
      <ChatSessionView variant="embedded" surface={surface({ surfaceType: 'embedded' })} onSend={vi.fn()} showShortcutsHelp={false} />,
    );

    await waitFor(() => {
      expect(runtimeEventHandlers.has('chat:skill_loaded')).toBe(true);
    });

    runtimeEventHandlers.get('chat:skill_loaded')?.({
      conversationId,
      slug: 'review',
      displayName: 'Review',
    });

    expect(announce).toHaveBeenCalledWith('chat.announce.skillLoaded');
  });

  it('? NÃO abre o painel (nem chama preventDefault) quando outro modal está aberto', () => {
    modalState.open = true;
    renderWithPanel(
      <ChatSessionView variant="embedded" surface={surface({ surfaceType: 'embedded' })} onSend={vi.fn()} showShortcutsHelp />,
    );

    const event = new KeyboardEvent('keypress', { key: '?', bubbles: true, cancelable: true });
    document.dispatchEvent(event);

    expect(event.defaultPrevented).toBe(false);
    expect(useShortcutsHelpStore.getState().isOpen).toBe(false);
  });

  // O listener global de Escape só é registrado enquanto há streaming
  // (isLoading), que no modelo de sessão vem do surfaceSession da superfície.
  const enableStreamingSurface = () => {
    const escSurface = surface({ surfaceType: 'embedded' });
    (chatStoreState.surfaceSessionsByKey as Record<string, ReturnType<typeof createEmptyChatSurfaceSession>>)[escSurface.sessionKey] = {
      ...createEmptyChatSurfaceSession(conversationId, escSurface.sessionKey),
      isLoading: true,
    };
    return escSurface;
  };

  it('Escape fora do campo de edição (streaming ativo) foca o input e NÃO cancela a geração', () => {
    contextMenuState.visible = false;
    const escSurface = enableStreamingSurface();
    renderWithPanel(
      <ChatSessionView variant="embedded" surface={escSurface} onSend={vi.fn()} showShortcutsHelp={false} />,
    );

    const input = screen.getByRole('button', { name: 'send' });
    // No uso real o Escape parte do elemento focado. Focamos um elemento focável
    // da lista (fora do input) para reproduzir fielmente o cenário.
    const outside = screen.getByRole('list');
    outside.focus();
    expect(document.activeElement).toBe(outside);
    fireEvent.keyDown(outside, { key: 'Escape' });

    expect(document.activeElement).toBe(input);
    expect(chatStoreState.cancelStreaming).not.toHaveBeenCalled();
  });

  it('Escape com foco no campo de edição não é interceptado pelo listener global (não chama preventDefault nem move o foco)', () => {
    contextMenuState.visible = false;
    const escSurface = enableStreamingSurface();
    renderWithPanel(
      <ChatSessionView variant="embedded" surface={escSurface} onSend={vi.fn()} showShortcutsHelp={false} />,
    );

    const input = screen.getByRole('button', { name: 'send' });
    // Garante que o foco está de fato no input antes do Escape, reproduzindo o
    // cenário em que o usuário está editando. Este teste verifica apenas que o
    // listener GLOBAL não age (não chama preventDefault nem move o foco),
    // deixando o Escape livre para o ChatInput. O cancelamento em si é
    // responsabilidade do ChatInput (mockado aqui como um simples <button> sem
    // handler de Escape) e é coberto pelos testes do próprio ChatInput.
    input.focus();
    expect(document.activeElement).toBe(input);
    const event = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true });
    input.dispatchEvent(event);

    expect(event.defaultPrevented).toBe(false);
    expect(document.activeElement).toBe(input);
    expect(chatStoreState.cancelStreaming).not.toHaveBeenCalled();
  });

  it.each([{ repeat: true }, { isComposing: true }, { keyCode: 229 }])('Escape inválido %j não restaura input nem cancela geração', flags => {
    contextMenuState.visible = false;
    renderWithPanel(<ChatSessionView variant="embedded" surface={enableStreamingSurface()} onSend={vi.fn()} showShortcutsHelp={false} />);
    const list = screen.getByRole('list'); list.focus();
    fireEvent.keyDown(list, { key: 'Escape', ...flags });
    expect(list).toHaveFocus(); expect(chatStoreState.cancelStreaming).not.toHaveBeenCalled();
    expect(commandPort.beginUICommand).not.toHaveBeenCalled();
  });

  it('Escape originado FORA do painel do chat não rouba o foco para o input (streaming ativo)', () => {
    contextMenuState.visible = false;
    const escSurface = enableStreamingSurface();
    renderWithPanel(
      <ChatSessionView variant="embedded" surface={escSurface} onSend={vi.fn()} showShortcutsHelp={false} />,
    );

    const input = screen.getByRole('button', { name: 'send' });
    // Simula o foco em outra superfície/painel do app (ex.: terminal, editor,
    // task list), fora do container do chat. O listener global do chat NÃO deve
    // devolver o foco ao input do chat — esse roteamento cabe ao sistema central
    // de landmarks, que respeita o painel ativo (Issue #202 / AEP-0058).
    const externalArea = document.createElement('button');
    externalArea.textContent = 'outro-painel';
    document.body.appendChild(externalArea);
    try {
      externalArea.focus();
      expect(document.activeElement).toBe(externalArea);
      fireEvent.keyDown(externalArea, { key: 'Escape' });

      expect(document.activeElement).toBe(externalArea);
      expect(document.activeElement).not.toBe(input);
      expect(chatStoreState.cancelStreaming).not.toHaveBeenCalled();
    } finally {
      document.body.removeChild(externalArea);
    }
  });

  it('Escape com modal aberto não cancela a geração nem mexe na UI de fundo', () => {
    contextMenuState.visible = false;
    const escSurface = enableStreamingSurface();
    modalState.open = true;
    renderWithPanel(
      <ChatSessionView variant="embedded" surface={escSurface} onSend={vi.fn()} showShortcutsHelp={false} />,
    );

    const input = screen.getByRole('button', { name: 'send' });
    // Com modal aberto, o Escape pertence ao modal: o listener global não pode
    // mexer no foco da UI de fundo. Focamos um elemento focável da lista e
    // verificamos que o foco permanece nele após o Escape.
    const outside = screen.getByRole('list');
    outside.focus();
    expect(document.activeElement).toBe(outside);
    fireEvent.keyDown(outside, { key: 'Escape' });

    expect(chatStoreState.cancelStreaming).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(outside);
    expect(document.activeElement).not.toBe(input);
  });

  it('Escape com menu de contexto aberto fecha o menu e não cancela a geração', () => {
    contextMenuState.visible = true;
    const escSurface = enableStreamingSurface();
    renderWithPanel(
      <ChatSessionView variant="embedded" surface={escSurface} onSend={vi.fn()} showShortcutsHelp={false} />,
    );

    // No uso real o Escape parte do elemento focado. Focamos um elemento focável
    // da lista para reproduzir fielmente o cenário: o handler fecha o menu e o
    // foco permanece no elemento de origem (não vai para o input).
    const outside = screen.getByRole('list');
    outside.focus();
    expect(document.activeElement).toBe(outside);
    fireEvent.keyDown(outside, { key: 'Escape' });

    expect(hideMenuMock).toHaveBeenCalled();
    expect(chatStoreState.cancelStreaming).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(outside);
  });

  it('embedded: aciona menu de contexto via MessageList', async () => {
    const onSend = vi.fn();
    renderWithPanel(<ChatSessionView variant="embedded" surface={surface({ surfaceType: 'embedded' })} onSend={onSend} showShortcutsHelp={false} />);

    await userEvent.click(screen.getByRole('button', { name: 'open-menu' }));

    expect(showMenuMock).toHaveBeenCalled();
    expect(screen.getByText('Copiar')).toBeInTheDocument();
  });

  it('oferece configurar voz e preserva a origem ao confirmar', async () => {
    requestConfirmMock.mockResolvedValueOnce(true);
    const chatSurface = surface({ surfaceType: 'page', tabId: 'chat-tab' });
    renderWithPanel(
      <ChatSessionView
        surface={chatSurface}
        onSend={vi.fn()}
        showShortcutsHelp={false}
        profileSlug="programacao"
      />,
    );

    await userEvent.click(screen.getByRole('button', { name: 'speak-message' }));

    await waitFor(() => {
      expect(requestConfirmMock).toHaveBeenCalledWith(expect.objectContaining({
        title: 'chat.voiceSetup.title',
      }));
    });
    expect(executeDeepLinkMock).toHaveBeenCalledWith(
      {
        type: 'resource:edit',
        resource: 'profiles',
        resourceId: 'programacao',
        tab: 'voice',
      },
      {
        navigate: navigateMock,
        caller: {
          kind: 'workspace',
          tabId: 'chat-tab',
          surfaceId: chatSurface.surfaceId,
          surfaceType: chatSurface.surfaceType,
          conversationId,
        },
      },
    );
    expect(speakMessageMock).not.toHaveBeenCalled();
  });

  it('preserva caller ao configurar voz em superfície embedded', async () => {
    requestConfirmMock.mockResolvedValueOnce(true);
    const chatSurface = surface({
      surfaceId: 'embedded:editor:chat-tab',
      surfaceType: 'embedded',
      tabId: 'chat-tab',
    });
    renderWithPanel(
      <ChatSessionView
        variant="embedded"
        surface={chatSurface}
        onSend={vi.fn()}
        showShortcutsHelp={false}
        profileSlug="programacao"
      />,
    );

    await userEvent.click(screen.getByRole('button', { name: 'speak-message' }));

    await waitFor(() => {
      expect(executeDeepLinkMock).toHaveBeenCalledWith(
        expect.objectContaining({ type: 'resource:edit', tab: 'voice' }),
        {
          navigate: navigateMock,
          caller: {
            kind: 'workspace',
            tabId: 'chat-tab',
            surfaceId: 'embedded:editor:chat-tab',
            surfaceType: 'embedded',
            conversationId,
          },
        },
      );
    });
  });

  it('informa erro quando não consegue abrir a configuração de voz', async () => {
    requestConfirmMock.mockResolvedValueOnce(true);
    executeDeepLinkMock.mockRejectedValueOnce(new Error('falha de navegação'));
    const chatSurface = surface({ surfaceType: 'page', tabId: 'chat-tab' });
    renderWithPanel(
      <ChatSessionView
        surface={chatSurface}
        onSend={vi.fn()}
        showShortcutsHelp={false}
        profileSlug="programacao"
      />,
    );

    await userEvent.click(screen.getByRole('button', { name: 'speak-message' }));

    await waitFor(() => {
      expect(handleErrorMock).toHaveBeenCalledWith(
        expect.any(Error),
        expect.objectContaining({
          source: 'ChatSessionView.voiceSetup',
          userMessage: 'chat.voiceSetup.error',
          severity: 'recoverable',
        }),
      );
    });
  });

  it('embedded: reconcilia erro de transporte sem oferecer retry de resultado desconhecido', async () => {
    const user = userEvent.setup();
    const onSend = vi.fn().mockRejectedValueOnce(new Error('fail'));
    commandPort.getUICommandResult.mockRejectedValueOnce(new Error('ledger unavailable'));
    const chatSurface = surface({ surfaceType: 'embedded' });
    renderWithPanel(<ChatSessionView variant="embedded" surface={chatSurface} onSend={onSend} showShortcutsHelp={false} />);

    await user.click(screen.getByRole('button', { name: 'send' }));

    await waitFor(() => expect(commandStatuses).toEqual(['outcome_unknown']));
    expect(onSend).toHaveBeenCalledTimes(1);
    expect(commandPort.beginUICommand).toHaveBeenCalledWith('chat.message.send');
    expect(commandPort.getUICommandResult).toHaveBeenCalledWith('ticket');
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'chat.retryAriaLabel' })).not.toBeInTheDocument();
    expect(chatStoreState.surfaceSessionsByKey[chatSurface.sessionKey].draftMessage).toBe('oi');
  });

  it('embedded: mostra banner de erro transitório da sessão sem retry local', async () => {
    const chatSurface = surface({ surfaceType: 'embedded' });
    (chatStoreState.surfaceSessionsByKey as Record<string, ReturnType<typeof createEmptyChatSurfaceSession>>)[chatSurface.sessionKey] = {
      ...createEmptyChatSurfaceSession(conversationId, chatSurface.sessionKey),
      sendFailureMessage: 'Falha ao enviar pela sessão',
      sendFailureRetryable: false,
      sendFailureRetryContent: null,
      sendFailureRetryMediaFiles: [],
    };
    renderWithPanel(<ChatSessionView variant="embedded" surface={chatSurface} onSend={vi.fn()} showShortcutsHelp={false} />);

    expect(await screen.findByText('Falha ao enviar pela sessão')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(announce).toHaveBeenCalledWith('Falha ao enviar pela sessão', 'assertive');
    expect(screen.queryByRole('button', { name: 'chat.retryAriaLabel' })).not.toBeInTheDocument();
  });

  it('embedded: não anuncia novamente falha que o controller já anunciou', async () => {
    const chatSurface = surface({ surfaceType: 'embedded' });
    (chatStoreState.surfaceSessionsByKey as Record<string, ReturnType<typeof createEmptyChatSurfaceSession>>)[chatSurface.sessionKey] = {
      ...createEmptyChatSurfaceSession(conversationId, chatSurface.sessionKey),
      sendFailureMessage: 'Falha já anunciada',
      sendFailureAnnounced: true,
      sendFailureRetryable: false,
      sendFailureRetryContent: null,
      sendFailureRetryMediaFiles: [],
    };
    renderWithPanel(<ChatSessionView variant="embedded" surface={chatSurface} onSend={vi.fn()} showShortcutsHelp={false} />);

    expect(await screen.findByText('Falha já anunciada')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(announce).not.toHaveBeenCalledWith('Falha já anunciada', 'assertive');
  });

  it('embedded: anuncia falha de sessão que aparece após o primeiro render', async () => {
    const chatSurface = surface({ surfaceType: 'embedded' });
    const { rerender } = renderWithPanel(
      <ChatSessionView variant="embedded" surface={chatSurface} onSend={vi.fn()} showShortcutsHelp={false} />,
    );

    expect(announce).not.toHaveBeenCalledWith('Falha hidratada da sessão', 'assertive');

    (chatStoreState.surfaceSessionsByKey as Record<string, ReturnType<typeof createEmptyChatSurfaceSession>>)[chatSurface.sessionKey] = {
      ...createEmptyChatSurfaceSession(conversationId, chatSurface.sessionKey),
      sendFailureMessage: 'Falha hidratada da sessão',
      sendFailureRetryable: false,
      sendFailureRetryContent: null,
      sendFailureRetryMediaFiles: [],
    };
    rerender(
      <WorkspacePanelProvider value={{ tab: panelTab, isActive: true }}>
        <ChatSessionView variant="embedded" surface={chatSurface} onSend={vi.fn()} showShortcutsHelp={false} />
      </WorkspacePanelProvider>,
    );

    expect(await screen.findByText('Falha hidratada da sessão')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(announce).toHaveBeenCalledWith('Falha hidratada da sessão', 'assertive');
  });

  it('page: anuncia falha de sessão mesmo com painel inativo', async () => {
    const chatSurface = surface({ surfaceType: 'page' });
    (chatStoreState.surfaceSessionsByKey as Record<string, ReturnType<typeof createEmptyChatSurfaceSession>>)[chatSurface.sessionKey] = {
      ...createEmptyChatSurfaceSession(conversationId, chatSurface.sessionKey),
      sendFailureMessage: 'Falha em aba inativa',
      sendFailureRetryable: false,
      sendFailureRetryContent: null,
      sendFailureRetryMediaFiles: [],
    };

    render(
      <WorkspacePanelProvider value={{ tab: panelTab, isActive: false }}>
        <ChatSessionView variant="page" surface={chatSurface} onSend={vi.fn()} showShortcutsHelp={false} />
      </WorkspacePanelProvider>,
    );

    expect(await screen.findByText('Falha em aba inativa')).toBeInTheDocument();
    expect(announce).toHaveBeenCalledWith('Falha em aba inativa', 'assertive');
  });

  it('embedded: Escape descarta e limpa falha persistida da sessão', async () => {
    const chatSurface = surface({ surfaceType: 'embedded' });
    (chatStoreState.surfaceSessionsByKey as Record<string, ReturnType<typeof createEmptyChatSurfaceSession>>)[chatSurface.sessionKey] = {
      ...createEmptyChatSurfaceSession(conversationId, chatSurface.sessionKey),
      sendFailureMessage: 'Falha ao enviar pela sessão',
      sendFailureRetryable: false,
      sendFailureRetryContent: null,
      sendFailureRetryMediaFiles: [],
    };
    renderWithPanel(<ChatSessionView variant="embedded" surface={chatSurface} onSend={vi.fn()} showShortcutsHelp={false} />);

    expect(await screen.findByText('Falha ao enviar pela sessão')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();

    fireEvent.keyDown(document, { key: 'Escape' });

    expect(chatStoreState.clearConversationSendFailure).toHaveBeenCalledWith(conversationId, chatSurface.sessionKey);
    await waitFor(() => {
      expect(screen.queryByText('Falha ao enviar pela sessão')).not.toBeInTheDocument();
    });
  });

  it('embedded: novo envio mostra falha de sessão repetida', async () => {
    const user = userEvent.setup();
    const chatSurface = surface({ surfaceType: 'embedded' });
    const onSend = vi.fn().mockImplementation(async () => {
      (chatStoreState.surfaceSessionsByKey as Record<string, ReturnType<typeof createEmptyChatSurfaceSession>>)[chatSurface.sessionKey] = {
        ...createEmptyChatSurfaceSession(conversationId, chatSurface.sessionKey),
        sendFailureMessage: 'Falha ao enviar pela sessão',
        sendFailureRetryable: false,
        sendFailureRetryContent: null,
        sendFailureRetryMediaFiles: [],
      };
    });
    (chatStoreState.surfaceSessionsByKey as Record<string, ReturnType<typeof createEmptyChatSurfaceSession>>)[chatSurface.sessionKey] = {
      ...createEmptyChatSurfaceSession(conversationId, chatSurface.sessionKey),
      sendFailureMessage: 'Falha ao enviar pela sessão',
      sendFailureRetryable: false,
      sendFailureRetryContent: null,
      sendFailureRetryMediaFiles: [],
    };
    const { rerender } = renderWithPanel(
      <ChatSessionView variant="embedded" surface={chatSurface} onSend={onSend} showShortcutsHelp={false} />,
    );

    expect(await screen.findByText('Falha ao enviar pela sessão')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'send' }));
    rerender(
      <WorkspacePanelProvider value={{ tab: panelTab, isActive: true }}>
        <ChatSessionView variant="embedded" surface={chatSurface} onSend={onSend} showShortcutsHelp={false} />
      </WorkspacePanelProvider>,
    );

    expect(await screen.findByText('Falha ao enviar pela sessão')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('embedded: permite retry de falha persistida com mídia sem texto', async () => {
    const user = userEvent.setup();
    const chatSurface = surface({ surfaceType: 'embedded' });
    const mediaFile: MediaFile = {
      id: 'media-1',
      file: new File(['conteudo'], 'imagem.png', { type: 'image/png' }),
      category: MediaCategory.IMAGE,
      mimeType: 'image/png',
      extension: 'png',
      fileName: 'imagem.png',
      fileSize: 8,
      fileSizeFormatted: '8 B',
      icon: 'image',
    };
    const onSend = vi.fn(async () => {
      // chatEventController clears persisted failure when the admitted pipeline starts.
      chatStoreState.surfaceSessionsByKey[chatSurface.sessionKey] = {
        ...chatStoreState.surfaceSessionsByKey[chatSurface.sessionKey],
        sendFailureMessage: null, sendFailureRetryable: false,
        sendFailureRetryContent: null, sendFailureRetryMediaFiles: [],
      };
    });
    (chatStoreState.surfaceSessionsByKey as Record<string, ReturnType<typeof createEmptyChatSurfaceSession>>)[chatSurface.sessionKey] = {
      ...createEmptyChatSurfaceSession(conversationId, chatSurface.sessionKey),
      sendFailureMessage: 'Falha ao enviar mídia',
      sendFailureRetryable: true,
      sendFailureRetryContent: null,
      sendFailureRetryMediaFiles: [mediaFile],
      draftMessage: 'rascunho novo',
    };
    const { rerender } = renderWithPanel(<ChatSessionView variant="embedded" surface={chatSurface} onSend={onSend} showShortcutsHelp={false} />);

    await user.click(await screen.findByRole('button', { name: 'chat.retryAriaLabel' }));

    await waitFor(() => expect(commandStatuses).toEqual(['succeeded']));
    expect(onSend).toHaveBeenCalledWith('', [mediaFile], chatSurface, expect.objectContaining({
      handoff: { ticket: 'ticket', handoffId: 'handoff' }, isCurrent: expect.any(Function),
    }));
    rerender(<WorkspacePanelProvider value={{ tab: panelTab, isActive: true }}>
      <ChatSessionView variant="embedded" surface={chatSurface} onSend={onSend} showShortcutsHelp={false} />
    </WorkspacePanelProvider>);
    expect(screen.queryByText('Falha ao enviar mídia')).not.toBeInTheDocument();
    expect(chatStoreState.clearConversationSendFailure).toHaveBeenCalledWith(conversationId, chatSurface.sessionKey);
    expect(screen.queryByRole('button', { name: 'chat.retryAriaLabel' })).not.toBeInTheDocument();
    expect(chatStoreState.clearConversationDraft).not.toHaveBeenCalled();
    expect(chatStoreState.surfaceSessionsByKey[chatSurface.sessionKey].draftMessage).toBe('rascunho novo');
  });

  it('embedded: mantém envio habilitado mesmo com isLoading global ativo', async () => {
    const user = userEvent.setup();
    const onSend = vi.fn().mockResolvedValue(undefined);
    chatStoreState.sessionsByConversationId[conversationId].isLoading = true;
    renderWithPanel(
      <ChatSessionView
        variant="embedded"
        surface={surface({
          surfaceId: 'embedded:workspace-chat-modal:tab-1',
          sessionKey: `embedded:workspace-chat-modal:tab-1:${conversationId}`,
          surfaceType: 'embedded',
        })}
        onSend={onSend}
        showShortcutsHelp={false}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'send' }));

    await waitFor(() => expect(commandStatuses).toEqual(['succeeded']));
    expect(onSend).toHaveBeenCalledWith('oi', undefined, expect.objectContaining({
      conversationId,
      sessionKey: `embedded:workspace-chat-modal:tab-1:${conversationId}`,
      surfaceId: 'embedded:workspace-chat-modal:tab-1',
      surfaceType: 'embedded',
    }), expect.objectContaining({ handoff: { ticket: 'ticket', handoffId: 'handoff' }, isCurrent: expect.any(Function) }));
  });

  it('preserva props estáveis da lista durante progresso e atualização de draft', async () => {
    contextMenuState.realHook = true;
    const chatSurface = surface({ surfaceType: 'embedded' });
    const surfaceSessions = chatStoreState.surfaceSessionsByKey as Record<string, ReturnType<typeof createEmptyChatSurfaceSession>>;
    surfaceSessions[chatSurface.sessionKey] = {
      ...createEmptyChatSurfaceSession(conversationId, chatSurface.sessionKey),
      isLoading: false,
      draftMessage: '',
    };
    const createView = () => (
      <WorkspacePanelProvider value={{ tab: panelTab, isActive: true }}>
        <ChatSessionView
          variant="embedded"
          surface={chatSurface}
          onSend={vi.fn()}
          showShortcutsHelp={false}
        />
      </WorkspacePanelProvider>
    );

    const { rerender } = render(createView());
    await waitFor(() => expect(messageListRenderMock).toHaveBeenCalled());
    const firstProps = messageListRenderMock.mock.calls[messageListRenderMock.mock.calls.length - 1]?.[0] as {
      onContextMenu?: unknown;
      onReachEnd?: unknown;
      onDelete?: unknown;
      origin?: unknown;
    };
    messageListRenderMock.mockClear();

    surfaceSessions[chatSurface.sessionKey] = {
      ...surfaceSessions[chatSurface.sessionKey],
      isLoading: true,
      draftMessage: 'rascunho digitado durante progresso',
    };
    rerender(createView());

    await waitFor(() => expect(messageListRenderMock).toHaveBeenCalled());
    const updatedProps = messageListRenderMock.mock.calls[messageListRenderMock.mock.calls.length - 1]?.[0] as {
      onContextMenu?: unknown;
      onReachEnd?: unknown;
      onDelete?: unknown;
      origin?: unknown;
      isLoading?: boolean;
    };
    expect(updatedProps.isLoading).toBe(true);
    expect(updatedProps.onContextMenu).toBe(firstProps.onContextMenu);
    expect(updatedProps.onReachEnd).toBe(firstProps.onReachEnd);
    expect(updatedProps.onDelete).toBe(firstProps.onDelete);
    expect(updatedProps.origin).toBe(firstProps.origin);
  });

  it('cancela exclusão preparada quando a superfície muda antes do commit', async () => {
    const nextId = '01926b90-7a5a-7c4e-8d3f-000000000002';
    const sessions = chatStoreState.sessionsByConversationId as Record<string, typeof chatStoreState.sessionsByConversationId[typeof conversationId]>;
    sessions[nextId] = {
      ...sessions[conversationId],
      conversation: { id: nextId, title: 'Outra conversa', threadedMessages: [] },
    };
    let finishPrepare!: () => void;
    prepareChatMessageCommandMock.mockImplementationOnce(() => new Promise<void>((resolve) => { finishPrepare = resolve; }));
    const view = (id: string) => (
      <WorkspacePanelProvider value={{ tab: panelTab, isActive: true }}>
        <ChatSessionView variant="embedded" surface={surface({ conversationId: id, surfaceType: 'embedded' })} onSend={vi.fn()} showShortcutsHelp={false} />
      </WorkspacePanelProvider>
    );
    const { rerender } = render(view(conversationId));
    await waitFor(() => expect(messageListRenderMock).toHaveBeenCalled());
    const props = messageListRenderMock.mock.lastCall![0] as { onDelete: (message: { id: string }) => Promise<void> };
    act(() => { void props.onDelete({ id: voiceMessage.id }); });
    await waitFor(() => expect(prepareChatMessageCommandMock).toHaveBeenCalledWith('ticket', voiceMessage.id));
    rerender(view(nextId));
    chatStoreState.loadConversationSession.mockClear();
    await act(async () => { finishPrepare(); });
    await waitFor(() => expect(commandPort.cancelUICommand).toHaveBeenCalled());
    expect(commitChatMessageCommandMock).not.toHaveBeenCalled();
    expect(chatStoreState.loadConversationSession).not.toHaveBeenCalledWith(conversationId, { refreshSurfaceWindows: true });
    expect(chatStoreState.loadConversationSession).not.toHaveBeenCalledWith(nextId, { refreshSurfaceWindows: true });
    delete sessions[nextId];
  });

  it('não mostra continuar resposta para mensagem com ID sintético', async () => {
    const syntheticMessage = {
      id: 'streaming-assistant-1',
      role: 'assistant',
      isStreaming: false,
      turnId: conversationId,
      content: 'resposta parcial',
    };
    chatStoreState.sessionsByConversationId[conversationId].conversation = {
      ...activeConversation,
      threadedMessages: [{ message: syntheticMessage, children: [], level: 0, childCount: 0 }],
    };
    (chatStoreState.sessionsByConversationId[conversationId] as typeof chatStoreState.sessionsByConversationId[typeof conversationId] & { lastInterruptedMessageId: string }).lastInterruptedMessageId = syntheticMessage.id;

    renderWithPanel(<ChatSessionView variant="embedded" surface={surface({ surfaceType: 'embedded' })} onSend={vi.fn()} showShortcutsHelp={false} />);

    await waitFor(() => {
      const node = screen.getByText(syntheticMessage.id);
      expect(node).toHaveAttribute('data-show-continue', 'false');
    });
  });

  it('restaura scroll pela âncora antes de usar scrollTop', async () => {
    const scrollIntoView = vi.fn();
    const originalScrollIntoView = HTMLElement.prototype.scrollIntoView;
    HTMLElement.prototype.scrollIntoView = scrollIntoView;
    const sessionKey = 'test-session';
    (activeConversation.threadedMessages as unknown[]) = [{ id: 'anchor-message' }];
    (chatStoreState.surfaceSessionsByKey as Record<string, ReturnType<typeof createEmptyChatSurfaceSession>>)[sessionKey] = {
      ...createEmptyChatSurfaceSession(conversationId, sessionKey),
      scrollTop: 320,
      scrollAnchorMessageId: 'anchor-message',
    };

    try {
      renderWithPanel(
        <ChatSessionView
          surface={surface({ sessionKey, surfaceId: 'test-surface' })}
          onSend={vi.fn().mockResolvedValue(undefined)}
          showShortcutsHelp={false}
        />,
      );

      await waitFor(() => {
        expect(scrollIntoView).toHaveBeenCalledWith({ block: 'start' });
      });

      expect(screen.getByTestId('message-list').scrollTop).not.toBe(320);
    } finally {
      HTMLElement.prototype.scrollIntoView = originalScrollIntoView;
    }
  });

  it('não reaplica fallback de scrollTop depois que âncora ausente já foi restaurada', async () => {
    const sessionKey = 'test-session';
    (chatStoreState.surfaceSessionsByKey as Record<string, ReturnType<typeof createEmptyChatSurfaceSession>>)[sessionKey] = {
      ...createEmptyChatSurfaceSession(conversationId, sessionKey),
      scrollTop: 320,
      scrollAnchorMessageId: 'missing-anchor',
    };

    const { rerender } = renderWithPanel(
      <ChatSessionView
        surface={surface({ sessionKey, surfaceId: 'test-surface' })}
        onSend={vi.fn().mockResolvedValue(undefined)}
        showShortcutsHelp={false}
      />,
    );

    const messageList = await screen.findByTestId('message-list');
    await waitFor(() => {
      expect(messageList.scrollTop).toBe(320);
    });

    messageList.scrollTop = 111;
    (activeConversation.threadedMessages as unknown[]) = [{ id: 'new-message' }];
    rerender(
      <WorkspacePanelProvider value={{ tab: panelTab, isActive: true }}>
        <ChatSessionView
          surface={surface({ sessionKey, surfaceId: 'test-surface' })}
          onSend={vi.fn().mockResolvedValue(undefined)}
          showShortcutsHelp={false}
        />
      </WorkspacePanelProvider>,
    );

    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(messageList.scrollTop).toBe(111);
  });

  it('navegação por Ctrl+Home/Ctrl+End carrega boundaries, anuncia janela e restaura foco', async () => {
    const sessionKey = 'boundary-session';
    let surfaceSession = {
      ...createEmptyChatSurfaceSession(conversationId, sessionKey),
      visibleThreadedMessages: [
        new chat.MessageNode({ message: { id: 'm1', role: 'user' }, children: [], level: 0, childCount: 0 }),
        new chat.MessageNode({ message: { id: 'm2', role: 'assistant' }, children: [], level: 0, childCount: 0 }),
      ],
      messageWindow: {
        scope: 'conversation' as const,
        conversationId,
        totalCount: 10,
        startIndex: 4,
        endIndex: 5,
        hasBefore: true,
        hasAfter: true,
      },
    };
    chatStoreState.surfaceSessionsByKey[sessionKey] = surfaceSession;
    chatStoreState.loadBoundaryMessagesForConversation.mockImplementation(async (_conversationId, _sessionKey, anchor) => {
      surfaceSession = {
        ...surfaceSession,
        messageWindow: anchor === 'start'
        ? {
          ...surfaceSession.messageWindow,
          startIndex: 0,
          endIndex: 1,
          hasBefore: false,
          hasAfter: true,
        }
        : {
          ...surfaceSession.messageWindow,
          startIndex: 8,
          endIndex: 9,
          hasBefore: true,
          hasAfter: false,
        },
      };
      chatStoreState.surfaceSessionsByKey[sessionKey] = surfaceSession;
    });

    const { rerender } = renderWithPanel(
      <ChatSessionView
        surface={surface({ sessionKey, surfaceId: 'boundary-surface' })}
        onSend={vi.fn().mockResolvedValue(undefined)}
        showShortcutsHelp={false}
      />,
    );

    const list = screen.getByRole('list');
    fireEvent.keyDown(list, { key: 'Home', ctrlKey: true });

    await waitFor(() => {
      expect(chatStoreState.loadBoundaryMessagesForConversation).toHaveBeenCalledWith(conversationId, sessionKey, 'start');
    });
    rerender(
      <WorkspacePanelProvider value={{ tab: panelTab, isActive: true }}>
        <ChatSessionView
          surface={surface({ sessionKey, surfaceId: 'boundary-surface' })}
          onSend={vi.fn().mockResolvedValue(undefined)}
          showShortcutsHelp={false}
        />
      </WorkspacePanelProvider>,
    );

    await waitFor(() => {
      // Navegação explícita: a pessoa pediu, então o aviso não espera leitura.
      expect(announceRequestMock).toHaveBeenCalledWith(expect.objectContaining({
        message: 'chat.announce.messageWindowLoaded:1-2-10',
        eventType: 'user-action',
        origin: expect.objectContaining({ surfaceId: 'boundary-surface' }),
      }));
      expect(document.activeElement).toHaveAttribute('data-message-id', 'm1');
    });

    fireEvent.keyDown(list, { key: 'End', ctrlKey: true });

    await waitFor(() => {
      expect(chatStoreState.loadBoundaryMessagesForConversation).toHaveBeenCalledWith(conversationId, sessionKey, 'end');
    });
    rerender(
      <WorkspacePanelProvider value={{ tab: panelTab, isActive: true }}>
        <ChatSessionView
          surface={surface({ sessionKey, surfaceId: 'boundary-surface' })}
          onSend={vi.fn().mockResolvedValue(undefined)}
          showShortcutsHelp={false}
        />
      </WorkspacePanelProvider>,
    );

    await waitFor(() => {
      expect(announceRequestMock).toHaveBeenCalledWith(expect.objectContaining({
        message: 'chat.announce.messageWindowLoaded:9-10-10',
        eventType: 'user-action',
      }));
      expect(document.activeElement).toHaveAttribute('data-message-id', 'm2');
    });
  });

  it('anuncia janela mesmo quando o carregamento demora a resolver', async () => {
    const sessionKey = 'slow-session';
    let surfaceSession = {
      ...createEmptyChatSurfaceSession(conversationId, sessionKey),
      messageWindow: {
        scope: 'conversation' as const,
        conversationId,
        totalCount: 10,
        startIndex: 4,
        endIndex: 5,
        hasBefore: true,
        hasAfter: true,
      },
    };
    (chatStoreState.surfaceSessionsByKey as Record<string, typeof surfaceSession>)[sessionKey] = surfaceSession;
    let resolveLoad: (() => void) | undefined;
    chatStoreState.loadNewerMessagesForConversation.mockImplementation(() => new Promise<void>((resolve) => {
      resolveLoad = () => {
        surfaceSession = {
          ...surfaceSession,
          messageWindow: { ...surfaceSession.messageWindow, startIndex: 8, endIndex: 9, hasAfter: false },
        };
        (chatStoreState.surfaceSessionsByKey as Record<string, typeof surfaceSession>)[sessionKey] = surfaceSession;
        resolve();
      };
    }));

    const { rerender } = renderWithPanel(
      <ChatSessionView
        surface={surface({ sessionKey, surfaceId: 'slow-surface' })}
        onSend={vi.fn().mockResolvedValue(undefined)}
        showShortcutsHelp={false}
      />,
    );

    vi.useFakeTimers();
    try {
      fireEvent.click(screen.getByText('load-newer-scroll'));

      // Backend lento: o carregamento demora mais que o prazo do pendente e a
      // janela ainda muda no meio do caminho, por streaming.
      await vi.advanceTimersByTimeAsync(10_000);
      rerender(
        <WorkspacePanelProvider value={{ tab: panelTab, isActive: true }}>
          <ChatSessionView
            surface={surface({ sessionKey, surfaceId: 'slow-surface' })}
            onSend={vi.fn().mockResolvedValue(undefined)}
            showShortcutsHelp={false}
          />
        </WorkspacePanelProvider>,
      );
      resolveLoad?.();
      await vi.advanceTimersByTimeAsync(0);

      rerender(
        <WorkspacePanelProvider value={{ tab: panelTab, isActive: true }}>
          <ChatSessionView
            surface={surface({ sessionKey, surfaceId: 'slow-surface' })}
            onSend={vi.fn().mockResolvedValue(undefined)}
            showShortcutsHelp={false}
          />
        </WorkspacePanelProvider>,
      );

      expect(announceRequestMock).toHaveBeenCalledWith(expect.objectContaining({
        message: 'chat.announce.messageWindowLoaded:9-10-10',
        eventType: 'progress',
      }));
    } finally {
      vi.useRealTimers();
    }
  });

  it('desarma o pendente quando o carregamento nunca termina', async () => {
    const sessionKey = 'hung-session';
    let surfaceSession = {
      ...createEmptyChatSurfaceSession(conversationId, sessionKey),
      messageWindow: {
        scope: 'conversation' as const,
        conversationId,
        totalCount: 10,
        startIndex: 4,
        endIndex: 5,
        hasBefore: true,
        hasAfter: true,
      },
    };
    (chatStoreState.surfaceSessionsByKey as Record<string, typeof surfaceSession>)[sessionKey] = surfaceSession;
    chatStoreState.loadNewerMessagesForConversation.mockImplementation(() => new Promise<void>(() => {}));

    const { rerender } = renderWithPanel(
      <ChatSessionView
        surface={surface({ sessionKey, surfaceId: 'hung-surface' })}
        onSend={vi.fn().mockResolvedValue(undefined)}
        showShortcutsHelp={false}
      />,
    );

    vi.useFakeTimers();
    try {
      fireEvent.click(screen.getByText('load-newer-scroll'));

      // Muito depois, o streaming mexe na janela. O pendente já expirou pelo
      // teto do carregamento e não empresta esse avanço para um aviso.
      await vi.advanceTimersByTimeAsync(90_000);
      surfaceSession = {
        ...surfaceSession,
        messageWindow: { ...surfaceSession.messageWindow, startIndex: 8, endIndex: 9, hasAfter: false },
      };
      (chatStoreState.surfaceSessionsByKey as Record<string, typeof surfaceSession>)[sessionKey] = surfaceSession;
      rerender(
        <WorkspacePanelProvider value={{ tab: panelTab, isActive: true }}>
          <ChatSessionView
            surface={surface({ sessionKey, surfaceId: 'hung-surface' })}
            onSend={vi.fn().mockResolvedValue(undefined)}
            showShortcutsHelp={false}
          />
        </WorkspacePanelProvider>,
      );

      expect(announceRequestMock).not.toHaveBeenCalledWith(expect.objectContaining({
        message: 'chat.announce.messageWindowLoaded:9-10-10',
      }));
    } finally {
      vi.useRealTimers();
    }
  });

  it('não deixa um carregamento antigo encurtar o prazo do que veio depois', async () => {
    const sessionKey = 'overlap-session';
    let surfaceSession = {
      ...createEmptyChatSurfaceSession(conversationId, sessionKey),
      messageWindow: {
        scope: 'conversation' as const,
        conversationId,
        totalCount: 10,
        startIndex: 4,
        endIndex: 5,
        hasBefore: true,
        hasAfter: true,
      },
    };
    (chatStoreState.surfaceSessionsByKey as Record<string, typeof surfaceSession>)[sessionKey] = surfaceSession;
    const resolvers: Array<() => void> = [];
    chatStoreState.loadNewerMessagesForConversation.mockImplementation(
      () => new Promise<void>((resolve) => { resolvers.push(resolve); }),
    );

    const { rerender } = renderWithPanel(
      <ChatSessionView
        surface={surface({ sessionKey, surfaceId: 'overlap-surface' })}
        onSend={vi.fn().mockResolvedValue(undefined)}
        showShortcutsHelp={false}
      />,
    );

    vi.useFakeTimers();
    try {
      // Dois carregamentos em voo: o primeiro termina depois do segundo começar.
      fireEvent.click(screen.getByText('load-newer-scroll'));
      fireEvent.click(screen.getByText('load-newer-scroll'));
      resolvers[0]?.();
      await vi.advanceTimersByTimeAsync(0);

      // O segundo ainda está carregando; passado o prazo curto do primeiro, ele
      // continua valendo pelo teto do carregamento.
      await vi.advanceTimersByTimeAsync(10_000);
      resolvers[1]?.();
      surfaceSession = {
        ...surfaceSession,
        messageWindow: { ...surfaceSession.messageWindow, startIndex: 8, endIndex: 9, hasAfter: false },
      };
      (chatStoreState.surfaceSessionsByKey as Record<string, typeof surfaceSession>)[sessionKey] = surfaceSession;
      await vi.advanceTimersByTimeAsync(0);
      rerender(
        <WorkspacePanelProvider value={{ tab: panelTab, isActive: true }}>
          <ChatSessionView
            surface={surface({ sessionKey, surfaceId: 'overlap-surface' })}
            onSend={vi.fn().mockResolvedValue(undefined)}
            showShortcutsHelp={false}
          />
        </WorkspacePanelProvider>,
      );

      expect(announceRequestMock).toHaveBeenCalledWith(expect.objectContaining({
        message: 'chat.announce.messageWindowLoaded:9-10-10',
      }));
    } finally {
      vi.useRealTimers();
    }
  });

  it('anuncia como progresso a janela carregada por scroll', async () => {
    const sessionKey = 'scroll-session';
    let surfaceSession = {
      ...createEmptyChatSurfaceSession(conversationId, sessionKey),
      messageWindow: {
        scope: 'conversation' as const,
        conversationId,
        totalCount: 10,
        startIndex: 4,
        endIndex: 5,
        hasBefore: true,
        hasAfter: true,
      },
    };
    (chatStoreState.surfaceSessionsByKey as Record<string, typeof surfaceSession>)[sessionKey] = surfaceSession;
    chatStoreState.loadNewerMessagesForConversation.mockImplementation(async () => {
      surfaceSession = {
        ...surfaceSession,
        messageWindow: { ...surfaceSession.messageWindow, startIndex: 8, endIndex: 9, hasAfter: false },
      };
      (chatStoreState.surfaceSessionsByKey as Record<string, typeof surfaceSession>)[sessionKey] = surfaceSession;
    });

    const { rerender } = renderWithPanel(
      <ChatSessionView
        surface={surface({ sessionKey, surfaceId: 'scroll-surface' })}
        onSend={vi.fn().mockResolvedValue(undefined)}
        showShortcutsHelp={false}
      />,
    );

    fireEvent.click(screen.getByText('load-newer-scroll'));

    await waitFor(() => {
      expect(chatStoreState.loadNewerMessagesForConversation).toHaveBeenCalled();
    });
    rerender(
      <WorkspacePanelProvider value={{ tab: panelTab, isActive: true }}>
        <ChatSessionView
          surface={surface({ sessionKey, surfaceId: 'scroll-surface' })}
          onSend={vi.fn().mockResolvedValue(undefined)}
          showShortcutsHelp={false}
        />
      </WorkspacePanelProvider>,
    );

    // Progresso espera a leitura do conteúdo terminar no broker; user-action não.
    // A origem viaja junto para o broker poder reavaliar a aba na hora de falar.
    await waitFor(() => {
      expect(announceRequestMock).toHaveBeenCalledWith(expect.objectContaining({
        message: 'chat.announce.messageWindowLoaded:9-10-10',
        eventType: 'progress',
        origin: expect.objectContaining({ surfaceId: 'scroll-surface' }),
      }));
    });
  });

  it('não anuncia janela quando o carregamento pendente já envelheceu', async () => {
    vi.useFakeTimers();
    try {
      const sessionKey = 'stale-session';
      let surfaceSession = {
        ...createEmptyChatSurfaceSession(conversationId, sessionKey),
        messageWindow: {
          scope: 'conversation' as const,
          conversationId,
          totalCount: 10,
          startIndex: 4,
          endIndex: 5,
          hasBefore: true,
          hasAfter: true,
        },
      };
      (chatStoreState.surfaceSessionsByKey as Record<string, typeof surfaceSession>)[sessionKey] = surfaceSession;
      chatStoreState.loadNewerMessagesForConversation.mockImplementation(async () => {});

      const { rerender } = renderWithPanel(
        <ChatSessionView
          surface={surface({ sessionKey, surfaceId: 'stale-surface' })}
          onSend={vi.fn().mockResolvedValue(undefined)}
          showShortcutsHelp={false}
        />,
      );

      fireEvent.click(screen.getByText('load-newer-scroll'));
      announceRequestMock.mockClear();

      // A janela só alcança o fim muito depois, por outro motivo (streaming).
      await vi.advanceTimersByTimeAsync(30_000);
      surfaceSession = {
        ...surfaceSession,
        messageWindow: { ...surfaceSession.messageWindow, startIndex: 8, endIndex: 9, hasAfter: false },
      };
      (chatStoreState.surfaceSessionsByKey as Record<string, typeof surfaceSession>)[sessionKey] = surfaceSession;
      rerender(
        <WorkspacePanelProvider value={{ tab: panelTab, isActive: true }}>
          <ChatSessionView
            surface={surface({ sessionKey, surfaceId: 'stale-surface' })}
            onSend={vi.fn().mockResolvedValue(undefined)}
            showShortcutsHelp={false}
          />
        </WorkspacePanelProvider>,
      );

      expect(announceRequestMock).not.toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });
});
