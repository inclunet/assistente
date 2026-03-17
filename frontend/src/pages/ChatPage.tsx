import { useRef, useState, useEffect, useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { useChatStore } from '../store/chatStore';
import { useEditorStore } from '../store/editorStore';
import { ttsService } from '../services/tts';
import { MessageList } from '../components/chat/MessageList';
import { ChatInput } from '../components/chat/ChatInput';
import { ChatToolbar } from '../components/chat/ChatToolbar';
import { ChatTabs } from '../components/tabs/ChatTabs';
import { ContextMenu } from '../components/menu';
import { KeyboardShortcutsHelp } from '../components/ui/KeyboardShortcutsHelp';
import { useChatKeyboardNav } from '../hooks/useChatKeyboardNav';
import { useTabsKeyboardShortcuts } from '../hooks/useTabsKeyboardShortcuts';
import { useLandmarkNavigation } from '../hooks/useLandmarkNavigation';
import { useContextMenu, useMessageActions } from '../hooks/useContextMenu';
import { MediaFile } from '../services/mediaService';
import { DeleteMessage } from '@wailsjs/go/main/App';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { announce } from '../hooks/useAnnouncer';
import { handleError, ErrorSeverity, ErrorMessages } from '../utils/errorHandler';
import './ChatPage.css';

export default function ChatPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const hasAutoFocusedRef = useRef(false);
  const retryButtonRef = useRef<HTMLButtonElement>(null);
  const wasLoadingRef = useRef(false);

  const {
    isLoading,
    sendMessage,
    getThreadedMessages,
    loadMessageChildren,
    getActiveTab,
    loadConversationInActiveTab,
    updateMessage,
    toggleReasoningExpanded,
    isReasoningExpanded,
  } = useChatStore();

  // TTS é controlado pelo perfil global via ttsService (fonte de verdade)
  const isTTSDisabled = !ttsService.isEnabled();

  // Estado do painel de ajuda de atalhos
  const [shortcutsHelpOpen, setShortcutsHelpOpen] = useState(false);


  // Estado de erro para recovery
  const [lastFailedMessage, setLastFailedMessage] = useState<{ content: string; media?: MediaFile[] } | null>(null);
  const [sendError, setSendError] = useState<string | null>(null);

  // Edição de mensagem via store (acionado pelo menu de contexto)
  const startEditing = useChatStore(state => state.startEditing);
  // Modo leitura via store (acionado pelo menu de contexto)
  const startReading = useChatStore(state => state.startReading);

  // Ações de mensagem
  const { copyMessage, speakMessage } = useMessageActions({
    onAnnounce: announce,
  });

  // Função para deletar mensagem (usada tanto no menu quanto no teclado)
  const handleDeleteMessage = useCallback(async (message: { id: string | number }) => {
    let messageId: number | null = null;
    try {
      const parsedId = typeof message.id === 'number' ? message.id : parseInt(String(message.id), 10);
      messageId = Number.isNaN(parsedId) ? null : parsedId;
      if (messageId !== null) {
        await DeleteMessage(messageId);
        announce('Mensagem excluída');
        // Recarrega a conversa atual sem page reload
        const activeTab = getActiveTab();
        if (activeTab?.conversationId) {
          await loadConversationInActiveTab(activeTab.conversationId, activeTab.title || 'Conversa');
        }
      }
    } catch (error) {
      // Se o usuário cancelou a exclusão, não mostra erro
      const errorMessage = error instanceof Error ? error.message : String(error);
      if (errorMessage.includes('cancelada')) {
        announce('Exclusão cancelada');
        return;
      }
      
      handleError(error, {
        source: 'ChatPage.onDelete',
        userMessage: ErrorMessages.CHAT.DELETE_FAILED,
        severity: ErrorSeverity.RECOVERABLE,
        metadata: { messageId },
      });
    }
  }, [announce, getActiveTab, loadConversationInActiveTab]);

  // Menu de contexto
  const sendToEditor = useCallback(
    (payload: {
      target: 'current' | 'new_tab';
      format: 'markdown' | 'html' | 'plain';
      title?: string;
      content: string;
    }) => {
      const chatState = useChatStore.getState();
      const chatTabId = chatState.activeTabId;
      const chatTab = chatTabId ? chatState.tabs.find((t) => t.id === chatTabId) : undefined;
      const conversationId = typeof chatTab?.conversationId === 'number' ? chatTab.conversationId : null;

      const content = String(payload?.content ?? '');
      if (!content) return;

      useEditorStore.getState().requestInsert({
        target: payload.target,
        format: payload.format,
        title: payload.title || 'Do chat',
        content,
        focus: true,
        source: {
          chatTabId: chatTabId ?? null,
          conversationId,
          messageId: null,
        },
      });

      navigate('/editor');
    },
    [navigate]
  );

  const { menuVisible, menuPosition, menuItems, showMenu, hideMenu } = useContextMenu({
    onCopy: copyMessage,
    onReadMessage: (message) => {
      startReading(message.id);
    },
    onSpeak: speakMessage,
    onEdit: (message) => {
      // Aciona a edição via store - também marca para pular restauração de foco
      startEditing(message.id);
    },
    onResend: async (message) => {
      if (message.content) {
        await sendMessage(message.content);
        announce('Mensagem reenviada');
      }
    },
    onDelete: handleDeleteMessage,
    onSendToEditor: (payload) => sendToEditor(payload),
    onPin: (_message) => {
      // Pin requer campo adicional no modelo ChatMessage
      // Deixar para implementação futura
      announce('Funcionalidade de fixar mensagem será implementada em breve');
    },
    onToggleReasoning: (message) => {
      toggleReasoningExpanded(message.id);
      const isExpanded = isReasoningExpanded(message.id);
      announce(isExpanded ? 'Raciocínio ocultado' : 'Raciocínio exibido');
    },
    isReasoningExpanded: (messageId: string) => isReasoningExpanded(messageId),
    isTTSDisabled,
  });

  // Enable keyboard navigation for chat messages
  useChatKeyboardNav({
    enabled: true,
    inputRef,
    messagesContainerRef,
  });

  // Enable global keyboard shortcuts for tabs (Ctrl+T, Ctrl+W, Ctrl+Tab, etc.)
  useTabsKeyboardShortcuts();

  // F6: circular foco entre guias, toolbar, histórico do chat e campo de mensagem
  useLandmarkNavigation({
    landmarks: useMemo(() => [
      {
        id: 'tabs',
        label: t('landmarks.tabs'),
        focus: () => {
          const active = document.querySelector('.chat-tabs [role="tab"][aria-selected="true"]') as HTMLElement | null;
          const anyTab = document.querySelector('.chat-tabs [role="tab"]') as HTMLElement | null;
          (active || anyTab)?.focus();
          return !!(active || anyTab);
        },
        contains: () => !!document.activeElement?.closest?.('.chat-tabs'),
      },
      {
        id: 'toolbar',
        label: t('landmarks.toolbar'),
        focus: () => {
          const toolbar = document.querySelector('.chat-page [role="toolbar"]') as Element | null;
          if (!toolbar) return false;
          const btn = toolbar.querySelector('button:not([disabled])') as HTMLButtonElement | null;
          if (!btn) return false;
          btn.focus();
          return true;
        },
        contains: () => !!document.activeElement?.closest?.('[role="toolbar"]'),
      },
      {
        id: 'chatHistory',
        label: t('landmarks.chatHistory'),
        focus: () => {
          const container = messagesContainerRef.current;
          if (!container) return false;
          const lastMsg = container.querySelector('[data-message-node]:last-child') as HTMLElement | null;
          if (lastMsg) { lastMsg.focus(); return true; }
          container.setAttribute('tabindex', '-1');
          container.focus();
          return true;
        },
        contains: () => !!document.activeElement?.closest?.('.message-list'),
      },
      {
        id: 'chatInput',
        label: t('landmarks.chatInput'),
        focus: () => {
          inputRef.current?.focus();
          return !!inputRef.current;
        },
        contains: () => !!document.activeElement?.closest?.('.chat-input'),
      },
    ], [t]),
    defaultLandmarkId: 'chatInput',
  });

  // Usa os MessageNode[] que o backend já enviou com childCount correto
  const threadedMessages = getThreadedMessages() || [];

  // Auto-focus no input ao carregar a página - apenas uma vez
  useEffect(() => {
    // Aguarda o componente renderizar completamente
    const checkTimer = setInterval(() => {
      const inputElement = inputRef.current;
      if (inputElement && !hasAutoFocusedRef.current) {
        hasAutoFocusedRef.current = true;
        clearInterval(checkTimer);
        
        // Pequeno delay para garantir que tudo está pronto
        setTimeout(() => {
          inputElement.focus();
        }, 100);
      }
    }, 100);
    
    return () => {
      clearInterval(checkTimer);
    };
  }, []); // Array vazio - roda apenas no mount

  // Restaura foco no input quando streaming termina (isLoading: true → false)
  useEffect(() => {
    if (wasLoadingRef.current && !isLoading) {
      const active = document.activeElement as HTMLElement | null;
      const isEditingMessage = active?.closest('.chat-message--editing') !== null;
      if (!isEditingMessage) {
        requestAnimationFrame(() => {
          inputRef.current?.focus();
        });
      }
    }
    wasLoadingRef.current = isLoading;
  }, [isLoading]);

  // Keyboard shortcut to open help (? key)
  useEffect(() => {
    const handleKeyPress = (e: KeyboardEvent) => {
      // Only open if not in an input/textarea and not already open
      const target = e.target as HTMLElement;
      const isInputElement = target.tagName === 'INPUT' || target.tagName === 'TEXTAREA';

      if (e.key === '?' && !isInputElement && !shortcutsHelpOpen) {
        e.preventDefault();
        setShortcutsHelpOpen(true);
      }
    };

    document.addEventListener('keypress', handleKeyPress);
    return () => document.removeEventListener('keypress', handleKeyPress);
  }, [shortcutsHelpOpen]);

  // Listen for message:updated events from backend
  useEffect(() => {
    const handleMessageUpdated = (data: unknown) => {
      const eventData = data as { message_id?: number | string; content?: string };
      // Em vez de recarregar toda a conversa, atualiza apenas a mensagem na store
      // Isso preserva o estado de foco e evita re-renders desnecessários
      const activeTab = getActiveTab();
      if (activeTab && eventData.message_id && eventData.content !== undefined) {
        // Atualiza a mensagem localmente na store (sem recarregar toda a conversa)
        updateMessage(activeTab.id, String(eventData.message_id), eventData.content);
      }
    };

    const unsubscribe = EventsOn('message:updated', handleMessageUpdated);
    return () => {
      if (unsubscribe) unsubscribe();
    };
  }, [getActiveTab, updateMessage]);

  // Auto-focus retry button when error banner appears
  useEffect(() => {
    if (sendError && retryButtonRef.current) {
      retryButtonRef.current.focus();
    }
  }, [sendError]);

  // Escape key to dismiss error banner
  useEffect(() => {
    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && sendError) {
        setSendError(null);
        setLastFailedMessage(null);
        announce('Erro descartado');
      }
    };

    document.addEventListener('keydown', handleEscape);
    return () => document.removeEventListener('keydown', handleEscape);
  }, [sendError]);


  const handleSendMessage = async (content: string, mediaFiles?: MediaFile[]) => {
    try {
      setSendError(null); // Clear previous error
      setLastFailedMessage(null);
      await sendMessage(content, mediaFiles);
    } catch (error: unknown) {
      const errorMessage = error instanceof Error ? error.message : String(error);
      console.error('[ChatPage] sendMessage error details:', errorMessage);
      // Store failed message for retry
      setLastFailedMessage({ content, media: mediaFiles });
      setSendError(ErrorMessages.CHAT.SEND_FAILED);

      handleError(error, {
        source: 'ChatPage.handleSendMessage',
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
      await sendMessage(lastFailedMessage.content, lastFailedMessage.media);
      setLastFailedMessage(null); // Clear on success
    } catch (error) {
      handleError(error, {
        source: 'ChatPage.handleRetry',
        userMessage: ErrorMessages.CHAT.SEND_FAILED,
        severity: ErrorSeverity.RECOVERABLE,
      });
    }
  };

  const handleReachEnd = () => {
    // Quando chega ao fim da lista principal, foca no input
    inputRef.current?.focus();
  };

  return (
    <div className="chat-page">
      <ChatTabs />
      <ChatToolbar inputRef={inputRef} />
      <MessageList
        threadedMessages={threadedMessages}
        onLoadChildren={loadMessageChildren}
        onReachEnd={handleReachEnd}
        isLoading={isLoading}
        ref={messagesContainerRef}
        onContextMenu={(event, message) => showMenu(event, message, message.role === 'user')}
        onSpeak={speakMessage}
        onDelete={handleDeleteMessage}
        onSendToEditor={(payload) => sendToEditor(payload)}
      />

      {/* Error banner with retry */}
      {sendError && lastFailedMessage && (
        <div
          className="chat-page__error-banner"
          role="alert"
          aria-live="assertive"
        >
          <span className="chat-page__error-message">{sendError}</span>
          <button
            ref={retryButtonRef}
            className="chat-page__retry-button"
            onClick={handleRetry}
            aria-label="Tentar enviar novamente"
          >
            Tentar novamente
          </button>
          <button
            className="chat-page__dismiss-button"
            onClick={() => {
              setSendError(null);
              setLastFailedMessage(null);
            }}
            aria-label="Descartar erro"
          >
            ✕
          </button>
        </div>
      )}

      <ChatInput 
        onSend={handleSendMessage} 
        disabled={isLoading}
        ref={inputRef}
        voiceEnabled={true}
        onArrowUp={() => {
          const container = messagesContainerRef.current;
          if (container) {
            // Foca na última mensagem da lista
            const lastMessage = container.querySelector('[data-message-node]:last-child') as HTMLElement;
            if (lastMessage) {
              lastMessage.focus();
            } else {
              // Se não houver mensagens em estrutura de árvore, foca no container
              container.focus();
            }
          }
        }}
      />

      {/* Menu de contexto */}
      <ContextMenu
        visible={menuVisible}
        items={menuItems}
        x={menuPosition.x}
        y={menuPosition.y}
        onClose={hideMenu}
        ariaLabel="Ações da mensagem"
      />

      {/* Painel de ajuda de atalhos de teclado */}
      <KeyboardShortcutsHelp
        isOpen={shortcutsHelpOpen}
        onClose={() => setShortcutsHelpOpen(false)}
      />
    </div>
  );
}
