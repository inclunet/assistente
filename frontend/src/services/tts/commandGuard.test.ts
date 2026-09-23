import { describe, expect, it, vi } from 'vitest';
const provider = vi.hoisted(() => ({ stop: vi.fn(), setRate: vi.fn(async () => {}), setVolume: vi.fn(async () => {}), setVoice: vi.fn(async () => {}), speak: vi.fn(async () => {}) }));
vi.mock('./factory', () => ({ ttsFactory: { initialize: vi.fn(async () => {}), getProviderWithFallback: () => provider } }));
import { ttsService } from './index';
describe('TTS guarded fallback', () => {
  it('guard falso recusa antes de tocar', async () => {
    await expect(ttsService.speakWithOverride('capturado', { providerId: 'webspeech', isCurrent: () => false })).rejects.toThrow('stale');
    expect(provider.speak).not.toHaveBeenCalled();
  });
  it('revalida após aguardar configuração da voz', async () => {
    let current = true;
    provider.setVoice.mockImplementationOnce(async () => { current = false; });
    await expect(ttsService.speakWithOverride('capturado', { providerId: 'webspeech', voiceName: 'voice', isCurrent: () => current })).rejects.toThrow('stale');
    expect(provider.speak).not.toHaveBeenCalled();
  });
  it('consumidor sem guard mantém reprodução', async () => {
    await ttsService.speakWithOverride('legado', { providerId: 'webspeech' });
    expect(provider.speak).toHaveBeenCalledExactlyOnceWith('legado');
  });
});
