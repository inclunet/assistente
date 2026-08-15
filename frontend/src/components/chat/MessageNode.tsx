import { logger } from '../../utils/logger';
import React, { useState, useCallback, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { ChatMessage } from './ChatMessage';
import { MessageNode as MessageNodeType, Message } from '../../store/chatStore';
import { useChatNodeSessionState } from './ChatSessionContext';
import { playBumpSound } from '../../services/audioFeedback';
import { UpdateMessage } from '@wailsjs/go/app/App';
import { announce } from '../../hooks/useAnnouncer';
import { useVirtualModal } from '../../hooks/useVirtualModal';
import { handleError, ErrorSeverity } from '../../utils/errorHandler';
import { messageAudioService } from '../../services/messageAudio';
import type { EditorSendTargetOption, SendToEditorPayload } from '../../lib/editorSendMenu';
import { ttsService } from '../../services/tts';
import './MessageNode.css';

const TOOL_ONLY_TURN_PLACEHOLDER_SOURCE = 'tool_only_turn_placeholder';

export interface MessageNodeProps {
  node: MessageNodeType;
  level?: number;
  siblingIndex?: number;
  siblingCount?: number;
  ariaPosition?: number;
  ariaSetSize?: number;
  onLoadChildren?: (messageId: string) => Promise<MessageNodeType[]>;
  onReachEnd?: () => void; // Chamado quando tenta ir além do último item no level 0
  onReachStart?: () => void | Promise<void>;
  /**
   * Quando a lista de nível 0 está virtualizada, a navegação por irmãos não pode
   * depender de `parentElement.children` (apenas itens visíveis existem no DOM).
   * A MessageList fornece este callback para rolar o índice até a viewport e focá-lo.
   */
  onFocusSiblingIndex?: (index: number) => void;
  onJumpToStart?: () => void | Promise<void>;
  onJumpToEnd?: () => void | Promise<void>;
  onContextMenu?: (e: React.MouseEvent, message: Message) => void;
  onSpeak?: (message: Message) => void;
  onDelete?: (message: Message) => void;
  editorTargets?: EditorSendTargetOption[];
  onSendToEditor?: (payload: SendToEditorPayload) => void;
}

export const MessageNode: React.FC<MessageNodeProps> = React.memo(({
  node,
  level = 0,
  siblingIndex = 0,
  siblingCount = 1,
  ariaPosition,
  ariaSetSize,
  onLoadChildren,
  onReachEnd,
  onReachStart,
  onJumpToStart,
  onJumpToEnd,
  onContextMenu,
  onSpeak,
  onDelete,
  editorTargets,
  onSendToEditor,
  onFocusSiblingIndex,
}) => {
  const { t } = useTranslation();
  const nodeRef = React.useRef<HTMLDivElement>(null);
  
  // IMPORTANTE: messageId deve ser definido primeiro, pois é usado em hooks abaixo
  const messageId = node.message.id;
  const {
    conversationId,
    editingMessageId,
    readingMessageId,
    streamingMessageId,
    streamingReasoning,
    isThinking: isThinkingGlobal,
    activeToolCalls,
    completedSegments,
    isExpanded,
    reasoningExpanded,
    setConversationEditingMessageId,
    setConversationReadingMessageId,
    toggleConversationThreadExpanded,
    toggleConversationReasoningExpanded,
  } = useChatNodeSessionState(messageId);
  
  const [isLoading, setIsLoading] = useState(false);
  const [isEditing, setIsEditing] = useState(false);
  const [editContent, setEditContent] = useState(node.message.content);
  const [isReading, setIsReading] = useState(false);
  const [isPlayingAudio, setIsPlayingAudio] = useState(false);

  // Virtual modal: transforma a mensagem em "dialog" para leitores de tela
  useVirtualModal({
    elementRef: nodeRef,
    isActive: isReading,
    onClose: () => setIsReading(false),
    openAnnouncement: t('chat.readingOpen'),
    closeAnnouncement: t('chat.readingClose'),
    dialogLabel: t('chat.readingDialog'),
  });
  
  // SIMPLIFICADO: Usa apenas node.children da store
  // - loadMessageChildren atualiza node.children na store
  // - addInternalMessage também atualiza node.children na store
  // - Não precisamos de estado local duplicado
  const children = node.children || [];

  // Detecta modo leitura acionado externamente (pelo menu de contexto)
  useEffect(() => {
    if (readingMessageId === node.message.id && !isReading) {
      if (!node.message.internal) {
        setIsReading(true);
      }
      // Limpa o estado na store
      if (conversationId) {
        setConversationReadingMessageId(conversationId, null);
      }
    }
  }, [conversationId, readingMessageId, node.message.id, node.message.internal, isReading, setConversationReadingMessageId]);

  // Detecta edição acionada externamente (pelo menu de contexto)
  useEffect(() => {
    if (editingMessageId === node.message.id && !isEditing) {
      // Só permite editar mensagens do usuário
      if (node.message.role === 'user' && !node.message.internal && !node.message.isStreaming) {
        setIsEditing(true);
        setEditContent(node.message.content);
        announce(t('chat.editingMessage'));
      }
      // Limpa o estado na store
      if (conversationId) {
        setConversationEditingMessageId(conversationId, null);
      }
    }
  }, [conversationId, editingMessageId, node.message.id, node.message.role, node.message.internal, node.message.isStreaming, node.message.content, isEditing, setConversationEditingMessageId]);

  // Handler de speak que controla o estado de playback
  const handleSpeak = useCallback(async (message: Message) => {
    // Se qualquer áudio está tocando (local, global/autoplay ou TTS API), para
    if (isPlayingAudio || messageAudioService.isCurrentlyPlaying() || ttsService.isSpeaking()) {
      messageAudioService.stopCurrentAudio();
      ttsService.stop();
      setIsPlayingAudio(false);
      return;
    }
    setIsPlayingAudio(true);
    try {
      if (onSpeak) {
        await onSpeak(message);
      }
    } finally {
      setIsPlayingAudio(false);
    }
  }, [isPlayingAudio, onSpeak]);

  const hasChildren = node.childCount > 0 || children.length > 0;

  const handleToggle = useCallback(async () => {
    if (!hasChildren) return;

    const wasExpanded = isExpanded;
    
    // Alterna expansão na store
    if (!conversationId) return;
    toggleConversationThreadExpanded(conversationId, node.message.id);
    
    // Aguarda um tick para garantir atualização do estado
    await new Promise(resolve => setTimeout(resolve, 0));
    
    // Se estava fechado e tem filhos para carregar (childCount > 0 mas children.length === 0)
    // Isso acontece quando os filhos ainda não foram carregados do banco
    if (!wasExpanded && children.length === 0 && node.childCount > 0 && onLoadChildren) {
      setIsLoading(true);
      try {
        // onLoadChildren atualiza node.children na store, causando re-render automático
        await onLoadChildren(node.message.id);
      } catch (error) {
        logger.error('[MessageNode] Error loading children:', error);
      } finally {
        setIsLoading(false);
      }
    }
  }, [conversationId, hasChildren, isExpanded, toggleConversationThreadExpanded, node.message.id, node.childCount, children.length, onLoadChildren]);

  const isInternal = node.message.internal || level > 0;
  const isToolOnlyTurnPlaceholder = node.message.source === TOOL_ONLY_TURN_PLACEHOLDER_SOURCE;

  // Handlers de edição
  const handleSaveEdit = async () => {
    if (!editContent.trim()) return;

    try {
      const messageId = node.message.id;
      await UpdateMessage(messageId, editContent);
      announce(t('chat.messageEdited'));
      setIsEditing(false);
      
      // Restaura o foco para a mensagem após salvar
      requestAnimationFrame(() => {
        nodeRef.current?.focus();
      });
    } catch (error) {
      handleError(error, {
        source: 'MessageNode.handleSaveEdit',
        userMessage: t('chat.editSaveError'),
        severity: ErrorSeverity.RECOVERABLE,
      });
    }
  };

  const handleCancelEdit = () => {
    setEditContent(node.message.content);
    setIsEditing(false);
    announce(t('chat.editCancelled'));
    
    // Restaura o foco para a mensagem após cancelar
    requestAnimationFrame(() => {
      nodeRef.current?.focus();
    });
  };

  // Funções de navegação por DOM (como no Svelte)
  const focusSibling = (idx: number) => {
    // Em listas virtualizadas (nível 0), delega para a MessageList rolar o índice
    // até a viewport antes de focar — nem todos os irmãos existem no DOM.
    if (onFocusSiblingIndex) {
      onFocusSiblingIndex(idx);
      return;
    }

    if (!nodeRef.current) return;
    
    const parent = nodeRef.current.parentElement;
    if (parent) {
      const siblings = Array.from(parent.children);
      const sibling = siblings[idx] as HTMLElement;
      if (sibling) {
        sibling.focus();
        return;
      }
    }
  };

  const focusParent = () => {
    if (!nodeRef.current || level === 0) return;
    
    // Estrutura: div > div.children > div (filho)
    const parentContainer = nodeRef.current.parentElement;
    if (parentContainer && parentContainer.classList.contains('message-node__children')) {
      const parentNode = parentContainer.parentElement;
      if (parentNode) {
        parentNode.focus();
      }
    }
  };

  const focusFirstChild = () => {
    if (!nodeRef.current) return;
    
    const childrenContainer = nodeRef.current.querySelector('.message-node__children');
    if (childrenContainer) {
      const firstChild = childrenContainer.firstElementChild as HTMLElement;
      if (firstChild) {
        firstChild.focus();
      }
    }
  };

  const expandAndFocusFirst = async () => {
    if (!hasChildren) return;
    
    if (!isExpanded) {
      await handleToggle();
      // Aguarda renderização dos filhos
      setTimeout(() => {
        focusFirstChild();
      }, 100);
    } else {
      // Já expandido, apenas foca no primeiro filho
      focusFirstChild();
    }
  };
  
  // Navegação por teclado (como no Svelte)
  const handleKeyDown = async (e: React.KeyboardEvent) => {
    const key = e.key;

    // Se está editando, deixar o editor tratar todas as teclas
    // Verifica também se o foco está em um textarea ou button (editor)
    const activeElement = document.activeElement;
    const isInEditor = activeElement?.tagName === 'TEXTAREA' ||
                       (activeElement?.tagName === 'BUTTON' && isEditing);

    if (isEditing || isInEditor) {
      // Parar a propagação para que outros handlers não capturem o evento
      e.stopPropagation();
      // Não prevenir default para deixar o textarea processar normalmente
      return;
    }

    // Espaço: reproduz TTS da mensagem
    if (key === ' ' && !node.message.isStreaming) {
      e.preventDefault();
      e.stopPropagation();
      if (onSpeak) {
        void handleSpeak(node.message);
      }
      return;
    }
    
    // Enter ativa modo de leitura (virtual modal)
    if (key === 'Enter' && !node.message.internal) {
      e.preventDefault();
      e.stopPropagation();
      setIsReading(true);
      return;
    }

    // F2: edita mensagem (somente mensagens do usuário)
    if (key === 'F2' && node.message.role === 'user' && !node.message.internal && !node.message.isStreaming) {
      e.preventDefault();
      e.stopPropagation();
      setIsEditing(true);
      setEditContent(node.message.content);
      announce(t('chat.editingMessage'));
      return;
    }

    // Delete: deleta mensagem
    if (
      key === 'Delete'
      && !node.message.internal
      && !node.message.isStreaming
      && !isToolOnlyTurnPlaceholder
      && onDelete
    ) {
      e.preventDefault();
      e.stopPropagation();
      onDelete(node.message);
      return;
    }

    // Ctrl+C: copia conteúdo da mensagem
    if (e.ctrlKey && key === 'c' && !e.altKey && !node.message.internal) {
      e.preventDefault();
      e.stopPropagation();
      const textToCopy = e.shiftKey 
        ? `[${node.message.role}] ${node.message.content}` // Ctrl+Shift+C: com role
        : node.message.content; // Ctrl+C: apenas conteúdo
      navigator.clipboard.writeText(textToCopy);
      announce(e.shiftKey ? t('chat.copiedWithRole') : t('chat.contentCopied'));
      return;
    }

    // R: toggle do reasoning (somente mensagens do assistente com reasoning)
    if ((key === 'r' || key === 'R') && node.message.role === 'assistant' && node.message.reasoning) {
      e.preventDefault();
      e.stopPropagation();
      if (!conversationId) return;
      toggleConversationReasoningExpanded(conversationId, node.message.id);
      // O estado é lido pela store, então precisamos verificar o novo estado
      const isNowExpanded = !reasoningExpanded; // Toggle do estado atual
      announce(isNowExpanded ? t('chat.reasoningShown') : t('chat.reasoningHidden'));
      return;
    }

    // ArrowDown: navega para próximo irmão
    if (key === 'ArrowDown') {
      e.preventDefault();
      e.stopPropagation();
      if (siblingIndex < siblingCount - 1) {
        focusSibling(siblingIndex + 1);
      } else if (level === 0 && onReachEnd) {
        // No nível principal, ao chegar no fim, vai para o input
        onReachEnd();
      } else if (level > 0) {
        // Em threads (level > 0), toca som ao tentar ir além
        playBumpSound();
      }
      // Nota: no level 0 não toca som porque vai para o input
      return;
    }
    
    // ArrowUp: navega para irmão anterior
    if (key === 'ArrowUp') {
      e.preventDefault();
      e.stopPropagation();
      if (siblingIndex > 0) {
        focusSibling(siblingIndex - 1);
      } else if (level === 0 && onReachStart) {
        await onReachStart();
      } else {
        playBumpSound();
      }
      return;
    }
    
    // ArrowRight: expande E foca no primeiro filho
    if (key === 'ArrowRight') {
      e.preventDefault();
      e.stopPropagation();
      if (hasChildren) {
        await expandAndFocusFirst();
      }
      return;
    }
    
    // ArrowLeft: colapsa thread ou volta para o pai
    if (key === 'ArrowLeft') {
      e.preventDefault();
      e.stopPropagation();
      if (isExpanded && hasChildren) {
        if (conversationId) {
          toggleConversationThreadExpanded(conversationId, node.message.id);
        }
      } else if (level > 0) {
        focusParent();
      }
      return;
    }

    // Escape: colapsa thread, volta ao pai, ou deixa borbulhar para ir à área padrão
    if (key === 'Escape') {
      if (isExpanded && hasChildren) {
        e.preventDefault();
        e.stopPropagation();
        if (conversationId) {
          toggleConversationThreadExpanded(conversationId, node.message.id);
        }
      } else if (level > 0) {
        e.preventDefault();
        e.stopPropagation();
        focusParent();
      }
      // Level 0 + não expandido: não intercepta, deixa borbulhar
      // para o sistema de landmarks redirecionar à área padrão
      return;
    }
    
    if (key === 'Home' && e.ctrlKey && level === 0 && onJumpToStart) {
      e.preventDefault();
      e.stopPropagation();
      await onJumpToStart();
      return;
    }

    if (key === 'End' && e.ctrlKey && level === 0 && onJumpToEnd) {
      e.preventDefault();
      e.stopPropagation();
      await onJumpToEnd();
      return;
    }

    // Home: foca no primeiro irmão
    if (key === 'Home' && !e.ctrlKey) {
      e.preventDefault();
      e.stopPropagation();
      focusSibling(0);
      return;
    }
    
    // End: foca no último irmão
    if (key === 'End' && !e.ctrlKey) {
      e.preventDefault();
      e.stopPropagation();
      focusSibling(siblingCount - 1);
      return;
    }
    
    // Page Down: pula 10 mensagens para baixo
    if (key === 'PageDown' && !e.ctrlKey) {
      e.preventDefault();
      e.stopPropagation();
      const targetIndex = Math.min(siblingIndex + 10, siblingCount - 1);
      focusSibling(targetIndex);
      if (targetIndex === siblingCount - 1 && siblingIndex === targetIndex) {
        if (level === 0 && onReachEnd) {
          onReachEnd();
        } else {
          playBumpSound();
        }
      }
      return;
    }
    
    // Page Up: pula 10 mensagens para cima
    if (key === 'PageUp' && !e.ctrlKey) {
      e.preventDefault();
      e.stopPropagation();
      const targetIndex = Math.max(siblingIndex - 10, 0);
      focusSibling(targetIndex);
      if (targetIndex === 0 && siblingIndex === 0) {
        if (level === 0 && onReachStart) {
          await onReachStart();
        } else {
          playBumpSound();
        }
      }
      return;
    }
  };

  // Handler para onKeyUp - captura ContextMenu key ou Shift+F10
  const handleKeyUp = (e: React.KeyboardEvent) => {
    if ((e.key === 'ContextMenu' || (e.shiftKey && e.key === 'F10')) && !node.message.internal) {
      e.preventDefault();
      e.stopPropagation();
      if (onContextMenu && nodeRef.current) {
        // Simula evento de mouse no centro do elemento
        const rect = nodeRef.current.getBoundingClientRect();
        const syntheticEvent = {
          preventDefault: () => {},
          stopPropagation: () => {},
          clientX: rect.left + rect.width / 2,
          clientY: rect.top + rect.height / 2,
          currentTarget: nodeRef.current, // Para restaurar foco após fechar menu
          target: nodeRef.current,
        } as unknown as React.MouseEvent;
        onContextMenu(syntheticEvent, node.message);
      }
    }
  };

  return (
    <div
      ref={nodeRef}
      className={`message-node message-node--level-${level} ${isInternal ? 'message-node--internal' : ''} ${isReading ? 'message-node--reading' : ''}`}
      data-level={level}
      data-sibling-index={siblingIndex}
      data-message-node
      data-message-id={node.message.id}
      onKeyDown={handleKeyDown}
      onKeyUp={handleKeyUp}
      tabIndex={-1}
      role="listitem"
      aria-posinset={ariaPosition}
      aria-setsize={ariaSetSize}
      aria-expanded={hasChildren ? isExpanded : undefined}
    >
      <div className="message-node__content">
        <ChatMessage
          message={node.message}
          hasThreadIndicator={hasChildren}
          threadChildCount={node.childCount || children.length}
          isThreadExpanded={isExpanded}
          isThreadLoading={isLoading}
          onThreadToggle={handleToggle}
          onContextMenu={onContextMenu}
          onSpeak={handleSpeak}
          editorTargets={editorTargets}
          onSendToEditor={onSendToEditor}
          isReading={isReading}
          isEditing={isEditing}
          editContent={editContent}
          onEditContentChange={setEditContent}
          onSaveEdit={handleSaveEdit}
          onCancelEdit={handleCancelEdit}
          // Reasoning/Thinking - passa apenas para a mensagem em streaming
          streamingReasoning={node.message.id === streamingMessageId ? (streamingReasoning || undefined) : undefined}
          isThinking={node.message.id === streamingMessageId ? isThinkingGlobal : false}
          isReasoningExpanded={reasoningExpanded}
          onToggleReasoning={() => {
            if (conversationId) {
              toggleConversationReasoningExpanded(conversationId, node.message.id);
            }
          }}
          // Tool calling - passa apenas para a mensagem em streaming
          activeToolCalls={node.message.id === streamingMessageId ? activeToolCalls : undefined}
          completedSegments={node.message.id === streamingMessageId ? completedSegments : undefined}
          isPlayingAudio={isPlayingAudio}
        />
      </div>

      {isExpanded && children.length > 0 && (
        <div className="message-node__children" role="list" aria-label="Respostas internas">
          {children.map((childNode, index) => (
            <MessageNode
              key={childNode.message.id || index}
              node={childNode}
              level={level + 1}
              siblingIndex={index}
              siblingCount={children.length}
              onLoadChildren={onLoadChildren}
              onContextMenu={onContextMenu}
              onSpeak={onSpeak}
              onDelete={onDelete}
              // Não passa onReachEnd para threads internas
            />
          ))}
        </div>
      )}
    </div>
  );
});
