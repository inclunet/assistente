import { afterEach, describe, expect, it, vi } from 'vitest';
import { createLocalCommandKeyboard, type LocalCommandKeyboardMap } from './commandLocalKeyboard';

describe('prazo da camada no mapa local', () => {
  afterEach(() => { vi.useRealTimers(); });
  const map = (validUntil?: number): LocalCommandKeyboardMap => ({ generation: 'g', validUntil,
    bindings: [{ shortcut: { version: 1, code: 'KeyK', modifiers: ['Control'] }, commandId: 'chat.focus.input', handler: 'local_ui' }],
  });
  const key = () => window.dispatchEvent(new KeyboardEvent('keydown', { code: 'KeyK', key: 'k', ctrlKey: true }));
  function setup(loadMap: () => Promise<LocalCommandKeyboardMap>) {
    const onDown = vi.fn(async () => {});
    const onMapInvalidated = vi.fn();
    const controller = createLocalCommandKeyboard({ target: window, loadMap, onDown, onUp: async () => {}, reset: async () => {}, blocked: () => false, onMapInvalidated });
    return { controller, onDown, onMapInvalidated };
  }
  it('retira o mapa no prazo e permanece fechado se a recarga falhar', async () => {
    vi.useFakeTimers();
    const load = vi.fn().mockResolvedValueOnce(map(Date.now() + 100)).mockRejectedValue(new Error('offline'));
    const { controller, onDown, onMapInvalidated } = setup(load);
    try {
      await controller.refresh(); key(); expect(onDown).toHaveBeenCalledOnce();
      await vi.advanceTimersByTimeAsync(100);
      key(); expect(onDown).toHaveBeenCalledOnce();
      expect(load).toHaveBeenCalledTimes(2);
      expect(onMapInvalidated).toHaveBeenCalledTimes(2);
    } finally { controller.dispose(); }
  });
  it('nega execução se o relógio passou do prazo antes de o timer rodar', async () => {
    vi.useFakeTimers();
    const deadline = Date.now() + 100;
    const { controller, onDown } = setup(async () => map(deadline));
    try { await controller.refresh(); vi.setSystemTime(deadline); key(); expect(onDown).not.toHaveBeenCalled(); }
    finally { controller.dispose(); }
  });
  it.each([0, -1, NaN, Infinity, 1.5])('recusa deadline inválido %s', async validUntil => {
    const { controller, onDown } = setup(async () => map(validUntil));
    try { await controller.refresh(); key(); expect(onDown).not.toHaveBeenCalled(); }
    finally { controller.dispose(); }
  });
  it('cancelamento do timer antigo não afeta o mapa novo sem prazo', async () => {
    vi.useFakeTimers();
    const load = vi.fn().mockResolvedValueOnce(map(Date.now() + 100)).mockResolvedValue(map());
    const { controller, onDown } = setup(load);
    try { await controller.refresh(); await controller.refresh(); await vi.advanceTimersByTimeAsync(100); key(); expect(onDown).toHaveBeenCalledOnce(); expect(load).toHaveBeenCalledTimes(2); }
    finally { controller.dispose(); }
  });
});
