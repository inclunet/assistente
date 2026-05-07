import { main } from '../../wailsjs/go/models';

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

export type MessageNode = main.MessageNode & {
  originalIndex?: number;
  isExpanded?: boolean;
};

export type Message = main.EnrichedMessage & {
  _turnSegments?: TurnSegment[];
};

export interface ChatTreeConversation {
  id: string;
  title: string;
  threadedMessages: MessageNode[];
  channel?: string;
  contactId?: string;
}

function cloneMessage(message: Message, overrides: Partial<Message> = {}): Message {
  return new main.EnrichedMessage({
    ...message,
    ...overrides,
  }) as Message;
}

function createNode(input: Partial<MessageNode> & { message: Message }): MessageNode {
  return new main.MessageNode({
    children: [],
    childCount: 0,
    level: 0,
    ...input,
  }) as MessageNode;
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

export function withOriginalIndex(node: main.MessageNode, index: number): MessageNode {
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

export function updateMessageReasoningInTree(nodes: MessageNode[], messageId: string, reasoning: string): MessageNode[] {
  return mapMessageTree(nodes, (node) => {
    if (String(node.message.id) !== messageId) return node;
    return cloneNode(node, { message: cloneMessage(node.message, { reasoning }) });
  });
}

function addChildToTree(
  nodes: MessageNode[],
  targetParentId: string,
  message: Message,
  level: number,
): { nodes: MessageNode[]; found: boolean } {
  let found = false;
  const updatedNodes = nodes.map((node) => {
    if (String(node.message.id) === targetParentId) {
      found = true;
      const existsInChildren = (node.children || []).some((child) => String(child.message.id) === String(message.id));
      if (existsInChildren) return node;
      const newChildNode = createNode({
        message,
        children: [],
        level: level + 1,
        childCount: 0,
      });
      return cloneNode(node, {
        children: [...(node.children || []), newChildNode],
        childCount: (node.childCount || 0) + 1,
      });
    }

    if (node.children && node.children.length > 0) {
      const result = addChildToTree(node.children, targetParentId, message, level + 1);
      if (result.found) {
        found = true;
        return cloneNode(node, { children: result.nodes });
      }
    }

    return node;
  });

  return { nodes: updatedNodes, found };
}

function findLastUserMessage(nodes: MessageNode[]): MessageNode | null {
  for (let i = nodes.length - 1; i >= 0; i -= 1) {
    if (nodes[i].message.role === 'user') return nodes[i];
  }
  return null;
}

export function appendInternalMessageToTree(nodes: MessageNode[], message: Message): MessageNode[] {
  const parentId = message.parentId?.toString();

  if (!parentId) {
    const newNode = createNode({
      message,
      children: [],
      level: 0,
      childCount: 0,
    });
    return [...nodes, newNode];
  }

  const directResult = addChildToTree(nodes, parentId, message, 0);
  if (directResult.found) return directResult.nodes;

  const lastUserMessage = findLastUserMessage(nodes);
  if (lastUserMessage) {
    const fallbackResult = addChildToTree(nodes, String(lastUserMessage.message.id), message, 0);
    if (fallbackResult.found) return fallbackResult.nodes;
  }

  const newNode = createNode({
    message,
    children: [],
    level: message.parentId ? 1 : 0,
    childCount: 0,
  });
  return [...nodes, newNode];
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
