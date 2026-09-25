import { useEffect, useLayoutEffect, useCallback, useRef, useId, createContext, useContext } from 'react';
import { UNSAFE_LocationContext } from 'react-router-dom';
import { registerDefaultFocus, unregisterDefaultFocus } from './useDefaultFocus';
import { captureLandmarkNavigationTarget, registerLandmarkNavigationSurface, requestLandmarkNavigationCommand, type NavigationLandmark } from '../lib/commandLandmarkNavigation';

const ParentLandmarkContext = createContext(false);
export const ParentLandmarkProvider = ParentLandmarkContext.Provider;
export const useHasParentLandmarks = () => useContext(ParentLandmarkContext);
export type Landmark = NavigationLandmark;
export interface UseLandmarkNavigationOptions {
  landmarks: Landmark[];
  enabled?: boolean;
  allowWhenModalOpen?: boolean;
  shouldHandleKey?: () => boolean;
  defaultLandmarkId?: string;
}

export function useLandmarkNavigation({ landmarks, enabled = true, allowWhenModalOpen = false, shouldHandleKey, defaultLandmarkId }: UseLandmarkNavigationOptions) {
  const instanceId = useId();
  const location = useContext(UNSAFE_LocationContext);
  const pathname = location?.location.pathname ?? window.location.pathname;
  const live = useRef({ landmarks, enabled, allowWhenModalOpen, shouldHandleKey, defaultLandmarkId, pathname });
  live.current = { landmarks, enabled, allowWhenModalOpen, shouldHandleKey, defaultLandmarkId, pathname };
  const listeners = useRef(new Set<() => void>());
  useLayoutEffect(() => { listeners.current.forEach(changed => changed()); });
  useLayoutEffect(() => registerLandmarkNavigationSurface({
    instanceId,
    read: () => live.current,
    subscribe(changed) { listeners.current.add(changed); return () => { listeners.current.delete(changed); }; },
  }), [instanceId]);

  // Modal/Menu restoration is a native callback, not a command ingress.
  const focusDefault = useCallback(() => {
    const state = live.current;
    const landmark = state.landmarks.find(l => l.id === state.defaultLandmarkId);
    return state.enabled && !!landmark && landmark.isAvailable?.() !== false && landmark.focus();
  }, []);
  useEffect(() => {
    if (!enabled || !defaultLandmarkId) return;
    registerDefaultFocus(focusDefault);
    return () => unregisterDefaultFocus(focusDefault);
  }, [enabled, defaultLandmarkId, focusDefault]);

  // F6 belongs only to the central adapter. Escape bubbles after controls.
  useEffect(() => {
    const onEscape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || event.defaultPrevented || event.repeat || event.isComposing || event.keyCode === 229 ||
        event.ctrlKey || event.altKey || event.shiftKey || event.metaKey || event.getModifierState('AltGraph')) return;
      const state = live.current;
      if (!state.enabled || !state.defaultLandmarkId) return;
      const current = state.landmarks.find(l => l.contains());
      if (!current || current.id === state.defaultLandmarkId) return;
      const lease = captureLandmarkNavigationTarget(() => live.current.pathname, instanceId);
      const eligible = lease?.canOpen('navigation.landmark.default');
      lease?.dispose();
      if (!eligible) return;
      if (requestLandmarkNavigationCommand('navigation.landmark.default', instanceId)) {
        event.preventDefault();
        event.stopPropagation();
      }
    };
    window.addEventListener('keydown', onEscape);
    return () => window.removeEventListener('keydown', onEscape);
  }, [instanceId]);
}
