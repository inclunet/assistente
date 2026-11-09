import { restoreDefaultFocus } from '../hooks/useDefaultFocus';
import { getTopmostModalID } from './modalRegistry';

/** Complete a deferred page return only while it still owns the focus.
 * onComplete is notified even when a modal or later navigation superseded it.
 */
export function scheduleDefaultFocusRestore({ onComplete, allowCurrentModal = false }: {
  onComplete?: () => void;
  allowCurrentModal?: boolean;
} = {}): void {
  const focusAtClose = document.activeElement;
  const modalAtClose = getTopmostModalID();
  requestAnimationFrame(() => {
    const currentFocus = document.activeElement;
    if ((modalAtClose === null || allowCurrentModal) && getTopmostModalID() === modalAtClose && (currentFocus === focusAtClose ||
      currentFocus === document.body || currentFocus === document.documentElement ||
      !currentFocus?.isConnected)) {
      restoreDefaultFocus();
    }
    onComplete?.();
  });
}
