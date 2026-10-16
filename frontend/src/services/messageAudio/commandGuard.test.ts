import { afterEach, describe, expect, it, vi } from 'vitest';
import { messageAudioService } from './index';
const backend = vi.hoisted(() => vi.fn());
vi.mock('@wailsjs/go/wailsapi/Speech', () => ({ SpeakMessage: backend }));
afterEach(() => { vi.restoreAllMocks(); backend.mockReset(); messageAudioService.clearMemoryCache(); });
describe('message audio command guard', () => {
  it('rejeição play antiga não interrompe player substituto', async () => {
    backend.mockResolvedValue({ audio: 'QUFB', mimeType: 'audio/mpeg' });
    const oldAudio = document.createElement('audio'); const newAudio = document.createElement('audio');
    let rejectOld!: (error: Error) => void;
    vi.spyOn(oldAudio, 'play').mockImplementation(() => new Promise<void>((_resolve, reject) => { rejectOld = reject; }));
    vi.spyOn(oldAudio, 'pause').mockImplementation(() => {});
    vi.spyOn(newAudio, 'play').mockResolvedValue(undefined);
    const pauseNew = vi.spyOn(newAudio, 'pause').mockImplementation(() => {});
    vi.spyOn(globalThis, 'Audio').mockImplementationOnce(function () { return oldAudio; }).mockImplementationOnce(function () { return newAudio; });
    Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn(() => 'blob:test') });
    Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() });
    const provider = { providerId: 'openai', model: 'tts', voiceId: 'v', rate: 1 };
    const oldRun = messageAudioService.speakMessage('old', 1, provider, () => true);
    for (let i = 0; i < 10 && !rejectOld; i++) await Promise.resolve();
    const newRun = messageAudioService.speakMessage('new', 1, provider, () => true);
    for (let i = 0; i < 10 && !vi.mocked(newAudio.play).mock.calls.length; i++) await Promise.resolve();
    rejectOld(new Error('late old failure'));
    await oldRun;
    expect(pauseNew).not.toHaveBeenCalled();
    newAudio.onended?.call(newAudio, new Event('ended'));
    expect(await newRun).toBe(true);
  });
  it('notifica início real antes de terminar áudio', async () => {
    backend.mockResolvedValue({ audio: 'QUFB', mimeType: 'audio/mpeg' });
    const audio = document.createElement('audio');
    vi.spyOn(audio, 'play').mockResolvedValue(undefined);
    vi.spyOn(audio, 'pause').mockImplementation(() => {});
    vi.spyOn(globalThis, 'Audio').mockImplementation(function () { return audio; });
    Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn(() => 'blob:test') });
    Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() });
    const started = vi.fn(); let ended = false;
    const pending = messageAudioService.speakMessage('message', 1, { providerId: 'openai', model: 'tts', voiceId: 'v', rate: 1 }, () => true, started).then(result => { ended = true; return result; });
    for (let i = 0; i < 10 && !started.mock.calls.length; i++) await Promise.resolve();
    expect(started).toHaveBeenCalledOnce(); expect(ended).toBe(false);
    audio.onended?.call(audio, new Event('ended'));
    expect(await pending).toBe(true);
  });
  it('recusa antes de IPC', async () => {
    expect(await messageAudioService.speakMessage('message', 1, { providerId: 'openai', model: 'tts', voiceId: 'v', rate: 1 }, () => false)).toBe(false);
    expect(backend).not.toHaveBeenCalled();
  });
  it('resposta atrasada não cria áudio nem fallback de reprodução', async () => {
    let resolve!: (value: { audio: string; mimeType: string }) => void;
    backend.mockImplementation(() => new Promise(done => { resolve = done; }));
    const audio = vi.spyOn(globalThis, 'Audio');
    let current = true;
    const pending = messageAudioService.speakMessage('message', 1, { providerId: 'openai', model: 'tts', voiceId: 'v', rate: 1 }, () => current);
    current = false;
    resolve({ audio: 'QUFB', mimeType: 'audio/mpeg' });
    expect(await pending).toBe(false);
    expect(audio).not.toHaveBeenCalled();
  });
});
