import { describe, expect, it, vi } from 'vitest';
import { createCommandLocalKeyboardWailsPort } from './commandLocalKeyboardWails';
import type { CommandShortcutSequence } from './commandShortcut';

vi.mock('./waitForWailsBridge', () => ({ waitForWailsBridge: vi.fn(async () => undefined) }));

describe('commandLocalKeyboardWails', () => {
  it('contexto usa somente o ingresso escopado e não serializa a guarda local', async () => {
    const app = {
      GetLocalCommandKeyboardMap: vi.fn(async () => ({ generation: 'g', bindings: [] })),
      DispatchLocalCommandKey: vi.fn(async () => null), BeginLocalCommandUIKey: vi.fn(async () => null),
      DispatchContextualLocalCommandKey: vi.fn(async () => null), BeginContextualLocalCommandUIKey: vi.fn(async () => null),
      ResetLocalCommandKeyboard: vi.fn(async () => undefined),
    };
    const port = createCommandLocalKeyboardWailsPort({ target: { go: { app: { App: app } } } as unknown as Window });
    const shortcut = { version: 1 as const, code: 'KeyY', modifiers: ['Control' as const] };
    const context = { surfaceId: 'tab', surfaceType: 'chat', isCurrent: () => true };
    await port.dispatchLocalCommandKey('g', shortcut, 'down', false, context);
    await port.beginLocalCommandUIKey('g', shortcut, false, context);
    expect(app.DispatchContextualLocalCommandKey).toHaveBeenCalledExactlyOnceWith('g', shortcut, 'down', false, { surfaceId: 'tab', surfaceType: 'chat' });
    expect(app.BeginContextualLocalCommandUIKey).toHaveBeenCalledExactlyOnceWith('g', shortcut, false, { surfaceId: 'tab', surfaceType: 'chat' });
    expect(app.DispatchLocalCommandKey).not.toHaveBeenCalled();
    expect(app.BeginLocalCommandUIKey).not.toHaveBeenCalled();
  });

  it('recusa contexto que envelheceu durante espera pela ponte sem fallback', async () => {
    const app = {
      GetLocalCommandKeyboardMap: vi.fn(async () => ({ generation: 'g', bindings: [] })),
      DispatchLocalCommandKey: vi.fn(async () => null), BeginLocalCommandUIKey: vi.fn(async () => null),
      DispatchContextualLocalCommandKey: vi.fn(async () => null), BeginContextualLocalCommandUIKey: vi.fn(async () => null),
      ResetLocalCommandKeyboard: vi.fn(async () => undefined),
    };
    const port = createCommandLocalKeyboardWailsPort({ target: { go: { app: { App: app } } } as unknown as Window });
    const shortcut = { version: 1 as const, code: 'KeyY', modifiers: ['Control' as const] };
    let current = true;
    const context = { surfaceId: 'tab', surfaceType: 'chat', isCurrent: () => current };
    const dispatch = port.dispatchLocalCommandKey('g', shortcut, 'down', false, context);
    current = false;
    await expect(dispatch).rejects.toThrow('context stale');
    await expect(port.beginLocalCommandUIKey('g', shortcut, false, context)).rejects.toThrow('context stale');
    expect(app.DispatchContextualLocalCommandKey).not.toHaveBeenCalled();
    expect(app.BeginContextualLocalCommandUIKey).not.toHaveBeenCalled();
    expect(app.DispatchLocalCommandKey).not.toHaveBeenCalled();
  });

  it('API contextual ausente não tenta o ingresso sem contexto', async () => {
    const app = {
      GetLocalCommandKeyboardMap: vi.fn(async () => ({ generation: 'g', bindings: [] })),
      DispatchLocalCommandKey: vi.fn(async () => null), BeginLocalCommandUIKey: vi.fn(async () => null),
      ResetLocalCommandKeyboard: vi.fn(async () => undefined),
    };
    const port = createCommandLocalKeyboardWailsPort({ target: { go: { app: { App: app } } } as unknown as Window });
    const shortcut = { version: 1 as const, code: 'KeyY', modifiers: ['Control' as const] };
    await expect(port.dispatchLocalCommandKey('g', shortcut, 'down', false, { surfaceId: 'tab', surfaceType: 'chat' })).rejects.toThrow('unavailable');
    expect(app.DispatchLocalCommandKey).not.toHaveBeenCalled();
  });

  it('adapta o mapa, dispatch down/up e reset', async () => {
    const app = {
      GetLocalCommandKeyboardMap: vi.fn(async () => ({ generation: 'g1', bindings: [] })),
      DispatchLocalCommandKey: vi.fn(async () => null),
      BeginLocalCommandUIKey: vi.fn(async () => null),
      ResetLocalCommandKeyboard: vi.fn(async () => undefined),
    };
    const port = createCommandLocalKeyboardWailsPort({ target: { go: { app: { App: app } } } as unknown as Window });
    const shortcut = { version: 1 as const, code: 'KeyK', modifiers: ['Control' as const] };
    await expect(port.loadMap()).resolves.toEqual({ generation: 'g1', bindings: [] });
    await port.dispatchLocalCommandKey('g1', shortcut, 'down', false);
    await port.dispatchLocalCommandKey('g1', shortcut, 'up', true);
    const sequence: CommandShortcutSequence = { version: 2, steps: [{ code: 'KeyK', modifiers: ['Control'] }, { code: 'KeyN', modifiers: [] }] };
    await port.dispatchLocalCommandKey('g1', sequence, 'down', false);
    await port.dispatchLocalCommandKey('g1', sequence, 'up', false);
    await port.beginLocalCommandUIKey('g1', shortcut, false);
    await port.resetLocalCommandKeyboard('g1');
    expect(app.DispatchLocalCommandKey).toHaveBeenNthCalledWith(1, 'g1', shortcut, 'down', false);
    expect(app.DispatchLocalCommandKey).toHaveBeenNthCalledWith(2, 'g1', shortcut, 'up', true);
    expect(app.DispatchLocalCommandKey).toHaveBeenNthCalledWith(3, 'g1', sequence, 'down', false);
    expect(app.DispatchLocalCommandKey).toHaveBeenNthCalledWith(4, 'g1', sequence, 'up', false);
    expect(app.BeginLocalCommandUIKey).toHaveBeenCalledWith('g1', shortcut, false);
    expect(app.ResetLocalCommandKeyboard).toHaveBeenCalledWith('g1');
  });

  it('clona profundamente o mapa contextual, incluindo bySurfaceId, sem roundtrip JSON', async () => {
    const source = {
      generation: 'deep',
      bindings: [{ shortcut: { version: 1 as const, code: 'KeyK', modifiers: ['Control' as const] }, commandId: 'workspace.open', handler: 'backend' as const }],
      contextualBindings: [{
        shortcut: { version: 1 as const, code: 'KeyY', modifiers: ['Control' as const] },
        bySurface: { editor: { shortcut: { version: 1 as const, code: 'KeyY', modifiers: ['Control' as const] }, commandId: 'workspace.editor', handler: 'backend' as const } },
        bySurfaceId: { editor: { 'editor-1': null } },
        sequenceFallbacks: { editor: { '': true } },
        fallback: null,
      }],
      localPaletteCommands: ['workspace.open'],
      localPaletteConditions: [{
        commandId: 'workspace.open',
        bySurface: { chat: true },
        bySurfaceId: { chat: { 'tab-1': false } },
        byProfile: { focused: { commandId: 'workspace.open', bySurface: { chat: false }, fallback: true } },
        fallback: false,
      }],
    };
    const app = {
      GetLocalCommandKeyboardMap: vi.fn(async () => source),
      DispatchLocalCommandKey: vi.fn(async () => null), BeginLocalCommandUIKey: vi.fn(async () => null),
      ResetLocalCommandKeyboard: vi.fn(async () => undefined),
    };
    const port = createCommandLocalKeyboardWailsPort({ target: { go: { app: { App: app } } } as unknown as Window });
    const map = await port.loadMap();
    map.bindings[0].shortcut.version === 1 && map.bindings[0].shortcut.modifiers.push('Alt');
    map.contextualBindings![0].bySurfaceId!.editor['editor-1'] = {
      shortcut: { version: 1, code: 'KeyY', modifiers: ['Control'] }, commandId: 'workspace.changed', handler: 'backend',
    };
    map.localPaletteCommands!.push('workspace.changed');
    map.localPaletteConditions![0].bySurface.chat = false;
    map.localPaletteConditions![0].bySurfaceId!.chat['tab-1'] = true;
    map.localPaletteConditions![0].byProfile!.focused.bySurface.chat = true;
    expect(source.bindings[0].shortcut.modifiers).toEqual(['Control']);
    expect(source.contextualBindings![0].bySurfaceId.editor['editor-1']).toBeNull();
    expect(source.contextualBindings![0].sequenceFallbacks).toEqual({ editor: { '': true } });
    expect(source.localPaletteCommands).toEqual(['workspace.open']);
    expect(source.localPaletteConditions).toEqual([{
      commandId: 'workspace.open',
      bySurface: { chat: true },
      bySurfaceId: { chat: { 'tab-1': false } },
      byProfile: { focused: { commandId: 'workspace.open', bySurface: { chat: false }, fallback: true } },
      fallback: false,
    }]);
  });

  it('falha fechado quando os bindings não estão disponíveis', async () => {
    const port = createCommandLocalKeyboardWailsPort({ target: { go: { app: { App: {} } } } as unknown as Window });
    await expect(port.loadMap()).rejects.toThrow('not available');
  });
});
