import type { TimelineItem, TimelineToolSummary } from '../types/chatTimelineEvents';

export type ToolInvocationSummary = TimelineToolSummary;

export interface TurnSegment {
  type: 'text' | 'tool_calls';
  content?: string;
  /** Resumos canônicos do ledger, inclusive durante streaming; detalhes sob demanda. */
  toolInvocations?: ToolInvocationSummary[];
}

export interface MessageNode {
  itemId: string;
  childCount: number;
  message: Message;
  children?: MessageNode[];
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
}
