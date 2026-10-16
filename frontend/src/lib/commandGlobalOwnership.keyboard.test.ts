import { describe, expect, it, vi } from 'vitest';
import { connectGlobalCommandOwnership } from './commandGlobalOwnershipWails';
import { createLocalCommandKeyboard, type LocalCommandKeyboardController } from './commandLocalKeyboard';

const instanceId = '019955aa-eeee-7000-8000-000000000001';
const frame = (revision: number, reserved: boolean) => ({
  version: 1, platform: 'windows', instanceId, revision,
  combinations: reserved ? [{ key: 75, modifiers: 2 }] : [],
});
const key = (type: string, repeat = false) => new KeyboardEvent(type, {
  code: 'KeyK', key: 'k', keyCode: 75, ctrlKey: true, repeat, cancelable: true,
});

describe('native ownership and local keyboard integration', () => {
  it('mantém o comando local executável enquanto renew carrega após evento de ownership', async () => {
    let listener!: (frame: unknown) => void;
    let keyboard!: LocalCommandKeyboardController;
    let renewal = Promise.resolve();
    let resolveRenewal!: (value: { generation: string; bindings: { commandId: string; handler: 'local_ui'; shortcut: { version: 1; code: string; modifiers: string[] } }[] }) => void;
    const pendingMap = new Promise<{ generation: string; bindings: { commandId: string; handler: 'local_ui'; shortcut: { version: 1; code: string; modifiers: string[] } }[] }>(resolve => { resolveRenewal = resolve; });
    const loadMap = vi.fn()
      .mockResolvedValueOnce({ generation: 'local', bindings: [{
        commandId: 'navigation.palette.open', handler: 'local_ui' as const,
        shortcut: { version: 1 as const, code: 'KeyK', modifiers: ['Control'] },
      }] })
      .mockReturnValueOnce(pendingMap);
    const onDown = vi.fn();
    const ownership = connectGlobalCommandOwnership({
      subscribe: next => { listener = next; return () => {}; },
      readSnapshot: async () => frame(0, false), acknowledge: async () => true,
      onChange: () => { renewal = keyboard.renew(); },
    });
    keyboard = createLocalCommandKeyboard({
      target: window, ownedGlobally: ownership.owns, blocked: () => !ownership.isReady(), loadMap,
      onDown, onUp: vi.fn(), reset: vi.fn(),
    });
    try {
      await ownership.ready;
      await keyboard.refresh();
      listener(frame(1, false));
      await vi.waitFor(() => expect(loadMap).toHaveBeenCalledTimes(2));
      const localPress = key('keydown');
      window.dispatchEvent(localPress);
      expect(localPress.defaultPrevented).toBe(true);
      expect(onDown).toHaveBeenCalledOnce();
      expect(onDown).toHaveBeenCalledWith(expect.objectContaining({ generation: 'local', commandId: 'navigation.palette.open' }));
      resolveRenewalMap();
      await renewal;
    } finally { keyboard.dispose(); ownership.dispose(); }

    function resolveRenewalMap() {
      resolveRenewal({ generation: 'local-renewed', bindings: [{
        commandId: 'navigation.palette.open', handler: 'local_ui',
        shortcut: { version: 1, code: 'KeyK', modifiers: ['Control'] },
      }] });
    }
  });

  it('installs exclusion before ACK, suppresses down/repeat/up and restores only fresh local presses', async () => {
    let listener!: (frame: unknown) => void;
    let keyboard!: LocalCommandKeyboardController;
    let refresh = Promise.resolve();
    const onDown = vi.fn();
    const onUp = vi.fn();
    const reset = vi.fn();
    const ack = vi.fn(async (_instance: string, revision: number) => {
      if (revision === 1) {
        const physical = key('keydown');
        window.dispatchEvent(physical);
        expect(physical.defaultPrevented).toBe(true);
        expect(onDown).not.toHaveBeenCalled();
      }
      return true;
    });
    const ownership = connectGlobalCommandOwnership({
      subscribe: (next) => { listener = next; return () => {}; },
      readSnapshot: async () => frame(0, false), acknowledge: ack,
      onChange: () => { refresh = keyboard.renew(); },
    });
    keyboard = createLocalCommandKeyboard({
      target: window, ownedGlobally: ownership.owns,
      blocked: () => !ownership.isReady(), canRepeat: () => true,
      loadMap: async () => ({ generation: 'local', bindings: [{
        commandId: 'navigation.palette.open', handler: 'local_ui',
        shortcut: { version: 1, code: 'KeyK', modifiers: ['Control'] },
      }] }), onDown, onUp, reset,
    });
    try {
      await ownership.ready;
      await keyboard.refresh();
      listener(frame(1, true));
      await refresh;
      const resetCount = reset.mock.calls.length;
      for (const event of [key('keydown'), key('keydown', true), key('keyup')]) {
        window.dispatchEvent(event);
        expect(event.defaultPrevented).toBe(true);
      }
      expect(onDown).not.toHaveBeenCalled();
      expect(onUp).not.toHaveBeenCalled();
      expect(reset).toHaveBeenCalledTimes(resetCount); // No per-key backend reset.
      listener(frame(2, false));
      await refresh;
      expect(reset).not.toHaveBeenCalled(); // Mesma geração: não revogar o mapa válido.
      window.dispatchEvent(key('keydown', true));
      expect(onDown).not.toHaveBeenCalled();
      window.dispatchEvent(key('keyup'));
      window.dispatchEvent(key('keydown'));
      expect(onDown).toHaveBeenCalledOnce();
      window.dispatchEvent(key('keyup'));
      expect(onUp).toHaveBeenCalledOnce();
    } finally { keyboard.dispose(); ownership.dispose(); }
  });
});
