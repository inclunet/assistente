/** @vitest-environment jsdom */
import { describe, it, expect, vi, beforeEach } from 'vitest';

import { messageAudioService } from './index';
import { base64ToBlob } from '../../lib/audioUtils';

const speakMessageMock = vi.fn();
const backendProvider = {
  providerId: 'openai',
  voiceId: 'nova',
  model: 'tts-1',
  rate: 1,
};

vi.mock('@wailsjs/go/wailsapi/Speech', () => ({
  SpeakMessage: (...args: unknown[]) => speakMessageMock(...args),
}));

class MockAudio {
  paused = false;
  volume = 1;
  onended: (() => void) | null = null;
  onerror: (() => void) | null = null;

  play = vi.fn(async () => {
    this.onended?.();
  });
  pause = vi.fn();

  constructor(public src?: string) {}
}

describe('messageAudioService', () => {
  beforeEach(() => {
    speakMessageMock.mockReset();
    messageAudioService.clearMemoryCache();
    globalThis.URL.createObjectURL = vi.fn(() => 'blob:url');
    globalThis.URL.revokeObjectURL = vi.fn();
    (globalThis as unknown as { Audio?: unknown }).Audio = MockAudio as never;
  });

  describe('speakMessage (backend-driven)', () => {
    it('reproduz audio retornado pelo backend', async () => {
      speakMessageMock.mockResolvedValue({ audio: 'QUFB', mimeType: 'audio/mpeg' });

      const result = await messageAudioService.speakMessage("42", 0.8, backendProvider);

      expect(result).toBe(true);
      expect(speakMessageMock).toHaveBeenCalledWith("42", 'openai', 'tts-1', 'nova', 1, '');
      expect(globalThis.URL.createObjectURL).toHaveBeenCalled();
    });

    it('retorna false quando backend retorna vazio', async () => {
      speakMessageMock.mockResolvedValue({ audio: '', mimeType: '' });

      const result = await messageAudioService.speakMessage("1", 1.0, backendProvider);

      expect(result).toBe(false);
    });

    it('retorna false quando backend retorna null', async () => {
      speakMessageMock.mockResolvedValue(null);

      const result = await messageAudioService.speakMessage("1", 1.0, backendProvider);

      expect(result).toBe(false);
    });

    it('retorna false quando backend lança erro', async () => {
      speakMessageMock.mockRejectedValue(new Error('TTS indisponível'));

      const result = await messageAudioService.speakMessage("1", 1.0, backendProvider);

      expect(result).toBe(false);
    });

    it('loga warn quando backend lança erro', async () => {
      const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});
      speakMessageMock.mockRejectedValue(new Error('TTS indisponível'));

      await messageAudioService.speakMessage("1", 1.0, backendProvider);

      expect(warnSpy).toHaveBeenCalledWith(
        '[messageAudio] speakMessage failed:',
        expect.any(Error),
      );
      warnSpy.mockRestore();
    });

    it('passa provider params ao backend', async () => {
      speakMessageMock.mockResolvedValue({ audio: 'QUFB', mimeType: 'audio/mpeg' });

      await messageAudioService.speakMessage("42", 1.0, {
        providerId: 'openai',
        voiceId: 'nova',
        model: 'tts-1',
        rate: 1.5,
        language: 'es-ES',
      });

      expect(speakMessageMock).toHaveBeenCalledWith("42", 'openai', 'tts-1', 'nova', 1.5, 'es-ES');
    });

    it('não chama backend para providers frontend ou referência', async () => {
      await expect(messageAudioService.speakMessage("1")).resolves.toBe(false);
      await expect(messageAudioService.speakMessage("2", 1.0, { ...backendProvider, providerId: 'webspeech' })).resolves.toBe(false);
      await expect(messageAudioService.speakMessage("3", 1.0, { ...backendProvider, providerId: 'ref_profile' })).resolves.toBe(false);

      expect(speakMessageMock).not.toHaveBeenCalled();
    });
  });

  describe('cache em memória', () => {
    it('segunda chamada usa cache sem chamar backend', async () => {
      speakMessageMock.mockResolvedValue({ audio: 'QUFB', mimeType: 'audio/mpeg' });

      await messageAudioService.speakMessage("100", 1.0, backendProvider);
      expect(speakMessageMock).toHaveBeenCalledTimes(1);

      await messageAudioService.speakMessage("100", 1.0, backendProvider);
      expect(speakMessageMock).toHaveBeenCalledTimes(1); // NÃO chamou de novo
    });

    it('mensagens diferentes usam caches separados', async () => {
      speakMessageMock.mockResolvedValue({ audio: 'QUFB', mimeType: 'audio/mpeg' });

      await messageAudioService.speakMessage("100", 1.0, backendProvider);
      await messageAudioService.speakMessage("200", 1.0, backendProvider);
      expect(speakMessageMock).toHaveBeenCalledTimes(2);

      // Ambas agora em cache
      await messageAudioService.speakMessage("100", 1.0, backendProvider);
      await messageAudioService.speakMessage("200", 1.0, backendProvider);
      expect(speakMessageMock).toHaveBeenCalledTimes(2);
    });

    it('clearMemoryCache limpa o cache', async () => {
      speakMessageMock.mockResolvedValue({ audio: 'QUFB', mimeType: 'audio/mpeg' });

      await messageAudioService.speakMessage("100", 1.0, backendProvider);
      expect(speakMessageMock).toHaveBeenCalledTimes(1);

      messageAudioService.clearMemoryCache();

      await messageAudioService.speakMessage("100", 1.0, backendProvider);
      expect(speakMessageMock).toHaveBeenCalledTimes(2); // Chamou de novo
    });

    it('getMessageAudioBlob também usa cache', async () => {
      speakMessageMock.mockResolvedValue({ audio: 'QUFB', mimeType: 'audio/mpeg' });

      // Popula cache via speakMessage
      await messageAudioService.speakMessage("100", 1.0, backendProvider);
      expect(speakMessageMock).toHaveBeenCalledTimes(1);

      // getMessageAudioBlob deve usar cache
      const blob = await messageAudioService.getMessageAudioBlob("100", backendProvider);
      expect(blob).toBeInstanceOf(Blob);
      expect(speakMessageMock).toHaveBeenCalledTimes(1);
    });

    it('não armazena resultado falho no cache', async () => {
      speakMessageMock.mockResolvedValue(null);

      await messageAudioService.speakMessage("100", 1.0, backendProvider);
      expect(speakMessageMock).toHaveBeenCalledTimes(1);

      speakMessageMock.mockResolvedValue({ audio: 'QUFB', mimeType: 'audio/mpeg' });
      await messageAudioService.speakMessage("100", 1.0, backendProvider);
      expect(speakMessageMock).toHaveBeenCalledTimes(2); // Tentou de novo
    });
  });

  describe('getMessageAudioBlob', () => {
    it('retorna blob quando backend retorna audio', async () => {
      speakMessageMock.mockResolvedValue({ audio: 'QUFB', mimeType: 'audio/mpeg' });

      const blob = await messageAudioService.getMessageAudioBlob("42", backendProvider);

      expect(blob).toBeInstanceOf(Blob);
      expect(blob!.type).toBe('audio/mpeg');
    });

    it('retorna null quando backend falha', async () => {
      speakMessageMock.mockRejectedValue(new Error('falha'));

      const blob = await messageAudioService.getMessageAudioBlob("1", backendProvider);

      expect(blob).toBeNull();
    });

    it('loga warn quando backend falha', async () => {
      const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});
      speakMessageMock.mockRejectedValue(new Error('falha'));

      await messageAudioService.getMessageAudioBlob("1", backendProvider);

      expect(warnSpy).toHaveBeenCalledWith(
        '[messageAudio] getMessageAudioBlob failed:',
        expect.any(Error),
      );
      warnSpy.mockRestore();
    });

    it('retorna null sem backend para providers frontend ou referência', async () => {
      await expect(messageAudioService.getMessageAudioBlob("1")).resolves.toBeNull();
      await expect(messageAudioService.getMessageAudioBlob("2", { ...backendProvider, providerId: 'webspeech' })).resolves.toBeNull();
      await expect(messageAudioService.getMessageAudioBlob("3", { ...backendProvider, providerId: 'ref_profile' })).resolves.toBeNull();

      expect(speakMessageMock).not.toHaveBeenCalled();
    });
  });

  describe('playAudioBase64', () => {
    it('cria object URL e reproduz', async () => {
      await messageAudioService.playAudioBase64('QUFB', 'audio/mpeg', 0.5);

      expect(globalThis.URL.createObjectURL).toHaveBeenCalled();
    });
  });

  describe('base64ToBlob (audioUtils)', () => {
    it('converte base64 em blob com tipo correto', () => {
      const blob = base64ToBlob('QQ==', 'audio/mpeg');

      expect(blob.type).toBe('audio/mpeg');
    });
  });

  describe('stopCurrentAudio', () => {
    it('pode ser chamado sem audio em reproducao', () => {
      expect(() => messageAudioService.stopCurrentAudio()).not.toThrow();
    });
  });
});
