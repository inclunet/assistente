import { logger } from '../../utils/logger';
import React, { useEffect, useLayoutEffect, useRef, useMemo, useState, useCallback, forwardRef } from 'react';
import { defaultRangeExtractor, useVirtualizer, type Range } from '@tanstack/react-virtual';
import { useTranslation } from 'react-i18next';
import { MessageOutlined } from '@ant-design/icons';
import { MessageNode as MessageNodeComponent } from './MessageNode';
import { MessageNode, Message } from '../../store/chatStore';
import type { EditorSendTargetOption, ChatSendToEditorPayload } from '../../lib/editorSendMenu';
import { getTimelineNodeKey, isPersistedTimelineNode, type MessageWindowState } from '../../services/chatSessionRegistry';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import type { VoiceAccessibilityOrigin } from '../../services/voiceAccessibility/types';
import './MessageList.css';

/**
 * Como a janela de mensagens foi carregada. `scroll` é automático (a lista
 * chegou perto da borda sozinha, inclusive ao rolar para o fim quando uma
 * resposta termina); `navigation` é a pessoa navegando de propósito.
 */
export type MessageWindowLoadTrigger = 'scroll' | 'navigation';

export interface MessageListProps {
  isLoading?: boolean;
  loadingText?: string; // Optional custom loading text
  // Estrutura hierárquica de mensagens (threads)
  threadedMessages: MessageNode[];
  messageWindow?: MessageWindowState;
  // Callback para carregar filhos de uma mensagem
  onLoadChildren?: (messageId: string) => Promise<MessageNode[]>;
  // Callback quando chega ao fim da lista principal
  onReachEnd?: () => void;
  onReachStart?: () => void | Promise<void>;
  hasOlderMessages?: boolean;
  hasNewerMessages?: boolean;
  isLoadingOlderMessages?: boolean;
  isLoadingMessageWindow?: boolean;
  onLoadOlder?: (trigger: MessageWindowLoadTrigger) => Promise<void> | void;
  onLoadNewer?: (trigger: MessageWindowLoadTrigger) => Promise<void> | void;
  onJumpToStart?: () => Promise<void> | void;
  onJumpToEnd?: () => Promise<void> | void;
  // Callbacks de ações
  onContextMenu?: (event: React.MouseEvent, message: Message) => void;
  onSpeak?: (message: Message) => void;
  onCopy?: (message: Message, markdown: boolean) => void;
  onEdit?: (message: Message) => void;
  onSaveEdit?: (message: Message) => void;
  commandPathname?: string;
  onDelete?: (message: Message) => void;
  editorTargets?: EditorSendTargetOption[];
  onSendToEditor?: (payload: ChatSendToEditorPayload) => void;
  origin?: VoiceAccessibilityOrigin;
}

/**
 * Acima deste número de mensagens de nível 0, a lista passa a renderizar apenas
 * os itens visíveis (virtualização). Conversas curtas seguem o caminho simples,
 * preservando o comportamento histórico e mantendo o DOM totalmente disponível
 * para tecnologias assistivas e navegação por teclado.
 */
const VIRTUALIZATION_THRESHOLD = 40;
/** Altura inicial estimada por mensagem antes da medição real (px). */
const ESTIMATED_MESSAGE_HEIGHT = 140;
/** Quantos itens extras renderizar acima/abaixo da viewport. */
const VIRTUAL_OVERSCAN = 6;

const isPersistedMessageNode = (node: MessageNode | undefined): boolean => {
  if (!node) return false;
  return isPersistedTimelineNode(node);
};

export const MessageList = React.memo(forwardRef<HTMLDivElement, MessageListProps>((
  {
    isLoading = false,
    loadingText,
    threadedMessages,
    messageWindow,
    onLoadChildren,
    onReachEnd,
    onReachStart,
    hasOlderMessages = false,
    hasNewerMessages = false,
    isLoadingOlderMessages = false,
    isLoadingMessageWindow = false,
    onLoadOlder,
    onLoadNewer,
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
    origin,
  },
  ref
) => {
  const { t } = useTranslation();
  const { announceRequest } = useAnnouncer();
  const effectiveLoadingText = loadingText ?? t('chat.typing');
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const previousLoadingAnnouncementRef = useRef<string | null>(null);
  // Fonte de verdade do elemento de scroll. Sempre apontado por um callback ref
  // próprio, então `.current` é confiável mesmo quando o ref externo é um callback.
  const innerContainerRef = useRef<HTMLDivElement | null>(null);
  const pendingScrollRestoreRef = useRef<{ scrollHeight: number; scrollTop: number } | null>(null);
  const suppressNextScrollLoadRef = useRef(false);
  const suppressScrollLoadTimerRef = useRef<number | null>(null);

  // Callback ref que mantém `innerContainerRef` como fonte de verdade e encaminha
  // o ref externo do forwardRef. Trata os dois formatos válidos do React:
  // função (callback ref) ou objeto mutável (RefObject); ignora null/undefined.
  const setContainerRef = useCallback((node: HTMLDivElement | null) => {
    innerContainerRef.current = node;
    if (typeof ref === 'function') {
      ref(node);
    } else if (ref) {
      (ref as React.MutableRefObject<HTMLDivElement | null>).current = node;
    }
  }, [ref]);

  useEffect(() => {
    const loadingAnnouncementKey = [
      effectiveLoadingText,
      origin?.surfaceType ?? '',
      origin?.surfaceId ?? '',
      origin?.sessionKey ?? '',
      origin?.tabId ?? '',
      origin?.conversationId ?? '',
    ].join('|');
    if (isLoading && previousLoadingAnnouncementRef.current !== loadingAnnouncementKey) {
      const didAnnounce = announceRequest({
        message: effectiveLoadingText,
        origin,
        eventType: 'progress',
      });
      if (didAnnounce) {
        previousLoadingAnnouncementRef.current = loadingAnnouncementKey;
      }
      return;
    }
    if (!isLoading) {
      previousLoadingAnnouncementRef.current = null;
    }
  }, [announceRequest, effectiveLoadingText, isLoading, origin]);

  // A timeline e o turnPatch já chegam consolidados pelo backend. Mensagens
  // role=tool existem somente no contexto transitório do loop, nunca na UI.
  const displayMessages = useMemo(
    () => threadedMessages.filter((node) => node.message.role !== 'tool'),
    [threadedMessages]
  );
  const canLoadNewerFromDisplayEnd = isPersistedMessageNode(displayMessages[displayMessages.length - 1]);
  const hasConsolidatedTurns = displayMessages.length !== threadedMessages.length;
  const messagePositions = useMemo(() => {
    // Until AEP-0059 phase 2.1 moves absolute counts to canonical timeline items,
    // raw message indices would announce jumps for locally consolidated turns.
    if (!messageWindow || messageWindow.totalCount <= 0 || hasConsolidatedTurns) {
      return displayMessages.map((_, index) => index + 1);
    }
    const explicitIndexes = displayMessages.map((node) => node.originalIndex);
    const canUseAbsolutePositions = explicitIndexes.every((index): index is number => index !== undefined)
      && explicitIndexes.every((index, position) => position === 0 || index > explicitIndexes[position - 1]!);
    if (!canUseAbsolutePositions) {
      return displayMessages.map((_, index) => index + 1);
    }
    return explicitIndexes.map((index) => index + 1);
  }, [displayMessages, hasConsolidatedTurns, messageWindow]);
  const usesAbsoluteMessagePositions = !!messageWindow
    && messageWindow.totalCount > 0
    && !hasConsolidatedTurns
    && displayMessages.length > 0
    && displayMessages.every((node, index) => (
      node.originalIndex !== undefined
      && (index === 0 || node.originalIndex > displayMessages[index - 1].originalIndex!)
    ));
  const ariaSetSize = usesAbsoluteMessagePositions
    ? Math.max(messageWindow.totalCount, ...messagePositions)
    : displayMessages.length;

  // Virtualização: conversas longas renderizam apenas os itens visíveis.
  const listRef = useRef<HTMLDivElement>(null);
  const shouldVirtualize = displayMessages.length > VIRTUALIZATION_THRESHOLD;
  const [scrollMargin, setScrollMargin] = useState(0);
  const [focusedKey, setFocusedKey] = useState<string | null>(null);
  const focusedKeyRef = useRef<string | null>(null);
  const focusedIndex = focusedKey === null ? -1 : displayMessages.findIndex(node => getTimelineNodeKey(node) === focusedKey);
  const rangeExtractor = useCallback((range: Range) => {
    const visible = defaultRangeExtractor(range);
    return focusedIndex < 0 || visible.includes(focusedIndex) ? visible : [...visible, focusedIndex].sort((a, b) => a - b);
  }, [focusedIndex]);

  const rowVirtualizer = useVirtualizer({
    count: displayMessages.length,
    enabled: shouldVirtualize,
    getScrollElement: () => innerContainerRef.current,
    estimateSize: () => ESTIMATED_MESSAGE_HEIGHT,
    overscan: VIRTUAL_OVERSCAN,
    scrollMargin,
    rangeExtractor,
    // Chave estável por nó preserva a medição ao paginar histórico (prepend/append).
    getItemKey: (index) => getTimelineNodeKey(displayMessages[index]),
  });

  // Move o foco para um irmão de nível 0 pelo índice. Quando virtualizado, rola
  // o índice até a viewport antes (nem todos os irmãos estão montados no DOM).
  // Retorna `true` quando o nó foi encontrado e focado.
  const focusSiblingByIndex = useCallback((index: number): boolean => {
    if (displayMessages.length === 0) return false;
    const clamped = Math.max(0, Math.min(index, displayMessages.length - 1));
    if (shouldVirtualize) {
      rowVirtualizer.scrollToIndex(clamped, { align: 'auto' });
    }
    const el = listRef.current?.querySelector(
      `[data-message-node][data-level="0"][data-sibling-index="${clamped}"]`
    ) as HTMLElement | null;
    if (el) {
      el.focus();
      return true;
    }
    return false;
  }, [displayMessages.length, rowVirtualizer, shouldVirtualize]);

  // Foca um irmão de nível 0 quando virtualizado: rola o índice até a viewport
  // (pode não estar montado) e então move o foco para o nó correspondente.
  const focusMessageAtIndex = useCallback((index: number) => {
    if (!focusSiblingByIndex(index)) {
      requestAnimationFrame(() => {
        focusSiblingByIndex(index);
      });
    }
  }, [focusSiblingByIndex]);

  // Mede o deslocamento da lista dentro do container rolável (botão "carregar
  // anteriores" ocupa espaço acima da lista virtualizada).
  useLayoutEffect(() => {
    if (!shouldVirtualize) return;
    const container = innerContainerRef.current;
    const list = listRef.current;
    if (!container || !list) return;
    const containerRect = container.getBoundingClientRect();
    const listRect = list.getBoundingClientRect();
    const nextMargin = listRect.top - containerRect.top + container.scrollTop;
    setScrollMargin((prev) => (Math.abs(prev - nextMargin) > 1 ? nextMargin : prev));
  }, [shouldVirtualize, hasOlderMessages, isLoadingOlderMessages, displayMessages.length]);

  useEffect(() => {
    if (!import.meta.env.DEV) return;

    const seen = new Set<string>();
    const duplicates = new Set<string>();

    for (const node of displayMessages) {
      const key = getTimelineNodeKey(node);
      if (seen.has(key)) {
        duplicates.add(key);
      } else {
        seen.add(key);
      }
    }

    if (duplicates.size === 0) return;
    logger.warn('[MessageList] duplicate timeline keys detected in display messages', Array.from(duplicates));
  }, [displayMessages, threadedMessages]);

  // Retenção visual por identidade, inclusive quando o foco está em um controle
  // ou filho da mensagem. Paginação pode mover o índice sem mudar o proprietário.
  const handleFocusCapture = (event: React.FocusEvent<HTMLDivElement>) => {
    const row = event.target.closest('[data-timeline-key]');
    const key = row && event.currentTarget.contains(row) ? row.getAttribute('data-timeline-key') : null;
    focusedKeyRef.current = key;
    setFocusedKey(key);
  };
  const handleBlurCapture = (event: React.FocusEvent<HTMLDivElement>) => {
    if (event.relatedTarget instanceof Node && event.currentTarget.contains(event.relatedTarget)) return;
    focusedKeyRef.current = null;
    setFocusedKey(null);
  };

  // Remoção/exclusão não transfere a interação para outro item no mesmo índice.
  useLayoutEffect(() => {
    if (focusedIndex < 0) {
      focusedKeyRef.current = null;
      setFocusedKey(null);
    }
  }, [focusedIndex]);

  const scrollToBottom = (behavior: ScrollBehavior = 'smooth') => {
    // Atualização de conteúdo não deve afastar a mensagem em interação/leitura.
    if (focusedKeyRef.current !== null) return;
    suppressNextScrollLoadRef.current = true;
    if (shouldVirtualize && displayMessages.length > 0) {
      // Garante que o último item esteja montado/medido antes do ajuste final.
      rowVirtualizer.scrollToIndex(displayMessages.length - 1, { align: 'end' });
    }
    messagesEndRef.current?.scrollIntoView({ behavior });
    if (suppressScrollLoadTimerRef.current !== null) {
      window.clearTimeout(suppressScrollLoadTimerRef.current);
    }
    suppressScrollLoadTimerRef.current = window.setTimeout(() => {
      suppressNextScrollLoadRef.current = false;
      suppressScrollLoadTimerRef.current = null;
    }, behavior === 'smooth' ? 500 : 0);
  };

  const handleLoadOlder = useCallback((trigger: MessageWindowLoadTrigger) => {
    const container = innerContainerRef.current;
    const snapshot = container
      ? { scrollHeight: container.scrollHeight, scrollTop: container.scrollTop }
      : null;
    pendingScrollRestoreRef.current = snapshot;

    const result = onLoadOlder?.(trigger);
    void Promise.resolve(result).finally(() => {
      window.setTimeout(() => {
        if (pendingScrollRestoreRef.current === snapshot) {
          pendingScrollRestoreRef.current = null;
        }
      }, 0);
    });
  }, [onLoadOlder]);

  const handleLoadNewer = useCallback((trigger: MessageWindowLoadTrigger) => {
    const result = onLoadNewer?.(trigger);
    void Promise.resolve(result);
  }, [onLoadNewer]);

  const handleReachStart = useCallback(() => {
    if (hasOlderMessages && onLoadOlder && !isLoadingMessageWindow) {
      handleLoadOlder('navigation');
      return;
    }
    void Promise.resolve(onReachStart?.());
  }, [handleLoadOlder, hasOlderMessages, isLoadingMessageWindow, onLoadOlder, onReachStart]);
  const effectiveReachStart = useMemo(
    () => ((hasOlderMessages && onLoadOlder) || onReachStart ? handleReachStart : undefined),
    [handleReachStart, hasOlderMessages, onLoadOlder, onReachStart],
  );

  const handleReachEnd = useCallback(() => {
    if (hasNewerMessages && onLoadNewer && !isLoadingMessageWindow && canLoadNewerFromDisplayEnd) {
      handleLoadNewer('navigation');
      return;
    }
    onReachEnd?.();
  }, [canLoadNewerFromDisplayEnd, handleLoadNewer, hasNewerMessages, isLoadingMessageWindow, onLoadNewer, onReachEnd]);

  useLayoutEffect(() => {
    const pendingRestore = pendingScrollRestoreRef.current;
    const container = innerContainerRef.current;
    if (pendingRestore && container) {
      const heightDelta = container.scrollHeight - pendingRestore.scrollHeight;
      container.scrollTop = pendingRestore.scrollTop + heightDelta;
      pendingScrollRestoreRef.current = null;
      return;
    }

    scrollToBottom();
  }, [displayMessages]);

  useEffect(() => {
    // Non-animated scroll on mount.
    scrollToBottom('auto');
  }, []);

  useEffect(() => {
    const container = innerContainerRef.current;
    if (!container) return;
    const handleScroll = () => {
      if (suppressNextScrollLoadRef.current) {
        return;
      }
      if (container.scrollTop < 48 && hasOlderMessages && !isLoadingMessageWindow) {
        handleLoadOlder('scroll');
        return;
      }
      const distanceToBottom = container.scrollHeight - container.scrollTop - container.clientHeight;
      if (distanceToBottom < 48 && hasNewerMessages && !isLoadingMessageWindow && canLoadNewerFromDisplayEnd) {
        handleLoadNewer('scroll');
      }
    };
    container.addEventListener('scroll', handleScroll, { passive: true });
    return () => {
      container.removeEventListener('scroll', handleScroll);
      if (suppressScrollLoadTimerRef.current !== null) {
        window.clearTimeout(suppressScrollLoadTimerRef.current);
        suppressScrollLoadTimerRef.current = null;
      }
      suppressNextScrollLoadRef.current = false;
    };
  }, [canLoadNewerFromDisplayEnd, hasNewerMessages, hasOlderMessages, isLoadingMessageWindow, onLoadNewer, onLoadOlder]);

  const renderMessageNode = (node: MessageNode, index: number) => (
    <MessageNodeComponent
      key={getTimelineNodeKey(node)}
      node={node}
      level={0}
      siblingIndex={index}
      siblingCount={displayMessages.length}
      ariaPosition={messagePositions[index] ?? index + 1}
      ariaSetSize={ariaSetSize}
      onLoadChildren={onLoadChildren}
      onReachStart={effectiveReachStart}
      onReachEnd={handleReachEnd}
      onJumpToStart={onJumpToStart}
      onJumpToEnd={onJumpToEnd}
      onContextMenu={onContextMenu}
      onSpeak={onSpeak}
      onCopy={onCopy}
      onEdit={onEdit}
      onSaveEdit={onSaveEdit}
      commandPathname={commandPathname}
      onDelete={onDelete}
      editorTargets={editorTargets}
      onSendToEditor={onSendToEditor}
      onFocusSiblingIndex={focusMessageAtIndex}
    />
  );

  const handleListKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.ctrlKey && e.key === 'Home' && onJumpToStart) {
      e.preventDefault();
      void Promise.resolve(onJumpToStart());
      return;
    }
    if (e.ctrlKey && e.key === 'End' && onJumpToEnd) {
      e.preventDefault();
      void Promise.resolve(onJumpToEnd());
      return;
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (shouldVirtualize) {
        focusMessageAtIndex(0);
        return;
      }
      const firstChild = e.currentTarget.querySelector('[data-message-node]') as HTMLElement | null;
      firstChild?.focus();
    }
  };

  if (threadedMessages.length === 0) {
    return (
      <div
        className="message-list message-list--empty"
        ref={setContainerRef}
        tabIndex={-1}
        role="region"
        aria-label={t('chat.messageListLabel')}
      >
        <div className="message-list__empty-state">
          <div className="message-list__empty-icon" aria-hidden="true">
            <MessageOutlined />
          </div>
          <h3 className="message-list__empty-title">
            {t('chat.emptyTitle')}
          </h3>
          <p className="message-list__empty-description">
            {t('chat.emptyDescription')}
          </p>
        </div>
      </div>
    );
  }

  const virtualRows = shouldVirtualize ? rowVirtualizer.getVirtualItems() : [];
  const virtualByIndex = new Map(virtualRows.map(row => [row.index, row]));
  const renderedIndexes = shouldVirtualize
    ? [...new Set([...virtualByIndex.keys(), ...(focusedIndex >= 0 ? [focusedIndex] : [])])].sort((a, b) => a - b)
    : displayMessages.map((_, index) => index);

  return (
    <div
      className="message-list"
      ref={setContainerRef}
      aria-label={t('chat.messageListLabel')}
    >
      <div className="message-list__messages">
        {hasOlderMessages && onLoadOlder && (
          <div className="message-list__load-older">
            <button
              type="button"
              className="message-list__load-older-button"
              onClick={() => handleLoadOlder('navigation')}
              disabled={isLoadingMessageWindow || isLoadingOlderMessages}
              aria-busy={isLoadingOlderMessages}
            >
              {isLoadingOlderMessages ? t('chat.loadingOlderMessages') : t('chat.loadOlderMessages')}
            </button>
          </div>
        )}
        <div
          ref={listRef}
          className="message-list__list"
          role="list"
          aria-label={t('chat.messagesRegion')}
          tabIndex={0}
          style={shouldVirtualize
            ? { position: 'relative', height: `${rowVirtualizer.getTotalSize()}px` }
            : undefined}
          onKeyDown={handleListKeyDown}
          onFocusCapture={handleFocusCapture}
          onBlurCapture={handleBlurCapture}
        >
          {renderedIndexes.map(index => {
            const node = displayMessages[index];
            const virtualItem = virtualByIndex.get(index);
            return (
              <div
                key={getTimelineNodeKey(node)}
                data-timeline-key={getTimelineNodeKey(node)}
                role="presentation"
                data-index={index}
                ref={shouldVirtualize ? rowVirtualizer.measureElement : undefined}
                className="message-list__item"
                style={virtualItem ? {
                  position: 'absolute',
                  top: 0,
                  left: 0,
                  width: '100%',
                  transform: `translateY(${virtualItem.start - rowVirtualizer.options.scrollMargin}px)`,
                } : undefined}
              >
                {renderMessageNode(node, index)}
              </div>
            );
          })}
        </div>
        {isLoading && (
          <div
            className="message-list__loading"

            aria-label={effectiveLoadingText}
          >
            <div className="message-list__loading-dots" aria-hidden="true">
              <span></span>
              <span></span>
              <span></span>
            </div>
            <span className="message-list__loading-text">{effectiveLoadingText}...</span>
          </div>
        )}
        <div ref={messagesEndRef} />
      </div>
    </div>
  );
}));

MessageList.displayName = 'MessageList';
