import { logger } from '../../utils/logger';
import { useRef, useState, useEffect, useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button } from 'antd';
import { useEditorStore } from '../../store/editorStore';
import { useChatStore } from '../../store/chatStore';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { ttsService } from '../../services/tts';
import { MessageList } from './MessageList';
import { ChatInput } from './ChatInput';
import { ChatToolbar, type ChatToolbarConversationChangeHandler } from './ChatToolbar';
import { ChatSessionProvider } from './ChatSessionContext';
import type {
  ChatSurfaceIdentity,
  ChatSurfaceOrigin,
} from '../../services/chatSessionRegistry';
import { ContextMenu } from '../menu';
import { useShortcutsHelpStore } from '../../store/shortcutsHelpStore';
import { isModalOpen } from '../ui/Modal';
import { useWorkspacePanel } from '../workspace/WorkspacePanelContext';
import { useChatKeyboardNav } from '../../hooks/useChatKeyboardNav';
import { useContextMenu, useMessageActions } from '../../hooks/useContextMenu';
import { isBackendId } from '../../lib/idUtils';
import type { MediaFile } from '../../services/mediaService';
import { DeleteMessage, EditorGetDraftPath, GetActiveProfile } from '@wailsjs/go/app/App';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { announce } from '../../hooks/useAnnouncer';
import { handleError, ErrorSeverity, ErrorMessages } from '../../utils/errorHandler';
import type { EditorSendTargetOption, SendToEditorPayload } from '../../lib/editorSendMenu';
import {
  useChatSurfaceController,
  type ChatSurfaceController,
} from './ChatSurfaceController';
import './ChatSessionView.css';

export interface ChatSessionViewProps {
  variant?: 'page' | 'embedded';
  surface: ChatSurfaceIdentity;
  /** Envio da mensagem (ex.: sendMessage da store ou adaptador do chat modal) */
  onSend: (content: string, mediaFiles: MediaFile[] | undefined, origin: ChatSurfaceOrigin) => Promise<void>;
  /** Solicitação de troca de conversa (controlada pelo dono da superfície). */
  onRequestConversationChange?: ChatToolbarConversationChangeHandler;
  showShortcutsHelp?: boolean;
  profileSlug?: string;
}

export function ChatSessionView({
  variant = 'page',
  surface,
  onSend,
  onRequestConversationChange,
  showShortcutsHelp,
  profileSlug,
}: ChatSessionViewProps) {
  return (
    <ChatSessionProvider surface={surface}>
      <ChatSessionViewControllerBridge
        variant={variant}
        surface={surface}
        onSend={onSend}
        onRequestConversationChange={onRequestConversationChange}
        showShortcutsHelp={showShortcutsHelp}
      />
    </ChatSessionProvider>
  );
}

function ChatSessionViewControllerBridge({
  onSend,
  onRequestConversationChange,
  variant,
  showShortcutsHelp,
}: ChatSessionViewProps) {
  const controller = useChatSurfaceController({
    onSend: (content, mediaFiles, context) => onSend(content, mediaFiles, context.origin),
  });

  return (
    <ChatSessionViewContent
      variant={variant}
      showShortcutsHelp={showShortcutsHelp}
      onRequestConversationChange={onRequestConversationChange}
      controller={controller}
      profileSlug={profileSlug}
    />
  );
}

interface ChatSessionViewContentProps extends Pick<ChatSessionViewProps, 'variant' | 'showShortcutsHelp' | 'onRequestConversationChange' | 'profileSlug'> {
  controller: ChatSurfaceController;
}

function ChatSessionViewContent({
  variant = 'page',
  showShortcutsHelp,
  onRequestConversationChange,
  profileSlug,
  controller,
}: ChatSessionViewContentProps) {
  const { t } = useTranslation();
  const rootRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const scrollFrameRef = useRef<number | null>(null);
  const restoreScrollFrameRef = useRef<number | null>(null);
  const scrollPersistTimerRef = useRef<number | null>(null);
  const latestScrollStateRef = useRef<{ scrollTop: number; scrollAnchorMessageId: string | null }>({
    scrollTop: 0,
    scrollAnchorMessageId: null,
  });
  const restoredScrollSessionKeyRef = useRef<string | null>(null);
  const hasAutoFocusedRef = useRef(false);
  const retryButtonRef = useRef<HTMLButtonElement>(null);
  const wasLoadingRef = useRef(false);
  const pendingWindowAnnouncementRef = useRef<{
    kind: 'start' | 'end' | 'older' | 'newer';
    previousStartIndex: number;
    previousEndIndex: number;
    previousWindowKey: string | null;
  } | null>(null);
  const latestWindowKeyRef = useRef<string | null>(null);
  const { isActive: isPanelActive } = useWorkspacePanel();
  const isInteractiveSurface = variant === 'embedded' || isPanelActive;

  const [showContinueEnabled, setShowContinueEnabled] = useState(false);
  const [activeProfileSlug, setActiveProfileSlug] = useState('');

  const {
    session,
    conversation,
    threadedMessages,
    isLoading,
    hasOlderMessages,
    hasNewerMessages,
    isLoadingOlderMessages,
    isLoadingMessageWindow,
    loadOlderMessages,
    loadNewerMessages,
    loadStartMessages,
    loadEndMessages,
    loadMessageChildren,
    loadConversationSession,
    retryMessageToConversation,
    updateConversationMessage,
    toggleConversationReasoningExpanded,
    isConversationReasoningExpanded,
    startConversationEditing,
    startConversationReading,
    origin,
    conversationId,
    draftMessage,
    draftMediaFiles,
    scrollTop,
    scrollAnchorMessageId,
    setDraftMessage,
    setDraftMediaFiles,
    setScrollState,
  } = controller;

  const cancelStreaming = useChatStore((state) => state.cancelStreaming);

  const handleCancelStreaming = useCallback(async () => {
    const targetConversationId = conversation?.id ?? conversationId;
    if (!targetConversationId) return;
    await cancelStreaming(targetConversationId, { origin });
  }, [cancelStreaming, conversation?.id, conversationId, origin]);
  const getSessionConversation = useCallback(() => conversation, [conversation]);
  const visibleMessageCount = useMemo(() => {
    if (!threadedMessages.length) return 0;
    const processedTurnIds = new Set<string>();
    let count = 0;
    for (const node of threadedMessages) {
      const message = node.message;
      if (!message) continue;
      const turnId = message.turnId;
      if (!turnId) {
        count += 1;
        continue;
      }
      if (message.role === 'tool' || processedTurnIds.has(turnId)) continue;
      processedTurnIds.add(turnId);
      count += 1;
    }
    return count;
  }, [threadedMessages]);
  const usesLocalVisualWindowCount = visibleMessageCount > 0 && visibleMessageCount !== threadedMessages.length;

  useEffect(() => {
    let mounted = true;

    const loadActiveProfile = async () => {
      try {
        const profile = await GetActiveProfile();
        if (!mounted) return;
        // A continuação é habilitada apenas pelo perfil: o backend sempre consegue
        // continuar — via assistant prefill quando o provider suporta, ou via
        // fallback por mensagem de usuário quando não suporta (Issue #124).
        const profileAllowsContinue = profile?.chat?.streaming_recovery_show_continue ?? true;
        setShowContinueEnabled(profileAllowsContinue);
        setActiveProfileSlug(profile?.slug || '');
      } catch {
        if (!mounted) return;
        setShowContinueEnabled(false);
        setActiveProfileSlug('');
      }
    };

    void loadActiveProfile();
    const unsubChanged = EventsOn('profile:changed', () => void loadActiveProfile());
    const unsubUpdated = EventsOn('profile:updated', () => void loadActiveProfile());

    return () => {
      mounted = false;
      unsubChanged();
      unsubUpdated();
    };
  }, []);

  useEffect(() => {
    if (!conversationId) return;
    if (session?.conversation) return;
    void loadConversationSession(conversationId);
  }, [conversationId, loadConversationSession, session?.conversation, variant]);

  useEffect(() => {
    const container = messagesContainerRef.current;
    if (!container || (scrollTop <= 0 && !scrollAnchorMessageId)) return;
    const restoreKey = `${origin.sessionKey}:${conversationId ?? 'none'}`;
    if (restoredScrollSessionKeyRef.current === restoreKey) return;
    if (restoreScrollFrameRef.current !== null) {
      window.cancelAnimationFrame(restoreScrollFrameRef.current);
    }
    restoreScrollFrameRef.current = window.requestAnimationFrame(() => {
      restoreScrollFrameRef.current = null;
      if (`${origin.sessionKey}:${conversationId ?? 'none'}` !== restoreKey) return;
      const currentContainer = messagesContainerRef.current;
      if (!currentContainer) return;
      if (scrollAnchorMessageId) {
        const anchorElement = currentContainer
          .querySelector<HTMLElement>(`[data-message-id="${CSS.escape(scrollAnchorMessageId)}"]`);
        if (anchorElement) {
          anchorElement.scrollIntoView({ block: 'start' });
          restoredScrollSessionKeyRef.current = restoreKey;
          return;
        }
        if (scrollTop > 0) {
          currentContainer.scrollTop = scrollTop;
          restoredScrollSessionKeyRef.current = restoreKey;
        }
        return;
      }
      if (scrollTop > 0) {
        currentContainer.scrollTop = scrollTop;
        restoredScrollSessionKeyRef.current = restoreKey;
      }
    });
    return () => {
      if (restoreScrollFrameRef.current !== null) {
        window.cancelAnimationFrame(restoreScrollFrameRef.current);
        restoreScrollFrameRef.current = null;
      }
    };
  }, [conversationId, origin.sessionKey, scrollAnchorMessageId, scrollTop, threadedMessages.length]);

  useEffect(() => {
    const container = messagesContainerRef.current;
    if (!container || !conversationId) return;

    const getAnchorMessageId = () => {
      if (!document.elementsFromPoint) return null;
      const rect = container.getBoundingClientRect();
      if (rect.width <= 0 || rect.height <= 0) return null;
      const probeX = Math.min(rect.right - 1, rect.left + Math.min(16, Math.max(1, rect.width / 2)));
      const probeYs = [
        rect.top + 1,
        Math.min(rect.bottom - 1, rect.top + 24),
        rect.top + rect.height / 2,
      ];

      for (const probeY of probeYs) {
        const messageNode = document
          .elementsFromPoint(probeX, probeY)
          .map((element) => element.closest?.('[data-message-node]'))
          .find((node): node is HTMLElement => !!node && container.contains(node));
        if (messageNode?.dataset.messageId) {
          return messageNode.dataset.messageId;
        }
      }

      return null;
    };

    const readScrollState = () => ({
      scrollTop: container.scrollTop,
      scrollAnchorMessageId: getAnchorMessageId(),
    });

    const persistLatestScrollState = () => {
      setScrollState(latestScrollStateRef.current);
    };

    const handleScroll = () => {
      if (scrollFrameRef.current !== null) return;
      scrollFrameRef.current = window.requestAnimationFrame(() => {
        scrollFrameRef.current = null;
        latestScrollStateRef.current = readScrollState();
        if (scrollPersistTimerRef.current !== null) {
          window.clearTimeout(scrollPersistTimerRef.current);
        }
        scrollPersistTimerRef.current = window.setTimeout(() => {
          scrollPersistTimerRef.current = null;
          persistLatestScrollState();
        }, 200);
      });
    };

    container.addEventListener('scroll', handleScroll, { passive: true });
    return () => {
      container.removeEventListener('scroll', handleScroll);
      if (scrollFrameRef.current !== null) {
        window.cancelAnimationFrame(scrollFrameRef.current);
        scrollFrameRef.current = null;
      }
      if (scrollPersistTimerRef.current !== null) {
        window.clearTimeout(scrollPersistTimerRef.current);
        scrollPersistTimerRef.current = null;
      }
      latestScrollStateRef.current = readScrollState();
      persistLatestScrollState();
    };
  }, [conversationId, setScrollState]);

  const [hasVoiceConfig, setHasVoiceConfig] = useState(() => ttsService.hasVoiceConfig());
  useEffect(() => {
    const handler = () => setHasVoiceConfig(ttsService.hasVoiceConfig());
    ttsService.on('voiceConfigChanged', handler);
    return () => {
      ttsService.off('voiceConfigChanged', handler);
    };
  }, []);
  const isTTSDisabled = !hasVoiceConfig;

  const shortcutsOpen = showShortcutsHelp ?? variant === 'page';

  const [lastFailedMessage, setLastFailedMessage] = useState<{ content: string; media?: MediaFile[] } | null>(null);
  const [sendError, setSendError] = useState<string | null>(null);

  const wsTabs = useWorkspaceStore((state) => state.workspace?.tabs);

  const editorTargets = useMemo<EditorSendTargetOption[]>(
    () =>
      (wsTabs || [])
        .filter((tab) => tab.type === 'editor')
        .map((tab) => ({
          id: tab.id,
          title: String(tab.title || '').trim() || t('editor.fallback.title'),
        })),
    [wsTabs, t],
  );

  const { copyMessage, speakMessage } = useMessageActions({
    onAnnounce: announce,
  });

  const handleDeleteMessage = useCallback(
    async (message: { id: string | number }) => {
      const messageId = String(message.id);
      if (!isBackendId(messageId)) return;
      try {
        await DeleteMessage(messageId);
        announce(t('chat.announce.messageDeleted'));
        const conv = getSessionConversation();
        if (conv?.id) {
          await loadConversationSession(conv.id, { refreshSurfaceWindows: true });
        }
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : String(error);
        const lower = errorMessage.toLowerCase();
        const userCanceled =
          lower.includes('cancelada') ||
          lower.includes('cancelado') ||
          lower.includes('canceled') ||
          lower.includes('cancelled');
        if (userCanceled) {
          announce(t('chat.announce.deleteCancelled'));
          return;
        }

        handleError(error, {
          source: 'ChatSessionView.onDelete',
          userMessage: ErrorMessages.CHAT.DELETE_FAILED,
          severity: ErrorSeverity.RECOVERABLE,
          metadata: { messageId },
        });
      }
    },
    [announce, conversationId, getSessionConversation, loadConversationSession, t, variant],
  );

  const sendToEditor = useCallback(
    async (payload: SendToEditorPayload) => {
      const content = String(payload?.content ?? '');
      if (!content) return;

      const title = payload.title || t('editor.fallback.fromChat');
      const { addTab, setActiveTab } = useWorkspaceStore.getState();
      const ensureActiveEditorTab = async (tabId: string) => {
        await setActiveTab(tabId);
        return useWorkspaceStore.getState().workspace?.activeTabId === tabId;
      };
      const createDraftEditorTab = async () => {
        const draftId =
          typeof crypto !== 'undefined' && crypto.randomUUID
            ? crypto.randomUUID()
            : `editor-${Date.now()}`;
        const draftPath = String((await EditorGetDraftPath(draftId)) ?? '');
        const tabId = await addTab('editor', title, { filePath: draftPath, draftId });
        const activated = await ensureActiveEditorTab(tabId);
        if (!activated) return null;
        useEditorStore.getState().createDocument({
          id: tabId,
          title,
          markdown: '',
          mode: 'markdown',
          filePath: draftPath,
          draftId,
        });
        return tabId;
      };

      if (payload.target === 'new_document') {
        const tabId = await createDraftEditorTab();
        if (!tabId) return;
        useEditorStore.getState().requestInsert({
          target: 'document',
          targetDocumentId: tabId,
          format: payload.format,
          title,
          content,
          focus: true,
        });
        return;
      }

      const targetDocumentId = String(payload.targetDocumentId || '').trim();
      if (!targetDocumentId) return;

      const activated = await ensureActiveEditorTab(targetDocumentId);
      if (!activated) return;

      useEditorStore.getState().requestInsert({
        target: 'document',
        targetDocumentId,
        format: payload.format,
        title,
        content,
        focus: true,
      });
    },
    [t],
  );

  const { menuVisible, menuPosition, menuItems, showMenu, hideMenu } = useContextMenu({
    sessionKey: origin.sessionKey,
    onCopy: copyMessage,
    onReadMessage: (message) => {
      if (conversation?.id) {
        startConversationReading(conversation.id, message.id);
      }
    },
    onSpeak: speakMessage,
    onEdit: (message) => {
      if (conversation?.id) {
        startConversationEditing(conversation.id, message.id);
      }
    },
    onResend: async (message) => {
      const conversationId = getSessionConversation()?.id;
      if (!conversationId || !isBackendId(message.id)) return;
      await retryMessageToConversation(conversationId, message.id, undefined, { origin });
      announce(t('chat.announce.messageResent'));
    },
    onContinue: async (message) => {
      const conversationId = getSessionConversation()?.id;
      const turnId = String(message.turnId || '').trim();
      if (!conversationId || !turnId) return;
      await retryMessageToConversation(conversationId, turnId, { allowAssistantPrefill: true }, { origin });
      announce(t('chat.announce.continuingResponse'));
    },
    shouldShowContinue: (message) => {
      if (!showContinueEnabled) return false;
      const interruptedId = session?.lastInterruptedMessageId;
      if (!interruptedId) return false;
      if (!isBackendId(String(interruptedId))) return false;
      if (String(message.id) !== String(interruptedId)) return false;
      if (!isBackendId(message.id)) return false;
      if (message.role !== 'assistant' || message.isStreaming) return false;
      if (!String(message.turnId || '').trim()) return false;
      if (!String(message.content || '').trim()) return false;
      return true;
    },
    onDelete: handleDeleteMessage,
    onCancelStreaming: (message) => {
      if (!message.isStreaming) return;
      void handleCancelStreaming();
    },
    onSendToEditor: sendToEditor,
    editorTargets,
    onPin: (_message) => {
      announce(t('chat.announce.pinComingSoon'));
    },
    onToggleReasoning: (message) => {
      const targetConversationId = conversation?.id;
      if (!targetConversationId) return;
      const isExpanded = isConversationReasoningExpanded(targetConversationId, message.id);
      toggleConversationReasoningExpanded(targetConversationId, message.id);
      announce(isExpanded ? t('chat.reasoningHidden') : t('chat.reasoningShown'));
    },
    isReasoningExpanded: (messageId: string) => (
      conversation?.id
        ? isConversationReasoningExpanded(conversation.id, messageId)
        : false
    ),
    isTTSDisabled,
  });

  useEffect(() => {
    if (!isInteractiveSurface || !isLoading) return;

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented) return;
      // Com um modal aberto (ex.: painel de atalhos), o Escape deve fechar o
      // modal — não cancelar o streaming nem o menu na UI de fundo.
      if (isModalOpen()) return;

      if (event.key !== 'Escape') return;

      if (menuVisible) {
        event.preventDefault();
        hideMenu();
        return;
      }

      // O cancelamento por Escape é escopado ao campo de edição (tratado pelo
      // próprio ChatInput). Aqui apenas devolvemos o foco ao input quando o
      // Escape vem de qualquer outro elemento do painel — sem cancelar.
      const input = inputRef.current;
      if (event.target === input) return;
      if (!input) return;

      // Só devolvemos o foco ao input quando o Escape se origina DENTRO do
      // painel do chat. Fora dele (outra superfície/painel, ex.: terminal,
      // editor, task list, ou o chat embutido enquanto inativo), o roteamento
      // "ESC → área padrão do painel atual" é responsabilidade do sistema
      // central de landmarks (useLandmarkNavigation no WorkspaceLayout), que
      // respeita o painel ativo. Assim este listener não "rouba" o foco para o
      // chat a partir de outras áreas do app (Issue #202 / AEP-0058).
      const root = rootRef.current;
      if (!root || !root.contains(event.target as Node | null)) return;

      event.preventDefault();
      input.focus();
    };

    // Registrado na fase de borbulhamento (sem captura) para que handlers locais
    // de Escape (ex.: colapso/navegação em MessageNode) rodem primeiro e possam
    // chamar preventDefault()/stopPropagation(). Aqui só agimos quando o Escape
    // não foi tratado localmente (event.defaultPrevented evita interferência).
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [hideMenu, isInteractiveSurface, isLoading, menuVisible]);

  useChatKeyboardNav({
    enabled: isInteractiveSurface,
    inputRef,
    messagesContainerRef,
  });

  useEffect(() => {
    if (variant !== 'page' || !isInteractiveSurface) return;
    let focusTimer: ReturnType<typeof setTimeout> | null = null;
    const checkTimer = setInterval(() => {
      const inputElement = inputRef.current;
      if (inputElement && !hasAutoFocusedRef.current) {
        hasAutoFocusedRef.current = true;
        clearInterval(checkTimer);
        focusTimer = setTimeout(() => {
          if (typeof document === 'undefined') return;
          const active = document.activeElement as HTMLElement | null;
          const hasMeaningfulFocus =
            !!active &&
            active !== document.body &&
            active !== document.documentElement &&
            active !== inputElement;
          if (document.querySelector('.ws-tabs__tab-edit')) return;
          if (active?.closest('.ws-tabs')) return;
          if (hasMeaningfulFocus) return;
          inputElement.focus();
        }, 100);
      }
    }, 100);

    return () => {
      clearInterval(checkTimer);
      if (focusTimer) clearTimeout(focusTimer);
    };
  }, [isInteractiveSurface, variant]);

  useEffect(() => {
    if (!isInteractiveSurface) {
      wasLoadingRef.current = isLoading;
      return;
    }
    if (wasLoadingRef.current && !isLoading) {
      const active = document.activeElement as HTMLElement | null;
      const isEditingMessage = active?.closest('.chat-message--editing') !== null;
      const isEditingWorkspaceTab = !!document.querySelector('.ws-tabs__tab-edit');
      if (!isEditingMessage && !isEditingWorkspaceTab) {
        requestAnimationFrame(() => {
          inputRef.current?.focus();
        });
      }
    }
    wasLoadingRef.current = isLoading;
  }, [isInteractiveSurface, isLoading]);

  useEffect(() => {
    if (!shortcutsOpen || !isInteractiveSurface) return;
    const handleKeyPress = (e: KeyboardEvent) => {
      // Não interceptar '?' quando outro modal está aberto: o Modal é portalado
      // para fora de #root, então o evento ainda chega em `document`. Sem esta
      // guarda o painel abriria por cima de outra UI modal e ainda chamaria
      // preventDefault sobre o '?'. (Quando o próprio painel está aberto,
      // isModalOpen() também é true; fechar é via ESC/Ctrl+?/overlay.)
      if (e.defaultPrevented || isModalOpen()) return;

      const target = e.target as HTMLElement;
      const isInputElement = target.tagName === 'INPUT' || target.tagName === 'TEXTAREA';

      if (e.key === '?' && !isInputElement) {
        e.preventDefault();
        useShortcutsHelpStore.getState().open();
      }
    };

    document.addEventListener('keypress', handleKeyPress);
    return () => document.removeEventListener('keypress', handleKeyPress);
  }, [isInteractiveSurface, shortcutsOpen]);

  useEffect(() => {
    const handleMessageUpdated = (data: unknown) => {
      const eventData = data as { message_id?: number | string; content?: string };
      if (eventData.message_id && eventData.content !== undefined && conversationId) {
        updateConversationMessage(conversationId, String(eventData.message_id), eventData.content);
      }
    };

    const unsubscribe = EventsOn('message:updated', handleMessageUpdated);
    return () => {
      if (unsubscribe) unsubscribe();
    };
  }, [conversationId, updateConversationMessage]);

  useEffect(() => {
    if (!conversationId) return;
    const unsubscribe = EventsOn('chat:skill_loaded', (data: unknown) => {
      const eventData = data as { conversationId?: string; displayName?: string; slug?: string };
      if (eventData.conversationId !== conversationId) return;
      const name = eventData.displayName || eventData.slug || '';
      if (!name) return;
      announce(t('chat.announce.skillLoaded', { name }));
    });
    return () => {
      if (unsubscribe) unsubscribe();
    };
  }, [announce, conversationId, t]);

  useEffect(() => {
    if (sendError && retryButtonRef.current) {
      retryButtonRef.current.focus();
    }
  }, [sendError]);

  useEffect(() => {
    const windowState = session?.messageWindow;
    latestWindowKeyRef.current = windowState
      ? `${windowState.startIndex}:${windowState.endIndex}:${windowState.totalCount}`
      : null;
    const pendingAnnouncement = pendingWindowAnnouncementRef.current;
    if (!pendingAnnouncement) return;
    if (!windowState || windowState.totalCount <= 0) {
      pendingWindowAnnouncementRef.current = null;
      return;
    }
    const didCompleteRequestedLoad =
      pendingAnnouncement.kind === 'older'
        ? windowState.startIndex < pendingAnnouncement.previousStartIndex || !windowState.hasBefore
        : pendingAnnouncement.kind === 'newer'
          ? windowState.endIndex > pendingAnnouncement.previousEndIndex || !windowState.hasAfter
          : pendingAnnouncement.kind === 'start'
            ? windowState.startIndex === 0
            : windowState.totalCount > 0 && windowState.endIndex >= windowState.totalCount - 1;
    if (!didCompleteRequestedLoad) return;
    pendingWindowAnnouncementRef.current = null;
    if (usesLocalVisualWindowCount) {
      announce(t('chat.announce.messageWindowLoaded', {
        start: 1,
        end: visibleMessageCount,
        total: visibleMessageCount,
      }));
      return;
    }
    announce(t('chat.announce.messageWindowLoaded', {
      start: windowState.startIndex + 1,
      end: windowState.endIndex + 1,
      total: windowState.totalCount,
    }));
  }, [announce, session?.messageWindow, t, usesLocalVisualWindowCount, visibleMessageCount]);

  useEffect(() => {
    if (!isInteractiveSurface) return;
    const handleEscape = (e: KeyboardEvent) => {
      // Com um modal aberto, o Escape fecha o modal; não descarta o banner de
      // erro na UI de fundo.
      if (isModalOpen()) return;
      if (e.key === 'Escape' && sendError) {
        setSendError(null);
        setLastFailedMessage(null);
        announce(t('chat.announce.errorDismissed'));
      }
    };

    document.addEventListener('keydown', handleEscape);
    return () => document.removeEventListener('keydown', handleEscape);
  }, [isInteractiveSurface, sendError, announce, t]);

  const handleSendMessage = async (content: string, mediaFiles?: MediaFile[]) => {
    try {
      setSendError(null);
      setLastFailedMessage(null);
      await controller.sendMessage(content, mediaFiles);
    } catch (error: unknown) {
      const errorMessage = error instanceof Error ? error.message : String(error);
      logger.error('[ChatSessionView] send error:', errorMessage);
      setLastFailedMessage({ content, media: mediaFiles });
      setSendError(ErrorMessages.CHAT.SEND_FAILED);

      handleError(error, {
        source: 'ChatSessionView.handleSendMessage',
        userMessage: ErrorMessages.CHAT.SEND_FAILED,
        severity: ErrorSeverity.RECOVERABLE,
        onRetry: () => handleRetry(),
      });
    }
  };

  const handleRetry = async () => {
    if (!lastFailedMessage) return;

    try {
      setSendError(null);
      await controller.sendMessage(lastFailedMessage.content, lastFailedMessage.media);
      setLastFailedMessage(null);
    } catch (error) {
      handleError(error, {
        source: 'ChatSessionView.handleRetry',
        userMessage: ErrorMessages.CHAT.SEND_FAILED,
        severity: ErrorSeverity.RECOVERABLE,
      });
    }
  };

  const handleReachEnd = () => {
    inputRef.current?.focus();
  };

  const handleJumpToStart = async () => {
    const previousWindowKey = latestWindowKeyRef.current;
    const windowState = session?.messageWindow;
    pendingWindowAnnouncementRef.current = {
      kind: 'start',
      previousStartIndex: windowState?.startIndex ?? 0,
      previousEndIndex: windowState?.endIndex ?? -1,
      previousWindowKey,
    };
    try {
      await loadStartMessages();
      requestAnimationFrame(() => {
        const container = messagesContainerRef.current;
        const firstMessage = container?.querySelector('[data-message-node]') as HTMLElement | null;
        firstMessage?.focus();
      });
    } finally {
      window.setTimeout(() => {
        if (pendingWindowAnnouncementRef.current?.previousWindowKey === latestWindowKeyRef.current) {
          pendingWindowAnnouncementRef.current = null;
        }
      }, 1_000);
    }
  };

  const handleJumpToEnd = async () => {
    const previousWindowKey = latestWindowKeyRef.current;
    const windowState = session?.messageWindow;
    pendingWindowAnnouncementRef.current = {
      kind: 'end',
      previousStartIndex: windowState?.startIndex ?? 0,
      previousEndIndex: windowState?.endIndex ?? -1,
      previousWindowKey,
    };
    try {
      await loadEndMessages();
      requestAnimationFrame(() => {
        const container = messagesContainerRef.current;
        const rootMessages = container?.querySelectorAll<HTMLElement>('[data-message-node][data-level="0"]');
        const lastMessage = rootMessages?.[rootMessages.length - 1] ?? null;
        lastMessage?.focus();
      });
    } finally {
      window.setTimeout(() => {
        if (pendingWindowAnnouncementRef.current?.previousWindowKey === latestWindowKeyRef.current) {
          pendingWindowAnnouncementRef.current = null;
        }
      }, 1_000);
    }
  };

  const handleLoadOlderMessages = useCallback(async () => {
    const previousWindowKey = latestWindowKeyRef.current;
    const windowState = session?.messageWindow;
    pendingWindowAnnouncementRef.current = {
      kind: 'older',
      previousStartIndex: windowState?.startIndex ?? 0,
      previousEndIndex: windowState?.endIndex ?? -1,
      previousWindowKey,
    };
    try {
      await loadOlderMessages();
    } finally {
      window.setTimeout(() => {
        if (pendingWindowAnnouncementRef.current?.previousWindowKey === latestWindowKeyRef.current) {
          pendingWindowAnnouncementRef.current = null;
        }
      }, 1_000);
    }
  }, [loadOlderMessages, session?.messageWindow]);

  const handleLoadNewerMessages = useCallback(async () => {
    const previousWindowKey = latestWindowKeyRef.current;
    const windowState = session?.messageWindow;
    pendingWindowAnnouncementRef.current = {
      kind: 'newer',
      previousStartIndex: windowState?.startIndex ?? 0,
      previousEndIndex: windowState?.endIndex ?? -1,
      previousWindowKey,
    };
    try {
      await loadNewerMessages();
    } finally {
      window.setTimeout(() => {
        if (pendingWindowAnnouncementRef.current?.previousWindowKey === latestWindowKeyRef.current) {
          pendingWindowAnnouncementRef.current = null;
        }
      }, 1_000);
    }
  }, [loadNewerMessages, session?.messageWindow]);

  const rootClass =
    variant === 'page' ? 'chat-page chat-session-view' : 'chat-session-view chat-session-view--embedded';

  return (
    <div className={rootClass} ref={rootRef}>
      <div className="ws-content-toolbar">
        <ChatToolbar
          inputRef={inputRef}
          conversationId={conversationId}
          enableShortcuts={isInteractiveSurface}
          onRequestConversationChange={onRequestConversationChange}
        />
      </div>
      <div className="ws-content-area">
        <MessageList
          threadedMessages={threadedMessages}
          messageWindow={session?.messageWindow}
          onLoadChildren={loadMessageChildren}
          onReachEnd={handleReachEnd}
          isLoading={isLoading}
          hasOlderMessages={hasOlderMessages}
          hasNewerMessages={hasNewerMessages}
          isLoadingOlderMessages={isLoadingOlderMessages}
          isLoadingMessageWindow={isLoadingMessageWindow}
          onLoadOlder={handleLoadOlderMessages}
          onLoadNewer={handleLoadNewerMessages}
          onJumpToStart={handleJumpToStart}
          onJumpToEnd={handleJumpToEnd}
          ref={messagesContainerRef}
          onContextMenu={(event, message) => showMenu(event, message, message.role === 'user')}
          onSpeak={hasVoiceConfig ? speakMessage : undefined}
          onDelete={handleDeleteMessage}
          editorTargets={editorTargets}
          onSendToEditor={sendToEditor}
        />

        {sendError && lastFailedMessage && (
          <Alert
            role="alert"
            type="error"
            showIcon
            closable
            message={sendError}
            action={
              <Button
                ref={retryButtonRef}
                size="small"
                danger
                onClick={handleRetry}
                aria-label={t('chat.retryAriaLabel')}
              >
                {t('chat.retry')}
              </Button>
            }
            onClose={() => {
              setSendError(null);
              setLastFailedMessage(null);
            }}
            style={{ flexShrink: 0 }}
          />
        )}

        <ChatInput
          onSend={handleSendMessage}
          disabled={variant === 'embedded' ? false : isLoading}
          isStreaming={isLoading}
          onCancelStreaming={() => void handleCancelStreaming()}
          ref={inputRef}
          voiceEnabled={true}
          message={draftMessage}
          mediaFiles={draftMediaFiles}
          onMessageChange={setDraftMessage}
          onMediaFilesChange={setDraftMediaFiles}
          profileSlug={profileSlug || activeProfileSlug}
          onArrowUp={() => {
            const container = messagesContainerRef.current;
            if (!container) return;
            // Durante o streaming a última mensagem de nível 0 é a que está em
            // curso; focá-la diretamente (em vez de `:last-child`, frágil quando
            // a lista é virtualizada ou há nós auxiliares no fim) garante entrar
            // na lista de forma navegável. (Issue #178)
            const rootMessages = container.querySelectorAll<HTMLElement>('[data-message-node][data-level="0"]');
            const lastMessage = rootMessages[rootMessages.length - 1] ?? null;
            if (lastMessage) {
              lastMessage.focus();
            } else {
              container.focus();
            }
          }}
        />
      </div>

      <ContextMenu
        visible={menuVisible}
        items={menuItems}
        x={menuPosition.x}
        y={menuPosition.y}
        onClose={hideMenu}
        ariaLabel={t('chat.contextMenuAriaLabel')}
      />
    </div>
  );
}
