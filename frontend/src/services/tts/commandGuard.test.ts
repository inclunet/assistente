import { describe, expect, it, vi } from 'vitest';
const provider = vi.hoisted(() => ({ stop: vi.fn(), setRate: vi.fn(async () => {}), setVolume: vi.fn(async () => {}), setVoice: vi.fn(async () => {}), speak: vi.fn(async () => {}) }));
vi.mock('./factory', () => ({ ttsFactory: { initialize: vi.fn(async () => {}), getProvider: () => provider } }));
vi.mock('@wailsjs/go/wailsapi/Speech', () => ({ SpeakPreview: vi.fn(async () => {}) }));
import { SpeakPreview } from '@wailsjs/go/wailsapi/Speech';
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


it('preserva o ID exato da voz com dois-pontos e hífen', async () => {
  await ttsService.speakWithOverride('teste', { providerId: 'webspeech', voiceName: 'tenant:voice-id' });
  expect(provider.setVoice).toHaveBeenCalledWith('tenant:voice-id');
});


it('envia modelo e voz HTTP intactos ao binding oficial', async () => {
 await ttsService.speakWithOverride('teste', { providerId: 'proxy', ttsModel: 'vendor/model', voiceName: 'tenant:voice-id' });
 expect(SpeakPreview).toHaveBeenCalledWith('proxy', 'vendor/model', 'tenant:voice-id', 1, 1, '', 'teste', expect.any(String));
});
