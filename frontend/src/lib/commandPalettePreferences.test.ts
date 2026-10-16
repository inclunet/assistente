import { describe, expect, it } from 'vitest';
import {
  commandPalettePreferenceKey,
  readCommandPalettePreferences,
  recordRecentCommand,
  toggleCommandFavorite,
  writeCommandPalettePreferences,
  type CommandPalettePreferences,
} from './commandPalettePreferences';

function memoryStorage(): { getItem(key: string): string | null; setItem(key: string, value: string): void } {
  const values = new Map<string, string>();
  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => { values.set(key, value); },
  };
}

describe('Command Palette preferences', () => {
  it('namespaces saved IDs by user and workspace', () => {
    expect(commandPalettePreferenceKey('user/a', 'workspace 1')).not.toBe(
      commandPalettePreferenceKey('user/a', 'workspace 2'),
    );
    expect(commandPalettePreferenceKey('', 'workspace')).toBeNull();
  });

  it('round-trips favorite and recent IDs without storing payloads', () => {
    const storage = memoryStorage();
    const key = commandPalettePreferenceKey('user-1', 'workspace-1');
    const favorites = toggleCommandFavorite({ favorites: [], recent: [] }, 'workspace.tab.new');
    const next = recordRecentCommand(recordRecentCommand(favorites, 'workspace.tab.new'), 'workspace.tab.close');
    writeCommandPalettePreferences(key, next, storage);

    expect(readCommandPalettePreferences(key, storage)).toEqual({
      favorites: ['workspace.tab.new'],
      recent: ['workspace.tab.close', 'workspace.tab.new'],
    });
    expect(storage.getItem(key!)).not.toContain('arguments');
  });

  it('deduplicates, removes a favorite, bounds recent entries, and ignores malformed storage', () => {
    let preferences: CommandPalettePreferences = { favorites: [], recent: [] };
    preferences = toggleCommandFavorite(preferences, 'workspace.tab.new');
    preferences = toggleCommandFavorite(preferences, 'workspace.tab.new');
    for (let index = 0; index < 24; index += 1) {
      preferences = recordRecentCommand(preferences, `workspace.command_${index}`);
    }
    expect(preferences.favorites).toEqual([]);
    expect(preferences.recent).toHaveLength(20);

    const storage = memoryStorage();
    storage.setItem('bad', '{');
    expect(readCommandPalettePreferences('bad', storage)).toEqual({ favorites: [], recent: [] });
    storage.setItem('bad', JSON.stringify({ favorites: ['../../secret'], recent: ['tool.run'] }));
    expect(readCommandPalettePreferences('bad', storage)).toEqual({ favorites: [], recent: ['tool.run'] });
  });
});
