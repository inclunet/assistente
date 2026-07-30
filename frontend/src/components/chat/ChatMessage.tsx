import React, { useMemo, useRef, useEffect, useState, useId } from 'react';
import { useTranslation } from 'react-i18next';
import {
  ToolOutlined, RobotOutlined, SendOutlined, LockOutlined,
  MessageOutlined, MobileOutlined, SoundOutlined, PauseCircleOutlined,
} from '@ant-design/icons';
import type { Message, TurnSegment } from '../../store/chatStore';
import { getMessageTurnSegments } from '../../lib/chatMessageTree';
import { MarkdownRenderer } from '../ui/MarkdownRenderer';
import { ThreadIndicator } from './ThreadIndicator';
import { ReasoningSection } from './ReasoningSection';
import { ToolCallsSection } from './ToolCallsSection';
import type { ToolCallStatus } from '../../types/chat';
import { useChatMessageLiveState } from './ChatSessionContext';
import { isAgentMessage } from '../../lib/chatUtils';
import { formatRelativeTime } from '../../lib/dateUtils';
import { buildChatMessageAriaLabel } from '../../lib/chatMessageAriaLabel';
import type { EditorSendTargetOption, SendToEditorPayload } from '../../lib/editorSendMenu';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { stripMarkdown, plainSpeechDelta } from '../../lib/stripMarkdown';
import type { VoiceAccessibilityOrigin } from '../../services/voiceAccessibility/types';
import './ChatMessage.css';

const HEAVY_MARKDOWN_CONTENT_LENGTH = 8_000;
const HEAVY_ARIA_CONTENT_PREVIEW_LENGTH = 1_200;
const HEAVY_AGENTIC_SEGMENT_COUNT = 8;
const EMPTY_STREAM_CLEANUP_MS = 30_000;
const TOOL_ONLY_TURN_PLACEHOLDER_SOURCE = 'tool_only_turn_placeholder';

interface StreamingAnnouncementState {
  previous: string;
  previousOriginKey: string;
  wasStreaming: boolean;
  emptyCompletionAnnounced: boolean;
  cleanupTimer?: number;
}

const streamingAnnouncementStates = new Map<string, StreamingAnnouncementState>();

function getStreamingAnnouncementState(messageId: string): StreamingAnnouncementState {
  let state = streamingAnnouncementStates.get(messageId);
  if (!state) {
    state = { previous: '', previousOriginKey: '', wasStreaming: false, emptyCompletionAnnounced: false };
    streamingAnnouncementStates.set(messageId, state);
  } else if (state.cleanupTimer !== undefined) {
    window.clearTimeout(state.cleanupTimer);
    state.cleanupTimer = undefined;
  }
  return state;
}

function getStreamingAnnouncementOriginKey(origin?: VoiceAccessibilityOrigin): string {
  if (!origin) return '';
  return [
    origin.surfaceType ?? '',
    origin.surfaceId ?? '',
    origin.sessionKey ?? '',
    origin.tabId ?? '',
    origin.conversationId ?? '',
    origin.profileSlug ?? '',
  ].join('|');
}

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
  editorTargets?: EditorSendTargetOption[];
  onSendToEditor?: (payload: SendToEditorPayload) => void;
  origin?: VoiceAccessibilityOrigin;
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
  editorTargets,
  onSendToEditor,
  origin,
}) => {
  const { t } = useTranslation();
  const { announceRequest } = useAnnouncer();
  const { role, content, timestamp, isStreaming, reasoning, toolCalls } = message;
  const messageRef = useRef<HTMLDivElement>(null);
  const chainRegionId = useId();
  // Issue #163: a cadeia do turno (segmentos intermediários + tool calls) ganha
  // uma affordance acessível de contrair/expandir. Inicia expandida para
  // preservar o layout visual já existente (a cadeia é exibida agrupada); o
  // controle expõe `aria-expanded` e permite recolher a cadeia por economia.
  const [isChainExpanded, setIsChainExpanded] = useState(true);
  const editTextareaRef = useRef<HTMLTextAreaElement>(null);
  const previousShouldDeferHeavyContentRef = useRef(false);
  const {
    liveContent,
    liveIsStreaming,
    liveReasoning,
    liveToolCallsRaw,
    liveSegments,
    liveToolCalls,
  } = useChatMessageLiveState(message);

  const completedSegments = liveSegments.length > 0 ? liveSegments : _completedSegmentsProp;
  const effectiveToolCalls = liveToolCalls.length > 0 ? liveToolCalls : activeToolCalls;

  const effectiveContent = liveContent !== null ? liveContent : content;
  const effectiveIsStreaming = liveIsStreaming !== null ? liveIsStreaming : isStreaming;
  const effectiveReasoning = liveReasoning !== null ? liveReasoning : reasoning;
  const effectiveToolCallsRaw = liveToolCallsRaw !== null ? liveToolCallsRaw : toolCalls;
  const isToolOnlyTurnPlaceholder = message.source === TOOL_ONLY_TURN_PLACEHOLDER_SOURCE;
  const placeholderContent = isToolOnlyTurnPlaceholder && !effectiveContent
    ? t('chat.toolOnlyTurnPlaceholder')
    : effectiveContent;

  // Segmentos cronológicos do turno: durante streaming usamos override em
  // memória; turnos persistidos vêm com `turnSegments` canônicos do backend
  // (Issue #150) para preservar a cadeia de raciocínio em UMA única entrada.
  const persistedTurnSegments = getMessageTurnSegments(message);
  const hasAgenticSegments = !!(persistedTurnSegments || (completedSegments && completedSegments.length > 0));
  const isAgenticStreaming = effectiveIsStreaming && hasAgenticSegments;

  // Turnos "tool-only" (assistente não emitiu texto, só executou tools) chegam
  // do backend como um único segmento `tool_calls`. Sem injetar o placeholder
  // textual antes das tools, o leitor de tela perde o contexto e a entrada
  // soa como resposta cortada — preserva paridade com o branch flat que já
  // renderiza `placeholderContent` (Issue #150 follow-up).
  const rawTurnSegments = persistedTurnSegments || completedSegments || [];
  const shouldInjectToolOnlyPlaceholder =
    isToolOnlyTurnPlaceholder &&
    rawTurnSegments.length > 0 &&
    !rawTurnSegments.some((seg) => seg.type === 'text' && !!seg.content);
  const displaySegments: TurnSegment[] = shouldInjectToolOnlyPlaceholder
    ? [
        { type: 'text', content: t('chat.toolOnlyTurnPlaceholder') } as TurnSegment,
        ...rawTurnSegments,
      ]
    : rawTurnSegments;

  // Issues #160/#163: em turnos agênticos (texto → tools → … → texto final) o
  // leitor de tela deve anunciar APENAS a CONCLUSÃO do turno — não trechos
  // intermediários. A regra de seleção da conclusão é determinística, nesta
  // ordem de precedência:
  //
  //   1. Streaming agêntico ativo: o conteúdo ao vivo (`effectiveContent`) tem
  //      precedência. O trecho mais recente costuma ainda estar em
  //      `effectiveContent` (iteração em curso) e só vira um `TurnSegment`
  //      concluído depois; usá-lo torna o anúncio substitutivo (reflete o
  //      último texto disponível, não o segmento anterior já fechado) — #160.
  //   2. `message.content` consolidado, quando presente e não-vazio: é a fonte
  //      de verdade da resposta final. O backend (`consolidateTimelineTurn`)
  //      define `content = finalContent`, ou seja, o ÚLTIMO conteúdo textual
  //      não-vazio emitido pelo assistente no turno — a conclusão canônica —
  //      mesmo quando ela não é o último `TurnSegment` da cadeia (#163).
  //   3. Fallback: último `TurnSegment` de texto não-vazio, para turnos cujo
  //      `content` escalar esteja ausente/vazio mas a cadeia traga texto.
  //   4. Placeholder, quando não há texto algum (ex.: turno tool-only).
  //
  // A cadeia completa continua acessível via browse mode (segmentos navegáveis)
  // e pelo modo de leitura (Enter), independente deste valor.
  const conclusionContent = useMemo(() => {
    if (effectiveIsStreaming && effectiveContent && effectiveContent.trim()) {
      return effectiveContent;
    }
    if (content && content.trim()) {
      return content;
    }
    for (let i = displaySegments.length - 1; i >= 0; i -= 1) {
      const seg = displaySegments[i];
      if (seg.type === 'text' && seg.content && seg.content.trim()) {
        return seg.content;
      }
    }
    return placeholderContent || '';
  }, [effectiveIsStreaming, effectiveContent, content, displaySegments, placeholderContent]);

  // Usa editContent externo se está editando
  const editContent = isEditing ? externalEditContent : effectiveContent;

  const toolCallsHasTextEdit =
    role === 'assistant' &&
    typeof effectiveToolCallsRaw === 'string' &&
    /"name"\s*:\s*"text_edit"/i.test(effectiveToolCallsRaw);

  // Quando `text_edit` é usado, o conteúdo do assistente pode vir poluído com fences (ex.: ```markdown).
  // Como a UI já mostra as tool calls, omitimos o corpo textual para evitar ruído.
  const displayContent = isEditing ? externalEditContent : (toolCallsHasTextEdit ? '' : placeholderContent);
  const segmentCount = (persistedTurnSegments || completedSegments || []).length;
  const shouldDeferHeavyContent =
    !effectiveIsStreaming &&
    !isReading &&
    !isEditing &&
    (
      displayContent.length > HEAVY_MARKDOWN_CONTENT_LENGTH ||
      (effectiveToolCallsRaw?.length ?? 0) > HEAVY_MARKDOWN_CONTENT_LENGTH ||
      segmentCount > HEAVY_AGENTIC_SEGMENT_COUNT
    );
  const [isHeavyContentReady, setIsHeavyContentReady] = useState(!shouldDeferHeavyContent);
  const canRenderHeavyContent = !shouldDeferHeavyContent
    || (isHeavyContentReady && previousShouldDeferHeavyContentRef.current)
    || isReading
    || isEditing;
  const deferredToolCallsAriaRaw = useMemo(() => {
    if (!shouldDeferHeavyContent || !effectiveToolCallsRaw) return effectiveToolCallsRaw;
    const matches = Array.from(
      effectiveToolCallsRaw
        .slice(0, HEAVY_ARIA_CONTENT_PREVIEW_LENGTH * 4)
        .matchAll(/"name"\s*:\s*"([^"]+)"/g),
    );
    const names = matches
      .map((match) => match[1])
      .filter((name): name is string => !!name)
      .slice(0, 5);
    return names.length
      ? JSON.stringify(names.map((name) => ({ function: { name } })))
      : null;
  }, [effectiveToolCallsRaw, shouldDeferHeavyContent]);

  useEffect(() => {
    if (role !== 'assistant') return;

    const text = conclusionContent.trim();
    if (effectiveIsStreaming) {
      const announcementState = getStreamingAnnouncementState(message.id);
      announcementState.wasStreaming = true;
      announcementState.emptyCompletionAnnounced = false;
      const progressMessage = text || (isAgenticStreaming ? t('chat.progressLabel') : '');
      if (!progressMessage) return;

      // previous guarda o texto JÁ anunciado (plain). Anunciamos só o delta
      // — com aria-atomic=true, mandar o acumulado faz o NVDA reler tudo.
      const codeBlockLabel = t('chat.codeBlockSpeechLabel');
      const plain = stripMarkdown(progressMessage, { codeBlockLabel });
      if (!plain) return;

      const previous = announcementState.previous;
      const originKey = getStreamingAnnouncementOriginKey(origin);
      const sameOrigin = announcementState.previousOriginKey === originKey;
      if (previous === plain && sameOrigin) return;

      // Nova superfície: reanunciar o acumulado (AEP-0058 / teste de origem).
      if (!sameOrigin) {
        const didAnnounce = announceRequest({
          message: plain,
          origin,
          eventType: 'progress',
        });
        if (didAnnounce) {
          announcementState.previous = plain;
          announcementState.previousOriginKey = originKey;
        }
        return;
      }

      // Reescrita do plain (fechamento de markdown / substituição): anunciar
      // o acumulado — o LCP sozinho pode ficar curto e o limiar de 80 silencia.
      if (previous && !plain.startsWith(previous)) {
        const didAnnounce = announceRequest({
          message: plain,
          origin,
          eventType: 'progress',
        });
        if (didAnnounce) {
          announcementState.previous = plain;
          announcementState.previousOriginKey = originKey;
        }
        return;
      }

      // LCP evita reler tudo quando o fechamento de markdown reescreve o plain.
      const delta = plainSpeechDelta(previous, plain);
      const progressedEnough = delta.length >= 80;
      const reachedSentenceBoundary = /[.!?…]\s*$/.test(plain);
      if (!delta.trim()) return;
      if (previous && !progressedEnough && !reachedSentenceBoundary) return;

      const didAnnounce = announceRequest({
        message: delta.trimStart(),
        origin,
        eventType: 'progress',
      });
      if (didAnnounce) {
        announcementState.previous = plain;
        announcementState.previousOriginKey = originKey;
      }
      return;
    }

    const announcementState = streamingAnnouncementStates.get(message.id);
    if (!announcementState) return;

    if (!announcementState.wasStreaming) {
      if (!text) announcementState.previous = '';
      return;
    }

    const previousPlain = announcementState.previous;
    announcementState.previousOriginKey = '';
    if (!text) {
      announcementState.previous = '';
      if (hasAgenticSegments && !announcementState.emptyCompletionAnnounced) {
        announcementState.emptyCompletionAnnounced = true;
        announceRequest({
          message: t('chat.progressLabel'),
          origin,
          eventType: 'completion',
        });
      }
      if (announcementState.cleanupTimer !== undefined) {
        window.clearTimeout(announcementState.cleanupTimer);
      }
      announcementState.cleanupTimer = window.setTimeout(() => {
        const latestState = streamingAnnouncementStates.get(message.id);
        if (latestState === announcementState) {
          latestState.cleanupTimer = undefined;
          if (!latestState.previous) {
            streamingAnnouncementStates.delete(message.id);
          }
        }
      }, EMPTY_STREAM_CLEANUP_MS);
      return;
    }

    const plain = stripMarkdown(text, { codeBlockLabel: t('chat.codeBlockSpeechLabel') });
    // Extensão limpa → só o sufixo; reescrita (LCP parcial) → anunciar o plain inteiro.
    const remainder = (
      previousPlain && plain.startsWith(previousPlain)
        ? plain.slice(previousPlain.length)
        : plain
    ).trimStart();
    streamingAnnouncementStates.delete(message.id);
    if (!remainder) return;
    announceRequest({
      message: remainder,
      origin,
      eventType: 'completion',
    });
  }, [announceRequest, conclusionContent, effectiveIsStreaming, hasAgenticSegments, isAgenticStreaming, message.id, origin, role, t]);

  useEffect(() => () => {
    const announcementState = streamingAnnouncementStates.get(message.id);
    if (!announcementState) return;
    if (announcementState.cleanupTimer !== undefined) {
      window.clearTimeout(announcementState.cleanupTimer);
    }
    announcementState.cleanupTimer = window.setTimeout(() => {
      if (streamingAnnouncementStates.get(message.id) === announcementState) {
        streamingAnnouncementStates.delete(message.id);
      }
    }, EMPTY_STREAM_CLEANUP_MS);
  }, [message.id]);

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
    const timePrefix = role === 'user' ? t('chat.sent') : t('chat.received');

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
        toolCallsRaw: deferredToolCallsAriaRaw,
        toolCallsHasTextEdit,
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
    <div
      ref={messageRef}
      className={`chat-message chat-message--${role} ${isEditing ? 'chat-message--editing' : ''} ${isReading ? 'chat-message--reading' : ''}`}
      aria-label={isEditing || isReading ? undefined : getAriaLabel()}
      aria-busy={effectiveIsStreaming && !isAgenticStreaming}
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
        ) : role === 'tool' ? (
          <div className="chat-message__avatar-tool"><ToolOutlined /></div>
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
          {message.source && message.source !== 'wails' && message.source !== '' && !isToolOnlyTurnPlaceholder && (
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
              tabIndex={-1}
            >
              {isPlayingAudio ? <PauseCircleOutlined aria-hidden="true" /> : <SoundOutlined aria-hidden="true" />}
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
            {/* Issue #163: com a cadeia recolhida (economia), ainda exibimos a
                CONCLUSÃO do turno — mesma fonte de verdade do aria-label — para a
                mensagem não ficar vazia. As tool calls e os segmentos
                intermediários ficam ocultos até expandir. Fica FORA da região
                controlada pelo toggle (chainRegionId) para manter `aria-expanded`
                coerente com o conteúdo da cadeia. */}
            {!isAgenticStreaming && !isChainExpanded && conclusionContent && (
              <div className="chat-message__text chat-message__text--segment chat-message__text--conclusion-preview">
                {canRenderHeavyContent ? (
                  <MarkdownRenderer
                    content={conclusionContent}
                    interactiveButtons={!!onSendToEditor}
                    focusableMermaid={!!onSendToEditor}
                    enableSendToEditorButtons={!!onSendToEditor}
                    editorTargets={editorTargets}
                    onSendToEditor={onSendToEditor}
                  />
                ) : (
                  <span>{t('chat.largeMessageDeferred')}</span>
                )}
              </div>
            )}
            {/* Completed segments stay navigable without creating a local live region;
                progress announcements are brokered globally with surface origin. */}
            <div
              id={chainRegionId}
              aria-label={isAgenticStreaming ? t('chat.progressLabel') : undefined}
              className="chat-message__segments-log"
            >
              {(isAgenticStreaming || isChainExpanded) && (canRenderHeavyContent ? displaySegments.map((seg, idx) => (
                <React.Fragment key={idx}>
                  {seg.type === 'text' && seg.content && (
                    <div className="chat-message__text chat-message__text--segment">
                      <MarkdownRenderer
                        content={seg.content}
                        interactiveButtons={!!onSendToEditor}
                        focusableMermaid={!!onSendToEditor}
                        enableSendToEditorButtons={!!onSendToEditor}
                        editorTargets={editorTargets}
                        onSendToEditor={onSendToEditor}
                      />
                    </div>
                  )}
                  {seg.type === 'tool_calls' && seg.toolCalls && (
                    <ToolCallsSection
                      toolCallsJson={JSON.stringify(seg.toolCalls)}
                    />
                  )}
                </React.Fragment>
              )) : (
                <div className="chat-message__text chat-message__text--segment">
                  {t('chat.largeMessageDeferred')}
                </div>
              ))}
            </div>

            {/* Current iteration keeps busy state without local aria-live updates. */}
            <div aria-busy={effectiveIsStreaming}>
              {effectiveIsStreaming && effectiveToolCalls && effectiveToolCalls.length > 0 && (
                <ToolCallsSection activeToolCalls={effectiveToolCalls} />
              )}

              {effectiveIsStreaming && displayContent && !persistedTurnSegments && (
                <div className="chat-message__text">
                  <MarkdownRenderer
                    content={displayContent}
                    interactiveButtons={!!onSendToEditor}
                    focusableMermaid={!!onSendToEditor}
                    enableSendToEditorButtons={!!onSendToEditor}
                    editorTargets={editorTargets}
                    onSendToEditor={onSendToEditor}
                  />
                </div>
              )}
              {effectiveIsStreaming && !displayContent && !persistedTurnSegments && (
                <div className="chat-message__text">
                  <span className="chat-message__cursor">▋</span>
                </div>
              )}
            </div>
          </>
        ) : (
          <>
            {/* Non-agentic messages: flat layout (reasoning → tools → content) */}
            {role === 'assistant' && (canRenderHeavyContent || effectiveIsStreaming) && (effectiveToolCallsRaw || (effectiveIsStreaming && effectiveToolCalls && effectiveToolCalls.length > 0)) && (
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
                    canRenderHeavyContent ? (
                    <MarkdownRenderer
                      content={displayContent}
                      interactiveButtons={!!onSendToEditor}
                      focusableMermaid={!!onSendToEditor}
                      enableSendToEditorButtons={!!onSendToEditor}
                      editorTargets={editorTargets}
                      onSendToEditor={onSendToEditor}
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
  );
});
