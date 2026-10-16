export interface CommandPalettePreferences {
  readonly favorites: readonly string[];
  readonly recent: readonly string[];
}

export interface CommandPalettePreferenceStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}

export const EMPTY_COMMAND_PALETTE_PREFERENCES: CommandPalettePreferences = Object.freeze({
  favorites: Object.freeze([]),
  recent: Object.freeze([]),
});

const MAX_FAVORITES = 200;
const MAX_RECENT = 20;

export function commandPalettePreferenceKey(userId: string, workspaceId: string): string | null {
  if (!userId || !workspaceId) return null;
  return `assistente.command-palette.v1.${encodeURIComponent(userId)}.${encodeURIComponent(workspaceId)}`;
}

function cleanIds(value: unknown, limit: number): string[] {
  if (!Array.isArray(value)) return [];
  const seen = new Set<string>();
  const result: string[] = [];
  for (const candidate of value) {
    if (typeof candidate !== 'string' || !/^[a-z][a-z0-9_]*(?:\.[a-z][a-z0-9_]*)+$/.test(candidate) || seen.has(candidate)) continue;
    seen.add(candidate);
    result.push(candidate);
    if (result.length >= limit) break;
  }
  return result;
}

export function readCommandPalettePreferences(
  key: string | null,
  storage?: CommandPalettePreferenceStorage,
): CommandPalettePreferences {
  if (!key) return EMPTY_COMMAND_PALETTE_PREFERENCES;
  try {
    const target = storage ?? globalThis.localStorage;
    const parsed: unknown = JSON.parse(target.getItem(key) ?? 'null');
    if (!parsed || typeof parsed !== 'object') return EMPTY_COMMAND_PALETTE_PREFERENCES;
    const value = parsed as { favorites?: unknown; recent?: unknown };
    return { favorites: cleanIds(value.favorites, MAX_FAVORITES), recent: cleanIds(value.recent, MAX_RECENT) };
  } catch {
    return EMPTY_COMMAND_PALETTE_PREFERENCES;
  }
}

export function writeCommandPalettePreferences(
  key: string | null,
  value: CommandPalettePreferences,
  storage?: CommandPalettePreferenceStorage,
): void {
  if (!key) return;
  try {
    (storage ?? globalThis.localStorage).setItem(key, JSON.stringify({
      favorites: cleanIds(value.favorites, MAX_FAVORITES),
      recent: cleanIds(value.recent, MAX_RECENT),
    }));
  } catch {
    // Preferences are optional; unavailable storage must not block commands.
  }
}

export function toggleCommandFavorite(
  preferences: CommandPalettePreferences,
  commandId: string,
): CommandPalettePreferences {
  const favorites = preferences.favorites.includes(commandId)
    ? preferences.favorites.filter((id) => id !== commandId)
    : [commandId, ...preferences.favorites].slice(0, MAX_FAVORITES);
  return { ...preferences, favorites };
}

export function recordRecentCommand(
  preferences: CommandPalettePreferences,
  commandId: string,
): CommandPalettePreferences {
  return { ...preferences, recent: [commandId, ...preferences.recent.filter((id) => id !== commandId)].slice(0, MAX_RECENT) };
}
