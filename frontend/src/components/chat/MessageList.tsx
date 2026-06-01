import { logger } from '../../utils/logger';
import React, { useEffect, useLayoutEffect, useRef, useMemo, useState, useCallback, forwardRef } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import { useTranslation } from 'react-i18next';
import { MessageOutlined } from '@ant-design/icons';
import { MessageNode as MessageNodeComponent } from './MessageNode';
import { MessageNode, Message, TurnSegment } from '../../store/chatStore';
import { getMessageTurnSegments } from '../../lib/chatMessageTree';
import { chat } from '../../../wailsjs/go/models';
import type { EditorSendTargetOption, SendToEditorPayload } from '../../lib/editorSendMenu';
import { getTimelineNodeKey, isPersistedTimelineNode, type MessageWindowState } from '../../services/chatSessionRegistry';
import './MessageList.css';

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
  onLoadOlder?: () => Promise<void> | void;
  onLoadNewer?: () => Promise<void> | void;
  onJumpToStart?: () => Promise<void> | void;
  onJumpToEnd?: () => Promise<void> | void;
  // Callbacks de ações
  onContextMenu?: (event: React.MouseEvent, message: Message) => void;
  onSpeak?: (message: Message) => void;
  onDelete?: (message: Message) => void;
  editorTargets?: EditorSendTargetOption[];
  onSendToEditor?: (payload: SendToEditorPayload) => void;
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

/**
 * Consolida mensagens de turnos de tool calling em entradas únicas com
 * segments intercalados (texto → tools → texto → tools → resposta final).
 * 
 * No agentic loop, um único turno gera múltiplas mensagens no banco:
 *   1. Assistant com toolCalls (intermediária)
 *   2. Tool results (role=tool)
 *   3. Assistant com toolCalls (outra iteração)
 *   4. Tool results...
 *   5. Assistant final (resposta)
 * 
 * Produz UMA entrada visual com `_turnSegments` que preserva a ordem
 * cronológica: [text, tool_calls, text, tool_calls, ..., text].
 */
function consolidateTurnMessages(nodes: MessageNode[]): MessageNode[] {
  if (!nodes || nodes.length === 0) return nodes;

  const turnMap = new Map<string, MessageNode[]>();

  for (const node of nodes) {
    const turnId = node.message.turnId;
    if (turnId) {
      if (!turnMap.has(turnId)) turnMap.set(turnId, []);
      turnMap.get(turnId)!.push(node);
    }
  }

  const hasSplitTurns = Array.from(turnMap.values()).some((turnNodes) => turnNodes.length > 1);
  if (!hasSplitTurns) return nodes;

  const processedTurnIds = new Set<string>();
  const result: MessageNode[] = [];

  for (const node of nodes) {
    const turnId = node.message.turnId;

    if (!turnId) {
      result.push(node);
      continue;
    }

    if (node.message.role === 'tool') continue;
    if (processedTurnIds.has(turnId)) continue;
    processedTurnIds.add(turnId);

    const turnNodes = turnMap.get(turnId) || [];

    // Index tool results by toolCallId
    const toolResults = new Map<string, string>();
    for (const tn of turnNodes) {
      if (tn.message.role === 'tool' && tn.message.toolCallId) {
        toolResults.set(tn.message.toolCallId, tn.message.content || '');
      }
    }

    // Build segments in chronological order from assistant messages.
    // Backend-provided segments are canonical; locally derived segments only cover transient split nodes.
    const canonicalSegments: TurnSegment[] = [];
    const derivedSegments: TurnSegment[] = [];
    const allToolCalls: unknown[] = [];
    let finalContent = '';
    let finalReasoning = '';
    let finalNode = node;

    for (const tn of turnNodes) {
      if (tn.message.role !== 'assistant') continue;
      finalNode = tn;
      const existingSegments = getMessageTurnSegments(tn.message as Message);
      const hasCanonicalSegments = !!(existingSegments && existingSegments.length > 0);
      if (hasCanonicalSegments) {
        canonicalSegments.push(...(existingSegments as TurnSegment[]));
      }

      // Text segment (intermediate reasoning or final answer)
      if (tn.message.content && !hasCanonicalSegments) {
        derivedSegments.push({ type: 'text', content: tn.message.content });
        finalContent = tn.message.content;
      } else if (tn.message.content) {
        finalContent = tn.message.content;
      }

      // Tool calls segment (enriched with results)
      if (tn.message.toolCalls) {
        try {
          const parsed = JSON.parse(tn.message.toolCalls);
          const calls = Array.isArray(parsed) ? parsed : [parsed];
          const enrichedCalls = calls.map((call) => {
            const callRecord = (typeof call === 'object' && call !== null)
              ? (call as Record<string, unknown>)
              : {};
            const callId = String(callRecord.id ?? '');
            const type = String(callRecord.type ?? 'function');
            const func = callRecord.function as { name?: unknown; arguments?: unknown } | undefined;
            const fnName = String(func?.name ?? callRecord.name ?? '');
            const fnArgs = String(func?.arguments ?? callRecord.arguments ?? '');

            return {
              id: callId,
              type,
              function: { name: fnName, arguments: fnArgs },
              result: callId ? toolResults.get(callId) ?? undefined : undefined,
            };
          });
          if (!hasCanonicalSegments) {
            derivedSegments.push({ type: 'tool_calls', toolCalls: enrichedCalls });
          }
          allToolCalls.push(...enrichedCalls);
        } catch {
          // Invalid local toolCalls payload; skip this derived segment in render.
        }
      }

      if (tn.message.reasoning) {
        finalReasoning = tn.message.reasoning;
      }
    }

    const consolidatedMessage = chat.EnrichedMessage.createFrom({
      ...finalNode.message,
      content: finalContent,
      reasoning: finalReasoning || finalNode.message.reasoning || '',
      toolCalls: allToolCalls.length > 0 ? JSON.stringify(allToolCalls) : undefined,
    });

    const consolidated = chat.MessageNode.createFrom({
      ...finalNode,
      message: consolidatedMessage,
    }) as MessageNode;
    const firstOriginalIndex = turnNodes
      .map((turnNode) => turnNode.originalIndex)
      .find((index): index is number => index !== undefined);
    consolidated.originalIndex = firstOriginalIndex ?? finalNode.originalIndex;

    const segments = canonicalSegments.length > 0
      ? [...canonicalSegments, ...derivedSegments]
      : derivedSegments;

    // Backend-provided segments are preserved even when a transient sibling temporarily splits the turn.
    if (canonicalSegments.length > 0 || segments.length > 1) {
      (consolidated.message as Message)._turnSegments = segments;
    }

    result.push(consolidated);
  }

  return result;
}

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
    onDelete,
    editorTargets,
    onSendToEditor,
  },
  ref
) => {
  const { t } = useTranslation();
  const effectiveLoadingText = loadingText ?? t('chat.typing');
  const messagesEndRef = useRef<HTMLDivElement>(null);
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

  // Fallback transitório: o backend já retorna timeline items canônicos;
  // durante streaming ainda podem existir múltiplos nós locais do mesmo turnId.
  const displayMessages = useMemo(
    () => consolidateTurnMessages(threadedMessages),
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

  const rowVirtualizer = useVirtualizer({
    count: displayMessages.length,
    enabled: shouldVirtualize,
    getScrollElement: () => innerContainerRef.current,
    estimateSize: () => ESTIMATED_MESSAGE_HEIGHT,
    overscan: VIRTUAL_OVERSCAN,
    scrollMargin,
    // Chave estável por nó preserva a medição ao paginar histórico (prepend/append).
    getItemKey: (index) => getTimelineNodeKey(displayMessages[index]),
  });

  // Foca um irmão de nível 0 quando virtualizado: rola o índice até a viewport
  // (pode não estar montado) e então move o foco para o nó correspondente.
  const focusMessageAtIndex = useCallback((index: number) => {
    if (displayMessages.length === 0) return;
    const clamped = Math.max(0, Math.min(index, displayMessages.length - 1));
    rowVirtualizer.scrollToIndex(clamped, { align: 'auto' });
    const focusTarget = (): boolean => {
      const el = listRef.current?.querySelector(
        `[data-message-node][data-level="0"][data-sibling-index="${clamped}"]`
      ) as HTMLElement | null;
      if (el) {
        el.focus();
        return true;
      }
      return false;
    };
    if (!focusTarget()) {
      requestAnimationFrame(() => {
        focusTarget();
      });
    }
  }, [displayMessages.length, rowVirtualizer]);

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

  const scrollToBottom = (behavior: ScrollBehavior = 'smooth') => {
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

  const handleLoadOlder = () => {
    const container = innerContainerRef.current;
    const snapshot = container
      ? { scrollHeight: container.scrollHeight, scrollTop: container.scrollTop }
      : null;
    pendingScrollRestoreRef.current = snapshot;

    const result = onLoadOlder?.();
    void Promise.resolve(result).finally(() => {
      window.setTimeout(() => {
        if (pendingScrollRestoreRef.current === snapshot) {
          pendingScrollRestoreRef.current = null;
        }
      }, 0);
    });
  };

  const handleLoadNewer = () => {
    const result = onLoadNewer?.();
    void Promise.resolve(result);
  };

  const handleReachStart = () => {
    if (hasOlderMessages && onLoadOlder && !isLoadingMessageWindow) {
      handleLoadOlder();
      return;
    }
    void Promise.resolve(onReachStart?.());
  };
  const effectiveReachStart = (hasOlderMessages && onLoadOlder) || onReachStart
    ? handleReachStart
    : undefined;

  const handleReachEnd = () => {
    if (hasNewerMessages && onLoadNewer && !isLoadingMessageWindow && canLoadNewerFromDisplayEnd) {
      handleLoadNewer();
      return;
    }
    onReachEnd?.();
  };

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
        handleLoadOlder();
        return;
      }
      const distanceToBottom = container.scrollHeight - container.scrollTop - container.clientHeight;
      if (distanceToBottom < 48 && hasNewerMessages && !isLoadingMessageWindow && canLoadNewerFromDisplayEnd) {
        handleLoadNewer();
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

  const renderMessageNode = (node: MessageNode, index: number, virtualized: boolean) => (
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
      onDelete={onDelete}
      editorTargets={editorTargets}
      onSendToEditor={onSendToEditor}
      onFocusSiblingIndex={virtualized ? focusMessageAtIndex : undefined}
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
              onClick={handleLoadOlder}
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
        >
          {shouldVirtualize
            ? rowVirtualizer.getVirtualItems().map((virtualItem) => {
                const node = displayMessages[virtualItem.index];
                return (
                  <div
                    key={virtualItem.key}
                    role="presentation"
                    data-index={virtualItem.index}
                    ref={rowVirtualizer.measureElement}
                    className="message-list__virtual-item"
                    style={{
                      position: 'absolute',
                      top: 0,
                      left: 0,
                      width: '100%',
                      transform: `translateY(${virtualItem.start - rowVirtualizer.options.scrollMargin}px)`,
                    }}
                  >
                    {renderMessageNode(node, virtualItem.index, true)}
                  </div>
                );
              })
            : displayMessages.map((node, index) => renderMessageNode(node, index, false))}
        </div>
        {isLoading && (
          <div
            className="message-list__loading"
            role="status"
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
