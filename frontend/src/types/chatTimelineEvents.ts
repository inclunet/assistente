// Espelho manual dos payloads de eventos em internal/apidto (AEP-0116).
// Não importar payload de evento dos bindings gerados pelo Wails.
export const CHAT_TIMELINE_EVENT = 'chat:timeline';

export interface TimelineToolSummary {
  invocationId: string;
  callId: string;
  name: string;
  origin?: string;
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
  securityOutcome?: string;
}

export interface TimelineSegment {
  segmentId: string;
  kind: string;
  text?: string;
  invocations?: TimelineToolSummary[];
}

export interface TimelineItem {
  executionId?: string;
  recoveryReference?: TimelineRecoveryReference;
  itemId: string;
  conversationId: string;
  scopeParentId: string | null;
  kind: string;
  turnId: string | null;
  replyToItemId: string;
  messageIds: string[];
  representativeMessageId: string | null;
  revision: string;
  state: string;
  durability: string;
  segments: TimelineSegment[];
  metadata: {
    thinking?: boolean;
    terminalReason?: string;
    createdAt: string;
    source: string;
    childCount: number;
    conclusionSegmentIds: string[];
    cacheReadTokens: number;
    cacheWriteTokens: number;
    cacheMissTokens: number;
    origin: string;
    role: string;
    pinned: boolean;
    model: string;
    promptTokens: number;
    completionTokens: number;
    totalTokens: number;
  };
}

// Identidade emitida pelo backend; não é confirmação de persistência/ownership.
export interface TimelineRecoveryReference {
  projectionId: string;
  incarnation: string;
  turnId: string;
  executionId: string;
}

export interface TimelineOperation {
  kind: string;
  itemId: string;
  revision: string;
  item?: TimelineItem;
  segmentId?: string;
  text?: string;
  segment?: TimelineSegment;
}

export interface TimelineWindowMetadata {
  retainedItemIds: string[];
  retentionBlocked: boolean;
  totalCount: number;
  startIndex: number;
  endIndex: number;
  hasBefore: boolean;
  hasAfter: boolean;
  beforeCursor: string;
  afterCursor: string;
}

export interface TimelineWindowPatch {
  windowId: string;
  previousRevision: string;
  revision: string;
  splices?: { index: number; deleteCount: number; insertIds: string[] }[];
  items?: TimelineItem[];
  metadata?: TimelineWindowMetadata;
  syncRequired?: boolean;
}

export interface TimelineUpdate {
  conversationId: string;
  epoch: string;
  sequence: string;
  operations: TimelineOperation[];
  windowPatches: TimelineWindowPatch[];
}

export interface ChatTimelineEvent {
  surfaceId: string;
  conversationId: string;
  windowId: string;
  epoch: string;
  snapshotRevision: string;
  syncRequired: boolean;
  reason?: string;
  updates: TimelineUpdate[];
}

/** Campos omitidos no wire têm estes defaults na única borda de entrada. */
export type TimelineWireItem = Omit<TimelineItem, 'metadata' | 'scopeParentId' | 'turnId' | 'representativeMessageId'> & {
  scopeParentId?: string | null; turnId?: string | null; representativeMessageId?: string | null;
  metadata: Pick<TimelineItem['metadata'], 'createdAt' | 'conclusionSegmentIds' | 'origin' | 'role'> & Partial<TimelineItem['metadata']>;
};
export type ChatTimelineWireEvent = Omit<ChatTimelineEvent, 'updates'> & { updates: Array<Omit<TimelineUpdate, 'operations' | 'windowPatches'> & {
  windowPatches: Array<Omit<TimelineWindowPatch, 'items'> & { items?: TimelineWireItem[] }>;
  operations: Array<Omit<TimelineOperation, 'item'> & { item?: TimelineWireItem }>;
}> };
