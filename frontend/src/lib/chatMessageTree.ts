import { chat } from '../../wailsjs/go/models';

export interface TurnSegment {
  type: 'text' | 'tool_calls';
  content?: string;
  toolCalls?: Array<{
    id: string;
    type: string;
    function: { name: string; arguments: string };
    result?: string;
  }>;
}

export type MessageNode = chat.MessageNode & {
  originalIndex?: number;
  isExpanded?: boolean;
};

// Issue #150: o backend agora envia segmentos canônicos via `turnSegments`
// (campo gerado pelo Wails). `_turnSegments` permanece como override
// transitório usado pelo controlador de streaming até o turno ser persistido.
export type Message = chat.EnrichedMessage & {
  _turnSegments?: TurnSegment[];
};

// getMessageTurnSegments retorna os segmentos cronológicos do turno preferindo
// o override transitório (`_turnSegments`, populado durante o streaming
// agentic) e caindo para os segmentos canônicos enviados pelo backend
// (`turnSegments`). Issue #150 garante que ambas as fontes existam para que
// recarregar o histórico mantenha a cadeia de raciocínio em UMA única entrada.
export function getMessageTurnSegments(message: Message): TurnSegment[] | undefined {
  if (message._turnSegments && message._turnSegments.length > 0) {
    return message._turnSegments as TurnSegment[];
  }
  const canonical = (message as Message & { turnSegments?: TurnSegment[] }).turnSegments;
  if (canonical && canonical.length > 0) return canonical;
  return undefined;
}

export interface ChatTreeConversation {
  id: string;
  title: string;
  threadedMessages: MessageNode[];
  channel?: string;
  contactId?: string;
}

function cloneMessage(message: Message, overrides: Partial<Message> = {}): Message {
  const cloned = new chat.EnrichedMessage({
    ...message,
    ...overrides,
  }) as Message;
  // Omitting _turnSegments in overrides preserves existing segments; pass [] to clear explicitly.
  const turnSegments = overrides._turnSegments ?? message._turnSegments;
  if (turnSegments) cloned._turnSegments = turnSegments;
  return cloned;
}

function createNode(input: Partial<MessageNode> & { message: Message }): MessageNode {
  const node = new chat.MessageNode({
    children: [],
    childCount: 0,
    level: 0,
    ...input,
  }) as MessageNode;
  if (!Object.prototype.hasOwnProperty.call(input.message, 'turnId')) {
    delete (node.message as Message & { turnId?: string }).turnId;
  }
  if ((input.message as Message)._turnSegments) {
    (node.message as Message)._turnSegments = (input.message as Message)._turnSegments;
  }
  if (input.children) {
    node.children = [...input.children];
  }
  return node;
}

function cloneNode(node: MessageNode, overrides: Partial<MessageNode> = {}): MessageNode {
  const cloned = createNode({
    ...node,
    ...overrides,
    message: overrides.message ?? node.message,
  });
  cloned.originalIndex = overrides.originalIndex ?? node.originalIndex;
  cloned.isExpanded = overrides.isExpanded ?? node.isExpanded;
  return cloned;
}

export function flattenThreadedMessages(nodes: MessageNode[] | undefined): Message[] {
  if (!nodes || nodes.length === 0) return [];
  const flat: Message[] = [];

  const traverse = (node: MessageNode) => {
    flat.push(node.message);
    if (node.children && node.children.length > 0) {
      node.children.forEach(traverse);
    }
  };

  nodes.forEach(traverse);
  return flat;
}

export function withOriginalIndex(node: chat.MessageNode, index: number): MessageNode {
  const typed = node as MessageNode;
  typed.originalIndex = index;
  return typed;
}

export function hasMessageId(
  nodes: MessageNode[] | undefined,
  targetId: string,
  excludeId?: string,
): boolean {
  if (!nodes || nodes.length === 0) return false;
  for (const node of nodes) {
    const id = String(node.message.id);
    if (id === targetId && id !== excludeId) return true;
    if (node.children?.length && hasMessageId(node.children, targetId, excludeId)) return true;
  }
  return false;
}

export function finalizeStreamingNode<TConversation extends ChatTreeConversation>(
  conversation: TConversation,
  syntheticId: string,
  finalId?: string | null,
  finalTurnId?: string | null,
): TConversation {
  const collidesWithExistingRealId = !!finalId && hasMessageId(conversation.threadedMessages, finalId, syntheticId);
  const finalMessagePatch: Partial<Message> = finalTurnId ? { turnId: finalTurnId } : {};
  const markDone = (nodes: MessageNode[]): MessageNode[] => nodes.flatMap((node) => {
    const id = String(node.message.id);
    if (id === syntheticId) {
      if (collidesWithExistingRealId) {
        return [];
      }
      return [cloneNode(node, {
        message: cloneMessage(node.message, {
          id: finalId ?? node.message.id,
          isStreaming: false,
          ...finalMessagePatch,
        }),
        children: node.children?.length ? markDone(node.children) : node.children,
      })];
    } else if (collidesWithExistingRealId && finalId && id === finalId) {
      return [cloneNode(node, {
        message: cloneMessage(node.message, { isStreaming: false, ...finalMessagePatch }),
        children: node.children?.length ? markDone(node.children) : node.children,
      })];
    }
    if (node.children?.length) {
      return [cloneNode(node, { children: markDone(node.children) })];
    }
    return [node];
  });

  return {
    ...conversation,
    threadedMessages: markDone(conversation.threadedMessages),
  };
}

function mapMessageTree(
  nodes: MessageNode[],
  visitor: (node: MessageNode) => MessageNode,
): MessageNode[] {
  return nodes.map((node) => {
    const nextNode = visitor(node);
    if (!nextNode.children?.length) return nextNode;
    return cloneNode(nextNode, { children: mapMessageTree(nextNode.children, visitor) });
  });
}

export function updateMessageContentInTree(nodes: MessageNode[], messageId: string, content: string): MessageNode[] {
  return mapMessageTree(nodes, (node) => {
    if (String(node.message.id) !== messageId) return node;
    return cloneNode(node, { message: cloneMessage(node.message, { content }) });
  });
}

export function markMessageStreamingInTree(nodes: MessageNode[], messageId: string, turnId?: string | null): MessageNode[] {
  const turnPatch: Partial<Message> = turnId ? { turnId } : {};
  return mapMessageTree(nodes, (node) => {
    if (String(node.message.id) !== messageId) return node;
    return cloneNode(node, { message: cloneMessage(node.message, { isStreaming: true, ...turnPatch }) });
  });
}

export function updateMessageReasoningInTree(nodes: MessageNode[], messageId: string, reasoning: string): MessageNode[] {
  return mapMessageTree(nodes, (node) => {
    if (String(node.message.id) !== messageId) return node;
    return cloneNode(node, { message: cloneMessage(node.message, { reasoning }) });
  });
}

export function attachChildrenToMessage(
  nodes: MessageNode[],
  messageId: string,
  children: MessageNode[],
): MessageNode[] {
  return nodes.map((node) => {
    if (String(node.message.id) === messageId) {
      return cloneNode(node, { children });
    }
    if (node.children && node.children.length > 0) {
      return cloneNode(node, {
        children: attachChildrenToMessage(node.children, messageId, children),
      });
    }
    return node;
  });
}
