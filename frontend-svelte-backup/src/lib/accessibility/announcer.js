// Utilitário para centralizar anúncios de acessibilidade e TTS
// Fornece uma API simples baseada em callbacks para integração com a UI

/**
 * Cria um announcer com callbacks de saída.
 * @param {Object} cfg
 * @param {(message: string, priority?: 'polite'|'assertive') => void} cfg.onLive - Atualiza região aria-live de mensagens.
 * @param {(message: string) => void} cfg.onNavigation - Atualiza região aria-live de navegação (assertive).
 * @param {(name: 'send'|'receive'|'error') => void} [cfg.onSound] - Dispara sons de feedback.
 * @param {Object} [cfg.ttsService] - Serviço TTS com método `speak(text)`.
 * @param {boolean} [cfg.autoSpeak] - Se verdadeiro, fala respostas automaticamente.
 * @param {boolean} [cfg.isTTSDisabled] - Se verdadeiro, não fala e usa aria-live.
 * @returns {Object} announcer API
 */
export function createAnnouncer({ onLive, onNavigation, onSound, ttsService, autoSpeak = false, isTTSDisabled = true }) {
  function announceNavigation(message) {
    try { onNavigation?.(message); } catch {}
  }

  function announceUserMessage(text) {
    if (isTTSDisabled) {
      try { onLive?.('Você: ' + text, 'polite'); } catch {}
    }
    onSound?.('send');
  }

  function speakOrAnnounceAssistant(content) {
    const preview = (content || '').trim();
    if (isTTSDisabled) {
      try { onLive?.('Assistente: ' + preview, 'polite'); } catch {}
      onSound?.('receive');
      return;
    }
    if (autoSpeak && preview) {
      try { ttsService?.speak(preview); } catch {}
      // Som de "receive" será tocado ao fim do TTS pela UI, se necessário.
    } else {
      onSound?.('receive');
    }
  }

  function announceToolsMessage(message) {
    if (isTTSDisabled) {
      try { onLive?.(message, 'polite'); } catch {}
    }
    onSound?.('send');
  }

  function announceToolResults(count) {
    if (count > 0) {
      if (isTTSDisabled) {
        try { onLive?.(`${count} ferramenta(s) executada(s) com sucesso.`, 'polite'); } catch {}
      }
      onSound?.('receive');
    }
  }

  function announceAgentEvent(agentName, role, content, toolCalls) {
    const name = (agentName || '').split('_').map(w => w.charAt(0).toUpperCase() + w.slice(1)).join(' ');
    const preview = (content || '').substring(0, 100);
    if (role === 'assistant' && toolCalls) {
      announceNavigation(`${name} chamando ferramenta`);
    } else if (role === 'tool') {
      announceNavigation(`Resposta de ferramenta: ${preview}`);
    } else {
      const roleLabel = role === 'tool' ? 'Resultado' : name;
      announceNavigation(`${roleLabel}: ${preview}`);
    }
  }

  return {
    announceNavigation,
    announceUserMessage,
    speakOrAnnounceAssistant,
    announceToolsMessage,
    announceToolResults,
    announceAgentEvent,
  };
}
