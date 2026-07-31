import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  announceWithOrigin,
  estimateAnnouncementReadingMs,
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

  describe('proteção da leitura do conteúdo', () => {
    const content = 'chat.assistant: A resposta completa que a pessoa está esperando ouvir.';
    const MAX_ADVANCE_MS = 120_000;

    beforeEach(() => {
      vi.useFakeTimers();
    });

    afterEach(() => {
      vi.useRealTimers();
    });

    it('adia aviso secundário e o fala depois da leitura terminar', () => {
      announceWithOrigin({ message: content, eventType: 'completion', protectsReading: true });
      sink.mockClear();

      expect(announceWithOrigin({
        message: 'Mensagens 1 a 20 de 34 carregadas',
        eventType: 'progress',
      })).toBe(true);
      expect(sink).not.toHaveBeenCalled();

      vi.advanceTimersByTime(estimateAnnouncementReadingMs(content));

      expect(sink).toHaveBeenCalledWith('Mensagens 1 a 20 de 34 carregadas', 'polite');
    });

    it('guarda apenas o aviso mais recente entre os adiados', () => {
      announceWithOrigin({ message: content, eventType: 'completion', protectsReading: true });
      sink.mockClear();

      announceWithOrigin({ message: 'Carregando mensagens', eventType: 'progress' });
      announceWithOrigin({ message: 'Mensagens 1 a 20 de 34 carregadas', eventType: 'progress' });

      vi.advanceTimersByTime(estimateAnnouncementReadingMs(content));

      expect(sink).toHaveBeenCalledTimes(1);
      expect(sink).toHaveBeenCalledWith('Mensagens 1 a 20 de 34 carregadas', 'polite');
    });

    it('não adia erro nem resposta a uma ação da pessoa', () => {
      announceWithOrigin({ message: content, eventType: 'completion', protectsReading: true });
      sink.mockClear();

      announceWithOrigin({ message: 'Falha ao enviar', eventType: 'error' });
      announceWithOrigin({ message: 'Mensagens 1 a 20 de 34 carregadas', eventType: 'user-action' });

      expect(sink).toHaveBeenNthCalledWith(1, 'Falha ao enviar', 'assertive');
      expect(sink).toHaveBeenNthCalledWith(2, 'Mensagens 1 a 20 de 34 carregadas', 'polite');
    });

    it('desiste do aviso que esperou tempo demais', () => {
      const respostaLonga = `chat.assistant: ${'palavra '.repeat(200)}`;
      announceWithOrigin({ message: respostaLonga, eventType: 'completion', protectsReading: true });
      announceWithOrigin({ message: 'Mensagens carregadas', eventType: 'progress' });
      sink.mockClear();

      vi.advanceTimersByTime(estimateAnnouncementReadingMs(respostaLonga));

      expect(sink).not.toHaveBeenCalled();
    });

    it('descarta o adiado quando mais conteúdo chega', () => {
      announceWithOrigin({ message: content, eventType: 'progress', protectsReading: true });
      announceWithOrigin({ message: 'Mensagens carregadas', eventType: 'progress' });
      announceWithOrigin({
        message: 'chat.assistant: Segundo trecho da resposta.',
        eventType: 'completion',
        protectsReading: true,
      });
      sink.mockClear();

      vi.advanceTimersByTime(MAX_ADVANCE_MS);

      expect(sink).not.toHaveBeenCalled();
    });

    it('deixa o conteúdo seguinte passar e reinicia a proteção', () => {
      announceWithOrigin({ message: content, eventType: 'progress', protectsReading: true });
      const segundo = 'chat.assistant: Segundo trecho da resposta.';
      announceWithOrigin({ message: segundo, eventType: 'completion', protectsReading: true });
      sink.mockClear();

      announceWithOrigin({ message: 'Mensagens carregadas', eventType: 'progress' });
      vi.advanceTimersByTime(estimateAnnouncementReadingMs(segundo) - 1);
      expect(sink).not.toHaveBeenCalled();

      vi.advanceTimersByTime(1);
      expect(sink).toHaveBeenCalledWith('Mensagens carregadas', 'polite');
    });

    it('descarta o adiado quando outro anúncio passa na frente', () => {
      announceWithOrigin({ message: content, eventType: 'completion', protectsReading: true });
      announceWithOrigin({ message: 'Mensagens 1 a 20 de 34 carregadas', eventType: 'progress' });
      // Navegação explícita muda a janela: o aviso automático de antes já não
      // descreve o estado atual.
      announceWithOrigin({ message: 'Mensagens 25 a 34 de 34 carregadas', eventType: 'user-action' });
      sink.mockClear();

      vi.advanceTimersByTime(MAX_ADVANCE_MS);

      expect(sink).not.toHaveBeenCalled();
    });

    it('não perde conclusão de aba inativa quando a conversa anda', () => {
      announceWithOrigin({ message: content, eventType: 'completion', protectsReading: true });
      unregisterResolver = registerVoiceAccessibilityActiveResolver(() => false);
      announceWithOrigin({
        message: 'terminou de responder',
        eventType: 'completion',
        origin: { tabId: 'tab-2', title: 'Chat B' },
      });
      unregisterResolver();
      unregisterResolver = undefined;

      const seguinte = 'chat.assistant: Continuando a resposta.';
      announceWithOrigin({ message: seguinte, eventType: 'completion', protectsReading: true });
      sink.mockClear();

      vi.advanceTimersByTime(estimateAnnouncementReadingMs(seguinte));

      expect(sink).toHaveBeenCalledWith('Chat B: terminou de responder', 'polite');
    });

    it('fala um adiado por vez para um não substituir o outro', () => {
      announceWithOrigin({ message: content, eventType: 'completion', protectsReading: true });
      unregisterResolver = registerVoiceAccessibilityActiveResolver(() => false);
      announceWithOrigin({
        message: 'terminou de responder',
        eventType: 'completion',
        origin: { tabId: 'tab-2', title: 'Chat B' },
      });
      announceWithOrigin({
        message: 'terminou de responder',
        eventType: 'completion',
        origin: { tabId: 'tab-3', title: 'Chat C' },
      });
      unregisterResolver();
      unregisterResolver = undefined;
      sink.mockClear();

      vi.advanceTimersByTime(estimateAnnouncementReadingMs(content));
      expect(sink).toHaveBeenCalledTimes(1);
      expect(sink).toHaveBeenCalledWith('Chat B: terminou de responder', 'polite');

      vi.advanceTimersByTime(estimateAnnouncementReadingMs('Chat B: terminou de responder'));
      expect(sink).toHaveBeenCalledWith('Chat C: terminou de responder', 'polite');
    });

    it('para de proteger quando o announcer é desmontado', () => {
      announceWithOrigin({ message: content, eventType: 'completion', protectsReading: true });
      unregisterAnnouncerSink();
      registerAnnouncerSink(sink);
      sink.mockClear();

      announceWithOrigin({ message: 'Mensagens carregadas', eventType: 'progress' });

      expect(sink).toHaveBeenCalledWith('Mensagens carregadas', 'polite');
    });
  });

  it('não deduplica mensagens idênticas quando o chamador solicita novo anúncio', () => {
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
