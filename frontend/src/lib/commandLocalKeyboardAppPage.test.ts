import { describe, expect, it, vi } from 'vitest';
import { createLocalCommandKeyboard, type LocalCommandContextLease, type LocalCommandKeyboardMap } from './commandLocalKeyboard';
import type { CommandShortcut } from './commandShortcut';

describe('keyboard application-page and profile projection', () => {
  it.each(['valid', 'missing-page', 'wrong-page', 'wrong-profile', 'stale-lease'] as const)(
    'validates and dispatches the route-level toolbar projection: %s', async mode => {
      const shortcut: CommandShortcut = { version: 1, code: 'KeyK', modifiers: ['Control'] };
      const binding = { shortcut, commandId: 'navigation.history.open', handler: 'local_ui' as const };
      const map: LocalCommandKeyboardMap = {
        generation: 'page-profile-map', bindings: [],
        contextualBindings: [{
          shortcut, bySurface: {}, fallback: null,
          byPage: { settings: {
            shortcut, bySurface: { toolbar: null }, fallback: null,
            byProfile: { focused: { shortcut, bySurface: { toolbar: binding }, fallback: null } },
          } },
        }],
      };
      const readContext = (): LocalCommandContextLease => ({
        surfaceId: 'command-toolbar', surfaceType: 'toolbar',
        ...(mode === 'missing-page' ? {} : { appPage: mode === 'wrong-page' ? 'workspace' as const : 'settings' as const }),
        profile: mode === 'wrong-profile' ? 'other' : 'focused',
        isCurrent: () => mode !== 'stale-lease',
      });
      const onDown = vi.fn(async () => {});
      const keyboard = createLocalCommandKeyboard({
        target: window, loadMap: async () => map,
        onDown, onUp: vi.fn(async () => {}), reset: vi.fn(async () => {}),
        blocked: () => false, readSurfaceType: () => 'toolbar', readContext,
      });
      try {
        await keyboard.refresh();
        window.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', code: 'KeyK', ctrlKey: true, cancelable: true }));
        if (mode === 'valid') {
          expect(onDown).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({
            commandId: 'navigation.history.open', handler: 'local_ui',
          }));
        } else {
          expect(onDown).not.toHaveBeenCalled();
        }
      } finally { keyboard.dispose(); }
    },
  );
});
