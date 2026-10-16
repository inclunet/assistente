import { describe, expect, it } from 'vitest';
import { createLocalPaletteConditionResolver, parseLocalPaletteConditions, resolveLocalPaletteConditionSelectionFromParsed, type LocalCommandPaletteVisualContext } from './commandLocalPaletteConditions';

const context = (overrides: Partial<LocalCommandPaletteVisualContext> = {}): LocalCommandPaletteVisualContext => ({
  surfaceType: 'chat', surfaceId: 'tab-a', ...overrides,
});

describe('local palette conditions', () => {
  it('selects surface, surface id and listed profile branches', () => {
    const resolve = createLocalPaletteConditionResolver([{
      commandId: 'chat.message.edit.open',
      bySurface: { chat: false },
      bySurfaceId: { chat: { 'tab-a': true } },
      byProfile: { focused: { commandId: 'chat.message.edit.open', bySurface: { chat: true }, fallback: false } },
      fallback: false,
    }]);
    expect(resolve('chat.message.edit.open', context({ profile: 'other' }))).toBe(true);
    expect(resolve('chat.message.edit.open', context({ surfaceId: 'tab-b', profile: 'other' }))).toBe(false);
    expect(resolve('chat.message.edit.open', context({ profile: 'focused' }))).toBe(true);
  });

  it('fails closed when a profile provider is absent', () => {
    const resolve = createLocalPaletteConditionResolver([{
      commandId: 'workspace.open', bySurface: { chat: true }, byProfile: {}, fallback: true,
    }]);
    expect(resolve('workspace.open', context())).toBe(false);
  });

  it('uses the root only for a profile that is not listed', () => {
    const resolve = createLocalPaletteConditionResolver([{
      commandId: 'workspace.open', bySurface: { chat: true }, byProfile: { focused: { commandId: 'workspace.open', bySurface: { chat: false }, fallback: false } }, fallback: false,
    }]);
    expect(resolve('workspace.open', context({ profile: 'other' }))).toBe(true);
    expect(resolve('workspace.open', context({ profile: 'focused' }))).toBe(false);
  });

  it('resolves the application page as its own dimension and fails closed when missing or unknown', () => {
    const resolve = createLocalPaletteConditionResolver([{
      commandId: 'workspace.open', bySurface: {}, fallback: false,
      byPage: {
        settings: { commandId: 'workspace.open', bySurface: {}, fallback: true },
        workspace: { commandId: 'workspace.open', bySurface: { chat: false }, fallback: false },
      },
    }]);
    expect(resolve('workspace.open', context({ appPage: 'settings' }))).toBe(true);
    expect(resolve('workspace.open', context({ appPage: 'workspace' }))).toBe(false);
    expect(resolve('workspace.open', context())).toBe(false);
    expect(resolve('workspace.open', context({ appPage: 'not-a-page' }))).toBe(false);
  });

  it('resolves the approved page then profile tree without allowing profile nesting', () => {
    const resolve = createLocalPaletteConditionResolver([{
      commandId: 'tasklists.delete', bySurface: {}, fallback: false,
      byPage: {
        tasklists: {
          commandId: 'tasklists.delete', bySurface: {}, fallback: false,
          byProfile: {
            focused: { commandId: 'tasklists.delete', bySurface: { tasklists: true }, fallback: false },
          },
        },
        workspace: { commandId: 'tasklists.delete', bySurface: { tasklist: false }, fallback: false },
      },
    }]);
    expect(resolve('tasklists.delete', context({ appPage: 'tasklists', profile: 'focused', surfaceType: 'tasklists' }))).toBe(true);
    expect(resolve('tasklists.delete', context({ appPage: 'tasklists', profile: 'other', surfaceType: 'tasklists' }))).toBe(false);
    expect(resolve('tasklists.delete', context({ appPage: 'workspace', profile: 'focused', surfaceType: 'tasklists' }))).toBe(false);
    expect(resolve('tasklists.delete', context({ profile: 'focused', surfaceType: 'tasklists' }))).toBe(false);
  });

  it('rejects a malformed page branch instead of using the root fallback', () => {
    const resolve = createLocalPaletteConditionResolver([{
      commandId: 'workspace.open', bySurface: { chat: true }, fallback: true,
      byPage: { 'settings.commands': { commandId: 'workspace.open', bySurface: { chat: true }, fallback: true } },
    }]);
    expect(resolve('workspace.open', context({ appPage: 'settings' }))).toBe(false);
  });

  it('uses a profile-specific surface-id branch for an unlisted profile', () => {
    const resolve = createLocalPaletteConditionResolver([{
      commandId: 'workspace.open', bySurface: { chat: false }, bySurfaceId: { chat: { 'tab-a': true } },
      byProfile: { focused: { commandId: 'workspace.open', bySurface: { chat: false }, fallback: false } }, fallback: false,
    }]);
    expect(resolve('workspace.open', context({ profile: 'other' }))).toBe(true);
  });

  it('rejects malformed maps, non-booleans, duplicate ids and unsafe nested data', () => {
    expect(createLocalPaletteConditionResolver([{
      commandId: 'workspace.open', bySurface: { chat: 'yes' }, fallback: true,
    }])('workspace.open', context())).toBe(false);
    expect(createLocalPaletteConditionResolver([{
      commandId: 'workspace.open', bySurface: { chat: true }, byProfile: { focused: null }, fallback: true,
    }])('workspace.open', context({ profile: 'focused' }))).toBe(false);
    expect(createLocalPaletteConditionResolver([
      { commandId: 'workspace.open', bySurface: {}, fallback: true },
      { commandId: 'workspace.open', bySurface: { chat: true }, fallback: true },
    ])('workspace.open', context())).toBe(false);
    expect(createLocalPaletteConditionResolver([{
      commandId: 'workspace.open', bySurface: { constructor: true }, fallback: true,
    }])('workspace.open', context())).toBe(false);
  });

  it('does not infer a surface or accept an unknown command', () => {
    const resolve = createLocalPaletteConditionResolver([{
      commandId: 'workspace.open', bySurface: { chat: true }, fallback: false,
    }]);
    expect(resolve('workspace.open', null)).toBe(false);
    expect(resolve('workspace.open', context({ surfaceType: '' }))).toBe(false);
    expect(resolve('unknown.command', context())).toBe(false);
  });

  it('detaches the resolver from later payload mutation', () => {
    const payload = [{ commandId: 'workspace.open', bySurface: { chat: true }, fallback: false }];
    const resolve = createLocalPaletteConditionResolver(payload);
    payload[0].bySurface.chat = false;
    expect(resolve('workspace.open', context())).toBe(true);
  });

  it('rejects a nested profile projection beyond the contract level', () => {
    expect(createLocalPaletteConditionResolver([{
      commandId: 'workspace.open', bySurface: { chat: true },
      byProfile: { focused: { commandId: 'workspace.open', bySurface: { chat: true }, byProfile: {}, fallback: true } }, fallback: false,
    }])('workspace.open', context({ profile: 'focused' }))).toBe(false);
  });

  it('rejects a nested projection whose command id differs from the root', () => {
    expect(createLocalPaletteConditionResolver([{
      commandId: 'workspace.open', bySurface: { chat: true },
      byProfile: { focused: { commandId: 'chat.open', bySurface: { chat: true }, fallback: true } }, fallback: false,
    }])('workspace.open', context({ profile: 'focused' }))).toBe(false);
  });

  it('preserves the selected go-to arguments for distinct surface and profile branches', () => {
    const conditions = parseLocalPaletteConditions([{
      commandId: 'workspace.tab.go_to', bySurface: { chat: true, editor: true }, fallback: false,
      bySurfaceArguments: {
        chat: { workspace_id: 'w', target_mode: 'position', position: 4 },
        editor: { workspace_id: 'w', target_mode: 'specific', tab_id: 'tab-editor' },
      },
      byProfile: { focused: { commandId: 'workspace.tab.go_to', bySurface: {}, fallback: true,
        fallbackArguments: { workspace_id: 'w', target_mode: 'specific', tab_id: 'tab-focused' } } },
    }]);
    expect(conditions).not.toBeNull();
    expect(resolveLocalPaletteConditionSelectionFromParsed(conditions!, 'workspace.tab.go_to', context({ surfaceType: 'chat', profile: 'other' })))
      .toEqual({ available: true, arguments: { workspace_id: 'w', target_mode: 'position', position: 4 } });
    expect(resolveLocalPaletteConditionSelectionFromParsed(conditions!, 'workspace.tab.go_to', context({ surfaceType: 'editor', profile: 'other' })))
      .toEqual({ available: true, arguments: { workspace_id: 'w', target_mode: 'specific', tab_id: 'tab-editor' } });
    expect(resolveLocalPaletteConditionSelectionFromParsed(conditions!, 'workspace.tab.go_to', context({ surfaceType: 'chat', profile: 'focused' })))
      .toEqual({ available: true, arguments: { workspace_id: 'w', target_mode: 'specific', tab_id: 'tab-focused' } });
    expect(parseLocalPaletteConditions([{ commandId: 'workspace.open', bySurface: { chat: true }, fallback: false,
      bySurfaceArguments: { chat: { workspace_id: 'w', target_mode: 'position', position: 1 } } }])).toBeNull();
  });

  it('preserves go-to arguments through appPage and profile branches with detached copies', () => {
    const source = [{
      commandId: 'workspace.tab.go_to', bySurface: {}, fallback: false,
      byPage: {
        settings: {
          commandId: 'workspace.tab.go_to', bySurface: { chat: true }, fallback: false,
          bySurfaceArguments: { chat: { workspace_id: 'w', target_mode: 'position', position: 3 } },
          byProfile: { focused: {
            commandId: 'workspace.tab.go_to', bySurface: { chat: true }, fallback: false,
            bySurfaceArguments: { chat: { workspace_id: 'w', target_mode: 'specific', tab_id: 'tab-settings' } },
          } },
        },
      },
    }];
    const parsed = parseLocalPaletteConditions(source);
    expect(parsed).not.toBeNull();
    const defaultBranch = resolveLocalPaletteConditionSelectionFromParsed(parsed!, 'workspace.tab.go_to',
      context({ appPage: 'settings', profile: 'other' }));
    const profileBranch = resolveLocalPaletteConditionSelectionFromParsed(parsed!, 'workspace.tab.go_to',
      context({ appPage: 'settings', profile: 'focused' }));
    expect(defaultBranch).toEqual({ available: true, arguments: { workspace_id: 'w', target_mode: 'position', position: 3 } });
    expect(profileBranch).toEqual({ available: true, arguments: { workspace_id: 'w', target_mode: 'specific', tab_id: 'tab-settings' } });
    expect(resolveLocalPaletteConditionSelectionFromParsed(parsed!, 'workspace.tab.go_to', context({ appPage: 'workspace' })))
      .toEqual({ available: false });
    expect(resolveLocalPaletteConditionSelectionFromParsed(parsed!, 'workspace.tab.go_to', context({ appPage: 'unknown' }))).toBeNull();

    source[0].byPage.settings.bySurfaceArguments.chat.position = 99;
    expect(defaultBranch?.arguments).toEqual({ workspace_id: 'w', target_mode: 'position', position: 3 });
    expect(Object.isFrozen(defaultBranch?.arguments)).toBe(true);
  });
});
