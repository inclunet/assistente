import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { LocalCommandKeyboardBinding, LocalCommandKeyboardMap } from './commandLocalKeyboard';
import { commandSequencePrefixHint, commandShortcutHints, publishCommandShortcutHints, useCommandShortcutHint } from './commandShortcutHints';
import type { CommandShortcut } from './commandShortcut';

const identity = vi.hoisted(() => ({ userId: 'owner', sessionId: 'session', workspaceId: 'workspace', authenticated: true, profile: 'dev', activeTabId: 'chat-a', override: '' }));
vi.mock('../store/authStore', () => ({ useAuthStore: (selector: (state: unknown) => unknown) => selector({
  isAuthenticated: identity.authenticated, user: { userId: identity.userId, sessionId: identity.sessionId },
}) }));
vi.mock('../store/workspaceStore', () => {
  const state = () => ({ workspace: { id: identity.workspaceId, profile: identity.profile, activeTabId: identity.activeTabId,
    tabs: [{ id: identity.activeTabId, type: 'chat', profileOverride: { slug: identity.override } }] } });
  return { useWorkspaceStore: Object.assign((selector: (value: unknown) => unknown) => selector(state()), { getState: state }) };
});

function binding(commandId: string, code = 'KeyK'): LocalCommandKeyboardBinding {
  return { commandId, handler: 'local_ui', shortcut: { version: 1, code, modifiers: ['Control'] } };
}
function map(bindings = [binding('navigation.palette.open')]): LocalCommandKeyboardMap {
  return { generation: 'g1', ownerId: 'owner', sessionId: 'session', workspaceId: 'workspace', bindings };
}
afterEach(() => {
  cleanup();
  publishCommandShortcutHints(null);
  Object.assign(identity, { userId: 'owner', sessionId: 'session', workspaceId: 'workspace', authenticated: true, profile: 'dev', activeTabId: 'chat-a', override: '' });
});

describe('commandShortcutHints', () => {
  it.each([
    ['toolbar', 'settings', 'command_settings.create.open'],
    ['profiles', 'profiles', 'profiles.create.open'],
    ['tasklists', 'tasklists', 'tasklists.create.open'],
    ['chat', 'workspace', 'workspace.tab.chat.create'],
  ] as const)('hook mantém dicas por página em %s sem fallback hardcoded', (surface, page, id) => {
    const chosen = binding(id, 'KeyN');
    const projection = map([]);
    projection.contextualBindings = [{ shortcut: chosen.shortcut, bySurface: {}, fallback: null,
      byPage: { [page]: { shortcut: chosen.shortcut, bySurface: { [surface]: chosen }, fallback: null } },
    }];
    publishCommandShortcutHints(projection);
    const { result } = renderHook(() => useCommandShortcutHint(id, surface, surface === 'toolbar' ? 'settings' : undefined));
    expect(result.current).toBe('Ctrl+N');
    projection.contextualBindings[0].byPage![page].bySurface[surface] = null;
    act(() => publishCommandShortcutHints(projection));
    expect(result.current).toBeUndefined();
  });
  it('compõe perfil e identidade da aba sem anunciar fallback quando falta contexto', () => {
    const projection = map([]);
    projection.contextualBindings = [{ shortcut: binding('').shortcut,
      bySurface: { chat: binding('navigation.palette.open') }, fallback: null,
      byProfile: { dev: { shortcut: binding('').shortcut, bySurface: { chat: null }, fallback: null,
        bySurfaceId: { chat: { 'chat-a': binding('navigation.settings.open'), 'chat-blocked': null } } } },
    }];
    const hints = (profile: string, surfaceId: string) => commandShortcutHints(projection, 'chat', { profile, surfaceId, surfaceType: 'chat' });
    expect(hints('dev', 'chat-a').get('navigation.settings.open')).toBe('Ctrl+K');
    expect(hints('dev', 'chat-blocked').size).toBe(0);
    expect(hints('dev', 'chat-other').size).toBe(0);
    expect(hints('other', 'chat-a').get('navigation.palette.open')).toBe('Ctrl+K');
    expect(commandShortcutHints(projection, 'chat').size).toBe(0);
  });

  it('anuncia sequência independente somente no NoMatch explícito do perfil e aba', () => {
    const sequence: LocalCommandKeyboardBinding = { commandId: 'workspace.tab.chat.create', handler: 'contextual',
      shortcut: { version: 2, steps: [{ code: 'KeyN', modifiers: ['Control'] }, { code: 'KeyC', modifiers: [] }] } };
    const projection = map([sequence]);
    const shortcut = binding('', 'KeyN').shortcut;
    projection.contextualBindings = [{ shortcut, bySurface: { chat: null }, fallback: null, byProfile: {
      dev: { shortcut, bySurface: { chat: null }, fallback: null,
        bySurfaceId: { chat: { 'chat-blocked': null } }, sequenceFallbacks: { chat: { '': true } } },
    } }];
    const context = { profile: 'dev', surfaceId: 'chat-a', surfaceType: 'chat' };
    expect(commandShortcutHints(projection, 'chat', context).get(sequence.commandId)).toBe('Ctrl+N C');
    expect(commandSequencePrefixHint(projection, [sequence.commandId], 'chat', context)).toBe('Ctrl+N');
    const blocked = { ...context, surfaceId: 'chat-blocked' };
    expect(commandShortcutHints(projection, 'chat', blocked).size).toBe(0);
    expect(commandSequencePrefixHint(projection, [sequence.commandId], 'chat', blocked)).toBeUndefined();
  });

  it('hook acompanha perfil e override sem republicar mapa', () => {
    const projection = map([]);
    projection.contextualBindings = [{ shortcut: binding('').shortcut, bySurface: { chat: null }, fallback: null,
      byProfile: { dev: { shortcut: binding('').shortcut, bySurface: { chat: binding('navigation.palette.open') }, fallback: null } },
    }];
    publishCommandShortcutHints(projection);
    const { result, rerender } = renderHook(() => useCommandShortcutHint('navigation.palette.open', 'chat'));
    expect(result.current).toBe('Ctrl+K');
    identity.profile = 'other';
    rerender();
    expect(result.current).toBeUndefined();
    identity.override = 'dev';
    rerender();
    expect(result.current).toBe('Ctrl+K');
  });

  it('não anuncia sequências quando o prefixo está ocupado ou bloqueado na página', () => {
    const sequence: LocalCommandKeyboardBinding = { ...binding('workspace.tab.chat.create'), shortcut: {
      version: 2, steps: [{ code: 'KeyN', modifiers: ['Control'] }, { code: 'KeyC', modifiers: [] }],
    } };
    const projection = map([sequence]);
    projection.contextualBindings = [{ shortcut: { version: 1, code: 'KeyN', modifiers: ['Control'] },
      bySurface: { tasklists: binding('tasklists.create.open', 'KeyN'), profiles: null }, fallback: null, fallbackToSequences: true,
    }];
    for (const surface of ['tasklists', 'profiles', undefined]) {
      expect(commandShortcutHints(projection, surface).has('workspace.tab.chat.create')).toBe(false);
      expect(commandSequencePrefixHint(projection, [sequence.commandId], surface)).toBeUndefined();
    }
    expect(commandShortcutHints(projection, 'tasklists').get('tasklists.create.open')).toBe('Ctrl+N');
    expect(commandShortcutHints(projection, 'chat').get(sequence.commandId)).toBe('Ctrl+N C');
    expect(commandSequencePrefixHint(projection, [sequence.commandId], 'chat')).toBe('Ctrl+N');
    projection.contextualBindings[0].fallbackToSequences = false;
    expect(commandShortcutHints(projection, 'chat').has(sequence.commandId)).toBe(false);
  });
  it('anuncia somente prefixos de sequências da família, não atalhos diretos ou de outra ação', () => {
    const sequence: LocalCommandKeyboardBinding = { ...binding('workspace.tab.chat.create'), shortcut: {
      version: 2, steps: [{ code: 'KeyB', modifiers: ['Control'] }, { code: 'KeyC', modifiers: [] }],
    } };
    const ids = ['workspace.tab.chat.create'];
    expect(commandSequencePrefixHint(map([sequence, sequence, binding(ids[0], 'KeyT')]), ids)).toBe('Ctrl+B');
    expect(commandSequencePrefixHint(map([sequence]), ['other.command'])).toBeUndefined();
    expect(commandSequencePrefixHint(map([binding(ids[0])]), ids)).toBeUndefined();
    expect(commandSequencePrefixHint(null, ids)).toBeUndefined();
  });
  it('não inventa defaults sem projeção nem quando o comando é suprimido', () => {
    expect(commandShortcutHints(null).size).toBe(0);
    expect(commandShortcutHints(map([])).get('navigation.palette.open')).toBeUndefined();
  });
  it('mostra remapeamentos, múltiplos bindings e sequências sem duplicar', () => {
    const sequence: LocalCommandKeyboardBinding = { ...binding('workspace.tab.chat.create'), shortcut: {
      version: 2, steps: [{ code: 'KeyN', modifiers: ['Control'] }, { code: 'KeyC', modifiers: [] }],
    } };
    const result = commandShortcutHints(map([binding('navigation.palette.open', 'KeyP'), binding('navigation.palette.open', 'KeyP'), binding('navigation.palette.open', 'F2'), sequence]));
    expect(result.get('navigation.palette.open')).toBe('Ctrl+P, Ctrl+F2');
    expect(result.get('workspace.tab.chat.create')).toBe('Ctrl+N C');
  });
  it('respeita resolução contextual, barreira null e superfície desconhecida', () => {
    const projection = map([binding('navigation.palette.open')]);
    projection.contextualBindings = [{ shortcut: binding('').shortcut as import('./commandShortcut').CommandShortcut,
      bySurface: { editor: binding('editor.menu.insert.open'), chat: null }, fallback: binding('navigation.palette.open'),
    }];
    expect(commandShortcutHints(projection, 'editor').get('editor.menu.insert.open')).toBe('Ctrl+K');
    expect(commandShortcutHints(projection, 'editor').has('navigation.palette.open')).toBe(false);
    expect(commandShortcutHints(projection, 'chat').size).toBe(0);
    expect(commandShortcutHints(projection).size).toBe(0);
    expect(commandShortcutHints(projection, 'toolbar').get('navigation.palette.open')).toBe('Ctrl+K');
  });
  it('não anuncia v2 quando v1 seleciona ou bloqueia o prefixo, mas anuncia no-match com fallback', () => {
    const prefix: CommandShortcut = { version: 1, code: 'KeyN', modifiers: ['Control'] };
    const contextualSequence: LocalCommandKeyboardBinding = {
      shortcut: { version: 2, steps: [{ code: 'KeyN', modifiers: ['Control'] }, { code: 'KeyC', modifiers: [] }] },
      commandId: 'workspace.list', handler: 'backend',
    };
    const projection = map([]);
    projection.contextualBindings = [
      { shortcut: prefix, bySurface: { editor: binding('tasklists.create.open', 'KeyN') }, fallback: null, fallbackToSequences: true },
      { shortcut: contextualSequence.shortcut, bySurface: { editor: contextualSequence }, fallback: contextualSequence },
    ];
    const contextualBindings = projection.contextualBindings!;
    expect(commandShortcutHints(projection, 'editor').has(contextualSequence.commandId)).toBe(false);
    contextualBindings[0].bySurface.editor = null;
    expect(commandShortcutHints(projection, 'editor').has(contextualSequence.commandId)).toBe(false);
    contextualBindings[0].bySurface = {};
    expect(commandShortcutHints(projection, 'chat').get(contextualSequence.commandId)).toBe('Ctrl+N C');
    expect(commandSequencePrefixHint(projection, [contextualSequence.commandId], 'chat')).toBe('Ctrl+N');
  });
  it('acompanha publicação, alteração e invalidação sem IPC próprio', () => {
    const { result } = renderHook(() => useCommandShortcutHint('navigation.palette.open'));
    expect(result.current).toBeUndefined();
    act(() => publishCommandShortcutHints(map()));
    expect(result.current).toBe('Ctrl+K');
    act(() => publishCommandShortcutHints(map([binding('navigation.palette.open', 'KeyP')])));
    expect(result.current).toBe('Ctrl+P');
    act(() => publishCommandShortcutHints(null));
    expect(result.current).toBeUndefined();
  });
  it.each(['userId', 'sessionId', 'workspaceId'] as const)('não exibe projeção de outro %s', field => {
    publishCommandShortcutHints(map());
    const { result, rerender } = renderHook(() => useCommandShortcutHint('navigation.palette.open'));
    expect(result.current).toBe('Ctrl+K');
    identity[field] = 'other';
    rerender();
    expect(result.current).toBeUndefined();
  });
  it('não exibe projeção após logout', () => {
    publishCommandShortcutHints(map());
    identity.authenticated = false;
    expect(renderHook(() => useCommandShortcutHint('navigation.palette.open')).result.current).toBeUndefined();
  });
  it('não mantém referência mutável ao mapa recebido', () => {
    const projection = map();
    publishCommandShortcutHints(projection);
    projection.bindings.length = 0;
    expect(renderHook(() => useCommandShortcutHint('navigation.palette.open')).result.current).toBe('Ctrl+K');
  });
});
