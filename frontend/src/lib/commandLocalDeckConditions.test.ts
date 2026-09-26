import { describe, expect, it } from 'vitest';
import { resolveLocalDeckConditionCommand, resolveLocalDeckConditionSelectionFromParsed } from './commandLocalDeckConditions';

const context = (overrides: Record<string, unknown> = {}) => ({
  surfaceType: 'chat', surfaceId: 'tab-a', ...overrides,
});

describe('local Deck conditions', () => {
  it('selects one LOCAL_UI command for the trusted visual context', () => {
    expect(resolveLocalDeckConditionCommand([
      { commandId: 'navigation.palette.open', bySurface: { chat: true }, fallback: false },
      { commandId: 'navigation.menu.open', bySurface: { chat: false }, fallback: false },
    ], context())).toBe('navigation.palette.open');
  });

  it('rejects missing provider/profile, mismatch and ambiguous positives', () => {
    const conditions = [{
      commandId: 'navigation.palette.open', bySurface: { chat: true },
      byProfile: { focused: { commandId: 'navigation.palette.open', bySurface: { chat: true }, fallback: false } },
      fallback: false,
    }];
    expect(resolveLocalDeckConditionCommand(conditions, undefined)).toBeNull();
    expect(resolveLocalDeckConditionCommand(conditions, context({ profile: 'other' }))).toBe('navigation.palette.open');
    expect(resolveLocalDeckConditionCommand(conditions, context({ profile: 'focused', surfaceType: 'editor' }))).toBeNull();
    expect(resolveLocalDeckConditionCommand([
      { commandId: 'navigation.palette.open', bySurface: { chat: true }, fallback: false },
      { commandId: 'navigation.menu.open', bySurface: { chat: true }, fallback: false },
    ], context())).toBeNull();
  });

  it('fails closed for malformed, non-LOCAL_UI and duplicate conditions', () => {
    expect(resolveLocalDeckConditionCommand([{ commandId: 'workspace.open', bySurface: { chat: true }, fallback: false }], context())).toBeNull();
    expect(resolveLocalDeckConditionCommand([{ commandId: 'navigation.palette.open', bySurface: { chat: 'yes' }, fallback: false }], context())).toBeNull();
    expect(resolveLocalDeckConditionCommand([
      { commandId: 'navigation.palette.open', bySurface: { chat: true }, fallback: false },
      { commandId: 'navigation.palette.open', bySurface: { chat: true }, fallback: false },
    ], context())).toBeNull();
  });

  it('requires a known profile and preserves the exact surface-id override', () => {
    const commandId = 'navigation.menu.open';
    const conditions = [{ commandId, bySurface: { chat: false }, fallback: false,
      byProfile: { dev: { commandId, bySurface: { chat: true },
        bySurfaceId: { chat: { 'tab-blocked': false } }, fallback: false } },
    }];
    expect(resolveLocalDeckConditionCommand(conditions, context())).toBeNull();
    expect(resolveLocalDeckConditionCommand(conditions, context({ profile: 'other' }))).toBeNull();
    expect(resolveLocalDeckConditionCommand(conditions, context({ profile: 'dev' }))).toBe(commandId);
    expect(resolveLocalDeckConditionCommand(conditions, context({ profile: 'dev', surfaceId: 'tab-blocked' }))).toBeNull();
    expect(resolveLocalDeckConditionCommand(conditions, context({ profile: 'dev', surfaceType: 'editor' }))).toBeNull();
  });

  it('returns arguments only for the selected branch of the same local command', () => {
    const condition = {
      commandId: 'workspace.tab.go_to', bySurface: { chat: true, editor: true }, fallback: false,
      bySurfaceArguments: {
        chat: { workspace_id: 'w', target_mode: 'position', position: 3 },
        editor: { workspace_id: 'w', target_mode: 'specific', tab_id: 'editor-tab' },
      },
    };
    expect(resolveLocalDeckConditionSelectionFromParsed([condition], context()))
      .toEqual({ commandId: 'workspace.tab.go_to', arguments: { workspace_id: 'w', target_mode: 'position', position: 3 } });
    expect(resolveLocalDeckConditionSelectionFromParsed([condition], context({ surfaceType: 'editor' })))
      .toEqual({ commandId: 'workspace.tab.go_to', arguments: { workspace_id: 'w', target_mode: 'specific', tab_id: 'editor-tab' } });
  });
});
