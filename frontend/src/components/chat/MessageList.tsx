import React, { useEffect, useRef, useMemo, forwardRef } from 'react';
import { MessageNode as MessageNodeComponent } from './MessageNode';
import { MessageNode, Message } from '../../store/chatStore';
import { main } from '../../../wailsjs/go/models';
import './MessageList.css';

export interface MessageListProps {
  isLoading?: boolean;
  loadingText?: string; // Optional custom loading text
  // Estrutura hierárquica de mensagens (threads)
  threadedMessages: MessageNode[];
  // Callback para carregar filhos de uma mensagem
  onLoadChildren?: (messageId: string) => Promise<MessageNode[]>;
  // Callback quando chega ao fim da lista principal
  onReachEnd?: () => void;
  // Callbacks de ações
  onContextMenu?: (event: React.MouseEvent, message: Message) => void;
  onSpeak?: (message: Message) => void;
  onDelete?: (message: Message) => void;
}

/**
 * Consolida mensagens de turnos de tool calling em entradas únicas.
 * 
 * No agentic loop, um único turno gera múltiplas mensagens no banco:
 *   1. Assistant com toolCalls (intermediária)
 *   2. Tool results (role=tool)
 *   3. Assistant final (resposta)
 * 
 * Esta função agrupa todas essas mensagens pelo `turnId` e produz UMA
 * única entrada visual: a resposta final com todos os toolCalls coletados
 * como seção colapsável.
 * 
 * Mensagens sem turnId (conversas simples) passam inalteradas.
 */
function consolidateTurnMessages(nodes: MessageNode[]): MessageNode[] {
  if (!nodes || nodes.length === 0) return nodes;

  // Agrupa mensagens por turnId
  const turnMap = new Map<number, MessageNode[]>();
  let hasTurns = false;

  for (const node of nodes) {
    const turnId = node.message.turnId;
    if (turnId) {
      hasTurns = true;
      if (!turnMap.has(turnId)) turnMap.set(turnId, []);
      turnMap.get(turnId)!.push(node);
    }
  }

  // Se não há turnIds, retorna como está (conversa sem tool calling)
  if (!hasTurns) return nodes;

  const processedTurnIds = new Set<number>();
  const result: MessageNode[] = [];

  for (const node of nodes) {
    const turnId = node.message.turnId;

    // Mensagem sem turnId (user, ou assistant simples sem tools) — passa direto
    if (!turnId) {
      result.push(node);
      continue;
    }

    // Role=tool é sempre ocultado (representado pelo ToolCallsSection)
    if (node.message.role === 'tool') continue;

    // Este turno já foi consolidado — pula mensagens intermediárias
    if (processedTurnIds.has(turnId)) continue;
    processedTurnIds.add(turnId);

    // Consolida todas as mensagens deste turno
    const turnNodes = turnMap.get(turnId) || [];

    // 1. Coleta resultados das tools (role=tool) indexados por toolCallId
    const toolResults = new Map<string, string>();
    for (const tn of turnNodes) {
      if (tn.message.role === 'tool' && tn.message.toolCallId) {
        toolResults.set(tn.message.toolCallId, tn.message.content || '');
      }
    }

    // 2. Coleta tool calls e casa com seus resultados
    const allToolCalls: unknown[] = [];
    let finalContent = '';
    let finalReasoning = '';
    let finalNode = node;

    for (const tn of turnNodes) {
      if (tn.message.role !== 'assistant') continue;

      // Coleta tool calls e embutir resultado de cada uma
      if (tn.message.toolCalls) {
        try {
          const parsed = JSON.parse(tn.message.toolCalls);
          const calls = Array.isArray(parsed) ? parsed : [parsed];
          for (const call of calls) {
            // Enriquece cada call com o resultado correspondente
            const result = toolResults.get(call.id);
            allToolCalls.push({
              ...call,
              result: result ?? undefined,
            });
          }
        } catch {
          // JSON inválido — ignora
        }
      }

      // O último assistant com conteúdo é a resposta final
      if (tn.message.content) {
        finalContent = tn.message.content;
        finalNode = tn;
      }

      // Coleta reasoning (pode vir de qualquer iteração)
      if (tn.message.reasoning) {
        finalReasoning = tn.message.reasoning;
      }
    }

    // 3. Cria mensagem consolidada: resposta final + toolCalls com resultados
    // Usa createFrom para manter a classe Wails (com convertValues)
    const consolidatedMessage = main.EnrichedMessage.createFrom({
      ...finalNode.message,
      content: finalContent,
      reasoning: finalReasoning || finalNode.message.reasoning || '',
      toolCalls: allToolCalls.length > 0 ? JSON.stringify(allToolCalls) : undefined,
    });

    const consolidated = main.MessageNode.createFrom({
      ...finalNode,
      message: consolidatedMessage,
    }) as MessageNode;

    result.push(consolidated);
  }

  return result;
}

export const MessageList = forwardRef<HTMLDivElement, MessageListProps>((
  { isLoading = false, loadingText = 'Assistente está digitando', threadedMessages, onLoadChildren, onReachEnd, onContextMenu, onSpeak, onDelete },
  ref
) => {
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const internalContainerRef = useRef<HTMLDivElement>(null);
  
  // Use external ref if provided, otherwise use internal ref
  const containerRef = (ref as React.RefObject<HTMLDivElement>) || internalContainerRef;

  // Consolida mensagens de turnos com tool calling em entradas únicas
  const displayMessages = useMemo(
    () => consolidateTurnMessages(threadedMessages),
    [threadedMessages]
  );

  const scrollToBottom = (behavior: ScrollBehavior = 'smooth') => {
    messagesEndRef.current?.scrollIntoView({ behavior });
  };

  useEffect(() => {
    // Scroll to bottom when messages change
    scrollToBottom();
  }, [displayMessages]);

  useEffect(() => {
    // Instant scroll on mount
    scrollToBottom('instant');
  }, []);

  if (threadedMessages.length === 0) {
    return (
      <div 
        className="message-list message-list--empty"
        role="region"
        aria-label="Lista de mensagens da conversa"
      >
        <div className="message-list__empty-state">
          <div className="message-list__empty-icon">💬</div>
          <h3 className="message-list__empty-title">
            Comece uma nova conversa
          </h3>
          <p className="message-list__empty-description">
            Digite sua mensagem abaixo para começar a conversar com o assistente de IA.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div 
      className="message-list" 
      ref={containerRef}
      aria-label="Lista de mensagens da conversa"
    >
      <div className="message-list__messages">
        <div 
          role="list" 
          aria-label="Mensagens da conversa"
          tabIndex={0}
          onKeyDown={(e) => {
            const target = e.currentTarget;
            const firstChild = target.querySelector('[data-message-node]') as HTMLElement;
            if (firstChild && e.key === 'ArrowDown') {
              e.preventDefault();
              firstChild.focus();
            }
          }}
        >
          {displayMessages.map((node, index) => (
            <MessageNodeComponent
              key={node.message.id}
              node={node}
              level={0}
              siblingIndex={index}
              siblingCount={displayMessages.length}
              onLoadChildren={onLoadChildren}
              onReachEnd={onReachEnd}
              onContextMenu={onContextMenu}
              onSpeak={onSpeak}
              onDelete={onDelete}
            />
          ))}
        </div>
        {isLoading && (
          <div
            className="message-list__loading"
            role="status"
            aria-label={loadingText}
          >
            <div className="message-list__loading-dots" aria-hidden="true">
              <span></span>
              <span></span>
              <span></span>
            </div>
            <span className="message-list__loading-text">{loadingText}...</span>
          </div>
        )}
        <div ref={messagesEndRef} />
      </div>
    </div>
  );
});
