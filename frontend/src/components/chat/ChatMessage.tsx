import React, { useRef, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Message, TurnSegment, useChatStore } from '../../store/chatStore';
import { MarkdownRenderer } from '../ui/MarkdownRenderer';
import { ThreadIndicator } from './ThreadIndicator';
import { ReasoningSection } from './ReasoningSection';
import { ToolCallsSection, ToolCallStatus } from './ToolCallsSection';
import { isAgentMessage } from '../../lib/chatUtils';
import { formatRelativeTime } from '../../lib/dateUtils';
import { buildChatMessageAriaLabel } from '../../lib/chatMessageAriaLabel';
import './ChatMessage.css';

export interface ChatMessageProps {
  message: Message;
  // Thread indicator props
  hasThreadIndicator?: boolean;
  threadChildCount?: number;
  isThreadExpanded?: boolean;
  isThreadLoading?: boolean;
  onThreadToggle?: () => void;
  // Event handlers
  onContextMenu?: (event: React.MouseEvent, message: Message) => void;
  onSpeak?: (message: Message) => void;
  // Modo leitura (virtual modal)
  isReading?: boolean;
  // Edit mode props
  isEditing?: boolean;
  editContent?: string;
  onEditContentChange?: (content: string) => void;
  onSaveEdit?: () => void;
  onCancelEdit?: () => void;
  // Reasoning/Thinking props
  streamingReasoning?: string; // Reasoning durante streaming
  isThinking?: boolean; // Se está recebendo reasoning
  isReasoningExpanded?: boolean; // Se o reasoning está expandido
  onToggleReasoning?: () => void; // Callback para toggle do reasoning
  // Tool calling props
  activeToolCalls?: ToolCallStatus[]; // Tool calls em execução (durante streaming)
  completedSegments?: TurnSegment[]; // Completed segments from previous agentic iterations (streaming)
  // Audio playback
  isPlayingAudio?: boolean; // Se está reproduzindo áudio desta mensagem

  // Envio de blocos para o editor
  onSendToEditor?: (payload: {
    target: 'current' | 'new_document';
    format: 'markdown' | 'html' | 'plain';
    title?: string;
    content: string;
  }) => void;
}

export const ChatMessage: React.FC<ChatMessageProps> = React.memo(({
  message,
  hasThreadIndicator = false,
  threadChildCount = 0,
  isThreadExpanded = false,
  isThreadLoading = false,
  onThreadToggle,
  onContextMenu,
  onSpeak,
  isReading = false,
  isEditing = false,
  editContent: externalEditContent = '',
  onEditContentChange,
  onSaveEdit,
  onCancelEdit,
  streamingReasoning,
  isThinking = false,
  isReasoningExpanded = false,
  onToggleReasoning,
  activeToolCalls,
  completedSegments: _completedSegmentsProp,
  isPlayingAudio = false,
  onSendToEditor,
}) => {
  const { t } = useTranslation();
  const { role, content, timestamp, isStreaming, reasoning, toolCalls } = message;
  const editTextareaRef = useRef<HTMLTextAreaElement>(null);
  const messageId = message.id;

  // Snapshot live de campos que são atualizados in-place na store.
  // Isso garante que aria-label e conteúdo reflitam streaming mesmo com React.memo acima.
  const [liveContent, setLiveContent] = useState<string | null>(null);
  const [liveIsStreaming, setLiveIsStreaming] = useState<boolean | null>(null);
  const [liveReasoning, setLiveReasoning] = useState<string | null>(null);
  const [liveToolCallsRaw, setLiveToolCallsRaw] = useState<string | null>(null);

  // Manual subscribe to completedSegments — bypasses useSyncExternalStore/React.memo entirely.
  // useState + subscribe guarantees re-render when segments change.
  const [liveSegments, setLiveSegments] = useState<TurnSegment[]>([]);
  const [liveToolCalls, setLiveToolCalls] = useState<ToolCallStatus[]>([]);

  useEffect(() => {
    const unsub = useChatStore.subscribe((state) => {
      if (state.streamingMessageId === messageId) {
        setLiveSegments(prev =>
          prev !== state.completedSegments ? state.completedSegments : prev
        );
        setLiveToolCalls(prev =>
          prev !== state.activeToolCalls ? state.activeToolCalls : prev
        );
      } else {
        setLiveSegments(prev => prev.length > 0 ? [] : prev);
        setLiveToolCalls(prev => prev.length > 0 ? [] : prev);
      }
    });

    // Sync initial state
    const initial = useChatStore.getState();
    if (initial.streamingMessageId === messageId) {
      setLiveSegments(initial.completedSegments);
      setLiveToolCalls(initial.activeToolCalls);
    }

    return unsub;
  }, [messageId]);

  useEffect(() => {
    const trackingRef = { current: !!isStreaming };

    const findMessageInState = (state: ReturnType<typeof useChatStore.getState>) => {
      const targetId = String(messageId);
      type ThreadedNode = {
        message?: { id?: string | number; content?: string; isStreaming?: boolean; reasoning?: string; toolCalls?: string | null };
        children?: ThreadedNode[];
      };
      const visit = (nodes: ThreadedNode[]): ThreadedNode['message'] | null => {
        for (const n of nodes || []) {
          const msg = n?.message;
          if (msg && String(msg.id) === targetId) return msg;
          if (n?.children?.length) {
            const hit = visit(n.children);
            if (hit) return hit;
          }
        }
        return null;
      };

      const conv = state.activeConversation;
      if (conv?.threadedMessages) {
        const hit = visit(conv.threadedMessages as ThreadedNode[]);
        if (hit) return hit;
      }
      return null;
    };

    const sync = (state: ReturnType<typeof useChatStore.getState>) => {
      const msg = findMessageInState(state);
      if (!msg) return;

      const nextContent = typeof msg.content === 'string' ? msg.content : '';
      const nextIsStreaming = !!msg.isStreaming;
      const nextReasoning = typeof msg.reasoning === 'string' ? msg.reasoning : '';
      const nextToolCalls = typeof msg.toolCalls === 'string' ? msg.toolCalls : null;

      setLiveContent((prev) => (prev === nextContent ? prev : nextContent));
      setLiveIsStreaming((prev) => (prev === nextIsStreaming ? prev : nextIsStreaming));
      setLiveReasoning((prev) => (prev === nextReasoning ? prev : nextReasoning));
      setLiveToolCallsRaw((prev) => (prev === nextToolCalls ? prev : nextToolCalls));

      // Desarma tracking após obter o estado final (best-effort).
      if (!nextIsStreaming && state.streamingMessageId !== messageId) {
        trackingRef.current = false;
      }
    };

    // Sync inicial (cobre caso de re-render tardio)
    sync(useChatStore.getState());

    const unsub = useChatStore.subscribe((state) => {
      if (state.streamingMessageId === messageId) trackingRef.current = true;
      if (!trackingRef.current) return;
      sync(state);
    });

    return unsub;
  }, [messageId, isStreaming]);

  const completedSegments = liveSegments.length > 0 ? liveSegments : _completedSegmentsProp;
  const effectiveToolCalls = liveToolCalls.length > 0 ? liveToolCalls : activeToolCalls;

  const effectiveContent = liveContent !== null ? liveContent : content;
  const effectiveIsStreaming = liveIsStreaming !== null ? liveIsStreaming : isStreaming;
  const effectiveReasoning = liveReasoning !== null ? liveReasoning : reasoning;
  const effectiveToolCallsRaw = liveToolCallsRaw !== null ? liveToolCallsRaw : toolCalls;

  const hasAgenticSegments = !!(message._turnSegments || (completedSegments && completedSegments.length > 0));
  const isAgenticStreaming = effectiveIsStreaming && hasAgenticSegments;

  // Usa editContent externo se está editando
  const editContent = isEditing ? externalEditContent : effectiveContent;

  const toolCallsHasTextEdit =
    role === 'assistant' &&
    typeof effectiveToolCallsRaw === 'string' &&
    /"name"\s*:\s*"text_edit"/i.test(effectiveToolCallsRaw);

  // Quando `text_edit` é usado, o conteúdo do assistente pode vir poluído com fences (ex.: ```markdown).
  // Como a UI já mostra as tool calls, omitimos o corpo textual para evitar ruído.
  const displayContent = isEditing ? externalEditContent : (toolCallsHasTextEdit ? '' : effectiveContent);

  const formatTime = (timestamp: number) => {
    const date = new Date(timestamp);
    return date.toLocaleTimeString('pt-BR', {
      hour: '2-digit',
      minute: '2-digit',
    });
  };

  const getDisplayRole = () => {
    // Usuário
    if (role === 'user') return t('chat.you');

    // Resposta de ferramenta — mostra ID do call
    if (role === 'tool') return t('chat.result');

    // Assistente com tool calls pendentes
    if (role === 'assistant' && toolCalls) return t('chat.assistant');

    // Assistente padrão
    return t('chat.assistant');
  };

  const getAriaLabel = () => {
    const roleLabel = getDisplayRole();
    const relativeTime = formatRelativeTime(timestamp);
    const timePrefix = role === 'user' ? 'enviado' : 'recebido';

    return buildChatMessageAriaLabel({
      roleLabel,
      role,
      displayContent,
      isStreaming: effectiveIsStreaming,
      timePrefix,
      relativeTime,
      isReasoningExpanded,
      reasoning: effectiveReasoning,
      streamingReasoning,
      toolCallsRaw: effectiveToolCallsRaw,
      toolCallsHasTextEdit,
    });
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    // Se está editando, tratar teclas de edição
    if (isEditing) {
      if (e.key === 'Escape' && onCancelEdit) {
        e.preventDefault();
        onCancelEdit();
        return;
      }
      // Ctrl+Enter salva
      if (e.key === 'Enter' && e.ctrlKey && onSaveEdit) {
        e.preventDefault();
        onSaveEdit();
        return;
      }

      // Focus trap: Tab entre textarea e botões
      if (e.key === 'Tab') {
        const editContainer = editTextareaRef.current?.parentElement;
        if (!editContainer) return;

        const focusableElements = editContainer.querySelectorAll<HTMLElement>(
          'textarea, button:not(:disabled)'
        );
        const focusableArray = Array.from(focusableElements);
        const currentIndex = focusableArray.indexOf(document.activeElement as HTMLElement);

        e.preventDefault();

        if (e.shiftKey) {
          // Shift+Tab: vai para trás
          const prevIndex = currentIndex <= 0 ? focusableArray.length - 1 : currentIndex - 1;
          focusableArray[prevIndex]?.focus();
        } else {
          // Tab: vai para frente
          const nextIndex = currentIndex >= focusableArray.length - 1 ? 0 : currentIndex + 1;
          focusableArray[nextIndex]?.focus();
        }
        return;
      }

      // Não processar outras teclas durante edição
      return;
    }

    // Teclas quando NÃO está editando
    // Spacebar - reproduz TTS da mensagem (assistente ou usuário)
    if (e.key === ' ' && !effectiveIsStreaming) {
      e.preventDefault();
      if (onSpeak) {
        onSpeak(message);
      }
      return;
    }

    // Enter é tratado pelo MessageNode (ativa modo leitura virtual)
    // Não processar aqui para evitar conflito
  };

  const handleKeyUp = (e: React.KeyboardEvent) => {
    // Tecla ContextMenu/Application (Windows) ou Shift+F10 abre menu de contexto
    // Usa onKeyUp para prevenir menu nativo do navegador
    if (e.key === 'ContextMenu' || (e.shiftKey && e.key === 'F10')) {
      e.preventDefault();
      e.stopPropagation();
      if (onContextMenu) {
        // Simula evento de mouse no centro do elemento
        const rect = e.currentTarget.getBoundingClientRect();
        const syntheticEvent = {
          preventDefault: () => {},
          stopPropagation: () => {},
          clientX: rect.left + rect.width / 2,
          clientY: rect.top + rect.height / 2,
          currentTarget: e.currentTarget, // Para restaurar foco após fechar menu
          target: e.currentTarget,
        } as unknown as React.MouseEvent;
        onContextMenu(syntheticEvent, message);
      }
    }
  };

  const handleContextMenuEvent = (e: React.MouseEvent) => {
    e.preventDefault();
    onContextMenu?.(e, message);
  };

  const handleFocus = () => {
    // Mensagem recebeu foco
  };

  // Auto-focus e posiciona cursor quando entra em modo de edição
  useEffect(() => {
    if (isEditing) {
      requestAnimationFrame(() => {
        if (editTextareaRef.current) {
          editTextareaRef.current.focus();
          const length = editTextareaRef.current.value.length;
          editTextareaRef.current.setSelectionRange(length, length);
        }
      });
    }
  }, [isEditing]);

  return (
    <div
      className={`chat-message chat-message--${role} ${isEditing ? 'chat-message--editing' : ''} ${isReading ? 'chat-message--reading' : ''}`}
      aria-label={isEditing ? undefined : getAriaLabel()}
      aria-live={effectiveIsStreaming && !isAgenticStreaming ? 'polite' : 'off'}
      aria-busy={effectiveIsStreaming && !isAgenticStreaming}
      onKeyDown={handleKeyDown}
      onKeyUp={handleKeyUp}
      onContextMenu={handleContextMenuEvent}
      onFocus={handleFocus}
      tabIndex={-1}
    >
      {isReading && (
        <div className="chat-message__reading-badge" aria-hidden="true">
          {t('chat.reading')}
        </div>
      )}
      <div className="chat-message__avatar" aria-hidden="true">
        {role === 'user' ? (
          <div className="chat-message__avatar-user">U</div>
        ) : role === 'tool' ? (
          <div className="chat-message__avatar-tool">🛠️</div>
        ) : (
          <div className="chat-message__avatar-assistant">
            {isAgentMessage(message) ? '🤖' : 'AI'}
          </div>
        )}
      </div>
      <div className="chat-message__content">
        <div className="chat-message__header">
          <h3 className="chat-message__role">
            {getDisplayRole()}
          </h3>
          {message.source && message.source !== 'wails' && message.source !== '' && (
            <span className="chat-message__source-badge" aria-label={`${t('chat.via')} ${message.source}`}>
              {message.source === 'telegram' && '✈'}
              {message.source === 'signal' && '🔒'}
              {message.source === 'whatsapp' && '💬'}
              {!['telegram', 'signal', 'whatsapp'].includes(message.source) && '📱'}
              <span className="chat-message__source-name">{message.source}</span>
            </span>
          )}
          <span className="chat-message__timestamp">
            {formatTime(timestamp)}
          </span>
          {!effectiveIsStreaming && !isEditing && displayContent && onSpeak && (
            <button
              className={`chat-message__play-btn${isPlayingAudio ? ' chat-message__play-btn--playing' : ''}`}
              onClick={(e) => { e.stopPropagation(); onSpeak(message); }}
              aria-label={isPlayingAudio ? t('chat.stopAudio') : t('chat.playAudio')}
              tabIndex={-1}
            >
              {isPlayingAudio ? '⏹' : '🔊'}
            </button>
          )}
          {hasThreadIndicator && onThreadToggle && (
            <ThreadIndicator
              childCount={threadChildCount}
              isExpanded={isThreadExpanded}
              isLoading={isThreadLoading}
              onToggle={onThreadToggle}
            />
          )}
        </div>

        {/* Seção de Reasoning/Thinking - exibe cadeia de pensamento do modelo */}
        {role === 'assistant' && (streamingReasoning || effectiveReasoning) && (
          <ReasoningSection 
            reasoning={streamingReasoning || effectiveReasoning || ''} 
            isStreaming={isThinking}
            isExpanded={isThinking || isReasoningExpanded}
            onToggle={onToggleReasoning}
          />
        )}

        {/* Interleaved segments: text → tools → text → tools → final answer */}
        {role === 'assistant' && hasAgenticSegments ? (
          <>
            {/* Completed segments — role="log" so screen readers announce each addition
                and browse mode users can navigate segment by segment */}
            <div
              role="log"
              aria-label={t('chat.progressLabel')}
              aria-relevant="additions"
              className="chat-message__segments-log"
            >
              {(message._turnSegments || completedSegments || []).map((seg, idx) => (
                <React.Fragment key={idx}>
                  {seg.type === 'text' && seg.content && (
                    <section
                      className="chat-message__text chat-message__text--segment"
                      aria-label={`${t('chat.step')} ${Math.floor(idx / 2) + 1}`}
                      tabIndex={-1}
                    >
                      <MarkdownRenderer
                        content={seg.content}
                        interactiveButtons={!!onSendToEditor}
                        focusableMermaid={!!onSendToEditor}
                        enableSendToEditorButtons={!!onSendToEditor}
                        onSendToEditor={onSendToEditor}
                      />
                    </section>
                  )}
                  {seg.type === 'tool_calls' && seg.toolCalls && (
                    <ToolCallsSection
                      toolCallsJson={JSON.stringify(seg.toolCalls)}
                    />
                  )}
                </React.Fragment>
              ))}
            </div>

            {/* Current iteration — aria-busy suppresses char-by-char updates */}
            <div aria-busy={effectiveIsStreaming} aria-live={effectiveIsStreaming ? 'polite' : 'off'}>
              {effectiveIsStreaming && effectiveToolCalls && effectiveToolCalls.length > 0 && (
                <ToolCallsSection activeToolCalls={effectiveToolCalls} />
              )}

              {effectiveIsStreaming && displayContent && !message._turnSegments && (
                <div className="chat-message__text">
                  <MarkdownRenderer
                    content={displayContent}
                    interactiveButtons={!!onSendToEditor}
                    focusableMermaid={!!onSendToEditor}
                    enableSendToEditorButtons={!!onSendToEditor}
                    onSendToEditor={onSendToEditor}
                  />
                </div>
              )}
              {effectiveIsStreaming && !displayContent && !message._turnSegments && (
                <div className="chat-message__text">
                  <span className="chat-message__cursor">▋</span>
                </div>
              )}
            </div>
          </>
        ) : (
          <>
            {/* Non-agentic messages: flat layout (reasoning → tools → content) */}
            {role === 'assistant' && (effectiveToolCallsRaw || (effectiveIsStreaming && effectiveToolCalls && effectiveToolCalls.length > 0)) && (
              <ToolCallsSection
                toolCallsJson={effectiveToolCallsRaw || undefined}
                activeToolCalls={effectiveIsStreaming ? effectiveToolCalls : undefined}
              />
            )}

            <div className="chat-message__text">
              {isEditing ? (
                <div className="chat-message__edit">
                  <textarea
                    ref={editTextareaRef}
                    className="chat-message__edit-textarea"
                    value={editContent}
                    onChange={(e) => onEditContentChange?.(e.target.value)}
                    placeholder={t('chat.editPlaceholder')}
                    rows={3}
                    tabIndex={0}
                    aria-label={t('chat.editMessage')}
                  />
                  <div className="chat-message__edit-actions">
                    <button
                      className="chat-message__edit-button chat-message__edit-button--cancel"
                      onClick={onCancelEdit}
                      aria-label={t('common.cancel')}
                    >
                      {t('common.cancel')}
                    </button>
                    <button
                      className="chat-message__edit-button chat-message__edit-button--save"
                      onClick={onSaveEdit}
                      disabled={!editContent.trim()}
                      aria-label={t('common.save')}
                    >
                      {t('common.save')}
                    </button>
                  </div>
                </div>
              ) : (
                <>
                  {role === 'assistant' && displayContent ? (
                    <MarkdownRenderer
                      content={displayContent}
                      interactiveButtons={!!onSendToEditor}
                      focusableMermaid={!!onSendToEditor}
                      enableSendToEditorButtons={!!onSendToEditor}
                      onSendToEditor={onSendToEditor}
                    />
                  ) : (
                    displayContent || (effectiveIsStreaming && <span className="chat-message__cursor">▋</span>)
                  )}
                </>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  );
});
