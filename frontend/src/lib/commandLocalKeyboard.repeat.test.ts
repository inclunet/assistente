import { afterEach, describe, expect, it, vi } from 'vitest';
import { createLocalCommandKeyboard, type LocalCommandKeyboardMap, type LocalCommandKeyRequest } from './commandLocalKeyboard';
import type { CommandShortcut } from './commandShortcut';

const controlK: CommandShortcut = { version: 1, code: 'KeyK', modifiers: ['Control'] };
const navMap = (generation = 'g-nav'): LocalCommandKeyboardMap => ({
  generation,
  bindings: [{ shortcut: controlK, commandId: 'workspace.tab.next', handler: 'local_ui' }],
});
const keyEvent = (type: 'keydown' | 'keyup', init: KeyboardEventInit = {}) =>
  new KeyboardEvent(type, {
    code: 'KeyK',
    key: 'k',
    ctrlKey: true,
    bubbles: true,
    cancelable: true,
    ...init,
  });

const controllers: Array<{ dispose: () => void }> = [];

afterEach(() => {
  controllers.splice(0).forEach((controller) => controller.dispose());
});

function makeController(
  map: LocalCommandKeyboardMap,
  overrides: Partial<Parameters<typeof createLocalCommandKeyboard>[0]> = {},
) {
  const onDown = vi.fn(async (_request: LocalCommandKeyRequest) => {});
  const onUp = vi.fn(async (_request: LocalCommandKeyRequest) => {});
  const reset = vi.fn(async () => {});
  const keyboard = createLocalCommandKeyboard({
    target: window,
    loadMap: vi.fn(async () => map),
    onDown,
    onUp,
    reset,
    blocked: () => false,
    ...overrides,
  });
  controllers.push(keyboard);
  return { keyboard, onDown, onUp, reset };
}

describe('commandLocalKeyboard repeat', () => {
  it('permite repeat de navegação na mesma combinação e geração', async () => {
    const c = makeController(navMap(), { canRepeat: (commandId) => commandId === 'workspace.tab.next' });
    await c.keyboard.refresh();

    window.dispatchEvent(keyEvent('keydown'));
    window.dispatchEvent(keyEvent('keydown', { repeat: true }));

    expect(c.onDown).toHaveBeenCalledTimes(2);
    expect(c.onDown.mock.calls[0]?.[0]).toMatchObject({
      generation: 'g-nav',
      commandId: 'workspace.tab.next',
      repeat: false,
    });
    expect(c.onDown.mock.calls[1]?.[0]).toMatchObject({
      generation: 'g-nav',
      commandId: 'workspace.tab.next',
      repeat: true,
    });
  });

  it('recusa repeat antes do down, depois do up, blur e refresh', async () => {
    const c = makeController(navMap(), { canRepeat: (commandId) => commandId === 'workspace.tab.next' });
    await c.keyboard.refresh();

    window.dispatchEvent(keyEvent('keydown', { repeat: true }));
    expect(c.onDown).not.toHaveBeenCalled();

    window.dispatchEvent(keyEvent('keydown'));
    window.dispatchEvent(keyEvent('keyup'));
    window.dispatchEvent(keyEvent('keydown', { repeat: true }));
    expect(c.onDown).toHaveBeenCalledTimes(1);

    window.dispatchEvent(keyEvent('keydown'));
    window.dispatchEvent(new FocusEvent('blur'));
    window.dispatchEvent(keyEvent('keydown', { repeat: true }));
    expect(c.onDown).toHaveBeenCalledTimes(2);

    window.dispatchEvent(keyEvent('keydown'));
    await c.keyboard.refresh();
    window.dispatchEvent(keyEvent('keydown', { repeat: true }));
    expect(c.onDown).toHaveBeenCalledTimes(2);
  });

  it('recusa repeat que muda modifiers mantendo o mesmo code', async () => {
    const c = makeController({
      generation: 'g-modifiers',
      bindings: [
        { shortcut: controlK, commandId: 'workspace.tab.next', handler: 'local_ui' },
        { shortcut: { version: 1, code: 'KeyK', modifiers: ['Control', 'Shift'] }, commandId: 'workspace.tab.previous', handler: 'local_ui' },
      ],
    }, { canRepeat: (commandId) => commandId === 'workspace.tab.next' });
    await c.keyboard.refresh();

    window.dispatchEvent(keyEvent('keydown'));
    window.dispatchEvent(keyEvent('keydown', { shiftKey: true, repeat: true }));

    expect(c.onDown).toHaveBeenCalledTimes(1);
    expect(c.onDown).toHaveBeenCalledWith(expect.objectContaining({ commandId: 'workspace.tab.next', repeat: false }));
  });

  it.each([
    ['workspace.tab.chat.create', 'contextual'],
    ['workspace.tab.chat.close', 'contextual'],
  ] as const)('não repete comando de criar/fechar: %s', async (commandId, handler) => {
    const c = makeController({
      generation: 'g-contextual',
      bindings: [{ shortcut: controlK, commandId, handler }],
    }, { canRepeat: (candidate) => candidate === 'workspace.tab.next' });
    await c.keyboard.refresh();

    window.dispatchEvent(keyEvent('keydown'));
    window.dispatchEvent(keyEvent('keydown', { repeat: true }));

    expect(c.onDown).toHaveBeenCalledTimes(1);
    expect(c.onDown.mock.calls[0]?.[0]).toMatchObject({ commandId, repeat: false });
  });

  it('rejeita known local ID com handler ui/contextual sem callback', async () => {
    const onMapInvalidated = vi.fn();
    const c = makeController({
      generation: 'g-invalid-local-handler',
      bindings: [{ shortcut: controlK, commandId: 'workspace.tab.next', handler: 'ui' }],
    }, { onMapInvalidated });
    await c.keyboard.refresh();

    window.dispatchEvent(keyEvent('keydown'));
    expect(onMapInvalidated).toHaveBeenCalled();
    expect(c.onDown).not.toHaveBeenCalled();
  });

  it('descarta callbacks tardios após dispose e refresh', async () => {
    let resolveMap!: (map: LocalCommandKeyboardMap) => void;
    const onDown = vi.fn(async () => {});
    const onMapAccepted = vi.fn();
    const keyboard = createLocalCommandKeyboard({
      target: window,
      loadMap: vi.fn(() => new Promise<LocalCommandKeyboardMap>((resolve) => { resolveMap = resolve; })),
      onDown,
      onUp: vi.fn(async () => {}),
      reset: vi.fn(async () => {}),
      blocked: () => false,
      onMapAccepted,
    });
    controllers.push(keyboard);

    const refresh = keyboard.refresh();
    await vi.waitFor(() => expect(resolveMap).toBeTypeOf('function'));
    keyboard.dispose();
    resolveMap(navMap('g-stale'));
    await refresh;

    window.dispatchEvent(keyEvent('keydown'));
    expect(onMapAccepted).not.toHaveBeenCalled();
    expect(onDown).not.toHaveBeenCalled();
  });
});
