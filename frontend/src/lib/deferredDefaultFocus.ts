import { restoreDefaultFocus } from '../hooks/useDefaultFocus';
import { getTopmostModalID } from './modalRegistry';

/** Complete a deferred page return only while it still owns the focus.
 * onComplete is notified even when a modal or later navigation superseded it.
 */
export function scheduleDefaultFocusRestore({ onComplete, allowCurrentModal = false, after, isCurrent }: {
  onComplete?: () => void;
  allowCurrentModal?: boolean;
  /** Wait for an asynchronous selection to apply its destination. False cancels. */
  after?: Promise<boolean>;
  isCurrent?: () => boolean;
} = {}): void {
  const focusAtClose = document.activeElement;
  const modalAtClose = getTopmostModalID();
  const schedule = () => requestAnimationFrame(() => {
    const currentFocus = document.activeElement;
    if ((!isCurrent || isCurrent()) && (modalAtClose === null || allowCurrentModal) && getTopmostModalID() === modalAtClose && (currentFocus === focusAtClose ||
      currentFocus === document.body || currentFocus === document.documentElement ||
      !currentFocus?.isConnected)) {
      restoreDefaultFocus();
    }
    onComplete?.();
  });
  if (after) {
    void after.then(ready => { if (ready) schedule(); else onComplete?.(); }, () => onComplete?.());
  } else schedule();
}
