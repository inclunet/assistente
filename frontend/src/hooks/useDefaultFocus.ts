/**
 * Global registry for the current page's "default focus area".
 *
 * Each page registers a focus function when it mounts (via useLandmarkNavigation
 * with `defaultLandmarkId`) and unregisters on unmount.  Any component — Modal,
 * Menu, toast, etc. — can call `restoreDefaultFocus()` to send focus back to the
 * page's primary interaction zone (e.g. the editor, the chat input, the data grid).
 */

let defaultFocusFn: (() => boolean) | null = null;

export function registerDefaultFocus(fn: () => boolean) {
  defaultFocusFn = fn;
}

export function unregisterDefaultFocus(fn: () => boolean) {
  if (defaultFocusFn === fn) {
    defaultFocusFn = null;
  }
}

/**
 * Attempts to restore focus to the current page's default area.
 * Returns true if focus was successfully restored.
 */
export function restoreDefaultFocus(): boolean {
  if (!defaultFocusFn) return false;
  return defaultFocusFn();
}
