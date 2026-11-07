import type { TimelineItem, TimelineWireItem } from '../types/chatTimelineEvents';

const timelineMetadataDefaults = {
  source: '',
  childCount: 0,
  cacheReadTokens: 0,
  cacheWriteTokens: 0,
  cacheMissTokens: 0,
  pinned: false,
  model: '',
  promptTokens: 0,
  completionTokens: 0,
  totalTokens: 0,
};

export function normalizeTimelineItem(item: TimelineWireItem): TimelineItem {
  const metadata = item.metadata;
  return { ...item, state: item.state ?? '', durability: item.durability ?? 'persisted', metadata: { ...metadata,
    source: metadata?.source ?? timelineMetadataDefaults.source, childCount: metadata?.childCount ?? timelineMetadataDefaults.childCount,
    cacheReadTokens: metadata?.cacheReadTokens ?? timelineMetadataDefaults.cacheReadTokens, cacheWriteTokens: metadata?.cacheWriteTokens ?? timelineMetadataDefaults.cacheWriteTokens,
    cacheMissTokens: metadata?.cacheMissTokens ?? timelineMetadataDefaults.cacheMissTokens, pinned: metadata?.pinned ?? timelineMetadataDefaults.pinned,
    model: metadata?.model ?? timelineMetadataDefaults.model, promptTokens: metadata?.promptTokens ?? timelineMetadataDefaults.promptTokens,
    completionTokens: metadata?.completionTokens ?? timelineMetadataDefaults.completionTokens, totalTokens: metadata?.totalTokens ?? timelineMetadataDefaults.totalTokens,
  }, scopeParentId: item.scopeParentId ?? null, turnId: item.turnId ?? null,
    representativeMessageId: item.representativeMessageId ?? null,
    segments: item.segments ?? [] };
}

/** O limite de recuperação pertence ao wire; defaults locais não são bytes recebidos. */
export function timelineItemWireBytes(item: TimelineItem): number {
  const { scopeParentId, turnId, state, durability, ...required } = item;
  const metadata = Object.fromEntries(Object.entries(item.metadata).filter(([key, value]) =>
    !Object.prototype.hasOwnProperty.call(timelineMetadataDefaults, key) || value !== timelineMetadataDefaults[key as keyof typeof timelineMetadataDefaults]));
  return new TextEncoder().encode(JSON.stringify({ ...required,
    ...(scopeParentId == null ? {} : { scopeParentId }), ...(turnId == null ? {} : { turnId }),
    ...(state ? { state } : {}), ...(durability === 'persisted' ? {} : { durability }), metadata })).length;
}
