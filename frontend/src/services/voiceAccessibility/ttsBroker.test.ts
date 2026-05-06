import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  registerAnnouncerSink,
  registerVoiceAccessibilityActiveResolver,
  unregisterAnnouncerSink,
} from './announcerBroker';
import { requestVoiceTTS, resetVoiceTTSBrokerForTests } from './ttsBroker';

describe('ttsBroker', () => {
  const sink = vi.fn();
  let unregisterResolver: (() => void) | undefined;

  beforeEach(() => {
    sink.mockReset();
    unregisterAnnouncerSink();
    unregisterResolver?.();
    unregisterResolver = undefined;
    registerAnnouncerSink(sink);
    resetVoiceTTSBrokerForTests();
  });

  it('executa TTS ativo com exclusividade de interrupção', async () => {
    const stopCurrent = vi.fn();
    const speak = vi.fn().mockResolvedValue(true);

    await expect(requestVoiceTTS({
      text: 'Resposta',
      origin: { tabId: 'tab-1' },
      priority: 'automatic-active',
      stopCurrent,
      speak,
    })).resolves.toBe(true);

    expect(stopCurrent).toHaveBeenCalled();
    expect(speak).toHaveBeenCalled();
    expect(sink).not.toHaveBeenCalled();
  });

  it('não interrompe quando interrupt=false', async () => {
    const stopCurrent = vi.fn();

    await requestVoiceTTS({
      text: 'Resposta',
      interrupt: false,
      stopCurrent,
      speak: vi.fn().mockResolvedValue(true),
    });

    expect(stopCurrent).not.toHaveBeenCalled();
  });

  it('converte TTS automático de aba inativa em anúncio contextual', async () => {
    unregisterResolver = registerVoiceAccessibilityActiveResolver(() => false);
    const speak = vi.fn();

    await expect(requestVoiceTTS({
      text: 'Resposta pronta',
      origin: { tabId: 'tab-2', title: 'Chat B' },
      priority: 'automatic-inactive',
      speak,
    })).resolves.toBe(false);

    expect(speak).not.toHaveBeenCalled();
    expect(sink).toHaveBeenCalledWith('Chat B: Resposta pronta', 'polite');
  });
});
