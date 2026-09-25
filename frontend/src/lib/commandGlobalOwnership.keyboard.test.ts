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
      onChange: () => { refresh = keyboard.refresh(); },
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
