import { useEffect, useCallback, useRef } from 'react';
import { announce } from './useAnnouncer';
import { isModalOpen } from '../components/ui/Modal';

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
}

/**
 * Generic hook for F6 / Shift+F6 landmark navigation.
 *
 * Follows the Visual Studio / VS Code convention where F6 cycles forward
 * through major page regions and Shift+F6 cycles backward.
 *
 * Each page supplies its own ordered list of landmarks so the same hook
 * powers every page in the app.
 */
export function useLandmarkNavigation({
  landmarks,
  enabled = true,
}: UseLandmarkNavigationOptions) {
  const landmarksRef = useRef(landmarks);
  landmarksRef.current = landmarks;

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
          announce(target.label);
          return;
        }
      }
    };

    window.addEventListener('keydown', onKeyDown, true);
    return () => window.removeEventListener('keydown', onKeyDown, true);
  }, [enabled, getCurrentIndex, getAvailable]);
}
