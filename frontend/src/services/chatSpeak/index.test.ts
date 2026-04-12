/** @vitest-environment jsdom */
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { dispatchChatSpeech, handleChatSpeak } from './index';

const dispatchSpeechMock = vi.fn();
const announceMock = vi.fn();
const stopCurrentAudioMock = vi.fn();
const speakMessageMock = vi.fn();
const stopTTSMock = vi.fn();
const getVolumeMock = vi.fn().mockReturnValue(0.7);
const speakWithOverrideMock = vi.fn();

vi.mock('@wailsjs/go/main/App', () => ({
  DispatchSpeech: (...args: unknown[]) => dispatchSpeechMock(...args),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  announce: (...args: unknown[]) => announceMock(...args),
}));

vi.mock('../messageAudio', () => ({
  messageAudioService: {
    stopCurrentAudio: (...args: unknown[]) => stopCurrentAudioMock(...args),
    speakMessage: (...args: unknown[]) => speakMessageMock(...args),
  },
}));

vi.mock('../tts', () => ({
  ttsService: {
    stop: (...args: unknown[]) => stopTTSMock(...args),
    getVolume: (...args: unknown[]) => getVolumeMock(...args),
    speakWithOverride: (...args: unknown[]) => speakWithOverrideMock(...args),
  },
}));

describe('chatSpeak service', () => {
  beforeEach(() => {
    dispatchSpeechMock.mockReset();
    announceMock.mockReset();
    stopCurrentAudioMock.mockReset();
    speakMessageMock.mockReset();
    stopTTSMock.mockReset();
    getVolumeMock.mockClear();
    speakWithOverrideMock.mockReset();
  });

  it('dispatcha fala efêmera para o backend', async () => {
    dispatchSpeechMock.mockResolvedValue(undefined);

    await dispatchChatSpeech({
      conversationId: 42,
      role: 'user',
      text: 'Olá mundo',
      origin: 'user_message',
    });

    expect(dispatchSpeechMock).toHaveBeenCalledWith({
      conversationId: 42,
      role: 'user',
      text: 'Olá mundo',
      origin: 'user_message',
    });
  });

  it('announce usa apenas announcer', async () => {
    await handleChatSpeak({
      role: 'system',
      text: 'Processando',
      strategy: 'announce',
      autoRead: false,
    });

    expect(announceMock).toHaveBeenCalledWith('Sistema: Processando');
    expect(speakWithOverrideMock).not.toHaveBeenCalled();
    expect(speakMessageMock).not.toHaveBeenCalled();
  });

  it('webspeech respeita os parâmetros do evento', async () => {
    await handleChatSpeak({
      role: 'assistant',
      text: 'Resposta',
      strategy: 'webspeech',
      autoRead: true,
      providerId: 'webspeech',
      voiceId: 'pt-BR',
      model: 'ignored',
      rate: 1.2,
      pitch: 0.9,
      volume: 0.4,
    });

    expect(stopCurrentAudioMock).toHaveBeenCalled();
    expect(stopTTSMock).toHaveBeenCalled();
    expect(speakWithOverrideMock).toHaveBeenCalledWith('Resposta', {
      providerId: 'webspeech',
      voiceName: 'pt-BR',
      ttsModel: 'ignored',
      rate: 1.2,
      pitch: 0.9,
      volume: 0.4,
    });
  });

  it('sapi5 respeita os parâmetros do evento', async () => {
    await handleChatSpeak({
      role: 'assistant',
      text: 'Resposta',
      strategy: 'sapi5',
      autoRead: true,
      providerId: 'sapi5',
      voiceId: 'Microsoft Maria',
      rate: 1.1,
      volume: 0.6,
    });

    expect(speakWithOverrideMock).toHaveBeenCalledWith('Resposta', {
      providerId: 'sapi5',
      voiceName: 'Microsoft Maria',
      ttsModel: undefined,
      rate: 1.1,
      pitch: undefined,
      volume: 0.6,
    });
  });

  it('backend_audio usa SpeakMessage com o contexto do evento', async () => {
    speakMessageMock.mockResolvedValue(true);

    await handleChatSpeak({
      messageId: 99,
      role: 'assistant',
      strategy: 'backend_audio',
      autoRead: true,
      providerId: 'openai-default',
      voiceId: 'nova',
      model: 'tts-1',
      rate: 1.3,
      volume: 0.5,
    });

    expect(speakMessageMock).toHaveBeenCalledWith(99, 0.5, {
      providerId: 'openai-default',
      voiceId: 'nova',
      model: 'tts-1',
      rate: 1.3,
    });
  });

  it('backend_audio faz fallback para announcer quando a reprodução falha', async () => {
    speakMessageMock.mockResolvedValue(false);

    await handleChatSpeak({
      messageId: 100,
      role: 'assistant',
      text: 'Fallback',
      strategy: 'backend_audio',
      fallbackStrategy: 'announce',
      autoRead: true,
    });

    expect(announceMock).toHaveBeenCalledWith('Assistente: Fallback');
  });

  it('interrupt=false preserva o áudio atual', async () => {
    await handleChatSpeak({
      role: 'system',
      text: 'Ferramenta concluída',
      strategy: 'announce',
      autoRead: false,
      interrupt: false,
    });

    expect(stopCurrentAudioMock).not.toHaveBeenCalled();
    expect(stopTTSMock).not.toHaveBeenCalled();
  });
});
