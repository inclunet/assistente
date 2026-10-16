import { afterEach, describe, expect, it, vi } from 'vitest';
import { TTSStreamPlayer } from './streamPlayer';
import { TTS_STREAM_CHUNK, TTS_STREAM_DONE } from '../../lib/speechEvents';
const handlers = vi.hoisted(() => new Map<string, (event: unknown) => void>());
vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: (name: string, callback: (event: unknown) => void) => { handlers.set(name, callback); return () => handlers.delete(name); } }));
afterEach(() => { vi.restoreAllMocks(); handlers.clear(); });
describe('stream player command guard', () => {
  it('descarta chunks após contexto invalidado e encerra listeners', () => {
    const audio = vi.spyOn(globalThis, 'Audio');
    let current = true;
    const onError = vi.fn();
    const player = new TTSStreamPlayer();
    player.startListening('captured', { isCurrent: () => current, onError });
    current = false;
    handlers.get(TTS_STREAM_CHUNK)!({ sessionId: 'captured', chunkBase64: 'QUFB' });
    expect(audio).not.toHaveBeenCalled(); expect(onError).toHaveBeenCalledOnce();
    expect(player.getState()).toBe('idle'); expect(handlers.size).toBe(0);
  });
  it('contexto muda depois dos chunks: done não começa reprodução', () => {
    const audio = vi.spyOn(globalThis, 'Audio');
    let current = true;
    const player = new TTSStreamPlayer();
    player.startListening('captured', { isCurrent: () => current });
    handlers.get(TTS_STREAM_CHUNK)!({ sessionId: 'captured', chunkBase64: 'QUFB' });
    current = false;
    handlers.get(TTS_STREAM_DONE)!({ sessionId: 'captured' });
    expect(audio).not.toHaveBeenCalled(); expect(player.getState()).toBe('idle');
  });
});
