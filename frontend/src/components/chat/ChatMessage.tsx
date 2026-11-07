import React, { useMemo, useRef, useEffect, useState, useId } from 'react';
import { useTranslation } from 'react-i18next';
import {
  RobotOutlined, SendOutlined, LockOutlined,
  MessageOutlined, MobileOutlined, SoundOutlined, PauseCircleOutlined,
} from '@ant-design/icons';
import type { Message, TurnSegment } from '../../store/chatStore';
import { MarkdownRenderer } from '../ui/MarkdownRenderer';
import { ThreadIndicator } from './ThreadIndicator';
import { ReasoningSection } from './ReasoningSection';
import { ToolCallsSection } from './ToolCallsSection';
import { ToolInvocationDialogsProvider, displayToolStatus } from './ToolInvocationDialogs';
import { isAgentMessage } from '../../lib/chatUtils';
import { formatRelativeTime } from '../../lib/dateUtils';
import { buildChatMessageAriaLabel } from '../../lib/chatMessageAriaLabel';
import type { ChatSendToEditorPayload, EditorSendTargetOption, SendToEditorPayload } from '../../lib/editorSendMenu';
import { formatToolPresentation, presentTool } from '../../lib/toolPresentation';
import { translateBackendErrorDiagnostic } from '../../lib/providerErrorMessage';
import './ChatMessage.css';

const HEAVY_MARKDOWN_CONTENT_LENGTH = 8_000;
const HEAVY_ARIA_CONTENT_PREVIEW_LENGTH = 1_200;
const HEAVY_AGENTIC_SEGMENT_COUNT = 8;

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
  // Audio playback
  isPlayingAudio?: boolean; // Se está reproduzindo áudio desta mensagem

  // Envio de blocos para o editor
  editorTargets?: EditorSendTargetOption[];
  onSendToEditor?: (payload: ChatSendToEditorPayload) => void;
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
  isPlayingAudio = false,
  editorTargets,
  onSendToEditor,
}) => {
  const { t } = useTranslation();
  const { role, content, timestamp, isStreaming, reasoning } = message;
  const sourceMessageId = message.id;
  const sendBlockToEditor = useMemo(() => onSendToEditor
    ? (payload: SendToEditorPayload) => onSendToEditor({
        ...payload, messageId: sourceMessageId, originalContent: content,
      })
    : undefined, [onSendToEditor, sourceMessageId, content]);
  const messageRef = useRef<HTMLDivElement>(null);
  const chainRegionId = useId();
  // Issue #163: a cadeia do turno (segmentos intermediários + tool calls) ganha
  // uma affordance acessível de contrair/expandir. Inicia expandida para
  // preservar o layout visual já existente (a cadeia é exibida agrupada); o
  // controle expõe `aria-expanded` e permite recolher a cadeia por economia.
  const [isChainExpanded, setIsChainExpanded] = useState(true);
  const editTextareaRef = useRef<HTMLTextAreaElement>(null);
  const previousShouldDeferHeavyContentRef = useRef(false);
  const effectiveContent = content;
  const localizedEffectiveContent = message.role === 'assistant' ? translateBackendErrorDiagnostic(content) : content;
  const effectiveIsStreaming = isStreaming;
  const effectiveReasoning = reasoning;
  const renderedTabNavigation = isReading ? 'enabled' : 'disabled';
  const rawTurnSegments = (message.turnSegments ?? []) as TurnSegment[];
  const dialogCalls = useMemo(() => (message.turnSegments ?? []).flatMap(segment => segment.toolInvocations ?? []), [message.turnSegments]);
  const hasAgenticSegments = rawTurnSegments.length > 0;
  const isAgenticStreaming = effectiveIsStreaming && hasAgenticSegments;
  const isToolOnlyTurn = !effectiveIsStreaming && !content.trim() && rawTurnSegments.some(segment => segment.toolInvocations?.length);
  const placeholderContent = isToolOnlyTurn ? t('chat.toolOnlyTurnPlaceholder') : '';
  const conclusionContent = message.role === 'assistant' ? translateBackendErrorDiagnostic(content || placeholderContent) : content;
  // Segmentos permanecem na mesma posição e com a mesma identidade ao finalizar.
  const chainSegments = rawTurnSegments;
  const expandedConclusionContent = conclusionContent;
  const showConclusionAfterChain = !effectiveIsStreaming && (!isChainExpanded || isToolOnlyTurn) && !!conclusionContent;

  // O host dos diálogos permanece estável ao concluir os segmentos.
  const toolLabels = useMemo(() => dialogCalls.map((call) => formatToolPresentation(
    presentTool(
      call.name,
      call.origin,
      call.serverLabel,
      call.inputPreview,
    ),
    (key, values) => t(key, values),
    displayToolStatus(call.status) === 'succeeded',
  )), [dialogCalls, t]);

  // Usa editContent externo se está editando
  const editContent = isEditing ? externalEditContent : effectiveContent;

  const displayContent = isEditing ? externalEditContent : localizedEffectiveContent || placeholderContent;
  const toolCallsHasTextEdit = dialogCalls.some(invocation => invocation.name === 'text_edit');
  const segmentCount = rawTurnSegments.length;
  const shouldDeferHeavyContent =
    !effectiveIsStreaming &&
    !isReading &&
    !isEditing &&
    (
      displayContent.length > HEAVY_MARKDOWN_CONTENT_LENGTH ||
      dialogCalls.reduce(
        (total, invocation) => total + (invocation.inputBytes ?? 0) + (invocation.outputBytes ?? 0),
        0,
      ) > HEAVY_MARKDOWN_CONTENT_LENGTH ||
      segmentCount > HEAVY_AGENTIC_SEGMENT_COUNT
    );
  const [isHeavyContentReady, setIsHeavyContentReady] = useState(!shouldDeferHeavyContent);
  const canRenderHeavyContent = !shouldDeferHeavyContent
    || (isHeavyContentReady && previousShouldDeferHeavyContentRef.current)
    || isReading
    || isEditing;
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

    // Assistente padrão
    return t('chat.assistant');
  };

  const getAriaLabel = () => {
    const roleLabel = getDisplayRole();
    const relativeTime = formatRelativeTime(timestamp);
    const timePrefix = role === 'user' ? t('chat.sent') : t('chat.received');
    const localized = {
      responding: t('chat.typing'),
      reasoning: t('chat.reasoning'),
      textEditApplied: t('chat.ariaTextEditApplied'),
      noTextContent: t('chat.pins.noTextContent'),
      playAudioHint: t('chat.ariaPlayAudioHint'),
    };

    // Issue #160: em turnos agênticos o anúncio usa só a conclusão do turno; nos
    // demais (mensagem simples) mantém-se o conteúdo principal `displayContent`.
    // `text_edit` continua suprimindo o corpo textual (displayContent === '').
    const ariaContent = hasAgenticSegments && !toolCallsHasTextEdit
      ? conclusionContent
      : displayContent;

    if (!canRenderHeavyContent) {
      return buildChatMessageAriaLabel({
        roleLabel,
        role,
        displayContent: ariaContent.length > HEAVY_ARIA_CONTENT_PREVIEW_LENGTH
          ? `${ariaContent.slice(0, HEAVY_ARIA_CONTENT_PREVIEW_LENGTH)}... ${t('chat.largeMessageDeferred')}`
          : ariaContent,
        isStreaming: effectiveIsStreaming,
        timePrefix,
        relativeTime,
        isReasoningExpanded: false,
        reasoning: null,
        streamingReasoning: null,
        toolLabels,
        toolCallsHasTextEdit,
        codeBlockLabel: t('chat.codeBlockSpeechLabel'),
        localized,
      });
    }

    return buildChatMessageAriaLabel({
      roleLabel,
      role,
      displayContent: ariaContent,
      isStreaming: effectiveIsStreaming,
      timePrefix,
      relativeTime,
      isReasoningExpanded,
      reasoning: effectiveReasoning,
      streamingReasoning,
      toolLabels,
      toolCallsHasTextEdit,
      codeBlockLabel: t('chat.codeBlockSpeechLabel'),
      localized,
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
        if (e.defaultPrevented || e.repeat || e.nativeEvent.isComposing || e.keyCode === 229 || e.altKey || e.metaKey) return;
        e.preventDefault();
        e.stopPropagation();
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

    // No modo de leitura, links, botões e demais controles internos devem
    // receber Enter/Espaço/Setas sem que os atalhos da mensagem os interceptem.
    // A propagação para aqui; o comportamento padrão do controle permanece.
    if (isReading) {
      e.stopPropagation();
      return;
    }

    // Teclas quando NÃO está editando
    // Spacebar - reproduz TTS da mensagem (assistente ou usuário)
    if (e.key === ' ' && !effectiveIsStreaming) {
      if (e.defaultPrevented || e.repeat || e.nativeEvent.isComposing || e.keyCode === 229) return;
      e.preventDefault();
      e.stopPropagation();
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

  // Issue #163: ao entrar no modo de leitura a cadeia precisa estar inteira no
  // DOM (segmentos + tool calls) para que o `role="document"` do useVirtualModal
  // a exponha por completo. Se o usuário a havia recolhido, força a expansão e
  // lembra o estado anterior para restaurá-lo ao SAIR (ESC) — mantendo
  // `aria-expanded` consistente com o conteúdo visível. A restauração só ocorre
  // quando a expansão foi forçada por nós: se o usuário recolher/expandir
  // manualmente dentro da leitura, o toggle limpa esta marca e preserva a
  // escolha dele. O ref espelha o valor atual para evitar dependência de estado
  // no effect (sem loops de render).
  const isChainExpandedRef = useRef(isChainExpanded);
  isChainExpandedRef.current = isChainExpanded;
  const forcedChainExpandRef = useRef<{ restore: boolean } | null>(null);
  useEffect(() => {
    if (isReading) {
      if (!isChainExpandedRef.current) {
        forcedChainExpandRef.current = { restore: isChainExpandedRef.current };
        setIsChainExpanded(true);
      }
    } else if (forcedChainExpandRef.current) {
      const { restore } = forcedChainExpandRef.current;
      forcedChainExpandRef.current = null;
      setIsChainExpanded(restore);
    }
  }, [isReading]);

  useEffect(() => {
    if (shouldDeferHeavyContent && !previousShouldDeferHeavyContentRef.current) {
      setIsHeavyContentReady(false);
    }
    if (!shouldDeferHeavyContent) {
      setIsHeavyContentReady(true);
    }
    previousShouldDeferHeavyContentRef.current = shouldDeferHeavyContent;
  }, [shouldDeferHeavyContent]);

  useEffect(() => {
    if (!shouldDeferHeavyContent || isHeavyContentReady) return;
    const element = messageRef.current;
    if (!element || typeof IntersectionObserver === 'undefined') {
      setIsHeavyContentReady(true);
      return;
    }
    const observer = new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) {
        setIsHeavyContentReady(true);
        observer.disconnect();
      }
    }, { rootMargin: '640px 0px' });
    observer.observe(element);
    return () => observer.disconnect();
  }, [isHeavyContentReady, shouldDeferHeavyContent]);

  return (
    <ToolInvocationDialogsProvider currentCalls={dialogCalls}>
    <div
      ref={messageRef}
      className={`chat-message chat-message--${role} ${isEditing ? 'chat-message--editing' : ''} ${isReading ? 'chat-message--reading' : ''}`}
      aria-label={isEditing || isReading
        ? undefined
        : `${message.pinned ? `${t('chat.pinnedMessage')}. ` : ''}${getAriaLabel()}`}
      aria-busy={!isReading && effectiveIsStreaming && !isAgenticStreaming}
      onKeyDown={handleKeyDown}
      onKeyUp={handleKeyUp}
      onContextMenu={handleContextMenuEvent}
      onFocus={handleFocus}
      tabIndex={-1}
    >
      {isReading && (
        <div className="chat-message__reading-badge">
          {t('chat.reading')}
        </div>
      )}
      <div className="chat-message__avatar" aria-hidden="true">
        {role === 'user' ? (
          <div className="chat-message__avatar-user">U</div>
        ) : (
          <div className="chat-message__avatar-assistant">
            {isAgentMessage(message) ? <RobotOutlined /> : 'AI'}
          </div>
        )}
      </div>
      <div className="chat-message__content">
        <div className="chat-message__header">
          <h3 className="chat-message__role">
            {getDisplayRole()}
          </h3>
          {message.pinned && (
            <span className="chat-message__pinned-badge">
              <span aria-hidden="true">📌</span>
              {t('chat.pinnedMessage')}
            </span>
          )}
          {message.source && message.source !== 'wails' && message.source !== '' && (
            <span className="chat-message__source-badge" aria-label={`${t('chat.via')} ${message.source}`}>
              {message.source === 'telegram' && <SendOutlined aria-hidden="true" />}
              {message.source === 'signal' && <LockOutlined aria-hidden="true" />}
              {message.source === 'whatsapp' && <MessageOutlined aria-hidden="true" />}
              {!['telegram', 'signal', 'whatsapp'].includes(message.source) && <MobileOutlined aria-hidden="true" />}
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
              tabIndex={isReading ? 0 : -1}
            >
              {isPlayingAudio ? <PauseCircleOutlined aria-hidden="true" /> : <SoundOutlined aria-hidden="true" />}
            </button>
          )}
          {hasThreadIndicator && onThreadToggle && (
            <ThreadIndicator
              childCount={threadChildCount}
              isExpanded={isThreadExpanded}
              isLoading={isThreadLoading}
              tabNavigationEnabled={isReading}
              onToggle={onThreadToggle}
            />
          )}
        </div>

        {role === 'assistant' && ['failed', 'interrupted'].includes(message.timelineItem?.state ?? '') && (
          <p>{t(message.timelineItem?.state === 'failed' ? 'chat.timelineFailed' :
            message.timelineItem?.metadata.terminalReason === 'limit_reached' ? 'chat.timelineLimitReached' : 'chat.timelineInterrupted')}</p>
        )}

        {/* Seção de Reasoning/Thinking - exibe cadeia de pensamento do modelo */}
        {role === 'assistant' && (streamingReasoning || effectiveReasoning) && (
          <ReasoningSection 
            reasoning={streamingReasoning || effectiveReasoning || ''} 
            isStreaming={isThinking}
            isExpanded={isThinking || isReasoningExpanded}
            tabNavigationEnabled={isReading}
            onToggle={onToggleReasoning}
          />
        )}

        {/* Interleaved segments: text → tools → text → tools → final answer */}
        {role === 'assistant' && hasAgenticSegments && !isEditing ? (
          <>
            {/* Issue #163: turnos concluídos expõem um controle acessível para
                contrair/expandir a cadeia inteira (segmentos + tool calls). Durante
                o streaming a cadeia permanece sempre visível (log ao vivo). */}
            {!isAgenticStreaming && (
              <button
                type="button"
                className={`chat-message__chain-toggle${isChainExpanded ? ' chat-message__chain-toggle--expanded' : ''}`}
                onClick={(e) => { e.stopPropagation(); forcedChainExpandRef.current = null; setIsChainExpanded((prev) => !prev); }}
                aria-expanded={isChainExpanded}
                aria-controls={chainRegionId}
                tabIndex={isReading ? 0 : -1}
              >
                {isChainExpanded ? t('chat.collapseChain') : t('chat.expandChain')}
              </button>
            )}
            {/* Completed segments stay navigable without creating a local live region;
                progress announcements are brokered globally with surface origin. */}
            <div
              id={chainRegionId}
              aria-label={isAgenticStreaming ? t('chat.progressLabel') : undefined}
              className="chat-message__segments-log"
            >
              {(isAgenticStreaming || isChainExpanded) && (canRenderHeavyContent ? chainSegments.map((seg, idx) => (
                <React.Fragment key={message.timelineItem?.segments.filter(segment => segment.kind !== 'reasoning')[idx]?.segmentId}>
                  {seg.type === 'text' && seg.content && (
                    <div className="chat-message__text chat-message__text--segment">
                      <MarkdownRenderer
                        content={message.role === 'assistant' ? translateBackendErrorDiagnostic(seg.content) : seg.content}
                        tabNavigation={renderedTabNavigation}
                        interactiveButtons={!!onSendToEditor}
                        enableSendToEditorButtons={!!onSendToEditor}
                        editorTargets={editorTargets}
                        onSendToEditor={sendBlockToEditor}
                      />
                    </div>
                  )}
                  {seg.type === 'tool_calls' && (seg.toolInvocations?.length) && (
                    <ToolCallsSection
                      toolInvocations={seg.toolInvocations}
                      tabNavigationEnabled={isReading}
                    />
                  )}
                </React.Fragment>
              )) : (
                <div className="chat-message__text chat-message__text--segment">
                  {t('chat.largeMessageDeferred')}
                </div>
              ))}
            </div>

            {isAgenticStreaming && rawTurnSegments[rawTurnSegments.length - 1]?.type === 'tool_calls' &&
              !dialogCalls.some(call => displayToolStatus(call.status) === 'running') && (
                <div className="chat-message__text chat-message__text--segment">{t('chat.waitingForNextStep')}</div>
              )}
            {/* Conclusão fora da região recolhível, sempre depois da atividade.
                O placeholder tool-only também descreve o estado final, não o início. */}
            {showConclusionAfterChain && (
              <div className="chat-message__text chat-message__text--segment chat-message__text--conclusion-preview">
                {canRenderHeavyContent ? (
                  <MarkdownRenderer
                    content={expandedConclusionContent}
                    tabNavigation={renderedTabNavigation}
                    interactiveButtons={!!onSendToEditor}
                    enableSendToEditorButtons={!!onSendToEditor}
                    editorTargets={editorTargets}
                    onSendToEditor={sendBlockToEditor}
                  />
                ) : (
                  <span>{t('chat.largeMessageDeferred')}</span>
                )}
              </div>
            )}

          </>
        ) : (
          <>
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
                    canRenderHeavyContent ? (
                    <MarkdownRenderer
                      content={displayContent}
                      tabNavigation={renderedTabNavigation}
                      interactiveButtons={!!onSendToEditor}
                      enableSendToEditorButtons={!!onSendToEditor}
                      editorTargets={editorTargets}
                      onSendToEditor={sendBlockToEditor}
                    />
                    ) : (
                      <span>{t('chat.largeMessageDeferred')}</span>
                    )
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
    </ToolInvocationDialogsProvider>
  );
});
