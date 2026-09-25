import { ReadFocusContext } from './commandContextProviders';
import { getModalRegistrySnapshot } from './modalRegistry';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore } from '../store/workspaceStore';

export const LANDMARK_COMMAND_IDS = ['navigation.landmark.next', 'navigation.landmark.previous', 'navigation.landmark.default'] as const;
export type LandmarkNavigationCommandID = typeof LANDMARK_COMMAND_IDS[number];
export const LANDMARK_COMMAND_EVENT = 'commands:landmark-navigation';
export interface LandmarkCommandRequest { commandID: LandmarkNavigationCommandID; instanceId: string }
export function isLandmarkNavigationCommand(id: string): id is LandmarkNavigationCommandID {
  return (LANDMARK_COMMAND_IDS as readonly string[]).includes(id);
}
export function requestLandmarkNavigationCommand(commandID: LandmarkNavigationCommandID, instanceId: string): boolean {
  if (!isLandmarkNavigationCommand(commandID) || !instanceId) return false;
  const event = new CustomEvent<LandmarkCommandRequest>(LANDMARK_COMMAND_EVENT, { detail: { commandID, instanceId }, cancelable: true });
  window.dispatchEvent(event);
  return event.defaultPrevented;
}
export interface NavigationLandmark {
  id: string;
  label: string;
  contains(): boolean;
  focus(): boolean;
  isAvailable?(): boolean;
}
export interface LandmarkNavigationSnapshot {
  landmarks: readonly NavigationLandmark[];
  enabled: boolean;
  defaultLandmarkId?: string;
  allowWhenModalOpen: boolean;
  shouldHandleKey?: () => boolean;
  pathname: string;
}
export interface LandmarkNavigationSurface {
  instanceId: string;
  read(): LandmarkNavigationSnapshot;
  subscribe(changed: () => void): () => void;
}
export interface LandmarkNavigationTarget {
  readonly instanceId: string;
  isCurrent(): boolean;
  canOpen(id: string, target?: EventTarget | null): boolean;
  open(id: string): boolean;
  dispose(): void;
}
const surfaces = new Map<string, { source: LandmarkNavigationSurface; leases: Set<() => void> }>();
export function registerLandmarkNavigationSurface(source: LandmarkNavigationSurface): () => void {
  // A new owner must not revive a previously unambiguous capture.
  surfaces.forEach(record => record.leases.forEach(dispose => dispose()));
  const record = { source, leases: new Set<() => void>() };
  surfaces.set(source.instanceId, record);
  return () => {
    record.leases.forEach(dispose => dispose());
    if (surfaces.get(source.instanceId) === record) surfaces.delete(source.instanceId);
  };
}
function contextKey(): string {
  const auth = useAuthStore.getState();
  const ws = useWorkspaceStore.getState().workspace;
  return JSON.stringify([auth.isAuthenticated, auth.user?.userId, auth.user?.sessionId, ws?.id, ws?.activeTabId]);
}
function contextReady(): boolean {
  const auth = useAuthStore.getState();
  const ws = useWorkspaceStore.getState().workspace;
  return auth.isAuthenticated && !!auth.user?.userId && !!auth.user.sessionId && !!ws?.id &&
    !!ws.activeTabId && ws.tabs.some(tab => tab.id === ws.activeTabId);
}
function usable(snapshot: LandmarkNavigationSnapshot): boolean {
  return snapshot.enabled && snapshot.shouldHandleKey?.() !== false && snapshot.landmarks.length > 0 &&
    new Set(snapshot.landmarks.map(l => l.id)).size === snapshot.landmarks.length;
}
function focusedModal(): string | null {
  return document.activeElement?.closest('[data-modal-id]')?.getAttribute('data-modal-id') ?? null;
}
function menuOwnsFocus(): boolean {
  return !!document.activeElement?.closest('[role="menu"],[role="listbox"]');
}
export function captureLandmarkNavigationTarget(readPathname: () => string, instanceId?: string): LandmarkNavigationTarget | undefined {
  let release: (() => void) | undefined;
  try {
    if (!contextReady() || !document.hasFocus() || ReadFocusContext().composition === 'active' || menuOwnsFocus()) return;
    const modal = getModalRegistrySnapshot();
    const modalElement = modal.topID ? document.activeElement?.closest('[data-modal-id]') : null;
    const path = readPathname();
    const candidates = [...surfaces.values()].filter(({ source }) => {
      const s = source.read();
      if (!usable(s) || s.pathname !== path) return false;
      return modal.topID ? s.allowWhenModalOpen && !!s.shouldHandleKey && focusedModal() === modal.topID && s.landmarks.some(l => l.contains()) : true;
    });
    const containing = candidates.filter(({ source }) => source.read().landmarks.some(l => l.contains()));
    const eligible = containing.length ? containing : candidates;
    if (eligible.length !== 1) return;
    const record = eligible[0];
    const source = record.source;
    if (instanceId !== undefined && instanceId !== source.instanceId) return;
    const snapshot = { ...source.read() };
    const landmarks = snapshot.landmarks.map(l => ({ ...l }));
    const origin = landmarks.findIndex(l => l.contains());
    const context = contextKey();
    let disposed = false;
    const cleanups: (() => void)[] = [];
    const dispose = () => {
      if (disposed) return;
      disposed = true;
      record.leases.delete(dispose);
      cleanups.splice(0).forEach(cleanup => cleanup());
    };
    release = dispose;
    const isCurrent = (): boolean => {
      if (disposed) return false;
      try {
        const s = source.read();
        const valid = surfaces.get(source.instanceId) === record && contextReady() && contextKey() === context &&
          readPathname() === path && s.pathname === path && usable(s) &&
          s.allowWhenModalOpen === snapshot.allowWhenModalOpen && s.defaultLandmarkId === snapshot.defaultLandmarkId &&
          s.landmarks.length === landmarks.length && s.landmarks.every((l, i) => l.id === landmarks[i].id) &&
          getModalRegistrySnapshot().generation === modal.generation && document.hasFocus() &&
          (!modal.topID || (modalElement?.isConnected === true && modalElement.getAttribute('data-modal-id') === modal.topID &&
            s.allowWhenModalOpen && !!s.shouldHandleKey && s.shouldHandleKey())) &&
          ReadFocusContext().composition !== 'active';
        if (!valid) dispose();
        return valid;
      } catch { dispose(); return false; }
    };
    const changed = () => { isCurrent(); };
    record.leases.add(dispose);
    cleanups.push(source.subscribe(changed));
    cleanups.push(useAuthStore.subscribe(changed));
    cleanups.push(useWorkspaceStore.subscribe(changed));
    const invalidate = () => dispose();
    window.addEventListener('blur', invalidate);
    document.addEventListener('compositionstart', invalidate, true);
    cleanups.push(() => window.removeEventListener('blur', invalidate), () => document.removeEventListener('compositionstart', invalidate, true));
    const ordered = (id: string) => {
      if (id === 'navigation.landmark.default') return landmarks.filter(l => l.id === snapshot.defaultLandmarkId);
      const direction = id === 'navigation.landmark.previous' ? -1 : 1;
      const start = origin >= 0 ? origin : direction === 1 ? -1 : 0;
      return landmarks.map((_, i) => landmarks[((start + direction * (i + 1)) % landmarks.length + landmarks.length) % landmarks.length]);
    };
    const canOpen = (id: string) => {
      try { return isLandmarkNavigationCommand(id) && isCurrent() && ordered(id).some(l => !l.isAvailable || l.isAvailable()); }
      catch { dispose(); return false; }
    };
    if (!isCurrent()) return;
    return { instanceId: source.instanceId, isCurrent, canOpen, dispose, open(id) {
      if (!canOpen(id) || menuOwnsFocus() || (modal.topID && focusedModal() !== modal.topID)) return false;
      const targets = ordered(id);
      for (const landmark of targets) {
        if (!isCurrent()) return false;
        try {
          if (landmark.isAvailable?.() === false) continue;
          if (landmark.focus()) { dispose(); return true; }
        } catch { dispose(); return false; }
      }
      dispose();
      return false;
    } };
  } catch { release?.(); return undefined; }
}
