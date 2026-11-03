import { afterEach, describe, expect, it, vi } from 'vitest';
import { createStore } from 'zustand/vanilla';
import { apidto } from '@wailsjs/go/models';

const mocks = vi.hoisted(() => ({
  open: vi.fn(), close: vi.fn().mockResolvedValue(undefined), eventsOn: vi.fn(), unsubscribe: vi.fn(),
}));
const auth = createStore<{ isAuthenticated: boolean; user: { userId: string; sessionId: string } | null }>(() => ({
  isAuthenticated: true, user: { userId: 'user', sessionId: 'session' },
}));
vi.mock('../store/authStore', () => ({ useAuthStore: auth }));
vi.mock('@wailsjs/go/wailsapi/ChatTimeline', () => ({
  Open: mocks.open, Close: mocks.close, Refresh: vi.fn(), Navigate: vi.fn(), SetRetention: vi.fn(),
}));
vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: mocks.eventsOn }));

afterEach(() => {
  auth.setState({ isAuthenticated: false, user: null });
  vi.clearAllMocks();
});

describe('chatTimelineWails', () => {
  it('normaliza nullables de DTO sem fabricar identidade ou conteúdo', async () => {
    const { normalizeTimelineSnapshot } = await import('./chatTimelineWails');
    const value = normalizeTimelineSnapshot(new apidto.SubscribedTimelineSnapshot({
      windowId: 'window', items: [{ itemId: 'item' }],
    }));
    expect(value.scopeParentId).toBeNull();
    expect(value.items[0]).toMatchObject({ itemId: 'item', scopeParentId: null, turnId: null,
      representativeMessageId: null, segments: [], messageIds: [] });
    expect(value.orderedItemIds).toEqual([]);
    expect(mocks.eventsOn).not.toHaveBeenCalled();
  });

  it('reutiliza cliente da sessão e descarta Open tardio depois de trocar sessão', async () => {
    auth.setState({ isAuthenticated: true, user: { userId: 'user', sessionId: 'first' } });
    mocks.eventsOn.mockReturnValue(mocks.unsubscribe);
    let resolve!: (value: apidto.SubscribedTimelineSnapshot) => void;
    mocks.open.mockReturnValue(new Promise<apidto.SubscribedTimelineSnapshot>(yes => { resolve = yes; }));
    const { getChatTimelineClient } = await import('./chatTimelineWails');
    const client = getChatTimelineClient();
    expect(getChatTimelineClient()).toBe(client);
    const opening = client.open('chat', 'surface', null, { cursor: '', anchor: 'tail', anchorItemId: '', direction: '', limit: 20 });
    await Promise.resolve();
    expect(mocks.open.mock.calls[0][0]).toBeInstanceOf(apidto.OpenTimelineRequest);
    auth.setState({ user: { userId: 'user', sessionId: 'second' } });
    expect(mocks.unsubscribe).toHaveBeenCalledOnce();
    expect(getChatTimelineClient()).not.toBe(client);
    resolve(new apidto.SubscribedTimelineSnapshot({ windowId: 'late' }));
    await opening;
    expect(mocks.close).toHaveBeenCalledExactlyOnceWith('late');
    expect(client.getWindow('chat', 'surface')).toBeUndefined();
  });
});
