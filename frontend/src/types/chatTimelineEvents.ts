// Espelho manual dos payloads de eventos em internal/apidto (AEP-0116).
// Não importar payload de evento dos bindings gerados pelo Wails.
export const CHAT_TIMELINE_EVENT = 'chat:timeline';

export interface TimelineToolSummary {
  invocationId?: string;
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
  itemId: string;
  conversationId: string;
  scopeParentId: string | null;
  kind: string;
  turnId: string | null;
  replyToItemId: string;
  messageIds: string[];
  representativeMessageId: string | null;
  position: string;
  revision: string;
  state: string;
  durability: string;
  segments: TimelineSegment[];
  metadata: {
    origin: string;
    role: string;
    pinned: boolean;
    model: string;
    promptTokens: number;
    completionTokens: number;
    totalTokens: number;
  };
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

export interface TimelineWindowPatch {
  windowId: string;
  previousRevision: string;
  revision: string;
  splices?: { index: number; deleteCount: number; insertIds: string[] }[];
  items?: TimelineItem[];
  metadata?: {
    retainedItemIds: string[];
    retentionBlocked: boolean;
    totalCount: number;
    startIndex: number;
    endIndex: number;
    hasBefore: boolean;
    hasAfter: boolean;
    beforeCursor: string;
    afterCursor: string;
  };
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
  conversationId: string;
  windowId: string;
  epoch: string;
  snapshotRevision: string;
  syncRequired: boolean;
  reason?: string;
  updates: TimelineUpdate[];
}
