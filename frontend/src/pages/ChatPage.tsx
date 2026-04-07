import { useRef, useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button } from 'antd';
import { useChatStore } from '../store/chatStore';
import { useEditorStore } from '../store/editorStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { ttsService } from '../services/tts';
import { MessageList } from '../components/chat/MessageList';
import { ChatInput } from '../components/chat/ChatInput';
import { ChatToolbar } from '../components/chat/ChatToolbar';
import { ContextMenu } from '../components/menu';
import { KeyboardShortcutsHelp } from '../components/ui/KeyboardShortcutsHelp';
import { useChatKeyboardNav } from '../hooks/useChatKeyboardNav';
import { useTabScrollState } from '../hooks/useTabScrollState';
import { useContextMenu, useMessageActions } from '../hooks/useContextMenu';
import { MediaFile } from '../services/mediaService';
import { DeleteMessage } from '@wailsjs/go/main/App';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { announce } from '../hooks/useAnnouncer';
import { handleError, ErrorSeverity, ErrorMessages } from '../utils/errorHandler';
import './ChatPage.css';

export default function ChatPage() {
  const { t } = useTranslation();
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  useTabScrollState(messagesContainerRef);
  const hasAutoFocusedRef = useRef(false);
  const retryButtonRef = useRef<HTMLButtonElement>(null);
  const wasLoadingRef = useRef(false);

  const {
    isLoading,
    sendMessage,
    getThreadedMessages,
    loadMessageChildren,
    getActiveConversation,
    loadConversation,
    updateMessage,
    toggleReasoningExpanded,
    isReasoningExpanded,
  } = useChatStore();

  // TTS disponível apenas quando há voz configurada no perfil ativo
  const [hasVoiceConfig, setHasVoiceConfig] = useState(() => ttsService.hasVoiceConfig());
  useEffect(() => {
    const handler = () => setHasVoiceConfig(ttsService.hasVoiceConfig());
    ttsService.on('voiceConfigChanged', handler);
    return () => { ttsService.off('voiceConfigChanged', handler); };
  }, []);
  const isTTSDisabled = !hasVoiceConfig;

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
        announce(t('chat.announce.messageDeleted'));
        const conv = getActiveConversation();
        if (conv?.id) {
          await loadConversation(conv.id);
        }
      }
    } catch (error) {
      // Se o usuário cancelou a exclusão, não mostra erro
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
        source: 'ChatPage.onDelete',
        userMessage: ErrorMessages.CHAT.DELETE_FAILED,
        severity: ErrorSeverity.RECOVERABLE,
        metadata: { messageId },
      });
    }
  }, [announce, getActiveConversation, loadConversation, t]);

  // Menu de contexto
  const sendToEditor = useCallback(
    async (payload: {
      target: 'current' | 'new_document';
      format: 'markdown' | 'html' | 'plain';
      title?: string;
      content: string;
    }) => {
      const content = String(payload?.content ?? '');
      if (!content) return;

      useEditorStore.getState().requestInsert({
        target: payload.target,
        format: payload.format,
        title: payload.title || t('editor.fallback.fromChat'),
        content,
        focus: true,
      });

      const { workspace, addTab, setActiveTab } = useWorkspaceStore.getState();
      const existingEditor = workspace?.tabs.find(tab => tab.type === 'editor');
      if (existingEditor) {
        void setActiveTab(existingEditor.id);
      } else {
        const tabId = await addTab('editor', t('workspace.newEditor', 'Novo documento'));
        void setActiveTab(tabId);
      }
    },
    [t]
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
        announce(t('chat.announce.messageResent'));
      }
    },
    onDelete: handleDeleteMessage,
    onSendToEditor: (payload) => sendToEditor(payload),
    onPin: (_message) => {
      // Pin requer campo adicional no modelo ChatMessage
      // Deixar para implementação futura
      announce(t('chat.announce.pinComingSoon'));
    },
    onToggleReasoning: (message) => {
      toggleReasoningExpanded(message.id);
      const isExpanded = isReasoningExpanded(message.id);
      announce(isExpanded ? t('chat.reasoningHidden') : t('chat.reasoningShown'));
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
      if (eventData.message_id && eventData.content !== undefined) {
        updateMessage(String(eventData.message_id), eventData.content);
      }
    };

    const unsubscribe = EventsOn('message:updated', handleMessageUpdated);
    return () => {
      if (unsubscribe) unsubscribe();
    };
  }, [updateMessage]);

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
        announce(t('chat.announce.errorDismissed'));
      }
    };

    document.addEventListener('keydown', handleEscape);
    return () => document.removeEventListener('keydown', handleEscape);
  }, [sendError, announce, t]);


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
      <div className="ws-content-toolbar">
        <ChatToolbar inputRef={inputRef} />
      </div>
      <div className="ws-content-area">
      <MessageList
        threadedMessages={threadedMessages}
        onLoadChildren={loadMessageChildren}
        onReachEnd={handleReachEnd}
        isLoading={isLoading}
        ref={messagesContainerRef}
        onContextMenu={(event, message) => showMenu(event, message, message.role === 'user')}
        onSpeak={hasVoiceConfig ? speakMessage : undefined}
        onDelete={handleDeleteMessage}
        onSendToEditor={(payload) => sendToEditor(payload)}
      />

      {/* Error banner with retry */}
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
        disabled={isLoading}
        ref={inputRef}
        voiceEnabled={true}
        onArrowUp={() => {
          const container = messagesContainerRef.current;
          if (container) {
            const lastMessage = container.querySelector('[data-message-node]:last-child') as HTMLElement;
            if (lastMessage) {
              lastMessage.focus();
            } else {
              container.focus();
            }
          }
        }}
      />
      </div>

      {/* Menu de contexto */}
      <ContextMenu
        visible={menuVisible}
        items={menuItems}
        x={menuPosition.x}
        y={menuPosition.y}
        onClose={hideMenu}
        ariaLabel={t('chat.contextMenuAriaLabel')}
      />

      {/* Painel de ajuda de atalhos de teclado */}
      <KeyboardShortcutsHelp
        isOpen={shortcutsHelpOpen}
        onClose={() => setShortcutsHelpOpen(false)}
      />
    </div>
  );
}
