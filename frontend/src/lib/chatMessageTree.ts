import type { ToolOrigin } from '../types/chat';
import type { TimelineItem } from '../types/chatTimelineEvents';

export interface ToolInvocationSummary {
  invocationId?: string;
  callId: string;
  name: string;
  origin?: ToolOrigin;
  serverLabel?: string;
  status: string;
  iteration?: number;
  durationMs?: number;
  inputPreview?: string;
  outputPreview?: string;
  inputBytes?: number;
  outputBytes?: number;
  hasDetails: boolean;
  resultAvailability: string;
  hasSearchResults?: boolean;
  searchResultCount?: number;
  securityOutcome?: 'approved' | 'blocked';
}

export interface StreamingToolCall {
  errorCode?: string;
  id: string;
  type: string;
  function: { name: string; arguments: string };
  result?: string;
  origin?: ToolOrigin;
  serverLabel?: string;
  status?: 'running' | 'done' | 'error' | 'cancelled';
}

export interface TurnSegment {
  type: 'text' | 'tool_calls';
  content?: string;
  /** Aviso da apresentação; não integra o conteúdo retomado pelo modelo. */
  diagnostic?: boolean;
  /** Projeção leve persistida; detalhes são carregados sob demanda. */
  toolInvocations?: ToolInvocationSummary[];
  /** Estado transitório enquanto o turno ainda está em streaming. */
  toolCalls?: StreamingToolCall[];
}

export interface MessageNode {
  itemId?: string;
  level: number;
  childCount: number;
  message: Message;
  children?: MessageNode[];
  originalIndex?: number;
  isExpanded?: boolean;
};

/** Dados de apresentação derivados exclusivamente dos itens canônicos. */
export interface Message {
  id: string;
  conversationId: string;
  parentId?: string;
  turnId?: string;
  role: string;
  content: string;
  reasoning?: string;
  media?: string;
  promptTokens?: number;
  completionTokens?: number;
  totalTokens?: number;
  cacheReadTokens?: number;
  cacheWriteTokens?: number;
  cacheMissTokens?: number;
  model?: string;
  source?: string;
  pinned: boolean;
  createdAt: string;
  timestamp: number;
  isStreaming: boolean;
  internal: boolean;
  turnSegments?: TurnSegment[];
  timelineItem?: TimelineItem;
  _turnSegments?: TurnSegment[];
  _resumableContent?: string;
}

// getMessageTurnSegments retorna os segmentos cronológicos do turno preferindo
// o override transitório (`_turnSegments`, populado durante o streaming
// agentic) e caindo para os segmentos canônicos enviados pelo backend
// (`turnSegments`). Issue #150 garante que ambas as fontes existam para que
// recarregar o histórico mantenha a cadeia de raciocínio em UMA única entrada.
export function getMessageTurnSegments(message: Message): TurnSegment[] | undefined {
  if (message._turnSegments !== undefined) {
    return message._turnSegments as TurnSegment[];
  }
  const canonical = (message as Message & { turnSegments?: TurnSegment[] }).turnSegments;
  if (canonical && canonical.length > 0) return canonical;
  return undefined;
}

export function isContentRepresentedBySegments(segments: TurnSegment[] | undefined, content: string): boolean {
  if (!content || !segments) return false;
  const texts = segments.filter(segment => segment.type === 'text' && !segment.diagnostic)
    .map(segment => segment.content ?? '');
  return texts.includes(content) || texts.some((_, index) => texts.slice(index).join('') === content);
}

export interface ChatTreeConversation {
  id: string;
  title: string;
  threadedMessages: MessageNode[];
  channel?: string;
  contactId?: string;
}

function cloneMessage(message: Message, overrides: Partial<Message> = {}): Message {
  return { ...message, ...overrides };
}

function createNode(input: Partial<MessageNode> & { message: Message }): MessageNode {
  return { childCount: 0, level: 0, ...input, children: input.children ? [...input.children] : [] };
}

function cloneNode(node: MessageNode, overrides: Partial<MessageNode> = {}): MessageNode {
  const children = overrides.children ?? node.children;
  const message = overrides.message ?? node.message;
  const cloned = createNode({
    ...node,
    ...overrides,
    message,
    children: [],
  });
  cloned.children = children;
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

export function withOriginalIndex(node: MessageNode, index: number): MessageNode {
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
  let changed = false;
  const nextNodes = nodes.map((node) => {
    const nextNode = visitor(node);
    let result = nextNode;
    if (nextNode.children?.length) {
      const nextChildren = mapMessageTree(nextNode.children, visitor);
      if (nextChildren !== nextNode.children) {
        result = cloneNode(nextNode, { children: nextChildren });
      }
    }
    if (result !== node) changed = true;
    return result;
  });
  return changed ? nextNodes : nodes;
}

export function updateMessageContentInTree(nodes: MessageNode[], messageId: string, content: string): MessageNode[] {
  return mapMessageTree(nodes, (node) => {
    if (String(node.message.id) !== messageId) return node;
    if (node.message.content === content) return node;
    return cloneNode(node, { message: cloneMessage(node.message, { content }) });
  });
}

export function updateMessageSegmentsInTree(nodes: MessageNode[], messageId: string, segments: TurnSegment[], resumableContent?: string): MessageNode[] {
  return mapMessageTree(nodes, (node) => {
    if (String(node.message.id) !== messageId) return node;
    const updated = cloneMessage(node.message, { _turnSegments: segments });
    if (resumableContent === undefined) delete updated._resumableContent;
    else updated._resumableContent = resumableContent;
    return cloneNode(node, { message: updated });
  });
}

// An edit replaces received text, but keeps executions that may not yet appear
// in the canonical projection. Streaming updates preserve all their segments.
export function updateEditedMessageContentInTree(nodes: MessageNode[], messageId: string, content: string): MessageNode[] {
  return mapMessageTree(nodes, (node) => {
    if (String(node.message.id) !== messageId) return node;
    const override = (node.message as Message)._turnSegments;
    const onlyEditedText = override?.length === 1 && override[0].type === 'text' && override[0].content === content && !override[0].diagnostic;
    if (node.message.content === content && (override === undefined || onlyEditedText)
      && (node.message as Message)._resumableContent === undefined) return node;
    const edited = cloneMessage(node.message, { content });
    delete edited._resumableContent;
    const previousSegments = getMessageTurnSegments(edited);
    const receivedTools = previousSegments?.filter(segment => segment.type === 'tool_calls') ?? [];
    if (previousSegments !== undefined) {
      edited._turnSegments = [...receivedTools, { type: 'text', content }];
    } else {
      delete edited._turnSegments;
    }
    return cloneNode(node, { message: edited });
  });
}

export function updateMessagePinnedInTree(nodes: MessageNode[], messageId: string, pinned: boolean): MessageNode[] {
  return mapMessageTree(nodes, (node) => {
    if (String(node.message.id) !== messageId) return node;
    return cloneNode(node, { message: cloneMessage(node.message, { pinned }) });
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
