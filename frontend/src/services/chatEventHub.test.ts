import { afterEach, describe, expect, it, vi } from 'vitest';
import { createChatTurnEventRouter, resetChatEventHubForTests } from './chatEventHub';

const { listeners } = vi.hoisted(() => ({ listeners: new Map<string, (event: unknown) => void>() }));
vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (name: string, callback: (event: unknown) => void) => {
    listeners.set(name, callback);
    return () => listeners.delete(name);
  },
}));

afterEach(resetChatEventHubForTests);

describe('identidade de execução do chat', () => {
  it('ignora terminal antigo antes de messages_ready da nova execução', () => {
    let turn: string | null = null;
    const done = vi.fn();
    const ready = vi.fn();
    const route = createChatTurnEventRouter('c', () => turn, value => { turn = value; }, 'new');
    route.on('chat:done', done);
    route.on('chat:messages_ready', ready);
    listeners.get('chat:done')?.({ conversationId: 'c', turnId: 'old-turn', surfaceOrigin: { executionId: 'old' } });
    listeners.get('chat:error')?.({ conversationId: 'c', error: 'unscoped' });
    expect(turn).toBeNull();
    expect(done).not.toHaveBeenCalled();
    listeners.get('chat:messages_ready')?.({ conversationId: 'c', turnId: 'new-turn', surfaceOrigin: { executionId: 'new' } });
    expect(turn).toBe('new-turn');
    expect(ready).toHaveBeenCalledOnce();
  });

  it('distingue retries do mesmo turnId e mantém eventos da tentativa atual', () => {
    const stream = vi.fn();
    const done = vi.fn();
    const route = createChatTurnEventRouter('c', () => 'user-1', () => {}, 'retry-2');
    route.on('chat:stream', stream);
    route.on('chat:done', done);
    for (const name of ['chat:stream', 'chat:done']) {
      listeners.get(name)?.({ conversationId: 'c', turnId: 'user-1', surfaceOrigin: { executionId: 'retry-1' } });
      listeners.get(name)?.({ conversationId: 'c', turnId: 'user-1', surfaceOrigin: { executionId: 'retry-2' } });
    }
    expect(stream).toHaveBeenCalledOnce();
    expect(done).toHaveBeenCalledOnce();
  });

  it('mantém rotas de canais sem execução e fala sem origem', () => {
    const speak = vi.fn();
    const external = vi.fn();
    createChatTurnEventRouter('c', () => null, () => {}, 'request').on('chat:speak', speak);
    createChatTurnEventRouter('external', () => null, () => {}).on('chat:done', external);
    listeners.get('chat:speak')?.({ conversationId: 'c', text: 'resposta' });
    listeners.get('chat:done')?.({ conversationId: 'external', turnId: 'u' });
    expect(speak).toHaveBeenCalledOnce();
    expect(external).toHaveBeenCalledOnce();
  });
});
