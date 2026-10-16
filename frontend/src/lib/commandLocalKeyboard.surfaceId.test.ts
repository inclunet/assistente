import { afterEach, describe, expect, it, vi } from 'vitest';
import { createLocalCommandKeyboard, type LocalCommandKeyboardBinding, type LocalCommandKeyboardMap } from './commandLocalKeyboard';
import type { CommandShortcut, CommandShortcutSequenceStep } from './commandShortcut';

const prefix: CommandShortcut = { version: 1, code: 'KeyI', modifiers: ['Control'] };
const step: CommandShortcutSequenceStep = { code: 'KeyI', modifiers: ['Control'] };

const binding = (commandId: string, shortcut: LocalCommandKeyboardBinding['shortcut'] = prefix): LocalCommandKeyboardBinding => ({
  shortcut, commandId, handler: 'backend',
});

const event = (type: string, code: string, options: KeyboardEventInit = {}) => {
  const result = new KeyboardEvent(type, { code, key: code, bubbles: true, cancelable: true, ...options });
  window.dispatchEvent(result);
  return result;
};

const baseMap = (bySurfaceId: Record<string, Record<string, LocalCommandKeyboardBinding | null>>): LocalCommandKeyboardMap => ({
  generation: 'surface-id',
  bindings: [],
  contextualBindings: [{ shortcut: prefix, bySurface: { editor: binding('workspace.fallback') }, bySurfaceId, fallback: binding('workspace.fallback') }],
});

describe('commandLocalKeyboard por surface.id', () => {
  afterEach(() => vi.restoreAllMocks());

  it('escolhe o ID exato e usa bySurface/fallback para ID desconhecido', async () => {
    let surfaceId = 'editor-a';
    const exact = binding('workspace.editor_a');
    const fallback = binding('workspace.fallback');
    const map = baseMap({ editor: { 'editor-a': exact } });
    map.contextualBindings![0].fallback = fallback;
    const onDown = vi.fn(async () => {});
    const controller = createLocalCommandKeyboard({ target: window, loadMap: async () => map, onDown,
      onUp: async () => {}, reset: async () => {}, blocked: () => false, readSurfaceType: () => 'editor',
      readContext: () => ({ surfaceId, surfaceType: 'editor', isCurrent: () => true }) });
    try {
      await controller.refresh();
      event('keydown', 'KeyI', { ctrlKey: true });
      expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ commandId: exact.commandId,
        context: { surfaceId: 'editor-a', surfaceType: 'editor' } }));
      event('keyup', 'KeyI');
      surfaceId = 'editor-unknown';
      event('keydown', 'KeyI', { ctrlKey: true });
      expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ commandId: fallback.commandId,
        context: { surfaceId: 'editor-unknown', surfaceType: 'editor' } }));
    } finally { controller.dispose(); }
  });

  it('trata null exato como supressão e exige lease válida quando há branches por ID', async () => {
    let current = true;
    const map = baseMap({ editor: { 'editor-blocked': null } });
    const onDown = vi.fn(async () => {});
    const controller = createLocalCommandKeyboard({ target: window, loadMap: async () => map, onDown,
      onUp: async () => {}, reset: async () => {}, blocked: () => false, readSurfaceType: () => 'editor',
      readContext: () => ({ surfaceId: 'editor-blocked', surfaceType: 'editor', isCurrent: () => current }) });
    try {
      await controller.refresh();
      expect(event('keydown', 'KeyI', { ctrlKey: true }).defaultPrevented).toBe(true);
      expect(onDown).not.toHaveBeenCalled();
      current = false;
      expect(event('keydown', 'KeyI', { ctrlKey: true }).defaultPrevented).toBe(true);
      expect(onDown).not.toHaveBeenCalled();
    } finally { controller.dispose(); }

    const noLeaseDown = vi.fn(async () => {});
    const noLease = createLocalCommandKeyboard({ target: window, loadMap: async () => map, onDown: noLeaseDown,
      onUp: async () => {}, reset: async () => {}, blocked: () => false, readSurfaceType: () => 'editor' });
    try {
      await noLease.refresh();
      expect(event('keydown', 'KeyI', { ctrlKey: true }).defaultPrevented).toBe(true);
      expect(noLeaseDown).not.toHaveBeenCalled();
    } finally { noLease.dispose(); }
  });

  it('seleciona IDs distintos também em sequências v2', async () => {
    let surfaceId = 'editor-a';
    const first: LocalCommandKeyboardBinding = { shortcut: { version: 2, steps: [step, { code: 'KeyA', modifiers: [] }] }, commandId: 'workspace.editor_a', handler: 'backend' };
    const fallback: LocalCommandKeyboardBinding = { shortcut: { version: 2, steps: [step, { code: 'KeyA', modifiers: [] }] }, commandId: 'workspace.fallback', handler: 'backend' };
    const map: LocalCommandKeyboardMap = { generation: 'surface-sequence', bindings: [], contextualBindings: [{
      shortcut: first.shortcut, bySurface: { editor: fallback }, bySurfaceId: { editor: { 'editor-a': first } }, fallback,
    }] };
    const onDown = vi.fn(async () => {});
    const controller = createLocalCommandKeyboard({ target: window, loadMap: async () => map, onDown,
      onUp: async () => {}, reset: async () => {}, blocked: () => false, readSurfaceType: () => 'editor',
      readContext: () => ({ surfaceId, surfaceType: 'editor', isCurrent: () => true }) });
    const press = () => { event('keydown', 'KeyI', { ctrlKey: true }); event('keyup', 'KeyI'); event('keydown', 'KeyA'); event('keyup', 'KeyA'); };
    try {
      await controller.refresh();
      press();
      expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ commandId: first.commandId }));
      surfaceId = 'editor-other';
      press();
      expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ commandId: fallback.commandId }));
    } finally { controller.dispose(); }
  });

  it('permite sequência flat para ID não listado somente com sequenceFallbacks', async () => {
    let surfaceId = 'chat-A';
    const v1 = binding('workspace.chat_a');
    const sequence: LocalCommandKeyboardBinding = {
      shortcut: { version: 2, steps: [step, { code: 'KeyL', modifiers: [] }] },
      commandId: 'workspace.chat_sequence', handler: 'backend',
    };
    const map: LocalCommandKeyboardMap = {
      generation: 'sequence-fallback',
      bindings: [sequence],
      contextualBindings: [{
        shortcut: prefix,
        bySurface: { chat: null },
        bySurfaceId: { chat: { 'chat-A': v1 } },
        fallback: null,
        sequenceFallbacks: { chat: { '': true } },
      }],
    };
    const onDown = vi.fn(async () => {});
    const invalid = vi.fn();
    const accepted = vi.fn();
    const controller = createLocalCommandKeyboard({ target: window, loadMap: async () => map, onDown,
      onUp: async () => {}, reset: async () => {}, blocked: () => false, readSurfaceType: () => 'chat',
      readContext: () => ({ surfaceId, surfaceType: 'chat', isCurrent: () => true }), onMapInvalidated: invalid, onMapAccepted: accepted });
    const pressSequence = () => {
      event('keydown', 'KeyI', { ctrlKey: true });
      event('keyup', 'KeyI');
      event('keydown', 'KeyL');
      event('keyup', 'KeyL');
    };
    try {
      await controller.refresh();
      expect(accepted).toHaveBeenCalledOnce();
      event('keydown', 'KeyI', { ctrlKey: true });
      expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ commandId: v1.commandId }));
      event('keyup', 'KeyI');
      surfaceId = 'chat-B';
      pressSequence();
      expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ commandId: sequence.commandId }));
    } finally { controller.dispose(); }
  });

  it('permite flag de ID exato sem exigir ausência do fallback por tipo', async () => {
    const sequence: LocalCommandKeyboardBinding = {
      shortcut: { version: 2, steps: [step, { code: 'KeyL', modifiers: [] }] },
      commandId: 'workspace.chat_sequence', handler: 'backend',
    };
    const map: LocalCommandKeyboardMap = {
      generation: 'sequence-exact-fallback', bindings: [sequence], contextualBindings: [{
        shortcut: prefix, bySurface: { chat: binding('workspace.chat_default') },
        bySurfaceId: { chat: { 'chat-B': null } }, fallback: null,
        sequenceFallbacks: { chat: { 'chat-B': true } },
      }],
    };
    const onDown = vi.fn(async () => {});
    const controller = createLocalCommandKeyboard({ target: window, loadMap: async () => map, onDown,
      onUp: async () => {}, reset: async () => {}, blocked: () => false, readSurfaceType: () => 'chat',
      readContext: () => ({ surfaceId: 'chat-B', surfaceType: 'chat', isCurrent: () => true }) });
    try {
      await controller.refresh();
      event('keydown', 'KeyI', { ctrlKey: true });
      event('keyup', 'KeyI');
      event('keydown', 'KeyL');
      expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ commandId: sequence.commandId }));
    } finally { controller.dispose(); }
  });

  it('mantém null como barreira quando o ID desconhecido não tem flag', async () => {
    const sequence: LocalCommandKeyboardBinding = {
      shortcut: { version: 2, steps: [step, { code: 'KeyL', modifiers: [] }] },
      commandId: 'workspace.chat_sequence', handler: 'backend',
    };
    const map: LocalCommandKeyboardMap = {
      generation: 'sequence-barrier', bindings: [sequence], contextualBindings: [{
        shortcut: prefix, bySurface: { chat: null }, bySurfaceId: { chat: { 'chat-A': binding('workspace.chat_a') } }, fallback: null,
      }],
    };
    const onDown = vi.fn(async () => {});
    const controller = createLocalCommandKeyboard({ target: window, loadMap: async () => map, onDown,
      onUp: async () => {}, reset: async () => {}, blocked: () => false, readSurfaceType: () => 'chat',
      readContext: () => ({ surfaceId: 'chat-B', surfaceType: 'chat', isCurrent: () => true }) });
    try {
      await controller.refresh();
      expect(event('keydown', 'KeyI', { ctrlKey: true }).defaultPrevented).toBe(true);
      event('keyup', 'KeyI');
      event('keydown', 'KeyL');
      expect(onDown).not.toHaveBeenCalled();
    } finally { controller.dispose(); }
  });

  it('permite fallback de sequência por tipo sem branches por ID', async () => {
    const sequence: LocalCommandKeyboardBinding = {
      shortcut: { version: 2, steps: [step, { code: 'KeyL', modifiers: [] }] },
      commandId: 'workspace.chat_sequence', handler: 'backend',
    };
    const map: LocalCommandKeyboardMap = {
      generation: 'sequence-type-fallback', bindings: [sequence], contextualBindings: [{
        shortcut: prefix, bySurface: { chat: null }, fallback: null,
        sequenceFallbacks: { chat: { '': true } },
      }],
    };
    const onDown = vi.fn(async () => {});
    const controller = createLocalCommandKeyboard({ target: window, loadMap: async () => map, onDown,
      onUp: async () => {}, reset: async () => {}, blocked: () => false, readSurfaceType: () => 'chat',
      readContext: () => ({ surfaceId: 'chat-B', surfaceType: 'chat', isCurrent: () => true }) });
    try {
      await controller.refresh();
      event('keydown', 'KeyI', { ctrlKey: true });
      event('keyup', 'KeyI');
      event('keydown', 'KeyL');
      event('keyup', 'KeyL');
      expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ commandId: sequence.commandId }));
    } finally { controller.dispose(); }
  });

  const unsafeSurfaceType = Object.create(null) as Record<string, Record<string, LocalCommandKeyboardBinding | null>>;
  Object.defineProperty(unsafeSurfaceType, '__proto__', { value: { editor: binding('workspace.bad') }, enumerable: true });
  it.each([
    { name: 'nested array', bySurfaceId: { editor: [] } },
    { name: 'unsafe type key', bySurfaceId: unsafeSurfaceType },
    { name: 'invalid branch', bySurfaceId: { editor: { 'editor-a': { commandId: 'bad', handler: 'backend', shortcut: prefix } } } },
    { name: 'flag false', bySurfaceId: { editor: { 'editor-a': null } }, sequenceFallbacks: { editor: { 'editor-a': false } } },
    { name: 'flag without null branch', bySurfaceId: { editor: { 'editor-a': binding('workspace.branch') } }, sequenceFallbacks: { editor: { 'editor-a': true } } },
    { name: 'flag type without bySurface null', bySurfaceId: { editor: { 'editor-a': null } }, sequenceFallbacks: { editor: { '': true } }, bySurface: { editor: binding('workspace.branch') } },
  ])('recusa DTO malformed de bySurfaceId: $name', async ({ bySurfaceId, sequenceFallbacks, bySurface }) => {
    const invalid = vi.fn();
    const map = baseMap(bySurfaceId as never);
    if (sequenceFallbacks !== undefined) map.contextualBindings![0].sequenceFallbacks = sequenceFallbacks as never;
    if (bySurface !== undefined) map.contextualBindings![0].bySurface = bySurface as never;
    const controller = createLocalCommandKeyboard({ target: window, loadMap: async () => map, onDown: async () => {},
      onUp: async () => {}, reset: async () => {}, blocked: () => false, onMapInvalidated: invalid });
    try { await controller.refresh(); expect(invalid).toHaveBeenCalled(); } finally { controller.dispose(); }
  });
});
