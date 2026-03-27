import { useEffect, useCallback, useRef, createContext, useContext } from 'react';
import { registerDefaultFocus, unregisterDefaultFocus } from './useDefaultFocus';
import { isModalOpen } from '../components/ui/Modal';

/**
 * When a parent component (e.g. SettingsPage) manages landmark navigation
 * for its children, it wraps them in this provider so child hooks like
 * useGridPageLandmarks automatically disable themselves.
 */
const ParentLandmarkContext = createContext(false);
export const ParentLandmarkProvider = ParentLandmarkContext.Provider;
export const useHasParentLandmarks = () => useContext(ParentLandmarkContext);

export interface Landmark {
  /** Unique id for the zone (e.g. 'tabs', 'toolbar', 'grid', 'editor') */
  id: string;
  /** Human-readable label announced to screen readers when entering the zone */
  label: string;
  /** Returns true when the landmark is currently available for focus */
  isAvailable?: () => boolean;
  /** Attempts to focus the landmark. Returns true on success. */
  focus: () => boolean;
  /** Returns true when the currently active element belongs to this landmark. */
  contains: () => boolean;
}

export interface UseLandmarkNavigationOptions {
  /** Ordered list of landmarks for this page (top-to-bottom visual order). */
  landmarks: Landmark[];
  /** Whether the hook is active. Defaults to true. */
  enabled?: boolean;
  /**
   * Id of the landmark that acts as the page's default focus area.
   * Pressing Escape from any other landmark sends focus here.
   * Also registered globally so Modal/Menu can call restoreDefaultFocus().
   */
  defaultLandmarkId?: string;
}

/**
 * Generic hook for F6 / Shift+F6 landmark navigation + Escape-to-default.
 *
 * Follows the Visual Studio / VS Code convention where F6 cycles forward
 * through major page regions and Shift+F6 cycles backward.
 * Escape from any non-default landmark returns focus to the default area.
 *
 * Each page supplies its own ordered list of landmarks so the same hook
 * powers every page in the app.
 */
export function useLandmarkNavigation({
  landmarks,
  enabled = true,
  defaultLandmarkId,
}: UseLandmarkNavigationOptions) {
  const landmarksRef = useRef(landmarks);
  landmarksRef.current = landmarks;

  const defaultIdRef = useRef(defaultLandmarkId);
  defaultIdRef.current = defaultLandmarkId;

  const getCurrentIndex = useCallback((): number => {
    const lms = landmarksRef.current;
    for (let i = 0; i < lms.length; i++) {
      if (lms[i].contains()) return i;
    }
    return -1;
  }, []);

  const getAvailable = useCallback((): Landmark[] => {
    return landmarksRef.current.filter((l) => !l.isAvailable || l.isAvailable());
  }, []);

  const focusDefault = useCallback((): boolean => {
    const id = defaultIdRef.current;
    if (!id) return false;
    const lm = landmarksRef.current.find((l) => l.id === id);
    if (!lm) return false;
    if (lm.isAvailable && !lm.isAvailable()) return false;
    return lm.focus();
  }, []);

  // Register the default focus function globally so Modal/Menu/etc. can use it
  useEffect(() => {
    if (!enabled || !defaultLandmarkId) return;
    registerDefaultFocus(focusDefault);
    return () => unregisterDefaultFocus(focusDefault);
  }, [enabled, defaultLandmarkId, focusDefault]);

  // F6 / Shift+F6: cycle between landmarks (capture phase)
  useEffect(() => {
    if (!enabled) return;

    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key !== 'F6') return;
      if (isModalOpen()) return;

      e.preventDefault();
      e.stopPropagation();

      const available = getAvailable();
      if (available.length === 0) return;

      const currentIdx = getCurrentIndex();
      const currentLandmark = currentIdx >= 0 ? landmarksRef.current[currentIdx] : null;

      const availIdx = currentLandmark
        ? available.findIndex((l) => l.id === currentLandmark.id)
        : -1;

      const dir = e.shiftKey ? -1 : 1;
      const startIdx = availIdx >= 0 ? availIdx : (dir === 1 ? -1 : 0);
      const len = available.length;

      for (let attempt = 0; attempt < len; attempt++) {
        const nextIdx = ((startIdx + dir * (attempt + 1)) % len + len) % len;
        const target = available[nextIdx];
        if (target.focus()) {
          return;
        }
      }
    };

    window.addEventListener('keydown', onKeyDown, true);
    return () => window.removeEventListener('keydown', onKeyDown, true);
  }, [enabled, getCurrentIndex, getAvailable]);

  // Escape: return to default landmark (bubbling phase — runs after
  // component-level handlers like MessageNode that may stopPropagation)
  useEffect(() => {
    if (!enabled || !defaultLandmarkId) return;

    const onEscape = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return;
      if (isModalOpen()) return;

      const defaultLm = landmarksRef.current.find((l) => l.id === defaultIdRef.current);
      if (!defaultLm) return;

      // Already in the default area — nothing to do
      if (defaultLm.contains()) return;

      // Only act when focus is inside a known landmark
      const currentIdx = getCurrentIndex();
      if (currentIdx < 0) return;

      e.preventDefault();
      e.stopPropagation();

      defaultLm.focus();
    };

    // Bubbling phase on window — fires after React handlers and document handlers
    window.addEventListener('keydown', onEscape);
    return () => window.removeEventListener('keydown', onEscape);
  }, [enabled, defaultLandmarkId, getCurrentIndex]);
}
