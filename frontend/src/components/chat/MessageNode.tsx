import { logger } from '../../utils/logger';
import React, { useState, useCallback, useEffect, useLayoutEffect, useId } from 'react';
import { useTranslation } from 'react-i18next';
import { ChatMessage } from './ChatMessage';
import { MessageNode as MessageNodeType, Message, useChatStore } from '../../store/chatStore';
import { useChatNodeSessionState } from './ChatSessionContext';
import { playBumpSound } from '../../services/audioFeedback';
import { useChatMessageEditDraft } from './useChatMessageEditDraft';
import { announce } from '../../hooks/useAnnouncer';
import { useVirtualModal } from '../../hooks/useVirtualModal';
import { messageAudioService } from '../../services/messageAudio';
import type { EditorSendTargetOption, ChatSendToEditorPayload } from '../../lib/editorSendMenu';
import { ttsService } from '../../services/tts';
import { useAuthStore } from '../../store/authStore';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { useOptionalWorkspacePanel } from '../workspace/WorkspacePanelContext';
import { useModalId } from '../ui/Modal';
import { captureChatNavigationTarget, registerChatNavigationSurface, requestChatNavigationCommand, type ChatNavigationCommandID, type ChatNavigationTarget } from '../../lib/commandChatNavigation';
import './MessageNode.css';

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
  onCopy?: (message: Message, markdown: boolean) => void;
  onEdit?: (message: Message) => void;
  onSaveEdit?: (message: Message) => void;
  commandPathname?: string;
  onDelete?: (message: Message) => void;
  editorTargets?: EditorSendTargetOption[];
  onSendToEditor?: (payload: ChatSendToEditorPayload) => void;
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
  onCopy,
  onEdit,
  onSaveEdit,
  commandPathname,
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
    sessionKey,
    editingMessageId,
    streamingMessageId,
    streamingReasoning,
    isThinking: isThinkingGlobal,
    activeToolCalls,
    completedSegments,
    isExpanded,
    reasoningExpanded,
    setConversationEditingMessageId,
    toggleConversationThreadExpanded,
    toggleConversationReasoningExpanded,
  } = useChatNodeSessionState(messageId);
  
  const [isLoading, setIsLoading] = useState(false);
  const editDraft = useChatMessageEditDraft(nodeRef, node.message, conversationId, sessionKey, commandPathname);
  const { isEditing, editContent } = editDraft;
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

  // Detecta edição acionada externamente (pelo menu de contexto)
  useEffect(() => {
    if (editingMessageId === node.message.id && !isEditing) {
      // Só permite editar mensagens do usuário
      if (node.message.role === 'user' && !node.message.internal && !node.message.isStreaming) {
        editDraft.open();
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

  const navigationInstanceId = useId();
  const panel = useOptionalWorkspacePanel();
  const modalId = useModalId();
  const owner = useAuthStore(state => state.user);
  const navigationLive = React.useRef({ node, conversationId, sessionKey, commandPathname, panel, isReading, isEditing, onContextMenu, onLoadChildren, streamingMessageId, streamingReasoning });
  navigationLive.current = { node, conversationId, sessionKey, commandPathname, panel, isReading, isEditing, onContextMenu, onLoadChildren, streamingMessageId, streamingReasoning };
  const navigationListeners = React.useRef(new Set<() => void>());
  const mounted = React.useRef(false);
  const pendingExpansion = React.useRef<{ lease: ChatNavigationTarget; root: HTMLElement; ready: boolean; focusChild: boolean } | null>(null);
  const loadingRef = React.useRef(false);

  const requestNavigation = (id: ChatNavigationCommandID) => requestChatNavigationCommand(id, navigationInstanceId);
  const startThreadExpansion = () => {
    const live = navigationLive.current;
    const root = nodeRef.current;
    if (!root || !live.conversationId || loadingRef.current) return false;
    // This separate lease belongs to the asynchronous child-load lifetime,
    // not to the central dispatcher's synchronous presentation effect.
    const lease = captureChatNavigationTarget(() => navigationLive.current.commandPathname ?? '', 'chat.message.thread.expand', navigationInstanceId);
    if (!lease) return false;
    pendingExpansion.current?.lease.dispose();
    const pending = { lease, root, ready: (live.node.children?.length ?? 0) > 0, focusChild: document.activeElement === root };
    pendingExpansion.current = pending;
    if (!lease.isCurrent()) { lease.dispose(); pendingExpansion.current = null; return false; }
    if (!pending.ready && live.onLoadChildren) {
      loadingRef.current = true;
      setIsLoading(true);
    }
    toggleConversationThreadExpanded(live.conversationId, live.node.message.id);
    if (!pending.ready && live.onLoadChildren) {
      void (async () => {
        try {
          if (!lease.isCurrent()) return;
          await live.onLoadChildren!(live.node.message.id);
          if (lease.isCurrent()) pending.ready = true;
        } catch (error) {
          if (lease.isCurrent()) logger.error('[MessageNode] Error loading children:', error);
          lease.dispose();
        } finally {
          if (mounted.current && pendingExpansion.current === pending) {
            loadingRef.current = false;
            setIsLoading(false);
          }
        }
      })();
    }
    return true;
  };

  useLayoutEffect(() => {
    navigationListeners.current.forEach(changed => changed());
    const pending = pendingExpansion.current;
    if (!pending) return;
    if (!pending.lease.isCurrent()) {
      pending.lease.dispose();
      pendingExpansion.current = null;
      if (loadingRef.current) { loadingRef.current = false; setIsLoading(false); }
      return;
    }
    if (!pending.ready) return;
    if (pending.focusChild && document.activeElement === pending.root) {
      pending.root.querySelector<HTMLElement>(':scope > .message-node__children > .message-node')?.focus();
    }
    pending.lease.dispose();
    pendingExpansion.current = null;
  });
  useLayoutEffect(() => {
    const root = nodeRef.current;
    const workspace = useWorkspaceStore.getState().workspace;
    if (!root || !owner || !workspace || !panel || !conversationId || !sessionKey || !commandPathname) return;
    mounted.current = true;
    const current = () => {
      const live = navigationLive.current;
      return mounted.current && nodeRef.current === root && live.panel?.isActive === true &&
        live.panel.tab.id === panel.tab.id && live.conversationId === conversationId && live.sessionKey === sessionKey &&
        useChatStore.getState().surfaceSessionsByKey[sessionKey]?.conversationId === conversationId &&
        useChatStore.getState().getConversationMessages(conversationId).some(message => message === live.node.message);
    };
    const off = registerChatNavigationSurface({
      root, instanceId: navigationInstanceId,
      allowedCommands: ['chat.message.read.open', 'chat.message.menu.open', 'chat.message.reasoning.toggle', 'chat.message.thread.expand', 'chat.message.thread.collapse'],
      readContext: () => ({ pathname: navigationLive.current.commandPathname ?? '', ownerId: owner.userId, sessionId: owner.sessionId,
        workspaceId: workspace.id, tabId: panel.tab.id, conversationId, chatSessionKey: sessionKey, modalId: modalId ?? undefined,
        messageId: navigationLive.current.node.message.id, message: navigationLive.current.node.message }),
      isCurrent: current,
      subscribe(changed) {
        navigationListeners.current.add(changed);
        const unsubscribe = useChatStore.subscribe(changed);
        return () => { navigationListeners.current.delete(changed); unsubscribe(); };
      },
      canOpen(id, target) {
        const live = navigationLive.current;
        const message = live.node.message;
        if (!current() || live.isReading || live.isEditing ||
          target instanceof Element && target !== root && !!target.closest('input,textarea,select,[contenteditable="true"]')) return false;
        const expanded = useChatStore.getState().isConversationThreadExpanded(conversationId, message.id, sessionKey);
        if (id === 'chat.message.read.open') return !message.internal;
        if (id === 'chat.message.menu.open') return !message.internal && !!live.onContextMenu;
        if (id === 'chat.message.reasoning.toggle') return message.role === 'assistant' &&
          (!!message.reasoning || message.id === live.streamingMessageId && !!live.streamingReasoning);
        if (id === 'chat.message.thread.expand') return !expanded && !loadingRef.current && ((live.node.children?.length ?? 0) > 0 || live.node.childCount > 0 && !!live.onLoadChildren);
        if (id === 'chat.message.thread.collapse') return expanded;
        return false;
      },
      open(id) {
        const live = navigationLive.current;
        if (id === 'chat.message.read.open') {
          root.focus();
          if (document.activeElement !== root) return false;
          setIsReading(true);
          return true;
        }
        if (id === 'chat.message.reasoning.toggle') {
          const wasExpanded = useChatStore.getState().isConversationReasoningExpanded(conversationId, live.node.message.id, sessionKey);
          toggleConversationReasoningExpanded(conversationId, live.node.message.id);
          announce(t(wasExpanded ? 'chat.reasoningHidden' : 'chat.reasoningShown'));
          return true;
        }
        if (id === 'chat.message.thread.expand') return startThreadExpansion();
        if (id === 'chat.message.thread.collapse') {
          pendingExpansion.current?.lease.dispose(); pendingExpansion.current = null;
          loadingRef.current = false; setIsLoading(false);
          toggleConversationThreadExpanded(conversationId, live.node.message.id);
          return true;
        }
        if (id === 'chat.message.menu.open' && live.onContextMenu) {
          const rect = root.getBoundingClientRect();
          live.onContextMenu({ preventDefault() {}, stopPropagation() {}, clientX: rect.left + rect.width / 2,
            clientY: rect.top + rect.height / 2, currentTarget: root, target: root } as unknown as React.MouseEvent, live.node.message);
          return true;
        }
        return false;
      },
    });
    return () => { mounted.current = false; off(); pendingExpansion.current?.lease.dispose(); pendingExpansion.current = null; loadingRef.current = false; };
  }, [navigationInstanceId, conversationId, sessionKey, panel?.tab.id, owner?.userId, owner?.sessionId, modalId, commandPathname]);

  const handleToggle = () => requestNavigation(isExpanded ? 'chat.message.thread.collapse' : 'chat.message.thread.expand');

  const isInternal = node.message.internal || level > 0;

  // Handlers de edição
  const handleSaveEdit = () => {
    onSaveEdit?.(node.message);
  };

  const handleCancelEdit = () => {
    editDraft.cancel();
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

  const expandAndFocusFirst = () => {
    if (!hasChildren) return;
    if (!isExpanded) requestNavigation('chat.message.thread.expand');
    else focusFirstChild();
  };

  // Navegação por teclado (como no Svelte)
  const handleKeyDown = async (e: React.KeyboardEvent) => {
    const key = e.key;
    if (e.target instanceof Element && e.target.closest('.message-node') !== nodeRef.current) return;
    if (e.defaultPrevented || e.nativeEvent.isComposing || e.keyCode === 229) return;
    if (e.repeat && (key === 'Enter' || key.toLowerCase() === 'r' || key === ' ' || key === 'F2' || key === 'Delete' || e.ctrlKey && key.toLowerCase() === 'c')) return;
    if (e.target instanceof Element && e.target.closest('input,textarea,select,[contenteditable="true"]')) return;

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

    // Durante a leitura isolada, a árvore deixa de funcionar como item da
    // lista: links, botões e o role=document precisam receber suas teclas
    // nativamente. Escape continua sob responsabilidade do useVirtualModal.
    if (isReading) {
      e.stopPropagation();
      return;
    }

    // Native controls own their activation keys; the message is not a second
    // Enter/Space handler for a thread button, link or context-menu trigger.
    if (e.target instanceof Element && e.target !== nodeRef.current &&
      e.target.closest('button,a[href],summary,[role="button"],[role="link"],[role="menuitem"],[role="combobox"]')) return;

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
    if (key === 'Enter' && !e.ctrlKey && !e.altKey && !e.metaKey && !e.shiftKey && !node.message.internal) {
      e.preventDefault();
      e.stopPropagation();
      requestNavigation('chat.message.read.open');
      return;
    }

    // F2: edita mensagem (somente mensagens do usuário)
    if (key === 'F2' && node.message.role === 'user' && !node.message.internal && !node.message.isStreaming) {
      e.preventDefault();
      e.stopPropagation();
      if (!e.repeat && !e.nativeEvent.isComposing && e.keyCode !== 229) onEdit?.(node.message);
      return;
    }

    // Delete: deleta mensagem
    if (
      key === 'Delete'
      && !node.message.internal
      && !node.message.isStreaming
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
      if (!e.repeat && !e.nativeEvent.isComposing && e.keyCode !== 229) onCopy?.(node.message, e.shiftKey);
      return;
    }

    // R keeps its native node ingress but delegates the effect to the registry.
    if ((key === 'r' || key === 'R') && !e.ctrlKey && !e.altKey && !e.metaKey && node.message.role === 'assistant' && node.message.reasoning) {
      e.preventDefault();
      e.stopPropagation();
      requestNavigation('chat.message.reasoning.toggle');
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
        requestNavigation('chat.message.thread.collapse');
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
        requestNavigation('chat.message.thread.collapse');
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

  // ContextMenu/Shift+F10 keep native release semantics; the effect is shared.
  const handleKeyUp = (e: React.KeyboardEvent) => {
    if (e.defaultPrevented || e.repeat || e.nativeEvent.isComposing || e.keyCode === 229 || e.ctrlKey || e.altKey || e.metaKey ||
      e.target instanceof Element && (e.target.closest('.message-node') !== nodeRef.current || !!e.target.closest('input,textarea,select,[contenteditable="true"]'))) return;
    if ((e.key === 'ContextMenu' || e.shiftKey && e.key === 'F10') && !node.message.internal) {
      if (requestNavigation('chat.message.menu.open')) { e.preventDefault(); e.stopPropagation(); }
    }
  };

  return (
    <div
      ref={nodeRef}
      className={`message-node message-node--level-${level} ${isInternal ? 'message-node--internal' : ''} ${isReading ? 'message-node--reading' : ''}`}
      data-level={level}
      data-sibling-index={siblingIndex}
      data-chat-navigation-instance={navigationInstanceId}
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
          onContextMenu={(event) => { event.preventDefault(); event.stopPropagation(); requestNavigation('chat.message.menu.open'); }}
          onSpeak={handleSpeak}
          editorTargets={editorTargets}
          onSendToEditor={onSendToEditor}
          isReading={isReading}
          isEditing={isEditing}
          editContent={editContent}
          onEditContentChange={editDraft.change}
          onSaveEdit={handleSaveEdit}
          onCancelEdit={handleCancelEdit}
          // Reasoning/Thinking - passa apenas para a mensagem em streaming
          streamingReasoning={node.message.id === streamingMessageId ? (streamingReasoning || undefined) : undefined}
          isThinking={node.message.id === streamingMessageId ? isThinkingGlobal : false}
          isReasoningExpanded={reasoningExpanded}
          onToggleReasoning={() => requestNavigation('chat.message.reasoning.toggle')}
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
              onCopy={onCopy}
              onEdit={onEdit}
              onSaveEdit={onSaveEdit}
              commandPathname={commandPathname}
              onDelete={onDelete}
              // Não passa onReachEnd para threads internas
            />
          ))}
        </div>
      )}
    </div>
  );
});
