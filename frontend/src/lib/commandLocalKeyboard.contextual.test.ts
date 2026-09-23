import { describe, expect, it, vi } from 'vitest';
import { createLocalCommandKeyboard, type LocalCommandKeyboardBinding, type LocalCommandKeyboardMap } from './commandLocalKeyboard';
import type { CommandShortcut, CommandShortcutSequenceStep } from './commandShortcut';

const shortcut: CommandShortcut = { version: 1, code: 'KeyI', modifiers: ['Alt'] };
const binding = (commandId: string): LocalCommandKeyboardBinding => ({ shortcut, commandId, handler: 'local_ui' });
const editor = binding('editor.menu.insert.open');
const other = binding('navigation.data.import.open');
const map = (): LocalCommandKeyboardMap => ({ generation: 'g1', bindings: [],
  contextualBindings: [{ shortcut, bySurface: { editor }, fallback: other }] });
const key = (type: string, extra: KeyboardEventInit = {}) => {
  const event = new KeyboardEvent(type, { code: 'KeyI', key: 'i', altKey: true, bubbles: true, cancelable: true, ...extra });
  window.dispatchEvent(event);
  return event;
};

describe('mapa contextual resolvido pelo host', () => {
  it('Ctrl+N contextual não inicia sequência; fallback explícito preserva sequência fora das páginas', async () => {
    let surface: string | undefined = 'tasklists';
    const prefix: CommandShortcut = { version: 1, code: 'KeyN', modifiers: ['Control'] };
    const onDown = vi.fn(async () => {});
    const onSequenceStarted = vi.fn();
    const config: LocalCommandKeyboardMap = { generation: 'pages', bindings: [{
      commandId: 'workspace.tab.chat.create', handler: 'contextual',
      shortcut: { version: 2, steps: [{ code: 'KeyN', modifiers: ['Control'] }, { code: 'KeyC', modifiers: [] }] },
    }], contextualBindings: [{ shortcut: prefix, bySurface: {
      tasklists: { shortcut: prefix, commandId: 'tasklists.create.open', handler: 'local_ui' }, profiles: null,
    }, fallback: null, fallbackToSequences: true }] };
    const controller = createLocalCommandKeyboard({ target: window, loadMap: async () => config, onDown,
      onUp: async () => {}, reset: async () => {}, blocked: () => false, readSurfaceType: () => surface, onSequenceStarted });
    const press = () => { key('keydown', { code: 'KeyN', key: 'n', altKey: false, ctrlKey: true }); key('keyup', { code: 'KeyN', altKey: false }); };
    try {
      await controller.refresh(); press();
      expect(onDown).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ commandId: 'tasklists.create.open' }));
      expect(onSequenceStarted).not.toHaveBeenCalled();
      surface = 'profiles'; press();
      surface = undefined; press();
      expect(onDown).toHaveBeenCalledTimes(1); expect(onSequenceStarted).not.toHaveBeenCalled();
      surface = 'chat'; press();
      expect(onSequenceStarted).toHaveBeenCalledOnce();
      key('keydown', { code: 'KeyC', key: 'c', altKey: false }); key('keyup', { code: 'KeyC', altKey: false });
      expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ commandId: 'workspace.tab.chat.create' }));
      config.contextualBindings![0].fallbackToSequences = false;
      await controller.refresh(); onSequenceStarted.mockClear(); press();
      expect(onSequenceStarted).not.toHaveBeenCalled();
      config.contextualBindings![0].fallbackToSequences = true;
      config.bindings = [];
      await controller.refresh();
      expect(key('keydown', { code: 'KeyN', key: 'n', altKey: false, ctrlKey: true }).defaultPrevented).toBe(true);
      expect(onSequenceStarted).not.toHaveBeenCalled();
    } finally { controller.dispose(); }
  });
  it('seleciona contexto vivo sem IPC, sem traduzir IDs e sem duplicar repeat', async () => {
    let surface: string | undefined = 'editor';
    const onDown = vi.fn(async () => {});
    const loadMap = vi.fn(async () => map());
    const controller = createLocalCommandKeyboard({ target: window, loadMap, onDown,
      onUp: async () => {}, reset: async () => {}, blocked: () => false, readSurfaceType: () => surface });
    try {
      await controller.refresh();
      key('keydown'); key('keydown', { repeat: true }); key('keyup');
      expect(onDown).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ commandId: editor.commandId }));
      surface = 'chat'; key('keydown'); key('keyup');
      expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ commandId: other.commandId }));
      surface = undefined;
      expect(key('keydown').defaultPrevented).toBe(true);
      expect(onDown).toHaveBeenCalledTimes(2);
      expect(loadMap).toHaveBeenCalledOnce();
    } finally { controller.dispose(); }
  });

  it('null bloqueia fallback e o guard recusa sem experimentar outro candidato', async () => {
    const config = map();
    config.contextualBindings![0].bySurface.editor = null;
    const onDown = vi.fn(async () => {});
    const controller = createLocalCommandKeyboard({ target: window, loadMap: async () => config, onDown,
      onUp: async () => {}, reset: async () => {}, blocked: () => false, readSurfaceType: () => 'editor' });
    try {
      await controller.refresh();
      expect(key('keydown').defaultPrevented).toBe(true); key('keyup');
      expect(onDown).not.toHaveBeenCalled();
    } finally { controller.dispose(); }
  });

  it.each(['duplicate', 'durable', 'different-key', 'missing-fallback', 'global-collision', 'fallback-and-sequence', 'invalid-sequence-flag'] as const)('recusa mapa contextual inválido: %s', async fault => {
    const config = map();
    const entry = config.contextualBindings![0];
    if (fault === 'duplicate') config.contextualBindings!.push(entry);
    if (fault === 'durable') entry.bySurface.editor = { ...editor, handler: 'contextual', commandId: 'workspace.create' };
    if (fault === 'different-key') entry.bySurface.editor = { ...editor, shortcut: { ...shortcut, code: 'KeyX' } };
    if (fault === 'missing-fallback') delete (entry as Partial<typeof entry>).fallback;
    if (fault === 'global-collision') config.bindings.push(other);
    if (fault === 'fallback-and-sequence') entry.fallbackToSequences = true;
    if (fault === 'invalid-sequence-flag') Object.assign(entry, { fallbackToSequences: 'true' });
    const onDown = vi.fn(async () => {});
    const invalid = vi.fn();
    const controller = createLocalCommandKeyboard({ target: window, loadMap: async () => config, onDown,
      onUp: async () => {}, reset: async () => {}, blocked: () => false, readSurfaceType: () => 'editor', onMapInvalidated: invalid });
    try {
      await controller.refresh(); key('keydown'); key('keyup');
      expect(onDown).not.toHaveBeenCalled(); expect(invalid).toHaveBeenCalled();
    } finally { controller.dispose(); }
  });

  it('respeita guard, IME e refresh suprimido sem usar fallback global', async () => {
    let config = map();
    let allowed = false;
    const onDown = vi.fn(async () => {});
    const controller = createLocalCommandKeyboard({ target: window, loadMap: async () => config, onDown,
      onUp: async () => {}, reset: async () => {}, blocked: () => false, readSurfaceType: () => 'editor', canHandle: () => allowed });
    try {
      await controller.refresh(); key('keydown'); key('keyup');
      allowed = true; key('keydown', { isComposing: true }); key('keyup');
      expect(onDown).not.toHaveBeenCalled();
      config = map(); config.contextualBindings![0].bySurface.editor = null;
      await controller.refresh(); key('keydown'); key('keyup');
      expect(onDown).not.toHaveBeenCalled();
    } finally { controller.dispose(); }
  });

  it('isola o snapshot aceito e não repete uma tecla em outra superfície', async () => {
    const config = map();
    let surface = 'editor';
    const onDown = vi.fn(async () => {});
    const controller = createLocalCommandKeyboard({ target: window, loadMap: async () => config, onDown,
      onUp: async () => {}, reset: async () => {}, blocked: () => false,
      readSurfaceType: () => surface, canRepeat: () => true });
    try {
      await controller.refresh();
      config.contextualBindings![0].bySurface.editor = null;
      config.contextualBindings![0].fallback = null;
      key('keydown');
      expect(onDown).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ commandId: editor.commandId }));
      surface = 'chat';
      key('keydown', { repeat: true });
      expect(onDown).toHaveBeenCalledTimes(1);
      key('keyup'); key('keydown'); key('keyup');
      expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ commandId: other.commandId }));
      expect(onDown).toHaveBeenCalledTimes(2);
    } finally { controller.dispose(); }
  });

  it('combina duas sequências contextuais, sequência flat e fallback v1 sem resolver prioridade', async () => {
    const prefix: CommandShortcut = { version: 1, code: 'KeyN', modifiers: ['Control'] };
    const prefixStep: CommandShortcutSequenceStep = { code: 'KeyN', modifiers: ['Control'] };
    const first: LocalCommandKeyboardBinding = {
      shortcut: { version: 2, steps: [prefixStep, { code: 'KeyC', modifiers: [] }] },
      commandId: 'workspace.list', handler: 'backend',
    };
    const second: LocalCommandKeyboardBinding = {
      shortcut: { version: 2, steps: [prefixStep, { code: 'KeyT', modifiers: [] }] },
      commandId: 'workspace.tab.chat.create', handler: 'ui',
    };
    const flat: LocalCommandKeyboardBinding = {
      shortcut: { version: 2, steps: [prefixStep, { code: 'KeyP', modifiers: [] }] },
      commandId: 'workspace.list', handler: 'backend',
    };
    const v1: LocalCommandKeyboardBinding = { shortcut: prefix, commandId: 'tasklists.create.open', handler: 'local_ui' };
    const config: LocalCommandKeyboardMap = {
      generation: 'v2-multi', bindings: [flat], contextualBindings: [
        { shortcut: first.shortcut, bySurface: { editor: first }, fallback: null },
        { shortcut: second.shortcut, bySurface: { editor: second }, fallback: null },
        { shortcut: prefix, bySurface: { tasklists: v1 }, fallback: null, fallbackToSequences: true },
      ],
    };
    let surface = 'editor';
    const onDown = vi.fn(async () => {});
    const context = () => ({ surfaceId: `surface-${surface}`, surfaceType: surface, isCurrent: () => true });
    const controller = createLocalCommandKeyboard({ target: window, loadMap: async () => config, onDown,
      onUp: async () => {}, reset: async () => {}, blocked: () => false,
      readSurfaceType: () => surface, readContext: context });
    const dispatch = (type: string, code: string, ctrlKey = false) => {
      const event = new KeyboardEvent(type, { code, key: code, ctrlKey, bubbles: true, cancelable: true });
      window.dispatchEvent(event);
      return event;
    };
    const press = (finalCode: string) => {
      dispatch('keydown', 'KeyN', true); dispatch('keyup', 'KeyN');
      dispatch('keydown', finalCode); dispatch('keyup', finalCode);
    };
    try {
      await controller.refresh();
      press('KeyC');
      expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ commandId: first.commandId,
        context: { surfaceId: 'surface-editor', surfaceType: 'editor' } }));
      press('KeyT');
      expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ commandId: second.commandId,
        context: { surfaceId: 'surface-editor', surfaceType: 'editor' } }));
      surface = 'chat';
      press('KeyP');
      expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ commandId: flat.commandId, context: undefined }));
      surface = 'tasklists';
      dispatch('keydown', 'KeyN', true);
      expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ commandId: v1.commandId }));
      dispatch('keyup', 'KeyN');
    } finally { controller.dispose(); }
  });

  it('isola null de uma sequência v2, filtra canHandle individualmente e exige lease durável', async () => {
    const prefixStep: CommandShortcutSequenceStep = { code: 'KeyN', modifiers: ['Control'] };
    const blocked: LocalCommandKeyboardBinding = {
      shortcut: { version: 2, steps: [prefixStep, { code: 'KeyC', modifiers: [] }] }, commandId: 'workspace.list', handler: 'backend',
    };
    const allowed: LocalCommandKeyboardBinding = {
      shortcut: { version: 2, steps: [prefixStep, { code: 'KeyT', modifiers: [] }] }, commandId: 'workspace.tab.chat.create', handler: 'ui',
    };
    const makeMap = (blockedBranch: LocalCommandKeyboardBinding | null = blocked) => ({ generation: 'v2-guards', bindings: [], contextualBindings: [
      { shortcut: blocked.shortcut, bySurface: { editor: blockedBranch }, fallback: null },
      { shortcut: allowed.shortcut, bySurface: { editor: allowed }, fallback: null },
    ] } satisfies LocalCommandKeyboardMap);
    const dispatch = (type: string, code: string) => {
      const event = new KeyboardEvent(type, { code, key: code, ctrlKey: type === 'keydown' && code === 'KeyN', bubbles: true, cancelable: true });
      window.dispatchEvent(event);
      return event;
    };
    const onDown = vi.fn(async () => {});
    const controller = createLocalCommandKeyboard({ target: window, loadMap: async () => makeMap(null), onDown,
      onUp: async () => {}, reset: async () => {}, blocked: () => false, readSurfaceType: () => 'editor',
      readContext: () => ({ surfaceId: 'editor-1', surfaceType: 'editor', isCurrent: () => true }),
      canHandle: commandId => commandId !== blocked.commandId });
    try {
      await controller.refresh();
      dispatch('keydown', 'KeyN'); dispatch('keyup', 'KeyN'); dispatch('keydown', 'KeyT'); dispatch('keyup', 'KeyT');
      expect(onDown).toHaveBeenCalledWith(expect.objectContaining({ commandId: allowed.commandId }));
      expect(onDown).not.toHaveBeenCalledWith(expect.objectContaining({ commandId: blocked.commandId }));
    } finally { controller.dispose(); }

    const invalid = vi.fn();
    const duplicateIdentity = createLocalCommandKeyboard({ target: window, loadMap: async () => ({
      generation: 'invalid-v2-identity', bindings: [blocked], contextualBindings: [{ shortcut: blocked.shortcut, bySurface: { editor: blocked }, fallback: null }],
    }), onDown: vi.fn(async () => {}), onUp: async () => {}, reset: async () => {}, blocked: () => false, onMapInvalidated: invalid });
    try { await duplicateIdentity.refresh(); expect(invalid).toHaveBeenCalled(); } finally { duplicateIdentity.dispose(); }

    const noLease = createLocalCommandKeyboard({ target: window, loadMap: async () => makeMap(), onDown: vi.fn(async () => {}),
      onUp: async () => {}, reset: async () => {}, blocked: () => false, readSurfaceType: () => 'editor' });
    try {
      await noLease.refresh();
      const prefixEvent = dispatch('keydown', 'KeyN');
      expect(prefixEvent.defaultPrevented).toBe(true);
    } finally { noLease.dispose(); }
  });
});
