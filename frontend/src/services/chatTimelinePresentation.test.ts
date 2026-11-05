import { describe, expect, it, vi } from 'vitest';
import type { TimelineItem } from '../types/chatTimelineEvents';
import { ChatTimelineClient, type TimelineTransport } from './chatTimelineClient';
import { ChatTimelinePresentation } from './chatTimelinePresentation';
import type { TimelineSnapshot } from './timelineConversationState';

function item(id: string, scope: string | null = null): TimelineItem {
  return { itemId: id, conversationId: 'conversation', scopeParentId: scope, kind: 'message', turnId: null,
    replyToItemId: '', messageIds: [id], representativeMessageId: id, position: 'opaque', revision: '1', state: 'completed', durability: 'persisted',
    segments: [{ segmentId: 'text', kind: 'text', text: id }],
    metadata: { createdAt: '2026-01-01T00:00:00Z', timestamp: 1767225600000, source: '', childCount: 0,
      threadParentMessageId: id, conclusionSegmentIds: ['text'], cacheReadTokens: 0, cacheWriteTokens: 0, cacheMissTokens: 0,
      origin: 'chat', role: 'user', pinned: false, model: '', promptTokens: 0, completionTokens: 0, totalTokens: 0 } };
}

function fixture() {
  const scopes = new Map<string, string | null>();
  let nextItems = [item('second'), item('first')];
  const snapshot = (surface: string, scope: string | null): TimelineSnapshot => ({
    conversationId: 'conversation', scopeParentId: scope, windowId: surface, epoch: 'epoch', windowRevision: '1', sequence: '1', mode: 'historical',
    orderedItemIds: nextItems.map(value => value.itemId), items: nextItems, retainedItemIds: [], retentionBlocked: false,
    totalCount: nextItems.length, startIndex: 0, endIndex: nextItems.length, hasBefore: false, hasAfter: false, beforeCursor: '', afterCursor: '',
  });
  const transport: TimelineTransport = {
    listen: () => () => {}, open: vi.fn(async (_conversation, surface, scope) => { scopes.set(surface, scope); return snapshot(surface, scope); }),
    navigate: vi.fn(async window => snapshot(window, scopes.get(window) ?? null)), refresh: vi.fn(async window => snapshot(window, null)), retain: vi.fn(async (windowId, epoch, windowRevision) => ({ windowId, epoch, windowRevision })), close: vi.fn(async () => {}),
    readRecovery: vi.fn(), reserveRecovery: vi.fn(), releaseRecovery: vi.fn(), reserveItem: vi.fn(), readItem: vi.fn(), releaseItem: vi.fn(),
  };
  const client = new ChatTimelineClient(transport, surface => surface);
  const presentation = new ChatTimelinePresentation(client, 'conversation', 'surface');
  return { presentation, transport, setItems(items: TimelineItem[]) { nextItems = items; } };
}

describe('canonical presentation', () => {
  it('keeps backend order and preserves message references while only navigation changes', async () => {
    const f = fixture();
    await f.presentation.open();
    const first = f.presentation.nodes();
    expect(first.map(node => node.itemId)).toEqual(['second', 'first']);
    expect(f.presentation.nodes()[0].message).toBe(first[0].message);
    expect(f.presentation.getMessage('second')).toBe(first[0].message);
    await f.presentation.close();
    expect(f.transport.close).toHaveBeenCalledWith('surface');
  });

  it('opens a thread in its own scope and closes both capabilities', async () => {
    const f = fixture();
    await f.presentation.open();
    f.setItems([item('child', 'second')]);
    await f.presentation.openThread('second');
    expect(f.transport.open).toHaveBeenLastCalledWith('conversation', 'surface:thread:second', 'second', expect.any(Object));
    expect(f.presentation.nodes()[0].children?.[0].itemId).toBe('child');
    await f.presentation.close();
    expect(f.transport.close).toHaveBeenCalledTimes(2);
  });

  it('uses the visual identity even when a response has no persisted action target', async () => {
    const f = fixture();
    f.setItems([{ ...item('turn:question'), kind: 'response', state: 'preparing', representativeMessageId: null,
      metadata: { ...item('turn:question').metadata, role: 'assistant', threadParentMessageId: null } }]);
    await f.presentation.open();
    const message = f.presentation.nodes()[0].message;
    expect(message.id).toBe('turn:question');
    expect(message.isStreaming).toBe(true);
    expect(message.timelineItem?.representativeMessageId).toBeNull();
    await f.presentation.close();
  });
});

it('retains a thread item and its ancestor in their respective windows', async () => {
 const f = fixture(); await f.presentation.open();
 f.setItems([item('child', 'second')]); await f.presentation.openThread('second');
 await f.presentation.retain(['child']);
 expect(f.transport.retain).toHaveBeenCalledWith('surface', 'epoch', '1', ['second']);
 expect(f.transport.retain).toHaveBeenCalledWith('surface:thread:second', 'epoch', '1', ['child']);
 await f.presentation.close();
});

it('retries thread Open after its failed capability was cleaned up', async () => {
 const f = fixture(); await f.presentation.open();
 vi.mocked(f.transport.open).mockRejectedValueOnce(new Error('offline'));
 await expect(f.presentation.openThread('second')).rejects.toThrow('offline');
 f.setItems([item('child', 'second')]); await f.presentation.openThread('second');
 expect(f.presentation.nodes()[0].children?.[0].itemId).toBe('child');
 await f.presentation.close();
});

it('closes descendant capabilities even after their parent item leaves the page', async () => {
 const f = fixture(); await f.presentation.open();
 f.setItems([item('child', 'second')]); await f.presentation.openThread('second');
 f.setItems([item('grandchild', 'child')]); await f.presentation.openThread('child');
 f.setItems([item('other', 'second')]); await f.presentation.navigate('end', 'second');
 await f.presentation.closeThread('second');
 expect(f.transport.close).toHaveBeenCalledWith('surface:thread:child');
 expect(f.transport.close).toHaveBeenCalledWith('surface:thread:second');
 await f.presentation.close();
});

it('assembles the whole conclusion using only the references supplied by the backend', async () => {
 const f = fixture(); const response = item('answer');
 response.segments = [{ segmentId: 'intermediate', kind: 'text', text: 'not conclusion' }, { segmentId: 'first', kind: 'text', text: 'hello ' }, { segmentId: 'last', kind: 'text', text: 'world' }];
 response.metadata.conclusionSegmentIds = ['first', 'last'];
 f.setItems([response]); await f.presentation.open();
 expect(f.presentation.getMessage('answer')?.content).toBe('hello world');
 await f.presentation.close();
});

it('recovers a failed initial Open without remounting the surface', async () => {
 const f = fixture(); vi.mocked(f.transport.open).mockRejectedValueOnce(new Error('offline'));
 await expect(f.presentation.open()).rejects.toThrow('offline');
 await f.presentation.refresh();
 expect(f.presentation.nodes().map(node => node.itemId)).toEqual(['second', 'first']);
 expect(f.transport.open).toHaveBeenCalledTimes(2);
 expect(f.transport.navigate).toHaveBeenLastCalledWith('surface', 'epoch', '1', 'followTail', expect.any(Object));
 await f.presentation.close();
});

it('normal Refresh preserves the backend position and mode instead of choosing a new historical window', async () => {
 const f = fixture(); await f.presentation.open();
 await f.presentation.refresh();
 expect(f.transport.refresh).toHaveBeenCalledWith('surface', '1', undefined);
 await f.presentation.close();
});

it.each(['preparing', 'completed'])('renders an empty conclusion received as a JSON array (%s)', async state => {
 const f = fixture(); const response = item('answer');
 response.state = state; response.segments = state === 'preparing' ? [] : [{ segmentId: 'tool', kind: 'tool_invocations', invocations: [] }];
 response.metadata.conclusionSegmentIds = [];
 f.setItems([JSON.parse(JSON.stringify(response))]); await f.presentation.open();
 expect(f.presentation.getMessage('answer')?.content).toBe('');
 await f.presentation.close();
});
