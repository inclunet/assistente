import React, { useRef, useEffect } from 'react';
import { Message } from '../../store/chatStore';
import { MarkdownRenderer } from '../ui/MarkdownRenderer';
import { ThreadIndicator } from './ThreadIndicator';
import { ReasoningSection } from './ReasoningSection';
import { ToolCallsSection, ToolCallStatus } from './ToolCallsSection';
import { isAgentMessage } from '../../lib/chatUtils';
import { stripMarkdown } from '../../lib/stripMarkdown';
import { formatRelativeTime } from '../../lib/dateUtils';
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
}) => {
  const { role, content, timestamp, isStreaming, reasoning, toolCalls } = message;
  const editTextareaRef = useRef<HTMLTextAreaElement>(null);

  // Usa editContent externo se está editando
  const editContent = isEditing ? externalEditContent : content;

  const formatTime = (timestamp: number) => {
    const date = new Date(timestamp);
    return date.toLocaleTimeString('pt-BR', {
      hour: '2-digit',
      minute: '2-digit',
    });
  };

  const getDisplayRole = () => {
    // Usuário
    if (role === 'user') return 'Você';

    // Resposta de ferramenta — mostra ID do call
    if (role === 'tool') return 'Resultado';

    // Assistente com tool calls pendentes
    if (role === 'assistant' && toolCalls) return 'Assistente';

    // Assistente padrão
    return 'Assistente';
  };

  const getAriaLabel = () => {
    const roleLabel = getDisplayRole();
    const contentPreview = content ? stripMarkdown(content) : 'Escrevendo';
    const playHint = role === 'assistant' && !isStreaming ? ' Pressione Espaço para reproduzir áudio.' : '';

    // Timestamp relativo com prefixo
    const relativeTime = formatRelativeTime(timestamp);
    const timePrefix = role === 'user' ? 'enviado' : 'recebido';

    // Inclui reasoning no aria-label quando expandido
    const reasoningText = reasoning || streamingReasoning;
    const reasoningLabel = (isReasoningExpanded && reasoningText) 
      ? ` Raciocínio: ${stripMarkdown(reasoningText)}.` 
      : '';

    return `${roleLabel}: ${contentPreview}.${reasoningLabel} ${timePrefix} ${relativeTime}.${playHint}`;
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
    if (e.key === ' ' && !isStreaming) {
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
      aria-live={isStreaming ? 'polite' : 'off'}
      aria-busy={isStreaming}
      onKeyDown={handleKeyDown}
      onKeyUp={handleKeyUp}
      onContextMenu={handleContextMenuEvent}
      onFocus={handleFocus}
      tabIndex={-1}
    >
      {isReading && (
        <div className="chat-message__reading-badge" aria-hidden="true">
          Lendo
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
            <span className="chat-message__source-badge" aria-label={`Via ${message.source}`}>
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
        {role === 'assistant' && (streamingReasoning || reasoning) && (
          <ReasoningSection 
            reasoning={streamingReasoning || reasoning || ''} 
            isStreaming={isThinking}
            isExpanded={isThinking || isReasoningExpanded} // Expandido durante streaming ou por toggle
            onToggle={onToggleReasoning}
          />
        )}

        {/* Seção de Tool Calls - exibe ferramentas chamadas pelo assistente */}
        {role === 'assistant' && (toolCalls || (isStreaming && activeToolCalls && activeToolCalls.length > 0)) && (
          <ToolCallsSection
            toolCallsJson={toolCalls}
            activeToolCalls={isStreaming ? activeToolCalls : undefined}
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
                placeholder="Edite sua mensagem..."
                rows={3}
                tabIndex={0}
                aria-label="Editar mensagem"
              />
              <div className="chat-message__edit-actions">
                <button
                  className="chat-message__edit-button chat-message__edit-button--cancel"
                  onClick={onCancelEdit}
                  aria-label="Cancelar"
                >
                  Cancelar
                </button>
                <button
                  className="chat-message__edit-button chat-message__edit-button--save"
                  onClick={onSaveEdit}
                  disabled={!editContent.trim()}
                  aria-label="Salvar"
                >
                  Salvar
                </button>
              </div>
            </div>
          ) : (
            <>
              {role === 'assistant' && content ? (
                <MarkdownRenderer content={content} />
              ) : (
                content || (isStreaming && <span className="chat-message__cursor">▋</span>)
              )}
            </>
          )}
        </div>
      </div>
    </div>
  );
});
