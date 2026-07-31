/** @vitest-environment jsdom */
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { dispatchChatSpeech, handleChatSpeak } from './index';

const dispatchSpeechMock = vi.fn();
const announceWithOriginMock = vi.fn();
const stopCurrentAudioMock = vi.fn();
const speakMessageMock = vi.fn();
const stopTTSMock = vi.fn();
const getVolumeMock = vi.fn().mockReturnValue(0.7);
const speakWithOverrideMock = vi.fn();

vi.mock('@wailsjs/go/app/App', () => ({
  DispatchSpeech: (...args: unknown[]) => dispatchSpeechMock(...args),
}));

vi.mock('../voiceAccessibility/announcerBroker', () => ({
  announceWithOrigin: (...args: unknown[]) => announceWithOriginMock(...args),
  isVoiceAccessibilityOriginCurrentlyActive: () => true,
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
    announceWithOriginMock.mockReset();
    stopCurrentAudioMock.mockReset();
    speakMessageMock.mockReset();
    stopTTSMock.mockReset();
    getVolumeMock.mockClear();
    speakWithOverrideMock.mockReset();
  });

  it('dispatcha fala efêmera para o backend', async () => {
    dispatchSpeechMock.mockResolvedValue(undefined);

    await dispatchChatSpeech({
      conversationId: "01926b90-7a5a-7c4e-8d3f-000000000042",
      role: 'user',
      text: 'Olá mundo',
      origin: 'user_message',
    });

    expect(dispatchSpeechMock).toHaveBeenCalledWith({
      conversationId: "01926b90-7a5a-7c4e-8d3f-000000000042",
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

    expect(announceWithOriginMock).toHaveBeenCalledWith({
      message: 'chat.system: Processando',
      origin: undefined,
      eventType: 'system',
    });
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
    expect(announceWithOriginMock).not.toHaveBeenCalled();
    expect(speakWithOverrideMock).toHaveBeenCalledWith('Resposta', {
      providerId: 'webspeech',
      voiceName: 'pt-BR',
      ttsModel: 'ignored',
      rate: 1.2,
      pitch: 0.9,
      volume: 0.4,
    });
  });

  it('backend_audio usa SpeakMessage com o contexto do evento', async () => {
    speakMessageMock.mockResolvedValue(true);

    await handleChatSpeak({
      messageId: "01926b90-7a5a-7c4e-8d3f-000000000099",
      role: 'assistant',
      text: 'Resposta backend',
      strategy: 'backend_audio',
      autoRead: true,
      providerId: 'openai-default',
      voiceId: 'nova',
      model: 'tts-1',
      rate: 1.3,
      volume: 0.5,
      speechLanguage: 'pt-BR',
    });

    expect(announceWithOriginMock).not.toHaveBeenCalled();
    expect(speakMessageMock).toHaveBeenCalledWith("01926b90-7a5a-7c4e-8d3f-000000000099", 0.5, {
      providerId: 'openai-default',
      voiceId: 'nova',
      model: 'tts-1',
      rate: 1.3,
      language: 'pt-BR',
    });
  });

  it('backend_audio faz fallback para announcer quando a reprodução falha', async () => {
    speakMessageMock.mockResolvedValue(false);

    await handleChatSpeak({
      messageId: "01926b90-7a5a-7c4e-8d3f-000000000100",
      role: 'assistant',
      text: 'Fallback',
      strategy: 'backend_audio',
      fallbackStrategy: 'announce',
      autoRead: true,
    });

    expect(announceWithOriginMock).toHaveBeenCalledWith({
      message: 'chat.assistant: Fallback',
      origin: undefined,
      eventType: 'system',
    });
  });

  it('backend_audio sem messageId faz fallback para segmentos', async () => {
    await handleChatSpeak({
      role: 'assistant',
      text: 'Segmento parcial',
      strategy: 'backend_audio',
      fallbackStrategy: 'announce',
      autoRead: true,
      origin: 'segment',
      interrupt: false,
    });

    expect(speakMessageMock).not.toHaveBeenCalled();
    // Segmento não interrompe: o áudio em curso segue até o fim.
    expect(stopCurrentAudioMock).not.toHaveBeenCalled();
    expect(stopTTSMock).not.toHaveBeenCalled();
    expect(announceWithOriginMock).toHaveBeenCalledWith({
      message: 'chat.assistant: Segmento parcial',
      origin: undefined,
      eventType: 'progress',
    });
  });

  it('backend_audio sem messageId fala avisos do sistema pelo fallback', async () => {
    await handleChatSpeak({
      role: 'system',
      text: 'Limite de iterações do agente atingido.',
      strategy: 'backend_audio',
      fallbackStrategy: 'announce',
      autoRead: true,
      origin: 'system_message',
    });

    expect(speakMessageMock).not.toHaveBeenCalled();
    // O aviso interrompe: o áudio do segmento anterior não pode continuar
    // tocando por cima do anúncio.
    expect(stopCurrentAudioMock).toHaveBeenCalled();
    expect(stopTTSMock).toHaveBeenCalled();
    expect(announceWithOriginMock).toHaveBeenCalledWith({
      message: 'chat.system: Limite de iterações do agente atingido.',
      origin: undefined,
      eventType: 'system',
    });
  });

  it('backend_audio sem messageId ignora origins não verbalizáveis', async () => {
    await handleChatSpeak({
      role: 'assistant',
      text: 'Pensando...',
      strategy: 'backend_audio',
      fallbackStrategy: 'announce',
      autoRead: true,
      origin: 'thinking',
    });

    expect(speakMessageMock).not.toHaveBeenCalled();
    expect(announceWithOriginMock).not.toHaveBeenCalled();
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
