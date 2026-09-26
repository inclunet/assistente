import { describe, expect, it, vi } from 'vitest';
import { createLocalCommandKeyboard, type LocalCommandKeyRequest, type LocalCommandKeyboardMap } from './commandLocalKeyboard';
import type { CommandShortcut, CommandShortcutSequence } from './commandShortcut';

const shortcut: CommandShortcut = { version: 1, code: 'KeyK', modifiers: ['Control'] };
const f1Shortcut: CommandShortcut = { version: 1, code: 'F1', modifiers: [] };
const f5Shortcut: CommandShortcut = { version: 1, code: 'F5', modifiers: [] };
const event = (type: 'keydown' | 'keyup', init: KeyboardEventInit = {}) =>
  new KeyboardEvent(type, { code: 'KeyK', key: 'k', ctrlKey: true, cancelable: true, ...init });
const sequence: CommandShortcutSequence = {
  version: 2,
  steps: [{ code: 'KeyK', modifiers: ['Control'] }, { code: 'KeyN', modifiers: [] }],
};
const sequenceEvent = (type: 'keydown' | 'keyup', code: string, init: KeyboardEventInit = {}) =>
  new KeyboardEvent(type, { code, key: code === 'KeyK' ? 'k' : 'n', cancelable: true, ...init });

async function controller(overrides: Partial<Parameters<typeof createLocalCommandKeyboard>[0]> = {}) {
  const onDown = vi.fn(async () => {});
  const onUp = vi.fn(async () => {});
  const reset = vi.fn(async () => {});
  const loadMap = vi.fn(async () => ({ generation: 'g1', bindings: [{ shortcut, commandId: 'workspace.list', handler: 'backend' as const }] }));
  const keyboard = createLocalCommandKeyboard({ target: window, loadMap, onDown, onUp, reset, blocked: () => false, ...overrides });
  await keyboard.refresh();
  return { keyboard, onDown, onUp, reset, loadMap };
}

describe('commandLocalKeyboard', () => {
  it.each([true, false])('checks the selected simple command, not dormant prefix sequences (allowed=%s)', async allowed => {
    const commandId = 'command_settings.create.open';
    const { keyboard, onDown } = await controller({
      loadMap: async () => ({ generation: 'settings-prefix',
        bindings: [{ shortcut: sequence, commandId: 'workspace.tab.chat.create', handler: 'contextual' }],
        contextualBindings: [{ shortcut, bySurface: {}, fallback: null, byPage: {
          settings: { shortcut, bySurface: { toolbar: { shortcut, commandId, handler: 'local_ui' } }, fallback: null },
        } }],
      }),
      readContext: () => ({ surfaceId: 'command-toolbar', surfaceType: 'toolbar', appPage: 'settings',
        allowedCommandIds: [allowed ? commandId : 'profiles.create.open'], isCurrent: () => true }),
    });
    try {
      window.dispatchEvent(event('keydown'));
      if (allowed) expect(onDown).toHaveBeenCalledWith(expect.objectContaining({ commandId }));
      else expect(onDown).not.toHaveBeenCalled();
      window.dispatchEvent(event('keyup'));
      window.dispatchEvent(sequenceEvent('keydown', 'KeyN'));
      expect(onDown).toHaveBeenCalledTimes(allowed ? 1 : 0);
    } finally { keyboard.dispose(); }
  });

  it('preserva argumentos do comando parametrizado no dispatch local e recusa binding sem alvo', async () => {
    const argumentsValue = { workspace_id: 'workspace-a', target_mode: 'position', position: 17 };
    const { keyboard, onDown } = await controller({
      loadMap: async () => ({ generation: 'tab-target', bindings: [{ shortcut, commandId: 'workspace.tab.go_to', handler: 'local_ui', arguments: argumentsValue }] }),
    });
    try {
      window.dispatchEvent(event('keydown'));
      expect(onDown).toHaveBeenCalledWith(expect.objectContaining({ commandId: 'workspace.tab.go_to', arguments: argumentsValue }));
    } finally { keyboard.dispose(); }

    const invalid = await controller({
      loadMap: async () => ({ generation: 'tab-target-missing-args', bindings: [{ shortcut, commandId: 'workspace.tab.go_to', handler: 'local_ui' }] }),
    });
    try {
      window.dispatchEvent(event('keydown'));
      expect(invalid.onDown).not.toHaveBeenCalled();
    } finally { invalid.keyboard.dispose(); }
  });

  it('preserva argumentos do comando parametrizado ao concluir sequência de teclado', async () => {
    const argumentsValue = { workspace_id: 'workspace-sequence', target_mode: 'specific', tab_id: 'tab-target' };
    const onDown = vi.fn(async (_request: LocalCommandKeyRequest) => {});
    const { keyboard } = await controller({
      loadMap: async () => ({ generation: 'tab-target-sequence', bindings: [{ shortcut: sequence, commandId: 'workspace.tab.go_to', handler: 'local_ui', arguments: argumentsValue }] }),
      onDown,
    });
    try {
      const prefix = sequenceEvent('keydown', 'KeyK', { ctrlKey: true });
      window.dispatchEvent(prefix);
      expect(prefix.defaultPrevented).toBe(true);
      expect(onDown).not.toHaveBeenCalled();

      const final = sequenceEvent('keydown', 'KeyN');
      window.dispatchEvent(final);
      expect(final.defaultPrevented).toBe(true);
      expect(onDown).toHaveBeenCalledOnce();
      expect(onDown).toHaveBeenCalledWith(expect.objectContaining({
        commandId: 'workspace.tab.go_to',
        handler: 'local_ui',
        arguments: argumentsValue,
      }));
      expect(onDown.mock.calls[0]?.[0].arguments).not.toBe(argumentsValue);
    } finally { keyboard.dispose(); }
  });

  it.each(['chat.focus.input', 'chat.focus.messages', 'chat.message.read.open', 'chat.message.menu.open', 'chat.message.reasoning.toggle', 'chat.message.thread.expand', 'chat.message.thread.collapse'])('aciona %s pelo binding local personalizado, sem repetição automática', async commandId => {
    const { keyboard, onDown } = await controller({
      loadMap: async () => ({ generation: 'chat-navigation', bindings: [{ shortcut, commandId, handler: 'local_ui' }] }),
    });
    try {
      window.dispatchEvent(event('keydown'));
      expect(onDown).toHaveBeenCalledWith(expect.objectContaining({ commandId, handler: 'local_ui' }));
      const repeated = event('keydown', { repeat: true });
      window.dispatchEvent(repeated);
      expect(repeated.defaultPrevented).toBe(true);
      expect(onDown).toHaveBeenCalledOnce();
      window.dispatchEvent(event('keyup'));
      window.dispatchEvent(event('keydown'));
      expect(onDown).toHaveBeenCalledTimes(2);
    } finally { keyboard.dispose(); }
  });

  it.each([false, true])('F6 com Shift=%s usa o mapa local em campo editável, sem repetir', async (shiftKey) => {
    const commandId = shiftKey ? 'navigation.landmark.previous' : 'navigation.landmark.next';
    const onDown = vi.fn(async () => {});
    const keyboard = createLocalCommandKeyboard({
      target: window,
      loadMap: async () => ({ generation: 'landmarks', bindings: [{
        shortcut: { version: 1, code: 'F6', modifiers: shiftKey ? ['Shift'] : [] }, commandId, handler: 'local_ui',
      }] }),
      onDown, onUp: vi.fn(async () => {}), reset: vi.fn(async () => {}),
      blocked: () => false, canHandleEditable: id => id === commandId,
    });
    const input = document.body.appendChild(document.createElement('textarea'));
    try {
      await keyboard.refresh();
      const down = new KeyboardEvent('keydown', { key: 'F6', code: 'F6', shiftKey, bubbles: true, cancelable: true });
      input.dispatchEvent(down);
      expect(down.defaultPrevented).toBe(true);
      expect(onDown).toHaveBeenCalledWith(expect.objectContaining({ commandId, handler: 'local_ui' }));
      input.dispatchEvent(new KeyboardEvent('keydown', { key: 'F6', code: 'F6', shiftKey, repeat: true, bubbles: true }));
      expect(onDown).toHaveBeenCalledOnce();
      input.dispatchEvent(new KeyboardEvent('keyup', { key: 'F6', code: 'F6', shiftKey, bubbles: true }));
      input.dispatchEvent(new KeyboardEvent('keydown', { key: 'F6', code: 'F6', shiftKey, bubbles: true }));
      expect(onDown).toHaveBeenCalledTimes(2);
    } finally { keyboard.dispose(); input.remove(); }
  });

  it('não amplia a exceção de F6 para outras teclas de função sem modificador', async () => {
    const { keyboard, onDown } = await controller({ loadMap: async () => ({ generation: 'bad-function', bindings: [{
      shortcut: { version: 1, code: 'F7', modifiers: [] }, commandId: 'navigation.landmark.next', handler: 'local_ui',
    }] }) });
    try {
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'F7', code: 'F7' }));
      expect(onDown).not.toHaveBeenCalled();
    } finally { keyboard.dispose(); }
  });

  it('permite somente a exceção editável explícita do comando mapeado', async () => {
    const canHandleEditable = vi.fn((commandId: string, event: KeyboardEvent) => (
      commandId === 'workspace.tab.chat.create' &&
      event.ctrlKey &&
      (event.target instanceof HTMLInputElement || event.target instanceof HTMLTextAreaElement)
    ));
    const onDown = vi.fn(async () => {});
    const keyboard = createLocalCommandKeyboard({
      target: window,
      loadMap: async () => ({
        generation: 'chat-create',
        bindings: [{ shortcut: { version: 1, code: 'KeyT', modifiers: ['Control'] }, commandId: 'workspace.tab.chat.create', handler: 'contextual' as const }],
      }),
      onDown,
      onUp: vi.fn(async () => {}),
      reset: vi.fn(async () => {}),
      blocked: () => false,
      canHandleEditable,
    });
    await keyboard.refresh();

    const input = document.body.appendChild(document.createElement('input'));
    const textarea = document.body.appendChild(document.createElement('textarea'));
    const select = document.body.appendChild(document.createElement('select'));
    const contenteditable = document.body.appendChild(document.createElement('div'));
    contenteditable.setAttribute('contenteditable', 'true');

    input.dispatchEvent(new KeyboardEvent('keydown', { code: 'KeyT', key: 't', ctrlKey: true, bubbles: true, cancelable: true }));
    expect(onDown).toHaveBeenCalledTimes(1);
    expect(canHandleEditable).toHaveBeenLastCalledWith('workspace.tab.chat.create', expect.any(KeyboardEvent));
    input.dispatchEvent(new KeyboardEvent('keyup', { code: 'KeyT', bubbles: true }));
    textarea.dispatchEvent(new KeyboardEvent('keydown', { code: 'KeyT', key: 't', ctrlKey: true, bubbles: true, cancelable: true }));
    expect(onDown).toHaveBeenCalledTimes(2);
    textarea.dispatchEvent(new KeyboardEvent('keyup', { code: 'KeyT', bubbles: true }));

    select.dispatchEvent(new KeyboardEvent('keydown', { code: 'KeyT', key: 't', ctrlKey: true, bubbles: true, cancelable: true }));
    contenteditable.dispatchEvent(new KeyboardEvent('keydown', { code: 'KeyT', key: 't', ctrlKey: true, bubbles: true, cancelable: true }));
    expect(onDown).toHaveBeenCalledTimes(2);

    window.dispatchEvent(new KeyboardEvent('keydown', { code: 'KeyK', key: 'k', ctrlKey: true, bubbles: true, cancelable: true }));
    expect(canHandleEditable).toHaveBeenCalledTimes(4);
    keyboard.dispose();
    input.remove();
    textarea.remove();
    select.remove();
    contenteditable.remove();
  });

  it('não consome tecla contextual indisponível nem consulta política para teclas comuns', async () => {
    const canHandle = vi.fn(() => false);
    const c = await controller({ canHandle });
    const down = event('keydown');
    window.dispatchEvent(down);
    expect(down.defaultPrevented).toBe(false);
    expect(c.onDown).not.toHaveBeenCalled();
    expect(canHandle).toHaveBeenCalledWith('workspace.list', down);
    window.dispatchEvent(new KeyboardEvent('keydown', { code: 'KeyX', ctrlKey: true }));
    expect(canHandle).toHaveBeenCalledTimes(1);
    canHandle.mockImplementation(() => { throw new Error('missing context'); });
    const failed = event('keydown');
    window.dispatchEvent(failed);
    expect(failed.defaultPrevented).toBe(false);
    expect(c.onDown).not.toHaveBeenCalled();
    canHandle.mockReturnValue(true);
    window.dispatchEvent(event('keydown'));
    expect(c.onDown).toHaveBeenCalledTimes(1);
    canHandle.mockReturnValue(false);
    window.dispatchEvent(event('keyup'));
    await vi.waitFor(() => expect(c.onUp).toHaveBeenCalledTimes(1));
    c.keyboard.dispose();
  });

  it('aceita F1 e F5 sem modificador no mapa efetivo e mantém outras teclas simples fechadas', async () => {
    const onDown = vi.fn(async (_request: LocalCommandKeyRequest) => {});
    const onUp = vi.fn(async () => {});
    const keyboard = createLocalCommandKeyboard({
      target: window,
      loadMap: vi.fn(async () => ({
        generation: 'g-f1-f5',
        bindings: [
          { shortcut: f1Shortcut, commandId: 'navigation.help.open', handler: 'local_ui' as const },
          { shortcut: f5Shortcut, commandId: 'editor.presentation.fullscreen', handler: 'local_ui' as const },
        ],
      })),
      onDown,
      onUp,
      reset: vi.fn(async () => {}),
      blocked: () => false,
    });
    await keyboard.refresh();

    const f1 = new KeyboardEvent('keydown', { code: 'F1', key: 'F1', cancelable: true });
    window.dispatchEvent(f1);
    expect(f1.defaultPrevented).toBe(true);
    expect(onDown).toHaveBeenCalledWith(expect.objectContaining({ commandId: 'navigation.help.open', handler: 'local_ui', repeat: false }));
    const f1Repeat = new KeyboardEvent('keydown', { code: 'F1', key: 'F1', repeat: true, cancelable: true });
    window.dispatchEvent(f1Repeat);
    expect(f1Repeat.defaultPrevented).toBe(true);
    expect(onDown).toHaveBeenCalledOnce();
    window.dispatchEvent(new KeyboardEvent('keyup', { code: 'F1', key: 'F1' }));

    const f5 = new KeyboardEvent('keydown', { code: 'F5', key: 'F5', cancelable: true });
    window.dispatchEvent(f5);
    expect(f5.defaultPrevented).toBe(true);
    expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ commandId: 'editor.presentation.fullscreen', handler: 'local_ui', repeat: false }));
    window.dispatchEvent(new KeyboardEvent('keyup', { code: 'F5', key: 'F5' }));

    const plain = new KeyboardEvent('keydown', { code: 'KeyA', key: 'a', cancelable: true });
    window.dispatchEvent(plain);
    expect(plain.defaultPrevented).toBe(false);
    expect(onDown).toHaveBeenCalledTimes(2);
    keyboard.dispose();
  });

  it('rejeita tecla de função sem modificador fora da whitelist F1/F5', async () => {
    const onDown = vi.fn(async (_request: LocalCommandKeyRequest) => {});
    const keyboard = createLocalCommandKeyboard({
      target: window,
      loadMap: vi.fn(async () => ({
        generation: 'g-f2',
        bindings: [{ shortcut: { version: 1 as const, code: 'F2', modifiers: [] }, commandId: 'editor.presentation.fullscreen', handler: 'local_ui' as const }],
      })),
      onDown,
      onUp: vi.fn(async () => {}),
      reset: vi.fn(async () => {}),
      blocked: () => false,
    });
    await keyboard.refresh();

    const f2 = new KeyboardEvent('keydown', { code: 'F2', key: 'F2', cancelable: true });
    window.dispatchEvent(f2);
    expect(f2.defaultPrevented).toBe(false);
    expect(onDown).not.toHaveBeenCalled();
    keyboard.dispose();
  });

  it('permite somente ajuda F1 quando blocked recebe o comando resolvido em modal', async () => {
    const onDown = vi.fn(async (_request: LocalCommandKeyRequest) => {});
    const onUp = vi.fn(async () => {});
    const blocked = vi.fn((commandID?: string) => commandID !== 'navigation.help.open');
    const keyboard = createLocalCommandKeyboard({
      target: window,
      loadMap: vi.fn(async () => ({ generation: 'g-f1-modal', bindings: [
        { shortcut: f1Shortcut, commandId: 'navigation.help.open', handler: 'local_ui' as const },
        { shortcut, commandId: 'workspace.list', handler: 'backend' as const },
      ] })),
      onDown,
      onUp,
      reset: vi.fn(async () => {}),
      blocked,
    });
    await keyboard.refresh();

    const command = event('keydown');
    window.dispatchEvent(command);
    expect(command.defaultPrevented).toBe(false);
    expect(blocked).toHaveBeenLastCalledWith('workspace.list', command);
    expect(onDown).not.toHaveBeenCalled();

    const f1 = new KeyboardEvent('keydown', { code: 'F1', key: 'F1', cancelable: true });
    window.dispatchEvent(f1);
    expect(f1.defaultPrevented).toBe(true);
    expect(blocked).toHaveBeenLastCalledWith('navigation.help.open', f1);
    expect(onDown).toHaveBeenCalledWith(expect.objectContaining({ commandId: 'navigation.help.open' }));
    window.dispatchEvent(new KeyboardEvent('keyup', { code: 'F1', key: 'F1' }));
    keyboard.dispose();
  });

  it('consome apenas mapa em memória e dispara down/up uma vez', async () => {
    const c = await controller();
    const repeatBeforeDown = event('keydown', { repeat: true });
    window.dispatchEvent(repeatBeforeDown);
    expect(repeatBeforeDown.defaultPrevented).toBe(true);
    expect(c.onDown).not.toHaveBeenCalled();
    const down = event('keydown');
    window.dispatchEvent(down);
    const duplicate = event('keydown');
    window.dispatchEvent(duplicate);
    window.dispatchEvent(event('keyup', { ctrlKey: false }));
    expect(c.onDown).toHaveBeenCalledTimes(1);
    await vi.waitFor(() => expect(c.onUp).toHaveBeenCalledTimes(1));
    expect(down.defaultPrevented).toBe(true);
    expect(c.loadMap).toHaveBeenCalledTimes(1);
    for (let i = 0; i < 1000; i += 1) window.dispatchEvent(new KeyboardEvent('keydown', { code: 'KeyX', ctrlKey: true }));
    expect(c.loadMap).toHaveBeenCalledTimes(1);
    expect(c.onDown).toHaveBeenCalledTimes(1);
    expect(c.onUp).toHaveBeenCalledTimes(1);
    c.keyboard.dispose();
  });

  it('ignora input, IME, AltGr, bloqueio e atalhos não mapeados', async () => {
    const blocked = vi.fn(() => false);
    const c = await controller({ blocked });
    const prevented = event('keydown');
    prevented.preventDefault();
    window.dispatchEvent(prevented);
    const input = document.body.appendChild(document.createElement('input'));
    const textarea = document.body.appendChild(document.createElement('textarea'));
    const select = document.body.appendChild(document.createElement('select'));
    const plainText = document.body.appendChild(document.createElement('div'));
    plainText.setAttribute('contenteditable', 'plaintext-only');
    const monaco = document.body.appendChild(document.createElement('div'));
    monaco.className = 'monaco-editor';
    for (const editable of [input, textarea, select, plainText, monaco]) {
      editable.dispatchEvent(event('keydown', { bubbles: true }));
    }
    window.dispatchEvent(event('keydown', { isComposing: true }));
    const altGr = event('keydown', { ctrlKey: true, altKey: true });
    Object.defineProperty(altGr, 'getModifierState', { value: (name: string) => name === 'AltGraph' });
    window.dispatchEvent(altGr);
    window.dispatchEvent(new KeyboardEvent('keydown', { code: 'KeyX', ctrlKey: true }));
    blocked.mockReturnValue(true);
    window.dispatchEvent(event('keydown'));
    expect(c.onDown).not.toHaveBeenCalled();
    blocked.mockReturnValue(false);
    window.dispatchEvent(event('keydown'));
    const blur = new FocusEvent('blur');
    window.dispatchEvent(blur);
    await vi.waitFor(() => expect(c.reset).toHaveBeenCalledWith('g1'));
    window.dispatchEvent(event('keyup'));
    expect(c.onUp).not.toHaveBeenCalled();
    c.keyboard.dispose();
    input.remove();
    textarea.remove();
    select.remove();
    plainText.remove();
    monaco.remove();
  });

  it('aguarda reset antes de carregar, descarta refresh A/B e blur em voo', async () => {
    let resolveA!: (value: { generation: string; bindings: Array<{ shortcut: CommandShortcut; commandId: string; handler: 'backend' | 'ui' }> }) => void;
    let resolveB!: (value: { generation: string; bindings: Array<{ shortcut: CommandShortcut; commandId: string; handler: 'backend' | 'ui' }> }) => void;
    let resolveBlur!: (value: { generation: string; bindings: Array<{ shortcut: CommandShortcut; commandId: string; handler: 'backend' | 'ui' }> }) => void;
    const loadMap = vi.fn()
      .mockResolvedValueOnce({ generation: 'g1', bindings: [{ shortcut, commandId: 'workspace.list', handler: 'backend' as const }] })
      .mockImplementationOnce(() => new Promise<{ generation: string; bindings: Array<{ shortcut: CommandShortcut; commandId: string; handler: 'backend' | 'ui' }> }>((resolve) => { resolveA = resolve; }))
      .mockImplementationOnce(() => new Promise<{ generation: string; bindings: Array<{ shortcut: CommandShortcut; commandId: string; handler: 'backend' | 'ui' }> }>((resolve) => { resolveB = resolve; }))
      .mockImplementationOnce(() => new Promise<{ generation: string; bindings: Array<{ shortcut: CommandShortcut; commandId: string; handler: 'backend' | 'ui' }> }>((resolve) => { resolveBlur = resolve; }));
    const reset = vi.fn(async () => {});
    const onDown = vi.fn(async (_request: LocalCommandKeyRequest) => {});
    const keyboard = createLocalCommandKeyboard({ target: window, loadMap, onDown, onUp: vi.fn(async () => {}), reset, blocked: () => false });
    await keyboard.refresh();
    const refreshA = keyboard.refresh();
    await vi.waitFor(() => expect(loadMap).toHaveBeenCalledTimes(2));
    const refreshB = keyboard.refresh();
    await vi.waitFor(() => expect(loadMap).toHaveBeenCalledTimes(3));
    resolveB({ generation: 'gB', bindings: [{ shortcut, commandId: 'workspace.list', handler: 'backend' }] });
    await refreshB;
    resolveA({ generation: 'gA', bindings: [{ shortcut, commandId: 'workspace.list', handler: 'backend' }] });
    await refreshA;
    const mapped = event('keydown');
    window.dispatchEvent(mapped);
    expect(onDown).toHaveBeenCalledTimes(1);
    expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ generation: 'gB' }));
    expect(mapped.defaultPrevented).toBe(true);
    window.dispatchEvent(event('keyup'));

    const refreshBlur = keyboard.refresh();
    await vi.waitFor(() => expect(loadMap).toHaveBeenCalledTimes(4));
    window.dispatchEvent(new FocusEvent('blur'));
    resolveBlur({ generation: 'stale', bindings: [{ shortcut, commandId: 'workspace.list', handler: 'backend' }] });
    await refreshBlur;
    const afterBlur = event('keydown');
    window.dispatchEvent(afterBlur);
    expect(afterBlur.defaultPrevented).toBe(false);
    expect(onDown).toHaveBeenCalledTimes(1);
    keyboard.dispose();
  });

  it('ignora blur de filho e reseta a geração publicada uma vez no dispose', async () => {
    const reset = vi.fn(async () => {});
    const c = await controller({ reset });
    const input = document.body.appendChild(document.createElement('input'));
    input.dispatchEvent(new Event('blur', { bubbles: true }));
    expect(reset).not.toHaveBeenCalled();
    c.keyboard.dispose();
    await Promise.resolve();
    expect(reset).toHaveBeenCalledTimes(1);
    expect(reset).toHaveBeenCalledWith('g1');
    input.remove();
  });

  it('não carrega um mapa enquanto o reset anterior está pendente', async () => {
    let release!: () => void;
    const reset = vi.fn(() => new Promise<void>((resolve) => { release = resolve; }));
    const c = await controller({ reset });
    const pending = c.keyboard.refresh();
    await vi.waitFor(() => expect(reset).toHaveBeenCalledWith('g1'));
    expect(c.loadMap).toHaveBeenCalledTimes(1);
    const down = event('keydown');
    window.dispatchEvent(down);
    expect(down.defaultPrevented).toBe(false);
    expect(c.onDown).not.toHaveBeenCalled();
    release();
    await pending;
    expect(c.loadMap).toHaveBeenCalledTimes(2);
    reset.mockResolvedValue(undefined);
    c.keyboard.dispose();
  });

  it('descarta promessa stale após dispose', async () => {
    let resolve!: (value: { generation: string; bindings: Array<{ shortcut: CommandShortcut; commandId: string; handler: 'backend' | 'ui' }> }) => void;
    const loadMap = vi.fn()
      .mockResolvedValueOnce({ generation: 'g1', bindings: [{ shortcut, commandId: 'workspace.list', handler: 'backend' as const }] })
      .mockImplementationOnce(() => new Promise<{ generation: string; bindings: Array<{ shortcut: CommandShortcut; commandId: string; handler: 'backend' | 'ui' }> }>((r) => { resolve = r; }));
    const onDown = vi.fn(async () => {});
    const onUp = vi.fn(async () => {});
    const reset = vi.fn(async () => {});
    const keyboard = createLocalCommandKeyboard({ target: window, loadMap, onDown, onUp, reset, blocked: () => false });
    await keyboard.refresh();
    const firstRefresh = keyboard.refresh();
    await vi.waitFor(() => expect(loadMap).toHaveBeenCalledTimes(2));
    keyboard.dispose();
    resolve({ generation: 'stale', bindings: [{ shortcut, commandId: 'workspace.list', handler: 'backend' }] });
    await firstRefresh;
    const afterDispose = event('keydown');
    window.dispatchEvent(afterDispose);
    expect(afterDispose.defaultPrevented).toBe(false);
    expect(onDown).not.toHaveBeenCalled();
  });

  it('falha fechado para entrada inválida e colisão ambígua, sem sobrescrever', async () => {
    const onDown = vi.fn(async () => {});
    const loadMap = vi.fn(async () => ({
      generation: 'g-invalid',
      bindings: [
        { shortcut, commandId: 'workspace.list', handler: 'backend' as const },
        { shortcut: { ...shortcut, modifiers: [...shortcut.modifiers] }, commandId: 'navigation.settings.open', handler: 'ui' as const },
      ],
    }));
    const keyboard = createLocalCommandKeyboard({ target: window, loadMap, onDown, onUp: vi.fn(async () => {}), reset: vi.fn(async () => {}), blocked: () => false });
    await keyboard.refresh();
    const down = event('keydown');
    window.dispatchEvent(down);
    expect(down.defaultPrevented).toBe(false);
    expect(onDown).not.toHaveBeenCalled();
    keyboard.dispose();
  });

  it('publica snapshot clonado do binding e requesta commandId/handler', async () => {
    const binding = { shortcut: { ...shortcut, modifiers: [...shortcut.modifiers] }, commandId: 'navigation.settings.open', handler: 'local_ui' as const };
    const onDown = vi.fn(async (_request: LocalCommandKeyRequest) => {});
    const loadMap = vi.fn(async () => ({ generation: 'g1', bindings: [binding] }));
    const keyboard = createLocalCommandKeyboard({ target: window, loadMap, onDown, onUp: vi.fn(async () => {}), reset: vi.fn(async () => {}), blocked: () => false });
    await keyboard.refresh();
    binding.shortcut.modifiers.push('Shift');
    window.dispatchEvent(event('keydown'));
    expect(onDown).toHaveBeenCalledWith(expect.objectContaining({ commandId: 'navigation.settings.open', handler: 'local_ui', generation: 'g1' }));
    const request = onDown.mock.calls[0]?.[0];
    expect(request).toBeDefined();
    expect(request?.shortcut).not.toBe(binding.shortcut);
    keyboard.dispose();
  });

  it('despacha IDs locais remapeados por onDown sem ledger e bloqueia repeat', async () => {
    const onDown = vi.fn(async (_request: LocalCommandKeyRequest) => {});
    const onUp = vi.fn(async () => {});
    const paletteShortcut: CommandShortcut = { version: 1, code: 'KeyE', modifiers: ['Alt'] };
    const keyboard = createLocalCommandKeyboard({
      target: window,
      loadMap: vi.fn(async () => ({
        generation: 'g-remapped-local',
        bindings: [
          { shortcut, commandId: 'navigation.history.open', handler: 'local_ui' as const },
          { shortcut: paletteShortcut, commandId: 'navigation.palette.open', handler: 'local_ui' as const },
        ],
      })),
      onDown,
      onUp,
      reset: vi.fn(async () => {}),
      blocked: () => false,
    });
    await keyboard.refresh();

    const history = event('keydown');
    window.dispatchEvent(history);
    expect(history.defaultPrevented).toBe(true);
    expect(onDown).toHaveBeenNthCalledWith(1, expect.objectContaining({
      generation: 'g-remapped-local', commandId: 'navigation.history.open', handler: 'local_ui', kind: 'down', repeat: false,
    }));
    const historyRepeat = event('keydown', { repeat: true });
    window.dispatchEvent(historyRepeat);
    expect(historyRepeat.defaultPrevented).toBe(true);
    expect(onDown).toHaveBeenCalledTimes(1);
    window.dispatchEvent(event('keyup', { ctrlKey: false }));

    const palette = new KeyboardEvent('keydown', { code: 'KeyE', key: 'e', altKey: true, cancelable: true });
    window.dispatchEvent(palette);
    expect(palette.defaultPrevented).toBe(true);
    expect(onDown).toHaveBeenNthCalledWith(2, expect.objectContaining({
      generation: 'g-remapped-local', commandId: 'navigation.palette.open', handler: 'local_ui', kind: 'down', repeat: false,
    }));
    const paletteRepeat = new KeyboardEvent('keydown', { code: 'KeyE', key: 'e', altKey: true, repeat: true, cancelable: true });
    window.dispatchEvent(paletteRepeat);
    expect(paletteRepeat.defaultPrevented).toBe(true);
    expect(onDown).toHaveBeenCalledTimes(2);
    window.dispatchEvent(new KeyboardEvent('keyup', { code: 'KeyE', key: 'e', altKey: false }));

    expect(onUp).toHaveBeenCalledTimes(2);
    keyboard.dispose();
  });

  it('emite release também para binding UI, permitindo novo pressionamento na mesma rota', async () => {
    const onDown = vi.fn(async () => {});
    const onUp = vi.fn(async () => {});
    const keyboard = createLocalCommandKeyboard({
      target: window,
      loadMap: vi.fn(async () => ({ generation: 'g-ui', bindings: [{ shortcut, commandId: 'navigation.settings.open', handler: 'local_ui' as const }] })),
      onDown,
      onUp,
      reset: vi.fn(async () => {}),
      blocked: () => false,
    });
    await keyboard.refresh();
    window.dispatchEvent(event('keydown'));
    window.dispatchEvent(event('keyup'));
    expect(onDown).toHaveBeenCalledWith(expect.objectContaining({ commandId: 'navigation.settings.open', handler: 'local_ui' }));
    expect(onUp).toHaveBeenCalledWith(expect.objectContaining({ commandId: 'navigation.settings.open', handler: 'local_ui', kind: 'up' }));
    window.dispatchEvent(event('keydown'));
    expect(onDown).toHaveBeenCalledTimes(2);
    expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ generation: 'g-ui', commandId: 'navigation.settings.open', handler: 'local_ui' }));
    keyboard.dispose();
  });

  it('aguarda onDown deferred antes do keyup e libera exatamente uma vez após resolve', async () => {
    let resolveDown!: () => void;
    const onDown = vi.fn(() => new Promise<void>((resolve) => { resolveDown = resolve; }));
    const onUp = vi.fn(async () => {});
    const keyboard = createLocalCommandKeyboard({
      target: window,
      loadMap: vi.fn(async () => ({ generation: 'g-deferred', bindings: [{ shortcut, commandId: 'workspace.list', handler: 'backend' as const }] })),
      onDown,
      onUp,
      reset: vi.fn(async () => {}),
      blocked: () => false,
    });
    await keyboard.refresh();

    window.dispatchEvent(event('keydown'));
    window.dispatchEvent(event('keyup', { ctrlKey: false }));
    expect(onDown).toHaveBeenCalledOnce();
    expect(onUp).not.toHaveBeenCalled();

    resolveDown();
    await vi.waitFor(() => expect(onUp).toHaveBeenCalledOnce());
    expect(onUp).toHaveBeenCalledWith(expect.objectContaining({
      generation: 'g-deferred', commandId: 'workspace.list', kind: 'up', repeat: false,
    }));
    keyboard.dispose();
  });

  it('descarta down/up pendente ao blur e não libera a geração antiga após publicar uma nova', async () => {
    let resolveDown!: () => void;
    let deferFirstDown = true;
    const onDown = vi.fn(() => deferFirstDown
      ? new Promise<void>((resolve) => { resolveDown = resolve; })
      : Promise.resolve());
    const onUp = vi.fn(async () => {});
    const reset = vi.fn(async () => {});
    const loadMap = vi.fn()
      .mockResolvedValueOnce({ generation: 'g-before-blur', bindings: [{ shortcut, commandId: 'workspace.list', handler: 'backend' as const }] })
      .mockResolvedValueOnce({ generation: 'g-after-blur', bindings: [{ shortcut, commandId: 'workspace.list', handler: 'backend' as const }] });
    const keyboard = createLocalCommandKeyboard({ target: window, loadMap, onDown, onUp, reset, blocked: () => false });
    await keyboard.refresh();

    window.dispatchEvent(event('keydown'));
    window.dispatchEvent(new FocusEvent('blur'));
    await vi.waitFor(() => expect(reset).toHaveBeenCalledWith('g-before-blur'));
    expect(onUp).not.toHaveBeenCalled();

    resolveDown();
    await Promise.resolve();
    window.dispatchEvent(event('keyup', { ctrlKey: false }));
    expect(onUp).not.toHaveBeenCalled();

    await keyboard.refresh();
    deferFirstDown = false;
    window.dispatchEvent(event('keydown'));
    window.dispatchEvent(event('keyup', { ctrlKey: false }));
    await vi.waitFor(() => expect(onUp).toHaveBeenCalledOnce());
    expect(onUp).toHaveBeenCalledWith(expect.objectContaining({ generation: 'g-after-blur' }));
    keyboard.dispose();
  });

  it('descarta down/up pendente ao dispose sem release tardio ou estado preso', async () => {
    let resolveDown!: () => void;
    const onDown = vi.fn(() => new Promise<void>((resolve) => { resolveDown = resolve; }));
    const onUp = vi.fn(async () => {});
    const keyboard = createLocalCommandKeyboard({
      target: window,
      loadMap: vi.fn(async () => ({ generation: 'g-dispose', bindings: [{ shortcut, commandId: 'workspace.list', handler: 'backend' as const }] })),
      onDown,
      onUp,
      reset: vi.fn(async () => {}),
      blocked: () => false,
    });
    await keyboard.refresh();

    window.dispatchEvent(event('keydown'));
    window.dispatchEvent(event('keyup', { ctrlKey: false }));
    keyboard.dispose();
    resolveDown();
    await Promise.resolve();
    expect(onUp).not.toHaveBeenCalled();
    window.dispatchEvent(event('keyup', { ctrlKey: false }));
    expect(onUp).not.toHaveBeenCalled();
  });

  it('permite repeat físico somente para allowlist explícita e mantém pressed até Up', async () => {
    const onDown = vi.fn(async (_request: LocalCommandKeyRequest) => {});
    const onUp = vi.fn(async () => {});
    const keyboard = createLocalCommandKeyboard({
      target: window,
      loadMap: vi.fn(async () => ({
        generation: 'g-navigation',
        bindings: [{ shortcut, commandId: 'workspace.tab.next', handler: 'local_ui' as const }],
      })),
      onDown,
      onUp,
      reset: vi.fn(async () => {}),
      blocked: () => false,
      canRepeat: (commandID) => commandID === 'workspace.tab.next',
    });
    await keyboard.refresh();
    window.dispatchEvent(event('keydown'));
    window.dispatchEvent(event('keydown', { repeat: true }));
    expect(onDown).toHaveBeenCalledTimes(2);
    expect(onDown.mock.calls[1]?.[0]).toMatchObject({ repeat: true, kind: 'down' });
    window.dispatchEvent(event('keyup'));
    expect(onUp).toHaveBeenCalledOnce();
    keyboard.dispose();
  });

  it('publica somente o mapa aceito e invalida respostas fora de ordem', async () => {
    let resolveOld!: (value: LocalCommandKeyboardMap) => void;
    let resolveNew!: (value: LocalCommandKeyboardMap) => void;
    const loadMap = vi.fn()
      .mockResolvedValueOnce({ generation: 'initial', ownerId: 'owner-current', bindings: [] })
      .mockImplementationOnce(() => new Promise((resolve) => { resolveOld = resolve; }))
      .mockImplementationOnce(() => new Promise((resolve) => { resolveNew = resolve; }));
    const accepted: string[] = [];
    const invalidated = vi.fn();
    const keyboard = createLocalCommandKeyboard({
      target: window,
      loadMap,
      onDown: vi.fn(async () => {}),
      onUp: vi.fn(async () => {}),
      reset: vi.fn(async () => {}),
      blocked: () => false,
      acceptMap: (map) => map.ownerId === 'owner-current',
      onMapAccepted: (map) => accepted.push(map.generation),
      onMapInvalidated: invalidated,
    });
    await keyboard.refresh();
    const first = keyboard.refresh();
    await vi.waitFor(() => expect(loadMap).toHaveBeenCalledTimes(2));
    const second = keyboard.refresh();
    await vi.waitFor(() => expect(loadMap).toHaveBeenCalledTimes(3));
    resolveNew({ generation: 'new', ownerId: 'owner-current', bindings: [] });
    await second;
    resolveOld({ generation: 'old', ownerId: 'owner-current', bindings: [] });
    await first;
    expect(accepted).toEqual(['initial', 'new']);
    expect(invalidated).toHaveBeenCalled();
    keyboard.dispose();
  });

  it('mantém o prefixo local e só envia down/up v2 na tecla final', async () => {
    const onSequenceStarted = vi.fn();
    const onSequenceCancelled = vi.fn();
    const onDown = vi.fn(async () => {});
    const onUp = vi.fn(async () => {});
    const keyboard = createLocalCommandKeyboard({
      target: window,
      loadMap: async () => ({ generation: 'g-sequence', bindings: [{ shortcut: sequence, commandId: 'workspace.tab.terminal.create', handler: 'contextual' as const }] }),
      onDown,
      onUp,
      reset: vi.fn(async () => {}),
      blocked: () => false,
      onSequenceStarted,
      onSequenceCancelled,
      canHandle: vi.fn(() => true),
    });
    await keyboard.refresh();

    const prefix = sequenceEvent('keydown', 'KeyK', { ctrlKey: true });
    window.dispatchEvent(prefix);
    expect(prefix.defaultPrevented).toBe(true);
    expect(onSequenceStarted).toHaveBeenCalledWith([expect.objectContaining({ commandId: 'workspace.tab.terminal.create' })]);
    expect(onDown).not.toHaveBeenCalled();
    window.dispatchEvent(sequenceEvent('keyup', 'KeyK', { ctrlKey: false }));
    const final = sequenceEvent('keydown', 'KeyN');
    window.dispatchEvent(final);
    expect(final.defaultPrevented).toBe(true);
    expect(onDown).toHaveBeenCalledWith(expect.objectContaining({ shortcut: sequence, kind: 'down', repeat: false }));
    window.dispatchEvent(sequenceEvent('keyup', 'KeyN'));
    await vi.waitFor(() => expect(onUp).toHaveBeenCalledWith(expect.objectContaining({ shortcut: sequence, kind: 'up' })));
    expect(onSequenceCancelled).not.toHaveBeenCalled();
    keyboard.dispose();
  });

  it('executa final Enter mapeado e não o entrega como navegação de menu', async () => {
    const enterSequence: CommandShortcutSequence = {
      version: 2,
      steps: [{ code: 'KeyN', modifiers: ['Control'] }, { code: 'Enter', modifiers: [] }],
    };
    const onSequenceCancelled = vi.fn();
    const onDown = vi.fn(async () => {});
    const onUp = vi.fn(async () => {});
    const keyboard = createLocalCommandKeyboard({
      target: window,
      loadMap: async () => ({ generation: 'g-enter-sequence', bindings: [{
        shortcut: enterSequence, commandId: 'workspace.tab.chat.create', handler: 'contextual' as const,
      }] }),
      onDown,
      onUp,
      reset: vi.fn(async () => {}),
      blocked: () => false,
      onSequenceCancelled,
    });
    await keyboard.refresh();

    window.dispatchEvent(sequenceEvent('keydown', 'KeyN', { ctrlKey: true }));
    const final = sequenceEvent('keydown', 'Enter');
    window.dispatchEvent(final);
    expect(final.defaultPrevented).toBe(true);
    expect(onSequenceCancelled).not.toHaveBeenCalledWith('menu-navigation');
    expect(onDown).toHaveBeenCalledWith(expect.objectContaining({
      commandId: 'workspace.tab.chat.create', shortcut: enterSequence, kind: 'down',
    }));

    window.dispatchEvent(sequenceEvent('keyup', 'Enter'));
    await vi.waitFor(() => expect(onUp).toHaveBeenCalledOnce());
    keyboard.dispose();
  });

  it('prioriza binding simples, processa tecla inesperada standalone e cancela Escape/timeout', async () => {
    vi.useFakeTimers();
    const simple = { version: 1 as const, code: 'KeyX', modifiers: ['Control'] as const };
    const onSequenceCancelled = vi.fn();
    const onDown = vi.fn(async () => {});
    const keyboard = createLocalCommandKeyboard({
      target: window,
      loadMap: async () => ({ generation: 'g-sequence-cancel', bindings: [
        { shortcut: sequence, commandId: 'workspace.tab.terminal.create', handler: 'contextual' as const },
        { shortcut: { ...simple, modifiers: [...simple.modifiers] }, commandId: 'workspace.list', handler: 'backend' as const },
      ] }),
      onDown,
      onUp: vi.fn(async () => {}),
      reset: vi.fn(async () => {}),
      blocked: () => false,
      onSequenceCancelled,
    });
    await keyboard.refresh();
    window.dispatchEvent(sequenceEvent('keydown', 'KeyK', { ctrlKey: true }));
    const unexpected = sequenceEvent('keydown', 'KeyX', { ctrlKey: true });
    window.dispatchEvent(unexpected);
    expect(unexpected.defaultPrevented).toBe(true);
    expect(onDown).toHaveBeenCalledWith(expect.objectContaining({ commandId: 'workspace.list', kind: 'down' }));
    expect(onSequenceCancelled).toHaveBeenLastCalledWith('unexpected');
    window.dispatchEvent(sequenceEvent('keyup', 'KeyX', { ctrlKey: false }));

    window.dispatchEvent(sequenceEvent('keydown', 'KeyK', { ctrlKey: true }));
    window.dispatchEvent(sequenceEvent('keydown', 'Escape'));
    expect(onSequenceCancelled).toHaveBeenLastCalledWith('escape');
    window.dispatchEvent(sequenceEvent('keydown', 'KeyK', { ctrlKey: true }));
    vi.advanceTimersByTime(1500);
    expect(onSequenceCancelled).toHaveBeenLastCalledWith('timeout');
    keyboard.dispose();
    vi.useRealTimers();
  });

  it('prioriza binding v1 simples quando ele compartilha exatamente o prefixo da sequência', async () => {
    const onSequenceStarted = vi.fn();
    const onDown = vi.fn(async () => {});
    const onUp = vi.fn(async () => {});
    const keyboard = createLocalCommandKeyboard({
      target: window,
      loadMap: async () => ({ generation: 'g-same-prefix', bindings: [
        { shortcut: sequence, commandId: 'workspace.tab.terminal.create', handler: 'contextual' as const },
        { shortcut: { version: 1 as const, code: 'KeyK', modifiers: ['Control'] as const }, commandId: 'workspace.list', handler: 'backend' as const },
      ] }),
      onDown,
      onUp,
      reset: vi.fn(async () => {}),
      blocked: () => false,
      onSequenceStarted,
    });
    await keyboard.refresh();

    const down = sequenceEvent('keydown', 'KeyK', { ctrlKey: true });
    window.dispatchEvent(down);
    expect(down.defaultPrevented).toBe(true);
    expect(onSequenceStarted).not.toHaveBeenCalled();
    expect(onDown).toHaveBeenCalledWith(expect.objectContaining({ commandId: 'workspace.list', kind: 'down' }));

    window.dispatchEvent(sequenceEvent('keyup', 'KeyK', { ctrlKey: false }));
    await vi.waitFor(() => expect(onUp).toHaveBeenCalledOnce());
    expect(onUp).toHaveBeenCalledWith(expect.objectContaining({ commandId: 'workspace.list', kind: 'up' }));
    keyboard.dispose();
  });

  it('cede setas e confirmação ao menu ao cancelar prefixo com menu-navigation', async () => {
    const onSequenceCancelled = vi.fn();
    const keyboard = createLocalCommandKeyboard({
      target: window,
      loadMap: async () => ({ generation: 'g-menu-sequence', bindings: [{ shortcut: sequence, commandId: 'workspace.tab.terminal.create', handler: 'contextual' as const }] }),
      onDown: vi.fn(async () => {}),
      onUp: vi.fn(async () => {}),
      reset: vi.fn(async () => {}),
      blocked: () => false,
      onSequenceCancelled,
    });
    await keyboard.refresh();
    window.dispatchEvent(sequenceEvent('keydown', 'KeyK', { ctrlKey: true }));
    const arrow = sequenceEvent('keydown', 'ArrowDown');
    window.dispatchEvent(arrow);
    expect(arrow.defaultPrevented).toBe(false);
    expect(onSequenceCancelled).toHaveBeenLastCalledWith('menu-navigation');
    window.dispatchEvent(sequenceEvent('keydown', 'KeyK', { ctrlKey: true }));
    const enter = sequenceEvent('keydown', 'Enter');
    window.dispatchEvent(enter);
    expect(enter.defaultPrevented).toBe(false);
    expect(onSequenceCancelled).toHaveBeenLastCalledWith('menu-navigation');
    keyboard.dispose();
  });

  it('não inicia nem finaliza sequência por repeat e cancela em IME/AltGraph/blur/refresh', async () => {
    const onSequenceStarted = vi.fn();
    const onSequenceCancelled = vi.fn();
    const onDown = vi.fn(async () => {});
    const keyboard = createLocalCommandKeyboard({
      target: window,
      loadMap: async () => ({ generation: 'g-sequence-guards', bindings: [{ shortcut: sequence, commandId: 'workspace.tab.terminal.create', handler: 'contextual' as const }] }),
      onDown,
      onUp: vi.fn(async () => {}),
      reset: vi.fn(async () => {}),
      blocked: () => false,
      onSequenceStarted,
      onSequenceCancelled,
    });
    await keyboard.refresh();
    const repeatedInitialPrefix = sequenceEvent('keydown', 'KeyK', { ctrlKey: true, repeat: true });
    window.dispatchEvent(repeatedInitialPrefix);
    expect(repeatedInitialPrefix.defaultPrevented).toBe(true);
    expect(onSequenceStarted).not.toHaveBeenCalled();
    const prefix = sequenceEvent('keydown', 'KeyK', { ctrlKey: true });
    window.dispatchEvent(prefix);
    const repeatedPrefix = sequenceEvent('keydown', 'KeyK', { ctrlKey: true, repeat: true });
    window.dispatchEvent(repeatedPrefix);
    expect(repeatedPrefix.defaultPrevented).toBe(true);
    expect(onSequenceStarted).toHaveBeenCalledOnce();
    expect(onSequenceCancelled).not.toHaveBeenCalled();
    const repeatedFinal = sequenceEvent('keydown', 'KeyN', { repeat: true });
    window.dispatchEvent(repeatedFinal);
    expect(repeatedFinal.defaultPrevented).toBe(true);
    expect(onDown).not.toHaveBeenCalled();
    window.dispatchEvent(sequenceEvent('keydown', 'KeyK', { ctrlKey: true, isComposing: true }));
    expect(onSequenceCancelled).toHaveBeenLastCalledWith('ime');
    window.dispatchEvent(sequenceEvent('keydown', 'KeyK', { ctrlKey: true }));
    const altGraph = sequenceEvent('keydown', 'KeyA', { ctrlKey: true, altKey: true });
    Object.defineProperty(altGraph, 'getModifierState', { value: (name: string) => name === 'AltGraph' });
    window.dispatchEvent(altGraph);
    expect(onSequenceCancelled).toHaveBeenLastCalledWith('altgraph');
    window.dispatchEvent(sequenceEvent('keydown', 'KeyK', { ctrlKey: true }));
    window.dispatchEvent(new FocusEvent('blur'));
    expect(onSequenceCancelled).toHaveBeenLastCalledWith('blur');
    keyboard.dispose();
  });
});
