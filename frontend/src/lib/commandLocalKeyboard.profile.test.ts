import { describe, expect, it } from 'vitest';
import { createLocalCommandKeyboard, resolveLocalCommandContextualBinding, type LocalCommandContextualBinding, type LocalCommandKeyboardMap } from './commandLocalKeyboard';

const shortcut = { version: 1 as const, code: 'KeyP', modifiers: ['Control' as const] };
const leaf = (commandId: string, surface: 'chat' | 'editor' | 'terminal' | 'tasklist' = 'chat') => ({
  shortcut, bySurface: { [surface]: { shortcut, commandId, handler: 'backend' as const } }, fallback: null,
});
const entry = (byProfile?: Record<string, LocalCommandContextualBinding>): LocalCommandContextualBinding => ({
  shortcut, bySurface: { chat: null }, fallback: null, byProfile,
});

describe('commandLocalKeyboard por perfil', () => {
  it('seleciona A, B e a raiz para perfil não listado sem fallback entre perfis', () => {
    const contextual = entry({ A: leaf('workspace.a'), B: leaf('workspace.b') });
    expect(resolveLocalCommandContextualBinding(contextual, 'chat', { surfaceId: 'tab', surfaceType: 'chat', profile: 'A' }).branch?.commandId).toBe('workspace.a');
    expect(resolveLocalCommandContextualBinding(contextual, 'chat', { surfaceId: 'tab', surfaceType: 'chat', profile: 'B' }).branch?.commandId).toBe('workspace.b');
    expect(resolveLocalCommandContextualBinding(contextual, 'chat', { surfaceId: 'tab', surfaceType: 'chat', profile: 'C' }).branch).toBeNull();
  });

  it('recusa ausência de perfil, superfície fora do workspace e projeção aninhada', () => {
    const contextual = entry({ A: leaf('workspace.a') });
    expect(resolveLocalCommandContextualBinding(contextual, 'chat').barrier).toBe(true);
    expect(resolveLocalCommandContextualBinding(contextual, 'history', { surfaceId: 'x', surfaceType: 'history', profile: 'A' }).barrier).toBe(true);
  });

  it('preserva a raiz parcial como projeção válida', () => {
    const contextual = entry({ A: leaf('workspace.a') });
    expect(resolveLocalCommandContextualBinding(contextual, 'chat', { surfaceId: 'tab', surfaceType: 'chat', profile: 'A' }).requiresLease).toBe(true);
  });

  it('recusa leaf de perfil com superfície fora do workspace', async () => {
    let invalidated = 0;
    const malformed = {
      generation: 'malformed-profile-leaf', bindings: [],
      contextualBindings: [{
        shortcut, bySurface: { chat: null }, fallback: null,
        byProfile: { A: { shortcut, bySurface: { settings: null }, fallback: null } },
      }],
    } satisfies LocalCommandKeyboardMap;
    const controller = createLocalCommandKeyboard({
      target: window,
      loadMap: async () => malformed,
      onDown: async () => undefined,
      onUp: async () => undefined,
      reset: async () => undefined,
      blocked: () => false,
      onMapInvalidated: () => { invalidated += 1; },
    });
    await controller.refresh();
    controller.dispose();
    expect(invalidated).toBeGreaterThan(0);
  });
});
