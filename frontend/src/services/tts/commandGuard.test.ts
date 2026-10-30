import { describe, expect, it, vi } from 'vitest';
const provider = vi.hoisted(() => ({ isSpeaking: () => false, stop: vi.fn(), setRate: vi.fn(async () => {}), setVolume: vi.fn(async () => {}), setVoice: vi.fn(async () => {}), speak: vi.fn(async () => {}) }));
vi.mock('./factory', () => ({ ttsFactory: { initialize: vi.fn(async () => {}), getProvider: (id: string) => id === 'openai' ? null : provider } }));
const stream = vi.hoisted(() => ({ startListening: vi.fn(), stop: vi.fn() }));
vi.mock('./streamPlayer', () => ({ getStreamPlayer: () => stream }));
vi.mock('@wailsjs/go/wailsapi/Speech', () => ({ SpeakPreview: vi.fn(async () => {
  stream.startListening.mock.lastCall?.[1].onEnd();
}) }));
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


it('reproduz prévia HTTP sem transporte registrado e preserva IDs exatos', async () => {
 await ttsService.speakWithOverride('teste', { providerId: 'proxy', ttsModel: 'vendor/model', voiceName: 'tenant:voice-id' });
 expect(stream.startListening).toHaveBeenCalledWith(expect.any(String), expect.any(Object));
 expect(SpeakPreview).toHaveBeenCalledWith('proxy', 'vendor/model', 'tenant:voice-id', 1, 1, '', 'teste', expect.any(String));
});

it('encerra o listener quando a síntese HTTP falha', async () => {
 vi.mocked(SpeakPreview).mockRejectedValueOnce(new Error('falha HTTP'));
 await expect(ttsService.speakWithOverride('teste', { providerId: 'proxy', ttsModel: 'manual' })).rejects.toThrow('falha HTTP');
 expect(stream.stop).toHaveBeenCalled();
});

it.each(['resolve', 'reject'])('resposta tardia da primeira prévia não interrompe a segunda: %s', async outcome => {
 let finishOld!: () => void;
 vi.mocked(SpeakPreview).mockImplementationOnce(() => new Promise<void>((resolve, reject) => {
   finishOld = () => outcome === 'resolve' ? resolve() : reject(new Error('resposta antiga'));
 }));
 const old = ttsService.speakWithOverride('primeira', { providerId: 'proxy', ttsModel: 'manual' });
 vi.mocked(SpeakPreview).mockImplementationOnce(async () => {});
 const current = ttsService.speakWithOverride('segunda', { providerId: 'proxy', ttsModel: 'manual' });
 const stops = stream.stop.mock.calls.length;
 finishOld();
 await old;
 expect(stream.stop).toHaveBeenCalledTimes(stops);
 expect(ttsService.isSpeaking()).toBe(true);
 stream.startListening.mock.lastCall?.[1].onEnd();
 await current;
});

it.each(['resolve', 'reject'])('propaga erro do player antes da resposta HTTP: %s', async outcome => {
 let finishRPC!: () => void;
 vi.mocked(SpeakPreview).mockImplementationOnce(() => new Promise<void>((resolve, reject) => {
   finishRPC = () => outcome === 'resolve' ? resolve() : reject(new Error('falha HTTP tardia'));
 }));
 const preview = ttsService.speakWithOverride('teste', { providerId: 'proxy', ttsModel: 'manual' });
 stream.startListening.mock.lastCall?.[1].onError(new Error('erro de áudio'));
 await expect(preview).rejects.toThrow('erro de áudio');
 expect(ttsService.isSpeaking()).toBe(false);
 finishRPC();
});
