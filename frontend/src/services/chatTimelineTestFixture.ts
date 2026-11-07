import { vi } from 'vitest';
import { ChatTimelineClient, type TimelineTransport } from './chatTimelineClient';
import { ChatTimelinePresentation } from './chatTimelinePresentation';
import type { TimelineSnapshot } from './timelineConversationState';
import type { ChatTimelineEvent, TimelineItem } from '../types/chatTimelineEvents';

type FixtureItem = TimelineItem & { conversationId?: string };

export function canonicalTestItem(id: string, content = id, overrides: Partial<FixtureItem> = {}): FixtureItem {
  return { itemId: id, conversationId: 'conversation', scopeParentId: null, turnId: 'turn',
    executionId: 'execution', representativeMessageId: 'assistant',
    revision: '1', state: 'completed', durability: 'persisted',
    segments: [{ segmentId: 'text', kind: 'text', text: content, conclusion: true }],
    metadata: { createdAt: '2026-01-01T00:00:00Z', source: '', childCount: 0,
      cacheReadTokens: 0, cacheWriteTokens: 0,
      cacheMissTokens: 0, role: 'assistant', pinned: false, model: '', promptTokens: 0, completionTokens: 0, totalTokens: 0 },
    ...overrides };
}

export function wireFixtureItem({ conversationId: _context, ...item }: FixtureItem): TimelineItem {
  return item;
}

/** O teste fornece DTOs/lotes explícitos; não interpreta eventos antigos. */
export function canonicalTestFixture(initial: FixtureItem[] = []) {
  let receive!: (event: ChatTimelineEvent) => void;
  let items = initial;
  const windows = new Map<string, TimelineSnapshot>();
  const makeSnapshot = (conversationId: string, surface: string, scope: string | null): TimelineSnapshot => {
    const selected = items.filter(item => (item.conversationId ?? 'conversation') === conversationId && item.scopeParentId === scope).map(wireFixtureItem);
    const value: TimelineSnapshot = { conversationId, scopeParentId: scope, windowId: surface, epoch: 'epoch',
      windowRevision: '1', sequence: '1', mode: 'historical', items: selected, orderedItemIds: selected.map(item => item.itemId),
      retainedItemIds: [], retentionBlocked: false, totalCount: selected.length, startIndex: 0, endIndex: selected.length - 1,
      hasBefore: false, hasAfter: false, beforeCursor: 'before', afterCursor: 'after' };
    windows.set(surface, value);
    return value;
  };
  const port: TimelineTransport = {
    listen: callback => { receive = callback; return () => {}; },
    open: vi.fn(async (conversation, surface, scope) => makeSnapshot(conversation, surface, scope)),
    navigate: vi.fn(async id => windows.get(id)!), refresh: vi.fn(async id => windows.get(id)!),
    retain: vi.fn(async (windowId, epoch, windowRevision) => ({ windowId, epoch, windowRevision })),
    close: vi.fn(async () => {}), reserveItem: vi.fn(), readItem: vi.fn(), releaseItem: vi.fn(),
    readRecovery: vi.fn(), reserveRecovery: vi.fn(), releaseRecovery: vi.fn(),
  };
  const client = new ChatTimelineClient(port, surface => surface);
  return { client, port, windows,
    presentation: (surface = 'surface', conversation = 'conversation') => new ChatTimelinePresentation(client, conversation, surface),
    setItems: (next: FixtureItem[]) => { items = next; },
    emit: (event: ChatTimelineEvent) => receive(event),
    publish: (surface: string, next: TimelineItem) => {
      const old = windows.get(surface)!;
      const sequence = String(Number(old.sequence) + 1);
      const revision = String(Number(old.windowRevision) + 1);
      const item = { ...wireFixtureItem(next), revision: sequence };
      windows.set(surface, { ...old, windowRevision: revision, sequence,
        items: old.items.map(value => value.itemId === item.itemId ? item : value) });
      const event: ChatTimelineEvent = { conversationId: old.conversationId, surfaceId: surface, windowId: surface, epoch: old.epoch,
        snapshotRevision: '1', syncRequired: false,
        updates: [{ conversationId: old.conversationId, epoch: old.epoch, sequence,
          operations: [{ kind: 'item_upsert', itemId: item.itemId, revision: sequence, item }],
          windowPatches: [{ windowId: surface, previousRevision: old.windowRevision, revision }] }] };
      receive(event);
      return event;
    },
  };
}
