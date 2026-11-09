import { restoreDefaultFocus } from '../hooks/useDefaultFocus';
import { isModalOpen } from './modalRegistry';

/** Complete a deferred page return only while it still owns the focus.
 * onComplete is notified even when a modal or later navigation superseded it.
 */
export function scheduleDefaultFocusRestore(onComplete?: () => void): void {
  const focusAtClose = document.activeElement;
  requestAnimationFrame(() => {
    const currentFocus = document.activeElement;
    if (!isModalOpen() && (currentFocus === focusAtClose ||
      currentFocus === document.body || currentFocus === document.documentElement ||
      !currentFocus?.isConnected)) {
      restoreDefaultFocus();
    }
    onComplete?.();
  });
}
