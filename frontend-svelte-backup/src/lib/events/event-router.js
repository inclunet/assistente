/**
 * EventRouter - assina eventos do Wails com escopo por conversa/aba
 *
 * Preferência: eventos escopados no formato `${base}:${conversationId}`.
 * Mantém compatibilidade com eventos legados `${base}` quando necessário.
 */

let _EventsOn = null;
let _EventsOff = null;
let _loaded = false;

async function ensureRuntime() {
  if (_loaded) return;
  try {
    const runtime = await import('../../../wailsjs/runtime/runtime.js');
    _EventsOn = runtime.EventsOn;
    _EventsOff = runtime.EventsOff;
  } catch (e) {
    console.warn('[EventRouter] Runtime Wails não disponível:', e);
  }
  _loaded = true;
}

export class EventRouter {
  constructor({ conversationId = null } = {}) {
    this.conversationId = conversationId;
    this._unsubs = [];
    this._ready = ensureRuntime();
  }

  setFilter({ conversationId = undefined } = {}) {
    if (typeof conversationId !== 'undefined') this.conversationId = conversationId;
  }

  /**
   * Assina um evento (preferindo o canal escopado) e retorna unsubscribe
   * @param {string} base - ex.: 'chat:stream', 'chat:done'
   * @param {(payload:any)=>void} handler
   * @param {Object} [opts]
   * @param {boolean} [opts.fallbackGlobal=true] - assina também o global se necessário
   */
  async on(base, handler, opts = {}) {
    const { fallbackGlobal = false } = opts;
    await this._ready;
    if (!_EventsOn) return () => {};

    const unsubs = [];
    // Escopo por conversa se disponível
    if (this.conversationId) {
      const scoped = `${base}:${this.conversationId}`;
      const off = _EventsOn(scoped, handler);
      unsubs.push(off ?? (() => _EventsOff && _EventsOff(scoped, handler)));
    } else if (fallbackGlobal) {
      const off = _EventsOn(base, handler);
      unsubs.push(off ?? (() => _EventsOff && _EventsOff(base, handler)));
    }

    // Retém para cleanup em massa
    const unsubscribe = () => unsubs.forEach(fn => {
      try { fn && fn(); } catch {}
    });
    this._unsubs.push(unsubscribe);
    return unsubscribe;
  }

  offAll() {
    this._unsubs.forEach(fn => {
      try { fn && fn(); } catch {}
    });
    this._unsubs = [];
  }

  dispose() { this.offAll(); }
}
