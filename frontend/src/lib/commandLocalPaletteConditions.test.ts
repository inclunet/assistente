import { describe, expect, it } from 'vitest';
import { createLocalPaletteConditionResolver, type LocalCommandPaletteVisualContext } from './commandLocalPaletteConditions';

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
});
