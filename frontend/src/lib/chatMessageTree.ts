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
  itemId: string;
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
}
