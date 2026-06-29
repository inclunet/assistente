import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  announceWithOrigin,
  registerAnnouncerSink,
  registerVoiceAccessibilityActiveResolver,
  unregisterAnnouncerSink,
} from './announcerBroker';

describe('announcerBroker', () => {
  const sink = vi.fn();
  let unregisterResolver: (() => void) | undefined;

  beforeEach(() => {
    sink.mockReset();
    unregisterAnnouncerSink();
    unregisterResolver?.();
    unregisterResolver = undefined;
    registerAnnouncerSink(sink);
  });

  it('anuncia eventos sem origem como ativos', () => {
    expect(announceWithOrigin({ message: 'Mensagem' })).toBe(true);

    expect(sink).toHaveBeenCalledWith('Mensagem', 'polite');
  });

  it('bloqueia progresso de origem inativa', () => {
    unregisterResolver = registerVoiceAccessibilityActiveResolver(() => false);

    expect(announceWithOrigin({
      message: 'Streaming...',
      eventType: 'progress',
      origin: { tabId: 'tab-2', title: 'Chat B' },
    })).toBe(false);

    expect(sink).not.toHaveBeenCalled();
  });

  it('permite conclusão de origem inativa com contexto', () => {
    unregisterResolver = registerVoiceAccessibilityActiveResolver(() => false);

    expect(announceWithOrigin({
      message: 'terminou de responder',
      eventType: 'completion',
      origin: { tabId: 'tab-2', title: 'Chat B' },
    })).toBe(true);

    expect(sink).toHaveBeenCalledWith('Chat B: terminou de responder', 'polite');
  });

  it('usa assertive por padrão para erros', () => {
    expect(announceWithOrigin({
      message: 'Falha',
      eventType: 'error',
    })).toBe(true);

    expect(sink).toHaveBeenCalledWith('Falha', 'assertive');
  });

  it('deduplica anúncios idênticos consecutivos', () => {
    const request = {
      message: 'Texto parcial',
      eventType: 'progress' as const,
      origin: { tabId: 'tab-1', title: 'Chat A' },
      deduplicate: true,
    };

    expect(announceWithOrigin(request)).toBe(true);
    expect(announceWithOrigin(request)).toBe(false);

    expect(sink).toHaveBeenCalledTimes(1);
    expect(sink).toHaveBeenCalledWith('Texto parcial', 'polite');
  });

  it('reanuncia mensagens idênticas quando deduplicação não é solicitada', () => {
    const request = {
      message: '1 turno na fila',
      eventType: 'progress' as const,
      origin: { tabId: 'tab-1', title: 'Chat A' },
    };

    expect(announceWithOrigin(request)).toBe(true);
    expect(announceWithOrigin(request)).toBe(true);

    expect(sink).toHaveBeenCalledTimes(2);
  });
});
