import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  commandShortcutFromKeyboardEvent,
  createCommandModifierState,
} from './commandShortcut';
import {
  createLocalCommandKeyboard,
  type LocalCommandKeyRequest,
  type LocalCommandKeyboardBinding,
} from './commandLocalKeyboard';

const headingBinding: LocalCommandKeyboardBinding = {
  shortcut: { version: 1, code: 'Digit1', modifiers: ['Control', 'Alt'] },
  commandId: 'editor.format.heading.h1',
  handler: 'ui' as const,
};

const keyboards: Array<{ dispose: () => void }> = [];

afterEach(() => {
  while (keyboards.length) keyboards.pop()?.dispose();
});

function keyboardEvent(
  type: 'keydown' | 'keyup',
  code: string,
  init: KeyboardEventInit = {},
): KeyboardEvent {
  return new KeyboardEvent(type, {
    code,
    key: code,
    bubbles: true,
    cancelable: true,
    ...init,
  });
}

function dispatch(type: 'keydown' | 'keyup', code: string, init: KeyboardEventInit = {}) {
  const event = keyboardEvent(type, code, init);
  window.dispatchEvent(event);
  return event;
}

async function createKeyboard() {
  const onDown = vi.fn(async (_request: LocalCommandKeyRequest) => {});
  const onUp = vi.fn(async (_request: LocalCommandKeyRequest) => {});
  const reset = vi.fn(async () => {});
  const keyboard = createLocalCommandKeyboard({
    target: window,
    loadMap: async () => ({ generation: 'g-control-alt', bindings: [headingBinding] }),
    onDown,
    onUp,
    reset,
    blocked: () => false,
  });
  keyboards.push(keyboard);
  await keyboard.refresh();
  return { keyboard, onDown, onUp, reset };
}

function observeControlAltInOrder(order: 'control-first' | 'alt-first') {
  if (order === 'control-first') {
    dispatch('keydown', 'ControlLeft', { ctrlKey: true });
    dispatch('keydown', 'AltLeft', { ctrlKey: true, altKey: true });
  } else {
    dispatch('keydown', 'AltLeft', { altKey: true });
    dispatch('keydown', 'ControlRight', { ctrlKey: true, altKey: true });
  }
}

describe('Ctrl+Alt físico em commandLocalKeyboard', () => {
  it('rejeita Ctrl+Alt sem observação e aceita após ControlLeft e AltLeft observados', async () => {
    const { onDown, onUp } = await createKeyboard();

    const unobserved = dispatch('keydown', 'Digit1', { ctrlKey: true, altKey: true });
    expect(unobserved.defaultPrevented).toBe(false);
    expect(onDown).not.toHaveBeenCalled();

    observeControlAltInOrder('control-first');
    const accepted = dispatch('keydown', 'Digit1', { ctrlKey: true, altKey: true });

    expect(accepted.defaultPrevented).toBe(true);
    expect(onDown).toHaveBeenCalledWith(expect.objectContaining({
      generation: 'g-control-alt',
      shortcut: { version: 1, code: 'Digit1', modifiers: ['Control', 'Alt'] },
      commandId: 'editor.format.heading.h1',
      handler: 'ui',
      kind: 'down',
      repeat: false,
    }));

    dispatch('keyup', 'Digit1', { ctrlKey: true, altKey: true });
    await vi.waitFor(() => expect(onUp).toHaveBeenCalledWith(expect.objectContaining({
      commandId: 'editor.format.heading.h1',
      kind: 'up',
      repeat: false,
    })));
  });

  it('aceita a ordem inversa AltLeft e ControlRight', async () => {
    const { onDown } = await createKeyboard();

    observeControlAltInOrder('alt-first');
    const accepted = dispatch('keydown', 'Digit1', { ctrlKey: true, altKey: true });

    expect(accepted.defaultPrevented).toBe(true);
    expect(onDown).toHaveBeenCalledOnce();
    expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ commandId: 'editor.format.heading.h1', handler: 'ui' }));
  });

  it('libera a observação em keyup e não recria estado com repeat dos modificadores', async () => {
    const first = await createKeyboard();

    dispatch('keydown', 'ControlLeft', { ctrlKey: true, repeat: true });
    dispatch('keydown', 'AltLeft', { ctrlKey: true, altKey: true, repeat: true });
    expect(dispatch('keydown', 'Digit1', { ctrlKey: true, altKey: true }).defaultPrevented).toBe(false);
    expect(first.onDown).not.toHaveBeenCalled();

    observeControlAltInOrder('control-first');
    dispatch('keyup', 'AltLeft', { ctrlKey: true, altKey: false });
    expect(dispatch('keydown', 'Digit1', { ctrlKey: true, altKey: true }).defaultPrevented).toBe(false);
    expect(first.onDown).not.toHaveBeenCalled();

    dispatch('keydown', 'AltLeft', { ctrlKey: true, altKey: true });
    expect(dispatch('keydown', 'Digit1', { ctrlKey: true, altKey: true }).defaultPrevented).toBe(true);
    expect(first.onDown).toHaveBeenCalledOnce();
  });

  it('descarta AltRight e AltGraph como estados ambíguos', async () => {
    const altRight = await createKeyboard();
    dispatch('keydown', 'ControlLeft', { ctrlKey: true });
    dispatch('keydown', 'AltRight', { ctrlKey: true, altKey: true });
    expect(dispatch('keydown', 'Digit1', { ctrlKey: true, altKey: true }).defaultPrevented).toBe(false);
    expect(altRight.onDown).not.toHaveBeenCalled();

    const altGraph = await createKeyboard();
    const event = keyboardEvent('keydown', 'Digit1', { ctrlKey: true, altKey: true });
    Object.defineProperty(event, 'getModifierState', { value: (name: string) => name === 'AltGraph' });
    window.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(false);
    expect(altGraph.onDown).not.toHaveBeenCalled();
  });

  it('limpa observação em blur e refresh do mapa', async () => {
    const c = await createKeyboard();
    observeControlAltInOrder('control-first');
    window.dispatchEvent(new FocusEvent('blur'));
    await vi.waitFor(() => expect(c.reset).toHaveBeenCalledWith('g-control-alt'));
    expect(dispatch('keydown', 'Digit1', { ctrlKey: true, altKey: true }).defaultPrevented).toBe(false);
    expect(c.onDown).not.toHaveBeenCalled();

    observeControlAltInOrder('control-first');
    await c.keyboard.refresh();
    expect(dispatch('keydown', 'Digit1', { ctrlKey: true, altKey: true }).defaultPrevented).toBe(false);
    expect(c.onDown).not.toHaveBeenCalled();
  });

  it('mantém Ctrl+Alt desconhecido bloqueado no conversor até receber admissão explícita', () => {
    const event = keyboardEvent('keydown', 'Digit1', { ctrlKey: true, altKey: true });
    const captureEvent = {
      code: event.code,
      repeat: event.repeat,
      isComposing: event.isComposing,
      keyCode: event.keyCode,
      ctrlKey: event.ctrlKey,
      altKey: event.altKey,
      shiftKey: event.shiftKey,
      metaKey: event.metaKey,
      getModifierState: event.getModifierState.bind(event),
    };

    expect(commandShortcutFromKeyboardEvent(captureEvent)).toBeNull();
    expect(commandShortcutFromKeyboardEvent(captureEvent, true)).toEqual({
      version: 1,
      code: 'Digit1',
      modifiers: ['Control', 'Alt'],
    });
  });

  it('expõe estado observável, libera em keyup e limpa explicitamente', () => {
    const state = createCommandModifierState();
    const observe = (type: 'keydown' | 'keyup', code: string, init: KeyboardEventInit = {}) => {
      const event = keyboardEvent(type, code, init);
      state.observe(event);
    };

    observe('keydown', 'ControlLeft', { ctrlKey: true });
    observe('keydown', 'AltLeft', { ctrlKey: true, altKey: true });
    expect(state.allowsControlAlt()).toBe(true);
    observe('keyup', 'AltLeft', { ctrlKey: true, altKey: false });
    expect(state.allowsControlAlt()).toBe(false);
    state.clear();
    expect(state.allowsControlAlt()).toBe(false);
  });

  it('não prende AltRight quando o keyup ainda informa AltGraph', () => {
    const state = createCommandModifierState();
    state.observe(keyboardEvent('keydown', 'ControlLeft', { ctrlKey: true }));
    state.observe(keyboardEvent('keydown', 'AltLeft', { ctrlKey: true, altKey: true }));
    const down = keyboardEvent('keydown', 'AltRight', { ctrlKey: true, altKey: true });
    Object.defineProperty(down, 'getModifierState', { value: (name: string) => name === 'AltGraph' });
    state.observe(down);
    expect(state.allowsControlAlt()).toBe(false);
    const up = keyboardEvent('keyup', 'AltRight', { ctrlKey: true, altKey: true });
    Object.defineProperty(up, 'getModifierState', { value: (name: string) => name === 'AltGraph' });
    state.observe(up);
    expect(state.allowsControlAlt()).toBe(true);
    expect(commandShortcutFromKeyboardEvent(up, state.allowsControlAlt())).toBeNull();
  });
});
