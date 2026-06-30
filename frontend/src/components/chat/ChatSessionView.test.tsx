import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MediaCategory, type MediaFile } from '../../services/mediaService';

const updateMessageMock = vi.fn();
const showMenuMock = vi.fn();
const hideMenuMock = vi.fn();
const copyMessageMock = vi.fn();
const speakMessageMock = vi.fn();
const conversationId = '01926b90-7a5a-7c4e-8d3f-000000000001';
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
const contextMenuState = vi.hoisted(() => ({ visible: true }));
const runtimeEventHandlers = vi.hoisted(() => new Map<string, (data: unknown) => void>());

vi.mock('../ui/Modal', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../ui/Modal')>();
  return { ...actual, isModalOpen: () => modalState.open };
});

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string, options?: { start?: number; end?: number; total?: number }) => (
      options?.total !== undefined ? `${key}:${options.start}-${options.end}-${options.total}` : key
    ),
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
  surfaceSessionsByKey: {},
  loadMessageChildren: vi.fn(),
  loadConversationSession: vi.fn(),
  updateConversationMessage: updateMessageMock,
  toggleConversationReasoningExpanded: vi.fn(),
  isConversationReasoningExpanded: () => false,
  startConversationEditing: vi.fn(),
  startConversationReading: vi.fn(),
  setConversationScrollState: vi.fn(),
  loadOlderMessagesForConversation: vi.fn(),
  loadNewerMessagesForConversation: vi.fn(),
  loadBoundaryMessagesForConversation: vi.fn(),
  setConversationDraftMessage: vi.fn(),
  setConversationDraftMediaFiles: vi.fn(),
  clearConversationDraft: vi.fn(),
  clearConversationSendFailure: vi.fn(),
  setConversationEditingMessageId: vi.fn(),
  setConversationReadingMessageId: vi.fn(),
  toggleConversationThreadExpanded: vi.fn(),
};

vi.mock('../../store/chatStore', () => ({
  useChatStore: (selector?: (s: typeof chatStoreState) => unknown) => {
    if (typeof selector === 'function') {
      return selector(chatStoreState);
    }
    return chatStoreState;
  },
}));

vi.mock('../../store/editorStore', () => ({
  useEditorStore: {
    getState: () => ({
      requestInsert: vi.fn(),
      activeTabId: 'chat-tab',
      tabs: [{ id: 'chat-tab', conversationId: '01926b90-7a5a-7c4e-8d3f-00000000000a' }],
    }),
  },
}));

vi.mock('../../hooks/useChatKeyboardNav', () => ({
  useChatKeyboardNav: () => {},
}));

vi.mock('../../hooks/useContextMenu', () => ({
  useContextMenu: () => ({
    menuVisible: contextMenuState.visible,
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

vi.mock('@wailsjs/go/app/App', () => ({
  DeleteMessage: vi.fn(),
  EditorGetDraftPath: vi.fn().mockResolvedValue(''),
  GetActiveProfile: vi.fn().mockResolvedValue({
    chat: { streaming_recovery_show_continue: true },
  }),
  GetActiveProfileSlug: vi.fn().mockResolvedValue('padrao'),
  GetActiveProviderInfo: vi.fn().mockResolvedValue({
    supports_assistant_prefill: true,
  }),
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
      threadedMessages?: Array<{ id?: string; message?: { id: string; role?: string; isStreaming?: boolean; turnId?: string; content?: string } }>;
      shouldShowContinue?: (message: { id: string; role?: string; isStreaming?: boolean; turnId?: string; content?: string }) => boolean;
      onJumpToStart?: () => Promise<void> | void;
      onJumpToEnd?: () => Promise<void> | void;
    }>((
    {
      onContextMenu,
      threadedMessages = [],
      shouldShowContinue,
      onJumpToStart,
      onJumpToEnd,
    },
    ref: React.Ref<HTMLDivElement>,
  ) => (
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
  )),
  };
});

vi.mock('./ChatInput', async () => {
  const React = await import('react');
  return {
    ChatInput: React.forwardRef<HTMLTextAreaElement, { onSend: (value: string) => void; disabled?: boolean }>(
      ({ onSend, disabled }, ref) => (
        <button
          ref={ref as React.RefObject<HTMLButtonElement>}
          type="button"
          disabled={disabled}
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

vi.mock('../../hooks/useAnnouncer', () => ({
  announce: vi.fn(),
}));

vi.mock('../../utils/errorHandler', () => ({
  ErrorSeverity: { RECOVERABLE: 'recoverable' },
  ErrorMessages: { CHAT: { SEND_FAILED: 'Falha ao enviar', DELETE_FAILED: 'Falha ao deletar' } },
  handleError: vi.fn(),
}));

import { ChatSessionView } from './ChatSessionView';
import { createEmptyChatSurfaceSession, type ChatSurfaceIdentity } from '../../services/chatSessionRegistry';
import { WorkspacePanelProvider } from '../workspace/WorkspacePanelContext';
import { announce } from '../../hooks/useAnnouncer';
import { useShortcutsHelpStore } from '../../store/shortcutsHelpStore';

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
    showMenuMock.mockReset();
    hideMenuMock.mockReset();
    chatStoreState.setConversationScrollState.mockReset();
    chatStoreState.loadBoundaryMessagesForConversation.mockReset();
    chatStoreState.sessionsByConversationId[conversationId].isLoading = false;
    chatStoreState.sessionsByConversationId[conversationId].conversation = activeConversation;
    chatStoreState.sessionsByConversationId[conversationId].hasOlderMessages = false;
    chatStoreState.sessionsByConversationId[conversationId].isLoadingOlderMessages = false;
    (chatStoreState.sessionsByConversationId[conversationId] as typeof chatStoreState.sessionsByConversationId[typeof conversationId] & { sendFailureMessage?: string | null; sendFailureRetryable?: boolean }).sendFailureMessage = null;
    (chatStoreState.sessionsByConversationId[conversationId] as typeof chatStoreState.sessionsByConversationId[typeof conversationId] & { sendFailureRetryable?: boolean; sendFailureRetryContent?: string | null; sendFailureRetryMediaFiles?: unknown[] }).sendFailureRetryable = false;
    (chatStoreState.sessionsByConversationId[conversationId] as typeof chatStoreState.sessionsByConversationId[typeof conversationId] & { sendFailureRetryContent?: string | null }).sendFailureRetryContent = null;
    (chatStoreState.sessionsByConversationId[conversationId] as typeof chatStoreState.sessionsByConversationId[typeof conversationId] & { sendFailureRetryMediaFiles?: unknown[] }).sendFailureRetryMediaFiles = [];
    (activeConversation.threadedMessages as unknown[]) = [];
    (announce as ReturnType<typeof vi.fn>).mockReset();
    chatStoreState.cancelStreaming.mockReset();
    chatStoreState.clearConversationSendFailure.mockReset();
    chatStoreState.surfaceSessionsByKey = {};
    modalState.open = false;
    contextMenuState.visible = true;
    runtimeEventHandlers.clear();
    useShortcutsHelpStore.setState({ isOpen: false });
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

  it('embedded: mostra banner de erro e retry quando onSend falha', async () => {
    const user = userEvent.setup();
    const onSend = vi.fn().mockRejectedValueOnce(new Error('fail'));
    renderWithPanel(<ChatSessionView variant="embedded" surface={surface({ surfaceType: 'embedded' })} onSend={onSend} showShortcutsHelp={false} />);

    await user.click(screen.getByRole('button', { name: 'send' }));

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Falha ao enviar');

    onSend.mockResolvedValueOnce(undefined);
    await user.click(screen.getByRole('button', { name: 'chat.retryAriaLabel' }));

    await waitFor(() => {
      expect(onSend).toHaveBeenCalledTimes(2);
    });
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

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Falha ao enviar pela sessão');
    expect(screen.queryByRole('button', { name: 'chat.retryAriaLabel' })).not.toBeInTheDocument();
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
    const onSend = vi.fn().mockResolvedValue(undefined);
    (chatStoreState.surfaceSessionsByKey as Record<string, ReturnType<typeof createEmptyChatSurfaceSession>>)[chatSurface.sessionKey] = {
      ...createEmptyChatSurfaceSession(conversationId, chatSurface.sessionKey),
      sendFailureMessage: 'Falha ao enviar mídia',
      sendFailureRetryable: true,
      sendFailureRetryContent: null,
      sendFailureRetryMediaFiles: [mediaFile],
    };
    renderWithPanel(<ChatSessionView variant="embedded" surface={chatSurface} onSend={onSend} showShortcutsHelp={false} />);

    await user.click(await screen.findByRole('button', { name: 'chat.retryAriaLabel' }));

    expect(onSend).toHaveBeenCalledWith('', [mediaFile], chatSurface);
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

    expect(onSend).toHaveBeenCalledWith('oi', undefined, expect.objectContaining({
      conversationId,
      sessionKey: `embedded:workspace-chat-modal:tab-1:${conversationId}`,
      surfaceId: 'embedded:workspace-chat-modal:tab-1',
      surfaceType: 'embedded',
    }));
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
        { message: { id: 'm1', role: 'user' }, children: [], level: 0, childCount: 0 },
        { message: { id: 'm2', role: 'assistant' }, children: [], level: 0, childCount: 0 },
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
    (chatStoreState.surfaceSessionsByKey as Record<string, typeof surfaceSession>)[sessionKey] = surfaceSession;
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
      (chatStoreState.surfaceSessionsByKey as Record<string, typeof surfaceSession>)[sessionKey] = surfaceSession;
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
      expect(announce).toHaveBeenCalledWith('chat.announce.messageWindowLoaded:1-2-10');
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
      expect(announce).toHaveBeenCalledWith('chat.announce.messageWindowLoaded:9-10-10');
      expect(document.activeElement).toHaveAttribute('data-message-id', 'm2');
    });
  });
});
